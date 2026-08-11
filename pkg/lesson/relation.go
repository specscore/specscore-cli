package lesson

// Human-confirmed relation storage for canonical Lessons.  This deliberately
// has no text-similarity path: a relation is a durable assertion, never an
// importer or linter guess.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofrs/flock"
)

type Relation struct {
	From string `json:"from"`
	Type string `json:"type"`
	To   string `json:"to"`
}

const relatedRelationsFile = ".relations.json"

type relationLocker interface {
	TryLock() (bool, error)
	Unlock() error
}

// relationDeps is private and created for every operation. Besides making
// filesystem failures deterministic to test, this keeps concurrency tests
// from mutating package globals shared by unrelated callers.
type relationDeps struct {
	fs            lessonFS
	abs           func(string) (string, error)
	evalSymlinks  func(string) (string, error)
	newLock       func(string) relationLocker
	marshal       func(any, string, string) ([]byte, error)
	rewrite       func([]byte, string, map[string]string) ([]byte, error)
	beforePublish func(string) error
	lockAcquired  func()
}

func defaultRelationDeps() relationDeps {
	return relationDeps{
		fs:           osLessonFS{},
		abs:          filepath.Abs,
		evalSymlinks: filepath.EvalSymlinks,
		newLock:      func(path string) relationLocker { return flock.New(path) },
		marshal:      json.MarshalIndent,
		rewrite:      rewriteRelationFields,
	}
}

func RelationToken(from, typ, to string) string {
	s := sha256.Sum256([]byte(from + "\x00" + typ + "\x00" + to))
	return hex.EncodeToString(s[:])[:16]
}

func ValidateRelation(from, typ, to string) error {
	if err := ValidateSlug(from); err != nil {
		return err
	}
	if err := ValidateSlug(to); err != nil {
		return err
	}
	if from == to {
		return fmt.Errorf("relation endpoints must be distinct")
	}
	if typ != "related" && typ != "duplicates" && typ != "supersedes" {
		return fmt.Errorf("relation type must be related, duplicates, or supersedes")
	}
	return nil
}

// ListRelations returns both metadata relations and directional fields. Its
// stable ordering makes this also safe for scripting and snapshot tests.
func ListRelations(lessonsDir, slug string) ([]Relation, error) {
	return listRelationsWithDeps(lessonsDir, slug, defaultRelationDeps())
}

func listRelationsWithDeps(lessonsDir, slug string, deps relationDeps) ([]Relation, error) {
	if _, err := ResolveLessonFile(lessonsDir, slug); err != nil {
		return nil, err
	}
	var out []Relation
	related, err := readRelatedRelationsWithFS(lessonsDir, deps.fs)
	if err != nil {
		return nil, err
	}
	for _, r := range related {
		if r.From == slug || r.To == slug {
			out = append(out, r)
		}
	}
	lessons, err := Discover(lessonsDir)
	if err != nil {
		return nil, err
	}
	for _, l := range lessons {
		fields, err := relationFields(l.Path, deps.fs)
		if err != nil {
			return nil, err
		}
		for _, f := range fields {
			if f.From == slug || f.To == slug {
				out = append(out, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return dedupeRelations(out), nil
}

func relationFields(path string, fs lessonFS) ([]Relation, error) {
	b, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	slug := filepath.Base(filepath.Dir(path))
	if filepath.Base(path) != "README.md" {
		slug = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	var out []Relation
	for _, line := range strings.Split(string(b), "\n") {
		name, value, ok := matchBoldField(strings.TrimSpace(line))
		if !ok || value == "" || value == "—" {
			continue
		}
		switch name {
		case "Duplicate Of":
			out = append(out, Relation{From: slug, Type: "duplicates", To: value})
		case "Supersedes":
			out = append(out, Relation{From: slug, Type: "supersedes", To: value})
		case "Superseded By":
			out = append(out, Relation{From: slug, Type: "superseded-by", To: value})
		}
	}
	return out, nil
}

func AddRelation(lessonsDir, from, typ, to string) error {
	return addRelationWithDeps(lessonsDir, from, typ, to, defaultRelationDeps())
}

func addRelationWithDeps(lessonsDir, from, typ, to string, deps relationDeps) error {
	return withRelationLockWithDeps(lessonsDir, deps, func() error {
		err := addRelationLockedWithDeps(lessonsDir, from, typ, to, deps)
		if err == nil {
			return nil
		}
		// Every untyped failure returned by the locked implementation occurs
		// before its first rename/link publication. Preserve explicit outcomes
		// from the atomic writers, but classify validation and preparation
		// failures so a caller can safely abort its prepared event.
		var mutation *MutationError
		if errors.As(err, &mutation) {
			return err
		}
		return mutationFailure(MutationPrePublication, err)
	})
}

// withRelationLock serializes cooperating SpecScore relation writers across
// processes. The ignored project-private lock is advisory and auto-released
// by the OS on process death, so it cannot wedge a tracked Lesson tree. Byte
// snapshots below also refuse a non-cooperating external write observed
// before publication; no advisory lock can make an arbitrary writer honor a
// read-to-rename critical section after that final observation.
func withRelationLockWithDeps(lessonsDir string, deps relationDeps, fn func() error) error {
	lockPath, err := relationLockPathWithDeps(lessonsDir, deps)
	if err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	if err := deps.fs.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return mutationFailure(MutationPrePublication, fmt.Errorf("creating relation lock directory: %w", err))
	}
	lock := deps.newLock(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return mutationFailure(MutationPrePublication, fmt.Errorf("acquiring relation publication lock: %w", err))
	}
	if !locked {
		return mutationFailure(MutationPrePublication, fmt.Errorf("another SpecScore relation writer is active"))
	}
	defer func() { _ = lock.Unlock() }()
	if deps.lockAcquired != nil {
		deps.lockAcquired()
	}
	return fn()
}

func relationLockPath(lessonsDir string) (string, error) {
	return relationLockPathWithDeps(lessonsDir, defaultRelationDeps())
}

func relationLockPathWithDeps(lessonsDir string, deps relationDeps) (string, error) {
	abs, err := deps.abs(lessonsDir)
	if err != nil {
		return "", fmt.Errorf("resolving relation lock root: %w", err)
	}
	root, err := deps.evalSymlinks(filepath.Dir(filepath.Dir(abs)))
	if err != nil {
		return "", fmt.Errorf("resolving physical relation lock root: %w", err)
	}
	return filepath.Join(root, ".specscore", "locks", "lesson-relations.lock"), nil
}

func addRelationLockedWithDeps(lessonsDir, from, typ, to string, deps relationDeps) error {
	if err := ValidateRelation(from, typ, to); err != nil {
		return err
	}
	fromPath, err := ResolveLessonFile(lessonsDir, from)
	if err != nil {
		return err
	}
	toPath, err := ResolveLessonFile(lessonsDir, to)
	if err != nil {
		return err
	}
	fromLesson, err := Parse(fromPath)
	if err != nil {
		return err
	}
	toLesson, err := Parse(toPath)
	if err != nil {
		return err
	}
	if !fromLesson.Canonical || !toLesson.Canonical {
		return fmt.Errorf("relations require canonical directory Lessons")
	}

	if typ == "related" {
		return appendRelatedRelationWithDeps(lessonsDir, Relation{From: from, Type: typ, To: to}, deps)
	}
	if typ == "supersedes" {
		cycle, err := wouldCycleWithFS(lessonsDir, from, to, deps.fs)
		if err != nil {
			return err
		}
		if cycle {
			return fmt.Errorf("supersedes relation would form a cycle: %s -> %s", from, to)
		}
	}
	if typ == "duplicates" {
		if fromLesson.Status == "Enforced" {
			return fmt.Errorf("enforced Lesson %q cannot be retained as a duplicate", from)
		}
		if fromLesson.Supersedes != "" && fromLesson.Supersedes != "—" {
			return fmt.Errorf("Lesson %q cannot be both a retained duplicate and a superseding Lesson", from)
		}
		cycle, err := wouldCycleWithFS(lessonsDir, from, to, deps.fs)
		if err != nil {
			return err
		}
		if cycle {
			return fmt.Errorf("duplicate relation would form a cycle: %s -> %s", from, to)
		}
		if err := ensureRelationFieldAvailableWithFS(fromPath, "Duplicate Of", to, deps.fs); err != nil {
			return err
		}
		if err := ensureRelationFieldAvailableWithFS(fromPath, "Superseded By", to, deps.fs); err != nil {
			return err
		}
		before, err := deps.fs.ReadFile(fromPath)
		if err != nil {
			return err
		}
		if err := ensureRelationFieldsAvailable(before, map[string]string{"Duplicate Of": to, "Superseded By": to}); err != nil {
			return err
		}
		if relationField(before, "Status") == "Enforced" {
			return fmt.Errorf("enforced Lesson %q cannot be retained as a duplicate", from)
		}
		after, err := deps.rewrite(before, fromPath, map[string]string{"Duplicate Of": to, "Superseded By": to, "Status": "Superseded"})
		if err != nil {
			return err
		}
		return replaceRelationCASWithDeps(fromPath, before, after, "duplicates", deps)
	}
	if err := ensureRelationFieldAvailableWithFS(fromPath, "Supersedes", to, deps.fs); err != nil {
		return err
	}
	if err := ensureRelationFieldAvailableWithFS(toPath, "Superseded By", from, deps.fs); err != nil {
		return err
	}
	// A successor declares the forward relation; the predecessor gains the
	// derived backwards pointer and status.  Snapshot both files so a partial
	// I/O failure is restored, not left as an asymmetric edge.
	a, err := deps.fs.ReadFile(fromPath)
	if err != nil {
		return err
	}
	b, err := deps.fs.ReadFile(toPath)
	if err != nil {
		return err
	}
	if err := ensureRelationFieldsAvailable(a, map[string]string{"Supersedes": to}); err != nil {
		return err
	}
	if err := ensureRelationFieldsAvailable(b, map[string]string{"Superseded By": from}); err != nil {
		return err
	}
	newA, err := deps.rewrite(a, fromPath, map[string]string{"Supersedes": to})
	if err != nil {
		return err
	}
	newB, err := deps.rewrite(b, toPath, map[string]string{"Superseded By": from, "Status": "Superseded"})
	if err != nil {
		return err
	}
	if err := relationPublicationHookWithDeps("supersedes", deps); err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	if err := relationSnapshotsMatchWithFS(map[string][]byte{fromPath: a, toPath: b}, deps.fs); err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	if err := atomicReplaceWithDeps(fromPath, newA, deps); err != nil {
		return err
	}
	if err := atomicReplaceWithDeps(toPath, newB, deps); err != nil {
		if MutationOutcomeOf(err) == MutationUncertain {
			// The second rename may already be visible. Do not call the first
			// rollback "compensated" while the pair can be asymmetric.
			return mutationFailure(MutationUncertain, fmt.Errorf("publishing superseding relation: %w", err))
		}
		return CompensatePublication(func() error { return replaceRelationCASWithDeps(fromPath, newA, a, "supersedes-rollback", deps) }, err)
	}
	return nil
}

func wouldCycle(lessonsDir, from, to string) (bool, error) {
	return wouldCycleWithFS(lessonsDir, from, to, osLessonFS{})
}

func wouldCycleWithFS(lessonsDir, from, to string, fs lessonFS) (bool, error) {
	seen := map[string]bool{}
	var visit func(string) (bool, error)
	visit = func(cur string) (bool, error) {
		if cur == from {
			return true, nil
		}
		if seen[cur] {
			return false, nil
		}
		seen[cur] = true
		p, err := ResolveLessonFile(lessonsDir, cur)
		if err != nil {
			return false, err
		}
		fields, err := relationFields(p, fs)
		if err != nil {
			return false, err
		}
		for _, r := range fields {
			if r.Type == "supersedes" || r.Type == "duplicates" {
				found, err := visit(r.To)
				if err != nil {
					return false, err
				}
				if found {
					return true, nil
				}
			}
		}
		return false, nil
	}
	return visit(to)
}

func ensureRelationFieldAvailableWithFS(path, name, want string, fs lessonFS) error {
	b, err := fs.ReadFile(path)
	if err != nil {
		return err
	}
	return ensureRelationFieldsAvailable(b, map[string]string{name: want})
}

func ensureRelationFieldsAvailable(b []byte, values map[string]string) error {
	seen := make(map[string]bool, len(values))
	for _, line := range strings.Split(string(b), "\n") {
		field, value, ok := matchBoldField(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if want, wanted := values[field]; wanted {
			seen[field] = true
			if value != "" && value != "—" && value != want {
				return fmt.Errorf("Lesson relation field %q already targets a different Lesson", field)
			}
		}
	}
	for name := range values {
		if !seen[name] {
			return fmt.Errorf("Lesson is missing relation field %q", name)
		}
	}
	return nil
}

func relationField(b []byte, want string) string {
	for _, line := range strings.Split(string(b), "\n") {
		name, value, ok := matchBoldField(strings.TrimSpace(line))
		if ok && name == want {
			return value
		}
	}
	return ""
}

func rewriteRelationFields(b []byte, path string, values map[string]string) ([]byte, error) {
	lines := strings.Split(string(b), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		name, _, ok := matchBoldField(strings.TrimSpace(line))
		if ok {
			if v, yes := values[name]; yes {
				lines[i] = "**" + name + ":** " + v
				seen[name] = true
			}
		}
	}
	for name := range values {
		if !seen[name] {
			return nil, fmt.Errorf("Lesson %s is missing required metadata field %q", path, name)
		}
	}
	if status, ok := values["Status"]; ok {
		for i, line := range lines {
			if strings.TrimSpace(line) == "status: Recorded" || strings.HasPrefix(strings.TrimSpace(line), "status: ") {
				lines[i] = "status: " + status
				break
			}
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func readRelatedRelations(lessonsDir string) ([]Relation, error) {
	return readRelatedRelationsWithFS(lessonsDir, osLessonFS{})
}

func readRelatedRelationsWithFS(lessonsDir string, fs lessonFS) ([]Relation, error) {
	b, err := fs.ReadFile(filepath.Join(lessonsDir, relatedRelationsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return decodeRelatedRelations(lessonsDir, b)
}

func decodeRelatedRelations(lessonsDir string, b []byte) ([]Relation, error) {
	var r []Relation
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", relatedRelationsFile, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("invalid %s: trailing JSON", relatedRelationsFile)
	}
	for _, edge := range r {
		if edge.Type != "related" {
			return nil, fmt.Errorf("invalid %s: sidecar only stores related edges", relatedRelationsFile)
		}
		if err := ValidateRelation(edge.From, edge.Type, edge.To); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", relatedRelationsFile, err)
		}
		if _, err := ResolveLessonFile(lessonsDir, edge.From); err != nil {
			return nil, fmt.Errorf("invalid %s: unresolved from endpoint", relatedRelationsFile)
		}
		if _, err := ResolveLessonFile(lessonsDir, edge.To); err != nil {
			return nil, fmt.Errorf("invalid %s: unresolved to endpoint", relatedRelationsFile)
		}
	}
	return r, nil
}

// appendRelatedRelationWithDeps derives both the sidecar snapshot and replacement
// from the same bytes. In particular, it never computes an "after" value
// from an older read and then overwrites a newer sidecar at the CAS boundary.
func appendRelatedRelationWithDeps(lessonsDir string, relation Relation, deps relationDeps) error {
	path := filepath.Join(lessonsDir, relatedRelationsFile)
	before, err := deps.fs.ReadFile(path)
	sidecarExists := err == nil
	if os.IsNotExist(err) {
		before = nil
	} else if err != nil {
		return err
	}
	var relations []Relation
	if sidecarExists {
		relations, err = decodeRelatedRelations(lessonsDir, before)
		if err != nil {
			return err
		}
	}
	for _, existing := range relations {
		if (existing.From == relation.From && existing.To == relation.To) || (existing.From == relation.To && existing.To == relation.From) {
			return nil
		}
	}
	relations = append(relations, relation)
	relations = dedupeRelations(relations)
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].From != relations[j].From {
			return relations[i].From < relations[j].From
		}
		return relations[i].To < relations[j].To
	})
	b, err := deps.marshal(relations, "", "  ")
	if err != nil {
		return err
	}
	return replaceRelationCASWithDeps(path, before, append(b, '\n'), "related", deps)
}

func relationPublicationHookWithDeps(kind string, deps relationDeps) error {
	if deps.beforePublish == nil {
		return nil
	}
	return deps.beforePublish(kind)
}

func relationSnapshotsMatch(expected map[string][]byte) error {
	return relationSnapshotsMatchWithFS(expected, osLessonFS{})
}

func relationSnapshotsMatchWithFS(expected map[string][]byte, fs lessonFS) error {
	for path, want := range expected {
		got, err := fs.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("Lesson changed during relation publication: %s", path)
		}
	}
	return nil
}

func replaceRelationCAS(path string, before, after []byte, kind string) error {
	return replaceRelationCASWithDeps(path, before, after, kind, defaultRelationDeps())
}

func replaceRelationCASWithDeps(path string, before, after []byte, kind string, deps relationDeps) error {
	if err := relationPublicationHookWithDeps(kind, deps); err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	current, err := deps.fs.ReadFile(path)
	if os.IsNotExist(err) && before == nil {
		current = nil
	} else if err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	if !bytes.Equal(current, before) {
		return mutationFailure(MutationPrePublication, fmt.Errorf("relation target changed during publication: %s", path))
	}
	return atomicReplaceWithDeps(path, after, deps)
}
func atomicReplace(path string, data []byte) error {
	return atomicReplaceWithDeps(path, data, defaultRelationDeps())
}

func atomicReplaceWithDeps(path string, data []byte, deps relationDeps) error {
	if err := deps.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := deps.fs.CreateTemp(filepath.Dir(path), ".relation-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = deps.fs.Remove(tmp) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err := deps.fs.Rename(tmp, path); err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	if err := syncDirectoryWithFS(filepath.Dir(path), deps.fs); err != nil {
		return mutationFailure(MutationUncertain, fmt.Errorf("durably publishing relation: %w", err))
	}
	return nil
}
func dedupeRelations(in []Relation) []Relation {
	seen := map[string]bool{}
	out := make([]Relation, 0, len(in))
	for _, r := range in {
		k := r.From + "\x00" + r.Type + "\x00" + r.To
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}
