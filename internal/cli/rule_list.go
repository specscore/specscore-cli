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
	// ScopeError is the parse failure when this rule's Scope cell is corrupt.
	// The rule is still listed: a rule whose scope cannot be read might bind
	// what you are about to touch, and the safe direction is to show it.
	ScopeError string `json:"scope_error" yaml:"scope_error"`
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
compose with AND semantics. An unrecognized --status, --enforcement or --scope
value exits 2 naming it, rather than silently matching nothing.

Scope matching is deliberately generous for paths and strict for repositories:

  fleet            matches everything
  path:<glob>      doublestar, matched against the path and every trailing
                   suffix of it, so path:cli/** also matches vendor/x/cli/y.go
  product:<name>   <name> appears as a whole path segment
  repo:<owner>/<n> <owner> and <n> appear as CONSECUTIVE segments; a bare <n>
                   never matches, so a rule scoped to a repo called docs or api
                   does not bind every docs/ directory in the fleet

A rule whose Scope cell does not parse is LISTED anyway, flagged scope_error,
and reported on stderr with exit 1 — a rule that might bind must not vanish
from the answer just because its scope is corrupt.

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
		// Case-folded like --status and --enforcement, as the help promises.
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

	entries := []ruleListEntry{}
	var scopeFindings []string
	for _, row := range rows {
		if wantStatus != "" && !strings.EqualFold(strings.TrimSpace(row.Status), wantStatus) {
			continue
		}
		if wantEnforcement != "" && !strings.EqualFold(strings.TrimSpace(row.Enforcement), wantEnforcement) {
			continue
		}
		scopes, scopeErr := rule.ParseScopes(row.ScopeList())
		scopeError := ""
		if scopeErr != nil {
			// Fail toward binding. Dropping the row would answer "no rule
			// applies" to a question whose real answer is "one rule might, and
			// its scope needs a human" — and `rule list` is run standalone at
			// the start of an agent stream, with no lint pass to catch it.
			scopeError = scopeErr.Error()
			scopeFindings = append(scopeFindings, fmt.Sprintf(
				"%s: Scope %q does not parse: %v", row.Slug, ruleCellText(row.Scope), scopeErr))
		}
		if wantScope != "" && scopeErr == nil && !scopeListContains(scopes, wantScope) {
			continue
		}
		if strings.TrimSpace(appliesTo) != "" && scopeErr == nil && !rule.ScopesMatch(scopes, appliesTo) {
			continue
		}
		entries = append(entries, ruleListEntry{
			Slug:        row.Slug,
			Statement:   structuredValue(row.Statement),
			Scope:       orEmptySlice(row.ScopeList()),
			Status:      structuredValue(row.Status),
			Enforcement: structuredValue(row.Enforcement),
			Control:     structuredValue(row.Control),
			Sources:     orEmptySlice(row.SourceList()),
			Detailed:    row.Detailed(),
			ScopeError:  scopeError,
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
			scope := dashIfEmpty(strings.Join(e.Scope, ","))
			if e.ScopeError != "" {
				scope = "!" + scope
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				e.Slug, dashIfEmpty(e.Status), scope, dashIfEmpty(e.Enforcement), e.Statement)
		}
	}

	if len(scopeFindings) > 0 {
		for _, finding := range scopeFindings {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", finding)
		}
		return exitcode.New(exitcode.Conflict, fmt.Sprintf(
			"%d rule(s) have an unparseable Scope and were listed anyway; run `specscore rule lint` for the R-006 findings",
			len(scopeFindings)))
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
