package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	pubpkg "github.com/specscore/specscore-cli/pkg/publication"
)

func pubSetResultStub() pubpkg.SetResult           { return pubpkg.SetResult{} }
func pubResolveResultStub() pubpkg.ResolveResult   { return pubpkg.ResolveResult{} }
func pubBranchCheckStub() pubpkg.BranchCheckResult { return pubpkg.BranchCheckResult{} }

// covErrWriter is an io.Writer that always fails, used to exercise encoder
// error paths. Named uniquely to avoid clashing with errWriter elsewhere.
type covErrWriter struct{}

func (covErrWriter) Write(_ []byte) (int, error) { return 0, errors.New("cov write error") }

func covExitCode(t *testing.T, err error) int {
	t.Helper()
	var ec *exitcode.Error
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	t.Fatalf("error %v (%T) is not *exitcode.Error", err, err)
	return -1
}

// --- publication ------------------------------------------------------------

func TestPublicationCommand_NoSubcommandPrintsHelp(t *testing.T) {
	cmd := publicationCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("publication with no subcommand returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Publication policy helpers") {
		t.Fatalf("expected help text; got:\n%s", out.String())
	}
}

func TestPublicationSet_MissingScope(t *testing.T) {
	_, _, err := runPublication(t, "set", "--default", "stage")
	if err == nil {
		t.Fatal("expected error for missing --scope")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationSet_InvalidFormat(t *testing.T) {
	_, _, err := runPublication(t, "set", "--scope", "user", "--default", "stage", "--format", "csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationSet_DefaultCombinedWithActions(t *testing.T) {
	_, _, err := runPublication(t, "set", "--scope", "user", "--default", "stage", "--actions", "stage")
	if err == nil {
		t.Fatal("expected error combining --default with --actions")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationSet_ProjectRootResolutionFails(t *testing.T) {
	// scope=project, no --project, cwd with no specscore.yaml → findRepoConfigRoot
	// error returned because required==true.
	empty := t.TempDir()
	withCwd(t, empty)
	_, _, err := runPublication(t, "set", "--scope", "project", "--default", "stage")
	if err == nil {
		t.Fatal("expected project-root resolution error")
	}
}

func TestPublicationSet_EventAndMilestoneMutuallyExclusive(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "project:\n  title: Demo\n")
	_, _, err := runPublication(t, "set", "--scope", "project",
		"--event", "idea.approved", "--milestone", "m1", "--actions", "stage")
	if err == nil {
		t.Fatal("expected --event/--milestone mutually exclusive error")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationSet_SetPolicyError(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "project:\n  title: Demo\n")
	_, _, err := runPublication(t, "set", "--scope", "project",
		"--event", "idea.approved", "--actions", "totally-bogus-action")
	if err == nil {
		t.Fatal("expected SetPolicy error for unknown action")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationSet_YAMLAndJSONOutput(t *testing.T) {
	// yaml output success path (outputPublicationSet yaml branch).
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "project:\n  title: Demo\n")
	out, _, err := runPublication(t, "set", "--scope", "project",
		"--event", "idea.approved", "--action", "stage", "--format", "yaml")
	if err != nil {
		t.Fatalf("set yaml: %v", err)
	}
	if !strings.Contains(out, "touched_paths:") {
		t.Fatalf("expected yaml output; got:\n%s", out)
	}
}

func TestPublicationSet_TextOutput(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "project:\n  title: Demo\n")
	out, _, err := runPublication(t, "set", "--scope", "project",
		"--event", "idea.approved", "--action", "stage", "--format", "text")
	if err != nil {
		t.Fatalf("set text: %v", err)
	}
	if !strings.Contains(out, "wrote ") {
		t.Fatalf("expected text output 'wrote ...'; got:\n%s", out)
	}
}

func TestPublicationResolve_InvalidFormat(t *testing.T) {
	// format=text is not allowed for resolve (allowText=false).
	_, _, err := runPublication(t, "resolve", "--format", "text")
	if err == nil {
		t.Fatal("expected invalid format error (text not allowed for resolve)")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationResolve_ProjectRootFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })
	_, _, err := runPublication(t, "resolve", "--project", "somewhere", "--format", "json")
	if err == nil {
		t.Fatal("expected abs error from --project resolution")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationResolve_GetwdError(t *testing.T) {
	old := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("getwd boom") }
	t.Cleanup(func() { osGetwdFn = old })
	_, _, err := runPublication(t, "resolve", "--format", "json")
	if err == nil {
		t.Fatal("expected getwd error")
	}
	if got := covExitCode(t, err); got != exitcode.Unexpected {
		t.Fatalf("exit code = %d, want Unexpected", got)
	}
}

func TestPublicationResolve_ResolveError(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	// Malformed specscore.yaml so publication.Resolve fails to parse it.
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile),
		[]byte(projectdef.SchemaHeader+"\npublication: [this is not a map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPublication(t, "resolve", "--project", root, "--format", "json")
	if err == nil {
		t.Fatal("expected Resolve error from malformed config")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationResolve_JSONAndProjectRootAutoDiscoverMiss(t *testing.T) {
	// No specscore.yaml in cwd: resolvePublicationProjectRoot returns ("", nil)
	// (required==false), and resolve proceeds with an empty project root.
	isolateUserConfig(t)
	empty := t.TempDir()
	withCwd(t, empty)
	out, _, err := runPublication(t, "resolve", "--task-policy", "stage", "--format", "json")
	if err != nil {
		t.Fatalf("resolve json: %v", err)
	}
	if !strings.Contains(out, "actions_resolved") {
		t.Fatalf("expected json output; got:\n%s", out)
	}
}

func TestPublicationBranchCheck_InvalidFormat(t *testing.T) {
	_, _, err := runPublication(t, "branch-check", "--format", "csv")
	if err == nil {
		t.Fatal("expected invalid format error")
	}
	if got := covExitCode(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want InvalidArgs", got)
	}
}

func TestPublicationBranchCheck_ProjectRootFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })
	_, _, err := runPublication(t, "branch-check", "--project", "x", "--format", "json")
	if err == nil {
		t.Fatal("expected abs error")
	}
}

func TestPublicationBranchCheck_ResolveError(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile),
		[]byte(projectdef.SchemaHeader+"\npublication: [not a map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPublication(t, "branch-check", "--project", root, "--branch", "feature", "--format", "json")
	if err == nil {
		t.Fatal("expected Resolve error from malformed config")
	}
}

func TestPublicationBranchCheck_TextAllowedAndDenied(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "publication:\n  push:\n    deny_branches: [main]\n")

	// allowed → text
	out, _, err := runPublication(t, "branch-check", "--branch", "feature", "--format", "text")
	if err != nil {
		t.Fatalf("branch-check allowed text: %v", err)
	}
	if !strings.Contains(out, "push allowed for feature") {
		t.Fatalf("expected allowed text; got:\n%s", out)
	}

	// denied → text + InvalidState error
	out, _, err = runPublication(t, "branch-check", "--branch", "main", "--format", "text")
	if err == nil {
		t.Fatal("expected denial error")
	}
	if !strings.Contains(out, "push denied for main") {
		t.Fatalf("expected denied text; got:\n%s", out)
	}
	if got := covExitCode(t, err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want InvalidState", got)
	}
}

func TestPublicationBranchCheck_JSONOutput(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "publication:\n  push:\n    deny_branches: []\n")
	out, _, err := runPublication(t, "branch-check", "--branch", "main", "--format", "json")
	if err != nil {
		t.Fatalf("branch-check json: %v", err)
	}
	if !strings.Contains(out, "branch_push_allowed") {
		t.Fatalf("expected json output; got:\n%s", out)
	}
}

func TestPublicationBranchCheck_OutputWriterError(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writePublicationSpecConfig(t, root, "publication:\n  push:\n    deny_branches: []\n")
	cmd := publicationCommand()
	cmd.SetOut(covErrWriter{})
	cmd.SetErr(covErrWriter{})
	cmd.SetArgs([]string{"branch-check", "--branch", "feature", "--format", "json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected writer error from outputPublicationBranchCheck")
	}
}

// outputPublicationSet yaml/json writer error paths exercised directly.
func TestOutputPublicationSet_WriterErrors(t *testing.T) {
	cmd := publicationCommand()
	cmd.SetOut(covErrWriter{})
	for _, format := range []string{"yaml", "json"} {
		if err := outputPublicationSet(cmd, pubSetResultStub(), format); err == nil {
			t.Errorf("format %q: expected writer error", format)
		}
	}
}

func TestOutputPublicationResolve_WriterErrors(t *testing.T) {
	cmd := publicationCommand()
	cmd.SetOut(covErrWriter{})
	for _, format := range []string{"yaml", "json"} {
		if err := outputPublicationResolve(cmd, pubResolveResultStub(), format); err == nil {
			t.Errorf("format %q: expected writer error", format)
		}
	}
}

func TestOutputPublicationBranchCheck_YAMLWriterError(t *testing.T) {
	cmd := publicationCommand()
	cmd.SetOut(covErrWriter{})
	if err := outputPublicationBranchCheck(cmd, pubBranchCheckStub(), "yaml"); err == nil {
		t.Fatal("expected yaml writer error")
	}
}

func TestActionFlagSlice(t *testing.T) {
	if actionFlagSlice("  ") != nil {
		t.Fatal("blank should return nil")
	}
	got := actionFlagSlice("stage,commit")
	if len(got) != 1 || got[0] != "stage,commit" {
		t.Fatalf("actionFlagSlice = %v", got)
	}
}

func TestResolvePublicationProjectRoot_FlagAbsSuccess(t *testing.T) {
	cmd := publicationResolveCommand()
	dir := t.TempDir()
	if err := cmd.Flags().Set("project", dir); err != nil {
		t.Fatal(err)
	}
	got, err := resolvePublicationProjectRoot(cmd, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Fatalf("got %q, want %q", got, abs)
	}
}

func TestResolvePublicationProjectRoot_AutoDiscoverMissNotRequired(t *testing.T) {
	empty := t.TempDir()
	withCwd(t, empty)
	cmd := publicationResolveCommand()
	got, err := resolvePublicationProjectRoot(cmd, false)
	if err != nil {
		t.Fatalf("expected nil error when not required: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty root, got %q", got)
	}
}

// --- spec: outputLintFixEnvelope -------------------------------------------

func TestOutputLintFixEnvelope_NilSlicesEmitEmptyArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := outputLintFixEnvelope(&buf, nil, nil, "json"); err != nil {
		t.Fatalf("envelope json: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"fixed": []`) || !strings.Contains(got, `"violations": []`) {
		t.Fatalf("expected empty arrays; got: %s", got)
	}
}

func TestOutputLintFixEnvelope_YAMLWriterError(t *testing.T) {
	err := outputLintFixEnvelope(covErrWriter{}, []string{"a"}, []lint.Violation{}, "yaml")
	if err == nil {
		t.Fatal("expected yaml encode error")
	}
}

func TestOutputLintFixEnvelope_JSONWriterError(t *testing.T) {
	err := outputLintFixEnvelope(covErrWriter{}, []string{"a"}, []lint.Violation{}, "json")
	if err == nil {
		t.Fatal("expected json encode error")
	}
}

// runSpecLint: envelope-output error path (spec.go:164). Drive `--fix
// --format json` against an autofixable project with a failing stdout writer.
func TestRunSpecLint_EnvelopeOutputError(t *testing.T) {
	root := setupAutofixProject(t)
	cmd := specCommand()
	cmd.SilenceUsage = true
	cmd.SetOut(covErrWriter{})
	cmd.SetErr(covErrWriter{})
	cmd.SetArgs([]string{"lint", "--project", root, "--rules=adherence-footer", "--fix", "--format=json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected envelope output error")
	}
}
