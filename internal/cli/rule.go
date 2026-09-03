package cli

// Features implemented: cli/rule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// ruleCommand returns the "rule" command group. With no Run/RunE, cobra prints
// help and exits 0 for `specscore rule`.
//
// Note the deliberate singular: `specscore rules` (plural) is the unrelated
// read-only catalog of *lint* rules. `specscore rule` (singular) is the CRUD
// surface for the Rule artifact kind.
func ruleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "Record and query normative rules — the transferable half of agent memory",
		Long: `A Rule is one normative sentence — MUST or NEVER, in plain words — with the
scope it binds, the reason it exists, the sources that produced it, and the
control that enforces it.

Rules exist because durable operating knowledge otherwise accumulates in
per-agent memory files, which do not transfer: a new session, a new machine or
a new runtime starts blind.

A rule has two forms and one identity:

  INLINE    one row in spec/rules/README.md and nothing else. Most rules are
            one sentence; a directory each would be ceremony that discourages
            recording them at all.
  DETAILED  the identical row, linked to spec/rules/<slug>/README.md, which
            carries the reason, worked compliant and violating examples, agent
            instructions, exceptions, and supersession.

The index row is the source of truth for every field it carries. A detail
document repeats those fields for readability and lint (R-011) requires them to
agree, with --fix rewriting the document from the row and never the reverse.

Rules cross-link two ways. ` + "`rule promote --from-lesson <slug>`" + ` turns a Lesson's
control into a rule and writes both halves of the ` + "`**Promotes To:**`" + ` / ` + "`lesson:`" + `
pair (R-008). A detail document's Instructions may name ` + "`skill:<name>`" + `, and a
skill's ` + "`## Rules`" + ` section may name ` + "`rule:<slug>`" + `; R-010 checks that pair in
both directions too.`,
	}
	cmd.AddCommand(
		ruleNewCommand(),
		ruleExpandCommand(),
		ruleListCommand(),
		ruleShowCommand(),
		ruleUpdateCommand(),
		ruleDeleteCommand(),
		rulePromoteCommand(),
		ruleLintCommand(),
	)
	return cmd
}

// resolveRulesDir resolves the rules directory from --project or CWD. An absent
// directory is not an error: the path is returned regardless, so every read
// verb works in a repository that has recorded no rule yet.
func resolveRulesDir(projectFlag string) (string, error) {
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return "", err
	}
	return rule.RulesDir(root), nil
}

// ruleNewCommand records a rule, inline by default.
func ruleNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Record a rule as one index row, or with --detailed a full document",
		Long: `Adds one row to spec/rules/README.md. That row IS the rule: an inline rule
has no directory and no second file.

Only the slug is required. An unflagged ` + "`rule new`" + ` records a lint-clean Draft row
with TODO text in its Statement, so writing a rule down under time pressure is
one command; ` + "`specscore rule update`" + ` sharpens it later.

--detailed additionally writes spec/rules/<slug>/README.md and links the row to
it. Any document-only flag (--why, --exceptions, --instructions, --compliant,
--violation, --supersedes, --skill) implies --detailed, because those fields
have nowhere else to live.

An Enforced or Automated rule MUST name --control; a rule that claims a tier no
mechanism backs is a Stated rule wearing a stronger label.

Examples:

  specscore rule new never-mock-backends \
    --statement "Never ship a mocked extension backend; ship real dalgo-backed routes or ship nothing." \
    --scope fleet --source lesson:never-mock-extension-backends

  specscore rule new gofmt-first --scope path:'**/*.go' --detailed \
    --statement "Always run gofmt on changed Go files before any build, lint or test command." \
    --enforcement Enforced --control "wb pre-commit hook profile go-standard" \
    --why "An unformatted file reddens CI on a cosmetic diff and hides the real failure."`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleNew,
	}
	addRuleAuthoringFlags(cmd)
	cmd.Flags().Bool("detailed", false, "also write spec/rules/<slug>/README.md and link the row to it")
	cmd.Flags().Bool("force", false, "overwrite an existing rule at that slug")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

// addRuleAuthoringFlags registers the flag set `new` and `promote` share, so
// the two verbs cannot drift apart on names or defaults.
func addRuleAuthoringFlags(cmd *cobra.Command) {
	cmd.Flags().String("title", "", "rule title (defaults to the title-cased slug); detail documents only")
	cmd.Flags().String("statement", "", "the one normative sentence (MUST / NEVER)")
	cmd.Flags().StringArray("scope", nil, "scope this rule binds; repeatable: fleet | product:<name> | repo:<owner/repo> | path:<glob> (default fleet)")
	cmd.Flags().StringArray("source", nil, "artifact that produced this rule; repeatable: lesson:<slug> | decision:<id> | idea:<slug> | <url>")
	cmd.Flags().String("enforcement", "", "enforcement tier: Stated, Enforced, or Automated (default Stated)")
	cmd.Flags().String("control", "", "the mechanism that refuses; required for Enforced and Automated")
	cmd.Flags().String("status", "", "initial status: Draft, Active, or Superseded (default Draft)")
	cmd.Flags().String("owner", "", "owner/author (defaults to $USER); detail documents only")
	cmd.Flags().String("date", "", "ISO-8601 date (defaults to today, UTC); detail documents only")

	cmd.Flags().String("why", "", "the reason plus one concrete use case; implies --detailed")
	cmd.Flags().String("exceptions", "", "the explicit escape hatch and who may use it (default \"none\"); implies --detailed")
	cmd.Flags().String("instructions", "", "what an agent should do to comply; implies --detailed")
	cmd.Flags().String("compliant", "", "a worked example that satisfies the rule; implies --detailed")
	cmd.Flags().String("violation", "", "a worked example that breaks it; implies --detailed")
	cmd.Flags().String("supersedes", "", "the rule this one replaces; implies --detailed")
	cmd.Flags().StringArray("skill", nil, "skill this rule binds, by name; repeatable; implies --detailed")
}

// ruleDetailFlags are the flags that only a detail document can hold, and so
// imply --detailed when any of them is set.
var ruleDetailFlags = []string{"why", "exceptions", "instructions", "compliant", "violation", "supersedes", "skill"}

// ruleWriteResult is the structured result every mutating rule verb emits.
type ruleWriteResult struct {
	Slug     string `json:"slug" yaml:"slug"`
	Form     string `json:"form" yaml:"form"`
	Path     string `json:"path" yaml:"path"`
	Status   string `json:"status" yaml:"status"`
	Action   string `json:"action" yaml:"action"`
	Detailed bool   `json:"detailed" yaml:"detailed"`
}

// ruleFormName renders the two forms for humans and scripts alike.
func ruleFormName(detailed bool) string {
	if detailed {
		return "detailed"
	}
	return "inline"
}

func runRuleNew(cmd *cobra.Command, args []string) error {
	slug, err := singleRuleSlugArg(cmd, args, "new")
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}

	detailed, _ := cmd.Flags().GetBool("detailed")
	for _, name := range ruleDetailFlags {
		if cmd.Flags().Changed(name) {
			detailed = true
			break
		}
	}

	opts, err := ruleOptionsFromFlags(cmd, slug)
	if err != nil {
		return err
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	rulesDir := rule.RulesDir(root)

	force, _ := cmd.Flags().GetBool("force")
	if err := ensureRuleAncestorIndexes(root); err != nil {
		return exitcode.UnexpectedErrorf("materializing ancestor indexes: %v", err)
	}
	if err := preflightRuleIndex(rulesDir, slug); err != nil {
		return err
	}
	if err := requireResolvableSources(filepath.Join(root, "spec"), opts.Sources); err != nil {
		return err
	}
	if err := requireSupersedableForm(slug, opts.Status, detailed); err != nil {
		return err
	}
	if !force {
		if _, err := rule.ResolveRow(rulesDir, slug); err == nil {
			return exitcode.ConflictErrorf("rule already exists: %s is already listed in %s (pass --force to overwrite)",
				slug, rule.IndexPath(rulesDir))
		}
	}

	if detailed {
		if err := writeRuleDetail(rulesDir, opts, force); err != nil {
			return err
		}
	}
	if err := ruleUpsertIndexRowFn(rulesDir, opts.Row(detailed)); err != nil {
		return exitcode.UnexpectedErrorf("writing the rule's index row: %v", err)
	}

	return writeRuleResult(cmd, format, ruleWriteResult{
		Slug: slug, Form: ruleFormName(detailed), Path: ruleResultPath(rulesDir, detailed, slug),
		Status: opts.Status, Action: "created", Detailed: detailed,
	}, root)
}

// ruleResultPath names the file a verb's caller should open: the detail
// document for a detailed rule, the index for an inline one.
func ruleResultPath(rulesDir string, detailed bool, slug string) string {
	if detailed {
		return rule.DetailPath(rulesDir, slug)
	}
	return rule.IndexPath(rulesDir)
}

// writeRuleDetail scaffolds and publishes the detail document.
func writeRuleDetail(rulesDir string, opts rule.Options, force bool) error {
	target := rule.DetailPath(rulesDir, opts.Slug)
	if _, statErr := osStatCLI(target); statErr == nil && !force {
		return exitcode.ConflictErrorf("rule detail document already exists: %s (pass --force to overwrite)", target)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return exitcode.UnexpectedErrorf("preflighting %s: %v", target, statErr)
	}
	body, err := ruleScaffoldDetailFn(opts)
	if err != nil {
		return exitcode.InvalidArgsErrorf("scaffolding the rule detail document: %v", err)
	}
	if err := osMkdirAllCLI(filepath.Dir(target), 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating rule directory: %v", err)
	}
	if err := ruleWriteFileAtomicFn(target, body); err != nil {
		return exitcode.UnexpectedErrorf("writing the rule detail document: %v", err)
	}
	return nil
}

// ruleOptionsFromFlags builds and validates the authoring options, so `new` and
// `promote` cannot disagree about defaults or validation.
func ruleOptionsFromFlags(cmd *cobra.Command, slug string) (rule.Options, error) {
	flags := cmd.Flags()
	get := func(name string) string { v, _ := flags.GetString(name); return v }
	getArr := func(name string) []string { v, _ := flags.GetStringArray(name); return v }

	owner := get("owner")
	if owner == "" {
		owner = osGetenvCLI("USER")
	}
	opts := rule.Options{
		Slug: slug, Title: get("title"), Owner: owner, Date: get("date"), Status: get("status"),
		Statement: get("statement"), Scopes: getArr("scope"), Sources: getArr("source"),
		Enforcement: get("enforcement"), Control: get("control"),
		Why: get("why"), Exceptions: get("exceptions"), Supersedes: get("supersedes"),
		Instructions: get("instructions"), Compliant: get("compliant"), Violation: get("violation"),
		Skills: getArr("skill"),
	}
	if err := opts.Normalize(); err != nil {
		return rule.Options{}, exitcode.InvalidArgsErrorf("%v", err)
	}
	return opts, nil
}

// preflightRuleIndex refuses a mutating verb while the index carries a
// row-like line that does not parse and is not one of the rows this verb is
// about to write.
//
// The library preserves such lines rather than dropping them, so nothing is
// lost either way. This guard exists for the second half of the problem: a
// caller who runs `rule new innocent` and sees exit 0 has no reason to look at
// the index, and a broken row that nobody looks at is a rule that has quietly
// stopped binding. Refusing puts it in front of the one person already editing
// the file.
func preflightRuleIndex(rulesDir string, addressed ...string) error {
	report, err := ruleReadIndexFn(rule.IndexPath(rulesDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return exitcode.UnexpectedErrorf("reading the rules index: %v", err)
	}
	blocking := report.MalformedExcept(addressed...)
	if len(blocking) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s has %d row(s) that do not parse; refusing to rewrite it until they are repaired",
		rule.IndexPath(rulesDir), len(blocking))
	for _, m := range blocking {
		subject := "row"
		if m.SlugHint != "" {
			subject = "rule " + m.SlugHint
		}
		fmt.Fprintf(&b, "\n  line %d: %s — %s", m.Line, subject, m.Reason)
	}
	b.WriteString("\nrun `specscore rule lint --fix` (it repairs an unescaped `|` in a Statement) or edit the row by hand, then retry")
	return exitcode.ConflictError(b.String())
}

// requireSupersedableForm refuses to park an INLINE rule at Superseded.
//
// Supersession lives in the detail document, so an inline row at Superseded has
// nowhere to name its successor — and `spec lint` used to call that tree clean,
// leaving a retired rule with no forwarding address and nothing to notice it.
// Refusing here is the cheap half of the fix; R-009 reports the trees that
// already contain one.
func requireSupersedableForm(slug, status string, detailed bool) error {
	if !strings.EqualFold(strings.TrimSpace(status), "Superseded") || detailed {
		return nil
	}
	return exitcode.InvalidStateErrorf(
		"rule %q is inline, so Superseded has nowhere to record **Superseded By:**; run `specscore rule expand %s` first, then set the successor",
		slug, slug)
}

// singleRuleSlugArg validates the shared "exactly one <slug>" positional
// contract every rule verb carries.
func singleRuleSlugArg(_ *cobra.Command, args []string, verb string) (string, error) {
	if len(args) == 0 {
		return "", exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return "", exitcode.InvalidArgsErrorf(
			"too many positional arguments: %s accepts exactly one <slug>, got %d", verb, len(args))
	}
	slug := args[0]
	if err := rule.ValidateSlug(slug); err != nil {
		return "", exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}
	return slug, nil
}

// ensureRuleAncestorIndexes materializes spec/README.md and spec/rules/README.md
// when they don't already exist. Existing files are left untouched.
func ensureRuleAncestorIndexes(root string) error {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		cfg = projectdef.SpecConfig{}
	}
	if err := writeMissingIndex(root, "spec/README.md", specReadmeContent(cfg)); err != nil {
		return fmt.Errorf("writing spec/README.md: %w", err)
	}
	return ruleEnsureIndexFn(rule.RulesDir(root))
}

// writeRuleResult renders a mutating verb's result in the requested format.
// Structured output carries a repo-relative path; text keeps the absolute one,
// because that is what a caller pipes to an editor.
func writeRuleResult(cmd *cobra.Command, format string, result ruleWriteResult, root string) error {
	w := cmd.OutOrStdout()
	switch format {
	case "json":
		result.Path = repoRelative(root, result.Path)
		return newJSONEnc(w).Encode(result)
	case "yaml":
		result.Path = repoRelative(root, result.Path)
		enc := newYAMLEnc(w)
		if err := enc.Encode(result); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	default:
		_, _ = fmt.Fprintf(w, "%s\n", result.Path)
		return nil
	}
}

// ruleExpandCommand promotes an inline rule to the detailed form.
func ruleExpandCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "expand <slug>",
		Short: "Give an inline rule a detail document, seeded from its index row",
		Long: `Writes spec/rules/<slug>/README.md for a rule that so far exists only as an
index row, seeding its mirrored header from that row, and links the row to it.

Everything the row already says is carried over verbatim: the row stays the
source of truth, and the new document starts in agreement with it. Only the
fields a document alone owns — Why, Instructions, Examples, Exceptions,
Supersedes — need writing, and each can be supplied here or edited afterwards.

Expanding a rule that already has a document is a conflict, not an overwrite.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleExpand,
	}
	cmd.Flags().String("title", "", "rule title (defaults to the title-cased slug)")
	cmd.Flags().String("owner", "", "owner/author (defaults to $USER)")
	cmd.Flags().String("date", "", "ISO-8601 date (defaults to today, UTC)")
	cmd.Flags().String("why", "", "the reason plus one concrete use case")
	cmd.Flags().String("exceptions", "", "the explicit escape hatch and who may use it (default \"none\")")
	cmd.Flags().String("instructions", "", "what an agent should do to comply")
	cmd.Flags().String("compliant", "", "a worked example that satisfies the rule")
	cmd.Flags().String("violation", "", "a worked example that breaks it")
	cmd.Flags().String("supersedes", "", "the rule this one replaces")
	cmd.Flags().StringArray("skill", nil, "skill this rule binds, by name; repeatable")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runRuleExpand(cmd *cobra.Command, args []string) error {
	slug, err := singleRuleSlugArg(cmd, args, "expand")
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	rulesDir := rule.RulesDir(root)
	if err := preflightRuleIndex(rulesDir, slug); err != nil {
		return err
	}
	row, err := rule.ResolveRow(rulesDir, slug)
	if err != nil {
		return err
	}
	if _, statErr := osStatCLI(rule.DetailPath(rulesDir, slug)); statErr == nil {
		return exitcode.ConflictErrorf("rule %q already has a detail document at %s", slug, rule.DetailPath(rulesDir, slug))
	} else if !os.IsNotExist(statErr) {
		return exitcode.UnexpectedErrorf("preflighting %s: %v", rule.DetailPath(rulesDir, slug), statErr)
	}

	flags := cmd.Flags()
	get := func(name string) string { v, _ := flags.GetString(name); return v }
	skills, _ := flags.GetStringArray("skill")
	owner := get("owner")
	if owner == "" {
		owner = osGetenvCLI("USER")
	}
	// Everything the row owns is copied verbatim, so the new document starts
	// in agreement with it and R-011 has nothing to report.
	mirrored := rule.MirroredValuesOf(row)
	opts := rule.Options{
		Slug: slug, Title: get("title"), Owner: owner, Date: get("date"),
		Status: mirrored["Status"], Statement: mirrored["Statement"],
		Scopes: row.ScopeList(), Sources: row.SourceList(),
		Enforcement: mirrored["Enforcement"], Control: mirrored["Control"],
		Why: get("why"), Exceptions: get("exceptions"), Supersedes: get("supersedes"),
		Instructions: get("instructions"), Compliant: get("compliant"), Violation: get("violation"),
		Skills: skills,
	}
	if err := opts.Normalize(); err != nil {
		return exitcode.InvalidArgsErrorf("%v", err)
	}
	if err := writeRuleDetail(rulesDir, opts, false); err != nil {
		return err
	}
	row.Linked = true
	if err := ruleUpsertIndexRowFn(rulesDir, row); err != nil {
		return exitcode.UnexpectedErrorf("linking the rule's index row: %v", err)
	}
	return writeRuleResult(cmd, format, ruleWriteResult{
		Slug: slug, Form: ruleFormName(true), Path: rule.DetailPath(rulesDir, slug),
		Status: strings.TrimSpace(row.Status), Action: "expanded", Detailed: true,
	}, root)
}
