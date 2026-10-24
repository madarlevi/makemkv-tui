//go:build !windows

package batchstate

import "path/filepath"

func Canonical(path string) (string, error) { return filepath.EvalSymlinks(path) }
