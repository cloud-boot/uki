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

## License

[BSD 3-Clause](../../go-coff/stub/LICENSE).
