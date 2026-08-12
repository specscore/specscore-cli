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
	return runLessonImportLegacyWithDeps(cmd, defaultLessonCLIDeps())
}

func runLessonImportLegacyWithDeps(cmd *cobra.Command, deps lessonCLIDeps) error {
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
	inv, err := deps.inventoryLegacy(source)
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
	b, err := deps.fs.read(mappingPath)
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
	cfg, err := deps.readConfig(root)
	if err != nil {
		return exitcode.InvalidStateErrorf("legacy apply requires a valid specscore.yaml: %v", err)
	}
	allowedClassifications := lessonClassificationsFromConfig(cfg)
	if len(allowedClassifications) == 0 {
		return exitcode.InvalidStateError("legacy apply requires non-empty lessons.classifications in specscore.yaml")
	}
	dir := filepath.Join(root, "spec", "lessons")
	affected := legacyMappingSlugs(mapping)
	var result lesson.LegacyApplyResult
	transactionErr := deps.withMutationLocks(root, affected, func() error {
		if err := deps.preflightLegacy(dir, allowedClassifications, inv, mapping); err != nil {
			return exitcode.InvalidStateErrorf("legacy import preflight refused: %v", err)
		}
		if err := preflightOccurrenceIndex(root); err != nil {
			return exitcode.InvalidStateErrorf("legacy import requires a readable lessons index: %v", err)
		}
		inspection, err := deps.inspectLegacy(dir, allowedClassifications, inv, mapping)
		if err != nil {
			return exitcode.InvalidStateErrorf("legacy import inspection refused: %v", err)
		}
		result = inspection.Result
		eventPayload := map[string]any{"source_sha256": inv.Source.SHA256, "source_revision": inv.Source.Revision, "mapping_count": len(mapping.Entries)}
		// LegacyMapping contains only JSON-native values. Typed re-encoding makes
		// object ordering deterministic while retaining the mutation-significant
		// entry and classification order.
		normalizedMapping, _ := json.Marshal(mapping)
		intentFacts := map[string]any{"mapping_sha256": lessonIntentDigest(normalizedMapping)}
		prepared, err := deps.resumeEvent(root, "lesson.legacy-import-applied", "legacy-import", eventPayload, intentFacts)
		if err != nil {
			return exitcode.UnexpectedErrorf("inspecting prepared import event: %v", err)
		}
		if !inspection.MutationRequired && prepared == nil {
			// A fully completed retry is observation-only. Index repair is a
			// distinct mutation and must not be smuggled into an event-free no-op.
			return nil
		}
		if inspection.MutationRequired {
			if prepared == nil {
				committedAt, parseErr := time.Parse(time.RFC3339, inv.Source.CommittedAt)
				if parseErr != nil {
					return exitcode.UnexpectedErrorf("parsing validated legacy source timestamp: %v", parseErr)
				}
				prepared, err = deps.prepareIntentEvent(root, "lesson.legacy-import-applied", "legacy-import", eventPayload, intentFacts, committedAt)
				if err != nil {
					return exitcode.UnexpectedErrorf("preparing import event: %v", err)
				}
			}
			result, err = deps.applyLegacy(dir, allowedClassifications, inv, mapping)
			if err != nil {
				recovery, resolved := prepared.ResolveMutationFailure("applying legacy import", err)
				if recovery {
					return exitcode.UnexpectedErrorf("%v", resolved)
				}
				return exitcode.InvalidStateErrorf("legacy import refused: %v", resolved)
			}
		}
		extraFence := []string{result.Manifest}
		for _, slug := range affected {
			owner := filepath.Join(dir, slug, ".legacy-import-owner")
			if _, statErr := deps.fs.stat(owner); statErr == nil {
				extraFence = append(extraFence, owner)
			} else if !os.IsNotExist(statErr) {
				failure := &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("checking legacy owner marker for %s: %w", slug, statErr)}
				_, resolved := prepared.ResolveMutationFailure("reconciling legacy import", failure)
				return exitcode.UnexpectedErrorf("%v", resolved)
			}
		}
		if err := reconcileLockedLessons(root, affected, extraFence, deps); err != nil {
			failure := &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: err}
			_, resolved := prepared.ResolveMutationFailure("reconciling legacy import", failure)
			return exitcode.UnexpectedErrorf("%v", resolved)
		}
		if prepared != nil {
			delivery, commitErr := prepared.Commit(cmd.Context())
			if commitErr != nil {
				return exitcode.UnexpectedErrorf("legacy import applied but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
			}
			for _, failure := range delivery.Failed {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
			}
		}
		return nil
	})
	if transactionErr != nil {
		if _, ok := transactionErr.(interface{ ExitCode() int }); ok {
			return transactionErr
		}
		return exitcode.UnexpectedErrorf("legacy import transaction: %v", transactionErr)
	}
	return writeLegacyOutput(cmd, format, result)
}

func legacyMappingSlugs(mapping lesson.LegacyMapping) []string {
	seen := make(map[string]bool, len(mapping.Entries))
	var slugs []string
	for _, entry := range mapping.Entries {
		if (entry.Action == "new" || entry.Action == "occurrence") && !seen[entry.Slug] {
			seen[entry.Slug] = true
			slugs = append(slugs, entry.Slug)
		}
	}
	return slugs
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
