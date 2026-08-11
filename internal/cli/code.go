package cli

// Features implemented: cli/code

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/sourceref"
	"github.com/spf13/cobra"
)

// codeCommand returns the "code" command group.
func codeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Query source code relationships to SpecScore resources",
		Long: `Commands for querying source code relationships to SpecScore resources.
Scans source files for specscore: annotations and URLs embedded in comments,
showing the resources (features, plans, docs) that code depends on.`,
	}
	cmd.AddCommand(
		codeDepsCommand(),
	)
	return cmd
}

func codeDepsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deps [flags]",
		Short: "Show SpecScore resources that source files depend on",
		Long: `Shows the SpecScore resources (features, plans, docs) that source files
depend on. Scans source files for specscore: annotations and URLs in comments
and lists the referenced resources.

This is a read-only command that scans the working tree and does not mutate anything.`,
		RunE: runCodeDeps,
	}
	cmd.Flags().String("path", "**/*", "glob pattern to select files (e.g., pkg/**/*.go). Defaults to **/* (all files)")
	cmd.Flags().String("type", "", "filter results to a specific resource type: feature, plan, or doc")
	cmd.Flags().Bool("check", false, "validate Feature #REQ citations against local configured repositories (offline; no fetch)")
	return cmd
}

func runCodeDeps(cmd *cobra.Command, _ []string) error {
	pathPattern, _ := cmd.Flags().GetString("path")
	typeFilter, _ := cmd.Flags().GetString("type")
	check, _ := cmd.Flags().GetBool("check")

	if typeFilter != "" && typeFilter != "feature" && typeFilter != "plan" && typeFilter != "doc" {
		return exitcode.InvalidArgsErrorf("invalid --type value: %s (must be feature, plan, or doc)", typeFilter)
	}

	files, err := sourceref.ExpandGlobPattern(pathPattern)
	if err != nil {
		return exitcode.InvalidArgsErrorf("invalid glob pattern %q: %v", pathPattern, err)
	}
	if len(files) == 0 {
		return nil
	}

	result, err := sourceref.ScanFiles(files)
	if err != nil {
		return exitcode.UnexpectedErrorf("scanning files: %v", err)
	}
	if check {
		root, rootErr := feature.FindSpecRepoRoot(".")
		if rootErr != nil {
			return exitcode.UnexpectedErrorf("resolving project for --check: %v", rootErr)
		}
		resolver := sourceref.NewLocalResolver(filepath.Join(root, "spec"))
		files := make([]string, 0, len(result.FileRefs)+len(result.ParseErrors))
		seenFiles := map[string]bool{}
		for file := range result.FileRefs {
			files = append(files, file)
			seenFiles[file] = true
		}
		for file := range result.ParseErrors {
			if !seenFiles[file] {
				files = append(files, file)
			}
		}
		sort.Strings(files)
		var failures []string
		for _, file := range files {
			for _, parseErr := range result.ParseErrors[file] {
				failures = append(failures, fmt.Sprintf("%s:%d: %s: %v", file, parseErr.Line, parseErr.Token, parseErr.Err))
			}
			refs := append([]*sourceref.Reference(nil), result.FileRefs[file]...)
			sort.Slice(refs, func(i, j int) bool { return refs[i].Canonical() < refs[j].Canonical() })
			for _, ref := range refs {
				if typeFilter != "" && ref.Type != typeFilter {
					continue
				}
				// --check presently makes an anchor-liveness claim only for Feature
				// citations. Plan and doc references stay visible in the listing but
				// have no #REQ address model to validate yet.
				if ref.Type != "feature" {
					continue
				}
				if _, validateErr := resolver.ValidateRequirementCitation(ref); validateErr != nil {
					failures = append(failures, fmt.Sprintf("%s: %s: %v", file, ref.Canonical(), validateErr))
				}
			}
		}
		if len(failures) > 0 {
			return exitcode.InvalidStateError("source citation validation failed:\n" + strings.Join(failures, "\n"))
		}
	}

	singleFile := len(result.FileRefs) == 1

	w := cmd.OutOrStdout()
	output := sourceref.FormatOutput(result, singleFile, typeFilter)
	if output != "" {
		_, _ = fmt.Fprint(w, output)
	}

	return nil
}
