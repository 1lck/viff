//go:build unix

package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLFSLeaseCancelReapsThreeLevels(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "pids")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	proof := rand.Text()
	worker := `printf '%s\n' "$$" >> "$1"
sh -c 'printf "%s\n" "$$" >> "$1"; sleep 60 & printf "%s\n" "$!" >> "$1"; wait' sh "$1" &
wait`
	cmd := exec.CommandContext(ctx, "sh", "-c", lfsLeaseScript, "sh", proof, "sh", "-c", worker, "sh", ready)
	done := make(chan error, 1)
	go func() {
		_, err := (&Runner{}).runLFSLease(ctx, cmd, "", []string{"lfs", "ls-files"}, proof)
		done <- err
	}()
	var pids []string
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for len(pids) != 3 {
		select {
		case err := <-done:
			t.Fatalf("scan exited before its descendants started: %v", err)
		case <-ctx.Done():
			t.Fatal("descendants did not start")
		case <-ticker.C:
			raw, _ := os.ReadFile(ready)
			pids = strings.Fields(string(raw))
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || errors.Is(err, ErrLFSReapUnconfirmed) || KindOf(err) != ErrCancelled {
			t.Fatalf("expected confirmed cancellation: %v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("scan did not exit after closing the lease")
	}
	for _, pid := range pids {
		out, err := exec.Command("ps", "-p", pid, "-o", "stat=").Output()
		// A container's PID 1 may defer reaping an already-dead grandchild.
		if err == nil && len(strings.TrimSpace(string(out))) > 0 && !strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
			t.Fatalf("process %s survived cancellation: %s", pid, out)
		}
	}
}

func TestLFSLeaseSuccessAndMissingProof(t *testing.T) {
	for _, tt := range []struct {
		name  string
		lease bool
	}{
		{"normal_success", true},
		{"transport_exited_without_confirmation", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			proof := rand.Text()
			args := []string{"-c", "printf result"}
			if tt.lease {
				args = []string{"-c", lfsLeaseScript, "sh", proof, "sh", "-c", "printf result"}
			}
			cmd := exec.CommandContext(ctx, "sh", args...)
			out, err := (&Runner{}).runLFSLease(ctx, cmd, "", []string{"lfs", "ls-files"}, proof)
			if tt.lease {
				if err != nil || string(out) != "result" {
					t.Fatalf("successful lease: %q, %v", out, err)
				}
			} else if !errors.Is(err, ErrLFSReapUnconfirmed) {
				t.Fatalf("lost proof must preserve the reservation: %v", err)
			}
		})
	}
}

func TestLFSLeaseInspectionFailureDoesNotConfirmExit(t *testing.T) {
	dir := t.TempDir()
	realPS, err := exec.LookPath("ps")
	if err != nil {
		t.Skip("ps is unavailable")
	}
	// The startup capability check succeeds; inspection during cancellation
	// fails. The worker then exits normally, which must not mask that failure.
	stub := `#!/bin/sh
if [ -f "$VIFF_LEASE_CHECKED" ]; then
	touch "$VIFF_LEASE_FAILED"
	exit 1
fi
touch "$VIFF_LEASE_CHECKED"
exec "$VIFF_REAL_PS" "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "ps"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proof := rand.Text()
	ready, failed := filepath.Join(dir, "ready"), filepath.Join(dir, "failed")
	worker := `touch "$1"; while [ ! -f "$2" ]; do sleep 0.01; done`
	cmd := exec.CommandContext(ctx, "sh", "-c", lfsLeaseScript, "sh", proof, "sh", "-c", worker, "sh", ready, failed)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"),
		"VIFF_REAL_PS="+realPS, "VIFF_LEASE_CHECKED="+filepath.Join(dir, "checked"), "VIFF_LEASE_FAILED="+failed)
	done := make(chan error, 1)
	go func() {
		_, err := (&Runner{}).runLFSLease(ctx, cmd, "", []string{"lfs", "ls-files"}, proof)
		done <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			t.Fatalf("wrapper exited before the worker started: %v", err)
		case <-ctx.Done():
			t.Fatal("worker did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrLFSReapUnconfirmed) {
			t.Fatalf("failed inspection returned an exit proof: %v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("wrapper did not finish")
	}
}

func TestLFSLeaseCancelNormalCompletionRace(t *testing.T) {
	for range 5 {
		dir := t.TempDir()
		ready, finish := filepath.Join(dir, "ready"), filepath.Join(dir, "finish")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		proof := rand.Text()
		worker := `touch "$1"; while [ ! -f "$2" ]; do sleep 0.01; done`
		cmd := exec.CommandContext(ctx, "sh", "-c", lfsLeaseScript, "sh", proof, "sh", "-c", worker, "sh", ready, finish)
		done := make(chan error, 1)
		go func() {
			_, err := (&Runner{}).runLFSLease(ctx, cmd, "", []string{"lfs", "ls-files"}, proof)
			done <- err
		}()
		ticker := time.NewTicker(10 * time.Millisecond)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			select {
			case <-ticker.C:
			case err := <-done:
				cancel()
				ticker.Stop()
				t.Fatalf("worker exited early: %v", err)
			case <-ctx.Done():
				ticker.Stop()
				t.Fatal("worker did not start")
			}
		}
		ticker.Stop()
		if err := os.WriteFile(finish, nil, 0o600); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
		select {
		case err := <-done:
			if errors.Is(err, ErrLFSReapUnconfirmed) {
				t.Fatalf("completion/cancel race lost its proof: %v", err)
			}
		case <-time.After(7 * time.Second):
			t.Fatal("completion/cancel race did not finish")
		}
	}
}
