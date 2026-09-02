package lint

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

type testLessonIndexLock struct {
	lockErr   error
	unlockErr error
}

func (l testLessonIndexLock) Lock() error   { return l.lockErr }
func (l testLessonIndexLock) Unlock() error { return l.unlockErr }

func TestUpsertLessonIndexRowSerializesConcurrentRowsAndLockFailures(t *testing.T) {
	specRoot := t.TempDir()
	lessonsDir := filepath.Join(specRoot, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "# Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	var parsed []*lesson.Lesson
	for i := 0; i < 16; i++ {
		slug := fmt.Sprintf("concurrent-%02d", i)
		path := filepath.Join(lessonsDir, slug, "README.md")
		if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Owner: "codex", Date: "2026-08-11"}, []string{"process"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		item, err := lesson.Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, item)
	}
	start := make(chan struct{})
	errs := make(chan error, len(parsed))
	var wg sync.WaitGroup
	for _, item := range parsed {
		wg.Add(1)
		go func(l *lesson.Lesson) {
			defer wg.Done()
			<-start
			errs <- UpsertLessonIndexRow(specRoot, l)
		}(item)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filepath.Join(lessonsDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range parsed {
		if !bytes.Contains(got, []byte("["+item.Slug+"]("+item.Slug+"/README.md)")) {
			t.Errorf("concurrent row missing for %s", item.Slug)
		}
	}

	boom := errors.New("lock boundary")
	deps := lessonIndexLockDeps{mkdirAll: func(string, os.FileMode) error { return boom }, newLock: func(string) lessonIndexLocker { return testLessonIndexLock{} }}
	if err := upsertLessonIndexRowWithLock(specRoot, parsed[0], deps); !errors.Is(err, boom) {
		t.Fatalf("mkdir lock error = %v", err)
	}
	deps.mkdirAll = func(string, os.FileMode) error { return nil }
	deps.newLock = func(string) lessonIndexLocker { return testLessonIndexLock{lockErr: boom} }
	if err := upsertLessonIndexRowWithLock(specRoot, parsed[0], deps); !errors.Is(err, boom) {
		t.Fatalf("acquire lock error = %v", err)
	}
	deps.newLock = func(string) lessonIndexLocker { return testLessonIndexLock{unlockErr: boom} }
	if err := upsertLessonIndexRowWithLock(specRoot, parsed[0], deps); !errors.Is(err, boom) || lesson.MutationOutcomeOf(err) != lesson.MutationUncertain {
		t.Fatalf("release lock error = %v", err)
	}
}

func TestLessonRulesFixLocksIndexBeforeDiscoverySoLifecycleRowSurvives(t *testing.T) {
	projectRoot := t.TempDir()
	specRoot := filepath.Join(projectRoot, "spec")
	lessonsDir := filepath.Join(specRoot, "lessons")
	lessonPath := filepath.Join(lessonsDir, "serialized-fix", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(lessonPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "serialized-fix", Owner: "codex", Date: "2026-08-12"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	index := "# Lessons\n\n## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n\n_No lessons recorded yet._\n"
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := lesson.Parse(lessonPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, parsed); err != nil {
		t.Fatal(err)
	}

	indexLocked := make(chan struct{})
	allowDiscovery := make(chan struct{})
	checker := &lessonRulesChecker{fixIndex: true}
	checker.withIndexLock = func(root string, mutate func() error) error {
		return withLessonIndexLock(root, defaultLessonIndexLockDeps(), func() error {
			close(indexLocked)
			<-allowDiscovery
			return mutate()
		})
	}
	fixDone := make(chan error, 1)
	go func() { fixDone <- checker.fix(specRoot) }()
	select {
	case <-indexLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("whole-index fix did not acquire the shared index lock")
	}

	artifactPublished := make(chan struct{})
	transitionDone := make(chan error, 1)
	go func() {
		_, transitionErr := lesson.ChangeStatus(lesson.ChangeStatusOptions{
			SpecRoot: projectRoot,
			Slug:     "serialized-fix",
			To:       lifecycle.LessonStated,
			PostMutation: func() error {
				close(artifactPublished)
				current, parseErr := lesson.Parse(lessonPath)
				if parseErr != nil {
					return parseErr
				}
				return UpsertLessonIndexRow(specRoot, current)
			},
		})
		transitionDone <- transitionErr
	}()
	select {
	case <-artifactPublished:
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle writer did not publish before index reconciliation")
	}
	close(allowDiscovery)

	select {
	case err := <-fixDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("whole-index fix deadlocked while holding the shared index lock")
	}
	select {
	case err := <-transitionDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle row reconcile did not resume after whole-index fix")
	}
	finalLesson, err := lesson.Parse(lessonPath)
	if err != nil {
		t.Fatal(err)
	}
	if finalLesson.Status != "Stated" {
		t.Fatalf("final Lesson status = %q", finalLesson.Status)
	}
	got, err := os.ReadFile(filepath.Join(lessonsDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("| [serialized-fix](serialized-fix/README.md) | Stated |")) {
		t.Fatalf("whole-index fix lost the cooperating lifecycle row:\n%s", got)
	}

	boom := errors.New("whole-index lock")
	checker.withIndexLock = func(string, func() error) error { return boom }
	if err := checker.fix(specRoot); !errors.Is(err, boom) {
		t.Fatalf("whole-index lock failure = %v", err)
	}
}

func TestLessonRules_CanonicalLessonChecksConfiguredControlAndOccurrenceSchema(t *testing.T) {
	projectRoot := t.TempDir()
	specRoot := filepath.Join(projectRoot, "spec")
	lessonsDir := filepath.Join(specRoot, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\nlessons:\n  classifications:\n    - process\n"
	if err := os.WriteFile(filepath.Join(projectRoot, "specscore.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	lessonPath := filepath.Join(lessonsDir, "durable-boundary", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(lessonPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "durable-boundary", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	c := newLessonRulesChecker()
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("scaffolded canonical lesson with configured class must lint clean: %#v", violations)
	}

	enforced := strings.Replace(string(body), "status: Recorded", "status: Enforced", 1)
	enforced = strings.Replace(enforced, "**Status:** Recorded", "**Status:** Enforced", 1)
	enforced = strings.Replace(enforced, "**Control:** —", "**Control:** run the deterministic gate", 1)
	enforced = strings.Replace(enforced, "**Verification:** —", "**Verification:** manual review", 1)
	enforced = strings.Replace(enforced, "**Evidence:** —", "**Evidence:** sha256:0123456789abcdef", 1)
	if err := os.WriteFile(lessonPath, []byte(enforced), 0o644); err != nil {
		t.Fatal(err)
	}
	badOccurrence := filepath.Join(filepath.Dir(lessonPath), "occurrences", "01234567-89ab-4def-8123-456789abcdef.json")
	if err := os.WriteFile(badOccurrence, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	violations, err = c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if lessonViolation(violations, "L-007") == nil || lessonViolation(violations, "L-009") == nil {
		t.Fatalf("enforced manual verification and malformed child must both be visible: %#v", violations)
	}
}

func TestUpsertLessonIndexRow_IsBoundedIdempotentAndRejectsDuplicateOwnership(t *testing.T) {
	specRoot := t.TempDir()
	lessonsDir := filepath.Join(specRoot, "lessons")
	lessonPath := filepath.Join(lessonsDir, "durable-boundary", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(lessonPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "durable-boundary", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(lessonsDir, "README.md")
	index := "# Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := lesson.Parse(lessonPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "| [durable-boundary](durable-boundary/README.md) | Recorded | process | 0 |  | — |") {
		t.Fatalf("canonical projection missing from bounded upsert:\n%s", first)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("repeating the same bounded index upsert changed unrelated content")
	}
	duplicated := append(append([]byte(nil), second...), []byte("| [durable-boundary](durable-boundary/README.md) | Recorded | process | 0 |  | — |\n")...)
	if err := os.WriteFile(indexPath, duplicated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err == nil {
		t.Fatal("ambiguous duplicate row ownership must be refused")
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(duplicated, after) {
		t.Fatal("duplicate-row refusal mutated the index")
	}
}

// A flat Lesson already listed in the six-column table owns exactly one row,
// linked as <slug>.md. Migrating it to directory form rewrites that same row —
// the index contract is one row per slug, not one row per link shape.
func TestUpsertLessonIndexRow_RewritesStaleFlatRowForTheSameSlug(t *testing.T) {
	specRoot := t.TempDir()
	lessonsDir := filepath.Join(specRoot, "lessons")
	lessonPath := filepath.Join(lessonsDir, "migrated-boundary", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(lessonPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "migrated-boundary", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(lessonsDir, "README.md")
	index := "# Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n" +
		"| [untouched](untouched.md) | Recorded | Legacy | 0 |  | — |\n" +
		"| [migrated-boundary](migrated-boundary.md) | Recorded | Legacy | 1 |  | — |\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := lesson.Parse(lessonPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, l); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(after), "[migrated-boundary]"); got != 1 {
		t.Fatalf("expected exactly one migrated-boundary row, got %d:\n%s", got, after)
	}
	if !strings.Contains(string(after), "| [migrated-boundary](migrated-boundary/README.md) | Recorded | process | 0 |  | — |") {
		t.Fatalf("canonical projection missing after flat-row rewrite:\n%s", after)
	}
	if !strings.Contains(string(after), "| [untouched](untouched.md) | Recorded | Legacy | 0 |  | — |") {
		t.Fatalf("bounded upsert disturbed an unrelated flat row:\n%s", after)
	}
}

func TestLessonRules_EvidenceAndRelationsRequirePortableVerifiableReferences(t *testing.T) {
	projectRoot := t.TempDir()
	specRoot := filepath.Join(projectRoot, "spec")
	if err := os.MkdirAll(filepath.Join(specRoot, "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "evidence.txt"), []byte("receipt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"sha256:0123456789abcdef0", true},
		{"evidence.txt", true},
		{"https://github.com/example/repo/commit/abc", true},
		{"../escape", false},
		{"/absolute", false},
		{"missing.txt", false},
	} {
		if got := stableLessonEvidence(specRoot, tc.value); got != tc.want {
			t.Fatalf("stableLessonEvidence(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}

	for _, slug := range []string{"a", "b"} {
		path := filepath.Join(specRoot, "lessons", slug, "README.md")
		if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Owner: "codex", Date: "2026-08-10"}, []string{"process"})
		if err != nil {
			t.Fatal(err)
		}
		if slug == "a" {
			body = []byte(strings.Replace(string(body), "**Supersedes:** —", "**Supersedes:** b", 1))
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	violations, err := newLessonRulesChecker().check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if lessonViolation(violations, "L-005") == nil || lessonViolation(violations, "L-008") == nil {
		t.Fatalf("missing configured vocabulary and unmatched inverse relation must be visible: %#v", violations)
	}
}

func TestUpsertLessonIndexRow_PreservesLegacyTableColumnsAndRejectsAmbiguity(t *testing.T) {
	e := newLessonRulesEnv(t)
	path := e.writeLesson(t, "legacy-boundary", fullLesson("Recorded"))
	l, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	e.writeIndex(t, "# Lessons\n\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n## Open Questions\n\nNone at this time.\n")
	if err := UpsertLessonIndexRow(e.specRoot, l); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(e.specRoot, "lessons", "README.md")
	first, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "| [legacy-boundary](legacy-boundary.md) | Recorded | 0 |  |  |") {
		t.Fatalf("legacy upsert omitted compatibility row:\n%s", first)
	}
	duplicated := append(append([]byte(nil), first...), []byte("| [legacy-boundary](legacy-boundary.md) | Recorded | 0 |  |  |\n")...)
	if err := os.WriteFile(indexPath, duplicated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(e.specRoot, l); err == nil {
		t.Fatal("ambiguous legacy ownership must be refused")
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(duplicated, after) {
		t.Fatal("legacy duplicate refusal mutated the index")
	}
}

func TestLessonFooterWalksAndIndexRowsTreatCanonicalLessonPathsPrecisely(t *testing.T) {
	specRoot := t.TempDir()
	index := filepath.Join(specRoot, "lessons", "README.md")
	lessonReadme := filepath.Join(specRoot, "lessons", "durable-boundary", "README.md")
	if err := os.MkdirAll(filepath.Dir(lessonReadme), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte("# Lessons\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonReadme, []byte("# Lesson: Durable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var indexPaths, lessonPaths []string
	if err := walkLessonsIndex(specRoot, func(path string, _ []byte) { indexPaths = append(indexPaths, path) }); err != nil {
		t.Fatal(err)
	}
	if err := walkLessonReadmes(specRoot, func(path string, _ []byte) { lessonPaths = append(lessonPaths, path) }); err != nil {
		t.Fatal(err)
	}
	if len(indexPaths) != 1 || indexPaths[0] != index || len(lessonPaths) != 1 || lessonPaths[0] != lessonReadme {
		t.Fatalf("lesson footer walkers included the wrong documents: index=%v lessons=%v", indexPaths, lessonPaths)
	}
	if phantom, ok := phantomDirInTableRowAtColumn("| [durable-boundary](durable-boundary/README.md) | summary |", 0, map[string]bool{}); !ok || phantom != "durable-boundary" {
		t.Fatalf("canonical first-column child link not recognized: %q, %v", phantom, ok)
	}
	if phantom, ok := phantomDirInTableRowAtColumn("| summary [other](other/README.md) | [durable-boundary](durable-boundary/README.md) |", 0, map[string]bool{}); ok || phantom != "" {
		t.Fatalf("non-identity-column link must not become a phantom child: %q, %v", phantom, ok)
	}
}

func TestCanonicalLint_ReportsMalformedDurableControlMetadataAndRelations(t *testing.T) {
	root := t.TempDir()
	l := &lesson.Lesson{
		Path:                filepath.Join(root, "missing.md"),
		Slug:                "broken",
		Canonical:           true,
		OccurrencesDir:      filepath.Join(root, "missing-occurrences"),
		Status:              "Enforced",
		StatusLine:          4,
		Date:                "not-a-date",
		DateLine:            3,
		Owner:               "",
		Classifications:     []string{"unknown", "unknown"},
		ClassificationsLine: 2,
		LegacyProvenance:    "",
		DuplicateOf:         "missing-target",
		DuplicateOfLine:     2,
		Supersedes:          "missing-target",
		SupersedesLine:      2,
		SupersededBy:        "",
		FieldCounts:         map[string]int{"Status": 2},
		SectionLines:        map[string]int{"Lesson": 8, "Process Gap": 7, "Tracking": 6, "Enforcement": 5},
		Control:             "—",
		Verification:        "manual review",
		Evidence:            "../escape",
		HasLessonTitle:      false,
	}
	canonical := lintCanonicalLesson(filepath.Join(root, "spec"), l, "lessons/broken/README.md", map[string]bool{"process": true}, nil)
	if lessonViolation(canonical, "L-005") == nil || lessonViolation(canonical, "L-006") == nil || lessonViolation(canonical, "L-007") == nil {
		t.Fatalf("malformed canonical durable control must emit L-005/L-006/L-007: %#v", canonical)
	}
	occurrences := lintOccurrenceChildren(filepath.Join(root, "spec"), l)
	if lessonViolation(occurrences, "L-009") == nil {
		t.Fatalf("missing canonical occurrence store must emit L-009: %#v", occurrences)
	}
	relations := lintLessonRelations(filepath.Join(root, "spec"), map[string]*lesson.Lesson{"broken": l})
	if lessonViolation(relations, "L-008") == nil {
		t.Fatalf("unresolved canonical relation must emit L-008: %#v", relations)
	}
}

func TestLessonClassifications_RejectsAbsentEmptyInvalidAndDuplicateVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"absent", "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n"},
		{"empty", "# SpecScore Repo Config Schema: https://specscore.md/repo-config\nlessons:\n  classifications: []\n"},
		{"invalid", "# SpecScore Repo Config Schema: https://specscore.md/repo-config\nlessons:\n  classifications: [Not Safe]\n"},
		{"duplicate", "# SpecScore Repo Config Schema: https://specscore.md/repo-config\nlessons:\n  classifications: [process, process]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, "specscore.yaml"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := lessonClassifications(filepath.Join(project, "spec")); err == nil {
				t.Fatal("unsafe classification vocabulary must be rejected")
			}
		})
	}
}

func TestLessonIndexParserAndRewriteRefuseAmbiguousOrMalformedTables(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	for _, body := range []string{
		"## Lessons\n\n| malformed |\n",
		"## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n| not-a-link | x | x | 0 | | — |\n",
		"## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n| [wrong](different/README.md) | x | x | 0 | | — |\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, malformed, err := readLessonIndexRows(path)
		if err != nil || !malformed {
			t.Fatalf("malformed index row must be reported: malformed=%v err=%v", malformed, err)
		}
	}
	if _, _, ok := parseMarkdownLink("[unterminated](x"); ok {
		t.Fatal("unterminated markdown link must be rejected")
	}
	if firstMarkdownCell("||") != "" {
		t.Fatal("empty markdown row must not manufacture a link cell")
	}
	if err := os.WriteFile(path, []byte("# No index here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteLessonIndex(path, nil, nil); err == nil {
		t.Fatal("rewrite without an index heading must be refused")
	}
	if err := os.WriteFile(path, []byte("# Lessons\n\n## Lessons\n\nold\n\n## Open Questions\n\nNone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteLessonIndex(path, nil, map[string]*lesson.Lesson{}); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "_No lessons recorded yet._") || !strings.Contains(string(updated), "## Open Questions") {
		t.Fatalf("empty index rewrite must preserve epilogue and write empty projection:\n%s", updated)
	}
	if err := UpsertLessonIndexRow(root, nil); err == nil {
		t.Fatal("nil bounded-upsert target must be refused")
	}
}

func TestLessonRules_RejectsMixedLayoutsMalformedRelationsAndOccurrenceStores(t *testing.T) {
	root := t.TempDir()
	lessons := filepath.Join(root, "lessons")
	if err := os.MkdirAll(filepath.Join(lessons, "same", "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "same.md"), []byte(fullLesson("Recorded")), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "same", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "same", "README.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := newLessonRulesChecker().check(root); err != nil || lessonViolation(got, "L-005") == nil {
		t.Fatalf("mixed flat/canonical layout must be visible: %#v, %v", got, err)
	}

	// In a canonical-only tree, malformed relation metadata and both directory
	// and non-JSON occurrence entries are refused rather than silently ignored.
	if err := os.Remove(filepath.Join(lessons, "same.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, ".relations.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(lessons, "same", "occurrences", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "same", "occurrences", "note.txt"), []byte("not JSON"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := newLessonRulesChecker().check(root)
	if err != nil {
		t.Fatal(err)
	}
	if lessonViolation(got, "L-008") == nil || lessonViolation(got, "L-009") == nil {
		t.Fatalf("malformed relation and occurrence storage must both be visible: %#v", got)
	}
}

func TestLessonIndexRules_ReportsProjectionDriftOrphansAndDuplicates(t *testing.T) {
	root := t.TempDir()
	lessons := filepath.Join(root, "lessons")
	if err := os.MkdirAll(filepath.Join(lessons, "one", "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "one", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(lessons, "one", "README.md")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	index := "## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n| [one](one/README.md) | Wrong | process | 0 |  | — |\n| [ghost](ghost/README.md) | Recorded | process | 0 |  | — |\n| [ghost](ghost/README.md) | Recorded | process | 0 |  | — |\n"
	if err := os.WriteFile(filepath.Join(lessons, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	vs, fixed := lessonIndexRules(root, map[string]*lesson.Lesson{"one": l}, false)
	if fixed || lessonViolation(vs, "L-004") == nil {
		t.Fatalf("drifted/orphaned/duplicate projection must be reported: %#v fixed=%v", vs, fixed)
	}
	vs, fixed = lessonIndexRules(root, map[string]*lesson.Lesson{"one": l}, true)
	if !fixed || len(vs) != 0 {
		t.Fatalf("bounded index fix must regenerate canonical projection: %#v fixed=%v", vs, fixed)
	}
}

func TestLessonRuleEdgeFailuresRemainVisibleAndWriteFree(t *testing.T) {
	root := t.TempDir()
	unsafe := filepath.Join(root, "unsafe.md")
	if err := os.WriteFile(unsafe, []byte("token=not-allowed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := &lesson.Lesson{Path: unsafe, Slug: "cycle", Canonical: true, HasLessonTitle: true, Title: "Cycle", Status: "Recorded", StatusLine: 1, Date: "bad", DateLine: 2, Owner: "codex", OwnerLine: 3, ClassificationsLine: 4, LegacyProvenance: "—", LegacyProvenanceLine: 5, DuplicateOf: "—", DuplicateOfLine: 6, Supersedes: "cycle", SupersedesLine: 7, SupersededBy: "—", SupersededByLine: 8, FieldCounts: map[string]int{}, SectionLines: map[string]int{}}
	if vs := lintCanonicalLesson(filepath.Join(root, "spec"), l, "cycle", map[string]bool{"process": true}, nil); lessonViolation(vs, "L-005") == nil {
		t.Fatal("unsafe README and invalid date must remain visible")
	}
	if vs := lintCanonicalLesson(filepath.Join(root, "spec"), l, "cycle", map[string]bool{"process": true}, nil); len(vs) == 0 {
		t.Fatal("empty canonical classifications must be visible")
	}
	if vs := lintLessonRelations(filepath.Join(root, "spec"), map[string]*lesson.Lesson{"cycle": l}); lessonViolation(vs, "L-008") == nil {
		t.Fatal("self supersession cycle must be visible")
	}
	l.OccurrencesDir = filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(l.OccurrencesDir, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if vs := lintOccurrenceChildren(filepath.Join(root, "spec"), l); lessonViolation(vs, "L-009") == nil {
		t.Fatal("non-directory occurrence store must be visible")
	}
	if err := os.MkdirAll(filepath.Join(root, "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(root, l); err == nil {
		t.Fatal("upsert without index must be write-free failure")
	}
	legacyIndex := filepath.Join(root, "lessons", "README.md")
	if err := os.WriteFile(legacyIndex, []byte("# Lessons\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := &lesson.Lesson{Slug: "legacy", Status: "Recorded", Recurred: 1}
	if err := UpsertLessonIndexRow(root, legacy); err == nil {
		t.Fatal("legacy upsert without compatibility table must fail")
	}
}

func TestLessonIndexProjectionTracksOccurrenceEvidenceAndLegacyColumns(t *testing.T) {
	root := t.TempDir()
	lessons := filepath.Join(root, "lessons")
	path := filepath.Join(lessons, "durable", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "durable", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lesson.AddOccurrence(lesson.AddOccurrenceOptions{LessonPath: path, ID: "01234567-89ab-4def-8123-456789abcdef", Summary: "Durable projection observes child evidence.", Now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	row, err := expectedLessonIndexRow("durable", l)
	if err != nil || row.occurrences != "1" || row.lastOccurred == "" {
		t.Fatalf("canonical row must project occurrence evidence: %#v, %v", row, err)
	}
	l.DuplicateOf = "retained"
	if row, err = expectedLessonIndexRow("durable", l); err != nil || row.enforcement != "Duplicate Of: retained" {
		t.Fatalf("duplicate projection = %#v, %v", row, err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "README.md"), []byte("## Lessons\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(root, l); err == nil {
		t.Fatal("canonical header without separator must be refused")
	}
	legacyPath := filepath.Join(lessons, "legacy.md")
	if err := os.WriteFile(legacyPath, []byte(fullLesson("Recorded")), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy, err := lesson.Parse(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyIndex := "# Lessons\n\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n| [legacy](legacy.md) | Old | 9 | 2020-01-01 | owner |\n"
	if err := os.WriteFile(filepath.Join(lessons, "README.md"), []byte(legacyIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(root, legacy); err != nil {
		t.Fatal(err)
	}
	updated, _ := os.ReadFile(filepath.Join(lessons, "README.md"))
	if !strings.Contains(string(updated), "| [legacy](legacy.md) | Recorded | 0 | 2020-01-01 | owner |") {
		t.Fatalf("legacy update must preserve date/owner columns:\n%s", updated)
	}
}

func TestLessonIndexFailuresPropagateMalformedOccurrenceInsteadOfProjectingIt(t *testing.T) {
	root := t.TempDir()
	lessons := filepath.Join(root, "lessons")
	path := filepath.Join(lessons, "broken", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "broken", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.OccurrencesDir, "01234567-89ab-4def-8123-456789abcdef.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := expectedLessonIndexRow(l.Slug, l); err == nil {
		t.Fatal("malformed immutable occurrence must prevent an index projection")
	}
	index := filepath.Join(lessons, "README.md")
	if err := os.WriteFile(index, []byte("## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteLessonIndex(index, []string{l.Slug}, map[string]*lesson.Lesson{l.Slug: l}); err == nil {
		t.Fatal("full index rewrite must propagate malformed occurrence evidence")
	}
	if err := UpsertLessonIndexRow(root, l); err == nil {
		t.Fatal("bounded upsert must propagate malformed occurrence evidence")
	}
	if err := os.WriteFile(index, []byte("## Lessons\nplain prose is not a table row\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readLessonIndexRows(index); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertLessonIndexRow_SkipsExistingProjectionRowsBeforeInsertion(t *testing.T) {
	root := t.TempDir()
	lessons := filepath.Join(root, "lessons")
	for _, slug := range []string{"existing", "new"} {
		path := filepath.Join(lessons, slug, "README.md")
		if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Owner: "codex", Date: "2026-08-10"}, []string{"process"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := "## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n| [existing](existing/README.md) | Recorded | process | 0 |  | — |\n"
	if err := os.WriteFile(filepath.Join(lessons, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := lesson.Parse(filepath.Join(lessons, "new", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(root, l); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertLessonIndexRow_MigratesLegacyProjectionForFirstCanonicalLesson(t *testing.T) {
	specRoot := t.TempDir()
	lessons := filepath.Join(specRoot, "lessons")
	legacyPath := filepath.Join(lessons, "legacy.md")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(fullLesson("Recorded")), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalPath := filepath.Join(lessons, "canonical", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(canonicalPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "canonical", Owner: "codex", Date: "2026-08-10"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonicalPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(lessons, "README.md")
	legacyIndex := "## Lessons\n\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n| [legacy](legacy.md) | Recorded | 0 | 2026-08-10 | codex |\n"
	if err := os.WriteFile(indexPath, []byte(legacyIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	canonical, err := lesson.Parse(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, canonical); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |") || !strings.Contains(text, "[legacy](legacy.md)") || !strings.Contains(text, "[canonical](canonical/README.md)") {
		t.Fatalf("first canonical lesson must rewrite the complete flat projection:\n%s", got)
	}

	if err := os.WriteFile(filepath.Join(lessons, "canonical.md"), []byte(fullLesson("Recorded")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte(legacyIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLessonIndexRow(specRoot, canonical); err == nil || !strings.Contains(err.Error(), "discovering Lessons") {
		t.Fatalf("duplicate co-owned Lesson must prevent an incomplete migration, got %v", err)
	}
}

// TestLessonIndexFix_JoinsWrappedEnforcementParagraph guards against REQ
// regression: the same truncation the feature-index Summary fix (c910c04)
// closed also existed in the Lesson-index projection. A canonical Lesson's
// **Control:** value is routinely hand-wrapped across multiple source lines
// (see spec/lessons/<slug>/README.md in sneat-co/backstage); before this
// fix, `spec lint --fix` regenerated the Enforcement cell from only the
// field's first physical line, cutting it mid-sentence.
func TestLessonIndexFix_JoinsWrappedEnforcementParagraph(t *testing.T) {
	specRoot := t.TempDir()
	lessonsDir := filepath.Join(specRoot, "lessons")
	slug := "clean-clone-guard"
	lessonPath := filepath.Join(lessonsDir, slug, "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(lessonPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Lesson: Clean Clone Guard\n\n" +
		"**Status:** Recorded\n**Date:** 2026-08-27\n**Owner:** alex\n" +
		"**Classifications:** tooling-determinism\n" +
		"**Legacy Provenance:** —\n**Duplicate Of:** —\n**Supersedes:** —\n**Superseded By:** —\n\n" +
		"## Lesson\n\nx\n\n## Process Gap\n\nx\n\n## Tracking\n\n" +
		"- **Occurrence store:** `occurrences/`\n" +
		"- **Recurrence metadata:** derived from child JSON; never hand-maintained here.\n" +
		"- **Occurrence schema:** `https://specscore.md/new/lesson-occurrence.schema.json`\n\n" +
		"## Enforcement\n\n" +
		"**Control:** the clean-clone guard resolves each candidate write to an absolute\n" +
		"path and refuses only when that path lies inside a canonical clone and outside\n" +
		"any linked worktree cut from it — never on command name or shell cwd.\n" +
		"**Verification:** a regression suite.\n**Evidence:** —\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(lessonPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	index := "# Lessons\n\n## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n\n_No lessons recorded yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	l, err := lesson.Parse(lessonPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed := map[string]*lesson.Lesson{slug: l}

	vs, fixed := lessonIndexRules(specRoot, parsed, true)
	if !fixed {
		t.Fatalf("expected the index to be rewritten, violations: %+v", vs)
	}

	got, err := os.ReadFile(filepath.Join(lessonsDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "the clean-clone guard resolves each candidate write to an absolute " +
		"path and refuses only when that path lies inside a canonical clone and outside " +
		"any linked worktree cut from it — never on command name or shell cwd."
	if !strings.Contains(string(got), want) {
		t.Fatalf("regenerated index Enforcement cell must contain the full joined paragraph, not a mid-sentence cut:\n%s", got)
	}
	if strings.Contains(string(got), "to an absolute |") {
		t.Fatalf("regenerated index Enforcement cell was truncated mid-sentence:\n%s", got)
	}
}

func TestLessonClassificationAndMarkdownCellCompatibilityBranches(t *testing.T) {
	classes, err := lessonClassificationsFrom([]string{"process", "quality"})
	if err != nil || !classes["process"] || !classes["quality"] {
		t.Fatalf("typed classification compatibility = %#v, %v", classes, err)
	}
	if got := firstMarkdownCell(""); got != "" {
		t.Fatalf("empty markdown row first cell = %q", got)
	}
}
