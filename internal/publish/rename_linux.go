//go:build linux

package publish

import (
	"golang.org/x/sys/unix"
	"os"
)

func renameNoReplace(a *os.File, from string, b *os.File, to string) error {
	return unix.Renameat2(int(a.Fd()), from, int(b.Fd()), to, unix.RENAME_NOREPLACE)
}
