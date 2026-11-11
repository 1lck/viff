package gitcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"viff/internal/sshpath"
)

// The transport process is local, but its children run on another kernel.
// Keep stdin open as a lease. Cancellation closes it, asking the wrapper to
// reap the scan there before ssh/wsl returns; a dropped connection closes the
// same lease. No daemon or repository files are needed.
func (r *Runner) lfsTransportFiles(ctx context.Context, dir string, args []string) ([]byte, error) {
	proof := rand.Text()
	full := append(append([]string{}, baseArgs...), args...)
	var cmd *exec.Cmd
	var err error
	if sshpath.Is(dir) {
		target, parseErr := sshpath.Parse(dir)
		if parseErr != nil {
			return nil, parseErr
		}
		program := []string{"sh", "-c", shellQuote(lfsLeaseScript), "sh", shellQuote(proof), "git", "-C", shellQuote(target.Path)}
		for _, arg := range full {
			program = append(program, shellQuote(arg))
		}
		cmd, err = r.sshCommand(ctx, target, "env GIT_TERMINAL_PROMPT=0 GIT_OPTIONAL_LOCKS=0 LC_ALL=C LANGUAGE=C "+strings.Join(program, " "))
	} else {
		program := append([]string{"sh", "-c", lfsLeaseScript, "sh", proof, "git"}, full...)
		cmd, err = r.wslProgram(ctx, dir, program)
	}
	if err != nil {
		return nil, err
	}
	return r.runLFSLease(ctx, cmd, dir, args, proof)
}

// ErrLFSReapUnconfirmed means a transport exited without the wrapper's exit
// proof. Callers retain the worktree reservation, preventing a second scan
// when the first scan's lifetime is uncertain.
var ErrLFSReapUnconfirmed = errors.New("Git LFS scan exit was not confirmed")

func (r *Runner) runLFSLease(ctx context.Context, cmd *exec.Cmd, dir string, args []string, proof string) ([]byte, error) {
	lease, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Git LFS query lease: %w", err)
	}
	defer lease.Close()
	cmd.Cancel = lease.Close
	cmd.WaitDelay = 5 * time.Second
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: stderrLimit}
	tail := &lfsProofBuffer{proof: "\n" + proof + "\n", stderr: stderr}
	cmd.Stdout, cmd.Stderr = &stdout, tail
	err = cmd.Run()
	if err != nil {
		err = r.wrap(ctx, dir, args, err, stderr.buf.String(), stdout.String())
	}
	if !tail.confirmed() {
		if cmd.ProcessState == nil {
			// No process started, so there cannot be a remote worker.
			return nil, err
		}
		return nil, errors.Join(ErrLFSReapUnconfirmed, err)
	}
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

type lfsProofBuffer struct {
	stderr *limitedBuffer
	tail   []byte
	proof  string
}

func (b *lfsProofBuffer) Write(p []byte) (int, error) {
	n, _ := b.stderr.Write(p)
	b.tail = append(b.tail, p...)
	if len(b.tail) > 256 {
		b.tail = b.tail[len(b.tail)-256:]
	}
	return n, nil
}

func (b *lfsProofBuffer) confirmed() bool { return bytes.HasSuffix(b.tail, []byte(b.proof)) }

// Freeze each process before discovering its children, so it cannot fork
// after the snapshot. Reap leaves before their parent. A platform without the
// required ps/awk fails before a scan starts rather than leaking a worker.
const lfsLeaseScript = `proof=$1
shift
finish() { printf '\n%s\n' "$proof" >&2; exit "$1"; }
command -v awk >/dev/null || finish 126
ps -eo pid=,ppid= >/dev/null || finish 126
stop_tree() {
	kill -STOP "$1" 2>/dev/null || return 0
	snapshot=$(ps -eo pid=,ppid=) || { kill -CONT "$1" 2>/dev/null; return 1; }
	children=$(printf '%s\n' "$snapshot" | awk -v parent="$1" '$2 == parent {print $1}') || { kill -CONT "$1" 2>/dev/null; return 1; }
	for child in $children; do
		stop_tree "$child" || { kill -CONT "$1" 2>/dev/null; return 1; }
	done
	kill -KILL "$1" 2>/dev/null || :
}
exec 3<&0
"$@" </dev/null 3<&- &
scan=$!
(
	IFS= read -r ignored <&3 || :
	stop_tree "$scan"
) <&3 &
guard=$!
wait "$scan"
result=$?
kill "$guard" 2>/dev/null || :
wait "$guard" 2>/dev/null
guard_result=$?
# 143 is the guard we stopped after normal scanner completion. A guard that
# failed while inspecting a subtree must never produce an exit proof.
case "$guard_result" in 0|143) ;; *) exit 1 ;; esac
finish "$result"
`
