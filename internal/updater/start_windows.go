//go:build windows

package updater

import (
	"os/exec"

	"viff/internal/process"
)

// startDetached launches the batch helper without a console window. Windows
// does not terminate a child when its parent exits, so the helper can wait for
// Viff to close before replacing viff.exe.
func startDetached(cmd *exec.Cmd) error {
	process.HideConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
