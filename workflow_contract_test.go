package specscore_cli_test

import (
	"os"
	"strings"
	"testing"
)

func TestPublishedArtifactCallerUsesReadOnlyReusableWorkflow(t *testing.T) {
	workflow, err := os.ReadFile(".github/workflows/validate-published-artifact.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"permissions:\n  contents: read",
		"permissions:\n      contents: read",
		"uses: strongo/cicd/.github/workflows/validate-published-artifact.yml@v1",
		"release_tag: ${{ inputs.release_tag }}",
		"artifact_binary: specscore",
		"artifact_command: --version",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("published-artifact caller is missing read-only contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"strongo/cicd/.github/workflows/release.yml",
		"existing_artifact_tag",
		"artifact_smoke_test_homebrew_cask",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("published-artifact caller contains release-capable contract %q", forbidden)
		}
	}
}
