package cli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/spf13/cobra"
)

var taskAmendNow = time.Now

func taskAmendCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "amend <task>", Short: "Correct Task Note/Evidence without changing Status", Long: `Replaces or removes the singleton **Note:** and **Evidence:** fields without changing task Status or **Implemented-by:** provenance. Every successful correction appends an **Annotation Amendment:** audit line carrying the supplied actor and reason, UTC time, and SHA-256 digest of the exact prior artifact.`, Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true, RunE: runTaskAmend}
	cmd.Flags().String("note", "", "replacement **Note:** value")
	cmd.Flags().Bool("clear-note", false, "remove the singleton **Note:** field")
	cmd.Flags().String("evidence", "", "replacement comma-separated **Evidence:** values")
	cmd.Flags().Bool("clear-evidence", false, "remove the singleton **Evidence:** field")
	cmd.Flags().String("actor", "", "required actor recording this correction")
	cmd.Flags().String("reason", "", "required reason for this correction")
	cmd.Flags().String("plan", "", "resolve <task> by **Id:** in a plan (flat or directory layout)")
	cmd.Flags().String("project", "", "project root (autodetected when omitted)")
	cmd.Flags().Bool(coordinationForceFlagName, false, coordinationForceFlagUsage+" (only applies with --plan)")
	return cmd
}

type annotationAmendment struct {
	Note, Evidence *string
	Actor, Reason  string
}

func runTaskAmend(cmd *cobra.Command, args []string) error {
	a, err := amendmentFromFlags(cmd)
	if err != nil {
		return err
	}
	if slug, _ := cmd.Flags().GetString("plan"); strings.TrimSpace(slug) != "" {
		return amendPlanTask(cmd, args[0], strings.TrimSpace(slug), a)
	}
	dir, err := resolveTasksDir(flagString(cmd, "project"))
	if err != nil {
		return err
	}
	path, err := resolveBoardTaskPath(dir, args[0])
	if err != nil {
		return err
	}
	return amendTaskArtifact(cmd, path, args[0], 0, -1, a)
}

func amendmentFromFlags(cmd *cobra.Command) (annotationAmendment, error) {
	note, evidence, actor, reason := strings.TrimSpace(flagString(cmd, "note")), evidenceFromFlags(cmd), strings.TrimSpace(flagString(cmd, "actor")), strings.TrimSpace(flagString(cmd, "reason"))
	noteSet, evidenceSet := cmd.Flags().Changed("note"), cmd.Flags().Changed("evidence")
	clearNote, _ := cmd.Flags().GetBool("clear-note")
	clearEvidence, _ := cmd.Flags().GetBool("clear-evidence")
	if actor == "" || reason == "" || strings.ContainsAny(actor+reason, "\r\n") {
		return annotationAmendment{}, exitcode.InvalidArgsError("--actor and --reason are required single-line values")
	}
	if (noteSet && clearNote) || (evidenceSet && clearEvidence) {
		return annotationAmendment{}, exitcode.InvalidArgsError("a replacement flag cannot be combined with its --clear-* flag")
	}
	if !noteSet && !clearNote && !evidenceSet && !clearEvidence {
		return annotationAmendment{}, exitcode.InvalidArgsError("supply --note, --evidence, --clear-note, or --clear-evidence")
	}
	if (noteSet && note == "") || (evidenceSet && len(evidence) == 0) {
		return annotationAmendment{}, exitcode.InvalidArgsError("replacement annotation values must be non-blank; use --clear-note or --clear-evidence to remove a field")
	}
	a := annotationAmendment{Actor: actor, Reason: reason}
	if noteSet {
		a.Note = &note
	} else if clearNote {
		empty := ""
		a.Note = &empty
	}
	if evidenceSet {
		joined := strings.Join(evidence, ", ")
		a.Evidence = &joined
	} else if clearEvidence {
		empty := ""
		a.Evidence = &empty
	}
	return a, nil
}

func flagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func resolveBoardTaskPath(dir, slug string) (string, error) {
	for _, p := range []string{filepath.Join(dir, slug, "README.md"), filepath.Join(dir, slug+".md")} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", exitcode.NotFoundErrorf("task not found: %s", slug)
}

func amendPlanTask(cmd *cobra.Command, taskSlug, planSlug string, a annotationAmendment) error {
	root, err := resolveSpecRoot(flagString(cmd, "project"))
	if err != nil {
		return err
	}
	var path string
	for _, p := range []string{filepath.Join(root, "spec", "plans", planSlug+".md"), filepath.Join(root, "spec", "plans", planSlug, "README.md")} {
		if _, e := os.Stat(p); e == nil {
			path = p
			break
		}
	}
	if path == "" {
		return exitcode.NotFoundErrorf("plan not found: %s", planSlug)
	}
	var coordinationWarning bytes.Buffer
	err = taskTransformArtifactFn(path, func(before []byte) ([]byte, error) {
		p, err := plan.ParseBytes(path, before)
		if err != nil {
			return nil, err
		}
		force, _ := cmd.Flags().GetBool(coordinationForceFlagName)
		if err := enforceCoordinationBranch(p, root, force, &coordinationWarning); err != nil {
			return nil, err
		}
		target, err := uniquePlanTaskByID(p, taskSlug)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(before), "\n")
		end := len(lines)
		for line := target.HeadingLine; line < len(lines); line++ {
			trimmed := strings.TrimSpace(lines[line])
			if strings.HasPrefix(trimmed, "### Task ") || strings.HasPrefix(trimmed, "## ") {
				end = line
				break
			}
		}
		return amendTaskBytes(before, taskSlug, target.HeadingLine-1, end, a)
	})
	if err != nil {
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return exitcode.ConflictErrorf("task %s changed concurrently; re-read before amending", taskSlug)
		}
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return err
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("writing plan-inline amendment: %v", err), err)
	}
	_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: annotations amended\n", taskSlug)
	return nil
}

// start/end are zero-based line bounds; start=0/end=-1 means the full board file.
func amendTaskArtifact(cmd *cobra.Command, path, slug string, start, end int, a annotationAmendment) error {
	if err := taskTransformArtifactFn(path, func(before []byte) ([]byte, error) {
		return amendTaskBytes(before, slug, start, end, a)
	}); err != nil {
		if err == lifecycle.ErrConcurrentMutation {
			return exitcode.ConflictErrorf("task %s changed concurrently; re-read before amending", slug)
		}
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return err
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("writing amendment: %v", err), err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: annotations amended\n", slug)
	return nil
}

func amendTaskBytes(before []byte, slug string, start, end int, a annotationAmendment) ([]byte, error) {
	newline := "\n"
	if strings.Contains(string(before), "\r\n") {
		newline = "\r\n"
	}
	hasFinalNewline := strings.HasSuffix(string(before), newline)
	lines := strings.Split(string(before), newline)
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	status, statusCount := -1, 0
	for i := start; i < end; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "**Status:**") {
			status = i
			statusCount++
		}
	}
	if statusCount != 1 {
		return nil, exitcode.UnexpectedErrorf("task %s must have exactly one **Status:** line", slug)
	}
	fields := map[string]int{}
	for i := status + 1; i < end; i++ {
		s := strings.TrimSpace(lines[i])
		for _, name := range []string{"Note", "Evidence"} {
			prefix := "**" + name + ":**"
			if strings.HasPrefix(s, prefix) {
				if strings.TrimSpace(strings.TrimPrefix(s, prefix)) == "" || fields[name] != 0 {
					return nil, exitcode.InvalidArgsErrorf("task %s has ambiguous or malformed **%s:** fields", slug, name)
				}
				fields[name] = i + 1
			}
		}
	}
	insert := status + 1
	for insert < end {
		s := strings.TrimSpace(lines[insert])
		if strings.HasPrefix(s, "**Implemented-by:**") || strings.HasPrefix(s, "**Note:**") || strings.HasPrefix(s, "**Evidence:**") {
			insert++
			continue
		}
		break
	}
	kept := make([]string, 0, len(lines)+1)
	for i, l := range lines {
		if (i == fields["Note"]-1 && a.Note != nil) || (i == fields["Evidence"]-1 && a.Evidence != nil) {
			continue
		}
		kept = append(kept, l)
	}
	// Recalculate insertion after removals and add replacement fields in canonical order.
	status = -1
	for i := start; i < len(kept); i++ {
		l := kept[i]
		if strings.HasPrefix(strings.TrimSpace(l), "**Status:**") {
			status = i
			break
		}
	}
	if status < 0 {
		return nil, exitcode.UnexpectedErrorf("task %s lost its **Status:** line during amendment", slug)
	}
	insert = status + 1
	for insert < len(kept) && (strings.HasPrefix(strings.TrimSpace(kept[insert]), "**Implemented-by:**") || strings.HasPrefix(strings.TrimSpace(kept[insert]), "**Note:**") || strings.HasPrefix(strings.TrimSpace(kept[insert]), "**Evidence:")) {
		insert++
	}
	add := []string{}
	if a.Note != nil && *a.Note != "" {
		add = append(add, "**Note:** "+*a.Note)
	}
	if a.Evidence != nil && *a.Evidence != "" {
		add = append(add, "**Evidence:** "+*a.Evidence)
	}
	if len(add) > 0 {
		kept = append(kept[:insert], append(add, kept[insert:]...)...)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(before))
	audit := fmt.Sprintf("**Annotation Amendment:** actor=%s; at=%s; reason=%s; before_sha256=%s", a.Actor, taskAmendNow().UTC().Format(time.RFC3339), a.Reason, digest)
	// The audit is append-only and stays within a plan task block (before next task/H2).
	auditAt := len(kept)
	if hasFinalNewline {
		// strings.Split represents the trailing newline as an empty final line;
		// place the audit before it so the original EOF convention is preserved.
		auditAt--
	}
	if start > 0 {
		for i := status + 1; i < len(kept); i++ {
			s := strings.TrimSpace(kept[i])
			if strings.HasPrefix(s, "### Task ") || strings.HasPrefix(s, "## ") {
				auditAt = i
				break
			}
		}
	}
	kept = append(kept[:auditAt], append([]string{audit}, kept[auditAt:]...)...)
	return []byte(strings.Join(kept, newline)), nil
}
