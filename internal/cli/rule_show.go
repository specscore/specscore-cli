package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// ruleShowDoc is the structured output of `rule show`. Every field is always
// present (no omitempty), so an absent value renders as a present but empty key
// and a script never has to distinguish "missing" from "unset".
type ruleShowDoc struct {
	Slug        string   `json:"slug" yaml:"slug"`
	Form        string   `json:"form" yaml:"form"`
	Statement   string   `json:"statement" yaml:"statement"`
	Status      string   `json:"status" yaml:"status"`
	Scope       []string `json:"scope" yaml:"scope"`
	Enforcement string   `json:"enforcement" yaml:"enforcement"`
	Control     string   `json:"control" yaml:"control"`
	Sources     []string `json:"sources" yaml:"sources"`
	IndexPath   string   `json:"index_path" yaml:"index_path"`

	// Detail-only fields, empty for an inline rule.
	DetailPath   string   `json:"detail_path" yaml:"detail_path"`
	Title        string   `json:"title" yaml:"title"`
	Date         string   `json:"date" yaml:"date"`
	Owner        string   `json:"owner" yaml:"owner"`
	Why          string   `json:"why" yaml:"why"`
	Exceptions   string   `json:"exceptions" yaml:"exceptions"`
	Supersedes   string   `json:"supersedes" yaml:"supersedes"`
	SupersededBy string   `json:"superseded_by" yaml:"superseded_by"`
	Skills       []string `json:"skills" yaml:"skills"`

	// Resolved links — the half a reader cannot get by opening the file.
	PromotingLessons []string `json:"promoting_lessons" yaml:"promoting_lessons"`
	CitingFeatures   []string `json:"citing_features" yaml:"citing_features"`
	UnresolvedLinks  []string `json:"unresolved_links" yaml:"unresolved_links"`
}

func ruleShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <slug>",
		Short: "Show a rule — inline or detailed — with its resolved links",
		Long: `Prints a rule's index row and, when it has one, its detail document,
together with the links a reader cannot get by opening either: the Lessons
whose **Promotes To:** points at it, the Features whose text cites it, the
skills it binds, and any source reference that does not resolve.

Both forms render the same way; the ` + "`form`" + ` field says which one this is.

Default output is YAML; use --format for JSON or a text summary. A rule that is
not listed in the index exits 3 with no stdout output.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleShow,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, text")
	return cmd
}

func runRuleShow(cmd *cobra.Command, args []string) error {
	slug, err := singleRuleSlugArg(cmd, args, "show")
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
	row, err := rule.ResolveRow(rulesDir, slug)
	if err != nil {
		return err
	}

	specSub := filepath.Join(root, "spec")
	doc := ruleShowDoc{
		Slug: slug, Form: ruleFormName(row.Detailed()),
		Statement: ruleCellText(row.Statement), Status: strings.TrimSpace(row.Status),
		Scope: orEmptySlice(row.ScopeList()), Enforcement: strings.TrimSpace(row.Enforcement),
		Control: ruleCellText(row.Control), Sources: orEmptySlice(row.SourceList()),
		IndexPath: rule.IndexPath(rulesDir),
		Skills:    []string{},
		// The three link sets below are always present, so a JSON consumer can
		// index them without a nil check.
		PromotingLessons: lessonsPromotingTo(specSub, slug),
		CitingFeatures:   featuresCitingRule(specSub, slug),
		UnresolvedLinks:  unresolvedRuleSources(specSub, row.SourceList()),
	}

	if row.Detailed() {
		detailPath := rule.DetailPath(rulesDir, slug)
		detail, parseErr := ruleParseDetailFn(detailPath)
		if parseErr != nil {
			return exitcode.UnexpectedErrorf("parsing the rule detail document %s: %v", detailPath, parseErr)
		}
		doc.DetailPath = detailPath
		doc.Title = detail.Title
		doc.Date = detail.Date
		doc.Owner = detail.Owner
		doc.Why = detail.Why
		doc.Exceptions = detail.Exceptions
		doc.Supersedes = detail.Supersedes
		doc.SupersededBy = detail.SupersededBy
		doc.Skills = orEmptySlice(detail.SkillRefs)
	}

	w := cmd.OutOrStdout()
	switch format {
	case "json":
		return newJSONEnc(w).Encode(doc)
	case "text":
		return writeRuleShowText(w, doc)
	default:
		enc := newYAMLEnc(w)
		if err := enc.Encode(doc); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	}
}

func orEmptySlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// lessonsPromotingTo returns the slugs of Lessons whose **Promotes To:** names
// this rule.
func lessonsPromotingTo(specSub, ruleSlug string) []string {
	lessons, err := lessonDiscoverFn(filepath.Join(specSub, "lessons"))
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, l := range lessons {
		target, ok, parseErr := rule.ParsePromotesTo(l.PromotesTo)
		if parseErr == nil && ok && target == ruleSlug {
			out = append(out, l.Slug)
		}
	}
	return out
}

// featuresCitingRule returns the ids of Feature READMEs whose text names this
// rule, either as `rule:<slug>` or as a relative link into spec/rules/<slug>/.
func featuresCitingRule(specSub, ruleSlug string) []string {
	out := []string{}
	featuresDir := filepath.Join(specSub, "features")
	features, err := featureDiscoverFn(featuresDir)
	if err != nil {
		return out
	}
	needles := []string{"rule:" + ruleSlug, "rules/" + ruleSlug + "/README.md"}
	for _, f := range features {
		content, readErr := osReadFileCLI(feature.ReadmePath(featuresDir, f.ID))
		if readErr != nil {
			continue
		}
		text := string(content)
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				out = append(out, f.ID)
				break
			}
		}
	}
	return out
}

// unresolvedRuleSources lists source references that do not resolve locally, so
// `rule show` surfaces the same finding R-007 reports without running lint.
func unresolvedRuleSources(specSub string, sources []string) []string {
	out := []string{}
	for _, raw := range sources {
		ref, err := rule.ParseSource(raw)
		if err != nil {
			out = append(out, raw)
			continue
		}
		if ref.Kind == rule.SourceLesson {
			if _, err := lessonResolveLessonFileFn(filepath.Join(specSub, "lessons"), ref.Value); err != nil {
				out = append(out, raw)
			}
		}
	}
	return out
}

func writeRuleShowText(w io.Writer, doc ruleShowDoc) error {
	bw := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(bw, "Slug:          %s\n", doc.Slug)
	_, _ = fmt.Fprintf(bw, "Form:          %s\n", doc.Form)
	_, _ = fmt.Fprintf(bw, "Statement:     %s\n", doc.Statement)
	_, _ = fmt.Fprintf(bw, "Status:        %s\n", dashIfEmpty(doc.Status))
	_, _ = fmt.Fprintf(bw, "Scope:         %s\n", dashIfEmpty(strings.Join(doc.Scope, ", ")))
	_, _ = fmt.Fprintf(bw, "Enforcement:   %s\n", dashIfEmpty(doc.Enforcement))
	_, _ = fmt.Fprintf(bw, "Control:       %s\n", dashIfEmpty(doc.Control))
	_, _ = fmt.Fprintf(bw, "Sources:       %s\n", dashIfEmpty(strings.Join(doc.Sources, ", ")))
	if doc.DetailPath != "" {
		_, _ = fmt.Fprintf(bw, "Detail:        %s\n", doc.DetailPath)
		_, _ = fmt.Fprintf(bw, "Why:           %s\n", doc.Why)
		_, _ = fmt.Fprintf(bw, "Exceptions:    %s\n", dashIfEmpty(doc.Exceptions))
		_, _ = fmt.Fprintf(bw, "Supersedes:    %s\n", dashIfEmpty(doc.Supersedes))
		_, _ = fmt.Fprintf(bw, "Superseded By: %s\n", dashIfEmpty(doc.SupersededBy))
		_, _ = fmt.Fprintf(bw, "Skills:        %s\n", dashIfEmpty(strings.Join(doc.Skills, ", ")))
	}
	_, _ = fmt.Fprintf(bw, "Promoted from: %s\n", dashIfEmpty(strings.Join(doc.PromotingLessons, ", ")))
	_, _ = fmt.Fprintf(bw, "Cited by:      %s\n", dashIfEmpty(strings.Join(doc.CitingFeatures, ", ")))
	if len(doc.UnresolvedLinks) > 0 {
		_, _ = fmt.Fprintf(bw, "Unresolved:    %s\n", strings.Join(doc.UnresolvedLinks, ", "))
	}
	return bw.Flush()
}

// osIsNotExistCLI is the seam-aware not-exist test the read verbs share.
func osIsNotExistCLI(err error) bool { return os.IsNotExist(err) }
