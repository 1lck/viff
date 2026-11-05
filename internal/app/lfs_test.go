package app

import (
	"context"
	"errors"
	"testing"

	"viff/internal/gitcli"
)

// TestLFSStateFollowsAttributes opens a repository whose attributes select
// filter=lfs while git-lfs is simulated missing, then edits the rules and
// checks that a plain refresh notices the change.
func TestLFSStateFollowsAttributes(t *testing.T) {
	f := newFixture(t)
	f.write(".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	f.w.Git.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git: 'lfs' is not a git command")
	}
	f.open()

	rs := f.w.Repo
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
	f.w.Git.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git: 'lfs' is not a git command")
	}
	f.open()

	first := f.w.Repo.lfsAt
	if first.IsZero() {
		t.Fatal("opening the repository did not run LFS detection")
	}
	seq := f.w.Repo.statusSeq
	f.w.Refresh(false)
	f.settle(func() bool {
		rs := f.w.Repo
		return rs.statusSeq > seq && !rs.refreshing && f.idle()
	})
	if got := f.w.Repo.lfsAt; !got.Equal(first) {
		t.Fatalf("plain refresh re-ran LFS detection: %v -> %v", first, got)
	}

	f.w.Refresh(true)
	f.settle(func() bool {
		rs := f.w.Repo
		return rs.statusSeq > seq+1 && !rs.refreshing && f.idle()
	})
	if got := f.w.Repo.lfsAt; !got.After(first) {
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
	fields := lfsEventFields(st)
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
	fields = lfsEventFields(st)
	if got := fields["error"]; got != "git.failed" {
		t.Fatalf("error = %v, want the classified code", got)
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
