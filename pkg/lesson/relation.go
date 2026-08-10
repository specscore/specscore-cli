package lesson

// Human-confirmed relation storage for canonical Lessons.  This deliberately
// has no text-similarity path: a relation is a durable assertion, never an
// importer or linter guess.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	for _, r := range readRelatedRelations(lessonsDir) {
		if r.From == slug || r.To == slug {
			out = append(out, r)
		}
	}
	lessons, err := Discover(lessonsDir)
	if err != nil {
		return nil, err
	}
	for _, l := range lessons {
		if l.Slug == slug && l.SupersededBy != "" && l.SupersededBy != "—" {
			out = append(out, Relation{From: slug, Type: "superseded-by", To: l.SupersededBy})
		}
		if l.Slug == slug && l.SupersededBy == "" { /* legacy field omitted */
		}
		if l.Slug == slug {
			for _, f := range relationFields(l.Path) {
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

func relationFields(path string) []Relation {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
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
	return out
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
		relations := readRelatedRelations(lessonsDir)
		for _, r := range relations {
			if r.Type == typ && ((r.From == from && r.To == to) || (r.From == to && r.To == from)) {
				return nil
			}
		}
		relations = append(relations, Relation{From: from, Type: typ, To: to})
		return writeRelatedRelations(lessonsDir, relations)
	}
	if typ == "supersedes" && wouldCycle(lessonsDir, from, to) {
		return fmt.Errorf("supersedes relation would form a cycle: %s -> %s", from, to)
	}
	if typ == "duplicates" {
		return updateRelationFields(fromPath, map[string]string{"Duplicate Of": to, "Superseded By": to, "Status": "Superseded"})
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
	if err := updateRelationFields(fromPath, map[string]string{"Supersedes": to}); err != nil {
		return err
	}
	if err := updateRelationFields(toPath, map[string]string{"Superseded By": from, "Status": "Superseded"}); err != nil {
		_ = os.WriteFile(fromPath, a, 0o644)
		_ = os.WriteFile(toPath, b, 0o644)
		return err
	}
	return nil
}

func wouldCycle(lessonsDir, from, to string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(cur string) bool {
		if cur == from {
			return true
		}
		if seen[cur] {
			return false
		}
		seen[cur] = true
		p, err := ResolveLessonFile(lessonsDir, cur)
		if err != nil {
			return false
		}
		for _, r := range relationFields(p) {
			if r.Type == "supersedes" && visit(r.To) {
				return true
			}
		}
		return false
	}
	return visit(to)
}

func updateRelationFields(path string, values map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
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
			return fmt.Errorf("Lesson %s is missing required metadata field %q", path, name)
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
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func readRelatedRelations(lessonsDir string) []Relation {
	b, err := os.ReadFile(filepath.Join(lessonsDir, relatedRelationsFile))
	if err != nil {
		return nil
	}
	var r []Relation
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return r
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
	return os.WriteFile(filepath.Join(lessonsDir, relatedRelationsFile), append(b, '\n'), 0o644)
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
