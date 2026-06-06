package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// footerFormatMirrorChecker enforces artifact-frontmatter-convention
// REQ:footer-format-mirror: the frontmatter `format:` field and the
// adherence-footer URL MUST carry the same canonical spec URL for the type
// (trailing slash optional). The frontmatter `format:` is canonical for the
// pair — it is static and type-derived — so `specscore spec lint --fix` derives
// the footer URL from `format:`, never the reverse.
//
// The rule only acts when `format:` is present. A migrated artifact carries
// `format:`, giving a canonical value to mirror the footer against; until then
// the footer's own correctness is owned by adherence-footer (whose `--fix` is
// insert-only and leaves a wrong footer URL as a hard error). Confining this
// rule to the format-present case is what keeps the two footer fixers from
// fighting: in the migrated steady state `format:` equals the type URL, so
// deriving the footer from `format:` and from the type URL converge.
//
// Graced at "warning" severity during the migration rollout
// (REQ:migration-sequencing): excluded at the default --severity=error.
type footerFormatMirrorChecker struct{}

func newFooterFormatMirrorChecker() checker { return &footerFormatMirrorChecker{} }

func (c *footerFormatMirrorChecker) name() string     { return "footer-format-mirror" }
func (c *footerFormatMirrorChecker) severity() string { return "error" }

// specURLRe matches a canonical SpecScore spec URL (without a trailing slash).
var specURLRe = regexp.MustCompile(`https://specscore\.md/[a-z0-9-]+-specification`)

func (c *footerFormatMirrorChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	for _, t := range docTypeTargets {
		target := t
		err := target.walk(specRoot, func(path string, content []byte) {
			format, footer, ok := footerFormatPair(content)
			if !ok || formatURLMatches(footer, format) {
				return
			}
			relPath, _ := filepath.Rel(specRoot, path)
			violations = append(violations, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "footer-format-mirror",
				Message: fmt.Sprintf(
					"adherence-footer URL %q on %s does not match the canonical frontmatter `format: %s`",
					footer, target.description, format) + migrateHint,
			})
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// fix rewrites the adherence-footer URL from the frontmatter `format:` for every
// artifact whose footer and `format:` disagree. The frontmatter is canonical;
// the footer is the derivative. Artifacts missing either surface are left for
// the format-field and adherence-footer rules respectively.
func (c *footerFormatMirrorChecker) fix(specRoot string) error {
	for _, t := range docTypeTargets {
		target := t
		var writeErr error
		err := target.walk(specRoot, func(path string, content []byte) {
			if writeErr != nil {
				return
			}
			format, footer, ok := footerFormatPair(content)
			if !ok || formatURLMatches(footer, format) {
				return
			}
			updated := replaceLastSpecURL(content, format)
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

// footerFormatPair returns the frontmatter `format:` URL and the
// adherence-footer URL for content. ok is false unless both are present — the
// missing-field cases belong to the format-field and adherence-footer rules.
func footerFormatPair(content []byte) (format, footer string, ok bool) {
	fields, present := parseLeadingFrontmatter(content)
	if !present {
		return "", "", false
	}
	format = fields["format"]
	if format == "" {
		return "", "", false
	}
	footer = extractFooterURL(content)
	if footer == "" {
		return "", "", false
	}
	return format, footer, true
}

// extractFooterURL returns the adherence-footer's spec URL: the last spec URL in
// the body (the footer is the document's final meaningful line). The leading
// frontmatter block is skipped so the `format:` value is never read as the
// footer. Returns "" when the body carries no spec URL.
func extractFooterURL(content []byte) string {
	matches := specURLRe.FindAllString(bodyAfterFrontmatter(content), -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

// replaceLastSpecURL replaces the last spec URL occurrence in content with
// newURL. The footer is the final meaningful line, so its URL is the last
// occurrence in the whole document (the frontmatter `format:` precedes it).
func replaceLastSpecURL(content []byte, newURL string) []byte {
	s := string(content)
	locs := specURLRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return content
	}
	last := locs[len(locs)-1]
	return []byte(s[:last[0]] + newURL + s[last[1]:])
}
