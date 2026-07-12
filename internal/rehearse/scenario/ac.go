package scenario

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// AC is a parsed thin acceptance criterion (`_acs/<slug>.ac.md`): a statement of
// intent — "what must be true" — with no verification inside (proof is a
// scenario's job). Feature: cli/rehearse/thin-acs.
type AC struct {
	// Slug is the id from the `# AC: <slug>` heading.
	Slug string
	// Statement is the prose under the `## Statement` heading (what must be true).
	Statement string
	// Verifies is the `**Verifies:**` URL reference to the feature/requirement.
	Verifies string
	// Status is the `**Status:**` value (e.g. "accepted", "draft").
	Status string
	// AppliesTo lists the `**Applies-to:**` tags (e.g. sign-in methods). Never nil.
	AppliesTo []string
	// Path is the file the AC was parsed from.
	Path string
}

var acHeading = regexp.MustCompile(`(?m)^#\s*AC:\s*(\S+)`)

// ParseAC reads and parses the thin-AC file at path.
func ParseAC(path string) (*AC, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading AC: %w", err)
	}
	return ParseACBytes(path, data)
}

// ParseACBytes parses AC content already in memory: the `# AC:` slug, the
// `**Verifies:**` / `**Status:**` / `**Applies-to:**` metadata, and the prose
// under the `## Statement` heading. A file with no `# AC:` heading is an error.
func ParseACBytes(path string, data []byte) (*AC, error) {
	ac := &AC{AppliesTo: []string{}, Path: path}
	if m := acHeading.FindSubmatch(data); m != nil {
		ac.Slug = string(m[1])
	} else {
		return nil, fmt.Errorf("%s: not an AC file (missing `# AC:` heading)", path)
	}

	var (
		inStatement bool
		statement   []string
	)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inStatement = strings.EqualFold(strings.TrimSpace(trimmed[3:]), "Statement")
			continue
		}
		if inStatement {
			statement = append(statement, line)
			continue
		}
		if v, ok := metaValue(trimmed, "Verifies"); ok && ac.Verifies == "" {
			ac.Verifies = v
		}
		if v, ok := metaValue(trimmed, "Status"); ok && ac.Status == "" {
			ac.Status = v
		}
		if v, ok := metaValue(trimmed, "Applies-to"); ok && len(ac.AppliesTo) == 0 {
			for _, t := range strings.Split(v, ",") {
				if t = strings.TrimSpace(t); t != "" {
					ac.AppliesTo = append(ac.AppliesTo, t)
				}
			}
		}
	}
	ac.Statement = strings.TrimSpace(strings.Join(statement, "\n"))
	return ac, nil
}

// GenerateACSummary renders the `## Acceptance Criteria` read-model table for a
// feature from its thin ACs: one row per AC with its slug (linked), one-line
// statement, verifies target, and status. This is a denormalized projection of
// the `_acs/` source of truth so a reader — human or agent — gets every AC's
// intent inline, in one read, without opening each file. ACs are sorted by slug
// for a stable, drift-free output. Feature: cli/rehearse/thin-acs.
func GenerateACSummary(acs []AC) string {
	sorted := append([]AC(nil), acs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })

	var b strings.Builder
	b.WriteString("## Acceptance Criteria\n\n")
	b.WriteString("<!-- generated from _acs/ by `specscore rehearse acs --write`; do not edit by hand -->\n\n")
	b.WriteString("| AC | Statement | Verifies | Status |\n")
	b.WriteString("|----|-----------|----------|--------|\n")
	for _, ac := range sorted {
		fmt.Fprintf(&b, "| [%s](_acs/%s.ac.md) | %s | %s | %s |\n",
			ac.Slug, ac.Slug, oneLine(ac.Statement), dashIfEmpty(ac.Verifies), dashIfEmpty(ac.Status))
	}
	return b.String()
}

// oneLine collapses statement prose to a single table-safe line.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
