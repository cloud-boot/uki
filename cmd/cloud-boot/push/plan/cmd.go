// Package plan implements `cloud-boot push plan`: upload an HCL boot plan
// to a registry as a single-layer OCI artifact.
package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloud-boot/init/pkg/oci"
)

type Opts struct {
	File string
	Ref  string
}

func Cmd() *cobra.Command {
	var o Opts
	cmd := &cobra.Command{
		Use:   "plan <ref>",
		Short: "Push an HCL boot plan as a single-layer artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Ref = args[0]
			return Run(o)
		},
	}
	cmd.Flags().StringVar(&o.File, "file", "", "path to plan.hcl (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func Run(o Opts) error {
	if o.File == "" {
		return fmt.Errorf("plan: --file is required")
	}
	ref, err := oci.ParseRef(o.Ref)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(o.File)
	if err != nil {
		return err
	}
	c := oci.NewClient()

	// image-spec requires *some* config blob; emit a minimal one.
	cfgJSON, _ := json.Marshal(map[string]any{"created": nil})
	cfgDigest, err := c.PushBlob(ref, cfgJSON)
	if err != nil {
		return fmt.Errorf("push config: %w", err)
	}

	planDigest, err := c.PushBlob(ref, body)
	if err != nil {
		return fmt.Errorf("push plan: %w", err)
	}

	m := &ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: oci.MediaTypeConfig,
			Digest:    cfgDigest,
			Size:      int64(len(cfgJSON)),
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType:   oci.MediaTypePlan,
				Digest:      planDigest,
				Size:        int64(len(body)),
				Annotations: map[string]string{ocispec.AnnotationTitle: filepath.Base(o.File)},
			},
		},
	}
	m.SchemaVersion = 2
	mDigest, err := c.PushManifest(ref, m)
	if err != nil {
		return fmt.Errorf("push manifest: %w", err)
	}
	fmt.Printf("%s://%s/%s@%s\n", ref.Scheme, ref.Host, ref.Repo, mDigest)
	return nil
}
