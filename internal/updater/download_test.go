package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloadVerifiesAndReportsProgress(t *testing.T) {
	payload := []byte(strings.Repeat("viff", 4096))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/asset", http.StatusFound)
			return
		}
		w.Header().Set("Content-Length", "16384")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	sum, err := hashReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("v0.1.0")
	client.CacheDir = t.TempDir()
	update := &Update{
		Asset:     Asset{Name: "viff-v0.1.1-darwin-arm64.dmg", URL: server.URL + "/redirect", Size: int64(len(payload))},
		Checksums: map[string]string{"viff-v0.1.1-darwin-arm64.dmg": sum},
	}
	var lastDone, lastTotal int64
	path, err := client.Download(context.Background(), update, func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("downloaded %d bytes, want %d", len(data), len(payload))
	}
	if lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("progress = %d/%d, want %d/%d", lastDone, lastTotal, len(payload), len(payload))
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists after a completed download: %v", err)
	}
}

func TestDownloadRejectsWrongChecksum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()
	client := NewClient("v0.1.0")
	client.CacheDir = t.TempDir()
	update := &Update{
		Asset:     Asset{Name: "pkg.zip", URL: server.URL, Size: 7},
		Checksums: map[string]string{"pkg.zip": strings.Repeat("0", 64)},
	}
	if _, err := client.Download(context.Background(), update, nil); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("Download error = %v, want a checksum mismatch", err)
	}
	if _, err := os.Stat(client.cacheDir() + "/pkg.zip"); !os.IsNotExist(err) {
		t.Fatal("a rejected download was kept")
	}
}

func TestDownloadRejectsSizeMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()
	client := NewClient("v0.1.0")
	client.CacheDir = t.TempDir()
	sum, _ := hashReader(strings.NewReader("payload"))
	update := &Update{
		Asset:     Asset{Name: "pkg.zip", URL: server.URL, Size: 6},
		Checksums: map[string]string{"pkg.zip": sum},
	}
	if _, err := client.Download(context.Background(), update, nil); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("Download error = %v, want a size mismatch", err)
	}
}

func TestDownloadReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := NewClient("v0.1.0")
	client.CacheDir = t.TempDir()
	update := &Update{
		Asset:     Asset{Name: "pkg.zip", URL: server.URL},
		Checksums: map[string]string{"pkg.zip": strings.Repeat("0", 64)},
	}
	if _, err := client.Download(context.Background(), update, nil); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("Download error = %v, want HTTP 404", err)
	}
}

func TestDownloadHonorsCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()
	client := NewClient("v0.1.0")
	client.CacheDir = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	update := &Update{
		Asset:     Asset{Name: "pkg.zip", URL: server.URL},
		Checksums: map[string]string{"pkg.zip": strings.Repeat("0", 64)},
	}
	if _, err := client.Download(ctx, update, nil); err == nil {
		t.Fatal("Download succeeded with a cancelled context")
	}
}

func TestDownloadRequiresChecksumEntry(t *testing.T) {
	client := NewClient("v0.1.0")
	client.CacheDir = t.TempDir()
	update := &Update{Asset: Asset{Name: "pkg.zip", URL: "https://example.invalid/pkg.zip"}}
	if _, err := client.Download(context.Background(), update, nil); err == nil || !strings.Contains(err.Error(), "no sha256") {
		t.Fatalf("Download error = %v, want a missing checksum error", err)
	}
}
