package build

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloud-boot/init/pkg/cpio"
	"github.com/cloud-boot/uki/internal/uki"
)

// DefaultStubRoots lists the system directories searched for the UEFI stub.
// Exported as a var so tests can point at a tempdir.
var DefaultStubRoots = []string{
	"/usr/lib/systemd/boot/efi",
	"/usr/lib64/systemd/boot/efi",
	"/lib/systemd/boot/efi",
}

func defaultStub(stubName string) string {
	for _, root := range DefaultStubRoots {
		p := filepath.Join(root, stubName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Indirections used by tests to stand in for external binaries and the
// expensive `go build` of the init binary.
var (
	LookPath = exec.LookPath
	CmdRun   = func(name string, args ...string) error {
		c := exec.Command(name, args...)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		return c.Run()
	}
	BuildInitFn = buildInit
)

// Build runs the full ISO-assembly pipeline. Exposed so tests can drive it
// without going through the cobra flag layer.
func Build(o Opts) error {
	for _, t := range []string{"go", "mformat", "mmd", "mcopy", "xorriso"} {
		if _, err := LookPath(t); err != nil {
			return fmt.Errorf("required tool %q not in PATH", t)
		}
	}

	wd := o.WorkDir
	if wd == "" {
		var err error
		wd, err = os.MkdirTemp("", "cloud-boot-build-")
		if err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(wd, 0o755); err != nil {
			return err
		}
	}
	log.Printf("workdir: %s (arch=%s)", wd, o.Arch.GoArch)
	if !o.Keep && o.WorkDir == "" {
		defer os.RemoveAll(wd)
	}

	initBin := filepath.Join(wd, "cloud-boot-init")
	if err := BuildInitFn(initBin, o.Arch.GoArch); err != nil {
		return err
	}

	var cosignPEM []byte
	if o.CosignKey != "" {
		var err error
		cosignPEM, err = os.ReadFile(o.CosignKey)
		if err != nil {
			return fmt.Errorf("read cosign key: %w", err)
		}
		log.Printf("embedding cosign public key (%d B) at /etc/cosign.pub", len(cosignPEM))
	}

	initramfs := filepath.Join(wd, "initramfs.cpio.gz")
	if err := buildInitramfs(initBin, initramfs, cosignPEM); err != nil {
		return err
	}

	fullCmd := BuildCmdline(o)
	log.Printf("cmdline: %s", fullCmd)

	ukiPath := filepath.Join(wd, o.Arch.EFIName)
	if err := uki.Build(uki.Sections{
		Stub:    o.Stub,
		Linux:   o.Kernel,
		Initrd:  initramfs,
		Cmdline: fullCmd,
		Uname:   o.Uname,
	}, ukiPath); err != nil {
		return err
	}

	esp := filepath.Join(wd, "efiboot.img")
	if err := buildESP(ukiPath, o.Arch.EFIName, esp); err != nil {
		return err
	}
	if err := buildISO(esp, o.Out); err != nil {
		return err
	}
	log.Printf("built %s (arch=%s, %s)", o.Out, o.Arch.GoArch, o.Arch.EFIName)
	return nil
}

// BuildCmdline assembles the kernel command line the UKI ships with.
// The base --cmdline is augmented with the cloudboot.* directives the init
// understands, plus — by default — `quiet` so the chained kernel only
// emits error-level (and higher) messages. `quiet` alone sets
// console_loglevel to KERN_WARNING (4); levels 0-3 (EMERG/ALERT/CRIT/ERR)
// pass through, levels 4-7 (WARN/NOTICE/INFO/DEBUG) are suppressed. Adding
// `loglevel=3` on top would also hide errors and is too aggressive for a
// default. Pass --verbose to drop the quiet injection entirely.
func BuildCmdline(o Opts) string {
	parts := []string{o.Cmdline}
	if !o.Verbose {
		parts = append(parts, "quiet")
	}
	if o.PlanRef != "" {
		parts = append(parts, "cloudboot.plan="+o.PlanRef)
	}
	if o.Image != "" {
		parts = append(parts, "cloudboot.image="+o.Image)
	}
	if o.Target != "" {
		parts = append(parts, "cloudboot.target="+o.Target)
	}
	if o.Insec {
		parts = append(parts, "cloudboot.insecure=1")
	}
	if o.Extra != "" {
		parts = append(parts, o.Extra)
	}
	return joinWords(parts)
}

func joinWords(p []string) string {
	out := ""
	for _, s := range p {
		if s == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += s
	}
	return out
}

func buildInit(out, goarch string) error {
	log.Printf("building cloud-boot-init for linux/%s -> %s", goarch, out)
	// The init binary lives in a sibling module (github.com/cloud-boot/init).
	// Build from init/ itself so Go uses init/go.sum, which already has every
	// transitive entry (hcl, dhcp, netlink, …). Running this from uki/ instead
	// would fail with "missing go.sum entry" because uki/go.sum only covers
	// the host CLIs' direct imports.
	initDir, err := locateInitModule()
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build",
		"-trimpath",
		"-ldflags", "-s -w -extldflags '-static'",
		"-tags", "netgo,osusergo",
		"-o", out,
		"./cmd/cloud-boot-init")
	cmd.Dir = initDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+goarch,
	)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// locateInitModule finds the init module checkout. In the local layout
// (cloud-boot/uki next to cloud-boot/init) it's "../init" relative to uki's
// CWD. CLOUD_BOOT_INIT_DIR overrides for non-standard layouts.
func locateInitModule() (string, error) {
	if v := os.Getenv("CLOUD_BOOT_INIT_DIR"); v != "" {
		return v, nil
	}
	candidate := filepath.Clean("../init")
	if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("cannot find init module: set CLOUD_BOOT_INIT_DIR or run from cloud-boot/uki")
}

func buildInitramfs(initBin, out string, cosignPEM []byte) error {
	log.Printf("packing initramfs -> %s", out)
	data, err := os.ReadFile(initBin)
	if err != nil {
		return err
	}

	var raw bytes.Buffer
	w := cpio.NewWriter(&raw)

	for _, d := range []string{"dev", "proc", "sys", "run", "etc", "tmp", "sbin", "bin"} {
		if err := w.WriteDir(d, 0o755); err != nil {
			return err
		}
	}
	if err := w.WriteNod("dev/console", 0o020600, 5, 1); err != nil {
		return err
	}
	if err := w.WriteNod("dev/null", 0o020666, 1, 3); err != nil {
		return err
	}
	if err := w.WriteFile(cpio.Header{Name: "init", Mode: 0100755}, data); err != nil {
		return err
	}
	if err := w.WriteSymlink("sbin/init", "/init", 0o777); err != nil {
		return err
	}
	if len(cosignPEM) > 0 {
		if err := w.WriteFile(cpio.Header{Name: "etc/cosign.pub", Mode: 0100644}, cosignPEM); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(raw.Bytes()); err != nil {
		return err
	}
	return gz.Close()
}

func buildESP(efi, efiName, out string) error {
	log.Printf("creating ESP image -> %s (/EFI/BOOT/%s)", out, efiName)
	if err := truncate(out, 64*1024*1024); err != nil {
		return err
	}
	if err := run("mformat", "-i", out, "-F", "::"); err != nil {
		return err
	}
	if err := run("mmd", "-i", out, "::/EFI"); err != nil {
		return err
	}
	if err := run("mmd", "-i", out, "::/EFI/BOOT"); err != nil {
		return err
	}
	return run("mcopy", "-i", out, efi, "::/EFI/BOOT/"+efiName)
}

// buildISO produces a hybrid ISO that is simultaneously:
//
//   - El Torito bootable for plain ISO9660 firmware paths (the same
//     code path every UEFI firmware uses to autoboot a removable
//     "CD"-shaped device).
//   - A GPT-partitioned block device whose partition 2 carries the
//     EFI System Partition type GUID (C12A7328-F81F-11D2-BA4B-
//     00A0C93EC93B). The ESP partition's bytes ARE the FAT32 image
//     in `esp` — appended at the tail of the ISO via xorriso's
//     -append_partition mechanism. The El Torito boot catalog uses
//     `--interval:appended_partition_2:all::` so the firmware reads
//     boot sectors from the same byte range that GPT exposes as
//     partition 2 — one image, two views.
//
// Why this layout matters: the menu-then-reboot sink in cloud-boot-
// init/ needs to write \EFI\Linux\<target>-* files onto the FAT
// ESP and then call reboot(2). On the prior pure-iso9660 layout
// the firmware-visible FAT image lived inside a read-only iso9660
// file (efiboot.img) — Linux could not mount it as a writable
// partition. With the hybrid GPT layout the same FAT image is
// exposed as a real GPT partition that mounts vfat r/w under
// Linux, so the sink can mutate it and reboot.
//
// See memory:uki-menu-then-reboot for the architecture rationale.
func buildISO(esp, out string) error {
	log.Printf("creating ISO -> %s (hybrid GPT, ESP=appended partition 2)", out)
	if _, err := os.Stat(esp); err != nil {
		return fmt.Errorf("esp image: %w", err)
	}
	stage, err := os.MkdirTemp("", "iso-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// xorriso refuses to author an iso9660 filesystem with zero
	// files — drop a tiny README so the iso9660 portion is well-
	// formed even though we don't put efiboot.img inside it.
	readme := filepath.Join(stage, "README.txt")
	if err := os.WriteFile(readme, []byte("cloud-boot bootable image. See partition 2 (ESP) for boot files.\n"), 0o644); err != nil {
		return err
	}
	return run("xorriso",
		"-as", "mkisofs",
		"-V", "GOPXE",
		"-o", out,
		// El Torito boot catalog points at the GPT-appended ESP
		// (partition 2), not at a file inside iso9660. The
		// `--interval:appended_partition_2:all::` token tells
		// mkisofs to use the appended partition's byte range as
		// the El Torito boot image.
		"-no-emul-boot",
		"-e", "--interval:appended_partition_2:all::",
		// Append the FAT ESP as GPT partition 2 with the proper
		// EFI System type GUID. -appended_part_as_gpt makes it a
		// real GPT entry (vs. an MBR-only partition).
		"-append_partition", "2", "C12A7328-F81F-11D2-BA4B-00A0C93EC93B", esp,
		"-appended_part_as_gpt",
		stage,
	)
}

func truncate(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Truncate(size)
}

func run(name string, args ...string) error {
	return CmdRun(name, args...)
}
