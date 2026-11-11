package gitcli

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestParseLFSAttributes(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		data string
		want []string
	}{
		{
			name: "git lfs track line",
			data: "*.bin filter=lfs diff=lfs merge=lfs -text\n",
			want: []string{"*.bin"},
		},
		{
			name: "comments and blank lines",
			data: "# binaries\n\n   \n*.zip filter=lfs\n",
			want: []string{"*.zip"},
		},
		{
			name: "other attributes",
			data: "*.txt text eol=lf\n*.png -filter\n",
			want: nil,
		},
		{
			name: "filter set to another driver",
			data: "*.dat filter=git-crypt\n",
			want: nil,
		},
		{
			name: "last rule for a pattern wins",
			data: "*.bin filter=lfs\n*.bin -filter\n",
			want: nil,
		},
		{
			name: "unset then lfs",
			data: "*.bin -filter\n*.bin filter=lfs\n",
			want: []string{"*.bin"},
		},
		{
			name: "duplicate rules are reported once",
			data: "*.bin filter=lfs\n*.bin filter=lfs diff=lfs\n",
			want: []string{"*.bin"},
		},
		{
			name: "quoted pattern with spaces",
			data: "\"final model.bin\" filter=lfs\n",
			want: []string{"final model.bin"},
		},
		{
			name: "tabs and CRLF",
			data: "*.mov\tfilter=lfs\r\n*.avi filter=lfs\r\n",
			want: []string{"*.mov", "*.avi"},
		},
		{
			name: "nested file is prefixed with its directory",
			dir:  "assets/models",
			data: "*.bin filter=lfs\n",
			want: []string{"assets/models/*.bin"},
		},
		{
			name: "anchored pattern stays anchored under the directory",
			dir:  "assets",
			data: "/raw.bin filter=lfs\n",
			want: []string{"assets/raw.bin"},
		},
		{
			name: "pattern with no attributes is ignored",
			data: "*.bin\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLFSAttributes([]byte(tt.data), tt.dir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseLFSAttributes(%q, %q) = %v, want %v", tt.data, tt.dir, got, tt.want)
			}
		})
	}
}

func TestParseLFSFiles(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{name: "empty", out: "", want: nil},
		{name: "NUL separated", out: "a.bin\x00dir with space/b.bin\x00", want: []string{"a.bin", "dir with space/b.bin"}},
		{name: "newline fallback", out: "a.bin\r\nb.bin\n\n", want: []string{"a.bin", "b.bin"}},
		{name: "path is not trimmed", out: " leading.bin\x00trailing.bin \x00", want: []string{" leading.bin", "trailing.bin "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLFSFiles([]byte(tt.out)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseLFSFiles(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}

func TestLFSDetectsRulesWhenGitLFSMissing(t *testing.T) {
	tr := newTestRepo(t)
	tr.write(".gitattributes", "# assets\n*.bin filter=lfs diff=lfs merge=lfs -text\n*.psd -filter\n")

	probes := 0
	tr.r.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		probes++
		return nil, errors.New("git: 'lfs' is not a git command")
	}
	st := tr.r.LFS(ctx, tr.repo)
	if st.Installed {
		t.Fatalf("Installed = true without git-lfs; state = %+v", st)
	}
	if st.Version != "" {
		t.Fatalf("Version = %q, want empty", st.Version)
	}
	if !st.Managed {
		t.Fatalf("Managed = false for a repository with filter=lfs rules")
	}
	if !st.NeedsInstall() {
		t.Fatalf("NeedsInstall = false for rules without git-lfs")
	}
	if want := []string{"*.bin"}; !reflect.DeepEqual(st.Patterns, want) {
		t.Fatalf("Patterns = %v, want %v", st.Patterns, want)
	}
	if st.Err != nil {
		t.Fatalf("missing git-lfs is not an error: %v", st.Err)
	}

	// The capability probe is cached for the process; the rule scan still
	// answers without it.
	if second := tr.r.LFS(ctx, tr.repo); !second.Managed || probes != 1 {
		t.Fatalf("second detection = %+v, probes = %d", second, probes)
	}
}

func TestLFSDetectionDoesNotListFilesAndCachesProbe(t *testing.T) {
	tr := newTestRepo(t)
	tr.write(".gitattributes", "assets/*.bin filter=lfs\n")
	probes, listings := 0, 0
	tr.r.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		switch {
		case len(args) == 1 && args[0] == "version":
			probes++
			return []byte("git-lfs/9.9.9 (fake build)\nextra line\n"), nil
		case len(args) > 1 && args[0] == "ls-files":
			listings++
			return []byte("assets/a.bin\x00assets/b.bin\x00"), nil
		}
		return nil, errors.New("unexpected git-lfs command")
	}
	st := tr.r.LFS(ctx, tr.repo)
	if !st.Installed || st.Version != "git-lfs/9.9.9 (fake build)" {
		t.Fatalf("installed state = %+v", st)
	}
	if !st.Managed || st.NeedsInstall() {
		t.Fatalf("managed state = %+v", st)
	}
	if st.Err != nil {
		t.Fatalf("Err = %v", st.Err)
	}
	tr.r.LFS(ctx, tr.repo)
	if probes != 1 || listings != 0 {
		t.Fatalf("automatic probes = %d, listings = %d, want 1 and 0", probes, listings)
	}
	files, err := tr.r.LFSFiles(ctx, tr.repo)
	if want := []string{"assets/a.bin", "assets/b.bin"}; err != nil || !reflect.DeepEqual(files, want) {
		t.Fatalf("explicit listing = %v, %v, want %v", files, err, want)
	}
	if listings != 1 {
		t.Fatalf("managed files listed %d times, want 1", listings)
	}
}

func TestLFSListFailureKeepsRulesAndDiagnostic(t *testing.T) {
	tr := newTestRepo(t)
	tr.write(".gitattributes", "*.bin filter=lfs\n")
	tr.r.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "version" {
			return []byte("git-lfs/9.9.9"), nil
		}
		return nil, errors.New("listing failed")
	}
	st := tr.r.LFS(ctx, tr.repo)
	if !st.Installed {
		t.Fatal("Installed = false with a working probe")
	}
	if !st.Managed || st.Err != nil {
		t.Fatalf("state = %+v, want rules and no detection error", st)
	}
	if _, err := tr.r.LFSFiles(ctx, tr.repo); err == nil || CodeOf(err) != "git.failed" {
		t.Fatalf("list error = %v, want git.failed", err)
	}
}

func TestLFSRepositoryWithoutRulesIsNotManaged(t *testing.T) {
	tr := newTestRepo(t)
	tr.r.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("missing")
	}
	st := tr.r.LFS(ctx, tr.repo)
	if st.Managed || st.NeedsInstall() || len(st.Patterns) != 0 {
		t.Fatalf("state = %+v, want a non-LFS repository", st)
	}
}

func TestLFSReadsNestedAttributeFile(t *testing.T) {
	tr := newTestRepo(t)
	tr.write(".gitattributes", "*.bin filter=lfs\n")
	tr.write("assets/.gitattributes", "*.psd filter=lfs\n")
	st := tr.r.LFS(ctx, tr.repo)
	if want := []string{"*.bin", "assets/*.psd"}; !reflect.DeepEqual(st.Patterns, want) {
		t.Fatalf("Patterns = %v, want %v", st.Patterns, want)
	}
}

func TestLFSFallsBackToJSONListing(t *testing.T) {
	tr := newTestRepo(t)
	tr.write(".gitattributes", "*.bin filter=lfs\n")
	tr.r.LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		switch {
		case len(args) == 1 && args[0] == "version":
			return []byte("git-lfs/2.0.0 (old)"), nil
		case len(args) > 0 && args[0] == "ls-files":
			for _, a := range args {
				if a == "-z" {
					return nil, errors.New("unknown flag -z")
				}
			}
			return []byte(`{"files":[{"name":"assets/a.bin"},{"name":"assets/a\nb.bin"}]}`), nil
		}
		return nil, errors.New("unexpected command")
	}
	files, err := tr.r.LFSFiles(ctx, tr.repo)
	if want := []string{"assets/a.bin", "assets/a\nb.bin"}; err != nil || !reflect.DeepEqual(files, want) {
		t.Fatalf("Files = %v, want %v (err = %v)", files, want, err)
	}
}

func TestLFSInputsSupportsUnbornAndStagedBlobChanges(t *testing.T) {
	tr := newTestRepo(t)
	first, err := tr.r.LFSInputs(ctx, tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	tr.write("staged.txt", "one\n")
	tr.git("add", "staged.txt")
	second, err := tr.r.LFSInputs(ctx, tr.repo)
	if err != nil || first == second {
		t.Fatalf("unborn index change not detected: %v", err)
	}
	tr.write("staged.txt", "two\n")
	tr.git("add", "staged.txt")
	third, err := tr.r.LFSInputs(ctx, tr.repo)
	if err != nil || second == third {
		t.Fatalf("restaged blob change not detected: %v", err)
	}
}

func TestLFSJSONRejectsIncompleteOutput(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"files":[{}]}`, `{"files":[`} {
		if _, err := parseLFSJSON([]byte(raw)); err == nil {
			t.Fatalf("accepted an incomplete JSON file list: %q", raw)
		}
	}
	for _, raw := range []string{`{"files":[]}`, `{"files":null}`} {
		if files, err := parseLFSJSON([]byte(raw)); err != nil || len(files) != 0 {
			t.Fatalf("empty files array = %v, %v", files, err)
		}
	}
}
