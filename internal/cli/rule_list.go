package cli

import (
	"fmt"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// ruleListEntry is the structured representation emitted in yaml/json format.
// It is exactly the index row: `rule list` never opens a detail document, which
// is what lets a tool run it at the start of every agent stream.
type ruleListEntry struct {
	Slug        string   `json:"slug" yaml:"slug"`
	Statement   string   `json:"statement" yaml:"statement"`
	Scope       []string `json:"scope" yaml:"scope"`
	Status      string   `json:"status" yaml:"status"`
	Enforcement string   `json:"enforcement" yaml:"enforcement"`
	Control     string   `json:"control" yaml:"control"`
	Sources     []string `json:"sources" yaml:"sources"`
	Detailed    bool     `json:"detailed" yaml:"detailed"`
}

func ruleListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List rules from the index, optionally filtered by scope, status, enforcement, or an affected path",
		Long: `Lists rules sorted by slug, reading only spec/rules/README.md — never a
detail document. That is deliberate: the listing has to stay cheap enough for a
tool to run it at the start of every agent stream and print the rules that
apply.

--applies-to <path> is the headline query: "which rules apply to what I am
about to touch?". It resolves each row's Scope against the given path —
` + "`fleet`" + ` always matches, ` + "`path:<glob>`" + ` is matched with doublestar (including
against every trailing suffix of the path, so a repo-relative pattern still
matches an absolute path), and ` + "`product:`" + ` / ` + "`repo:`" + ` match when their identifier
appears as a whole path segment. When you already know the repository or
product for certain, filter with --scope instead of inferring it from a path.

--scope, --status and --enforcement are exact, case-insensitive filters and
compose with AND semantics. An unrecognized --status or --enforcement value
exits 2 naming it, rather than silently matching nothing.

Output is empty (exit 0) when no rules match.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleList,
	}
	cmd.Flags().String("scope", "", "exact scope filter, e.g. fleet, product:sneat, repo:owner/name, path:**/*.go")
	cmd.Flags().String("status", "", "filter by status: Draft, Active, Superseded (case-insensitive)")
	cmd.Flags().String("enforcement", "", "filter by tier: Stated, Enforced, Automated (case-insensitive)")
	cmd.Flags().String("applies-to", "", "list only rules whose scope covers this path")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runRuleList(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	scopeFilter, _ := cmd.Flags().GetString("scope")
	statusFilter, _ := cmd.Flags().GetString("status")
	enforcementFilter, _ := cmd.Flags().GetString("enforcement")
	appliesTo, _ := cmd.Flags().GetString("applies-to")
	projectFlag, _ := cmd.Flags().GetString("project")

	var wantStatus string
	if strings.TrimSpace(statusFilter) != "" {
		canonical, ok := rule.ParseStatus(statusFilter)
		if !ok {
			return exitcode.InvalidArgsErrorf(
				"unrecognized --status value %q; legal values: %s", statusFilter, rule.StatusList())
		}
		wantStatus = canonical
	}
	var wantEnforcement string
	if strings.TrimSpace(enforcementFilter) != "" {
		canonical, ok := rule.ParseEnforcement(enforcementFilter)
		if !ok {
			return exitcode.InvalidArgsErrorf(
				"unrecognized --enforcement value %q; legal values: %s", enforcementFilter, rule.EnforcementList())
		}
		wantEnforcement = canonical
	}
	var wantScope string
	if strings.TrimSpace(scopeFilter) != "" {
		parsed, err := rule.ParseScope(scopeFilter)
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

	entries := []ruleListEntry{}
	for _, row := range rows {
		if wantStatus != "" && !strings.EqualFold(strings.TrimSpace(row.Status), wantStatus) {
			continue
		}
		if wantEnforcement != "" && !strings.EqualFold(strings.TrimSpace(row.Enforcement), wantEnforcement) {
			continue
		}
		scopes, scopeErr := rule.ParseScopes(row.ScopeList())
		if wantScope != "" {
			if scopeErr != nil || !scopeListContains(scopes, wantScope) {
				continue
			}
		}
		if strings.TrimSpace(appliesTo) != "" {
			if scopeErr != nil || !rule.ScopesMatch(scopes, appliesTo) {
				continue
			}
		}
		entries = append(entries, ruleListEntry{
			Slug:        row.Slug,
			Statement:   ruleCellText(row.Statement),
			Scope:       orEmptySlice(row.ScopeList()),
			Status:      strings.TrimSpace(row.Status),
			Enforcement: strings.TrimSpace(row.Enforcement),
			Control:     ruleCellText(row.Control),
			Sources:     orEmptySlice(row.SourceList()),
			Detailed:    row.Detailed(),
		})
	}

	w := cmd.OutOrStdout()
	switch format {
	case "json":
		return newJSONEnc(w).Encode(entries)
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(entries); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	default:
		for _, e := range entries {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				e.Slug, dashIfEmpty(e.Status), dashIfEmpty(strings.Join(e.Scope, ",")),
				dashIfEmpty(e.Enforcement), e.Statement)
		}
	}
	return nil
}

// readRuleRows reads the index rows, treating an absent index as an empty rule
// set rather than an error: every read verb must work in a repository that has
// recorded no rule yet.
func readRuleRows(rulesDir string) ([]rule.Row, error) {
	report, err := ruleReadIndexFn(rule.IndexPath(rulesDir))
	if err != nil {
		if osIsNotExistCLI(err) {
			return nil, nil
		}
		return nil, exitcode.UnexpectedErrorf("reading the rules index: %v", err)
	}
	return report.Rows, nil
}

func scopeListContains(scopes []rule.Scope, want string) bool {
	for _, s := range scopes {
		if s.String() == want {
			return true
		}
	}
	return false
}

// ruleCellText renders a table cell as plain text: the escape a Markdown table
// needs is a storage detail, not something a JSON consumer should see.
func ruleCellText(cell string) string {
	return strings.ReplaceAll(strings.TrimSpace(cell), `\|`, "|")
}

// dashIfEmpty renders the em-dash sentinel for an empty column so a text row
// never collapses two tab stops into one.
func dashIfEmpty(v string) string {
	if strings.TrimSpace(v) == "" {
		return rule.Sentinel
	}
	return v
}
