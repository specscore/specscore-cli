package cli

// Generic "transitions [<id>]" query verb, shared by every lifecycle.Kind
// that runs change-status through pkg/lifecycle's matrix (feature, idea,
// plan, task, lesson, decision). Issue and sidekick have their own tiny,
// independent matrices (pkg/issue, pkg/sidekick) and get their own thin
// wiring below rather than being forced through this generic builder.
//
// This is a pure read: it never mutates, and it derives its output from the
// SAME transition tables `change-status` validates against — there is no
// second, hand-authored list of statuses or transitions to drift out of
// sync with the ones enforced at mutation time.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// artifactStatusResult is the per-artifact payload for `<kind> transitions
// <id>` structured output (--format json|yaml): the artifact's current
// status plus what it can legally become next (and, for context, what could
// have legally led here).
type artifactStatusResult struct {
	ID       string             `json:"id" yaml:"id"`
	Status   lifecycle.Status   `json:"status" yaml:"status"`
	Previous []lifecycle.Status `json:"previous" yaml:"previous"`
	Next     []lifecycle.Status `json:"next" yaml:"next"`
}

// transitionsCommand builds the generic "transitions [<id>]" subcommand for
// a lifecycle.Kind.
//
//   - No positional argument: prints the kind's complete bidirectional
//     status matrix — lifecycle.BidirectionalMatrix(kind) — every
//     recognized status with its legal predecessors ("previous") and legal
//     successors ("next").
//   - With <id>: resolve maps id to the artifact's file path exactly as the
//     kind's own change-status/info verbs do; this command then reads the
//     artifact's CURRENT status and reports that one status's previous/next
//     — i.e., exactly the --to values change-status would currently accept.
//
// idUse is the positional's placeholder name in Use/Long text (e.g.
// "feature_id" or "slug"). resolve returns an *exitcode.Error (NotFound,
// typically) on an unresolvable id, matching the kind's other query verbs.
func transitionsCommand(kind lifecycle.Kind, idUse, short string, resolve func(projectFlag, id string) (path string, err error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("transitions [<%s>]", idUse),
		Short: short,
		Long: fmt.Sprintf(`Prints the %s kind's legal-transition matrix: every recognized status
with the statuses that can legally precede it ("previous") and the statuses
it can legally become ("next"), derived from the SAME matrix change-status
validates against. An empty "previous" means the status is set only by the
kind's scaffold/new verb, never by change-status; an empty "next" means the
status is terminal — no change-status call can move an artifact away from it.

With <%s>, reports that ONE artifact's CURRENT status and the "next" values
a change-status call would accept for it right now (plus "previous" for
context). Read-only — it never mutates the artifact.`, string(kind), idUse),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			if format != "text" && format != "json" && format != "yaml" {
				return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, json, yaml)", format)
			}
			if len(args) == 0 {
				return printKindMatrix(cmd.OutOrStdout(), kind, format)
			}
			projectFlag, _ := cmd.Flags().GetString("project")
			path, err := resolve(projectFlag, args[0])
			if err != nil {
				return err
			}
			current, err := lifecycle.CurrentStatus(kind, path)
			if err != nil {
				return exitcode.UnexpectedErrorf("reading %s status: %v", string(kind), err)
			}
			edge := lifecycle.EdgeFor(kind, current)
			return printArtifactEdge(cmd.OutOrStdout(), args[0], edge.Status, edge.Previous, edge.Next, format)
		},
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, json, yaml")
	return cmd
}

func printKindMatrix(w io.Writer, kind lifecycle.Kind, format string) error {
	edges := lifecycle.BidirectionalMatrix(kind)
	switch format {
	case "json":
		return printJSON(w, edges)
	case "yaml":
		return printYAML(w, edges)
	default:
		_, _ = fmt.Fprint(w, lifecycle.RenderBidirectionalMatrix(kind))
		return nil
	}
}

func printArtifactEdge(w io.Writer, id string, status lifecycle.Status, previous, next []lifecycle.Status, format string) error {
	switch format {
	case "json":
		return printJSON(w, artifactStatusResult{ID: id, Status: status, Previous: previous, Next: next})
	case "yaml":
		return printYAML(w, artifactStatusResult{ID: id, Status: status, Previous: previous, Next: next})
	default:
		_, _ = fmt.Fprintf(w, "%s: %s\n", id, string(status))
		_, _ = fmt.Fprintf(w, "  previous: %s\n", statusListOrPlaceholder(previous, "(none — initial status, set only by the kind's scaffold/new verb)"))
		_, _ = fmt.Fprintf(w, "  next:     %s\n", statusListOrPlaceholder(next, "(none — terminal status)"))
		return nil
	}
}

func statusListOrPlaceholder(ss []lifecycle.Status, placeholder string) string {
	if len(ss) == 0 {
		return placeholder
	}
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

func printJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return exitcode.UnexpectedErrorf("marshaling json: %v", err)
	}
	_, _ = fmt.Fprintln(w, string(b))
	return nil
}

func printYAML(w io.Writer, v any) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = exitcode.UnexpectedErrorf("marshaling yaml: %v", recovered)
		}
	}()
	b, err := yaml.Marshal(v)
	if err != nil {
		return exitcode.UnexpectedErrorf("marshaling yaml: %v", err)
	}
	_, _ = fmt.Fprint(w, string(b))
	return nil
}
