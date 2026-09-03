package rule

import (
	"fmt"
	"strings"
)

// FieldEdit is one in-place bold-field rewrite of a detail document. Value is
// written verbatim after `**<Name>:** `, so callers normalize it first.
type FieldEdit struct {
	Name  string
	Value string
}

// detailFieldPosition returns a field's index in the canonical order, or -1.
func detailFieldPosition(name string) int {
	for i, f := range DetailFields {
		if f == name {
			return i
		}
	}
	return -1
}

// ApplyFieldEdits rewrites the named bold fields of a rule detail document in
// place, preserving every other byte — comments, worked examples, hand-written
// Open Questions, section order, trailing whitespace. Nothing is regenerated
// from a template, because an update that reformatted the whole document would
// silently discard an author's examples and make `rule update` unsafe to run on
// a reviewed rule.
//
// When **Status:** is edited, the frontmatter `status:` mirror is rewritten
// with it, so the status-mirror lint rule can never be broken by an update.
//
// A named field absent from the document is inserted directly after the last
// present field that precedes it in DetailFields order; that keeps the
// canonical ordering intact for a document written before the field existed.
func ApplyFieldEdits(content []byte, edits []FieldEdit) ([]byte, error) {
	if len(edits) == 0 {
		return content, nil
	}
	for _, e := range edits {
		if !detailFieldSet[e.Name] {
			return nil, fmt.Errorf("field %q is not a Rule metadata field", e.Name)
		}
	}
	lines := strings.Split(string(content), "\n")

	for _, edit := range edits {
		replaced := false
		for i, raw := range lines {
			name, _, ok := matchBoldField(strings.TrimSpace(raw))
			if !ok || name != edit.Name {
				continue
			}
			lines[i] = fmt.Sprintf("**%s:** %s", edit.Name, edit.Value)
			replaced = true
			break
		}
		if !replaced {
			at, err := insertionPointFor(lines, edit.Name)
			if err != nil {
				return nil, err
			}
			line := fmt.Sprintf("**%s:** %s", edit.Name, edit.Value)
			lines = append(lines, "")
			copy(lines[at+1:], lines[at:])
			lines[at] = line
		}
		if edit.Name == "Status" {
			rewriteFrontmatterStatus(lines, edit.Value)
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// insertionPointFor returns the line index a missing field should be inserted
// at: immediately after the last present field that precedes it in the
// canonical order, falling back to immediately after the `# Rule:` title.
func insertionPointFor(lines []string, name string) (int, error) {
	position := detailFieldPosition(name)
	if position < 0 {
		return 0, fmt.Errorf("field %q is not a Rule metadata field", name)
	}
	present := map[string]int{}
	titleLine := -1
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if titleLine < 0 && strings.HasPrefix(trimmed, "# Rule:") {
			titleLine = i
		}
		if fieldName, _, ok := matchBoldField(trimmed); ok {
			if _, seen := present[fieldName]; !seen {
				present[fieldName] = i
			}
		}
	}
	for i := position - 1; i >= 0; i-- {
		if at, ok := present[DetailFields[i]]; ok {
			return at + 1, nil
		}
	}
	// No preceding field exists: place it after any following field's
	// predecessor slot, i.e. before the first present required field.
	for i := position + 1; i < len(DetailFields); i++ {
		if at, ok := present[DetailFields[i]]; ok {
			return at, nil
		}
	}
	if titleLine >= 0 {
		return titleLine + 2, nil
	}
	return 0, fmt.Errorf("cannot place **%s:**: the document has no `# Rule:` title", name)
}

// rewriteFrontmatterStatus updates the leading frontmatter `status:` key in
// place. It only touches the first `status:` line inside the leading `---`
// block, so a body line that happens to begin with `status:` is never rewritten.
func rewriteFrontmatterStatus(lines []string, value string) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return
	}
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" {
			return
		}
		if strings.HasPrefix(trimmed, "status:") {
			lines[i] = "status: " + value
			return
		}
	}
}

// MergeSources applies --add-source / --remove-source to an existing list,
// preserving order, rejecting duplicates and unknown removals. Both inputs are
// validated as source references first, so a typo can never silently no-op.
func MergeSources(current, add, remove []string) ([]string, error) {
	if _, err := ParseSources(add); err != nil {
		return nil, err
	}
	if _, err := ParseSources(remove); err != nil {
		return nil, err
	}
	out := append([]string(nil), current...)
	for _, value := range remove {
		value = strings.TrimSpace(value)
		found := -1
		for i, existing := range out {
			if existing == value {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("source %q is not listed on this rule", value)
		}
		out = append(out[:found], out[found+1:]...)
	}
	for _, value := range add {
		value = strings.TrimSpace(value)
		duplicate := false
		for _, existing := range out {
			if existing == value {
				duplicate = true
				break
			}
		}
		if duplicate {
			return nil, fmt.Errorf("source %q is already listed on this rule", value)
		}
		out = append(out, value)
	}
	return out, nil
}
