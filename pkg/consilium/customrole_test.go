package consilium

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRole writes body to <dir>/<name>.md and returns the file path. Used by
// custom-role tests so ParseCustomRole is exercised through real bytes on disk.
func writeRole(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s.md: %v", name, err)
	}
	return path
}

// validRoleBody is a conforming custom-role markdown file for slug
// `accessibility`: required body-metadata, a `## Role Prompt` H2, and a
// `## Example Vote` H2 carrying one valid vote in a fenced YAML block.
const validRoleBody = `# Accessibility

**Name:** accessibility
**Group:** customers
**Output Schema Version:** 1

## Role Prompt

You speak for users with disabilities. Weigh inclusive design.

## Example Vote

` + "```yaml" + `
- verdict: should-implement
  confidence: high
  cost: 🟢
  complexity: 🟡
  argument: Improves access for screen-reader users.
  role: accessibility
` + "```" + `
`

func TestParseCustomRole_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := writeRole(t, dir, "accessibility", validRoleBody)

	entry, err := ParseCustomRole(path)
	if err != nil {
		t.Fatalf("ParseCustomRole: %v", err)
	}
	if entry.Name != "accessibility" {
		t.Errorf("Name = %q, want accessibility", entry.Name)
	}
	if entry.Group != GroupCustomers {
		t.Errorf("Group = %q, want %q", entry.Group, GroupCustomers)
	}
	if entry.Path != path {
		t.Errorf("Path = %q, want %q", entry.Path, path)
	}
}

// AC:custom-role-missing-field-rejected — a file omitting the `**Group:**`
// metadata line must fail with an error naming the missing field (Group) and
// the file path.
func TestParseCustomRole_MissingGroupField(t *testing.T) {
	dir := t.TempDir()
	body := `# Accessibility

**Name:** accessibility
**Output Schema Version:** 1

## Role Prompt

Prompt.

## Example Vote

` + "```yaml" + `
- verdict: should-implement
  confidence: high
  cost: 🟢
  complexity: 🟡
  argument: ok
` + "```" + `
`
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for missing Group field, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"Group", path} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q; got: %s", want, msg)
		}
	}
}

func TestParseCustomRole_MissingFile(t *testing.T) {
	_, err := ParseCustomRole(filepath.Join(t.TempDir(), "nope.md"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseCustomRole_NameMismatchesFilename(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(validRoleBody, "**Name:** accessibility", "**Name:** a11y", 1)
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for name/filename mismatch, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"Name", path} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q; got: %s", want, msg)
		}
	}
}

func TestParseCustomRole_BadGroupEnum(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(validRoleBody, "**Group:** customers", "**Group:** stakeholders", 1)
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for out-of-enum group, got nil")
	}
	if !strings.Contains(err.Error(), "Group") {
		t.Errorf("error message missing Group; got: %s", err.Error())
	}
}

func TestParseCustomRole_BadSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(validRoleBody, "**Output Schema Version:** 1", "**Output Schema Version:** 2", 1)
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for bad schema version, got nil")
	}
	if !strings.Contains(err.Error(), "Output Schema Version") {
		t.Errorf("error message missing Output Schema Version; got: %s", err.Error())
	}
}

func TestParseCustomRole_MissingRolePromptSection(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(validRoleBody, "## Role Prompt", "## Persona", 1)
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for missing Role Prompt section, got nil")
	}
	if !strings.Contains(err.Error(), "Role Prompt") {
		t.Errorf("error message missing Role Prompt; got: %s", err.Error())
	}
}

func TestParseCustomRole_MissingExampleVoteSection(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(validRoleBody, "## Example Vote", "## Sample", 1)
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for missing Example Vote section, got nil")
	}
	if !strings.Contains(err.Error(), "Example Vote") {
		t.Errorf("error message missing Example Vote; got: %s", err.Error())
	}
}

func TestParseCustomRole_InvalidExampleVote(t *testing.T) {
	dir := t.TempDir()
	// Out-of-enum verdict in the example vote: ParseVotes must reject it.
	body := strings.Replace(validRoleBody, "verdict: should-implement", "verdict: maybe", 1)
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for invalid example vote, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, path) {
		t.Errorf("error message missing file path; got: %s", msg)
	}
}

func TestParseCustomRole_EmptyExampleVote(t *testing.T) {
	dir := t.TempDir()
	body := `# Accessibility

**Name:** accessibility
**Group:** customers
**Output Schema Version:** 1

## Role Prompt

Prompt.

## Example Vote

No fenced vote here.
`
	path := writeRole(t, dir, "accessibility", body)

	_, err := ParseCustomRole(path)
	if err == nil {
		t.Fatal("expected error for example-vote section with no vote, got nil")
	}
	if !strings.Contains(err.Error(), "Example Vote") {
		t.Errorf("error message missing Example Vote; got: %s", err.Error())
	}
}
