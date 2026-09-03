package rule

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Detail is a parsed rule detail document at spec/rules/<slug>/README.md.
//
// Every `<Field>Line` is the 1-based source line of that bold field, or 0 when
// the field is absent — the same shape pkg/lesson uses, so a lint violation can
// point at a line rather than a whole file.
type Detail struct {
	Path string
	Slug string

	HasRuleTitle bool
	TitleLine    int
	Title        string

	Status     string
	StatusLine int

	Date     string
	DateLine int

	Owner     string
	OwnerLine int

	Statement     string
	StatementLine int

	// ScopesRaw holds every value written on a **Scope:** line, comma-split, in
	// source order. Both repeated lines and one comma-separated line parse the
	// same; the CLI always writes one line.
	ScopesRaw []string
	ScopeText string
	ScopeLine int

	Enforcement     string
	EnforcementLine int

	Control     string
	ControlLine int

	SourcesRaw  []string
	SourcesText string
	SourcesLine int

	Why     string
	WhyLine int

	Exceptions     string
	ExceptionsLine int

	Supersedes     string
	SupersedesLine int

	SupersededBy     string
	SupersededByLine int

	FrontmatterStatus     string
	FrontmatterStatusLine int

	// FieldCounts records how many times each bold field name appeared, so a
	// duplicated field is reportable rather than a silent last-one-wins.
	FieldCounts map[string]int
	// FieldOrder lists the canonical fields in the order they were found.
	FieldOrder []string

	SectionLines    map[string]int
	SubsectionLines map[string]int

	// SkillRefs are the `skill:<name>` references the document's body carries,
	// deduplicated and sorted. They are the rule half of the rule<->skill pair.
	SkillRefs []string
}

// HasSection reports whether title is present as an H2 heading.
func (d *Detail) HasSection(title string) bool {
	_, ok := d.SectionLines[title]
	return ok
}

// MissingSections returns the required H2 headings absent from the body.
func (d *Detail) MissingSections() []string {
	var missing []string
	for _, req := range DetailSections {
		if !d.HasSection(req) {
			missing = append(missing, req)
		}
	}
	return missing
}

// MissingExampleSubsections returns the required H3 headings absent from
// `## Examples`.
func (d *Detail) MissingExampleSubsections() []string {
	var missing []string
	for _, req := range ExampleSubsections {
		if _, ok := d.SubsectionLines[req]; !ok {
			missing = append(missing, req)
		}
	}
	return missing
}

// Scopes parses the raw scope list.
func (d *Detail) Scopes() ([]Scope, error) { return ParseScopes(d.ScopesRaw) }

// Sources parses the raw source list.
func (d *Detail) Sources() ([]SourceRef, error) { return ParseSources(d.SourcesRaw) }

// HasControl reports whether the document names a real control.
func (d *Detail) HasControl() bool { return isRealValue(d.Control) }

// MirroredValues projects the document's copy of the row-owned fields, so the
// mirror check compares like with like.
func (d *Detail) MirroredValues() map[string]string {
	return map[string]string{
		"Status":      strings.TrimSpace(d.Status),
		"Statement":   collapseWhitespace(d.Statement),
		"Scope":       joinOrSentinel(d.ScopesRaw),
		"Enforcement": strings.TrimSpace(d.Enforcement),
		"Control":     sentinelOr(collapseWhitespace(d.Control)),
		"Sources":     joinOrSentinel(d.SourcesRaw),
	}
}

// MirroredValuesOf projects a row into the same shape, for comparison.
func MirroredValuesOf(row Row) map[string]string {
	return map[string]string{
		"Status":      strings.TrimSpace(unescapeCell(row.Status)),
		"Statement":   collapseWhitespace(unescapeCell(row.Statement)),
		"Scope":       joinOrSentinel(row.ScopeList()),
		"Enforcement": strings.TrimSpace(unescapeCell(row.Enforcement)),
		"Control":     sentinelOr(collapseWhitespace(unescapeCell(row.Control))),
		"Sources":     joinOrSentinel(row.SourceList()),
	}
}

// boldFieldRe matches `**Name:** value` at the start of a trimmed line.
var boldFieldRe = regexp.MustCompile(`^\*\*([^*]+?):\*\*\s*(.*)$`)

func matchBoldField(line string) (name, value string, ok bool) {
	m := boldFieldRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), true
}

// inlineControlRe captures the `(control: …)` suffix an author may write on the
// **Enforcement:** line instead of filling **Control:** separately. Accepting it
// keeps a hand-written document usable; the canonical form always splits the
// tier from the control.
var inlineControlRe = regexp.MustCompile(`(?i)^(.*?)\s*\(\s*control\s*:\s*(.+?)\s*\)\s*$`)

func splitInlineControl(value string) (tier, control string) {
	m := inlineControlRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return strings.TrimSpace(value), ""
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
}

// skillRefRe matches a `skill:<name>` reference anywhere in the body.
var skillRefRe = regexp.MustCompile(`\bskill:([a-z0-9]+(?:-[a-z0-9]+)*)\b`)

// joinWrappedFieldValue extends a bold field's value across hand-wrapped
// continuation lines, joined with a single space. A **Why:** or **Statement:**
// routinely soft-wraps; reading only the first physical line is exactly the
// mid-sentence truncation that made index rows unreadable before c910c04, so
// this package joins from the outset. A continuation stops at the first blank
// line, the next heading, the next bold field, or a horizontal rule.
func joinWrappedFieldValue(lines []string, start int, first string) (string, int) {
	parts := make([]string, 0, 2)
	if first != "" {
		parts = append(parts, first)
	}
	consumed := 0
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			break
		}
		if _, _, ok := matchBoldField(trimmed); ok {
			break
		}
		parts = append(parts, trimmed)
		consumed++
	}
	return strings.Join(parts, " "), consumed
}

// appendFieldText accumulates the raw text of a repeatable field across its
// lines, so an author who wrote two `**Scope:**` lines still yields one value
// lint can test for emptiness.
func appendFieldText(current, value string) string {
	value = strings.TrimSpace(value)
	switch {
	case current == "":
		return value
	case value == "":
		return current
	default:
		return current + ", " + value
	}
}

// detailFieldSet is the membership test FieldOrder uses to ignore
// non-canonical bold fields an author may have added.
var detailFieldSet = func() map[string]bool {
	m := make(map[string]bool, len(DetailFields))
	for _, f := range DetailFields {
		m[f] = true
	}
	return m
}()

// ParseDetail reads a candidate rule detail document. It returns a populated
// Detail even when the file is not actually one (HasRuleTitle == false), so
// callers can tell "not a rule document" from "malformed rule document".
func ParseDetail(path string) (*Detail, error) {
	f, err := osOpen(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	d := &Detail{
		Path:            path,
		Slug:            filepath.Base(filepath.Dir(path)),
		SectionLines:    map[string]int{},
		SubsectionLines: map[string]int{},
		FieldCounts:     map[string]int{},
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// inlineControl holds a control written as `Enforced (control: …)`. It is a
	// fallback only: an explicit **Control:** always wins, so the two spellings
	// can never disagree silently.
	var inlineControl string
	skills := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		for _, m := range skillRefRe.FindAllStringSubmatch(trimmed, -1) {
			skills[m[1]] = true
		}

		if strings.HasPrefix(trimmed, "status:") && d.FrontmatterStatusLine == 0 {
			d.FrontmatterStatus = strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
			d.FrontmatterStatusLine = i + 1
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "# "); !d.HasRuleTitle && ok {
			if title, ok := strings.CutPrefix(rest, "Rule:"); ok {
				d.HasRuleTitle = true
				d.TitleLine = i + 1
				d.Title = strings.TrimSpace(title)
			}
			continue
		}
		if title, ok := strings.CutPrefix(trimmed, "### "); ok {
			t := strings.TrimSpace(title)
			if _, seen := d.SubsectionLines[t]; !seen {
				d.SubsectionLines[t] = i + 1
			}
			continue
		}
		if title, ok := strings.CutPrefix(trimmed, "## "); ok {
			t := strings.TrimSpace(title)
			if _, seen := d.SectionLines[t]; !seen {
				d.SectionLines[t] = i + 1
			}
			continue
		}
		name, value, ok := matchBoldField(trimmed)
		if !ok {
			continue
		}
		d.FieldCounts[name]++
		if detailFieldSet[name] && d.FieldCounts[name] == 1 {
			d.FieldOrder = append(d.FieldOrder, name)
		}
		switch name {
		case "Status":
			d.Status, d.StatusLine = value, i+1
		case "Date":
			d.Date, d.DateLine = value, i+1
		case "Owner":
			d.Owner, d.OwnerLine = value, i+1
		case "Statement":
			d.StatementLine = i + 1
			joined, consumed := joinWrappedFieldValue(lines, i, value)
			d.Statement = joined
			i += consumed
		case "Scope":
			if d.ScopeLine == 0 {
				d.ScopeLine = i + 1
			}
			d.ScopeText = appendFieldText(d.ScopeText, value)
			d.ScopesRaw = append(d.ScopesRaw, splitList(value)...)
		case "Enforcement":
			d.EnforcementLine = i + 1
			tier, control := splitInlineControl(value)
			d.Enforcement = tier
			inlineControl = control
		case "Control":
			d.ControlLine = i + 1
			joined, consumed := joinWrappedFieldValue(lines, i, value)
			d.Control = joined
			i += consumed
		case "Sources":
			if d.SourcesLine == 0 {
				d.SourcesLine = i + 1
			}
			d.SourcesText = appendFieldText(d.SourcesText, value)
			d.SourcesRaw = append(d.SourcesRaw, splitList(value)...)
		case "Why":
			d.WhyLine = i + 1
			joined, consumed := joinWrappedFieldValue(lines, i, value)
			d.Why = joined
			i += consumed
		case "Exceptions":
			d.ExceptionsLine = i + 1
			joined, consumed := joinWrappedFieldValue(lines, i, value)
			d.Exceptions = joined
			i += consumed
		case "Supersedes":
			d.Supersedes, d.SupersedesLine = value, i+1
		case "Superseded By":
			d.SupersededBy, d.SupersededByLine = value, i+1
		}
	}

	if !isRealValue(d.Control) && inlineControl != "" {
		d.Control = inlineControl
	}
	for name := range skills {
		d.SkillRefs = append(d.SkillRefs, name)
	}
	sort.Strings(d.SkillRefs)

	return d, nil
}

// DiscoverDetails parses every rule detail document under rulesDir, sorted by
// slug. A missing directory is not an error: it yields an empty set, so every
// read verb works in a repository that has recorded no rule yet.
func DiscoverDetails(rulesDir string) ([]*Detail, error) {
	entries, err := osReadDir(rulesDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Detail
	for _, e := range entries {
		if !e.IsDir() || ValidateSlug(e.Name()) != nil {
			continue
		}
		path := DetailPath(rulesDir, e.Name())
		if _, statErr := osStat(path); statErr != nil {
			continue
		}
		d, parseErr := ParseDetail(path)
		if parseErr != nil {
			return nil, parseErr
		}
		if !d.HasRuleTitle {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// DetailsBySlug is DiscoverDetails keyed by slug.
func DetailsBySlug(rulesDir string) (map[string]*Detail, error) {
	details, err := DiscoverDetails(rulesDir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*Detail, len(details))
	for _, d := range details {
		out[d.Slug] = d
	}
	return out, nil
}
