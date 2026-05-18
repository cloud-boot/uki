package modpack

import (
	"compress/gzip"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloud-boot/init/pkg/cpio"
)

// packModules walks src and emits a gzipped cpio (newc) tree mirroring
// /lib/modules/<release>/<...> into out. The src path is expected to end in
// a kernel release directory; the cpio entries are rooted at
// "lib/modules/<basename(src)>/".
func packModules(src, out string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("modpack: --src must be a directory")
	}
	release := filepath.Base(strings.TrimRight(src, string(filepath.Separator)))
	rootInCpio := path.Join("lib", "modules", release)

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	w := cpio.NewWriter(gz)

	if err := w.WriteDir("lib", 0o755); err != nil {
		return err
	}
	if err := w.WriteDir("lib/modules", 0o755); err != nil {
		return err
	}

	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			rel = ""
		}
		name := path.Join(rootInCpio, filepath.ToSlash(rel))

		ftype, err := d.Info()
		if err != nil {
			return err
		}
		mode := uint32(ftype.Mode().Perm())
		switch {
		case d.IsDir():
			return w.WriteDir(name, mode)
		case d.Type()&os.ModeSymlink != 0:
			tgt, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return w.WriteSymlink(name, tgt, mode)
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return w.WriteFile(cpio.Header{Name: name, Mode: 0o100000 | mode}, data)
		default:
			// Skip sockets/fifos/devices.
			return nil
		}
	})
	if err != nil {
		return err
	}
	return w.Close()
}
