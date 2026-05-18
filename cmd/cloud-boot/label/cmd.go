// Package label implements `cloud-boot label`: read or write the
// ext4 volume label of a disk image. The image format (raw / QCOW2 /
// UDIF-UDRW DMG) is detected automatically — same dispatcher as
// go-diskimages' OpenBlockDevice.
//
// Typical use is pre-boot stamping of a cloud image so the plan can
// reference `device = "LABEL=…"` instead of a fragile `/dev/vd*N`
// path. See the `label:debian:cloud` Taskfile target.
package label

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/go-diskimages/diskimage"
)

// Cmd returns the cobra subcommand.
func Cmd() *cobra.Command {
	var partIndex int
	cmd := &cobra.Command{
		Use:   "label [flags] <image> [<new-label>]",
		Short: "Read or write the ext4 volume label of a disk image (raw / QCOW2 / UDIF-UDRW)",
		Long: `Read or write the ext4 volume label of a disk image.

The image format is detected automatically:
  - raw           — direct file I/O
  - QCOW2         — through the copy-on-write layer
  - UDIF DMG      — UDRW subformat only (in place, koly checksums refreshed
                    on close). Compressed DMGs are rejected; use
                    dmg.UnpackToTemp + PackFromTemp if you really need them.

With one argument, prints the current label. With two arguments, writes
the second as the new label (max 16 bytes for ext4).

The --part flag picks which partition to look at when the image is
GPT/MBR-partitioned. Use -1 to auto-detect the first Linux partition.

Examples:
  cloud-boot label genericcloud.qcow2                        # read
  cloud-boot label --part 0 genericcloud.raw cloudimg-rootfs # stamp
`,
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if len(args) == 1 {
				lbl, err := diskimage.Ext4Label(path, partIndex)
				if err != nil {
					return err
				}
				fmt.Println(lbl)
				return nil
			}
			if err := diskimage.SetExt4Label(path, partIndex, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stamped LABEL=%q on %s (part %d)\n", args[1], path, partIndex)
			return nil
		},
	}
	cmd.Flags().IntVar(&partIndex, "part", 0, "partition index (0-based; -1 = auto-detect first Linux partition)")
	return cmd
}
