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
)

func runAgentCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := agentCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func setupSpecScoreProject(t *testing.T, title string) string {
	t.Helper()
	root := t.TempDir()
	cfg := projectdef.SpecConfig{}
	if title != "" {
		cfg.Project = &projectdef.ProjectConfig{Title: title}
	}
	if err := projectdef.WriteSpecConfig(root, cfg); err != nil {
		t.Fatalf("writing specscore.yaml: %v", err)
	}
	return root
}

func TestAgentSetup_CommaSeparatedAgents(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	// "claude,cursor" (with stray spaces and a trailing comma) must be
	// equivalent to passing the agents as separate positional args.
	out, _, err := runAgentCmd(t, "setup", " claude , cursor ,", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "CLAUDE.md (created)") {
		t.Errorf("missing CLAUDE.md line: %q", out)
	}
	if !strings.Contains(out, ".cursor/rules/specscore.mdc (created)") {
		t.Errorf("missing cursor line: %q", out)
	}
	for _, rel := range []string{"CLAUDE.md", ".cursor/rules/specscore.mdc"} {
		if _, statErr := os.Stat(filepath.Join(root, rel)); statErr != nil {
			t.Errorf("%s not created: %v", rel, statErr)
		}
	}
}

func TestAgentSkillsDirRegistry(t *testing.T) {
	// Only claude and cursor have a skills directory in the MVP.
	want := map[string]string{
		"claude": ".claude/skills",
		"cursor": ".cursor/skills",
	}
	for _, a := range supportedAgents {
		if got := a.skillsDir; got != want[a.name] {
			t.Errorf("agent %q skillsDir = %q, want %q", a.name, got, want[a.name])
		}
	}
}

func TestAgentSetup_NonSkillsDirAgentInstructionOnly(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	if _, _, err := runAgentCmd(t, "setup", "codex", "--project", root); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "codex.md")); statErr != nil {
		t.Errorf("codex.md not created: %v", statErr)
	}
	// Codex has no skills directory — none must be created for it.
	if _, statErr := os.Stat(filepath.Join(root, ".codex", "skills")); statErr == nil {
		t.Error("no skills directory should be created for codex")
	}
}

func TestAgentSetup_SingleAgent(t *testing.T) {
	root := setupSpecScoreProject(t, "My Project")
	out, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "CLAUDE.md (created)") {
		t.Errorf("expected created message, got: %q", out)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if readErr != nil {
		t.Fatalf("CLAUDE.md missing: %v", readErr)
	}
	if !strings.Contains(string(content), "My Project") {
		t.Error("CLAUDE.md should contain the project title")
	}
}

func TestAgentSetup_MultipleAgents(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	out, _, err := runAgentCmd(t, "setup", "claude", "copilot", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "CLAUDE.md (created)") {
		t.Errorf("missing CLAUDE.md line: %q", out)
	}
	if !strings.Contains(out, ".github/copilot-instructions.md (created)") {
		t.Errorf("missing copilot line: %q", out)
	}
	if _, statErr := os.Stat(filepath.Join(root, "CLAUDE.md")); statErr != nil {
		t.Errorf("CLAUDE.md not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".github", "copilot-instructions.md")); statErr != nil {
		t.Errorf(".github/copilot-instructions.md not created: %v", statErr)
	}
}

func TestAgentSetup_AllFlag(t *testing.T) {
	root := setupSpecScoreProject(t, "All Agents")
	out, _, err := runAgentCmd(t, "setup", "--all", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	expectedFiles := []string{
		"GEMINI.md",
		"CLAUDE.md",
		"codex.md",
		".github/copilot-instructions.md",
		".cursor/rules/specscore.mdc",
	}
	for _, f := range expectedFiles {
		if _, statErr := os.Stat(filepath.Join(root, f)); statErr != nil {
			t.Errorf("missing %s: %v", f, statErr)
		}
	}
	// AGENTS.md should be created (for opencode/pi.dev), but only once.
	if _, statErr := os.Stat(filepath.Join(root, "AGENTS.md")); statErr != nil {
		t.Errorf("missing AGENTS.md: %v", statErr)
	}
	// Second agent sharing AGENTS.md should be skipped.
	if !strings.Contains(out, "already written this run") {
		t.Errorf("expected duplicate-path skip message, got: %q", out)
	}
}

func TestAgentSetup_UnknownAgent(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	_, _, err := runAgentCmd(t, "setup", "notreal", "--project", root)
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("expected 'unknown agent' in error: %v", err)
	}
}

func TestAgentSetup_NoArgsNoAll(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	_, _, err := runAgentCmd(t, "setup", "--project", root)
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "specify at least one agent") {
		t.Errorf("expected usage hint in error: %v", err)
	}
}

func TestAgentSetup_AllAndPositionalConflict(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	_, _, err := runAgentCmd(t, "setup", "--all", "claude", "--project", root)
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error: %v", err)
	}
}

func TestAgentSetup_SkipsExisting(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	existing := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(existing, []byte("existing content"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "skipped, already exists") {
		t.Errorf("expected skip message, got: %q", out)
	}
	content, _ := os.ReadFile(existing)
	if string(content) != "existing content" {
		t.Error("existing file should be preserved")
	}
}

func TestAgentSetup_ForceOverwrites(t *testing.T) {
	root := setupSpecScoreProject(t, "Forced")
	existing := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runAgentCmd(t, "setup", "claude", "--force", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "overwritten") {
		t.Errorf("expected overwritten message, got: %q", out)
	}
	content, _ := os.ReadFile(existing)
	if string(content) == "old" {
		t.Error("file should have been overwritten")
	}
	if !strings.Contains(string(content), "Forced") {
		t.Error("overwritten file should contain project title")
	}
}

func TestAgentSetup_NotSpecScoreRepo(t *testing.T) {
	root := t.TempDir() // no specscore.yaml, no spec/features/
	_, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if err == nil {
		t.Fatal("expected error for non-specscore repo")
	}
	got := exitCodeOf(err)
	// resolveSpecRoot returns NotFound (3) when walking up finds nothing.
	if got != exitcode.NotFound {
		t.Errorf("exit code: got %d want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestAgentSetup_SpecFeaturesButNoYaml(t *testing.T) {
	root := t.TempDir()
	// Create spec/features/ so resolveSpecRoot finds a root, but no specscore.yaml.
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if got := exitCodeOf(err); got != exitcode.TargetNotSpecScore {
		t.Errorf("exit code: got %d want %d (TargetNotSpecScore)", got, exitcode.TargetNotSpecScore)
	}
}

func TestAgentSetup_CreatesParentDirs(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	_, _, err := runAgentCmd(t, "setup", "copilot", "cursor", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".github")); statErr != nil {
		t.Errorf(".github/ not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".cursor", "rules")); statErr != nil {
		t.Errorf(".cursor/rules/ not created: %v", statErr)
	}
}

func TestAgentSetup_ContentCallerFlag(t *testing.T) {
	tests := []struct {
		agent    string
		file     string
		callerID string
	}{
		{"claude", "CLAUDE.md", "claude"},
		{"copilot", ".github/copilot-instructions.md", "copilot"},
		{"cursor", ".cursor/rules/specscore.mdc", "cursor"},
		{"codex", "codex.md", "codex"},
		{"antigravity.google", "GEMINI.md", "antigravity.google"},
		{"opencode", "AGENTS.md", "opencode"},
	}
	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			root := setupSpecScoreProject(t, "TestProj")
			_, _, err := runAgentCmd(t, "setup", tc.agent, "--project", root)
			if err != nil {
				t.Fatalf("setup %s: %v", tc.agent, err)
			}
			content, readErr := os.ReadFile(filepath.Join(root, tc.file))
			if readErr != nil {
				t.Fatalf("missing %s: %v", tc.file, readErr)
			}
			if !strings.Contains(string(content), "--caller "+tc.callerID) {
				t.Errorf("%s should contain --caller %s", tc.file, tc.callerID)
			}
			if !strings.Contains(string(content), "TestProj") {
				t.Errorf("%s should contain project title", tc.file)
			}
		})
	}
}

func TestAgentSetup_PiDevContent(t *testing.T) {
	root := setupSpecScoreProject(t, "Pi Test")
	_, _, err := runAgentCmd(t, "setup", "pi.dev", "--project", root)
	if err != nil {
		t.Fatalf("setup pi.dev: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(string(content), "--caller pi.dev") {
		t.Error("AGENTS.md should contain --caller pi.dev")
	}
	if !strings.Contains(string(content), "SPECSCORE_CALLER=pi.dev") {
		t.Error("AGENTS.md should contain SPECSCORE_CALLER=pi.dev")
	}
}

func TestAgentSetup_CursorMDCFormat(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	_, _, err := runAgentCmd(t, "setup", "cursor", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "specscore.mdc"))
	s := string(content)
	if !strings.HasPrefix(s, "---\n") {
		t.Error("cursor MDC file should start with YAML frontmatter delimiter")
	}
	if !strings.Contains(s, "alwaysApply: true") {
		t.Error("cursor MDC should have alwaysApply: true")
	}
	if !strings.Contains(s, "description:") {
		t.Error("cursor MDC should have a description field")
	}
}

func TestAgentSetup_ClaudePluginPointer(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	_, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(content), "/plugin install specscore@specscore") {
		t.Error("CLAUDE.md should reference the plugin install command")
	}
}

func TestAgentSetup_DuplicateRelPath(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	out, _, err := runAgentCmd(t, "setup", "pi.dev", "opencode", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	// First writes AGENTS.md, second is skipped.
	if !strings.Contains(out, "AGENTS.md (created)") {
		t.Errorf("expected created message: %q", out)
	}
	if !strings.Contains(out, "already written this run") {
		t.Errorf("expected duplicate skip message: %q", out)
	}
}

func TestAgentSetup_MkdirAllError(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	old := osMkdirAllFn
	osMkdirAllFn = func(_ string, _ os.FileMode) error {
		return errors.New("mock mkdir error")
	}
	t.Cleanup(func() { osMkdirAllFn = old })

	_, _, err := runAgentCmd(t, "setup", "copilot", "--project", root)
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit code: got %d want %d", got, exitcode.Unexpected)
	}
}

func TestAgentSetup_WriteFileError(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	old := osWriteFileFn
	osWriteFileFn = func(_ string, _ []byte, _ os.FileMode) error {
		return errors.New("mock write error")
	}
	t.Cleanup(func() { osWriteFileFn = old })

	_, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit code: got %d want %d", got, exitcode.Unexpected)
	}
}

func TestAgentSetup_ReadSpecConfigTitle(t *testing.T) {
	root := setupSpecScoreProject(t, "Custom Title")
	_, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(content), "Custom Title") {
		t.Error("should use project title from specscore.yaml")
	}
}

func TestAgentSetup_FallbackTitleOnBadConfig(t *testing.T) {
	root := t.TempDir()
	// Write a specscore.yaml that ReadSpecConfig will fail on (no schema header).
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("title: broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	_, _, err := runAgentCmd(t, "setup", "claude", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	// Should fall back to directory basename.
	baseName := filepath.Base(root)
	if !strings.Contains(string(content), baseName) {
		t.Errorf("should fall back to dir basename %q, got: %s", baseName, content)
	}
}

func TestAgentSetup_ResolvesProjectViaCwd(t *testing.T) {
	root := setupSpecScoreProject(t, "CWD Test")
	withCwd(t, root)
	out, _, err := runAgentCmd(t, "setup", "claude")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "CLAUDE.md (created)") {
		t.Errorf("expected created: %q", out)
	}
}

func TestAgentSetup_GetCwdError(t *testing.T) {
	old := osGetwdFn
	osGetwdFn = func() (string, error) {
		return "", errors.New("mock cwd error")
	}
	t.Cleanup(func() { osGetwdFn = old })

	_, _, err := runAgentCmd(t, "setup", "claude")
	if err == nil {
		t.Fatal("expected error when cwd fails")
	}
}

func TestAgentSetup_FilepathAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(_ string) (string, error) {
		return "", errors.New("mock abs error")
	}
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runAgentCmd(t, "setup", "claude", "--project", "/some/path")
	if err == nil {
		t.Fatal("expected error when filepath.Abs fails")
	}
}

func TestAgentSetup_ForceCreatesNew(t *testing.T) {
	root := setupSpecScoreProject(t, "")
	out, _, err := runAgentCmd(t, "setup", "claude", "--force", "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "overwritten") {
		t.Errorf("expected 'overwritten' message: %q", out)
	}
}

func TestAgentSetup_HelpOutput(t *testing.T) {
	out, _, err := runAgentCmd(t, "setup", "--help")
	if err != nil {
		t.Fatalf("help should not error: %v", err)
	}
	for _, agent := range supportedAgentNames() {
		if !strings.Contains(out, agent) {
			t.Errorf("help should list agent %q: %s", agent, out)
		}
	}
}

func TestAgentCommand_BareShowsHelp(t *testing.T) {
	out, _, _ := runAgentCmd(t)
	if !strings.Contains(out, "setup") {
		t.Errorf("bare 'agent' should show help listing setup subcommand: %q", out)
	}
}

func TestSupportedAgentNames(t *testing.T) {
	names := supportedAgentNames()
	if len(names) != len(supportedAgents) {
		t.Errorf("supportedAgentNames length: got %d want %d", len(names), len(supportedAgents))
	}
	for i, n := range names {
		if n != supportedAgents[i].name {
			t.Errorf("name[%d]: got %q want %q", i, n, supportedAgents[i].name)
		}
	}
}

func TestFindAgent(t *testing.T) {
	if _, ok := findAgent("claude"); !ok {
		t.Error("findAgent should find 'claude'")
	}
	if _, ok := findAgent("nonexistent"); ok {
		t.Error("findAgent should not find 'nonexistent'")
	}
}
