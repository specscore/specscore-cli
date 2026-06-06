package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// statusMirrorChecker enforces artifact-frontmatter-convention REQ:status-field,
// REQ:body-canonical and REQ:lint-status-mirror for status-bearing types: the
// frontmatter `status:` field MUST be present and MUST mirror the body
// `**Status:**` token (compared case-insensitively, whitespace-trimmed). The
// body line is canonical; `specscore spec lint --fix` writes or rewrites the
// frontmatter `status:` from the body value, never the reverse.
//
// The check is keyed by type (docTypeTarget.statusBearing), not file location
// (REQ:status-concept-by-type): a status-bearing type's mirror is enforced as
// above, while a status-less type (the *-index READMEs, scenarios, properties,
// entities) MUST NOT carry `status:` at all — its presence is a violation.
//
// Graced at "warning" severity during the migration rollout
// (REQ:migration-sequencing): `specscore spec lint` defaults to
// --severity=error and FilterBySeverity excludes "warning" violations at that
// level, so un-migrated repos are not broken on landing. The severity flips to
// "error" once the target repos are migrated.
type statusMirrorChecker struct{}

func newStatusMirrorChecker() checker { return &statusMirrorChecker{} }

func (c *statusMirrorChecker) name() string     { return "status-mirror" }
func (c *statusMirrorChecker) severity() string { return "error" }

// bodyStatusRe matches the canonical body status line `**Status:** <value>`.
// The bold markers distinguish it from the lowercase frontmatter `status:`
// mirror, which this rule must never read as the canonical value.
var bodyStatusRe = regexp.MustCompile(`^\*\*Status:\*\*\s*(.+)$`)

func (c *statusMirrorChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	for _, t := range docTypeTargets {
		target := t
		err := target.walk(specRoot, func(path string, content []byte) {
			var v Violation
			var ok bool
			if target.statusBearing {
				v, ok = statusMirrorViolation(specRoot, path, content, target.description)
			} else {
				v, ok = statusLessStatusViolation(specRoot, path, content, target.description)
			}
			if ok {
				violations = append(violations, v)
			}
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// statusLessStatusViolation flags a status-less artifact that carries a
// frontmatter `status:` field, which its type has no Status concept to mirror
// (REQ:status-field, REQ:lint-status-mirror case (b)). An artifact with no
// frontmatter block, or a block without a `status:` key, passes.
func statusLessStatusViolation(specRoot, path string, content []byte, description string) (Violation, bool) {
	fields, present := parseLeadingFrontmatter(content)
	if !present {
		return Violation{}, false
	}
	if _, ok := fields["status"]; !ok {
		return Violation{}, false
	}
	relPath, _ := filepath.Rel(specRoot, path)
	return Violation{
		File:     relPath,
		Line:     0,
		Severity: "error",
		Rule:     "status-mirror",
		Message:  fmt.Sprintf("status-less %s must not carry a frontmatter `status:` field (its type has no Status concept)", description),
	}, true
}

// statusMirrorViolation returns the mirror violation, if any, for one
// status-bearing artifact. When the body carries no `**Status:**` line there is
// nothing to mirror (the missing-body-status rule is enforced per type
// elsewhere), so the artifact is skipped.
func statusMirrorViolation(specRoot, path string, content []byte, description string) (Violation, bool) {
	body := extractBodyStatus(content)
	if body == "" {
		return Violation{}, false
	}
	fmStatus := frontmatterStatus(content)
	relPath, _ := filepath.Rel(specRoot, path)
	if fmStatus == "" {
		return Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "status-mirror",
			Message:  fmt.Sprintf("missing required frontmatter field `status: %s` on %s (mirror of body **Status:**)", body, description) + migrateHint,
		}, true
	}
	if !strings.EqualFold(strings.TrimSpace(fmStatus), strings.TrimSpace(body)) {
		return Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "status-mirror",
			Message:  fmt.Sprintf("frontmatter `status: %s` on %s does not mirror body **Status:** %q", fmStatus, description, body) + migrateHint,
		}, true
	}
	return Violation{}, false
}

// fix rewrites the frontmatter `status:` mirror from the body `**Status:**` for
// every status-bearing artifact whose mirror is missing or drifted *within an
// existing frontmatter block*. The body stays canonical; an artifact already in
// sync is untouched.
//
// An artifact with no frontmatter block at all is deliberately left alone:
// fabricating a block here would push the title off line 1 and break the
// structural rules (title-format, header-fields) that do not yet tolerate
// leading frontmatter. Bootstrapping blocks across a repo — and updating those
// rules to tolerate them — is the one-shot migration's job
// (artifact-frontmatter-convention REQ:migration-sequencing). The missing-mirror
// violation remains reported (at graced "warning" severity) until then.
func (c *statusMirrorChecker) fix(specRoot string) error {
	for _, t := range docTypeTargets {
		if !t.statusBearing {
			continue
		}
		target := t
		var writeErr error
		err := target.walk(specRoot, func(path string, content []byte) {
			if writeErr != nil {
				return
			}
			fields, present := parseLeadingFrontmatter(content)
			if !present {
				return
			}
			body := extractBodyStatus(content)
			if body == "" {
				return
			}
			fmStatus := fields["status"]
			if fmStatus != "" && strings.EqualFold(strings.TrimSpace(fmStatus), strings.TrimSpace(body)) {
				return
			}
			updated := setFrontmatterStatus(content, body)
			if err := os.WriteFile(path, updated, 0o644); err != nil {
				writeErr = err
			}
		})
		if err != nil {
			return err
		}
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// frontmatterStatus returns the leading frontmatter `status:` value, or "" when
// there is no frontmatter block or no `status:` key.
func frontmatterStatus(content []byte) string {
	fields, present := parseLeadingFrontmatter(content)
	if !present {
		return ""
	}
	return fields["status"]
}

// extractBodyStatus returns the canonical body `**Status:**` value (with any
// surrounding backticks/quotes trimmed), or "" when the body has no status
// line. The leading frontmatter block is skipped so the lowercase frontmatter
// mirror is never read as the body value.
func extractBodyStatus(content []byte) string {
	for _, raw := range strings.Split(bodyAfterFrontmatter(content), "\n") {
		line := strings.TrimSpace(raw)
		if m := bodyStatusRe.FindStringSubmatch(line); m != nil {
			return strings.Trim(strings.TrimSpace(m[1]), "`\"'")
		}
	}
	return ""
}

// setFrontmatterStatus returns content with the leading frontmatter `status:`
// field set to status: an existing `status:` line is rewritten in place,
// otherwise a new one is inserted before the closing fence. The caller
// guarantees a complete leading frontmatter block exists; content with no such
// block is returned unchanged (block creation is the migration's job, not this
// rule's — see fix).
func setFrontmatterStatus(content []byte, status string) []byte {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return content
	}
	return []byte(strings.Join(upsertFrontmatterField(lines, "status", status), "\n"))
}

// upsertFrontmatterField sets `key: value` within the leading frontmatter block
// (lines[0] == "---" up to the next "---"): an existing top-level `key:` line is
// rewritten in place, otherwise a new line is inserted just before the closing
// fence. Other lines are preserved. When there is no closing fence the lines are
// returned unchanged. The caller guarantees lines[0] opens a fence.
func upsertFrontmatterField(lines []string, key, value string) []string {
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") != "---" {
			continue
		}
		// lines[i] is the closing fence; lines[1:i] is the block body.
		for j := 1; j < i; j++ {
			line := strings.TrimRight(lines[j], "\r")
			if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
				continue
			}
			idx := strings.Index(line, ":")
			if idx <= 0 {
				continue
			}
			if strings.TrimSpace(line[:idx]) == key {
				lines[j] = key + ": " + value
				return lines
			}
		}
		inserted := append([]string{}, lines[:i]...)
		inserted = append(inserted, key+": "+value)
		inserted = append(inserted, lines[i:]...)
		return inserted
	}
	return lines
}
