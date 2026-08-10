package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

func lessonImportLegacyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "import-legacy", Short: "Inventory or losslessly import a historical lessons log", Long: "Inventory first, then apply only a reviewed explicit mapping. Docs: docs/agent-lessons.md#import-and-deduplicate-without-losing-history", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true, RunE: runLessonImportLegacy}
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
	if !filepath.IsAbs(source) {
		root, err := resolveSpecRoot("")
		if err != nil {
			return err
		}
		source = filepath.Join(root, source)
	}
	inv, err := lesson.InventoryLegacy(source)
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
	if err := json.Unmarshal(b, &mapping); err != nil {
		return exitcode.InvalidArgsErrorf("parsing mapping JSON: %v", err)
	}
	project, _ := cmd.Flags().GetString("project")
	dir, err := resolveLessonsDir(project)
	if err != nil {
		return err
	}
	result, err := lesson.ApplyLegacy(dir, inv, mapping)
	if err != nil {
		return exitcode.InvalidStateErrorf("legacy import refused: %v", err)
	}
	if root, err := resolveSpecRoot(project); err == nil {
		for _, slug := range result.CreatedLessons {
			if err := emitLessonEvent(cmd.Context(), root, "lesson.imported", slug, map[string]any{"source_sha256": inv.SourceSHA256, "kind": "lesson"}, time.Time{}); err != nil {
				return exitcode.UnexpectedErrorf("queueing import event: %v", err)
			}
		}
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
