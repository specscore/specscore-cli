package rule

import (
	"sort"
	"strings"
)

// RenderEntry is one line of a `rule render` progressive-discovery listing:
// the trigger phrase that identifies the known problem, the rule it points
// to, and the optional Section it groups under.
type RenderEntry struct {
	Trigger string
	Slug    string
	Section string
}

// ResolveTrigger implements the fallback chain a rendered entry's trigger
// text follows, in order:
//
//  1. The detail document's own **Trigger:** header line, when present.
//  2. The first line of its `## Instructions` section, when that line is
//     written `Trigger: <text>` — the shape `sneat-co/backstage`'s rule tree
//     already uses, so a document authored there needs no edit to render.
//  3. The row's Statement, cut at its first comma, semicolon, or colon — the
//     clause that names the situation, before the sentence gets normative.
//
// detail is nil for an inline rule, which has no header or Instructions to
// fall back to and renders from its Statement alone.
func ResolveTrigger(statement string, detail *Detail) string {
	if detail != nil {
		if isRealValue(detail.Trigger) {
			return strings.TrimSpace(detail.Trigger)
		}
		if rest, ok := strings.CutPrefix(strings.TrimSpace(detail.InstructionsFirstLine), "Trigger: "); ok {
			if isRealValue(rest) {
				return strings.TrimSpace(rest)
			}
		}
	}
	return firstClause(statement)
}

// firstClause returns s up to (not including) its first comma, semicolon, or
// colon, trimmed. A statement with none of those is returned whole.
func firstClause(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, ",;:"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// BuildRenderEntries resolves each row's trigger and section, sorted stably
// by Section then Trigger then Slug — a rule with no Section sorts by Trigger
// alone once every entry with one is interleaved by the empty-string key, so
// a tree where no rule carries a Section reduces to a plain trigger sort.
// rows is expected to already be filtered to the rules the caller wants
// rendered (status and scope); details maps a slug to its parsed detail
// document, absent for an inline rule.
func BuildRenderEntries(rows []Row, details map[string]*Detail) []RenderEntry {
	out := make([]RenderEntry, 0, len(rows))
	for _, row := range rows {
		d := details[row.Slug]
		entry := RenderEntry{
			Trigger: ResolveTrigger(unescapeCell(row.Statement), d),
			Slug:    row.Slug,
		}
		if d != nil {
			entry.Section = strings.TrimSpace(d.Section)
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		if out[i].Trigger != out[j].Trigger {
			return out[i].Trigger < out[j].Trigger
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// RenderLine renders one entry in the canonical `- <trigger> → rule:<slug>`
// shape, shared by the text and Markdown writers.
func (e RenderEntry) RenderLine() string {
	return "- " + e.Trigger + " → rule:" + e.Slug
}
