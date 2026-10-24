//go:build freebsd || linux || darwin

package batch

import (
	"errors"
	"os"
	"syscall"
)

func syncHandle(f *os.File) error {
	e := f.Sync()
	if errors.Is(e, syscall.EINVAL) || errors.Is(e, syscall.ENOTSUP) {
		return nil
	}
	return e
}
