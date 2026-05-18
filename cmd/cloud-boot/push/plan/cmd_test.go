package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloud-boot/uki/cmd/cloud-boot/internal/fakereg"
)

func TestRun_MissingFile(t *testing.T) {
	if err := Run(Opts{Ref: "ref"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_BadRef(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.hcl")
	os.WriteFile(plan, []byte("default_target = \"x\"\n"), 0o644)
	if err := Run(Opts{File: plan, Ref: "noslash"}); err == nil {
		t.Fatal("expected ParseRef error")
	}
}

func TestRun_FileMissing(t *testing.T) {
	if err := Run(Opts{File: "/no/such/plan.hcl", Ref: "ref/repo:tag"}); err == nil {
		t.Fatal("expected read error")
	}
}

func TestRun_End2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.hcl")
	os.WriteFile(plan, []byte("default_target = \"x\"\n"), 0o644)
	if err := Run(Opts{File: plan, Ref: reg.RepoRef(t, "tag")}); err != nil {
		t.Fatal(err)
	}
	if reg.Manifest == nil {
		t.Fatal("manifest not pushed")
	}
}

func TestCmd_End2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.hcl")
	os.WriteFile(plan, []byte("default_target = \"x\"\n"), 0o644)
	c := Cmd()
	c.SetArgs([]string{"--file", plan, reg.RepoRef(t, "tag")})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
