package scaffold

import (
	"strings"
)

// Generate assembles the full scaffold markdown file content for a Rehearse
// scenario. It combines the feature/AC slugs and extracted Given/When/Then
// text into a seven-part structure: YAML frontmatter, title heading, metadata
// block, scenario source line, extracted text, and placeholder bash block.
// The returned string has no trailing newline.
func Generate(featureSlug, acSlug string, extracted *ExtractedAC) string {
	var b strings.Builder

	// 1. YAML frontmatter.
	b.WriteString("---\n")
	b.WriteString("format: https://specscore.md/scenario-specification\n")
	b.WriteString("---\n")

	// 2. Title heading: convert acSlug from kebab-case to title case.
	b.WriteString("\n# Rehearse: ")
	b.WriteString(kebabToTitle(acSlug))
	b.WriteString("\n")

	// 3. Metadata block.
	b.WriteString("\n**Status:** pending\n")
	b.WriteString("**Verifies:** ")
	b.WriteString(featureSlug)
	b.WriteString("#ac:")
	b.WriteString(acSlug)
	b.WriteString("\n")

	// 4. Scenario source comment.
	b.WriteString("\nScenario source: [../README.md](../README.md) → `### AC: ")
	b.WriteString(acSlug)
	b.WriteString("`.\n")

	// 5. Extracted Given/When/Then text.
	b.WriteString("\n")
	b.WriteString(strings.Join(extracted.Text, "\n"))
	b.WriteString("\n")

	// 6. Step placeholder block.
	b.WriteString("\n### Step: [TODO — implement the scenario steps]\n")
	b.WriteString("\nA placeholder bash block with guidance:\n")
	b.WriteString("\n```bash\n")
	b.WriteString("# TODO: Implement steps to verify the scenario's Given/When/Then.\n")
	b.WriteString("# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse run ...).\n")
	b.WriteString("# Assert exit codes and output as needed.\n")
	b.WriteString("echo \"TODO: implement scenario steps\"\n")
	b.WriteString("```")

	return b.String()
}

// kebabToTitle converts a kebab-case slug to title case.
// Each word separated by "-" is capitalised with its first letter upper-cased.
// Example: "resolve-ac-reference" → "Resolve Ac Reference".
func kebabToTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
