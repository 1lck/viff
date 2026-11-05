package app

import (
	"errors"
	"strings"
	"testing"

	"viff/internal/domain"
)

// The remote URL forms a clone can leave behind: https, ssh with and without a
// port, scp-like, with a ".git" suffix or without, on the SaaS hosts and on
// self-hosted instances. Each case checks the service, the project path and the
// repository page, which is the one address every host has.
func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		forge   Forge
		host    string
		path    string
		repoURL string
	}{
		{
			name: "https with .git", raw: "https://github.com/acme/demo.git",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
		{
			name: "https without .git", raw: "https://github.com/acme/demo",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
		{
			name: "trailing slash", raw: "https://github.com/acme/demo.git/",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
		{
			name: "uppercase host and suffix", raw: "https://GitHub.COM/Acme/Demo.GIT",
			forge: ForgeGitHub, host: "github.com", path: "Acme/Demo",
			repoURL: "https://github.com/Acme/Demo",
		},
		{
			name: "scp-like", raw: "git@github.com:acme/demo.git",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
		{
			name: "scp-like without suffix and user", raw: "github.com:acme/demo",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
		{
			name: "https port is kept", raw: "https://git.example.com:8443/acme/demo.git",
			forge: ForgeUnknown, host: "git.example.com:8443", path: "acme/demo",
			repoURL: "https://git.example.com:8443/acme/demo",
		},
		{
			name: "ssh port is dropped", raw: "ssh://git@git.example.com:2222/acme/demo.git",
			forge: ForgeUnknown, host: "git.example.com", path: "acme/demo",
			repoURL: "https://git.example.com/acme/demo",
		},
		{
			name: "git protocol", raw: "git://github.com/acme/demo.git",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
		{
			name: "gitlab subgroup", raw: "https://gitlab.com/group/subgroup/demo.git",
			forge: ForgeGitLab, host: "gitlab.com", path: "group/subgroup/demo",
			repoURL: "https://gitlab.com/group/subgroup/demo",
		},
		{
			name: "self-hosted gitlab", raw: "ssh://git@gitlab.example.com/team/demo.git",
			forge: ForgeGitLab, host: "gitlab.example.com", path: "team/demo",
			repoURL: "https://gitlab.example.com/team/demo",
		},
		{
			name: "bitbucket", raw: "https://bitbucket.org/team/demo.git",
			forge: ForgeBitbucket, host: "bitbucket.org", path: "team/demo",
			repoURL: "https://bitbucket.org/team/demo",
		},
		{
			name: "codeberg is gitea", raw: "https://codeberg.org/team/demo.git",
			forge: ForgeGitea, host: "codeberg.org", path: "team/demo",
			repoURL: "https://codeberg.org/team/demo",
		},
		{
			name: "self-hosted gitea", raw: "https://gitea.example.com/team/demo.git",
			forge: ForgeGitea, host: "gitea.example.com", path: "team/demo",
			repoURL: "https://gitea.example.com/team/demo",
		},
		{
			name: "self-hosted instance", raw: "https://git.example.com/team/demo.git",
			forge: ForgeUnknown, host: "git.example.com", path: "team/demo",
			repoURL: "https://git.example.com/team/demo",
		},
		{
			// A token in the remote must never reach a browser address.
			name: "credentials are dropped", raw: "https://user:s3cret@github.com/acme/demo.git",
			forge: ForgeGitHub, host: "github.com", path: "acme/demo",
			repoURL: "https://github.com/acme/demo",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			link, err := ParseRemoteURL(c.raw)
			if err != nil {
				t.Fatalf("ParseRemoteURL(%q): %v", c.raw, err)
			}
			if link.Forge != c.forge || link.Host != c.host || link.Path != c.path {
				t.Fatalf("%q = forge %q host %q path %q, want %q %q %q",
					c.raw, link.Forge, link.Host, link.Path, c.forge, c.host, c.path)
			}
			if got := link.RepoURL(); got != c.repoURL {
				t.Fatalf("RepoURL = %q, want %q", got, c.repoURL)
			}
		})
	}
}

// A remote Viff cannot read is an error the window shows, not a panic and not
// an empty address that would open the browser's home page.
func TestParseRemoteURLErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"local path", "/srv/git/demo.git"},
		{"relative path", "../demo"},
		{"windows drive path", "C:/Users/me/remote.git"},
		{"windows drive path with backslashes", `C:\Users\me\remote.git`},
		{"file transport", "file:///srv/git/demo.git"},
		{"owner only", "https://github.com/acme"},
		{"scp owner only", "git@github.com:demo.git"},
		{"no host", "https://"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			link, err := ParseRemoteURL(c.raw)
			if err == nil {
				t.Fatalf("ParseRemoteURL(%q) = %q, want an error", c.raw, link.RepoURL())
			}
			if !errors.Is(err, ErrRemoteURL) {
				t.Fatalf("error %q does not wrap ErrRemoteURL", err)
			}
			if link.RepoURL() != "" {
				t.Fatalf("failed parse returned a URL: %q", link.RepoURL())
			}
		})
	}
}

// The page URLs of each service, including the two layouts Viff cannot build
// (Bitbucket's comparison and pull request pages) and the GitHub layout assumed
// for a host it does not recognise.
func TestRemoteLinkPages(t *testing.T) {
	parse := func(t *testing.T, raw string) RemoteLink {
		t.Helper()
		link, err := ParseRemoteURL(raw)
		if err != nil {
			t.Fatalf("ParseRemoteURL(%q): %v", raw, err)
		}
		return link
	}
	page := func(t *testing.T, got string, err error, want string) {
		t.Helper()
		if err != nil {
			t.Fatalf("link: %v, want %q", err, want)
		}
		if got != want {
			t.Fatalf("link = %q, want %q", got, want)
		}
	}

	t.Run("github", func(t *testing.T) {
		l := parse(t, "https://github.com/acme/demo.git")
		commit, err := l.CommitURL("abc123")
		page(t, commit, err, "https://github.com/acme/demo/commit/abc123")
		branch, err := l.BranchURL("feature/search")
		page(t, branch, err, "https://github.com/acme/demo/tree/feature/search")
		compare, err := l.CompareURL("main", "feature/search")
		page(t, compare, err, "https://github.com/acme/demo/compare/main...feature/search")
		pr, err := l.NewPullRequestURL("main", "feature/search")
		page(t, pr, err, "https://github.com/acme/demo/compare/main...feature/search?expand=1")
	})

	t.Run("gitlab", func(t *testing.T) {
		l := parse(t, "https://gitlab.com/group/subgroup/demo.git")
		commit, err := l.CommitURL("abc123")
		page(t, commit, err, "https://gitlab.com/group/subgroup/demo/-/commit/abc123")
		branch, err := l.BranchURL("main")
		page(t, branch, err, "https://gitlab.com/group/subgroup/demo/-/tree/main")
		compare, err := l.CompareURL("main", "feature")
		page(t, compare, err, "https://gitlab.com/group/subgroup/demo/-/compare/main...feature")
		pr, err := l.NewPullRequestURL("main", "feature/x")
		page(t, pr, err, "https://gitlab.com/group/subgroup/demo/-/merge_requests/new?merge_request%5Bsource_branch%5D=feature%2Fx&merge_request%5Btarget_branch%5D=main")
	})

	t.Run("gitea", func(t *testing.T) {
		l := parse(t, "https://codeberg.org/acme/demo.git")
		commit, err := l.CommitURL("abc123")
		page(t, commit, err, "https://codeberg.org/acme/demo/commit/abc123")
		branch, err := l.BranchURL("main")
		page(t, branch, err, "https://codeberg.org/acme/demo/src/branch/main")
		compare, err := l.CompareURL("main", "feature")
		page(t, compare, err, "https://codeberg.org/acme/demo/compare/main...feature")
		pr, err := l.NewPullRequestURL("main", "feature")
		page(t, pr, err, "https://codeberg.org/acme/demo/compare/main...feature?expand=1")
	})

	t.Run("bitbucket", func(t *testing.T) {
		l := parse(t, "https://bitbucket.org/team/demo.git")
		commit, err := l.CommitURL("abc123")
		page(t, commit, err, "https://bitbucket.org/team/demo/commits/abc123")
		branch, err := l.BranchURL("main")
		page(t, branch, err, "https://bitbucket.org/team/demo/branch/main")
		// Best effort stops here: a comparison address Viff invented would 404.
		if _, err := l.CompareURL("main", "feature"); !errors.Is(err, ErrLinkUnsupported) {
			t.Fatalf("CompareURL error = %v, want ErrLinkUnsupported", err)
		}
		if _, err := l.NewPullRequestURL("main", "feature"); !errors.Is(err, ErrLinkUnsupported) {
			t.Fatalf("NewPullRequestURL error = %v, want ErrLinkUnsupported", err)
		}
	})

	t.Run("unknown host uses the github layout", func(t *testing.T) {
		l := parse(t, "https://git.example.com/team/demo.git")
		if got := l.Label(); got != "git.example.com" {
			t.Fatalf("Label = %q, want the host", got)
		}
		commit, err := l.CommitURL("abc123")
		page(t, commit, err, "https://git.example.com/team/demo/commit/abc123")
		compare, err := l.CompareURL("main", "feature")
		page(t, compare, err, "https://git.example.com/team/demo/compare/main...feature")
	})

	t.Run("branch names are escaped per segment", func(t *testing.T) {
		l := parse(t, "https://github.com/acme/demo.git")
		compare, err := l.CompareURL("main", "feat/ü#1")
		page(t, compare, err, "https://github.com/acme/demo/compare/main...feat/%C3%BC%231")
	})
}

// A page that cannot exist is an error: comparing a branch with itself and
// asking for a page of an empty link are both refused before a browser starts.
func TestRemoteLinkRefusesEmptyPages(t *testing.T) {
	l, err := ParseRemoteURL("https://github.com/acme/demo.git")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.CompareURL("main", "main"); err == nil {
		t.Fatal("comparing main with main is not an error")
	}
	if _, err := l.CompareURL("", "main"); err == nil {
		t.Fatal("an empty base is not an error")
	}
	if _, err := l.NewPullRequestURL("main", ""); err == nil {
		t.Fatal("an empty head is not an error")
	}
	var zero RemoteLink
	if zero.RepoURL() != "" {
		t.Fatalf("zero link RepoURL = %q", zero.RepoURL())
	}
	if _, err := zero.CommitURL("abc123"); !errors.Is(err, ErrRemoteURL) {
		t.Fatalf("zero link CommitURL error = %v", err)
	}
	if _, err := zero.BranchURL("main"); !errors.Is(err, ErrRemoteURL) {
		t.Fatalf("zero link BranchURL error = %v", err)
	}
}

// The base of a comparison is the branch's own upstream, else the default
// branch of the remote, else main or master; a branch is never its own base.
func TestBaseBranchFromRepository(t *testing.T) {
	f := newFixture(t)
	remote := t.TempDir() + "/remote.git"
	f.git("init", "-q", "--bare", "-b", "main", remote)
	f.write("a.txt", "a\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "init")
	f.git("remote", "add", "origin", remote)
	f.git("push", "-q", "-u", "origin", "main")
	f.git("checkout", "-q", "-b", "feature")
	f.git("push", "-q", "-u", "origin", "feature")
	f.open()
	rs := f.w.Repo
	if rs.RemoteHeads["origin"] != "" {
		t.Fatalf("remote head = %q, want none yet", rs.RemoteHeads["origin"])
	}

	// The branch follows origin/feature, which is the branch itself: main is the
	// only sensible base.
	if base, ok := rs.BaseBranch("feature"); !ok || base != "main" {
		t.Fatalf("BaseBranch(feature) = %q, %v, want main", base, ok)
	}

	// The remote's default branch wins over main when Git knows it.
	f.git("branch", "develop")
	f.git("push", "-q", "origin", "develop")
	f.git("remote", "set-head", "origin", "develop")
	f.w.Refresh(true)
	f.settle(func() bool { return f.w.Repo.RemoteHeads["origin"] == "develop" && f.idle() })
	if base, ok := f.w.Repo.BaseBranch("feature"); !ok || base != "develop" {
		t.Fatalf("BaseBranch(feature) = %q, %v, want develop", base, ok)
	}

	// An upstream that is a different branch wins over the default branch.
	f.git("checkout", "-q", "-b", "fix")
	f.git("branch", "--set-upstream-to=origin/develop", "fix")
	f.w.Refresh(true)
	f.settle(func() bool { return f.idle() && len(f.w.Repo.RefsOf(domain.RefLocal)) >= 4 })
	if base, ok := f.w.Repo.BaseBranch("fix"); !ok || base != "develop" {
		t.Fatalf("BaseBranch(fix) = %q, %v, want develop", base, ok)
	}
}

// Without upstream, remote head, main or master there is no base to guess:
// ok is false and the window asks the user instead of opening a page.
func TestBaseBranchUnknown(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "init")
	f.git("branch", "-m", "main", "trunk")
	f.open()
	if base, ok := f.w.Repo.BaseBranch("trunk"); ok || base != "" {
		t.Fatalf("BaseBranch(trunk) = %q, %v, want no base", base, ok)
	}
	if name := f.w.Repo.BranchRemote("trunk"); name != "" {
		t.Fatalf("BranchRemote = %q, want no remote", name)
	}
	if _, err := f.w.Repo.LinkFor("trunk"); err == nil {
		t.Fatal("LinkFor without a remote is not an error")
	} else if !errors.Is(err, ErrRemoteURL) {
		t.Fatalf("LinkFor error %q does not wrap ErrRemoteURL", err)
	}
}

// A remote whose URL has no web page (a local bare repository here) still
// resolves as a remote, and the link is the error the window reports.
func TestLinkForLocalRemote(t *testing.T) {
	f := newFixture(t)
	remote := t.TempDir() + "/remote.git"
	f.git("init", "-q", "--bare", "-b", "main", remote)
	f.write("a.txt", "a\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "init")
	f.git("remote", "add", "origin", remote)
	f.git("push", "-q", "-u", "origin", "main")
	f.open()
	if name := f.w.Repo.BranchRemote("main"); name != "origin" {
		t.Fatalf("BranchRemote = %q, want origin", name)
	}
	_, err := f.w.Repo.LinkFor("main")
	if !errors.Is(err, ErrRemoteURL) {
		t.Fatalf("LinkFor error = %v, want ErrRemoteURL", err)
	}
	if !strings.Contains(err.Error(), "local path") {
		t.Fatalf("LinkFor error %q does not say why", err)
	}
}
