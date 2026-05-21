# cloud-boot/uki

Host-side toolchain that assembles **bootable UEFI ISOs** from the
sibling [`cloud-boot/init`](../init) and [`cloud-boot/kernel`](../kernel)
artifacts, and pushes those artifacts as OCI images for the init to
pull at boot.

One CLI, three top-level cobra subcommands:

| Subcommand            | Role |
| --------------------- | ---- |
| `cloud-boot build`    | Cross-compiles `cloud-boot-init` for the target arch, builds an initramfs (cpio.gz), assembles a UKI (`.linux` + `.initrd` + `.cmdline` + …) via [`github.com/go-coff/pe`](https://github.com/go-coff/pe), stages a FAT32 ESP image, and writes a hybrid GPT + El Torito ISO whose appended GPT partition 2 (type EFI System Partition) is byte-identical to the ESP — so the in-VM Linux can mount it r/w and the menu-then-reboot sink can write `\EFI\Linux\*` for the next firmware boot. |
| `cloud-boot push`     | Pushes single-arch artifacts (`push artifact`), HCL boot plans (`push plan`), multi-arch image indexes (`push index`), and packs `/lib/modules` trees into `modules.cpio.gz` (`push modpack`). |
| `cloud-boot label`    | Reads or writes the ext4 volume label of a disk image (raw, QCOW2, or UDIF-UDRW DMG). Used by `task label:debian:cloud` to stamp `LABEL=cloudimg-rootfs` on the Debian cloud image before booting it through a `device = "LABEL=…"` disk-mode plan target. Format detection + in-place writes (incl. UDRW koly checksum refresh) come from `github.com/go-diskimages/diskimage`. |

## Layout

| Path                              | Role |
| --------------------------------- | ---- |
| `cmd/cloud-boot`                  | The host `cloud-boot` cobra CLI (build + push + label subcommands) |
| `internal/uki`                    | UKI assembly helper (wraps `pe.Append`) |
| `examples/plan.hcl`               | Sample HCL boot plan (production-shaped) |
| `examples/plan-qemu-test.hcl`     | QEMU NAT test plan (host = `10.0.2.2:5000`) |
| `examples/plan-vz-test.hcl`       | Apple `Virtualization.framework` test plan (host = `192.168.64.1:5000`) |
| `examples/cloud-init/`            | NoCloud `user-data` / `meta-data` consumed by `task cidata` |
| `scripts/test-qemu.sh`            | End-to-end smoke test driver |

Shared infrastructure (`pkg/cpio`, `pkg/oci`) is imported from
[`github.com/cloud-boot/init`](../init). The PE/COFF section appender
comes from [`github.com/go-coff/pe`](https://github.com/go-coff/pe).
The label subcommand consumes
[`github.com/go-diskimages/diskimage`](https://github.com/go-diskimages/diskimage).

## Build

```sh
task build           # → bin/cloud-boot
```

## Assemble a UKI ISO

```sh
task iso \
    ARCH=amd64 \
    KERNEL=../kernel/bzImage-disk \
    STUB=../../go-coff/stub/BOOTX64.EFI \
    PLAN_REF=127.0.0.1:5000/boot/plan:latest \
    INSECURE=1
```

## Push artifacts

```sh
task registry                                          # local OCI on :5000
task push:plan PLAN=examples/plan.hcl \
               PLAN_REF=127.0.0.1:5000/boot/plan:latest
```

See `task -l` for the full task list.

## Returning to the cloud-boot menu after a target was staged

Once you've picked a target in the cloud-boot menu, the reboot sink
writes a UEFI `Boot0001` entry pointing at
`\EFI\Linux\<target>-vmlinuz.efi` on the cache disk and prepends
`0001` to `BootOrder`. Every subsequent firmware boot of the VM
honours that — you don't see the cloud-boot menu again until those
NVRAM entries are cleared.

Under vfkit you can just `rm /tmp/vz-test/vfkit/menu-vars.fd` and
re-launch with `,create` to wipe the var-store file. Under
hypervisors that persist NVRAM (OpenStack/libvirt with a per-instance
`<nvram>` file, bare metal, …) you need an in-band reset.

`uki/scripts/reset-cloud-boot.sh` runs from inside the staged distro
and clears `Boot0001` + `BootOrder` via `efibootmgr` (or directly
via `efivarfs` if efibootmgr isn't installed). Next reboot falls
back to the boot media's default loader (`\EFI\BOOT\BOOTAA64.EFI`
on arm64 = cloud-boot's UKI).

```sh
# inside the running Alpine / Debian / Ubuntu / …
sudo /path/to/reset-cloud-boot.sh --reboot
# (or omit --reboot to clear vars now and reboot manually later)
```

If you don't ship the script with your staged distro, the one-liner
equivalent is:

```sh
sudo efibootmgr -b 0001 -B   # delete Boot0001 + remove from BootOrder
sudo reboot
```

### OpenStack-specific notes

In OpenStack/libvirt each VM has its NVRAM in a per-instance
`<nvram>` file (default `/var/lib/libvirt/qemu/nvram/<id>_VARS.fd`).
The in-band script above is the only path that works without
compute-node admin access.

If you DO have admin access on the compute node and want to wipe
the NVRAM without booting the VM:

```sh
# Stop the instance via the dashboard / openstack server stop
sudo virsh undefine <instance-id> --keep-nvram=no \
                                  --nvram-template /usr/share/OVMF/AAVMF_VARS.fd
sudo virsh define /etc/libvirt/qemu/<instance-id>.xml
# openstack server start
```

(Path depends on whether your image is amd64 (`OVMF_VARS.fd`) or
arm64 (`AAVMF_VARS.fd`).)

## License

[BSD 3-Clause](../../go-coff/stub/LICENSE).
