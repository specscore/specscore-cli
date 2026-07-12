package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// rehearseACsCommand returns the "rehearse acs" subcommand: it generates a
// feature's `## Acceptance Criteria` summary — a denormalized read-model — from
// the thin ACs in its `_acs/` directory, so a reader gets every AC's intent
// inline in one read. Feature: cli/rehearse/thin-acs.
func rehearseACsCommand() *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "acs <feature-dir>",
		Short: "Generate a feature's Acceptance Criteria summary from its _acs/ thin ACs",
		Long: `Read the thin acceptance criteria in <feature-dir>/_acs/*.ac.md and render the
` + "`## Acceptance Criteria`" + ` summary table — a generated read-model over the
single source of truth. Prints to stdout by default; --write regenerates the
section in <feature-dir>/README.md in place.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRehearseACs(args[0], write, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "regenerate the ## Acceptance Criteria section in the feature README.md")
	return cmd
}

// runRehearseACs is the testable body of `rehearse acs`.
func runRehearseACs(featureDir string, write bool, out io.Writer) error {
	acsDir := filepath.Join(featureDir, "_acs")
	entries, err := os.ReadDir(acsDir)
	if err != nil {
		return exitcode.InvalidArgsErrorf("cannot read %s: %v", acsDir, err)
	}
	var acs []scenario.AC
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ac.md") {
			continue
		}
		ac, perr := scenario.ParseAC(filepath.Join(acsDir, e.Name()))
		if perr != nil {
			return exitcode.InvalidArgsErrorf("%v", perr)
		}
		acs = append(acs, *ac)
	}

	summary := scenario.GenerateACSummary(acs)
	if !write {
		_, _ = fmt.Fprint(out, summary)
		return nil
	}

	readmePath := filepath.Join(featureDir, "README.md")
	data, rerr := os.ReadFile(readmePath)
	if rerr != nil {
		return exitcode.InvalidArgsErrorf("cannot read %s: %v", readmePath, rerr)
	}
	if werr := os.WriteFile(readmePath, []byte(injectACSummary(string(data), summary)), 0o644); werr != nil {
		return exitcode.InvalidArgsErrorf("cannot write %s: %v", readmePath, werr)
	}
	_, _ = fmt.Fprintf(out, "updated %s (%d AC(s))\n", readmePath, len(acs))
	return nil
}

// injectACSummary replaces the `## Acceptance Criteria` section of a feature
// README (from its heading to the next `## ` heading or end of file) with the
// generated summary, or appends the summary when the section is absent. This
// also migrates an old inline AC section (which nests `### AC:` H3 subheadings,
// not H2) into the generated table in one pass.
func injectACSummary(readme, summary string) string {
	summary = strings.TrimRight(summary, "\n")
	lines := strings.Split(readme, "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "## Acceptance Criteria" {
			start = i
			break
		}
	}
	if start < 0 {
		return strings.TrimRight(readme, "\n") + "\n\n" + summary + "\n"
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}

	before := strings.TrimRight(strings.Join(lines[:start], "\n"), "\n")
	after := strings.TrimLeft(strings.Join(lines[end:], "\n"), "\n")
	out := before + "\n\n" + summary + "\n"
	if after != "" {
		out += "\n" + after + "\n"
	}
	return out
}
