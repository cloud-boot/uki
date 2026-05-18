// Package modpack implements `cloud-boot push modpack`: pack a local
// /lib/modules/<release> tree into a gzipped cpio so the result can be
// concatenated onto an initrd at boot time.
package modpack

import (
	"fmt"

	"github.com/spf13/cobra"
)

type Opts struct {
	Src string
	Out string
}

func Cmd() *cobra.Command {
	var o Opts
	cmd := &cobra.Command{
		Use:   "modpack",
		Short: "Pack a /lib/modules/<release> tree into a kernel-modules cpio.gz",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Run(o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Src, "src", "", "source directory (e.g. /lib/modules/6.6.0) (required)")
	f.StringVarP(&o.Out, "output", "o", "", "output cpio.gz path (required)")
	_ = cmd.MarkFlagRequired("src")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func Run(o Opts) error {
	if o.Src == "" || o.Out == "" {
		return fmt.Errorf("modpack: --src and --output are required")
	}
	return packModules(o.Src, o.Out)
}
