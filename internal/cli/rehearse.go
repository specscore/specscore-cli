package cli

// Feature implemented: cli/rehearse/run (REQ: scenario-discovery,
// REQ: scenario-shape, REQ: bash-block, REQ: sql-block, REQ: dtql-block,
// REQ: run-report)

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/bash"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/dtqlblock"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/sqlblock"
	"github.com/specscore/specscore-cli/internal/rehearse/runner"
	"github.com/specscore/specscore-cli/pkg/exitcode"
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
	return cmd
}

// rehearseRegistry returns the block-executor registry for rehearse run.
// v0.3 registers bash, sql and dtql; hurl/graphql executors join here as
// they land.
func rehearseRegistry() blocks.Registry {
	return blocks.NewRegistry(bash.New(), sqlblock.New(), dtqlblock.New())
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
	return cmd
}

func runRehearseRun(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "human" && format != "json" {
		return exitcode.InvalidArgsErrorf("unknown --format %q: expected human or json", format)
	}

	// The working directory matters only for the no-argument default
	// discovery; explicit paths never require a SpecScore repo.
	cwd := ""
	if len(args) == 0 {
		var err error
		cwd, err = osGetwdFn()
		if err != nil {
			return exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
	}

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

	if failed := runner.CountFailed(reports); failed > 0 {
		return exitcode.ConflictErrorf("%d of %d scenario(s) failed", failed, len(reports))
	}
	return nil
}
