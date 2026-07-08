package graph

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// legacyModelspecTokenRe matches a modelspec:// reference token in a frontmatter
// line, stopping at whitespace or a surrounding quote so a trailing YAML comment
// is not swallowed. Each match is parsed; only legacy-form tokens are rewritten.
var legacyModelspecTokenRe = regexp.MustCompile(`modelspec://[^\s"']+`)

// fixLegacyModelspecForms rewrites every legacy modelspec://x.Y reference
// (authority present, empty path) to its triple-slash form in the frontmatter
// of every graph artifact, preserving all other bytes. It returns the
// repo-root-relative, sorted, de-duplicated paths of files it changed. This is
// the graph rule family's first fixer with specified semantics (decision 0010).
func fixLegacyModelspecForms(g *Graph) ([]string, error) {
	var changed []string
	for _, m := range g.Modules {
		for _, a := range collectArtifacts(m) {
			content, err := readFileFn(a.Path)
			if err != nil {
				return nil, err
			}
			out, did := rewriteLegacyModelspecFrontmatter(content)
			if !did {
				continue
			}
			if err := writeFileFn(a.Path, out, 0o644); err != nil {
				return nil, err
			}
			// Re-parse the rewritten artifact in place so the subsequent lint
			// pass reports remaining violations against the fixed content.
			reparsed, err := ParseArtifact(a.Path, a.Module, a.CollectionDir)
			if err != nil {
				return nil, err
			}
			*a = *reparsed
			rel, _ := filepath.Rel(g.RepoRoot, a.Path)
			changed = append(changed, filepath.ToSlash(rel))
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// rewriteLegacyModelspecFrontmatter rewrites legacy modelspec tokens within the
// artifact's leading frontmatter block only (where model:/metadata/inputs
// references live), leaving the body untouched. It returns the possibly-updated
// content and whether any rewrite happened. Idempotent: a token already in the
// triple-slash or cross-repo authority form parses as valid and is left as-is.
func rewriteLegacyModelspecFrontmatter(content []byte) ([]byte, bool) {
	lines := strings.Split(string(content), "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 || strings.TrimSpace(lines[first]) != "---" {
		return content, false
	}
	end := -1
	for j := first + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			end = j
			break
		}
	}
	if end < 0 {
		return content, false
	}
	changed := false
	for i := first + 1; i < end; i++ {
		lines[i] = legacyModelspecTokenRe.ReplaceAllStringFunc(lines[i], func(tok string) string {
			if pe, ok := isLegacyForm(parseErr(tok)); ok {
				changed = true
				return pe.Rewrite
			}
			return tok
		})
	}
	if !changed {
		return content, false
	}
	return []byte(strings.Join(lines, "\n")), true
}

// parseErr returns the parse error for a modelspec token (nil on success).
func parseErr(tok string) error {
	_, err := ParseModelspecRef(tok)
	return err
}
