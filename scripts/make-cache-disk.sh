#!/usr/bin/env bash
# make-cache-disk.sh — create a GPT-partitioned cache disk for the
# menu-then-reboot sink under Apple VZ.
#
# The cache disk is the writable counterpart to the immutable
# boot.iso. The reboot sink writes `\EFI\Linux\<target>-vmlinuz.efi`
# + `\EFI\Linux\<target>-initrd` onto the cache disk's FAT ESP,
# then calls reboot(2). On the next firmware pass the boot manager
# honours the Boot0001/BootOrder NVRAM entries and loads the staged
# kernel from the cache disk — boot.iso is never written to.
#
# Why a dedicated cache disk (rather than mutating boot.iso):
#
#   * boot.iso stays byte-identical across runs → hashable,
#     shareable between VMs, swappable with a freshly-built ISO
#     without losing already-staged targets.
#   * Mutation surface is bounded to one well-known file.
#   * The FAT ESP also doubles as a future blob cache for OCI
#     artifacts — kernel/initrd pulled once, reused on subsequent
#     boots.
#
# Layout:
#
#   * MBR + GPT (standard protective MBR)
#   * Partition 1: EFI System Partition (FAT32)
#     - type-GUID  C12A7328-F81F-11D2-BA4B-00A0C93EC93B
#     - name       "cloud-boot-cache"  (used by findAndMountESP
#                  in cloud-boot-init to pick this ESP over any
#                  others present, e.g. an attached cloud-image
#                  ESP for a DISK target)
#     - aligned at LBA 2048 (1 MiB) per UEFI convention
#
# Usage:
#   make-cache-disk.sh PATH [SIZE_MiB]
#
#   PATH      Where to create the cache disk. Idempotent: if a
#             file already exists at PATH AND it carries a GPT
#             with a partition named "cloud-boot-cache", the
#             script exits 0 without touching it. This is the
#             expected steady-state behaviour: every vfkit run
#             can call the script as a precondition and only
#             pay the create cost on first boot.
#   SIZE_MiB  Total disk size in MiB. Default 256. The ESP
#             occupies all of it minus GPT overhead.

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "usage: $0 PATH [SIZE_MiB]" >&2
  exit 64
fi

DISK_PATH=$1
SIZE_MIB=${2:-256}

# Idempotency check: a pre-existing cache disk with the named
# partition is left untouched.
if [ -f "$DISK_PATH" ]; then
  existing_name=$(python3 - "$DISK_PATH" <<'PY' 2>/dev/null || echo ""
import struct, sys
try:
    with open(sys.argv[1], "rb") as f:
        f.seek(512); h = f.read(92)
        if h[:8] != b"EFI PART": sys.exit(0)
        pte_lba = struct.unpack("<Q", h[72:80])[0]
        f.seek(pte_lba * 512)
        e = f.read(128)
        if e[:16] == b"\x00" * 16: sys.exit(0)
        name = e[56:128].decode("utf-16-le").rstrip("\x00")
        print(name)
except Exception:
    pass
PY
  )
  if [ "$existing_name" = "cloud-boot-cache" ]; then
    echo "$DISK_PATH already has a cloud-boot-cache partition; leaving untouched."
    exit 0
  fi
  echo "warning: $DISK_PATH exists but is not a cloud-boot cache disk; overwriting." >&2
fi

# 1 MiB GPT prefix + ESP partition.
ESP_FIRST_LBA=2048
ESP_LBAS=$(( (SIZE_MIB * 1024 * 1024 / 512) - ESP_FIRST_LBA - 33 ))
ESP_BYTES=$(( ESP_LBAS * 512 ))
TOTAL_LBA=$(( ESP_FIRST_LBA + ESP_LBAS + 33 ))

# Build a fresh FAT32 image with mformat. We stamp it into the
# disk image at the right offset below.
TMPESP=$(mktemp -t cloud-boot-cache-esp.XXXXXX)
trap "rm -f \"$TMPESP\"" EXIT
truncate -s "$ESP_BYTES" "$TMPESP"
mformat -i "$TMPESP" -F ::
mmd -i "$TMPESP" ::/EFI
mmd -i "$TMPESP" ::/EFI/Linux

# Allocate the disk and write the GPT.
truncate -s "$(( TOTAL_LBA * 512 ))" "$DISK_PATH"

python3 - "$DISK_PATH" "$ESP_FIRST_LBA" "$ESP_LBAS" "$TOTAL_LBA" <<'PY'
import struct, sys, uuid, zlib

path = sys.argv[1]
esp_first = int(sys.argv[2])
esp_lbas  = int(sys.argv[3])
total_lba = int(sys.argv[4])

ESP_TYPE_GUID = uuid.UUID("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")
DISK_GUID = uuid.uuid4()
ESP_GUID  = uuid.uuid4()

PTE_COUNT = 128
PTE_SIZE  = 128
PTE_LBA   = 2
FIRST_USABLE = PTE_LBA + (PTE_COUNT * PTE_SIZE) // 512        # 34
LAST_USABLE  = total_lba - 34                                  # last_lba is total-1; -33 PTE block, -1 secondary header
SEC_PTE_LBA  = total_lba - 33
SEC_HDR_LBA  = total_lba - 1

# Single partition entry (ESP, named).
entries = bytearray(PTE_COUNT * PTE_SIZE)
e = bytearray(PTE_SIZE)
e[0:16]   = ESP_TYPE_GUID.bytes_le
e[16:32]  = ESP_GUID.bytes_le
e[32:40]  = struct.pack("<Q", esp_first)
e[40:48]  = struct.pack("<Q", esp_first + esp_lbas - 1)
e[48:56]  = struct.pack("<Q", 0)
name      = "cloud-boot-cache".encode("utf-16-le")
e[56:56 + len(name)] = name
entries[0:PTE_SIZE] = e
entries_crc = zlib.crc32(bytes(entries))

def make_hdr(my_lba, alt_lba, pte_lba):
    h = bytearray(92)
    h[0:8]   = b"EFI PART"
    h[8:12]  = struct.pack("<I", 0x00010000)
    h[12:16] = struct.pack("<I", 92)
    h[16:20] = struct.pack("<I", 0)
    h[20:24] = b"\x00" * 4
    h[24:32] = struct.pack("<Q", my_lba)
    h[32:40] = struct.pack("<Q", alt_lba)
    h[40:48] = struct.pack("<Q", FIRST_USABLE)
    h[48:56] = struct.pack("<Q", LAST_USABLE)
    h[56:72] = DISK_GUID.bytes_le
    h[72:80] = struct.pack("<Q", pte_lba)
    h[80:84] = struct.pack("<I", PTE_COUNT)
    h[84:88] = struct.pack("<I", PTE_SIZE)
    h[88:92] = struct.pack("<I", entries_crc)
    h[16:20] = struct.pack("<I", zlib.crc32(bytes(h)))
    return h

primary   = make_hdr(1, SEC_HDR_LBA, PTE_LBA)
secondary = make_hdr(SEC_HDR_LBA, 1, SEC_PTE_LBA)

# Protective MBR.
mbr = bytearray(512)
mbr[446]      = 0x00
mbr[447:450]  = bytes([0x00, 0x02, 0x00])
mbr[450]      = 0xEE
mbr[451:454]  = bytes([0xFF, 0xFF, 0xFF])
mbr[454:458]  = struct.pack("<I", 1)
mbr[458:462]  = struct.pack("<I", min(total_lba - 1, 0xFFFFFFFF))
mbr[510:512]  = b"\x55\xAA"

with open(path, "r+b") as f:
    f.seek(0)
    f.write(mbr)
    f.seek(512)
    f.write(primary + b"\x00" * (512 - 92))
    f.seek(PTE_LBA * 512)
    f.write(entries)
    f.seek(SEC_PTE_LBA * 512)
    f.write(entries)
    f.seek(SEC_HDR_LBA * 512)
    f.write(secondary + b"\x00" * (512 - 92))
PY

# Stamp the FAT32 image into the ESP partition byte range.
dd if="$TMPESP" of="$DISK_PATH" bs=512 seek="$ESP_FIRST_LBA" count="$ESP_LBAS" conv=notrunc status=none

echo "cache disk created: $DISK_PATH (${SIZE_MIB} MiB, ESP at LBA ${ESP_FIRST_LBA}..$((ESP_FIRST_LBA + ESP_LBAS - 1)), name='cloud-boot-cache')"
