package observability

import (
	"os"
	"runtime"
	"testing"
)

// checkPerm verifies Unix permission bits. Windows does not model them, so a
// 0700 directory or 0600 file reads back as 0777/0666 there; the privacy
// boundary still exists through the OS user profile, and the failure is not a
// product bug.
func checkPerm(t *testing.T, info os.FileInfo, want os.FileMode, what string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", what, got, want)
	}
}
