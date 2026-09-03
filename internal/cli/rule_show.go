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
	"github.com/specscore/specscore-cli/pkg/idea"
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
	// ScopeError is the parse failure when this rule's Scope cell is corrupt,
	// carried here for the same reason `rule list` carries it: a reader must
	// not be told a scope that does not actually parse. `show` evaluates no
	// scope, so it reports the flag and exits 0.
	ScopeError string `json:"scope_error" yaml:"scope_error"`

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
		Statement: structuredValue(row.Statement), Status: structuredValue(row.Status),
		Scope: orEmptySlice(row.ScopeList()), Enforcement: structuredValue(row.Enforcement),
		Control: structuredValue(row.Control), Sources: orEmptySlice(row.SourceList()),
		IndexPath: repoRelative(root, rule.IndexPath(rulesDir)),
		Skills:    []string{},
		// The three link sets below are always present, so a JSON consumer can
		// index them without a nil check.
		ScopeError:       ruleScopeError(row),
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
		doc.DetailPath = repoRelative(root, detailPath)
		doc.Title = strings.TrimSpace(detail.Title)
		doc.Date = strings.TrimSpace(detail.Date)
		doc.Owner = strings.TrimSpace(detail.Owner)
		doc.Why = structuredValue(detail.Why)
		doc.Exceptions = structuredValue(detail.Exceptions)
		doc.Supersedes = structuredValue(detail.Supersedes)
		doc.SupersededBy = structuredValue(detail.SupersededBy)
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

// ruleScopeError reports why a row's Scope cell does not parse, or "".
func ruleScopeError(row rule.Row) string {
	if _, err := rule.ParseScopes(row.ScopeList()); err != nil {
		return err.Error()
	}
	return ""
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

// requireResolvableSources refuses a write whose typed source references do not
// resolve. Grammar was already validated at write time; resolution was not,
// which let `--add-source lesson:ghost` exit 0 and hand back a tree that lint
// immediately calls dirty. Refusing is the consistent half: a duplicate and an
// absent source both already exit 2.
func requireResolvableSources(specSub string, sources []string) error {
	bad := unresolvedRuleSources(specSub, sources)
	if len(bad) == 0 {
		return nil
	}
	return exitcode.InvalidArgsErrorf(
		"source(s) do not resolve to an artifact in this spec tree: %s; record the artifact first, or cite it from the rule's **Why:** instead",
		strings.Join(bad, ", "))
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
		switch ref.Kind {
		case rule.SourceLesson:
			if _, err := lessonResolveLessonFileFn(filepath.Join(specSub, "lessons"), ref.Value); err != nil {
				out = append(out, raw)
			}
		case rule.SourceIdea:
			if !ruleIdeaSourceExists(specSub, ref.Value) {
				out = append(out, raw)
			}
		case rule.SourceDecision:
			if !ruleDecisionSourceExists(specSub, ref.Value) {
				out = append(out, raw)
			}
		}
	}
	return out
}

// ruleIdeaSourceExists and ruleDecisionSourceExists mirror the resolution the
// R-007 checker performs, so the verb and the linter agree about what exists.
func ruleIdeaSourceExists(specSub, slug string) bool {
	ideasDir := idea.ResolveIdeasDir(specSub)
	for _, candidate := range []string{
		filepath.Join(ideasDir, slug+".md"),
		filepath.Join(ideasDir, "archived", slug+".md"),
	} {
		if info, err := osStatCLI(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func ruleDecisionSourceExists(specSub, ref string) bool {
	decisionsDir := filepath.Join(specSub, "decisions")
	number, _, _ := strings.Cut(ref, "-")
	for _, dir := range []string{decisionsDir, filepath.Join(decisionsDir, "archived")} {
		entries, err := osReadDirCLI(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), ".md")
			if stem == ref || strings.HasPrefix(stem, number+"-") {
				return true
			}
		}
	}
	return false
}

func writeRuleShowText(w io.Writer, doc ruleShowDoc) error {
	bw := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(bw, "Slug:          %s\n", doc.Slug)
	_, _ = fmt.Fprintf(bw, "Form:          %s\n", doc.Form)
	_, _ = fmt.Fprintf(bw, "Statement:     %s\n", doc.Statement)
	_, _ = fmt.Fprintf(bw, "Status:        %s\n", dashIfEmpty(doc.Status))
	scope := dashIfEmpty(strings.Join(doc.Scope, ", "))
	if doc.ScopeError != "" {
		scope += "  (does not parse: " + doc.ScopeError + ")"
	}
	_, _ = fmt.Fprintf(bw, "Scope:         %s\n", scope)
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

// repoRelative renders a path relative to the project root for structured
// output. An absolute path makes the documented JSON contract machine-dependent
// and un-diffable, while every other field in the document is already
// repo-relative. Text output keeps absolute paths: that one is for a human to
// hand to an editor.
func repoRelative(root, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// structuredValue normalizes a stored cell for machine output: the em-dash
// sentinel is a storage convention of the Markdown table, not a value, so it
// never reaches a consumer. A missing scalar is the empty string and a missing
// list is an empty array — one convention, applied to every field.
func structuredValue(v string) string {
	trimmed := strings.TrimSpace(ruleCellText(v))
	if trimmed == rule.Sentinel || trimmed == "-" {
		return ""
	}
	return trimmed
}
