//go:build darwin

package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleRoot(t *testing.T) {
	cases := map[string]string{
		"/Applications/Viff.app/Contents/MacOS/viff":           "/Applications/Viff.app",
		"/Users/ada/Applications/Viff.app/Contents/MacOS/viff": "/Users/ada/Applications/Viff.app",
		"/usr/local/bin/viff":                                  "",
		"/tmp/Viff.app":                                        "/tmp/Viff.app",
	}
	for path, want := range cases {
		got, ok := bundleRoot(path)
		if want == "" {
			if ok {
				t.Errorf("bundleRoot(%q) = %q, want no bundle", path, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("bundleRoot(%q) = %q/%v, want %q", path, got, ok, want)
		}
	}
}

func TestFindAppBundle(t *testing.T) {
	mount := t.TempDir()
	if _, err := findAppBundle(mount); err == nil {
		t.Fatal("findAppBundle accepted an empty mount")
	}
	if err := os.MkdirAll(filepath.Join(mount, "Viff.app", "Contents"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := findAppBundle(mount)
	if err != nil {
		t.Fatalf("findAppBundle: %v", err)
	}
	if got != filepath.Join(mount, "Viff.app") {
		t.Fatalf("findAppBundle = %q", got)
	}
}
