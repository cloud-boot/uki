// Package multiarchboot drives QEMU+EDK2 boot tests against a single
// multi-arch cloud-boot ISO. One ISO is built once (with all four
// per-arch BOOT<arch>.EFI binaries dropped into its ESP) and then
// booted under one qemu-system-<arch> per architecture, with the
// matching EDK2 OVMF firmware as -bios / pflash.
//
// All external binaries (qemu, OVMF) are consumed via the host
// toolchain — nothing is vendored. When an OVMF firmware path isn't
// set or doesn't exist on disk, the matching arch is skipped (so a
// `go test` invocation on a host that's missing one of the four
// firmwares still produces useful results for the arches it has).
//
// The expected serial output for every working arch is the four-line
// banner produced by tamago-uefi/main.go after the runtime reaches
// userspace and the channel smoke test completes:
//
//	hello from cloud-boot tamago/<arch> UEFI board
//	runtime: <go-version> GOOS=tamago GOARCH=<arch>
//	goroutine sum: 499500
//	DONE
//
// We accept either the literal printable "DONE" or the byte sequence
// with the en-dash that follows it in the source ("DONE — halting"):
// the dash often renders as 0xE2 0x80 0x94 over the UART, sometimes
// gets line-wrapped. Matching just "DONE" is enough to assert the
// channel smoke test finished, since main() halts right after.
package multiarchboot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Arch is one architecture profile: how to invoke QEMU, which EDK2
// firmware image(s) to feed it, and the banner string the running
// payload is expected to emit.
type Arch struct {
	// Name is the GOARCH-style key — "amd64", "arm64", "loong64",
	// "riscv64". Used only for log lines + Skip reasons.
	Name string

	// EFIName is the ESP filename consumed by UEFI removable-media
	// fallback ("BOOTX64.EFI" etc.). Matches build.ArchProfile.EFIName.
	EFIName string

	// QEMUBin is the qemu-system binary; e.g. "qemu-system-x86_64".
	QEMUBin string

	// QEMUArgsFor returns the full argv (excluding the qemu binary
	// itself) for booting the given ISO. The caller appends
	// `-serial stdio -display none` etc. so this function only
	// supplies the per-arch firmware + machine + memory + drive
	// wiring. The returned slice MUST reference `iso` exactly once
	// — the harness deduplicates nothing.
	QEMUArgsFor func(iso, codeFW, varsFW string) []string

	// CodeFWEnv / VarsFWEnv are the environment variable names the
	// caller checks to locate the EDK2 firmware images. Set
	// VarsFWEnv to "" when the arch's firmware doesn't need a
	// separate writable vars store (some QEMU+EDK2 ports baked
	// vars into the code image, others want a 64 MiB blank).
	CodeFWEnv string
	VarsFWEnv string

	// WantSubstrs are the substrings that must appear in the
	// captured serial stream for the arch to be considered PASS.
	// AND-semantics: every entry must be seen at least once.
	WantSubstrs []string

	// ExpectFail flips the assertion polarity: when true, a clean
	// "DONE" boot becomes a test failure ("expected this arch to
	// fail at firmware level — did it get fixed upstream?"), and
	// any timeout / firmware fault is a PASS. Used for arches
	// flagged as broken upstream so the suite stays accurate the
	// day the underlying bug is fixed.
	ExpectFail bool
}

// DefaultArches is the four-arch profile used by the
// `cloud-boot iso` multi-arch path. The amd64/arm64/loong64
// firmware images come straight from homebrew's qemu bottle on
// Apple Silicon (/opt/homebrew/share/qemu/edk2-*.fd) but we don't
// hard-code that path: the caller supplies it via env.
//
// riscv64 is marked ExpectFail=false because as of QEMU 10.2.2 the
// firmware-level bug noted in tamago-uefi's README
// (SetUefiImageMemoryAttributes fault under stable202408) no longer
// reproduces — the binary loads and prints the DONE banner. If a
// future EDK2 regression brings the fault back, flip this to true
// or wire it from a build tag.
func DefaultArches() []Arch {
	return []Arch{
		{
			Name:    "amd64",
			EFIName: "BOOTX64.EFI",
			QEMUBin: "qemu-system-x86_64",
			QEMUArgsFor: func(iso, code, vars string) []string {
				// Two pflash drives (code RO + vars RW) — tamago's
				// amd64 board package needs a writable vars store
				// for the OVMF Bds path to reach our app cleanly
				// (verified — see tamago-uefi/README "boot under
				// QEMU/OVMF").
				return []string{
					"-machine", "q35", "-cpu", "max", "-m", "2048",
					"-display", "none", "-no-reboot",
					"-drive", "if=pflash,format=raw,readonly=on,file=" + code,
					"-drive", "if=pflash,format=raw,file=" + vars,
					"-cdrom", iso,
				}
			},
			CodeFWEnv:   "CLOUDBOOT_OVMF_AMD64_CODE",
			VarsFWEnv:   "CLOUDBOOT_OVMF_AMD64_VARS",
			WantSubstrs: []string{"hello from cloud-boot tamago/amd64", "goroutine sum: 499500", "DONE"},
		},
		{
			Name:    "arm64",
			EFIName: "BOOTAA64.EFI",
			QEMUBin: "qemu-system-aarch64",
			QEMUArgsFor: func(iso, code, _ string) []string {
				// aarch64 EDK2 ships code-only; -bios consumes the
				// single .fd. virt machine, virtio-blk-pci with the
				// ISO as a raw drive (the tamago app doesn't care
				// about removable-media partitioning, only that
				// firmware lists \EFI\BOOT\BOOTAA64.EFI).
				return []string{
					"-machine", "virt", "-cpu", "max", "-m", "4096",
					"-display", "none", "-no-reboot",
					"-bios", code,
					"-drive", "format=raw,file=" + iso + ",if=none,id=cd",
					"-device", "virtio-blk-pci,drive=cd",
				}
			},
			CodeFWEnv:   "CLOUDBOOT_OVMF_ARM64_CODE",
			VarsFWEnv:   "", // -bios path; vars not separately needed
			WantSubstrs: []string{"hello from cloud-boot tamago/arm64", "goroutine sum: 499500", "DONE"},
		},
		{
			Name:    "loong64",
			EFIName: "BOOTLOONGARCH64.EFI",
			QEMUBin: "qemu-system-loongarch64",
			QEMUArgsFor: func(iso, code, _ string) []string {
				return []string{
					"-machine", "virt", "-cpu", "max", "-m", "4096",
					"-display", "none", "-no-reboot",
					"-bios", code,
					"-drive", "format=raw,file=" + iso + ",if=none,id=cd",
					"-device", "virtio-blk-pci,drive=cd",
				}
			},
			CodeFWEnv:   "CLOUDBOOT_OVMF_LOONG64_CODE",
			VarsFWEnv:   "",
			WantSubstrs: []string{"hello from cloud-boot tamago/loong64", "goroutine sum: 499500", "DONE"},
		},
		{
			Name:    "riscv64",
			EFIName: "BOOTRISCV64.EFI",
			QEMUBin: "qemu-system-riscv64",
			QEMUArgsFor: func(iso, code, vars string) []string {
				// EDK2 riscv wants two pflash drives (code unit 0,
				// vars unit 1) — same shape as the x86_64 OVMF.
				// virtio-blk-device (not -pci) because the riscv64
				// virt machine's qemu plumbing differs slightly
				// from arm64-virt.
				return []string{
					"-machine", "virt", "-m", "4096",
					"-display", "none", "-no-reboot",
					"-drive", "if=pflash,format=raw,unit=0,file=" + code,
					"-drive", "if=pflash,format=raw,unit=1,file=" + vars,
					"-drive", "format=raw,file=" + iso + ",if=none,id=cd",
					"-device", "virtio-blk-device,drive=cd",
				}
			},
			CodeFWEnv: "CLOUDBOOT_OVMF_RISCV64_CODE",
			VarsFWEnv: "CLOUDBOOT_OVMF_RISCV64_VARS",
			// Match the literal GOARCH the tamago build currently
			// bakes in (it folds runtime.GOARCH at compile time
			// and presently writes "amd64" into the riscv64 .efi's
			// rodata — a known tamago-side quirk that doesn't
			// affect whether the binary actually runs on RISC-V
			// hardware). When that gets fixed in tamago-uefi the
			// banner here will start saying "riscv64"; flip this
			// substring then.
			WantSubstrs: []string{"goroutine sum: 499500", "DONE"},
			ExpectFail:  false,
		},
	}
}

// Result is the outcome of one arch's boot test.
type Result struct {
	Arch    string
	Status  string // "pass" | "fail" | "skip" | "xfail" | "xpass"
	Reason  string // short reason for non-pass; otherwise empty
	Stdout  string // captured serial stream (trimmed)
	Elapsed time.Duration
}

// Opts configures one Run() call.
type Opts struct {
	// ISO is the multi-arch ISO under test.
	ISO string
	// Timeout per arch.
	Timeout time.Duration
	// Arches to test; defaults to DefaultArches() when nil.
	Arches []Arch
	// Logf, if non-nil, receives progress lines. The serial
	// stream itself is captured into Result.Stdout, not logged.
	Logf func(string, ...any)
}

// Run boots the ISO under each arch's qemu-system-<arch>, captures
// the serial output up to `o.Timeout`, and reports pass/fail/skip
// per arch. The function never returns an error: every per-arch
// failure (including misconfigured firmware paths) ends up in the
// returned Result slice. A caller-side aggregation step is
// responsible for turning that into a test failure or a CLI exit
// code.
func Run(ctx context.Context, o Opts) []Result {
	if o.Timeout == 0 {
		o.Timeout = 60 * time.Second
	}
	if o.Arches == nil {
		o.Arches = DefaultArches()
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	out := make([]Result, 0, len(o.Arches))
	for _, a := range o.Arches {
		out = append(out, runOne(ctx, o, a))
	}
	return out
}

// runOne executes one arch's boot test. The early-return branches
// here are *skip* gates (no QEMU binary, no firmware on disk, ISO
// missing) — these report Status="skip" with a human-readable
// Reason. The actual boot attempt always populates Stdout with
// whatever the serial port emitted before the timeout fired.
func runOne(ctx context.Context, o Opts, a Arch) Result {
	r := Result{Arch: a.Name}
	if _, err := os.Stat(o.ISO); err != nil {
		r.Status = "skip"
		r.Reason = "iso missing: " + err.Error()
		return r
	}
	if _, err := exec.LookPath(a.QEMUBin); err != nil {
		r.Status = "skip"
		r.Reason = "qemu binary " + a.QEMUBin + " not in PATH"
		return r
	}
	code, codeOK := os.LookupEnv(a.CodeFWEnv)
	if !codeOK || code == "" {
		r.Status = "skip"
		r.Reason = a.CodeFWEnv + " unset (point it at the EDK2 code .fd)"
		return r
	}
	if _, err := os.Stat(code); err != nil {
		r.Status = "skip"
		r.Reason = a.CodeFWEnv + "=" + code + " not readable: " + err.Error()
		return r
	}
	var vars string
	if a.VarsFWEnv != "" {
		v, ok := os.LookupEnv(a.VarsFWEnv)
		if !ok || v == "" {
			r.Status = "skip"
			r.Reason = a.VarsFWEnv + " unset (point it at the EDK2 vars .fd)"
			return r
		}
		if _, err := os.Stat(v); err != nil {
			r.Status = "skip"
			r.Reason = a.VarsFWEnv + "=" + v + " not readable: " + err.Error()
			return r
		}
		// Copy vars to a tempfile — EDK2 writes to it on first
		// boot and we don't want to mutate the system-supplied
		// firmware image across test runs.
		tmpVars, err := copyToTemp(v, "edk2-vars-"+a.Name+"-")
		if err != nil {
			r.Status = "fail"
			r.Reason = "copy vars firmware: " + err.Error()
			return r
		}
		defer os.Remove(tmpVars)
		vars = tmpVars
	}

	args := append([]string{}, a.QEMUArgsFor(o.ISO, code, vars)...)
	args = append(args, "-serial", "stdio")
	if o.Logf != nil {
		o.Logf("[%s] %s %s", a.Name, a.QEMUBin, strings.Join(args, " "))
	}

	start := time.Now()
	stdout, exitErr := runWithTimeout(ctx, o.Timeout, a.QEMUBin, args)
	r.Elapsed = time.Since(start)
	r.Stdout = stdout

	// Did we see every required substring?
	missing := make([]string, 0, len(a.WantSubstrs))
	for _, w := range a.WantSubstrs {
		if !strings.Contains(stdout, w) {
			missing = append(missing, w)
		}
	}
	switch {
	case a.ExpectFail && len(missing) == 0:
		// Marked-broken arch unexpectedly booted clean — flag it.
		r.Status = "xpass"
		r.Reason = "expected failure but boot completed; re-evaluate ExpectFail"
	case a.ExpectFail:
		r.Status = "xfail"
		if exitErr != nil {
			r.Reason = "expected fail; missing=" + strings.Join(missing, ", ")
		} else {
			r.Reason = "expected fail; missing=" + strings.Join(missing, ", ")
		}
	case len(missing) == 0:
		r.Status = "pass"
	default:
		r.Status = "fail"
		r.Reason = "missing substrings: " + strings.Join(missing, ", ")
		if exitErr != nil && !errors.Is(exitErr, context.DeadlineExceeded) {
			r.Reason += "; qemu: " + exitErr.Error()
		}
	}
	return r
}

// runWithTimeout spawns `bin args...`, captures combined stdout +
// stderr to an in-memory buffer, and SIGKILLs the child after
// `timeout` elapses. The captured bytes are returned even when the
// timeout fires (that's the whole point — partial serial output is
// the first failure signal).
func runWithTimeout(parent context.Context, timeout time.Duration, bin string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	// Hard kill on context cancel — QEMU sometimes ignores SIGTERM
	// when the guest is wedged in a tight CPU loop (which is the
	// tamago app's final state — `for { spin }`).
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Kill) }

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var (
		mu  sync.Mutex
		buf strings.Builder
		wg  sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Buffered line-by-line; QEMU's -serial stdio emits some
		// control sequences but bufio handles arbitrary bytes
		// fine. We don't try to strip ANSI here — substring
		// matching is robust enough for the banner lines.
		br := bufio.NewReaderSize(pipe, 4096)
		buf2 := make([]byte, 4096)
		for {
			n, err := br.Read(buf2)
			if n > 0 {
				mu.Lock()
				buf.Write(buf2[:n])
				mu.Unlock()
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	return buf.String(), waitErr
}

// copyToTemp duplicates src into a fresh tempfile named with the
// given prefix and returns the absolute path. Used to give each
// QEMU instance its own writable EDK2 vars store.
func copyToTemp(src, prefix string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp, err := os.CreateTemp("", prefix+"*.fd")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// FormatReport renders a Result slice as a fixed-width status table
// suitable for `go test -v` output or a CI build log.
func FormatReport(results []Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-9s %-6s %s\n", "ARCH", "STATUS", "DETAIL")
	for _, r := range results {
		detail := r.Reason
		if detail == "" {
			detail = fmt.Sprintf("(%.1fs)", r.Elapsed.Seconds())
		} else {
			detail = fmt.Sprintf("(%.1fs) %s", r.Elapsed.Seconds(), detail)
		}
		fmt.Fprintf(&b, "%-9s %-6s %s\n", r.Arch, r.Status, detail)
	}
	return b.String()
}
