//go:build !linux && !darwin

package publish

import "os"

// FreeBSD's exposed renameat interface has no no-replace flag. Plain rename
// plus an existence check would race; use the explicit exclusive-copy fallback.
func renameNoReplace(*os.File, string, *os.File, string) error { return ErrNoReplace }
