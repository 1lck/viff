package platform

import (
	"strings"
	"testing"
)

// OpenURL refuses every address that is not http(s) before it starts a process,
// so these cases never open a browser or a file manager. The accepted path is
// deliberately not covered here: running it would open a real window on the
// machine that runs the suite, and the per-platform command is a one-liner a
// reviewer can read. webURL is the check, and it is covered on its own below.
func TestOpenURLRefusesOtherSchemes(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"file", "file:///etc/passwd", "not an http(s) address"},
		{"script", "javascript:alert(1)", "not an http(s) address"},
		{"custom scheme", "vscode://file/tmp/x", "not an http(s) address"},
		{"data", "data:text/html,hi", "not an http(s) address"},
		{"no scheme", "github.com/acme/demo", "not an http(s) address"},
		{"empty", "", "not an http(s) address"},
		{"no host", "https://", "names no host"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := OpenURL(c.url)
			if err == nil {
				t.Fatalf("OpenURL(%q) = nil, want an error", c.url)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("OpenURL(%q) = %q, want it to mention %q", c.url, err, c.want)
			}
		})
	}
}

// webURL accepts exactly what a remote link produces: an http or https address
// with a host, including a port, a path and a query. This keeps the accepted
// side covered without launching the system browser.
func TestWebURLAcceptsLinks(t *testing.T) {
	cases := []string{
		"https://github.com/acme/demo/commit/abc123",
		"http://git.example.com:8080/team/demo",
		"https://gitlab.com/group/sub/demo/-/merge_requests/new?merge_request%5Bsource_branch%5D=feat&merge_request%5Btarget_branch%5D=main",
	}
	for _, raw := range cases {
		u, err := webURL(raw)
		if err != nil {
			t.Fatalf("webURL(%q): %v", raw, err)
		}
		if got := u.String(); got != raw {
			t.Fatalf("webURL(%q) = %q", raw, got)
		}
	}
	// Surrounding whitespace is tolerated because a URL copied from a terminal
	// often carries a newline.
	if _, err := webURL("  https://github.com/acme/demo  "); err != nil {
		t.Fatalf("webURL with surrounding spaces: %v", err)
	}
}
