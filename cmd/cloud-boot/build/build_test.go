package build

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoinWords(t *testing.T) {
	if got := joinWords(nil); got != "" {
		t.Errorf("nil: %q", got)
	}
	if got := joinWords([]string{""}); got != "" {
		t.Errorf("single empty: %q", got)
	}
	if got := joinWords([]string{"a", "", "b"}); got != "a b" {
		t.Errorf("got %q", got)
	}
	if got := joinWords([]string{"only"}); got != "only" {
		t.Errorf("got %q", got)
	}
}

func TestBuildCmdline(t *testing.T) {
	o := Opts{
		Cmdline: "console=ttyS0",
		PlanRef: "registry/plan:1",
		Target:  "rescue",
		Insec:   true,
		Extra:   "debug",
		Verbose: true, // skip auto-injection so the assertions are precise
	}
	got := BuildCmdline(o)
	for _, want := range []string{"console=ttyS0", "cloudboot.plan=registry/plan:1", "cloudboot.target=rescue", "cloudboot.insecure=1", "debug"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildCmdline_ImageMode(t *testing.T) {
	got := BuildCmdline(Opts{Cmdline: "x", Image: "img:tag", Verbose: true})
	if !strings.Contains(got, "cloudboot.image=img:tag") {
		t.Errorf("got %q", got)
	}
}

func TestBuildCmdline_NoOptions(t *testing.T) {
	// Verbose=true → no auto-injection, just the base cmdline.
	got := BuildCmdline(Opts{Cmdline: "x", Verbose: true})
	if got != "x" {
		t.Errorf("got %q", got)
	}
}

func TestBuildCmdline_QuietByDefault(t *testing.T) {
	// Verbose left at zero value → quiet injected.
	got := BuildCmdline(Opts{Cmdline: "console=ttyS0"})
	for _, want := range []string{"console=ttyS0", "quiet"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildCmdline_VerboseSkipsQuiet(t *testing.T) {
	got := BuildCmdline(Opts{Cmdline: "console=ttyS0", Verbose: true})
	if strings.Contains(got, "quiet") {
		t.Errorf("verbose=true should drop \"quiet\", got %q", got)
	}
}

func TestDefaultStub_Missing(t *testing.T) {
	if got := defaultStub("definitely-not-a-real-stub.efi.stub"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestDefaultStub_Found(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "linuxx64.efi.stub")
	os.WriteFile(stub, []byte{0}, 0o644)
	prev := DefaultStubRoots
	DefaultStubRoots = []string{dir}
	t.Cleanup(func() { DefaultStubRoots = prev })
	if got := defaultStub("linuxx64.efi.stub"); got != stub {
		t.Errorf("got %q, want %q", got, stub)
	}
}

func TestTruncate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x")
	if err := truncate(p, 4096); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Size() != 4096 {
		t.Errorf("size = %d", st.Size())
	}
}

func TestTruncate_BadPath(t *testing.T) {
	if err := truncate("/no/such/path/x", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_NonexistentBinary(t *testing.T) {
	if err := run("definitely-not-a-real-binary-12345"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildInitramfs(t *testing.T) {
	dir := t.TempDir()
	initBin := filepath.Join(dir, "init")
	if err := os.WriteFile(initBin, []byte("FAKE-INIT"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ramfs.cpio.gz")
	if err := buildInitramfs(initBin, out, []byte("cosign-pem")); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(out)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	raw, _ := io.ReadAll(gz)
	for _, want := range []string{"FAKE-INIT", "cosign-pem", "init", "etc/cosign.pub"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in cpio output", want)
		}
	}
}

func TestBuildInitramfs_NoCosign(t *testing.T) {
	dir := t.TempDir()
	initBin := filepath.Join(dir, "init")
	os.WriteFile(initBin, []byte("X"), 0o755)
	if err := buildInitramfs(initBin, filepath.Join(dir, "r.cpio.gz"), nil); err != nil {
		t.Fatal(err)
	}
}

func TestBuildInitramfs_ReadInitBinFails(t *testing.T) {
	dir := t.TempDir()
	if err := buildInitramfs(filepath.Join(dir, "missing"), filepath.Join(dir, "out"), nil); err == nil {
		t.Fatal("expected error")
	}
}

// withStubs swaps every external-tool indirection for a deterministic mock
// for the duration of the test.
func withStubs(t *testing.T, runFn func(string, ...string) error) {
	t.Helper()
	prevLP := LookPath
	prevRun := CmdRun
	prevBI := BuildInitFn
	LookPath = func(string) (string, error) { return "/bin/true", nil }
	CmdRun = runFn
	BuildInitFn = func(out, _ string) error {
		return os.WriteFile(out, []byte("FAKE-INIT"), 0o755)
	}
	t.Cleanup(func() {
		LookPath = prevLP
		CmdRun = prevRun
		BuildInitFn = prevBI
	})
}

func TestBuild_FullStubbedPath(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	kernel := filepath.Join(dir, "vmlinuz")
	os.WriteFile(kernel, []byte("KERNEL"), 0o644)
	out := filepath.Join(dir, "out.iso")

	calls := 0
	withStubs(t, func(name string, args ...string) error {
		calls++
		return nil
	})

	if err := Build(Opts{
		Arch:   ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: kernel, Stub: stub, Image: "x:y", Out: out, Insec: true, Extra: "loglevel=3", Uname: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Error("expected CmdRun to be invoked at least once")
	}
}

func TestBuild_BuildInitFails(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	prevLP := LookPath
	prevBI := BuildInitFn
	LookPath = func(string) (string, error) { return "/bin/true", nil }
	BuildInitFn = func(string, string) error { return errString("boom") }
	t.Cleanup(func() {
		LookPath = prevLP
		BuildInitFn = prevBI
	})
	err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: filepath.Join(dir, "k"), Stub: stub, Image: "x:y", Out: filepath.Join(dir, "out.iso"),
	})
	if err == nil {
		t.Fatal("expected buildInit error")
	}
}

func TestBuild_CosignKeyMissing(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	withStubs(t, func(string, ...string) error { return nil })
	err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel:    filepath.Join(dir, "k"),
		Stub:      stub,
		Image:     "x:y",
		Out:       filepath.Join(dir, "out.iso"),
		CosignKey: filepath.Join(dir, "missing-key.pem"),
	})
	if err == nil {
		t.Fatal("expected cosign-read error")
	}
}

func TestBuild_CosignKeyEmbedded(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	keyPath := filepath.Join(dir, "cosign.pub")
	os.WriteFile(keyPath, []byte("-----BEGIN PUBLIC KEY-----\n"), 0o644)
	kernel := filepath.Join(dir, "vmlinuz")
	os.WriteFile(kernel, []byte("K"), 0o644)
	withStubs(t, func(string, ...string) error { return nil })
	if err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: kernel, Stub: stub, Image: "x:y", Out: filepath.Join(dir, "out.iso"),
		CosignKey: keyPath, PlanRef: "p:t", Target: "primary",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_MkdirAllFails(t *testing.T) {
	dir := t.TempDir()
	clash := filepath.Join(dir, "clash")
	os.WriteFile(clash, []byte("x"), 0o644)
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	withStubs(t, func(string, ...string) error { return nil })
	err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: filepath.Join(dir, "k"), Stub: stub, Image: "x:y",
		Out: filepath.Join(dir, "out.iso"), WorkDir: filepath.Join(clash, "wd"),
	})
	if err == nil {
		t.Fatal("expected mkdir-all error")
	}
}

func TestBuild_UKIBuildFails(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, []byte("not pe"), 0o644)
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	withStubs(t, func(string, ...string) error { return nil })
	err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: kernel, Stub: stub, Image: "x:y", Out: filepath.Join(dir, "out.iso"),
	})
	if err == nil {
		t.Fatal("expected uki.Build error")
	}
}

func TestBuild_ESPFails(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	withStubs(t, func(name string, _ ...string) error {
		if name == "mformat" {
			return errString("mformat boom")
		}
		return nil
	})
	err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: kernel, Stub: stub, Image: "x:y", Out: filepath.Join(dir, "out.iso"),
	})
	if err == nil {
		t.Fatal("expected ESP build error")
	}
}

func TestBuild_ISOFails(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	os.WriteFile(stub, minimalPE(), 0o644)
	kernel := filepath.Join(dir, "k")
	os.WriteFile(kernel, []byte("K"), 0o644)
	withStubs(t, func(name string, _ ...string) error {
		if name == "xorriso" {
			return errString("xorriso boom")
		}
		return nil
	})
	err := Build(Opts{
		Arch: ArchProfile{GoArch: "amd64", EFIName: "BOOTX64.EFI", StubName: "linuxx64.efi.stub"},
		Kernel: kernel, Stub: stub, Image: "x:y", Out: filepath.Join(dir, "out.iso"),
	})
	if err == nil {
		t.Fatal("expected ISO build error")
	}
}

func TestBuildInit_GoNotFound(t *testing.T) {
	if err := buildInit("/no/such/dir/out", "amd64"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildESP_Wiring(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "esp.img")
	calls := 0
	prev := CmdRun
	CmdRun = func(string, ...string) error { calls++; return nil }
	t.Cleanup(func() { CmdRun = prev })
	if err := buildESP("efi", "BOOTX64.EFI", out); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Errorf("CmdRun calls = %d, want 4", calls)
	}
}

func TestBuildISO_Wiring(t *testing.T) {
	dir := t.TempDir()
	esp := filepath.Join(dir, "efiboot.img")
	os.WriteFile(esp, []byte("FAT"), 0o644)
	prev := CmdRun
	CmdRun = func(string, ...string) error { return nil }
	t.Cleanup(func() { CmdRun = prev })
	if err := buildISO(esp, filepath.Join(dir, "out.iso")); err != nil {
		t.Fatal(err)
	}
}

func TestBuildESP_TruncateFails(t *testing.T) {
	if err := buildESP("efi", "BOOTX64.EFI", "/no/such/path/esp.img"); err == nil {
		t.Fatal("expected truncate error")
	}
}

func TestBuildISO_CopyFails(t *testing.T) {
	dir := t.TempDir()
	prev := CmdRun
	CmdRun = func(string, ...string) error { return nil }
	t.Cleanup(func() { CmdRun = prev })
	if err := buildISO(filepath.Join(dir, "missing.img"), filepath.Join(dir, "out.iso")); err == nil {
		t.Fatal("expected copy error")
	}
}

func TestLocateInitModule_FromEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x"), 0o644)
	t.Setenv("CLOUD_BOOT_INIT_DIR", dir)
	got, err := locateInitModule()
	if err != nil || got != dir {
		t.Errorf("got %q,%v", got, err)
	}
}

// errString is a sentinel error type used to drive the stubbed CmdRun.
type errString string

func (e errString) Error() string { return string(e) }

// minimalPE is the smallest PE32+ that pe.Append accepts.
func minimalPE() []byte {
	const (
		dosSize       = 0x40
		optSize       = 240
		secTableSlots = 8
		fileAlign     = 512
		sectionAlign  = 0x1000
	)
	headerEnd := dosSize + 4 + 20 + optSize + secTableSlots*40
	sizeOfHeaders := uint32(headerEnd)
	if r := sizeOfHeaders % fileAlign; r != 0 {
		sizeOfHeaders += fileAlign - r
	}
	textRaw := uint32(fileAlign)
	buf := make([]byte, sizeOfHeaders+textRaw)
	buf[0], buf[1] = 'M', 'Z'
	put32(buf[0x3C:], dosSize)
	copy(buf[dosSize:dosSize+4], []byte("PE\x00\x00"))
	coff := dosSize + 4
	put16(buf[coff:], 0x8664)
	put16(buf[coff+2:], 1)
	put16(buf[coff+16:], optSize)
	put16(buf[coff+18:], 0x002E)
	opt := coff + 20
	put16(buf[opt:], 0x020B)
	put32(buf[opt+32:], sectionAlign)
	put32(buf[opt+36:], fileAlign)
	put32(buf[opt+56:], sectionAlign+0x1000)
	put32(buf[opt+60:], sizeOfHeaders)
	put32(buf[opt+108:], 16)
	sec := opt + optSize
	copy(buf[sec:sec+8], []byte(".text"))
	put32(buf[sec+8:], 16)
	put32(buf[sec+12:], sectionAlign)
	put32(buf[sec+16:], textRaw)
	put32(buf[sec+20:], sizeOfHeaders)
	put32(buf[sec+36:], 0x60000040)
	for i := range buf[sizeOfHeaders : sizeOfHeaders+16] {
		buf[int(sizeOfHeaders)+i] = 0x90
	}
	return buf
}

func put16(b []byte, v uint16) { b[0] = byte(v); b[1] = byte(v >> 8) }
func put32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
