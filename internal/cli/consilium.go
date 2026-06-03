package cli

import "github.com/spf13/cobra"

// consiliumCommand returns the "consilium" parent command group — the
// attachment point for the deterministic consilium engine's three verbs
// (`verdict`, `roster`, `config`), each owned by its own child Feature.
//
// This command carries no run behavior of its own: a bare
// `specscore consilium` prints help and exits per the shared exit-code
// contract (cobra's default parent-with-no-Run behavior → exit 0). The
// three planned subcommands are not registered here — they are wired in by
// their respective child Features — so the help references them via the
// Long description instead. See spec/features/cli/consilium/README.md
// (REQ:consilium-parent-command, AC:consilium-parent-prints-help).
func consiliumCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "consilium",
		Short: "Deterministic consilium engine — verdict, roster, and config verbs",
		Long: `The consilium command group drives the deterministic consilium engine
that turns an expert panel's votes into a reproducible verdict.

Subcommands:
  verdict   Read votes, roster, and gate config and emit a deterministic verdict.
  roster    Resolve and print the active roster (defaults + custom − exclude).
  config    Print the active gate-knob configuration the arbiter would apply.

Docs: https://specscore.md/consilium
`,
	}
}
