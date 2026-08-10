package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

func lessonRelationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "relation", Short: "Record human-confirmed Lesson relations"}
	cmd.AddCommand(lessonRelationAddCommand(), lessonRelationListCommand())
	return cmd
}

func lessonRelationAddCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "add <from> <to>", Short: "Add a confirmed related, duplicate, or supersession relation", Long: "Every relation requires a preview token; semantic duplication is never inferred. Docs: docs/agent-lessons.md#import-and-deduplicate-without-losing-history", Args: cobra.ExactArgs(2), SilenceUsage: true, SilenceErrors: true, RunE: runLessonRelationAdd}
	cmd.Flags().String("type", "", "relation type: related, duplicates, supersedes (required)")
	cmd.Flags().String("confirm", "", "required confirmation token printed by the preview")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func runLessonRelationAdd(cmd *cobra.Command, args []string) error {
	from, to := args[0], args[1]
	typ, _ := cmd.Flags().GetString("type")
	if err := lesson.ValidateRelation(from, typ, to); err != nil {
		return exitcode.InvalidArgsErrorf("invalid relation: %v", err)
	}
	token := lesson.RelationToken(from, typ, to)
	confirm, _ := cmd.Flags().GetString("confirm")
	if confirm != token {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "preview: %s %s %s\nconfirmation-token: %s\n", from, typ, to, token)
		return exitcode.InvalidArgsError("relation requires the exact --confirm token from this preview; --yes is intentionally unsupported")
	}
	project, _ := cmd.Flags().GetString("project")
	dir, err := resolveLessonsDir(project)
	if err != nil {
		return err
	}
	if err := lesson.AddRelation(dir, from, typ, to); err != nil {
		return exitcode.InvalidStateErrorf("adding relation: %v", err)
	}
	if root, err := resolveSpecRoot(project); err == nil {
		if err := emitLessonEvent(cmd.Context(), root, "lesson.relation-recorded", from, map[string]any{"type": typ, "to": to}, time.Time{}); err != nil {
			return exitcode.UnexpectedErrorf("queueing relation event: %v", err)
		}
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", from, typ, to)
	return nil
}

func lessonRelationListCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list <lesson>", Short: "List durable Lesson relations", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true, RunE: runLessonRelationList}
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func runLessonRelationList(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	project, _ := cmd.Flags().GetString("project")
	dir, err := resolveLessonsDir(project)
	if err != nil {
		return err
	}
	items, err := lesson.ListRelations(dir, args[0])
	if err != nil {
		return err
	}
	switch format {
	case "json":
		return newJSONEnc(cmd.OutOrStdout()).Encode(items)
	case "yaml":
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(items); err != nil {
			return err
		}
		return enc.Close()
	default:
		for _, r := range items {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.Join([]string{r.From, r.Type, r.To}, " "))
		}
		return nil
	}
}
