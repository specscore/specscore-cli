package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/specscore/specscore-cli/pkg/config"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/planstore"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect resolved SpecScore configuration across layers",
	}
	cmd.AddCommand(configShowCommand(), configGetCommand())
	return cmd
}

func configShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the configuration resolved across all layers",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
	}
	cmd.Flags().StringP("project", "p", "", "path to spec repository root")
	cmd.Flags().Bool("origin", false, "annotate each key with its source layer (local/project/home)")
	return cmd
}

func configGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <dotted.key>",
		Short: "Print a single resolved config value",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigGet,
	}
	cmd.Flags().StringP("project", "p", "", "path to spec repository root")
	cmd.Flags().Bool("origin", false, "also print the value's source layer")
	return cmd
}

// resolveLayeredConfig resolves the three config layers for the repo addressed
// by the --project flag (or the cwd) plus the user's home directory.
func resolveLayeredConfig(cmd *cobra.Command) (config.Resolved, error) {
	projectFlag, _ := cmd.Flags().GetString("project")

	var startDir string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return config.Resolved{}, exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		startDir = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return config.Resolved{}, exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		startDir = cwd
	}

	root, err := findRepoConfigRoot(startDir)
	if err != nil {
		return config.Resolved{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Resolved{}, exitcode.UnexpectedErrorf("cannot determine home directory: %v", err)
	}
	// OrgConfigPath's only error path is filepath.Abs failing on a relative
	// sourceRoot when os.Getwd is itself broken (e.g. a deleted cwd) —
	// verified empirically not reproducible even by deliberately deleting
	// the working directory mid-test on this platform, so this branch has
	// no deterministic, non-racy reproduction.
	orgPath, err := planstore.OrgConfigPath(root)
	if err != nil {
		return config.Resolved{}, exitcode.InvalidStateErrorf("resolving organization config: %v", err)
	}
	return config.ResolveDirWithOrg(root, home, orgPath)
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	res, err := resolveLayeredConfig(cmd)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	if origin, _ := cmd.Flags().GetBool("origin"); origin {
		keys := make([]string, 0, len(res.Origin))
		for k := range res.Origin {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(out, "%s: %s  # %s\n", k, renderValue(lookupValue(res.Values, k)), res.Origin[k])
		}
		return nil
	}

	data, _ := yaml.Marshal(res.Values)
	_, _ = fmt.Fprint(out, string(data))
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	res, err := resolveLayeredConfig(cmd)
	if err != nil {
		return err
	}
	key := args[0]
	val, ok := lookup(res.Values, key)
	if !ok {
		return exitcode.NotFoundErrorf("config key %q is not set", key)
	}
	out := cmd.OutOrStdout()
	rendered := renderValue(val)
	if origin, _ := cmd.Flags().GetBool("origin"); origin {
		if o := res.Origin[key]; o != "" {
			_, _ = fmt.Fprintf(out, "%s  # %s\n", rendered, o)
			return nil
		}
	}
	_, _ = fmt.Fprintln(out, rendered)
	return nil
}

// lookup resolves a dotted key in the merged config, reporting presence.
func lookup(m map[string]any, dotted string) (any, bool) {
	var cur any = m
	for _, p := range strings.Split(dotted, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := mm[p]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// lookupValue resolves a dotted key, returning nil when absent.
func lookupValue(m map[string]any, dotted string) any {
	v, _ := lookup(m, dotted)
	return v
}

// renderValue renders a config value: scalars inline, maps/sequences as YAML.
func renderValue(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		data, _ := yaml.Marshal(v)
		return strings.TrimRight(string(data), "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}
