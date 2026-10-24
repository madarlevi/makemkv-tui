//go:build !freebsd && !linux && !darwin

package batch

import "os"

func syncHandle(*os.File) error { return nil }
