package cli

// Features implemented: cli/feature

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/dryrun"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// featureCommand returns the "feature" command group.
func featureCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feature",
		Short: "Query features — listing, hierarchy, dependencies, references",
	}
	cmd.AddCommand(
		featureInfoCommand(),
		featureListCommand(),
		featureTreeCommand(),
		featureDepsCommand(),
		featureRefsCommand(),
		featureNewCommand(),
		featureChangeStatusCommand(),
		featureTransitionsCommand(),
		featureParkCommand(),
		featureUnparkCommand(),
	)
	return cmd
}

// featureTransitionsCommand registers `specscore feature transitions
// [<feature_id>]`, the read-only counterpart to change-status: with no
// argument it prints the Feature kind's full bidirectional status matrix;
// with a feature_id it reports that feature's current status and what it
// can legally become next.
func featureTransitionsCommand() *cobra.Command {
	return transitionsCommand(lifecycle.KindFeature, "feature_id", "Show the Feature status matrix, or one feature's legal next statuses",
		func(projectFlag, featureID string) (string, error) {
			featuresDir, err := resolveFeaturesDir(projectFlag)
			if err != nil {
				return "", err
			}
			if !feature.Exists(featuresDir, featureID) {
				return "", exitcode.NotFoundErrorf("feature not found: %s (expected README at %s)", featureID, feature.ReadmePath(featuresDir, featureID))
			}
			return feature.ReadmePath(featuresDir, featureID), nil
		})
}

// resolveFeaturesDir resolves the features directory from a --project flag or CWD.
func resolveFeaturesDir(projectFlag string) (string, error) {
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

	root, err := feature.FindSpecRepoRoot(startDir)
	if err != nil {
		return "", err
	}

	featDir := filepath.Join(root, "spec", "features")
	info, err := os.Stat(featDir)
	if err != nil || !info.IsDir() {
		return "", exitcode.NotFoundErrorf("features directory not found: %s", featDir)
	}
	return featDir, nil
}

// effectiveFormat determines the output format from flags.
// With --fields and no explicit --format, auto-switches to YAML.
func effectiveFormat(cmd *cobra.Command) string {
	format, _ := cmd.Flags().GetString("format")
	if format != "" {
		return format
	}
	fields, _ := cmd.Flags().GetString("fields")
	if fields != "" {
		return "yaml"
	}
	return "text"
}

// validateFormat checks the format flag value is valid.
func validateFormat(fmt string) error {
	if fmt != "text" && fmt != "yaml" && fmt != "json" {
		return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, yaml, json)", fmt)
	}
	return nil
}

// writeEnrichedOutput writes enriched features in the specified format.
func writeEnrichedOutput(w io.Writer, features []*feature.EnrichedFeature, fields []string, format string) error {
	switch format {
	case "yaml":
		return writeEnrichedYAML(w, features)
	case "json":
		return writeEnrichedJSON(w, features)
	default:
		return writeEnrichedText(w, features, fields)
	}
}

func writeEnrichedYAML(w io.Writer, features []*feature.EnrichedFeature) error {
	enc := newYAMLEnc(w)
	if err := enc.Encode(features); err != nil {
		return err
	}
	return enc.Close()
}

func writeEnrichedJSON(w io.Writer, features []*feature.EnrichedFeature) error {
	return newJSONEnc(w).Encode(features)
}

func writeEnrichedText(w io.Writer, features []*feature.EnrichedFeature, fields []string) error {
	bw := bufio.NewWriter(w)
	for _, ef := range features {
		writeEnrichedTextNode(bw, ef, fields, 0)
	}
	return bw.Flush()
}

func writeEnrichedTextNode(w *bufio.Writer, ef *feature.EnrichedFeature, fields []string, depth int) {
	indent := strings.Repeat("\t", depth)
	prefix := ""
	if ef.Focus != nil && *ef.Focus {
		prefix = "* "
	}
	if ef.Cycle != nil && *ef.Cycle {
		_, _ = fmt.Fprintf(w, "%s%s (cycle)\n", indent, ef.Path)
		return
	}

	var meta []string
	for _, f := range fields {
		switch f {
		case "title":
			if ef.Title != "" {
				meta = append(meta, fmt.Sprintf("title=%q", ef.Title))
			}
		case "status":
			if ef.Status != "" {
				meta = append(meta, fmt.Sprintf("status=%s", ef.Status))
			}
		case "oq":
			if ef.OQ != nil {
				meta = append(meta, fmt.Sprintf("oq=%d", *ef.OQ))
			}
		case "questions":
			if len(ef.Questions) > 0 {
				meta = append(meta, fmt.Sprintf("questions=[%s]", strings.Join(ef.Questions, "; ")))
			}
		case "deps":
			if len(ef.Deps) > 0 {
				meta = append(meta, fmt.Sprintf("deps=[%s]", strings.Join(ef.Deps, ",")))
			}
		case "refs":
			if len(ef.Refs) > 0 {
				meta = append(meta, fmt.Sprintf("refs=[%s]", strings.Join(ef.Refs, ",")))
			}
		case "plans":
			if len(ef.Plans) > 0 {
				meta = append(meta, fmt.Sprintf("plans=[%s]", strings.Join(ef.Plans, ",")))
			}
		case "proposals":
			if len(ef.Proposals) > 0 {
				meta = append(meta, fmt.Sprintf("proposals=[%s]", strings.Join(ef.Proposals, ",")))
			}
		case "parked":
			if ef.Parked != nil && *ef.Parked {
				meta = append(meta, "parked=true")
			}
		}
	}

	suffix := ""
	if len(meta) > 0 {
		suffix = " " + strings.Join(meta, " ")
	}
	_, _ = fmt.Fprintf(w, "%s%s%s%s\n", indent, prefix, ef.Path, suffix)

	for _, child := range ef.ChildNodes {
		writeEnrichedTextNode(w, child, fields, depth+1)
	}
}

// printTransitiveText writes transitive results as indented text.
func printTransitiveText(sb *strings.Builder, nodes []*feature.EnrichedFeature, depth int) {
	for _, node := range nodes {
		for i := 0; i < depth; i++ {
			sb.WriteByte('\t')
		}
		sb.WriteString(node.Path)
		if node.Cycle != nil && *node.Cycle {
			sb.WriteString(" (cycle)")
		}
		sb.WriteByte('\n')
		if len(node.ChildNodes) > 0 {
			printTransitiveText(sb, node.ChildNodes, depth+1)
		}
	}
}

// --- feature info ---

func featureInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <feature_id>",
		Short: "Show feature metadata, section TOC, and children",
		Long: `Returns structured metadata and a section table-of-contents with line
ranges for a feature's README.md. Default output is YAML; use --format for JSON or text.`,
		Args: cobra.ExactArgs(1),
		RunE: runFeatureInfo,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, text")
	return cmd
}

func runFeatureInfo(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")
	formatFlag, _ := cmd.Flags().GetString("format")

	if formatFlag != "yaml" && formatFlag != "json" && formatFlag != "text" {
		return exitcode.InvalidArgsErrorf("invalid format: %s (supported: yaml, json, text)", formatFlag)
	}

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}

	if !feature.Exists(featuresDir, featureID) {
		return exitcode.NotFoundErrorf("feature not found: %s", featureID)
	}

	info, err := feature.GetInfo(featuresDir, featureID)
	if err != nil {
		return exitcode.UnexpectedErrorf("getting feature info: %v", err)
	}

	return writeFeatureInfo(cmd.OutOrStdout(), formatFlag, info)
}

func writeFeatureInfo(w io.Writer, format string, info *feature.Info) error {
	switch format {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(info); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	case "json":
		return newJSONEnc(w).Encode(info)
	case "text":
		return writeTextInfo(w, info)
	}
	return nil
}

func writeTextInfo(w io.Writer, info *feature.Info) error {
	bw := bufio.NewWriter(w)

	_, _ = fmt.Fprintf(bw, "Feature: %s\n", info.Path)
	_, _ = fmt.Fprintf(bw, "Status:  %s\n", info.Status)

	if len(info.Deps) > 0 {
		_, _ = fmt.Fprintf(bw, "Deps:    %s\n", strings.Join(info.Deps, ", "))
	} else {
		_, _ = fmt.Fprintln(bw, "Deps:    (none)")
	}

	if len(info.Refs) > 0 {
		_, _ = fmt.Fprintf(bw, "Refs:    %s\n", strings.Join(info.Refs, ", "))
	} else {
		_, _ = fmt.Fprintln(bw, "Refs:    (none)")
	}

	if len(info.Children) > 0 {
		_, _ = fmt.Fprintln(bw, "\nChildren:")
		for _, c := range info.Children {
			marker := "✓"
			if !c.InReadme {
				marker = "✗"
			}
			_, _ = fmt.Fprintf(bw, "  %s %s (in_readme: %v)\n", marker, c.Path, c.InReadme)
		}
	}

	if len(info.Plans) > 0 {
		_, _ = fmt.Fprintf(bw, "\nPlans:   %s\n", strings.Join(info.Plans, ", "))
	}

	if len(info.Sections) > 0 {
		_, _ = fmt.Fprintln(bw, "\nSections:")
		printTextSections(bw, info.Sections, 1)
	}

	return bw.Flush()
}

func printTextSections(w *bufio.Writer, sections []feature.SectionInfo, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, s := range sections {
		itemsSuffix := ""
		if s.Items > 0 {
			itemsSuffix = fmt.Sprintf(" (%d items)", s.Items)
		}
		_, _ = fmt.Fprintf(w, "%s%s [%s]%s\n", indent, s.Title, s.Lines, itemsSuffix)
		if len(s.Children) > 0 {
			printTextSections(w, s.Children, depth+1)
		}
	}
}

// --- feature list ---

func featureListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all feature IDs, one per line",
		Long: `Lists all features in a project as full feature IDs, one per line,
sorted alphabetically. Use --fields to include metadata for each feature.
Use --parked to show only features carrying the **Parked:** true axis
(deliberately deferred, independent of their **Status:**).`,
		Args: cobra.NoArgs,
		RunE: runFeatureList,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("fields", "", "comma-separated metadata fields to include (e.g., status,oq)")
	cmd.Flags().String("format", "", "output format: yaml, json, text (auto-selects yaml when --fields is set)")
	cmd.Flags().Bool("parked", false, "show only parked features (excludes unparked features)")
	return cmd
}

func runFeatureList(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	fieldsFlag, _ := cmd.Flags().GetString("fields")
	parkedOnly, _ := cmd.Flags().GetBool("parked")

	fields, err := feature.ParseFieldNames(fieldsFlag)
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	format := effectiveFormat(cmd)
	if err := validateFormat(format); err != nil {
		return err
	}

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}

	features, err := feature.Discover(featuresDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering features: %v", err)
	}
	featureIDs := feature.FeatureIDs(features)

	if parkedOnly {
		var filtered []string
		for _, id := range featureIDs {
			info, err := lifecycle.ReadParked(feature.ReadmePath(featuresDir, id))
			if err == nil && info.Parked {
				filtered = append(filtered, id)
			}
		}
		featureIDs = filtered
	}

	w := cmd.OutOrStdout()

	if len(fields) > 0 || format == "yaml" || format == "json" {
		var enriched []*feature.EnrichedFeature
		for _, id := range featureIDs {
			ef, _ := feature.ResolveFields(featuresDir, id, fields)
			enriched = append(enriched, ef)
		}
		return writeEnrichedOutput(w, enriched, fields, format)
	}

	for _, id := range featureIDs {
		_, _ = fmt.Fprintln(w, id)
	}
	return nil
}

// --- feature tree ---

func featureTreeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree [feature_id]",
		Short: "Display feature hierarchy as an indented tree",
		Long: `Displays the feature hierarchy as an indented tree. Without a feature ID,
shows the full project tree. With a feature ID, shows the feature in context —
ancestors (path to root) plus its subtree by default. Use --direction to narrow
to ancestors only (up) or subtree only (down).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runFeatureTree,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("direction", "", "up (ancestors only) or down (subtree only); requires feature_id")
	cmd.Flags().String("fields", "", "comma-separated metadata fields to include (e.g., status,oq)")
	cmd.Flags().String("format", "", "output format: yaml, json, text (auto-selects yaml when --fields is set)")
	return cmd
}

func runFeatureTree(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	directionFlag, _ := cmd.Flags().GetString("direction")
	fieldsFlag, _ := cmd.Flags().GetString("fields")

	if directionFlag != "" && directionFlag != "up" && directionFlag != "down" {
		return exitcode.InvalidArgsErrorf("invalid --direction: %s (valid: up, down)", directionFlag)
	}
	if directionFlag != "" && len(args) == 0 {
		return exitcode.InvalidArgsError("--direction requires a feature_id argument")
	}

	fields, err := feature.ParseFieldNames(fieldsFlag)
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	format := effectiveFormat(cmd)
	if err := validateFormat(format); err != nil {
		return err
	}

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}

	features, err := feature.Discover(featuresDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering features: %v", err)
	}
	featureIDs := feature.FeatureIDs(features)

	w := cmd.OutOrStdout()

	var targetID string
	if len(args) > 0 {
		targetID = args[0]
		if !feature.Exists(featuresDir, targetID) {
			return exitcode.NotFoundErrorf("feature not found: %s", targetID)
		}
	}

	if len(fields) > 0 || format == "yaml" || format == "json" {
		var filtered []string
		if targetID == "" {
			filtered = featureIDs
		} else {
			filtered = feature.FilterFocusedFeatures(featureIDs, targetID, directionFlag)
		}
		roots := feature.BuildEnrichedTree(featuresDir, filtered, fields, targetID)
		switch format {
		case "yaml":
			return writeEnrichedYAML(w, roots)
		case "json":
			return writeEnrichedJSON(w, roots)
		default:
			return writeEnrichedText(w, roots, fields)
		}
	}

	if targetID == "" {
		roots := feature.BuildTree(featureIDs)
		var sb strings.Builder
		feature.PrintTree(&sb, roots, 0)
		_, _ = fmt.Fprint(w, sb.String())
		return nil
	}

	filtered := feature.FilterFocusedFeatures(featureIDs, targetID, directionFlag)
	roots := feature.BuildTree(filtered)
	feature.MarkFocus(roots, targetID)
	var sb strings.Builder
	feature.PrintTree(&sb, roots, 0)
	_, _ = fmt.Fprint(w, sb.String())
	return nil
}

// --- feature deps ---

func featureDepsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deps <feature_id>",
		Short: "Show features that a given feature depends on",
		Long: `Shows the features that a given feature depends on. Dependencies are
read from the ## Dependencies section in the feature's README.md. Use --transitive
to follow the full dependency chain. Use --fields to include metadata.`,
		Args: cobra.ExactArgs(1),
		RunE: runFeatureDeps,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("fields", "", "comma-separated metadata fields to include (e.g., status,oq)")
	cmd.Flags().String("format", "", "output format: yaml, json, text (auto-selects yaml when --fields is set)")
	cmd.Flags().Bool("transitive", false, "follow dependency chain recursively")
	return cmd
}

func runFeatureDeps(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")
	fieldsFlag, _ := cmd.Flags().GetString("fields")
	transitive, _ := cmd.Flags().GetBool("transitive")

	fields, err := feature.ParseFieldNames(fieldsFlag)
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	format := effectiveFormat(cmd)
	if err := validateFormat(format); err != nil {
		return err
	}

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}

	if !feature.Exists(featuresDir, featureID) {
		return exitcode.NotFoundErrorf("feature not found: %s", featureID)
	}

	w := cmd.OutOrStdout()

	if transitive {
		nodes := feature.TransitiveDeps(featuresDir, featureID)
		if len(fields) > 0 {
			feature.EnrichTransitiveNodes(featuresDir, nodes, fields)
		}
		switch format {
		case "yaml":
			return writeEnrichedYAML(w, nodes)
		case "json":
			return writeEnrichedJSON(w, nodes)
		default:
			if len(fields) > 0 {
				return writeEnrichedText(w, nodes, fields)
			}
			var sb strings.Builder
			printTransitiveText(&sb, nodes, 0)
			_, _ = fmt.Fprint(w, sb.String())
		}
		return nil
	}

	readmePath := feature.ReadmePath(featuresDir, featureID)
	deps, err := feature.ParseDependencies(readmePath)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading feature %s: %v", featureID, err)
	}

	if len(fields) > 0 || format == "yaml" || format == "json" {
		var enriched []*feature.EnrichedFeature
		for _, dep := range deps {
			ef, _ := feature.ResolveFields(featuresDir, dep, fields)
			enriched = append(enriched, ef)
		}
		return writeEnrichedOutput(w, enriched, fields, format)
	}

	errW := cmd.ErrOrStderr()
	for _, dep := range deps {
		if !feature.Exists(featuresDir, dep) {
			_, _ = fmt.Fprintf(errW, "%s (not found)\n", dep)
		} else {
			_, _ = fmt.Fprintln(w, dep)
		}
	}
	return nil
}

// --- feature refs ---

func featureRefsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refs <feature_id>",
		Short: "Show features that reference a given feature as a dependency",
		Long: `Shows features that reference (depend on) a given feature. This is the
inverse of deps — it scans all features' ## Dependencies sections to find
those that list the given feature ID. Use --transitive to follow the full chain.`,
		Args: cobra.ExactArgs(1),
		RunE: runFeatureRefs,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("fields", "", "comma-separated metadata fields to include (e.g., status,oq)")
	cmd.Flags().String("format", "", "output format: yaml, json, text (auto-selects yaml when --fields is set)")
	cmd.Flags().Bool("transitive", false, "follow reference chain recursively")
	return cmd
}

func runFeatureRefs(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")
	fieldsFlag, _ := cmd.Flags().GetString("fields")
	transitive, _ := cmd.Flags().GetBool("transitive")

	fields, err := feature.ParseFieldNames(fieldsFlag)
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	format := effectiveFormat(cmd)
	if err := validateFormat(format); err != nil {
		return err
	}

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}

	if !feature.Exists(featuresDir, featureID) {
		return exitcode.NotFoundErrorf("feature not found: %s", featureID)
	}

	w := cmd.OutOrStdout()

	if transitive {
		nodes := feature.TransitiveRefs(featuresDir, featureID)
		if len(fields) > 0 {
			feature.EnrichTransitiveNodes(featuresDir, nodes, fields)
		}
		switch format {
		case "yaml":
			return writeEnrichedYAML(w, nodes)
		case "json":
			return writeEnrichedJSON(w, nodes)
		default:
			if len(fields) > 0 {
				return writeEnrichedText(w, nodes, fields)
			}
			var sb strings.Builder
			printTransitiveText(&sb, nodes, 0)
			_, _ = fmt.Fprint(w, sb.String())
		}
		return nil
	}

	refs, err := featureFindRefsFn(featuresDir, featureID)
	if err != nil {
		return exitcode.UnexpectedErrorf("finding references: %v", err)
	}

	if len(fields) > 0 || format == "yaml" || format == "json" {
		var enriched []*feature.EnrichedFeature
		for _, ref := range refs {
			ef, _ := feature.ResolveFields(featuresDir, ref, fields)
			enriched = append(enriched, ef)
		}
		return writeEnrichedOutput(w, enriched, fields, format)
	}

	for _, ref := range refs {
		_, _ = fmt.Fprintln(w, ref)
	}
	return nil
}

// --- feature new ---

func featureNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Scaffold a new feature directory with a README template",
		Long: `Creates a new feature directory with a README containing all required
sections. Changes are local by default; use --commit to create a git
commit, or --push for atomic commit-and-push.`,
		RunE: runFeatureNew,
	}
	cmd.Flags().String("title", "", "human-readable feature title (required)")
	cmd.Flags().String("slug", "", "feature slug (directory name); auto-generated from title if omitted")
	cmd.Flags().String("parent", "", "parent feature ID for creating a sub-feature")
	cmd.Flags().String("status", "Draft", "initial feature status: Draft, In Review, Approved, Implementing, Stable, Amending, Rejected, Deprecated")
	cmd.Flags().String("description", "", "short description placed in the Summary section")
	cmd.Flags().String("depends-on", "", "comma-separated list of feature IDs this feature depends on")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, text")
	cmd.Flags().Bool("commit", false, "create a git commit with the changes")
	cmd.Flags().Bool("push", false, "commit and push atomically (implies --commit)")
	return cmd
}

func runFeatureNew(cmd *cobra.Command, _ []string) error {
	title, _ := cmd.Flags().GetString("title")
	slugFlag, _ := cmd.Flags().GetString("slug")
	parentFlag, _ := cmd.Flags().GetString("parent")
	statusFlag, _ := cmd.Flags().GetString("status")
	descFlag, _ := cmd.Flags().GetString("description")
	depsFlag, _ := cmd.Flags().GetString("depends-on")
	projectFlag, _ := cmd.Flags().GetString("project")
	formatFlag, _ := cmd.Flags().GetString("format")
	commitFlag, _ := cmd.Flags().GetBool("commit")
	pushFlag, _ := cmd.Flags().GetBool("push")

	if title == "" {
		return exitcode.InvalidArgsError("missing required flag: --title")
	}
	if formatFlag != "yaml" && formatFlag != "json" && formatFlag != "text" {
		return exitcode.InvalidArgsErrorf("invalid format: %s (supported: yaml, json, text)", formatFlag)
	}
	if pushFlag {
		commitFlag = true
	}

	// Parse --depends-on
	var deps []string
	if depsFlag != "" {
		for _, d := range strings.Split(depsFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				deps = append(deps, d)
			}
		}
	}

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}

	opts := feature.NewOptions{
		Title:       title,
		Slug:        slugFlag,
		Parent:      parentFlag,
		Status:      statusFlag,
		Description: descFlag,
		DependsOn:   deps,
	}

	// cli-template-runtime-fetch: a bare feature (no description, deps, parent,
	// or non-default status) pulls the published template as its body; on any
	// fetch failure feature.New falls back to the generated README.
	if descFlag == "" && len(deps) == 0 && parentFlag == "" &&
		(statusFlag == "" || statusFlag == "Draft") {
		if t, ok := resolveBareTemplate("feature", map[string]string{
			"<Feature Name>": templateTitleOrSlug(title, slugFlag),
		}); ok {
			opts.Body = string(t)
		} else {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), fellBackWarning)
		}
	}

	result, err := feature.New(featuresDir, opts)
	if err != nil {
		return err
	}

	// Reconcile the freshly scaffolded README with project-specific lint
	// rules that a static template cannot satisfy on its own — chiefly
	// studio-toolbar, whose canonical line is composed from the project's
	// host/org/repo and so must be backfilled here. Mirrors the
	// scaffold→--fix pattern used by `idea/decision/issue new`. specSub is
	// the spec tree root (featuresDir is <specSub>/features).
	repoRoot := filepath.Dir(filepath.Dir(featuresDir)) // spec/features/ → repo root
	if _, statErr := os.Stat(filepath.Join(repoRoot, projectdef.SpecConfigFile)); statErr == nil {
		specSub := filepath.Dir(featuresDir)
		if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
			return exitcode.UnexpectedErrorf("running lint fix: %v", err)
		}
	} else if os.IsNotExist(statErr) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: no specscore.yaml; feature scaffold did not run lint --fix. Run `specscore init` then `specscore spec lint --fix` explicitly to migrate this repository.")
	} else {
		return exitcode.UnexpectedErrorf("reading %s: %v", projectdef.SpecConfigFile, statErr)
	}

	if commitFlag {
		if !isGitRepo(repoRoot) {
			return exitcode.UnexpectedError("not a git repository; cannot commit")
		}

		relFiles := make([]string, 0, len(result.ChangedFiles))
		for _, f := range result.ChangedFiles {
			rel, relErr := filepathRelFn(repoRoot, f)
			if relErr != nil {
				rel = f
			}
			relFiles = append(relFiles, rel)
		}

		commitMsg := fmt.Sprintf("feat(spec): add feature %s", result.FeatureID)

		if pushFlag {
			if err := gitCommitAndPush(repoRoot, relFiles, commitMsg); err != nil {
				return exitcode.ConflictErrorf("commit and push failed: %v", err)
			}
		} else {
			if err := gitCommitOnly(repoRoot, relFiles, commitMsg); err != nil {
				return exitcode.UnexpectedErrorf("commit failed: %v", err)
			}
		}
	}

	return writeFeatureInfo(cmd.OutOrStdout(), formatFlag, &result.Info)
}

// isGitRepo returns true if dir is inside a git repository.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// gitCommitOnly stages files and creates a commit without pushing.
func gitCommitOnly(repoDir string, files []string, message string) error {
	addArgs := append([]string{"-C", repoDir, "add"}, files...)
	addCmd := exec.Command("git", addArgs...)
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w\n%s", err, out)
	}

	commitCmd := exec.Command("git", "-C", repoDir, "commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}

// --- feature change-status ---

// featureChangeStatusHelpMatrix is the rendered legal-transition table
// for the Feature kind, embedded in `--help` so users see the matrix
// at the point of invocation. Computed once at package load from the
// authoritative lifecycle.LegalTargets data, so a future matrix change
// keeps `--help` current automatically.
var featureChangeStatusHelpMatrix = buildFeatureChangeStatusMatrix()

func buildFeatureChangeStatusMatrix() string {
	// Source ordering by lifecycle progression so the matrix reads
	// top-down through the Feature lifecycle. lifecycle.LegalTargets
	// returns its slice sorted alphabetically; we re-order the
	// surrounding rows by hand-defined progression for readability.
	froms := []lifecycle.Status{
		lifecycle.FeatureDraft,
		lifecycle.FeatureInReview,
		lifecycle.FeatureApproved,
		lifecycle.FeatureImplementing,
		lifecycle.FeatureStable,
		lifecycle.FeatureAmending,
		// Planned is not part of the normal forward progression — it is a
		// legacy side-entry (see lifecycle.FeaturePlanned's doc comment) —
		// so it is appended at the end rather than interleaved above.
		lifecycle.FeaturePlanned,
	}
	const fromWidth = 14
	var b strings.Builder
	b.WriteString("Legal transitions:\n\n")
	fmt.Fprintf(&b, "  %-*s To\n", fromWidth, "From")
	fmt.Fprintf(&b, "  %-*s --\n", fromWidth, "-----")
	for _, f := range froms {
		targets := lifecycle.LegalTargets(lifecycle.KindFeature, f)
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = string(t)
		}
		fmt.Fprintf(&b, "  %-*s %s\n", fromWidth, string(f), strings.Join(names, ", "))
	}
	return b.String()
}

// featureChangeStatusCommand registers `specscore feature change-status`.
// The verb implements the cli/lifecycle-transitions Meta contract for
// the Feature kind: validate state-machine, rewrite the **Status:**
// line, run `spec lint --fix` to sync the features-index row via
// feature-index-row-sync, roll back on lint failure.
func featureChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <feature_id>",
		Short: "Transition a feature to a new lifecycle status",
		Long: `Transitions a Feature artifact from its current **Status:** to the value
named by --to, then runs ` + "`specscore spec lint --fix`" + ` to sync the
features-index row via the feature-index-row-sync rule.

The transition is strict: the (current, --to) pair MUST appear in the
matrix below or the command exits 4 (InvalidTransition) with the file
left unchanged. There is no idempotent shortcut — re-running with the
current status as --to is rejected.

` + featureChangeStatusHelpMatrix + `
` + `
On exit 0, exactly one line is written to stdout:

    <feature_id>: <from> → <to>

On lint failure after a successful rewrite, the original Status line is
restored and the command exits 10.`,
		// Argument validation is performed manually inside RunE so a
		// missing positional yields a typed exit-2 error rather than
		// cobra's default unwrapped error (which would surface as
		// exit-1 via Fatal).
		Args: cobra.ArbitraryArgs,
		// Suppress cobra's usage-on-error dump so stdout stays empty
		// on non-zero exits, per the lifecycle-transitions Meta REQ
		// error-to-stderr / success-output-format guarantees that the
		// single stdout line (or nothing) is what callers see.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runFeatureChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required; case-insensitive)")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool("dry-run", false, "report the transition and every file that would change, writing nothing")
	return cmd
}

func runFeatureChangeStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <feature_id>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <feature_id>, got %d", len(args),
		)
	}
	featureID := args[0]

	toFlag, _ := cmd.Flags().GetString("to")
	if strings.TrimSpace(toFlag) == "" {
		return exitcode.InvalidArgsError("missing required flag: --to=<status>")
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}
	// featuresDir is always <projectRoot>/spec/features (resolveFeaturesDir's
	// contract); undo both joins to recover the project root dryrun.Sandbox
	// needs (it copies <root>/spec).
	projectRoot := filepath.Dir(filepath.Dir(featuresDir))

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		result, changes, err := dryrun.Sandbox(projectRoot, func(sandboxRoot string) (feature.ChangeStatusResult, error) {
			return featureChangeStatusMutate(sandboxRoot, featureID, toFlag)
		})
		if err != nil {
			return err
		}
		dryrun.PrintReport(cmd.OutOrStdout(), featureID, string(result.From), string(result.To), changes)
		return nil
	}

	result, err := featureChangeStatusMutate(projectRoot, featureID, toFlag)
	if err != nil {
		return err
	}

	// Success: exactly one line to stdout (Meta REQ
	// success-output-format), using the unicode arrow.
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", featureID, string(result.From), string(result.To))
	return nil
}

// featureChangeStatusMutate performs the Feature Status rewrite plus its
// post-rewrite `spec lint --fix`/verify/rollback dance, rooted at root (the
// project root containing spec/). It is the single mutation path both the
// real command and its --dry-run sandbox invoke — dry-run passes a
// throwaway copy of root here instead of the real one (see dryrun.Sandbox),
// so this function derives every path from its root argument rather than
// from any ambient/pre-resolved value.
func featureChangeStatusMutate(root, featureID, toFlag string) (feature.ChangeStatusResult, error) {
	featuresDir := filepath.Join(root, "spec", "features")
	result, err := feature.ChangeStatus(featuresDir, featureID, toFlag)
	if err != nil {
		return feature.ChangeStatusResult{}, err
	}

	// Post-rewrite: run `spec lint --fix` against the project's spec
	// tree to pick up the feature-index-row-sync rule. Per the Meta
	// REQ rollback-on-lint-failure, ANY error-severity violation
	// after `--fix` triggers rollback — pre-existing violations
	// elsewhere in the tree included.
	specRoot := filepath.Join(root, "spec")

	if _, fixErr := lintLintFn(lint.Options{SpecRoot: specRoot, Fix: true}); fixErr != nil {
		// Lint --fix itself errored; roll back so the on-disk state
		// matches its pre-invocation snapshot, then map to exit-10.
		if rbErr := result.Restore(); rbErr != nil {
			return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"lint --fix failed: %v; rollback also failed: %v",
				fixErr, rbErr,
			)
		}
		return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf("lint --fix failed: %v (rolled back)", fixErr)
	}

	violations, lintErr := lintLintFn(lint.Options{SpecRoot: specRoot})
	if lintErr != nil {
		if rbErr := result.Restore(); rbErr != nil {
			return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"post-fix lint failed: %v; rollback also failed: %v",
				lintErr, rbErr,
			)
		}
		return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf("post-fix lint failed: %v (rolled back)", lintErr)
	}

	var errs []lint.Violation
	for _, v := range violations {
		if v.Severity == "error" {
			errs = append(errs, v)
		}
	}
	if len(errs) > 0 {
		if rbErr := result.Restore(); rbErr != nil {
			return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"lint reported %d error-severity violation(s) after --fix; rollback also failed: %v",
				len(errs), rbErr,
			)
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "lint reported %d error-severity violation(s) after --fix (rolled back):\n", len(errs))
		for _, v := range errs {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return feature.ChangeStatusResult{}, exitcode.UnexpectedError(sb.String())
	}

	// A clean lint pass proves only that the derived-index work did not
	// ERROR — never that it left the requested status on disk. `--fix`
	// rules rewrite **Status:** lines themselves, so confirm the write
	// survived before the caller prints `<feature_id>: <from> → <to>`.
	// Reporting a success the file does not carry is a self-concealing
	// failure: the success line quotes the correct pair either way.
	if verifyErr := lifecycle.VerifyPersistedStatus(result.ReadmePath, result.To); verifyErr != nil {
		if rbErr := result.Restore(); rbErr != nil {
			return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"%v; rollback also failed: %v", verifyErr, rbErr)
		}
		return feature.ChangeStatusResult{}, exitcode.UnexpectedErrorf("%v (rolled back)", verifyErr)
	}

	return *result, nil
}

// gitCommitAndPush stages files, commits, and pushes. On push conflict it
// pulls, re-stages, and retries once.
func gitCommitAndPush(repoDir string, files []string, message string) error {
	if err := gitCommitOnly(repoDir, files, message); err != nil {
		return err
	}

	pushCmd := exec.Command("git", "-C", repoDir, "push")
	if out, err := pushCmd.CombinedOutput(); err != nil {
		// Try pull + re-push once.
		pullCmd := exec.Command("git", "-C", repoDir, "pull", "--rebase")
		if pullOut, pullErr := pullCmd.CombinedOutput(); pullErr != nil {
			return fmt.Errorf("git push: %w\n%s\ngit pull: %v\n%s", err, out, pullErr, pullOut)
		}
		retryCmd := exec.Command("git", "-C", repoDir, "push")
		if retryOut, retryErr := retryCmd.CombinedOutput(); retryErr != nil {
			return fmt.Errorf("git push (retry): %w\n%s", retryErr, retryOut)
		}
	}
	return nil
}
