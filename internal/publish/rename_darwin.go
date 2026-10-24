//go:build darwin

package publish

import (
	"golang.org/x/sys/unix"
	"os"
)

func renameNoReplace(a *os.File, from string, b *os.File, to string) error {
	return unix.RenameatxNp(int(a.Fd()), from, int(b.Fd()), to, unix.RENAME_EXCL)
}
