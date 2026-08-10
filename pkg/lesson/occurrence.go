package lesson

// Canonical directory-form Lesson occurrences. This package deliberately owns
// only filesystem facts: callers supply context, while capture of ambient git
// state stays in the CLI adapter so library users never accidentally touch it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var occurrenceUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var gitCommit = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
var repositoryID = regexp.MustCompile(`^[^/\s]+/[^/\s]+/[^/\s]+$`)

// occurrenceForbiddenNames match the published schema policy. Callers must
// redact before write; validators reject unsafe shape instead of silently
// discarding fields.
var occurrenceForbiddenNames = map[string]struct{}{
	"prompt": {}, "raw_prompt": {}, "raw_log": {}, "log": {}, "diff": {},
	"transcript": {}, "authorization": {}, "token": {}, "password": {}, "secret": {},
}

var (
	githubToken = regexp.MustCompile(`(?i)gh[pousr]_[a-z0-9_]{20,}`)
	awsKey      = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
)

// ContentScanner is an integration seam for the repository-standard secret
// scanner. It is invoked for every persisted string both before write and
// while validating existing occurrences. It must return a value-free error.
var ContentScanner = func(_ string) error { return nil }

// Occurrence is the v1 on-disk JSON contract. Context intentionally remains a
// typed generic map so the CLI can preserve the format's vendor-neutral shape.
type Occurrence struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Summary       string         `json:"summary"`
	Context       map[string]any `json:"context"`
	Evidence      Evidence       `json:"evidence"`
	Redactions    []string       `json:"redactions"`
	Path          string         `json:"-"`
}

type Evidence struct {
	Kind string  `json:"kind"`
	Ref  *string `json:"ref"`
}

type AddOccurrenceOptions struct {
	LessonPath string
	Summary    string
	Context    map[string]any
	Evidence   Evidence
	Redactions []string
	Now        time.Time
}

func ValidateOccurrence(o Occurrence) error {
	if o.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if !occurrenceUUID.MatchString(o.ID) {
		return fmt.Errorf("id must be a lowercase UUID v4")
	}
	if o.OccurredAt.IsZero() || o.OccurredAt.Location() != time.UTC {
		return fmt.Errorf("occurred_at must be RFC-3339 UTC")
	}
	if n := utf8.RuneCountInString(o.Summary); n == 0 || n > 500 {
		return fmt.Errorf("summary must contain 1..500 Unicode code points")
	}
	if o.Context == nil {
		return fmt.Errorf("context is required")
	}
	if err := validateContext(o.Context); err != nil {
		return err
	}
	if err := validateEvidence(o.Evidence); err != nil {
		return err
	}
	if len(o.Redactions) > 20 {
		return fmt.Errorf("redactions has more than 20 entries")
	}
	for _, s := range o.Redactions {
		if err := validateBounded("redactions", s); err != nil {
			return err
		}
	}
	return nil
}

func validateBounded(field, s string) error {
	if n := utf8.RuneCountInString(s); n == 0 || n > 500 {
		return fmt.Errorf("%s must contain 1..500 Unicode code points", field)
	}
	lower := strings.ToLower(s)
	for _, banned := range []string{"-----begin", "authorization:", "authorization=", "api_key", "api-key", "access_token", "bearer ", "password:", "password=", "token:", "token="} {
		if strings.Contains(lower, banned) {
			return fmt.Errorf("%s contains a credential-bearing value", field)
		}
	}
	if githubToken.MatchString(s) || awsKey.MatchString(s) {
		return fmt.Errorf("%s contains a credential-bearing value", field)
	}
	if err := ContentScanner(s); err != nil {
		return fmt.Errorf("%s rejected by content scanner", field)
	}
	return nil
}

func validateContext(c map[string]any) error {
	allowed := map[string]bool{"repository": true, "git": true, "worktree": true, "execution": true}
	for k := range c {
		if _, forbidden := occurrenceForbiddenNames[strings.ToLower(k)]; forbidden {
			return fmt.Errorf("context contains forbidden content field %q", k)
		}
		if !allowed[k] {
			return fmt.Errorf("context has unsupported field %q", k)
		}
	}
	for _, key := range []string{"repository", "git", "worktree", "execution"} {
		v, ok := c[key]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			if key != "repository" {
				return fmt.Errorf("context.%s must be an object or null", key)
			}
			if err := validateBounded("context."+key, x); err != nil {
				return err
			}
			if key == "repository" && !repositoryID.MatchString(x) {
				return fmt.Errorf("context.repository must be host/org/repository")
			}
		case map[string]any:
			if err := validateContextObject(key, x); err != nil {
				return err
			}
		default:
			return fmt.Errorf("context.%s must be an object or null", key)
		}
	}
	return nil
}

func validateContextObject(kind string, values map[string]any) error {
	allowed := map[string]bool{}
	switch kind {
	case "git":
		allowed["commit"], allowed["branch"] = true, true
	case "worktree":
		allowed["path_hint"], allowed["id"] = true, true
	case "execution":
		allowed["kind"], allowed["id"] = true, true
	}
	for k, v := range values {
		if _, forbidden := occurrenceForbiddenNames[strings.ToLower(k)]; forbidden {
			return fmt.Errorf("context.%s contains forbidden content field %q", kind, k)
		}
		if !allowed[k] {
			return fmt.Errorf("context.%s has unsupported field %q", kind, k)
		}
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("context.%s.%s must be a string or null", kind, k)
		}
		if err := validateBounded("context."+kind+"."+k, s); err != nil {
			return err
		}
		switch kind + "." + k {
		case "git.commit":
			if !gitCommit.MatchString(s) {
				return fmt.Errorf("context.git.commit must be a lowercase Git SHA")
			}
		case "worktree.path_hint":
			if strings.HasPrefix(s, "/") || strings.Contains(s, "..") {
				return fmt.Errorf("context.worktree.path_hint must be relative or redacted")
			}
		case "execution.kind":
			if s != "interactive" && s != "automation" && s != "ci" && s != "unknown" {
				return fmt.Errorf("context.execution.kind must be interactive, automation, ci, or unknown")
			}
		}
	}
	return nil
}

func validateEvidence(e Evidence) error {
	if e.Kind != "url" && e.Kind != "path" && e.Kind != "command" && e.Kind != "none" {
		return fmt.Errorf("evidence.kind must be url, path, command, or none")
	}
	if e.Kind == "none" {
		if e.Ref != nil {
			return fmt.Errorf("evidence.ref must be null when evidence.kind is none")
		}
		return nil
	}
	if e.Ref == nil {
		return fmt.Errorf("evidence.ref is required when evidence.kind is %s", e.Kind)
	}
	return validateBounded("evidence.ref", *e.Ref)
}

// AddOccurrence writes exactly one new immutable child JSON file.
func AddOccurrence(opts AddOccurrenceOptions) (Occurrence, error) {
	l, err := Parse(opts.LessonPath)
	if err != nil {
		return Occurrence{}, err
	}
	if !l.Canonical {
		return Occurrence{}, fmt.Errorf("lesson %q is legacy flat form", l.Slug)
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.Context == nil {
		opts.Context = map[string]any{}
	}
	if opts.Evidence.Kind == "" {
		opts.Evidence = Evidence{Kind: "none"}
	}
	o := Occurrence{SchemaVersion: 1, ID: uuid.NewString(), OccurredAt: opts.Now.UTC(), Summary: strings.TrimSpace(opts.Summary), Context: opts.Context, Evidence: opts.Evidence, Redactions: opts.Redactions}
	if o.Summary == "" {
		o.Summary = "Lesson gap observed."
	}
	if err := ValidateOccurrence(o); err != nil {
		return Occurrence{}, err
	}
	if err := os.MkdirAll(l.OccurrencesDir, 0o755); err != nil {
		return Occurrence{}, err
	}
	o.Path = filepath.Join(l.OccurrencesDir, o.ID+".json")
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return Occurrence{}, err
	}
	f, err := os.OpenFile(o.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Occurrence{}, err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		_ = os.Remove(o.Path)
		return Occurrence{}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(o.Path)
		return Occurrence{}, err
	}
	return o, nil
}

// DiscoverOccurrences validates every child file and returns deterministic
// chronological order. A malformed child is an error, never silently ignored.
func DiscoverOccurrences(lessonPath string) ([]Occurrence, error) {
	l, err := Parse(lessonPath)
	if err != nil {
		return nil, err
	}
	if !l.Canonical {
		return nil, nil
	}
	entries, err := os.ReadDir(l.OccurrencesDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Occurrence
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(l.OccurrencesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var o Occurrence
		if err := validateOccurrenceRaw(data); err != nil {
			return nil, fmt.Errorf("invalid occurrence %s: %w", path, err)
		}
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return nil, fmt.Errorf("invalid occurrence %s: %w", path, err)
		}
		o.Path = path
		if strings.TrimSuffix(e.Name(), ".json") != o.ID {
			return nil, fmt.Errorf("occurrence filename and id differ: %s", path)
		}
		if err := ValidateOccurrence(o); err != nil {
			return nil, fmt.Errorf("invalid occurrence %s: %w", path, err)
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return filepath.Base(out[i].Path) < filepath.Base(out[j].Path)
		}
		return out[i].OccurredAt.Before(out[j].OccurredAt)
	})
	return out, nil
}

func validateOccurrenceRaw(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	timestamp, ok := raw["occurred_at"]
	if !ok {
		return fmt.Errorf("occurred_at is required")
	}
	var text string
	if err := json.Unmarshal(timestamp, &text); err != nil {
		return fmt.Errorf("occurred_at must be a string")
	}
	if !strings.HasSuffix(text, "Z") {
		return fmt.Errorf("occurred_at must use a UTC Z suffix")
	}
	return nil
}

func FindOccurrence(lessonPath, id string) (Occurrence, error) {
	occurrences, err := DiscoverOccurrences(lessonPath)
	if err != nil {
		return Occurrence{}, err
	}
	for _, o := range occurrences {
		if o.ID == id {
			return o, nil
		}
	}
	return Occurrence{}, os.ErrNotExist
}
