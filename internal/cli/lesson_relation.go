package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

func lessonRelationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "relation", Short: "Record human-confirmed Lesson relations"}
	cmd.AddCommand(lessonRelationAddCommand(), lessonRelationListCommand())
	return cmd
}

func lessonRelationAddCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "add <from> <to>", Short: "Add a confirmed related, duplicate, or supersession relation", Long: "Every relation requires a preview token; semantic duplication is never inferred. A retained duplicate becomes Superseded with Duplicate Of and Superseded By pointing to the unchanged canonical Lesson. Enforced duplicates, cycles, conflicts, malformed state, and overwrites are refused before publication. Docs: docs/agent-lessons.md#import-and-deduplicate-without-losing-history", Args: cobra.ExactArgs(2), SilenceUsage: true, SilenceErrors: true, RunE: runLessonRelationAdd}
	cmd.Flags().String("type", "", "relation type: related, duplicates, supersedes (required)")
	cmd.Flags().String("confirm", "", "required confirmation token printed by the preview")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func runLessonRelationAdd(cmd *cobra.Command, args []string) error {
	return runLessonRelationAddWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonRelationAddWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
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
	root, err := resolveSpecRoot(project)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "spec", "lessons")
	postMutation, err := prepareRelationPostMutationWithDeps(root, from, typ, to, deps)
	if err != nil {
		return err
	}
	prepared, err := deps.prepareEvent(root, "lesson.relation-recorded", from, map[string]any{"type": typ, "to": to}, time.Time{})
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing relation event: %v", err)
	}
	if err := deps.addRelationWithPostMutation(dir, from, typ, to, postMutation); err != nil {
		if recovery, resolved := prepared.ResolveMutationFailure("adding relation", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.InvalidStateErrorf("adding relation: %v", resolved)
		}
	}
	delivery, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("relation recorded but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", from, typ, to)
	return nil
}

func prepareRelationPostMutationWithDeps(root, from, typ, to string, deps lessonCLIDeps) (lesson.RelationPostMutationHook, error) {
	specRoot := filepath.Join(root, "spec")
	lessonsDir := filepath.Join(specRoot, "lessons")
	indexPath := filepath.Join(lessonsDir, "README.md")
	if _, err := deps.fs.stat(indexPath); err != nil {
		return nil, exitcode.UnexpectedErrorf("preflighting lessons index: %v", err)
	}
	return func() error {
		paths := make([]string, 0, 4)
		for _, slug := range []string{from, to} {
			path, err := lesson.ResolveLessonFile(lessonsDir, slug)
			if err != nil {
				return exitcode.UnexpectedErrorf("resolving relation endpoint %s: %v", slug, err)
			}
			updated, err := deps.parse(path)
			if err != nil {
				return exitcode.UnexpectedErrorf("parsing relation endpoint %s: %v", slug, err)
			}
			if err := deps.indexUpsert(specRoot, updated); err != nil {
				return exitcode.UnexpectedErrorf("upserting relation endpoint %s: %v", slug, err)
			}
			paths = append(paths, path)
		}
		violations, err := deps.lint(lint.Options{SpecRoot: specRoot})
		if err != nil {
			return exitcode.UnexpectedErrorf("running read-only relation lint: %v", err)
		}
		for _, violation := range violations {
			ownedEndpoint := violation.File == filepath.ToSlash(filepath.Join("lessons", from, "README.md")) || violation.File == filepath.ToSlash(filepath.Join("lessons", to, "README.md"))
			ownedIndex := violation.File == "lessons/README.md" && (violation.Rule == "L-003" || violation.Rule == "L-004") && (strings.Contains(violation.Message, from) || strings.Contains(violation.Message, to))
			if violation.Severity == "error" && (ownedEndpoint || ownedIndex) {
				return exitcode.UnexpectedErrorf("relation failed lint: %s:%d [%s] %s", violation.File, violation.Line, violation.Rule, violation.Message)
			}
		}
		if typ == "related" {
			paths = append(paths, filepath.Join(lessonsDir, ".relations.json"))
		}
		paths = append(paths, indexPath)
		if err := newLessonMutationCoordinator(nil, deps.durable).Fence(paths...); err != nil {
			return exitcode.UnexpectedErrorf("%v", err)
		}
		return nil
	}, nil
}

func lessonRelationListCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list <lesson>", Short: "List durable Lesson relations", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true, RunE: runLessonRelationList}
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func runLessonRelationList(cmd *cobra.Command, args []string) error {
	return runLessonRelationListWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonRelationListWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	project, _ := cmd.Flags().GetString("project")
	dir, err := resolveLessonsDir(project)
	if err != nil {
		return err
	}
	items, err := deps.listRelations(dir, args[0])
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
