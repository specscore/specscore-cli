package lint

import (
	"fmt"
	"path/filepath"
	"strings"
)

// formatFieldChecker enforces artifact-frontmatter-convention REQ:format-field
// and REQ:lint-format-required: every SpecScore artifact MUST carry a `format:`
// frontmatter field whose value is the canonical spec URL for its type.
//
// It reuses docTypeTargets — the same per-type URL + walker registry the
// adherence-footer check uses — because, by REQ:footer-format-mirror, the
// footer URL and the frontmatter `format:` URL are the same URL for a type.
//
// The rule is enforced at "error" severity. The grace period
// (REQ:migration-sequencing) has ended for this repo: every artifact has been
// migrated by `specscore spec migrate` to carry `format:`, and the create verbs
// emit it on new artifacts.
type formatFieldChecker struct{}

func newFormatFieldChecker() checker { return &formatFieldChecker{} }

func (c *formatFieldChecker) name() string     { return "format-field" }
func (c *formatFieldChecker) severity() string { return "error" }

func (c *formatFieldChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	for _, t := range docTypeTargets {
		target := t
		err := target.walk(specRoot, func(path string, content []byte) {
			fields, present := parseLeadingFrontmatter(content)
			got := ""
			if present {
				got = fields["format"]
			}
			if formatURLMatches(got, target.url) {
				return
			}
			relPath, _ := filepath.Rel(specRoot, path)
			var msg string
			if got == "" {
				msg = fmt.Sprintf("missing required frontmatter field `format: %s` on %s", target.url, target.description)
			} else {
				msg = fmt.Sprintf("frontmatter `format: %s` on %s does not match the canonical URL %q", got, target.description, target.url)
			}
			violations = append(violations, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "format-field",
				Message:  msg,
			})
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// formatURLMatches reports whether got equals want, tolerating an optional
// trailing slash on either side (per adherence-footer REQ:trailing-slash). An
// empty got never matches a (non-empty) canonical want.
func formatURLMatches(got, want string) bool {
	return strings.TrimSuffix(got, "/") == strings.TrimSuffix(want, "/")
}
