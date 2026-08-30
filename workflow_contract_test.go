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

func TestReleaseCallerEnforcesNotarizedMacOSRelease(t *testing.T) {
	workflow, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflowText := string(workflow)
	for _, required := range []string{
		"uses: strongo/cicd/.github/workflows/release.yml@v1.14.14",
		"release-artifact-smoke-darwin-arm64:",
		"require_notarized_macos: true",
		"artifact_smoke_test_homebrew_cask: true",
	} {
		if !strings.Contains(workflowText, required) {
			t.Fatalf("release caller is missing the notarized-release contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"artifact_smoke_test: false",
		"require_notarized_macos: false",
		"artifact_smoke_test_homebrew_cask: false",
		"uses: strongo/cicd/.github/workflows/release.yml@v1\n",
	} {
		if strings.Contains(workflowText, forbidden) {
			t.Fatalf("release caller retained the pre-notarization contract %q", forbidden)
		}
	}

	goreleaser, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goreleaser), "homebrew_casks:") {
		t.Fatal("GoReleaser must retain Homebrew cask packaging now that its install smoke is verified")
	}
}

func TestMacOSInstallGuidanceRecommendsVerifiedCask(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(readme)
	for _, required := range []string{
		"### macOS — Homebrew cask (recommended)",
		"brew install --cask specscore/tap/specscore",
		"brew upgrade --cask specscore",
		"go install github.com/specscore/specscore-cli/cmd/specscore@latest",
		"pin an exact released tag or merged commit SHA",
		"never use `@main`",
		"bypass Gatekeeper",
		"remove quarantine attributes.",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("macOS installation guidance is missing %q", required)
		}
	}
	for _, forbidden := range []string{"xattr", "cmd/specscore@main", "Homebrew cask is blocked"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("macOS installation guidance retains blocked-cask behavior %q", forbidden)
		}
	}

	feature, err := os.ReadFile("spec/features/cli/release-distribution/README.md")
	if err != nil {
		t.Fatal(err)
	}
	featureText := string(feature)
	for _, required := range []string{
		"REQ: block-homebrew-cask-until-verified",
		"**Cask distribution status:** Verified.",
		"MAY recommend, install, upgrade, and collect runtime evidence",
		"artifact_smoke_test_homebrew_cask: true",
		"pin an exact released tag or merged commit SHA",
		"reverted from **Verified** back to\n**Blocked**",
	} {
		if !strings.Contains(featureText, required) {
			t.Fatalf("release distribution contract is missing %q", required)
		}
	}
	for _, forbidden := range []string{"**Cask distribution status:** Blocked."} {
		if strings.Contains(featureText, forbidden) {
			t.Fatalf("release distribution contract retains blocked-cask status %q", forbidden)
		}
	}
}

func TestCIAggregateWorkflowContract(t *testing.T) {
	workflow := readWorkflowNode(t, ".github/workflows/go-ci.yml")
	triggers := mappingValue(t, workflow, "on")
	for _, event := range []string{"pull_request", "push"} {
		eventNode := mappingValue(t, triggers, event)
		for _, filter := range []string{"paths", "paths-ignore"} {
			if eventNode.Kind == yaml.MappingNode && mappingOptional(eventNode, filter) != nil {
				t.Fatalf("%s must not use a %s filter: CI must be emitted for every event", event, filter)
			}
		}
	}
	if mappingOptional(triggers, "workflow_dispatch") == nil {
		t.Fatal("workflow_dispatch trigger must be preserved")
	}

	push := mappingValue(t, triggers, "push")
	branches := mappingValue(t, push, "branches")
	if len(branches.Content) != 1 || branches.Content[0].Value != "main" {
		t.Fatalf("push trigger must remain limited to main, got %#v", branches.Content)
	}
	concurrency := mappingValue(t, workflow, "concurrency")
	if got := mappingValue(t, concurrency, "group").Value; got != "go-ci-${{ github.workflow }}-${{ github.ref }}" {
		t.Fatalf("concurrency group = %q, want preserved Go CI group", got)
	}
	if got := mappingValue(t, concurrency, "cancel-in-progress").Value; got != "true" {
		t.Fatalf("cancel-in-progress = %q, want true", got)
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
	steps := mappingValue(t, aggregate, "steps")
	if steps.Kind != yaml.SequenceNode || len(steps.Content) != 2 {
		t.Fatalf("aggregate steps must be checkout then reducer, got %d entries", len(steps.Content))
	}
	checkout := steps.Content[0]
	if got := mappingValue(t, checkout, "uses").Value; got != "actions/checkout@v6" {
		t.Fatalf("aggregate first step = %q, want checkout before the repository script", got)
	}
	step := steps.Content[1]
	if got := mappingValue(t, step, "run").Value; got != "./scripts/ci-aggregate.sh" {
		t.Fatalf("aggregate command = %q, want the executable aggregate contract", got)
	}
	environment := mappingValue(t, step, "env")
	for key, want := range map[string]string{
		"CLASSIFY_RESULT":        "${{ needs.classify.result }}",
		"GO_APPLICABLE":          "${{ needs.classify.outputs.go }}",
		"DOGFOOD_APPLICABLE":     "${{ needs.classify.outputs.dogfood }}",
		"TEST_RESULT":            "${{ needs.test.result }}",
		"WINDOWS_RESULT":         "${{ needs.windows-event-process-tree.result }}",
		"RELEASE_TARGETS_RESULT": "${{ needs.release-targets.result }}",
		"REHEARSE_CORPUS_RESULT": "${{ needs.rehearse-corpus.result }}",
		"DOGFOOD_RESULT":         "${{ needs.dogfood.result }}",
	} {
		if got := mappingValue(t, environment, key).Value; got != want {
			t.Fatalf("aggregate %s = %q, want %q", key, got, want)
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

func TestCIAggregateResultContract(t *testing.T) {
	accepted := []struct {
		name string
		env  map[string]string
	}{
		{"all gates applicable and successful", aggregateEnvironment("true", "true", "success", "success")},
		{"docs-only gates inapplicable and skipped", aggregateEnvironment("false", "false", "skipped", "skipped")},
		{"Go-only gates successful and dogfood skipped", aggregateEnvironment("true", "false", "success", "skipped")},
		{"spec-only Go gates skipped and dogfood successful", aggregateEnvironment("false", "true", "skipped", "success")},
	}
	for _, test := range accepted {
		t.Run(test.name, func(t *testing.T) {
			if output, err := runAggregate(t, test.env); err != nil {
				t.Fatalf("aggregate rejected valid results: %v\n%s", err, output)
			}
		})
	}

	for _, result := range []string{"skipped", "failure", "cancelled", "timed_out", "unknown", ""} {
		t.Run("classifier rejects "+result, func(t *testing.T) {
			environment := aggregateEnvironment("true", "true", "success", "success")
			environment["CLASSIFY_RESULT"] = result
			if output, err := runAggregate(t, environment); err == nil {
				t.Fatalf("aggregate accepted classifier result %q\n%s", result, output)
			}
		})
	}

	for _, result := range []string{"skipped", "failure", "cancelled", "timed_out", "unknown", ""} {
		t.Run("applicable result rejects "+result, func(t *testing.T) {
			environment := aggregateEnvironment("true", "false", "success", "skipped")
			environment["TEST_RESULT"] = result
			if output, err := runAggregate(t, environment); err == nil {
				t.Fatalf("aggregate accepted applicable gate result %q\n%s", result, output)
			}
		})
	}

	for _, result := range []string{"success", "failure", "cancelled", "timed_out", "unknown", ""} {
		t.Run("inapplicable result rejects "+result, func(t *testing.T) {
			environment := aggregateEnvironment("false", "true", "skipped", "success")
			environment["TEST_RESULT"] = result
			if output, err := runAggregate(t, environment); err == nil {
				t.Fatalf("aggregate accepted inapplicable gate result %q\n%s", result, output)
			}
		})
	}

	for _, applicabilityKey := range []string{"GO_APPLICABLE", "DOGFOOD_APPLICABLE"} {
		for _, applicability := range []string{"yes", "", "TRUE"} {
			t.Run("invalid "+applicabilityKey+" rejects "+applicability, func(t *testing.T) {
				environment := aggregateEnvironment("true", "true", "success", "success")
				environment[applicabilityKey] = applicability
				if output, err := runAggregate(t, environment); err == nil {
					t.Fatalf("aggregate accepted %s %q\n%s", applicabilityKey, applicability, output)
				}
			})
		}
	}

	for _, gate := range []struct {
		name string
		key  string
		kind string
	}{
		{"test", "TEST_RESULT", "go"},
		{"windows", "WINDOWS_RESULT", "go"},
		{"release targets", "RELEASE_TARGETS_RESULT", "go"},
		{"Rehearse corpus", "REHEARSE_CORPUS_RESULT", "go"},
		{"dogfood", "DOGFOOD_RESULT", "dogfood"},
	} {
		t.Run(gate.name+" applicable skipped is rejected", func(t *testing.T) {
			environment := aggregateEnvironment("true", "true", "success", "success")
			environment[gate.key] = "skipped"
			if output, err := runAggregate(t, environment); err == nil {
				t.Fatalf("aggregate accepted applicable skipped %s\n%s", gate.name, output)
			}
		})
		t.Run(gate.name+" inapplicable success is rejected", func(t *testing.T) {
			var environment map[string]string
			if gate.kind == "go" {
				environment = aggregateEnvironment("false", "true", "skipped", "success")
			} else {
				environment = aggregateEnvironment("true", "false", "success", "skipped")
			}
			environment[gate.key] = "success"
			if output, err := runAggregate(t, environment); err == nil {
				t.Fatalf("aggregate accepted inapplicable successful %s\n%s", gate.name, output)
			}
		})
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
			name:        "coverage gate changes run every Go gate",
			changes:     map[string]string{"scripts/coverage-gate.sh": "#!/usr/bin/env bash\n"},
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

func TestCIPathClassifierFailsClosedOnMalformedStatusStream(t *testing.T) {
	tests := []struct {
		name   string
		stream []byte
	}{
		{"unknown X status", nulFields("X", "docs/guide.md")},
		{"unknown status word", nulFields("unknown", "docs/guide.md")},
		{"rename score omitted", nulFields("R", "old.go", "docs/new.md")},
		{"rename score too short", nulFields("R10", "old.go", "docs/new.md")},
		{"copy score too long", nulFields("C1000", "old.go", "docs/new.md")},
		{"rename score exceeds 100", nulFields("R101", "old.go", "docs/new.md")},
		{"partial unterminated status", []byte("M")},
		{"partial unterminated ordinary path", append(nulFields("M"), []byte("docs/guide.md")...)},
		{"partial unterminated rename destination", append(nulFields("R100", "old.go"), []byte("docs/new.md")...)},
		{"partial record after complete record", append(nulFields("M", "docs/guide.md"), []byte("partial")...)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runClassifierWithDiffStream(t, test.stream)
			if got["go"] != "true" || got["dogfood"] != "true" {
				t.Fatalf("malformed stream classifier = %#v, want fail-closed both gates", got)
			}
		})
	}
}

func TestCIPathClassifierUsesBothRenameAndCopyPathsAndDeletedPaths(t *testing.T) {
	t.Run("rename from Go into docs still runs the source-path Go gate", func(t *testing.T) {
		repo, base, head := classifierFixtureWithMutation(t,
			map[string]string{"source\nname.go": "package specscore_cli\n"},
			func(repo string) {
				writeTestFile(t, filepath.Join(repo, "docs", ".keep"), "keep\n")
				if err := os.Rename(
					filepath.Join(repo, "source\nname.go"),
					filepath.Join(repo, "docs", "renamed\nfile.md"),
				); err != nil {
					t.Fatal(err)
				}
			},
		)
		got := runClassifier(t, repo, base, head)
		if got["go"] != "true" || got["dogfood"] != "false" {
			t.Fatalf("rename classifier = %#v, want source-path Go gate only", got)
		}
	})

	t.Run("copy from Go into docs still runs Go gates", func(t *testing.T) {
		repo, base, head := classifierFixtureWithMutation(t,
			map[string]string{"tool.go": "package specscore_cli\n"},
			func(repo string) {
				data, err := os.ReadFile(filepath.Join(repo, "tool.go"))
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(repo, "docs", "copied.md"), string(data))
			},
		)
		got := runClassifier(t, repo, base, head)
		if got["go"] != "true" || got["dogfood"] != "false" {
			t.Fatalf("copy classifier = %#v, want source-path Go gate only", got)
		}
	})

	t.Run("deleted Go path still runs the Go gate", func(t *testing.T) {
		repo, base, head := classifierFixtureWithMutation(t,
			map[string]string{"deleted.go": "package specscore_cli\n"},
			func(repo string) {
				if err := os.Remove(filepath.Join(repo, "deleted.go")); err != nil {
					t.Fatal(err)
				}
			},
		)
		got := runClassifier(t, repo, base, head)
		if got["go"] != "true" || got["dogfood"] != "false" {
			t.Fatalf("deletion classifier = %#v, want deleted-path Go gate only", got)
		}
	})
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
	return classifierFixtureWithMutation(t, nil, func(repo string) {
		for path, contents := range changes {
			writeTestFile(t, filepath.Join(repo, path), contents)
		}
	})
}

func classifierFixtureWithMutation(
	t *testing.T,
	baseFiles map[string]string,
	mutate func(repo string),
) (repo, base, head string) {
	t.Helper()
	repo = t.TempDir()
	runCommand(t, repo, "git", "init", "-q")
	runCommand(t, repo, "git", "config", "user.email", "ci@example.test")
	runCommand(t, repo, "git", "config", "user.name", "CI contract test")
	writeTestFile(t, filepath.Join(repo, "README.md"), "initial\n")
	for path, contents := range baseFiles {
		writeTestFile(t, filepath.Join(repo, path), contents)
	}
	runCommand(t, repo, "git", "add", ".")
	runCommand(t, repo, "git", "commit", "-qm", "initial")
	base = strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	mutate(repo)
	runCommand(t, repo, "git", "add", "-A")
	runCommand(t, repo, "git", "commit", "-qm", "change")
	head = strings.TrimSpace(runCommand(t, repo, "git", "rev-parse", "HEAD"))
	return repo, base, head
}

func aggregateEnvironment(goApplicable, dogfoodApplicable, goResult, dogfoodResult string) map[string]string {
	return map[string]string{
		"CLASSIFY_RESULT":        "success",
		"GO_APPLICABLE":          goApplicable,
		"DOGFOOD_APPLICABLE":     dogfoodApplicable,
		"TEST_RESULT":            goResult,
		"WINDOWS_RESULT":         goResult,
		"RELEASE_TARGETS_RESULT": goResult,
		"REHEARSE_CORPUS_RESULT": goResult,
		"DOGFOOD_RESULT":         dogfoodResult,
	}
}

func runAggregate(t *testing.T, environment map[string]string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("scripts/ci-aggregate.sh")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(script)
	command.Env = os.Environ()
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, runErr := command.CombinedOutput()
	return string(output), runErr
}

func runClassifier(t *testing.T, repo, base, head string) map[string]string {
	t.Helper()
	return runClassifierCommand(t, repo, []string{
		"EVENT_NAME=pull_request",
		"PR_BASE_SHA=" + base,
		"HEAD_SHA=" + head,
	})
}

func runClassifierWithDiffStream(t *testing.T, stream []byte) map[string]string {
	t.Helper()
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	writeTestFile(t, fakeGit, `#!/usr/bin/env bash
set -eu
case "${1:-}" in
cat-file) exit 0 ;;
diff) /bin/cat "${FAKE_GIT_DIFF_STREAM:?}" ;;
*) exit 64 ;;
esac
`)
	if err := os.Chmod(fakeGit, 0o755); err != nil {
		t.Fatal(err)
	}
	streamFile := filepath.Join(t.TempDir(), "diff-stream")
	if err := os.WriteFile(streamFile, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	return runClassifierCommand(t, t.TempDir(), []string{
		"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"FAKE_GIT_DIFF_STREAM=" + streamFile,
		"EVENT_NAME=pull_request",
		"PR_BASE_SHA=base",
		"HEAD_SHA=head",
	})
}

func runClassifierCommand(t *testing.T, repo string, environment []string) map[string]string {
	t.Helper()
	script, err := filepath.Abs("scripts/ci-classify-paths.sh")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "github-output")
	command := exec.Command(script)
	command.Dir = repo
	command.Env = append(os.Environ(), environment...)
	command.Env = append(command.Env, "GITHUB_OUTPUT="+output)
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

func nulFields(fields ...string) []byte {
	var result []byte
	for _, field := range fields {
		result = append(result, field...)
		result = append(result, 0)
	}
	return result
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
