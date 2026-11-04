package app

import (
	"strings"
	"testing"

	"viff/internal/domain"
)

// TestFileViewShowsOneRevision covers the read-only file tab behind
// "Open Repository Version": a working-tree and a revision view share the
// diff tab but read one side only.
func TestFileViewShowsOneRevision(t *testing.T) {
	f := newFixture(t)
	f.write("dir/a.txt", "one\r\ntwo\r\n")
	f.commitAll("First")
	head := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.open()

	src := DiffSource{Path: "dir/a.txt", Commit: head, View: true}
	if got := src.Key(); got != "v"+head+":dir/a.txt" {
		t.Fatalf("key = %q", got)
	}
	view := f.loadDiff(src)
	if view.Notice != "" || !view.New.Text || len(view.New.Lines) != 2 || view.New.Lines[1] != "two" {
		t.Fatalf("revision view = %+v", view)
	}
	if view.New.Title != domain.ShortHash(head) || view.New.LineEnding != "CRLF" || !view.Old.Text || len(view.Old.Lines) != 0 {
		t.Fatalf("sides = %+v / %+v", view.Old, view.New)
	}

	// The working tree is a separate source: it follows edits and has no hash.
	work := DiffSource{Path: "dir/a.txt", View: true}
	if work.Key() == src.Key() {
		t.Fatal("working and revision views share a key")
	}
	f.write("dir/a.txt", "one\r\nTWO\r\nthree\r\n")
	f.sync()
	wt := f.loadDiff(work)
	if wt.Notice != "" || len(wt.New.Lines) != 3 || wt.New.Lines[1] != "TWO" || wt.New.Title != "Your version" {
		t.Fatalf("working view = %+v", wt)
	}

	// An absent path is an explanation, not an error.
	miss := f.loadDiff(DiffSource{Path: "dir/nope.txt", Commit: head, View: true})
	if miss.Notice == "" || len(miss.New.Lines) != 0 {
		t.Fatalf("missing = %+v", miss)
	}

	// Binary content is reported instead of decoded.
	f.write("dir/bin", "a\x00b")
	f.commitAll("Binary")
	binRev := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	bin := f.loadDiff(DiffSource{Path: "dir/bin", Commit: binRev, View: true})
	if !strings.Contains(bin.Notice, "Binary") {
		t.Fatalf("binary = %+v", bin)
	}
}
