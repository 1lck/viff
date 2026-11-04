package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// newSubmoduleManageRepo creates a repository at dir with an isolated
// environment, for the tests that need a child repository in a directory whose
// name matters (for example one with a space).
func newSubmoduleManageRepo(t *testing.T, dir string, env []string) *testRepo {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, _ = filepath.EvalSymlinks(dir)
	tr := &testRepo{t: t, dir: dir, r: &Runner{Env: env}}
	tr.git("init", "-q", "-b", "main")
	tr.git("config", "user.name", "Viff Test")
	tr.git("config", "user.email", "test@example.com")
	tr.git("config", "commit.gpgsign", "false")
	repo, err := tr.r.Discover(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	tr.repo = repo
	return tr
}

// fileProtocolEnv is the isolated test environment with local submodule
// transport allowed. Production Viff never enables the file transport.
func fileProtocolEnv(t *testing.T) []string {
	t.Helper()
	return append(isolatedEnv(t), "GIT_ALLOW_PROTOCOL=file")
}

func submodulePaths(t *testing.T, r *Runner, repo domain.Repo) []domain.Submodule {
	t.Helper()
	subs, err := r.Submodules(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return subs
}

func findSub(t *testing.T, subs []domain.Submodule, path string) (domain.Submodule, bool) {
	t.Helper()
	for _, sm := range subs {
		if sm.Path == path {
			return sm, true
		}
	}
	return domain.Submodule{}, false
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestSubmoduleAddDeinitRemove drives the three write operations against real
// repositories, including a path and a URL directory that contain spaces.
func TestSubmoduleAddDeinitRemove(t *testing.T) {
	env := fileProtocolEnv(t)
	child := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "child lib"), env)
	child.write("lib.txt", "v1\n")
	child.commitAll("child")

	parent := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "parent"), env)
	parent.write("p.txt", "parent\n")
	parent.commitAll("parent")

	const path = "deps/lib dir"
	if err := parent.r.AddSubmodule(ctx, parent.repo, child.dir, path, nil); err != nil {
		t.Fatal(err)
	}
	subs := submodulePaths(t, parent.r, parent.repo)
	sm, ok := findSub(t, subs, path)
	if !ok || !sm.Initialized() || sm.State != domain.SubmoduleClean {
		t.Fatalf("after add: %+v", subs)
	}
	if _, err := os.Stat(filepath.Join(parent.dir, "deps", "lib dir", "lib.txt")); err != nil {
		t.Fatalf("submodule working tree missing: %v", err)
	}
	// Git wrote .gitmodules and staged the entry; committing is the user's.
	if modules := readFileString(t, filepath.Join(parent.dir, ".gitmodules")); !strings.Contains(modules, path) {
		t.Fatalf(".gitmodules does not record %q:\n%s", path, modules)
	}
	parent.commitAll("add submodule")

	dirty, err := parent.r.SubmoduleDirty(ctx, parent.repo, path)
	if err != nil || dirty {
		t.Fatalf("clean submodule reported dirty=%v err=%v", dirty, err)
	}

	// Deinit empties the working tree but keeps the index entry and
	// .gitmodules, so the submodule can be checked out again.
	if err := parent.r.DeinitSubmodule(ctx, parent.repo, path, false); err != nil {
		t.Fatal(err)
	}
	// Git clears the working tree but may keep the now-empty directory; what
	// matters is that no files are left behind.
	if entries, err := os.ReadDir(filepath.Join(parent.dir, "deps", "lib dir")); err == nil && len(entries) != 0 {
		t.Fatalf("deinit left files in the working tree: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("deinit left the working tree: %v", err)
	}
	subs = submodulePaths(t, parent.r, parent.repo)
	if sm, ok := findSub(t, subs, path); !ok || sm.Initialized() {
		t.Fatalf("after deinit: %+v", subs)
	}
	if index := parent.git("ls-files", "-s", "--", path); !strings.Contains(index, path) {
		t.Fatalf("deinit removed the index entry: %q", index)
	}

	if err := parent.r.UpdateSubmodule(ctx, parent.repo, path, true, nil); err != nil {
		t.Fatal(err)
	}
	subs = submodulePaths(t, parent.r, parent.repo)
	if sm, ok := findSub(t, subs, path); !ok || sm.State != domain.SubmoduleClean {
		t.Fatalf("after update: %+v", subs)
	}

	// Uncommitted work inside the submodule refuses a non-forced removal and
	// leaves everything in place.
	libPath := filepath.Join(parent.dir, "deps", "lib dir", "lib.txt")
	if err := os.WriteFile(libPath, []byte("changed by the user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = parent.r.SubmoduleDirty(ctx, parent.repo, path)
	if err != nil || !dirty {
		t.Fatalf("dirty submodule reported dirty=%v err=%v", dirty, err)
	}
	// Deinit refuses the same changes without force and keeps them.
	if err := parent.r.DeinitSubmodule(ctx, parent.repo, path, false); !errors.Is(err, ErrSubmoduleNeedsForce) {
		t.Fatalf("deinit of a dirty submodule = %v, want ErrSubmoduleNeedsForce", err)
	}
	if got := readFileString(t, libPath); got != "changed by the user\n" {
		t.Fatalf("refused deinit changed the checkout: %q", got)
	}
	if subs := submodulePaths(t, parent.r, parent.repo); len(subs) != 1 || !subs[0].Initialized() {
		t.Fatalf("refused deinit changed the list: %+v", subs)
	}
	// Force clears the working tree; update then checks out the recorded
	// commit again.
	if err := parent.r.DeinitSubmodule(ctx, parent.repo, path, true); err != nil {
		t.Fatalf("forced deinit: %v", err)
	}
	if err := parent.r.UpdateSubmodule(ctx, parent.repo, path, true, nil); err != nil {
		t.Fatalf("update after forced deinit: %v", err)
	}
	if got := readFileString(t, libPath); got != "v1\n" {
		t.Fatalf("update after forced deinit restored %q", got)
	}
	if err := os.WriteFile(libPath, []byte("changed by the user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := parent.r.RemoveSubmodule(ctx, parent.repo, path, false); !errors.Is(err, ErrSubmoduleNeedsForce) {
		t.Fatalf("remove of a dirty submodule = %v, want ErrSubmoduleNeedsForce", err)
	}
	if got := readFileString(t, libPath); got != "changed by the user\n" {
		t.Fatalf("refused removal changed the checkout: %q", got)
	}
	if index := parent.git("ls-files", "-s", "--", path); !strings.Contains(index, path) {
		t.Fatalf("refused removal changed the index: %q", index)
	}

	// Force removes the checkout, the gitlink and the .gitmodules entry.
	if err := parent.r.RemoveSubmodule(ctx, parent.repo, path, true); err != nil {
		t.Fatalf("forced remove: %v", err)
	}
	if subs := submodulePaths(t, parent.r, parent.repo); len(subs) != 0 {
		t.Fatalf("after remove: %+v", subs)
	}
	if _, err := os.Stat(filepath.Join(parent.dir, "deps", "lib dir")); !os.IsNotExist(err) {
		t.Fatalf("removal left the working tree: %v", err)
	}
	if index := parent.git("ls-files", "-s", "--", path); index != "" {
		t.Fatalf("removal left the index entry: %q", index)
	}
	if modules := readFileString(t, filepath.Join(parent.dir, ".gitmodules")); strings.Contains(modules, path) {
		t.Fatalf(".gitmodules still records %q:\n%s", path, modules)
	}
}

// TestSubmoduleRemovalRefusesDirtyGitmodules checks the failure the review
// found: when .gitmodules has local changes git rm refuses, and the refusal
// must come before anything is taken out of the checkout.
func TestSubmoduleRemovalRefusesDirtyGitmodules(t *testing.T) {
	env := fileProtocolEnv(t)
	child := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "child"), env)
	child.write("lib.txt", "v1\n")
	child.commitAll("child")

	parent := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "parent"), env)
	parent.write("p.txt", "parent\n")
	parent.commitAll("parent")
	const path = "sub"
	if err := parent.r.AddSubmodule(ctx, parent.repo, child.dir, path, nil); err != nil {
		t.Fatal(err)
	}
	parent.commitAll("add submodule")

	modulesPath := filepath.Join(parent.dir, ".gitmodules")
	modulesBefore := readFileString(t, modulesPath)
	withNote := modulesBefore + "# a local note\n"
	if err := os.WriteFile(modulesPath, []byte(withNote), 0o644); err != nil {
		t.Fatal(err)
	}
	indexBefore := parent.git("ls-files", "-s", "--", path)
	libPath := filepath.Join(parent.dir, path, "lib.txt")

	for _, force := range []bool{false, true} {
		err := parent.r.RemoveSubmodule(ctx, parent.repo, path, force)
		if err == nil {
			t.Fatalf("remove with force=%v succeeded despite local .gitmodules changes", force)
		}
		if errors.Is(err, ErrSubmoduleNeedsForce) {
			t.Fatalf("force=%v: .gitmodules refusal was reported as a discard request: %v", force, err)
		}
		if got := readFileString(t, libPath); got != "v1\n" {
			t.Fatalf("force=%v: removal changed the submodule checkout: %q", force, got)
		}
		if got := parent.git("ls-files", "-s", "--", path); got != indexBefore {
			t.Fatalf("force=%v: removal changed the index: %q, want %q", force, got, indexBefore)
		}
		if got := readFileString(t, modulesPath); got != withNote {
			t.Fatalf("force=%v: removal changed .gitmodules:\n%s", force, got)
		}
		if subs := submodulePaths(t, parent.r, parent.repo); len(subs) != 1 {
			t.Fatalf("force=%v: submodule list changed: %+v", force, subs)
		}
	}
}

// TestSubmoduleLiteralPaths makes sure a path that Git would read as a glob
// (deps/x[1]) only ever touches the submodule in that row, not deps/x1.
func TestSubmoduleLiteralPaths(t *testing.T) {
	env := fileProtocolEnv(t)
	child := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "child"), env)
	child.write("lib.txt", "v1\n")
	child.commitAll("one")
	child.write("lib.txt", "v2\n")
	child.commitAll("two")

	parent := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "parent"), env)
	parent.write("p.txt", "parent\n")
	parent.commitAll("parent")
	const bracket, plain = "deps/x[1]", "deps/x1"
	for _, p := range []string{bracket, plain} {
		if err := parent.r.AddSubmodule(ctx, parent.repo, child.dir, p, nil); err != nil {
			t.Fatalf("add %q: %v", p, err)
		}
	}
	parent.commitAll("add submodules")

	// Updating one path must not update the other, even though it matches the
	// glob deps/x[1].
	parent.git("-C", bracket, "checkout", "-q", "HEAD~1")
	parent.git("-C", plain, "checkout", "-q", "HEAD~1")
	if err := parent.r.UpdateSubmodule(ctx, parent.repo, bracket, false, nil); err != nil {
		t.Fatal(err)
	}
	subs := submodulePaths(t, parent.r, parent.repo)
	gotBracket, _ := findSub(t, subs, bracket)
	gotPlain, _ := findSub(t, subs, plain)
	if gotBracket.State != domain.SubmoduleClean || gotPlain.State != domain.SubmoduleModified {
		t.Fatalf("update touched the wrong submodule: %+v", subs)
	}

	// Sync is literal too: rewriting both URLs in .gitmodules and syncing
	// only deps/x1 must leave deps/x[1]'s configured URL alone. The URLs are
	// written through git config so .gitmodules gets Git's quoting, in
	// particular the escaped backslashes of a Windows path.
	other := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "other"), env)
	other.write("lib.txt", "other\n")
	other.commitAll("other")
	setGitmodulesURL(t, parent, bracket, other.dir)
	setGitmodulesURL(t, parent, plain, other.dir)
	if err := parent.r.SyncSubmodules(ctx, parent.repo, plain); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(parent.git("config", "--get", "submodule."+plain+".url")); got != other.dir {
		t.Fatalf("sync url for %q = %q, want %q", plain, got, other.dir)
	}
	if got := strings.TrimSpace(parent.git("config", "--get", "submodule."+bracket+".url")); got != child.dir {
		t.Fatalf("sync with %q changed %q's url to %q", plain, bracket, got)
	}
	// Restore .gitmodules so the later deinit/remove see a clean file.
	parent.git("checkout", "-q", "--", ".gitmodules")

	// Deinit must clear only deps/x[1].
	if err := parent.r.DeinitSubmodule(ctx, parent.repo, bracket, false); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Join(parent.dir, filepath.FromSlash(bracket))); err == nil && len(entries) != 0 {
		t.Fatalf("deinit did not clear %q: %v", bracket, entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("deinit left %q: %v", bracket, err)
	}
	if _, err := os.Stat(filepath.Join(parent.dir, filepath.FromSlash(plain), "lib.txt")); err != nil {
		t.Fatalf("deinit cleared the other submodule: %v", err)
	}
	subs = submodulePaths(t, parent.r, parent.repo)
	if sm, ok := findSub(t, subs, plain); !ok || !sm.Initialized() {
		t.Fatalf("the other submodule was deinitialized: %+v", subs)
	}

	// Removing deps/x[1] must leave deps/x1 alone.
	if err := parent.r.RemoveSubmodule(ctx, parent.repo, bracket, false); err != nil {
		t.Fatalf("remove %q: %v", bracket, err)
	}
	subs = submodulePaths(t, parent.r, parent.repo)
	if _, ok := findSub(t, subs, bracket); ok {
		t.Fatalf("%q still listed: %+v", bracket, subs)
	}
	if sm, ok := findSub(t, subs, plain); !ok || !sm.Initialized() {
		t.Fatalf("%q was removed together with %q: %+v", plain, bracket, subs)
	}
	modulesAfter := readFileString(t, filepath.Join(parent.dir, ".gitmodules"))
	if strings.Contains(modulesAfter, bracket) {
		t.Fatalf(".gitmodules still records %q:\n%s", bracket, modulesAfter)
	}
	if status := parent.git("status", "--porcelain"); !strings.Contains(status, "M  .gitmodules") || !strings.Contains(status, "D  "+bracket) {
		t.Fatalf("removal was not staged as git rm stages it: %q", status)
	}
}

// TestDropSubmoduleSection covers the section removal used when Git's own
// .gitmodules cleanup misses a glob-like path.
func TestDropSubmoduleSection(t *testing.T) {
	const data = `[submodule "deps/x[1]"]
	path = deps/x[1]
	url = ../child
[submodule "deps/lib dir"]
	path = deps/lib dir
	branch = main
[submodule "quote\"name"]
	path = quote"name
	url = ../quoted
`
	out, removed := dropSubmoduleSection([]byte(data), "deps/lib dir")
	if !removed {
		t.Fatal("section was not removed")
	}
	if strings.Contains(string(out), "deps/lib dir") || strings.Contains(string(out), "branch = main") {
		t.Fatalf("removed section left lines behind:\n%s", out)
	}
	for _, keep := range []string{`[submodule "deps/x[1]"]`, "[submodule \"quote\\\"name\"]"} {
		if !strings.Contains(string(out), keep) {
			t.Fatalf("removal damaged another section (missing %q):\n%s", keep, out)
		}
	}

	out, removed = dropSubmoduleSection([]byte(data), `quote"name`)
	if !removed || strings.Contains(string(out), "quote") || !strings.Contains(string(out), "deps/lib dir") {
		t.Fatalf("quoted name removal = %q, %v", out, removed)
	}

	if _, removed := dropSubmoduleSection([]byte(data), "elsewhere"); removed {
		t.Fatal("a missing section was reported removed")
	}

	// No trailing newline in the file: the surviving block stays intact.
	noNewline := []byte("[submodule \"one\"]\n\tpath = one\n[submodule \"two\"]\n\tpath = two")
	out, removed = dropSubmoduleSection(noNewline, "two")
	if !removed || string(out) != "[submodule \"one\"]\n\tpath = one\n" {
		t.Fatalf("no-newline removal = %q, %v", out, removed)
	}
}

func TestUnquoteConfigString(t *testing.T) {
	cases := map[string]string{
		`"plain"`:        "plain",
		`"with space"`:   "with space",
		`"deps/x[1]"`:    "deps/x[1]",
		`"quote\"name"`:  `quote"name`,
		`"back\\slash"`:  `back\slash`,
		`"tab\there"`:    "tab\there",
		`"中文"`:           "中文",
		`"\346\226\207"`: "文",
	}
	for in, want := range cases {
		got, ok := unquoteConfigString(in)
		if !ok || got != want {
			t.Fatalf("unquoteConfigString(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", `"unterminated`, `"bad\q"`, `"\1"`} {
		if got, ok := unquoteConfigString(bad); ok {
			t.Fatalf("unquoteConfigString(%q) = %q, true; want false", bad, got)
		}
	}
}

// TestSubmoduleNestedRefused checks that a nested submodule listed by the
// recursive status cannot be deinitialized or removed through the parent
// repository: it belongs to the repository that records it.
func TestSubmoduleNestedRefused(t *testing.T) {
	env := fileProtocolEnv(t)
	leaf := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "leaf"), env)
	leaf.write("leaf.txt", "leaf\n")
	leaf.commitAll("leaf")

	mid := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "mid"), env)
	mid.write("mid.txt", "mid\n")
	mid.commitAll("mid")
	if err := mid.r.AddSubmodule(ctx, mid.repo, leaf.dir, "leaf", nil); err != nil {
		t.Fatal(err)
	}
	mid.commitAll("add leaf")

	top := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "top"), env)
	top.write("top.txt", "top\n")
	top.commitAll("top")
	if err := top.r.AddSubmodule(ctx, top.repo, mid.dir, "mid", nil); err != nil {
		t.Fatal(err)
	}
	top.commitAll("add mid")
	if err := top.r.UpdateSubmodule(ctx, top.repo, "", true, nil); err != nil {
		t.Fatal(err)
	}
	subs := submodulePaths(t, top.r, top.repo)
	if _, ok := findSub(t, subs, "mid/leaf"); !ok {
		t.Fatalf("nested submodule not listed recursively: %+v", subs)
	}
	nestedDir := filepath.Join(top.dir, "mid", "leaf", "leaf.txt")
	if _, err := os.Stat(nestedDir); err != nil {
		t.Fatalf("nested working tree missing: %v", err)
	}

	for _, err := range []error{
		top.r.DeinitSubmodule(ctx, top.repo, "mid/leaf", false),
		top.r.RemoveSubmodule(ctx, top.repo, "mid/leaf", false),
	} {
		if err == nil {
			t.Fatal("nested submodule operation succeeded through the parent repository")
		}
		if !strings.Contains(err.Error(), "nested submodule") || !strings.Contains(err.Error(), "mid") {
			t.Fatalf("nested refusal = %v, want the owning repository named", err)
		}
	}
	if _, err := os.Stat(nestedDir); err != nil {
		t.Fatalf("refused nested operation changed the checkout: %v", err)
	}
	if subs := submodulePaths(t, top.r, top.repo); len(subs) != 2 {
		t.Fatalf("refused nested operation changed the list: %+v", subs)
	}

	// A path that is not a submodule is refused before any Git write.
	if err := top.r.RemoveSubmodule(ctx, top.repo, "top.txt", false); err == nil || !strings.Contains(err.Error(), "not a submodule") {
		t.Fatalf("remove of a plain file = %v", err)
	}
	if err := top.r.DeinitSubmodule(ctx, top.repo, "../outside", false); err == nil {
		t.Fatal("deinit of an escaping path succeeded")
	}
}

func TestCleanSubmodulePath(t *testing.T) {
	valid := map[string]string{
		"deps/lib":       "deps/lib",
		" deps/lib ":     "deps/lib",
		"deps/lib dir":   "deps/lib dir",
		"deps/x[1]":      "deps/x[1]",
		"中文/子模块":         "中文/子模块",
		"a/b/c":          "a/b/c",
		"a.b-c_d":        "a.b-c_d",
		"deps/lib[back]": "deps/lib[back]",
		"deps/x?.y":      "deps/x?.y",
		"deps/*star":     "deps/*star",
	}
	for in, want := range valid {
		got, err := CleanSubmodulePath(in)
		if err != nil || got != want {
			t.Fatalf("CleanSubmodulePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	invalid := []string{"", "   ", "../x", "a/../b", "/abs", "a//b", "a/", ".", "..", "a\x00b", "a\nb", ".git/x", "a/.git", ".gitmodules"}
	for _, in := range invalid {
		if got, err := CleanSubmodulePath(in); err == nil {
			t.Fatalf("CleanSubmodulePath(%q) = %q, want an error", in, got)
		}
	}
}

// TestAddSubmoduleReportsFailures checks that a failed add is reported as a
// failure and leaves the repository untouched.
func TestAddSubmoduleReportsFailures(t *testing.T) {
	env := fileProtocolEnv(t)
	parent := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "parent"), env)
	parent.write("p.txt", "parent\n")
	parent.commitAll("parent")
	indexBefore := parent.git("ls-files")

	// Invalid input is refused before Git runs.
	for _, tc := range []struct{ url, path, want string }{
		{"", "deps/lib", "URL is empty"},
		{"-x", "deps/lib", "must not start with '-'"},
		{"../child", "../outside", ".."},
		{"../child", "/abs", "relative"},
	} {
		err := parent.r.AddSubmodule(ctx, parent.repo, tc.url, tc.path, nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("AddSubmodule(%q, %q) = %v, want %q", tc.url, tc.path, err, tc.want)
		}
	}
	if got := parent.git("ls-files"); got != indexBefore {
		t.Fatalf("invalid add changed the index: %q", got)
	}
	if subs := submodulePaths(t, parent.r, parent.repo); len(subs) != 0 {
		t.Fatalf("invalid add created a submodule: %+v", subs)
	}

	// A URL Git cannot reach is Git's failure, reported with Git's message.
	err := parent.r.AddSubmodule(ctx, parent.repo, filepath.Join(t.TempDir(), "missing"), "deps/lib", nil)
	if err == nil {
		t.Fatal("adding a missing repository succeeded")
	}
	if KindOf(err) == ErrCancelled {
		t.Fatalf("missing repository reported as cancelled: %v", err)
	}
	if subs := submodulePaths(t, parent.r, parent.repo); len(subs) != 0 {
		t.Fatalf("failed add left a submodule: %+v", subs)
	}
	if _, statErr := os.Stat(filepath.Join(parent.dir, ".gitmodules")); statErr == nil {
		t.Fatalf("failed add left .gitmodules")
	}

	// The file transport stays closed unless the user's Git config opens it.
	closed := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "closed"), isolatedEnv(t))
	closed.write("p.txt", "p\n")
	closed.commitAll("p")
	child := newSubmoduleManageRepo(t, filepath.Join(t.TempDir(), "child"), env)
	child.write("lib.txt", "v1\n")
	child.commitAll("child")
	err = closed.r.AddSubmodule(ctx, closed.repo, child.dir, "sub", nil)
	if err == nil {
		t.Fatal("file transport was allowed by default")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "transport 'file' not allowed") {
		t.Fatalf("unexpected error for a closed file transport: %v", err)
	}
	if subs := submodulePaths(t, closed.r, closed.repo); len(subs) != 0 {
		t.Fatalf("failed add left a submodule: %+v", subs)
	}
}
