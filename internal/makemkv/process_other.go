//go:build !freebsd && !linux && !darwin

package makemkv

import "os/exec"

func configure(cmd *exec.Cmd)          {}
func forceProcess(cmd *exec.Cmd) error { return cmd.Process.Kill() }
