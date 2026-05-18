// Example boot plan consumed by cloud-boot-init when the boot UKI
// carries cloudboot.plan=<ref-of-this-plan-as-an-OCI-artifact>.
//
// Each referenced ref may be a multi-arch image index; the init resolves it
// at boot time against runtime.GOARCH.
//
// Fields are HCL expressions evaluated against an EvalContext exposing:
//
//   arch                runtime arch ("amd64" | "arm64" | "riscv64")
//   lldp.available      true if an LLDP frame was observed at boot
//   lldp.chassis_id     upstream switch chassis MAC (or other subtype value)
//   lldp.port_id        upstream switch port ID (typically the iface name)
//   lldp.system_name    sysName MIB advertised by the switch
//   lldp.system_desc    sysDescr MIB advertised by the switch
//   lldp.port_desc      port description (often a VLAN label or use string)
//   lldp.mgmt_addr      first management address (v4 or v6)

default_target = "primary"

# Shared registry base, factored into a `locals` block so multiple
# targets stay in sync. The `_oci._tcp.` prefix triggers an RFC-2782
# SRV lookup at boot — cloud-boot-init resolves it to one of N
# registry hosts in priority/weight order, so a single registry node
# going down doesn't break the boot. Targets reference it as
# ${local.registry}.
locals {
  registry         = "_oci._tcp.registry.example.com/boot"
  fallback_registry = "127.0.0.1:5000/boot"
}

# Optional: list nameservers cloud-boot-init writes into /etc/resolv.conf
# *after* parsing this plan, replacing whatever DHCP supplied. Use it
# when the target/modloop refs sit behind an SRV name that only resolves
# via a private DNS (e.g. dev CoreDNS, in-cluster Consul, etc.). The
# `cloudboot.dns=ip[,ip...]` kernel cmdline knob takes precedence and
# applies *before* the plan is fetched — set both when the plan registry
# itself is behind a custom DNS.
#
# dns = ["10.0.2.2", "1.1.1.1"]

# Interactive boot menu. When present, cloud-boot-init lists every
# arch-eligible target on the console and waits up to `timeout` for the
# operator to pick one. On timeout or empty input the default_target wins.
#
# - timeout: Go duration ("5s", "1m30s") or a bare integer (seconds).
#            "" or omitted = wait forever; the block is removed to disable.
# - prompt:  optional header text; defaults to "Select a boot target:".
#
# Cmdline overrides (set on the UKI's kernel command line):
#   cloudboot.target=<name>       skip the menu, pick this target directly
#   cloudboot.menu=0              force the menu off
#   cloudboot.menu=1              force the menu on (even without this block)
#   cloudboot.menu.timeout=<dur>  override the timeout below
#   cloudboot.menu.prompt=<text>  override the prompt below
menu {
  timeout = "5s"
  prompt  = "cloud-boot: choose a target"
}

# A host that starts with "_<service>._tcp." or "_<service>._udp." is
# resolved at boot via DNS SRV (RFC 2782). The result is a list of
# replicas tried in priority+weight order, with automatic failover on
# network errors / 5xx responses. This makes the registry highly available
# without any client-side load-balancer config.
target "primary" {
  label   = "Production Linux 6.6"
  # Demonstrates the four-ref split: a kernel-only ref + an initrd-only
  # ref + a modules-only ref, all merged at boot time. Use this shape
  # to share one kernel across many initramfs / modules flavours.
  kernel  = "${local.registry}/linux:6.6"
  initrd  = "${local.registry}/initrd:fedora"
  modules = "${local.registry}/modules:6.6"
  cmdline = lldp.available ? "console=ttyS0 ro root=LABEL=root hostname=node-${lldp.port_id}" : "console=ttyS0 ro root=LABEL=root"
}

target "rescue" {
  label   = "Rescue shell (rd.break)"
  arch    = "amd64"
  # `index` here because the rescue image bundles kernel + initramfs in
  # a single OCI artifact (typical for self-contained installers).
  index   = "${local.fallback_registry}/rescue:latest"
  cmdline = "console=ttyS0 single rd.break"
}

target "arm-edge" {
  label   = "ARM edge node"
  arch    = "arm64"
  kernel  = "${local.fallback_registry}/linux:6.6"
  modules = "${local.fallback_registry}/modules:6.6"
  cmdline = "console=ttyAMA0 ro"
}

# Boot the kernel that already lives on a local disk. No OCI pull happens
# for this target — cloud-boot-init mounts disk.device read-only, picks the
# kernel + initrd from it, and kexecs straight into them. Useful for "first
# boot from network, subsequent boots from the disk we just provisioned".
#
# disk fields:
#   device   block device to mount (required)
#   fs       filesystem type (default ext4)
#   kernel   path on the mount; default: newest /boot/vmlinuz-*
#   initrd   path on the mount; default: paired with kernel by version suffix
#
# cmdline (the Target's, not a disk field) is forwarded as-is to the new
# kernel. Leaving it empty falls back to /etc/kernel/cmdline on the mount.
target "from-disk" {
  label   = "Boot from local disk (/dev/vda2)"
  cmdline = "console=ttyS0 ro root=/dev/vda2"
  disk {
    device = "/dev/vda2"
    fs     = "ext4"
  }
}
