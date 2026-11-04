package gitcli

import (
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

func TestParseSubmodules(t *testing.T) {
	const (
		clean   = " 6300bb431e2faf7dce7803e07bd952c6b244ec34 sub dir (heads/main)\n"
		dirty   = "+a1555190f6bce77ea790241911377fd746fed31f deps/lib (v1.2.0-3-gabc1234)\n"
		uninit  = "-a0fc8efa737b650a450187d7031c01ef520a99f2 sub dir/gc-sub\n"
		confl   = "U1111111111111111111111111111111111111111 vendored\n"
		garbage = "not a status line\n"
	)
	got := ParseSubmodules([]byte(clean + dirty + uninit + confl + garbage))
	want := []domain.Submodule{
		{Path: "sub dir", SHA: "6300bb431e2faf7dce7803e07bd952c6b244ec34", State: domain.SubmoduleClean, Describe: "heads/main"},
		{Path: "deps/lib", SHA: "a1555190f6bce77ea790241911377fd746fed31f", State: domain.SubmoduleModified, Describe: "v1.2.0-3-gabc1234"},
		{Path: "sub dir/gc-sub", SHA: "a0fc8efa737b650a450187d7031c01ef520a99f2", State: domain.SubmoduleUninitialized},
		{Path: "vendored", SHA: "1111111111111111111111111111111111111111", State: domain.SubmoduleConflict},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Empty and truncated output never panics.
	for _, in := range []string{"", "\n", " ", " \n", "-", " abc"} {
		if list := ParseSubmodules([]byte(in)); len(list) != 0 {
			t.Fatalf("%q parsed to %+v", in, list)
		}
	}
	// Only the last parenthesized suffix is the description; parentheses
	// inside the path stay part of it.
	if got := ParseSubmodules([]byte(" 1234567890123456789012345678901234567890 odd (dir) (heads/main)\n")); len(got) != 1 ||
		got[0].Path != "odd (dir)" || got[0].Describe != "heads/main" {
		t.Fatalf("parenthesized path = %+v", got)
	}
}

func TestSubmoduleStateText(t *testing.T) {
	cases := map[domain.SubmoduleState]string{
		domain.SubmoduleClean:         "Up to date",
		domain.SubmoduleUninitialized: "Not initialized",
		domain.SubmoduleModified:      "Modified",
		domain.SubmoduleConflict:      "Conflict",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Fatalf("%q = %q, want %q", state, got, want)
		}
	}
	sm := domain.Submodule{State: domain.SubmoduleModified, SHA: "0123456789abcdef"}
	if sm.Initialized() != true || !sm.Changed() {
		t.Fatalf("modified submodule = %+v", sm)
	}
}

// TestSubmoduleUpdateAndSync drives Git in real repositories: a parent that
// records a local child, then an out-of-date checkout, a fresh clone that has
// no working tree yet, and a URL change followed by sync.
func TestSubmoduleUpdateAndSync(t *testing.T) {
	child := newTestRepo(t)
	child.write("lib.txt", "v1\n")
	child.commitAll("child v1")
	first := strings.TrimSpace(child.git("rev-parse", "HEAD"))
	child.write("lib.txt", "v2\n")
	child.commitAll("child v2")
	second := strings.TrimSpace(child.git("rev-parse", "HEAD"))

	parent := newTestRepo(t)
	parent.write("p.txt", "parent\n")
	parent.commitAll("parent")
	// Tests use local paths; Git requires opting in to the file transport.
	parent.r.Env = append(parent.r.Env, "GIT_ALLOW_PROTOCOL=file")
	parent.git("-c", "protocol.file.allow=always", "submodule", "add", child.dir, "sub dir")
	parent.commitAll("add submodule")

	repo := parent.repo
	subs, err := parent.r.Submodules(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("submodules = %+v", subs)
	}
	sm := subs[0]
	if sm.Path != "sub dir" || sm.SHA != second || sm.State != domain.SubmoduleClean || !sm.Initialized() {
		t.Fatalf("initial = %+v", sm)
	}
	if sm.Describe == "" {
		t.Fatalf("describe is empty: %+v", sm)
	}

	// Checking out an older commit inside the submodule makes it modified.
	parent.git("-C", "sub dir", "checkout", "-q", first)
	subs, err = parent.r.Submodules(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].State != domain.SubmoduleModified || subs[0].SHA != first || !subs[0].Changed() {
		t.Fatalf("modified = %+v", subs)
	}
	if err := parent.r.UpdateSubmodule(ctx, repo, "sub dir", true, nil); err != nil {
		t.Fatal(err)
	}
	subs, err = parent.r.Submodules(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].State != domain.SubmoduleClean || subs[0].SHA != second {
		t.Fatalf("after update = %+v", subs)
	}

	// A fresh clone has the commit recorded but no working tree.
	cloneDir := filepath.Join(t.TempDir(), "clone")
	parent.git("clone", "-q", parent.dir, cloneDir)
	cloneDir, _ = filepath.EvalSymlinks(cloneDir)
	clone, err := parent.r.Discover(ctx, cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	subs, err = parent.r.Submodules(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].State != domain.SubmoduleUninitialized || subs[0].SHA != second || subs[0].Describe != "" {
		t.Fatalf("clone = %+v", subs)
	}
	if subs[0].Initialized() {
		t.Fatalf("clone submodule reported as initialized: %+v", subs[0])
	}
	if err := parent.r.UpdateSubmodule(ctx, clone, "", true, nil); err != nil {
		t.Fatal(err)
	}
	subs, err = parent.r.Submodules(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].State != domain.SubmoduleClean {
		t.Fatalf("after init = %+v", subs)
	}

	// Sync copies a changed .gitmodules URL into .git/config. The URL is set
	// through git config so the value is written with the quoting Git expects:
	// replacing the text in the file would depend on how Git stored the old
	// value, and a Windows path's backslashes are escaped in .gitmodules.
	other := newTestRepo(t)
	other.write("lib.txt", "other\n")
	other.commitAll("other")
	setGitmodulesURL(t, parent, "sub dir", other.dir)
	if got := gitmodulesURL(t, parent, "sub dir"); got != other.dir {
		t.Fatalf(".gitmodules url = %q, want %q", got, other.dir)
	}
	if err := parent.r.SyncSubmodules(ctx, repo, "sub dir"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(parent.git("config", "--get", "submodule.sub dir.url")); got != other.dir {
		t.Fatalf("submodule url = %q, want %q", got, other.dir)
	}
}

// gitmodulesURL reads a submodule URL from .gitmodules through Git, so the
// value is decoded the same way on every platform: Git stores a Windows path
// with C-style escaped backslashes in the file.
func gitmodulesURL(t *testing.T, tr *testRepo, path string) string {
	t.Helper()
	out := tr.git("config", "-f", filepath.Join(tr.dir, ".gitmodules"), "--get", "submodule."+path+".url")
	return strings.TrimSpace(out)
}

// setGitmodulesURL rewrites a submodule URL in .gitmodules through Git, which
// applies the quoting the value needs on the platform.
func setGitmodulesURL(t *testing.T, tr *testRepo, path, url string) {
	t.Helper()
	tr.git("config", "-f", filepath.Join(tr.dir, ".gitmodules"), "submodule."+path+".url", url)
}

// TestSubmoduleRecursiveStatus covers nested submodules: status lists them
// with their full path, and update initializes the whole tree.
func TestSubmoduleRecursiveStatus(t *testing.T) {
	leaf := newTestRepo(t)
	leaf.write("leaf.txt", "leaf\n")
	leaf.commitAll("leaf")

	mid := newTestRepo(t)
	mid.write("mid.txt", "mid\n")
	mid.commitAll("mid")
	mid.r.Env = append(mid.r.Env, "GIT_ALLOW_PROTOCOL=file")
	mid.git("-c", "protocol.file.allow=always", "submodule", "add", leaf.dir, "leaf")
	mid.commitAll("add leaf")
	if err := mid.r.UpdateSubmodule(ctx, mid.repo, "", true, nil); err != nil {
		t.Fatal(err)
	}

	top := newTestRepo(t)
	top.write("top.txt", "top\n")
	top.commitAll("top")
	top.r.Env = append(top.r.Env, "GIT_ALLOW_PROTOCOL=file")
	top.git("-c", "protocol.file.allow=always", "submodule", "add", mid.dir, "mid")
	top.commitAll("add mid")

	subs, err := top.r.Submodules(ctx, top.repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("recursive = %+v", subs)
	}
	if subs[0].Path != "mid" || subs[0].State != domain.SubmoduleClean {
		t.Fatalf("top = %+v", subs[0])
	}
	// The nested leaf is recorded by mid but not checked out in it.
	if subs[1].Path != "mid/leaf" || subs[1].State != domain.SubmoduleUninitialized {
		t.Fatalf("nested = %+v", subs[1])
	}
	if err := top.r.UpdateSubmodule(ctx, top.repo, "", true, nil); err != nil {
		t.Fatal(err)
	}
	subs, err = top.r.Submodules(ctx, top.repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 || subs[1].State != domain.SubmoduleClean {
		t.Fatalf("after recursive init = %+v", subs)
	}
}
