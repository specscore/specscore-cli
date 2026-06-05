package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/decision"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

func decisionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Decision management — scaffold Decision artifacts",
	}
	cmd.AddCommand(decisionNewCommand())
	return cmd
}

func decisionNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Decision artifact",
		Long: `Creates a lint-clean Decision skeleton at spec/decisions/<next-number>-<slug>.md.

The sequence number is auto-assigned (highest existing + 1, including archived/).
Each required section is emitted with an HTML-comment prompt. Supply content
via flags (--title, --owner, --source-idea, --supersedes, --tags). The generated
file is always lint-clean — running ` + "`specscore spec lint`" + ` immediately
afterwards passes.`,
		Args: cobra.ExactArgs(1),
		RunE: runDecisionNew,
	}
	cmd.Flags().String("title", "", "Decision title (defaults to title-cased slug)")
	cmd.Flags().String("owner", "", "author identifier (defaults to $USER)")
	cmd.Flags().String("source-idea", "", "Source Idea slug")
	cmd.Flags().String("supersedes", "", "Decision ID this one replaces")
	cmd.Flags().String("tags", "", "comma-separated tags")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool("force", false, "overwrite an existing decision file at that slug")
	return cmd
}

func runDecisionNew(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := decision.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	title, _ := cmd.Flags().GetString("title")
	owner, _ := cmd.Flags().GetString("owner")
	sourceIdea, _ := cmd.Flags().GetString("source-idea")
	supersedes, _ := cmd.Flags().GetString("supersedes")
	tags, _ := cmd.Flags().GetString("tags")
	projectFlag, _ := cmd.Flags().GetString("project")
	force, _ := cmd.Flags().GetBool("force")

	if owner == "" {
		if u := os.Getenv("USER"); u != "" {
			owner = u
		}
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	specSub := filepath.Join(specRoot, "spec")

	nextNum, err := decisionNextNumberFn(specSub)
	if err != nil {
		return exitcode.UnexpectedErrorf("determining next number: %v", err)
	}

	filename := fmt.Sprintf("%04d-%s.md", nextNum, slug)
	decisionsDir := filepath.Join(specSub, "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating %s: %v", decisionsDir, err)
	}

	target := filepath.Join(decisionsDir, filename)

	if _, err := os.Stat(target); err == nil && !force {
		return exitcode.ConflictErrorf("decision already exists: %s (pass --force to overwrite)", target)
	}

	opts := decision.ScaffoldOptions{
		Slug:       slug,
		Title:      title,
		Owner:      owner,
		Tags:       tags,
		SourceIdea: sourceIdea,
		Supersedes: supersedes,
	}

	// cli-template-runtime-fetch: a bare decision (no tags / source-idea /
	// supersedes) pulls the published template; otherwise the embedded
	// scaffolder fills those fields.
	bare := tags == "" && sourceIdea == "" && supersedes == ""
	body, err := bareOrEmbedded(bare, "decision", map[string]string{
		"<Decision Name>": templateTitleOrSlug(title, slug),
		"YYYY-MM-DD":      templateTodayUTC(),
		"<your-handle>":   templateOwnerOrUnknown(owner),
	}, cmd.ErrOrStderr(), func() ([]byte, error) {
		return decisionScaffoldFn(opts)
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding decision: %v", err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
	}

	// Run lint in --fix mode to update index, then re-run to verify.
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		_ = os.Remove(target)
		return exitcode.UnexpectedErrorf("running lint fix: %v", err)
	}
	violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
	if err != nil {
		return exitcode.UnexpectedErrorf("running lint: %v", err)
	}
	relTarget, _ := filepath.Rel(specSub, target)
	var own []lint.Violation
	for _, v := range violations {
		if v.Severity == "error" && (v.File == relTarget || strings.HasPrefix(v.File, "decisions/")) {
			own = append(own, v)
		}
	}
	if len(own) > 0 {
		var sb strings.Builder
		sb.WriteString("generated decision failed lint:\n")
		for _, v := range own {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return exitcode.UnexpectedError(sb.String())
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}
