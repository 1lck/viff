package svncli

import (
	"os/exec"
	"testing"
)

func TestFindHomebrewSVNWithoutShellPath(t *testing.T) {
	var want string
	for _, candidate := range []string{"/opt/homebrew/bin/svn", "/usr/local/bin/svn"} {
		if p, err := exec.LookPath(candidate); err == nil {
			want = p
			break
		}
	}
	if want == "" {
		t.Skip("no Homebrew svn in either standard prefix")
	}
	t.Setenv("PATH", t.TempDir())

	got, err := (&Runner{}).ExecutableIn(t.Context(), "")
	if err != nil || got != want {
		t.Fatalf("resolved svn = %q, %v; want %q", got, err, want)
	}
}
