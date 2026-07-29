package lint

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
)

// migrateHint is appended to the frontmatter-rule violations that a one-shot
// `specscore migrate` run resolves, so an upgrading user who hits the
// now-enforced rules is pointed at the fix.
const migrateHint = " (run `specscore migrate` to backfill)"

// Migrate performs the one-shot artifact-frontmatter-convention backfill
// (cli/spec/migrate): for every artifact the convention rules walk, it ensures
// a leading frontmatter block carrying the type's canonical `format:` URL and,
// for status-bearing types, a `status:` mirroring the body `**Status:**`; it
// then aligns the adherence-footer URL to `format:`. It is deterministic,
// offline, and idempotent (a conformant artifact is left byte-unchanged).
//
// Returns the spec-root-relative, slash-separated paths of the files it changed,
// sorted.
func Migrate(specRoot string) ([]string, error) {
	var changed []string
	for _, t := range docTypeTargets {
		target := t
		var writeErr error
		var bodyStatusErr error
		err := target.walk(specRoot, func(path string, content []byte) {
			if writeErr != nil || bodyStatusErr != nil {
				return
			}
			updated, err := migrateArtifact(path, content, target)
			if err != nil {
				bodyStatusErr = err
				return
			}
			if bytes.Equal(updated, content) {
				return
			}
			if err := writeLintFile(specRoot, path, content, updated, 0o644); err != nil {
				writeErr = err
				return
			}
			rel, _ := filepath.Rel(specRoot, path)
			changed = append(changed, filepath.ToSlash(rel))
		})
		if err != nil {
			return nil, err
		}
		if bodyStatusErr != nil {
			return nil, bodyStatusErr
		}
		if writeErr != nil {
			return nil, writeErr
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// migrateArtifact returns content with the convention frontmatter ensured and
// the footer aligned to `format:`, or an error when its canonical body-status
// reader cannot parse an artifact. Status-bearing types whose body carries a
// `**Status:**` gain a mirrored `status:`; status-less types get `format:` only.
func migrateArtifact(path string, content []byte, t docTypeTarget) ([]byte, error) {
	fields := [][2]string{{"format", t.url}}
	if t.statusBearing {
		bodyStatus, err := canonicalArtifactBodyStatus(path, content, t)
		if err != nil {
			return nil, err
		}
		if bodyStatus != "" {
			fields = append(fields, [2]string{"status", bodyStatus})
		}
	}
	out := ensureFrontmatter(content, fields)
	if footer := extractFooterURL(out); footer != "" && !formatURLMatches(footer, t.url) {
		out = replaceLastSpecURL(out, t.url)
	}
	return out, nil
}

// ensureFrontmatter returns content with the ordered key/value fields present in
// its leading frontmatter block. When a complete block exists each field is
// upserted (existing key rewritten, missing key inserted before the closing
// fence), preserving other lines. When no complete block exists, a fresh
// `---`-opened block carrying the fields is prepended. A complete existing
// block may use either `---` or `...` as its closer.
func ensureFrontmatter(content []byte, fields [][2]string) []byte {
	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && isLeadingFrontmatterFence(lines[0]) && hasClosingFence(lines) {
		for _, f := range fields {
			lines = upsertFrontmatterField(lines, f[0], f[1])
		}
		return []byte(strings.Join(lines, "\n"))
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, f := range fields {
		b.WriteString(f[0] + ": " + f[1] + "\n")
	}
	b.WriteString("---\n\n")
	b.Write(content)
	return []byte(b.String())
}

// hasClosingFence reports whether lines (whose first line opens a frontmatter
// fence) contain a later `---` or `...` closing fence.
func hasClosingFence(lines []string) bool {
	for i := 1; i < len(lines); i++ {
		if isFrontmatterFence(lines[i]) {
			return true
		}
	}
	return false
}
