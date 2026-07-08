package cli

// Features implemented: cli/graph, cli/graph/new, cli/graph/validation,
// cli/graph/navigation

import (
	"fmt"
	"io"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/graph"
	"github.com/spf13/cobra"
)

// graphCommand returns the "graph" command group for GraphSpec tooling.
func graphCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Author, validate, and navigate GraphSpec artifacts",
		Long: `The graph command group provides tooling around GraphSpec artifacts:
scaffolding (new), validation (lint), and navigation (list, refs).

Running "specscore graph" with no subcommand prints this help and exits 0.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		graphNewCommand(),
		graphLintCommand(),
		graphListCommand(),
		graphRefsCommand(),
	)
	return cmd
}

// resolveGraphRepoRoot resolves the repository root (dir containing
// specscore.yaml) from --project or the working directory.
func resolveGraphRepoRoot(projectFlag string) (string, error) {
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
	return findRepoConfigRoot(startDir)
}

// --- graph new ---

var retiredKindTokens = map[string]bool{"value-object": true, "enum": true}

func isKindToken(tok string) bool {
	for _, k := range graph.Kinds {
		if k == tok {
			return true
		}
	}
	return false
}

func graphNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <kind>",
		Short: "Scaffold a GraphSpec artifact (module|entity|relationship|command|event)",
		Long: `Scaffolds a GraphSpec artifact. <kind> is one of module, entity,
relationship, command, or event. Value objects and enums are ModelSpec
concepts and are not scaffolded here.`,
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE:         runGraphNew,
	}
	cmd.Flags().String("name", "", "display name (required)")
	cmd.Flags().String("id", "", "explicit bare local id (derived from --name when omitted)")
	cmd.Flags().String("module", "", "owning module id (required for non-module kinds)")
	cmd.Flags().String("root", "", "graph root relative to the repo (default spec/graph)")
	cmd.Flags().String("status", "draft", "artifact status")
	cmd.Flags().String("summary", "", "one-line summary")
	cmd.Flags().String("from", "", "relationship source endpoint (qualified ref)")
	cmd.Flags().String("to", "", "relationship target endpoint (qualified ref)")
	cmd.Flags().String("subject", "", "command/event subject (qualified ref)")
	cmd.Flags().Bool("bare", false, "module: skip collection directory scaffolding")
	cmd.Flags().String("project", "", "repository root (autodetected from CWD when omitted)")
	return cmd
}

func runGraphNew(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		_ = cmd.Help()
		return exitcode.InvalidArgsError("missing <kind>; expected one of module, entity, relationship, command, event")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf("`graph new` accepts exactly one <kind>, got %d", len(args))
	}
	kind := args[0]
	if retiredKindTokens[kind] {
		return exitcode.InvalidArgsErrorf(
			"`graph new %s` is not a GraphSpec kind — value objects and enums are ModelSpec concepts; model them in <module>/models/*.hcl", kind)
	}
	if !isKindToken(kind) {
		return exitcode.InvalidArgsErrorf("unknown kind %q; expected one of module, entity, relationship, command, event", kind)
	}

	name, _ := cmd.Flags().GetString("name")
	id, _ := cmd.Flags().GetString("id")
	if name == "" && id == "" {
		return exitcode.InvalidArgsError("--name is required")
	}
	projectFlag, _ := cmd.Flags().GetString("project")
	repoRoot, err := resolveGraphRepoRoot(projectFlag)
	if err != nil {
		return err
	}

	module, _ := cmd.Flags().GetString("module")
	root, _ := cmd.Flags().GetString("root")
	status, _ := cmd.Flags().GetString("status")
	summary, _ := cmd.Flags().GetString("summary")
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	subject, _ := cmd.Flags().GetString("subject")
	bare, _ := cmd.Flags().GetBool("bare")

	res, err := graph.Scaffold(graph.ScaffoldOptions{
		Kind:     kind,
		Name:     name,
		ID:       id,
		Module:   module,
		RepoRoot: repoRoot,
		Root:     root,
		Status:   status,
		Summary:  summary,
		From:     from,
		To:       to,
		Subject:  subject,
		Bare:     bare,
	})
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "Created %s\n", res.Target)
	for _, f := range res.Created {
		_, _ = fmt.Fprintf(w, "  %s\n", f)
	}
	return nil
}

// --- graph lint ---

func graphLintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Validate GraphSpec structure, references, ownership, and dependencies",
		Long: `Validates the repository's GraphSpec roots (the repo-level spec/graph plus
per-module graph roots) against the graph-* rule set. Exits 1 when violations
at or above --severity are found, 0 when clean or when no graph root exists.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runGraphLint,
	}
	cmd.Flags().String("project", "", "repository root (autodetected from CWD when omitted)")
	cmd.Flags().String("root", "", "graph root relative to the repo (default spec/graph)")
	cmd.Flags().String("rules", "", "enable only specified rules (comma-separated)")
	cmd.Flags().String("ignore", "", "disable specified rules (comma-separated)")
	cmd.Flags().String("severity", "error", "minimum severity: error, warning, info")
	cmd.Flags().String("format", "text", "output format: text, json, yaml")
	cmd.Flags().Bool("fix", false, "apply graph autofixes on disk (currently: rewrite legacy modelspec://x.Y references to modelspec:///x.Y); other violations still report")
	return cmd
}

func runGraphLint(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	root, _ := cmd.Flags().GetString("root")
	rulesStr, _ := cmd.Flags().GetString("rules")
	ignoreStr, _ := cmd.Flags().GetString("ignore")
	severity, _ := cmd.Flags().GetString("severity")
	format, _ := cmd.Flags().GetString("format")
	fix, _ := cmd.Flags().GetBool("fix")

	if rulesStr != "" && ignoreStr != "" {
		return exitcode.InvalidArgsError("--rules and --ignore are mutually exclusive")
	}
	if severity != "error" && severity != "warning" && severity != "info" {
		return exitcode.InvalidArgsErrorf("invalid severity level: %s", severity)
	}
	if format != "text" && format != "json" && format != "yaml" {
		return exitcode.InvalidArgsErrorf("invalid format: %s", format)
	}
	rules := splitList(rulesStr)
	ignore := splitList(ignoreStr)
	if err := graph.ValidateRuleNames(rules); err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}
	if err := graph.ValidateRuleNames(ignore); err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	repoRoot, err := resolveGraphRepoRoot(projectFlag)
	if err != nil {
		return err
	}

	res, err := graph.Lint(graph.LintOptions{
		RepoRoot: repoRoot,
		Root:     root,
		Rules:    rules,
		Ignore:   ignore,
		Severity: severity,
		Fix:      fix,
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("graph lint error: %v", err)
	}

	w := cmd.OutOrStdout()
	if res.NoGraphRoot {
		if format == "text" {
			_, _ = fmt.Fprintln(w, "No graph root found (spec/graph or a configured module graph root); nothing to lint.")
		} else {
			if err := outputLintViolations(w, nil, format); err != nil {
				return exitcode.UnexpectedErrorf("output error: %v", err)
			}
		}
		return nil
	}

	// Under --fix with a structured format, emit a single {fixed, violations}
	// envelope (matching `spec lint --fix`); otherwise report fixed files to
	// stderr and violations to stdout.
	if fix && (format == "json" || format == "yaml") {
		if err := outputLintFixEnvelope(w, res.Fixed, res.Violations, format); err != nil {
			return exitcode.UnexpectedErrorf("output error: %v", err)
		}
	} else {
		if fix && format == "text" && len(res.Fixed) > 0 {
			ew := cmd.ErrOrStderr()
			_, _ = fmt.Fprintf(ew, "Fixed %d file(s):\n", len(res.Fixed))
			for _, f := range res.Fixed {
				_, _ = fmt.Fprintf(ew, "  %s\n", f)
			}
		}
		if err := outputLintViolations(w, res.Violations, format); err != nil {
			return exitcode.UnexpectedErrorf("output error: %v", err)
		}
	}
	if len(res.Violations) > 0 {
		return exitcode.ConflictErrorf("%d violation(s) found", len(res.Violations))
	}
	return nil
}

// splitList splits a comma-separated flag value into trimmed non-empty tokens.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.Split(s, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// --- graph list ---

func graphListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List GraphSpec artifact IDs, one per line, sorted",
		Long: `Lists GraphSpec artifact IDs across the repository's graph roots, sorted.
--kind filters by kind; --module filters by owning module. With --format
yaml|json, each row carries id, kind, name, and path.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runGraphList,
	}
	cmd.Flags().String("project", "", "repository root (autodetected from CWD when omitted)")
	cmd.Flags().String("root", "", "graph root relative to the repo (default spec/graph)")
	cmd.Flags().String("kind", "", "filter by kind (module|entity|relationship|command|event)")
	cmd.Flags().String("module", "", "filter by owning module id")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runGraphList(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	root, _ := cmd.Flags().GetString("root")
	kind, _ := cmd.Flags().GetString("kind")
	module, _ := cmd.Flags().GetString("module")
	format, _ := cmd.Flags().GetString("format")

	if kind != "" && !isKindToken(kind) {
		return exitcode.InvalidArgsErrorf("invalid --kind %q; expected one of module, entity, relationship, command, event", kind)
	}
	if format != "text" && format != "json" && format != "yaml" {
		return exitcode.InvalidArgsErrorf("invalid format: %s", format)
	}

	repoRoot, err := resolveGraphRepoRoot(projectFlag)
	if err != nil {
		return err
	}
	g, ok, err := graph.Load(repoRoot, root)
	if err != nil {
		return exitcode.UnexpectedErrorf("loading graph: %v", err)
	}
	var items []graph.ListItem
	if ok {
		items = graph.List(g, kind, module)
	}

	w := cmd.OutOrStdout()
	switch format {
	case "yaml":
		return writeGraphYAML(w, items)
	case "json":
		return writeGraphJSON(w, items)
	default:
		for _, it := range items {
			_, _ = fmt.Fprintln(w, it.ID)
		}
		return nil
	}
}

// --- graph refs ---

func graphRefsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refs <ref>",
		Short: "Show artifacts that reference the target (inbound)",
		Long: `Reports every artifact that references <ref>. <ref> resolves by qualified id
first, then by shorthand (bare id or display name); an ambiguous shorthand
exits 5 with the candidate ids. --transitive follows reference chains,
cycle-safe.`,
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE:         runGraphRefs,
	}
	cmd.Flags().String("project", "", "repository root (autodetected from CWD when omitted)")
	cmd.Flags().String("root", "", "graph root relative to the repo (default spec/graph)")
	cmd.Flags().Bool("transitive", false, "follow reference chains transitively")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runGraphRefs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <ref>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf("`graph refs` accepts exactly one <ref>, got %d", len(args))
	}
	ref := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")
	root, _ := cmd.Flags().GetString("root")
	transitive, _ := cmd.Flags().GetBool("transitive")
	format, _ := cmd.Flags().GetString("format")
	if format != "text" && format != "json" && format != "yaml" {
		return exitcode.InvalidArgsErrorf("invalid format: %s", format)
	}

	repoRoot, err := resolveGraphRepoRoot(projectFlag)
	if err != nil {
		return err
	}
	g, ok, err := graph.Load(repoRoot, root)
	if err != nil {
		return exitcode.UnexpectedErrorf("loading graph: %v", err)
	}
	if !ok {
		return exitcode.NotFoundErrorf("no graph root found; nothing to resolve for %q", ref)
	}

	qid, candidates, found := graph.ResolveRef(g, ref)
	if !found {
		return exitcode.NotFoundErrorf("no artifact matches %q", ref)
	}
	if len(candidates) > 0 {
		return exitcode.Newf(exitcode.AmbiguousSlug,
			"%q is ambiguous; candidates: %s", ref, strings.Join(candidates, ", "))
	}

	items := graph.Refs(g, qid, transitive)

	w := cmd.OutOrStdout()
	switch format {
	case "yaml":
		return writeGraphRefsYAML(w, items)
	case "json":
		return writeGraphRefsJSON(w, items)
	default:
		for _, it := range items {
			_, _ = fmt.Fprintln(w, it.ID)
		}
		return nil
	}
}

// --- structured output helpers ---

func writeGraphYAML(w io.Writer, items []graph.ListItem) error {
	if items == nil {
		items = []graph.ListItem{}
	}
	enc := newYAMLEnc(w)
	if err := enc.Encode(items); err != nil {
		return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
	}
	return enc.Close()
}

func writeGraphJSON(w io.Writer, items []graph.ListItem) error {
	if items == nil {
		items = []graph.ListItem{}
	}
	return newJSONEnc(w).Encode(items)
}

func writeGraphRefsYAML(w io.Writer, items []graph.RefItem) error {
	if items == nil {
		items = []graph.RefItem{}
	}
	enc := newYAMLEnc(w)
	if err := enc.Encode(items); err != nil {
		return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
	}
	return enc.Close()
}

func writeGraphRefsJSON(w io.Writer, items []graph.RefItem) error {
	if items == nil {
		items = []graph.RefItem{}
	}
	return newJSONEnc(w).Encode(items)
}
