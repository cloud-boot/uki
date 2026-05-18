// Package uki assembles a Unified Kernel Image (UKI) by appending PE/COFF
// sections (.osrel, .cmdline, .uname, .linux, .initrd) to a UEFI stub.
// The PE manipulation is delegated to github.com/go-coff/peln/appender so
// no external `objcopy` dependency is needed.
package uki

import (
	"fmt"
	"io"
	"os"

	"github.com/go-coff/peln/appender"
)

// Sections describes the input artifacts to embed into the stub.
type Sections struct {
	Stub    string // path to the UEFI stub PE binary (e.g. linuxx64.efi.stub)
	Linux   string // path to vmlinuz
	Initrd  string // path to initramfs (cpio.gz)
	Cmdline string // kernel command line text (no trailing newline)
	OSRel   string // contents of os-release for the embedded OS
	Uname   string // kernel version string ("uname -r")
}

// Build writes the assembled UKI to outPath.
func Build(s Sections, outPath string) error {
	stub, err := os.ReadFile(s.Stub)
	if err != nil {
		return fmt.Errorf("read stub: %w", err)
	}
	linux, err := os.ReadFile(s.Linux)
	if err != nil {
		return fmt.Errorf("read kernel: %w", err)
	}
	initrd, err := os.ReadFile(s.Initrd)
	if err != nil {
		return fmt.Errorf("read initrd: %w", err)
	}

	osrel := []byte(s.OSRel)
	if len(osrel) == 0 {
		osrel = []byte("ID=cloud-boot\nNAME=cloud-boot\nVERSION_ID=1\n")
	}
	cmdline := []byte(s.Cmdline + "\n")
	uname := []byte(s.Uname + "\n")

	out, err := appender.Append(stub, []appender.Section{
		{Name: ".osrel", Data: osrel, Characteristics: appender.DefaultCharacteristics},
		{Name: ".cmdline", Data: cmdline, Characteristics: appender.DefaultCharacteristics},
		{Name: ".uname", Data: uname, Characteristics: appender.DefaultCharacteristics},
		{Name: ".linux", Data: linux, Characteristics: appender.DefaultCharacteristics},
		{Name: ".initrd", Data: initrd, Characteristics: appender.DefaultCharacteristics},
	})
	if err != nil {
		return fmt.Errorf("uki: %w", err)
	}
	return os.WriteFile(outPath, out, 0o644)
}

// Copy is a small helper that copies src to dst preserving content (not perms).
func Copy(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
