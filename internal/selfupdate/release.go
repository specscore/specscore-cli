// Package selfupdate resolves the latest stable release of the specscore CLI
// from GitHub and compares it against the running build's version.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// devVersion is the placeholder reported by binaries built without -ldflags.
const devVersion = "dev"

// defaultReleasesURL is the GitHub REST endpoint listing releases, newest first.
const defaultReleasesURL = "https://api.github.com/repos/specscore/specscore-cli/releases"

// Verdict is the outcome of comparing the current build against the latest
// stable release.
type Verdict int

const (
	// UpToDate means the current version equals the latest stable release.
	UpToDate Verdict = iota
	// Available means a newer stable release exists.
	Available
	// Undetermined means the current version could not be established (e.g. a
	// dev build).
	Undetermined
)

// CheckResult captures the comparison between the current build and the latest
// stable release.
type CheckResult struct {
	Current string
	Latest  string
	Verdict Verdict
}

// ExitCode maps a verdict to the process exit code the CLI should use.
// UpToDate exits 0; Available and Undetermined exit 10.
func (v Verdict) ExitCode() int {
	if v == UpToDate {
		return 0
	}
	return 10
}

// release mirrors the subset of the GitHub release JSON we consume.
type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// Resolver fetches release information from GitHub. The base URL and HTTP
// client are injectable so tests can target an httptest.Server.
type Resolver struct {
	// BaseURL is the releases endpoint. When empty, defaultReleasesURL is used.
	BaseURL string
	// Client is the HTTP client used for requests. When nil, http.DefaultClient
	// is used.
	Client *http.Client
}

// LatestStableTag returns the tag of the newest non-prerelease, non-draft
// release. Releases are returned newest-first by the GitHub API; this skips
// prereleases and drafts and selects the first stable entry.
func (r Resolver) LatestStableTag(ctx context.Context) (string, error) {
	url := r.BaseURL
	if url == "" {
		url = defaultReleasesURL
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github releases request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var releases []release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("decode github releases: %w", err)
	}

	for _, rel := range releases {
		if rel.Prerelease || rel.Draft {
			continue
		}
		return rel.TagName, nil
	}
	return "", fmt.Errorf("no stable release found")
}

// Compare determines the verdict for a current build version against the latest
// stable release tag. A "dev" current version is Undetermined. Leading "v"
// prefixes are normalized before comparison.
func Compare(current, latestTag string) CheckResult {
	latest := normalize(latestTag)
	if current == devVersion {
		return CheckResult{Current: current, Latest: latest, Verdict: Undetermined}
	}
	cur := normalize(current)
	verdict := Available
	if cur == latest {
		verdict = UpToDate
	}
	return CheckResult{Current: cur, Latest: latest, Verdict: verdict}
}

// normalize strips a single leading "v" from a version string.
func normalize(v string) string {
	return strings.TrimPrefix(v, "v")
}
