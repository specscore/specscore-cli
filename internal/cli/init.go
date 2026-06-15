package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/config"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/gitremote"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// isTerminal reports whether r is an interactive terminal. Indirected
// through a package-level var so tests can stub it without depending on
// real TTY behavior. Default implementation: stat the underlying os.File
// and check ModeCharDevice. A non-*os.File reader (e.g. *bytes.Buffer in
// tests) is never a terminal.
var isTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// initCommand returns the "init" command — scaffolds a SpecScore-managed
// project root: specscore.yaml + spec/{,ideas,features}/README.md.
func initCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a SpecScore-managed project root",
		Long: `Creates specscore.yaml with the mandatory schema-pointer header on
line 1, the spec/ tree with lint-clean Ideas and Features indexes,
and infers project metadata from git remote / cwd / flags.

Refuses to clobber an existing specscore.yaml unless --force.
Idempotent on partial state — completes missing pieces without
erroring on what's already there.`,
		Args: cobra.NoArgs,
		RunE: runInit,
	}
	cmd.Flags().String("title", "", "Project title (defaults to project root basename)")
	cmd.Flags().String("host", "", "Repository host (defaults to inference from git remote)")
	cmd.Flags().String("org", "", "Repository org / owner (defaults to inference)")
	cmd.Flags().String("repo", "", "Repository name (defaults to inference)")
	cmd.Flags().String("project", "", "Project root (autodetected from current working directory if omitted)")
	cmd.Flags().BoolP("interactive", "i", false, "Prompt for each project-metadata field on stdin (requires a TTY)")
	cmd.Flags().Bool("force", false, "Overwrite an existing specscore.yaml")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	title, _ := cmd.Flags().GetString("title")
	host, _ := cmd.Flags().GetString("host")
	org, _ := cmd.Flags().GetString("org")
	repo, _ := cmd.Flags().GetString("repo")
	projectFlag, _ := cmd.Flags().GetString("project")
	interactive, _ := cmd.Flags().GetBool("interactive")
	force, _ := cmd.Flags().GetBool("force")

	root, err := resolveProjectRootForInit(projectFlag)
	if err != nil {
		return err
	}

	configPath := filepath.Join(root, projectdef.SpecConfigFile)
	if _, statErr := os.Stat(configPath); statErr == nil {
		if !force {
			return exitcode.ConflictErrorf(
				"specscore.yaml already exists at %s (pass --force to overwrite)",
				configPath,
			)
		}
	} else if !os.IsNotExist(statErr) {
		return exitcode.UnexpectedErrorf("checking %s: %v", configPath, statErr)
	}

	// Interactive: gather metadata before inference.
	if interactive {
		if !isTerminal(cmd.InOrStdin()) {
			return exitcode.InvalidArgsError(
				"-i requires an interactive terminal; either run in a TTY or omit -i to use flags + inference",
			)
		}
		if err := promptProjectMetadata(cmd.InOrStdin(), cmd.OutOrStdout(), &title, &host, &org, &repo); err != nil {
			return err
		}
	}

	// Inference fills any still-empty fields. Always runs after interactive
	// so that inference fills only what neither flag nor prompt supplied.
	cfg := buildSpecConfig(root, title, host, org, repo)

	if err := projectdef.WriteSpecConfig(root, cfg); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", projectdef.SpecConfigFile, err)
	}

	// Index files are written only when missing — idempotent partial-state
	// resume per req:partial-state-resume.
	for _, w := range []struct {
		path    string
		content string
	}{
		{"spec/README.md", specReadmeContent(cfg)},
		{"spec/ideas/README.md", ideasIndexContent(cfg)},
		{"spec/features/README.md", featuresIndexContent(cfg)},
	} {
		if err := writeMissingIndex(root, w.path, w.content); err != nil {
			return exitcode.UnexpectedErrorf("writing %s: %v", w.path, err)
		}
	}

	// Ensure specscore.local.yaml (the uncommitted per-user config layer) is
	// git-ignored, and warn if it is already tracked.
	if msg := config.EnsureLocalGitignoredMsg(root); msg != "" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "warning: "+msg)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized SpecScore project at %s\n", root)
	return nil
}

// resolveProjectRootForInit resolves the project root for init. Unlike
// resolveSpecRoot used by other commands, init does NOT walk upward
// looking for an existing root — it operates on the resolved path itself.
// The path MUST exist and MUST be a directory.
func resolveProjectRootForInit(projectFlag string) (string, error) {
	var path string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		path = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return "", exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		path = cwd
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", exitcode.InvalidArgsErrorf("project root does not exist: %s", path)
		}
		return "", exitcode.UnexpectedErrorf("stat %s: %v", path, err)
	}
	if !st.IsDir() {
		return "", exitcode.InvalidArgsErrorf("project root is not a directory: %s", path)
	}
	return path, nil
}

// promptProjectMetadata reads four lines from r (title / host / org / repo).
// The flag value (when non-empty) is shown as the default. Empty input is
// "omit this field" per req:interactive-mode.
func promptProjectMetadata(r io.Reader, w io.Writer, title, host, org, repo *string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	prompts := []struct {
		label string
		field *string
	}{
		{"title", title},
		{"host", host},
		{"org", org},
		{"repo", repo},
	}
	for _, p := range prompts {
		def := *p.field
		if def != "" {
			_, _ = fmt.Fprintf(w, "%s [%s]: ", p.label, def)
		} else {
			_, _ = fmt.Fprintf(w, "%s: ", p.label)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return exitcode.UnexpectedErrorf("reading %s: %v", p.label, err)
			}
			// EOF — treat remaining unread fields as "accept default" (no change).
			return nil
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			// Two interpretations of empty input: "accept default" or
			// "omit this field". Per req:interactive-mode, empty means
			// "omit". When the user wants to keep a flag-supplied default,
			// they re-type it. This is the explicit choice.
			*p.field = ""
		} else {
			*p.field = input
		}
	}
	return nil
}

// buildSpecConfig assembles the final SpecConfig: explicit/prompt values
// take precedence; remaining empty fields get filled by git-remote inference
// and project-root basename. Fields that are still empty after all sources
// are omitted from the output rather than emitted as empty strings.
func buildSpecConfig(root, title, host, org, repo string) projectdef.SpecConfig {
	if title == "" {
		title = filepath.Base(root)
	}

	if host == "" || org == "" || repo == "" {
		if originURL, err := gitremote.OriginURL(root); err == nil {
			if remote, ok := gitremote.Parse(originURL); ok {
				if host == "" {
					host = remote.Host
				}
				if org == "" {
					org = remote.Owner
				}
				if repo == "" {
					repo = remote.Repo
				}
			}
		}
	}

	cfg := projectdef.SpecConfig{}
	if title != "" || host != "" || org != "" || repo != "" {
		cfg.Project = &projectdef.ProjectConfig{
			Title: title,
			Host:  host,
			Org:   org,
			Repo:  repo,
		}
	}
	return cfg
}

// writeMissingIndex writes content to root/relPath only if no file already
// exists at that path. Existing files are preserved untouched.
func writeMissingIndex(root, relPath, content string) error {
	abs := filepath.Join(root, relPath)
	if _, err := os.Stat(abs); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// specReadmeContent / ideasIndexContent / featuresIndexContent render the
// canonical index files for `specscore init`. Index files do NOT carry a
// studio toolbar — toolbars are scoped to feature artifact READMEs by the
// studio-toolbar Feature.
func specReadmeContent(_ projectdef.SpecConfig) string {
	return `# Specifications

SpecScore-formatted specifications for this project.

## Contents

| Directory | Purpose |
|---|---|
| [` + "`features/`" + `](features/README.md) | Feature specifications — one per sub-system |
| [` + "`ideas/`" + `](ideas/README.md) | Pre-spec one-pagers exploring problem-direction-MVP |

## Open Questions

None at this time.
`
}

func ideasIndexContent(_ projectdef.SpecConfig) string {
	return `---
format: https://specscore.md/ideas-index-specification
---

# Ideas

Pre-spec one-pagers. Each Idea is a lint-clean problem-direction-MVP one-pager that may later promote into one or more SpecScore Features under [` + "`features/`" + `](../features/README.md).

## Index

| Idea | Status | Date | Owner | Promotes To |
|------|--------|------|-------|-------------|

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/ideas-index-specification*
`
}

func plansIndexContent(_ projectdef.SpecConfig) string {
	return `---
format: https://specscore.md/plans-index-specification
---

# Plans

Canonical index of all plans in this repository. Each plan is a single Markdown file under ` + "`spec/plans/`" + ` governed by the [plan specification](https://specscore.md/plan-specification).

## Contents

| Plan | Status | Source | Date | Owner |
|---|---|---|---|---|

## Recently Closed

None at this time.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plans-index-specification*
`
}

func decisionsIndexContent(_ projectdef.SpecConfig) string {
	return `---
format: https://specscore.md/decisions-index-specification
---

# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/decisions-index-specification*
`
}

func featuresIndexContent(_ projectdef.SpecConfig) string {
	return `---
format: https://specscore.md/features-index-specification
---

# Features

Feature specifications for this project.

## Index

| Feature | Status | Description |
|---------|--------|-------------|

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
`
}
