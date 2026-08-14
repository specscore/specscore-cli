package lint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func canonicalLessonFixture(t *testing.T, specRoot, slug string) *lesson.Lesson {
	t.Helper()
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{
		Slug: slug, Owner: "tester", Date: "2026-08-10",
	}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(specRoot, "lessons", slug, "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func copyLessonForLint(in *lesson.Lesson) *lesson.Lesson {
	out := *in
	out.FieldCounts = make(map[string]int, len(in.FieldCounts))
	for k, v := range in.FieldCounts {
		out.FieldCounts[k] = v
	}
	out.SectionLines = make(map[string]int, len(in.SectionLines))
	for k, v := range in.SectionLines {
		out.SectionLines[k] = v
	}
	out.Classifications = append([]string(nil), in.Classifications...)
	return &out
}

func hasLessonRule(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

func TestLessonClassifications_AllShapes(t *testing.T) {
	projectRoot := t.TempDir()
	specRoot := filepath.Join(projectRoot, "spec")
	if _, err := lessonClassifications(specRoot); err == nil {
		t.Fatal("missing config must fail")
	}
	write := func(extras map[string]any) {
		t.Helper()
		if err := projectdef.WriteSpecConfig(projectRoot, projectdef.SpecConfig{Extras: extras}); err != nil {
			t.Fatal(err)
		}
	}
	for name, extras := range map[string]map[string]any{
		"missing lessons":         {},
		"wrong lessons shape":     {"lessons": "bad"},
		"missing classifications": {"lessons": map[string]any{}},
		"empty any list":          {"lessons": map[string]any{"classifications": []any{1}}},
		"invalid slug":            {"lessons": map[string]any{"classifications": []string{"Bad"}}},
		"duplicate after trim":    {"lessons": map[string]any{"classifications": []string{"process", " process "}}},
	} {
		t.Run(name, func(t *testing.T) {
			write(extras)
			if _, err := lessonClassifications(specRoot); err == nil {
				t.Fatal("invalid vocabulary must fail")
			}
		})
	}
	write(map[string]any{"lessons": map[string]any{"classifications": []any{" process ", 7, "validation"}}})
	got, err := lessonClassifications(specRoot)
	if err != nil || !got["process"] || !got["validation"] || len(got) != 2 {
		t.Fatalf("[]any vocabulary = %#v, err=%v", got, err)
	}
	write(map[string]any{"lessons": map[string]any{"classifications": []string{"process"}}})
	if got, err := lessonClassifications(specRoot); err != nil || !got["process"] {
		t.Fatalf("[]string vocabulary = %#v, err=%v", got, err)
	}
	if got, err := lessonClassificationsFromConfig(projectdef.SpecConfig{Extras: map[string]any{"lessons": map[string]any{"classifications": []string{"process"}}}}); err != nil || !got["process"] {
		t.Fatalf("typed []string vocabulary = %#v, err=%v", got, err)
	}
}

func TestLessonRulesChecker_CanonicalConflictAndMalformedRelations(t *testing.T) {
	t.Run("canonical and flat conflict", func(t *testing.T) {
		projectRoot := t.TempDir()
		specRoot := filepath.Join(projectRoot, "spec")
		canonicalLessonFixture(t, specRoot, "same")
		if err := os.WriteFile(filepath.Join(specRoot, "lessons", "same.md"), []byte(fullLesson("Recorded")), 0o644); err != nil {
			t.Fatal(err)
		}
		vs, err := newLessonRulesChecker().check(specRoot)
		if err != nil || !hasLessonRule(vs, "L-005") {
			t.Fatalf("conflict = %+v, err=%v", vs, err)
		}
	})

	t.Run("canonical traversal and malformed sidecar", func(t *testing.T) {
		projectRoot := t.TempDir()
		specRoot := filepath.Join(projectRoot, "spec")
		if err := projectdef.WriteSpecConfig(projectRoot, projectdef.SpecConfig{Extras: map[string]any{"lessons": map[string]any{"classifications": []any{"process"}}}}); err != nil {
			t.Fatal(err)
		}
		canonicalLessonFixture(t, specRoot, "only")
		if err := os.WriteFile(filepath.Join(specRoot, "lessons", ".relations.json"), []byte("{bad"), 0o644); err != nil {
			t.Fatal(err)
		}
		vs, err := newLessonRulesChecker().check(specRoot)
		if err != nil || !hasLessonRule(vs, "L-008") {
			t.Fatalf("malformed sidecar = %+v, err=%v", vs, err)
		}
	})
}

func TestLintCanonicalLesson_MetadataConfigurationTrackingAndEnforcement(t *testing.T) {
	projectRoot := t.TempDir()
	specRoot := filepath.Join(projectRoot, "spec")
	base := canonicalLessonFixture(t, specRoot, "canonical")
	allowed := map[string]bool{"process": true}
	if got := lintCanonicalLesson(specRoot, base, "lessons/canonical/README.md", allowed, nil); len(got) != 0 {
		t.Fatalf("clean canonical Lesson: %+v", got)
	}

	t.Run("title metadata order duplicates and empty values", func(t *testing.T) {
		l := copyLessonForLint(base)
		l.HasLessonTitle, l.Title = false, ""
		l.StatusLine = 0
		l.DateLine, l.Date = 7, ""
		l.OwnerLine, l.Owner = 6, ""
		l.ClassificationsLine = 0
		l.LegacyProvenanceLine, l.LegacyProvenance = 5, ""
		l.DuplicateOfLine, l.DuplicateOf = 4, ""
		l.SupersedesLine, l.Supersedes = 3, ""
		l.SupersededByLine, l.SupersededBy = 2, ""
		l.FieldCounts["Owner"] = 2
		if got := lintCanonicalLesson(specRoot, l, "lesson.md", allowed, nil); !hasLessonRule(got, "L-005") {
			t.Fatalf("metadata violations missing: %+v", got)
		}
	})

	t.Run("date section order and config", func(t *testing.T) {
		l := copyLessonForLint(base)
		l.Date = "10/08/2026"
		l.SectionLines["Lesson"] = 50
		l.SectionLines["Process Gap"] = 40
		l.SectionLines["Tracking"] = 0
		if got := lintCanonicalLesson(specRoot, l, "lesson.md", nil, errors.New("bad classification config")); !hasLessonRule(got, "L-005") {
			t.Fatalf("date/order/config violations missing: %+v", got)
		}
	})

	for name, classifications := range map[string][]string{
		"empty":              nil,
		"duplicate":          {"process", "process"},
		"outside vocabulary": {"unknown"},
	} {
		t.Run("classification "+name, func(t *testing.T) {
			l := copyLessonForLint(base)
			l.Classifications = classifications
			if got := lintCanonicalLesson(specRoot, l, "lesson.md", allowed, nil); !hasLessonRule(got, "L-005") {
				t.Fatalf("classification violation missing: %+v", got)
			}
		})
	}

	t.Run("unsafe body and tracking", func(t *testing.T) {
		l := copyLessonForLint(base)
		l.RecurredLine = 1
		if err := os.WriteFile(l.Path, []byte("password: exposed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := lintCanonicalLesson(specRoot, l, "lesson.md", allowed, nil)
		if !hasLessonRule(got, "L-005") || !hasLessonRule(got, "L-006") {
			t.Fatalf("unsafe/tracking violations missing: %+v", got)
		}
	})

	// Restore the clean file for the enforcement matrix.
	base = canonicalLessonFixture(t, specRoot, "canonical")
	for name, mutate := range map[string]func(*lesson.Lesson){
		"empty control":      func(l *lesson.Lesson) { l.Control = "" },
		"dash control":       func(l *lesson.Lesson) { l.Control = "—" },
		"empty verification": func(l *lesson.Lesson) { l.Control = "run"; l.Verification = "" },
		"dash verification":  func(l *lesson.Lesson) { l.Control = "run"; l.Verification = "—" },
		"empty evidence":     func(l *lesson.Lesson) { l.Control, l.Verification, l.Evidence = "run", "go test ./...", "" },
		"dash evidence":      func(l *lesson.Lesson) { l.Control, l.Verification, l.Evidence = "run", "go test ./...", "—" },
		"manual verification": func(l *lesson.Lesson) {
			l.Control, l.Verification, l.Evidence = "run", "Manual review", "sha256:0123456789abcdef0"
		},
		"unstable evidence": func(l *lesson.Lesson) { l.Control, l.Verification, l.Evidence = "run", "go test ./...", "missing.txt" },
	} {
		t.Run(name, func(t *testing.T) {
			l := copyLessonForLint(base)
			l.Status = "Enforced"
			mutate(l)
			if got := lintCanonicalLesson(specRoot, l, "lesson.md", allowed, nil); !hasLessonRule(got, "L-007") {
				t.Fatalf("enforcement violation missing: %+v", got)
			}
		})
	}
	l := copyLessonForLint(base)
	l.Status, l.Control, l.Verification, l.Evidence = "Enforced", "run", "go test ./...", "sha256:0123456789abcdef0"
	if got := lintCanonicalLesson(specRoot, l, "lesson.md", allowed, nil); hasLessonRule(got, "L-007") {
		t.Fatalf("stable enforcement rejected: %+v", got)
	}
}

func TestStableLessonEvidence_AllForms(t *testing.T) {
	projectRoot := t.TempDir()
	specRoot := filepath.Join(projectRoot, "spec")
	if err := os.WriteFile(filepath.Join(projectRoot, "evidence.txt"), []byte("proof"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"sha256:short", false},
		{"sha256:0123456789abcdef0", true},
		{"https://example.test/commit/abc", true},
		{"https://example.test/blob/main/x", true},
		{"http://example.test/proof?sha256=abc", true},
		{"https://example.test/latest", false},
		{`dir\\file`, false},
		{filepath.Join(projectRoot, "absolute"), false},
		{"../escape", false},
		{"dir/../escape", false},
		{".", false},
		{"..", false},
		{"evidence.txt", true},
		{"missing.txt", false},
	} {
		if got := stableLessonEvidence(specRoot, tc.value); got != tc.want {
			t.Errorf("stableLessonEvidence(%q)=%v want %v", tc.value, got, tc.want)
		}
	}
}

func relationLesson(slug string) *lesson.Lesson {
	return &lesson.Lesson{Slug: slug, Canonical: true, Path: filepath.Join("spec", "lessons", slug, "README.md"), Status: "Recorded", DuplicateOf: "—", Supersedes: "—", SupersededBy: "—"}
}

func TestLintLessonRelations_ResolutionDispositionInverseAndCycles(t *testing.T) {
	specRoot := t.TempDir()
	if got := lintLessonRelations(specRoot, map[string]*lesson.Lesson{"legacy": {Slug: "legacy"}}); len(got) != 0 {
		t.Fatalf("legacy relation candidate: %+v", got)
	}

	bad := relationLesson("bad")
	bad.DuplicateOf = "Invalid/Slug"
	bad.SupersededBy = "missing"
	bad.Supersedes = "missing"
	if got := lintLessonRelations(specRoot, map[string]*lesson.Lesson{"bad": bad}); len(got) < 2 {
		t.Fatalf("unresolved targets not diagnosed: %+v", got)
	}

	for name, mutate := range map[string]func(*lesson.Lesson){
		"enforced":      func(l *lesson.Lesson) { l.Status = "Enforced" },
		"wrong status":  func(l *lesson.Lesson) { l.Status = "Recorded" },
		"wrong inverse": func(l *lesson.Lesson) { l.Status, l.SupersededBy = "Superseded", "other" },
	} {
		t.Run(name, func(t *testing.T) {
			canonical := relationLesson("canonical")
			retained := relationLesson("retained")
			retained.DuplicateOf, retained.SupersededBy = "canonical", "canonical"
			retained.Status = "Superseded"
			mutate(retained)
			if got := lintLessonRelations(specRoot, map[string]*lesson.Lesson{"canonical": canonical, "retained": retained}); len(got) == 0 {
				t.Fatal("invalid duplicate disposition was accepted")
			}
		})
	}

	newer, prior := relationLesson("newer"), relationLesson("prior")
	newer.Supersedes = "prior"
	prior.Status, prior.SupersededBy = "Recorded", "wrong"
	if got := lintLessonRelations(specRoot, map[string]*lesson.Lesson{"newer": newer, "prior": prior}); len(got) == 0 {
		t.Fatal("broken supersedes inverse was accepted")
	}
	prior.Status, prior.SupersededBy = "Superseded", "newer"
	if got := lintLessonRelations(specRoot, map[string]*lesson.Lesson{"newer": newer, "prior": prior}); len(got) != 0 {
		t.Fatalf("valid supersedes inverse rejected: %+v", got)
	}

	a, b := relationLesson("a"), relationLesson("b")
	a.Supersedes, b.Supersedes = "b", "a"
	if got := lintLessonRelations(specRoot, map[string]*lesson.Lesson{"a": a, "b": b}); len(got) == 0 {
		t.Fatal("cycle was accepted")
	}
}

func TestLintOccurrenceChildren_MissingUnreadableNonJSONMalformedAndValid(t *testing.T) {
	specRoot := t.TempDir()
	l := &lesson.Lesson{Path: filepath.Join(specRoot, "lessons", "x", "README.md"), OccurrencesDir: filepath.Join(specRoot, "lessons", "x", "occurrences")}
	if got := lintOccurrenceChildren(specRoot, l); len(got) != 1 || got[0].Rule != "L-009" {
		t.Fatalf("missing occurrence directory: %+v", got)
	}
	if err := os.MkdirAll(filepath.Dir(l.OccurrencesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.OccurrencesDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lintOccurrenceChildren(specRoot, l); len(got) != 1 {
		t.Fatalf("unreadable occurrence directory: %+v", got)
	}
	if err := os.Remove(l.OccurrencesDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(l.OccurrencesDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.OccurrencesDir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.OccurrencesDir, "bad.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lintOccurrenceChildren(specRoot, l); len(got) != 3 {
		t.Fatalf("non-JSON/malformed children: %+v", got)
	}

	if err := os.RemoveAll(l.OccurrencesDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(l.OccurrencesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.OccurrencesDir, lesson.OccurrenceStoreKeepFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lintOccurrenceChildren(specRoot, l); len(got) != 0 {
		t.Fatalf("empty-store marker should be accepted: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(l.OccurrencesDir, lesson.OccurrenceStoreKeepFile), []byte("not empty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lintOccurrenceChildren(specRoot, l); len(got) != 1 || got[0].Rule != "L-009" {
		t.Fatalf("non-empty store marker should be rejected: %+v", got)
	}

	if err := os.RemoveAll(l.OccurrencesDir); err != nil {
		t.Fatal(err)
	}
	canonical := canonicalLessonFixture(t, specRoot, "valid")
	if _, err := lesson.AddOccurrence(lesson.AddOccurrenceOptions{
		LessonPath: canonical.Path,
		ID:         "01234567-89ab-4def-8123-456789abcdef",
		Summary:    "validated child",
		Context:    map[string]any{},
		Evidence:   lesson.Evidence{Kind: "none"},
		Now:        time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if got := lintOccurrenceChildren(specRoot, canonical); len(got) != 0 {
		t.Fatalf("valid occurrence rejected: %+v", got)
	}
}

func TestExpectedLessonIndexRow_LegacyCanonicalOccurrenceAndEnforcement(t *testing.T) {
	legacy, err := expectedLessonIndexRow("legacy", &lesson.Lesson{Slug: "legacy", Status: " Stated ", Recurred: 3})
	if err != nil || legacy.link != "legacy.md" || legacy.classifications != "Legacy" || legacy.occurrences != "3" || legacy.status != "Stated" {
		t.Fatalf("legacy row=%+v err=%v", legacy, err)
	}

	specRoot := filepath.Join(t.TempDir(), "spec")
	canonical := canonicalLessonFixture(t, specRoot, "indexed")
	if _, err := lesson.AddOccurrence(lesson.AddOccurrenceOptions{
		LessonPath: canonical.Path,
		ID:         "11234567-89ab-4def-8123-456789abcdef",
		Summary:    "index projection",
		Context:    map[string]any{},
		Evidence:   lesson.Evidence{Kind: "none"},
		Now:        time.Date(2026, 8, 10, 12, 34, 56, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	canonical.DuplicateOf = "canonical"
	row, err := expectedLessonIndexRow("indexed", canonical)
	if err != nil || row.occurrences != "1" || row.lastOccurred != "2026-08-10T12:34:56Z" || row.enforcement != "Duplicate Of: canonical" {
		t.Fatalf("canonical duplicate row=%+v err=%v", row, err)
	}
	canonical.DuplicateOf, canonical.Control = "—", " deterministic control "
	row, err = expectedLessonIndexRow("indexed", canonical)
	if err != nil || row.enforcement != "deterministic control" {
		t.Fatalf("canonical control row=%+v err=%v", row, err)
	}
	canonical.Path = filepath.Join(t.TempDir(), "missing", "README.md")
	if _, err := expectedLessonIndexRow("indexed", canonical); err == nil {
		t.Fatal("unreadable canonical occurrence store must fail")
	}
}

func TestReadLessonIndexRows_CompatibilityAndMalformedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	body := `# Lessons

ignored before table
## Index
| Lesson | Status | Recurred | Date | Owner |
|---|---|---|---|---|
not a row
| [legacy](legacy.md) | Recorded | 2 | 2026-01-01 | alex |
| [wrong](other.md) | Recorded | 0 | x | y |
| too | few |
## End
| [ignored](ignored.md) | Recorded | 0 | x | y |
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, canonical, malformed, err := readLessonIndexRows(path)
	if err != nil || canonical || !malformed || len(rows) != 1 || rows[0].slug != "legacy" {
		t.Fatalf("legacy rows=%+v canonical=%v malformed=%v err=%v", rows, canonical, malformed, err)
	}

	body = `## Lessons
| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |
|---|---|---|---:|---|---|
| no-link | Recorded | process | 0 | | — |
| [](x/README.md) | Recorded | process | 0 | | — |
| [label](other/README.md) | Recorded | process | 0 | | — |
| [good](good/README.md) | Recorded | process | 0 | | — |
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, canonical, malformed, err = readLessonIndexRows(path)
	if err != nil || !canonical || !malformed || len(rows) != 1 || rows[0].slug != "good" {
		t.Fatalf("canonical rows=%+v canonical=%v malformed=%v err=%v", rows, canonical, malformed, err)
	}

	for _, cell := range []string{"plain", "[](x)", "[x](missing-suffix"} {
		if _, _, ok := parseMarkdownLink(cell); ok {
			t.Fatalf("malformed link accepted: %q", cell)
		}
	}
	if label, link, ok := parseMarkdownLink("[x](x.md)"); !ok || label != "x" || link != "x.md" {
		t.Fatalf("valid link = %q %q %v", label, link, ok)
	}
}

func TestLessonIndexRules_ReportsDuplicateOrphanShapeAndExpectedRowFailure(t *testing.T) {
	specRoot := filepath.Join(t.TempDir(), "spec")
	if err := os.MkdirAll(filepath.Join(specRoot, "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	index := `## Index
| Lesson | Status | Recurred | Date | Owner |
|---|---|---|---|---|
| [orphan](orphan.md) | Recorded | 0 | | |
| [orphan](orphan.md) | Recorded | 0 | | |
`
	if err := os.WriteFile(filepath.Join(specRoot, "lessons", "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := &lesson.Lesson{Slug: "missing", Canonical: true, Path: filepath.Join(specRoot, "lessons", "missing", "README.md")}
	vs, fixed := lessonIndexRules(specRoot, map[string]*lesson.Lesson{"missing": missing}, false)
	if fixed || !hasLessonRule(vs, "L-003") || !hasLessonRule(vs, "L-004") {
		t.Fatalf("index mismatch = %+v fixed=%v", vs, fixed)
	}
	msg := lessonViolation(vs, "L-004").Message
	for _, want := range []string{"table shape", "orphaned: orphan", "duplicated: orphan"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("L-004 missing %q: %s", want, msg)
		}
	}

	canonicalIndex := `## Lessons
| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |
|---|---|---|---:|---|---|
| [missing](missing/README.md) | Recorded | process | 0 | | — |
`
	if err := os.WriteFile(filepath.Join(specRoot, "lessons", "README.md"), []byte(canonicalIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	vs, _ = lessonIndexRules(specRoot, map[string]*lesson.Lesson{"missing": missing}, false)
	if got := lessonViolation(vs, "L-004"); got == nil || !strings.Contains(got.Message, "drifted: missing") {
		t.Fatalf("expected-row failure not drifted: %+v", vs)
	}
}

func TestRewriteLessonIndex_PropagatesExpectedRowFailureWithoutFollowingSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("# Lessons\n\n## Lessons\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := &lesson.Lesson{Slug: "broken", Canonical: true, Path: filepath.Join(t.TempDir(), "missing", "README.md")}
	if err := rewriteLessonIndex(path, []string{"broken"}, map[string]*lesson.Lesson{"broken": l}); err == nil {
		t.Fatal("expected row failure must abort index rewrite")
	}
}

func canonicalIndexBody(rows ...string) string {
	return "## Lessons\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n" + strings.Join(rows, "\n") + "\n"
}

func legacyIndexBody(rows ...string) string {
	return "## Index\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n" + strings.Join(rows, "\n") + "\n"
}

func TestUpsertLessonIndexRow_CanonicalAndLegacyContracts(t *testing.T) {
	specRoot := filepath.Join(t.TempDir(), "spec")
	lessonsDir := filepath.Join(specRoot, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, nil); err == nil {
		t.Fatal("nil Lesson accepted")
	}
	l := canonicalLessonFixture(t, specRoot, "new-row")
	if err := UpsertLessonIndexRow(specRoot, l); err == nil {
		t.Fatal("missing index accepted")
	}
	indexPath := filepath.Join(lessonsDir, "README.md")
	if err := os.WriteFile(indexPath, []byte("## Lessons\ninvalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err == nil {
		t.Fatal("missing canonical table accepted")
	}
	if err := os.WriteFile(indexPath, []byte("## Lessons\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n\n|---|---|---|---:|---|---|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err == nil {
		t.Fatal("non-adjacent separator accepted")
	}
	if err := os.WriteFile(indexPath, []byte(canonicalIndexBody("| [other](other/README.md) | Recorded | process | 0 | | — |")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err != nil {
		t.Fatalf("insert canonical row: %v", err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err != nil {
		t.Fatalf("update canonical row: %v", err)
	}
	body, _ := os.ReadFile(indexPath)
	if strings.Count(string(body), "[new-row](new-row/README.md)") != 1 {
		t.Fatalf("canonical upsert is not idempotent:\n%s", body)
	}
	duplicate := canonicalIndexBody(
		"| [new-row](new-row/README.md) | Recorded | process | 0 | | — |",
		"| [new-row](new-row/README.md) | Recorded | process | 0 | | — |",
	)
	if err := os.WriteFile(indexPath, []byte(duplicate), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err == nil {
		t.Fatal("duplicate canonical row accepted")
	}

	broken := copyLessonForLint(l)
	broken.Slug = "broken"
	broken.Path = filepath.Join(t.TempDir(), "missing", "README.md")
	if err := os.WriteFile(indexPath, []byte(canonicalIndexBody()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, broken); err == nil {
		t.Fatal("occurrence discovery failure hidden")
	}

	legacy := &lesson.Lesson{Slug: "legacy", Status: "Stated", Recurred: 2}
	if err := os.WriteFile(indexPath, []byte("## Index\ninvalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, legacy); err == nil {
		t.Fatal("missing legacy table accepted")
	}
	if err := os.WriteFile(indexPath, []byte(legacyIndexBody("| [other](other.md) | Recorded | 0 | 2026-01-01 | alex |")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, legacy); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := UpsertLessonIndexRow(specRoot, legacy); err != nil {
		t.Fatalf("update legacy row: %v", err)
	}
	body, _ = os.ReadFile(indexPath)
	if !strings.Contains(string(body), "| [legacy](legacy.md) | Stated | 2 |  |  |") {
		t.Fatalf("legacy projection missing:\n%s", body)
	}
	legacyDuplicate := legacyIndexBody(
		"| [legacy](legacy.md) | Recorded | 1 | one | owner |",
		"| [legacy](legacy.md) | Recorded | 1 | two | owner |",
	)
	if err := os.WriteFile(indexPath, []byte(legacyDuplicate), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, legacy); err == nil {
		t.Fatal("duplicate legacy row accepted")
	}
	if got := firstMarkdownCell(""); got != "" {
		t.Fatalf("empty markdown cell = %q", got)
	}
	if got := firstMarkdownCell("| [x](x.md) | value |"); got != "[x](x.md)" {
		t.Fatalf("first markdown cell = %q", got)
	}
}

func TestLessonAdherenceWalkersAndACHeadingWalkError(t *testing.T) {
	specRoot := t.TempDir()
	called := 0
	if err := walkLessonsIndex(specRoot, func(string, []byte) { called++ }); err != nil || called != 0 {
		t.Fatalf("missing index walk called=%d err=%v", called, err)
	}
	index := filepath.Join(specRoot, "lessons", "README.md")
	if err := os.MkdirAll(filepath.Dir(index), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := walkLessonsIndex(specRoot, func(path string, content []byte) {
		called++
		if path != index || string(content) != "index" {
			t.Errorf("walked %s %q", path, content)
		}
	}); err != nil || called != 1 {
		t.Fatalf("index walk called=%d err=%v", called, err)
	}
	if err := os.MkdirAll(filepath.Join(specRoot, "lessons", "one"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specRoot, "lessons", "one", "README.md"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specRoot, "lessons", "one", "ignored.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	readmes := 0
	if err := walkLessonReadmes(specRoot, func(string, []byte) { readmes++ }); err != nil || readmes != 1 {
		t.Fatalf("Lesson README walk=%d err=%v", readmes, err)
	}

	features := filepath.Join(specRoot, "features")
	if err := os.MkdirAll(features, 0o755); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(features, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(features, 0o755) })
		if _, err := newACHeadingFormatChecker().check(specRoot); err == nil {
			t.Fatal("unreadable features tree must fail")
		}
	}
}
