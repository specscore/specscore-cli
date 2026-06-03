package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	// osExit is a testable indirection for os.Exit. Tests replace it with a
	// stub to verify exit codes without killing the test process.
	osExit = os.Exit
)

// Run executes the specscore CLI with the given arguments.
func Run(args []string) error {
	rootCmd := &cobra.Command{
		Use:           "specscore",
		Short:         "SpecScore CLI — validate and query specification repositories",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	// `--version` prints just the bare semver (e.g. `0.11.0`) for scripting.
	// Use the `version` subcommand for the full human-readable line with commit and date.
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.SetErr(os.Stderr)

	rootCmd.AddCommand(
		versionCommand(),
		agentCommand(),
		codeCommand(),
		entityCommand(),
		featureCommand(),
		propertyCommand(),
		rulesCommand(),
		specCommand(),
		taskCommand(),
		ideaCommand(),
		decisionCommand(),
		issueCommand(),
		proposalCommand(),
		initCommand(),
		configCommand(),
		consiliumCommand(),
		eventCommand(),
		publicationCommand(),
		telemetryCommand(),
		debugCommand(),
		selfUpdateCommand(),
	)

	// Attach telemetry persistent-flag + PersistentPreRun. Emission happens
	// after Execute returns so the actual exit code is captured.
	attachTelemetry(rootCmd)

	if len(args) > 1 {
		rootCmd.SetArgs(args[1:])
	}
	return executeWithPanicRecovery(rootCmd)
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the specscore version",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = cmd.OutOrStdout().Write([]byte("specscore " + version + " (" + commit + ") " + date + "\n"))
		},
	}
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
