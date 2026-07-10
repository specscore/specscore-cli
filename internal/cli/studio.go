package cli

// Feature implemented: cli/studio/index (REQ: workspace-config,
// REQ: workspace-errors, REQ: rebuild-only, REQ: partial-tolerance,
// REQ: facts-query, REQ: adapter-registries, REQ: ingr-export)

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/studio/adapters"
	"github.com/specscore/specscore-cli/internal/studio/contradictions"
	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/internal/studio/ingr"
	"github.com/specscore/specscore-cli/internal/studio/probe"
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
	// probeRunDomainFn is the test seam for the domain-liveness probe kind; the
	// verb runs it through this var so tests inject deterministic results
	// without exercising the HTTP seam.
	probeRunDomainFn = probe.RunDomain
	// probeRunCIFn is the test seam for the ci-state probe kind; the verb runs
	// it through this var so tests inject deterministic results without
	// exercising the exec (git/gh) seam.
	probeRunCIFn = probe.RunCI
	// storeMergeFn is the test seam for the non-destructive store merge the
	// probe verb writes through.
	storeMergeFn = store.Merge
	// osOpenFn is the test seam for os.Open; the contradictions verb reads the
	// ignore-list file through this var so tests can inject read failures.
	osOpenFn = func(name string) (io.ReadCloser, error) { return os.Open(name) }
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
		studioProbeCommand(),
		studioFactsCommand(),
		studioContradictionsCommand(),
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

// --- studio probe ---

// probeKinds are the accepted --kind selectors: the domain-liveness kind, the
// ci-state kind, or all of them.
const (
	kindDomain = "domain"
	kindCI     = "ci"
	kindAll    = "all"
)

func studioProbeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Run live checks and merge verified-behavior facts into the store",
		Long: `Reads probe targets from the fact store built by "studio index" and runs
live checks, merging the resulting verified-behavior facts back into the same
store without rebuilding it. Unlike index, probe performs network I/O; it never
runs as a side effect of index.

The domain kind issues an HTTP(S) GET to every domain carrying a serves-status
fact (https first, http on transport failure) and records the answering status
— or the reserved object "down" when neither scheme responds. A dead domain is
a successful observation, not a run failure.

The ci kind reads each workspace repo's latest default-branch CI run conclusion
via "gh api". A repo with no origin remote or a non-GitHub remote is skipped
with a per-repo notice; a repo whose gh api call fails records a per-repo
warning and no fact. If "gh" is not on PATH the ci kind is skipped entirely
with one warning (the domain kind still runs under --kind all).

--kind <domain|ci|all> selects which probe families run (default all).
--format <human|json> controls the run summary; the JSON summary is an object
with kinds, facts_written, verified_refreshed and warnings.

Running probe before "studio index" exits 2 with a message naming the expected
store path and suggesting "specscore studio index".

Re-verification cadences (guidance for scheduling this verb / index re-runs,
not in-process timers) by evidence class:

  verified-behavior  hours to a day (schedule probe via CI cron)
  derived            on push / on "studio index" re-run
  declared           on repo change / on "studio index" re-run
  claimed            never — rendered as decaying (no producer this phase)
  attested           quarterly (Phase 2)`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runStudioProbe,
	}
	cmd.Flags().String("workspace", "./studio.yaml", "path to the studio.yaml workspace file")
	cmd.Flags().String("db", "", "fact store path (default <workspace-dir>/.specscore-studio/facts.db)")
	cmd.Flags().String("kind", kindAll, "probe families to run: domain, ci or all")
	cmd.Flags().String("format", "human", "run-summary format: human or json")
	return cmd
}

// probeSummary is the JSON shape of the probe run summary (REQ: probe-verb):
// the kinds that ran, how many facts were written (fresh observations) and
// refreshed (re-verifications), and any per-target/per-kind warnings.
type probeSummary struct {
	Kinds           []string `json:"kinds"`
	FactsWritten    int      `json:"facts_written"`
	VerifiedRefresh int      `json:"verified_refreshed"`
	Warnings        []string `json:"warnings"`
}

func runStudioProbe(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "human" && format != "json" {
		return exitcode.InvalidArgsErrorf("unknown --format %q: expected human or json", format)
	}
	kind, _ := cmd.Flags().GetString("kind")
	if kind != kindDomain && kind != kindCI && kind != kindAll {
		return exitcode.InvalidArgsErrorf("unknown --kind %q: expected domain, ci or all", kind)
	}

	dbPath, err := studioFactsStorePath(cmd)
	if err != nil {
		return err
	}

	// Query serves as the missing/empty-store guard: it returns the same exit-2
	// guidance `studio facts` gives when `studio index` has never run.
	existing, err := store.Query(dbPath, store.Filter{})
	if err != nil {
		return err
	}

	now := studioNowFn()
	var kinds []string
	var produced []fact.Fact
	var warnings []string
	if kind == kindDomain || kind == kindAll {
		res := probeRunDomainFn(existing, version, now)
		kinds = append(kinds, res.Kinds...)
		produced = append(produced, res.Facts...)
		warnings = append(warnings, res.Warnings...)
	}
	if kind == kindCI || kind == kindAll {
		repos, err := probeCITargets(cmd)
		if err != nil {
			return err
		}
		res := probeRunCIFn(repos, version, now)
		kinds = append(kinds, res.Kinds...)
		produced = append(produced, res.Facts...)
		warnings = append(warnings, res.Warnings...)
	}

	merged, err := storeMergeFn(dbPath, produced)
	if err != nil {
		return err
	}

	summary := probeSummary{
		Kinds:           kinds,
		FactsWritten:    merged.Written,
		VerifiedRefresh: merged.Refreshed,
		Warnings:        warnings,
	}
	return renderProbeSummary(cmd, format, summary)
}

// probeCITargets resolves the ci kind's targets: the workspace's repos, in the
// same order the index run resolved them (ResolveRepos plus the missing literal
// paths appended), with their slugs re-minted via a fresh fact.RepoSlugger so
// the emitted ci-status fact subjects join the store's existing repo slugs (the
// slug-join invariant: identical order in, identical `-N` collision suffixes
// out). The ci kind runs `git remote get-url origin` per repo, so a repo dir
// need not exist here — a missing dir simply fails the git call and is skipped
// as a non-GitHub repo with a per-repo notice (REQ: ci-state).
func probeCITargets(cmd *cobra.Command) ([]probe.CIRepo, error) {
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	ws, err := workspace.Load(workspaceFlag)
	if err != nil {
		return nil, err
	}
	repos, missing, err := ws.ResolveRepos()
	if err != nil {
		return nil, err
	}
	// Mirror the index run's ordering: existing repos then missing literals,
	// slugged in that exact order so the collision counter matches.
	repos = append(repos, missing...)
	slugger := fact.NewRepoSlugger()
	targets := make([]probe.CIRepo, 0, len(repos))
	for _, dir := range repos {
		targets = append(targets, probe.CIRepo{
			Dir:       dir,
			Slug:      slugger.Slug(dir),
			Ecosystem: ws.Name,
		})
	}
	return targets, nil
}

// renderProbeSummary prints the probe run summary as JSON or free-form human
// prose over the same data (REQ: probe-verb).
func renderProbeSummary(cmd *cobra.Command, format string, s probeSummary) error {
	w := cmd.OutOrStdout()
	if format == "json" {
		if s.Kinds == nil {
			s.Kinds = []string{}
		}
		if s.Warnings == nil {
			s.Warnings = []string{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	kinds := "none"
	if len(s.Kinds) > 0 {
		kinds = strings.Join(s.Kinds, ", ")
	}
	_, _ = fmt.Fprintf(w, "Probe kinds: %s\n", kinds)
	_, _ = fmt.Fprintf(w, "Facts written: %d\n", s.FactsWritten)
	_, _ = fmt.Fprintf(w, "Verified refreshed: %d\n", s.VerifiedRefresh)
	_, _ = fmt.Fprintf(w, "Warnings: %d\n", len(s.Warnings))
	for _, warn := range s.Warnings {
		_, _ = fmt.Fprintf(w, "  %s\n", warn)
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

// --- studio contradictions ---
//
// Feature: cli/studio/answers (REQ: contradictions-verb,
// REQ: status-vs-behavior-drift, REQ: same-predicate-disagreement,
// REQ: contradiction-facts, REQ: suppression-ignore-list)

func studioContradictionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contradictions",
		Short: "Report contradictions between facts in the store (offline)",
		Long: `Reads the fact store built by "studio index" (enriched by "studio probe")
and reports contradictions between facts. It performs no network I/O — it is a
pure query over the store.

Two detector classes are computed:

  status-drift    a declared claim a verified-behavior observation contradicts:
                  a shipped-implying has-status (Approved, Stable, Implementing)
                  whose feature has a failing has-verification-status, or a
                  declared serves-status (e.g. 200) a live probe disagrees with
                  (e.g. down).
  naming-conflict two declared facts asserting different objects for the same
                  subject and predicate from different evidence pointers.

Each reported item carries both evidence sets — for each side the subject,
predicate, object, evidence class, evidence pointer, adapter id, and
observed_at — plus the detector id that flagged it. A store with no
contradictions exits 0 and prints an empty list (JSON []).

Each detected item is also written back as a "contradicts" fact in the store
(adapter id "contradictions", class "derived") so "studio facts --predicate
contradicts" can query the conflict set. Re-running is idempotent — the merge
key advances verified_at without duplicating. Use --no-write to skip the
write-back.

A workspace ignore-list file at <workspace-dir>/.specscore-studio/contradictions-ignore.txt
suppresses accepted contradictions: one canonical "<side-a>  <side-b>" identity
per line (two spaces between refs), with "#"-prefixed comments and blank lines
ignored. Suppressed items are omitted from both the output and the write-back.
Override the file path with --ignore-file. Use --show-ignored to list what was
suppressed.

--format <human|json> selects the output format (default human).

Running before "studio index" exits 2 with a message naming the expected store
path and suggesting "specscore studio index".`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runStudioContradictions,
	}
	cmd.Flags().String("workspace", "./studio.yaml", "path to the studio.yaml workspace file")
	cmd.Flags().String("db", "", "fact store path (default <workspace-dir>/.specscore-studio/facts.db)")
	cmd.Flags().String("format", "human", "output format: human or json")
	cmd.Flags().Bool("no-write", false, "compute and print items without writing contradicts facts to the store")
	cmd.Flags().String("ignore-file", "", "path to the ignore-list file (default <workspace-dir>/.specscore-studio/contradictions-ignore.txt)")
	cmd.Flags().Bool("show-ignored", false, "list suppressed items alongside detected items (marked as ignored)")
	return cmd
}

// contradictionSide is one side of a contradiction in the verb's output: the
// fact's triple plus the provenance fields the Feature's item shape requires
// (REQ: contradictions-verb).
type contradictionSide struct {
	Subject    string `json:"subject"`
	Predicate  string `json:"predicate"`
	Object     string `json:"object"`
	Class      string `json:"evidence_class"`
	Pointer    string `json:"evidence_pointer"`
	Adapter    string `json:"adapter"`
	ObservedAt string `json:"observed_at"`
}

// contradictionItem is the JSON/human shape of one detected contradiction: the
// detector id plus both evidence sets (REQ: contradictions-verb). For items
// listed under --show-ignored the suppression fields are populated
// (REQ: suppression-ignore-list): Ignored marks the item as suppressed,
// Identity carries the canonical "<side-a>  <side-b>" pair the ignore file
// uses, and IgnoreReason the inline comment from the matching line, if any.
type contradictionItem struct {
	Detector string            `json:"detector"`
	A        contradictionSide `json:"a"`
	B        contradictionSide `json:"b"`
	Ignored  bool              `json:"ignored,omitempty"`
	// Identity is the canonical "<side-a-ref>  <side-b-ref>" suppression
	// identity, present only on ignored items surfaced by --show-ignored.
	Identity string `json:"identity,omitempty"`
	// IgnoreReason is the reason comment on the matching ignore-file line,
	// present only on ignored items whose line carried an inline "#" comment.
	IgnoreReason string `json:"ignore_reason,omitempty"`
}

func runStudioContradictions(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "human" && format != "json" {
		return exitcode.InvalidArgsErrorf("unknown --format %q: expected human or json", format)
	}

	dbPath, err := studioFactsStorePath(cmd)
	if err != nil {
		return err
	}

	// Query serves as the missing/empty-store guard: it returns the same exit-2
	// guidance `studio facts` gives when `studio index` has never run
	// (REQ: contradictions-verb; AC: contradictions-without-index-errors).
	facts, err := store.Query(dbPath, store.Filter{})
	if err != nil {
		return err
	}

	detected := contradictions.Detect(facts)

	// Resolve the ignore-list file path (REQ: suppression-ignore-list).
	// --ignore-file overrides the workspace-relative default.
	ignoreFilePath, _ := cmd.Flags().GetString("ignore-file")
	ignoreExplicit := cmd.Flags().Changed("ignore-file")
	if ignoreFilePath == "" {
		// Default: <workspace-dir>/.specscore-studio/contradictions-ignore.txt.
		studioDir := filepath.Dir(dbPath)
		ignoreFilePath = filepath.Join(studioDir, "contradictions-ignore.txt")
	}

	active, suppressed, err := applyIgnoreList(detected, ignoreFilePath, ignoreExplicit)
	if err != nil {
		return err
	}

	// Write-back: merge the active (non-suppressed) detected items into the
	// store as `contradicts` facts (REQ: contradiction-facts). Suppressed items
	// are excluded from write-back per REQ: suppression-ignore-list.
	noWrite, _ := cmd.Flags().GetBool("no-write")
	if !noWrite && len(active) > 0 {
		now := studioNowFn()
		contraFacts := contradictions.ToFacts(active, now.UTC().Format(time.RFC3339), version)
		if _, err := storeMergeFn(dbPath, contraFacts); err != nil {
			return fmt.Errorf("writing contradicts facts: %w", err)
		}
	}

	showIgnored, _ := cmd.Flags().GetBool("show-ignored")

	// Build output items from active detections.
	items := make([]contradictionItem, 0, len(active))
	for _, it := range active {
		items = append(items, contradictionItem{
			Detector: string(it.Detector),
			A:        toSide(it.A),
			B:        toSide(it.B),
		})
	}

	// If --show-ignored, append suppressed items marked with a special detector
	// suffix plus their canonical suppression identity and the reason comment
	// from the matching ignore-file line (REQ: suppression-ignore-list).
	if showIgnored {
		for _, s := range suppressed {
			items = append(items, contradictionItem{
				Detector:     string(s.Item.Detector) + " [ignored]",
				A:            toSide(s.Item.A),
				B:            toSide(s.Item.B),
				Ignored:      true,
				Identity:     s.Identity,
				IgnoreReason: s.Reason,
			})
		}
	}

	return renderContradictions(cmd, format, items)
}

// applyIgnoreList reads the ignore file at path (if it exists) and partitions
// detected items into active items and suppressed results (each suppressed
// result carrying its canonical identity and the matching line's reason
// comment). A missing file at a default path is silently treated as empty
// (no suppression); a missing file at an explicitly requested path
// (explicit=true) exits 2 naming the path. Any other read error exits 2.
func applyIgnoreList(detected []contradictions.Item, ignoreFilePath string, explicit bool) (active []contradictions.Item, suppressed []contradictions.Suppressed, err error) {
	f, openErr := osOpenFn(ignoreFilePath)
	if openErr != nil {
		if os.IsNotExist(openErr) && !explicit {
			// Missing default ignore file → no suppression (normal state).
			return detected, nil, nil
		}
		return nil, nil, exitcode.InvalidArgsErrorf("reading ignore file %s: %v", ignoreFilePath, openErr)
	}
	defer func() { _ = f.Close() }()
	a, s := contradictions.FilterIgnored(detected, f)
	return a, s, nil
}

// toSide projects a fact onto the verb's side shape.
func toSide(f fact.Fact) contradictionSide {
	return contradictionSide{
		Subject:    f.Subject,
		Predicate:  f.Predicate,
		Object:     f.Object,
		Class:      string(f.Class),
		Pointer:    f.Pointer,
		Adapter:    f.Adapter.ID,
		ObservedAt: f.ObservedAt,
	}
}

// renderContradictions prints the detected items as a JSON array or a human
// list. A clean store prints an empty JSON array / a "No contradictions" line
// (REQ: contradictions-verb).
func renderContradictions(cmd *cobra.Command, format string, items []contradictionItem) error {
	w := cmd.OutOrStdout()
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "No contradictions found.")
		return nil
	}
	_, _ = fmt.Fprintf(w, "Contradictions: %d\n", len(items))
	for _, it := range items {
		_, _ = fmt.Fprintf(w, "\n[%s]\n", it.Detector)
		if it.Ignored {
			// Suppressed items surfaced by --show-ignored list the canonical
			// suppression identity (the exact "<side-a>  <side-b>" ignore-file
			// form) plus the reason comment, so suppression is never silent
			// (REQ: suppression-ignore-list).
			_, _ = fmt.Fprintf(w, "  suppressed: %s\n", it.Identity)
			if it.IgnoreReason != "" {
				_, _ = fmt.Fprintf(w, "  reason: %s\n", it.IgnoreReason)
			}
		}
		printContradictionSide(w, "a", it.A)
		printContradictionSide(w, "b", it.B)
	}
	return nil
}

// printContradictionSide prints one side's triple and provenance under a label.
func printContradictionSide(w io.Writer, label string, s contradictionSide) {
	_, _ = fmt.Fprintf(w, "  %s: (%s, %s, %s)\n", label, s.Subject, s.Predicate, s.Object)
	_, _ = fmt.Fprintf(w, "     class=%s pointer=%s adapter=%s observed_at=%s\n",
		s.Class, s.Pointer, s.Adapter, s.ObservedAt)
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
