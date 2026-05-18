package multi

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloud-boot/uki/cmd/cloud-boot/internal/fakereg"
)

func TestPathList_Set(t *testing.T) {
	var l PathList
	if err := l.Set("linux/amd64=/path/k"); err != nil {
		t.Fatal(err)
	}
	if err := l.Set("malformed"); err == nil {
		t.Error("expected error for malformed entry")
	}
	if err := l.Set("=missing-platform"); err == nil {
		t.Error("expected error for empty platform")
	}
	if err := l.Set("linux/amd64="); err == nil {
		t.Error("expected error for empty path")
	}
	if len(l) != 1 || l[0].Platform != "linux/amd64" || l[0].Path != "/path/k" {
		t.Errorf("unexpected list: %+v", l)
	}
	if l.Type() == "" || l.String() == "" {
		t.Error("Type or String returned empty")
	}
}

func TestPathList_Replace(t *testing.T) {
	var l PathList
	_ = l.Set("linux/amd64=a")
	if err := l.Replace([]string{"linux/arm64=b", "linux/riscv64=c"}); err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 || l[0].Platform != "linux/arm64" || l[1].Path != "c" {
		t.Errorf("got %+v", l)
	}
	if got := l.GetSlice(); len(got) != 2 || got[0] != "linux/arm64=b" {
		t.Errorf("GetSlice = %v", got)
	}
}

func TestPathList_Replace_BadEntry(t *testing.T) {
	var l PathList
	if err := l.Replace([]string{"malformed"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestPathList_PathFor(t *testing.T) {
	l := PathList{{"linux/amd64", "a"}, {"linux/arm64", "b"}}
	if got := l.pathFor("linux/arm64"); got != "b" {
		t.Errorf("got %q", got)
	}
	if got := l.pathFor("linux/riscv64"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestUniquePlatforms(t *testing.T) {
	a := PathList{{"linux/amd64", "k1"}, {"linux/arm64", "k2"}}
	b := PathList{{"linux/arm64", "i2"}}
	c := PathList{{"linux/riscv64", "m3"}}
	got := uniquePlatforms(a, b, c)
	want := []string{"linux/amd64", "linux/arm64", "linux/riscv64"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestDerivePerArchRef(t *testing.T) {
	got, err := derivePerArchRef("registry.example.com/boot/linux:6.6", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != "registry.example.com/boot/linux:6.6-amd64" {
		t.Errorf("got %q", got)
	}
}

func TestDerivePerArchRef_BadPlatform(t *testing.T) {
	if _, err := derivePerArchRef("r/r:t", "bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDerivePerArchRef_BadRef(t *testing.T) {
	if _, err := derivePerArchRef("noslash", "linux/amd64"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_NoEntries(t *testing.T) {
	if err := Run(Opts{Ref: "registry/r:t"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_BadRef(t *testing.T) {
	o := Opts{Ref: "noslash"}
	_ = o.Kernels.Set("linux/amd64=/k")
	if err := Run(o); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_End2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	kAmd := filepath.Join(dir, "k-amd64")
	kArm := filepath.Join(dir, "k-arm64")
	iAmd := filepath.Join(dir, "i-amd64")
	for _, p := range []string{kAmd, kArm, iAmd} {
		os.WriteFile(p, []byte(filepath.Base(p)), 0o644)
	}
	o := Opts{
		Ref:     reg.RepoRef(t, "6.6"),
		Cmdline: "console=ttyS0",
	}
	_ = o.Kernels.Set("linux/amd64=" + kAmd)
	_ = o.Kernels.Set("linux/arm64=" + kArm)
	_ = o.Initrds.Set("linux/amd64=" + iAmd)

	if err := Run(o); err != nil {
		t.Fatal(err)
	}
	// Three manifests should be visible: amd64, arm64, and the index.
	u, _ := url.Parse(reg.Server.URL)
	_ = u
	if len(reg.Manifests) < 3 {
		t.Errorf("expected at least 3 manifests, got %d (%v)", len(reg.Manifests), keys(reg.Manifests))
	}
	// The index should live at the bare tag and be a multi-arch index.
	idxBody := reg.Manifests["/v2/r/manifests/6.6"]
	if idxBody == nil {
		t.Fatalf("index not stored at /v2/r/manifests/6.6; saw %v", keys(reg.Manifests))
	}
	var idx struct {
		MediaType string                    `json:"mediaType"`
		Manifests []ocispec.Descriptor      `json:"manifests"`
	}
	if err := json.Unmarshal(idxBody, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.MediaType != ocispec.MediaTypeImageIndex {
		t.Errorf("mediaType = %q", idx.MediaType)
	}
	if len(idx.Manifests) != 2 {
		t.Errorf("index has %d entries", len(idx.Manifests))
	}
	for _, m := range idx.Manifests {
		if m.Platform == nil {
			t.Error("index entry missing platform")
			continue
		}
		if m.Platform.OS != "linux" {
			t.Errorf("os = %q", m.Platform.OS)
		}
		if m.Platform.Architecture != "amd64" && m.Platform.Architecture != "arm64" {
			t.Errorf("arch = %q", m.Platform.Architecture)
		}
	}
}

func TestCmd_End2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	k := filepath.Join(dir, "k")
	os.WriteFile(k, []byte("K"), 0o644)
	c := Cmd()
	c.SetArgs([]string{
		"--kernel", "linux/amd64=" + k,
		"--kernel", "linux/arm64=" + k,
		reg.RepoRef(t, "tag"),
	})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Sanity check: an empty Ref is rejected (ParseRef returns an error before
// we hit the platform check).
func TestRun_EmptyRef(t *testing.T) {
	o := Opts{}
	_ = o.Kernels.Set("linux/amd64=/k")
	err := Run(o)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing repository") && err.Error() != "" {
		// Either error wording is acceptable; just confirm one occurred.
	}
}
