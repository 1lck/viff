package app

// Remote links: the web pages of a repository, built from the URL of its
// remote. Viff never calls the hosting service's API and never needs a token;
// a link is the address the user could have typed into the browser, which also
// means it works for self-hosted instances the application knows nothing about
// beyond their URL. See docs/decisions/0029-pr-links.md.

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"viff/internal/domain"
)

// Forge is a hosting service whose web address layout Viff knows. The name is
// the one the menus show ("Open Commit on GitHub").
type Forge string

const (
	ForgeGitHub    Forge = "GitHub"
	ForgeGitLab    Forge = "GitLab"
	ForgeBitbucket Forge = "Bitbucket"
	ForgeGitea     Forge = "Gitea"
	// ForgeUnknown is a host Viff does not recognise, which in practice is a
	// self-hosted instance. Its repository, commit and compare pages follow the
	// GitHub layout, which Gitea, Forgejo, GitHub Enterprise and most small
	// forges share; when the guess is wrong the service answers with its own 404
	// rather than Viff hiding the entry for every host it does not know.
	ForgeUnknown Forge = ""
)

var (
	// ErrRemoteURL reports a remote Viff cannot turn into a web page: a local
	// path, a transport without a browser view, or a URL without owner/repo.
	ErrRemoteURL = errors.New("the remote has no web page")
	// ErrLinkUnsupported reports a page the service does not serve at a layout
	// Viff knows. The caller shows the repository or branch page instead of
	// opening an address that would not exist.
	ErrLinkUnsupported = errors.New("the service has no such page")
)

// remoteURLError builds a displayable reason wrapped in ErrRemoteURL.
func remoteURLError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRemoteURL, fmt.Sprintf(format, args...))
}

// RemoteLink is a repository's web address on the service its remote URL
// names. The zero value has no pages; the methods on it return an error rather
// than an empty string that would open the options page of the browser.
type RemoteLink struct {
	// Forge is the service whose layout the links follow, ForgeUnknown when the
	// host is not recognised.
	Forge Forge
	// Host is the host name, with the port the remote names for http(s).
	Host string
	// Path is the project path on the host: "owner/repo", or
	// "group/subgroup/repo" on GitLab.
	Path string
	// scheme is http or https; empty means https.
	scheme string
}

// ParseRemoteURL turns a remote URL, as `git remote -v` prints it, into the
// repository's web location. It accepts the https, ssh:// and scp-like
// ("git@host:owner/repo.git") forms, keeps a port named by an http(s) URL, and
// strips credentials: a token in the remote URL must not end up in a browser
// address. A URL it cannot read is an error, never a panic.
func ParseRemoteURL(raw string) (RemoteLink, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return RemoteLink{}, remoteURLError("the URL is empty")
	}
	var (
		host, path, scheme string
	)
	if i := strings.Index(s, "://"); i > 0 {
		u, err := url.Parse(s)
		if err != nil {
			return RemoteLink{}, remoteURLError("%q is not a URL", raw)
		}
		if u.Host == "" {
			return RemoteLink{}, remoteURLError("%q names no host", raw)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			// The port of an http(s) remote is the port of its web pages.
			scheme, host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
		case "ssh", "ssh+git", "git+ssh", "git":
			// The port of an ssh remote is the ssh port, not a web port: the
			// pages live on 443 unless the user's browser knows better.
			scheme, host = "https", hostOnly(u.Hostname())
		default:
			return RemoteLink{}, remoteURLError("%s remotes have no web page", u.Scheme)
		}
		path = u.Path
	} else if i := strings.IndexByte(s, ':'); i > 0 && !strings.ContainsAny(s[:i], `/\`) {
		// scp-like: [user@]host:path
		host = hostOnly(s[:i])
		path = s[i+1:]
	} else {
		return RemoteLink{}, remoteURLError("%q is a local path", raw)
	}
	p, err := projectPath(path)
	if err != nil {
		return RemoteLink{}, remoteURLError("%q has no owner/repo path", raw)
	}
	host = strings.ToLower(host)
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		// An IPv6 literal from an ssh or scp remote has no brackets yet; a URL
		// needs them.
		host = "[" + host + "]"
	}
	if host == "" {
		return RemoteLink{}, remoteURLError("%q names no host", raw)
	}
	link := RemoteLink{Forge: forgeOf(host), Host: host, Path: p, scheme: scheme}
	return link, nil
}

// hostOnly drops the user and the port from "[user@]host[:port]".
func hostOnly(s string) string {
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

// projectPath normalises the path part of a remote URL: no leading or trailing
// slashes, no ".git" suffix, at least owner and repository, and no empty or
// relative segments.
func projectPath(p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if strings.HasSuffix(strings.ToLower(p), ".git") {
		p = p[:len(p)-len(".git")]
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "", errBadProjectPath
	}
	segments := strings.Split(p, "/")
	if len(segments) < 2 {
		return "", errBadProjectPath
	}
	for _, seg := range segments {
		switch seg {
		case "", ".", "..":
			return "", errBadProjectPath
		}
	}
	return strings.Join(segments, "/"), nil
}

var errBadProjectPath = errors.New("no owner/repo path")

// forgeOf names the service behind a host. A host is recognised by its exact
// name or by a name that carries the product's name, which is how self-hosted
// instances are usually named (gitlab.example.com, gitea.example.com); anything
// else stays unknown instead of being guessed.
func forgeOf(host string) Forge {
	h := strings.ToLower(host)
	if name, _, err := net.SplitHostPort(h); err == nil {
		h = name
	}
	h = strings.Trim(h, "[]")
	switch {
	case h == "github.com" || strings.Contains(h, "github"):
		return ForgeGitHub
	case h == "gitlab.com" || strings.Contains(h, "gitlab"):
		return ForgeGitLab
	case h == "bitbucket.org" || strings.Contains(h, "bitbucket"):
		return ForgeBitbucket
	case h == "codeberg.org" || h == "gitea.com" ||
		strings.Contains(h, "gitea") || strings.Contains(h, "forgejo"):
		return ForgeGitea
	}
	return ForgeUnknown
}

// Label names the service in a menu entry: "GitHub", or the host itself for a
// service Viff does not recognise.
func (l RemoteLink) Label() string {
	if l.Forge != ForgeUnknown {
		return string(l.Forge)
	}
	return l.Host
}

// WebBase is the scheme and host of the repository's pages, "" when the link
// is empty.
func (l RemoteLink) WebBase() string {
	if l.Host == "" {
		return ""
	}
	scheme := l.scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + l.Host
}

// RepoURL is the repository's page, the one address every service has.
func (l RemoteLink) RepoURL() string {
	if l.WebBase() == "" || l.Path == "" {
		return ""
	}
	return l.WebBase() + "/" + escapePath(l.Path)
}

// CommitURL is the page of one commit.
func (l RemoteLink) CommitURL(hash string) (string, error) {
	if l.RepoURL() == "" {
		return "", ErrRemoteURL
	}
	if hash = strings.TrimSpace(hash); hash == "" {
		return "", errors.New("no commit to link to")
	}
	switch l.Forge {
	case ForgeGitLab:
		return l.RepoURL() + "/-/commit/" + escapeRef(hash), nil
	case ForgeBitbucket:
		return l.RepoURL() + "/commits/" + escapeRef(hash), nil
	}
	// GitHub and Gitea share this layout, and so does an unknown host.
	return l.RepoURL() + "/commit/" + escapeRef(hash), nil
}

// BranchURL is the page listing a branch's files.
func (l RemoteLink) BranchURL(branch string) (string, error) {
	if l.RepoURL() == "" {
		return "", ErrRemoteURL
	}
	if branch = strings.TrimSpace(branch); branch == "" {
		return "", errors.New("no branch to link to")
	}
	switch l.Forge {
	case ForgeGitLab:
		return l.RepoURL() + "/-/tree/" + escapeRef(branch), nil
	case ForgeBitbucket:
		return l.RepoURL() + "/branch/" + escapeRef(branch), nil
	case ForgeGitea:
		return l.RepoURL() + "/src/branch/" + escapeRef(branch), nil
	}
	return l.RepoURL() + "/tree/" + escapeRef(branch), nil
}

// CompareURL lists the commits head has that base does not. Bitbucket has no
// comparison page Viff can be sure of, so it reports ErrLinkUnsupported and the
// caller opens the branch page instead.
func (l RemoteLink) CompareURL(base, head string) (string, error) {
	if err := l.checkBranches(base, head); err != nil {
		return "", err
	}
	switch l.Forge {
	case ForgeGitLab:
		return l.RepoURL() + "/-/compare/" + escapeRef(base) + "..." + escapeRef(head), nil
	case ForgeBitbucket:
		return "", fmt.Errorf("%w: Bitbucket comparison pages", ErrLinkUnsupported)
	}
	return l.RepoURL() + "/compare/" + escapeRef(base) + "..." + escapeRef(head), nil
}

// NewPullRequestURL is the service page that creates a pull request (a merge
// request on GitLab) from head into base. Bitbucket reports
// ErrLinkUnsupported for the same reason CompareURL does.
func (l RemoteLink) NewPullRequestURL(base, head string) (string, error) {
	if err := l.checkBranches(base, head); err != nil {
		return "", err
	}
	switch l.Forge {
	case ForgeGitLab:
		q := url.Values{}
		q.Set("merge_request[source_branch]", head)
		q.Set("merge_request[target_branch]", base)
		return l.RepoURL() + "/-/merge_requests/new?" + q.Encode(), nil
	case ForgeBitbucket:
		return "", fmt.Errorf("%w: Bitbucket pull request pages", ErrLinkUnsupported)
	}
	return l.RepoURL() + "/compare/" + escapeRef(base) + "..." + escapeRef(head) + "?expand=1", nil
}

// checkBranches rejects the pairs no page can describe: an empty branch, the
// same branch on both sides, and a link with no repository behind it.
func (l RemoteLink) checkBranches(base, head string) error {
	if l.RepoURL() == "" {
		return ErrRemoteURL
	}
	base, head = strings.TrimSpace(base), strings.TrimSpace(head)
	if base == "" || head == "" {
		return errors.New("no branch to compare")
	}
	if base == head {
		return fmt.Errorf("the branch %q cannot be compared with itself", base)
	}
	return nil
}

// escapePath escapes one embedded path such as "group/subgroup/repo" segment by
// segment, keeping the slashes that make it a path.
func escapePath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// escapeRef escapes a branch name or object ID. Git allows "/" in branch names
// and the services resolve it inside the path, so only the segments are
// escaped; escaping the slashes would name a different branch.
func escapeRef(ref string) string { return escapePath(ref) }

// RemoteLink resolves a remote to its web location. fetchURL and pushURL are
// the remote's URLs as Git prints them; an http(s) fetch URL wins because that
// is the address the browser uses, and a remote without a URL is an error.
func (rs *RepoState) RemoteLink(name string) (RemoteLink, error) {
	for _, r := range rs.Remotes {
		if r.Name != name {
			continue
		}
		if raw := strings.TrimSpace(r.FetchURL); raw != "" {
			return ParseRemoteURL(raw)
		}
		if raw := strings.TrimSpace(r.PushURL); raw != "" {
			return ParseRemoteURL(raw)
		}
		return RemoteLink{}, remoteURLError("the remote %s has no URL", name)
	}
	if len(rs.Remotes) == 0 {
		return RemoteLink{}, remoteURLError("the repository has no remote")
	}
	return RemoteLink{}, remoteURLError("there is no remote named %s", name)
}

// BranchRemote is the remote a branch works with: the remote of its upstream,
// else "origin", else the first one. "" when the repository has no remote.
func (rs *RepoState) BranchRemote(branch string) string {
	if ref, ok := rs.localBranch(branch); ok && ref.Upstream != "" {
		if i := strings.IndexByte(ref.Upstream, '/'); i > 0 {
			return ref.Upstream[:i]
		}
	}
	for _, r := range rs.Remotes {
		if r.Name == "origin" {
			return r.Name
		}
	}
	if len(rs.Remotes) > 0 {
		return rs.Remotes[0].Name
	}
	return ""
}

// LinkFor is the link of the remote a branch works with. branch may be "" when
// the caller only needs the repository, as a commit page does.
func (rs *RepoState) LinkFor(branch string) (RemoteLink, error) {
	name := rs.BranchRemote(branch)
	if name == "" {
		return RemoteLink{}, remoteURLError("the repository has no remote")
	}
	return rs.RemoteLink(name)
}

// BaseBranch is the branch a comparison or a pull request from branch targets:
// the branch's own upstream without the remote prefix, else the default branch
// of its remote (refs/remotes/<remote>/HEAD), else main, else master. A branch
// is never its own base: comparing main with main is a page of nothing, so that
// candidate falls through. ok is false when none of them is known, and the
// caller asks the user instead of guessing.
func (rs *RepoState) BaseBranch(branch string) (string, bool) {
	remote := rs.BranchRemote(branch)
	usable := func(name string) bool { return name != "" && name != branch }
	if ref, ok := rs.localBranch(branch); ok && ref.Upstream != "" {
		if base := upstreamBranch(ref.Upstream); usable(base) {
			return base, true
		}
	}
	if base := rs.RemoteHeads[remote]; usable(base) {
		return base, true
	}
	for _, name := range []string{"main", "master"} {
		if !usable(name) {
			continue
		}
		// A remote-tracking branch is what the pull request will target; the
		// local branch of the same name usually follows it.
		if remote != "" && rs.hasRemoteBranch(remote+"/"+name) {
			return name, true
		}
		if _, ok := rs.localBranch(name); ok {
			return name, true
		}
	}
	return "", false
}

// localBranch finds a local branch by its short name.
func (rs *RepoState) localBranch(name string) (domain.Ref, bool) {
	if name == "" {
		return domain.Ref{}, false
	}
	for _, r := range rs.Refs {
		if r.Kind == domain.RefLocal && r.Short == name {
			return r, true
		}
	}
	return domain.Ref{}, false
}

// hasRemoteBranch reports a remote-tracking branch by its short name, which is
// "<remote>/<branch>".
func (rs *RepoState) hasRemoteBranch(short string) bool {
	for _, r := range rs.Refs {
		if r.Kind == domain.RefRemote && r.Short == short {
			return true
		}
	}
	return false
}

// upstreamBranch drops the remote prefix from an upstream name: "origin/main"
// becomes "main". A name without a prefix is returned as is.
func upstreamBranch(upstream string) string {
	if i := strings.IndexByte(upstream, '/'); i > 0 {
		return upstream[i+1:]
	}
	return upstream
}
