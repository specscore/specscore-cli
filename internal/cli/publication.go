package cli

// Features implemented: cli/publication-policy

import (
	"fmt"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/publication"
	"github.com/spf13/cobra"
)

func publicationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publication",
		Short: "Publication policy helpers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		publicationSetCommand(),
		publicationResolveCommand(),
		publicationBranchCheckCommand(),
	)
	return cmd
}

func publicationSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set durable publication policy",
		Args:  cobra.NoArgs,
		RunE:  runPublicationSet,
	}
	cmd.Flags().String("scope", "", "config scope: user or project (required)")
	cmd.Flags().String("project", "", "project root for project-scoped config (default: auto-discover from CWD)")
	cmd.Flags().String("default", "", "set default workflow shorthand: just-edit, stage, commit, commit-and-push")
	cmd.Flags().String("event", "", "event policy name, e.g. idea.approved")
	cmd.Flags().String("command", "", "command policy name, e.g. implement")
	cmd.Flags().String("milestone", "", "command milestone policy name")
	cmd.Flags().String("actions", "", "canonical actions or shorthand, comma-separated")
	cmd.Flags().StringArray("action", nil, "canonical action; may be repeated")
	cmd.Flags().String("format", "text", "output format: text, json, yaml")
	return cmd
}

func publicationResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve effective publication policy",
		Args:  cobra.NoArgs,
		RunE:  runPublicationResolve,
	}
	cmd.Flags().String("project", "", "project root (default: auto-discover from CWD when available)")
	cmd.Flags().String("command", "", "command context")
	cmd.Flags().String("event", "", "event context")
	cmd.Flags().String("milestone", "", "milestone context")
	cmd.Flags().String("task-policy", "", "task-level publication workflow or actions")
	cmd.Flags().String("session-policy", "", "session-level publication workflow or actions")
	cmd.Flags().String("branch", "", "branch name (default: current git branch when project is available)")
	cmd.Flags().String("format", "yaml", "output format: json, yaml")
	return cmd
}

func publicationBranchCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch-check",
		Short: "Check whether publication policy allows pushing a branch",
		Args:  cobra.NoArgs,
		RunE:  runPublicationBranchCheck,
	}
	cmd.Flags().String("project", "", "project root (default: auto-discover from CWD when available)")
	cmd.Flags().String("branch", "", "branch name (default: current git branch when project is available)")
	cmd.Flags().String("format", "yaml", "output format: text, json, yaml")
	return cmd
}

func runPublicationSet(cmd *cobra.Command, _ []string) error {
	scope, _ := cmd.Flags().GetString("scope")
	if scope == "" {
		return exitcode.InvalidArgsError("missing required flag --scope (expected user or project)")
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validatePublicationFormat(format, true); err != nil {
		return err
	}

	defaultValue, _ := cmd.Flags().GetString("default")
	defaultChanged := cmd.Flags().Changed("default")
	actionsValue, _ := cmd.Flags().GetString("actions")
	repeatedActions, _ := cmd.Flags().GetStringArray("action")
	if defaultChanged && (actionsValue != "" || len(repeatedActions) > 0) {
		return exitcode.InvalidArgsError("--default cannot be combined with --actions or --action")
	}
	if !defaultChanged && actionsValue == "" && len(repeatedActions) == 0 {
		return exitcode.InvalidArgsError("missing publication actions; pass --actions, --action, or --default <workflow>")
	}

	actions := repeatedActions
	if defaultChanged {
		actions = []string{defaultValue}
	} else if actionsValue != "" {
		actions = append(actions, actionsValue)
	}

	projectRoot, err := resolvePublicationProjectRoot(cmd, scope == "project")
	if err != nil {
		return err
	}
	commandName, _ := cmd.Flags().GetString("command")
	eventName, _ := cmd.Flags().GetString("event")
	milestoneName, _ := cmd.Flags().GetString("milestone")
	if eventName != "" && milestoneName != "" {
		return exitcode.InvalidArgsError("--event and --milestone are mutually exclusive")
	}

	result, err := publication.SetPolicy(publication.SetOptions{
		Scope:       scope,
		ProjectRoot: projectRoot,
		Command:     commandName,
		Event:       eventName,
		Milestone:   milestoneName,
		Default:     defaultChanged,
		Actions:     actions,
	})
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}
	return outputPublicationSet(cmd, result, format)
}

func runPublicationResolve(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validatePublicationFormat(format, false); err != nil {
		return err
	}
	projectRoot, err := resolvePublicationProjectRoot(cmd, false)
	if err != nil {
		return err
	}
	commandName, _ := cmd.Flags().GetString("command")
	eventName, _ := cmd.Flags().GetString("event")
	milestoneName, _ := cmd.Flags().GetString("milestone")
	taskPolicy, _ := cmd.Flags().GetString("task-policy")
	sessionPolicy, _ := cmd.Flags().GetString("session-policy")
	branch, _ := cmd.Flags().GetString("branch")

	result, err := publication.Resolve(publication.ResolveOptions{
		ProjectRoot:   projectRoot,
		Command:       commandName,
		Event:         eventName,
		Milestone:     milestoneName,
		TaskPolicy:    actionFlagSlice(taskPolicy),
		SessionPolicy: actionFlagSlice(sessionPolicy),
		Branch:        branch,
	})
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}
	return outputPublicationResolve(cmd, result, format)
}

func runPublicationBranchCheck(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validatePublicationFormat(format, true); err != nil {
		return err
	}
	projectRoot, err := resolvePublicationProjectRoot(cmd, false)
	if err != nil {
		return err
	}
	branch, _ := cmd.Flags().GetString("branch")
	result, err := publication.Resolve(publication.ResolveOptions{ProjectRoot: projectRoot, Branch: branch})
	if err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}
	check := publication.BranchCheckResult{
		Branch:  result.Branch,
		Allowed: result.BranchPushAllowed,
		Reason:  result.BranchBlockReason,
		Sources: result.PolicySources,
	}
	if err := outputPublicationBranchCheck(cmd, check, format); err != nil {
		return err
	}
	if !check.Allowed {
		return exitcode.InvalidStateError(check.Reason)
	}
	return nil
}

func validatePublicationFormat(format string, allowText bool) error {
	switch format {
	case "json", "yaml":
		return nil
	case "text":
		if allowText {
			return nil
		}
	}
	return exitcode.InvalidArgsErrorf("invalid format %q", format)
}

func resolvePublicationProjectRoot(cmd *cobra.Command, required bool) (string, error) {
	projectFlag, _ := cmd.Flags().GetString("project")
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		return abs, nil
	}
	cwd, err := osGetwdFn()
	if err != nil {
		return "", exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
	}
	root, err := findRepoConfigRoot(cwd)
	if err != nil {
		if required {
			return "", err
		}
		return "", nil
	}
	return root, nil
}

func actionFlagSlice(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func outputPublicationSet(cmd *cobra.Command, result publication.SetResult, format string) error {
	switch format {
	case "json":
		return newJSONEnc(cmd.OutOrStdout()).Encode(result)
	case "yaml":
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(result); err != nil {
			return err
		}
		return enc.Close()
	default:
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", result.Path)
		return nil
	}
}

func outputPublicationResolve(cmd *cobra.Command, result publication.ResolveResult, format string) error {
	switch format {
	case "json":
		return newJSONEnc(cmd.OutOrStdout()).Encode(result)
	default:
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(result); err != nil {
			return err
		}
		return enc.Close()
	}
}

func outputPublicationBranchCheck(cmd *cobra.Command, result publication.BranchCheckResult, format string) error {
	switch format {
	case "json":
		return newJSONEnc(cmd.OutOrStdout()).Encode(result)
	case "yaml":
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(result); err != nil {
			return err
		}
		return enc.Close()
	default:
		if result.Allowed {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "push allowed for %s\n", result.Branch)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "push denied for %s: %s\n", result.Branch, result.Reason)
		}
		return nil
	}
}
