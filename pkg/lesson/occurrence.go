package lesson

// Canonical directory-form Lesson occurrences. This package deliberately owns
// only filesystem facts: callers supply context, while capture of ambient git
// state stays in the CLI adapter so library users never accidentally touch it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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
var occurredAtLexical = regexp.MustCompile(`^[0-9]{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12][0-9]|3[01])T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]{1,9})?Z$`)

// occurrenceForbiddenNames match the published schema policy. Callers must
// redact before write; validators reject unsafe shape instead of silently
// discarding fields.
var occurrenceForbiddenNames = map[string]struct{}{
	"prompt": {}, "original_prompt": {}, "user_prompt": {}, "agent_prompt": {},
	"raw_prompt": {}, "raw_log": {}, "log": {}, "diff": {},
	"transcript": {}, "authorization": {}, "token": {}, "password": {}, "secret": {},
}

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
	// ID is optional for ordinary writes. Deterministic importers may supply a
	// UUID v4-shaped content-derived identity to make replay idempotent.
	ID         string
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
	if err := validateBounded("summary", o.Summary); err != nil {
		return err
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
	seenRedactions := map[string]struct{}{}
	for _, s := range o.Redactions {
		if err := validateBounded("redactions", s); err != nil {
			return err
		}
		if _, exists := seenRedactions[s]; exists {
			return fmt.Errorf("redactions entries must be unique")
		}
		seenRedactions[s] = struct{}{}
	}
	return nil
}

func validateBounded(field, s string) error {
	if n := utf8.RuneCountInString(s); n == 0 || n > 500 {
		return fmt.Errorf("%s must contain 1..500 Unicode code points", field)
	}
	if strings.ContainsAny(s, "\r\n") {
		return fmt.Errorf("%s must be a single line", field)
	}
	lower := strings.ToLower(s)
	for _, banned := range []string{"-----begin", "authorization:", "authorization=", "api_key", "api-key", "access_token", "bearer ", "password:", "password=", "token:", "token="} {
		if strings.Contains(lower, banned) {
			return fmt.Errorf("%s contains a credential-bearing value", field)
		}
	}
	return ValidateSafeContent(field, s)
}

func validateContext(c map[string]any) error {
	return validateContextWithRuntime(c, json.Marshal, decodeNormalizedContext)
}

func decodeNormalizedContext(b []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.UseNumber()
	var normalized map[string]any
	err := decoder.Decode(&normalized)
	return normalized, err
}

func validateContextWithRuntime(c map[string]any, marshal func(any) ([]byte, error), decode func([]byte) (map[string]any, error)) error {
	// Normalize through JSON so library callers may use ordinary typed numeric
	// and slice values while validation still reasons about the exact JSON
	// shape that will be persisted.
	b, err := marshal(c)
	if err != nil {
		return fmt.Errorf("context must contain only JSON values: %w", err)
	}
	normalized, err := decode(b)
	if err != nil {
		return fmt.Errorf("context must contain only JSON values: %w", err)
	}
	if err := validateOpaqueContextValue("context", normalized); err != nil {
		return err
	}
	for _, key := range []string{"repository", "git", "worktree", "execution"} {
		v, ok := normalized[key]
		if !ok || v == nil {
			continue
		}
		if key == "repository" {
			x, ok := v.(string)
			if !ok {
				return fmt.Errorf("context.repository must be a string or null")
			}
			if err := validateBounded("context."+key, x); err != nil {
				return err
			}
			if !repositoryID.MatchString(x) {
				return fmt.Errorf("context.repository must be host/org/repository")
			}
			continue
		}
		x, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("context.%s must be an object or null", key)
		}
		if err := validateContextObject(key, x); err != nil {
			return err
		}
	}
	return nil
}

func validateOpaqueContextValue(field string, value any) error {
	switch x := value.(type) {
	case nil, bool, json.Number:
		return nil
	case string:
		return validateContextText(field, x)
	case []any:
		if len(x) > 20 {
			return fmt.Errorf("%s has more than 20 entries", field)
		}
		for i, item := range x {
			if err := validateOpaqueContextValue(fmt.Sprintf("%s[%d]", field, i), item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		if len(x) > 20 {
			return fmt.Errorf("%s has more than 20 entries", field)
		}
		for key, item := range x {
			if _, forbidden := occurrenceForbiddenNames[strings.ToLower(key)]; forbidden {
				return fmt.Errorf("%s contains forbidden content field %q", field, key)
			}
			if err := validateContextText(field+" property name", key); err != nil {
				return err
			}
			if err := validateOpaqueContextValue(field+"."+key, item); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s contains a non-JSON value", field)
	}
}

func validateContextText(field, value string) error {
	if utf8.RuneCountInString(value) > 500 {
		return fmt.Errorf("%s must contain at most 500 Unicode code points", field)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must be a single line", field)
	}
	if value == "" {
		return ValidateSafeContent(field, value)
	}
	return validateBounded(field, value)
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
			if err := validateRepoRelativePath(s); err != nil {
				return fmt.Errorf("context.worktree.path_hint must be a normalized repository-relative forward-slash path or redacted")
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
	if err := validateBounded("evidence.ref", *e.Ref); err != nil {
		return err
	}
	switch e.Kind {
	case "path":
		if err := validateRepoRelativePath(*e.Ref); err != nil {
			return fmt.Errorf("evidence.ref must be a normalized repository-relative forward-slash path")
		}
	case "url":
		u, err := url.ParseRequestURI(*e.Ref)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("evidence.ref must be an http or https URL")
		}
	}
	return nil
}

func validateRepoRelativePath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("invalid repository-relative path")
	}
	if len(value) >= 2 && value[1] == ':' {
		return fmt.Errorf("invalid repository-relative path")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid repository-relative path")
		}
	}
	return nil
}

// AddOccurrence writes exactly one new immutable child JSON file.
func AddOccurrence(opts AddOccurrenceOptions) (Occurrence, error) {
	return addOccurrenceWithFS(opts, osLessonFS{})
}

func addOccurrenceWithFS(opts AddOccurrenceOptions, fs lessonFS) (Occurrence, error) {
	return addOccurrenceWithRuntime(opts, fs, json.MarshalIndent)
}

func addOccurrenceWithRuntime(opts AddOccurrenceOptions, fs lessonFS, marshal func(any, string, string) ([]byte, error)) (Occurrence, error) {
	l, err := Parse(opts.LessonPath)
	if err != nil {
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if !l.Canonical {
		return Occurrence{}, mutationFailure(MutationPrePublication, fmt.Errorf("lesson %q is legacy flat form", l.Slug))
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
	id := opts.ID
	if id == "" {
		id = uuid.NewString()
	}
	o := Occurrence{SchemaVersion: 1, ID: id, OccurredAt: opts.Now.UTC(), Summary: strings.TrimSpace(opts.Summary), Context: opts.Context, Evidence: opts.Evidence, Redactions: opts.Redactions}
	if o.Summary == "" {
		o.Summary = "Lesson gap observed."
	}
	if err := ValidateOccurrence(o); err != nil {
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if err := ensureOccurrenceDirectoryWithFS(l.OccurrencesDir, fs); err != nil {
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	o.Path = filepath.Join(l.OccurrencesDir, o.ID+".json")
	b, err := marshal(o, "", "  ")
	if err != nil {
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	f, err := fs.CreateTemp(l.OccurrencesDir, ".occurrence-")
	if err != nil {
		return Occurrence{}, err
	}
	tmp := f.Name()
	defer func() { _ = fs.Remove(tmp) }()
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if err := f.Close(); err != nil {
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if err := fs.Link(tmp, o.Path); err != nil {
		// link is an exclusive publication primitive: an existing destination
		// belongs to another writer and is not evidence that we published.
		return Occurrence{}, mutationFailure(MutationPrePublication, err)
	}
	if err := syncDirectoryWithFS(l.OccurrencesDir, fs); err != nil {
		// The exclusive link made the child visible. A path check followed by
		// Remove cannot prove that the same inode still occupies the UUID path;
		// a concurrent replacement could otherwise be deleted. Retain the child
		// and classify the durability boundary as uncertain for explicit retry.
		return o, mutationFailure(MutationUncertain, fmt.Errorf("occurrence retained after durability fence failure: %w", err))
	}
	return o, nil
}

// RemoveOccurrence performs an explicit caller-requested deletion and fences
// its parent directory. It is never safe as automatic post-publication
// compensation: a path returned by AddOccurrence is not an ownership token,
// because another writer can replace that path before a later unlink.
func RemoveOccurrence(path string) error {
	return removeOccurrenceWithFS(path, osLessonFS{})
}

func removeOccurrenceWithFS(path string, fs lessonFS) error {
	if err := fs.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectoryWithFS(filepath.Dir(path), fs)
}

func ensureOccurrenceDirectory(path string) error {
	return ensureOccurrenceDirectoryWithFS(path, osLessonFS{})
}

func ensureOccurrenceDirectoryWithFS(path string, fs lessonFS) error {
	info, err := fs.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("occurrence store is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := fs.Mkdir(path, 0o755); err != nil {
		return err
	}
	if err := syncDirectoryWithFS(filepath.Dir(path), fs); err != nil {
		return err
	}
	return syncDirectoryWithFS(path, fs)
}

// DiscoverOccurrences validates every child file and returns deterministic
// chronological order. A malformed child is an error, never silently ignored.
func DiscoverOccurrences(lessonPath string) ([]Occurrence, error) {
	return discoverOccurrencesWithFS(lessonPath, osLessonFS{})
}

func discoverOccurrencesWithFS(lessonPath string, fs lessonFS) ([]Occurrence, error) {
	l, err := Parse(lessonPath)
	if err != nil {
		return nil, err
	}
	if !l.Canonical {
		return nil, nil
	}
	entries, err := fs.ReadDir(l.OccurrencesDir)
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
		o, err := validateOccurrenceFileWithFS(path, fs)
		if err != nil {
			return nil, err
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

// ValidateOccurrenceFile validates one child independently so lint can report
// every malformed child instead of aborting a directory scan at the first.
func ValidateOccurrenceFile(path string) (Occurrence, error) {
	return validateOccurrenceFileWithFS(path, osLessonFS{})
}

func validateOccurrenceFileWithFS(path string, fs lessonFS) (Occurrence, error) {
	return validateOccurrenceFileWithRuntime(path, fs, func(data string) *json.Decoder {
		decoder := json.NewDecoder(strings.NewReader(data))
		decoder.UseNumber()
		return decoder
	})
}

func validateOccurrenceFileWithRuntime(path string, fs lessonFS, newDecoder func(string) *json.Decoder) (Occurrence, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return Occurrence{}, err
	}
	var o Occurrence
	if err := validateOccurrenceRaw(data); err != nil {
		return Occurrence{}, fmt.Errorf("invalid occurrence %s: %w", path, err)
	}
	dec := newDecoder(string(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return Occurrence{}, fmt.Errorf("invalid occurrence %s: %w", path, err)
	}
	if err := requireOccurrenceJSONEOF(dec); err != nil {
		return Occurrence{}, fmt.Errorf("invalid occurrence %s: trailing JSON", path)
	}
	o.Path = path
	if strings.TrimSuffix(filepath.Base(path), ".json") != o.ID {
		return Occurrence{}, fmt.Errorf("occurrence filename and id differ: %s", path)
	}
	if err := ValidateOccurrence(o); err != nil {
		return Occurrence{}, fmt.Errorf("invalid occurrence %s: %w", path, err)
	}
	return o, nil
}

func requireOccurrenceJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func validateOccurrenceRaw(data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	if err := scanOccurrenceJSON(document); err != nil {
		return err
	}
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
	if len(text) > 40 {
		return fmt.Errorf("occurred_at exceeds 40 bytes")
	}
	if !occurredAtLexical.MatchString(text) {
		return fmt.Errorf("occurred_at must use the strict RFC-3339 UTC Z form")
	}
	return nil
}

func scanOccurrenceJSON(value any) error {
	switch x := value.(type) {
	case map[string]any:
		for key, child := range x {
			if _, forbidden := occurrenceForbiddenNames[strings.ToLower(key)]; forbidden {
				return fmt.Errorf("occurrence contains a forbidden property name")
			}
			if err := ValidateSafeContent("occurrence property name", key); err != nil {
				return err
			}
			if err := scanOccurrenceJSON(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range x {
			if err := scanOccurrenceJSON(child); err != nil {
				return err
			}
		}
	case string:
		return ValidateSafeContent("occurrence string", x)
	}
	return nil
}

func FindOccurrence(lessonPath, id string) (Occurrence, error) {
	return findOccurrenceWithFS(lessonPath, id, osLessonFS{})
}

func findOccurrenceWithFS(lessonPath, id string, fs lessonFS) (Occurrence, error) {
	occurrences, err := discoverOccurrencesWithFS(lessonPath, fs)
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
