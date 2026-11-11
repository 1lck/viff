//go:build unix

package process

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunTreeReapsDescendantsAfterExit(t *testing.T) {
	for _, code := range []int{0, 1} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", `sleep 60 >/dev/null 2>&1 & printf '%s\n' "$!"; exit "$1"`, "sh", strconv.Itoa(code))
			cmd.WaitDelay = time.Second
			var out bytes.Buffer
			cmd.Stdout = &out
			err := RunTree(cmd)
			pid, parseErr := strconv.Atoi(strings.TrimSpace(out.String()))
			if parseErr != nil {
				t.Fatalf("child PID %q: %v (run: %v)", out.String(), parseErr, err)
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			var exited *exec.ExitError
			if code == 0 && err != nil || code != 0 && (!errors.As(err, &exited) || exited.ExitCode() != code) {
				t.Fatalf("exit %d: %v", code, err)
			}
			deadline := time.NewTimer(2 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			for {
				states, err := exec.Command("ps", "-eo", "pid=,stat=").Output()
				if err != nil {
					t.Fatal(err)
				}
				live := false
				for _, line := range strings.Split(string(states), "\n") {
					fields := strings.Fields(line)
					live = live || len(fields) >= 2 && fields[0] == strconv.Itoa(pid) && !strings.HasPrefix(fields[1], "Z")
				}
				if !live {
					return
				}
				select {
				case <-tick.C:
				case <-deadline.C:
					t.Fatalf("child %d survived parent exit", pid)
				}
			}
		})
	}
}
