//go:build windows

package batchstate

import (
	"golang.org/x/sys/windows"
	"os"
	"strings"
)

// Canonical resolves a path through its opened handle without requiring listing
// rights on unrelated ancestor directories.
func Canonical(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	b := make([]uint16, 1024)
	for {
		n, e := windows.GetFinalPathNameByHandle(windows.Handle(f.Fd()), &b[0], uint32(len(b)), 0)
		if e != nil {
			return "", e
		}
		if n >= uint32(len(b)) {
			b = make([]uint16, n+1)
			continue
		}
		p := windows.UTF16ToString(b[:n])
		if strings.HasPrefix(p, "\\\\?\\UNC\\") {
			return "\\\\" + p[8:], nil
		}
		return strings.TrimPrefix(p, "\\\\?\\"), nil
	}
}
