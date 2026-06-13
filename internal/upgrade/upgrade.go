// Package upgrade implements self-update for the clk binary. It resolves the
// latest GitHub Release, downloads the archive matching the running platform,
// and atomically replaces the executable in place. Network and filesystem
// concerns are separated so the release-resolution and archive-handling logic
// can be exercised against an httptest server and a temp dir.
package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// defaultAPIBaseURL is the GitHub REST API root used to resolve releases.
const defaultAPIBaseURL = "https://api.github.com"

// Client resolves and downloads clk releases from a GitHub repository.
type Client struct {
	httpClient *http.Client
	apiBaseURL string
	repo       string // "owner/name"
}

// New creates an upgrade client for the given "owner/name" repository.
func New(repo string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		apiBaseURL: defaultAPIBaseURL,
		repo:       repo,
	}
}

// Release is the subset of a GitHub Release we consume.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is a single downloadable file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// AssetURL returns the download URL of the asset with the given name, or an
// empty string when the release carries no such asset.
func (r Release) AssetURL(name string) string {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

// LatestRelease resolves the most recent published release of the repository.
func (c *Client) LatestRelease(ctx context.Context) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.apiBaseURL, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("query latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Release{}, fmt.Errorf("query latest release: github returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decode release: %w", err)
	}
	return rel, nil
}

// AssetName returns the archive name for a platform, matching the GoReleaser
// archives name_template ("clk_{{ .Os }}_{{ .Arch }}").
func AssetName(goos, goarch string) string {
	return fmt.Sprintf("clk_%s_%s.tar.gz", goos, goarch)
}

// CurrentAssetName returns the archive name for the running platform.
func CurrentAssetName() string {
	return AssetName(runtime.GOOS, runtime.GOARCH)
}

// NeedsUpgrade reports whether latest differs from current and is therefore
// worth installing. A "dev" or empty current version always upgrades; tags are
// compared ignoring a leading "v" so "v1.2.0" and "1.2.0" match.
func NeedsUpgrade(current, latest string) bool {
	if latest == "" {
		return false
	}
	if current == "" || current == "dev" {
		return true
	}
	return normalizeVersion(current) != normalizeVersion(latest)
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// downloadAsset fetches the bytes of an asset URL.
func (c *Client) downloadAsset(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download asset: github returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// extractBinary reads a gzip-compressed tar archive and returns the contents of
// the entry named binName.
func extractBinary(archive []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if filepath.Base(hdr.Name) == binName && hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read %s from archive: %w", binName, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("archive did not contain %q", binName)
}

// replaceExecutable atomically swaps the file at path with newBin. It writes to
// a temp file in the same directory (so the final rename stays on one
// filesystem) and renames over the target, which is safe even while the old
// binary is running on Unix.
func replaceExecutable(path string, newBin []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".clk-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(newBin); err != nil {
		tmp.Close()
		return fmt.Errorf("write new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	// Match a typical executable mode; preserve the old one when we can read it.
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}

// Result reports the outcome of an upgrade attempt.
type Result struct {
	From       string // version before the upgrade
	To         string // release tag installed (or the latest tag when no-op)
	Upgraded   bool   // false when already up to date
	BinaryPath string // executable that was (or would be) replaced
}

// Run performs the end-to-end self-update: resolve the latest release, and when
// it differs from currentVersion, download the platform archive and replace the
// executable at execPath. When already current it returns Upgraded=false.
func (c *Client) Run(ctx context.Context, currentVersion, execPath string) (Result, error) {
	res := Result{From: currentVersion, BinaryPath: execPath}

	rel, err := c.LatestRelease(ctx)
	if err != nil {
		return res, err
	}
	res.To = rel.TagName

	if !NeedsUpgrade(currentVersion, rel.TagName) {
		return res, nil
	}

	assetName := CurrentAssetName()
	url := rel.AssetURL(assetName)
	if url == "" {
		return res, fmt.Errorf("release %s has no asset %q for this platform", rel.TagName, assetName)
	}

	archive, err := c.downloadAsset(ctx, url)
	if err != nil {
		return res, err
	}
	bin, err := extractBinary(archive, "clk")
	if err != nil {
		return res, err
	}
	if err := replaceExecutable(execPath, bin); err != nil {
		return res, err
	}

	res.Upgraded = true
	return res, nil
}
