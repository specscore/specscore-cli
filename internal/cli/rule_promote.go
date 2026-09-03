package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// rulePromoteResult reports both halves of the pair the verb wrote, so a caller
// can see that the Lesson was touched as well as the rule.
type rulePromoteResult struct {
	Slug        string `json:"slug" yaml:"slug"`
	Form        string `json:"form" yaml:"form"`
	Path        string `json:"path" yaml:"path"`
	Status      string `json:"status" yaml:"status"`
	Action      string `json:"action" yaml:"action"`
	FromLesson  string `json:"from_lesson" yaml:"from_lesson"`
	LessonPath  string `json:"lesson_path" yaml:"lesson_path"`
	Enforcement string `json:"enforcement" yaml:"enforcement"`
	Detailed    bool   `json:"detailed" yaml:"detailed"`
}

func rulePromoteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote --from-lesson <lesson-slug> <rule-slug>",
		Short: "Turn a Lesson's control into a rule and link the pair both ways",
		Long: `Records a rule from a Lesson, writes ` + "`**Promotes To:** rule:<rule-slug>`" + ` back
into that Lesson, and records ` + "`lesson:<lesson-slug>`" + ` in the rule's sources. Lint
(R-008) checks that pair in both directions afterwards, so neither half can be
edited away unnoticed.

A promoted rule is detailed by default: a Lesson already carries the reason a
rule needs, so throwing it away to save a file would lose the more valuable
half. Pass --inline to record only the index row.

Fields are pre-filled from the Lesson and can each be overridden:

  --statement  defaults to the Lesson's **Control:** (its enforceable sentence),
               falling back to its title.
  --why        defaults to the Lesson's **Verification:**, then its title.
  --control    defaults to the Lesson's **Control:** when promoting at the
               Enforced or Automated tier.

A Lesson carries exactly one promotion pointer, so promoting a Lesson that
already promotes to another rule exits 4 rather than silently repointing it.

Example:

  specscore rule promote --from-lesson never-mock-extension-backends never-mock-backends \
    --scope fleet --enforcement Stated`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRulePromote,
	}
	cmd.Flags().String("from-lesson", "", "slug of the Lesson to promote (required)")
	addRuleAuthoringFlags(cmd)
	cmd.Flags().Bool("inline", false, "record only the index row instead of a full detail document")
	cmd.Flags().Bool("force", false, "overwrite an existing rule at that slug")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runRulePromote(cmd *cobra.Command, args []string) error {
	ruleSlug, err := singleRuleSlugArg(cmd, args, "promote")
	if err != nil {
		return err
	}
	lessonSlug, _ := cmd.Flags().GetString("from-lesson")
	lessonSlug = strings.TrimSpace(lessonSlug)
	if lessonSlug == "" {
		return exitcode.InvalidArgsError("missing required flag: --from-lesson <lesson-slug>")
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	inline, _ := cmd.Flags().GetBool("inline")
	detailed := !inline

	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	lessonPath, err := lessonResolveLessonFileFn(lessonsDir, lessonSlug)
	if err != nil {
		return err
	}
	l, err := lesson.Parse(lessonPath)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", lessonSlug, err)
	}

	// A Lesson has one promotion pointer. Repointing it silently would break
	// the "which rule did this Lesson become?" answer the pair exists to give.
	if existing, ok, parseErr := rule.ParsePromotesTo(l.PromotesTo); parseErr == nil && ok && existing != ruleSlug {
		return exitcode.InvalidStateErrorf(
			"lesson %q already promotes to rule:%s; a Lesson carries one promotion pointer — update that rule, or clear the Lesson's **Promotes To:** first",
			lessonSlug, existing)
	}

	opts, err := rulePromoteOptions(cmd, ruleSlug, l)
	if err != nil {
		return err
	}

	rulesDir := rule.RulesDir(root)
	force, _ := cmd.Flags().GetBool("force")
	if err := ensureRuleAncestorIndexes(root); err != nil {
		return exitcode.UnexpectedErrorf("materializing ancestor indexes: %v", err)
	}
	if !force {
		if _, err := rule.ResolveRow(rulesDir, ruleSlug); err == nil {
			return exitcode.ConflictErrorf("rule already exists: %s is already listed in %s (pass --force to overwrite)",
				ruleSlug, rule.IndexPath(rulesDir))
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
	// The Lesson half is written after the rule exists, so a failure can never
	// leave a Lesson promoting to a rule that was never created.
	if err := ruleSetLessonPromotesToFn(lessonPath, ruleSlug); err != nil {
		return exitcode.UnexpectedErrorf("writing **Promotes To:** into lesson %s: %v", lessonSlug, err)
	}

	result := rulePromoteResult{
		Slug: ruleSlug, Form: ruleFormName(detailed), Path: ruleResultPath(rulesDir, detailed, ruleSlug),
		Status: opts.Status, Action: "promoted", FromLesson: lessonSlug, LessonPath: lessonPath,
		Enforcement: opts.Enforcement, Detailed: detailed,
	}
	w := cmd.OutOrStdout()
	switch format {
	case "json":
		return newJSONEnc(w).Encode(result)
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(result); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	default:
		_, _ = fmt.Fprintf(w, "%s\n", result.Path)
		_, _ = fmt.Fprintf(w, "%s: **Promotes To:** rule:%s\n", lessonPath, ruleSlug)
		return nil
	}
}

// rulePromoteOptions pre-fills the authoring options from the Lesson and
// applies every explicit override. The Lesson's Control is the enforceable
// sentence it already carries, which is exactly what a rule's Statement is for
// — so the common promotion needs no --statement at all.
func rulePromoteOptions(cmd *cobra.Command, ruleSlug string, l *lesson.Lesson) (rule.Options, error) {
	flags := cmd.Flags()
	get := func(name string) string { v, _ := flags.GetString(name); return v }
	getArr := func(name string) []string { v, _ := flags.GetStringArray(name); return v }

	title := get("title")
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSpace(l.Title)
	}
	statement := get("statement")
	if strings.TrimSpace(statement) == "" {
		statement = firstNonSentinel(l.Control, l.Title)
	}
	why := get("why")
	if strings.TrimSpace(why) == "" {
		why = firstNonSentinel(l.Verification, l.Title)
	}
	owner := get("owner")
	if strings.TrimSpace(owner) == "" {
		owner = strings.TrimSpace(l.Owner)
	}
	if strings.TrimSpace(owner) == "" {
		owner = osGetenvCLI("USER")
	}
	control := get("control")
	if strings.TrimSpace(control) == "" && rule.RequiresControl(canonicalTierOrEmpty(get("enforcement"))) {
		control = firstNonSentinel(l.Control, "")
	}

	// The promoted Lesson is always the first source; explicit --source values
	// follow, deduplicated.
	merged := []string{rule.SourceLesson + ":" + l.Slug}
	for _, s := range getArr("source") {
		if s = strings.TrimSpace(s); s != "" && !containsString(merged, s) {
			merged = append(merged, s)
		}
	}

	opts := rule.Options{
		Slug: ruleSlug, Title: title, Owner: owner, Date: get("date"), Status: get("status"),
		Statement: statement, Scopes: getArr("scope"), Sources: merged,
		Enforcement: get("enforcement"), Control: control,
		Why: why, Exceptions: get("exceptions"), Supersedes: get("supersedes"),
		Instructions: get("instructions"), Compliant: get("compliant"), Violation: get("violation"),
		Skills: getArr("skill"),
	}
	if err := opts.Normalize(); err != nil {
		return rule.Options{}, exitcode.InvalidArgsErrorf("%v", err)
	}
	return opts, nil
}

func canonicalTierOrEmpty(value string) string {
	tier, ok := rule.ParseEnforcement(value)
	if !ok {
		return ""
	}
	return tier
}

// firstNonSentinel returns the first value that carries real content.
func firstNonSentinel(values ...string) string {
	for _, v := range values {
		if isRealRuleValue(v) {
			return strings.Join(strings.Fields(v), " ")
		}
	}
	return ""
}

// ruleSkillsPath reads the optional `rules.skills_path` override from the
// project config, falling back to the default location.
func ruleSkillsPath(projectRoot string) string {
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		return ""
	}
	raw, ok := cfg.Extras["rules"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := raw["skills_path"].(string)
	return value
}
