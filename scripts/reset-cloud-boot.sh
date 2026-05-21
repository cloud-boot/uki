#!/bin/sh
# reset-cloud-boot.sh — clear the NVRAM entries that point the
# firmware at the last cloud-boot-staged target, so the next
# reboot falls back to `\EFI\BOOT\BOOTAA64.EFI` on the boot.iso
# (= cloud-boot's menu).
#
# Run from INSIDE a running staged distro (Alpine, Debian,
# Ubuntu, etc.) — needs root + an EFI-booted system + an
# efivarfs mount (auto-mounted from CONFIG_EFIVAR_FS=y on
# every modern Linux).
#
# Why this is needed: cloud-boot's reboot sink writes
# Boot0001 = "load \EFI\Linux\<target>-vmlinuz.efi" and
# prepends 0001 to BootOrder. Until you either delete those
# vars or the underlying NVRAM is reset, every reboot of the
# VM boots straight into the staged target, bypassing the
# cloud-boot menu. Under hypervisors that persist NVRAM
# (OpenStack/libvirt with a per-instance <nvram> file, real
# hardware, …) you can't "just restart vfkit with `,create`"
# to wipe — you need this in-band reset.
#
# Out-of-band alternatives:
#   - vfkit (macOS dev): `rm /tmp/vz-test/vfkit/menu-vars.fd`
#     then re-launch with `,create`.
#   - OpenStack/libvirt: `virsh nvram-define <vm> --template
#     /usr/share/OVMF/OVMF_VARS.fd` (resets to firmware
#     defaults; needs the VM stopped + admin access on the
#     compute node).
#   - Any cloud where you control the API: usually a
#     snapshot/restore or an explicit "reset firmware vars"
#     call. Vendor-specific.

set -eu

EFIVARS=/sys/firmware/efi/efivars
EFI_GLOBAL_GUID=8be4df61-93ca-11d2-aa0d-00e098032b8c
BOOTNUM=${BOOTNUM:-0001}

usage() {
    cat <<EOF >&2
usage: $(basename "$0") [-r|--reboot]

Reads Boot${BOOTNUM} + BootOrder from /sys/firmware/efi/efivars
and removes them so the next firmware boot returns to
\\EFI\\BOOT\\BOOTAA64.EFI on the boot media.

Options:
  -r, --reboot   reboot immediately after clearing the vars.
  -h, --help     show this help.

Environment:
  BOOTNUM=NNNN  override the Boot#### entry to delete (defaults
                to 0001 — what cloud-boot's reboot sink writes).
EOF
}

reboot_after=0
case "${1:-}" in
    -r|--reboot) reboot_after=1 ;;
    -h|--help)   usage; exit 0 ;;
    "")          ;;
    *)           usage; exit 64 ;;
esac

[ "$(id -u)" = 0 ] || { echo "$(basename "$0"): must run as root" >&2; exit 1; }

# Ensure efivarfs is mounted. CONFIG_EFIVAR_FS systemd / openrc
# defaults usually auto-mount under /sys/firmware/efi/efivars but
# minimal distros sometimes skip it.
if [ ! -d "$EFIVARS" ] || [ -z "$(ls -A "$EFIVARS" 2>/dev/null)" ]; then
    echo "mounting efivarfs at $EFIVARS"
    mkdir -p "$EFIVARS"
    mount -t efivarfs efivarfs "$EFIVARS"
fi

# Prefer efibootmgr — it knows the LoadOption format and won't
# leave BootOrder pointing at a deleted Boot####. Falls back to
# raw efivarfs manipulation if efibootmgr isn't installed.
if command -v efibootmgr >/dev/null 2>&1; then
    echo "efibootmgr -b ${BOOTNUM} -B"
    efibootmgr -b "${BOOTNUM}" -B
    # -B already updates BootOrder to drop the entry, so we're
    # done.
else
    echo "(efibootmgr not found, falling back to direct efivarfs manipulation)"
    # Clear immutable bit + remove the Boot#### entry.
    boot_file="$EFIVARS/Boot${BOOTNUM}-${EFI_GLOBAL_GUID}"
    if [ -e "$boot_file" ]; then
        chattr -i "$boot_file" 2>/dev/null || true
        rm -f "$boot_file"
        echo "removed Boot${BOOTNUM}"
    else
        echo "Boot${BOOTNUM} not present — already gone"
    fi
    # Strip Boot#### from BootOrder. Without efibootmgr we can't
    # easily do a partial rewrite (BootOrder is a packed LE UINT16
    # array prefixed by efivarfs's 4-byte attribute header), so
    # we just remove BootOrder entirely — firmware will then use
    # the default removable-media boot path, which is exactly
    # what we want for a cloud-boot return.
    bo_file="$EFIVARS/BootOrder-${EFI_GLOBAL_GUID}"
    if [ -e "$bo_file" ]; then
        chattr -i "$bo_file" 2>/dev/null || true
        rm -f "$bo_file"
        echo "removed BootOrder"
    fi
fi

echo "next reboot will fall back to the boot media's default loader"
echo "(\\EFI\\BOOT\\BOOTAA64.EFI = cloud-boot menu, for arm64)"

if [ "$reboot_after" = 1 ]; then
    echo "rebooting now (-r)..."
    sync
    reboot
fi
