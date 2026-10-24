//go:build freebsd || linux || darwin

package publish

import (
	"errors"
	"os"
	"syscall"
)

func unavailable(e error) bool {
	return errors.Is(e, ErrNoReplace) || errors.Is(e, syscall.ENOSYS) || errors.Is(e, syscall.EOPNOTSUPP) || errors.Is(e, syscall.EINVAL) || errors.Is(e, syscall.EXDEV)
}
func syncDirectory(f *os.File) error {
	e := f.Sync()
	if errors.Is(e, syscall.EINVAL) || errors.Is(e, syscall.ENOTSUP) {
		return nil
	}
	return e
}
