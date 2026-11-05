package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// semver is a parsed release version. Build metadata is accepted on input but
// has no effect on precedence, as defined by Semantic Versioning 2.0.0.
type semver struct {
	major int
	minor int
	patch int
	pre   []string
}

// parseVersion parses vMAJOR.MINOR.PATCH[-prerelease][+build]. The leading "v"
// is optional because release tags and the built-in version differ there.
func parseVersion(text string) (semver, error) {
	original := strings.TrimSpace(text)
	if original == "" {
		return semver{}, fmt.Errorf("empty version")
	}
	s := original
	if strings.HasPrefix(s, "v") {
		s = s[1:]
	}
	// Build metadata is ignored for precedence; drop it before splitting the
	// prerelease because it follows the prerelease part.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	core := s
	var pre []string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core = s[:i]
		preText := s[i+1:]
		if preText == "" {
			return semver{}, fmt.Errorf("version %q has an empty prerelease", original)
		}
		pre = strings.Split(preText, ".")
		for _, id := range pre {
			if id == "" || !isIdentifier(id) {
				return semver{}, fmt.Errorf("version %q has an invalid prerelease identifier %q", original, id)
			}
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", original)
	}
	numbers := make([]int, 3)
	for i, part := range parts {
		n, err := parseNumber(part)
		if err != nil {
			return semver{}, fmt.Errorf("version %q: %w", original, err)
		}
		numbers[i] = n
	}
	return semver{major: numbers[0], minor: numbers[1], patch: numbers[2], pre: pre}, nil
}

// parseNumber parses one numeric version component.
func parseNumber(text string) (int, error) {
	if text == "" {
		return 0, fmt.Errorf("empty numeric component")
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, fmt.Errorf("numeric component %q contains a non-digit", text)
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("numeric component %q: %w", text, err)
	}
	return n, nil
}

// isIdentifier reports whether a prerelease identifier only uses the ASCII
// characters allowed by Semantic Versioning.
func isIdentifier(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// prerelease reports whether the version has a prerelease suffix; a release
// sorts above every prerelease of the same MAJOR.MINOR.PATCH.
func (v semver) prerelease() bool { return len(v.pre) > 0 }

// isPrerelease reports whether a version string is a valid prerelease version.
// Invalid or empty versions are not prereleases.
func isPrerelease(version string) bool {
	v, err := parseVersion(version)
	if err != nil {
		return false
	}
	return v.prerelease()
}

// compareSemver returns -1, 0, or 1 when a sorts below, equal to, or above b.
func compareSemver(a, b semver) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		switch {
		case pair[0] < pair[1]:
			return -1
		case pair[0] > pair[1]:
			return 1
		}
	}
	return comparePrerelease(a.pre, b.pre)
}

// comparePrerelease implements the Semantic Versioning prerelease precedence
// rules: identifiers are compared item by item, numeric identifiers compare
// numerically, alphanumeric identifiers compare lexicographically, numeric
// identifiers sort below alphanumeric ones, and a shorter list sorts below a
// longer one with an equal prefix.
func comparePrerelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	// A release version sorts above every prerelease.
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		aNum, aOK := numericIdentifier(a[i])
		bNum, bOK := numericIdentifier(b[i])
		switch {
		case aOK && bOK:
			if aNum < bNum {
				return -1
			}
			if aNum > bNum {
				return 1
			}
		case aOK:
			return -1
		case bOK:
			return 1
		default:
			if cmp := strings.Compare(a[i], b[i]); cmp != 0 {
				return cmp
			}
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// numericIdentifier reports whether an identifier is all digits and returns its
// value.
func numericIdentifier(text string) (int64, bool) {
	if text == "" {
		return 0, false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// compareVersions parses two versions and compares their precedence.
func compareVersions(a, b string) (int, error) {
	av, err := parseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", a, err)
	}
	bv, err := parseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", b, err)
	}
	return compareSemver(av, bv), nil
}
