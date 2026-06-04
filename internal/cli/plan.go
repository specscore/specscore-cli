package cli

// Features implemented: cli/plan

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/spf13/cobra"
)

// planCommand returns the "plan" command group. Subcommands (list, info)
// are added by later plans; with no Run/RunE and no subcommands, cobra
// prints help and exits 0 for `specscore plan`.
func planCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Query plans — listing and inspecting plan metadata",
	}
}

// resolvePlansDir resolves the plans directory from a --project flag or CWD.
// Unlike resolveFeaturesDir, an absent plans directory is not an error: the
// path is returned regardless (emptiness is handled by the list command).
// Only a missing project root produces an error.
func resolvePlansDir(projectFlag string) (string, error) {
	var startDir string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		startDir = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return "", exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		startDir = cwd
	}

	root, err := feature.FindSpecRepoRoot(startDir)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "spec", "plans"), nil
}

// resolvePlanPath joins plansDir/<slug>.md and verifies the file exists.
// A missing file yields an exit-3 NotFound error naming the slug.
func resolvePlanPath(plansDir, slug string) (string, error) {
	path := filepath.Join(plansDir, slug+".md")
	if _, err := os.Stat(path); err != nil {
		return "", exitcode.NotFoundErrorf("plan not found: %s", slug)
	}
	return path, nil
}
