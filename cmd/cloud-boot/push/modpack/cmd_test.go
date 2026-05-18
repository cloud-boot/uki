package modpack

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_MissingArgs(t *testing.T) {
	if err := Run(Opts{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_End2End(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "6.6.0-test")
	if err := os.MkdirAll(filepath.Join(src, "kernel/drivers/net"), 0o755); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(src, "kernel/drivers/net/virtio_net.ko")
	os.WriteFile(mod, []byte("FAKE-MODULE"), 0o644)
	os.Symlink("virtio_net.ko", filepath.Join(src, "kernel/drivers/net/alias.ko"))

	out := filepath.Join(dir, "modules.cpio.gz")
	if err := Run(Opts{Src: src, Out: out}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	raw, _ := io.ReadAll(gz)
	if !strings.Contains(string(raw), "FAKE-MODULE") {
		t.Error("expected module bytes in archive")
	}
}

func TestPackModules_SrcNotDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	os.WriteFile(file, []byte("x"), 0o644)
	if err := packModules(file, filepath.Join(dir, "out.cpio.gz")); err == nil {
		t.Fatal("expected not-a-directory error")
	}
}

func TestPackModules_SrcMissing(t *testing.T) {
	dir := t.TempDir()
	if err := packModules(filepath.Join(dir, "missing"), filepath.Join(dir, "out.cpio.gz")); err == nil {
		t.Fatal("expected error")
	}
}

func TestPackModules_CreateOutFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.MkdirAll(src, 0o755)
	if err := packModules(src, filepath.Join(dir, "no", "such", "out")); err == nil {
		t.Fatal("expected create error")
	}
}

func TestPackModules_BrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "release")
	os.MkdirAll(src, 0o755)
	os.Symlink("/no/such/target", filepath.Join(src, "link"))
	if err := packModules(src, filepath.Join(dir, "out.cpio.gz")); err != nil {
		t.Fatal(err)
	}
}

func TestPackModules_SkipsNonRegular(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "release")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := makeFIFO(filepath.Join(src, "pipe")); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	if err := packModules(src, filepath.Join(dir, "out.cpio.gz")); err != nil {
		t.Fatal(err)
	}
}

func TestCmd_End2End(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "rel")
	os.MkdirAll(src, 0o755)
	out := filepath.Join(dir, "m.cpio.gz")
	c := Cmd()
	c.SetArgs([]string{"--src", src, "--output", out})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
