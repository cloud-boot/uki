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

## Multi-arch boot ISO (TamaGo + UEFI bring-up)

`cloud-boot iso` packs N already-built per-arch UEFI binaries into a
single hybrid iso9660 + GPT image. Each binary lands at the UEFI
removable-media fallback path for its CPU
(`\EFI\BOOT\BOOTX64.EFI` for amd64, `\EFI\BOOT\BOOTAA64.EFI` for
arm64, `\EFI\BOOT\BOOTLOONGARCH64.EFI` for loongarch64,
`\EFI\BOOT\BOOTRISCV64.EFI` for riscv64). Firmware on each CPU
reads only its own arch's file, so the same ISO boots on all four
hosts.

The end-to-end recipe below uses the four EFI binaries produced
by the sibling [`cloud-boot/tamago-uefi`](../tamago-uefi) board
package as inputs.

### 1. Build the multi-arch ISO

```sh
task iso:multiarch
# →  ./bin/cloud-boot iso \
#      --uki linux/amd64=../tamago-uefi/BOOTX64.EFI \
#      --uki linux/arm64=../tamago-uefi/BOOTAA64.EFI \
#      --uki linux/loongarch64=../tamago-uefi/BOOTLOONGARCH64.EFI \
#      --uki linux/riscv64=../tamago-uefi/BOOTRISCV64.EFI \
#      --output boot-multi.iso
```

Override any path with `EFI_AMD64=… EFI_ARM64=… EFI_LOONG64=…
EFI_RISCV64=…` if your binaries live elsewhere. The output ISO
path defaults to `boot-multi.iso`; override with `MULTI_ISO=…`.

Inspect the result:

```sh
xorriso -indev boot-multi.iso -toc          # boot record + ESP partition
mdir -i <(dd if=boot-multi.iso bs=512 skip=140) -/ ::/EFI/BOOT
```

You should see all four `BOOTX64.EFI`, `BOOTAA64.EFI`,
`BOOTLOONGARCH64.EFI`, `BOOTRISCV64.EFI` files in the ESP.

### 2. Boot-test each arch under QEMU + EDK2

```sh
task test:multiarch:boot
```

This wraps a pure-Go test driver in
[`internal/multiarchboot`](internal/multiarchboot). For each arch
it spawns `qemu-system-<arch>` with the matching EDK2 firmware
and asserts the captured serial output contains the
tamago-uefi banner:

```
hello from cloud-boot tamago/<arch> UEFI board
runtime: go1.26.3 GOOS=tamago GOARCH=<arch>
goroutine sum: 499500
DONE — halting
```

Each arch is skip-gated on its OVMF firmware env var being set
to an existing file — the suite stays green on hosts that don't
ship one of the four firmwares. Defaults assume homebrew's qemu
bottle on macOS:

| Env var                       | Default                                                          | Required? |
| ----------------------------- | ---------------------------------------------------------------- | --------- |
| `CLOUDBOOT_OVMF_AMD64_CODE`   | `$QEMU_EDK2_DIR/edk2-x86_64-code.fd`                             | always    |
| `CLOUDBOOT_OVMF_AMD64_VARS`   | `$QEMU_EDK2_DIR/edk2-i386-vars.fd`                               | always    |
| `CLOUDBOOT_OVMF_ARM64_CODE`   | `$QEMU_EDK2_DIR/edk2-aarch64-code.fd`                            | always    |
| `CLOUDBOOT_OVMF_LOONG64_CODE` | `$QEMU_EDK2_DIR/edk2-loongarch64-code.fd`                        | always    |
| `CLOUDBOOT_OVMF_RISCV64_CODE` | `$QEMU_EDK2_DIR/edk2-riscv-code.fd`                              | always    |
| `CLOUDBOOT_OVMF_RISCV64_VARS` | `$QEMU_EDK2_DIR/edk2-riscv-vars.fd`                              | always    |
| `QEMU_EDK2_DIR`               | `/opt/homebrew/Cellar/qemu/10.2.2/share/qemu`                    | macOS     |
| `CLOUDBOOT_BOOT_TIMEOUT`      | `60s` (per arch)                                                 | optional  |

Override `QEMU_EDK2_DIR` to point at any other directory holding
the six `edk2-*.fd` images (Debian: `/usr/share/qemu/`; Arch:
`/usr/share/edk2/...`; Fedora: `/usr/share/edk2/...`).

### 3. Where to get OVMF binaries

- **macOS** — `brew install qemu` ships `edk2-{x86_64,aarch64,
  loongarch64,riscv}-code.fd` and `edk2-{i386,arm,loongarch64,
  riscv}-vars.fd` under
  `/opt/homebrew/Cellar/qemu/<ver>/share/qemu/`.
- **Debian / Ubuntu** — `apt install ovmf qemu-efi-aarch64`. The
  aarch64 code lives at `/usr/share/AAVMF/AAVMF_CODE.fd`; amd64
  at `/usr/share/OVMF/OVMF_CODE.fd`. loongarch64 + riscv64 ship
  with `qemu-efi-loongarch64` / `qemu-efi-riscv64` in recent
  releases (Debian trixie / Ubuntu 24.04+); otherwise pull
  prebuilt `edk2-stable*` images from
  [retrage's edk2 build](https://retrage.github.io/edk2-nightly/)
  or build EDK2 from source for the missing arches.
- **Arch / Fedora** — `pacman -S edk2-aarch64 edk2-x86_64 edk2-
  loongarch64 edk2-riscv64-virt` (or the Fedora `edk2-*` package
  set).

The QEMU command lines invoked by `task test:multiarch:boot`
match the ones documented in
[`../tamago-uefi/README.md`](../tamago-uefi/README.md#how-this-is-built)
section "boot under QEMU/OVMF". Running the test target by hand
is equivalent to running each of those four invocations against
the multi-arch ISO instead of the per-arch ones.

### 4. Known per-arch status

| Arch     | Firmware                            | Status (QEMU 10.2.2 + EDK2 stable202408) |
| -------- | ----------------------------------- | ---------------------------------------- |
| amd64    | `edk2-x86_64-code.fd` + vars        | PASS — full banner over serial           |
| arm64    | `edk2-aarch64-code.fd`              | PASS — full banner over serial           |
| loong64  | `edk2-loongarch64-code.fd`          | PASS — full banner over serial           |
| riscv64  | `edk2-riscv-code.fd` + vars         | PASS — banner shown (note: tamago-uefi's riscv64 build currently bakes `GOARCH=amd64` into its banner string; the actual machine code is RISC-V and `goroutine sum: 499500` + `DONE` are emitted correctly. tracked in tamago-uefi.) |

The RISC-V firmware bug noted in tamago-uefi's README
(`SetUefiImageMemoryAttributes` fault under earlier
`edk2-stable202408` snapshots) does not reproduce on the homebrew
qemu 10.2.2 EDK2 bottle — the binary loads, runs and prints its
banner. If you see the fault on a different EDK2 build, flip the
`ExpectFail` field on the riscv64 entry in
[`internal/multiarchboot/multiarchboot.go`](internal/multiarchboot/multiarchboot.go)
to keep the suite green.

## License

[BSD 3-Clause](../../go-coff/stub/LICENSE).
