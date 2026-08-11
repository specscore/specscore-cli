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

func TestReleaseCallerDefersCaskInstallSmokeUntilNotarization(t *testing.T) {
	workflow, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflowText := string(workflow)
	for _, required := range []string{
		"uses: strongo/cicd/.github/workflows/release.yml@v1",
		"release-artifact-smoke-darwin-arm64:",
		"artifact_smoke_test_homebrew_cask: false",
	} {
		if !strings.Contains(workflowText, required) {
			t.Fatalf("release caller is missing cask-smoke deferral contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"artifact_smoke_test: false",
		"require_notarized_macos: true",
	} {
		if strings.Contains(workflowText, forbidden) {
			t.Fatalf("release caller changed the pre-notarization contract %q", forbidden)
		}
	}

	goreleaser, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goreleaser), "homebrew_casks:") {
		t.Fatal("GoReleaser must retain Homebrew cask packaging while its install smoke is deferred")
	}
}

func TestMacOSInstallGuidanceFailsClosedUntilCaskVerification(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(readme)
	for _, required := range []string{
		"### macOS — source build (current channel)",
		"go install github.com/specscore/specscore-cli/cmd/specscore@latest",
		"pin an exact released tag or merged commit SHA",
		"never use `@main`",
		"Homebrew cask is blocked",
		"bypass Gatekeeper",
		"remove quarantine attributes.",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("macOS installation guidance is missing %q", required)
		}
	}
	for _, forbidden := range []string{"brew install --cask", "brew upgrade specscore", "xattr", "cmd/specscore@main"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("macOS installation guidance recommends blocked cask behavior %q", forbidden)
		}
	}

	feature, err := os.ReadFile("spec/features/cli/release-distribution/README.md")
	if err != nil {
		t.Fatal(err)
	}
	featureText := string(feature)
	for _, required := range []string{
		"REQ: block-homebrew-cask-until-verified",
		"**Cask distribution status:** Blocked.",
		"MUST NOT recommend, install,\nupgrade, or collect runtime evidence",
		"artifact_smoke_test_homebrew_cask: false",
		"temporary current\nsource-built channel",
		"pin an exact released tag or merged\ncommit SHA",
		"**Blocked** to **Verified**",
	} {
		if !strings.Contains(featureText, required) {
			t.Fatalf("release distribution contract is missing %q", required)
		}
	}
}
