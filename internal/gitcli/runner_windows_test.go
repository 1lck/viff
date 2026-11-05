//go:build windows

package gitcli

import (
	"context"
	"testing"
)

// Git is a console program. Started from the GUI build without the hidden
// console flags, Windows gives each command its own console window, which is
// what made the application flash black boxes on every refresh.
func TestCommandHidesConsoleWindow(t *testing.T) {
	r := &Runner{}
	cmd, err := r.command(context.Background(), "", []string{"status"})
	if err != nil {
		if KindOf(err) == ErrNoGit {
			t.Skip("git is not installed")
		}
		t.Fatalf("command: %v", err)
	}
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, git would open a console window")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow is false, git would open a console window")
	}
}
