package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/observability"
)

// TestLFSStateFollowsAttributes opens a repository whose attributes select
// filter=lfs while git-lfs is simulated missing, then edits the rules and
// checks that a plain refresh notices the change.
func TestLFSStateFollowsAttributes(t *testing.T) {
	f := newFixture(t)
	f.write(".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	f.w.Git.(*gitcli.Runner).LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git: 'lfs' is not a git command")
	}
	f.open()

	rs := f.w.Repo
	f.settle(func() bool { return !rs.lfs.at.IsZero() })
	if !rs.LFS.Managed || rs.LFS.Installed || !rs.LFS.NeedsInstall() {
		t.Fatalf("LFS state = %+v, want managed rules without git-lfs", rs.LFS)
	}
	if len(rs.LFS.Patterns) != 1 || rs.LFS.Patterns[0] != "*.bin" {
		t.Fatalf("Patterns = %v", rs.LFS.Patterns)
	}

	// The root .gitattributes fingerprint changed, so the next refresh must
	// re-run detection even though it is not a metadata refresh.
	f.write(".gitattributes", "*.bin text\n")
	f.w.Refresh(false)
	f.settle(func() bool { return f.idle() && !f.w.Repo.LFS.Managed })
	if got := f.w.Repo.LFS.Patterns; len(got) != 0 {
		t.Fatalf("Patterns after removing the rules = %v", got)
	}
}

// TestLFSDetectionIsThrottled pins the cache: an unchanged repository does not
// probe or list LFS files on every file-system event, while a metadata refresh
// (the refresh button, or the end of a write task) re-runs it.
func TestLFSDetectionIsThrottled(t *testing.T) {
	f := newFixture(t)
	f.write(".gitattributes", "*.bin filter=lfs\n")
	f.w.Git.(*gitcli.Runner).LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git: 'lfs' is not a git command")
	}
	f.open()

	f.settle(func() bool { return !f.w.Repo.lfs.at.IsZero() })
	first := f.w.Repo.lfs.at
	if first.IsZero() {
		t.Fatal("opening the repository did not run LFS detection")
	}
	seq := f.w.Repo.statusSeq
	f.w.Refresh(false)
	f.settle(func() bool {
		rs := f.w.Repo
		return rs.statusSeq > seq && !rs.refreshing && f.idle()
	})
	if got := f.w.Repo.lfs.at; !got.Equal(first) {
		t.Fatalf("plain refresh re-ran LFS detection: %v -> %v", first, got)
	}

	f.w.Refresh(true)
	f.settle(func() bool {
		rs := f.w.Repo
		return rs.statusSeq > seq+1 && !rs.refreshing && rs.lfs.at.After(first) && f.idle()
	})
	if got := f.w.Repo.lfs.at; !got.After(first) {
		t.Fatalf("metadata refresh did not re-run LFS detection: %v -> %v", first, got)
	}
}

// TestLFSEventFieldsKeepCountsOnly guards the privacy rule: the refresh event
// carries flags and counts, never a pattern, path or file name.
func TestLFSEventFieldsKeepCountsOnly(t *testing.T) {
	st := gitcli.LFSStatus{
		Installed: false,
		Patterns:  []string{"*.bin", "assets/*.psd"},
		Managed:   true,
	}
	fields := lfsEventFields(st, LFSStats{})
	if fields["installed"] != false || fields["managed"] != true {
		t.Fatalf("fields = %v", fields)
	}
	if fields["patterns"] != 2 || fields["files"] != 0 {
		t.Fatalf("counts = %v", fields)
	}
	if _, ok := fields["error"]; ok {
		t.Fatalf("unexpected error field: %v", fields)
	}

	st.Err = errors.New("unreadable attributes")
	fields = lfsEventFields(st, LFSStats{})
	if got := fields["error"]; got != "git.failed" {
		t.Fatalf("error = %v, want the classified code", got)
	}
}

func TestLFSListingCommandLogOmitsRawFailures(t *testing.T) {
	f := newFixture(t)
	obs := &fakeObs{}
	f.w.SetRecorder(obs, obs)
	err := &gitcli.Error{Kind: gitcli.ErrOther, Stderr: "private-assets/model.bin failed"}
	f.w.logCommand(f.dir, []string{"lfs", "ls-files", "--json"}, time.Minute, err)
	f.w.Drain()
	if len(f.w.Console) != 0 || len(obs.eventsNamed(observability.EventGitFail)) != 0 {
		t.Fatal("LFS listing raw output reached the Console or event log")
	}
	f.w.logCommand(f.dir, []string{"lfs", "version"}, time.Second, err)
	f.w.Drain()
	if len(f.w.Console) != 1 || len(obs.eventsNamed(observability.EventGitFail)) != 1 {
		t.Fatal("the filter suppressed failures outside file enumeration")
	}
}

// The fingerprint distinguishes size and modification time changes and treats
// a missing file as its own state; the app test above only exercises the first
// two indirectly.
func TestLFSAttributesKey(t *testing.T) {
	dir := t.TempDir()
	if got := lfsAttributesKey(dir); got != "missing" {
		t.Fatalf("missing key = %q", got)
	}
	f := newFixture(t)
	f.write(".gitattributes", "*.bin filter=lfs\n")
	first := lfsAttributesKey(f.dir)
	if first == "missing" {
		t.Fatal("existing .gitattributes reported missing")
	}
	f.write(".gitattributes", "*.bin filter=lfs\n*.psd filter=lfs\n")
	if second := lfsAttributesKey(f.dir); second == first {
		t.Fatalf("key did not change with the file: %q", second)
	}
}

// Both automatic paths, metadata and time-based rule checks, stay enumeration
// free. Advance the clock so the test doesn't wait five real minutes.
func TestLFSAutomaticRefreshNeverEnumerates(t *testing.T) {
	f := newFixture(t)
	f.write(".gitattributes", "*.bin filter=lfs\n")
	var listings atomic.Int32
	f.w.Git.(*gitcli.Runner).LFSCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "version" {
			return []byte("git-lfs/test"), nil
		}
		listings.Add(1)
		return nil, nil
	}
	start := time.Now()
	var elapsed atomic.Int64
	f.w.Now = func() time.Time { return start.Add(time.Duration(elapsed.Load())) }
	f.open()
	rs := f.w.Repo
	f.settle(func() bool { return rs.lfs.at.Equal(start) })
	for _, step := range []struct {
		offset time.Duration
		meta   bool
	}{
		{time.Minute, false}, {2 * time.Minute, true}, {5 * time.Minute, false},
	} {
		elapsed.Store(int64(step.offset))
		f.w.Refresh(step.meta)
		f.settle(func() bool { return f.idle() && rs.lfs.at.Equal(start.Add(step.offset)) })
	}
	if got := listings.Load(); got != 0 || !rs.LFS.Managed || rs.LFSStats.Status != LFSStatsUnqueried {
		t.Fatalf("automatic listings = %d, rules = %+v, stats = %+v", got, rs.LFS, rs.LFSStats)
	}
}

func TestLFSRuleDetectionDoesNotDelayStatus(t *testing.T) {
	f := newFixture(t)
	f.write("changed.txt", "new\n")
	started := make(chan struct{})
	f.w.Git.(*gitcli.Runner).LFSCommand = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	defer f.w.Close()
	f.open()
	awaitLFS(t, started, "rule probe")
	if _, _, untracked, _ := f.w.Repo.Status.Counts(); untracked != 1 {
		t.Fatalf("status did not show changes during rule detection: %+v", f.w.Repo.Status)
	}
}

type lfsTestListing struct {
	started  chan struct{}
	canceled chan struct{}
	finished chan struct{}
	result   chan []string
	reap     <-chan struct{}
	err      error
}

type lfsTestBackend struct {
	*gitcli.Runner
	queue chan *lfsTestListing
	calls atomic.Int32
}

func (b *lfsTestBackend) LFSFiles(ctx context.Context, _ domain.Repo) ([]string, error) {
	b.calls.Add(1)
	select {
	case q := <-b.queue:
		defer close(q.finished)
		close(q.started)
		select {
		case result := <-q.result:
			return result, q.err
		case <-ctx.Done():
			close(q.canceled)
			if q.reap != nil {
				<-q.reap
			}
			if q.err != nil {
				return nil, q.err
			}
			return nil, ctx.Err()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *lfsTestBackend) next(err error, reap <-chan struct{}) *lfsTestListing {
	q := &lfsTestListing{
		started: make(chan struct{}), canceled: make(chan struct{}), finished: make(chan struct{}),
		result: make(chan []string, 1), reap: reap, err: err,
	}
	b.queue <- q
	return q
}

func newLFSStatsFixture(t *testing.T) (*fixture, *lfsTestBackend) {
	t.Helper()
	f := newFixture(t)
	f.write("model.bin", "base\n")
	f.git("add", "model.bin")
	f.git("commit", "-m", "base")
	b := &lfsTestBackend{Runner: f.w.Git.(*gitcli.Runner), queue: make(chan *lfsTestListing, 2)}
	b.LFSCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] != "version" {
			return nil, errors.New("unexpected automatic enumeration")
		}
		return []byte("git-lfs/test"), nil
	}
	f.w.Git = b
	f.w.Caps = CapabilitiesOf(b)
	f.open()
	t.Cleanup(f.w.Close)
	f.settle(f.w.CanRefreshLFSStats)
	return f, b
}

func awaitLFS(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(15 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestLFSStatsIndependentAndKeepsLastSuccess(t *testing.T) {
	f, backend := newLFSStatsFixture(t)
	rs := f.w.Repo
	if rs.LFS.Managed || rs.LFSStats.Status != LFSStatsUnqueried {
		t.Fatalf("no discovered rules should leave stats unqueried: %+v", rs.LFSStats)
	}
	q := backend.next(nil, nil)
	f.w.RefreshLFSStats()
	awaitLFS(t, q.started, "listing")
	for range 4 {
		f.w.RefreshLFSStats()
	}
	f.write("new.txt", "unstaged\n")
	f.w.Refresh(false)
	f.settle(f.idle)
	_, _, untracked, _ := rs.Status.Counts()
	if untracked != 1 || rs.LFSStats.Status != LFSStatsLoading || backend.calls.Load() != 1 {
		t.Fatalf("changes = %d, stats = %+v, calls = %d", untracked, rs.LFSStats, backend.calls.Load())
	}
	q.result <- nil
	f.settle(func() bool { return rs.LFSStats.Status == LFSStatsCurrent })
	if rs.LFSStats.UpdatedAt.IsZero() || len(rs.LFSStats.Files) != 0 {
		t.Fatalf("successful zero was not recorded: %+v", rs.LFSStats)
	}
	q = backend.next(nil, nil)
	q.result <- []string{"model.bin"}
	f.w.RefreshLFSStats()
	f.settle(func() bool { return rs.LFSStats.Status == LFSStatsCurrent })
	success := rs.LFSStats
	q = backend.next(errors.New("failed"), nil)
	q.result <- nil
	f.w.RefreshLFSStats()
	f.settle(func() bool { return rs.LFSStats.Status == LFSStatsFailed })
	if s := rs.LFSStats; len(s.Files) != 1 || !s.UpdatedAt.Equal(success.UpdatedAt) || s.Code != "git.failed" {
		t.Fatalf("failure did not preserve the previous result: %+v", s)
	}
}

func TestLFSStatsRejectsRestagingWithSameStatusLetter(t *testing.T) {
	f, backend := newLFSStatsFixture(t)
	rs := f.w.Repo
	f.write("model.bin", "first staged content\n")
	f.git("add", "model.bin")
	f.w.Refresh(false)
	f.settle(f.idle)
	q := backend.next(nil, nil)
	f.w.RefreshLFSStats()
	awaitLFS(t, q.started, "listing")
	f.write("model.bin", "second staged content\n")
	f.git("add", "model.bin")
	// No status refresh before the scan returns: its own before/after guard
	// must catch the changed blob even though status is still M.
	q.result <- []string{"old.bin"}
	f.settle(func() bool { return f.idle() && rs.LFSStats.Status != LFSStatsLoading })
	if s := rs.LFSStats; s.Status != LFSStatsStale || !s.UpdatedAt.IsZero() || len(s.Files) != 0 {
		t.Fatalf("a stale scan was published: %+v", s)
	}

	q = backend.next(nil, nil)
	q.result <- []string{"model.bin"}
	f.w.RefreshLFSStats()
	f.settle(func() bool { return rs.LFSStats.Status == LFSStatsCurrent })
	f.write("model.bin", "third staged content\n")
	f.git("add", "model.bin")
	f.w.Refresh(false)
	f.settle(f.idle)
	if s := rs.LFSStats; s.Status != LFSStatsStale || len(s.Files) != 1 || backend.calls.Load() != 2 {
		t.Fatalf("refresh should retain the expired count without re-enumerating: %+v", s)
	}
}

func TestLFSStatsCancelsOnSwitchAndReservesWorktreeUntilExit(t *testing.T) {
	f, backend := newLFSStatsFixture(t)
	other := t.TempDir()
	f.git("worktree", "add", "-b", "other", other)
	reap := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(reap)
		}
	}()
	q := backend.next(nil, reap)
	f.w.RefreshLFSStats()
	awaitLFS(t, q.started, "listing")
	old := f.w.Repo
	f.w.Open(other, nil)
	f.settle(func() bool { return f.w.Repo != old && f.w.CanRefreshLFSStats() })
	awaitLFS(t, q.canceled, "cancellation on switch")
	f.open()
	reopened := f.w.Repo
	f.settle(func() bool { return reopened.LFS.Installed })
	f.w.RefreshLFSStats()
	if reopened == old || reopened.LFSStats.Status != LFSStatsUnqueried || backend.calls.Load() != 1 {
		t.Fatalf("a reopened worktree reused or started a scan before exit: %+v", reopened.LFSStats)
	}
	// Let the canceled scan finish; a new query is allowed without applying
	// the old result to the new repository generation.
	close(reap)
	released = true
	awaitLFS(t, q.finished, "canceled listing to finish")
	f.settle(f.w.CanRefreshLFSStats)
	if reopened.LFSStats.Status != LFSStatsUnqueried {
		t.Fatalf("late result overwrote reopened stats: %+v", reopened.LFSStats)
	}
}

func TestLFSStatsCloseReopenAndHEADChange(t *testing.T) {
	f, backend := newLFSStatsFixture(t)
	q := backend.next(nil, nil)
	f.w.RefreshLFSStats()
	awaitLFS(t, q.started, "listing")
	old := f.w.Repo
	f.w.Close()
	awaitLFS(t, q.canceled, "close cancellation")
	awaitLFS(t, q.finished, "listing exit")
	f.open()
	rs := f.w.Repo
	f.settle(f.w.CanRefreshLFSStats)
	if rs == old || rs.Gen == old.Gen || rs.LFSStats.Status != LFSStatsUnqueried {
		t.Fatalf("reopened state = %+v", rs.LFSStats)
	}
	q = backend.next(nil, nil)
	f.w.RefreshLFSStats()
	awaitLFS(t, q.started, "second listing")
	f.git("commit", "--allow-empty", "-m", "external HEAD change")
	f.w.Refresh(true)
	f.settle(f.idle)
	awaitLFS(t, q.canceled, "HEAD invalidation")
	awaitLFS(t, q.finished, "invalidated listing exit")
	f.settle(f.w.CanRefreshLFSStats)
	if s := rs.LFSStats; s.Status != LFSStatsStale || !s.UpdatedAt.IsZero() || backend.calls.Load() != 2 {
		t.Fatalf("invalidated result = %+v, calls = %d", s, backend.calls.Load())
	}
}

func TestLFSUnsupportedBackendSkipsQueries(t *testing.T) {
	f := newFixture(t)
	// Wrapping only the required contract deliberately omits Git-only LFS
	// capabilities, like the Subversion adapter.
	f.w.Git = struct{ Git }{f.w.Git}
	f.open()
	f.w.RefreshLFSStats()
	if f.w.SupportsLFSStats() || f.w.CanRefreshLFSStats() || f.w.Repo.LFSStats.Status != LFSStatsUnqueried {
		t.Fatalf("unexpected LFS support/state: %+v", f.w.Repo.LFSStats)
	}
}

func TestLFSStatsUnconfirmedTransportExitKeepsReservation(t *testing.T) {
	for _, cause := range []string{"transport_failure", "epoch", "close_reopen"} {
		t.Run(cause, func(t *testing.T) {
			f, backend := newLFSStatsFixture(t)
			q := backend.next(gitcli.ErrLFSReapUnconfirmed, nil)
			f.w.RefreshLFSStats()
			awaitLFS(t, q.started, "listing")
			switch cause {
			case "transport_failure":
				q.result <- nil
			case "epoch":
				f.w.invalidateLFSStats(f.w.Repo)
			case "close_reopen":
				f.w.Close()
				awaitLFS(t, q.finished, "unconfirmed transport exit")
				f.w.Drain()
				f.open()
			}
			f.settle(func() bool { return f.w.Repo.LFSStats.Status == LFSStatsFailed })
			if f.w.CanRefreshLFSStats() || f.w.Repo.LFSStats.Code != "lfs.cleanup_unconfirmed" {
				t.Fatalf("uncertain exit released the scan: %+v", f.w.Repo.LFSStats)
			}
			f.w.RefreshLFSStats()
			if backend.calls.Load() != 1 {
				t.Fatalf("an unconfirmed scan was duplicated: %d", backend.calls.Load())
			}
			f.write("model.bin", "new staged content\n")
			f.git("add", "model.bin")
			f.w.Refresh(true)
			f.settle(f.idle)
			if s := f.w.Repo.LFSStats; s.Status != LFSStatsFailed || s.Code != "lfs.cleanup_unconfirmed" {
				t.Fatalf("refresh hid the blocked query's reason: %+v", s)
			}
		})
	}
}
