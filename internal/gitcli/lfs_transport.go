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

// Bash supports noninteractive job control where the system sh may not.
// Other shells must establish a process group before the worker can start.
var lfsLeaseScript = `shell=sh
if command -v bash >/dev/null 2>&1; then shell=bash; fi
exec "$shell" -c ` + shellQuote(lfsLeaseBody) + ` sh "$@"`

// A process group retains orphaned descendants after the Git parent exits.
// Only confirm cleanup once no live member remains; zombies await their OS
// reaper and no longer execute. Inspection failures keep the reservation.
const lfsLeaseBody = `proof=$1
shift
finish() { printf '\n%s\n' "$proof" >&2; exit "$1"; }
command -v awk >/dev/null || finish 126
ps -eo pgid=,stat= >/dev/null || finish 126
exec 3<&0
set -m
sh -c 'kill -0 "-$$" 2>/dev/null || exit 126; exec "$@"' sh "$@" </dev/null 3<&- &
scan=$!
set +m
(
	IFS= read -r ignored <&3 || :
	kill -KILL "-$scan" 2>/dev/null || :
) <&3 &
guard=$!
wait "$scan"
result=$?
kill "$guard" 2>/dev/null || :
wait "$guard" 2>/dev/null || :
kill -KILL "-$scan" 2>/dev/null || :
for attempt in 1 2 3 4 5 6 7 8 9 10; do
	snapshot=$(ps -eo pgid=,stat=) || exit 1
	live=$(printf '%s\n' "$snapshot" | awk -v group="$scan" '$1 == group && $2 !~ /^Z/ { print $1 }') || exit 1
	[ -z "$live" ] && finish "$result"
	sleep 0.01
done
exit 1
`
