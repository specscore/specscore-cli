package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

func lessonImportLegacyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "import-legacy", Short: "Inventory or losslessly import a historical lessons log", Long: `Dry-run inventories every heading and inline recurrence marker with exact byte
spans/hashes, an ordered-entry projection hash, duplicate numeric-ID collisions,
and unmatched candidates. Apply
requires immutable committed repository/path/revision identity and an explicit
reviewed new|occurrence disposition for every row; any manual row fails closed.

Each new mapping requires status Recorded, concise safe lesson and process_gap
text, and classifications present in the target specscore.yaml vocabulary.
Historical status remains provenance and the result reports the conservative
Recorded decision; import never fabricates Enforced evidence. The source
observation becomes one deterministic provider occurrence. An aggregate marker
remains one ambiguity-tagged evidence record. Raw source prose is referenced by
immutable provenance and never copied. Reapply is collision-safe and
byte-identical.

Docs: docs/agent-lessons.md#import-and-deduplicate-without-losing-history`, Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true, RunE: runLessonImportLegacy}
	cmd.Flags().String("source", "", "authoritative legacy log path (required)")
	cmd.Flags().String("mapping", "", "reviewed JSON mapping required by --apply")
	cmd.Flags().Bool("dry-run", false, "produce a write-free inventory")
	cmd.Flags().Bool("apply", false, "apply the reviewed mapping to missing targets")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func runLessonImportLegacy(cmd *cobra.Command, _ []string) error {
	source, _ := cmd.Flags().GetString("source")
	if source == "" {
		return exitcode.InvalidArgsError("--source is required")
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	apply, _ := cmd.Flags().GetBool("apply")
	if dry == apply {
		return exitcode.InvalidArgsError("choose exactly one of --dry-run or --apply")
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	project, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(project)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join(root, source)
	}
	inv, err := lessonInventoryLegacyFn(source)
	if err != nil {
		return exitcode.InvalidArgsErrorf("reading legacy source: %v", err)
	}
	if dry {
		return writeLegacyOutput(cmd, format, inv)
	}
	mappingPath, _ := cmd.Flags().GetString("mapping")
	if mappingPath == "" {
		return exitcode.InvalidArgsError("--apply requires --mapping with an explicit disposition for every source entry")
	}
	b, err := os.ReadFile(mappingPath)
	if err != nil {
		return exitcode.InvalidArgsErrorf("reading mapping: %v", err)
	}
	var mapping lesson.LegacyMapping
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&mapping); err != nil {
		return exitcode.InvalidArgsErrorf("parsing mapping JSON: %v", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return exitcode.InvalidArgsError("parsing mapping JSON: trailing JSON")
	}
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		return exitcode.InvalidStateErrorf("legacy apply requires a valid specscore.yaml: %v", err)
	}
	allowedClassifications := lessonClassificationsFromConfig(cfg)
	if len(allowedClassifications) == 0 {
		return exitcode.InvalidStateError("legacy apply requires non-empty lessons.classifications in specscore.yaml")
	}
	dir := filepath.Join(root, "spec", "lessons")
	if err := lesson.PreflightLegacyApply(dir, allowedClassifications, inv, mapping); err != nil {
		return exitcode.InvalidStateErrorf("legacy import preflight refused: %v", err)
	}
	prepared, err := prepareLessonEvent(root, "lesson.legacy-import-applied", "legacy-import", map[string]any{"source_sha256": inv.Source.SHA256, "source_revision": inv.Source.Revision, "mapping_count": len(mapping.Entries)}, time.Time{})
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing import event: %v", err)
	}
	result, err := lessonApplyLegacyFn(dir, allowedClassifications, inv, mapping)
	if err != nil {
		if recovery, resolved := prepared.ResolveMutationFailure("applying legacy import", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.InvalidStateErrorf("legacy import refused: %v", resolved)
		}
	}
	delivery, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("legacy import applied but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}
	return writeLegacyOutput(cmd, format, result)
}
func writeLegacyOutput(cmd *cobra.Command, format string, v any) error {
	switch format {
	case "json":
		return newJSONEnc(cmd.OutOrStdout()).Encode(v)
	case "yaml":
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(v); err != nil {
			return err
		}
		return enc.Close()
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return err
	}
}
