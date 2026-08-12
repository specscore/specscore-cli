package event

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// validEvent returns a fully-valid envelope used as the base for negative
// tests. Each test mutates a single field to assert that rule's enforcement.
func validEvent() Event {
	ts, _ := time.Parse(time.RFC3339, "2026-05-22T10:00:00Z")
	return Event{
		Name:      "idea.drafted",
		Version:   1,
		UUID:      "00000000-0000-4000-8000-000000000000",
		Timestamp: ts,
		Actor:     Actor{Kind: "skill", ID: "specstudio:ideate"},
		Artifact:  Artifact{Type: "idea", ID: "demo", Path: "spec/ideas/demo.md", Revision: "uncommitted"},
		Payload:   json.RawMessage(`{}`),
	}
}

// TestValidate_PositiveCase asserts a fully-valid envelope passes validation.
func TestValidate_PositiveCase(t *testing.T) {
	if err := Validate(validEvent()); err != nil {
		t.Fatalf("Validate(valid) = %v, want nil", err)
	}
}

// TestValidate_RejectsBadName verifies AC:envelope-validation-rejects-bad-name.
// An uppercase name MUST be rejected; the error message MUST name the field
// (`name`) and the pattern rule.
func TestValidate_RejectsBadName(t *testing.T) {
	e := validEvent()
	e.Name = "Idea.Drafted"

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(bad name) = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "name") {
		t.Errorf("error message %q does not contain field name 'name'", msg)
	}
	if !strings.Contains(msg, `^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`) {
		t.Errorf("error message %q does not contain pattern rule", msg)
	}
}

// TestValidate_RejectsBadActorKind verifies
// AC:envelope-validation-rejects-bad-actor-kind. An out-of-enum actor kind
// MUST be rejected; the error MUST name the field, the offending value, and
// the three accepted values.
func TestValidate_RejectsBadActorKind(t *testing.T) {
	e := validEvent()
	e.Actor.Kind = "robot"

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(bad actor.kind) = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "actor.kind") {
		t.Errorf("error message %q does not contain field name 'actor.kind'", msg)
	}
	if !strings.Contains(msg, "robot") {
		t.Errorf("error message %q does not contain offending value 'robot'", msg)
	}
	for _, want := range []string{"skill", "user", "external"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not contain accepted value %q", msg, want)
		}
	}
}

// TestValidate_PayloadOpaque verifies the validator's contribution to
// AC:payload-is-opaque-passthrough. Arbitrary, unknown payload fields MUST
// NOT be rejected; the validator confirms only that the bytes parse as a
// JSON object.
func TestValidate_PayloadOpaque(t *testing.T) {
	e := validEvent()
	e.Payload = json.RawMessage(`{"made_up_field_1":[1,2,3],"nested":{"anything":null},"unicode":"héllo 🎉"}`)

	if err := Validate(e); err != nil {
		t.Fatalf("Validate(opaque payload) = %v, want nil", err)
	}
}

// TestValidate_RejectsBadVersion exercises the positive-integer rule for
// version.
func TestValidate_RejectsBadVersion(t *testing.T) {
	e := validEvent()
	e.Version = 0

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(version=0) = nil, want error")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error %q does not name 'version'", err.Error())
	}
}

// TestValidate_RejectsBadUUID exercises the UUID v4 regex rule.
func TestValidate_RejectsBadUUID(t *testing.T) {
	e := validEvent()
	e.UUID = "not-a-uuid"

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(bad uuid) = nil, want error")
	}
	if !strings.Contains(err.Error(), "uuid") {
		t.Errorf("error %q does not name 'uuid'", err.Error())
	}
}

// TestValidate_RejectsNonUTCTimestamp asserts a non-UTC timestamp fails.
func TestValidate_RejectsNonUTCTimestamp(t *testing.T) {
	e := validEvent()
	loc := time.FixedZone("EST", -5*3600)
	e.Timestamp = time.Date(2026, 5, 22, 10, 0, 0, 0, loc)

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(non-UTC timestamp) = nil, want error")
	}
	if !strings.Contains(err.Error(), "timestamp") {
		t.Errorf("error %q does not name 'timestamp'", err.Error())
	}
}

// TestValidate_RejectsBadArtifactType exercises the artifact.type enum.
func TestValidate_RejectsBadArtifactType(t *testing.T) {
	e := validEvent()
	e.Artifact.Type = "blueprint"

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(bad artifact.type) = nil, want error")
	}
	if !strings.Contains(err.Error(), "artifact.type") {
		t.Errorf("error %q does not name 'artifact.type'", err.Error())
	}
}

func TestValidate_AcceptsLessonArtifactType(t *testing.T) {
	e := validEvent()
	e.Artifact = Artifact{Type: "lesson", ID: "retry-safety", Path: "spec/lessons/retry-safety/README.md", Revision: "uncommitted"}
	if err := Validate(e); err != nil {
		t.Fatalf("Validate(lesson artifact) = %v, want nil", err)
	}
}

// TestValidate_RejectsEmptyStringFields walks each required non-empty string
// field and confirms the empty value is rejected with that field named.
func TestValidate_RejectsEmptyStringFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Event)
		field  string
	}{
		{"actor.id", func(e *Event) { e.Actor.ID = "" }, "actor.id"},
		{"artifact.id", func(e *Event) { e.Artifact.ID = "" }, "artifact.id"},
		{"artifact.path", func(e *Event) { e.Artifact.Path = "" }, "artifact.path"},
		{"artifact.revision", func(e *Event) { e.Artifact.Revision = "" }, "artifact.revision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := validEvent()
			tc.mutate(&e)
			err := Validate(e)
			if err == nil {
				t.Fatalf("Validate(empty %s) = nil, want error", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q does not name %q", err.Error(), tc.field)
			}
		})
	}
}

// TestValidate_RejectsNonObjectPayload covers the JSON-object rule for
// payload. An array is valid JSON but not a JSON object.
func TestValidate_RejectsNonObjectPayload(t *testing.T) {
	e := validEvent()
	e.Payload = json.RawMessage(`[1,2,3]`)

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(array payload) = nil, want error")
	}
	if !strings.Contains(err.Error(), "payload") {
		t.Errorf("error %q does not name 'payload'", err.Error())
	}
}

// TestValidate_RejectsZeroTimestamp asserts a zero-valued timestamp fails.
func TestValidate_RejectsZeroTimestamp(t *testing.T) {
	e := validEvent()
	e.Timestamp = time.Time{} // zero value

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(zero timestamp) = nil, want error")
	}
	if !strings.Contains(err.Error(), "timestamp") {
		t.Errorf("error %q does not name 'timestamp'", err.Error())
	}
}

// TestValidate_RejectsEmptyPayload asserts that an empty payload is rejected.
func TestValidate_RejectsEmptyPayload(t *testing.T) {
	e := validEvent()
	e.Payload = json.RawMessage(``)

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(empty payload) = nil, want error")
	}
	if !strings.Contains(err.Error(), "payload") {
		t.Errorf("error %q does not name 'payload'", err.Error())
	}
}

// TestValidate_RejectsInvalidJSONPayload asserts that bytes starting with '{'
// but not valid JSON are rejected.
func TestValidate_RejectsInvalidJSONPayload(t *testing.T) {
	e := validEvent()
	e.Payload = json.RawMessage(`{bad json`)

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(invalid JSON payload) = nil, want error")
	}
	if !strings.Contains(err.Error(), "payload") {
		t.Errorf("error %q does not name 'payload'", err.Error())
	}
}

// TestValidate_RejectsScalarPayload asserts that a JSON scalar (not an object)
// is rejected.
func TestValidate_RejectsScalarPayload(t *testing.T) {
	e := validEvent()
	e.Payload = json.RawMessage(`42`)

	err := Validate(e)
	if err == nil {
		t.Fatalf("Validate(scalar payload) = nil, want error")
	}
	if !strings.Contains(err.Error(), "payload") {
		t.Errorf("error %q does not name 'payload'", err.Error())
	}
}

// TestValidationError_Format verifies both branches of ValidationError.Error()
// (with and without Value).
func TestValidationError_Format(t *testing.T) {
	t.Run("with_value", func(t *testing.T) {
		ve := &ValidationError{Field: "name", Value: "Bad", Rule: "must match pattern"}
		got := ve.Error()
		if !strings.Contains(got, "name") || !strings.Contains(got, `"Bad"`) || !strings.Contains(got, "must match pattern") {
			t.Fatalf("Error() = %q, missing expected components", got)
		}
	})
	t.Run("without_value", func(t *testing.T) {
		ve := &ValidationError{Field: "actor.id", Rule: "must be a non-empty string"}
		got := ve.Error()
		if !strings.Contains(got, "actor.id") || !strings.Contains(got, "must be a non-empty string") {
			t.Fatalf("Error() = %q, missing expected components", got)
		}
		// When Value is empty, no value= should appear.
		if strings.Contains(got, "value=") {
			t.Fatalf("Error() = %q, should not contain 'value=' when Value is empty", got)
		}
	})
}
