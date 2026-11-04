package app

import (
	"strings"
	"testing"

	"viff/internal/domain"
)

// TestCompareCommits compares arbitrary commit pairs, including commits that
// are not parent and child.
func TestCompareCommits(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.write("keep.txt", "same\n")
	f.commitAll("A")
	first := strings.TrimSpace(f.git("rev-parse", "HEAD"))

	f.write("a.txt", "one\ntwo\n")
	f.write("b.txt", "new\n")
	f.commitAll("B")
	second := strings.TrimSpace(f.git("rev-parse", "HEAD"))

	// A side branch from the first commit, so the pair has no ancestor link.
	f.git("checkout", "-q", "-b", "side", first)
	f.write("c.txt", "side\n")
	f.commitAll("C")
	third := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.open()

	load := func(from, to string) []domain.Change {
		t.Helper()
		var out []domain.Change
		var err error
		done := false
		f.w.CompareCommits(from, to, func(list []domain.Change, e error) {
			out, err, done = list, e, true
		})
		f.settle(func() bool { return done })
		if err != nil {
			t.Fatalf("compare %s → %s: %v", from, to, err)
		}
		return out
	}
	kinds := func(list []domain.Change) map[string]domain.ChangeKind {
		t.Helper()
		m := make(map[string]domain.ChangeKind, len(list))
		for _, ch := range list {
			m[ch.Path] = ch.Kind
		}
		return m
	}

	got := kinds(load(first, second))
	if len(got) != 2 || got["a.txt"] != domain.ChangeModified || got["b.txt"] != domain.ChangeAdded {
		t.Fatalf("A → B = %+v", got)
	}

	// Unrelated tips: B has a.txt and b.txt, C (from A) has a.txt and c.txt.
	got = kinds(load(second, third))
	if len(got) != 3 || got["a.txt"] != domain.ChangeModified || got["b.txt"] != domain.ChangeDeleted || got["c.txt"] != domain.ChangeAdded {
		t.Fatalf("B → C = %+v", got)
	}

	// The reverse direction swaps additions and deletions.
	got = kinds(load(third, second))
	if len(got) != 3 || got["a.txt"] != domain.ChangeModified || got["b.txt"] != domain.ChangeAdded || got["c.txt"] != domain.ChangeDeleted {
		t.Fatalf("C → B = %+v", got)
	}

	if got := load(first, first); len(got) != 0 {
		t.Fatalf("A → A = %+v", got)
	}

	var failed bool
	f.w.CompareCommits("no-such-rev", second, func(_ []domain.Change, err error) { failed = err != nil })
	f.settle(func() bool { return failed })
	if !failed {
		t.Fatal("comparing an unknown revision should report an error")
	}
}

// TestCompareCommitsDiffSource checks that a pair of commits is a valid diff
// source: unique key, A on the old side and B on the new one, either direction.
func TestCompareCommitsDiffSource(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("A")
	first := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.write("a.txt", "one\ntwo\n")
	f.commitAll("B")
	second := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.open()

	src := DiffSource{Path: "a.txt", Commit: second, Parent: first}
	res := f.loadDiff(src)
	if len(res.Old.Lines) != 1 || res.Old.Lines[0] != "one" || len(res.New.Lines) != 2 || res.New.Lines[1] != "two" {
		t.Fatalf("A → B sides = %+v / %+v", res.Old.Lines, res.New.Lines)
	}
	if res.Old.Title != domain.ShortHash(first) || res.New.Title != domain.ShortHash(second) || len(res.Blocks) != 1 {
		t.Fatalf("A → B titles/blocks = %q / %q / %+v", res.Old.Title, res.New.Title, res.Blocks)
	}

	reverse := DiffSource{Path: "a.txt", Commit: first, Parent: second}
	if reverse.Key() == src.Key() {
		t.Fatal("A → B and B → A share a diff key")
	}
	for _, other := range []DiffSource{
		{Path: "a.txt", Working: true},
		{Path: "a.txt", Commit: second, Parent: first, View: true},
		{Path: "a.txt", Commit: second},
	} {
		if other.Key() == src.Key() {
			t.Fatalf("%+v shares the compare key", other)
		}
	}
	res = f.loadDiff(reverse)
	if len(res.Old.Lines) != 2 || len(res.New.Lines) != 1 || res.New.Lines[0] != "one" {
		t.Fatalf("B → A sides = %+v / %+v", res.Old.Lines, res.New.Lines)
	}
}
