package specscore_cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestCIAggregateWorkflowContract(t *testing.T) {
	workflow := readWorkflowNode(t, ".github/workflows/go-ci.yml")
	triggers := mappingValue(t, workflow, "on")
	for _, event := range []string{"pull_request", "push"} {
		eventNode := mappingValue(t, triggers, event)
		if eventNode.Kind == yaml.MappingNode && mappingOptional(eventNode, "paths") != nil {
			t.Fatalf("%s must not use a paths filter: CI must be emitted for every event", event)
		}
	}

	push := mappingValue(t, triggers, "push")
	branches := mappingValue(t, push, "branches")
	if len(branches.Content) != 1 || branches.Content[0].Value != "main" {
		t.Fatalf("push trigger must remain limited to main, got %#v", branches.Content)
	}

	jobs := mappingValue(t, workflow, "jobs")
	for _, job := range []struct {
		id        string
		condition string
	}{
		{"test", "${{ needs.classify.outputs.go == 'true' }}"},
		{"windows-event-process-tree", "${{ needs.classify.outputs.go == 'true' }}"},
		{"release-targets", "${{ needs.classify.outputs.go == 'true' }}"},
		{"rehearse-corpus", "${{ needs.classify.outputs.go == 'true' }}"},
		{"dogfood", "${{ needs.classify.outputs.dogfood == 'true' }}"},
	} {
		jobNode := mappingValue(t, jobs, job.id)
		if got := mappingValue(t, jobNode, "if").Value; got != job.condition {
			t.Fatalf("%s condition = %q, want %q", job.id, got, job.condition)
		}
	}

	aggregate := mappingValue(t, jobs, "ci")
	if got := mappingValue(t, aggregate, "name").Value; got != "CI" {
		t.Fatalf("aggregate job name = %q, want stable exact name CI", got)
	}
	if got := mappingValue(t, aggregate, "if").Value; got != "${{ always() }}" {
		t.Fatalf("aggregate if = %q, want always() so failures and cancellations are observed", got)
	}
	for _, required := range []string{"classify", "test", "windows-event-process-tree", "release-targets", "rehearse-corpus", "dogfood"} {
		if !sequenceContains(mappingValue(t, aggregate, "needs"), required) {
			t.Fatalf("aggregate does not need %s", required)
		}
	}
	run := mappingValue(t, mappingValue(t, aggregate, "steps").Content[0], "run").Value
	if !strings.Contains(run, "success|skipped)") {
		t.Fatal("aggregate must allow only successful or genuinely inapplicable skipped jobs")
	}
	for _, rejected := range []string{"failure", "cancelled", "timed_out", "unknown"} {
		if strings.Contains(run, "success|skipped|"+rejected) {
			t.Fatalf("aggregate incorrectly accepts %s", rejected)
		}
	}

	if _, err := os.Stat(".github/workflows/dogfood.yml"); !os.IsNotExist(err) {
		t.Fatalf("standalone dogfood workflow must be removed after consolidation, stat error: %v", err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(readme), "actions/workflows/dogfood.yml") {
		t.Fatal("README must not retain a badge for the removed standalone dogfood workflow")
	}
}

func TestCIPathClassifierContract(t *testing.T) {
	for _, test := range []struct {
		name        string
		changes     map[string]string
		base        string
		wantGo      string
		wantDogfood string
	}{
		{
			name:        "docs-only skips heavy gates while CI still runs",
			changes:     map[string]string{"docs/guide.md": "guide\n"},
			wantGo:      "false",
			wantDogfood: "false",
		},
		{
			name:        "Go-only runs every Go gate",
			changes:     map[string]string{"workflow_contract.go": "package specscore_cli\n"},
			wantGo:      "true",
			wantDogfood: "false",
		},
		{
			name:        "spec-only runs dogfood",
			changes:     map[string]string{"spec/features/cli/README.md": "# changed\n"},
			wantGo:      "false",
			wantDogfood: "true",
		},
		{
			name:        "mixed Go and dogfood paths run both",
			changes:     map[string]string{"pkg/lint/example.go": "package lint\n"},
			wantGo:      "true",
			wantDogfood: "true",
		},
		{
			name:        "unknown base fails closed",
			changes:     map[string]string{"docs/guide.md": "guide\n"},
			base:        "not-a-commit",
			wantGo:      "true",
			wantDogfood: "true",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, base, head := classifierFixture(t, test.changes)
			if test.base != "" {
				base = test.base
			}
			got := runClassifier(t, repo, base, head)
			if got["go"] != test.wantGo || got["dogfood"] != test.wantDogfood {
				t.Fatalf("classifier = %#v, want go=%s dogfood=%s", got, test.wantGo, test.wantDogfood)
			}
		})
	}
}

func readWorkflowNode(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatalf("unexpected YAML document in %s", path)
	}
	return document.Content[0]
}

func mappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value := mappingOptional(node, key)
	if value == nil {
		t.Fatalf("missing YAML key %q", key)
	}
	return value
}

func mappingOptional(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func sequenceContains(node *yaml.Node, want string) bool {
	for _, item := range node.Content {
		if item.Value == want {
			return true
		}
	}
	return false
}

func classifierFixture(t *testing.T, changes map[string]string) (repo, base, head string) {
	t.Helper()
	repo = t.TempDir()
	runCommand(t, repo, "git", "init", "-q")
	runCommand(t, repo, "git", "config", "user.email", "ci@example.test")
	runCommand(t, repo, "git", "config", "user.name", "CI contract test")
	writeTestFile(t, filepath.Join(repo, "README.md"), "initial\n")
	runCommand(t, repo, "git", "add", ".")
	runCommand(t, repo, "git", "commit", "-qm", "initial")
	base = strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	for path, contents := range changes {
		writeTestFile(t, filepath.Join(repo, path), contents)
	}
	runCommand(t, repo, "git", "add", ".")
	runCommand(t, repo, "git", "commit", "-qm", "change")
	head = strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	return repo, base, head
}

func runClassifier(t *testing.T, repo, base, head string) map[string]string {
	t.Helper()
	script, err := filepath.Abs("scripts/ci-classify-paths.sh")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "github-output")
	command := exec.Command(script)
	command.Dir = repo
	command.Env = append(os.Environ(),
		"EVENT_NAME=pull_request",
		"PR_BASE_SHA="+base,
		"HEAD_SHA="+head,
		"GITHUB_OUTPUT="+output,
	)
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("classifier failed: %v\n%s", err, combined)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid GITHUB_OUTPUT line %q", line)
		}
		values[key] = value
	}
	return values
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCommand(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}
