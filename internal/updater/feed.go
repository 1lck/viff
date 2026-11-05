package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	// releasesPerPage is the page size requested from the releases endpoint.
	releasesPerPage = 30
	// releasesBodyLimit bounds the decoded JSON. Thirty releases with full
	// release notes stay far below it.
	releasesBodyLimit = 8 << 20
	// checksumAssetName is the release asset that lists SHA-256 digests.
	checksumAssetName = "SHA256SUMS"
)

// githubRelease is the subset of the GitHub Releases API the updater uses.
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

// githubAsset is one release asset.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// releaseCandidate pairs a release with its parsed version.
type releaseCandidate struct {
	release githubRelease
	version semver
}

// Check returns the newest eligible release, or nil when the running version
// is current. Prereleases are considered only when the running version is a
// prerelease itself.
func (c *Client) Check(ctx context.Context) (*Update, error) {
	current, err := parseVersion(c.Current)
	if err != nil {
		return nil, fmt.Errorf("updater: current version %q is not a release version: %w", c.Current, err)
	}
	goos, goarch := c.goos(), c.goarch()
	suffix, ok := c.packageSuffix()
	if !ok {
		return nil, fmt.Errorf("%w: no update package for %s/%s", ErrUnsupported, goos, goarch)
	}

	releases, err := c.fetchReleases(ctx)
	if err != nil {
		return nil, err
	}

	candidates := make([]releaseCandidate, 0, len(releases))
	for _, release := range releases {
		if release.Draft {
			continue
		}
		version, err := parseVersion(release.TagName)
		if err != nil {
			// Unknown tags cannot be compared; ignore them instead of failing
			// the whole check.
			continue
		}
		// GitHub's prerelease flag and a semver prerelease suffix both mark a
		// preview build.
		preview := release.Prerelease || version.prerelease()
		if preview && !current.prerelease() {
			continue
		}
		if compareSemver(version, current) <= 0 {
			continue
		}
		candidates = append(candidates, releaseCandidate{release: release, version: version})
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	// Newest first. A release without a package for this platform is skipped so
	// an older release can still offer the update.
	sort.SliceStable(candidates, func(i, j int) bool {
		return compareSemver(candidates[i].version, candidates[j].version) > 0
	})

	for _, candidate := range candidates {
		want := assetName(candidate.release.TagName, goos, goarch, suffix)
		asset, ok := findAsset(candidate.release.Assets, want)
		if !ok {
			continue
		}
		if asset.URL == "" {
			return nil, fmt.Errorf("updater: release %s asset %s has no download URL", candidate.release.TagName, want)
		}
		sums, err := c.fetchChecksums(ctx, candidate.release)
		if err != nil {
			return nil, err
		}
		return &Update{
			Version:   candidate.release.TagName,
			Notes:     candidate.release.Body,
			PageURL:   candidate.release.HTMLURL,
			Asset:     asset,
			Checksums: sums,
		}, nil
	}
	return nil, nil
}

// fetchReleases reads the public releases feed.
func (c *Client) fetchReleases(ctx context.Context) ([]githubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=%d", c.baseURL(), c.repo(), releasesPerPage)
	resp, err := c.get(ctx, url, githubAccept)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := readAllLimited(resp.Body, releasesBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("updater: read release feed: %w", err)
	}
	var releases []githubRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("updater: decode release feed: %w", err)
	}
	return releases, nil
}

// fetchChecksums downloads and parses the SHA256SUMS asset of a release.
func (c *Client) fetchChecksums(ctx context.Context, release githubRelease) (map[string]string, error) {
	asset, ok := findAsset(release.Assets, checksumAssetName)
	if !ok {
		return nil, fmt.Errorf("updater: release %s has no %s asset", release.TagName, checksumAssetName)
	}
	if asset.URL == "" {
		return nil, fmt.Errorf("updater: release %s asset %s has no download URL", release.TagName, checksumAssetName)
	}
	resp, err := c.get(ctx, asset.URL, "")
	if err != nil {
		return nil, fmt.Errorf("updater: fetch %s for %s: %w", checksumAssetName, release.TagName, err)
	}
	defer resp.Body.Close()
	data, err := readAllLimited(resp.Body, checksumFileSizeLimit)
	if err != nil {
		return nil, fmt.Errorf("updater: read %s for %s: %w", checksumAssetName, release.TagName, err)
	}
	sums, err := parseChecksums(data)
	if err != nil {
		return nil, fmt.Errorf("updater: parse %s for %s: %w", checksumAssetName, release.TagName, err)
	}
	return sums, nil
}

// packageSuffix returns the package suffix for this build. A Linux binary
// installed under /usr is managed by the system package manager, so it
// downloads the DEB its owner can install with apt; a portable binary updates
// itself from the tar.gz.
func (c *Client) packageSuffix() (string, bool) {
	switch c.goos() {
	case "darwin":
		return ".dmg", true
	case "windows":
		return ".zip", true
	case "linux":
		if linuxManagedInstall() {
			return ".deb", true
		}
		return ".tar.gz", true
	default:
		return "", false
	}
}

// linuxManagedInstall reports whether this process runs from a system prefix
// that only a package manager should replace.
func linuxManagedInstall() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return underSystemPrefix(exe)
}

// underSystemPrefix reports whether path lives under /usr, where the release
// DEB installs viff.
func underSystemPrefix(exe string) bool {
	clean := path.Clean(exe)
	return clean == "/usr" || strings.HasPrefix(clean, "/usr/")
}

// assetName is the exact release asset name for one platform and architecture.
func assetName(tag, goos, goarch, suffix string) string {
	return fmt.Sprintf("viff-%s-%s-%s%s", tag, goos, goarch, suffix)
}

// findAsset returns the asset whose name matches exactly.
func findAsset(assets []githubAsset, name string) (Asset, bool) {
	for _, candidate := range assets {
		if candidate.Name != name {
			continue
		}
		return Asset{Name: candidate.Name, URL: candidate.BrowserDownloadURL, Size: candidate.Size}, true
	}
	return Asset{}, false
}
