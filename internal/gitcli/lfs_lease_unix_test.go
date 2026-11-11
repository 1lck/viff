//go:build unix

package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func startTestLFSLease(ctx context.Context, cmd *exec.Cmd, proof string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := (&Runner{}).runLFSLease(ctx, cmd, "", []string{"lfs", "ls-files"}, proof)
		done <- err
	}()
	return done
}

func waitLFSWorker(t *testing.T, ctx context.Context, done <-chan error, ready string) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			return
		}
		select {
		case <-tick.C:
		case err := <-done:
			t.Fatalf("worker exited before becoming ready: %v", err)
		case <-ctx.Done():
			t.Fatal("worker did not start")
		}
	}
}

func assertLFSProcessesExited(t *testing.T, pids ...string) {
	t.Helper()
	states, err := exec.Command("ps", "-eo", "pid=,stat=").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(states), "\n") {
		fields := strings.Fields(line)
		// Container PID 1 may defer reaping an already-dead grandchild.
		if len(fields) >= 2 && slices.Contains(pids, fields[0]) && !strings.HasPrefix(fields[1], "Z") {
			t.Fatalf("exit confirmed while process %s was still alive", fields[0])
		}
	}
}

func TestLFSLeaseReapsDescendantsAfterExit(t *testing.T) {
	for _, code := range []int{0, 1} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			proof := rand.Text()
			ready := filepath.Join(t.TempDir(), "child")
			worker := `sleep 60 >/dev/null 2>&1 & printf '%s\n' "$!" > "$1"; exit "$2"`
			cmd := exec.CommandContext(ctx, "sh", "-c", lfsLeaseScript, "sh", proof, "sh", "-c", worker, "sh", ready, strconv.Itoa(code))
			_, err := (&Runner{}).runLFSLease(ctx, cmd, "", []string{"lfs", "ls-files"}, proof)
			raw, readErr := os.ReadFile(ready)
			if readErr != nil {
				t.Fatalf("child PID: %v (run: %v)", readErr, err)
			}
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if code == 0 && err != nil || code != 0 && exitCodeOf(err) != code || errors.Is(err, ErrLFSReapUnconfirmed) {
				t.Fatalf("exit %d: %v", code, err)
			}
			assertLFSProcessesExited(t, strconv.Itoa(pid))
		})
	}
}

func TestLFSLeaseCancelReapsThreeLevels(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "pids")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	proof := rand.Text()
	worker := `printf '%s\n' "$$" >> "$1"
sh -c 'printf "%s\n" "$$" >> "$1"; sleep 60 & printf "%s\n" "$!" >> "$1"; wait' sh "$1" &
wait`
	cmd := exec.CommandContext(ctx, "sh", "-c", lfsLeaseScript, "sh", proof, "sh", "-c", worker, "sh", ready)
	done := startTestLFSLease(ctx, cmd, proof)
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
	assertLFSProcessesExited(t, pids...)
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
	done := startTestLFSLease(ctx, cmd, proof)
	waitLFSWorker(t, ctx, done, ready)
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
		defer cancel()
		proof := rand.Text()
		worker := `touch "$1"; while [ ! -f "$2" ]; do sleep 0.01; done`
		cmd := exec.CommandContext(ctx, "sh", "-c", lfsLeaseScript, "sh", proof, "sh", "-c", worker, "sh", ready, finish)
		done := startTestLFSLease(ctx, cmd, proof)
		waitLFSWorker(t, ctx, done, ready)
		if err := os.WriteFile(finish, nil, 0o600); err != nil {
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
