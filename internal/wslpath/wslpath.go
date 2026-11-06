// Package wslpath converts between the Windows UNC spelling of paths inside
// the Windows Subsystem for Linux (\\wsl.localhost\<distro>\..., with the
// legacy \\wsl$ alias) and the Linux paths WSL-side programs use.
//
// The conversions are purely lexical: no process runs and no file system is
// touched. Viff keeps every path outside the Git execution layer in the
// Windows identity domain, so file reads, watchers and external opens go
// through the 9P redirector unchanged; only arguments handed to a Git process
// running inside a distro (and absolute paths it reports back) cross the
// boundary here. See docs/decisions/0035-wsl-repositories.md.
//
// Distro names are case-insensitive identifiers, so the canonical forms this
// package produces lowercase the distro component while preserving the case of
// everything below it: inside a distro the file system is case-sensitive and
// Repo and repo can coexist.
package wslpath

import (
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
// drive paths become "<x>:\" drive paths. A non-absolute path maps to "".
func FromLinux(distro, linuxPath string) string {
	linuxPath = strings.TrimSpace(linuxPath)
	if !strings.HasPrefix(linuxPath, "/") {
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

// ToLinuxArg maps an absolute path p to a form usable by Git running in
// distro, for position arguments such as a worktree path: a same-distro UNC
// path becomes its Linux path, a Windows drive path becomes /mnt/<x>/..., a
// path that is already Linux-absolute (or relative) passes through unchanged.
// A UNC path of a different distro cannot be reached from distro and passes
// through unchanged so Git itself reports the failure.
func ToLinuxArg(distro, p string) string {
	if d, rest, ok := split(p); ok {
		if d == distro {
			return linuxOf(rest)
		}
		return p
	}
	if strings.HasPrefix(p, "/") {
		return p
	}
	// A Windows drive path ("C:\Users\...") maps onto the distro's drive
	// mounts so a worktree can live on the Windows file system.
	if len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0]) {
		drive := p[0]
		if drive >= 'A' && drive <= 'Z' {
			drive += 'a' - 'A'
		}
		tail := strings.ReplaceAll(strings.TrimPrefix(p[2:], `\`), `\`, "/")
		if tail == "" {
			return mountPrefix + string(drive)
		}
		return mountPrefix + string(drive) + "/" + tail
	}
	return p
}

// split parses the UNC aliases. It returns the distro lowercased and the
// remainder below the distro root with original separators ("" for none).
func split(p string) (distro, rest string, ok bool) {
	s := strings.ReplaceAll(p, "/", `\`)
	rest = ""
	lower := strings.ToLower(s)
	for _, host := range []string{uncHost, uncHostOld} {
		prefix := `\\` + host + `\`
		if len(lower) > len(prefix) && strings.HasPrefix(lower, prefix) {
			d, r, _ := strings.Cut(s[len(prefix):], `\`)
			return strings.ToLower(d), r, d != ""
		}
	}
	return "", "", false
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
