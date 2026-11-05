// Package updater implements Viff's in-application update check, download,
// verification, and self-replacement. It performs no UI work: callers own
// progress presentation and quitting the process.
//
// The release feed is the public GitHub Releases API for the project. An update
// is only installed after its SHA-256 matches the release's SHA256SUMS asset.
// Install replaces the current installation and launches a small detached
// helper that waits for Viff to exit, swaps the files, and starts the new
// version.
package updater

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrManualInstall reports that Viff downloaded an update but cannot replace
// the current installation by itself (for example a DEB install owned by root).
// errors.As with *ManualInstallError carries the download path for the caller.
var ErrManualInstall = errors.New("updater: manual installation required")

// ErrUnsupported reports that this build cannot replace itself on this
// platform (an unsupported GOOS or an unusual installation layout).
var ErrUnsupported = errors.New("updater: self-update is not supported here")

// Asset is one downloadable release asset.
type Asset struct {
	// Name is the asset file name, such as viff-v0.1.1-darwin-arm64.dmg.
	Name string
	// URL is the HTTPS download address.
	URL string
	// Size is the advertised size in bytes, 0 when unknown.
	Size int64
}

// Update is a release newer than the running version.
type Update struct {
	// Version is the release tag, such as v0.1.1.
	Version string
	// Notes is the release body in Markdown.
	Notes string
	// PageURL is the release's html_url.
	PageURL string
	// Asset is the package for the running platform and architecture.
	Asset Asset
	// Checksums maps every asset name in SHA256SUMS to its hex SHA-256.
	Checksums map[string]string
}

// InstallResult describes a scheduled self-replacement.
type InstallResult struct {
	// Target is the path that will be replaced.
	Target string
}

// ManualInstallError explains why Viff could not install an update itself.
type ManualInstallError struct {
	// Target is the installation path that is not writable.
	Target string
	// Path is the downloaded package the user can install manually.
	Path string
	// Reason is a short user-facing explanation.
	Reason string
}

// Error implements error.
func (e *ManualInstallError) Error() string {
	return fmt.Sprintf("updater: cannot replace %s: %s", e.Target, e.Reason)
}

// Unwrap links the error to ErrManualInstall.
func (e *ManualInstallError) Unwrap() error { return ErrManualInstall }

// DefaultRepo is the GitHub repository releases are read from.
const DefaultRepo = "1lck/viff"

// Client checks the release feed and manages update downloads for one build.
// The zero value is not usable; use NewClient. Fields may be overridden by
// tests and development builds before the first call.
type Client struct {
	// Current is the running version, such as v0.1.0; version.Version.
	Current string
	// Repo is "owner/name"; empty means DefaultRepo.
	Repo string
	// GOOS and GOARCH select the release asset; empty means runtime values.
	GOOS   string
	GOARCH string
	// BaseURL is the API root; empty means https://api.github.com.
	BaseURL string
	// HTTP is the HTTP client; nil means a client with a 30-second timeout.
	HTTP *http.Client
	// CacheDir is where downloads and swap helpers live; empty means a
	// per-user cache directory under Viff/updates.
	CacheDir string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// NewClient returns a Client for the running version.
func NewClient(current string) *Client {
	return &Client{Current: current, Now: time.Now}
}
