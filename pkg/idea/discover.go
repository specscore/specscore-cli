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

// Discover walks `<specRoot>/ideas` and returns every idea file found.
// Returns ([], nil) if the directory does not exist.
func Discover(specRoot string) ([]Discovered, error) {
	ideasDir := filepath.Join(specRoot, "ideas")
	info, err := os.Stat(ideasDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var out []Discovered
	// Active: direct children *.md (exclude README.md).
	entries, err := os.ReadDir(ideasDir)
	if err != nil {
		return nil, err
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
	if ai, err := os.Stat(archivedDir); err == nil && ai.IsDir() {
		aEntries, err := os.ReadDir(archivedDir)
		if err != nil {
			return nil, err
		}
		for _, e := range aEntries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "README.md" || !strings.HasSuffix(name, ".md") {
				continue
			}
			out = append(out, Discovered{
				Slug:     strings.TrimSuffix(name, ".md"),
				Path:     filepath.Join(archivedDir, name),
				Archived: true,
			})
		}
	}

	// Proposals: scan spec/features/*/proposals/*.md
	featuresDir := filepath.Join(specRoot, "features")
	if fi, ferr := os.Stat(featuresDir); ferr == nil && fi.IsDir() {
		featureEntries, ferr := os.ReadDir(featuresDir)
		if ferr == nil {
			for _, fe := range featureEntries {
				if !fe.IsDir() {
					continue
				}
				proposalsDir := filepath.Join(featuresDir, fe.Name(), "proposals")
				pi, perr := os.Stat(proposalsDir)
				if perr != nil || !pi.IsDir() {
					continue
				}
				pEntries, perr := os.ReadDir(proposalsDir)
				if perr != nil {
					continue
				}
				for _, pe := range pEntries {
					if pe.IsDir() {
						continue
					}
					name := pe.Name()
					if name == "README.md" || !strings.HasSuffix(name, ".md") {
						continue
					}
					out = append(out, Discovered{
						Slug:       strings.TrimSuffix(name, ".md"),
						Path:       filepath.Join(proposalsDir, name),
						IsProposal: true,
						FeatureDir: fe.Name(),
					})
				}
			}
		}
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
	ideasDir := filepath.Join(specRoot, "ideas")
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

// sourceIdeasRe extracts the value portion of the **Source Ideas:** line.
var sourceIdeasRe = regexp.MustCompile(`^\*\*Source Ideas:\*\*\s*(.*)$`)

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
