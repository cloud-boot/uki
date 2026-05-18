package uki

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	gpe "github.com/go-coff/peln/appender"
)

// minimalStub produces the smallest PE32+ shell that go-coff/pe can append to:
// one `.text` section, plenty of header padding.
func minimalStub(t *testing.T) []byte {
	t.Helper()
	const (
		dosSize       = 0x40
		optSize       = 240
		secTableSlots = 8
		fileAlign     = 512
		sectionAlign  = 0x1000
	)
	headerEnd := dosSize + 4 + 20 + optSize + secTableSlots*40
	sizeOfHeaders := alignUp(uint32(headerEnd), fileAlign)
	textData := bytes.Repeat([]byte{0x90}, 16)
	textRaw := alignUp(uint32(len(textData)), fileAlign)
	textVA := uint32(sectionAlign)

	buf := make([]byte, sizeOfHeaders+textRaw)
	buf[0], buf[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:], dosSize)
	copy(buf[dosSize:dosSize+4], []byte("PE\x00\x00"))
	coff := dosSize + 4
	binary.LittleEndian.PutUint16(buf[coff:], 0x8664)
	binary.LittleEndian.PutUint16(buf[coff+2:], 1)
	binary.LittleEndian.PutUint16(buf[coff+16:], optSize)
	binary.LittleEndian.PutUint16(buf[coff+18:], 0x002E)
	opt := coff + 20
	binary.LittleEndian.PutUint16(buf[opt:], 0x020B)
	binary.LittleEndian.PutUint32(buf[opt+32:], sectionAlign)
	binary.LittleEndian.PutUint32(buf[opt+36:], fileAlign)
	binary.LittleEndian.PutUint32(buf[opt+56:], textVA+alignUp(uint32(len(textData)), sectionAlign))
	binary.LittleEndian.PutUint32(buf[opt+60:], sizeOfHeaders)
	binary.LittleEndian.PutUint32(buf[opt+108:], 16)
	sec := opt + optSize
	copy(buf[sec:sec+8], []byte(".text"))
	binary.LittleEndian.PutUint32(buf[sec+8:], uint32(len(textData)))
	binary.LittleEndian.PutUint32(buf[sec+12:], textVA)
	binary.LittleEndian.PutUint32(buf[sec+16:], textRaw)
	binary.LittleEndian.PutUint32(buf[sec+20:], sizeOfHeaders)
	binary.LittleEndian.PutUint32(buf[sec+36:], gpe.SCN_CNT_INITIALIZED_DATA|gpe.SCN_MEM_READ|gpe.SCN_MEM_EXECUTE)
	copy(buf[sizeOfHeaders:], textData)
	return buf
}

func alignUp(v, a uint32) uint32 {
	if a == 0 {
		return v
	}
	if r := v % a; r != 0 {
		return v + (a - r)
	}
	return v
}

func writeAll(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	linux := filepath.Join(dir, "vmlinuz")
	initrd := filepath.Join(dir, "initrd")
	out := filepath.Join(dir, "uki.efi")

	writeAll(t, stub, minimalStub(t))
	writeAll(t, linux, bytes.Repeat([]byte{0x42}, 512))
	writeAll(t, initrd, []byte("CPIO"))

	if err := Build(Sections{
		Stub: stub, Linux: linux, Initrd: initrd,
		Cmdline: "console=ttyS0",
		OSRel:   "ID=t\n",
		Uname:   "x",
	}, out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	f, err := pe.NewFile(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range f.Sections {
		got[s.Name] = true
	}
	for _, n := range []string{".text", ".osrel", ".cmdline", ".uname", ".linux", ".initrd"} {
		if !got[n] {
			t.Errorf("missing section %s", n)
		}
	}
}

func TestBuild_DefaultOSRel(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	linux := filepath.Join(dir, "vmlinuz")
	initrd := filepath.Join(dir, "initrd")
	out := filepath.Join(dir, "uki.efi")
	writeAll(t, stub, minimalStub(t))
	writeAll(t, linux, []byte{0x01})
	writeAll(t, initrd, []byte{0x02})
	if err := Build(Sections{
		Stub: stub, Linux: linux, Initrd: initrd,
		Cmdline: "", Uname: "",
	}, out); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_StubMissing(t *testing.T) {
	dir := t.TempDir()
	err := Build(Sections{Stub: filepath.Join(dir, "missing")}, filepath.Join(dir, "x.efi"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuild_KernelMissing(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	writeAll(t, stub, minimalStub(t))
	err := Build(Sections{Stub: stub, Linux: filepath.Join(dir, "missing")}, filepath.Join(dir, "x.efi"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuild_InitrdMissing(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	linux := filepath.Join(dir, "vmlinuz")
	writeAll(t, stub, minimalStub(t))
	writeAll(t, linux, []byte{1})
	err := Build(Sections{Stub: stub, Linux: linux, Initrd: filepath.Join(dir, "missing")}, filepath.Join(dir, "x.efi"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuild_AppendError(t *testing.T) {
	// A non-MZ stub fails inside pe.Append and surfaces wrapped error.
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	linux := filepath.Join(dir, "vmlinuz")
	initrd := filepath.Join(dir, "initrd")
	writeAll(t, stub, bytes.Repeat([]byte{0xFF}, 0x80))
	writeAll(t, linux, []byte{1})
	writeAll(t, initrd, []byte{1})
	err := Build(Sections{Stub: stub, Linux: linux, Initrd: initrd}, filepath.Join(dir, "x.efi"))
	if err == nil {
		t.Fatal("expected pe.Append error")
	}
}

func TestBuild_WriteFails(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	linux := filepath.Join(dir, "vmlinuz")
	initrd := filepath.Join(dir, "initrd")
	writeAll(t, stub, minimalStub(t))
	writeAll(t, linux, []byte{1})
	writeAll(t, initrd, []byte{1})
	// Target a path with a non-existent parent dir.
	err := Build(Sections{Stub: stub, Linux: linux, Initrd: initrd}, filepath.Join(dir, "no", "such", "dir", "x.efi"))
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestCopy_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeAll(t, src, []byte("hello"))
	if err := Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Errorf("dst content = %q", b)
	}
}

func TestCopy_SrcMissing(t *testing.T) {
	dir := t.TempDir()
	if err := Copy(filepath.Join(dir, "x"), filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error")
	}
}

func TestCopy_DstNotWritable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeAll(t, src, []byte("x"))
	if err := Copy(filepath.Join(dir, "no", "such", "dst"), src); err == nil {
		t.Fatal("expected error")
	}
}
