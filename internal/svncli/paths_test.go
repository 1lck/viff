package svncli

import (
	"context"
	"strings"
	"testing"

	"viff/internal/vcs"
)

// TestCommitDetailPathsUnderTrunk is the regression for the reported failure:
// a working copy rooted at trunk reported changed paths relative to the
// repository root ("/trunk/src/main.go"), and joining that to the working copy
// URL built "trunk/trunk/src/main.go", which svn rejects with W160013.
func TestCommitDetailPathsUnderTrunk(t *testing.T) {
	r, repo, wc := newLocalRepo(t)
	ctx := context.Background()

	write(t, wc, "src/main.go", "package main\n")
	write(t, wc, "README.md", "readme\n")
	if err := r.Stage(ctx, repo, []string{"src/main.go", "README.md"}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if out, err := r.Commit(ctx, repo, vcs.CommitOptions{Message: "initial"}); err != nil {
		t.Fatalf("commit: %v: %s", err, out.Stdout)
	}

	reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer reader.Close()
	commits, _, err := reader.Next(10)
	if err != nil || len(commits) == 0 {
		t.Fatalf("log: %v (%d commits)", err, len(commits))
	}
	rev := commits[0].Hash

	detail, err := r.CommitDetail(ctx, repo, rev)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Changes) == 0 {
		t.Fatal("commit detail has no changed paths")
	}
	for _, c := range detail.Changes {
		if strings.HasPrefix(c.Path, "trunk/") {
			t.Fatalf("path %q still carries the working copy prefix", c.Path)
		}
	}
	var sawMain bool
	for _, c := range detail.Changes {
		if c.Path == "src/main.go" {
			sawMain = true
		}
	}
	if !sawMain {
		t.Fatalf("changes = %+v, want src/main.go", detail.Changes)
	}

	// The paths from the detail must be readable at that revision; this is what
	// the diff view does with them.
	for _, c := range detail.Changes {
		if c.Kind == 5 { // deleted paths have no content at this revision
			continue
		}
		content, err := r.Read(ctx, repo, vcs.RevCommit(rev), c.Path)
		if err != nil {
			t.Fatalf("read %q at %s: %v", c.Path, rev, err)
		}
		if !content.Exists {
			t.Fatalf("read %q at %s: reported absent", c.Path, rev)
		}
	}
}

// TestWCRelative covers the prefix logic directly, including a working copy
// that is not at trunk and an unversioned path outside it.
func TestWCRelative(t *testing.T) {
	cases := []struct {
		relURL, in, want string
		inside           bool
	}{
		{"^/trunk", "/trunk/src/main.go", "src/main.go", true},
		{"^/trunk", "/trunk", "", true},
		{"^/branches/feature", "/branches/feature/a.txt", "a.txt", true},
		{"^/branches/feature", "/trunk/a.txt", "trunk/a.txt", false},
		{"^/", "/a.txt", "a.txt", true},
		{"^/trunk", "/trunkx/a.txt", "trunkx/a.txt", false},
	}
	for _, c := range cases {
		got, inside := wcRelative(c.relURL, c.in)
		if got != c.want || inside != c.inside {
			t.Fatalf("wcRelative(%q, %q) = (%q, %v), want (%q, %v)", c.relURL, c.in, got, inside, c.want, c.inside)
		}
	}
}
