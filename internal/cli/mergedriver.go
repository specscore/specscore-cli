package cli

// Features implemented: cli/merge-driver. See docs/merge-drivers.md.
//
// The event-ledger merge driver itself lives under `event merge-driver`
// (event.go) — it is event-specific, reusing pkg/event's union-merge
// algorithm. This file holds:
//
//   - `merge-driver install`, the one-time-per-clone installer that wires up
//     BOTH the events driver and the index driver via .gitattributes and
//     repo-local git config.
//   - `merge-driver index`, a second driver for the generated index README
//     files that pkg/lint's *-index-row-sync family of rules can fully
//     regenerate from their source artifacts (feature/idea/plan/task/lesson/
//     decision README files). Unlike the append-only event ledger, these
//     files have no stable merge algorithm of their own worth writing — they
//     are wholly derived, so the deterministic resolution is to regenerate
//     them, not to merge their text.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/gitremote"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

// eventsDriverName and indexDriverName are the `merge=<name>` attribute
// values and `merge.<name>.*` git config keys `merge-driver install` writes.
// Kept short and specscore-prefixed so they cannot collide with a driver an
// unrelated tool has already registered in the same repo.
const (
	eventsDriverName = "specscore-events"
	indexDriverName  = "specscore-index"
)

// indexReadmePaths are the project-root-relative paths of the generated
// index README files pkg/lint's *-index-row-sync rule family can fully
// regenerate from their source artifacts (see feature_index.go, idea_index.go,
// plan_index.go, task_index.go, decisions_index_rules.go, lesson_rules.go's
// L-003/L-004). Listed unconditionally: a .gitattributes pattern that
// matches no file in a given project is harmless, and a project that later
// grows one of these directories should not need to re-run install.
//
// This is the fixed set of index kinds pkg/lint currently knows how to
// regenerate end to end; it is not every README.md the lint package
// touches (e.g. per-feature/per-plan child READMEs are source files, not
// derived indexes, and are deliberately excluded).
var indexReadmePaths = []string{
	filepath.Join("spec", "features", "README.md"),
	filepath.Join("spec", "ideas", "README.md"),
	filepath.Join("spec", "ideas", "archived", "README.md"),
	filepath.Join("spec", "plans", "README.md"),
	filepath.Join("spec", "tasks", "README.md"),
	filepath.Join("spec", "lessons", "README.md"),
	filepath.Join("spec", "decisions", "README.md"),
	filepath.Join("spec", "decisions", "archived", "README.md"),
}

// mergeDriverLintFixFn is a test seam wrapping lint.LintWithResult so
// `merge-driver index` tests can exercise the driver's own plumbing (arg
// handling, file copy, error mapping) without needing a fully valid spec
// tree on disk; pkg/lint's own test suite covers fixer correctness.
var mergeDriverLintFixFn = lint.LintWithResult

// mergeDriverCommand returns the "merge-driver" command group.
func mergeDriverCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge-driver",
		Short: "Deterministic git merge drivers for generated SpecScore files",
		Long: `Registers and implements git custom merge drivers (gitattributes(5))
that replace line-based text merging with deterministic, format-aware
resolution for two kinds of generated SpecScore file:

  - .specscore/events.jsonl (or wherever specscore.yaml's events: block
    points it) — an append-only ledger merged by union, never by diff.
  - the generated index README files under spec/ — regenerated from their
    source artifacts rather than text-merged, since they are wholly derived.

Run 'specscore merge-driver install' once per clone/worktree; the two
'... merge-driver ...' subcommands below are what git invokes for you
during 'git merge' and are not meant to be run by hand.

Docs: docs/merge-drivers.md`,
	}
	cmd.AddCommand(mergeDriverInstallCommand())
	cmd.AddCommand(mergeDriverIndexCommand())
	return cmd
}

func mergeDriverInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Register the events-ledger and index-README git merge drivers for this repository",
		Long: `Writes the .gitattributes entries and the repo-local (never --global)
git config that route the JSONL event ledger and the generated index README
files through specscore's deterministic merge drivers instead of git's
default line-based text merge.

Both drivers invoke the 'specscore' command found on PATH at merge time, not
a path baked in at install time, so upgrading the CLI (e.g. via
'brew upgrade specscore') does not require re-running install.

Idempotent: safe to run again — matching .gitattributes lines and git config
values are left as-is rather than duplicated. Re-run it after every fresh
'git clone' or 'git worktree add': .gitattributes travels with the repository,
but the git config half of the wiring is per-clone/per-worktree and is not.

Docs: docs/merge-drivers.md`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runMergeDriverInstall,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runMergeDriverInstall(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot, err := resolveEventProjectRoot(projectFlag)
	if err != nil {
		return err
	}
	gitRoot, err := gitTopLevelFn(projectRoot)
	if err != nil {
		return exitcode.InvalidStateErrorf("resolving git repository top level: %v", err)
	}

	ledgerPath, err := event.ConfiguredLedgerPath(projectRoot)
	if err != nil {
		return exitcode.InvalidArgsErrorf("event ledger configuration: %v", err)
	}
	ledgerRel, err := filepath.Rel(gitRoot, ledgerPath)
	if err != nil || strings.HasPrefix(ledgerRel, ".."+string(filepath.Separator)) || ledgerRel == ".." {
		return exitcode.InvalidStateErrorf("configured event ledger %s is outside the git repository at %s", ledgerPath, gitRoot)
	}

	attrLines := []string{gitattributesLine(filepath.ToSlash(ledgerRel), eventsDriverName)}
	for _, rel := range indexReadmePaths {
		attrLines = append(attrLines, gitattributesLine(filepath.ToSlash(rel), indexDriverName))
	}

	added, err := ensureGitattributesFn(filepath.Join(gitRoot, ".gitattributes"), attrLines)
	if err != nil {
		return exitcode.UnexpectedErrorf("writing .gitattributes: %v", err)
	}

	configs := [][2]string{
		{"merge." + eventsDriverName + ".name", "SpecScore deterministic union merge for the JSONL event ledger"},
		{"merge." + eventsDriverName + ".driver", "specscore event merge-driver %O %A %B"},
		{"merge." + indexDriverName + ".name", "SpecScore regenerated-index merge (spec lint --fix; never text-merged)"},
		{"merge." + indexDriverName + ".driver", "specscore merge-driver index %O %A %B %P"},
	}
	for _, kv := range configs {
		if err := gitConfigSetFn(gitRoot, kv[0], kv[1]); err != nil {
			return exitcode.UnexpectedErrorf("git config %s: %v", kv[0], err)
		}
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "gitattributes=%s added=%d\n", filepath.Join(gitRoot, ".gitattributes"), added)
	_, _ = fmt.Fprintf(w, "merge.%s.driver installed\n", eventsDriverName)
	_, _ = fmt.Fprintf(w, "merge.%s.driver installed\n", indexDriverName)
	return nil
}

// gitTopLevelFn and gitConfigSetFn / ensureGitattributesFn are test seams
// (see test_seams.go convention used throughout this package).
var (
	gitTopLevelFn         = gitremote.TopLevel
	gitConfigSetFn        = gitremote.ConfigSet
	ensureGitattributesFn = ensureGitattributes
)

// gitattributesLine renders one `<pattern> merge=<driver>` line.
func gitattributesLine(pattern, driver string) string {
	return pattern + " merge=" + driver
}

// ensureGitattributes appends any of lines not already present (as an exact
// line match) to the .gitattributes file at path, creating the file if
// necessary. Returns how many lines were newly added. Existing content and
// line order are preserved; new lines are appended in the order given.
func ensureGitattributes(path string, lines []string) (int, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	present := make(map[string]struct{})
	for _, l := range strings.Split(string(existing), "\n") {
		present[strings.TrimRight(l, "\r")] = struct{}{}
	}

	content := existing
	added := 0
	for _, line := range lines {
		if _, ok := present[line]; ok {
			continue
		}
		if len(content) > 0 && content[len(content)-1] != '\n' {
			content = append(content, '\n')
		}
		content = append(content, []byte(line+"\n")...)
		present[line] = struct{}{}
		added++
	}
	if added == 0 {
		return 0, nil
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", path, err)
	}
	return added, nil
}

// mergeDriverIndexCommand returns `merge-driver index` — a git custom merge
// driver for the generated index README files. See the package doc comment
// at the top of this file for why regeneration, not text-merging, is the
// deterministic resolution for these files.
func mergeDriverIndexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index <base> <ours> <theirs> <path>",
		Short: "Git custom merge driver for generated SpecScore index README files (see `specscore merge-driver install`)",
		Long: `Implements the 4-argument contract a git custom merge driver command is
invoked with when its merge.<name>.driver config includes %P (the path,
relative to the repository root, of the attributed file — gitattributes(5)).
%P is required here because one driver command handles every generated
index kind (features, ideas, plans, tasks, lessons, decisions).

Every attributed index README (spec/features/README.md, spec/ideas/README.md
and its archived/ counterpart, spec/plans/README.md, spec/tasks/README.md,
spec/lessons/README.md, spec/decisions/README.md and its archived/
counterpart) is entirely DERIVED from the individual artifact files it
lists — see the *-index-row-sync rule family in docs/lint-rules.md. A
textual git conflict on one of these files is therefore never a real
content conflict to resolve line by line: this command ignores the three
git-supplied versions of the file and instead regenerates it in place from
whatever artifact files are currently checked out (the equivalent of
'specscore spec lint --fix', scoped to the project containing <path>), then
copies the freshly regenerated bytes over <ours> (%A).

Regeneration reads sibling files as they exist on disk at the moment git
invokes this driver, from the very same merge — if one of those sibling
files is itself still conflict-marked (rare: it would mean this merge
touched both an index and one of the rows it derives from in ways git could
not fast-forward past each other), the regenerated content may be built
from a temporarily inconsistent tree. This command does not try to detect
that case; it only fails closed (leaves <ours> untouched, exits non-zero)
when regeneration itself errors, e.g. a source file fails to parse. When in
doubt after a merge with conflicts elsewhere, re-run
'specscore spec lint --fix' once every conflict is resolved — it is
idempotent, so a redundant run changes nothing.

Install once per repository with:

    specscore merge-driver install

Docs: docs/merge-drivers.md`,
		Args:          cobra.ExactArgs(4),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runMergeDriverIndex,
	}
	return cmd
}

func runMergeDriverIndex(cmd *cobra.Command, args []string) error {
	// args[0] is %O (base) — unused: regeneration reads current sibling
	// files on disk, not any of the three git-supplied blobs.
	ours, path := args[1], args[3]

	startDir, err := osGetwdFn()
	if err != nil {
		return exitcode.UnexpectedErrorf("getwd: %v", err)
	}
	projectRoot, err := findRepoConfigRoot(startDir)
	if err != nil {
		return err
	}
	specRoot := filepath.Join(projectRoot, "spec")

	if _, lintErr := mergeDriverLintFixFn(lint.Options{
		SpecRoot:   specRoot,
		Fix:        true,
		CLIVersion: buildInfo.Version,
	}); lintErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "specscore merge-driver index: regenerating %s: %v\n", path, lintErr)
		return exitcode.ConflictErrorf("regenerating index %s: %v", path, lintErr)
	}

	regenerated := filepath.Join(projectRoot, filepath.FromSlash(path))
	data, err := osReadFileFn(regenerated)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "specscore merge-driver index: reading regenerated %s: %v\n", regenerated, err)
		return exitcode.ConflictErrorf("reading regenerated index %s: %v", path, err)
	}
	if err := osWriteFileFn(ours, data, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing merged index to %s: %v", ours, err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "regenerated=%s\n", path)
	return nil
}
