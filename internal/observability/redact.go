package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Redaction selects what standard redaction keeps. The zero value is the
// default "standard" profile: paths outside the repository become placeholders
// and remote URLs keep only their scheme and host.
//
// The optional extras exist for the settings UI and future profiles; the
// logger always runs the standard profile (DefaultRedaction).
type Redaction struct {
	// IncludeRepoName keeps the repository directory basename after the
	// repository hash (strict profiles drop it).
	IncludeRepoName bool
	// IncludeRemoteURL keeps a remote URL's path component. Userinfo and
	// query strings are always removed.
	IncludeRemoteURL bool
	// IncludeArgs keeps whitelisted Git arguments instead of dropping them
	// entirely.
	IncludeArgs bool
}

// DefaultRedaction returns the standard profile: everything that could
// identify a machine path, credential, or private repository is replaced.
func DefaultRedaction() Redaction {
	return Redaction{}
}

// Placeholders written by redaction. They are stable so tests and docs can
// match on them.
const (
	redactedValue   = "***"
	placeholderRepo = "<repo>"
	placeholderPath = "<path>"
	placeholderHome = "~"
)

var (
	// rePrivateKey matches a complete PEM private-key block. [\s\S] rather
	// than (?s) keeps the expression simple to read.
	rePrivateKey = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	// rePrivateKeyOpen catches a truncated block: a BEGIN marker without a
	// matching END, where everything after it is key material.
	rePrivateKeyOpen = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*`)

	// tokenPatterns are the credential shapes from docs/observability.md §4.
	tokenPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{16,}\b`),    // GitHub tokens
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),  // GitHub fine-grained PAT
		regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{16,}\b`),     // GitLab PAT
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),              // AWS access key
		regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9\-]{10,}\b`), // Slack tokens
	}
	// reCredentialKeyValue matches "token=...", "password: ..." and friends.
	reCredentialKeyValue = regexp.MustCompile(`(?i)(token|password|passwd|secret|apikey|api_key|authorization)\s*[=:]\s*\S+`)
	// reURL matches an absolute URL. The trailing class is deliberately
	// broad so a token embedded in a query is removed with the URL.
	reURL = regexp.MustCompile("[A-Za-z][A-Za-z0-9+.\\-]*://[^\\s\"'<>`]+")
	// reSCPLike matches the scp-style remote form git@host:org/repo.git,
	// which carries the same private repository path as an https remote but
	// is not a URL. The mandatory colon after the host keeps email addresses
	// untouched.
	reSCPLike = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@([A-Za-z0-9.\-]+\.[A-Za-z]{2,}):[^\s"'<>]+`)

	// pathSegmentChars matches one character of an absolute path component:
	// anything but whitespace, quotes, angle brackets, separators, and
	// characters that cannot appear in a path. Unicode is allowed so non-ASCII
	// directories do not leak.
	pathSegmentChars = `[^\x00-\x20"'<>|/\\:*?]`
	// pathSegment is one or more such characters.
	pathSegment = pathSegmentChars + `+`
	// reWindowsAbsPath matches C:\Users\... and C:/Users/... .
	reWindowsAbsPath = regexp.MustCompile(`(?i)\b[A-Za-z]:[\\/](?:` + pathSegment + `[\\/])*` + pathSegmentChars + `*`)
	// reUnixAbsPath matches /Users/..., /home/... and other absolute POSIX
	// paths. The leading group preserves the character before the path (a
	// space, quote, parenthesis, ...) so the surrounding text survives; a '>'
	// is not such a boundary, which keeps a previously inserted "<repo>"
	// placeholder from being redacted again.
	reUnixAbsPath = regexp.MustCompile(`(^|[\s"'(=:,;\[\]{}|&])((?:/` + pathSegment + `)+)`)
)

// Redact returns s with credentials, remote-URL paths, home directories and
// absolute paths replaced by stable placeholders. It is the single entry point
// for redaction: the logger, crash reports, and diagnostic bundles all go
// through it, so a new rule only has to be added here.
func Redact(s string, r Redaction) string {
	if s == "" {
		return s
	}
	s = rePrivateKey.ReplaceAllString(s, redactedValue)
	s = rePrivateKeyOpen.ReplaceAllString(s, redactedValue)
	for _, re := range tokenPatterns {
		s = re.ReplaceAllString(s, redactedValue)
	}
	s = reCredentialKeyValue.ReplaceAllString(s, "${1}="+redactedValue)
	s = reSCPLike.ReplaceAllString(s, placeholderPath)
	s = redactURLs(s, r)
	s = redactHomeDir(s)
	s = reWindowsAbsPath.ReplaceAllString(s, placeholderPath)
	s = reUnixAbsPath.ReplaceAllString(s, "${1}"+placeholderPath)
	return s
}

// RedactPath redacts a path that belongs to the repository at root: the root
// prefix becomes "<repo>" and the rest of the path stays relative. Paths
// outside root are redacted like any other absolute path. An already relative
// path is returned unchanged (standard profiles keep repository-relative
// paths, which are required to debug).
func RedactPath(root, s string) string {
	if s == "" {
		return s
	}
	if root = strings.TrimSpace(root); root != "" {
		s = replacePathPrefix(s, filepath.Clean(root), placeholderRepo)
	}
	return Redact(s, DefaultRedaction())
}

// RepoHash returns a stable 8-hex-digit identity for a repository root. Logs
// and bundles only ever contain this hash, never the absolute path. An empty
// root hashes to "" so records can omit the field.
func RepoHash(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:])[:8]
}

// redactURLs keeps only scheme and host (plus the path when the caller opted
// in), dropping userinfo, query strings and fragments.
func redactURLs(s string, r Redaction) string {
	return reURL.ReplaceAllStringFunc(s, func(match string) string {
		sep := strings.Index(match, "://")
		if sep < 0 {
			return match
		}
		scheme := match[:sep]
		rest := match[sep+3:]

		host := rest
		if i := strings.IndexAny(host, "/?#"); i >= 0 {
			host = host[:i]
		}
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		if host == "" {
			return scheme + "://"
		}
		if !r.IncludeRemoteURL {
			return scheme + "://" + host
		}
		path := ""
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			path = rest[i:]
		}
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
		return scheme + "://" + host + path
	})
}

// redactHomeDir replaces the current user's home directory prefix with "~".
func redactHomeDir(s string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	home = filepath.Clean(home)
	if home == "" || home == "." || home == string(filepath.Separator) {
		return s
	}
	// os.UserHomeDir returns the OS-native form; also handle the other
	// separator so a log line copied between platforms still redacts.
	s = replacePathPrefix(s, home, placeholderHome)
	if alt := filepath.ToSlash(home); alt != home {
		s = replacePathPrefix(s, alt, placeholderHome)
	}
	if alt := filepath.FromSlash(home); alt != home {
		s = replacePathPrefix(s, alt, placeholderHome)
	}
	return s
}

// replacePathPrefix replaces occurrences of prefix that form a whole path
// token (not a prefix of a longer name) with repl.
func replacePathPrefix(s, prefix, repl string) string {
	if prefix == "" || prefix == "." || prefix == string(filepath.Separator) {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, prefix)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := i + len(prefix)
		if pathBoundaryBefore(s, i) && pathBoundaryAfter(s, end) {
			b.WriteString(s[:i])
			b.WriteString(repl)
			s = s[end:]
			continue
		}
		// Not a whole path token: keep this occurrence and continue after it.
		b.WriteString(s[:end])
		s = s[end:]
	}
}

// pathBoundaryBefore reports whether index i starts a path token.
func pathBoundaryBefore(s string, i int) bool {
	if i == 0 {
		return true
	}
	return !isPathNameByte(s[i-1])
}

// pathBoundaryAfter reports whether index i ends a path token.
func pathBoundaryAfter(s string, i int) bool {
	if i == len(s) {
		return true
	}
	c := s[i]
	if c == '/' || c == '\\' {
		return true
	}
	return !isPathNameByte(c)
}

// isPathNameByte reports whether c can continue a file name. A prefix followed
// by one of these characters is a longer name, not the same path.
func isPathNameByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '_', c == '-', c == '.', c == '@', c == '+', c == '%', c == '~':
		return true
	case c >= 0x80:
		return true
	}
	return false
}
