// Package sshpath parses and formats the identity of a repository Viff opens
// on another machine over SSH: ssh://user@host:port/absolute/path.
//
// The path is kept raw (spaces and non-ASCII bytes stay as typed) because it
// is an identity, not a URL that is handed to a browser. Only the authority
// has a fixed grammar: an optional user, a host (an IPv6 literal in brackets)
// and an optional port. Everything is lexical: this package starts no process
// and touches no file system.
package sshpath

import (
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"
	"unicode"
)

// Scheme is the prefix that marks a remote repository.
const Scheme = "ssh://"

// Repo is one repository on a remote host. Port 0 means the ssh default.
type Repo struct {
	User string
	Host string
	Port int
	Path string
}

// Is reports whether s names an SSH repository.
func Is(s string) bool { return strings.HasPrefix(s, Scheme) }

// Parse reads an ssh:// URL. The path must be absolute and is otherwise kept
// verbatim, so a repository below a directory with spaces or non-ASCII bytes
// round-trips.
func Parse(s string) (Repo, error) {
	if !Is(s) {
		return Repo{}, fmt.Errorf("%q is not an SSH repository URL", s)
	}
	rest := strings.TrimPrefix(s, Scheme)
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return Repo{}, fmt.Errorf("SSH repository URL %q has no path", s)
	}
	authority, p := rest[:slash], rest[slash:]
	user, host, port, err := parseAuthority(authority)
	if err != nil {
		return Repo{}, err
	}
	if err := ValidatePath(p); err != nil {
		return Repo{}, err
	}
	return Repo{User: user, Host: host, Port: port, Path: p}, nil
}

// String renders the canonical identity. It is the value stored in recent
// projects, write locks and caches, so it must be stable: the host is
// lowercased, the default port is omitted and the path is cleaned.
func (r Repo) String() string {
	raw := strings.ToLower(r.Host)
	host := raw
	if strings.Contains(raw, ":") {
		host = "[" + raw + "]"
	}
	authority := host
	if r.Port != 0 && r.Port != 22 {
		authority = net.JoinHostPort(raw, strconv.Itoa(r.Port))
	}
	if r.User != "" {
		authority = r.User + "@" + authority
	}
	return Scheme + authority + cleanPath(r.Path)
}

// Display is the copy-friendly spelling: user@host:path. A non-default port
// keeps the full ssh:// URL, because scp-style spelling cannot carry a port
// without ambiguity.
func (r Repo) Display() string {
	if r.Port != 0 && r.Port != 22 {
		return r.String()
	}
	host := r.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if r.User != "" {
		host = r.User + "@" + host
	}
	return host + ":" + r.Path
}

// HostInput turns one dialog field such as "dev", "me@10.0.0.5:2222" or
// "me@[::1]:22" into an authority. It is separate from Parse because the user
// types a host, not a URL.
func HostInput(s string) (user, host string, port int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", 0, fmt.Errorf("the host is empty")
	}
	if strings.ContainsAny(s, "\x00\r\n\t") {
		return "", "", 0, fmt.Errorf("the host contains a control character")
	}
	if at := strings.LastIndexByte(s, '@'); at >= 0 {
		user, s = s[:at], s[at+1:]
		if user == "" {
			return "", "", 0, fmt.Errorf("the user before @ is empty")
		}
		if err := validateUser(user); err != nil {
			return "", "", 0, err
		}
	}
	host, port, err = splitHostPort(s)
	if err != nil {
		return "", "", 0, err
	}
	if err := validateHost(host); err != nil {
		return "", "", 0, err
	}
	return user, host, port, nil
}

// URL builds the identity from the two dialog fields and validates the path.
func URL(hostInput, remotePath string) (string, error) {
	user, host, port, err := HostInput(hostInput)
	if err != nil {
		return "", err
	}
	p := strings.TrimRight(remotePath, "/")
	if p == "" {
		if remotePath != "/" {
			return "", fmt.Errorf("the path is empty")
		}
		p = "/"
	}
	if err := ValidatePath(p); err != nil {
		return "", err
	}
	return Repo{User: user, Host: host, Port: port, Path: cleanPath(p)}.String(), nil
}

// ValidatePath checks that p can be an absolute path on the server. Control
// characters are refused because the value becomes a command-line argument and
// a log field; spaces are allowed and quoted later. ".." is refused so a path
// typed into the dialog cannot silently address a different directory than the
// one shown.
func ValidatePath(p string) error {
	if p == "" {
		return fmt.Errorf("the path is empty")
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return fmt.Errorf("the path contains a control character")
		}
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("the path must be absolute on the server, for example /srv/app")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("the path must not contain '..'")
		}
	}
	if cleanPath(p) == "" {
		return fmt.Errorf("the path is empty")
	}
	return nil
}

// Normalize canonicalizes s when it is an SSH URL. The bool is false for
// every other input, including local paths and WSL paths.
func Normalize(s string) (string, bool) {
	if !Is(s) {
		return "", false
	}
	r, err := Parse(s)
	if err != nil {
		return "", false
	}
	return r.String(), true
}

// URLOf maps an absolute path reported by Git on the server back to the
// identity domain. base supplies the user, host and port. A Windows path from
// a test transport is turned into "/C:/..." so it is still an absolute path;
// real servers report POSIX paths.
func URLOf(base, abs string) (string, error) {
	r, err := Parse(base)
	if err != nil {
		return "", err
	}
	p := strings.ReplaceAll(abs, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if err := ValidatePath(p); err != nil {
		return "", err
	}
	r.Path = cleanPath(p)
	return r.String(), nil
}

// GitPath returns the path a Git process on the server can open for the
// repository identity root.
func GitPath(root string) (string, error) {
	r, err := Parse(root)
	if err != nil {
		return "", err
	}
	return r.Path, nil
}

// GitPathFrom converts one path argument into the spelling a remote Git
// understands. A path that is already remote (same host) or relative passes
// through; a local absolute path is refused because the server cannot see it.
func GitPathFrom(root, p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if Is(p) {
		other, err := Parse(p)
		if err != nil {
			return "", err
		}
		mine, err := Parse(root)
		if err != nil {
			return "", err
		}
		if !sameAuthority(mine, other) {
			return "", fmt.Errorf("%s is on another host than %s", other.Display(), mine.Display())
		}
		return other.Path, nil
	}
	if strings.HasPrefix(p, "/") {
		return p, nil
	}
	if isLocalAbsolute(p) {
		return "", fmt.Errorf("%s is a local path; a remote repository needs a path on the server", p)
	}
	return p, nil
}

// AbsPath joins a repository-relative Git path to the remote root. It refuses
// an absolute path, NUL, and a path that would leave the repository, so a
// value from the interface can never address another directory on the server.
func AbsPath(root, rel string) (string, error) {
	r, err := Parse(root)
	if err != nil {
		return "", err
	}
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsRune(rel, '\x00') {
		return "", fmt.Errorf("%q is not a repository-relative path", rel)
	}
	joined := path.Join(r.Path, rel)
	if joined == r.Path || !strings.HasPrefix(joined, strings.TrimRight(r.Path, "/")+"/") {
		return "", fmt.Errorf("%q is outside the repository", rel)
	}
	return joined, nil
}

// DisplayPath is the copy-friendly absolute location of a Git path:
// user@host:/srv/app/file.
func DisplayPath(root, rel string) string {
	r, err := Parse(root)
	if err != nil {
		return root
	}
	p := r.Path
	if rel != "" {
		p = path.Join(p, rel)
	}
	d := r
	d.Path = p
	return d.Display()
}

// Name is the repository's display name: its last path segment, or the host
// for the server root.
func Name(root string) string {
	r, err := Parse(root)
	if err != nil {
		return path.Base(strings.TrimRight(root, "/"))
	}
	p := strings.TrimRight(r.Path, "/")
	if p == "" || p == "/" {
		return r.Host
	}
	return path.Base(p)
}

// ParseAuthority is exported for tests of the authority grammar.
func ParseAuthority(s string) (user, host string, port int, err error) { return parseAuthority(s) }

func parseAuthority(authority string) (user, host string, port int, err error) {
	if authority == "" {
		return "", "", 0, fmt.Errorf("the SSH repository URL has no host")
	}
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		user, authority = authority[:at], authority[at+1:]
		if user == "" {
			return "", "", 0, fmt.Errorf("the user before @ is empty")
		}
		if err := validateUser(user); err != nil {
			return "", "", 0, err
		}
	}
	host, port, err = splitHostPort(authority)
	if err != nil {
		return "", "", 0, err
	}
	if err := validateHost(host); err != nil {
		return "", "", 0, err
	}
	return user, strings.ToLower(host), port, nil
}

func splitHostPort(s string) (host string, port int, err error) {
	if s == "" {
		return "", 0, fmt.Errorf("the host is empty")
	}
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return "", 0, fmt.Errorf("the IPv6 address has no closing bracket")
		}
		host = s[1:end]
		rest := s[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return "", 0, fmt.Errorf("unexpected text after the IPv6 address")
			}
			port, err = parsePort(rest[1:])
			if err != nil {
				return "", 0, err
			}
		}
		return host, port, nil
	}
	if strings.Count(s, ":") > 1 {
		return "", 0, fmt.Errorf("an IPv6 address must be written in [brackets]")
	}
	if colon := strings.LastIndexByte(s, ':'); colon >= 0 {
		host = s[:colon]
		port, err = parsePort(s[colon+1:])
		if err != nil {
			return "", 0, err
		}
		if host == "" {
			return "", 0, fmt.Errorf("the host is empty")
		}
		return host, port, nil
	}
	return s, 0, nil
}

func parsePort(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("the port is empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("the port %q is not a number between 1 and 65535", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("the port %q is not a number between 1 and 65535", s)
	}
	if n == 22 {
		return 22, nil
	}
	return n, nil
}

func validateUser(s string) error {
	if s == "" {
		return fmt.Errorf("the user is empty")
	}
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("the user must not start with '-'")
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '/' || r == '@' || r == ':' {
			return fmt.Errorf("the user contains an invalid character")
		}
	}
	return nil
}

func validateHost(s string) error {
	if s == "" {
		return fmt.Errorf("the host is empty")
	}
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("the host must not start with '-'")
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '/' || r == '@' || r == '\\' {
			return fmt.Errorf("the host contains an invalid character")
		}
	}
	// A host alias is whatever ssh accepts; only the shell-sensitive and
	// structurally impossible bytes are refused above. Ports, IPv6 literals
	// and config aliases are all valid here.
	return nil
}

// cleanPath keeps the root "/" and drops a trailing slash. It deliberately
// does not resolve "..": a Git path never contains one, and an identity typed
// by the user should not silently point somewhere else.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func sameAuthority(a, b Repo) bool {
	return strings.EqualFold(a.Host, b.Host) && a.User == b.User && a.Port == b.Port
}

func isLocalAbsolute(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return true
	}
	return false
}

// Display is Repo.Display for a URL. It is the copy-friendly spelling used by
// the interface; an invalid value is returned unchanged.
func Display(root string) string {
	r, err := Parse(root)
	if err != nil {
		return root
	}
	return r.Display()
}
