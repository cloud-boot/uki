package iso

import (
	"strings"
	"testing"
)

func TestParseUKIList_Valid(t *testing.T) {
	got, err := parseUKIList([]string{
		"linux/amd64=boot-amd64.efi",
		"linux/arm64=boot-arm64.efi",
		"linux/riscv64=boot-riscv64.efi",
		"linux/loongarch64=boot-loongarch64.efi",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4", len(got))
	}
	wantEFINames := []string{"BOOTX64.EFI", "BOOTAA64.EFI", "BOOTRISCV64.EFI", "BOOTLOONGARCH64.EFI"}
	wantPaths := []string{"boot-amd64.efi", "boot-arm64.efi", "boot-riscv64.efi", "boot-loongarch64.efi"}
	for i, u := range got {
		if u.Arch.EFIName != wantEFINames[i] {
			t.Errorf("[%d] EFIName = %q, want %q", i, u.Arch.EFIName, wantEFINames[i])
		}
		if u.UKI != wantPaths[i] {
			t.Errorf("[%d] UKI = %q, want %q", i, u.UKI, wantPaths[i])
		}
	}
}

func TestParseUKIList_PreservesOrder(t *testing.T) {
	// arm64 first, amd64 second — order should match input.
	got, err := parseUKIList([]string{
		"linux/arm64=a.efi",
		"linux/amd64=b.efi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Arch.EFIName != "BOOTAA64.EFI" || got[1].Arch.EFIName != "BOOTX64.EFI" {
		t.Errorf("order not preserved: %v", got)
	}
}

func TestParseUKIList_Errors(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantErr  string
	}{
		{"no equals", "linux/amd64-foo.efi", "want linux/<arch>=<path>"},
		{"empty key", "=foo.efi", "want linux/<arch>=<path>"},
		{"empty path", "linux/amd64=", "want linux/<arch>=<path>"},
		{"no slash", "amd64=foo.efi", "want linux/<arch>"},
		{"empty arch", "linux/=foo.efi", "want linux/<arch>"},
		{"unknown arch", "linux/m68k=foo.efi", "unsupported arch"},
		{"sneaky arch", "linux/sparc64=foo.efi", "unsupported arch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseUKIList([]string{c.in})
			if err == nil {
				t.Fatalf("want error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err=%q, want substring %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestParseUKIList_EmptyList(t *testing.T) {
	got, err := parseUKIList(nil)
	if err != nil {
		t.Fatalf("nil input should produce empty list, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}
