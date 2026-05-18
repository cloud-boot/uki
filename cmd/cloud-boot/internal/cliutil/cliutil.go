// Package cliutil holds tiny helpers shared across cloud-boot CLI
// subcommands. It lives under cmd/cloud-boot/internal/ because nothing
// outside cmd/cloud-boot has any business depending on it.
package cliutil

import (
	"fmt"
	"strings"
)

// ParsePlatform splits "linux/amd64" into ("linux", "amd64"). Both the
// `push artifact` and `push index` subcommands consume <os>/<arch> tokens,
// so the splitter lives here instead of being duplicated in each package.
func ParsePlatform(s string) (osName, arch string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid platform %q (want os/arch)", s)
	}
	return parts[0], parts[1], nil
}
