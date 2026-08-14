package cli

// The Lesson agents surface intentionally knows nothing about an orchestrator.
// It reads an adapter-produced durable projection and, only on an explicit
// action, streams a generic external-resource request to one configured
// executable. Lesson semantics stop at this boundary: the adapter may map the
// project-relative resource reference into an orchestrator association, but
// the orchestrator must not parse SpecScore Doc-Kinds. Synchestra owns
// authorization, live transport, idempotency, receipts/retry, and resume audit;
// the executable owns only the generic mapping, public-interface invocation,
// and projection refresh write. SpecScore never opens an orchestrator database
// or queue.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

const lessonAgentsHookEnv = "SPECSCORE_LESSON_AGENTS_HOOK"

var canonicalLessonAgentsProjectRootForCommand = canonicalLessonAgentsProjectRoot

type lessonAgent struct {
	ID            string `json:"id" yaml:"id"`
	Role          string `json:"role" yaml:"role"`
	State         string `json:"state" yaml:"state"`
	ObservedAt    string `json:"observed_at" yaml:"observed_at"`
	URL           string `json:"url" yaml:"url"`
	SourceEventID string `json:"source_event_id" yaml:"source_event_id"`
}

type lessonAgentsProjection struct {
	Version string        `json:"version" yaml:"version"`
	Agents  []lessonAgent `json:"agents" yaml:"agents"`
}

type lessonAgentsRequest struct {
	Version string `json:"version"`
	Action  string `json:"action"`
	Project struct {
		Root string `json:"root"`
	} `json:"project"`
	ExternalResource struct {
		Ref      string `json:"ref"`
		Revision string `json:"revision"`
	} `json:"external_resource"`
	AgentID string `json:"agent_id,omitempty"`
	Text    string `json:"text,omitempty"`
}

type lessonAgentsHookRunner func(context.Context, string, string, []byte, io.Writer, io.Writer) error

func runLessonAgentsHook(ctx context.Context, hook, root string, payload []byte, stdout, stderr io.Writer) error {
	external := exec.CommandContext(ctx, hook)
	external.Dir = root
	external.Stdin = bytes.NewReader(payload)
	external.Stdout, external.Stderr = stdout, stderr
	return external.Run()
}

func lessonAgentsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "agents <lesson>", Short: "Read durable Lesson agent context or invoke an explicit external hook", Long: "Reads the adapter-produced canonical Lesson projection at spec/lessons/<slug>/agents.json without network access. The core never writes this operational projection. --refresh, --open, --message, and --resume invoke only the executable configured by SPECSCORE_LESSON_AGENTS_HOOK, sending project context plus a canonical external-resource reference on stdin and streaming its result. Lesson semantics remain inside SpecScore; the adapter's orchestrator stays generic. Synchestra owns authorization, live messaging, idempotency, receipts/retry, and resume audit; the adapter owns generic mapping, public-interface invocation, and atomic projection refresh.", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true, RunE: runLessonAgents}
	cmd.Flags().Bool("refresh", false, "ask the configured external hook to refresh its projection")
	cmd.Flags().String("open", "", "ask the configured external hook to open one agent")
	cmd.Flags().String("message", "", "ask the configured external hook to message one agent")
	cmd.Flags().String("text", "", "message text; required with --message")
	cmd.Flags().String("resume", "", "ask the configured external hook to resume one agent")
	cmd.Flags().String("format", "yaml", "output format for offline projection: yaml, json, text")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runLessonAgents(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	project, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(project)
	if err != nil {
		return err
	}
	root, err = canonicalLessonAgentsProjectRootForCommand(root)
	if err != nil {
		return err
	}
	path, err := lesson.ResolveLessonFile(filepath.Join(root, "spec", "lessons"), args[0])
	if err != nil {
		return err
	}
	l, err := lesson.Parse(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", args[0], err)
	}
	if !l.Canonical {
		return exitcode.InvalidStateErrorf("lesson %q is legacy flat form; migrate it before attaching a durable agent projection", args[0])
	}
	action, agentID, text, err := lessonAgentsAction(cmd)
	if err != nil {
		return err
	}
	if action != "read" {
		return invokeLessonAgentsHook(cmd, action, root, path, agentID, text)
	}
	projection, err := readLessonAgentsProjection(filepath.Join(filepath.Dir(path), "agents.json"))
	if os.IsNotExist(err) {
		return exitcode.NotFoundErrorf("no durable agent projection for lesson %q; run --refresh with a configured external hook", l.Slug)
	}
	if err != nil {
		return exitcode.InvalidStateErrorf("reading durable agent projection: %v", err)
	}
	return writeLessonAgentsProjection(cmd.OutOrStdout(), format, projection)
}

func canonicalLessonAgentsProjectRoot(root string) (string, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", exitcode.UnexpectedErrorf("resolving canonical project root: %v", err)
	}
	return canonical, nil
}

func lessonAgentsAction(cmd *cobra.Command) (string, string, string, error) {
	refresh, _ := cmd.Flags().GetBool("refresh")
	open, _ := cmd.Flags().GetString("open")
	message, _ := cmd.Flags().GetString("message")
	resume, _ := cmd.Flags().GetString("resume")
	n := 0
	if refresh {
		n++
	}
	if open != "" {
		n++
	}
	if message != "" {
		n++
	}
	if resume != "" {
		n++
	}
	if n > 1 {
		return "", "", "", exitcode.InvalidArgsError("--refresh, --open, --message, and --resume are mutually exclusive")
	}
	if n == 0 {
		return "read", "", "", nil
	}
	if refresh {
		return "refresh", "", "", nil
	}
	if open != "" {
		return "open", open, "", nil
	}
	if resume != "" {
		return "resume", resume, "", nil
	}
	text, _ := cmd.Flags().GetString("text")
	if strings.TrimSpace(text) == "" {
		return "", "", "", exitcode.InvalidArgsError("--text is required with --message")
	}
	return "message", message, text, nil
}

func readLessonAgentsProjection(path string) (lessonAgentsProjection, error) {
	var projection lessonAgentsProjection
	b, err := os.ReadFile(path)
	if err != nil {
		return projection, err
	}
	if err := json.Unmarshal(b, &projection); err != nil {
		return projection, fmt.Errorf("invalid JSON: %w", err)
	}
	if projection.Version != "1" {
		return projection, fmt.Errorf("unsupported projection version %q", projection.Version)
	}
	for _, agent := range projection.Agents {
		if strings.TrimSpace(agent.ID) == "" {
			return projection, fmt.Errorf("agent id is required")
		}
	}
	return projection, nil
}

func writeLessonAgentsProjection(w io.Writer, format string, projection lessonAgentsProjection) error {
	switch format {
	case "json":
		return newJSONEnc(w).Encode(projection)
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(projection); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	default:
		for _, agent := range projection.Agents {
			if _, err := fmt.Fprintf(w, "%s %s %s %s\n", agent.ID, agent.Role, agent.State, agent.URL); err != nil {
				return exitcode.UnexpectedErrorf("writing projection: %v", err)
			}
		}
		return nil
	}
}

func invokeLessonAgentsHook(cmd *cobra.Command, action, root, path, agentID, text string) error {
	return invokeLessonAgentsHookWithRunner(cmd, action, root, path, agentID, text, runLessonAgentsHook)
}

func invokeLessonAgentsHookWithRunner(cmd *cobra.Command, action, root, path, agentID, text string, run lessonAgentsHookRunner) error {
	hook := strings.TrimSpace(os.Getenv(lessonAgentsHookEnv))
	if hook == "" {
		return exitcode.InvalidStateErrorf("%s must name the external lesson-agents hook for --%s", lessonAgentsHookEnv, action)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading lesson revision: %v", err)
	}
	resourceRef, err := filepath.Rel(root, path)
	if err != nil || resourceRef == "." || resourceRef == ".." || strings.HasPrefix(resourceRef, ".."+string(filepath.Separator)) || filepath.IsAbs(resourceRef) {
		return exitcode.UnexpectedErrorf("resolving external resource reference for %s", path)
	}
	req := lessonAgentsRequest{Version: "2", Action: action, AgentID: agentID, Text: text}
	req.Project.Root = root
	req.ExternalResource.Ref = filepath.ToSlash(filepath.Clean(resourceRef))
	sum := sha256.Sum256(b)
	req.ExternalResource.Revision = fmt.Sprintf("sha256:%x", sum[:])
	payload, _ := json.Marshal(req) // request contains strings only; encoding cannot fail.
	// The selected project, not the caller's ambient CWD, is the hook's
	// canonical anchor. This makes --project deterministic for native adapters
	// that locate their configuration and durable state relative to a repo.
	if err := run(cmd.Context(), hook, root, payload, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return exitcode.UnexpectedErrorf("external lesson-agents hook failed: %v", err)
	}
	return nil
}
