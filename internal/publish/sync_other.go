//go:build !freebsd && !linux && !darwin

package publish

import (
	"errors"
	"os"
)

func unavailable(e error) bool     { return errors.Is(e, ErrNoReplace) }
func syncDirectory(*os.File) error { return nil }
