package main

import (
	"github.com/spf13/cobra"

	"github.com/cloud-boot/uki/cmd/cloud-boot/build"
	"github.com/cloud-boot/uki/cmd/cloud-boot/iso"
	"github.com/cloud-boot/uki/cmd/cloud-boot/label"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push"
)

// newRootCmd builds the cobra tree fresh on every call so tests can swap
// it out without leaking state between cases.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cloud-boot",
		Short: "Host-side toolchain for the cloud-boot UKI/ISO + OCI workflow",
		Long: `cloud-boot is the host-side CLI for the cloud-boot toolchain.

Use "cloud-boot build" to assemble a bootable UEFI UKI/ISO from a kernel,
an EFI stub, and an OCI plan/image reference.

Use "cloud-boot iso" to assemble ONE hybrid ISO that embeds multiple
per-arch UKIs (amd64, arm64, riscv64) under their respective
\EFI\BOOT\BOOT<ARCH>.EFI removable-media fallback paths — the same ISO
then boots on any of the supported CPUs.

Use "cloud-boot push" to upload the artifacts referenced by the plan to an
OCI registry — kernel/initrd/modules manifests, HCL plans, multi-arch
indexes, and kernel-modules cpio.gz bundles.`,
		SilenceUsage: true,
	}
	root.AddCommand(build.Cmd())
	root.AddCommand(iso.Cmd())
	root.AddCommand(label.Cmd())
	root.AddCommand(push.Cmd())
	return root
}
