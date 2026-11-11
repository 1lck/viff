package gitcli

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLFSTrackedAttributesRejectsReplacedIndex(t *testing.T) {
	tr := newTestRepo(t)
	const rel = "中文 space/.gitattributes"
	tr.write(rel, "*.psd filter=lfs\n")
	tr.commitAll("nested attributes")
	// A change after ls-files finishes but before the revision is checked
	// must not attach that result to the new index.
	replace := true
	tr.r.SetCommandLog(func(_ string, args []string, _ time.Duration, _ error) {
		if replace && args[0] == "ls-files" {
			replace = false
			tr.git("rm", "--cached", "-q", "--", rel)
		}
	})
	if cache, paths, err := tr.r.LFSTrackedAttributes(ctx, tr.repo, nil); err == nil || cache != nil || paths != nil {
		t.Fatalf("discovery across index replacement = %v, %v, %v", cache, paths, err)
	}
	tr.git("reset", "-q", "HEAD", "--", rel)
	cache, paths, err := tr.r.LFSTrackedAttributes(ctx, tr.repo, nil)
	if err != nil || !slices.Equal(paths, []string{rel}) {
		t.Fatalf("discovery after restoring index = %v, %v", paths, err)
	}
	again, paths, err := tr.r.LFSTrackedAttributes(ctx, tr.repo, cache)
	if err != nil || again != cache || !slices.Equal(paths, []string{rel}) {
		t.Fatalf("reusing stable index = %v, %v, %v", again == cache, paths, err)
	}
}

func TestLFSTrackedAttributesUsesActualIndexPath(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("base/.gitattributes", "*.bin filter=lfs\n")
	tr.commitAll("primary index")
	tr.r.Env = append(tr.r.Env, "GIT_INDEX_FILE="+filepath.Join(tr.dir, "alternate index"))
	tr.write("alternate/.gitattributes", "*.psd filter=lfs\n")
	tr.git("add", "--", "alternate/.gitattributes")
	_, paths, err := tr.r.LFSTrackedAttributes(ctx, tr.repo, nil)
	if err != nil || !slices.Equal(paths, []string{"alternate/.gitattributes"}) {
		t.Fatalf("alternate index discovery = %v, %v", paths, err)
	}
}

func TestLFSParseCacheBounds(t *testing.T) {
	tr := newTestRepo(t)
	tr.r.LFSCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil }
	paths := make([]string, 0, lfsRuleCacheEntries+10)
	for i := 0; i < cap(paths); i++ {
		paths = append(paths, fmt.Sprintf("%04d/.gitattributes", i))
	}
	// Missing tracked paths are cheap to probe but still take a map entry.
	st, cache := tr.r.LFS(ctx, tr.repo, paths, nil)
	if st.Err != nil || len(cache) > lfsRuleCacheEntries {
		t.Fatalf("cache with missing paths = %d entries, %v", len(cache), st.Err)
	}
	paths = paths[:8]
	for _, rel := range paths {
		tr.write(rel, strings.Repeat("a", lfsAttributesMaxSize-32)+" filter=lfs\n")
	}
	st, cache = tr.r.LFS(ctx, tr.repo, paths, nil)
	var bytes int
	for path, rules := range cache {
		bytes += len(path)
		for _, pattern := range rules.patterns {
			bytes += len(pattern)
		}
	}
	if st.Err != nil || len(st.Patterns) != len(paths) || bytes > lfsRuleCacheBytes {
		t.Fatalf("cache = %d bytes; rules = %d, %v", bytes, len(st.Patterns), st.Err)
	}
}
