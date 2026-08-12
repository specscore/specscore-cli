package lint

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// Legacy→canonical artifact-status maps. Each is a CLOSED set: only these exact
// tokens are auto-rewritten by `specscore spec lint --fix`; any other invalid
// value remains a plain error (correcting it needs human intent). The mappings
// are the legacy renames documented by the upstream status-vocabulary Feature.
//
// See spec/features/cli/spec/lint/legacy-status-autofix/README.md.
var (
	legacyPlanStatusMap = map[string]string{
		"Completed":    "Implemented",
		"Under Review": "In Review",
	}
	legacyDecisionStatusMap = map[string]string{
		"Accepted": "Approved",
		"Proposed": "In Review",
	}
	legacyIdeaStatusMap = map[string]string{
		"Archived": "Stale",
	}
)

// rewriteLegacyBodyStatus rewrites the first body `**Status:**` line whose
// (frontmatter-skipped) value is a key in legacyMap to the mapped canonical
// value, keeping any mirrored frontmatter `status:` in sync. It returns the
// (possibly unchanged) content and whether a rewrite happened. A value not in
// legacyMap — including free-form prose — leaves the content untouched.
func rewriteLegacyBodyStatus(content []byte, legacyMap map[string]string) ([]byte, bool) {
	canonical, ok := legacyMap[extractBodyStatus(content)]
	if !ok {
		return content, false
	}
	lines := strings.Split(string(content), "\n")
	for i := frontmatterEndIndex(lines); i < len(lines); i++ {
		if bodyStatusRe.MatchString(strings.TrimSpace(lines[i])) {
			lines[i] = "**Status:** " + canonical
			break
		}
	}
	out := []byte(strings.Join(lines, "\n"))
	return setFrontmatterStatus(out, canonical), true
}

// fixLegacyStatusesInTree walks every `.md` file under root (recursively,
// skipping README.md), applying rewriteLegacyBodyStatus with legacyMap. When
// ideaArchived is true, a successful rewrite also sets the `**Archived:** true`
// body flag (the orthogonal-archival half of the idea Archived→Stale
// migration). Idempotent: a second pass rewrites nothing.
func fixLegacyStatusesInTree(root string, legacyMap map[string]string, ideaArchived bool) error {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) == "README.md" || !strings.HasSuffix(path, ".md") {
			return nil
		}
		return lifecycle.TransformArtifact(path, func(content []byte) ([]byte, error) {
			out, changed := rewriteLegacyBodyStatus(content, legacyMap)
			if !changed {
				return content, nil
			}
			if ideaArchived {
				out = ensureArchivedFlag(out)
			}
			return out, nil
		})
	})
}

// ensureArchivedFlag guarantees a `**Archived:** true` body-metadata line: an
// existing `**Archived:**` line is set to true, otherwise a new line is
// inserted immediately after the body `**Status:**` line. Frontmatter is left
// untouched (archival is a body-flag axis).
func ensureArchivedFlag(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	start := frontmatterEndIndex(lines)
	statusIdx := -1
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if strings.HasPrefix(trimmed, "**Archived:**") {
			lines[i] = "**Archived:** true"
			return []byte(strings.Join(lines, "\n"))
		}
		if statusIdx == -1 && bodyStatusRe.MatchString(trimmed) {
			statusIdx = i
		}
	}
	if statusIdx == -1 {
		return content
	}
	inserted := append([]string{}, lines[:statusIdx+1]...)
	inserted = append(inserted, "**Archived:** true")
	inserted = append(inserted, lines[statusIdx+1:]...)
	return []byte(strings.Join(inserted, "\n"))
}
