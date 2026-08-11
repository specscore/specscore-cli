package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/gitremote"
	"github.com/spf13/cobra"
)

// eventCommand returns the "event" command group — emits SpecScore events
// onto the JSONL bus and (in later tasks) dispatches them to configured
// subscribers. See spec/features/cli/event/README.md.
func eventCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Event emission — append SpecScore events to .specscore/events.jsonl",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(eventEmitCommand())
	cmd.AddCommand(eventReplayCommand())
	cmd.AddCommand(eventReconcileCommand())
	return cmd
}

func eventReplayCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "replay", Short: "Replay pending durable subscriber deliveries", Long: "Reconstructs pending delivery from the immutable committed ledger, then replays only the named subscriber's unacknowledged entries in ledger timestamp/UUID order. --from starts inclusively at a committed event and remains valid after that event is acknowledged. Success for one sink never acknowledges another. Delivery is at-least-once, so subscribers must deduplicate the event UUID. Interrupted prepared records remain visible for explicit reconciliation. Docs: docs/agent-lessons.md#durable-event-delivery", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true, RunE: runEventReplay}
	cmd.Flags().String("subscriber", "", "stable subscriber name (required)")
	cmd.Flags().String("from", "", "inclusive committed event UUID cursor")
	cmd.Flags().Int("limit", 0, "maximum pending entries to attempt (0 is all)")
	return cmd
}
func runEventReplay(cmd *cobra.Command, _ []string) error {
	return runEventReplayWithDeps(cmd, defaultEventCLIDeps())
}

type eventCLIOutbox interface {
	Replay(context.Context, []event.Subscriber, string, int) (event.ReplayResult, error)
	ReplayFrom(context.Context, []event.Subscriber, string, string, int) (event.ReplayResult, error)
	Prepared() ([]event.PreparedRecord, error)
	Commit(string) error
	Abort(string) error
	Enqueue(event.Event, []event.Subscriber) error
}

type eventCLIDeps struct {
	getwd     func() (string, error)
	findRoot  func(string) (string, error)
	load      func(string) ([]event.Subscriber, error)
	outboxFor func(string) eventCLIOutbox
}

func defaultEventCLIDeps() eventCLIDeps {
	return eventCLIDeps{osGetwdFn, findRepoConfigRoot, event.LoadSubscribers, func(root string) eventCLIOutbox { return event.NewOutbox(root) }}
}

func runEventReplayWithDeps(cmd *cobra.Command, deps eventCLIDeps) error {
	name, _ := cmd.Flags().GetString("subscriber")
	if name == "" {
		return exitcode.InvalidArgsError("--subscriber is required")
	}
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 0 {
		return exitcode.InvalidArgsError("--limit must be >= 0")
	}
	from, _ := cmd.Flags().GetString("from")
	if from != "" {
		if _, err := uuid.Parse(from); err != nil {
			return exitcode.InvalidArgsErrorf("--from must be a valid event UUID: %v", err)
		}
	}
	start, err := deps.getwd()
	if err != nil {
		return exitcode.UnexpectedErrorf("getwd: %v", err)
	}
	root, err := deps.findRoot(start)
	if err != nil {
		return err
	}
	subs, err := deps.load(root)
	if err != nil {
		return exitcode.InvalidArgsErrorf("%v", err)
	}
	r, err := deps.outboxFor(root).ReplayFrom(cmd.Context(), subs, name, from, limit)
	if err != nil {
		return exitcode.UnexpectedErrorf("replaying outbox: %v", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "delivered=%d failed=%d pending=%d prepared=%d\n", r.Delivered, len(r.Failed), r.Pending, len(r.Prepared))
	for _, prepared := range r.Prepared {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "prepared event requires reconciliation: id=%s name=%s artifact=%s artifact_exists=%t; inspect it, then run `specscore event reconcile %s --decision commit|abort` for a confirmation token\n", prepared.EventUUID, prepared.EventName, prepared.ArtifactPath, prepared.ArtifactExists, prepared.EventUUID)
	}
	if len(r.Failed) > 0 {
		return exitcode.UnexpectedErrorf("%d delivery attempt(s) remain pending", len(r.Failed))
	}
	return nil
}

func eventReconcileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "reconcile <event-uuid>",
		Short:         "Resolve an interrupted prepared event after inspecting artifact evidence",
		Long:          "Previews the immutable event and whether its repository-relative artifact path exists. Commit makes every recorded subscriber delivery pending; abort retains an auditable non-deliverable record. The exact preview token is required, so reconciliation is never inferred from path existence alone. Docs: docs/agent-lessons.md#durable-event-delivery",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runEventReconcile,
	}
	cmd.Flags().String("decision", "", "reconciliation decision: commit or abort (required)")
	cmd.Flags().String("confirm", "", "exact confirmation token from the preview")
	return cmd
}

func runEventReconcile(cmd *cobra.Command, args []string) error {
	return runEventReconcileWithDeps(cmd, args, defaultEventCLIDeps())
}

func runEventReconcileWithDeps(cmd *cobra.Command, args []string, deps eventCLIDeps) error {
	decision, _ := cmd.Flags().GetString("decision")
	if decision != "commit" && decision != "abort" {
		return exitcode.InvalidArgsError("--decision must be commit or abort")
	}
	start, err := deps.getwd()
	if err != nil {
		return exitcode.UnexpectedErrorf("getwd: %v", err)
	}
	root, err := deps.findRoot(start)
	if err != nil {
		return err
	}
	outbox := deps.outboxFor(root)
	prepared, err := outbox.Prepared()
	if err != nil {
		return exitcode.UnexpectedErrorf("inspecting prepared events: %v", err)
	}
	var target *event.PreparedRecord
	for i := range prepared {
		if prepared[i].EventUUID == args[0] {
			target = &prepared[i]
			break
		}
	}
	if target == nil {
		return exitcode.NotFoundErrorf("prepared event not found: %s", args[0])
	}
	token := event.ReconciliationToken(*target, decision)
	confirm, _ := cmd.Flags().GetString("confirm")
	if confirm != token {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "preview: event=%s name=%s artifact=%s artifact_exists=%t decision=%s\nconfirmation-token: %s\n", target.EventUUID, target.EventName, target.ArtifactPath, target.ArtifactExists, decision, token)
		return exitcode.InvalidArgsError("reconciliation requires the exact --confirm token from this preview")
	}
	if decision == "commit" {
		err = outbox.Commit(target.EventUUID)
	} else {
		err = outbox.Abort(target.EventUUID)
	}
	if err != nil {
		return exitcode.UnexpectedErrorf("reconciling prepared event: %v", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", target.EventUUID, decision)
	return nil
}

// eventEmitCommand returns `specscore event emit` — the user-facing emission
// verb. See spec/features/cli/event/emit/README.md.
//
// This task wires only the seven REQ:envelope-flags and cobra plumbing
// (help text, required-flag enforcement, no-positional-args). Payload
// reading (REQ:payload-input-modes), envelope auto-fill, and dispatch
// land in subsequent tasks.
func eventEmitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Emit a SpecScore event",
		Long: `Emits a SpecScore event by constructing an envelope from the supplied
flags and dispatching it to configured subscribers (per spec/features/
cli/event/README.md).

The envelope's stable fields come from the seven flags below. Bookkeeping
fields (version, uuid, timestamp, artifact.revision) are auto-filled when
not supplied. The payload bytes are read via one of three input modes
(--payload-json, --payload-file, or stdin).

Every valid event is first committed to the durable generic outbox with the
complete subscriber set, then replayed independently per subscriber. Delivery
is at-least-once and event UUID is the subscriber idempotency key.

The verb accepts flag-form input only; positional arguments are rejected
to keep the call shape stable across shells.

Docs: https://specscore.md/event-emit
`,
		Args: cobra.NoArgs,
		RunE: runEventEmit,
	}

	// REQ:envelope-flags — the seven envelope flags.
	// Required-flag enforcement is handled manually in RunE so that the
	// error path maps to exitcode.InvalidArgs (exit 2) per the AC, rather
	// than cobra's default exit-1 "required flag(s) not set" path. The
	// stderr message names both the flag and the envelope field it
	// supplies (per REQ:envelope-flags last paragraph).
	cmd.Flags().String("name", "", "event name (e.g. idea.drafted); supplies envelope field `name` (required)")
	cmd.Flags().String("actor-kind", "", "one of skill|user|external; supplies envelope field `actor.kind` (required)")
	cmd.Flags().String("actor-id", "", "actor identifier; supplies envelope field `actor.id` (required)")
	cmd.Flags().String("artifact-type", "", "one of idea|feature|plan|task|idea-seed|consilium-review; supplies envelope field `artifact.type` (required)")
	cmd.Flags().String("artifact-id", "", "artifact slug or id; supplies envelope field `artifact.id` (required)")
	cmd.Flags().String("artifact-path", "", "repo-relative path to the artifact; supplies envelope field `artifact.path` (required)")
	cmd.Flags().String("artifact-revision", "", "git SHA or the literal `uncommitted`; supplies envelope field `artifact.revision` (optional; overrides auto-fill)")

	// REQ:payload-input-modes — the two payload-source flags. Both are
	// optional in the cobra wiring; mode arbitration (at most one of the
	// two; stdin only when neither is set) lands in Task 5.
	cmd.Flags().String("payload-json", "", "inline JSON payload; the flag value IS the envelope `payload` bytes")
	cmd.Flags().String("payload-file", "", "path to a file containing the JSON payload (relative paths resolve against the project root)")

	return cmd
}

// resolvePayload returns the payload bytes for the envelope, using the
// first non-empty source in priority order: --payload-json flag value,
// --payload-file contents, then stdin to EOF.
//
// This task (4) covers the three happy-path ACs only. Mode conflicts
// (--payload-json AND --payload-file both set), TTY-stdin rejection,
// and JSON-parse validation land in Task 5 — so this function does
// NOT inspect the bytes nor detect mutually-exclusive flag usage.
//
// File-path resolution: relative --payload-file paths join against
// projectRoot; absolute paths are used verbatim.
func resolvePayload(payloadJSON, payloadFile string, stdin io.Reader, projectRoot string) (json.RawMessage, error) {
	if payloadJSON != "" {
		return json.RawMessage(payloadJSON), nil
	}
	if payloadFile != "" {
		path := payloadFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectRoot, path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// payloadModesHint is the canonical human-readable enumeration of the
// three accepted payload input modes; every exit-2 error in the payload
// arbitration path references it so callers see a consistent menu.
const payloadModesHint = "accepted payload input modes: --payload-json <bytes>, --payload-file <path>, or piped stdin"

// arbitratePayloadMode rejects ambiguous or unsupplied payload sources
// per cli/event/emit#ac:payload-mode-conflict-fails-2 and #ac:payload-
// tty-stdin-fails-2. Caller passes the two flag values plus two stdin
// state bits (TTY? has-data?) probed BEFORE any stdin read so that the
// verb never blocks on an unintended keyboard read.
//
// Returns nil when exactly one of {--payload-json, --payload-file, piped
// stdin} is the intended source.
func arbitratePayloadMode(payloadJSON, payloadFile string, stdinIsTTY bool, stdinHasData bool) error {
	_ = stdinHasData // reserved for future "flag-set-and-stdin-also-piped" rejection
	if payloadJSON != "" && payloadFile != "" {
		return exitcode.InvalidArgsErrorf(
			"cannot use --payload-json and --payload-file together; %s",
			payloadModesHint)
	}
	if payloadJSON == "" && payloadFile == "" && stdinIsTTY {
		return exitcode.InvalidArgsErrorf(
			"no payload supplied and stdin is a TTY (refusing to block on keyboard input); %s",
			payloadModesHint)
	}
	return nil
}

// validatePayloadJSON parses the resolved payload bytes and returns an
// exit-2 error when the bytes are not a JSON object (per the parent
// Feature's REQ:envelope-validation). The inputMode string (e.g.
// `--payload-json`, `--payload-file /tmp/p.json`, or `stdin`) is woven
// into the error so the user knows which source produced the bad bytes.
// Covers cli/event/emit#ac:payload-bad-json-fails-2.
func validatePayloadJSON(payload json.RawMessage, inputMode string) error {
	// Decode as the most permissive type (interface{}) first so the error
	// message carries Go's standard json parse description; then assert
	// the top-level value is a JSON object (map).
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return exitcode.InvalidArgsErrorf(
			"payload from %s is not valid JSON: %v",
			inputMode, err)
	}
	if _, ok := v.(map[string]any); !ok {
		return exitcode.InvalidArgsErrorf(
			"payload from %s is not a JSON object (got %T)",
			inputMode, v)
	}
	return nil
}

// stdinIsTTY reports whether os.Stdin is connected to a terminal (i.e.
// a character device). The check uses os.Stat rather than peeking bytes
// because a peek would consume input destined for the stdin reader path.
//
// Per the os package docs: a TTY has ModeCharDevice set; a pipe has
// ModeNamedPipe set (or neither flag, but never ModeCharDevice).
// stdinStatFn is a testable indirection for os.Stdin.Stat.
var stdinStatFn = func() (os.FileInfo, error) { return os.Stdin.Stat() }

func stdinIsTTY() bool {
	stat, err := stdinStatFn()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// envelopeRequiredFlag pairs a CLI flag with the envelope field it supplies,
// so the missing-required error message names both per REQ:envelope-flags.
type envelopeRequiredFlag struct {
	flag  string // e.g. "name"
	field string // e.g. "name" or "actor.kind"
}

var envelopeRequiredFlags = []envelopeRequiredFlag{
	{"name", "name"},
	{"actor-kind", "actor.kind"},
	{"actor-id", "actor.id"},
	{"artifact-type", "artifact.type"},
	{"artifact-id", "artifact.id"},
	{"artifact-path", "artifact.path"},
}

// autofillEnvelope populates the four bookkeeping fields of e that the
// caller is not required to supply: version, uuid, timestamp, and
// artifact.revision. Per cli/event/emit#ac:envelope-auto-fill-*:
//
//   - version is always 1.
//   - uuid is a fresh lowercase-hyphenated v4 (matches the AC regex).
//   - timestamp is time.Now().UTC() formatted RFC 3339 with the `Z`
//     suffix (Go's time.RFC3339 format yields `Z` for UTC times).
//   - artifact.revision is, in priority order: (1) the override
//     argument when non-empty, else (2) the output of `git rev-parse
//     HEAD` run from projectRoot, else (3) the literal string
//     "uncommitted" when git fails (no .git/, no commits, etc.).
//
// The function is intentionally pure-ish — it mutates only e and reads
// the filesystem only through gitremote.HeadSHA. End-to-end wiring
// (RunE → dispatch) lands in Task 6; this function is exercised
// directly by unit tests in this batch.
func autofillEnvelope(e *event.Event, projectRoot string, revisionOverride string) {
	e.Version = 1
	e.UUID = uuid.NewString()
	e.Timestamp = time.Now().UTC()

	switch {
	case revisionOverride != "":
		e.Artifact.Revision = revisionOverride
	default:
		sha, err := gitremote.HeadSHA(projectRoot)
		if err != nil {
			e.Artifact.Revision = "uncommitted"
		} else {
			e.Artifact.Revision = sha
		}
	}
}

// determineInputMode names the payload source for stderr/error messages so the
// user can tell which input produced bad bytes. Mirrors the priority order
// implemented in resolvePayload (flag > file > stdin).
func determineInputMode(payloadJSON, payloadFile string) string {
	switch {
	case payloadJSON != "":
		return "--payload-json"
	case payloadFile != "":
		return "--payload-file " + payloadFile
	default:
		return "stdin"
	}
}

func runEventEmit(cmd *cobra.Command, _ []string) error {
	return runEventEmitWithDeps(cmd, defaultEventCLIDeps())
}

func runEventEmitWithDeps(cmd *cobra.Command, deps eventCLIDeps) error {
	for _, rf := range envelopeRequiredFlags {
		v, _ := cmd.Flags().GetString(rf.flag)
		if v == "" {
			return exitcode.InvalidArgsErrorf(
				"missing required flag --%s (supplies envelope field `%s`)",
				rf.flag, rf.field)
		}
	}

	// Read flag values up front so the rest of the function reads as
	// straight-line composition of the helpers introduced in Tasks 3-5.
	flagName, _ := cmd.Flags().GetString("name")
	flagActorKind, _ := cmd.Flags().GetString("actor-kind")
	flagActorID, _ := cmd.Flags().GetString("actor-id")
	flagArtifactType, _ := cmd.Flags().GetString("artifact-type")
	flagArtifactID, _ := cmd.Flags().GetString("artifact-id")
	flagArtifactPath, _ := cmd.Flags().GetString("artifact-path")
	flagArtifactRevision, _ := cmd.Flags().GetString("artifact-revision")
	flagPayloadJSON, _ := cmd.Flags().GetString("payload-json")
	flagPayloadFile, _ := cmd.Flags().GetString("payload-file")

	// Discover the project root. Same heuristic the rest of the CLI uses
	// (`findRepoConfigRoot` in spec.go); missing root → exit 3 (NotFound).
	startDir, err := deps.getwd()
	if err != nil {
		return exitcode.UnexpectedErrorf("getwd: %v", err)
	}
	projectRoot, err := deps.findRoot(startDir)
	if err != nil {
		return err
	}

	// Arbitrate payload mode BEFORE reading any input — see Task 5.
	if err := arbitratePayloadMode(flagPayloadJSON, flagPayloadFile, stdinIsTTY(), false); err != nil {
		return err
	}

	// Resolve payload bytes (flag → file → stdin).
	payloadBytes, err := resolvePayload(flagPayloadJSON, flagPayloadFile, os.Stdin, projectRoot)
	if err != nil {
		return exitcode.InvalidArgsErrorf("reading payload: %v", err)
	}

	inputMode := determineInputMode(flagPayloadJSON, flagPayloadFile)
	if err := validatePayloadJSON(payloadBytes, inputMode); err != nil {
		return err
	}

	e := event.Event{
		Name:  flagName,
		Actor: event.Actor{Kind: flagActorKind, ID: flagActorID},
		Artifact: event.Artifact{
			Type: flagArtifactType,
			ID:   flagArtifactID,
			Path: flagArtifactPath,
		},
		Payload: payloadBytes,
	}
	autofillEnvelope(&e, projectRoot, flagArtifactRevision)

	subscribers, err := deps.load(projectRoot)
	if err != nil {
		return exitcode.InvalidArgsErrorf("%v", err)
	}
	// The explicit empty list disables event persistence and delivery. Durable
	// ledger records otherwise always carry a nonempty canonical recipient set.
	if len(subscribers) == 0 {
		return nil
	}

	outbox := deps.outboxFor(projectRoot)
	if err := outbox.Enqueue(e, subscribers); err != nil {
		return exitcode.UnexpectedErrorf("durably queueing event: %v", err)
	}
	result, err := outbox.Replay(cmd.Context(), subscribers, "", 0)
	if err != nil {
		return exitcode.UnexpectedErrorf("delivering event: %v", err)
	}
	for _, failure := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "event-delivery pending: subscriber=%s event=%s error=%q\n", failure.Name, e.Name, failure.Err.Error())
	}
	if len(result.Failed) > 0 {
		return exitcode.UnexpectedErrorf("%d subscriber delivery attempt(s) remain pending", len(result.Failed))
	}
	return nil
}
