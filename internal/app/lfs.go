package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/observability"
)

type lfsReporter interface {
	LFSTrackedAttributes(context.Context, domain.Repo, *gitcli.LFSIndexCache) (*gitcli.LFSIndexCache, []string, error)
	LFS(context.Context, domain.Repo, []string, gitcli.LFSRuleCache) (gitcli.LFSStatus, gitcli.LFSRuleCache)
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
	rules lfsRulesState
	epoch uint64
}

type lfsRulesState struct {
	generation uint64
	cancel     context.CancelFunc
	pending    *lfsRuleInputs
	index      *gitcli.LFSIndexCache
	parsed     gitcli.LFSRuleCache
	failed     *lfsRuleInputs
	retryAt    time.Time
}

type lfsRuleInputs struct {
	head, branch string
	detached     bool
	indexKey     [32]byte
	untracked    []string
}

func (i lfsRuleInputs) equal(other lfsRuleInputs) bool {
	return i.head == other.head && i.branch == other.branch && i.detached == other.detached &&
		i.indexKey == other.indexKey && slices.Equal(i.untracked, other.untracked)
}

type lfsScan struct {
	cancel      context.CancelFunc
	unconfirmed bool
}

const lfsRuleRetryDelay = 5 * time.Second

// refreshLFS is called after publishing the working-copy snapshot. It
// extracts candidates without retaining the full status in a background task.
func (w *Workspace) refreshLFS(rs *RepoState, status domain.Status) {
	backend, ok := w.Git.(lfsReporter)
	if !ok {
		return
	}
	inputs := lfsRuleInputs{
		head: status.Branch.Head, branch: status.Branch.Branch,
		detached: status.Branch.Detached, indexKey: status.IndexKey,
	}
	for _, e := range status.Entries {
		if e.Untracked() && strings.HasSuffix(e.Path, "/.gitattributes") {
			inputs.untracked = append(inputs.untracked, e.Path)
		}
	}
	slices.Sort(inputs.untracked)
	w.detectLFSRules(rs, backend, inputs)
}

func (w *Workspace) detectLFSRules(rs *RepoState, backend lfsReporter, inputs lfsRuleInputs) {
	state := &rs.lfs.rules
	if state.cancel != nil {
		state.pending = &inputs
		return
	}
	if state.failed != nil && state.failed.equal(inputs) && w.now().Before(state.retryAt) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	state.cancel = cancel
	generation, index, parsed := state.generation, state.index, state.parsed
	repo := rs.Repo
	start := time.Now()
	go func() {
		defer cancel()
		nextIndex, tracked, err := backend.LFSTrackedAttributes(ctx, repo, index)
		var st gitcli.LFSStatus
		var next gitcli.LFSRuleCache
		if err == nil {
			paths := slices.Concat(tracked, inputs.untracked)
			st, next = backend.LFS(ctx, repo, paths, parsed)
			err = st.Err
		}
		if err == nil {
			err = ctx.Err()
		}
		w.post(func() {
			if w.Repo != rs {
				return
			}
			state.cancel = nil
			pending := state.pending
			state.pending = nil
			if generation == state.generation {
				if nextIndex != nil {
					state.index = nextIndex
				}
				if pending == nil || pending.equal(inputs) {
					if err == nil {
						state.parsed, state.failed = next, nil
						rs.LFS = st
					} else {
						state.failed, state.retryAt = &inputs, w.now().Add(lfsRuleRetryDelay)
						rs.LFS.Err = err
					}
					w.record(observability.Event{
						Event: observability.EventRepoLFS, Level: observability.LevelDebug,
						Repo: repo.Root, Op: "rules", Dur: time.Since(start),
						Fields: lfsEventFields(rs.LFS, rs.LFSStats),
					})
				}
			}
			if pending != nil {
				w.detectLFSRules(rs, backend, *pending)
			}
		})
	}()
}

// invalidateLFSRules requires a new complete status before detection can
// publish again. It preserves the last result for display on failure.
func invalidateLFSRules(rs *RepoState) {
	state := &rs.lfs.rules
	state.generation++
	state.index, state.parsed, state.pending, state.failed = nil, nil, nil, nil
	if state.cancel != nil {
		state.cancel()
	}
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
	invalidateLFSRules(rs)
	w.invalidateLFSStats(rs)
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
