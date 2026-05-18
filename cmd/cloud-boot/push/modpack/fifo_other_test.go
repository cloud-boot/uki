//go:build !unix

package modpack

import "errors"

func makeFIFO(string) error { return errors.New("mkfifo unsupported") }
