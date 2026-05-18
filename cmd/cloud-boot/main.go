// cloud-boot is the host-side CLI for the cloud-boot toolchain. It bundles
// what used to ship as two separate binaries — cloud-boot-build (ISO/UKI
// assembly) and cloud-boot-push (OCI artifact upload) — into one cobra
// command with two top-level subcommands:
//
//	cloud-boot build   …    build a UEFI UKI/ISO from a kernel + plan
//	cloud-boot push    …    upload artifacts/plans/indexes to an OCI registry
//
// Run `cloud-boot <command> --help` for the per-command flags.
package main

import (
	"fmt"
	"os"
)

// osExit and logFatal are overridable for tests so a single bad-flow call
// neither aborts the test binary nor writes to the real stderr.
var (
	osExit   = os.Exit
	logFatal = func(v ...any) {
		fmt.Fprintln(os.Stderr, v...)
		os.Exit(1)
	}
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// cobra has already printed the error+usage; just exit non-zero.
		osExit(1)
	}
}
