package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// ruleRenderCommand prints the progressive-discovery trigger index: one line
// per Active or Draft rule, naming the situation that identifies its known
// problem rather than its whole Statement — the same shape an AI agent skills
// listing uses, so a reader (or a fresh agent) scans triggers first and reads
// the full rule only once one matches.
func ruleRenderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Print a progressive-discovery trigger index of Active and Draft rules",
		Long: `Prints one line per Active or Draft rule as:

  - <trigger> → rule:<slug>

<trigger> is the situation that identifies the rule's known problem, resolved
in this order:

  1. the detail document's own **Trigger:** header line;
  2. failing that, the first line of its ` + "`## Instructions`" + ` section, when
     written ` + "`Trigger: <text>`" + ` — the shape sneat-co/backstage's rule tree
     already uses;
  3. failing that, the row's Statement, cut at its first comma, semicolon, or
     colon.

An inline rule has no detail document, so it always renders from its
Statement. Superseded rules are never printed: they are not a live trigger.

Output is sorted by Section then trigger (a plain trigger sort when no rule in
the result carries a **Section:**), and is stable across runs.

--format md groups the listing under a ` + "`## <Section>`" + ` heading per distinct
**Section:** value, with every ungrouped rule under ` + "`## Other`" + `. The default
text format never prints headings.

--section <name> restricts the listing to rules whose detail document names
that Section. --scope is the same exact, case-insensitive filter ` + "`rule list --scope`" + `
applies.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleRender,
	}
	cmd.Flags().String("scope", "", "exact scope filter, e.g. fleet, product:sneat, repo:owner/name, path:**/*.go")
	cmd.Flags().String("section", "", "restrict output to rules whose detail document names this Section")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, md")
	return cmd
}

func validateRuleRenderFormat(format string) error {
	if format != "text" && format != "md" {
		return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, md)", format)
	}
	return nil
}

func runRuleRender(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateRuleRenderFormat(format); err != nil {
		return err
	}
	scopeFilter, _ := cmd.Flags().GetString("scope")
	sectionFilter, _ := cmd.Flags().GetString("section")
	projectFlag, _ := cmd.Flags().GetString("project")

	var wantScope string
	if strings.TrimSpace(scopeFilter) != "" {
		parsed, err := rule.ParseScope(strings.ToLower(strings.TrimSpace(scopeFilter)))
		if err != nil {
			return exitcode.InvalidArgsErrorf("invalid --scope: %v", err)
		}
		wantScope = parsed.String()
	}

	rulesDir, err := resolveRulesDir(projectFlag)
	if err != nil {
		return err
	}
	rows, err := readRuleRows(rulesDir)
	if err != nil {
		return err
	}

	renderable := make([]rule.Row, 0, len(rows))
	for _, row := range rows {
		status := strings.TrimSpace(row.Status)
		if status != "Active" && status != "Draft" {
			continue
		}
		if wantScope != "" {
			scopes, scopeErr := rule.ParseScopes(row.ScopeList())
			if scopeErr != nil || !scopeListContains(scopes, wantScope) {
				continue
			}
		}
		renderable = append(renderable, row)
	}

	details, err := ruleDetailsBySlugFn(rulesDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading rule detail documents: %v", err)
	}

	entries := rule.BuildRenderEntries(renderable, details)
	if strings.TrimSpace(sectionFilter) != "" {
		kept := entries[:0:0]
		for _, e := range entries {
			if e.Section == sectionFilter {
				kept = append(kept, e)
			}
		}
		entries = kept
	}

	w := cmd.OutOrStdout()
	if format == "md" {
		writeRuleRenderMD(w, entries)
	} else {
		writeRuleRenderText(w, entries)
	}
	return nil
}

func writeRuleRenderText(w io.Writer, entries []rule.RenderEntry) {
	for _, e := range entries {
		_, _ = fmt.Fprintln(w, e.RenderLine())
	}
}

// writeRuleRenderMD groups entries carrying a Section under a `## <Section>`
// heading each, and every entry with none under a trailing `## Other`.
// entries is already sorted (Section, Trigger, Slug), so same-section entries
// are already adjacent; this only has to notice the boundaries.
func writeRuleRenderMD(w io.Writer, entries []rule.RenderEntry) {
	var other []rule.RenderEntry
	currentSection := ""
	open := false
	for _, e := range entries {
		if e.Section == "" {
			other = append(other, e)
			continue
		}
		if !open || e.Section != currentSection {
			if open {
				_, _ = fmt.Fprintln(w)
			}
			_, _ = fmt.Fprintf(w, "## %s\n\n", e.Section)
			currentSection = e.Section
			open = true
		}
		_, _ = fmt.Fprintln(w, e.RenderLine())
	}
	if len(other) > 0 {
		if open {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintln(w, "## Other")
		_, _ = fmt.Fprintln(w)
		for _, e := range other {
			_, _ = fmt.Fprintln(w, e.RenderLine())
		}
	}
}
