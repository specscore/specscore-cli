package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// ruleDeleteResult reports what the verb removed and what it rewrote, so a
// caller can see the blast radius without diffing the tree.
type ruleDeleteResult struct {
	Slug            string   `json:"slug" yaml:"slug"`
	Form            string   `json:"form" yaml:"form"`
	Action          string   `json:"action" yaml:"action"`
	SupersededWith  string   `json:"superseded_with" yaml:"superseded_with"`
	RewrittenLinks  []string `json:"rewritten_links" yaml:"rewritten_links"`
	RemovedDetail   string   `json:"removed_detail" yaml:"removed_detail"`
	RemainingRefsTo []string `json:"remaining_refs_to" yaml:"remaining_refs_to"`
}

func ruleDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete a rule, refusing while anything still links to it",
		Long: `Removes the rule's row from spec/rules/README.md and, for a detailed rule,
its spec/rules/<slug>/ directory.

The verb refuses (exit 4) while any Lesson promotes to the rule, any other
rule's detail document supersedes or is superseded by it, any skill lists it,
or any Feature cites it — deleting an artifact out from under a live reference
is how a spec tree silently stops linting.

--supersede-with <slug> makes the deletion safe instead of refusing it: every
Lesson pointing here is repointed at the successor and added to the successor's
sources, every rule relation is rewritten, and only then is the rule removed.
Skill and Feature references are prose and are reported rather than rewritten;
they must be edited by hand.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleDelete,
	}
	cmd.Flags().String("supersede-with", "", "rewrite every incoming link to this rule before deleting")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runRuleDelete(cmd *cobra.Command, args []string) error {
	slug, err := singleRuleSlugArg(cmd, args, "delete")
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	successor, _ := cmd.Flags().GetString("supersede-with")
	successor = strings.TrimSpace(successor)
	if successor == slug {
		return exitcode.InvalidArgsError("--supersede-with must name a different rule")
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	specSub := filepath.Join(root, "spec")
	rulesDir := rule.RulesDir(root)
	row, err := rule.ResolveRow(rulesDir, slug)
	if err != nil {
		return err
	}
	var successorRow rule.Row
	if successor != "" {
		successorRow, err = rule.ResolveRow(rulesDir, successor)
		if err != nil {
			return exitcode.InvalidArgsErrorf("--supersede-with %q does not resolve to a rule in the index", successor)
		}
	}

	promoting := lessonsPromotingTo(specSub, slug)
	relatedRules := rulesReferencing(rulesDir, slug)
	citingFeatures := featuresCitingRule(specSub, slug)
	citingSkills := skillsReferencing(root, slug)

	if successor == "" {
		var blockers []string
		for _, l := range promoting {
			blockers = append(blockers, "lesson:"+l)
		}
		for _, r := range relatedRules {
			blockers = append(blockers, "rule:"+r)
		}
		for _, s := range citingSkills {
			blockers = append(blockers, "skill:"+s)
		}
		for _, f := range citingFeatures {
			blockers = append(blockers, "feature:"+f)
		}
		if len(blockers) > 0 {
			sort.Strings(blockers)
			return exitcode.InvalidStateErrorf(
				"rule %q is still linked from %s; pass --supersede-with <slug> to repoint those links, or remove them first",
				slug, strings.Join(blockers, ", "))
		}
	}

	var rewritten []string
	if successor != "" {
		rewritten, err = repointRuleLinks(rulesDir, specSub, slug, successorRow, promoting, relatedRules)
		if err != nil {
			return err
		}
	}

	removedDetail := ""
	if row.Detailed() {
		removedDetail = filepath.Dir(rule.DetailPath(rulesDir, slug))
		if err := osRemoveAllCLI(removedDetail); err != nil {
			return exitcode.UnexpectedErrorf("removing the rule directory: %v", err)
		}
	}
	if err := ruleRemoveIndexRowFn(rulesDir, slug); err != nil {
		return exitcode.UnexpectedErrorf("removing the rule's index row: %v", err)
	}

	result := ruleDeleteResult{
		Slug: slug, Form: ruleFormName(row.Detailed()), Action: "deleted",
		SupersededWith: successor, RewrittenLinks: orEmptySlice(rewritten),
		RemovedDetail: removedDetail,
		// Both kinds are prefixed, so a caller can tell a Feature citation from a
		// skill one without knowing the order they were appended in.
		RemainingRefsTo: orEmptySlice(append(prefixEach("feature:", citingFeatures), prefixEach("skill:", citingSkills)...)),
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
		_, _ = fmt.Fprintf(w, "deleted rule %s (%s)\n", slug, result.Form)
		for _, link := range result.RewrittenLinks {
			_, _ = fmt.Fprintf(w, "repointed %s -> rule:%s\n", link, successor)
		}
		for _, ref := range result.RemainingRefsTo {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s still cites rule:%s in prose; edit it by hand\n", ref, slug)
		}
		return nil
	}
}

func prefixEach(prefix string, values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, prefix+v)
	}
	return out
}

// rulesReferencing returns the slugs of other rules whose detail document names
// slug in a supersession pointer.
func rulesReferencing(rulesDir, slug string) []string {
	details, err := rule.DiscoverDetails(rulesDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range details {
		if d.Slug == slug {
			continue
		}
		if strings.TrimSpace(d.Supersedes) == slug || strings.TrimSpace(d.SupersededBy) == slug {
			out = append(out, d.Slug)
		}
	}
	sort.Strings(out)
	return out
}

// skillsReferencing returns the names of skills whose `## Rules` section lists
// this rule.
func skillsReferencing(projectRoot, slug string) []string {
	skills, err := rule.DiscoverSkills(rule.SkillsDir(projectRoot, ruleSkillsPath(projectRoot)))
	if err != nil {
		return nil
	}
	var out []string
	for _, s := range skills {
		for _, ref := range s.RuleRefs {
			if ref == slug {
				out = append(out, s.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// repointRuleLinks rewrites every incoming link from slug to successor and
// returns the human-readable list of what it touched.
func repointRuleLinks(rulesDir, specSub, slug string, successorRow rule.Row, promoting, relatedRules []string) ([]string, error) {
	var rewritten []string

	lessonsDir := filepath.Join(specSub, "lessons")
	successor := successorRow.Slug
	successorSources := successorRow.SourceList()

	for _, lessonSlug := range promoting {
		lessonPath, err := lessonResolveLessonFileFn(lessonsDir, lessonSlug)
		if err != nil {
			return nil, exitcode.UnexpectedErrorf("resolving lesson %s: %v", lessonSlug, err)
		}
		if err := ruleSetLessonPromotesToFn(lessonPath, successor); err != nil {
			return nil, exitcode.UnexpectedErrorf("repointing lesson %s: %v", lessonSlug, err)
		}
		ref := rule.SourceLesson + ":" + lessonSlug
		if !containsString(successorSources, ref) {
			successorSources = append(successorSources, ref)
		}
		rewritten = append(rewritten, "lesson:"+lessonSlug)
	}

	// The successor must cite every Lesson it just inherited, or R-008 would
	// report the pair as broken the moment the old rule disappears.
	if len(promoting) > 0 {
		successorRow.Sources = escapeRuleCell(strings.Join(successorSources, ", "))
		if err := ruleUpsertIndexRowFn(rulesDir, successorRow); err != nil {
			return nil, exitcode.UnexpectedErrorf("recording inherited sources on rule %s: %v", successor, err)
		}
		if successorRow.Detailed() {
			if err := mirrorRuleRowIntoDetail(rulesDir, successor, successorRow); err != nil {
				return nil, err
			}
		}
	}

	for _, otherSlug := range relatedRules {
		otherPath := rule.DetailPath(rulesDir, otherSlug)
		other, err := ruleParseDetailFn(otherPath)
		if err != nil {
			return nil, exitcode.UnexpectedErrorf("parsing rule %s: %v", otherSlug, err)
		}
		var edits []rule.FieldEdit
		if strings.TrimSpace(other.Supersedes) == slug {
			edits = append(edits, rule.FieldEdit{Name: "Supersedes", Value: successor})
		}
		if strings.TrimSpace(other.SupersededBy) == slug {
			edits = append(edits, rule.FieldEdit{Name: "Superseded By", Value: successor})
		}
		if err := applyRuleDetailEdits(otherPath, edits); err != nil {
			return nil, err
		}
		rewritten = append(rewritten, "rule:"+otherSlug)
	}

	sort.Strings(rewritten)
	return rewritten, nil
}

// mirrorRuleRowIntoDetail rewrites a detail document's mirrored header from its
// row, which is the source of truth.
func mirrorRuleRowIntoDetail(rulesDir, slug string, row rule.Row) error {
	mirrored := rule.MirroredValuesOf(row)
	edits := make([]rule.FieldEdit, 0, len(rule.MirroredFields))
	for _, name := range rule.MirroredFields {
		edits = append(edits, rule.FieldEdit{Name: name, Value: mirrored[name]})
	}
	return applyRuleDetailEdits(rule.DetailPath(rulesDir, slug), edits)
}

// applyRuleDetailEdits rewrites bold fields of a detail document on disk.
func applyRuleDetailEdits(path string, edits []rule.FieldEdit) error {
	if len(edits) == 0 {
		return nil
	}
	content, err := osReadFileCLI(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading %s: %v", path, err)
	}
	updated, err := ruleApplyFieldEditsFn(content, edits)
	if err != nil {
		return exitcode.UnexpectedErrorf("editing %s: %v", path, err)
	}
	if err := ruleWriteFileAtomicFn(path, updated); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", path, err)
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
