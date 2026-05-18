// Package push wires together the cloud-boot push subcommand tree:
//
//	cloud-boot push artifact   single-arch kernel/initrd/modules manifest
//	cloud-boot push plan       HCL boot plan
//	cloud-boot push index      multi-arch image index
//	cloud-boot push modpack    /lib/modules → cpio.gz
//
// Each child is implemented in its own sub-package so a feature on
// `push artifact` doesn't drag the rest of the push code into recompiles.
package push

import (
	"github.com/spf13/cobra"

	"github.com/cloud-boot/uki/cmd/cloud-boot/push/artifact"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push/index"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push/modpack"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push/multi"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push/plan"
)

// Cmd returns the `cloud-boot push` parent. It owns no flags of its own —
// every subcommand carries the flags it needs.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload boot artifacts to an OCI registry",
		Long: `Push the artifacts referenced by a cloud-boot plan to an OCI registry.

Subcommands:
  artifact   single-arch kernel/initrd/modules/cmdline manifest
  multi      one shot: per-arch manifests + multi-arch index
  plan       HCL boot plan as a single-layer artifact
  index      multi-arch image index that fans out to existing per-arch manifests
  modpack    build a kernel-modules cpio.gz from a /lib/modules/<release> tree`,
	}
	cmd.AddCommand(artifact.Cmd())
	cmd.AddCommand(multi.Cmd())
	cmd.AddCommand(plan.Cmd())
	cmd.AddCommand(index.Cmd())
	cmd.AddCommand(modpack.Cmd())
	return cmd
}
