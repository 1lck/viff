//go:build !unix && !windows

package process

import "os/exec"

func RunTree(cmd *exec.Cmd) error { return cmd.Run() }
