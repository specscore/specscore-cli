package cli

// Feature implemented: cli/studio/index (REQ: workspace-config,
// REQ: workspace-errors, REQ: rebuild-only, REQ: partial-tolerance,
// REQ: facts-query, REQ: adapter-registries, REQ: ingr-export)

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/studio/adapters"
	"github.com/specscore/specscore-cli/internal/studio/ingr"
	"github.com/specscore/specscore-cli/internal/studio/store"
	"github.com/specscore/specscore-cli/internal/studio/workspace"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	adaptersRunFn = adapters.Run
	ingrExportFn  = ingr.Export
	// studioNowFn is the clock the facts verb reads for the --stale cutoff and
	// the VERIFIED age column; tests replace it for deterministic ages.
	studioNowFn = time.Now
)

// studioCommand returns the "studio" command group for SpecScore Studio —
// the multi-repo ecosystem fact indexer.
func studioCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "studio",
		Short: "Index and query multi-repo ecosystem facts (SpecScore Studio)",
		Long: `The studio command group federates per-repo artifacts (spec trees,
codegraph snapshots, dependency manifests, ops registries) listed in a
studio.yaml workspace into one queryable, rebuildable fact store.

Running "specscore studio" with no subcommand prints this help and exits 0.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		studioIndexCommand(),
		studioFactsCommand(),
	)
	return cmd
}

// --- studio index ---

func studioIndexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Rebuild the ecosystem fact store from a studio.yaml workspace",
		Long: `Reads the studio.yaml workspace (ecosystem name + repos list of paths or
globs), resolves the repos, and rebuilds the fact store from scratch.

A missing or unparsable workspace file, or a workspace whose repos resolve
to zero existing directories, exits 2 with a one-line actionable error.

Individual broken repos never abort the run: a missing repo path, a
panicking adapter, or a malformed artifact file is skipped at the smallest
possible granularity and reported as a warning in the run summary. The
command exits 0 with warnings by default; --strict makes any warning exit 3
after the run completes and the summary is printed.

Every run also exports the facts as INGR recordsets, one directory per repo
slug under <workspace-dir>/.specscore-studio/ingr/ (override the root with
--ingr-dir; skip the export with --no-ingr). An export failure is a warning,
not a fatal error — the fact store remains the query surface.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runStudioIndex,
	}
	cmd.Flags().String("workspace", "./studio.yaml", "path to the studio.yaml workspace file")
	cmd.Flags().String("db", "", "fact store path (default <workspace-dir>/.specscore-studio/facts.db)")
	cmd.Flags().Bool("strict", false, "exit 3 when the run collected any warning")
	cmd.Flags().String("ingr-dir", "", "INGR export root (default <workspace-dir>/.specscore-studio/ingr)")
	cmd.Flags().Bool("no-ingr", false, "skip the per-repo INGR recordset export")
	return cmd
}

func runStudioIndex(cmd *cobra.Command, _ []string) error {
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	dbFlag, _ := cmd.Flags().GetString("db")

	ws, err := workspace.Load(workspaceFlag)
	if err != nil {
		return err
	}
	repos, missing, err := ws.ResolveRepos()
	if err != nil {
		return err
	}
	// Missing literal repo paths flow through the pipeline so they surface
	// as repo-level warnings + per-repo summary lines (REQ: partial-tolerance).
	repos = append(repos, missing...)

	dbPath := ws.DefaultDBPath()
	if dbFlag != "" {
		abs, err := filepathAbsFn(dbFlag)
		if err != nil {
			return exitcode.InvalidArgsErrorf("resolving --db path %s: %v", dbFlag, err)
		}
		dbPath = abs
	}
	ingrDir := ws.DefaultIngrDir()
	if ingrDirFlag, _ := cmd.Flags().GetString("ingr-dir"); ingrDirFlag != "" {
		abs, err := filepathAbsFn(ingrDirFlag)
		if err != nil {
			return exitcode.InvalidArgsErrorf("resolving --ingr-dir path %s: %v", ingrDirFlag, err)
		}
		ingrDir = abs
	}

	all := adapters.All(adapters.Options{Registries: ws.ResolveRegistries()})
	result := adaptersRunFn(all, repos, ws.Name)
	if err := store.Rebuild(dbPath, result.Facts); err != nil {
		return err
	}

	// INGR export (REQ: ingr-export): one recordset directory per repo slug,
	// each holding exactly the facts attributed to that repo by the run. An
	// export failure is downgraded to a run-level warning — the store stays
	// the query surface; the export is interchange.
	noIngr, _ := cmd.Flags().GetBool("no-ingr")
	if !noIngr {
		exportRepos := make([]ingr.Repo, 0, len(result.Repos))
		for _, r := range result.Repos {
			exportRepos = append(exportRepos, ingr.Repo{Slug: r.Slug, Facts: result.FactsByRepo[r.Slug]})
		}
		if err := ingrExportFn(ingrDir, exportRepos); err != nil {
			result.Warnings = append(result.Warnings, adapters.Warning{
				Message: fmt.Sprintf("INGR export failed: %v — facts remain queryable in the store", err)})
		}
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "Ecosystem: %s\n", ws.Name)
	_, _ = fmt.Fprintf(w, "Workspace: %s\n", ws.Path)
	_, _ = fmt.Fprintf(w, "Repos: %d\n", len(repos))
	for _, r := range result.Repos {
		_, _ = fmt.Fprintf(w, "  %s: %d facts, %d warnings\n", r.Path, r.Facts, r.Warnings)
	}
	_, _ = fmt.Fprintln(w, "Facts by adapter:")
	for _, a := range all {
		_, _ = fmt.Fprintf(w, "  %s: %d\n", a.ID(), result.FactsByAdapter[a.ID()])
	}
	_, _ = fmt.Fprintf(w, "Warnings: %d\n", len(result.Warnings))
	for _, warn := range result.Warnings {
		switch {
		case warn.Repo == "": // run-level warning (e.g. INGR export): no repo to blame
			_, _ = fmt.Fprintf(w, "  %s\n", warn.Message)
		case warn.Adapter == "": // repo-level warning: no adapter to blame
			_, _ = fmt.Fprintf(w, "  %s: %s\n", warn.Repo, warn.Message)
		default:
			_, _ = fmt.Fprintf(w, "  %s [%s]: %s\n", warn.Repo, warn.Adapter, warn.Message)
		}
	}
	_, _ = fmt.Fprintf(w, "Fact store: %s\n", dbPath)
	if noIngr {
		_, _ = fmt.Fprintln(w, "INGR export: disabled (--no-ingr)")
	} else {
		_, _ = fmt.Fprintf(w, "INGR export: %s\n", ingrDir)
	}

	// --strict escalates warnings only after the run completed and the full
	// summary is printed (REQ: partial-tolerance).
	if strict, _ := cmd.Flags().GetBool("strict"); strict && len(result.Warnings) > 0 {
		return exitcode.NotFoundErrorf("studio index collected %d warning(s) — failing because --strict is set", len(result.Warnings))
	}
	return nil
}

// --- studio facts ---

func studioFactsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "facts",
		Short: "Query the ecosystem fact store built by studio index",
		Long: `Filters the fact store by any combination of --subject, --predicate,
--object, --class and --adapter (exact match; --subject and --object also
accept a trailing "*" for prefix match) and prints a table by default,
JSON with --format json, or only the row count with --count.

--stale <duration> (Go duration syntax, e.g. 24h or 720h) selects only facts
whose verified_at is older than now minus the duration; it composes (AND) with
every other filter and with --count. A malformed --stale duration exits 2.

The table output carries a VERIFIED column showing each fact's freshness age
derived from verified_at (fresh < 24h, aging < 30d, else "stale"). JSON output
includes observed_at and verified_at verbatim.

Querying a missing or empty store exits 2 with an actionable message
naming the expected store path and suggesting "specscore studio index".`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runStudioFacts,
	}
	cmd.Flags().String("workspace", "./studio.yaml", "path to the studio.yaml workspace file")
	cmd.Flags().String("db", "", "fact store path (default <workspace-dir>/.specscore-studio/facts.db)")
	cmd.Flags().String("subject", "", "filter by subject (trailing * = prefix match)")
	cmd.Flags().String("predicate", "", "filter by predicate (exact match)")
	cmd.Flags().String("object", "", "filter by object (trailing * = prefix match)")
	cmd.Flags().String("class", "", "filter by evidence class (declared, derived or verified-behavior)")
	cmd.Flags().String("adapter", "", "filter by adapter id (exact match)")
	cmd.Flags().String("stale", "", "select facts whose verified_at is older than a Go duration (e.g. 24h)")
	cmd.Flags().String("format", "table", "output format: table or json")
	cmd.Flags().Bool("count", false, "print only the number of matching facts")
	return cmd
}

func runStudioFacts(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "table" && format != "json" {
		return exitcode.InvalidArgsErrorf("unknown --format %q: expected table or json", format)
	}

	dbPath, err := studioFactsStorePath(cmd)
	if err != nil {
		return err
	}

	now := studioNowFn()
	filter := store.Filter{}
	filter.Subject, _ = cmd.Flags().GetString("subject")
	filter.Predicate, _ = cmd.Flags().GetString("predicate")
	filter.Object, _ = cmd.Flags().GetString("object")
	filter.Class, _ = cmd.Flags().GetString("class")
	filter.Adapter, _ = cmd.Flags().GetString("adapter")
	if staleFlag, _ := cmd.Flags().GetString("stale"); staleFlag != "" {
		d, err := time.ParseDuration(staleFlag)
		if err != nil {
			return exitcode.InvalidArgsErrorf("invalid --stale duration %q: expected Go duration syntax (e.g. 24h)", staleFlag)
		}
		filter.StaleBefore = now.Add(-d)
	}

	facts, err := store.Query(dbPath, filter)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if countFlag, _ := cmd.Flags().GetBool("count"); countFlag {
		_, _ = fmt.Fprintln(w, len(facts))
		return nil
	}
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(facts)
	}
	tw := tabwriter.NewWriter(w, 2, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SUBJECT\tPREDICATE\tOBJECT\tCLASS\tADAPTER\tVERIFIED")
	for _, f := range facts {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			f.Subject, f.Predicate, f.Object, f.Class, f.Adapter.ID,
			humanAge(f.VerifiedAt, now))
	}
	return tw.Flush()
}

// humanAge renders a fact's freshness age from its verified_at timestamp,
// relative to now, for the VERIFIED table column (REQ: age-rendering). The
// thresholds mirror the UX design's freshness dots: fresh (< 24h) renders in
// hours, aging (< 30d) in days, and anything older renders as "stale". An
// empty or unparseable timestamp renders "?".
func humanAge(verifiedAt string, now time.Time) string {
	if verifiedAt == "" {
		return "?"
	}
	t, err := time.Parse(time.RFC3339, verifiedAt)
	if err != nil {
		return "?"
	}
	age := now.Sub(t)
	switch {
	case age < 24*time.Hour:
		h := int(age.Hours())
		if h < 0 {
			h = 0
		}
		return fmt.Sprintf("%dh", h)
	case age < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(age.Hours())/24)
	default:
		return "stale"
	}
}

// studioFactsStorePath resolves the fact-store path for the facts verb:
// --db when given (absolutized), else the workspace's default store path.
// The workspace file is loaded only when --db is not set.
func studioFactsStorePath(cmd *cobra.Command) (string, error) {
	if dbFlag, _ := cmd.Flags().GetString("db"); dbFlag != "" {
		abs, err := filepathAbsFn(dbFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --db path %s: %v", dbFlag, err)
		}
		return abs, nil
	}
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	ws, err := workspace.Load(workspaceFlag)
	if err != nil {
		return "", err
	}
	return ws.DefaultDBPath(), nil
}
