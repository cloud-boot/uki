// Package artifact implements `cloud-boot push artifact`: assemble a
// single-arch OCI image manifest from a vmlinuz / initrd / modules / cmdline
// tuple and push it to a registry.
package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloud-boot/init/pkg/oci"
	"github.com/cloud-boot/uki/cmd/cloud-boot/internal/cliutil"
)

// Opts is the resolved input to Run. Cobra populates it from the flags;
// tests can drive Run() directly with a struct literal.
type Opts struct {
	Platform string
	Kernel   string
	Initrd   string
	Modules  string
	Modloop  string
	Apkovl   string
	Cmdline  string
	Ref      string
}

// Cmd returns the cobra subcommand.
func Cmd() *cobra.Command {
	var o Opts
	cmd := &cobra.Command{
		Use:   "artifact <ref>",
		Short: "Push a single-arch kernel/initrd/modules/cmdline manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Ref = args[0]
			return Run(o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Platform, "platform", "", "OS/arch e.g. linux/amd64 (required)")
	f.StringVar(&o.Kernel, "kernel", "", "path to vmlinuz (optional)")
	f.StringVar(&o.Initrd, "initrd", "", "path to initramfs (optional)")
	f.StringVar(&o.Modules, "modules", "", "path to modules cpio.gz (optional)")
	f.StringVar(&o.Modloop, "modloop", "", "path to a raw squashfs blob (Alpine modloop-virt or equivalent; optional)")
	f.StringVar(&o.Apkovl, "apkovl", "", "path to an Alpine apkovl tar.gz overlay (optional)")
	f.StringVar(&o.Cmdline, "cmdline", "", "kernel cmdline payload (optional)")
	_ = cmd.MarkFlagRequired("platform")
	return cmd
}

// Run executes the push. It's the business-logic surface tests target.
func Run(o Opts) error {
	if o.Platform == "" {
		return fmt.Errorf("artifact: --platform is required")
	}
	osName, arch, err := cliutil.ParsePlatform(o.Platform)
	if err != nil {
		return err
	}
	if o.Kernel == "" && o.Initrd == "" && o.Modules == "" && o.Modloop == "" && o.Apkovl == "" && o.Cmdline == "" {
		return fmt.Errorf("artifact: at least one of --kernel/--initrd/--modules/--modloop/--apkovl/--cmdline is required")
	}
	ref, err := oci.ParseRef(o.Ref)
	if err != nil {
		return err
	}
	c := oci.NewClient()

	cfg := map[string]any{"architecture": arch, "os": osName, "created": nil}
	cfgJSON, _ := json.Marshal(cfg)
	cfgDigest, err := c.PushBlob(ref, cfgJSON)
	if err != nil {
		return fmt.Errorf("push config: %w", err)
	}
	cfgDesc := ocispec.Descriptor{
		MediaType: oci.MediaTypeConfig,
		Digest:    cfgDigest,
		Size:      int64(len(cfgJSON)),
	}

	var layers []ocispec.Descriptor
	add := func(path, mediaType, title string) error {
		if path == "" {
			return nil
		}
		d, err := pushFile(c, ref, path, mediaType, title)
		if err != nil {
			return err
		}
		layers = append(layers, d)
		return nil
	}
	if err := add(o.Kernel, oci.MediaTypeKernel, "vmlinuz"); err != nil {
		return err
	}
	if err := add(o.Initrd, oci.MediaTypeInitrd, "initrd"); err != nil {
		return err
	}
	if err := add(o.Modules, oci.MediaTypeModules, "modules"); err != nil {
		return err
	}
	if err := add(o.Modloop, oci.MediaTypeModloop, "modloop"); err != nil {
		return err
	}
	if err := add(o.Apkovl, oci.MediaTypeApkovl, "apkovl"); err != nil {
		return err
	}
	if o.Cmdline != "" {
		d, err := c.PushBlob(ref, []byte(o.Cmdline))
		if err != nil {
			return fmt.Errorf("push cmdline: %w", err)
		}
		layers = append(layers, ocispec.Descriptor{
			MediaType:   oci.MediaTypeCmdline,
			Digest:      d,
			Size:        int64(len(o.Cmdline)),
			Annotations: map[string]string{ocispec.AnnotationTitle: "cmdline"},
		})
	}

	m := &ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    cfgDesc,
		Layers:    layers,
	}
	m.SchemaVersion = 2
	mDigest, err := c.PushManifest(ref, m)
	if err != nil {
		return fmt.Errorf("push manifest: %w", err)
	}
	fmt.Printf("%s://%s/%s@%s\n", ref.Scheme, ref.Host, ref.Repo, mDigest)
	return nil
}

// pushFile reads path, uploads it as a blob, and returns a descriptor
// annotated with the file's basename. Kept package-private — artifact is
// the only caller.
func pushFile(c *oci.Client, ref *oci.Ref, path, mediaType, title string) (ocispec.Descriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("read %s: %w", path, err)
	}
	d, err := c.PushBlob(ref, data)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push %s: %w", path, err)
	}
	return ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    d,
		Size:      int64(len(data)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle:    title,
			"cloudboot.source.basename": filepath.Base(path),
		},
	}, nil
}
