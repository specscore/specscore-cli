// Package sidekick scaffolds lint-clean sidekick-seed artifacts — the
// scaled-down Idea one-pagers parked under spec/ideas/seeds/.
//
// Features implemented: cli/sidekick/new
package sidekick

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxOneLinerChars caps the trimmed one-liner length
	// (cli/sidekick/new#req:one-liner-length).
	MaxOneLinerChars = 500
	// MaxBodyChars caps the body region (H1 line through end of optional
	// content) at capture. A freshly captured seed is `status: queued`, so
	// this matches the sidekick-seed lint rule's OPEN (queued) hard cap
	// (cli/sidekick/new#req:optional-body). Authors are advised (via a lint
	// warning) to keep a queued seed under 2500.
	MaxBodyChars = 3000
	// MaxSlugChars caps the derived slug length
	// (cli/sidekick/new#req:slug-derivation).
	MaxSlugChars = 60
)

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateSlug returns nil when slug is a lowercase, hyphen-separated, URL-safe
// identifier with no `/` — the same shape DeriveSlug produces. It is used to
// validate a caller-supplied `--slug` override (cli/sidekick/new#req:slug-override).
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug must not be empty")
	}
	if len(slug) > MaxSlugChars {
		return fmt.Errorf("slug too long (%d chars); max is %d", len(slug), MaxSlugChars)
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase, hyphen-separated, and URL-safe (no '/')", slug)
	}
	return nil
}

// DeriveSlug turns a one-liner into a lowercase, hyphen-separated, URL-safe
// slug, mirroring the specstudio:sidekick skill's algorithm so the CLI and
// the skill produce identical slugs for identical input
// (cli/sidekick/new#req:slug-derivation). It returns an error when the
// one-liner contains no slug-safe characters.
func DeriveSlug(oneLiner string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(oneLiner))
	s = nonSlugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > MaxSlugChars {
		s = s[:MaxSlugChars]
		if i := strings.LastIndex(s, "-"); i > 0 {
			s = s[:i]
		}
		s = strings.Trim(s, "-")
	}
	if s == "" {
		return "", fmt.Errorf("one-liner has no slug-safe characters")
	}
	return s, nil
}

// ScaffoldOptions controls the lint-clean seed file Scaffold emits.
type ScaffoldOptions struct {
	OneLiner string // the H1 text; trimmed before use
	// CapturedBy defaults to "user" when empty.
	CapturedBy string
	// Body is optional markdown appended after the H1 line.
	Body string
}

// Scaffold returns a lint-clean sidekick-seed file body: the minimal
// `{captured_by, status}` frontmatter the `sidekick-seed` lint rule enforces,
// followed by an H1 whose heading is the one-liner verbatim and any optional
// body. The `type: sidekick-seed` key is NOT written here — captured seeds in
// spec/ideas/seeds/ are identified by location; `type` is added only when a
// seed is archived. It validates the one-liner and body cap, returning an
// error on violation.
func Scaffold(opts ScaffoldOptions) ([]byte, error) {
	oneLiner := strings.TrimSpace(opts.OneLiner)
	if oneLiner == "" {
		return nil, fmt.Errorf("one-liner must not be empty")
	}
	if len(oneLiner) > MaxOneLinerChars {
		return nil, fmt.Errorf("one-liner too long (%d chars); max is %d", len(oneLiner), MaxOneLinerChars)
	}

	capturedBy := strings.TrimSpace(opts.CapturedBy)
	if capturedBy == "" {
		capturedBy = "user"
	}

	var front strings.Builder
	front.WriteString("---\n")
	front.WriteString("captured_by: " + capturedBy + "\n")
	front.WriteString("status: queued\n")
	front.WriteString("---\n")

	var body strings.Builder
	body.WriteString("# " + oneLiner + "\n")
	if extra := strings.TrimRight(opts.Body, "\n"); strings.TrimSpace(extra) != "" {
		body.WriteString("\n" + extra + "\n")
	}
	if body.Len() > MaxBodyChars {
		return nil, fmt.Errorf("body too long (%d chars); max (incl. H1 line) is %d", body.Len(), MaxBodyChars)
	}

	return []byte(front.String() + body.String()), nil
}
