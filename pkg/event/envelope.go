package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// REQ:envelope-validation rules. Each regex is package-level so it is compiled
// exactly once at process startup; the rule strings are reused inside the
// ValidationError messages to satisfy the AC clause that asserts the message
// names the violated rule.
const (
	namePattern = `^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`
	uuidPattern = `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
)

var (
	nameRegex = regexp.MustCompile(namePattern)
	uuidRegex = regexp.MustCompile(uuidPattern)

	actorKinds    = []string{"skill", "user", "external"}
	artifactTypes = []string{"idea", "feature", "plan", "task", "lesson", "idea-seed", "consilium-review"}
)

// ValidationError is the deterministic error returned by Validate. The
// dispatcher prints its Error() string verbatim to stderr; the
// envelope-validation ACs assert that the message names both the offending
// Field and the Rule violated, so both elements MUST appear in the formatted
// string.
type ValidationError struct {
	// Field is the dotted JSON path of the offending field (e.g. "name",
	// "actor.kind", "artifact.type").
	Field string
	// Rule is a human-readable description of the rule that was violated;
	// regex rules embed the pattern verbatim so callers can grep for it.
	Rule string
	// Value is the offending value rendered for stderr. Empty when the
	// rule is "must not be empty" and the field is a string.
	Value string
}

// Error formats the validation error as `envelope validation failed: field=<f>
// value=<v> rule=<r>`. The shape is stable so tests can string-match on it.
func (e *ValidationError) Error() string {
	if e.Value == "" {
		return fmt.Sprintf("envelope validation failed: field=%s rule=%s", e.Field, e.Rule)
	}
	return fmt.Sprintf("envelope validation failed: field=%s value=%q rule=%s", e.Field, e.Value, e.Rule)
}

// Validate checks an Event against REQ:envelope-validation. It is pure: no
// I/O, no clock access. On a valid envelope it returns nil; otherwise it
// returns a *ValidationError naming the first failing field and the rule
// violated. Payload field-level inspection is explicitly out — the function
// only confirms the payload bytes parse as a JSON object.
func Validate(e Event) error {
	if !nameRegex.MatchString(e.Name) {
		return &ValidationError{
			Field: "name",
			Value: e.Name,
			Rule:  "must match " + namePattern,
		}
	}

	if e.Version < 1 {
		return &ValidationError{
			Field: "version",
			Value: fmt.Sprintf("%d", e.Version),
			Rule:  "must be a positive integer (>=1)",
		}
	}

	if !uuidRegex.MatchString(e.UUID) {
		return &ValidationError{
			Field: "uuid",
			Value: e.UUID,
			Rule:  "must match lowercase UUID v4 " + uuidPattern,
		}
	}

	// Timestamp must be UTC. time.Time.Location().String() is "UTC" when the
	// timestamp was parsed with a Z suffix or +00:00 offset via RFC 3339; any
	// other offset is rejected. A zero-valued time also fails this check.
	if e.Timestamp.IsZero() {
		return &ValidationError{
			Field: "timestamp",
			Rule:  "must be a non-zero RFC 3339 UTC timestamp",
		}
	}
	if _, offset := e.Timestamp.Zone(); offset != 0 {
		return &ValidationError{
			Field: "timestamp",
			Value: e.Timestamp.String(),
			Rule:  "must be UTC (Z or +00:00 offset)",
		}
	}

	if !contains(actorKinds, e.Actor.Kind) {
		return &ValidationError{
			Field: "actor.kind",
			Value: e.Actor.Kind,
			Rule:  "must be one of " + strings.Join(actorKinds, ", "),
		}
	}
	if e.Actor.ID == "" {
		return &ValidationError{
			Field: "actor.id",
			Rule:  "must be a non-empty string",
		}
	}

	if !contains(artifactTypes, e.Artifact.Type) {
		return &ValidationError{
			Field: "artifact.type",
			Value: e.Artifact.Type,
			Rule:  "must be one of " + strings.Join(artifactTypes, ", "),
		}
	}
	if e.Artifact.ID == "" {
		return &ValidationError{
			Field: "artifact.id",
			Rule:  "must be a non-empty string",
		}
	}
	if e.Artifact.Path == "" {
		return &ValidationError{
			Field: "artifact.path",
			Rule:  "must be a non-empty string",
		}
	}
	if e.Artifact.Revision == "" {
		return &ValidationError{
			Field: "artifact.revision",
			Rule:  "must be a non-empty string (literal `uncommitted` is permitted)",
		}
	}

	// Payload must parse as a JSON object. We deliberately do NOT decode into
	// a typed structure or round-trip through json.Marshal — the dispatcher
	// delivers the original bytes verbatim (REQ:payload-opaque). A cheap
	// prefix check rejects arrays/scalars; json.Valid then catches malformed
	// bytes without inspecting field names.
	trimmed := bytes.TrimLeft(e.Payload, " \t\n\r")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return &ValidationError{
			Field: "payload",
			Rule:  "must parse as a JSON object",
		}
	}
	if !json.Valid(e.Payload) {
		return &ValidationError{
			Field: "payload",
			Rule:  "must parse as a JSON object",
		}
	}

	return nil
}

// contains reports whether s appears in haystack. Inlined to avoid pulling in
// slices.Contains for a six-element list and to keep Go 1.20 compatibility.
func contains(haystack []string, s string) bool {
	for _, h := range haystack {
		if h == s {
			return true
		}
	}
	return false
}
