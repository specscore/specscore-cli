package cli

import (
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

func ruleUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <slug>",
		Short: "Edit a rule's index row and, when it has one, mirror the change into its detail document",
		Long: `Rewrites the named fields of the rule's row in spec/rules/README.md — the
source of truth — and, when the rule has a detail document, mirrors every
changed row field into that document's header so the two cannot drift apart.

Document-only edits (--why, --exceptions, --instructions, --supersedes,
--superseded-by) require a detail document: on an inline rule they exit 4
naming ` + "`specscore rule expand`" + `. Everything else works on both forms.

Editing the document is in place: only the named bold fields change, and every
other byte — comments, worked examples, hand-written Open Questions — is
preserved. Regenerating from a template would silently discard an author's
examples and make this verb unsafe to run on a reviewed rule.

--scope replaces the whole scope list and is repeatable. --add-source and
--remove-source edit the sources list incrementally: adding a source already
present, or removing one that is absent, exits 2 rather than silently
succeeding.

Changing --enforcement to Enforced or Automated requires the rule to end up
with a non-empty control, so pass --control in the same invocation unless the
rule already names one.

At least one edit flag is required; ` + "`rule update`" + ` with no edits exits 2.

Examples:

  specscore rule update never-mock-backends --status Active
  specscore rule update gofmt-first --enforcement Enforced --control "wb pre-commit hook"
  specscore rule update gofmt-first --add-source lesson:kinder-fake-hides-bug`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleUpdate,
	}
	cmd.Flags().String("statement", "", "replace the normative sentence")
	cmd.Flags().StringArray("scope", nil, "replace the scope list; repeatable")
	cmd.Flags().String("status", "", "set status: Draft, Active, or Superseded")
	cmd.Flags().String("enforcement", "", "set the tier: Stated, Enforced, or Automated")
	cmd.Flags().String("control", "", "set the enforcing control (\"\" clears it to the — sentinel)")
	cmd.Flags().StringArray("add-source", nil, "add a source reference; repeatable")
	cmd.Flags().StringArray("remove-source", nil, "remove a source reference; repeatable")

	cmd.Flags().String("title", "", "replace the detail document's title")
	cmd.Flags().String("why", "", "replace the reason and use case (detail document only)")
	cmd.Flags().String("exceptions", "", "replace the escape hatch (detail document only)")
	cmd.Flags().String("supersedes", "", "set the rule this one supersedes (detail document only)")
	cmd.Flags().String("superseded-by", "", "set the rule that supersedes this one (detail document only)")

	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

// ruleUpdateRowFlags edit the index row (and mirror into the document).
var ruleUpdateRowFlags = []string{"statement", "scope", "status", "enforcement", "control", "add-source", "remove-source"}

// ruleUpdateDetailFlags edit only the detail document.
var ruleUpdateDetailFlags = []string{"title", "why", "exceptions", "supersedes", "superseded-by"}

func runRuleUpdate(cmd *cobra.Command, args []string) error {
	slug, err := singleRuleSlugArg(cmd, args, "update")
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}

	changedRow, changedDetail := false, false
	for _, name := range ruleUpdateRowFlags {
		if cmd.Flags().Changed(name) {
			changedRow = true
		}
	}
	for _, name := range ruleUpdateDetailFlags {
		if cmd.Flags().Changed(name) {
			changedDetail = true
		}
	}
	if !changedRow && !changedDetail {
		return exitcode.InvalidArgsErrorf("no edit requested: pass at least one of --%s",
			strings.Join(append(append([]string{}, ruleUpdateRowFlags...), ruleUpdateDetailFlags...), ", --"))
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	rulesDir := rule.RulesDir(root)
	row, err := rule.ResolveRow(rulesDir, slug)
	if err != nil {
		return err
	}
	if changedDetail && !row.Detailed() {
		return exitcode.InvalidStateErrorf(
			"rule %q is inline, so it has nowhere to record %s; run `specscore rule expand %s` first",
			slug, strings.Join(changedRuleFlagNames(cmd, ruleUpdateDetailFlags), ", "), slug)
	}

	updated, err := applyRuleRowEdits(cmd, row)
	if err != nil {
		return err
	}
	if err := ruleUpsertIndexRowFn(rulesDir, updated); err != nil {
		return exitcode.UnexpectedErrorf("writing the rule's index row: %v", err)
	}

	if row.Detailed() {
		if err := updateRuleDetail(cmd, rulesDir, slug, updated); err != nil {
			return err
		}
	}

	return writeRuleResult(cmd, format, ruleWriteResult{
		Slug: slug, Form: ruleFormName(row.Detailed()),
		Path:   ruleResultPath(rulesDir, row.Detailed(), slug),
		Status: strings.TrimSpace(updated.Status), Action: "updated", Detailed: row.Detailed(),
	})
}

func changedRuleFlagNames(cmd *cobra.Command, names []string) []string {
	var out []string
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			out = append(out, "--"+name)
		}
	}
	return out
}

// applyRuleRowEdits validates every requested row edit against the closed
// vocabularies before any byte is written, so a rejected update leaves both the
// index and the document untouched.
func applyRuleRowEdits(cmd *cobra.Command, row rule.Row) (rule.Row, error) {
	flags := cmd.Flags()
	updated := row

	if flags.Changed("statement") {
		v, _ := flags.GetString("statement")
		if strings.TrimSpace(v) == "" {
			return row, exitcode.InvalidArgsError("--statement must not be empty; a rule with no statement is not a rule")
		}
		updated.Statement = escapeRuleCell(v)
	}
	if flags.Changed("scope") {
		values, _ := flags.GetStringArray("scope")
		if len(values) == 0 {
			return row, exitcode.InvalidArgsError("--scope must name at least one scope")
		}
		if _, err := rule.ParseScopes(values); err != nil {
			return row, exitcode.InvalidArgsErrorf("%v", err)
		}
		updated.Scope = escapeRuleCell(strings.Join(values, ", "))
	}
	if flags.Changed("status") {
		v, _ := flags.GetString("status")
		canonical, ok := rule.ParseStatus(v)
		if !ok {
			return row, exitcode.InvalidArgsErrorf(
				"unrecognized --status value %q; legal values: %s", v, rule.StatusList())
		}
		updated.Status = canonical
	}
	if flags.Changed("add-source") || flags.Changed("remove-source") {
		add, _ := flags.GetStringArray("add-source")
		remove, _ := flags.GetStringArray("remove-source")
		merged, err := rule.MergeSources(row.SourceList(), add, remove)
		if err != nil {
			return row, exitcode.InvalidArgsErrorf("%v", err)
		}
		updated.Sources = escapeRuleCell(strings.Join(merged, ", "))
	}

	// Enforcement and Control are validated together: the tier decides whether
	// a control is mandatory, and the resulting pair must be legal even when
	// only one of the two flags was passed.
	tier := strings.TrimSpace(row.Enforcement)
	if flags.Changed("enforcement") {
		v, _ := flags.GetString("enforcement")
		canonical, ok := rule.ParseEnforcement(v)
		if !ok {
			return row, exitcode.InvalidArgsErrorf(
				"unrecognized --enforcement value %q; legal values: %s", v, rule.EnforcementList())
		}
		tier = canonical
		updated.Enforcement = canonical
	}
	control := ruleCellText(row.Control)
	if flags.Changed("control") {
		v, _ := flags.GetString("control")
		control = collapseFlagValue(v)
		updated.Control = escapeRuleCell(control)
	}
	if rule.RequiresControl(tier) && !isRealRuleValue(control) {
		return row, exitcode.InvalidArgsErrorf(
			"enforcement %s requires --control naming the mechanism that refuses", tier)
	}
	return updated, nil
}

// updateRuleDetail mirrors the row's fields into the detail document and
// applies the document-only edits, in one in-place rewrite.
func updateRuleDetail(cmd *cobra.Command, rulesDir, slug string, row rule.Row) error {
	path := rule.DetailPath(rulesDir, slug)
	content, err := osReadFileCLI(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading %s: %v", path, err)
	}
	mirrored := rule.MirroredValuesOf(row)
	edits := make([]rule.FieldEdit, 0, len(rule.MirroredFields)+len(ruleUpdateDetailFlags))
	for _, name := range rule.MirroredFields {
		edits = append(edits, rule.FieldEdit{Name: name, Value: mirrored[name]})
	}

	flags := cmd.Flags()
	if flags.Changed("why") {
		v, _ := flags.GetString("why")
		if strings.TrimSpace(v) == "" {
			return exitcode.InvalidArgsError("--why must not be empty")
		}
		edits = append(edits, rule.FieldEdit{Name: "Why", Value: collapseFlagValue(v)})
	}
	if flags.Changed("exceptions") {
		v, _ := flags.GetString("exceptions")
		if strings.TrimSpace(v) == "" {
			v = "none"
		}
		edits = append(edits, rule.FieldEdit{Name: "Exceptions", Value: collapseFlagValue(v)})
	}
	if flags.Changed("supersedes") {
		v, _ := flags.GetString("supersedes")
		edits = append(edits, rule.FieldEdit{Name: "Supersedes", Value: sentinelOrValue(v)})
	}
	if flags.Changed("superseded-by") {
		v, _ := flags.GetString("superseded-by")
		edits = append(edits, rule.FieldEdit{Name: "Superseded By", Value: sentinelOrValue(v)})
	}

	updated, err := ruleApplyFieldEditsFn(content, edits)
	if err != nil {
		return exitcode.UnexpectedErrorf("editing %s: %v", path, err)
	}
	if title, ok := ruleTitleEdit(cmd); ok {
		updated = rewriteRuleTitle(updated, title)
	}
	if err := ruleWriteFileAtomicFn(path, updated); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", path, err)
	}
	return nil
}

func ruleTitleEdit(cmd *cobra.Command) (string, bool) {
	if !cmd.Flags().Changed("title") {
		return "", false
	}
	v, _ := cmd.Flags().GetString("title")
	if v = collapseFlagValue(v); v == "" {
		return "", false
	}
	return v, true
}

// rewriteRuleTitle replaces the `# Rule: <title>` heading in place.
func rewriteRuleTitle(content []byte, title string) []byte {
	lines := strings.Split(string(content), "\n")
	for i, raw := range lines {
		if strings.HasPrefix(strings.TrimSpace(raw), "# Rule:") {
			lines[i] = "# Rule: " + title
			break
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// collapseFlagValue folds a multi-line flag value into one line, so a shell
// heredoc still writes a single field or table cell.
func collapseFlagValue(v string) string { return strings.Join(strings.Fields(v), " ") }

// escapeRuleCell normalizes a free-text flag value into a table cell.
func escapeRuleCell(v string) string {
	collapsed := collapseFlagValue(v)
	if collapsed == "" {
		return rule.Sentinel
	}
	return strings.ReplaceAll(collapsed, "|", `\|`)
}

func sentinelOrValue(v string) string {
	if strings.TrimSpace(v) == "" {
		return rule.Sentinel
	}
	return strings.TrimSpace(v)
}

func isRealRuleValue(v string) bool {
	t := strings.TrimSpace(v)
	return t != "" && t != rule.Sentinel && t != "-"
}
