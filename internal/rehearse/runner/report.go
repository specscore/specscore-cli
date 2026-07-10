package runner

import (
	"fmt"
	"io"
	"strings"
)

// RenderHuman writes the human report: one line per scenario — status, file
// path, Verifies AC ids, duration — plus a totals line (REQ: run-report).
// Failing scenarios additionally get their failure detail and captured step
// output as indented diagnostic lines (REQ: bash-block).
func RenderHuman(w io.Writer, reports []ScenarioReport) {
	counts := map[string]int{}
	for _, r := range reports {
		counts[r.Status]++
		_, _ = fmt.Fprintf(w, "%-8s  %s  [%s]  %dms\n",
			r.Status, r.File, strings.Join(r.Verifies, ", "), r.DurationMS)
		if r.Status != StatusFail {
			continue
		}
		writeIndented(w, r.Detail)
		for _, s := range r.Steps {
			if s.Status != StatusFail {
				continue
			}
			writeIndented(w, fmt.Sprintf("%s step: %s", s.Kind, s.Detail))
			writeIndented(w, s.Output)
		}
	}
	_, _ = fmt.Fprintf(w, "Total: %d scenario(s) — %d pass, %d fail, %d skipped, %d no-steps\n",
		len(reports), counts[StatusPass], counts[StatusFail], counts[StatusSkipped], counts[StatusNoSteps])
}

// writeIndented writes each line of text indented as a diagnostic under a
// scenario line. Empty text writes nothing.
func writeIndented(w io.Writer, text string) {
	if text == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		_, _ = fmt.Fprintf(w, "    %s\n", line)
	}
}

// CountFailed returns how many scenarios failed (drives exit code 1,
// REQ: run-report).
func CountFailed(reports []ScenarioReport) int {
	failed := 0
	for _, r := range reports {
		if r.Status == StatusFail {
			failed++
		}
	}
	return failed
}
