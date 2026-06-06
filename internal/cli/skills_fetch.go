package cli

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
	"strings"
	"time"
)

// Skill-bundle download — `agent setup` copies SpecScore skills into each
// agent's skills directory. Skills live in the GitHub plugin repos listed by
// the marketplace manifest (specscore/ai-marketplace). The CLI downloads the
// marketplace manifest, then each plugin repo's codeload tarball, and extracts
// the skills/ subtree. There is no embedded fallback: any fetch failure is
// surfaced to the caller. Verifies cli/agent/setup#req:skill-bundle-source.

// fetchSkillBundleFn wraps fetchSkillBundle so tests can stub the download.
var fetchSkillBundleFn = fetchSkillBundle

// skillsFetchTimeout bounds the total network wait. A var so tests can lower it.
var skillsFetchTimeout = 30 * time.Second

// skillsHTTPClient is the client used for skill-bundle fetches; a package var
// so tests can swap it.
var skillsHTTPClient = &http.Client{}

// resolveMarketplaceRef resolves the git ref used to download the marketplace
// manifest and plugin tarballs: the flag value wins, then the
// SPECSCORE_MARKETPLACE_REF environment variable, then "main".
func resolveMarketplaceRef(flagRef string) string {
	if r := strings.TrimSpace(flagRef); r != "" {
		return r
	}
	if r := strings.TrimSpace(os.Getenv("SPECSCORE_MARKETPLACE_REF")); r != "" {
		return r
	}
	return "main"
}

// marketplaceBaseURL is the raw base serving the marketplace manifest.
// Override via SPECSCORE_MARKETPLACE_BASE_URL (tests / self-hosted mirrors).
func marketplaceBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("SPECSCORE_MARKETPLACE_BASE_URL")); v != "" {
		return v
	}
	return "https://raw.githubusercontent.com"
}

// pluginArchiveBaseURL is the base serving per-plugin codeload tarballs.
// Override via SPECSCORE_PLUGIN_ARCHIVE_BASE_URL.
func pluginArchiveBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("SPECSCORE_PLUGIN_ARCHIVE_BASE_URL")); v != "" {
		return v
	}
	return "https://codeload.github.com"
}

type marketplaceManifest struct {
	Plugins []struct {
		Name   string `json:"name"`
		Source struct {
			Repo string `json:"repo"`
		} `json:"source"`
	} `json:"plugins"`
}

// skillFile is one file within a skill bundle, keyed by its path relative to a
// plugin's skills/ directory (e.g. "ideate/SKILL.md").
type skillFile struct {
	relPath string
	content []byte
}

func httpGetBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := skillsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// fetchSkillBundle downloads the marketplace manifest, then each referenced
// plugin's tarball, extracting every file under skills/. Returns the merged set
// keyed by path relative to skills/ (first plugin wins on a path collision). Any
// failure returns an error and no partial bundle.
func fetchSkillBundle(ref string) ([]skillFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), skillsFetchTimeout)
	defer cancel()

	mURL := strings.TrimRight(marketplaceBaseURL(), "/") +
		"/specscore/ai-marketplace/" + ref + "/.claude-plugin/marketplace.json"
	raw, err := httpGetBytes(ctx, mURL)
	if err != nil {
		return nil, fmt.Errorf("fetching marketplace manifest: %w", err)
	}
	var mf marketplaceManifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, fmt.Errorf("parsing marketplace manifest: %w", err)
	}

	var bundle []skillFile
	seen := map[string]bool{}
	for _, p := range mf.Plugins {
		repo := strings.TrimSpace(p.Source.Repo)
		if repo == "" {
			continue
		}
		files, err := fetchPluginSkills(ctx, repo, ref)
		if err != nil {
			return nil, fmt.Errorf("fetching skills for %s: %w", repo, err)
		}
		for _, f := range files {
			if seen[f.relPath] {
				continue
			}
			seen[f.relPath] = true
			bundle = append(bundle, f)
		}
	}
	return bundle, nil
}

func fetchPluginSkills(ctx context.Context, repo, ref string) ([]skillFile, error) {
	url := strings.TrimRight(pluginArchiveBaseURL(), "/") + "/" + repo + "/tar.gz/" + ref
	raw, err := httpGetBytes(ctx, url)
	if err != nil {
		return nil, err
	}
	return extractSkillsFromTarball(raw)
}

// extractSkillsFromTarball reads a gzipped tarball (a GitHub codeload archive)
// and returns every regular file under the top-level skills/ directory, keyed
// by its path relative to skills/.
func extractSkillsFromTarball(gzData []byte) ([]skillFile, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(gzData))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var files []skillFile
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		rel := stripSkillsPrefix(hdr.Name)
		if rel == "" {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files = append(files, skillFile{relPath: rel, content: content})
	}
	return files, nil
}

// stripSkillsPrefix turns "ai-plugin-specscore-main/skills/idea/SKILL.md" into
// "idea/SKILL.md". Returns "" for paths not under a top-level skills/ directory.
func stripSkillsPrefix(name string) string {
	parts := strings.SplitN(name, "/", 3) // [archiveRoot, "skills", rest]
	if len(parts) < 3 || parts[1] != "skills" || parts[2] == "" {
		return ""
	}
	return parts[2]
}
