//go:build windows

package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRunTreeCancelsDescendantWindows(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRunTreeWindowsHelper$", "--", "parent", ready)
	cmd.WaitDelay = 2 * time.Second
	done := make(chan error, 1)
	go func() { done <- RunTree(cmd) }()

	var child windows.Handle
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for child == 0 {
		select {
		case err := <-done:
			t.Fatalf("parent exited before its descendant started: %v", err)
		case <-ctx.Done():
			t.Fatal("descendant did not start")
		case <-tick.C:
			raw, err := os.ReadFile(ready)
			if err != nil || !strings.HasSuffix(string(raw), "\n") {
				continue
			}
			pid, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			child, err = windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	defer windows.CloseHandle(child)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled parent reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled parent did not exit")
	}
	if wait, err := windows.WaitForSingleObject(child, 2000); err != nil || wait != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant is still running: wait=%d err=%v", wait, err)
	}
}

func TestRunTreeWindowsHelper(t *testing.T) {
	if len(os.Args) != 5 || os.Args[2] != "--" {
		return
	}
	switch os.Args[3] {
	case "parent":
		cmd := exec.Command(os.Args[0], "-test.run=^TestRunTreeWindowsHelper$", "--", "child", os.Args[4])
		HideConsoleWindow(cmd)
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	case "child":
		if err := os.WriteFile(os.Args[4], []byte(fmt.Sprintln(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		<-time.After(30 * time.Second) // normally terminated by the test
	}
}
