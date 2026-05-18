// Package build implements the `cloud-boot build` subcommand: it cross-
// compiles cloud-boot-init for the target arch, packs it into an initramfs,
// assembles a UKI via go-coff/pe, stages a FAT16 EFI System Partition, and
// produces a UEFI-bootable El Torito ISO with xorriso.
package build

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ArchProfile gathers per-arch toolchain knobs.
type ArchProfile struct {
	GoArch   string // GOARCH value for go build
	EFIName  string // ESP file name expected by UEFI removable-media fallback
	StubName string // basename of the systemd stub binary
}

// ArchProfiles lists every arch this CLI knows how to assemble for.
var ArchProfiles = map[string]ArchProfile{
	"amd64":   {"amd64", "BOOTX64.EFI", "linuxx64.efi.stub"},
	"arm64":   {"arm64", "BOOTAA64.EFI", "linuxaa64.efi.stub"},
	"riscv64": {"riscv64", "BOOTRISCV64.EFI", "linuxriscv64.efi.stub"},
}

// Opts is the resolved input to Build. Cobra populates it from the flags
// below; tests can drive Build() directly with a struct literal.
type Opts struct {
	Arch                                                                        ArchProfile
	Kernel, Stub, Cmdline, Extra, Image, PlanRef, Target, Uname, Out, CosignKey string
	WorkDir                                                                     string
	Keep, Insec, Verbose                                                        bool
}

// Cmd returns the `cloud-boot build` cobra subcommand.
func Cmd() *cobra.Command {
	var (
		arch      string
		kernel    string
		stub      string
		cmdline   string
		image     string
		planRef   string
		target    string
		extra     string
		uname     string
		out       string
		workDir   string
		keep      bool
		insec     bool
		verbose   bool
		cosignKey string
	)
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Assemble a bootable UEFI UKI/ISO",
		Long: `Cross-compile cloud-boot-init for the target arch, pack it into an
initramfs, assemble a UKI (.linux + .initrd + .cmdline + …) via the
go-coff/pe library, stage a FAT16 EFI System Partition, and produce a
UEFI-bootable El Torito ISO with xorriso.

Exactly one of --plan or --image must be set. --plan is the recommended
path; --image keeps the legacy single-manifest flow.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			prof, ok := ArchProfiles[arch]
			if !ok {
				return fmt.Errorf("unsupported --arch %q (allowed: amd64, arm64, riscv64)", arch)
			}
			if kernel == "" {
				return fmt.Errorf("--kernel is required")
			}
			if (planRef == "") == (image == "") {
				return fmt.Errorf("exactly one of --plan or --image must be set")
			}
			if stub == "" {
				stub = defaultStub(prof.StubName)
			}
			if stub == "" {
				return fmt.Errorf("UEFI stub not found for %s; pass --stub <%s>", arch, prof.StubName)
			}
			return Build(Opts{
				Arch: prof, Kernel: kernel, Stub: stub, Cmdline: cmdline,
				Extra: extra, Image: image, PlanRef: planRef, Target: target,
				Uname: uname, Out: out, WorkDir: workDir, Keep: keep,
				Insec: insec, Verbose: verbose, CosignKey: cosignKey,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&arch, "arch", "amd64", "target arch (amd64|arm64|riscv64)")
	f.StringVar(&kernel, "kernel", "", "path to host vmlinuz for the target arch (with EFI stub + virtio)")
	f.StringVar(&stub, "stub", "", "path to systemd UEFI stub (auto-detected per arch)")
	f.StringVar(&cmdline, "cmdline", "console=ttyS0", "kernel cmdline (augmented with cloudboot.* keys; 'quiet' injected unless --verbose)")
	f.StringVar(&image, "image", "", "OCI ref of a single image (legacy mode)")
	f.StringVar(&planRef, "plan", "", "OCI ref of an HCL boot plan (preferred)")
	f.StringVar(&target, "target", "", "plan target name (sets cloudboot.target=)")
	f.StringVar(&extra, "extra-cmdline", "", "additional cmdline parameters")
	f.StringVar(&uname, "uname", "6.6.0-cloud-boot", "kernel version label embedded in UKI .uname")
	f.StringVarP(&out, "output", "o", "boot.iso", "output ISO path")
	f.StringVar(&workDir, "work", "", "build workdir (default: tempdir)")
	f.BoolVar(&keep, "keep", false, "keep workdir on success")
	f.BoolVar(&insec, "insecure", false, "pass cloudboot.insecure=1 to the init (allow http registry)")
	f.BoolVarP(&verbose, "verbose", "v", false, "skip the automatic 'quiet' injection (full kernel boot log)")
	f.StringVar(&cosignKey, "cosign-key", "", "PEM-encoded cosign public key; embedded in initramfs")
	return cmd
}
