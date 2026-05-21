// Example plan for chaining an existing ZFS-rooted Linux install
// (typical: Proxmox VE / Ubuntu Server ZSYS) via cloud-boot.
//
// PRECONDITIONS — this plan only works when:
//
//   1. The cloud-boot UKI was built against the `disk-zfs` kernel
//      variant (Image-disk-zfs), NOT the default `disk` variant.
//      The base kernel doesn't ship the OpenZFS module.
//
//   2. The cloud-boot initramfs bundles zfsutils-linux (zpool +
//      zfs binaries). Build-time wiring TBD — see memory:zfs-
//      root-support for the roadmap.
//
//   3. The VM has the existing ZFS pool attached as one or more
//      virtio-blk devices. Apple VZ tested only with single-vdev
//      pools; multi-vdev RAID-Z layouts need each member device
//      attached so zpool import can assemble them.
//
// Until preconditions 1+2 land, picking this target will surface
// a clear error from disk_zfs_linux.go: "kernel has no zfs module"
// or "zpool not in PATH". That's intentional — better than a
// cryptic mount(2) failure.

default_target = "proxmox-installed"

locals {
  console = "ttyAMA0"
}

# Proxmox VE on the default ZFS layout. The installer creates
# rpool/ROOT/pve-1 as the root dataset, with /boot inside that
# same dataset (no separate bpool — Proxmox bundles GRUB modules
# that read the full feature-set rpool directly).
target "proxmox-installed" {
  label   = "Proxmox VE (existing install, rpool/ROOT/pve-1)"
  disk {
    device = "rpool/ROOT/pve-1"
    fs     = "zfs"
    kernel = "/boot/vmlinuz-pve"   # symlink to the latest
    initrd = "/boot/initrd.img-pve"
  }
  cmdline = "root=ZFS=rpool/ROOT/pve-1 boot=zfs console=${local.console} ro"
}

# Ubuntu Server with the installer's ZSYS layout — root on rpool,
# /boot on bpool (a separate pool with a GRUB-compatible feature
# subset). Replace ubuntu_xxxxxx with the suffix the installer
# generated for your install (visible in `zfs list`).
target "ubuntu-zsys" {
  label   = "Ubuntu Server (ZSYS install, rpool + bpool)"
  disk {
    device = "rpool/ROOT/ubuntu_xxxxxx"
    fs     = "zfs"
    kernel = "/boot/vmlinuz"      # provided by bpool, symlinked into rpool's /boot/efi/... layout
    initrd = "/boot/initrd.img"
  }
  cmdline = "root=ZFS=rpool/ROOT/ubuntu_xxxxxx console=${local.console} ro"
}
