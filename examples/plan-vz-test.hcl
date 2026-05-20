// Test plan for the local QEMU smoke run. Identical in shape to plan.hcl
// but with OCI refs that resolve inside a QEMU user-mode network (the host
// is reachable as 10.0.2.2). Push to 127.0.0.1:5000 from the host CLI; the
// guest pulls from 192.168.64.1:5000 — both URLs hit the same registry.

default_target = "alpine"

menu {
  timeout = "5s"
  prompt  = "cloud-boot QEMU test — pick a target"
}

# Shared registry base. In a real deployment this would be an SRV name
# like "_oci._tcp.registry.example.com/boot" so DNS-SRV gives us
# multi-host failover; here we use the QEMU NAT host directly. Targets
# below reference it as ${local.registry}.
locals {
  registry    = "192.168.64.1:5000/boot"
  alpine_repo = "https://dl-cdn.alpinelinux.org/alpine"
  # Per-arch serial console name: arm64-virt routes its primary UART
  # to PL011 (ttyAMA0); x86_64-q35 and riscv64-virt both expose a
  # plain 8250 (ttyS0). One local, every target reuses it — `arch` is
  # the runtime arch the plan was evaluated for, so this resolves
  # once per boot.
  console     = arch == "arm64" ? "ttyAMA0" : "ttyS0"
}

# Alpine netboot smoke target. Boots the upstream vmlinuz-virt +
# initramfs-virt; without modloop the init script drops to a busybox
# shell on the serial console — which is exactly what we want to
# confirm cloud-boot's kexec works end-to-end.
#
# Push the artifacts first with:
#
#   task push:alpine:multi
#
# (which fetches
#   https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/{x86_64,aarch64}/netboot/{vmlinuz-virt,initramfs-virt}
# and pushes them as a manifest list at
#   127.0.0.1:5000/boot/alpine:3.21).
target "alpine" {
  # `version` is a free-form release tag exposed to the target's own
  # HCL expressions as `self.version` (alongside `self.name` and
  # `self.arch`). Lets us name it once and weave it into the OCI tag
  # *and* the apk repo URL below — no DRY violation. We use the bare
  # form ("3.21") here because `push:alpine` pushes to that exact OCI
  # tag; the dl-cdn URL path needs a "v" prefix which goes directly
  # in the cmdline string ("…/v${self.version}/main").
  version = "3.21"
  label   = "Alpine ${self.version} netboot (smoke test)"
  # Multi-arch image index — its per-arch manifest bundles four
  # layers:
  #   vmlinuz-virt     → kernel
  #   initramfs-virt   → initrd
  #   modloop-virt     → modloop squashfs (kernel modules)
  #   apkovl.tar.gz    → Alpine system overlay (hostname, world,
  #                      inittab with auto-login getty, …)
  # The squashfs + apkovl are *not* downloaded by cloud-boot-init:
  # we hand their OCI blob URLs to Alpine's init via `modloop=` and
  # `apkovl=` cmdline directives (auto-injected when the layers are
  # present; you can also reference them explicitly via
  # `{{.ModloopURL}}` / `{{.ApkovlURL}}` in cmdlineVars).
  index   = "${local.registry}/alpine:${self.version}"
  # cmdline can be a string or — for readability when there are many
  # tokens — a list of strings joined with spaces. Each entry is HCL,
  # so ${...} interpolation, ternaries, `self.*` and `local.*` work
  # per-element. Go-template `{{...}}` runs at materialize time
  # against cmdlineVars (registry / network facts cloud-boot already
  # learned).
  #
  # `{{.IPSpec}}` reuses the DHCP lease cloud-boot negotiated (klibc
  # ip= positional form, autoconf=off) so Alpine's initramfs doesn't
  # redo a DHCP round-trip — `ip=dhcp` would work too but is wasteful.
  #
  # `alpine_repo=` is the apk source for the initial package install
  # (alpine-base, openrc, openssh, …). Alpine's netboot init copies
  # this URL into /tmp/repositories *before* running `apk add` against
  # the world list shipped in the apkovl. Without it apk has no repo
  # and the boot drops to emergency shell. dl-cdn.alpinelinux.org is
  # reachable from the QEMU guest via user-mode NAT.
  cmdline = [
    "console=${local.console}",
    "console=hvc0",
    "console=tty0",
    # `quiet` raises the kernel loglevel so dmesg chatter is hidden
    # — the openrc service lines stay visible because they go
    # through busybox `* foo: ok.` printk-bypass output. Pick the
    # `alpine-debug` target below if you want the full firehose.
    "quiet",
    "ip={{.IPSpec}}",
    "alpine_repo=${local.alpine_repo}/v${self.version}/main",
  ]
}

# Same artifact as `alpine`, with the kernel chatter unmuted and
# Alpine's /init in `set -x` mode (KOPT_debug_init). Use when the
# silent target fails and you need to see every step before the
# emergency shell.
target "alpine-debug" {
  version = "3.21"
  label   = "Alpine ${self.version} netboot (debug)"
  index   = "${local.registry}/alpine:${self.version}"
  cmdline = [
    "console=${local.console}",
    "console=hvc0",
    "console=tty0",
    "ip={{.IPSpec}}",
    "alpine_repo=${local.alpine_repo}/v${self.version}/main",
    # KOPT_debug_init triggers `set -x` in Alpine's /init script.
    # No leading `cloudboot.` — this is honoured by Alpine, not by
    # cloud-boot-init.
    "debug_init=yes",
  ]
}

# Debian 13 (Trixie) netboot installer. The d-i initrd is self-
# contained — no modloop/apkovl story — so the target is much shorter
# than alpine. Boots into the text-mode installer (Debian-Installer);
# pass `auto=true priority=critical` + a preseed URL to drive it
# unattended.
target "debian" {
  version = "13"
  label   = "Debian ${self.version} netboot installer"
  index   = "${local.registry}/debian:${self.version}"
  cmdline = [
    "console=${local.console},115200n8",
    "ip={{.IPSpec}}",
  ]
}

# Debian 13.5 Live (standard flavor — minimal, no DE). Mirrors the
# alpine target's shape: the OCI artifact carries (vmlinuz, initrd,
# filesystem.squashfs) and cloud-boot-init auto-injects
# `boot=live fetch=<oci-blob-url-of-squashfs>` so live-boot's initrd
# pulls the squashfs at switch_root time. Push the artifact first:
#
#   task push:debian-live ARCH=amd64
#   # OR with a desktop variant:
#   task push:debian-live ARCH=amd64 FLAVOR=xfce
#
# (which fetches debian-live-13.5.0-amd64-standard.iso, extracts
# /live/{vmlinuz,initrd.img,filesystem.squashfs}, and pushes them
# under 127.0.0.1:5000/boot/debian-live:13.5.0-standard-amd64).
#
# `toram` makes live-boot copy the squashfs into RAM before
# switch_root so the boot isn't tied to the registry's uptime past
# the initial fetch; drop it on memory-constrained hosts. `noeject`
# skips the eject-the-CD prompt at shutdown (we're netbooted, no CD
# to eject).
#
# HTTP/2: the OCI manifest+blob pulls from cloud-boot-init happen
# over Go's stdlib http.Client. Plaintext registries (the local
# 127.0.0.1:5000 here) default to HTTP/1.1; export
# REGISTRY_HTTP2_CLEARTEXT=1 before boot to opt into h2c, which
# multiplexes the kernel/initrd/squashfs pulls onto one TCP stream
# (saves ~3 connection roundtrips vs. HTTP/1.1).
target "debian-live" {
  version = "13.5.0"
  label   = "Debian ${self.version} Live (standard, netbooted squashfs)"
  index   = "${local.registry}/debian-live:${self.version}-standard-amd64"
  cmdline = [
    "console=${local.console},115200n8",
    "console=hvc0",
    "ip={{.IPSpec}}",
    "live-config",
    "noeject",
    "toram",
    # boot=live + fetch=<url> is injected automatically by
    # cloud-boot-init when the manifest carries a squashfs layer.
  ]
}

# Debian 13 generic-cloud image booted in DISK MODE. The qcow2 is
# attached to QEMU as /dev/vda by `task qemu:arm64:debian-cloud`;
# cloud-boot mounts /dev/vda1 (the root partition), finds the
# pre-installed /boot/vmlinuz-*-cloud-arm64 + matching initrd, and
# kexecs into them. Once Debian comes up, cloud-init reads the
# CIDATA ISO sitting on /dev/vdb and applies the configuration in
# examples/cloud-init/user-data.yaml (creates the `debian` user with
# password `debian`).
#
# No OCI fetches happen for this target — everything lives on the
# host-side drives QEMU attaches. The target's `index` field is
# absent on purpose; disk{} is mutually exclusive.
target "debian-cloud" {
  version = "13"
  label   = "Debian ${self.version} cloud image + cloud-init"
  # device = "LABEL=cloudimg-rootfs" is resolved by cloud-boot-init
  # scanning every partition's ext4 superblock. The label is stamped
  # on the raw image at build time by `task label:debian:cloud`
  # (Debian's cloud images ship unlabeled, hence the host-side step).
  # Same syntax also supports UUID=…, PARTLABEL=…, PARTUUID=… —
  # see the kernel's `root=` documentation.
  cmdline = "console=${local.console},115200n8 console=hvc0 console=tty0 root=LABEL=cloudimg-rootfs ro"
  disk {
    device = "LABEL=cloudimg-rootfs"
    fs     = "ext4"
  }
}

target "primary" {
  label   = "Production Linux 6.6 (OCI)"
  index   = "${local.registry}/linux:6.6"
  cmdline = "console=${local.console} console=hvc0"
}

target "rescue" {
  label   = "Rescue shell (rd.break)"
  index   = "${local.registry}/rescue:latest"
  cmdline = "console=${local.console} console=hvc0 single rd.break"
}

target "from-disk" {
  label   = "Boot kernel from /dev/vda2"
  cmdline = "console=${local.console} console=hvc0 ro root=/dev/vda2"
  disk {
    device = "/dev/vda2"
    fs     = "ext4"
  }
}
