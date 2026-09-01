package event

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// configFileName is the per-project SpecScore config filename. It is mirrored
// verbatim from pkg/projectdef so that error messages here use the literal
// "specscore.yaml" string asserted by AC:events-config-unknown-type-rejected
// without taking a package dependency in either direction.
const configFileName = "specscore.yaml"

// defaultJsonlRelPath is the path synthesized when the events: block is absent.
// Per REQ:default-and-empty-config the loader returns exactly one JsonlWriter
// at <project-root>/.specscore/events.jsonl.
var defaultJsonlRelPath = filepath.Join(".specscore", "events.jsonl")

// defaultExecTimeoutMs is the timeout applied to exec subscribers when
// timeout_ms is omitted. The [100, 30000] bounds check applies only when the
// field is present; the default is in-range.
const defaultExecTimeoutMs = 2000

// subscriberKindEnum is the closed set of accepted `type:` values, kept in
// stable order so error messages list them deterministically.
var subscriberKindEnum = []string{"jsonl", "noop", "exec"}

// LoadSubscribers parses <projectRoot>/specscore.yaml and returns the
// configured Subscriber list. When the file does not exist OR exists but does
// not contain an `events:` key, the default JsonlWriter at
// `.specscore/events.jsonl` is synthesized. An explicit `events: {subscribers:
// []}` is honored as the zero-subscriber list (no synthesis).
func LoadSubscribers(projectRoot string) ([]Subscriber, error) {
	path := filepath.Join(projectRoot, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No config file → treat as absent events: block.
			return defaultSubscribers(projectRoot), nil
		}
		return nil, fmt.Errorf("read %s: %w", configFileName, err)
	}

	// Two-step parse so we can distinguish "events: omitted" from
	// "events: {subscribers: []}". yaml.v3 collapses both to a zero-value
	// struct when decoded directly.
	present, raw, err := extractEventsNode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: parse: %w", configFileName, err)
	}
	if !present {
		return defaultSubscribers(projectRoot), nil
	}

	var block eventsBlock
	if err := raw.Decode(&block); err != nil {
		return nil, fmt.Errorf("%s: events: %w", configFileName, err)
	}
	if block.Subscribers == nil {
		// `events:` present but no `subscribers:` key — treat as empty list
		// (operator opted into the events block but listed nothing).
		return []Subscriber{}, nil
	}

	subs := make([]Subscriber, 0, len(block.Subscribers))
	for i, raw := range block.Subscribers {
		s, err := buildSubscriber(i, raw, projectRoot)
		if err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, nil
}

// defaultSubscribers builds the default-when-absent subscriber list:
// a single JsonlWriter at <projectRoot>/.specscore/events.jsonl.
func defaultSubscribers(projectRoot string) []Subscriber {
	return []Subscriber{NewJsonlWriter(defaultJsonlRelPath, projectRoot)}
}

// ConfiguredLedgerPath returns the project's configured JSONL event ledger.
// A project may configure at most one JSONL sink for commands that merge
// ledgers; multiple JSONL sinks are ambiguous and fail closed. Projects with
// only non-file subscribers have no merge target.
func ConfiguredLedgerPath(projectRoot string) (string, error) {
	subscribers, err := LoadSubscribers(projectRoot)
	if err != nil {
		return "", err
	}
	var path string
	for _, subscriber := range subscribers {
		writer, ok := subscriber.(*JsonlWriter)
		if !ok {
			continue
		}
		if path != "" {
			return "", fmt.Errorf("multiple jsonl event ledgers are configured; merge target is ambiguous")
		}
		path = writer.Path()
	}
	if path == "" {
		return "", fmt.Errorf("no jsonl event ledger is configured")
	}
	return path, nil
}

// EventLedgerPath is an alias for ConfiguredLedgerPath for callers that use
// the event terminology directly.
func EventLedgerPath(projectRoot string) (string, error) {
	return ConfiguredLedgerPath(projectRoot)
}

// eventsBlock is the deserialization shape of the `events:` mapping. The
// subscriber list is kept as a slice of raw mappings so we can validate the
// `type:` discriminator before decoding the type-specific fields.
type eventsBlock struct {
	Subscribers []map[string]any `yaml:"subscribers"`
}

// extractEventsNode walks the top-level mapping and returns the raw yaml.Node
// for the `events:` key when present. present=false signals the key was
// omitted entirely (vs. present-with-null/explicit-empty).
func extractEventsNode(data []byte) (present bool, node *yaml.Node, err error) {
	var doc yaml.Node
	if uerr := yaml.Unmarshal(data, &doc); uerr != nil {
		return false, nil, uerr
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return false, nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false, nil, nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		v := root.Content[i+1]
		if k.Value == "events" {
			return true, v, nil
		}
	}
	return false, nil, nil
}

// buildSubscriber turns one raw subscriber mapping into a typed Subscriber.
// Per REQ:events-config-schema, unknown `type` values and missing required
// fields produce errors that name the file, key path, offending value, and
// accepted enum (where applicable).
func buildSubscriber(idx int, raw map[string]any, projectRoot string) (Subscriber, error) {
	typeVal, _ := raw["type"].(string)
	switch typeVal {
	case "jsonl":
		path, _ := raw["path"].(string)
		if path == "" {
			return nil, configErrorf(
				"events.subscribers[%d].path", idx,
				"missing required field for type=jsonl",
			)
		}
		return NewJsonlWriter(path, projectRoot), nil

	case "noop":
		return NoOp{}, nil

	case "exec":
		argv, err := decodeArgv(raw["command"])
		if err != nil || len(argv) == 0 {
			return nil, configErrorf(
				"events.subscribers[%d].command", idx,
				"missing required non-empty argv list for type=exec",
			)
		}
		env, err := decodeEnv(raw["env"])
		if err != nil {
			return nil, configErrorf(
				"events.subscribers[%d].env", idx,
				"must be a mapping of string to string",
			)
		}
		timeoutMs := defaultExecTimeoutMs
		if v, ok := raw["timeout_ms"]; ok && v != nil {
			n, ok := toInt(v)
			if !ok {
				return nil, configErrorf(
					"events.subscribers[%d].timeout_ms", idx,
					fmt.Sprintf("must be an integer, got %v", v),
				)
			}
			if n < 100 || n > 30000 {
				return nil, configErrorf(
					"events.subscribers[%d].timeout_ms", idx,
					fmt.Sprintf("must be in [100, 30000], got %d", n),
				)
			}
			timeoutMs = n
		}
		return NewExec(argv, env, time.Duration(timeoutMs)*time.Millisecond), nil

	default:
		return nil, fmt.Errorf(
			"%s: events.subscribers[%d].type: unknown value %q; expected one of %s",
			configFileName, idx, typeVal, enumList(),
		)
	}
}

// configErrorf builds the standard error form: <file>: <keyPath>: <message>.
// All non-enum errors funnel through this so the format stays stable.
func configErrorf(keyPathFmt string, idx int, message string) error {
	return fmt.Errorf("%s: %s: %s", configFileName, fmt.Sprintf(keyPathFmt, idx), message)
}

// enumList renders the accepted `type:` enum as a comma-separated string in
// the order declared by subscriberKindEnum.
func enumList() string {
	s := ""
	for i, k := range subscriberKindEnum {
		if i > 0 {
			s += ", "
		}
		s += k
	}
	return s
}

// decodeArgv normalizes a raw YAML value into a string slice. Accepts only
// sequences of strings; anything else yields an error.
func decodeArgv(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	seq, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("not a sequence")
	}
	out := make([]string, 0, len(seq))
	for _, item := range seq {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("argv item not a string")
		}
		out = append(out, s)
	}
	return out, nil
}

// decodeEnv normalizes a raw YAML value into a string→string map. Accepts
// only mappings whose keys and values stringify cleanly; anything else yields
// an error.
func decodeEnv(v any) (map[string]string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not a mapping")
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("env value for %q not a string", k)
		}
		out[k] = s
	}
	return out, nil
}

// toInt extracts an integer from a YAML-decoded value. yaml.v3 may surface
// numeric scalars as int or float64 depending on quoting, so both shapes are
// accepted; non-numeric values return ok=false.
func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		// Reject non-integral floats so `timeout_ms: 1.5` isn't silently
		// truncated.
		if x != float64(int(x)) {
			return 0, false
		}
		return int(x), true
	}
	return 0, false
}
