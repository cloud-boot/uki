package multiarchboot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMultiArchBoot drives a real QEMU+OVMF boot for every arch in
// DefaultArches. It expects an ISO at $CLOUDBOOT_TEST_ISO and one
// EDK2 firmware image per arch under the env vars listed in
// DefaultArches() (CLOUDBOOT_OVMF_<ARCH>_CODE / _VARS).
//
// When CLOUDBOOT_TEST_ISO isn't set OR all four firmware envs are
// missing, the test is t.Skip()'d — the suite stays green on hosts
// that don't have the external tooling. This matches the broader
// project rule: don't hard-fail on missing externals.
//
// Set CLOUDBOOT_BOOT_TIMEOUT to override the default 60s per-arch
// budget (useful under CI where TCG emulation is slower).
func TestMultiArchBoot(t *testing.T) {
	iso := os.Getenv("CLOUDBOOT_TEST_ISO")
	if iso == "" {
		t.Skip("CLOUDBOOT_TEST_ISO unset — point it at a multi-arch boot.iso (cloud-boot iso --uki ...)")
	}
	if _, err := os.Stat(iso); err != nil {
		t.Skipf("CLOUDBOOT_TEST_ISO=%s not readable: %v", iso, err)
	}

	timeout := 60 * time.Second
	if s := os.Getenv("CLOUDBOOT_BOOT_TIMEOUT"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			t.Fatalf("CLOUDBOOT_BOOT_TIMEOUT=%q: %v", s, err)
		}
		timeout = d
	}

	results := Run(context.Background(), Opts{
		ISO:     iso,
		Timeout: timeout,
		Logf:    t.Logf,
	})

	t.Logf("\n%s", FormatReport(results))

	allSkipped := true
	for _, r := range results {
		if r.Status != "skip" {
			allSkipped = false
		}
	}
	if allSkipped {
		t.Skip("no arch had its OVMF firmware env vars set — set CLOUDBOOT_OVMF_<ARCH>_CODE/_VARS")
	}

	for _, r := range results {
		r := r // capture
		t.Run(r.Arch, func(t *testing.T) {
			switch r.Status {
			case "pass":
				// ok
			case "xfail", "xpass":
				// xfail = expected failure, observed failure → ok.
				// xpass = expected failure but it passed → flag
				// loudly so somebody can flip ExpectFail off.
				if r.Status == "xpass" {
					t.Fatalf("xpass for %s — please remove the ExpectFail flag: %s", r.Arch, r.Reason)
				}
			case "skip":
				t.Skipf("%s: %s", r.Arch, r.Reason)
			case "fail":
				t.Fatalf("%s boot failed: %s\n--- captured serial ---\n%s\n--- end serial ---", r.Arch, r.Reason, r.Stdout)
			default:
				t.Fatalf("%s unknown status %q: %s", r.Arch, r.Status, r.Reason)
			}
		})
	}
}

// TestDefaultArches_StructureSane validates the static arch profiles
// don't drift away from the convention enforced elsewhere
// (build.ArchProfiles), e.g. that every EFIName ends with .EFI and
// every arch lists at least one substring to match against.
func TestDefaultArches_StructureSane(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range DefaultArches() {
		if a.Name == "" {
			t.Errorf("arch with empty Name: %+v", a)
		}
		if seen[a.Name] {
			t.Errorf("duplicate arch %q", a.Name)
		}
		seen[a.Name] = true
		if !strings.HasSuffix(a.EFIName, ".EFI") {
			t.Errorf("%s: EFIName %q doesn't end in .EFI", a.Name, a.EFIName)
		}
		if !strings.HasPrefix(a.QEMUBin, "qemu-system-") {
			t.Errorf("%s: QEMUBin %q doesn't look like a qemu-system binary", a.Name, a.QEMUBin)
		}
		if a.QEMUArgsFor == nil {
			t.Errorf("%s: QEMUArgsFor is nil", a.Name)
		}
		if a.CodeFWEnv == "" {
			t.Errorf("%s: CodeFWEnv must be set", a.Name)
		}
		if len(a.WantSubstrs) == 0 {
			t.Errorf("%s: WantSubstrs is empty", a.Name)
		}
		// Smoke-test the argv generator: it should reference the
		// iso path exactly once and any firmware path it was
		// handed.
		argv := a.QEMUArgsFor("/tmp/x.iso", "/tmp/code.fd", "/tmp/vars.fd")
		got := strings.Join(argv, " ")
		if !strings.Contains(got, "/tmp/x.iso") {
			t.Errorf("%s: argv doesn't mention the iso: %q", a.Name, got)
		}
		if !strings.Contains(got, "/tmp/code.fd") {
			t.Errorf("%s: argv doesn't mention the code fw: %q", a.Name, got)
		}
	}
}

// TestRun_SkipsOnMissingISO verifies the skip-gate fires when the
// ISO doesn't exist (the most common misconfiguration). Every
// returned Result should be Status="skip" with a meaningful Reason.
func TestRun_SkipsOnMissingISO(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such.iso")
	results := Run(context.Background(), Opts{ISO: missing, Timeout: time.Second})
	for _, r := range results {
		if r.Status != "skip" {
			t.Errorf("%s: status=%q, want skip (iso missing)", r.Arch, r.Status)
		}
		if !strings.Contains(r.Reason, "iso missing") {
			t.Errorf("%s: reason=%q, want 'iso missing' substring", r.Arch, r.Reason)
		}
	}
}

// TestRun_SkipsOnUnsetEnv covers the second-most-common skip path:
// the ISO is fine but the OVMF firmware env vars aren't set. We
// create a real (empty) ISO file so the per-arch loop gets past
// the `os.Stat(iso)` check and reaches the env-var gate.
func TestRun_SkipsOnUnsetEnv(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("not a real iso"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Unset every CLOUDBOOT_OVMF_* env var the suite reads so we
	// know exactly what's missing. t.Setenv("", "") doesn't exist
	// so we Setenv each to "" — runOne treats empty as unset.
	for _, a := range DefaultArches() {
		t.Setenv(a.CodeFWEnv, "")
		if a.VarsFWEnv != "" {
			t.Setenv(a.VarsFWEnv, "")
		}
	}
	results := Run(context.Background(), Opts{ISO: iso, Timeout: time.Second})
	for _, r := range results {
		if r.Status != "skip" {
			t.Errorf("%s: status=%q, want skip (env unset)", r.Arch, r.Status)
		}
	}
}

// TestFormatReport_RendersAllStatuses covers the table formatter so
// any tweaks to status / column widths get caught.
func TestFormatReport_RendersAllStatuses(t *testing.T) {
	out := FormatReport([]Result{
		{Arch: "amd64", Status: "pass", Elapsed: 12 * time.Second},
		{Arch: "arm64", Status: "fail", Reason: "no DONE", Elapsed: 60 * time.Second},
		{Arch: "loong64", Status: "skip", Reason: "env unset"},
		{Arch: "riscv64", Status: "xfail", Reason: "firmware fault"},
	})
	for _, want := range []string{"amd64", "pass", "12.0s", "arm64", "fail", "no DONE", "loong64", "skip", "env unset", "riscv64", "xfail", "firmware fault", "ARCH", "STATUS", "DETAIL"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatReport output missing %q\n--full--\n%s", want, out)
		}
	}
}

// TestRunWithTimeout_CapturesOutputAndKillsOnTimeout exercises the
// QEMU-runner core without actually invoking QEMU: we use `sh -c` to
// emit a short banner then sleep past the deadline. The function must
// (a) return the captured banner, (b) hit the timeout path, (c)
// return a non-nil error reflecting the context deadline.
func TestRunWithTimeout_CapturesOutputAndKillsOnTimeout(t *testing.T) {
	stdout, err := runWithTimeout(
		context.Background(),
		800*time.Millisecond,
		"sh", []string{"-c", "echo HELLO-FROM-MOCK; sleep 30"},
	)
	if !strings.Contains(stdout, "HELLO-FROM-MOCK") {
		t.Errorf("stdout missing banner; got %q", stdout)
	}
	if err == nil {
		t.Errorf("err was nil; want a deadline / signal error")
	}
}

// TestRunWithTimeout_CleanExitReturnsNoError covers the
// no-timeout-fired branch: a fast-exiting child should produce a
// nil error and full stdout. Together with the previous test this
// gives us both error edges.
func TestRunWithTimeout_CleanExitReturnsNoError(t *testing.T) {
	stdout, err := runWithTimeout(
		context.Background(),
		5*time.Second,
		"sh", []string{"-c", "echo clean-line; exit 0"},
	)
	if err != nil {
		t.Errorf("err=%v, want nil", err)
	}
	if !strings.Contains(stdout, "clean-line") {
		t.Errorf("stdout missing line; got %q", stdout)
	}
}

// TestRunWithTimeout_PropagatesStartError exercises the early-exit
// branch when exec.Cmd.Start fails (the binary doesn't exist).
func TestRunWithTimeout_PropagatesStartError(t *testing.T) {
	_, err := runWithTimeout(
		context.Background(),
		time.Second,
		"/does/not/exist/no-such-binary-2026", nil,
	)
	if err == nil {
		t.Fatalf("err was nil; want a start error")
	}
}

// TestCopyToTemp_DuplicatesContent verifies the helper's happy path:
// source file content is reproduced byte-for-byte in the returned
// temp file path.
func TestCopyToTemp_DuplicatesContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.fd")
	want := []byte("BEEF\x00FACE\x01CAFE\x02")
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	dst, err := copyToTemp(src, "mb-test-")
	if err != nil {
		t.Fatalf("copyToTemp: %v", err)
	}
	defer os.Remove(dst)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("dst content = %q, want %q", got, want)
	}
}

// TestCopyToTemp_FailsOnMissingSource confirms the helper returns an
// error (not an empty file) when the source path doesn't exist.
func TestCopyToTemp_FailsOnMissingSource(t *testing.T) {
	_, err := copyToTemp("/does/not/exist/missing.fd", "mb-fail-")
	if err == nil {
		t.Fatal("err was nil; want a stat/open error")
	}
}

// TestRunOne_SkipsOnUnreadableFirmwareCode covers the
// "env is set but the file isn't readable" branch — distinct from
// "env unset", and important for diagnostics so the operator sees
// the exact path that's wrong.
func TestRunOne_SkipsOnUnreadableFirmwareCode(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Arch{
		Name:        "test",
		EFIName:     "BOOT.EFI",
		QEMUBin:     "sh", // exists; ensures we get past the LookPath gate
		QEMUArgsFor: func(string, string, string) []string { return nil },
		CodeFWEnv:   "CLOUDBOOT_MB_FW_CODE",
		WantSubstrs: []string{"x"},
	}
	t.Setenv("CLOUDBOOT_MB_FW_CODE", "/does/not/exist/code.fd")
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: time.Second}, a)
	if r.Status != "skip" {
		t.Errorf("status=%q, want skip", r.Status)
	}
	if !strings.Contains(r.Reason, "not readable") {
		t.Errorf("reason=%q, want 'not readable' substring", r.Reason)
	}
}

// TestRunOne_SkipsOnUnreadableFirmwareVars covers the matching
// branch for the writable vars store (riscv64 + amd64 take this
// path).
func TestRunOne_SkipsOnUnreadableFirmwareVars(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dir, "code.fd")
	if err := os.WriteFile(code, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Arch{
		Name:        "test",
		EFIName:     "BOOT.EFI",
		QEMUBin:     "sh",
		QEMUArgsFor: func(string, string, string) []string { return nil },
		CodeFWEnv:   "CLOUDBOOT_MB_FW_CODE",
		VarsFWEnv:   "CLOUDBOOT_MB_FW_VARS",
		WantSubstrs: []string{"x"},
	}
	t.Setenv("CLOUDBOOT_MB_FW_CODE", code)
	t.Setenv("CLOUDBOOT_MB_FW_VARS", "/does/not/exist/vars.fd")
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: time.Second}, a)
	if r.Status != "skip" {
		t.Errorf("status=%q, want skip", r.Status)
	}
	if !strings.Contains(r.Reason, "not readable") {
		t.Errorf("reason=%q, want 'not readable' substring", r.Reason)
	}
}

// TestRunOne_SkipsOnUnsetVarsEnv covers the variant where the
// VarsFWEnv is required but completely unset (empty).
func TestRunOne_SkipsOnUnsetVarsEnv(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dir, "code.fd")
	if err := os.WriteFile(code, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Arch{
		Name:        "test",
		EFIName:     "BOOT.EFI",
		QEMUBin:     "sh",
		QEMUArgsFor: func(string, string, string) []string { return nil },
		CodeFWEnv:   "CLOUDBOOT_MB_FW_CODE",
		VarsFWEnv:   "CLOUDBOOT_MB_FW_VARS_UNSET",
		WantSubstrs: []string{"x"},
	}
	t.Setenv("CLOUDBOOT_MB_FW_CODE", code)
	t.Setenv("CLOUDBOOT_MB_FW_VARS_UNSET", "")
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: time.Second}, a)
	if r.Status != "skip" {
		t.Errorf("status=%q, want skip; reason=%q", r.Status, r.Reason)
	}
	if !strings.Contains(r.Reason, "unset") {
		t.Errorf("reason=%q, want 'unset' substring", r.Reason)
	}
}

// TestRunOne_FailsWhenSerialNeverPrintsBanner drives a fake "QEMU"
// (sh) that exits cleanly without ever emitting the wanted
// substrings. runOne should classify that as fail (not skip) and
// surface the missing substring in Reason. This is the path that
// catches a regression in the tamago app itself.
func TestRunOne_FailsWhenSerialNeverPrintsBanner(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dir, "code.fd")
	if err := os.WriteFile(code, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Arch{
		Name:        "test",
		EFIName:     "BOOT.EFI",
		QEMUBin:     "sh",
		QEMUArgsFor: func(iso, code, vars string) []string {
			// Print something irrelevant, then exit fast.
			return []string{"-c", "echo not-the-banner; exit 0"}
		},
		CodeFWEnv:   "CLOUDBOOT_MB_FW_CODE",
		WantSubstrs: []string{"goroutine sum: 499500", "DONE"},
	}
	t.Setenv("CLOUDBOOT_MB_FW_CODE", code)
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: 5 * time.Second}, a)
	if r.Status != "fail" {
		t.Errorf("status=%q, want fail; reason=%q stdout=%q", r.Status, r.Reason, r.Stdout)
	}
	if !strings.Contains(r.Reason, "missing substrings") {
		t.Errorf("reason=%q, want 'missing substrings' substring", r.Reason)
	}
	if !strings.Contains(r.Stdout, "not-the-banner") {
		t.Errorf("stdout=%q, want it to contain the emitted line", r.Stdout)
	}
}

// TestRunOne_PassesWhenSerialPrintsBanner is the symmetric success
// path: the fake QEMU emits every required substring, runOne
// returns pass.
func TestRunOne_PassesWhenSerialPrintsBanner(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dir, "code.fd")
	if err := os.WriteFile(code, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Arch{
		Name:    "test",
		EFIName: "BOOT.EFI",
		QEMUBin: "sh",
		QEMUArgsFor: func(iso, code, vars string) []string {
			return []string{"-c", "printf 'hello from cloud-boot tamago/test UEFI board\\ngoroutine sum: 499500\\nDONE\\n'; exit 0"}
		},
		CodeFWEnv:   "CLOUDBOOT_MB_FW_CODE",
		WantSubstrs: []string{"hello from cloud-boot tamago/test", "goroutine sum: 499500", "DONE"},
	}
	t.Setenv("CLOUDBOOT_MB_FW_CODE", code)
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: 5 * time.Second}, a)
	if r.Status != "pass" {
		t.Errorf("status=%q, want pass; reason=%q stdout=%q", r.Status, r.Reason, r.Stdout)
	}
}

// TestRunOne_ExpectFailPolarity asserts that when an Arch is
// flagged ExpectFail=true, an actual failure yields "xfail" and a
// surprise pass yields "xpass" — the polarity flip that keeps the
// suite honest the day a firmware bug is fixed upstream.
func TestRunOne_ExpectFailPolarity(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dir, "code.fd")
	if err := os.WriteFile(code, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLOUDBOOT_MB_FW_CODE", code)

	// xfail: ExpectFail=true AND the banner never appears.
	xfailArch := Arch{
		Name:    "test-xfail",
		EFIName: "BOOT.EFI", QEMUBin: "sh",
		QEMUArgsFor: func(string, string, string) []string {
			return []string{"-c", "echo nope; exit 0"}
		},
		CodeFWEnv: "CLOUDBOOT_MB_FW_CODE",
		WantSubstrs: []string{"DONE"},
		ExpectFail: true,
	}
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: 5 * time.Second}, xfailArch)
	if r.Status != "xfail" {
		t.Errorf("xfail case: status=%q, want xfail; reason=%q", r.Status, r.Reason)
	}

	// xpass: ExpectFail=true but the banner DOES appear.
	xpassArch := Arch{
		Name:    "test-xpass",
		EFIName: "BOOT.EFI", QEMUBin: "sh",
		QEMUArgsFor: func(string, string, string) []string {
			return []string{"-c", "printf 'DONE\\n'; exit 0"}
		},
		CodeFWEnv: "CLOUDBOOT_MB_FW_CODE",
		WantSubstrs: []string{"DONE"},
		ExpectFail: true,
	}
	r2 := runOne(context.Background(), Opts{ISO: iso, Timeout: 5 * time.Second}, xpassArch)
	if r2.Status != "xpass" {
		t.Errorf("xpass case: status=%q, want xpass; reason=%q", r2.Status, r2.Reason)
	}
}

// TestRunOne_SkipsOnMissingQEMUBinary verifies the LookPath gate: a
// nonexistent qemu binary path leads to a skip, not a fail.
func TestRunOne_SkipsOnMissingQEMUBinary(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Use a deliberately-impossible binary name.
	a := Arch{
		Name:        "test",
		EFIName:     "BOOT.EFI",
		QEMUBin:     "qemu-system-does-not-exist-2026",
		QEMUArgsFor: func(string, string, string) []string { return nil },
		CodeFWEnv:   "CLOUDBOOT_NONEXISTENT_CODE",
		WantSubstrs: []string{"x"},
	}
	r := runOne(context.Background(), Opts{ISO: iso, Timeout: time.Second}, a)
	if r.Status != "skip" {
		t.Errorf("status=%q, want skip; reason=%q", r.Status, r.Reason)
	}
	if !strings.Contains(r.Reason, "not in PATH") {
		t.Errorf("reason=%q, want 'not in PATH'", r.Reason)
	}
}
