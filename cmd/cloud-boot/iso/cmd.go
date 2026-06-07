// Package iso implements the `cloud-boot iso` subcommand: assemble one
// hybrid iso9660 + GPT image that embeds UKIs for multiple CPU
// architectures, so a single ISO boots on amd64, arm64, and riscv64.
package iso

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloud-boot/uki/cmd/cloud-boot/build"
)

// Cmd returns the `cloud-boot iso` cobra subcommand.
func Cmd() *cobra.Command {
	var (
		ukiFlags []string
		out      string
		workDir  string
		keep     bool
	)
	c := &cobra.Command{
		Use:   "iso",
		Short: "Assemble a multi-arch hybrid ISO from per-arch UKIs",
		Long: `Pack one or more already-built UKIs into a single hybrid
iso9660 + El Torito + GPT image.

Each UKI is dropped into the ESP at the UEFI removable-media fallback
path for its architecture (\EFI\BOOT\BOOTX64.EFI for amd64,
\EFI\BOOT\BOOTAA64.EFI for arm64, \EFI\BOOT\BOOTRISCV64.EFI for riscv64).
Firmware on each CPU reads only its own arch's file, so the same ISO
boots on amd64, arm64 and riscv64 hosts.

Example:

  cloud-boot iso \
    --uki linux/amd64=boot-amd64.efi \
    --uki linux/arm64=boot-arm64.efi \
    --uki linux/riscv64=boot-riscv64.efi \
    --output boot.iso

The per-arch .efi files are produced by 'cloud-boot build --arch <a>' —
just point one build per arch at the right kernel/initrd inputs, then
collect them here.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(ukiFlags) == 0 {
				return fmt.Errorf("at least one --uki linux/<arch>=<path> is required")
			}
			ukis, err := parseUKIList(ukiFlags)
			if err != nil {
				return err
			}
			return build.BuildMultiArchISO(build.MultiOpts{
				UKIs:    ukis,
				Out:     out,
				WorkDir: workDir,
				Keep:    keep,
			})
		},
	}
	f := c.Flags()
	f.StringArrayVar(&ukiFlags, "uki", nil, "per-arch UKI as linux/<arch>=<path>; repeat for each arch (amd64, arm64, riscv64, loongarch64)")
	f.StringVarP(&out, "output", "o", "boot.iso", "output ISO path")
	f.StringVar(&workDir, "work", "", "build workdir (default: tempdir)")
	f.BoolVar(&keep, "keep", false, "keep workdir on success")
	return c
}

// parseUKIList turns the repeated --uki flag values into ArchUKIs.
// Each entry is "linux/<arch>=<path>"; the arch is looked up in
// build.ArchProfiles for its EFIName.
func parseUKIList(raw []string) ([]build.ArchUKI, error) {
	out := make([]build.ArchUKI, 0, len(raw))
	for _, entry := range raw {
		key, path, ok := strings.Cut(entry, "=")
		if !ok || key == "" || path == "" {
			return nil, fmt.Errorf("invalid --uki %q (want linux/<arch>=<path>)", entry)
		}
		_, arch, ok := strings.Cut(key, "/")
		if !ok || arch == "" {
			return nil, fmt.Errorf("invalid --uki platform %q (want linux/<arch>)", key)
		}
		prof, ok := build.ArchProfiles[arch]
		if !ok {
			return nil, fmt.Errorf("unsupported arch %q (allowed: amd64, arm64, riscv64, loongarch64)", arch)
		}
		out = append(out, build.ArchUKI{Arch: prof, UKI: path})
	}
	return out, nil
}
