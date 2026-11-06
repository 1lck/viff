// Package wslpath converts between the Windows UNC spelling of paths inside
// the Windows Subsystem for Linux (\\wsl.localhost\<distro>\..., with the
// legacy \\wsl$ alias) and the Linux paths WSL-side programs use.
//
// The conversions are purely lexical: no process runs and no file system is
// touched. Viff keeps every path outside the Git execution layer in the
// Windows identity domain, so file reads, watchers and external opens go
// through the 9P redirector unchanged; only arguments handed to a Git process
// running inside a distro (and absolute paths it reports back) cross the
// boundary here. See docs/decisions/0038-wsl-repositories.md.
//
// Distro names are case-insensitive identifiers, so the canonical forms this
// package produces lowercase the distro component while preserving the case of
// everything below it: inside a distro the file system is case-sensitive and
// Repo and repo can coexist.
package wslpath

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	uncHost     = `wsl.localhost` // current alias
	uncHostOld  = `wsl$`          // legacy alias, same file system
	mountPrefix = "/mnt/"         // where a distro mounts Windows drives
)

// Is reports whether p names a path inside a WSL distro via one of the UNC
// aliases. Both forward and back slashes are accepted; Windows tools produce
// either.
func Is(p string) bool {
	_, _, ok := split(p)
	return ok
}

// Normalize returns the canonical identity form of a WSL path:
// \\wsl.localhost\<distro>\<path> with the legacy alias folded away, the
// distro lowercased and the remainder cleaned. ok is false when p is not a
// WSL path or names no distro; callers keep their own spelling in that case.
func Normalize(p string) (string, bool) {
	distro, rest, ok := split(p)
	if !ok {
		return "", false
	}
	return unc(distro, rest), true
}

// Split breaks a WSL path into its distro and the Linux path below the
// distro root ("/home/u/repo"; "/" for the distro root itself). The distro is
// lowercased. ok is false for non-WSL paths.
func Split(p string) (distro, linuxPath string, ok bool) {
	distro, rest, ok := split(p)
	if !ok {
		return "", "", false
	}
	return distro, linuxOf(rest), true
}

// FromLinux maps a Linux absolute path inside distro back to the Windows
// identity domain: distro paths become \\wsl.localhost UNC paths and /mnt/<x>
// drive paths become "<x>:\" drive paths. A non-absolute path, or one whose
// distro is unknown, maps to "", so a missing distro can never produce the
// \\wsl.localhost\\path spelling that names no distribution.
func FromLinux(distro, linuxPath string) string {
	distro = strings.TrimSpace(distro)
	// Output arrives with a trailing newline; a trailing space is part of the
	// name on a file system that allows it, so only the line breaks go.
	linuxPath = strings.Trim(linuxPath, "\r\n")
	if distro == "" || !strings.HasPrefix(linuxPath, "/") {
		return ""
	}
	if rest, ok := strings.CutPrefix(linuxPath, mountPrefix); ok {
		if drive, tail, isMount := cutDrive(rest); isMount {
			return driveRoot(drive) + winPath(tail)
		}
		// Not a drive mount: /mnt is an ordinary distro directory here.
	}
	return unc(distro, linuxPath)
}

// ErrOutsideDistro reports a path a Git process running in a distro cannot
// open. A UNC path names another machine or another distribution, which the
// Linux side has no mount for; passing it through would make Git treat the
// text as a relative path and create or read a literal directory with that
// name instead of failing.
var ErrOutsideDistro = errors.New("path is outside the distro")

// ToLinuxArg maps an absolute path p to a form usable by Git running in
// distro, for position arguments such as a worktree path: a same-distro UNC
// path becomes its Linux path, a Windows drive path becomes /mnt/<x>/..., a
// path that is already Linux-absolute (or relative) passes through unchanged.
// A path the distro cannot reach is an error rather than a silently literal
// argument.
func ToLinuxArg(distro, p string) (string, error) {
	p = strings.Trim(p, "\r\n")
	if strings.HasPrefix(p, "/") {
		// Already the spelling the Linux side uses.
		return p, nil
	}
	s := stripPrefixes(strings.ReplaceAll(p, "/", `\`))
	if d, rest, ok := split(s); ok {
		if d != strings.ToLower(strings.TrimSpace(distro)) {
			return "", fmt.Errorf("%w: %s is in distro %s, not %s", ErrOutsideDistro, p, d, distro)
		}
		return linuxOf(rest), nil
	}
	// A Windows drive path ("C:\Users\...") maps onto the distro's drive mounts
	// so a worktree can live on the Windows file system.
	if len(s) >= 2 && s[1] == ':' && isDriveLetter(s[0]) {
		drive := s[0]
		if drive >= 'A' && drive <= 'Z' {
			drive += 'a' - 'A'
		}
		tail := strings.ReplaceAll(strings.TrimLeft(s[2:], `\`), `\`, "/")
		if tail == "" {
			return mountPrefix + string(drive), nil
		}
		return mountPrefix + string(drive) + "/" + tail, nil
	}
	if strings.HasPrefix(s, `\\`) {
		return "", fmt.Errorf("%w: %s is not a path inside distro %s", ErrOutsideDistro, p, distro)
	}
	return p, nil
}

// split parses the UNC aliases. It returns the distro lowercased and the
// remainder below the distro root with original separators ("" for none).
func split(p string) (distro, rest string, ok bool) {
	parts := uncParts(p)
	if len(parts) < 2 || parts[1] == "" {
		return "", "", false
	}
	if !strings.EqualFold(parts[0], uncHost) && !strings.EqualFold(parts[0], uncHostOld) {
		return "", "", false
	}
	return strings.ToLower(parts[1]), strings.Join(parts[2:], `\`), true
}

// uncParts returns the components of a UNC path, or nil when p is not one. It
// folds separators and strips the prefixes Windows tools add, so that
// \\?\UNC\wsl.localhost\Ubuntu\home, \\wsl.localhost\\ubuntu\home and
// \\wsl.localhost\ubuntu\home are the same path. Dropping empty components is
// what folds the doubled separators; the distro component being empty is
// rejected by the caller, because \\wsl.localhost\\home names no distro.
func uncParts(p string) []string {
	s := stripPrefixes(strings.ReplaceAll(p, "/", `\`))
	if !strings.HasPrefix(s, `\\`) {
		return nil
	}
	return strings.FieldsFunc(s[2:], func(r rune) bool { return r == '\\' })
}

// stripPrefixes removes the extended-length and device prefixes Windows tools
// emit, turning \\?\UNC\server\share into \\server\share. Inputs are already
// backslash-separated.
func stripPrefixes(s string) string {
	if len(s) >= len(`\\?\UNC\`) && strings.EqualFold(s[:len(`\\?\UNC\`)], `\\?\UNC\`) {
		return `\\` + s[len(`\\?\UNC\`):]
	}
	for _, prefix := range []string{`\\?\`, `\\.\`} {
		if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
			return s[len(prefix):]
		}
	}
	return s
}

// unc rebuilds the canonical UNC form from a distro and its internal rest.
func unc(distro, rest string) string {
	p := `\\` + uncHost + `\` + strings.ToLower(distro)
	if clean := winPath(rest); clean != "" {
		p += `\` + clean
	}
	return p
}

// linuxOf turns the distro-internal remainder into a Linux absolute path.
func linuxOf(rest string) string {
	if rest == "" {
		return "/"
	}
	return path.Clean("/" + strings.ReplaceAll(rest, `\`, "/"))
}

// winPath cleans a distro-internal remainder and renders it with backslash
// separators; "" means the distro root itself.
func winPath(rest string) string {
	if rest == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.ReplaceAll(rest, `\`, "/"))
	if cleaned == "/" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimPrefix(cleaned, "/"), "/", `\`)
}

// cutDrive splits "c" or "c/users/momo" after /mnt/ (or "c:\..." before it)
// into the drive letter and the remainder. The drive must be a single letter;
// "repo" after /mnt/ is an ordinary distro path, not a mount.
func cutDrive(rest string) (drive byte, tail string, ok bool) {
	if rest == "" {
		return 0, "", false
	}
	d := rest[0]
	if !isDriveLetter(d) {
		return 0, "", false
	}
	tail = rest[1:]
	if tail == "" {
		return d, "", true
	}
	if tail[0] == '/' || tail[0] == '\\' {
		return d, tail[1:], true
	}
	return 0, "", false
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// driveRoot renders the Windows root of a drive letter, uppercased like
// Windows itself reports it ("C:\").
func driveRoot(drive byte) string {
	if drive >= 'a' && drive <= 'z' {
		drive -= 'a' - 'A'
	}
	return string(drive) + `:\`
}
