//go:build !windows

package process

import "os/exec"

// HideConsoleWindow is a no-op outside Windows: a child started from a
// terminal inherits it, and a child started from a desktop application has no
// console window to flash in the first place.
func HideConsoleWindow(cmd *exec.Cmd) {}
