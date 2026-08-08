package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/dryrun"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/issue"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

// issueCommand returns the "issue" command group — scaffold, transition, and list Issue artifacts.
func issueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Issue management — scaffold, transition, and list Issue artifacts",
	}
	cmd.AddCommand(issueNewCommand(), issueChangeStatusCommand(), issueListCommand(), issueTransitionsCommand())
	return cmd
}

// issueTransitionsCommand registers `specscore issue transitions [<slug>]`,
// the read-only counterpart to change-status. Issue keeps its own tiny
// matrix (pkg/issue.LegalTransitions) rather than a pkg/lifecycle.Kind, so
// this command is wired by hand instead of through the shared
// transitionsCommand builder used by the pkg/lifecycle-backed kinds — but it
// derives its output from that SAME map (pkg/issue.BidirectionalMatrix), not
// a hand-authored duplicate.
func issueTransitionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transitions [<slug>]",
		Short: "Show the Issue status matrix, or one issue's legal next statuses",
		Long: `Prints the Issue kind's legal-transition matrix: every recognized status
with the statuses that can legally precede it ("previous") and the statuses
it can legally become ("next"), derived from the same map change-status
validates against. An empty "previous" means the status is set only by
` + "`issue new`" + `, never by change-status; an empty "next" means the status is
terminal.

With <slug>, reports that ONE issue's CURRENT status and the "next" values a
change-status call would accept for it right now. Read-only — it never
mutates the issue.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runIssueTransitions,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, json, yaml")
	return cmd
}

func runIssueTransitions(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "text" && format != "json" && format != "yaml" {
		return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, json, yaml)", format)
	}

	if len(args) == 0 {
		switch format {
		case "json":
			return printJSON(cmd.OutOrStdout(), issue.BidirectionalMatrix())
		case "yaml":
			return printYAML(cmd.OutOrStdout(), issue.BidirectionalMatrix())
		default:
			_, _ = fmt.Fprint(cmd.OutOrStdout(), issue.RenderBidirectionalMatrix())
			return nil
		}
	}

	slug := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	specDir := filepath.Join(specRoot, "spec")
	discovered, err := issue.DiscoverAll(specDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering issues: %v", err)
	}
	var issuePath string
	for _, d := range discovered {
		if d.Slug == slug {
			issuePath = d.Path
			break
		}
	}
	if issuePath == "" {
		return exitcode.NotFoundErrorf("issue %q not found under %s", slug, specDir)
	}
	parsed, err := issue.Parse(issuePath)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading %s: %v", issuePath, err)
	}
	current := parsed.Frontmatter["status"]
	if current == "" {
		current = "open"
	}
	edge := issue.EdgeFor(current)

	switch format {
	case "json":
		return printJSON(cmd.OutOrStdout(), struct {
			ID       string   `json:"id"`
			Status   string   `json:"status"`
			Previous []string `json:"previous"`
			Next     []string `json:"next"`
		}{slug, edge.Status, edge.Previous, edge.Next})
	case "yaml":
		return printYAML(cmd.OutOrStdout(), struct {
			ID       string   `yaml:"id"`
			Status   string   `yaml:"status"`
			Previous []string `yaml:"previous"`
			Next     []string `yaml:"next"`
		}{slug, edge.Status, edge.Previous, edge.Next})
	default:
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", slug, edge.Status)
		prev := "(none — initial status, set only by `issue new`)"
		if len(edge.Previous) > 0 {
			prev = strings.Join(edge.Previous, ", ")
		}
		next := "(none — terminal status)"
		if len(edge.Next) > 0 {
			next = strings.Join(edge.Next, ", ")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  previous: %s\n", prev)
		fmt.Fprintf(cmd.OutOrStdout(), "  next:     %s\n", next)
		return nil
	}
}

// issueNewCommand scaffolds a lint-clean Issue artifact at spec/issues/<slug>.md
// or spec/features/<feature>/issues/<slug>.md when --feature is supplied.
func issueNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Issue artifact",
		Long: `Creates a lint-clean Issue skeleton at spec/issues/<slug>.md.

When --feature=<feature-slug> is supplied, creates the file at
spec/features/<feature-slug>/issues/<slug>.md (verifying the parent Feature
directory exists).

Supply content via flags (--title, --severity, --affected-component,
--captured-by). The generated file is always lint-clean — running
` + "`specscore lint`" + ` immediately afterwards passes.`,
		Args: cobra.ExactArgs(1),
		RunE: runIssueNew,
	}
	cmd.Flags().String("feature", "", "parent Feature slug (places issue under spec/features/<slug>/issues/)")
	cmd.Flags().String("severity", "", "severity: low, medium, high, critical")
	cmd.Flags().String("affected-component", "", "affected component (Feature slug)")
	cmd.Flags().String("captured-by", "", "author identifier (defaults to $USER)")
	cmd.Flags().String("title", "", "Issue title (defaults to title-cased slug)")
	cmd.Flags().Bool("force", false, "overwrite an existing issue file at that slug")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runIssueNew(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := issue.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	featureFlag, _ := cmd.Flags().GetString("feature")
	severity, _ := cmd.Flags().GetString("severity")
	affectedComponent, _ := cmd.Flags().GetString("affected-component")
	capturedBy, _ := cmd.Flags().GetString("captured-by")
	title, _ := cmd.Flags().GetString("title")
	force, _ := cmd.Flags().GetBool("force")
	projectFlag, _ := cmd.Flags().GetString("project")

	// Validate --severity if supplied.
	if severity != "" && !issue.ValidSeverityValues[severity] {
		return exitcode.InvalidArgsErrorf(
			"invalid --severity %q; valid values: low, medium, high, critical", severity)
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	// Determine target path.
	var target string
	if featureFlag != "" {
		featureDir := filepath.Join(specRoot, "spec", "features", featureFlag)
		if _, err := os.Stat(featureDir); os.IsNotExist(err) {
			return exitcode.NotFoundErrorf("target feature directory does not exist: %s", featureDir)
		}
		issuesDir := filepath.Join(featureDir, "issues")
		if err := os.MkdirAll(issuesDir, 0o755); err != nil {
			return exitcode.UnexpectedErrorf("creating %s: %v", issuesDir, err)
		}
		target = filepath.Join(issuesDir, slug+".md")
	} else {
		issuesDir := filepath.Join(specRoot, "spec", "issues")
		if err := os.MkdirAll(issuesDir, 0o755); err != nil {
			return exitcode.UnexpectedErrorf("creating %s: %v", issuesDir, err)
		}
		target = filepath.Join(issuesDir, slug+".md")
	}

	// Check collision.
	if _, err := os.Stat(target); err == nil && !force {
		return exitcode.ConflictErrorf("issue already exists: %s (pass --force to overwrite)", target)
	}

	opts := issue.ScaffoldOptions{
		Slug:              slug,
		Title:             title,
		CapturedBy:        capturedBy,
		Severity:          severity,
		AffectedComponent: affectedComponent,
	}

	// cli-template-runtime-fetch: a bare issue (no severity / affected-component)
	// pulls the published template; otherwise the embedded scaffolder fills
	// those frontmatter fields.
	bare := severity == "" && affectedComponent == ""
	body, err := bareOrEmbedded(bare, "issue", map[string]string{
		"<Issue Title>":        templateTitleOrSlug(title, slug),
		"<slug>":               slug,
		"<ISO-8601 timestamp>": templateNowRFC3339(),
		"<your-handle>":        templateOwnerOrUnknown(capturedBy),
	}, cmd.ErrOrStderr(), func() ([]byte, error) {
		return issueScaffoldFn(opts)
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding issue: %v", err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
	}

	// Run lint --fix then verify.
	specSub := filepath.Join(specRoot, "spec")
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
		if v.Severity == "error" && (v.File == relTarget || strings.HasPrefix(v.File, "issues/")) {
			own = append(own, v)
		}
	}
	if len(own) > 0 {
		var sb strings.Builder
		sb.WriteString("generated issue failed lint:\n")
		for _, v := range own {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return exitcode.UnexpectedError(sb.String())
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

// issueChangeStatusCommand transitions an Issue's status field via the
// issue lifecycle state-machine.
func issueChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition an Issue's Status",
		Long: `Transitions an Issue from its current status to the value named by --to.
The transition is validated against the Issue legal-transition matrix;
illegal (from, to) pairs exit 4. On success, the verb runs
` + "`specscore spec lint --fix`" + ` to keep indexes in sync, prints
"<slug>: <from> → <to>" to stdout, and exits 0.

Legal target statuses: ` + strings.Join(issue.ValidTargetStatuses, ", ") + `

Examples:

  specscore issue change-status payment-timeout --to=investigating
  specscore issue change-status payment-timeout --to=resolved --severity=high
  specscore issue change-status payment-timeout --to=rejected --reason=not-a-defect
`,
		Args: cobra.ExactArgs(1),
		RunE: runIssueChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required). Legal values: "+
		strings.Join(issue.ValidTargetStatuses, ", "))
	_ = cmd.MarkFlagRequired("to")
	cmd.Flags().String("severity", "", "severity override: low, medium, high, critical")
	cmd.Flags().String("reason", "", "rejection reason (required when --to=rejected). Valid: "+
		strings.Join(issue.ValidReasonValues, ", "))
	cmd.Flags().String("notes", "", "rejection notes (optional, only with --to=rejected)")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool("dry-run", false, "report the transition and every file that would change, writing nothing")
	return cmd
}

func runIssueChangeStatus(cmd *cobra.Command, args []string) error {
	slug := args[0]

	toRaw, _ := cmd.Flags().GetString("to")
	to := strings.ToLower(strings.TrimSpace(toRaw))

	// Validate --to value.
	valid := false
	for _, v := range issue.ValidTargetStatuses {
		if v == to {
			valid = true
			break
		}
	}
	if !valid {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q; legal values: %s",
			toRaw, strings.Join(issue.ValidTargetStatuses, ", "))
	}

	severity, _ := cmd.Flags().GetString("severity")
	reason, _ := cmd.Flags().GetString("reason")
	notes, _ := cmd.Flags().GetString("notes")
	projectFlag, _ := cmd.Flags().GetString("project")

	// --reason is only allowed with --to=rejected.
	if to != "rejected" && reason != "" {
		return exitcode.InvalidArgsErrorf("--reason is only valid when --to=rejected")
	}
	// --notes is only allowed with --to=rejected.
	if to != "rejected" && notes != "" {
		return exitcode.InvalidArgsErrorf("--notes is only valid when --to=rejected")
	}

	// Validate --severity if supplied.
	if severity != "" && !issue.ValidSeverityValues[severity] {
		return exitcode.InvalidArgsErrorf(
			"invalid --severity %q; valid values: low, medium, high, critical", severity)
	}

	// Validate --reason if supplied.
	if reason != "" {
		validReason := false
		for _, r := range issue.ValidReasonValues {
			if r == reason {
				validReason = true
				break
			}
		}
		if !validReason {
			return exitcode.InvalidArgsErrorf(
				"invalid --reason %q; valid values: %s",
				reason, strings.Join(issue.ValidReasonValues, ", "))
		}
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	if dryRunFlag, _ := cmd.Flags().GetBool("dry-run"); dryRunFlag {
		result, changes, err := dryrun.Sandbox(specRoot, func(sandboxRoot string) (issue.ChangeStatusResult, error) {
			return issueChangeStatusMutate(sandboxRoot, slug, to, severity, reason, notes)
		})
		if err != nil {
			return err
		}
		dryrun.PrintReport(cmd.OutOrStdout(), result.Slug, result.From, result.To, changes)
		return nil
	}

	result, err := issueChangeStatusMutate(specRoot, slug, to, severity, reason, notes)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, result.From, result.To)
	return nil
}

// issueChangeStatusMutate performs the Issue Status transition rooted at
// root (the project root containing spec/). It is the single mutation path
// both the real command and its --dry-run sandbox invoke.
func issueChangeStatusMutate(root, slug, to, severity, reason, notes string) (issue.ChangeStatusResult, error) {
	specSub := filepath.Join(root, "spec")
	return issue.ChangeStatus(issue.ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         slug,
		To:           to,
		Severity:     severity,
		Reason:       reason,
		Notes:        notes,
		PostMutation: issueLintPostMutationHook(specSub, slug),
	})
}

// issueLintPostMutationHook returns a post-mutation hook scoped to Issue
// violations. It runs lint --fix to sync indexes, then verifies only
// issue-related violations (files under issues/ or features/*/issues/).
// This avoids failing on pre-existing violations in other parts of the tree.
func issueLintPostMutationHook(specSub, slug string) issue.PostMutationHook {
	return func() error {
		if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
			return exitcode.UnexpectedErrorf("running lint --fix: %v", err)
		}
		violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
		if err != nil {
			return exitcode.UnexpectedErrorf("running lint: %v", err)
		}
		var errs []lint.Violation
		for _, v := range violations {
			if v.Severity == "error" && isIssueRelatedViolation(v.File) {
				errs = append(errs, v)
			}
		}
		if len(errs) > 0 {
			var sb strings.Builder
			sb.WriteString("lint failed after status rewrite (rollback applied):\n")
			for _, v := range errs {
				fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
			}
			return exitcode.UnexpectedError(sb.String())
		}
		return nil
	}
}

// isIssueRelatedViolation reports whether a violation file path is in an
// issues directory (root or Feature-scoped).
func isIssueRelatedViolation(file string) bool {
	return strings.HasPrefix(file, "issues/") || strings.Contains(file, "/issues/")
}

// issueListCommand returns the "issue list" subcommand.
func issueListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all discovered Issue artifacts",
		Long: `Lists all issues in a project, sorted by status priority (open first,
then investigating, then resolved/rejected) and by captured_at descending
within each group. Use --status, --severity, or --feature to filter.`,
		Args: cobra.NoArgs,
		RunE: runIssueList,
	}
	cmd.Flags().String("status", "", "filter by status (case-insensitive)")
	cmd.Flags().String("severity", "", "filter by severity (case-insensitive)")
	cmd.Flags().String("feature", "", "filter by parent feature slug")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

// issueListEntry is the structured representation emitted in yaml/json format.
type issueListEntry struct {
	Slug       string  `json:"slug" yaml:"slug"`
	Title      string  `json:"title" yaml:"title"`
	Status     string  `json:"status" yaml:"status"`
	Severity   *string `json:"severity" yaml:"severity"`
	Feature    *string `json:"feature" yaml:"feature"`
	CapturedAt string  `json:"captured_at" yaml:"captured_at"`
	CapturedBy string  `json:"captured_by" yaml:"captured_by"`
}

// issueStatusPriority maps status to sort priority (lower = first).
var issueStatusPriority = map[string]int{
	"open":          0,
	"investigating": 1,
	"resolved":      2,
	"rejected":      2,
}

func runIssueList(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	statusFilter, _ := cmd.Flags().GetString("status")
	severityFilter, _ := cmd.Flags().GetString("severity")
	featureFilter, _ := cmd.Flags().GetString("feature")
	projectFlag, _ := cmd.Flags().GetString("project")

	// Validate format.
	if format != "text" && format != "yaml" && format != "json" {
		return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, yaml, json)", format)
	}

	// Validate --status if supplied.
	if statusFilter != "" {
		valid := false
		for _, s := range []string{"open", "investigating", "resolved", "rejected"} {
			if strings.EqualFold(s, statusFilter) {
				valid = true
				break
			}
		}
		if !valid {
			return exitcode.InvalidArgsErrorf(
				"invalid --status %q; valid values: open, investigating, resolved, rejected", statusFilter)
		}
	}

	// Validate --severity if supplied.
	if severityFilter != "" && !issue.ValidSeverityValues[strings.ToLower(severityFilter)] {
		return exitcode.InvalidArgsErrorf(
			"invalid --severity %q; valid values: low, medium, high, critical", severityFilter)
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	specDir := filepath.Join(specRoot, "spec")
	discovered, err := issue.DiscoverAll(specDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering issues: %v", err)
	}

	// Parse each discovered issue and build entries.
	var entries []issueListEntry
	for _, d := range discovered {
		parsed, parseErr := issueParseFn(d.Path)
		if parseErr != nil {
			continue
		}

		status := parsed.Frontmatter["status"]
		if status == "" {
			status = "open"
		}
		title := parsed.Frontmatter["slug"]
		// Extract title from body heading if available.
		if t := extractIssueTitle(parsed.Body); t != "" {
			title = t
		}
		sev := parsed.Frontmatter["severity"]
		capturedAt := parsed.Frontmatter["captured_at"]
		capturedBy := parsed.Frontmatter["captured_by"]

		var sevPtr *string
		if sev != "" {
			sevPtr = &sev
		}
		var featurePtr *string
		if d.FeatureSlug != "" {
			fs := d.FeatureSlug
			featurePtr = &fs
		}

		entry := issueListEntry{
			Slug:       d.Slug,
			Title:      title,
			Status:     status,
			Severity:   sevPtr,
			Feature:    featurePtr,
			CapturedAt: capturedAt,
			CapturedBy: capturedBy,
		}

		// Apply filters.
		if statusFilter != "" && !strings.EqualFold(entry.Status, statusFilter) {
			continue
		}
		if severityFilter != "" {
			if entry.Severity == nil || !strings.EqualFold(*entry.Severity, severityFilter) {
				continue
			}
		}
		if featureFilter != "" {
			if entry.Feature == nil || *entry.Feature != featureFilter {
				continue
			}
		}

		entries = append(entries, entry)
	}

	// Sort: by status priority ascending, then captured_at descending.
	sort.Slice(entries, func(i, j int) bool {
		pi := issueStatusPriority[entries[i].Status]
		pj := issueStatusPriority[entries[j].Status]
		if pi != pj {
			return pi < pj
		}
		// Descending captured_at (lexicographic on RFC3339 works).
		return entries[i].CapturedAt > entries[j].CapturedAt
	})

	w := cmd.OutOrStdout()

	switch format {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(entries); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	case "json":
		return newJSONEnc(w).Encode(entries)
	default:
		// Text: padded column format.
		if len(entries) == 0 {
			return nil
		}
		for _, e := range entries {
			sev := "-"
			if e.Severity != nil {
				sev = *e.Severity
			}
			feat := "-"
			if e.Feature != nil {
				feat = *e.Feature
			}
			_, _ = fmt.Fprintf(w, "%-30s %-14s %-10s %-8s %s\n",
				e.Slug, e.Status, sev, feat, e.CapturedAt)
		}
	}
	return nil
}

// extractIssueTitle extracts the title from a "# Issue: <title>" heading.
func extractIssueTitle(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# Issue:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# Issue:"))
		}
	}
	return ""
}
