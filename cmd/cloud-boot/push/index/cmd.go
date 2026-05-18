// Package index implements `cloud-boot push index`: assemble a multi-arch
// image index that points at existing per-arch manifests.
package index

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloud-boot/init/pkg/oci"
	"github.com/cloud-boot/uki/cmd/cloud-boot/internal/cliutil"
)

// PlatformSpec is one --platform pair: an os/arch glued to an existing
// per-arch manifest reference (typically by digest).
type PlatformSpec struct {
	Platform string
	Ref      string
}

// PlatformList collects repeated --platform flags. Implements pflag.Value
// so cobra can call .Set once per occurrence.
type PlatformList []PlatformSpec

func (l *PlatformList) String() string { return fmt.Sprintf("%v", *l) }
func (l *PlatformList) Set(v string) error {
	kv := strings.SplitN(v, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("expected <os/arch>=<ref>, got %q", v)
	}
	*l = append(*l, PlatformSpec{Platform: kv[0], Ref: kv[1]})
	return nil
}
func (l *PlatformList) Type() string         { return "platform=ref" }
func (l *PlatformList) Append(v string) error { return l.Set(v) }
func (l *PlatformList) Replace(vals []string) error {
	*l = (*l)[:0]
	for _, v := range vals {
		if err := l.Set(v); err != nil {
			return err
		}
	}
	return nil
}
func (l *PlatformList) GetSlice() []string {
	out := make([]string, len(*l))
	for i, p := range *l {
		out[i] = p.Platform + "=" + p.Ref
	}
	return out
}

type Opts struct {
	Platforms PlatformList
	OutRef    string
}

func Cmd() *cobra.Command {
	var o Opts
	cmd := &cobra.Command{
		Use:   "index <out-ref>",
		Short: "Assemble a multi-arch image index from existing per-arch manifests",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.OutRef = args[0]
			return Run(o)
		},
	}
	cmd.Flags().VarP(&o.Platforms, "platform", "p", "<os/arch>=<ref-or-digest> (repeatable)")
	return cmd
}

func Run(o Opts) error {
	if len(o.Platforms) == 0 {
		return fmt.Errorf("index: at least one --platform is required")
	}
	outRef, err := oci.ParseRef(o.OutRef)
	if err != nil {
		return err
	}
	c := oci.NewClient()

	var entries []ocispec.Descriptor
	for _, p := range o.Platforms {
		osName, arch, err := cliutil.ParsePlatform(p.Platform)
		if err != nil {
			return err
		}
		r, err := oci.ParseRef(p.Ref)
		if err != nil {
			return err
		}
		desc, err := c.DescribeManifest(r)
		if err != nil {
			return fmt.Errorf("describe %s: %w", p.Ref, err)
		}
		desc.Platform = &ocispec.Platform{OS: osName, Architecture: arch}
		entries = append(entries, desc)
	}

	idx := &ocispec.Index{
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: entries,
	}
	idx.SchemaVersion = 2
	mDigest, err := c.PushIndex(outRef, idx)
	if err != nil {
		return fmt.Errorf("push index: %w", err)
	}
	fmt.Printf("%s://%s/%s@%s\n", outRef.Scheme, outRef.Host, outRef.Repo, mDigest)
	return nil
}
