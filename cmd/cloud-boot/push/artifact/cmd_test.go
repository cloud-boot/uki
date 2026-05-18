package artifact

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloud-boot/init/pkg/oci"
	"github.com/cloud-boot/uki/cmd/cloud-boot/internal/fakereg"
)

func TestRun_MissingPlatform(t *testing.T) {
	if err := Run(Opts{Ref: "ref"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_NothingToPush(t *testing.T) {
	if err := Run(Opts{Platform: "linux/amd64", Ref: "ref"}); err == nil {
		t.Fatal("expected nothing-to-push error")
	}
}

func TestRun_InvalidPlatform(t *testing.T) {
	dir := t.TempDir()
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	if err := Run(Opts{Platform: "bad", Kernel: kernel, Ref: "ref"}); err == nil {
		t.Fatal("expected platform error")
	}
}

func TestRun_BadRef(t *testing.T) {
	dir := t.TempDir()
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	if err := Run(Opts{Platform: "linux/amd64", Kernel: kernel, Ref: "noslash"}); err == nil {
		t.Fatal("expected ParseRef error")
	}
}

func TestRun_End2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	kernel := filepath.Join(dir, "vmlinuz")
	initrd := filepath.Join(dir, "initrd")
	modules := filepath.Join(dir, "modules.cpio.gz")
	for _, p := range []string{kernel, initrd, modules} {
		os.WriteFile(p, []byte(filepath.Base(p)), 0o644)
	}
	if err := Run(Opts{
		Platform: "linux/amd64",
		Kernel:   kernel, Initrd: initrd, Modules: modules,
		Cmdline: "console=ttyS0",
		Ref:     reg.RepoRef(t, "tag"),
	}); err != nil {
		t.Fatal(err)
	}
	if reg.Manifest == nil {
		t.Fatal("expected manifest to be pushed")
	}
}

func TestRun_KernelOnlyEnd2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	kernel := filepath.Join(dir, "vmlinuz")
	os.WriteFile(kernel, []byte("K"), 0o644)
	if err := Run(Opts{Platform: "linux/amd64", Kernel: kernel, Ref: reg.RepoRef(t, "tag")}); err != nil {
		t.Fatal(err)
	}
}

func TestRun_KernelMissing(t *testing.T) {
	dir := t.TempDir()
	err := Run(Opts{Platform: "linux/amd64", Kernel: filepath.Join(dir, "nope"), Ref: "ref/repo:tag"})
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestRun_KernelPushFails(t *testing.T) {
	srv := fakereg.StatefulFailReg(t, 2) // config OK, kernel push fails
	u, _ := url.Parse(srv.URL)
	dir := t.TempDir()
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	err := Run(Opts{Platform: "linux/amd64", Kernel: kernel, Ref: u.Host + "/r:tag"})
	if err == nil {
		t.Fatal("expected kernel push error")
	}
}

func TestRun_ConfigPushFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed := srv.URL
	srv.Close()
	u, _ := url.Parse(closed)
	dir := t.TempDir()
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	err := Run(Opts{Platform: "linux/amd64", Kernel: kernel, Ref: u.Host + "/r:tag"})
	if err == nil {
		t.Fatal("expected push-config error")
	}
}

// pushFile is private; cover its two error paths via Opts.
func TestPushFile_ReadFails(t *testing.T) {
	dir := t.TempDir()
	c := oci.NewClient()
	ref := &oci.Ref{Scheme: "http", Host: "x", Repo: "r"}
	if _, err := pushFile(c, ref, filepath.Join(dir, "missing"), "media", "title"); err == nil {
		t.Fatal("expected read error")
	}
}

func TestPushFile_PushFails(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data")
	os.WriteFile(file, []byte("x"), 0o644)
	c := oci.NewClient()
	ref := &oci.Ref{Scheme: "http", Host: "127.0.0.1:1", Repo: "r"}
	if _, err := pushFile(c, ref, file, "media", "title"); err == nil {
		t.Fatal("expected push error")
	}
}

// Cobra-integration smoke test: drive the cmd by SetArgs.
func TestCmd_End2End(t *testing.T) {
	reg := fakereg.Start(t)
	dir := t.TempDir()
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	c := Cmd()
	c.SetArgs([]string{"--platform", "linux/amd64", "--kernel", kernel, reg.RepoRef(t, "tag")})
	c.SetOut(os.Stderr) // discard isn't required; just keep test output quiet
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
