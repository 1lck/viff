package gitcli

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"

	"viff/internal/wslpath"
)

// WSL repositories keep every path outside the Git execution layer in the
// Windows identity domain (\\wsl.localhost\<distro>\...): file reads, the
// watcher and external opens go through the 9P redirector unchanged. Only at
// the process boundary do paths cross into the Linux execution domain, and
// only here: cwd goes through wsl.exe --cd, absolute paths Git reports come
// back through toWinPath, absolute paths handed to Git go through toGitPath.
// See docs/decisions/0038-wsl-repositories.md.

// wslDistroOf names the distro a Git process running in dir executes in, ""
// when dir is not a WSL repository.
func wslDistroOf(dir string) string {
	distro, _, ok := wslpath.Split(dir)
	if !ok {
		return ""
	}
	return distro
}

// toWinPath converts an absolute path Git reported (the Linux spelling inside
// a distro, a drive path for its Windows mounts) into the Windows identity
// domain. Repositories outside WSL pass paths through unchanged.
func toWinPath(dir, p string) string {
	distro := wslDistroOf(dir)
	if distro == "" {
		return p
	}
	return wslpath.FromLinux(distro, p)
}

// toGitPath converts an absolute path p of the Windows identity domain into
// the spelling a Git process running in dir can open: same-distro UNC paths
// become Linux paths, drive paths become /mnt/<x>/... and already-Linux or
// relative paths pass through. Repositories outside WSL pass paths through
// unchanged — the conversion is only defined against a distro.
func toGitPath(dir, p string) string {
	distro := wslDistroOf(dir)
	if distro == "" {
		return p
	}
	return wslpath.ToLinuxArg(distro, p)
}

// decodeWSLText decodes output captured from a Git invocation. Commands set
// WSL_UTF8=1 so wsl.exe writes UTF-8, and the Git it execs passes its bytes
// through untouched; this is a fallback for a wsl.exe too old to honor that
// variable, which writes its own diagnostics as UTF-16LE (measured: no
// byte-order mark on a pipe, so the encoding has to be inferred). Git's own
// output is plain text and never looks like UTF-16LE, so the inference cannot
// corrupt it.
func decodeWSLText(s string) string {
	b := []byte(s)
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		// Byte-order mark present.
	case looksUTF16LE(b):
	default:
		return s
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// looksUTF16LE reports the signature of wsl.exe's diagnostic stream without a
// BOM: little-endian pairs whose high byte is zero for the ASCII tokens
// (Wsl/Service/...) it always includes. Plain UTF-8 text has no zero bytes at
// all, so a tenth of the bytes being NUL is far outside what it produces.
func looksUTF16LE(b []byte) bool {
	if len(b) < 8 || len(b)%2 != 0 {
		return false
	}
	zeros := 0
	for i := 0; i < len(b); i++ {
		if b[i] == 0 {
			zeros++
		}
	}
	return zeros*10 >= len(b)
}

// classifyWSLError maps a failed wsl.exe invocation to an error kind. The
// match keys are the locale-independent Wsl/Service/... tokens and the relay
// diagnostic execvpe reports, never localized text: a Chinese or German WSL
// must classify the same way. Measured against WSL2 on Windows 11 (see ADR
// 0038 for the exact outputs).
func classifyWSLError(stderr string) ErrorKind {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "wsl_e_distro_not_found"):
		return ErrNoDistro
	case strings.Contains(s, "wsl_e_wsl_not_installed"), strings.Contains(s, "is not a supported wsl installation"):
		return ErrNoWSL
	case strings.Contains(s, "execvpe(") && strings.Contains(s, "no such file or directory"):
		return ErrNoWSLGit
	}
	return ErrOther
}
