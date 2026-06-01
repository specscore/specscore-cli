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

// DevVersion is the placeholder reported by binaries built without -ldflags.
const DevVersion = "dev"

// devVersion is the unexported alias retained for existing internal references.
const devVersion = DevVersion

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

// CompareVersions orders two semver-ish version strings, returning -1 if a < b,
// 0 if they are equal, and +1 if a > b. A leading "v" is ignored. Comparison is
// by numeric major/minor/patch; a prerelease suffix (after "-") sorts lower than
// the same release without it, per semver. This is a minimal comparison
// sufficient for the self-update downgrade guard, not a full semver implementation.
func CompareVersions(a, b string) int {
	ac, apre := splitVersion(a)
	bc, bpre := splitVersion(b)

	for i := 0; i < 3; i++ {
		if ac[i] != bc[i] {
			if ac[i] < bc[i] {
				return -1
			}
			return 1
		}
	}
	// Core versions equal: a prerelease is lower than its release.
	switch {
	case apre == "" && bpre == "":
		return 0
	case apre == "" && bpre != "":
		return 1
	case apre != "" && bpre == "":
		return -1
	default:
		return strings.Compare(apre, bpre)
	}
}

// splitVersion parses a version string into its numeric [major, minor, patch]
// and any prerelease suffix (the portion after the first "-"). Missing or
// non-numeric components are treated as 0.
func splitVersion(v string) ([3]int, string) {
	v = normalize(strings.TrimSpace(v))
	var pre string
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	var core [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i >= 3 {
			break
		}
		n := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				n = 0
				break
			}
			n = n*10 + int(r-'0')
		}
		core[i] = n
	}
	return core, pre
}
