// Package sidekick scaffolds lint-clean sidekick-seed artifacts — the
// scaled-down Idea one-pagers parked under spec/ideas/seeds/.
//
// Features implemented: cli/sidekick/new
package sidekick

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// TypeValue is the literal `type:` frontmatter value every seed carries.
	TypeValue = "sidekick-seed"
	// MaxOneLinerChars caps the trimmed one-liner length
	// (cli/sidekick/new#req:one-liner-length).
	MaxOneLinerChars = 500
	// MaxBodyChars caps the body region (H1 line through end of optional
	// content) per the sidekick-seed lint rule
	// (cli/sidekick/new#req:optional-body).
	MaxBodyChars = 2000
	// MaxSlugChars caps the derived slug length
	// (cli/sidekick/new#req:slug-derivation).
	MaxSlugChars = 60
)

// TriggerValues enumerates the legal `trigger:` values
// (cli/sidekick/new#req:frontmatter-values).
var TriggerValues = []string{"heuristic", "explicit"}

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

// validTrigger reports whether t is one of the legal trigger values.
func validTrigger(t string) bool {
	for _, v := range TriggerValues {
		if t == v {
			return true
		}
	}
	return false
}

// ScaffoldOptions controls the lint-clean seed file Scaffold emits.
type ScaffoldOptions struct {
	Slug     string
	OneLiner string // the H1 text; trimmed before use
	// CapturedBy defaults to "user" when empty.
	CapturedBy string
	// CapturedDuring is written verbatim; empty renders the literal `null`.
	CapturedDuring string
	// Trigger defaults to "explicit" when empty; must be heuristic|explicit.
	Trigger string
	// Body is optional markdown appended after the H1 line.
	Body string
	// CapturedAt is an ISO-8601 timestamp; defaults to time.Now().UTC() when empty.
	CapturedAt string
}

// Scaffold returns a lint-clean sidekick-seed file body: the closed 8-key
// frontmatter the `sidekick-seed` lint rule enforces, followed by an H1 whose
// heading is the one-liner verbatim and any optional body. It validates the
// one-liner, trigger, and body cap, returning an error on violation.
func Scaffold(opts ScaffoldOptions) ([]byte, error) {
	oneLiner := strings.TrimSpace(opts.OneLiner)
	if oneLiner == "" {
		return nil, fmt.Errorf("one-liner must not be empty")
	}
	if len(oneLiner) > MaxOneLinerChars {
		return nil, fmt.Errorf("one-liner too long (%d chars); max is %d", len(oneLiner), MaxOneLinerChars)
	}
	if strings.TrimSpace(opts.Slug) == "" {
		return nil, fmt.Errorf("slug must not be empty")
	}

	capturedBy := strings.TrimSpace(opts.CapturedBy)
	if capturedBy == "" {
		capturedBy = "user"
	}
	trigger := strings.TrimSpace(opts.Trigger)
	if trigger == "" {
		trigger = "explicit"
	}
	if !validTrigger(trigger) {
		return nil, fmt.Errorf("trigger must be one of {%s}; got %q",
			strings.Join(TriggerValues, ", "), trigger)
	}
	capturedAt := strings.TrimSpace(opts.CapturedAt)
	if capturedAt == "" {
		capturedAt = time.Now().UTC().Format(time.RFC3339)
	}
	capturedDuring := strings.TrimSpace(opts.CapturedDuring)
	if capturedDuring == "" {
		capturedDuring = "null"
	}

	var front strings.Builder
	front.WriteString("---\n")
	front.WriteString("type: " + TypeValue + "\n")
	front.WriteString("slug: " + opts.Slug + "\n")
	front.WriteString("captured_at: " + capturedAt + "\n")
	front.WriteString("captured_by: " + capturedBy + "\n")
	front.WriteString("captured_during: " + capturedDuring + "\n")
	front.WriteString("trigger: " + trigger + "\n")
	front.WriteString("status: queued\n")
	front.WriteString("synchestra_task: null\n")
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
