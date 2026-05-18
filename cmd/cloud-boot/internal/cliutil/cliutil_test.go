package cliutil

import "testing"

func TestParsePlatform(t *testing.T) {
	osName, arch, err := ParsePlatform("linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if osName != "linux" || arch != "amd64" {
		t.Errorf("got %s/%s", osName, arch)
	}
}

func TestParsePlatform_Invalid(t *testing.T) {
	for _, in := range []string{"", "linux", "linux/", "/amd64"} {
		if _, _, err := ParsePlatform(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
