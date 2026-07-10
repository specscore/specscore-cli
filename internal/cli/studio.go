package cli

// Feature implemented: cli/studio/index (REQ: workspace-config,
// REQ: workspace-errors, REQ: facts-query)

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/studio/store"
	"github.com/specscore/specscore-cli/internal/studio/workspace"
	"github.com/specscore/specscore-cli/pkg/exitcode"
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
to zero existing directories, exits 2 with a one-line actionable error.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runStudioIndex,
	}
	cmd.Flags().String("workspace", "./studio.yaml", "path to the studio.yaml workspace file")
	cmd.Flags().String("db", "", "fact store path (default <workspace-dir>/.specscore-studio/facts.db)")
	return cmd
}

func runStudioIndex(cmd *cobra.Command, _ []string) error {
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	dbFlag, _ := cmd.Flags().GetString("db")

	ws, err := workspace.Load(workspaceFlag)
	if err != nil {
		return err
	}
	repos, err := ws.ResolveRepos()
	if err != nil {
		return err
	}

	dbPath := ws.DefaultDBPath()
	if dbFlag != "" {
		abs, err := filepathAbsFn(dbFlag)
		if err != nil {
			return exitcode.InvalidArgsErrorf("resolving --db path %s: %v", dbFlag, err)
		}
		dbPath = abs
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "Ecosystem: %s\n", ws.Name)
	_, _ = fmt.Fprintf(w, "Workspace: %s\n", ws.Path)
	_, _ = fmt.Fprintf(w, "Repos: %d\n", len(repos))
	for _, r := range repos {
		_, _ = fmt.Fprintf(w, "  %s\n", r)
	}
	_, _ = fmt.Fprintf(w, "Fact store: %s\n", dbPath)
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
	cmd.Flags().String("class", "", "filter by evidence class (declared or derived)")
	cmd.Flags().String("adapter", "", "filter by adapter id (exact match)")
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

	filter := store.Filter{}
	filter.Subject, _ = cmd.Flags().GetString("subject")
	filter.Predicate, _ = cmd.Flags().GetString("predicate")
	filter.Object, _ = cmd.Flags().GetString("object")
	filter.Class, _ = cmd.Flags().GetString("class")
	filter.Adapter, _ = cmd.Flags().GetString("adapter")

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
	_, _ = fmt.Fprintln(tw, "SUBJECT\tPREDICATE\tOBJECT\tCLASS\tADAPTER")
	for _, f := range facts {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			f.Subject, f.Predicate, f.Object, f.Class, f.Adapter.ID)
	}
	return tw.Flush()
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
