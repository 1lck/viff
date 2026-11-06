package gitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// linear makes commits A..E on main, each adding one file.
func linear(t *testing.T) (*testRepo, map[string]string) {
	tr := newTestRepo(t)
	hashes := map[string]string{}
	for _, n := range []string{"A", "B", "C", "D", "E"} {
		tr.write(strings.ToLower(n)+".txt", n+"\n")
		tr.commitAll(n)
		hashes[n] = strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	}
	return tr, hashes
}

func subjects(tr *testRepo) string {
	return strings.TrimSpace(strings.ReplaceAll(tr.git("log", "--format=%s", "--reverse"), "\n", " "))
}

func TestRebaseCommitsRange(t *testing.T) {
	tr, h := linear(t)
	list, base, err := tr.r.RebaseCommits(ctx, tr.repo, h["C"])
	if err != nil || base != h["B"] || len(list) != 3 || list[0].Subject != "C" || list[2].Subject != "E" {
		t.Fatalf("from C: base=%s list=%+v err=%v", base, list, err)
	}
	root, base, err := tr.r.RebaseCommits(ctx, tr.repo, h["A"])
	if err != nil || base != "" || len(root) != 5 {
		t.Fatalf("from root: base=%q n=%d err=%v", base, len(root), err)
	}
	tr.git("checkout", "-q", "-b", "side", h["B"])
	tr.write("side.txt", "side\n")
	tr.commitAll("Side")
	side := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	tr.git("checkout", "-q", "main")
	if _, _, err := tr.r.RebaseCommits(ctx, tr.repo, side); err == nil {
		t.Fatal("a commit off the current branch must be refused")
	}
	tr.git("merge", "-q", "--no-ff", "-m", "Merge side", "side")
	list, _, err = tr.r.RebaseCommits(ctx, tr.repo, h["C"])
	merges := 0
	for _, c := range list {
		if c.Merge {
			merges++
		}
	}
	if err != nil || merges != 1 {
		t.Fatalf("merge commit not flagged: %+v", list)
	}
}

func TestInteractiveRebasePlan(t *testing.T) {
	tr, h := linear(t)
	steps := []RebaseStep{
		{Action: RebasePick, Hash: h["B"]},
		{Action: RebaseReword, Hash: h["D"], Message: "D, reworded\n\nwith a body"},
		{Action: RebaseFixup, Hash: h["C"]},
		{Action: RebaseDrop, Hash: h["E"]},
	}
	if _, err := tr.r.InteractiveRebase(ctx, tr.repo, h["A"], steps); err != nil {
		t.Fatal(err)
	}
	if got := subjects(tr); got != "A B D, reworded" {
		t.Fatalf("history = %q", got)
	}
	if body := tr.git("log", "-1", "--format=%B"); body != "D, reworded\n\nwith a body\n\n" {
		t.Fatalf("reworded message = %q", body)
	}
	// The fixup folded C's file into the reworded commit; E's file is gone.
	files := tr.git("show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "c.txt") || !strings.Contains(files, "d.txt") {
		t.Fatalf("fixup lost changes: %q", files)
	}
	if _, err := os.Stat(filepath.Join(tr.repo.Root, "e.txt")); !os.IsNotExist(err) {
		t.Fatal("dropped commit's file still exists")
	}
	if author := strings.TrimSpace(tr.git("log", "-1", "--format=%an")); author == "" {
		t.Fatal("author lost")
	}
	if op := tr.r.Operation(ctx, tr.repo); op.Active() {
		t.Fatalf("rebase left running: %+v", op)
	}
	if _, err := os.Stat(filepath.Join(tr.repo.MetaDir, "viff-rebase")); !os.IsNotExist(err) {
		t.Fatal("plan files not cleaned up")
	}
}

func TestInteractiveRebaseSquashFromRootWithLocalChanges(t *testing.T) {
	tr, h := linear(t)
	tr.write("a.txt", "local edit\n") // kept by --autostash
	steps := []RebaseStep{
		{Action: RebasePick, Hash: h["A"]},
		{Action: RebaseSquash, Hash: h["B"]},
		{Action: RebasePick, Hash: h["C"]},
		{Action: RebasePick, Hash: h["D"]},
		{Action: RebasePick, Hash: h["E"]},
	}
	if _, err := tr.r.InteractiveRebase(ctx, tr.repo, "", steps); err != nil {
		t.Fatal(err)
	}
	if n := strings.TrimSpace(tr.git("rev-list", "--count", "HEAD")); n != "4" {
		t.Fatalf("commits after squash = %s", n)
	}
	if msg := tr.git("log", "--reverse", "--format=%B", "-n", "1", "HEAD~3"); !strings.Contains(msg, "A") || !strings.Contains(msg, "B") {
		t.Fatalf("squashed message = %q", msg)
	}
	if data, _ := os.ReadFile(filepath.Join(tr.repo.Root, "a.txt")); string(data) != "local edit\n" {
		t.Fatalf("local change lost: %q", data)
	}
}

func TestInteractiveRebaseReorderConflictThenContinue(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("f.txt", "1\n")
	tr.commitAll("one")
	base := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	tr.write("f.txt", "2\n")
	tr.commitAll("two")
	two := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	tr.write("f.txt", "3\n")
	tr.commitAll("three")
	three := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	// Swapping two dependent edits must stop on a conflict, with a reword pending.
	steps := []RebaseStep{
		{Action: RebaseReword, Hash: three, Message: "three first"},
		{Action: RebasePick, Hash: two},
	}
	_, err := tr.r.InteractiveRebase(ctx, tr.repo, base, steps)
	if KindOf(err) != ErrConflict {
		t.Fatalf("expected a conflict, got %v (%v)", err, KindOf(err))
	}
	if op := tr.r.Operation(ctx, tr.repo); op.Kind != domain.OpRebase || !op.Interactive {
		t.Fatalf("operation = %+v", op)
	}
	tr.write("f.txt", "3\n")
	tr.git("add", "f.txt")
	// Continue runs the pending amend from the plan files that were kept.
	out, err := tr.r.OperationCommand(ctx, tr.repo, domain.OpRebase, "continue")
	if KindOf(err) == ErrConflict {
		tr.write("f.txt", "2\n")
		tr.git("add", "f.txt")
		out, err = tr.r.OperationCommand(ctx, tr.repo, domain.OpRebase, "continue")
	}
	if err != nil {
		t.Fatalf("continue: %v\n%s", err, out.Stdout)
	}
	if got := subjects(tr); got != "one three first two" {
		t.Fatalf("history = %q", got)
	}
}

func TestValidateRebasePlan(t *testing.T) {
	bad := [][]RebaseStep{
		{{Action: RebaseSquash, Hash: "a"}},
		{{Action: RebaseDrop, Hash: "a"}, {Action: RebaseFixup, Hash: "b"}},
		{{Action: RebaseReword, Hash: "a", Message: "  \n"}},
		{{Action: "edit", Hash: "a"}},
	}
	for i, plan := range bad {
		if ValidateRebasePlan(plan) == nil {
			t.Fatalf("plan %d accepted", i)
		}
	}
	if err := ValidateRebasePlan([]RebaseStep{{Action: RebaseDrop, Hash: "a"}, {Action: RebasePick, Hash: "b"}, {Action: RebaseSquash, Hash: "c"}}); err != nil {
		t.Fatal(err)
	}
	if got := shellQuote("/tmp/it's here"); got != `'/tmp/it'\''s here'` {
		t.Fatalf("shellQuote = %s", got)
	}
}
