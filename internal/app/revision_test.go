package app

import (
	"strings"
	"testing"

	"viff/internal/domain"
)

// TestRevisionTree lists a commit's tree through the workspace: the tree of
// an older commit stays what it was, the default revision is HEAD, and an
// unknown revision is an error.
func TestRevisionTree(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.write("dir/b.txt", "two\n")
	f.commitAll("A")
	first := f.head()

	f.write("c.txt", "three\n")
	f.commitAll("B")
	tip := f.head()
	f.open()

	load := func(rev string) ([]domain.TreeEntry, error) {
		t.Helper()
		var list []domain.TreeEntry
		var err error
		done := false
		f.w.RevisionTree(rev, func(l []domain.TreeEntry, e error) {
			list, err, done = l, e, true
		})
		f.settle(func() bool { return done })
		return list, err
	}
	paths := func(list []domain.TreeEntry) string {
		t.Helper()
		out := make([]string, len(list))
		for i, e := range list {
			out[i] = e.Path
		}
		return strings.Join(out, "|")
	}

	got, err := load(first)
	if err != nil {
		t.Fatal(err)
	}
	if p := paths(got); p != "a.txt|dir/b.txt" {
		t.Fatalf("A tree = %s", p)
	}
	if e := got[1]; e.Size != int64(len("two\n")) || e.Mode != "100644" {
		t.Fatalf("dir/b.txt = %+v", e)
	}

	// The tip has the later file, and "" means HEAD.
	got, err = load(tip)
	if err != nil || paths(got) != "a.txt|c.txt|dir/b.txt" {
		t.Fatalf("B tree = %s, %v", paths(got), err)
	}
	got, err = load("")
	if err != nil || paths(got) != "a.txt|c.txt|dir/b.txt" {
		t.Fatalf("default tree = %s, %v", paths(got), err)
	}

	var failed bool
	f.w.RevisionTree("no-such-revision", func(_ []domain.TreeEntry, err error) {
		failed = err != nil
	})
	f.settle(func() bool { return failed })
	if !failed {
		t.Fatal("an unknown revision should report an error")
	}
}
