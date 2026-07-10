package cli

// Feature implemented: cli/rehearse/run (REQ: scenario-discovery,
// REQ: scenario-shape, REQ: bash-block, REQ: hurl-block, REQ: sql-block,
// REQ: dtql-block, REQ: graphql-block, REQ: context-bag, REQ: run-report)
// Feature: cli/rehearse/evidence (REQ: report-out, REQ: report-provenance)
// Feature: cli/rehearse/new (REQ: scaffold-new)

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/bash"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/dtqlblock"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/graphql"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/hurl"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/sqlblock"
	"github.com/specscore/specscore-cli/internal/rehearse/runner"
	"github.com/specscore/specscore-cli/internal/rehearse/scaffold"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// writeReportFn is the test seam for runner.WriteReport so CLI-layer tests can
// stub the write without touching the filesystem.
// Feature: cli/rehearse/evidence (REQ: report-out)
var writeReportFn = runner.WriteReport

// Test seams for rehearseNewCommand. Defaults to real OS/git implementations.
// Feature: cli/rehearse/new (REQ: scaffold-new)
//
// rehearseNewGitExecFn receives the full shell-style command string (e.g.
// "git commit -m \"msg\" path") for test assertion convenience; the default
// production implementation rebuilds exec.Command args from the string via
// strings.Fields then re-joins any -m value that was quoted.  Tests inject
// a stub that simply records the string and returns success.
var (
	rehearseNewReadFileFn func(path string) (string, error) = func(path string) (string, error) {
		b, err := os.ReadFile(path)
		return string(b), err
	}
	rehearseNewMkdirAllFn  func(path string, perm os.FileMode) error              = os.MkdirAll
	rehearseNewWriteFileFn func(path string, data []byte, perm os.FileMode) error = os.WriteFile
	rehearseNewStatFn      func(path string) (os.FileInfo, error)                 = os.Stat
	// gitExecFn receives the git sub-command and all its arguments as separate
	// strings (i.e. everything after "git"). Production default shells out to
	// exec.Command; tests inject a stub.
	rehearseNewGitExecFn func(args ...string) ([]byte, error) = gitRunArgs
)

// rehearseCommand returns the "rehearse" command group — the acceptance-
// evidence runner for markdown scenarios.
func rehearseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rehearse",
		Short: "Execute markdown acceptance scenarios (Rehearse evidence runner)",
		Long: `The rehearse command group executes markdown acceptance scenarios:
files carrying **Verifies:** AC identity in body metadata plus fenced
executable step blocks.

Running "specscore rehearse" with no subcommand prints this help and exits 0.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(rehearseRunCommand())
	cmd.AddCommand(rehearseNewCommand())
	return cmd
}

// rehearseRegistry returns the block-executor registry for rehearse run:
// the five v0.3 block kinds (bash, hurl, sql, dtql, graphql).
func rehearseRegistry() blocks.Registry {
	return blocks.NewRegistry(bash.New(), hurl.New(), sqlblock.New(), dtqlblock.New(), graphql.New())
}

func rehearseRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [paths...]",
		Short: "Run markdown scenario files and report per-scenario pass/fail",
		Long: `Executes markdown acceptance scenarios. Paths may be files, directories
(scanned recursively for *.md, excluding README.md), or globs. With no
paths inside a SpecScore repo, all spec/features/**/_tests/ scenarios run.
Explicit paths work in any directory — no specscore.yaml required
(standalone mode).

A scenario's fenced step blocks run in order in one scenario-scoped temp
working dir; the first failing step fails the scenario and the remaining
steps are skipped-after-failure. A scenario with zero step blocks is
reported no-steps.

Exit code: 0 when no scenario failed; 1 when any failed; 2 on usage or
config errors — including when discovery matches zero scenario files.`,
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE:         runRehearseRun,
	}
	cmd.Flags().String("format", "human", "output format: human or json")
	cmd.Flags().String("report-out", "", "persist the run report as a JSON envelope to this path (creates parent dirs; exit 2 if unwritable)")
	return cmd
}

func runRehearseRun(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "human" && format != "json" {
		return exitcode.InvalidArgsErrorf("unknown --format %q: expected human or json", format)
	}
	reportOut, _ := cmd.Flags().GetString("report-out")

	// The working directory is needed for default discovery (no args) and for
	// git provenance when --report-out is set.
	var cwd string
	if len(args) == 0 || reportOut != "" {
		var err error
		cwd, err = osGetwdFn()
		if err != nil {
			return exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
	}

	startedAt := runner.NowFn()

	files, err := runner.Discover(args, cwd)
	if err != nil {
		return err
	}
	reports := runner.Run(rehearseRegistry(), files)

	w := cmd.OutOrStdout()
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			return exitcode.UnexpectedErrorf("output error: %v", err)
		}
	} else {
		runner.RenderHuman(w, reports)
	}

	// Write the persisted report envelope after the stdout report is printed
	// so the user always sees scenario results. An unwritable path exits 2.
	// Feature: cli/rehearse/evidence (REQ: report-out)
	if reportOut != "" {
		if err := writeReportFn(reportOut, version, startedAt, reports, cwd); err != nil {
			return exitcode.InvalidArgsErrorf("cannot write report to %q: %v", reportOut, err)
		}
	}

	if failed := runner.CountFailed(reports); failed > 0 {
		return exitcode.ConflictErrorf("%d of %d scenario(s) failed", failed, len(reports))
	}
	return nil
}

// rehearseNewCommand returns the "rehearse new" subcommand that scaffolds a
// scenario file from an acceptance criterion reference.
//
// Usage: specscore rehearse new <feature-slug>#ac:<ac-slug>
//
// Feature: cli/rehearse/new (REQ: scaffold-new)
// Verifies: cli/rehearse/new#ac:resolve-ac-reference
// Verifies: cli/rehearse/new#ac:missing-ac-error
func rehearseNewCommand() *cobra.Command {
	var (
		force  bool
		commit bool
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "new <ac-ref>",
		Short: "Scaffold a scenario from an acceptance criterion",
		Long: `Scaffold a Rehearse scenario file for a given acceptance criterion.

<ac-ref> must be in the form <feature-slug>#ac:<ac-slug>, for example:

    specscore rehearse new cli/rehearse/new#ac:resolve-ac-reference

The command reads spec/features/<feature-slug>/README.md, extracts the
Given/When/Then text for the named AC, generates a scaffold markdown file, and
writes it to:

    spec/features/<feature-slug>/_tests/<ac-slug>.md

If the output file already exists the command exits 2 unless --force is set.

If --commit is set, a git commit is created after writing the file with the
message:

    feat(rehearse): scaffold <ac-slug> scenario

    Verifies: <feature-slug>#ac:<ac-slug>

If the commit fails but the file was written, the command exits 1 (the scaffold
survives on disk).

If --dry-run is set, the scaffold is printed to stdout without writing any file
or creating a git commit. --force and --commit are ignored in this mode.`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRehearseNew(args[0], force, commit, dryRun, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing file without error")
	cmd.Flags().BoolVar(&commit, "commit", false, "create a git commit after writing the file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print scaffold to stdout without writing files or committing")
	return cmd
}

// runRehearseNew is the testable body of `rehearse new`.
func runRehearseNew(acRef string, force, commit, dryRun bool, out io.Writer) error {
	// 1. Resolve the AC reference → raw body.
	result, err := scaffold.Resolve(acRef, rehearseNewReadFileFn)
	if err != nil {
		return exitcode.InvalidArgsErrorf("%v", err)
	}

	// 2. Extract Given/When/Then from the body.
	extracted := scaffold.Extract(result.RawBody)

	// 3. Generate the scaffold markdown.
	content := scaffold.Generate(result.FeatureSlug, result.ACSlug, extracted)

	// 4. Dry-run: print to writer and return without writing files or committing.
	// Feature: cli/rehearse/new-dry-run (AC: dry-run-flag, dry-run-ignores-flags)
	if dryRun {
		fmt.Fprint(out, content)
		return nil
	}

	// 5. Determine the output path.
	outPath := filepath.Join(
		"spec", "features", result.FeatureSlug, "_tests", result.ACSlug+".md",
	)

	// 6. Create parent directories.
	parentDir := filepath.Dir(outPath)
	if mkErr := rehearseNewMkdirAllFn(parentDir, 0o755); mkErr != nil {
		return exitcode.InvalidArgsErrorf("cannot create directory %q: %v", parentDir, mkErr)
	}

	// 7. Check for existing file (unless --force).
	if !force {
		if _, statErr := rehearseNewStatFn(outPath); statErr == nil {
			return exitcode.InvalidArgsErrorf(
				"file already exists: %s\nUse --force to overwrite or delete the file first.",
				outPath,
			)
		}
	}

	// 8. Write the scaffold file.
	if writeErr := rehearseNewWriteFileFn(outPath, []byte(content), 0o644); writeErr != nil {
		return exitcode.InvalidArgsErrorf("cannot write scaffold to %q: %v", outPath, writeErr)
	}

	// 9. Optionally create a git commit.
	if commit {
		msg := fmt.Sprintf(
			"feat(rehearse): scaffold %s scenario\n\nVerifies: %s#ac:%s",
			result.ACSlug, result.FeatureSlug, result.ACSlug,
		)
		if _, gitErr := rehearseNewGitExecFn("commit", "-m", msg, outPath); gitErr != nil {
			return exitcode.ConflictErrorf(
				"scaffold written to %s but git commit failed: %v",
				outPath, gitErr,
			)
		}
	}

	return nil
}

// gitRunArgs shells out to git with the provided sub-command arguments
// (everything after "git") and returns combined stdout+stderr output.
func gitRunArgs(args ...string) ([]byte, error) {
	return exec.Command("git", args...).CombinedOutput()
}
