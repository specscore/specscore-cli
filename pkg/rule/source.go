package rule

import (
	"fmt"
	"regexp"
	"strings"
)

// Source kinds. A Rule's **Sources:** list names the artifacts that produced
// it, so a reader can always get back to *why* — the Lesson whose process gap
// it closes, the Decision that chose it, the Idea that proposed it — without
// the Rule itself having to restate any of them.
const (
	SourceLesson   = "lesson"
	SourceDecision = "decision"
	SourceIdea     = "idea"
	SourceURL      = "url" // a free http(s) reference; never resolved locally
)

// SourceRef is one parsed entry of a Rule's **Sources:** list.
type SourceRef struct {
	Kind  string // one of the Source* constants
	Value string // the slug, decision id, or full URL
	Raw   string // the entry exactly as written
}

// String renders a SourceRef back to its canonical wire form.
func (s SourceRef) String() string {
	if s.Kind == SourceURL {
		return s.Value
	}
	return s.Kind + ":" + s.Value
}

// decisionRefRe matches the two decision-reference shapes the CLI already
// writes elsewhere: a bare four-digit sequence number (`0012`) or the full
// `NNNN-slug` filename stem.
var decisionRefRe = regexp.MustCompile(`^\d{4}(-[a-z0-9]+(-[a-z0-9]+)*)?$`)

// ParseSource parses one raw source token. An `http://` or `https://` prefix is
// a free URL; everything else MUST carry an explicit `<kind>:` prefix, so a
// bare word is rejected rather than silently filed under a guessed kind.
func ParseSource(raw string) (SourceRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return SourceRef{}, fmt.Errorf("source must not be empty")
	}
	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		return SourceRef{Kind: SourceURL, Value: trimmed, Raw: trimmed}, nil
	}
	kind, value, found := strings.Cut(trimmed, ":")
	if !found {
		return SourceRef{}, fmt.Errorf(
			"source %q must be %s:<slug>, %s:<id>, %s:<slug>, or an http(s) URL",
			trimmed, SourceLesson, SourceDecision, SourceIdea)
	}
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if value == "" {
		return SourceRef{}, fmt.Errorf("source %q is missing a value after %q", trimmed, kind+":")
	}
	switch kind {
	case SourceLesson, SourceIdea:
		if err := ValidateSlug(value); err != nil {
			return SourceRef{}, fmt.Errorf("source %q: %v", trimmed, err)
		}
	case SourceDecision:
		if !decisionRefRe.MatchString(value) {
			return SourceRef{}, fmt.Errorf("source %q: decision reference must be NNNN or NNNN-slug", trimmed)
		}
	default:
		return SourceRef{}, fmt.Errorf(
			"source %q has unknown kind %q (expected %s, %s, %s, or an http(s) URL)",
			trimmed, kind, SourceLesson, SourceDecision, SourceIdea)
	}
	return SourceRef{Kind: kind, Value: value, Raw: trimmed}, nil
}

// ParseSources parses every entry of a source list, reporting the first error.
func ParseSources(raws []string) ([]SourceRef, error) {
	out := make([]SourceRef, 0, len(raws))
	for _, raw := range raws {
		s, err := ParseSource(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// LessonSources returns just the lesson slugs a source list names, in order.
func LessonSources(sources []SourceRef) []string {
	var out []string
	for _, s := range sources {
		if s.Kind == SourceLesson {
			out = append(out, s.Value)
		}
	}
	return out
}

// PromotesToRef is the value a Lesson carries in its optional
// `**Promotes To:**` field when it has been promoted into a Rule.
func PromotesToRef(slug string) string { return "rule:" + slug }

// ParsePromotesTo extracts the rule slug from a Lesson's `**Promotes To:**`
// value. It returns ok=false for the empty value or the em-dash sentinel, and
// an error for a value that is present but not a `rule:<slug>` reference.
func ParsePromotesTo(value string) (slug string, ok bool, err error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == Sentinel || trimmed == "-" {
		return "", false, nil
	}
	rest, found := strings.CutPrefix(trimmed, "rule:")
	if !found {
		return "", false, fmt.Errorf("**Promotes To:** %q must be rule:<slug>", trimmed)
	}
	rest = strings.TrimSpace(rest)
	if err := ValidateSlug(rest); err != nil {
		return "", false, fmt.Errorf("**Promotes To:** %q: %v", trimmed, err)
	}
	return rest, true, nil
}
