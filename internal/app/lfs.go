package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/observability"
	"viff/internal/sshpath"
)

type lfsReporter interface {
	LFS(context.Context, domain.Repo) gitcli.LFSStatus
	LFSInputs(context.Context, domain.Repo) (gitcli.LFSInputs, error)
	LFSFiles(context.Context, domain.Repo) ([]string, error)
}

type LFSStatsStatus string

const (
	LFSStatsUnqueried LFSStatsStatus = ""
	LFSStatsLoading   LFSStatsStatus = "loading"
	LFSStatsCurrent   LFSStatsStatus = "current"
	LFSStatsStale     LFSStatsStatus = "stale"
	LFSStatsFailed    LFSStatsStatus = "failed"
)

// LFSStats retains the last successful result even when a later query fails
// or HEAD/the index moves. UpdatedAt distinguishes an empty result from none.
type LFSStats struct {
	Status    LFSStatsStatus
	Files     []string
	UpdatedAt time.Time
	Code      string
}

type lfsState struct {
	key           string
	at            time.Time
	detectCancel  context.CancelFunc
	detectPending bool
	pendingKey    string
	epoch         uint64
}

type lfsScan struct {
	cancel      context.CancelFunc
	unconfirmed bool
}

const lfsRecheckInterval = 30 * time.Second

// refreshLFS is called after publishing the working-copy snapshot. It
// coalesces overlapping detection requests and never runs a file listing.
func (w *Workspace) refreshLFS(rs *RepoState, key string, meta bool) {
	backend, ok := w.Git.(lfsReporter)
	if !ok {
		return
	}
	state := &rs.lfs
	if state.detectCancel != nil {
		if meta || key != state.key || state.detectPending {
			state.detectPending, state.pendingKey = true, key
		}
		return
	}
	if !meta && key == state.key && !state.at.IsZero() && w.now().Sub(state.at) < lfsRecheckInterval {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	state.key, state.detectCancel = key, cancel
	repo := rs.Repo
	start := time.Now()
	go func() {
		defer cancel()
		st := backend.LFS(ctx, repo)
		w.post(func() {
			if w.Repo != rs {
				return
			}
			state.detectCancel = nil
			pending, next := state.detectPending, state.pendingKey
			state.detectPending = false
			// A result from before a known root rule change is obsolete.
			if !pending || next == key {
				rs.LFS, state.at = st, w.now()
				w.record(observability.Event{
					Event: observability.EventRepoLFS, Level: observability.LevelDebug,
					Repo: repo.Root, Op: "rules", Dur: time.Since(start),
					Fields: lfsEventFields(st, rs.LFSStats),
				})
			}
			if pending {
				w.refreshLFS(rs, next, true)
			}
		})
	}()
}

// SupportsLFSStats reports whether the active adapter provides a listing.
func (w *Workspace) SupportsLFSStats() bool {
	_, ok := w.Git.(lfsReporter)
	return w.Repo != nil && ok
}

// CanRefreshLFSStats gates the explicit action, including while an earlier
// scan of this worktree is still exiting.
func (w *Workspace) CanRefreshLFSStats() bool {
	rs := w.Repo
	return w.SupportsLFSStats() && rs.StatusLoaded && rs.LFS.Installed &&
		!w.Busy() && w.lfsScans[rs.rootKey] == nil
}

// RefreshLFSStats starts one explicitly requested enumeration. It is
// independent of status refresh and does not hold the repository write lock.
func (w *Workspace) RefreshLFSStats() {
	if !w.CanRefreshLFSStats() {
		return
	}
	rs := w.Repo
	repo, epoch := rs.Repo, rs.lfs.epoch
	backend := w.Git.(lfsReporter)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	scan := &lfsScan{cancel: cancel}
	if w.lfsScans == nil {
		w.lfsScans = make(map[string]*lfsScan)
	}
	w.lfsScans[rs.rootKey] = scan
	rs.LFSStats.Status, rs.LFSStats.Code = LFSStatsLoading, ""
	start := time.Now()
	go func() {
		defer cancel()
		files, stale, err := queryLFSStats(ctx, backend, repo)
		w.post(func() {
			if w.lfsScans[rs.rootKey] != scan {
				return
			}
			if !errors.Is(err, gitcli.ErrLFSReapUnconfirmed) {
				delete(w.lfsScans, rs.rootKey)
			} else {
				scan.unconfirmed = true
				// A reopened generation needs the reason its action remains
				// disabled, without inheriting an old generation's files.
				if current := w.Repo; current != nil && current != rs && current.rootKey == rs.rootKey {
					current.LFSStats.Status, current.LFSStats.Code = LFSStatsFailed, gitcli.CodeOf(err)
				}
			}
			status := LFSStatsCurrent
			code := ""
			switch {
			case errors.Is(err, gitcli.ErrLFSReapUnconfirmed):
				status, code = LFSStatsFailed, gitcli.CodeOf(err)
			case w.Repo != rs || rs.lfs.epoch != epoch:
				status = LFSStatsStale
			case stale:
				status = LFSStatsStale
				w.Refresh(true)
			case err != nil:
				status, code = LFSStatsFailed, gitcli.CodeOf(err)
			default:
				rs.LFSStats.Files, rs.LFSStats.UpdatedAt = files, w.now()
			}
			if w.Repo == rs && (rs.lfs.epoch == epoch || errors.Is(err, gitcli.ErrLFSReapUnconfirmed)) {
				rs.LFSStats.Status, rs.LFSStats.Code = status, code
			}
			w.record(observability.Event{
				Event: observability.EventRepoLFS, Level: observability.LevelInfo,
				Repo: repo.Root, Op: "stats", Dur: time.Since(start), Code: code,
				Fields: map[string]any{"status": status, "files": len(rs.LFSStats.Files)},
			})
		})
	}()
}

func queryLFSStats(ctx context.Context, backend lfsReporter, repo domain.Repo) ([]string, bool, error) {
	before, err := backend.LFSInputs(ctx, repo)
	if err != nil {
		return nil, false, err
	}
	files, err := backend.LFSFiles(ctx, repo)
	if err != nil {
		return nil, false, err
	}
	after, err := backend.LFSInputs(ctx, repo)
	if err != nil {
		return nil, false, err
	}
	return files, before != after, nil
}

func lfsStatusInputsChanged(before, after domain.Status) bool {
	a, b := before.Branch, after.Branch
	return a.Head != b.Head || a.Branch != b.Branch || a.Detached != b.Detached || before.IndexKey != after.IndexKey
}

func (w *Workspace) invalidateLFSStats(rs *RepoState) {
	rs.lfs.epoch++
	if scan := w.lfsScans[rs.rootKey]; scan != nil && scan.unconfirmed {
		rs.LFSStats.Status, rs.LFSStats.Code = LFSStatsFailed, gitcli.CodeOf(gitcli.ErrLFSReapUnconfirmed)
		return
	}
	if rs.LFSStats.Status != LFSStatsUnqueried {
		rs.LFSStats.Status, rs.LFSStats.Code = LFSStatsStale, ""
	}
	if scan := w.lfsScans[rs.rootKey]; scan != nil {
		scan.cancel()
	}
}

func (w *Workspace) cancelLFS(rs *RepoState) {
	if rs.lfs.detectCancel != nil {
		rs.lfs.detectCancel()
	}
	w.invalidateLFSStats(rs)
}

// Root .gitattributes changes are checked cheaply on each refresh. Nested
// rule discovery keeps the existing metadata/interval cadence.
func lfsAttributesKey(root string) string {
	if sshpath.Is(root) {
		return "ssh"
	}
	st, err := os.Stat(filepath.Join(root, ".gitattributes"))
	if err != nil {
		return "missing"
	}
	return strconv.FormatInt(st.Size(), 10) + ":" + strconv.FormatInt(st.ModTime().UnixNano(), 10)
}

// Only flags, counts and stable codes are recorded, never paths or rules.
func lfsEventFields(st gitcli.LFSStatus, stats LFSStats) map[string]any {
	fields := map[string]any{
		"installed": st.Installed,
		"managed":   st.Managed,
		"patterns":  len(st.Patterns),
		"files":     len(stats.Files),
		"status":    stats.Status,
	}
	if st.Err != nil {
		fields["error"] = gitcli.CodeOf(st.Err)
	}
	return fields
}
