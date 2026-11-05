package updater

import "testing"

func TestParseVersionRejectsInvalidTags(t *testing.T) {
	for _, text := range []string{"", "v", "dev", "v1", "v1.2", "v1.2.x", "v1.2.3-", "v1.2.3- bad", "v1.2.3-alpha_1"} {
		if _, err := parseVersion(text); err == nil {
			t.Errorf("parseVersion(%q) accepted an invalid version", text)
		}
	}
}

func TestParseVersionAcceptsReleasedForms(t *testing.T) {
	cases := map[string]semver{
		"v1.2.3":          {major: 1, minor: 2, patch: 3},
		"1.2.3":           {major: 1, minor: 2, patch: 3},
		"v0.1.0-alpha.2":  {major: 0, minor: 1, patch: 0, pre: []string{"alpha", "2"}},
		"v2.0.0+build.17": {major: 2, minor: 0, patch: 0},
	}
	for text, want := range cases {
		got, err := parseVersion(text)
		if err != nil {
			t.Fatalf("parseVersion(%q): %v", text, err)
		}
		if got.major != want.major || got.minor != want.minor || got.patch != want.patch || len(got.pre) != len(want.pre) {
			t.Errorf("parseVersion(%q) = %+v, want %+v", text, got, want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.3.0", "v1.2.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.0.0-alpha", "v1.0.0", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-alpha.2", "v1.0.0-alpha.10", -1},
		{"v1.0.0-alpha.1", "v1.0.0-beta", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.11", -1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0-alpha+meta", "v1.0.0-alpha+other", 0},
		{"v0.1.0-alpha.2", "v0.1.0-alpha.10", -1},
	}
	for _, tc := range cases {
		got, err := compareVersions(tc.a, tc.b)
		if err != nil {
			t.Fatalf("compareVersions(%q, %q): %v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestIsPrerelease(t *testing.T) {
	for text, want := range map[string]bool{
		"v0.1.0":         false,
		"v0.1.0-alpha.1": true,
		"0.1.0":          false,
		"dev":            false,
		"":               false,
	} {
		if got := isPrerelease(text); got != want {
			t.Errorf("isPrerelease(%q) = %v, want %v", text, got, want)
		}
	}
}
