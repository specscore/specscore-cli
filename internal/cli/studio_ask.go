package cli

// Feature: cli/studio/answers (REQ: ask-verb-and-router, REQ: ask-citations,
// REQ: ask-unroutable)
//
// `specscore studio ask "<question>"` is the deterministic question router: it
// routes a natural-language question to one of 13 parameterized templates
// (internal/studio/ask), canonicalises the captured parameter through resolve
// semantics where the template targets an entity, runs the template's store
// query, and renders an answer that always carries citations.
//
// Exit codes (pkg/exitcode vocabulary):
//   routed + non-empty  → 0, answer + citations (human) / JSON object
//   unroutable          → 1, "no template matched" + the template list
//   routed-but-empty    → 3, "routed to <t> but found no facts for <p>"
//   ambiguous parameter → 5, candidate list (AmbiguousSlug)
//   empty question / bad --format / missing store → 2

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/studio/ask"
	"github.com/specscore/specscore-cli/internal/studio/resolve"
	"github.com/specscore/specscore-cli/internal/studio/store"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func studioAskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Answer a question by routing it to a store-query template (offline)",
		Long: `Answers a natural-language question by routing it — deterministically, with
no LLM — to one of a fixed library of 13 parameterized question templates, each
backed by a query over the fact store built by "studio index" (enriched by
"studio probe" and "studio contradictions"). It performs no network I/O.

The router lowercases the question, matches it against each template's trigger
keywords (e.g. "who fronts <X>", "status of <X>", "is <X> live"), resolves the
captured parameter through the same case-insensitive resolution "studio resolve"
uses where the template targets an entity, runs the template's store query, and
renders the answer. Every answer carries citations: the subject, predicate,
object, evidence class, evidence pointer, and adapter id of every fact the
answer stands on. An answer with zero supporting facts is not an answer.

Exit codes:
  0  routed and answered — the answer plus an Evidence block (or a JSON object
     {question, template, parameter, answer, citations} with --format json)
  1  unroutable — no template matched; prints a "no template matched" notice and
     the routable template list (the same content as --list)
  3  routed but found no facts for the parameter (a real template, no data)
  5  the parameter resolves to more than one candidate id (disambiguate first)
  2  empty question, an unknown --format, or a missing store

Use --list to print the routable templates without asking a question.

Running before "studio index" exits 2 with a message naming the expected store
path and suggesting "specscore studio index".`,
		Args:         cobra.ArbitraryArgs, // validated in RunE for better messages
		SilenceUsage: true,
		RunE:         runStudioAsk,
	}
	cmd.Flags().String("workspace", "./studio.yaml", "path to the studio.yaml workspace file")
	cmd.Flags().String("db", "", "fact store path (default <workspace-dir>/.specscore-studio/facts.db)")
	cmd.Flags().String("format", "human", "output format: human or json")
	cmd.Flags().Bool("list", false, "print the routable question templates and exit")
	return cmd
}

// askJSONResponse is the JSON shape of a routed answer (REQ: ask-verb-and-router).
type askJSONResponse struct {
	Question  string         `json:"question"`
	Template  string         `json:"template"`
	Parameter string         `json:"parameter"`
	Answer    string         `json:"answer"`
	Citations []ask.Citation `json:"citations"`
}

func runStudioAsk(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "human" && format != "json" {
		return exitcode.InvalidArgsErrorf("unknown --format %q: expected human or json", format)
	}

	// --list prints the template library without needing a question or a store.
	if list, _ := cmd.Flags().GetBool("list"); list {
		return renderTemplateList(cmd, "")
	}

	question := strings.TrimSpace(strings.Join(args, " "))
	if question == "" {
		return exitcode.InvalidArgsErrorf("studio ask requires a non-empty <question> argument")
	}

	dbPath, err := studioFactsStorePath(cmd)
	if err != nil {
		return err
	}

	// Query serves as the missing/empty-store guard: it returns the same exit-2
	// guidance `studio facts` gives when `studio index` has never run.
	facts, err := store.Query(dbPath, store.Filter{})
	if err != nil {
		return err
	}

	tmpl, param, ok := ask.Route(question)
	if !ok {
		// Unroutable (REQ: ask-unroutable): exit 1 with the "no template matched"
		// notice and the routable template list.
		return renderTemplateList(cmd, question)
	}

	// Entity-targeting templates canonicalise the captured parameter through
	// resolve semantics (REQ: ask-verb-and-router). An ambiguous parameter
	// follows the house AmbiguousSlug rule (exit 5); an unknown one flows through
	// as-is so the template's own query reports routed-but-empty.
	resolvedParam := param
	if tmpl.TargetsEntity {
		r := ask.Resolve(facts, param)
		switch r.Kind {
		case resolve.Unique:
			resolvedParam = r.ID
		case resolve.Ambiguous:
			return ambiguousParamError(param, r.Candidates)
		default: // resolve.NotFound — keep the raw parameter; the query decides.
		}
	}

	answer, answered := tmpl.Query(facts, resolvedParam)
	if !answered {
		// Routed-but-empty (REQ: ask-unroutable): exit 3, distinguishing a
		// template gap from a data gap, with no citation-free prose.
		return exitcode.NotFoundErrorf(
			"studio ask: routed to %s but found no facts for %q", tmpl.ID, param)
	}

	return renderAskAnswer(cmd, format, question, tmpl.ID, param, answer)
}

// ambiguousParamError builds the exit-5 error listing every candidate id and its
// citation (REQ: ask-verb-and-router routes through resolve; the house
// AmbiguousSlug rule).
func ambiguousParamError(param string, candidates []resolve.Candidate) error {
	parts := make([]string, 0, len(candidates))
	for _, c := range candidates {
		parts = append(parts, fmt.Sprintf("%s (via %s)", c.ID, c.Citation.Predicate))
	}
	return exitcode.Newf(exitcode.AmbiguousSlug,
		"studio ask: parameter %q is ambiguous — candidates: %s", param, strings.Join(parts, "; "))
}

// renderAskAnswer prints a routed answer as JSON or human prose with an Evidence
// block (REQ: ask-verb-and-router, REQ: ask-citations).
func renderAskAnswer(cmd *cobra.Command, format, question, templateID, param string, answer ask.Answer) error {
	w := cmd.OutOrStdout()
	if format == "json" {
		resp := askJSONResponse{
			Question:  question,
			Template:  templateID,
			Parameter: param,
			Answer:    answer.Answer,
			Citations: answer.Citations,
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}
	_, _ = fmt.Fprintln(w, answer.Answer)
	_, _ = fmt.Fprintln(w, "Evidence:")
	for _, c := range answer.Citations {
		_, _ = fmt.Fprintf(w, "  (%s, %s, %s) class=%s pointer=%s adapter=%s\n",
			c.Subject, c.Predicate, c.Object, c.Class, c.Pointer, c.Adapter)
	}
	return nil
}

// renderTemplateList prints the routable template list. When question is empty
// (the --list flag) it exits 0; when question is non-empty (an unroutable
// question) it prints a "no template matched" notice and returns the exit-1
// Conflict error (REQ: ask-unroutable).
func renderTemplateList(cmd *cobra.Command, question string) error {
	w := cmd.OutOrStdout()
	if question != "" {
		_, _ = fmt.Fprintf(w, "no template matched %q\n", question)
	}
	_, _ = fmt.Fprintln(w, "Routable templates:")
	for _, tmpl := range ask.Templates() {
		_, _ = fmt.Fprintf(w, "  %s: %s\n", tmpl.ID, strings.Join(tmpl.Forms, " / "))
	}
	if question != "" {
		return exitcode.ConflictErrorf("no template matched the question")
	}
	return nil
}
