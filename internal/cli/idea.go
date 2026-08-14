package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/dryrun"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

var discoverIdeasForTransitions = idea.Discover

// ideaCommand returns the "idea" command group — query and scaffold Idea artifacts.
func ideaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "idea",
		Short: "Idea management — list, scaffold, and transition Idea artifacts",
	}
	cmd.AddCommand(ideaListCommand(), ideaChangeStatusCommand(), ideaArchiveCommand(), ideaUnarchiveCommand(), ideaNewCommand(), ideaRelocateCommand(), ideaPromoteCommand(), ideaTransitionsCommand(), ideaParkCommand(), ideaUnparkCommand())
	return cmd
}

// ideaTransitionsCommand registers `specscore idea transitions [<slug>]`,
// the read-only counterpart to change-status. Resolution mirrors
// idea.ChangeStatus's step (1): prefer the canonical active-Idea path, else
// fall back to a Feature-local change-request proposal. Archived Ideas are
// never matched, same as change-status.
func ideaTransitionsCommand() *cobra.Command {
	return transitionsCommand(lifecycle.KindIdea, "slug", "Show the Idea status matrix, or one idea's legal next statuses",
		func(projectFlag, slug string) (string, error) {
			specRoot, err := resolveSpecRoot(projectFlag)
			if err != nil {
				return "", err
			}
			activePath := filepath.Join(specRoot, "spec", "ideas", slug+".md")
			if _, statErr := os.Stat(activePath); statErr == nil {
				return activePath, nil
			}
			discovered, discoverErr := discoverIdeasForTransitions(filepath.Join(specRoot, "spec"))
			if discoverErr != nil {
				return "", exitcode.UnexpectedErrorf("discovering idea %q: %v", slug, discoverErr)
			}
			for _, candidate := range discovered {
				if candidate.Slug == slug && candidate.IsProposal && !candidate.Archived {
					return candidate.Path, nil
				}
			}
			return "", exitcode.NotFoundErrorf("idea not found at %s", activePath)
		})
}

// ideaChangeStatusCommand transitions an Idea's **Status:** field via the
// shared lifecycle state-machine contract. Archival is a separate axis —
// see `idea archive`/`idea unarchive`; change-status never relocates a file.
// See spec/features/cli/idea/change-status/README.md.
func ideaChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition an Idea's Status",
		Long: `Transitions spec/ideas/<slug>.md from its current **Status:** to the value
named by --to. The transition is validated against the Idea legal-transition
matrix below; illegal (from, to) pairs exit 4. On success, the verb runs
` + "`specscore spec lint --fix`" + ` to keep the ideas-index README in sync,
prints "<slug>: <from> → <to>" to stdout, and exits 0.

Archival is NOT a status. To file an Idea out of active view, use
` + "`specscore idea archive <slug>`" + ` (it keeps the Idea's terminal status
and adds the **Archived:** axis). If anything fails after the status rewrite
(lint failure, I/O error), the status line is restored to its pre-invocation
value before the verb exits.

` + idea.LegalTransitionMatrix() + `
Examples:

  specscore idea change-status foo --to=approved
  specscore idea change-status foo --to=rejected
  specscore idea change-status foo --to="In Review"   (case-insensitive)
`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required). Legal values: "+
		strings.Join(idea.LegalChangeStatusTargetNames(), ", ")+
		" (case-insensitive).")
	_ = cmd.MarkFlagRequired("to")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool("dry-run", false, "report the transition and every file that would change, writing nothing")
	return cmd
}

func runIdeaChangeStatus(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	toRaw, _ := cmd.Flags().GetString("to")
	to, ok := lifecycle.ParseStatus(lifecycle.KindIdea, toRaw)
	if !ok {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q for idea; legal values: %s",
			toRaw, strings.Join(idea.LegalChangeStatusTargetNames(), ", "))
	}
	// Even within the recognized Idea statuses, only those that appear
	// as a To column in the matrix are valid as --to values. Reject
	// e.g. --to=draft at flag-parse time (exit 2), BEFORE state-machine
	// check (which would otherwise return exit 4). See REQ:
	// target-status-flag and AC: unrecognized-to-value-rejected.
	if !idea.IsLegalChangeStatusTarget(to) {
		return exitcode.InvalidArgsErrorf(
			"--to value %q is not a user-settable Idea target; legal values: %s",
			toRaw, strings.Join(idea.LegalChangeStatusTargetNames(), ", "))
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		result, changes, err := dryrun.Sandbox(specRoot, func(sandboxRoot string) (idea.ChangeStatusResult, error) {
			return ideaChangeStatusMutate(sandboxRoot, slug, to)
		})
		if err != nil {
			return err
		}
		dryrun.PrintReport(cmd.OutOrStdout(), result.Slug, string(result.From), string(result.To), changes)
		return nil
	}

	result, err := ideaChangeStatusMutate(specRoot, slug, to)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, string(result.From), string(result.To))
	return nil
}

// ideaChangeStatusMutate performs the Idea Status transition rooted at root
// (the project root containing spec/). It is the single mutation path both
// the real command and its --dry-run sandbox invoke (see dryrun.Sandbox),
// so it must derive every path from its root argument.
func ideaChangeStatusMutate(root, slug string, to lifecycle.Status) (idea.ChangeStatusResult, error) {
	return idea.ChangeStatus(idea.ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         slug,
		To:           to,
		PostMutation: lintPostMutationHook(filepath.Join(root, "spec")),
	})
}

// ideaArchiveCommand files an Idea out of active view along the orthogonal
// archived axis: it sets **Archived:** true (plus an optional --note) and
// relocates the file to spec/ideas/archived/<slug>.md, preserving the
// Idea's terminal **Status:**. See spec/features/cli/idea/archive/README.md.
func ideaArchiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <slug>",
		Short: "File an Idea out of active view (sets **Archived:** true and relocates)",
		Long: `Marks spec/ideas/<slug>.md with **Archived:** true and moves it to
spec/ideas/archived/<slug>.md, keeping the Idea's terminal **Status:**
(e.g. Rejected, Stale, Implemented) unchanged — archival is orthogonal to
status. A pre-existing file at the archived path is a collision (exit 1).
On success the verb runs ` + "`specscore spec lint --fix`" + ` to sync the
indexes, prints "<slug>: <active-path> → <archived-path>", and exits 0.

If anything fails after the move (collision, lint failure, I/O error), the
on-disk state is restored to its pre-invocation form (file back at the
active path, original content) before the verb exits.

Examples:

  specscore idea archive foo
  specscore idea archive foo --note "abandoned after the v2 pivot"
`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaArchive,
	}
	cmd.Flags().String("note", "", "optional **Archive Note:** tied to the archive action")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runIdeaArchive(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	note, _ := cmd.Flags().GetString("note")
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	result, err := idea.Archive(idea.ArchiveOptions{
		SpecRoot:     specRoot,
		Slug:         slug,
		Note:         note,
		PostMutation: lintPostMutationHook(filepath.Join(specRoot, "spec")),
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", result.Slug, result.From, result.To)
	return nil
}

// ideaUnarchiveCommand reverses `idea archive`: it clears the **Archived:**
// axis and relocates the file from spec/ideas/archived/<slug>.md back to
// spec/ideas/<slug>.md, preserving the **Status:**.
func ideaUnarchiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unarchive <slug>",
		Short: "Return an archived Idea to active view (clears **Archived:** and relocates)",
		Long: `Removes the **Archived:** (and **Archive Note:**) header lines from
spec/ideas/archived/<slug>.md and moves it back to spec/ideas/<slug>.md,
keeping the Idea's **Status:** unchanged. A pre-existing file at the active
path is a collision (exit 1). On success the verb runs
` + "`specscore spec lint --fix`" + `, prints "<slug>: <archived-path> →
<active-path>", and exits 0. Failures after the move roll back to the
pre-invocation form.

Example:

  specscore idea unarchive foo
`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaUnarchive,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runIdeaUnarchive(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	result, err := idea.Unarchive(idea.UnarchiveOptions{
		SpecRoot:     specRoot,
		Slug:         slug,
		PostMutation: lintPostMutationHook(filepath.Join(specRoot, "spec")),
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", result.Slug, result.From, result.To)
	return nil
}

// lintPostMutationHook returns the standard spec-lint post-mutation hook
// invoked by lifecycle verbs. It first runs lint --fix to sync any
// index rows touched by the status rewrite, then re-runs lint in
// verify mode to surface any remaining error-severity violations.
//
// Any error-severity violation after the fix pass fails the callback — not
// just violations touching the mutated file. The caller's declared mutation
// profile determines recovery: historical Idea/Feature callers compensate;
// transaction-profile Plan callers retain the committed artifact.
//
// Returned error is wrapped via exitcode.UnexpectedErrorf (exit 10) so
// the caller surfaces a uniform exit code regardless of which lint
// path failed.
func lintPostMutationHook(specSub string) idea.PostMutationHook {
	return func() error {
		if _, err := lintLintFn(lint.Options{SpecRoot: specSub, ProjectRoot: filepath.Dir(specSub), Fix: true}); err != nil {
			return exitcode.UnexpectedErrorf("running lint --fix: %v", err)
		}
		return verifyLintPostMutation(specSub)
	}
}

// verifyLintPostMutation performs only the read-only half of the standard
// lifecycle lint hook. Whole-tree transactions call this after explicitly
// updating their declared derived indexes, so no broad lint fixer can expand
// the transaction's write set.
func verifyLintPostMutation(specSub string) error {
	violations, err := lintLintFn(lint.Options{SpecRoot: specSub, ProjectRoot: filepath.Dir(specSub)})
	if err != nil {
		return exitcode.UnexpectedErrorf("running lint: %v", err)
	}
	var errs []lint.Violation
	for _, v := range violations {
		if v.Severity == "error" {
			errs = append(errs, v)
		}
	}
	if len(errs) > 0 {
		var sb strings.Builder
		sb.WriteString("lint failed after status rewrite:\n")
		for _, v := range errs {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return exitcode.UnexpectedError(sb.String())
	}
	return nil
}

// ideaNewCommand scaffolds a lint-clean Idea artifact at spec/ideas/<slug>.md,
// or a Proposal artifact at spec/features/<target>/proposals/<slug>.md when
// --type=change-request is supplied.
func ideaNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Idea or Proposal artifact",
		Long: `Creates a lint-clean Idea skeleton at spec/ideas/<slug>.md.

When --type=change-request --targets=<feature-slug> is supplied, creates the
file at spec/features/<feature-slug>/proposals/<slug>.md with a "# Proposal:"
title prefix and Type/Targets header fields.

Each required section is emitted with an HTML-comment prompt describing
what belongs there. Supply content via flags (--title, --owner, --hmw,
--not-doing) or run with -i to be prompted interactively. The generated
file is always lint-clean — running ` + "`specscore lint`" + ` immediately
afterwards passes.`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaNew,
	}
	cmd.Flags().String("title", "", "Idea title (defaults to title-cased slug)")
	cmd.Flags().String("owner", "", "author identifier (defaults to $USER)")
	cmd.Flags().String("hmw", "", "Problem Statement (How Might We…) sentence")
	cmd.Flags().String("context", "", "Context section content")
	cmd.Flags().String("recommended-direction", "", "Recommended Direction content")
	cmd.Flags().String("mvp", "", "MVP Scope content")
	cmd.Flags().StringArray("not-doing", nil, "Not Doing exclusion (repeatable). Format: `<thing> — <reason>`")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().BoolP("interactive", "i", false, "prompt for each field on stdin")
	cmd.Flags().Bool("force", false, "overwrite an existing idea file at that slug")
	cmd.Flags().String("type", "", "idea type: feature-request (default) or change-request")
	cmd.Flags().String("targets", "", "target feature slug (required when --type=change-request)")
	cmd.Flags().String("phase", "", "optional lifecycle phase to pre-populate")
	return cmd
}

func runIdeaNew(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	title, _ := cmd.Flags().GetString("title")
	owner, _ := cmd.Flags().GetString("owner")
	hmw, _ := cmd.Flags().GetString("hmw")
	ctx, _ := cmd.Flags().GetString("context")
	direction, _ := cmd.Flags().GetString("recommended-direction")
	mvp, _ := cmd.Flags().GetString("mvp")
	notDoing, _ := cmd.Flags().GetStringArray("not-doing")
	projectFlag, _ := cmd.Flags().GetString("project")
	interactive, _ := cmd.Flags().GetBool("interactive")
	force, _ := cmd.Flags().GetBool("force")
	ideaType, _ := cmd.Flags().GetString("type")
	targets, _ := cmd.Flags().GetString("targets")
	phase, _ := cmd.Flags().GetString("phase")

	// Validate --type value.
	isChangeRequest := false
	if ideaType != "" {
		if !idea.ValidIdeaTypes[ideaType] {
			return exitcode.InvalidArgsErrorf("invalid --type %q; valid values: feature-request, change-request", ideaType)
		}
		isChangeRequest = ideaType == "change-request"
	}

	// --targets requires --type=change-request.
	if targets != "" && !isChangeRequest {
		return exitcode.InvalidArgsErrorf("--targets requires --type=change-request")
	}
	// --type=change-request requires --targets.
	if isChangeRequest && targets == "" {
		return exitcode.InvalidArgsErrorf("--type=change-request requires --targets=<feature-slug>")
	}

	if owner == "" {
		if u := os.Getenv("USER"); u != "" {
			owner = u
		}
	}

	opts := idea.ScaffoldOptions{
		Slug:                 slug,
		Title:                title,
		Owner:                owner,
		Type:                 ideaType,
		Targets:              targets,
		Phase:                phase,
		HMW:                  hmw,
		Context:              ctx,
		RecommendedDirection: direction,
		MVP:                  mvp,
		NotDoing:             notDoing,
	}

	if interactive {
		if err := runInteractivePrompts(cmd.InOrStdin(), cmd.OutOrStdout(), &opts); err != nil {
			return err
		}
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	var target string
	if isChangeRequest {
		// Proposal: scaffold at spec/features/<targets>/proposals/<slug>.md.
		featureDir := filepath.Join(specRoot, "spec", "features", targets)
		if _, err := os.Stat(featureDir); os.IsNotExist(err) {
			return exitcode.NotFoundErrorf("target feature directory does not exist: %s", featureDir)
		}
		proposalsDir := filepath.Join(featureDir, "proposals")
		if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
			return exitcode.UnexpectedErrorf("creating %s: %v", proposalsDir, err)
		}
		target = filepath.Join(proposalsDir, slug+".md")
	} else {
		// Standard idea: scaffold at the resolved ideas dir/<slug>.md.
		ideasDir := idea.ResolveIdeasDir(filepath.Join(specRoot, "spec"))
		if err := os.MkdirAll(ideasDir, 0o755); err != nil {
			return exitcode.UnexpectedErrorf("creating %s: %v", ideasDir, err)
		}
		target = filepath.Join(ideasDir, slug+".md")
	}

	if _, err := os.Stat(target); err == nil && !force {
		kind := "idea"
		if isChangeRequest {
			kind = "proposal"
		}
		return exitcode.ConflictErrorf("%s already exists: %s (pass --force to overwrite)", kind, target)
	}

	// cli/idea/new#req:ancestor-indexes-materialized — create the spec/
	// and spec/ideas/ index READMEs before the Idea file, so a fresh
	// project ends up lint-clean for everything except the new Idea.
	// Done BEFORE WriteFile(target) so a failure here cannot leave a
	// half-scaffolded state. Existing files are left untouched.
	if err := ensureIdeaAncestorIndexes(specRoot); err != nil {
		return exitcode.UnexpectedErrorf("materializing ancestor indexes: %v", err)
	}

	// cli-template-runtime-fetch: a *bare* scaffold (no authored content,
	// non-interactive) pulls the published template from the gallery; anything
	// carrying authored content — or an offline fetch — uses the embedded
	// scaffolder (which fills those fields, as the static template cannot).
	bare := !interactive &&
		hmw == "" && ctx == "" && direction == "" && mvp == "" && len(notDoing) == 0

	fetchType := "idea"
	repl := map[string]string{
		"<Idea Name>":   templateTitleOrSlug(title, slug),
		"YYYY-MM-DD":    templateTodayUTC(),
		"<your-handle>": templateOwnerOrUnknown(owner),
	}
	if isChangeRequest {
		fetchType = "proposal"
		repl = map[string]string{
			"<Proposal Name>": templateTitleOrSlug(title, slug),
			"<feature-slug>":  targets,
			"YYYY-MM-DD":      templateTodayUTC(),
			"<your-handle>":   templateOwnerOrUnknown(owner),
		}
	}

	body, err := bareOrEmbedded(bare, fetchType, repl, cmd.ErrOrStderr(), func() ([]byte, error) {
		return ideaScaffoldFn(opts)
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding %s: %v", fetchType, err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
	}

	// Run lint in --fix mode to update the active index, then re-run
	// without fix to surface any remaining errors touching this file.
	specSub := filepath.Join(specRoot, "spec")
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		// Remove the partial file so re-runs don't trip over conflict.
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
		if v.Severity == "error" && (v.File == relTarget || strings.HasPrefix(v.File, "ideas/")) {
			own = append(own, v)
		}
	}
	if len(own) > 0 {
		var sb strings.Builder
		sb.WriteString("generated idea failed lint:\n")
		for _, v := range own {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return exitcode.UnexpectedError(sb.String())
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

// ensureIdeaAncestorIndexes materializes spec/README.md and
// spec/ideas/README.md when they don't already exist, using the same
// templates as `specscore init`. Project metadata is read from
// specscore.yaml when present; on absence we fall back to an empty
// SpecConfig — the resulting index files are lint-clean regardless.
// Existing files are left untouched per
// cli/idea/new#req:ancestor-indexes-materialized.
func ensureIdeaAncestorIndexes(root string) error {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		// Absence (or malformed) → use defaults. Lint will surface any
		// specscore.yaml issues separately; idea new shouldn't fail on
		// them.
		cfg = projectdef.SpecConfig{}
	}
	// The ideas index lives at the resolved ideas dir's README.md, which may
	// be relocated out of spec/ via path_overrides.ideas_path
	// (configurable-ideas-path#req:single-resolver). Compute the repo-relative
	// ideas dir from the root module's config.
	rootMod := cfg.EffectiveModules()[0]
	ideasIndexRel := filepath.Join(
		rootMod.EffectivePath(),
		filepath.FromSlash(rootMod.EffectiveIdeasPath()),
		"README.md",
	)
	for _, w := range []struct {
		path    string
		content string
	}{
		{"spec/README.md", specReadmeContent(cfg)},
		{ideasIndexRel, ideasIndexContent(cfg)},
	} {
		if err := writeMissingIndex(root, w.path, w.content); err != nil {
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
	}
	return nil
}

// resolveSpecRoot resolves the project root (repo root, not spec/ itself)
// from --project or cwd, using the same heuristic as feature commands.
func resolveSpecRoot(projectFlag string) (string, error) {
	var startDir string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		startDir = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return "", exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		startDir = cwd
	}
	return feature.FindSpecRepoRoot(startDir)
}

// runInteractivePrompts fills unset fields in opts by reading line-delimited
// input from r. An empty line keeps the current default.
func runInteractivePrompts(r io.Reader, w io.Writer, opts *idea.ScaffoldOptions) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	type prompt struct {
		label string
		dst   *string
	}
	prompts := []prompt{
		{"Title", &opts.Title},
		{"Owner", &opts.Owner},
		{"Problem Statement (How Might We…)", &opts.HMW},
		{"Context", &opts.Context},
		{"Recommended Direction", &opts.RecommendedDirection},
		{"MVP Scope", &opts.MVP},
	}
	for _, p := range prompts {
		cur := *p.dst
		if cur != "" {
			_, _ = fmt.Fprintf(w, "%s [%s]: ", p.label, cur)
		} else {
			_, _ = fmt.Fprintf(w, "%s: ", p.label)
		}
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			*p.dst = line
		}
	}

	// Not Doing — accept multiple lines until blank.
	if len(opts.NotDoing) == 0 {
		_, _ = fmt.Fprintln(w, "Not Doing exclusions (one per line, blank to finish):")
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				break
			}
			opts.NotDoing = append(opts.NotDoing, line)
		}
	}

	return scanner.Err()
}
