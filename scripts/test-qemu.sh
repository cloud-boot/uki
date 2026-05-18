#!/usr/bin/env bash
# End-to-end smoke test (single arch):
#   1) start a local registry container on :5000
#   2) push per-arch kernel/initrd/modules artifacts
#   3) push the HCL plan
#   4) build boot.iso for the chosen arch
#   5) boot it in QEMU with virtio-net
#
# Required: KERNEL (host vmlinuz), STUB (linux*.efi.stub), ARCH (default amd64)
# Optional: PAY_KERNEL, PAY_INITRD, PAY_MODULES, PAY_CMDLINE
set -euo pipefail

: "${KERNEL:?set KERNEL=/path/to/vmlinuz}"
: "${STUB:?set STUB=/path/to/linux*.efi.stub}"
ARCH=${ARCH:-amd64}
PAY_KERNEL=${PAY_KERNEL:-$KERNEL}
PAY_INITRD=${PAY_INITRD:-}
PAY_MODULES=${PAY_MODULES:-}
PAY_CMDLINE=${PAY_CMDLINE:-console=ttyS0}

PLAN_REF="127.0.0.1:5000/boot/plan:latest"
ARTIFACT_REF="127.0.0.1:5000/boot/linux:6.6-$ARCH"

task build

# Local registry.
if ! docker ps --format '{{.Names}}' | grep -q '^cloud-boot-registry$'; then
  task registry
  for _ in $(seq 1 30); do
    curl -fsS http://127.0.0.1:5000/v2/ >/dev/null && break || sleep 0.5
  done
fi

# 1) Push payload for $ARCH (kernel + optional initrd / modules / cmdline).
push_args=(--platform "linux/$ARCH" --kernel "$PAY_KERNEL")
[[ -n "$PAY_INITRD"  ]] && push_args+=(--initrd  "$PAY_INITRD")
[[ -n "$PAY_MODULES" ]] && push_args+=(--modules "$PAY_MODULES")
[[ -n "$PAY_CMDLINE" ]] && push_args+=(--cmdline "$PAY_CMDLINE")
./bin/cloud-boot push artifact "${push_args[@]}" "$ARTIFACT_REF"

# 2) Push the HCL plan (referenced by cloud-boot-init via cloudboot.plan=).
task push:plan PLAN=examples/plan.hcl PLAN_REF="$PLAN_REF"

# 3) Build ISO referencing the plan.
task iso ARCH="$ARCH" KERNEL="$KERNEL" STUB="$STUB" \
         PLAN_REF="$PLAN_REF" INSECURE=1

# 4) Boot.
if [[ "$ARCH" == "arm64" ]]; then
  exec task qemu:arm64 ARCH="$ARCH"
else
  exec task qemu ARCH="$ARCH"
fi
