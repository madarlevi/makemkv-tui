//go:build freebsd || linux || darwin

package batchstate

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(f *os.File) error   { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }
func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
func SyncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	e = f.Sync()
	if errors.Is(e, syscall.EINVAL) || errors.Is(e, syscall.ENOTSUP) {
		e = nil
	}
	return errors.Join(e, f.Close())
}
