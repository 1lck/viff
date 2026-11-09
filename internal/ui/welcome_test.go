package viffui

import "testing"

// TestProjectMatches pins the two search scopes: the name scope is a
// case-insensitive substring of the repository's own directory, the path scope
// keeps the subsequence matching over the whole path.
func TestProjectMatches(t *testing.T) {
	const (
		nested = "/home/ada/code/ips/alpha"
		named  = "/home/ada/code/ips-service"
	)
	cases := []struct {
		name  string
		root  string
		query string
		scope welcomeSearchScope
		want  bool
	}{
		{"empty query keeps rows", nested, "", welcomeSearchName, true},
		{"empty query in path scope", nested, "", welcomeSearchPath, true},
		{"name ignores the parent folder", nested, "ips", welcomeSearchName, false},
		{"name matches its own folder", named, "ips", welcomeSearchName, true},
		{"name is case-insensitive", named, "IPS-SERVICE", welcomeSearchName, true},
		{"name is a substring, not a subsequence", named, "is", welcomeSearchName, false},
		{"name does not match another path part", named, "ada", welcomeSearchName, false},
		{"path includes the parent folder", nested, "ips", welcomeSearchPath, true},
		{"path keeps the subsequence match", named, "is", welcomeSearchPath, true},
		{"path matches a deeper part", nested, "alpha", welcomeSearchPath, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := projectMatches(c.root, c.query, c.scope); got != c.want {
				t.Fatalf("projectMatches(%q, %q, %v) = %v, want %v", c.root, c.query, c.scope, got, c.want)
			}
		})
	}
}
