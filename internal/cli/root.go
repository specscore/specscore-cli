package cli

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
	"github.com/strongo/buildinfo"
	"github.com/strongo/buildinfo/fangcmd"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// buildInfo is this process's resolved build identity (version, commit,
// build date), resolved once at package-init time by
// github.com/strongo/buildinfo. It replaces the old ldflags-stamped
// version/commit/date package vars: every command that previously read
// those directly now reads buildInfo.Version — the only field anything
// besides the `version` subcommand and `--version`/`-v` flag ever needed.
// Those two surfaces are wired directly from buildInfo by fangcmd.Wire in
// newRootCommand(), so they cannot disagree.
var buildInfo = buildinfo.Get("specscore")

var (
	// osExit is a testable indirection for os.Exit. Tests replace it with a
	// stub to verify exit codes without killing the test process.
	osExit = os.Exit
)

// Run executes the specscore CLI with the given arguments.
func Run(args []string) error {
	rootCmd, fangOpts := newRootCommand()

	if len(args) > 1 {
		rootCmd.SetArgs(args[1:])
	}
	return executeWithPanicRecovery(rootCmd, fangOpts...)
}

// newRootCommand builds the root cobra command and returns the
// []fang.Option that fangcmd.Wire produced for it (e.g. the resolved
// --version/-v value). Callers MUST pass those options into fang.Execute
// alongside the returned command so the `version` subcommand it wires in
// and the --version/-v flag stay in agreement.
func newRootCommand() (*cobra.Command, []fang.Option) {
	rootCmd := &cobra.Command{
		Use:           "specscore",
		Short:         "SpecScore CLI — validate and query specification repositories",
		Version:       buildInfo.Short(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	rootCmd.SetErr(os.Stderr)

	rootCmd.AddCommand(
		agentCommand(),
		codeCommand(),
		entityCommand(),
		featureCommand(),
		graphCommand(),
		installCommand(),
		lessonCommand(),
		planCommand(),
		propertyCommand(),
		rehearseCommand(),
		ruleCommand(),
		rulesCommand(),
		rootMigrateCommand(),
		skillsCommand(),
		specCommand(),
		studioCommand(),
		taskCommand(),
		ideaCommand(),
		sidekickCommand(),
		decisionCommand(),
		issueCommand(),
		proposalCommand(),
		initCommand(),
		configCommand(),
		consiliumCommand(),
		eventCommand(),
		mergeDriverCommand(),
		publicationCommand(),
		telemetryCommand(),
		lifecycleRecoveryCommand(),
		debugCommand(),
		selfUpdateCommand(),
		uninstallCommand(),
		upgradeCommand(),
	)

	// `version` subcommand + `--version`/`-v` flag are both wired from
	// buildInfo here, so the two surfaces cannot disagree. Wire also sets
	// rootCmd's version template to print the bare version with no
	// decoration, which is what the fang.Option below relies on.
	fangOpts := fangcmd.Wire(rootCmd, buildInfo)

	// Attach telemetry persistent-flag + PersistentPreRun. Emission happens
	// after Execute returns so the actual exit code is captured.
	attachTelemetry(rootCmd)

	return rootCmd, fangOpts
}

func rootMigrateCommand() *cobra.Command {
	cmd := specMigrateCommand()
	cmd.Short = "Backfill artifact-frontmatter-convention frontmatter across the spec tree"
	cmd.Long = "Equivalent to `specscore spec migrate`.\n\n" + cmd.Long
	return cmd
}

// mapUnsupportedCommand maps cobra's "unknown command" error to the dedicated
// UnsupportedCommand exit code (8). This lets callers distinguish an outdated
// specscore that predates a required subcommand from a generic failure (1),
// while the shell still reports a wholly-absent binary as 127. Scoped to
// unknown subcommands only — unknown flags keep their existing semantics — and
// it never clobbers an error that already carries an exit code.
func mapUnsupportedCommand(err error) error {
	if err == nil {
		return nil
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if errors.As(err, &ec) {
		return err
	}
	if strings.HasPrefix(err.Error(), `unknown command "`) {
		return exitcode.UnsupportedCommandError(err.Error())
	}
	return err
}

// cobraUsageError matches, exactly, the errors Cobra itself returns when a
// command's positional-argument validator (ExactArgs, MinimumNArgs,
// MaximumNArgs, RangeArgs) rejects the argument count, or when a flag marked
// with MarkFlagRequired is absent. Those errors are plain fmt.Errorf values
// with no ExitCode(), so without mapping they fall through to Fatal's generic
// exit 1. TestMapInvalidArgs_PinsCobraMessages drives Cobra's real validators,
// so a Cobra upgrade that rewords one of them fails the test instead of
// silently reverting the affected verbs to exit 1.
var cobraUsageError = regexp.MustCompile(`^(?:` +
	`accepts \d+ arg\(s\), received \d+` +
	`|accepts at most \d+ arg\(s\), received \d+` +
	`|accepts between \d+ and \d+ arg\(s\), received \d+` +
	`|requires at least \d+ arg\(s\), only received \d+` +
	`|required flag\(s\) ".+" not set` +
	`)$`)

// mapInvalidArgs maps Cobra's positional-argument-count and required-flag
// errors to the InvalidArgs exit code (2), as the shared exit-code contract
// requires (cli#req:usage-errors-exit-2). It is applied once, where the root
// command executes, so every verb gets it without per-command wiring. Like
// mapUnsupportedCommand it never clobbers an error that already carries an
// exit code; Cobra's "unknown command" error (also returned by NoArgs) stays
// with mapUnsupportedCommand.
func mapInvalidArgs(err error) error {
	if err == nil {
		return nil
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if errors.As(err, &ec) {
		return err
	}
	if cobraUsageError.MatchString(err.Error()) {
		return exitcode.Wrap(exitcode.InvalidArgs, err.Error(), err)
	}
	return err
}

// Fatal prints the error and exits with the appropriate code.
func Fatal(err error) {
	if err == nil {
		return
	}
	_, _ = os.Stderr.WriteString(err.Error() + "\n")

	type exitCoder interface {
		ExitCode() int
	}
	var ec exitCoder
	if errors.As(err, &ec) {
		osExit(ec.ExitCode())
		return
	}
	osExit(1)
}
