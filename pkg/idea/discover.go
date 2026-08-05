package idea

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Discovered is a summary of an Idea file found during discovery.
type Discovered struct {
	Slug       string
	Path       string // absolute or relative path to the .md file
	Archived   bool   // true if located under archived/
	IsProposal bool   // true if located under spec/features/*/proposals/
	FeatureDir string // non-empty for proposals: the feature directory slug
}

// Discover walks `<specRoot>/ideas` and every
// `<specRoot>/features/**/proposals` directory, returning every Idea and
// change-request Proposal found. Returns ([], nil) when neither tree exists.
func Discover(specRoot string) ([]Discovered, error) {
	ideasDir := ResolveIdeasDir(specRoot)
	info, err := os.Stat(ideasDir)
	var out []Discovered
	if err == nil && info.IsDir() {
		// Active: direct children *.md (exclude README.md).
		entries, readErr := os.ReadDir(ideasDir)
		if readErr != nil {
			return nil, readErr
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "README.md" || !strings.HasSuffix(name, ".md") {
				continue
			}
			out = append(out, Discovered{
				Slug:     strings.TrimSuffix(name, ".md"),
				Path:     filepath.Join(ideasDir, name),
				Archived: false,
			})
		}

		archivedDir := filepath.Join(ideasDir, "archived")
		if ai, statErr := os.Stat(archivedDir); statErr == nil && ai.IsDir() {
			aEntries, readErr := os.ReadDir(archivedDir)
			if readErr != nil {
				return nil, readErr
			}
			for _, e := range aEntries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if name == "README.md" || !strings.HasSuffix(name, ".md") {
					continue
				}
				path := filepath.Join(archivedDir, name)
				// Skip promoted sidekick-seeds parked in archived/: they declare
				// `type: sidekick-seed` and are validated by the sidekick-seed
				// rule, not discovered as Ideas. (Skipping them also avoids a
				// slug collision with the active Idea they were promoted to.)
				if isSidekickSeedFile(path) {
					continue
				}
				out = append(out, Discovered{
					Slug:     strings.TrimSuffix(name, ".md"),
					Path:     path,
					Archived: true,
				})
			}
		}
	}

	// Proposals: recursively scan spec/features/**/proposals/*.md. The
	// scaffolder accepts nested Feature IDs, so discovery must use the same
	// path model rather than stopping after one Feature directory.
	featuresDir := filepath.Join(specRoot, "features")
	if fi, ferr := os.Stat(featuresDir); ferr == nil && fi.IsDir() {
		var scanFeatureTree func(dir, featureID string)
		scanFeatureTree = func(dir, featureID string) {
			entries, readErr := os.ReadDir(dir)
			if readErr == nil {
				for _, entry := range entries {
					name := entry.Name()
					if !entry.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
						continue
					}
					childDir := filepath.Join(dir, name)
					if name == "proposals" {
						proposalEntries, proposalErr := os.ReadDir(childDir)
						if proposalErr == nil {
							for _, proposal := range proposalEntries {
								proposalName := proposal.Name()
								if proposal.IsDir() || proposalName == "README.md" || !strings.HasSuffix(proposalName, ".md") {
									continue
								}
								out = append(out, Discovered{
									Slug:       strings.TrimSuffix(proposalName, ".md"),
									Path:       filepath.Join(childDir, proposalName),
									IsProposal: true,
									FeatureDir: filepath.ToSlash(featureID),
								})
							}
						}
						continue
					}
					childFeatureID := name
					if featureID != "" {
						childFeatureID = filepath.Join(featureID, name)
					}
					scanFeatureTree(childDir, childFeatureID)
				}
			}
		}
		scanFeatureTree(featuresDir, "")
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Archived != out[j].Archived {
			return !out[i].Archived
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

// FindIdeaDirectories returns directories that exist at `spec/ideas/<slug>/`
// (violation per REQ: single-file). Ignores the reserved `archived/` and
// `seeds/` directories — `seeds/` holds sidekick-seed documents, which
// are a separate document type, not malformed Ideas.
func FindIdeaDirectories(specRoot string) ([]string, error) {
	ideasDir := ResolveIdeasDir(specRoot)
	info, err := os.Stat(ideasDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(ideasDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "archived" || e.Name() == "seeds" {
			continue
		}
		out = append(out, filepath.Join(ideasDir, e.Name()))
	}
	return out, nil
}

// seedTypeRe matches a `type: sidekick-seed` line in YAML frontmatter,
// tolerating optional surrounding whitespace and quotes.
var seedTypeRe = regexp.MustCompile(`^type:\s*["']?sidekick-seed["']?\s*$`)

// isSidekickSeedFile reports whether the file at path begins with a
// `---`-delimited YAML frontmatter block declaring `type: sidekick-seed`.
// Unreadable or non-frontmatter files return false (treated as Ideas).
func isSidekickSeedFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() || strings.TrimRight(scanner.Text(), "\r") != "---" {
		return false
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "---" {
			return false
		}
		if seedTypeRe.MatchString(line) {
			return true
		}
	}
	return false
}

// sourceIdeasRe extracts the value portion of the **Source Ideas:** line.
var sourceIdeasRe = regexp.MustCompile(`^\*\*Source Ideas:\*\*\s*(.*)$`)

// mdLinkTargetRe captures the target of a `[label](target)` markdown link.
var mdLinkTargetRe = regexp.MustCompile(`^\[[^\]]*\]\(\s*([^)\s]+)\s*\)$`)

// IsCrossRepoRef reports whether a single comma-separated idea reference (from a
// **Source Ideas:** / **Related Ideas:** / **Supersedes:** value) points at an
// Idea in ANOTHER repository rather than a local idea slug. Cross-repo references
// use the URL form the linkage system already permits (entity#req:ref-target-exists,
// "the URL form is permitted when cross-repo imports land"): either a bare
// http(s) URL or a markdown link whose target is an http(s) URL. Such references
// cannot — and must not — be resolved against the local spec tree, so the local
// idea-cross-reference lint leaves them alone.
func IsCrossRepoRef(entry string) bool {
	e := strings.TrimSpace(entry)
	if strings.HasPrefix(e, "http://") || strings.HasPrefix(e, "https://") {
		return true
	}
	if m := mdLinkTargetRe.FindStringSubmatch(e); m != nil {
		t := m[1]
		return strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")
	}
	return false
}

// fpRel is a testable indirection for filepath.Rel used in FeatureSourceIdeas.
var fpRel = filepath.Rel

// FeatureSourceIdeas scans every `spec/features/**/README.md` and returns a
// map of feature slug -> []idea-slug based on the **Source Ideas:** header.
// Features without the field are omitted. The slug is the feature's path
// relative to `spec/features/` (e.g. `cli`, `cli/spec/lint`,
// `cli/lifecycle-transitions`), so nested sub-features are first-class.
func FeatureSourceIdeas(specRoot string) (map[string][]string, error) {
	featuresDir := filepath.Join(specRoot, "features")
	info, err := os.Stat(featuresDir)
	if err != nil || !info.IsDir() {
		return map[string][]string{}, nil
	}
	out := make(map[string][]string)
	err = filepath.Walk(featuresDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			return nil
		}
		// Skip hidden and underscore-convention dirs (e.g. `_args/`).
		base := filepath.Base(path)
		if path != featuresDir && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_")) {
			return filepath.SkipDir
		}
		// The features root itself has a README.md but is not a feature dir.
		if path == featuresDir {
			return nil
		}
		readme := filepath.Join(path, "README.md")
		if _, err := os.Stat(readme); err != nil {
			return nil
		}
		ideas, err := parseSourceIdeas(readme)
		if err != nil || len(ideas) == 0 {
			return nil
		}
		slug, err := fpRel(featuresDir, path)
		if err != nil {
			return nil
		}
		// Normalize to forward slashes so slugs are stable across platforms.
		slug = filepath.ToSlash(slug)
		out[slug] = ideas
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func parseSourceIdeas(readmePath string) ([]string, error) {
	f, err := os.Open(readmePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inFence := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Track fenced code blocks; "## " inside a fence is literal content,
		// not a section heading.
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		// Stop at first ## heading.
		if strings.HasPrefix(line, "## ") {
			break
		}
		if m := sourceIdeasRe.FindStringSubmatch(line); m != nil {
			return splitCSVSlugs(m[1]), scanner.Err()
		}
	}
	return nil, scanner.Err()
}
