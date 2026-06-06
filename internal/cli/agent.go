package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

var osMkdirAllFn = os.MkdirAll

type agentDef struct {
	name    string
	relPath string
	// skillsDir is the relative directory into which skill bundles are copied.
	// Empty means the agent has no known skills directory (instruction file only).
	// Verifies #ac:skills-dir-agents-mvp, #ac:non-skilldir-agent-instruction-only.
	skillsDir string
	render    func(projectTitle string) string
}

var supportedAgents = []agentDef{
	{"antigravity.google", "GEMINI.md", "", antigravityTemplate},
	{"claude", "CLAUDE.md", ".claude/skills", claudeTemplate},
	{"codex", "codex.md", "", codexTemplate},
	{"copilot", ".github/copilot-instructions.md", "", copilotTemplate},
	{"cursor", ".cursor/rules/specscore.mdc", ".cursor/skills", cursorTemplate},
	{"opencode", "AGENTS.md", "", opencodeTemplate},
	{"pi.dev", "AGENTS.md", "", piTemplate},
}

func supportedAgentNames() []string {
	names := make([]string, len(supportedAgents))
	for i, a := range supportedAgents {
		names[i] = a.name
	}
	return names
}

// expandAgentArgs splits each positional argument on commas so that
// "claude,codex" is equivalent to "claude codex". Whitespace around each
// field is trimmed and empty fields are dropped. Verifies #ac:comma-separated-agents.
func expandAgentArgs(args []string) []string {
	var out []string
	for _, a := range args {
		for _, part := range strings.Split(a, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func findAgent(name string) (agentDef, bool) {
	for _, a := range supportedAgents {
		if a.name == name {
			return a, true
		}
	}
	return agentDef{}, false
}

func agentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "AI coding agent integration — configure agents for this SpecScore project",
	}
	cmd.AddCommand(agentSetupCommand())
	return cmd
}

func agentSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup [agent-name...]",
		Short: "Generate agent-specific instruction files for this SpecScore project",
		Long: `Writes an instruction/rules file for each named AI coding agent,
teaching the agent about SpecScore conventions and CLI commands.

Supported agents: ` + strings.Join(supportedAgentNames(), ", ") + `

Examples:
  specscore agent setup claude
  specscore agent setup claude copilot cursor
  specscore agent setup --all`,
		Args: cobra.ArbitraryArgs,
		RunE: runAgentSetup,
	}
	cmd.Flags().Bool("all", false, "Configure all supported agents")
	cmd.Flags().Bool("force", false, "Overwrite existing config files and skill files")
	cmd.Flags().Bool("no-skills", false, "Write instruction files only; do not copy skill bundles")
	cmd.Flags().String("project", "", "Project root (autodetected from current directory if omitted)")
	return cmd
}

func runAgentSetup(cmd *cobra.Command, args []string) error {
	allFlag, _ := cmd.Flags().GetBool("all")
	force, _ := cmd.Flags().GetBool("force")
	noSkills, _ := cmd.Flags().GetBool("no-skills")
	projectFlag, _ := cmd.Flags().GetString("project")

	args = expandAgentArgs(args)

	if allFlag && len(args) > 0 {
		return exitcode.InvalidArgsError("--all and positional agent names are mutually exclusive")
	}
	if !allFlag && len(args) == 0 {
		return exitcode.InvalidArgsErrorf("specify at least one agent name or --all\nsupported agents: %s", strings.Join(supportedAgentNames(), ", "))
	}

	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(filepath.Join(root, projectdef.SpecConfigFile)); os.IsNotExist(statErr) {
		return exitcode.Newf(exitcode.TargetNotSpecScore,
			"no specscore.yaml at %s — run specscore init first", root)
	}

	projectTitle := filepath.Base(root)
	if cfg, readErr := projectdef.ReadSpecConfig(root); readErr == nil && cfg.Project != nil && cfg.Project.Title != "" {
		projectTitle = cfg.Project.Title
	}

	var agents []agentDef
	if allFlag {
		agents = supportedAgents
	} else {
		for _, name := range args {
			a, ok := findAgent(name)
			if !ok {
				return exitcode.InvalidArgsErrorf("unknown agent %q — supported agents: %s", name, strings.Join(supportedAgentNames(), ", "))
			}
			agents = append(agents, a)
		}
	}

	w := cmd.OutOrStdout()
	seen := make(map[string]bool)
	for _, a := range agents {
		if seen[a.relPath] {
			_, _ = fmt.Fprintf(w, "skipped %s (already written this run)\n", a.relPath)
			continue
		}
		seen[a.relPath] = true

		if err := writeAgentFile(w, root, a.relPath, []byte(a.render(projectTitle)), force); err != nil {
			return err
		}
	}

	if !noSkills {
		if err := copySkillBundles(w, root, agents, force); err != nil {
			return err
		}
	}

	return nil
}

// writeAgentFile writes content to root/relPath, creating parent directories as
// needed, and reports the action on its own line: "added" for a new file,
// "modified" for an overwrite under --force, or "skipped" for an existing file
// left untouched. Verifies #ac:per-path-change-report, #ac:skill-copy-skips-existing,
// #ac:skills-parent-dirs-created.
func writeAgentFile(w io.Writer, root, relPath string, content []byte, force bool) error {
	abs := filepath.Join(root, relPath)
	_, statErr := os.Stat(abs)
	exists := statErr == nil

	if exists && !force {
		_, _ = fmt.Fprintf(w, "skipped %s (exists — use --force to overwrite)\n", relPath)
		return nil
	}

	if dir := filepath.Dir(abs); dir != root {
		if mkErr := osMkdirAllFn(dir, 0o755); mkErr != nil {
			return exitcode.UnexpectedErrorf("creating directory %s: %v", dir, mkErr)
		}
	}
	if writeErr := osWriteFileFn(abs, content, 0o644); writeErr != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", relPath, writeErr)
	}

	action := "added"
	if exists {
		action = "modified"
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", action, relPath)
	return nil
}

// copySkillBundles downloads the SpecScore skill bundles and copies them into
// the skills directory of every requested agent that has one. Agents without a
// skills directory are left untouched. A download failure surfaces as exit 10
// with no partial skills directory written. Verifies #ac:skill-copy-default-on,
// #ac:skill-copy-cursor-default, #ac:skill-copy-claude-always,
// #ac:skill-source-offline-fails.
func copySkillBundles(w io.Writer, root string, agents []agentDef, force bool) error {
	var targets []agentDef
	for _, a := range agents {
		if a.skillsDir != "" {
			targets = append(targets, a)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	bundle, err := fetchSkillBundleFn()
	if err != nil {
		return exitcode.UnexpectedErrorf("downloading skill bundles: %v", err)
	}

	for _, a := range targets {
		for _, f := range bundle {
			rel := filepath.Join(a.skillsDir, f.relPath)
			if writeErr := writeAgentFile(w, root, rel, f.content, force); writeErr != nil {
				return writeErr
			}
		}
	}
	return nil
}

func specscopeSection(callerID, projectTitle string) string {
	return fmt.Sprintf(`This is a SpecScore-managed project (%s).

Specifications live under spec/ following the SpecScore format:
- spec/features/ — feature specifications (one per sub-system)
- spec/ideas/ — pre-spec one-pagers exploring problem-direction-MVP
- spec/issues/ — reported observations of broken behavior
- spec/decisions/ — architectural decision records
- specscore.yaml — project configuration

Key CLI commands (always pass --caller %s):

  specscore spec lint --caller %s              # validate all specs
  specscore feature list --caller %s           # list features
  specscore feature info <slug> --caller %s    # inspect a feature
  specscore idea new <slug> --caller %s        # scaffold an idea
  specscore feature new --title "..." --caller %s  # scaffold a feature
  specscore task list --caller %s              # show the task board

Conventions:
- Feature specs live at spec/features/<path>/README.md
- Ideas live at spec/ideas/<slug>.md
- Run specscore spec lint after modifying any spec artifact
- The spec tree is the source of truth for project capabilities`, projectTitle,
		callerID, callerID, callerID, callerID, callerID, callerID, callerID)
}

func claudeTemplate(projectTitle string) string {
	return fmt.Sprintf(`# %s

%s

## Claude Code Plugin

For richer integration, install the SpecScore plugin:

%s

The plugin provides per-command skills that teach Claude Code when to call
which command, which flags to pass, and how to interpret exit codes.
`, projectTitle, specscopeSection("claude", projectTitle),
		"```\n/plugin install specscore@specscore\n```")
}

func copilotTemplate(projectTitle string) string {
	return fmt.Sprintf(`# %s

%s
`, projectTitle, specscopeSection("copilot", projectTitle))
}

func cursorTemplate(projectTitle string) string {
	return fmt.Sprintf(`---
description: SpecScore project conventions and CLI usage
alwaysApply: true
---

# %s

%s
`, projectTitle, specscopeSection("cursor", projectTitle))
}

func codexTemplate(projectTitle string) string {
	return fmt.Sprintf(`# %s

%s
`, projectTitle, specscopeSection("codex", projectTitle))
}

func antigravityTemplate(projectTitle string) string {
	return fmt.Sprintf(`# %s

%s
`, projectTitle, specscopeSection("antigravity.google", projectTitle))
}

func piTemplate(projectTitle string) string {
	return agentsMDTemplate("pi.dev", projectTitle)
}

func opencodeTemplate(projectTitle string) string {
	return agentsMDTemplate("opencode", projectTitle)
}

func agentsMDTemplate(callerID, projectTitle string) string {
	return fmt.Sprintf(`# %s

%s

## Caller Identification

Set the environment variable SPECSCORE_CALLER=%s so that specscore
CLI telemetry can identify your agent. Alternatively, pass --caller %s
on every invocation.
`, projectTitle, specscopeSection(callerID, projectTitle), callerID, callerID)
}
