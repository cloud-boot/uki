//go:build unix

package modpack

import "syscall"

func makeFIFO(path string) error {
	return syscall.Mkfifo(path, 0o644)
}
