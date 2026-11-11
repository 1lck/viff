package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/observability"
)

// refreshBackend keeps the real Git adapter (including optional capabilities),
// but holds selected status results until the test releases them. No timing or
// installed git-lfs dependency is needed to overlap requests deterministically.
type refreshBackend struct {
	*gitcli.Runner
	blocks chan *refreshBlock
	calls  atomic.Int32
	active atomic.Int32
	max    atomic.Int32
	lists  atomic.Int32
}

type refreshBlock struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	err     error
}

func (b *refreshBlock) unblock() { b.once.Do(func() { close(b.release) }) }

func (b *refreshBackend) block(t *testing.T, err error) *refreshBlock {
	t.Helper()
	q := &refreshBlock{started: make(chan struct{}), release: make(chan struct{}), err: err}
	b.blocks <- q
	t.Cleanup(q.unblock)
	return q
}

func (b *refreshBackend) Status(ctx context.Context, repo domain.Repo) (domain.Status, error) {
	b.calls.Add(1)
	n := b.active.Add(1)
	defer b.active.Add(-1)
	for old := b.max.Load(); n > old; old = b.max.Load() {
		if b.max.CompareAndSwap(old, n) {
			break
		}
	}
	st, err := b.Runner.Status(ctx, repo)
	select {
	case q := <-b.blocks:
		close(q.started)
		select {
		case <-q.release:
			if q.err != nil {
				return domain.Status{}, q.err
			}
		case <-ctx.Done():
			return domain.Status{}, ctx.Err()
		}
	default:
	}
	return st, err
}

func newRefreshFixture(t *testing.T) (*fixture, *refreshBackend, *fakeObs) {
	t.Helper()
	f := newFixture(t)
	f.write("base.txt", "base\n")
	f.git("add", "base.txt")
	f.git("commit", "-q", "-m", "base")
	b := &refreshBackend{Runner: f.w.Git.(*gitcli.Runner), blocks: make(chan *refreshBlock, 4)}
	b.LFSCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "version" {
			return []byte("git-lfs/test"), nil
		}
		return []byte(fmt.Sprintf("asset-%d.bin\x00", b.lists.Add(1))), nil
	}
	f.w.Git = b
	f.open()
	obs := &fakeObs{}
	f.w.SetRecorder(obs, obs)
	return f, b, obs
}

func waitRefreshStarted(t *testing.T, q *refreshBlock) {
	t.Helper()
	select {
	case <-q.started:
	case <-time.After(15 * time.Second):
		t.Fatal("status query did not start")
	}
}

func refreshScopes(obs *fakeObs) []bool {
	var scopes []bool
	for _, e := range obs.eventsNamed(observability.EventRepoRefresh) {
		scopes = append(scopes, e.Fields["meta"].(bool))
	}
	return scopes
}

// Fork issue #1: only requests received while a refresh is running determine
// its follow-up scope. Ordinary polling must reuse the newly published LFS
// snapshot, and metadata demand must survive later ordinary requests.
func TestRefreshCoalescesScope(t *testing.T) {
	for _, tt := range []struct {
		name    string
		initial bool
		pending []bool
		meta    bool
	}{
		{"full_then_plain", true, []bool{false, false, false}, false},
		{"plain_then_plain", false, []bool{false, false}, false},
		{"plain_meta_plain", false, []bool{false, true, false}, true},
		{"meta_plain", true, []bool{true, false}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, b, obs := newRefreshFixture(t)
			q := b.block(t, nil)
			before, lists := b.calls.Load(), b.lists.Load()
			f.w.Refresh(tt.initial)
			waitRefreshStarted(t, q)
			for _, meta := range tt.pending {
				f.w.Refresh(meta)
			}
			q.unblock()
			f.settle(f.idle)
			if got := refreshScopes(obs); !reflect.DeepEqual(got, []bool{tt.initial, tt.meta}) {
				t.Errorf("refresh scopes = %v, want [%v %v]", got, tt.initial, tt.meta)
			}
			wantLists := lists
			if tt.initial {
				wantLists++
			}
			if tt.meta {
				wantLists++
			}
			if got := b.lists.Load(); got != wantLists {
				t.Errorf("LFS listings = %d, want %d", got, wantLists)
			}
			if got := f.w.Repo.LFS.Files; !reflect.DeepEqual(got, []string{fmt.Sprintf("asset-%d.bin", wantLists)}) {
				t.Errorf("LFS snapshot = %v, want latest listing %d", got, wantLists)
			}
			if got := b.calls.Load() - before; got != 2 || b.max.Load() != 1 {
				t.Errorf("status queries = %d, max concurrent = %d, want 2 and 1", got, b.max.Load())
			}
		})
	}
}

func TestRefreshConsumesPendingBeforeFollowUp(t *testing.T) {
	f, b, obs := newRefreshFixture(t)
	first, second := b.block(t, nil), b.block(t, nil)
	before := b.calls.Load()
	f.w.Refresh(false)
	waitRefreshStarted(t, first)
	// Change metadata after the first pass started; its follow-up must query
	// the new ref even when the first pass captured an older ref snapshot.
	f.git("branch", "during-refresh")
	f.w.Refresh(true)
	first.unblock()
	f.settle(func() bool { return b.calls.Load() == before+2 })
	waitRefreshStarted(t, second)
	for i := 0; i < 100; i++ {
		f.w.Refresh(false)
	}
	second.unblock()
	f.settle(f.idle)
	if got := refreshScopes(obs); !reflect.DeepEqual(got, []bool{false, true, false}) {
		t.Fatalf("refresh scopes = %v, want [false true false]", got)
	}
	found := false
	for _, ref := range f.w.Repo.RefsOf(domain.RefLocal) {
		found = found || ref.Short == "during-refresh"
	}
	if !found || b.calls.Load()-before != 3 || b.max.Load() != 1 {
		t.Fatalf("latest ref = %v, queries = %d, concurrent = %d", found, b.calls.Load()-before, b.max.Load())
	}
	// A later independent refresh must not inherit the consumed metadata bit.
	f.w.Refresh(false)
	f.settle(f.idle)
	if got := refreshScopes(obs); !reflect.DeepEqual(got, []bool{false, true, false, false}) {
		t.Fatalf("later refresh scopes = %v", got)
	}
}

func TestRefreshPendingAfterFailure(t *testing.T) {
	for _, failure := range []error{errors.New("status failed"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			f, b, obs := newRefreshFixture(t)
			q := b.block(t, failure)
			before, lists := b.calls.Load(), b.lists.Load()
			f.w.Refresh(false)
			waitRefreshStarted(t, q)
			f.w.Refresh(false)
			q.unblock()
			f.settle(f.idle)
			if got := refreshScopes(obs); !reflect.DeepEqual(got, []bool{false}) {
				t.Errorf("successful follow-up scopes = %v, want [false]", got)
			}
			if b.calls.Load()-before != 2 || b.lists.Load() != lists || !f.w.Repo.StatusLoaded {
				t.Fatalf("queries = %d, LFS listings = %d -> %d, status loaded = %v", b.calls.Load()-before, lists, b.lists.Load(), f.w.Repo.StatusLoaded)
			}
		})
	}
}

// Stale completion callbacks must neither publish their snapshot nor replay
// their pending request against a replacement repository or generation.
func TestRefreshPendingDoesNotCrossRepositoryLifetime(t *testing.T) {
	for _, action := range []string{"switch", "reopen", "close", "gone"} {
		t.Run(action, func(t *testing.T) {
			f, b, obs := newRefreshFixture(t)
			old := f.w.Repo
			var failure error
			if action == "gone" {
				failure = &gitcli.Error{Kind: gitcli.ErrNotRepository}
			}
			q := b.block(t, failure)
			f.w.Refresh(false)
			waitRefreshStarted(t, q)
			f.w.Refresh(true)
			if action != "gone" {
				f.w.Close()
			}
			switch action {
			case "switch":
				other := newFixture(t)
				other.write("other.txt", "other\n")
				f.w.Open(other.dir, func(err error) {
					if err != nil {
						t.Error(err)
					}
				})
				f.settle(func() bool { return f.w.Repo != nil && f.idle() })
			case "reopen":
				f.open()
			}
			current := f.w.Repo
			before := b.calls.Load()
			// All new-generation work has settled. The next posted result is
			// the deliberately late old refresh; wait without draining it.
			q.unblock()
			deadline := time.NewTimer(15 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for !f.w.Pending() {
				select {
				case <-tick.C:
				case <-deadline.C:
					t.Fatal("old refresh did not post its result")
				}
			}
			f.w.Drain()
			if action == "gone" {
				if f.w.Repo != nil {
					t.Fatal("missing repository remained open")
				}
			} else if f.w.Repo != current {
				t.Fatal("old result replaced the current repository")
			}
			if b.calls.Load() != before {
				t.Fatal("old pending request started a refresh in the new lifetime")
			}
			if current != nil && action != "gone" && current.Gen <= old.Gen {
				t.Fatal("reopened repository reused the old generation")
			}
			for _, e := range obs.eventsNamed(observability.EventRepoRefresh) {
				if e.Fields["gen"] == old.Gen {
					t.Fatal("stale refresh published an event")
				}
			}
		})
	}
}
