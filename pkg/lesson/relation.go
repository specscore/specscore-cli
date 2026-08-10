package lesson

// Human-confirmed relation storage for canonical Lessons.  This deliberately
// has no text-similarity path: a relation is a durable assertion, never an
// importer or linter guess.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Relation struct {
	From string `json:"from"`
	Type string `json:"type"`
	To   string `json:"to"`
}

const relatedRelationsFile = ".relations.json"

var relationRename = os.Rename

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
	if _, err := ResolveLessonFile(lessonsDir, slug); err != nil {
		return nil, err
	}
	var out []Relation
	related, err := readRelatedRelations(lessonsDir)
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
		fields, err := relationFields(l.Path)
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

func relationFields(path string) ([]Relation, error) {
	b, err := os.ReadFile(path)
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
		relations, err := readRelatedRelations(lessonsDir)
		if err != nil {
			return err
		}
		for _, r := range relations {
			if r.Type == typ && ((r.From == from && r.To == to) || (r.From == to && r.To == from)) {
				return nil
			}
		}
		relations = append(relations, Relation{From: from, Type: typ, To: to})
		return writeRelatedRelations(lessonsDir, relations)
	}
	if typ == "supersedes" {
		cycle, err := wouldCycle(lessonsDir, from, to)
		if err != nil {
			return err
		}
		if cycle {
			return fmt.Errorf("supersedes relation would form a cycle: %s -> %s", from, to)
		}
	}
	if typ == "duplicates" {
		if fromLesson.Status == "Enforced" {
			return fmt.Errorf("Enforced Lesson %q cannot be retained as a duplicate", from)
		}
		if fromLesson.Supersedes != "" && fromLesson.Supersedes != "—" {
			return fmt.Errorf("Lesson %q cannot be both a retained duplicate and a superseding Lesson", from)
		}
		cycle, err := wouldCycle(lessonsDir, from, to)
		if err != nil {
			return err
		}
		if cycle {
			return fmt.Errorf("duplicate relation would form a cycle: %s -> %s", from, to)
		}
		if err := ensureRelationFieldAvailable(fromPath, "Duplicate Of", to); err != nil {
			return err
		}
		if err := ensureRelationFieldAvailable(fromPath, "Superseded By", to); err != nil {
			return err
		}
		return updateRelationFields(fromPath, map[string]string{"Duplicate Of": to, "Superseded By": to, "Status": "Superseded"})
	}
	if err := ensureRelationFieldAvailable(fromPath, "Supersedes", to); err != nil {
		return err
	}
	if err := ensureRelationFieldAvailable(toPath, "Superseded By", from); err != nil {
		return err
	}
	// A successor declares the forward relation; the predecessor gains the
	// derived backwards pointer and status.  Snapshot both files so a partial
	// I/O failure is restored, not left as an asymmetric edge.
	a, err := os.ReadFile(fromPath)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(toPath)
	if err != nil {
		return err
	}
	newA, err := rewriteRelationFields(a, fromPath, map[string]string{"Supersedes": to})
	if err != nil {
		return err
	}
	newB, err := rewriteRelationFields(b, toPath, map[string]string{"Superseded By": from, "Status": "Superseded"})
	if err != nil {
		return err
	}
	if current, err := os.ReadFile(fromPath); err != nil || !bytes.Equal(current, a) {
		return fmt.Errorf("Lesson %q changed during relation publication", from)
	}
	if current, err := os.ReadFile(toPath); err != nil || !bytes.Equal(current, b) {
		return fmt.Errorf("Lesson %q changed during relation publication", to)
	}
	if err := atomicReplace(fromPath, newA); err != nil {
		return err
	}
	if err := atomicReplace(toPath, newB); err != nil {
		rollbackErr := atomicReplace(fromPath, a)
		if rollbackErr != nil {
			return fmt.Errorf("publishing relation failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	return nil
}

func wouldCycle(lessonsDir, from, to string) (bool, error) {
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
		fields, err := relationFields(p)
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

func ensureRelationFieldAvailable(path, name, want string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		field, value, ok := matchBoldField(strings.TrimSpace(line))
		if ok && field == name {
			if value != "" && value != "—" && value != want {
				return fmt.Errorf("Lesson relation field %q already targets a different Lesson", name)
			}
			return nil
		}
	}
	return fmt.Errorf("Lesson is missing relation field %q", name)
}

func updateRelationFields(path string, values map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := rewriteRelationFields(b, path, values)
	if err != nil {
		return err
	}
	return atomicReplace(path, updated)
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
	b, err := os.ReadFile(filepath.Join(lessonsDir, relatedRelationsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
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
func writeRelatedRelations(lessonsDir string, relations []Relation) error {
	relations = dedupeRelations(relations)
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].Type != relations[j].Type {
			return relations[i].Type < relations[j].Type
		}
		if relations[i].From != relations[j].From {
			return relations[i].From < relations[j].From
		}
		return relations[i].To < relations[j].To
	})
	b, err := json.MarshalIndent(relations, "", "  ")
	if err != nil {
		return err
	}
	return atomicReplace(filepath.Join(lessonsDir, relatedRelationsFile), append(b, '\n'))
}
func atomicReplace(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".relation-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
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
	return relationRename(tmp, path)
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
