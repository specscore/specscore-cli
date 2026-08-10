package cli

import (
	"bufio"
	"fmt"
	"io"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

// lessonInfoDoc is the structured output of `lesson info`. Metadata fields
// are always present (no omitempty) so an absent field renders as an empty
// but present key.
type lessonInfoDoc struct {
	Slug            string   `yaml:"slug" json:"slug"`
	Status          string   `yaml:"status" json:"status"`
	Date            string   `yaml:"date" json:"date"`
	Owner           string   `yaml:"owner" json:"owner"`
	Recurred        int      `yaml:"recurred" json:"recurred"`
	SupersededBy    string   `yaml:"superseded_by" json:"superseded_by"`
	Sections        []string `yaml:"sections" json:"sections"`
	MissingSections []string `yaml:"missing_sections" json:"missing_sections"`
}

func lessonInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <slug>",
		Short: "Show lesson metadata — status, recurrence count, and section coverage",
		Long: `Returns a lesson's metadata (slug, status, date, owner, recurrence
count, successor when superseded) together with which of the four required
sections (Incident, Process gap, Check, Enforcement) are present. Default
output is YAML; use --format for JSON or text. A missing lesson exits 3 with
no stdout output.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLessonInfo,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, text")
	return cmd
}

func runLessonInfo(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: info accepts exactly one <slug>, got %d", len(args),
		)
	}
	slug := args[0]

	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	lessonsDir, err := resolveLessonsDir(projectFlag)
	if err != nil {
		return err
	}

	path, err := resolveLessonPath(lessonsDir, slug)
	if err != nil {
		return err
	}

	l, err := lesson.Parse(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", slug, err)
	}

	var present []string
	for _, s := range lesson.RequiredSectionsFor(l) {
		if l.HasSection(s) {
			present = append(present, s)
		}
	}

	recurred := l.Recurred
	if l.Canonical {
		occurrences, err := lesson.DiscoverOccurrences(path)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading occurrences: %v", err)
		}
		recurred = len(occurrences)
	}
	doc := lessonInfoDoc{
		Slug:            l.Slug,
		Status:          l.Status,
		Date:            l.Date,
		Owner:           l.Owner,
		Recurred:        recurred,
		SupersededBy:    l.SupersededBy,
		Sections:        present,
		MissingSections: l.MissingRequiredSectionsForLayout(),
	}

	w := cmd.OutOrStdout()
	switch format {
	case "json":
		return newJSONEnc(w).Encode(doc)
	case "text":
		return writeLessonInfoText(w, doc)
	default:
		enc := newYAMLEnc(w)
		if err := enc.Encode(doc); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	}
}

func writeLessonInfoText(w io.Writer, doc lessonInfoDoc) error {
	bw := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(bw, "Slug:           %s\n", doc.Slug)
	_, _ = fmt.Fprintf(bw, "Status:         %s\n", doc.Status)
	_, _ = fmt.Fprintf(bw, "Date:           %s\n", doc.Date)
	_, _ = fmt.Fprintf(bw, "Owner:          %s\n", doc.Owner)
	_, _ = fmt.Fprintf(bw, "Recurred:       %d\n", doc.Recurred)
	if doc.SupersededBy != "" {
		_, _ = fmt.Fprintf(bw, "Superseded By:  %s\n", doc.SupersededBy)
	}
	if len(doc.MissingSections) == 0 {
		_, _ = fmt.Fprintf(bw, "Sections:       all present (%d/%d)\n", len(doc.Sections), len(doc.Sections))
	} else {
		_, _ = fmt.Fprintf(bw, "Sections:       %d present, missing: %v\n", len(doc.Sections), doc.MissingSections)
	}
	return bw.Flush()
}
