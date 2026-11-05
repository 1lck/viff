package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// assetsFor builds release assets whose download URLs point at the test server.
func assetsFor(server, tag string, names ...string) []githubAsset {
	assets := make([]githubAsset, 0, len(names))
	for _, name := range names {
		assets = append(assets, githubAsset{
			Name:               name,
			BrowserDownloadURL: server + "/assets/" + name,
			Size:               1024,
		})
	}
	return assets
}

// testFeed serves a release list and SHA256SUMS for hermetic checks.
type testFeed struct {
	releases []githubRelease
	status   int
	sums     string
}

func (f *testFeed) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/1lck/viff/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || !strings.HasPrefix(r.Header.Get("User-Agent"), userAgentPrefix) {
			t.Errorf("release request has User-Agent %q", r.Header.Get("User-Agent"))
		}
		if f.status != 0 {
			if f.status == http.StatusForbidden {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "4102444800")
			}
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"message":"denied"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(f.releases)
	})
	writeSums := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(f.sums))
	}
	mux.HandleFunc("/SHA256SUMS", writeSums)
	mux.HandleFunc("/assets/SHA256SUMS", writeSums)
	return mux
}

func sumsFor(names ...string) string {
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "%s  ./%s\n", hex64("ab"), name)
	}
	return b.String()
}

func newTestClient(t *testing.T, server, current, goos, goarch string) *Client {
	t.Helper()
	client := NewClient(current)
	client.Repo = "1lck/viff"
	client.BaseURL = server
	client.GOOS, client.GOARCH = goos, goarch
	client.CacheDir = t.TempDir()
	return client
}

func TestCheckSelectsNewestStableForStableBuild(t *testing.T) {
	const (
		tag   = "v0.1.1"
		asset = "viff-v0.1.1-darwin-arm64.dmg"
	)
	feed := &testFeed{sums: sumsFor(asset)}
	server := httptest.NewServer(feed.handler(t))
	defer server.Close()
	feed.releases = []githubRelease{
		{TagName: "v0.1.1", Assets: assetsFor(server.URL, tag, asset, checksumAssetName)},
		{TagName: "v0.2.0", Prerelease: true, Assets: assetsFor(server.URL, "v0.2.0", "viff-v0.2.0-darwin-arm64.dmg", checksumAssetName)},
		{TagName: "v0.1.0", Assets: assetsFor(server.URL, "v0.1.0", "viff-v0.1.0-darwin-arm64.dmg", checksumAssetName)},
	}
	client := newTestClient(t, server.URL, "v0.1.0", "darwin", "arm64")
	client.HTTP = server.Client()
	got, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got == nil {
		t.Fatal("Check returned no update")
	}
	if got.Version != tag || got.Asset.Name != asset {
		t.Fatalf("Check = %s/%s, want %s/%s", got.Version, got.Asset.Name, tag, asset)
	}
	if got.Checksums[asset] != hex64("ab") {
		t.Errorf("checksums not attached: %v", got.Checksums)
	}
}

func TestCheckAllowsPrereleaseForPrereleaseBuild(t *testing.T) {
	const (
		tag   = "v0.1.0-alpha.3"
		asset = "viff-v0.1.0-alpha.3-darwin-arm64.dmg"
	)
	feed := &testFeed{sums: sumsFor(asset)}
	server := httptest.NewServer(feed.handler(t))
	defer server.Close()
	feed.releases = []githubRelease{
		{TagName: tag, Prerelease: true, Assets: assetsFor(server.URL, tag, asset, checksumAssetName)},
		{TagName: "v0.1.0-alpha.1", Prerelease: true, Assets: assetsFor(server.URL, "v0.1.0-alpha.1", "viff-v0.1.0-alpha.1-darwin-arm64.dmg", checksumAssetName)},
	}
	client := newTestClient(t, server.URL, "v0.1.0-alpha.2", "darwin", "arm64")
	client.HTTP = server.Client()
	got, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got == nil || got.Version != tag {
		t.Fatalf("Check = %+v, want %s", got, tag)
	}
}

func TestCheckIgnoresPrereleaseForStableBuild(t *testing.T) {
	feed := &testFeed{sums: sumsFor("viff-v0.2.0-darwin-arm64.dmg")}
	server := httptest.NewServer(feed.handler(t))
	defer server.Close()
	feed.releases = []githubRelease{
		{TagName: "v0.2.0", Prerelease: true, Assets: assetsFor(server.URL, "v0.2.0", "viff-v0.2.0-darwin-arm64.dmg", checksumAssetName)},
		{TagName: "v0.1.0", Assets: assetsFor(server.URL, "v0.1.0", "viff-v0.1.0-darwin-arm64.dmg", checksumAssetName)},
	}
	client := newTestClient(t, server.URL, "v0.1.0", "darwin", "arm64")
	client.HTTP = server.Client()
	got, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != nil {
		t.Fatalf("Check offered %s to a stable build", got.Version)
	}
}

func TestCheckSkipsDraftsAndMissingPlatformAssets(t *testing.T) {
	const newer = "v0.2.0"
	const older = "v0.1.1"
	const olderAsset = "viff-v0.1.1-darwin-arm64.dmg"
	feed := &testFeed{sums: sumsFor(olderAsset)}
	server := httptest.NewServer(feed.handler(t))
	defer server.Close()
	feed.releases = []githubRelease{
		{TagName: "v0.3.0", Draft: true, Assets: assetsFor(server.URL, "v0.3.0", "viff-v0.3.0-darwin-arm64.dmg", checksumAssetName)},
		{TagName: newer, Assets: assetsFor(server.URL, newer, "viff-v0.2.0-windows-amd64.zip", checksumAssetName)},
		{TagName: older, Assets: assetsFor(server.URL, older, olderAsset, checksumAssetName)},
	}
	client := newTestClient(t, server.URL, "v0.1.0", "darwin", "arm64")
	client.HTTP = server.Client()
	got, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got == nil || got.Version != older {
		t.Fatalf("Check = %+v, want fallback to %s", got, older)
	}
}

func TestCheckReportsMissingChecksums(t *testing.T) {
	const asset = "viff-v0.1.1-darwin-arm64.dmg"
	feed := &testFeed{}
	server := httptest.NewServer(feed.handler(t))
	defer server.Close()
	feed.releases = []githubRelease{
		{TagName: "v0.1.1", Assets: assetsFor(server.URL, "v0.1.1", asset)},
	}
	client := newTestClient(t, server.URL, "v0.1.0", "darwin", "arm64")
	client.HTTP = server.Client()
	if _, err := client.Check(context.Background()); err == nil || !strings.Contains(err.Error(), checksumAssetName) {
		t.Fatalf("Check error = %v, want a missing SHA256SUMS error", err)
	}
}

func TestCheckReportsRateLimit(t *testing.T) {
	feed := &testFeed{status: http.StatusForbidden}
	server := httptest.NewServer(feed.handler(t))
	defer server.Close()
	client := newTestClient(t, server.URL, "v0.1.0", "darwin", "arm64")
	client.HTTP = server.Client()
	if _, err := client.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("Check error = %v, want a rate limit error", err)
	}
}

func TestCheckRejectsUnsupportedPlatform(t *testing.T) {
	client := NewClient("v0.1.0")
	client.GOOS, client.GOARCH = "plan9", "mips"
	if _, err := client.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("Check error = %v, want an unsupported-platform error", err)
	}
}
