package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	pub "github.com/specscore/specscore-cli/pkg/publication"
	"gopkg.in/yaml.v3"
)

func runPublication(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := publicationCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func writePublicationSpecConfig(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(projectdef.SchemaHeader+"\n\n"+body), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
}

func isolateUserConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path, err := pub.UserConfigPath()
	if err != nil {
		t.Fatalf("user config path: %v", err)
	}
	return path
}

func TestPublicationSet_ProjectEventPolicyWritesCanonicalActions(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "project:\n  title: Demo\ncustom_field: keep\n")

	out, _, err := runPublication(t,
		"set",
		"--scope", "project",
		"--event", "idea.approved",
		"--actions", "stage,commit,push",
		"--format", "yaml",
	)
	if err != nil {
		t.Fatalf("publication set: %v", err)
	}
	if !strings.Contains(out, "touched_paths:") || !strings.Contains(out, "specscore.yaml") {
		t.Fatalf("expected touched_paths in yaml output; got:\n%s", out)
	}

	data, err := os.ReadFile(filepath.Join(root, projectdef.SpecConfigFile))
	if err != nil {
		t.Fatalf("read specscore.yaml: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"custom_field: keep",
		"publication:",
		"events:",
		"idea.approved:",
		"- stage",
		"- commit",
		"- push",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("specscore.yaml missing %q:\n%s", want, got)
		}
	}
}

func TestPublicationSet_UserDefaultShorthandNormalized(t *testing.T) {
	userPath := isolateUserConfig(t)

	out, _, err := runPublication(t,
		"set",
		"--scope", "user",
		"--default", "commit-and-push",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("publication set user: %v", err)
	}
	if !strings.Contains(out, `"touched_paths"`) || !strings.Contains(out, userPath) {
		t.Fatalf("expected json touched_paths with user path; got:\n%s", out)
	}

	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("read user config: %v", err)
	}
	got := string(data)
	for _, want := range []string{"publication:", "default:", "- stage", "- commit", "- push"} {
		if !strings.Contains(got, want) {
			t.Errorf("user config missing %q:\n%s", want, got)
		}
	}
}

func TestPublicationResolve_MachineReadableBlocksUnsafePush(t *testing.T) {
	userPath := isolateUserConfig(t)
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte("publication:\n  default:\n    actions: [stage]\n"), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "publication:\n  events:\n    idea.approved:\n      actions: [stage, commit, push]\n  push:\n    deny_branches: [main]\n")

	out, _, err := runPublication(t,
		"resolve",
		"--command", "ideate",
		"--event", "idea.approved",
		"--branch", "main",
		"--format", "yaml",
	)
	if err != nil {
		t.Fatalf("publication resolve: %v", err)
	}
	var result pub.ResolveResult
	if err := yaml.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal resolve yaml: %v\n%s", err, out)
	}
	if strings.Join(result.ActionsResolved, ",") != "stage,commit,push" {
		t.Fatalf("actions_resolved = %v, want stage,commit,push", result.ActionsResolved)
	}
	if strings.Join(result.ActionsAllowed, ",") != "stage,commit" {
		t.Fatalf("actions_allowed = %v, want stage,commit", result.ActionsAllowed)
	}
	if result.Branch != "main" || result.BranchPushAllowed {
		t.Fatalf("branch decision = branch %q allowed %v, want main false", result.Branch, result.BranchPushAllowed)
	}
	if len(result.ActionsBlocked) != 1 || result.ActionsBlocked[0].Action != "push" {
		t.Fatalf("actions_blocked = %#v, want push blocked", result.ActionsBlocked)
	}
	if len(result.PolicySources) == 0 {
		t.Fatal("expected policy_sources to identify selected config")
	}
}

func TestPublicationBranchCheck_DeniesMain(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "publication:\n  push:\n    deny_branches: [main]\n")

	out, _, err := runPublication(t, "branch-check", "--branch", "main", "--format", "yaml")
	if err == nil {
		t.Fatal("expected branch-check denial error")
	}
	var ec *exitcode.Error
	if !errors.As(err, &ec) || ec.ExitCode() != exitcode.InvalidState {
		t.Fatalf("error = %v, want exit code %d", err, exitcode.InvalidState)
	}
	if !strings.Contains(out, "branch_push_allowed: false") || !strings.Contains(out, "denied") {
		t.Fatalf("expected denial yaml output; got:\n%s", out)
	}
}

func TestPublicationBranchCheck_DefaultDeniesMainUnlessProjectOverrides(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "project:\n  title: Demo\n")

	out, _, err := runPublication(t, "branch-check", "--branch", "main", "--format", "yaml")
	if err == nil {
		t.Fatal("expected built-in branch-check denial for main")
	}
	if !strings.Contains(out, "publication.push.deny_branches.default") {
		t.Fatalf("expected built-in deny source; got:\n%s", out)
	}

	writePublicationSpecConfig(t, root, "publication:\n  push:\n    deny_branches: []\n")
	out, _, err = runPublication(t, "branch-check", "--branch", "main", "--format", "yaml")
	if err != nil {
		t.Fatalf("expected explicit project empty deny list to override defaults: %v\n%s", err, out)
	}
	if !strings.Contains(out, "branch_push_allowed: true") {
		t.Fatalf("expected branch allowed after project override; got:\n%s", out)
	}
}

func TestPublicationSet_MissingRequiredArgsFails2(t *testing.T) {
	_, _, err := runPublication(t, "set", "--scope", "project", "--event", "idea.approved")
	if err == nil {
		t.Fatal("expected invalid args error")
	}
	var ec *exitcode.Error
	if !errors.As(err, &ec) || ec.ExitCode() != exitcode.InvalidArgs {
		t.Fatalf("error = %v, want exit code %d", err, exitcode.InvalidArgs)
	}
}
