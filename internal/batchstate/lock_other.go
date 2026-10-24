//go:build !freebsd && !linux && !darwin && !windows

package batchstate

import (
	"fmt"
	"os"
)

func lockFile(*os.File) error   { return fmt.Errorf("batch locking is unavailable on this OS") }
func unlockFile(*os.File) error { return nil }
func SyncDir(string) error      { return nil }
