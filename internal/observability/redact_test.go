package observability

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mustNotContain fails the test when got still contains a secret or a machine
// path. Every redaction case asserts on the absence of the sensitive input,
// which is the property that actually matters.
func mustNotContain(t *testing.T, got string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(got, secret) {
			t.Fatalf("redacted output still contains %q:\n%s", secret, got)
		}
	}
}

func TestRedactURLs(t *testing.T) {
	r := DefaultRedaction()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "userinfo and path removed",
			in:   "https://user:ghp_0123456789abcdefghij@github.com/org/repo.git",
			want: "https://github.com",
		},
		{
			name: "path removed",
			in:   "https://github.com/org/private-repo.git",
			want: "https://github.com",
		},
		{
			name: "query removed",
			in:   "https://example.com/api?token=abc123&x=1",
			want: "https://example.com",
		},
		{
			name: "port kept",
			in:   "http://localhost:8080/api/repos",
			want: "http://localhost:8080",
		},
		{
			name: "inside sentence",
			in:   "clone from https://example.com/org/repo please",
			want: "clone from https://example.com please",
		},
		{
			name: "credentials in userinfo removed",
			in:   "https://bob:hunter2@gitlab.com/group/project",
			want: "https://gitlab.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.in, r)
			if got != tc.want {
				t.Fatalf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for _, secret := range []string{"user", "ghp_", "hunter2", "org/", "/repo"} {
				if secret == "user" && tc.name == "inside sentence" {
					continue
				}
				mustNotContain(t, got, secret)
			}
		})
	}
}

func TestRedactIncludeRemoteURL(t *testing.T) {
	r := DefaultRedaction()
	r.IncludeRemoteURL = true
	got := Redact("https://user:ghp_0123456789abcdefghij@github.com/org/repo.git", r)
	if got != "https://github.com/org/repo.git" {
		t.Fatalf("IncludeRemoteURL output = %q", got)
	}
	mustNotContain(t, got, "user", "ghp_")
}

func TestRedactTokens(t *testing.T) {
	r := DefaultRedaction()
	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{"github pat", "token is ghp_0123456789abcdefghij", "ghp_0123456789abcdefghij"},
		{"github oauth", "gho_0123456789abcdefghij", "gho_0123456789abcdefghij"},
		{"github refresh", "ghr_0123456789abcdefghij", "ghr_0123456789abcdefghij"},
		{"github server", "ghs_0123456789abcdefghij", "ghs_0123456789abcdefghij"},
		{"github user", "ghu_0123456789abcdefghij", "ghu_0123456789abcdefghij"},
		{
			"github fine grained",
			"github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz0123456789",
			"github_pat_11ABCDEFG0123456789_",
		},
		{"gitlab pat", "glpat-abcdefghijklmnopqrst", "glpat-abcdefghijklmnopqrst"},
		{"aws key", "AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"slack bot", "xoxb-123456789012-abcdefghijkl", "xoxb-123456789012-abcdefghijkl"},
		{"slack user", "xoxp-123456789012-abcdefghijkl", "xoxp-123456789012-abcdefghijkl"},
		{"generic token kv", "token=supersecret123", "supersecret123"},
		{"password kv", "password: hunter2pass", "hunter2pass"},
		{"api key kv", "apiKey=abcdef123456", "abcdef123456"},
		{"authorization kv", "authorization=Bearerabcdef", "Bearerabcdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.in, r)
			mustNotContain(t, got, tc.secret)
			if !strings.Contains(got, redactedValue) {
				t.Fatalf("Redact(%q) = %q, expected placeholder %q", tc.in, got, redactedValue)
			}
		})
	}
}

func TestRedactPrivateKey(t *testing.T) {
	r := DefaultRedaction()
	key := "-----BEGIN RSA PRIVATE KEY-----\n" +
		"MIIBogIBAAKBgQD01yJ7Hh5k\n" +
		"abcdEFGH1234567890\n" +
		"-----END RSA PRIVATE KEY-----"
	got := Redact("before "+key+" after", r)
	mustNotContain(t, got, "MIIBog", "PRIVATE KEY", "abcdEFGH")
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("surrounding text lost: %q", got)
	}

	// An unterminated block is key material too: everything after the BEGIN
	// marker goes away.
	truncated := "-----BEGIN OPENSSH PRIVATE KEY-----\nsecretmaterial"
	if got := Redact(truncated, r); strings.Contains(got, "secretmaterial") {
		t.Fatalf("unterminated private key not redacted: %q", got)
	}
}

func TestRedactPaths(t *testing.T) {
	r := DefaultRedaction()
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"absolute tmp path", "open /tmp/viff-test/file.go now", "open <path> now"},
		{"absolute var path", "/var/log/system.log", "<path>"},
		{"absolute path at start", "/etc/passwd", "<path>"},
		{"windows backslash", `C:\Users\bob\secret.txt`, "<path>"},
		{"windows forward slash", `C:/Users/bob/secret.txt`, "<path>"},
		{"relative path untouched", "src/main.go", "src/main.go"},
		{"url host survives", "https://github.com/org/repo", "https://github.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.in, r)
			if got != tc.want {
				t.Fatalf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	if home != "" && home != "/" {
		t.Run("home directory", func(t *testing.T) {
			in := "config at " + filepath.Join(home, "work", "viff") + " and done"
			got := Redact(in, r)
			mustNotContain(t, got, home)
			if !strings.Contains(got, "~") {
				t.Fatalf("home directory not replaced with ~: %q", got)
			}
		})
	}
}

func TestRedactNoFalsePositives(t *testing.T) {
	r := DefaultRedaction()
	cases := []string{
		"user@example.com",
		"00:1A:2B:3C:4D:5E",
		"and/or",
		"2026/11/04",
		"src/main.go",
		"refs/heads/main",
		"goroutine 12 [running]:",
		"exit status 128",
	}
	for _, in := range cases {
		if got := Redact(in, r); got != in {
			t.Fatalf("Redact(%q) changed harmless text to %q", in, got)
		}
	}
}

func TestRedactPath(t *testing.T) {
	root := filepath.Join(os.TempDir(), "viff-redact-repo")
	r := DefaultRedaction()
	_ = r

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"inside repo absolute", filepath.Join(root, "src", "main.go"), "<repo>/src/main.go"},
		{"repo root", root, "<repo>"},
		{"outside repo", "/etc/hosts", "<path>"},
		{"relative stays", "src/main.go", "src/main.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactPath(root, tc.in)
			if got != tc.want {
				t.Fatalf("RedactPath(%q, %q) = %q, want %q", root, tc.in, got, tc.want)
			}
			mustNotContain(t, got, root, "/etc/hosts")
		})
	}

	// A sibling with the repo name as a prefix is not inside the repo.
	sibling := root + "-other/file.go"
	got := RedactPath(root, sibling)
	mustNotContain(t, got, sibling)
}

func TestRepoHash(t *testing.T) {
	root := filepath.Join(os.TempDir(), "viff-hash-repo")
	hash := RepoHash(root)
	if len(hash) != 8 {
		t.Fatalf("RepoHash length = %d, want 8 (%q)", len(hash), hash)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(hash) {
		t.Fatalf("RepoHash = %q, want lowercase hex", hash)
	}
	if again := RepoHash(root); again != hash {
		t.Fatalf("RepoHash is not deterministic: %q vs %q", hash, again)
	}
	if withSlash := RepoHash(root + string(filepath.Separator)); withSlash != hash {
		t.Fatalf("RepoHash should clean the path: %q vs %q", hash, withSlash)
	}
	if other := RepoHash(root + "-other"); other == hash {
		t.Fatalf("different roots hashed identically: %q", hash)
	}
	if RepoHash("") != "" {
		t.Fatalf("empty root should hash to the empty string")
	}
}

func TestRedactCombinedSentence(t *testing.T) {
	r := DefaultRedaction()
	home, _ := os.UserHomeDir()
	repo := filepath.Join(home, "code", "private-repo")
	in := "push https://bob:ghp_0123456789abcdefghij@github.com/org/repo.git failed; " +
		"repo=" + repo + "; token=abcd1234; log=/var/log/viff.log"
	got := Redact(in, r)
	mustNotContain(t, got, "bob", "ghp_", "hunter", repo, "/var/log", "abcd1234")
	if !strings.Contains(got, "https://github.com") {
		t.Fatalf("host lost: %q", got)
	}
}
