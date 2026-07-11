package scaffold

import (
	"os"
	"strings"
	"testing"
)

// readGoldenFile is a seam for loading golden fixture files.
// Tests may override this to inject canned content.
var readGoldenFile = func(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func TestGenerate_GoldenFixture(t *testing.T) {
	extracted := &ExtractedAC{
		HasGWT: true,
		Text: []string{
			"Scenario: index two repos",
			"Given a studio.yaml with two repos",
			"When I run specscore studio index",
			"Then both repos are listed",
		},
	}

	golden, err := readGoldenFile("testdata/scaffold.golden")
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	got := Generate("cli/studio/index", "index-two-repos", extracted)
	if got != golden {
		t.Errorf("generated output does not match golden fixture.\nGot:\n%s\n\nWant:\n%s", got, golden)
	}
}

func TestGenerate_KebabToTitle(t *testing.T) {
	cases := []struct {
		slug  string
		title string
	}{
		{"resolve-ac-reference", "Resolve Ac Reference"},
		{"extract-given-when-then", "Extract Given When Then"},
		{"placeholder-bash-block", "Placeholder Bash Block"},
		// Empty segments (trailing/leading/double dash) must not panic on w[:1].
		{"trailing-dash-", "Trailing Dash "},
		{"double--dash", "Double  Dash"},
	}
	for _, c := range cases {
		extracted := &ExtractedAC{HasGWT: false, Text: []string{}}
		got := Generate("cli/rehearse/new", c.slug, extracted)
		wantHeading := "# Rehearse: " + c.title
		if !strings.Contains(got, wantHeading) {
			t.Errorf("slug %q: heading %q not found in output:\n%s", c.slug, wantHeading, got)
		}
	}
}

func TestGenerate_FormatConsistency(t *testing.T) {
	extracted := &ExtractedAC{
		HasGWT: true,
		Text: []string{
			"Given something",
			"When action",
			"Then result",
		},
	}

	got := Generate("my/feature", "my-ac-slug", extracted)

	// YAML frontmatter
	if !strings.HasPrefix(got, "---\nformat: https://specscore.md/scenario-specification\n---\n") {
		t.Errorf("output does not start with expected YAML frontmatter:\n%s", got)
	}

	// Heading
	if !strings.Contains(got, "# Rehearse: My Ac Slug") {
		t.Errorf("missing title heading in output:\n%s", got)
	}

	// Metadata block
	if !strings.Contains(got, "**Status:** pending") {
		t.Errorf("missing Status metadata in output:\n%s", got)
	}
	if !strings.Contains(got, "**Verifies:** my/feature#ac:my-ac-slug") {
		t.Errorf("missing Verifies metadata in output:\n%s", got)
	}

	// Scenario source line
	if !strings.Contains(got, "Scenario source: [../README.md](../README.md) → `### AC: my-ac-slug`.") {
		t.Errorf("missing scenario source line in output:\n%s", got)
	}

	// Extracted GWT text
	if !strings.Contains(got, "Given something") {
		t.Errorf("missing GWT text in output:\n%s", got)
	}

	// Placeholder bash block
	if !strings.Contains(got, "### Step: [TODO — implement the scenario steps]") {
		t.Errorf("missing step heading in output:\n%s", got)
	}
	if !strings.Contains(got, "```bash") {
		t.Errorf("missing bash block in output:\n%s", got)
	}
	if !strings.Contains(got, `echo "TODO: implement scenario steps"`) {
		t.Errorf("missing echo placeholder in output:\n%s", got)
	}

	// No trailing newline
	if strings.HasSuffix(got, "\n") {
		t.Errorf("output has a trailing newline, want none")
	}
}
