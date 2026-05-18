// Package multi implements `cloud-boot push multi`: push one per-arch
// manifest per platform and assemble the multi-arch image index in a single
// invocation.
//
// Without this, a typical multi-arch upload is:
//
//	cloud-boot push artifact --platform linux/amd64 --kernel … repo:tag-amd64
//	cloud-boot push artifact --platform linux/arm64 --kernel … repo:tag-arm64
//	cloud-boot push index    --platform linux/amd64=repo:tag-amd64 \
//	                         --platform linux/arm64=repo:tag-arm64  repo:tag
//
// `push multi` collapses the three calls into one. Each artifact-kind flag
// (--kernel, --initrd, --modules) is repeatable and takes "<os/arch>=<path>".
// The set of arches is the union of platforms seen across the three flags.
// Each per-arch manifest is pushed under <ref>-<arch> (the user-supplied
// reference suffixed with the arch); the index lands at the bare <ref>.
package multi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloud-boot/init/pkg/oci"
	"github.com/cloud-boot/uki/cmd/cloud-boot/internal/cliutil"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push/artifact"
	"github.com/cloud-boot/uki/cmd/cloud-boot/push/index"
)

// PlatformPath is one --kernel/--initrd/--modules entry.
type PlatformPath struct{ Platform, Path string }

// PathList collects repeated <os/arch>=<path> flags. Implements pflag.Value
// + pflag.SliceValue.
type PathList []PlatformPath

func (l *PathList) String() string { return fmt.Sprintf("%v", *l) }
func (l *PathList) Set(v string) error {
	kv := strings.SplitN(v, "=", 2)
	if len(kv) != 2 || kv[0] == "" || kv[1] == "" {
		return fmt.Errorf("expected <os/arch>=<path>, got %q", v)
	}
	*l = append(*l, PlatformPath{Platform: kv[0], Path: kv[1]})
	return nil
}
func (l *PathList) Type() string          { return "platform=path" }
func (l *PathList) Append(v string) error { return l.Set(v) }
func (l *PathList) Replace(vals []string) error {
	*l = (*l)[:0]
	for _, v := range vals {
		if err := l.Set(v); err != nil {
			return err
		}
	}
	return nil
}
func (l *PathList) GetSlice() []string {
	out := make([]string, len(*l))
	for i, p := range *l {
		out[i] = p.Platform + "=" + p.Path
	}
	return out
}

// pathFor returns the path registered for platform, or "" if none.
func (l PathList) pathFor(platform string) string {
	for _, p := range l {
		if p.Platform == platform {
			return p.Path
		}
	}
	return ""
}

// Opts is the resolved input.
type Opts struct {
	Ref     string
	Kernels PathList
	Initrds PathList
	Modules PathList
	Cmdline string
}

// Cmd returns the cobra subcommand.
func Cmd() *cobra.Command {
	var o Opts
	cmd := &cobra.Command{
		Use:   "multi <ref>",
		Short: "Push per-arch manifests and a multi-arch index in one command",
		Long: `Push one OCI manifest per platform and assemble an image index in a
single invocation. The set of platforms is the union of --kernel,
--initrd, and --modules entries. Each per-arch manifest is pushed under
"<ref>-<arch>"; the multi-arch index lands at "<ref>".

Examples:

  cloud-boot push multi registry/boot/linux:6.6 \
    --kernel  linux/amd64=bzImage-amd64 --kernel  linux/arm64=Image-arm64 \
    --initrd  linux/amd64=initrd-amd64  --initrd  linux/arm64=initrd-arm64 \
    --modules linux/amd64=mods-amd64    --modules linux/arm64=mods-arm64 \
    --cmdline "console=ttyS0 ro"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Ref = args[0]
			return Run(o)
		},
	}
	f := cmd.Flags()
	f.VarP(&o.Kernels, "kernel", "k", "<os/arch>=<path> (repeatable)")
	f.VarP(&o.Initrds, "initrd", "i", "<os/arch>=<path> (repeatable)")
	f.Var(&o.Modules, "modules", "<os/arch>=<path> (repeatable)")
	f.StringVar(&o.Cmdline, "cmdline", "", "kernel cmdline payload (shared across all arches)")
	return cmd
}

// Run pushes every per-arch manifest, then assembles the index. Exposed so
// tests can drive it without going through cobra.
func Run(o Opts) error {
	platforms := uniquePlatforms(o.Kernels, o.Initrds, o.Modules)
	if len(platforms) == 0 {
		return fmt.Errorf("multi: at least one --kernel/--initrd/--modules entry is required")
	}
	if _, err := oci.ParseRef(o.Ref); err != nil {
		return err
	}

	indexOpts := index.Opts{OutRef: o.Ref}
	for _, platform := range platforms {
		archRef, err := derivePerArchRef(o.Ref, platform)
		if err != nil {
			return err
		}
		if err := artifact.Run(artifact.Opts{
			Platform: platform,
			Kernel:   o.Kernels.pathFor(platform),
			Initrd:   o.Initrds.pathFor(platform),
			Modules:  o.Modules.pathFor(platform),
			Cmdline:  o.Cmdline,
			Ref:      archRef,
		}); err != nil {
			return fmt.Errorf("push %s: %w", platform, err)
		}
		if err := indexOpts.Platforms.Set(platform + "=" + archRef); err != nil {
			return err
		}
	}
	return index.Run(indexOpts)
}

// derivePerArchRef rewrites the reference (tag) of refStr by appending the
// arch: "registry/repo:6.6" + linux/amd64 → "registry/repo:6.6-amd64".
func derivePerArchRef(refStr, platform string) (string, error) {
	_, arch, err := cliutil.ParsePlatform(platform)
	if err != nil {
		return "", err
	}
	r, err := oci.ParseRef(refStr)
	if err != nil {
		return "", err
	}
	// Reconstruct as <host>/<repo>:<tag>-<arch>; ParseRef will infer the
	// scheme again from the host on its next pass.
	return fmt.Sprintf("%s/%s:%s-%s", r.Host, r.Repo, r.Reference, arch), nil
}

// uniquePlatforms returns the set of platforms seen across one or more
// PathList values, sorted for deterministic test output.
func uniquePlatforms(lists ...PathList) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, l := range lists {
		for _, p := range l {
			if _, ok := seen[p.Platform]; !ok {
				seen[p.Platform] = struct{}{}
				out = append(out, p.Platform)
			}
		}
	}
	sort.Strings(out)
	return out
}
