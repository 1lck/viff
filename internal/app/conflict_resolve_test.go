package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// appConflictFixture leaves a real merge conflict on conflict.txt. path may be
// changed; the file is always "conflict.txt".
func appConflictFixture(t *testing.T, base, ours, theirs string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.write("conflict.txt", base)
	f.commitAll("base")
	f.git("checkout", "-q", "-b", "other")
	f.write("conflict.txt", theirs)
	f.commitAll("theirs")
	f.git("checkout", "-q", "main")
	f.write("conflict.txt", ours)
	f.commitAll("ours")
	f.open()
	f.w.Merge("other", false)
	f.settle(func() bool { return f.idle() && f.w.Repo.Operation.Kind == domain.OpMerge })
	if len(f.w.Repo.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", f.w.Repo.Conflicts)
	}
	return f
}

func (f *fixture) conflictDiff() *DiffResult {
	f.t.Helper()
	res := f.loadDiff(DiffSource{Path: "conflict.txt", Working: true, Area: domain.AreaConflicts, Kind: domain.ChangeConflicted})
	if res.Conflict == nil {
		f.t.Fatal("no conflict view")
	}
	return res
}

func (f *fixture) workingFile() string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "conflict.txt"))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

// resolveConflictInApp runs one resolution and waits for it.
func resolveConflictInApp(t *testing.T, f *fixture, d *DiffResult, block int, choice domain.ResolveChoice) error {
	t.Helper()
	var got error
	done := false
	f.w.ResolveConflict(d, block, choice, func(err error) { got, done = err, true })
	f.settle(func() bool { return done })
	f.sync()
	return got
}

func TestResolveConflictInAppThenMarkResolved(t *testing.T) {
	f := appConflictFixture(t, "base\n", "ours\n", "theirs\n")
	res := f.conflictDiff()
	if len(res.Conflict.Markers.Blocks) != 1 || res.Conflict.Resolved {
		t.Fatalf("view = %+v resolved=%v", res.Conflict.Markers, res.Conflict.Resolved)
	}
	if got := resolveConflictInApp(t, f, res, 0, domain.ResolveOurs); got != nil {
		t.Fatalf("resolve: %v", got)
	}
	if got := f.workingFile(); got != "ours\n" {
		t.Fatalf("file = %q, want ours", got)
	}
	// The file is resolved on disk but still unmerged: nothing was staged.
	if len(f.w.Repo.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", f.w.Repo.Conflicts)
	}
	if entries := f.w.Repo.Entries(domain.AreaConflicts); len(entries) != 1 || entries[0].Path != "conflict.txt" {
		t.Fatalf("conflict entries = %+v", entries)
	}
	if out := f.git("ls-files", "-u", "--", "conflict.txt"); strings.Count(out, "\n") != 3 {
		t.Fatalf("index stages changed: %q", out)
	}
	// Reloading the view shows the resolved file, as the UI does after a write.
	f.w.Refresh(true)
	f.settle(f.idle)
	reloaded := f.conflictDiff()
	if !reloaded.Conflict.Resolved || len(reloaded.Conflict.Markers.Blocks) != 0 {
		t.Fatalf("reloaded view = %+v", reloaded.Conflict.Markers)
	}
	// The existing Mark-as-Resolved step stages it, and the merge continues.
	f.w.StagePaths([]string{"conflict.txt"})
	f.settle(func() bool { return f.idle() && len(f.w.Repo.Conflicts) == 0 })
	if _, _, _, conflicts := f.w.Repo.Status.Counts(); conflicts != 0 {
		t.Fatalf("conflicts left after staging: %d", conflicts)
	}
	f.w.OperationAction("continue")
	f.settle(func() bool { return f.idle() && !f.w.Repo.Operation.Active() })
	if f.w.Repo.Operation.Active() {
		t.Fatalf("merge still active: %+v", f.w.Repo.Operation)
	}
	if parents := strings.Fields(f.git("log", "-1", "--format=%P")); len(parents) != 2 {
		t.Fatalf("merge commit parents = %v", parents)
	}
}

func TestResolveConflictInAppChoices(t *testing.T) {
	for _, c := range []struct {
		choice domain.ResolveChoice
		want   string
	}{
		{domain.ResolveTheirs, "theirs\n"},
		{domain.ResolveBoth, "ours\ntheirs\n"},
	} {
		t.Run(c.choice.String(), func(t *testing.T) {
			f := appConflictFixture(t, "base\n", "ours\n", "theirs\n")
			if got := resolveConflictInApp(t, f, f.conflictDiff(), 0, c.choice); got != nil {
				t.Fatalf("resolve: %v", got)
			}
			if got := f.workingFile(); got != c.want {
				t.Fatalf("file = %q, want %q", got, c.want)
			}
			if len(f.w.Repo.Conflicts) != 1 {
				t.Fatal("resolution must not stage the file")
			}
			if n := len(f.errorsSince(0)); n != 0 {
				t.Fatalf("unexpected errors: %+v", f.errorsSince(0))
			}
		})
	}
}

func TestResolveConflictInAppRefusesStaleView(t *testing.T) {
	f := appConflictFixture(t, "base\n", "ours\n", "theirs\n")
	res := f.conflictDiff()
	edited := "<<<<<<< HEAD\nexternal edit\n=======\ntheirs\n>>>>>>> other\n"
	f.write("conflict.txt", edited)
	notifications := len(f.w.Notifications)
	got := resolveConflictInApp(t, f, res, 0, domain.ResolveOurs)
	if !errors.Is(got, ErrStale) {
		t.Fatalf("err = %v, want ErrStale", got)
	}
	// The newer edit on disk is kept, and the conflict stays in the index.
	if now := f.workingFile(); now != edited {
		t.Fatalf("external edit overwritten: %q", now)
	}
	if len(f.w.Repo.Conflicts) != 1 {
		t.Fatal("stale request changed the conflict state")
	}
	last := f.w.Notifications[len(f.w.Notifications)-1]
	if len(f.w.Notifications) == notifications || last.Kind != NoticeWarning || last.Title != "File changed" {
		t.Fatalf("notifications = %+v", f.w.Notifications[notifications:])
	}
}

func TestResolveConflictInAppMultipleBlocks(t *testing.T) {
	f := appConflictFixture(t, "base\n", "ours\n", "theirs\n")
	text := "top\n" +
		"<<<<<<< ours\n" +
		"a-ours\n" +
		"=======\n" +
		"a-theirs\n" +
		">>>>>>> theirs\n" +
		"middle\n" +
		"<<<<<<< ours\n" +
		"b-ours\n" +
		"||||||| base\n" +
		"b-base\n" +
		"=======\n" +
		"b-theirs\n" +
		">>>>>>> theirs\n" +
		"tail\n"
	f.write("conflict.txt", text)
	f.w.Refresh(true)
	f.settle(f.idle)
	res := f.conflictDiff()
	if len(res.Conflict.Markers.Blocks) != 2 {
		t.Fatalf("blocks = %+v", res.Conflict.Markers.Blocks)
	}
	// Accepting the second block must leave the first one alone.
	if got := resolveConflictInApp(t, f, res, 1, domain.ResolveBoth); got != nil {
		t.Fatalf("resolve block 1: %v", got)
	}
	want := "top\n" +
		"<<<<<<< ours\n" +
		"a-ours\n" +
		"=======\n" +
		"a-theirs\n" +
		">>>>>>> theirs\n" +
		"middle\n" +
		"b-ours\n" +
		"b-theirs\n" +
		"tail\n"
	if got := f.workingFile(); got != want {
		t.Fatalf("after block 1 =\n%q\nwant\n%q", got, want)
	}
	// The remaining block is still there and can be resolved next.
	res = f.conflictDiff()
	if len(res.Conflict.Markers.Blocks) != 1 || res.Conflict.Resolved {
		t.Fatalf("view after block 1 = %+v", res.Conflict.Markers)
	}
	if got := resolveConflictInApp(t, f, res, 0, domain.ResolveOurs); got != nil {
		t.Fatalf("resolve block 0: %v", got)
	}
	want = "top\na-ours\nmiddle\nb-ours\nb-theirs\ntail\n"
	if got := f.workingFile(); got != want {
		t.Fatalf("final =\n%q\nwant\n%q", got, want)
	}
	f.w.Refresh(true)
	f.settle(f.idle)
	res = f.conflictDiff()
	if !res.Conflict.Resolved {
		t.Fatalf("final view = %+v", res.Conflict.Markers)
	}
}

func TestResolveConflictInAppRefusesUnsupported(t *testing.T) {
	// A binary conflict has no text blocks to accept.
	f := newFixture(t)
	f.write("b.bin", "\x00base")
	f.commitAll("base")
	f.git("checkout", "-q", "-b", "other")
	f.write("b.bin", "\x00theirs")
	f.commitAll("theirs")
	f.git("checkout", "-q", "main")
	f.write("b.bin", "\x00ours")
	f.commitAll("ours")
	f.open()
	f.w.Merge("other", false)
	f.settle(func() bool { return f.idle() && f.w.Repo.Operation.Kind == domain.OpMerge })
	if len(f.w.Repo.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", f.w.Repo.Conflicts)
	}
	res := f.loadDiff(DiffSource{Path: "b.bin", Working: true, Area: domain.AreaConflicts, Kind: domain.ChangeConflicted})
	if res.Conflict == nil {
		t.Fatal("no conflict view")
	}
	var got error
	done := false
	f.w.ResolveConflict(res, 0, domain.ResolveOurs, func(err error) { got, done = err, true })
	f.settle(func() bool { return done })
	if !errors.Is(got, gitcli.ErrResolveBinary) {
		t.Fatalf("binary err = %v", got)
	}
	if data, err := os.ReadFile(filepath.Join(f.dir, "b.bin")); err != nil || string(data) != "\x00ours" {
		t.Fatalf("binary file changed: %q %v", data, err)
	}

	// Input the view cannot support is refused before a task starts.
	var finish func(error)
	done = false
	finish = func(err error) { got, done = err, true }
	f.w.ResolveConflict(nil, 0, domain.ResolveOurs, finish)
	f.settle(func() bool { return done })
	if got == nil {
		t.Fatal("nil diff was accepted")
	}
	text := appConflictFixture(t, "base\n", "ours\n", "theirs\n")
	view := text.conflictDiff()
	if got := resolveConflictInApp(t, text, view, 9, domain.ResolveOurs); !errors.Is(got, domain.ErrConflictBlock) {
		t.Fatalf("bad block err = %v", got)
	}
	if len(text.w.Repo.Conflicts) != 1 || !strings.Contains(text.workingFile(), "<<<<<<<") {
		t.Fatal("bad block request changed the file")
	}
}
