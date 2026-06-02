package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/selfupdate"
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

// --- self-update: command + runCheck + classify + seam defaults -------------

func TestSelfUpdate_DetectInstallError(t *testing.T) {
	prev := detectInstall
	detectInstall = func() (selfupdate.Detection, error) {
		return selfupdate.Detection{}, errors.New("detect boom")
	}
	t.Cleanup(func() { detectInstall = prev })
	_, _, err := runSelfUpdate(t)
	if err == nil {
		t.Fatal("expected detect error")
	}
}

func TestSelfUpdate_ManagedUnknownManager(t *testing.T) {
	// Managed method but a manager with no known upgrade command → error.
	withDetection(t, selfupdate.Detection{Method: selfupdate.Managed, Manager: selfupdate.ManagerNone})
	_, _, err := runSelfUpdate(t)
	if err == nil {
		t.Fatal("expected error for managed install with no known upgrade command")
	}
	if !strings.Contains(err.Error(), "no upgrade command") {
		t.Fatalf("error = %v, want 'no upgrade command'", err)
	}
}

func TestSelfUpdate_UnpinnedInteractiveAbort(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	withInteractive(t, true)
	called := withSelfReplace(t, nil)

	cmd := selfUpdateCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("abort should return nil: %v", err)
	}
	if *called {
		t.Error("doSelfReplace called despite abort")
	}
	if !strings.Contains(strings.ToLower(out.String()), "aborted") {
		t.Fatalf("expected abort message; got:\n%s", out.String())
	}
}

func TestSelfUpdate_PinnedInteractiveAbort(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withInteractive(t, true)
	called := withSelfReplace(t, nil)

	cmd := selfUpdateCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"--version", "v1.2.0"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pinned abort should return nil: %v", err)
	}
	if *called {
		t.Error("doSelfReplace called despite pinned abort")
	}
	if !strings.Contains(strings.ToLower(out.String()), "aborted") {
		t.Fatalf("expected abort message; got:\n%s", out.String())
	}
}

func TestSelfUpdate_PinnedNonInteractiveWithoutYesRefuses(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withInteractive(t, false)
	called := withSelfReplace(t, nil)

	_, _, err := runSelfUpdate(t, "--version", "v1.2.0")
	if err == nil {
		t.Fatal("expected refusal without --yes non-interactive")
	}
	if *called {
		t.Error("doSelfReplace called; must refuse")
	}
}

func TestRunCheck_UndeterminedManual(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "dev")
	withLatest(t, "v1.0.0", nil)
	out, _, err := runSelfUpdate(t, "--check")
	if err == nil {
		t.Fatal("expected exit-10 error for undetermined/available")
	}
	if !strings.Contains(strings.ToLower(out), "undetermined") {
		t.Fatalf("expected undetermined line; got:\n%s", out)
	}
}

func TestRunCheck_ManagedBranch(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Managed, Manager: selfupdate.Homebrew})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	out, _, err := runSelfUpdate(t, "--check")
	if err == nil {
		t.Fatal("expected exit-10 error when update available")
	}
	if !strings.Contains(out, "brew upgrade specscore") {
		t.Fatalf("expected brew upgrade hint; got:\n%s", out)
	}
}

func TestRunCheck_AmbiguousBranch(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Ambiguous, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	out, _, err := runSelfUpdate(t, "--check")
	if err == nil {
		t.Fatal("expected exit-10 error when update available")
	}
	if !strings.Contains(out, "ambiguous") {
		t.Fatalf("expected ambiguous guidance; got:\n%s", out)
	}
}

func TestClassifySelfReplaceError_EmptyExecutableFallback(t *testing.T) {
	prev := osExecutableFn
	osExecutableFn = func() (string, error) { return "", errors.New("no exe") }
	t.Cleanup(func() { osExecutableFn = prev })

	cmd := selfUpdateCommand()
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)
	err := classifySelfReplaceError(cmd, fmt.Errorf("swap: %w", os.ErrPermission))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(errOut.String(), "the specscore executable") {
		t.Fatalf("expected fallback executable label; got:\n%s", errOut.String())
	}
}

// Cover the default doSelfReplace closure body via the package seams: drive
// it through the manual --yes path with download/replace/verify stubbed so no
// network or filesystem mutation occurs.
func TestDoSelfReplaceDefault_FullBody(t *testing.T) {
	prevExe := osExecutableFn
	prevDl := selfupdateDownloadFn
	prevRepl := selfupdateReplaceFn
	prevVer := selfupdateVerifyVerFn
	t.Cleanup(func() {
		osExecutableFn = prevExe
		selfupdateDownloadFn = prevDl
		selfupdateReplaceFn = prevRepl
		selfupdateVerifyVerFn = prevVer
	})

	osExecutableFn = func() (string, error) { return "/fake/specscore", nil }
	selfupdateDownloadFn = func(context.Context, string) (string, error) { return "/tmp/fake-binary", nil }
	var replacedTarget, replacedSrc string
	selfupdateReplaceFn = func(target, src string) error {
		replacedTarget, replacedSrc = target, src
		return nil
	}
	verifyCalled := false
	selfupdateVerifyVerFn = func(string, string) error { verifyCalled = true; return nil }

	if err := doSelfReplace(context.Background(), "v1.2.3"); err != nil {
		t.Fatalf("doSelfReplace default body: %v", err)
	}
	if replacedTarget != "/fake/specscore" || replacedSrc != "/tmp/fake-binary" {
		t.Fatalf("replace called with target=%q src=%q", replacedTarget, replacedSrc)
	}
	if !verifyCalled {
		t.Fatal("VerifyBinaryVersion seam not called")
	}
}

func TestDoSelfReplaceDefault_ExecutableError(t *testing.T) {
	prevExe := osExecutableFn
	t.Cleanup(func() { osExecutableFn = prevExe })
	osExecutableFn = func() (string, error) { return "", errors.New("no exe") }
	if err := doSelfReplace(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("expected executable-resolution error")
	}
}

func TestDoSelfReplaceDefault_DownloadError(t *testing.T) {
	prevExe := osExecutableFn
	prevDl := selfupdateDownloadFn
	t.Cleanup(func() {
		osExecutableFn = prevExe
		selfupdateDownloadFn = prevDl
	})
	osExecutableFn = func() (string, error) { return "/fake/specscore", nil }
	selfupdateDownloadFn = func(context.Context, string) (string, error) {
		return "", errors.New("download failed")
	}
	if err := doSelfReplace(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("expected download error")
	}
}

func TestDoSelfReplaceDefault_ReplaceError(t *testing.T) {
	prevExe := osExecutableFn
	prevDl := selfupdateDownloadFn
	prevRepl := selfupdateReplaceFn
	t.Cleanup(func() {
		osExecutableFn = prevExe
		selfupdateDownloadFn = prevDl
		selfupdateReplaceFn = prevRepl
	})
	osExecutableFn = func() (string, error) { return "/fake/specscore", nil }
	selfupdateDownloadFn = func(context.Context, string) (string, error) { return "/tmp/x", nil }
	selfupdateReplaceFn = func(string, string) error { return errors.New("replace failed") }
	if err := doSelfReplace(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("expected replace error")
	}
}

// isInteractive default: the success path (Stat succeeds) plus the Stat-error
// path (stdin closed) are both exercised here by invoking the package-level
// default directly (without overriding it), so the var-initializer body itself
// is covered.
func TestIsInteractiveDefault(t *testing.T) {
	// Happy path: exercise whatever the runner provides for stdin.
	_ = isInteractive()

	// Force the Stat-error branch by swapping os.Stdin for a closed file.
	origStdin := os.Stdin
	r, _, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if isInteractive() {
		t.Fatal("closed stdin should report non-interactive")
	}
}

// resolveLatest default: invoking the package-level default directly with a
// cancelled context exercises its var-initializer body without a real network
// round-trip (the request fails fast).
func TestResolveLatestDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolveLatest(ctx); err == nil {
		t.Fatal("expected error from cancelled-context release lookup")
	}
}

// selfupdateDownloadFn default: invoking the package-level default closure
// directly with a cancelled context exercises its body (the inline
// Downloader{}.DownloadAndVerify wrapper) without a real download.
func TestSelfupdateDownloadFnDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := selfupdateDownloadFn(ctx, "1.0.0"); err == nil {
		t.Fatal("expected error from cancelled-context download")
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
