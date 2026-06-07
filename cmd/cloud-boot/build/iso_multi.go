// Multi-arch ISO assembly.
//
// The single-arch `cloud-boot build` flow produces one UKI + one ESP + one
// hybrid ISO targeted at exactly one CPU architecture (BOOTX64.EFI for
// amd64, BOOTAA64.EFI for arm64, BOOTRISCV64.EFI for riscv64). That works
// per host, but it forces the operator to keep one ISO per arch in the
// fleet, picking the right one before booting each VM.
//
// UEFI firmware on every CPU only ever reads the file under
// \EFI\BOOT\ that matches its own arch — amd64 firmware reads BOOTX64.EFI
// and ignores BOOTAA64.EFI sitting next to it, and vice-versa. So a
// single ESP can carry multiple UKIs at the canonical removable-media
// fallback paths, and the same ISO boots on any of the supported CPUs.
//
// `BuildMultiArchISO` is the assembler: it takes a slice of already-built
// per-arch UKIs (each tagged with its ArchProfile) and produces ONE
// hybrid ISO. The per-arch UKIs are built upstream by the existing
// single-arch `Build` flow (the operator can also point at UKIs from a
// CI artifact store — anything that's a valid PE32+ EFI binary).
//
// RISC-V status: the riscv64 UKI itself is buildable today — Linux's EFI
// stub supports riscv64 since 5.10 and systemd ships `linuxriscv64.efi.stub`
// upstream, so the UKI assembler in `Build()` works for riscv64 without
// special-casing. The LLD COFF blocker noted in tinygo-riscv64-uefi/README
// only affects the *pure-UEFI loader* (Path B's BOOTRISCV64.EFI from
// go-coff/stub) — not the UKI/ISO path (Path A and Path C).

package build

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// ArchUKI is one entry in a multi-arch ISO build: a per-arch UKI that's
// already been assembled (e.g. by running `cloud-boot build --arch <a>
// --uki-only` for each arch, or pulled from a CI cache).
type ArchUKI struct {
	Arch ArchProfile // determines the ESP filename (BOOTX64.EFI, BOOTAA64.EFI, BOOTRISCV64.EFI)
	UKI  string      // path to the .efi binary on the host
}

// MultiOpts bundles inputs to BuildMultiArchISO.
type MultiOpts struct {
	UKIs    []ArchUKI // one per CPU arch; order doesn't matter
	Out     string    // output .iso path
	WorkDir string    // optional; defaults to a tempdir cleaned up on success
	Keep    bool      // keep workdir on success
}

// BuildMultiArchISO produces one hybrid iso9660 + El Torito + GPT image
// whose ESP carries every UKI in `o.UKIs` under
// \EFI\BOOT\<arch.EFIName>. UEFI firmware on each CPU reads only its own
// arch's file, so the same ISO is bootable on amd64, arm64 and riscv64
// hosts.
//
// The required external tools are the same as the single-arch path:
// mformat, mmd, mcopy and xorriso.
func BuildMultiArchISO(o MultiOpts) error {
	if len(o.UKIs) == 0 {
		return fmt.Errorf("no UKIs supplied")
	}
	for _, tool := range []string{"mformat", "mmd", "mcopy", "xorriso"} {
		if _, err := LookPath(tool); err != nil {
			return fmt.Errorf("required tool %q not in PATH", tool)
		}
	}
	seen := make(map[string]string, len(o.UKIs))
	for _, u := range o.UKIs {
		if u.Arch.EFIName == "" {
			return fmt.Errorf("arch profile missing EFIName")
		}
		if prev, ok := seen[u.Arch.EFIName]; ok {
			return fmt.Errorf("duplicate UKI for %s: %q and %q", u.Arch.EFIName, prev, u.UKI)
		}
		if _, err := os.Stat(u.UKI); err != nil {
			return fmt.Errorf("uki %s: %w", u.UKI, err)
		}
		seen[u.Arch.EFIName] = u.UKI
	}

	wd := o.WorkDir
	if wd == "" {
		var err error
		wd, err = os.MkdirTemp("", "cloud-boot-iso-multi-")
		if err != nil {
			return err
		}
		if !o.Keep {
			defer os.RemoveAll(wd)
		}
	}
	log.Printf("multi-arch ISO workdir: %s", wd)

	esp := filepath.Join(wd, "efiboot.img")
	if err := buildESPMulti(o.UKIs, esp); err != nil {
		return err
	}
	if err := buildISO(esp, o.Out); err != nil {
		return err
	}
	archs := make([]string, 0, len(o.UKIs))
	for _, u := range o.UKIs {
		archs = append(archs, u.Arch.GoArch)
	}
	log.Printf("built multi-arch %s (%v)", o.Out, archs)
	return nil
}

// buildESPMulti drops every UKI into a single FAT image at the
// corresponding \EFI\BOOT\<EFIName> path. The image size is computed
// from the total UKI bytes with 50% headroom plus an 8 MiB FAT
// floor, then rounded up to the next 16 MiB so mformat picks a
// sensible cluster geometry.
//
// The FAT type is left to mformat (no -F): for a small ESP (~16 MiB,
// the common single-/dual-arch case) forcing FAT32 yields fewer than
// the 65525 clusters the FAT32 spec requires, so mformat emits a
// sub-spec FAT32 that UEFI firmware (e.g. OVMF) refuses to mount —
// the image then "fails to load / Not Found" at boot. Letting mformat
// auto-select gives a spec-valid FAT16 for small ESPs (which UEFI
// supports on removable/ISO media) and FAT32 once the volume is large
// enough. Verified bootable under QEMU/OVMF.
func buildESPMulti(ukis []ArchUKI, out string) error {
	const (
		floor   = 8 * 1024 * 1024
		round   = 16 * 1024 * 1024
		hdroom  = 3 // multiplied / 2 → 1.5× total UKI bytes
	)
	var total int64
	for _, u := range ukis {
		st, err := os.Stat(u.UKI)
		if err != nil {
			return err
		}
		total += st.Size()
	}
	size := total*int64(hdroom)/2 + floor
	if size%round != 0 {
		size = ((size / round) + 1) * round
	}
	log.Printf("creating ESP image -> %s (%d MiB, %d UKIs)", out, size/(1024*1024), len(ukis))
	if err := truncate(out, size); err != nil {
		return err
	}
	if err := run("mformat", "-i", out, "::"); err != nil {
		return err
	}
	if err := run("mmd", "-i", out, "::/EFI"); err != nil {
		return err
	}
	if err := run("mmd", "-i", out, "::/EFI/BOOT"); err != nil {
		return err
	}
	for _, u := range ukis {
		if err := run("mcopy", "-i", out, u.UKI, "::/EFI/BOOT/"+u.Arch.EFIName); err != nil {
			return fmt.Errorf("mcopy %s: %w", u.Arch.EFIName, err)
		}
		log.Printf("  + /EFI/BOOT/%-16s (%s)", u.Arch.EFIName, u.UKI)
	}
	return nil
}
