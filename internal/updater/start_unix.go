//go:build !windows

package updater

import (
	"os"
	"os/exec"
	"syscall"
)

// startDetached launches a helper in its own session so it survives Viff
// exiting and never holds the terminal or parent process group open.
func startDetached(cmd *exec.Cmd) error {
	if devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0); err == nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
		defer devNull.Close()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
