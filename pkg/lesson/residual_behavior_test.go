package lesson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type residualFS struct {
	lessonFS
	readFile func(string) ([]byte, error)
	stat     func(string) (os.FileInfo, error)
	link     func(string, string) error
}

type legacyRollbackFS struct {
	lessonFS
	mode            string
	rollback        bool
	manifestRemoved bool
}

func (fs *legacyRollbackFS) Link(oldname, newname string) error {
	if fs.mode != "occurrence-remove-all" && strings.Contains(newname, string(filepath.Separator)+".legacy-import"+string(filepath.Separator)) {
		fs.rollback = true
		return mutationFailure(MutationPrePublication, errors.New("manifest publication"))
	}
	return fs.lessonFS.Link(oldname, newname)
}
func (fs *legacyRollbackFS) RemoveAll(path string) error {
	if fs.rollback && (fs.mode == "remove-all" || fs.mode == "occurrence-remove-all") {
		return errors.New("rollback remove-all")
	}
	return fs.lessonFS.RemoveAll(path)
}
func (fs *legacyRollbackFS) Remove(path string) error {
	if fs.rollback && filepath.Base(path) == ".legacy-import" {
		if fs.mode == "remove" {
			return errors.New("rollback remove")
		}
		fs.manifestRemoved = true
	}
	return fs.lessonFS.Remove(path)
}
func (fs *legacyRollbackFS) Open(path string) (lessonFile, error) {
	if fs.rollback && fs.manifestRemoved && fs.mode == "sync" {
		return nil, errors.New("rollback sync")
	}
	return fs.lessonFS.Open(path)
}
func (fs *legacyRollbackFS) CreateTemp(dir, pattern string) (lessonFile, error) {
	if fs.mode == "occurrence-remove-all" && strings.HasSuffix(dir, filepath.Join("reviewed-rule", "occurrences")) {
		f, err := fs.lessonFS.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &legacyRollbackFile{lessonFile: f, fs: fs}, nil
	}
	return fs.lessonFS.CreateTemp(dir, pattern)
}

type legacyRollbackFile struct {
	lessonFile
	fs *legacyRollbackFS
}

func (f *legacyRollbackFile) Chmod(os.FileMode) error {
	f.fs.rollback = true
	return errors.New("occurrence preparation")
}

func (fs residualFS) ReadFile(path string) ([]byte, error) {
	if fs.readFile != nil {
		return fs.readFile(path)
	}
	return fs.lessonFS.ReadFile(path)
}
func (fs residualFS) Stat(path string) (os.FileInfo, error) {
	if fs.stat != nil {
		return fs.stat(path)
	}
	return fs.lessonFS.Stat(path)
}
func (fs residualFS) Link(oldname, newname string) error {
	if fs.link != nil {
		return fs.link(oldname, newname)
	}
	return fs.lessonFS.Link(oldname, newname)
}

func TestPreflightFlatMigrationRejectsEveryInvalidBoundaryWithoutPublishing(t *testing.T) {
	if err := validateFlatMigrationOptions(FlatMigrationOptions{Slug: "rule"}); err == nil {
		t.Fatal("missing classifications accepted")
	}

	lessons, opts, deps := flatMatrixFixture(t, "baseline")
	before := snapshotTree(t, lessons)
	baselineFS := &faultMatrixFS{lessonFS: osLessonFS{}}
	deps.fs = baselineFS
	if _, err := preflightFlatMigrationWithDeps(opts, deps); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, snapshotTree(t, lessons)) {
		t.Fatal("successful preflight changed the complete Lesson tree")
	}
	for failAt, op := range baselineFS.trace {
		if op == "RemoveAll" || op == "Remove" || op == "Close" {
			continue
		}
		t.Run(fmt.Sprintf("filesystem-%03d-%s", failAt+1, op), func(t *testing.T) {
			lessons, opts, deps := flatMatrixFixture(t, "rule")
			before := snapshotTree(t, lessons)
			deps.fs = &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt + 1}
			if _, err := preflightFlatMigrationWithDeps(opts, deps); err == nil {
				t.Fatalf("injected %s failure accepted", op)
			}
			if !bytes.Equal(before, snapshotTree(t, lessons)) {
				t.Fatal("failed preflight changed the complete Lesson tree")
			}
		})
	}

	for name, marker := range map[string][]byte{
		"malformed": []byte("{"),
		"invalid":   []byte(`{"schema_version":2}`),
	} {
		t.Run("marker-"+name, func(t *testing.T) {
			lessons, opts, deps := flatMatrixFixture(t, "rule")
			if err := os.WriteFile(filepath.Join(lessons, ".flat-migration-rule.json"), marker, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := preflightFlatMigrationWithDeps(opts, deps); err == nil {
				t.Fatal("invalid durable marker accepted")
			}
		})
	}

	_, opts, deps = flatMatrixFixture(t, "rule")
	deps.sourceIdentity = func(string, []byte) (LegacySourceRef, error) { return LegacySourceRef{}, errors.New("identity") }
	if _, err := preflightFlatMigrationWithDeps(opts, deps); err == nil {
		t.Fatal("source identity failure accepted")
	}
	_, opts, deps = flatMatrixFixture(t, "rule")
	deps.sourceIdentity = func(string, []byte) (LegacySourceRef, error) { return LegacySourceRef{}, nil }
	if _, err := preflightFlatMigrationWithDeps(opts, deps); err == nil {
		t.Fatal("invalid source identity accepted")
	}

	// A real finalized tree, including its exact denormalized index row, is the
	// only marker-free state that preflight may classify as already migrated.
	lessons, opts, deps = flatMatrixFixture(t, "completed")
	result, err := migrateFlatWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	writeFlatMigrationIndex(t, lessons, result.CanonicalPath)
	if err := finalizeFlatMigrationWithDeps(opts, FlatMigrationEventUUID(depsSourceSHA(t, result.ManifestPath), opts.Slug), deps); err != nil {
		t.Fatal(err)
	}
	preflight, err := preflightFlatMigrationWithDeps(opts, deps)
	if err != nil || !preflight.AlreadyMigrated {
		t.Fatalf("completed preflight=%#v err=%v", preflight, err)
	}
}

func depsSourceSHA(t *testing.T, manifestPath string) string {
	t.Helper()
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest flatMigrationManifest
	if err := decodeStrictJSON(b, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.Source.SHA256
}

func finalizedFlatMatrixFixture(t *testing.T) (string, FlatMigrationOptions, string, flatMigrationDeps) {
	t.Helper()
	lessons, opts, deps := flatMatrixFixture(t, "rule")
	result, err := migrateFlatWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	writeFlatMigrationIndex(t, lessons, result.CanonicalPath)
	return lessons, opts, FlatMigrationEventUUID(depsSourceSHA(t, result.ManifestPath), opts.Slug), deps
}

func TestFinalizeFlatMigrationVerifiesCompleteTreeAtEveryFilesystemBoundary(t *testing.T) {
	_, opts, eventID, deps := finalizedFlatMatrixFixture(t)
	baselineFS := &faultMatrixFS{lessonFS: osLessonFS{}}
	deps.fs = baselineFS
	if err := finalizeFlatMigrationWithDeps(opts, eventID, deps); err != nil {
		t.Fatal(err)
	}
	for failAt, op := range baselineFS.trace {
		if op == "Close" {
			continue
		}
		t.Run(fmt.Sprintf("%03d-%s", failAt+1, op), func(t *testing.T) {
			lessons, opts, eventID, deps := finalizedFlatMatrixFixture(t)
			before := snapshotTree(t, lessons)
			deps.fs = &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt + 1}
			err := finalizeFlatMigrationWithDeps(opts, eventID, deps)
			if err == nil {
				t.Fatalf("injected %s failure accepted", op)
			}
			after := snapshotTree(t, lessons)
			if bytes.Equal(before, after) {
				return
			}
			marker := filepath.Join(lessons, ".flat-migration-rule.json")
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("changed finalization tree retained marker: %v", statErr)
			}
			if proofErr := validateCompletedFlatMigrationProofWithFS(lessons, filepath.Join(lessons, "rule", "README.md"), "rule", osLessonFS{}); proofErr != nil {
				t.Fatalf("post-marker failure lacks complete canonical/index/manifest proof: %v", proofErr)
			}
		})
	}
}

func TestFlatMigrationStageAndProofRejectMalformedCompleteArtifacts(t *testing.T) {
	validSource := coverageLegacySource()
	validSource.Path = "spec/lessons/rule.md"
	for name, body := range map[string]string{
		"parse":           strings.Repeat("x", 1<<20),
		"not structured":  "# note\n",
		"status mismatch": strings.Replace(flatFixture("Recorded", 0, ""), "status: Recorded", "status: Stated", 1),
		"bad recurred":    strings.Replace(flatFixture("Recorded", 0, ""), "**Recurred:** 0", "**Recurred:** no", 1),
		"bad date":        strings.Replace(flatFixture("Recorded", 0, ""), "2026-08-01", "not-a-date", 1),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "rule.md")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			ref := validSource
			ref.SHA256, ref.ByteCount = shaString([]byte(body)), len(body)
			if _, _, _, err := stageFlatMigrationWithDeps(FlatMigrationOptions{LessonsDir: root, Slug: "rule", Classifications: []string{"process"}}, []byte(body), path, ref, osLessonFS{}); err == nil {
				t.Fatal("invalid flat source staged")
			}
		})
	}

	body := flatFixture("Recorded", 0, "")
	root := t.TempDir()
	path := filepath.Join(root, "rule.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	validSource.SHA256, validSource.ByteCount = shaString([]byte(body)), len(body)
	if _, _, _, err := stageFlatMigrationWithDeps(FlatMigrationOptions{LessonsDir: root, Slug: "Bad Slug", Classifications: []string{"process"}}, []byte(body), path, validSource, osLessonFS{}); err == nil {
		t.Fatal("scaffold failure accepted")
	}
	if _, _, _, err := stageFlatMigrationWithDeps(FlatMigrationOptions{LessonsDir: root, Slug: "rule", Classifications: []string{"process"}, Control: "unexpected"}, []byte(body), path, validSource, osLessonFS{}); err == nil {
		t.Fatal("non-Enforced mapping accepted enforcement evidence")
	}

	if _, err := marshalSafeJSON("marshal", func() {}); err == nil {
		t.Fatal("unencodable marker accepted")
	}
	var marker flatMigrationMarker
	if err := decodeStrictJSON([]byte(`{"schema_version":1} x`), &marker); err == nil {
		t.Fatal("malformed trailing JSON accepted")
	}
	if err := requireOccurrenceJSONEOF(json.NewDecoder(strings.NewReader(`{}`))); err == nil {
		t.Fatal("extra occurrence JSON accepted")
	}

	lessons, opts, _, _ := finalizedFlatMatrixFixture(t)
	canonical := filepath.Join(lessons, opts.Slug, "README.md")
	if err := validateFlatMigrationIndexRowWithFS(lessons, canonical, osLessonFS{}); err != nil {
		t.Fatal(err)
	}
	if err := validateCompletedFlatMigrationProofWithFS(lessons, canonical, opts.Slug, osLessonFS{}); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPreflightRejectsEveryReviewedMappingContradictionWriteFree(t *testing.T) {
	type mutation func(string, *LegacyInventory, *LegacyMapping, *[]string)
	tests := map[string]mutation{
		"invalid source":     func(_ string, inv *LegacyInventory, _ *LegacyMapping, _ *[]string) { inv.Source.Repository = "bad" },
		"source mismatch":    func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Source.Path = "other" },
		"invalid vocabulary": func(_ string, _ *LegacyInventory, _ *LegacyMapping, allowed *[]string) { *allowed = []string{"Bad"} },
		"duplicate inventory key": func(_ string, inv *LegacyInventory, _ *LegacyMapping, _ *[]string) {
			inv.Entries = append(inv.Entries, inv.Entries[0])
		},
		"unknown mapping key": func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].Key = "missing" },
		"duplicate mapping key": func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) {
			m.Entries = append(m.Entries, m.Entries[0])
		},
		"missing disposition":   func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries = nil },
		"manual ignored fields": func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].Action = "manual" },
		"manual unresolved": func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) {
			m.Entries[0] = LegacyMappingEntry{Key: m.Entries[0].Key, Action: "manual"}
		},
		"marker creates lesson": func(_ string, inv *LegacyInventory, _ *LegacyMapping, _ *[]string) {
			inv.Entries[0].Kind = "recurrence-marker"
		},
		"missing title": func(_ string, inv *LegacyInventory, m *LegacyMapping, _ *[]string) {
			inv.Entries[0].Title, m.Entries[0].Title = "", ""
		},
		"unsafe source title": func(_ string, inv *LegacyInventory, m *LegacyMapping, _ *[]string) {
			inv.Entries[0].Title, m.Entries[0].Title = "person@example.test", ""
		},
		"missing lesson":         func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].Lesson = "" },
		"missing process gap":    func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].ProcessGap = "" },
		"wrong status":           func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].Status = "Stated" },
		"missing classification": func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].Classifications = nil },
		"bad classification": func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) {
			m.Entries[0].Classifications = []string{"validation"}
		},
		"flat collision": func(lessons string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) {
			_ = os.WriteFile(filepath.Join(lessons, m.Entries[0].Slug+".md"), []byte("x"), 0o644)
		},
		"target directory collision": func(lessons string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) {
			dir := filepath.Join(lessons, m.Entries[0].Slug)
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(filepath.Join(dir, "foreign"), []byte("foreign"), 0o644)
		},
		"invalid action":       func(_ string, _ *LegacyInventory, m *LegacyMapping, _ *[]string) { m.Entries[0].Action = "drop" },
		"missing local source": func(_ string, inv *LegacyInventory, _ *LegacyMapping, _ *[]string) { inv.localSource = "" },
		"changed source": func(_ string, inv *LegacyInventory, _ *LegacyMapping, _ *[]string) {
			_ = os.WriteFile(inv.localSource, []byte("changed"), 0o644)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			lessons, inv, mapping := legacyMatrixFixture(t)
			allowed := []string{"process"}
			mutate(lessons, &inv, &mapping, &allowed)
			before := snapshotTree(t, lessons)
			if _, err := preflightLegacyApplyWithFS(lessons, allowed, inv, mapping, osLessonFS{}); err == nil {
				t.Fatal("invalid mapping accepted")
			}
			if !bytes.Equal(before, snapshotTree(t, lessons)) {
				t.Fatal("preflight mutated the Lesson tree")
			}
		})
	}
}

func TestRelationResidualErrorsAndOrdering(t *testing.T) {
	if err := ValidateRelation("from", "related", "Bad To"); err == nil {
		t.Fatal("invalid destination accepted")
	}
	if _, err := listRelationsWithDeps(t.TempDir(), "missing", defaultRelationDeps()); err == nil {
		t.Fatal("missing requested Lesson accepted")
	}
	lessons := relationFixture(t, "a", "b", "c")
	if err := os.WriteFile(filepath.Join(lessons, relatedRelationsFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := listRelationsWithDeps(lessons, "a", defaultRelationDeps()); err == nil {
		t.Fatal("malformed sidecar accepted")
	}
	if _, err := relationFields(filepath.Join(lessons, "missing.md"), osLessonFS{}); err == nil {
		t.Fatal("missing relation metadata accepted")
	}
	flat := filepath.Join(lessons, "flat.md")
	if err := os.WriteFile(flat, []byte("**Duplicate Of:** a\n**Supersedes:** b\n**Superseded By:** c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fields, err := relationFields(flat, osLessonFS{})
	if err != nil || len(fields) != 3 || fields[0].From != "flat" {
		t.Fatalf("flat fields=%#v err=%v", fields, err)
	}

	deps := defaultRelationDeps()
	deps.marshal = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") }
	if err := appendRelatedRelationWithDeps(lessons, Relation{From: "a", Type: "related", To: "b"}, deps); err == nil {
		t.Fatal("relation marshal failure accepted")
	}
	if _, err := decodeRelatedRelations(lessons, []byte(`[{"from":"a","type":"related","to":"missing"}]`)); err == nil {
		t.Fatal("unresolved related endpoint accepted")
	}
	if _, err := decodeRelatedRelations(lessons, []byte(`[{"from":"missing","type":"related","to":"a"}]`)); err == nil {
		t.Fatal("unresolved related source accepted")
	}
}

func TestInventoryAndLegacyIdentityResidualFailures(t *testing.T) {
	deps := defaultLegacyImportDeps()
	deps.abs = func(string) (string, error) { return "", errors.New("abs") }
	if _, err := inventoryLegacyWithDeps("source", deps); err == nil {
		t.Fatal("absolute-path failure accepted")
	}
	deps = defaultLegacyImportDeps()
	deps.fs = &faultMatrixFS{lessonFS: osLessonFS{}, failAt: 1}
	if _, err := inventoryLegacyWithDeps("source", deps); err == nil {
		t.Fatal("source read failure accepted")
	}

	root := t.TempDir()
	source := filepath.Join(root, "LESSONS.md")
	raw := "## Lessons\n\n## L2 — two\n**Status:** Recorded\n\n## L2 — again\n**Status:** Recorded\n\n## L1 — one\n**Status:** Recorded\n\n## L1 — again\n**Status:** Recorded\n\n**Recurred malformed\n"
	if err := os.WriteFile(source, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	deps = defaultLegacyImportDeps()
	deps.sourceIdentity = func(string, []byte) (LegacySourceRef, error) { return LegacySourceRef{}, errors.New("not committed") }
	inv, err := inventoryLegacyWithDeps(source, deps)
	if err != nil || len(inv.Collisions) != 2 || len(inv.UnmatchedCandidates) == 0 {
		t.Fatalf("inventory=%#v err=%v", inv, err)
	}

	good := defaultLegacySourceIdentityDeps()
	good.evalSymlinks = func(string) (string, error) { return "/repo/spec/lessons.md", nil }
	good.root = func(string) (string, error) { return "/repo", nil }
	good.rel = func(string, string) (string, error) { return "spec/lessons.md", nil }
	good.repository = func(string) (string, error) { return "github.com/example/repo", nil }
	good.revision = func(string) (string, error) { return strings.Repeat("a", 40), nil }
	good.committedBytes = func(string, string, string) ([]byte, error) { return []byte("source"), nil }
	good.committedAt = func(string, string) (string, error) { return "2026-08-10T12:00:00Z", nil }
	if _, err := resolveLegacySourceIdentityWithDeps("source", []byte("source"), good); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*legacySourceIdentityDeps){
		"symlink": func(d *legacySourceIdentityDeps) {
			d.evalSymlinks = func(string) (string, error) { return "", errors.New("eval") }
		},
		"root": func(d *legacySourceIdentityDeps) {
			d.root = func(string) (string, error) { return "", errors.New("root") }
		},
		"relative": func(d *legacySourceIdentityDeps) {
			d.rel = func(string, string) (string, error) { return "", errors.New("rel") }
		},
		"portable": func(d *legacySourceIdentityDeps) {
			d.rel = func(string, string) (string, error) { return "../outside", nil }
		},
		"repository": func(d *legacySourceIdentityDeps) {
			d.repository = func(string) (string, error) { return "", errors.New("repo") }
		},
		"revision": func(d *legacySourceIdentityDeps) { d.revision = func(string) (string, error) { return "bad", nil } },
		"bytes": func(d *legacySourceIdentityDeps) {
			d.committedBytes = func(string, string, string) ([]byte, error) { return []byte("other"), nil }
		},
		"timestamp read": func(d *legacySourceIdentityDeps) {
			d.committedAt = func(string, string) (string, error) { return "", errors.New("time") }
		},
		"timestamp parse": func(d *legacySourceIdentityDeps) {
			d.committedAt = func(string, string) (string, error) { return "bad", nil }
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			deps := good
			mutate(&deps)
			if _, err := resolveLegacySourceIdentityWithDeps("source", []byte("source"), deps); err == nil {
				t.Fatal("identity failure accepted")
			}
		})
	}

	gitRoot := t.TempDir()
	if out, err := exec.Command("git", "-C", gitRoot, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if _, err := legacyRepositoryIdentity(gitRoot); err == nil {
		t.Fatal("repository without origin accepted")
	}
	if out, err := exec.Command("git", "-C", gitRoot, "remote", "add", "origin", "not-a-supported-remote").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	if _, err := legacyRepositoryIdentity(gitRoot); err == nil {
		t.Fatal("unsupported origin accepted")
	}

	ref := coverageLegacySource()
	ref.Path = "person@example.test"
	if err := validateLegacySourceRef(ref); err == nil {
		t.Fatal("unsafe source metadata accepted")
	}
	defer func() {
		if recover() == nil {
			t.Error("unencodable projection did not panic")
		}
	}()
	_ = mustMarshalLegacyProjection(func() {})
}

func TestLegacyArtifactValidationAndMarshalResiduals(t *testing.T) {
	lessons, inv, mapping := legacyMatrixFixture(t)
	entry := inv.Entries[0]
	path := filepath.Join(lessons, "rule", "README.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, setup := range map[string]func() lessonFS{
		"read": func() lessonFS { return &faultMatrixFS{lessonFS: osLessonFS{}, failAt: 1} },
		"unsafe": func() lessonFS {
			_ = os.WriteFile(path, []byte("person@example.test"), 0o644)
			return osLessonFS{}
		},
		"provenance": func() lessonFS {
			_ = os.WriteFile(path, []byte("safe"), 0o644)
			return osLessonFS{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateImportedLessonWithFS(path, entry, inv, setup()); err == nil {
				t.Fatal("invalid imported Lesson accepted")
			}
		})
	}
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "rule"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte("**Legacy Provenance:** —"), []byte("**Legacy Provenance:** "+legacyProvenance(inv, entry)), 1)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateImportedLessonWithFS(path, entry, inv, osLessonFS{}); err == nil {
		t.Fatal("missing occurrence store accepted")
	}

	badOccurrence := coverageOccurrence()
	badOccurrence.Context = map[string]any{}
	if err := validateLegacyOccurrence(badOccurrence, inv, entry, "rule"); err == nil {
		t.Fatal("wrong occurrence provenance accepted")
	}
	goodOccurrence := coverageOccurrence()
	opts := legacyOccurrenceOptions("", goodOccurrence.ID, inv, entry)
	goodOccurrence.OccurredAt, goodOccurrence.Summary, goodOccurrence.Evidence, goodOccurrence.Redactions, goodOccurrence.Context = opts.Now, opts.Summary, opts.Evidence, opts.Redactions, opts.Context
	goodOccurrence.Context = map[string]any{}
	if err := validateLegacyOccurrence(goodOccurrence, inv, entry, "rule"); err == nil {
		t.Fatal("missing execution provenance accepted")
	}

	if _, err := marshalLegacyManifest(func() {}); err == nil {
		t.Fatal("unencodable manifest accepted")
	}
	if _, err := marshalLegacyManifest(map[string]string{"unsafe": "person@example.test"}); err == nil {
		t.Fatal("unsafe manifest accepted")
	}
	if _, err := legacyManifestBytes(inv, mapping); err != nil {
		t.Fatal(err)
	}
	if err := syncDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory sync accepted")
	}
}

func TestValidateImportedLessonRequiresCompleteOwnerProof(t *testing.T) {
	for _, name := range []string{"nonregular marker", "unreadable marker", "marker mismatch", "missing occurrence store"} {
		t.Run(name, func(t *testing.T) {
			lessons, inv, _ := legacyMatrixFixture(t)
			entry := inv.Entries[0]
			path := filepath.Join(lessons, "rule", "README.md")
			if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
				t.Fatal(err)
			}
			body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "rule"}, []string{"process"})
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.Replace(body, []byte("**Legacy Provenance:** —"), []byte("**Legacy Provenance:** "+legacyProvenance(inv, entry)), 1)
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(filepath.Dir(path), ".legacy-import-owner")
			if err := os.WriteFile(markerPath, legacyImportOwnerMarker(body), 0o600); err != nil {
				t.Fatal(err)
			}
			var fs lessonFS = osLessonFS{}
			switch name {
			case "nonregular marker":
				if err := os.Remove(markerPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(markerPath, 0o700); err != nil {
					t.Fatal(err)
				}
			case "unreadable marker":
				fs = legacyOwnershipTestFS{lessonFS: osLessonFS{}, read: func(got string) ([]byte, error) {
					if got == markerPath {
						return nil, errors.New("read")
					}
					return os.ReadFile(got)
				}}
			case "marker mismatch":
				if err := os.WriteFile(markerPath, []byte("wrong\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing occurrence store":
				if err := os.Remove(filepath.Join(filepath.Dir(path), "occurrences")); err != nil {
					t.Fatal(err)
				}
			}
			if err := validateImportedLessonWithFS(path, entry, inv, fs); err == nil {
				t.Fatal("incomplete ownership proof accepted")
			}
		})
	}
}

func TestOccurrenceResidualFilesystemAndParsingFailures(t *testing.T) {
	if err := validateContext(map[string]any{"repository": 7}); err == nil {
		t.Fatal("non-object context accepted")
	}
	if err := validateContext(map[string]any{"git": map[string]any{"token": "safe"}}); err == nil {
		t.Fatal("nested forbidden context name accepted")
	}
	if _, err := addOccurrenceWithFS(AddOccurrenceOptions{LessonPath: filepath.Join(t.TempDir(), "missing")}, osLessonFS{}); err == nil {
		t.Fatal("missing Lesson accepted")
	}
	fs := &faultMatrixFS{lessonFS: osLessonFS{}, failAt: 1}
	if err := removeOccurrenceWithFS(filepath.Join(t.TempDir(), "missing.json"), fs); err == nil {
		t.Fatal("remove failure accepted")
	}
	occDir := filepath.Join(t.TempDir(), "occurrences")
	for _, failAt := range []int{1, 2, 3, 4, 6, 7} {
		fs := &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt}
		if err := ensureOccurrenceDirectoryWithFS(occDir+fmt.Sprint(failAt), fs); err == nil {
			t.Fatalf("directory operation %d failure accepted", failAt)
		}
	}
	if _, err := discoverOccurrencesWithFS(filepath.Join(t.TempDir(), "missing"), osLessonFS{}); err == nil {
		t.Fatal("invalid Lesson discovery accepted")
	}

	lessons := relationFixture(t, "rule")
	lessonPath := filepath.Join(lessons, "rule", "README.md")
	before := snapshotTree(t, lessons)
	badOptions := AddOccurrenceOptions{LessonPath: lessonPath, ID: "bad", Summary: "bad"}
	if _, err := addOccurrenceWithFS(badOptions, osLessonFS{}); err == nil {
		t.Fatal("invalid occurrence options accepted")
	}
	if !bytes.Equal(before, snapshotTree(t, lessons)) {
		t.Fatal("invalid occurrence mutated tree")
	}
	validOptions := AddOccurrenceOptions{LessonPath: lessonPath, ID: "01234567-89ab-4def-8123-456789abcdef", Summary: "safe"}
	if _, err := addOccurrenceWithRuntime(validOptions, osLessonFS{}, func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") }); err == nil {
		t.Fatal("occurrence marshal failure accepted")
	}
	fs = &faultMatrixFS{lessonFS: osLessonFS{}, failAt: 1}
	if _, err := discoverOccurrencesWithFS(lessonPath, fs); err == nil {
		t.Fatal("occurrence ReadDir failure accepted")
	}
	occurrences := filepath.Join(lessons, "rule", "occurrences")
	bad := filepath.Join(occurrences, "bad.json")
	if err := os.WriteFile(bad, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverOccurrencesWithFS(lessonPath, osLessonFS{}); err == nil {
		t.Fatal("malformed child occurrence accepted")
	}
	if _, err := validateOccurrenceFileWithFS(filepath.Join(occurrences, "missing.json"), osLessonFS{}); err == nil {
		t.Fatal("missing occurrence file accepted")
	}
	valid := coverageOccurrence()
	valid.ID = "01234567-89ab-4def-8123-456789abcdef"
	b, _ := json.Marshal(valid)
	wrongName := filepath.Join(occurrences, "wrong.json")
	if err := os.WriteFile(wrongName, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validateOccurrenceFileWithFS(wrongName, osLessonFS{}); err == nil {
		t.Fatal("filename/id mismatch accepted")
	}
	validPath := filepath.Join(occurrences, valid.ID+".json")
	badTyped := valid
	badTyped.Summary = ""
	badTypedBytes, _ := json.Marshal(badTyped)
	if err := os.WriteFile(validPath, badTypedBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validateOccurrenceFileWithFS(validPath, osLessonFS{}); err == nil {
		t.Fatal("typed occurrence validation failure accepted")
	}
	if err := os.WriteFile(validPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validateOccurrenceFileWithRuntime(validPath, osLessonFS{}, func(string) *json.Decoder {
		return json.NewDecoder(strings.NewReader(string(b) + ` {}`))
	}); err == nil {
		t.Fatal("extra typed occurrence JSON accepted")
	}
	if err := scanOccurrenceJSON(map[string]any{"person@example.test": "safe"}); err == nil {
		t.Fatal("unsafe property name accepted")
	}
	if err := scanOccurrenceJSON([]any{"person@example.test"}); err == nil {
		t.Fatal("unsafe array child accepted")
	}
	if _, err := findOccurrenceWithFS(filepath.Join(t.TempDir(), "missing"), valid.ID, osLessonFS{}); err == nil {
		t.Fatal("find swallowed discovery failure")
	}
}

func TestOccurrenceDiscoveryMissingStoreAndStableTieOrdering(t *testing.T) {
	lessons := relationFixture(t, "rule")
	lessonPath := filepath.Join(lessons, "rule", "README.md")
	occurrences := filepath.Join(lessons, "rule", "occurrences")
	if err := os.Remove(occurrences); err != nil {
		t.Fatal(err)
	}
	if got, err := discoverOccurrencesWithFS(lessonPath, osLessonFS{}); err != nil || got != nil {
		t.Fatalf("missing store=%#v err=%v", got, err)
	}
	if err := os.Mkdir(occurrences, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"11234567-89ab-4def-8123-456789abcdef", "01234567-89ab-4def-8123-456789abcdef"} {
		o := coverageOccurrence()
		o.ID = id
		b, _ := json.Marshal(o)
		if err := os.WriteFile(filepath.Join(occurrences, id+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := discoverOccurrencesWithFS(lessonPath, osLessonFS{})
	if err != nil || len(got) != 2 || got[0].ID != "01234567-89ab-4def-8123-456789abcdef" {
		t.Fatalf("tie order=%#v err=%v", got, err)
	}
}

func TestDiscoveryAndResolutionPropagateArtifactErrors(t *testing.T) {
	lessons := filepath.Join(t.TempDir(), "spec", "lessons")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(lessons, "bad.md")
	if err := os.WriteFile(bad, []byte("# Lesson: bad\n"+strings.Repeat("x", 1<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(lessons); err == nil {
		t.Fatal("discovery swallowed parse failure")
	}
	if err := os.Remove(bad); err != nil {
		t.Fatal(err)
	}
	loop := filepath.Join(lessons, "loop.md")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLessonFile(lessons, "loop"); err == nil {
		t.Fatal("resolution swallowed flat stat failure")
	}
	dirLoop := filepath.Join(lessons, "dir-loop")
	if err := os.Mkdir(dirLoop, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dirLoop, "README.md"), filepath.Join(dirLoop, "README.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(lessons); err == nil {
		t.Fatal("discovery swallowed canonical stat failure")
	}
}

func pendingFlatFixture(t *testing.T) (string, FlatMigrationOptions, flatMigrationDeps, []byte) {
	t.Helper()
	lessons, opts, deps := flatMatrixFixture(t, "rule")
	source, err := os.ReadFile(filepath.Join(lessons, "rule.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrateFlatWithDeps(opts, deps); err != nil {
		t.Fatal(err)
	}
	return lessons, opts, deps, source
}

func TestFlatMigrationResidualStateMachineAndProofFailures(t *testing.T) {
	if _, err := migrateFlatWithDeps(FlatMigrationOptions{}, defaultFlatMigrationDeps()); err == nil {
		t.Fatal("invalid migration accepted")
	}
	if err := finalizeFlatMigrationWithDeps(FlatMigrationOptions{}, "", defaultFlatMigrationDeps()); err == nil {
		t.Fatal("invalid finalization accepted")
	}

	for name, setup := range map[string]func(string, *FlatMigrationOptions){
		"malformed marker": func(root string, _ *FlatMigrationOptions) {
			_ = os.WriteFile(filepath.Join(root, ".flat-migration-rule.json"), []byte("{"), 0o644)
		},
		"classifications": func(root string, opts *FlatMigrationOptions) { opts.Classifications = []string{"other"} },
		"event":           func(_ string, opts *FlatMigrationOptions) { opts.EventUUID = "11234567-89ab-4def-8123-456789abcdef" },
		"expected bytes": func(root string, _ *FlatMigrationOptions) {
			_ = os.WriteFile(filepath.Join(root, "rule", "README.md"), []byte("changed"), 0o644)
		},
	} {
		t.Run("resume-"+name, func(t *testing.T) {
			lessons, opts, deps, _ := pendingFlatFixture(t)
			setup(lessons, &opts)
			if _, err := migrateFlatWithDeps(opts, deps); err == nil {
				t.Fatal("invalid resumable transaction accepted")
			}
		})
	}

	for name, identity := range map[string]func(string, []byte) (LegacySourceRef, error){
		"error":   func(string, []byte) (LegacySourceRef, error) { return LegacySourceRef{}, errors.New("identity") },
		"invalid": func(string, []byte) (LegacySourceRef, error) { return LegacySourceRef{}, nil },
		"bytes": func(_ string, b []byte) (LegacySourceRef, error) {
			r := coverageLegacySource()
			r.Path, r.SHA256, r.ByteCount = "spec/lessons/rule.md", shaString(b), len(b)+1
			return r, nil
		},
	} {
		t.Run("identity-"+name, func(t *testing.T) {
			_, opts, deps := flatMatrixFixture(t, "rule")
			deps.sourceIdentity = identity
			if _, err := migrateFlatWithDeps(opts, deps); err == nil {
				t.Fatal("invalid source identity accepted")
			}
		})
	}
	_, opts, deps := flatMatrixFixture(t, "rule")
	opts.EventUUID = "11234567-89ab-4def-8123-456789abcdef"
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
		t.Fatal("non-deterministic event accepted")
	}
	_, opts, deps = flatMatrixFixture(t, "rule")
	marshalCalls := 0
	deps.marshal = func(field string, value any) ([]byte, error) {
		marshalCalls++
		if marshalCalls == 2 {
			return nil, errors.New("marshal")
		}
		return marshalSafeJSON(field, value)
	}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
		t.Fatal("marker marshal failure accepted")
	}
	lessons, opts, deps := flatMatrixFixture(t, "rule")
	if err := os.WriteFile(filepath.Join(lessons, ".flat-migration-rule.json"), []byte("different"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
		t.Fatal("marker collision accepted")
	}
	lessons, opts, deps = flatMatrixFixture(t, "rule")
	markerPath := filepath.Join(lessons, ".flat-migration-rule.json")
	deps.fs = residualFS{lessonFS: osLessonFS{}, stat: func(path string) (os.FileInfo, error) {
		if path == markerPath {
			_ = os.WriteFile(path, []byte("appeared"), 0o644)
		}
		return os.Stat(path)
	}}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
		t.Fatal("racing marker accepted")
	}
	lessons, opts, deps, source := pendingFlatFixture(t)
	if err := os.WriteFile(filepath.Join(lessons, "rule.md"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "rule", "README.md"), []byte("collision"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
		t.Fatal("partial transaction collision accepted")
	}

	lessons, opts, deps = flatMatrixFixture(t, "rule")
	base := osLessonFS{}
	deps.fs = residualFS{lessonFS: base, link: func(oldname, newname string) error {
		if err := base.Link(oldname, newname); err != nil {
			return err
		}
		if strings.HasSuffix(newname, filepath.Join("rule", "README.md")) {
			return os.WriteFile(newname, []byte("corrupt"), 0o644)
		}
		return nil
	}}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
		t.Fatal("corrupt post-publication canonical accepted")
	}
	if _, err := os.Stat(filepath.Join(lessons, ".flat-migration-rule.json")); err != nil {
		t.Fatalf("post-publication validation failure lost recovery marker: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(lessons, "rule", "README.md")); err != nil || string(got) != "corrupt" {
		t.Fatalf("foreign/corrupt visible bytes were destructively replaced: %q, %v", got, err)
	}
}

func TestFlatMigrationDurableRecoveryResiduals(t *testing.T) {
	t.Run("resume requires private recovery source", func(t *testing.T) {
		lessons, opts, deps, source := pendingFlatFixture(t)
		eventUUID := FlatMigrationEventUUID(shaString(source), opts.Slug)
		if err := os.Remove(filepath.Join(flatMigrationRecoveryDir(lessons, eventUUID), "source.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := migrateFlatWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "source recovery") {
			t.Fatalf("missing recovery source err=%v", err)
		}
	})

	t.Run("fresh migration rejects occupied recovery target", func(t *testing.T) {
		lessons, opts, deps := flatMatrixFixture(t, "rule")
		body, err := os.ReadFile(filepath.Join(lessons, "rule.md"))
		if err != nil {
			t.Fatal(err)
		}
		recoveryDir := flatMigrationRecoveryDir(lessons, FlatMigrationEventUUID(shaString(body), opts.Slug))
		if err := os.MkdirAll(recoveryDir, 0o700); err != nil {
			t.Fatal(err)
		}
		foreign := []byte("foreign recovery bytes\n")
		target := filepath.Join(recoveryDir, "source.md")
		if err := os.WriteFile(target, foreign, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := migrateFlatWithDeps(opts, deps); err == nil || MutationOutcomeOf(err) != MutationUncertain {
			t.Fatalf("occupied recovery target err=%v outcome=%v", err, MutationOutcomeOf(err))
		}
		got, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(got, foreign) {
			t.Fatalf("foreign recovery bytes changed: %q err=%v", got, err)
		}
	})

	t.Run("source reappearing after atomic retirement is retained", func(t *testing.T) {
		lessons, opts, deps := flatMatrixFixture(t, "rule")
		flatPath := filepath.Join(lessons, "rule.md")
		baseRename := deps.renameNoReplace
		foreign := []byte("foreign replacement\n")
		deps.renameNoReplace = func(oldname, newname string) error {
			if err := baseRename(oldname, newname); err != nil {
				return err
			}
			return os.WriteFile(oldname, foreign, 0o644)
		}
		if _, err := migrateFlatWithDeps(opts, deps); err == nil || MutationOutcomeOf(err) != MutationUncertain {
			t.Fatalf("reappeared source err=%v outcome=%v", err, MutationOutcomeOf(err))
		}
		got, err := os.ReadFile(flatPath)
		if err != nil || !bytes.Equal(got, foreign) {
			t.Fatalf("foreign replacement changed: %q err=%v", got, err)
		}
	})

	for _, tc := range []struct {
		name  string
		setup func(string)
		deps  func(flatMigrationDeps) flatMigrationDeps
	}{
		{
			name: "completed marker collision",
			setup: func(path string) {
				if err := os.WriteFile(path, []byte("foreign completed marker\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			deps: func(deps flatMigrationDeps) flatMigrationDeps { return deps },
		},
		{
			name:  "atomic marker retirement failure",
			setup: func(string) {},
			deps: func(deps flatMigrationDeps) flatMigrationDeps {
				deps.renameNoReplace = func(string, string) error { return errors.New("rename") }
				return deps
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lessons, opts, deps, source := pendingFlatFixture(t)
			eventUUID := FlatMigrationEventUUID(shaString(source), opts.Slug)
			writeFlatMigrationIndex(t, lessons, filepath.Join(lessons, opts.Slug, "README.md"))
			completed := filepath.Join(flatMigrationRecoveryDir(lessons, eventUUID), "transaction.complete.json")
			tc.setup(completed)
			if err := finalizeFlatMigrationWithDeps(opts, eventUUID, tc.deps(deps)); err == nil {
				t.Fatal("unsafe finalization succeeded")
			}
		})
	}
}

func TestFlatMigrationResidualArtifactValidation(t *testing.T) {
	lessons, opts, deps, _ := pendingFlatFixture(t)
	marker := filepath.Join(lessons, ".flat-migration-rule.json")
	if err := os.WriteFile(marker, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalizeFlatMigrationWithDeps(opts, "11234567-89ab-4def-8123-456789abcdef", deps); err == nil {
		t.Fatal("malformed finalization marker accepted")
	}

	root := t.TempDir()
	badCanonical := filepath.Join(root, "bad.md")
	if err := os.WriteFile(badCanonical, []byte(strings.Repeat("x", 1<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateFlatMigrationIndexRowWithFS(root, badCanonical, osLessonFS{}); err == nil {
		t.Fatal("unparseable index target accepted")
	}
	lessons, opts, _, _ = pendingFlatFixture(t)
	canonical := filepath.Join(lessons, opts.Slug, "README.md")
	badOccurrence := filepath.Join(lessons, opts.Slug, "occurrences", "bad.json")
	if err := os.WriteFile(badOccurrence, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateFlatMigrationIndexRowWithFS(lessons, canonical, osLessonFS{}); err == nil {
		t.Fatal("invalid occurrence store accepted by index proof")
	}
	if err := validateCompletedFlatMigration(canonical); err == nil {
		t.Fatal("invalid occurrence store accepted by completed migration proof")
	}

	plain := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(plain, []byte("# Lesson: incomplete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCompletedFlatMigration(plain); err == nil {
		t.Fatal("incomplete canonical accepted")
	}

	for name, mutate := range map[string]func(string, string){
		"provenance": func(_ string, canonical string) {
			b, _ := os.ReadFile(canonical)
			start := bytes.Index(b, []byte("**Legacy Provenance:** "))
			end := start + bytes.IndexByte(b[start:], '\n')
			b = append(append(append([]byte(nil), b[:start]...), []byte("**Legacy Provenance:** malformed")...), b[end:]...)
			_ = os.WriteFile(canonical, b, 0o644)
		},
		"manifest json": func(lessons, _ string) {
			entries, _ := os.ReadDir(filepath.Join(lessons, ".legacy-import"))
			_ = os.WriteFile(filepath.Join(lessons, ".legacy-import", entries[0].Name()), []byte("{"), 0o644)
		},
		"manifest identity": func(lessons, _ string) {
			mutateFlatManifest(t, lessons, func(m *flatMigrationManifest) { m.Kind = "wrong" })
		},
		"source range": func(lessons, _ string) {
			mutateFlatManifest(t, lessons, func(m *flatMigrationManifest) { m.SourceRange.EndByte++ })
		},
		"canonical mismatch": func(lessons, _ string) {
			mutateFlatManifest(t, lessons, func(m *flatMigrationManifest) { m.SourceStatus = "Stated" })
		},
	} {
		t.Run(name, func(t *testing.T) {
			lessons, opts, _, _ := pendingFlatFixture(t)
			canonical := filepath.Join(lessons, opts.Slug, "README.md")
			writeFlatMigrationIndex(t, lessons, canonical)
			mutate(lessons, canonical)
			if err := validateCompletedFlatMigrationProofWithFS(lessons, canonical, opts.Slug, osLessonFS{}); err == nil {
				t.Fatal("corrupt completed proof accepted")
			}
		})
	}
}

func mutateFlatManifest(t *testing.T, lessons string, mutate func(*flatMigrationManifest)) {
	t.Helper()
	dir := filepath.Join(lessons, ".legacy-import")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, entries[0].Name())
	b, _ := os.ReadFile(path)
	var manifest flatMigrationManifest
	if err := decodeStrictJSON(b, &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	b, _ = marshalSafeJSON("manifest", manifest)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFlatMigrationResidualStageTransformations(t *testing.T) {
	for name, mutate := range map[string]func(*FlatMigrationOptions, *LegacySourceRef, *[]byte){
		"unsafe title owner": func(_ *FlatMigrationOptions, _ *LegacySourceRef, b *[]byte) {
			*b = bytes.Replace(*b, []byte("# Lesson: "), []byte("# Lesson: person@example.test "), 1)
			*b = bytes.Replace(*b, []byte("**Owner:** codex"), []byte("**Owner:** person@example.test"), 1)
		},
		"unsafe enforcement": func(opts *FlatMigrationOptions, _ *LegacySourceRef, b *[]byte) {
			*b = []byte(flatFixture("Enforced", 0, ""))
			opts.Control, opts.Verification, opts.Evidence = "person@example.test", "safe", "safe"
		},
		"unsafe canonical": func(_ *FlatMigrationOptions, source *LegacySourceRef, _ *[]byte) {
			source.Repository = "person@example.test"
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "spec", "lessons")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			opts := FlatMigrationOptions{LessonsDir: root, Slug: "rule", Classifications: []string{"process"}}
			body := []byte(flatFixture("Recorded", 0, ""))
			source := coverageLegacySource()
			source.Path = "spec/lessons/rule.md"
			mutate(&opts, &source, &body)
			source.SHA256, source.ByteCount = shaString(body), len(body)
			path := filepath.Join(root, "rule.md")
			_ = os.WriteFile(path, body, 0o644)
			stage, _, redacted, err := stageFlatMigrationWithDeps(opts, body, path, source, osLessonFS{})
			if name == "unsafe title owner" {
				if err != nil || len(redacted) < 2 {
					t.Fatalf("redactions=%v err=%v", redacted, err)
				}
				_ = os.RemoveAll(stage)
				return
			}
			if err == nil {
				_ = os.RemoveAll(stage)
				t.Fatal("unsafe transformation accepted")
			}
		})
	}

	root := filepath.Join(t.TempDir(), "spec", "lessons")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(flatFixture("Recorded", 0, ""))
	path := filepath.Join(root, "rule.md")
	_ = os.WriteFile(path, body, 0o644)
	source := coverageLegacySource()
	source.Path, source.SHA256, source.ByteCount = "spec/lessons/rule.md", shaString(body), len(body)
	if _, _, _, err := stageFlatMigrationWithRuntime(FlatMigrationOptions{LessonsDir: root, Slug: "rule", Classifications: []string{"process"}}, body, path, source, osLessonFS{}, func(string, any) ([]byte, error) { return nil, errors.New("manifest") }); err == nil {
		t.Fatal("manifest marshal failure accepted")
	}
	section := flatSection{start: 0, end: len("- one\n*This document follows the template")}
	if got := flatRecurrenceObservations([]byte("- one\n*This document follows the template"), section); len(got) != 1 || strings.Contains(got[0].text, "template") {
		t.Fatalf("footer not trimmed: %#v", got)
	}
	stage := t.TempDir()
	_ = os.WriteFile(filepath.Join(stage, "README.md"), []byte("readme"), 0o644)
	_ = os.WriteFile(filepath.Join(stage, "manifest.json"), []byte("manifest"), 0o644)
	_ = os.Mkdir(filepath.Join(stage, "occurrences"), 0o755)
	if _, err := collectFlatExpectedFilesWithRuntime(stage, root, "rule", filepath.Join(root, "manifest"), osLessonFS{}, func(string, string) (string, error) { return "", errors.New("rel") }); err == nil {
		t.Fatal("relative manifest path failure accepted")
	}
}

func TestFlatMigrationIndexRepresentsEmptyEnforcementAsDash(t *testing.T) {
	lessons := filepath.Join(t.TempDir(), "spec", "lessons")
	canonical := filepath.Join(lessons, "rule", "README.md")
	if err := os.MkdirAll(filepath.Join(lessons, "rule", "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "rule"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte("**Control:** —"), []byte("**Control:**"), 1)
	if err := os.WriteFile(canonical, body, 0o644); err != nil {
		t.Fatal(err)
	}
	row := "| [rule](rule/README.md) | Recorded | process | 0 |  | — |"
	if err := os.WriteFile(filepath.Join(lessons, "README.md"), []byte(row+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateFlatMigrationIndexRowWithFS(lessons, canonical, osLessonFS{}); err != nil {
		t.Fatal(err)
	}
}

func writeRelationFieldForResidual(t *testing.T, lessons, slug, field, value string) {
	t.Helper()
	path := filepath.Join(lessons, slug, "README.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err = rewriteRelationFields(b, path, map[string]string{field: value})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRelationResidualValidationAndPublicationBranches(t *testing.T) {
	deps := defaultRelationDeps()
	for name, args := range map[string][3]string{
		"invalid": {"Bad", "related", "b"},
		"from":    {"missing", "related", "b"},
		"to":      {"a", "related", "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			lessons := relationFixture(t, "a", "b")
			if err := addRelationLockedWithDeps(lessons, args[0], args[1], args[2], deps); err == nil {
				t.Fatal("invalid relation accepted")
			}
		})
	}
	for name, slug := range map[string]string{"from parse": "a", "to parse": "b"} {
		t.Run(name, func(t *testing.T) {
			lessons := relationFixture(t, "a", "b")
			if err := os.WriteFile(filepath.Join(lessons, slug, "README.md"), []byte(strings.Repeat("x", 1<<20)), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := addRelationLockedWithDeps(lessons, "a", "related", "b", deps); err == nil {
				t.Fatal("unparseable endpoint accepted")
			}
		})
	}
	lessons := relationFixture(t, "a", "b")
	_ = os.RemoveAll(filepath.Join(lessons, "a"))
	_ = os.WriteFile(filepath.Join(lessons, "a.md"), []byte("# Lesson: a\n"), 0o644)
	if err := addRelationLockedWithDeps(lessons, "a", "related", "b", deps); err == nil {
		t.Fatal("flat endpoint accepted")
	}

	lessons = relationFixture(t, "a", "b")
	writeRelationFieldForResidual(t, lessons, "a", "Supersedes", "b")
	if err := addRelationLockedWithDeps(lessons, "a", "duplicates", "b", deps); err == nil {
		t.Fatal("duplicate plus supersedes accepted")
	}
	lessons = relationFixture(t, "a", "b")
	writeRelationFieldForResidual(t, lessons, "b", "Supersedes", "a")
	if err := addRelationLockedWithDeps(lessons, "a", "duplicates", "b", deps); err == nil {
		t.Fatal("duplicate cycle accepted")
	}

	for name, typ := range map[string]string{"duplicate": "duplicates", "supersedes": "supersedes"} {
		t.Run(name, func(t *testing.T) {
			lessons := relationFixture(t, "a", "b")
			deps := defaultRelationDeps()
			deps.rewrite = func([]byte, string, map[string]string) ([]byte, error) { return nil, errors.New("rewrite") }
			before := snapshotTree(t, lessons)
			if err := addRelationLockedWithDeps(lessons, "a", typ, "b", deps); err == nil {
				t.Fatal("rewrite failure accepted")
			}
			if !bytes.Equal(before, snapshotTree(t, lessons)) {
				t.Fatal("rewrite failure changed complete tree")
			}
		})
	}
	lessons = relationFixture(t, "a", "b")
	deps = defaultRelationDeps()
	deps.beforePublish = func(string) error { return errors.New("hook") }
	if err := addRelationLockedWithDeps(lessons, "a", "supersedes", "b", deps); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("hook error=%v outcome=%v", err, MutationOutcomeOf(err))
	}
}

func TestRelationResidualRereadDefenses(t *testing.T) {
	for name, mutate := range map[string]func([]byte, string) []byte{
		"combined fields": func(b []byte, path string) []byte {
			out, _ := rewriteRelationFields(b, path, map[string]string{"Duplicate Of": "c"})
			return out
		},
		"enforced status": func(b []byte, path string) []byte {
			out, _ := rewriteRelationFields(b, path, map[string]string{"Status": "Enforced"})
			return out
		},
	} {
		t.Run("duplicate-"+name, func(t *testing.T) {
			lessons := relationFixture(t, "a", "b", "c")
			path := filepath.Join(lessons, "a", "README.md")
			reads := 0
			deps := defaultRelationDeps()
			deps.fs = residualFS{lessonFS: osLessonFS{}, readFile: func(got string) ([]byte, error) {
				b, err := os.ReadFile(got)
				if got == path {
					reads++
					if reads == 3 {
						b = mutate(b, got)
					}
				}
				return b, err
			}}
			if err := addRelationLockedWithDeps(lessons, "a", "duplicates", "b", deps); err == nil {
				t.Fatal("reread conflict accepted")
			}
		})
	}
	for _, conflictAt := range []string{"a", "b"} {
		t.Run("supersedes-reread-"+conflictAt, func(t *testing.T) {
			lessons := relationFixture(t, "a", "b", "c")
			counts := map[string]int{}
			deps := defaultRelationDeps()
			deps.fs = residualFS{lessonFS: osLessonFS{}, readFile: func(path string) ([]byte, error) {
				b, err := os.ReadFile(path)
				slug := filepath.Base(filepath.Dir(path))
				counts[slug]++
				want := 2
				if slug == "b" {
					want = 3
				}
				if slug == conflictAt && counts[slug] == want {
					field := "Supersedes"
					if slug == "b" {
						field = "Superseded By"
					}
					b, _ = rewriteRelationFields(b, path, map[string]string{field: "c"})
				}
				return b, err
			}}
			if err := addRelationLockedWithDeps(lessons, "a", "supersedes", "b", deps); err == nil {
				t.Fatal("reread conflict accepted")
			}
		})
	}
	lessons := relationFixture(t, "a", "b")
	deps := defaultRelationDeps()
	rewrites := 0
	deps.rewrite = func(b []byte, path string, values map[string]string) ([]byte, error) {
		rewrites++
		if rewrites == 2 {
			return nil, errors.New("second rewrite")
		}
		return rewriteRelationFields(b, path, values)
	}
	if err := addRelationLockedWithDeps(lessons, "a", "supersedes", "b", deps); err == nil {
		t.Fatal("second rewrite failure accepted")
	}
}

func TestRelationResidualReadsCyclesSidecarAndStableOrdering(t *testing.T) {
	lessons := relationFixture(t, "a", "b", "c")
	deps := defaultRelationDeps()
	deps.fs = residualFS{lessonFS: osLessonFS{}, readFile: func(path string) ([]byte, error) {
		if filepath.Base(path) == relatedRelationsFile {
			return nil, errors.New("read")
		}
		return os.ReadFile(path)
	}}
	if _, err := listRelationsWithDeps(lessons, "a", deps); err == nil {
		t.Fatal("sidecar read failure accepted")
	}
	deps = defaultRelationDeps()
	deps.fs = residualFS{lessonFS: osLessonFS{}, readFile: func(path string) ([]byte, error) {
		if strings.HasSuffix(path, filepath.Join("b", "README.md")) {
			return nil, errors.New("metadata")
		}
		return os.ReadFile(path)
	}}
	if _, err := listRelationsWithDeps(lessons, "a", deps); err == nil {
		t.Fatal("metadata read failure accepted")
	}
	loopDir := filepath.Join(lessons, "loop")
	if err := os.Mkdir(loopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(loopDir, "README.md"), filepath.Join(loopDir, "README.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := listRelationsWithDeps(lessons, "a", defaultRelationDeps()); err == nil {
		t.Fatal("Lesson discovery failure accepted")
	}
	_ = os.RemoveAll(loopDir)

	writeRelationFieldForResidual(t, lessons, "a", "Supersedes", "c")
	writeRelationFieldForResidual(t, lessons, "b", "Duplicate Of", "a")
	if err := appendRelatedRelationWithDeps(lessons, Relation{From: "c", Type: "related", To: "a"}, defaultRelationDeps()); err != nil {
		t.Fatal(err)
	}
	got, err := listRelationsWithDeps(lessons, "a", defaultRelationDeps())
	if err != nil || len(got) < 3 {
		t.Fatalf("relations=%#v err=%v", got, err)
	}
	if err := os.WriteFile(filepath.Join(lessons, relatedRelationsFile), []byte(`[
  {"from":"c","type":"related","to":"a"},
  {"from":"b","type":"related","to":"a"},
  {"from":"a","type":"related","to":"c"},
  {"from":"a","type":"related","to":"b"}
]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := listRelationsWithDeps(lessons, "a", defaultRelationDeps()); err != nil {
		t.Fatal(err)
	}

	lessons = relationFixture(t, "a", "b", "c")
	writeRelationFieldForResidual(t, lessons, "b", "Supersedes", "c")
	writeRelationFieldForResidual(t, lessons, "b", "Duplicate Of", "c")
	if cycle, err := wouldCycleWithFS(lessons, "a", "b", osLessonFS{}); err != nil || cycle {
		t.Fatalf("shared-node graph cycle=%v err=%v", cycle, err)
	}
	writeRelationFieldForResidual(t, lessons, "c", "Supersedes", "missing")
	if _, err := wouldCycleWithFS(lessons, "a", "b", osLessonFS{}); err == nil {
		t.Fatal("recursive endpoint failure accepted")
	}

	if _, err := readRelatedRelationsWithFS(lessons, residualFS{lessonFS: osLessonFS{}, readFile: func(string) ([]byte, error) { return nil, errors.New("read") }}); err == nil {
		t.Fatal("arbitrary sidecar read failure accepted")
	}
	if _, err := decodeRelatedRelations(lessons, []byte(`[{"from":"Bad","type":"related","to":"a"}]`)); err == nil {
		t.Fatal("invalid sidecar endpoint accepted")
	}

	lessons = relationFixture(t, "a", "b", "c")
	deps = defaultRelationDeps()
	for _, relation := range []Relation{{From: "b", Type: "related", To: "c"}, {From: "a", Type: "related", To: "c"}, {From: "a", Type: "related", To: "b"}, {From: "b", Type: "related", To: "a"}} {
		if err := appendRelatedRelationWithDeps(lessons, relation, deps); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(lessons, relatedRelationsFile))
	if err != nil {
		t.Fatal(err)
	}
	var edges []Relation
	if err := json.Unmarshal(b, &edges); err != nil || len(edges) != 3 || edges[0].From != "a" || edges[0].To != "b" || edges[1].To != "c" {
		t.Fatalf("stable sidecar=%#v err=%v", edges, err)
	}
	deps.marshal = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") }
	if err := appendRelatedRelationWithDeps(relationFixture(t, "a", "b"), Relation{From: "a", Type: "related", To: "b"}, deps); err == nil {
		t.Fatal("sidecar marshal failure accepted")
	}
}

func makeLegacyCanonical(t *testing.T, lessons, slug string) string {
	t.Helper()
	path := filepath.Join(lessons, slug, "README.md")
	if err := os.MkdirAll(filepath.Join(lessons, slug, "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: slug}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLegacyResidualPreflightArtifactCollisions(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, string, *LegacyInventory, *LegacyMapping){
		"occurrence fields": func(t *testing.T, lessons string, _ *LegacyInventory, m *LegacyMapping) {
			makeLegacyCanonical(t, lessons, "target")
			m.Entries[0] = LegacyMappingEntry{Key: m.Entries[0].Key, Action: "occurrence", Slug: "target", Title: "ignored"}
		},
		"occurrence slug": func(_ *testing.T, _ string, _ *LegacyInventory, m *LegacyMapping) {
			m.Entries[0] = LegacyMappingEntry{Key: m.Entries[0].Key, Action: "occurrence", Slug: "Bad"}
		},
		"occurrence missing": func(_ *testing.T, _ string, _ *LegacyInventory, m *LegacyMapping) {
			m.Entries[0] = LegacyMappingEntry{Key: m.Entries[0].Key, Action: "occurrence", Slug: "missing"}
		},
		"occurrence flat": func(t *testing.T, lessons string, _ *LegacyInventory, m *LegacyMapping) {
			body := []byte(flatFixture("Recorded", 0, ""))
			if err := os.WriteFile(filepath.Join(lessons, "target.md"), body, 0o644); err != nil {
				t.Fatal(err)
			}
			m.Entries[0] = LegacyMappingEntry{Key: m.Entries[0].Key, Action: "occurrence", Slug: "target"}
		},
		"manifest collision": func(t *testing.T, lessons string, inv *LegacyInventory, _ *LegacyMapping) {
			path := filepath.Join(lessons, ".legacy-import", inv.Source.SHA256+".json")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("different"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"manifest parent": func(t *testing.T, lessons string, _ *LegacyInventory, _ *LegacyMapping) {
			if err := os.WriteFile(filepath.Join(lessons, ".legacy-import"), []byte("file"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			lessons, inv, mapping := legacyMatrixFixture(t)
			mutate(t, lessons, &inv, &mapping)
			before := snapshotTree(t, lessons)
			if _, err := preflightLegacyApplyWithFS(lessons, []string{"process"}, inv, mapping, osLessonFS{}); err == nil {
				t.Fatal("invalid artifact state accepted")
			}
			if !bytes.Equal(before, snapshotTree(t, lessons)) {
				t.Fatal("preflight changed complete tree")
			}
		})
	}
	lessons, inv, mapping := legacyMatrixFixture(t)
	if _, err := preflightLegacyApplyWithRuntime(lessons, []string{"process"}, inv, mapping, osLessonFS{}, func(LegacyInventory, LegacyMapping) ([]byte, error) { return nil, errors.New("manifest") }); err == nil {
		t.Fatal("manifest marshal failure accepted")
	}
	manifestParent := filepath.Join(lessons, ".legacy-import")
	if err := os.WriteFile(manifestParent, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := residualFS{lessonFS: osLessonFS{}, readFile: func(path string) ([]byte, error) {
		if strings.HasPrefix(path, manifestParent+string(filepath.Separator)) {
			return nil, os.ErrNotExist
		}
		return os.ReadFile(path)
	}}
	if _, err := preflightLegacyApplyWithFS(lessons, []string{"process"}, inv, mapping, fs); err == nil {
		t.Fatal("non-directory manifest parent accepted after absent manifest")
	}
	if err := os.Remove(manifestParent); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyLegacy(lessons, []string{"process"}, inv, mapping); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "reviewed-rule", "README.md"), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightLegacyApplyWithFS(lessons, []string{"process"}, inv, mapping, osLessonFS{}); err == nil {
		t.Fatal("invalid existing imported target accepted")
	}
}

func TestLegacyResidualApplyRacesAndRollbackFailures(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, string, LegacyInventory, *LegacyMapping, *legacyImportDeps){
		"provider collision": func(t *testing.T, lessons string, inv LegacyInventory, _ *LegacyMapping, deps *legacyImportDeps) {
			if _, err := ApplyLegacy(lessons, []string{"process"}, inv, legacyMapping(inv, reviewedNew("L1#1", "reviewed-rule"))); err != nil {
				t.Fatal(err)
			}
			deps.findOccurrence = func(string, string, lessonFS) (Occurrence, error) { return Occurrence{}, nil }
		},
		"provider discovery": func(t *testing.T, lessons string, inv LegacyInventory, _ *LegacyMapping, deps *legacyImportDeps) {
			if _, err := ApplyLegacy(lessons, []string{"process"}, inv, legacyMapping(inv, reviewedNew("L1#1", "reviewed-rule"))); err != nil {
				t.Fatal(err)
			}
			deps.findOccurrence = func(string, string, lessonFS) (Occurrence, error) { return Occurrence{}, errors.New("find") }
		},
		"occurrence resolve": func(t *testing.T, lessons string, _ LegacyInventory, mapping *LegacyMapping, deps *legacyImportDeps) {
			makeLegacyCanonical(t, lessons, "target")
			mapping.Entries[0] = LegacyMappingEntry{Key: mapping.Entries[0].Key, Action: "occurrence", Slug: "target"}
			deps.afterPreflight = func() { _ = os.RemoveAll(filepath.Join(lessons, "target")) }
		},
		"occurrence collision": func(t *testing.T, lessons string, inv LegacyInventory, mapping *LegacyMapping, _ *legacyImportDeps) {
			path := makeLegacyCanonical(t, lessons, "target")
			mapping.Entries[0] = LegacyMappingEntry{Key: mapping.Entries[0].Key, Action: "occurrence", Slug: "target"}
			id := legacyOccurrenceID(inv.Source.SHA256, inv.Entries[0].Key, "target")
			if _, err := addOccurrenceWithFS(legacyOccurrenceOptions(path, id, inv, inv.Entries[0]), osLessonFS{}); err != nil {
				t.Fatal(err)
			}
			occPath := filepath.Join(lessons, "target", "occurrences", id+".json")
			b, _ := os.ReadFile(occPath)
			b = bytes.Replace(b, []byte("Imported legacy"), []byte("Wrong legacy"), 1)
			_ = os.WriteFile(occPath, b, 0o644)
		},
		"occurrence discovery": func(t *testing.T, lessons string, _ LegacyInventory, mapping *LegacyMapping, deps *legacyImportDeps) {
			makeLegacyCanonical(t, lessons, "target")
			mapping.Entries[0] = LegacyMappingEntry{Key: mapping.Entries[0].Key, Action: "occurrence", Slug: "target"}
			deps.findOccurrence = func(string, string, lessonFS) (Occurrence, error) { return Occurrence{}, errors.New("find") }
		},
	} {
		t.Run(name, func(t *testing.T) {
			lessons, inv, mapping := legacyMatrixFixture(t)
			deps := defaultLegacyImportDeps()
			setup(t, lessons, inv, &mapping, &deps)
			if _, err := applyLegacyWithDeps(lessons, []string{"process"}, inv, mapping, deps); err == nil {
				t.Fatal("apply race/collision accepted")
			}
		})
	}
	for _, tc := range []struct {
		mode string
		want MutationOutcome
	}{{"remove-all", MutationUncertain}, {"remove", MutationUncertain}, {"sync", MutationUncertain}} {
		t.Run("rollback-"+tc.mode, func(t *testing.T) {
			lessons, inv, mapping := legacyMatrixFixture(t)
			deps := defaultLegacyImportDeps()
			deps.fs = &legacyRollbackFS{lessonFS: osLessonFS{}, mode: tc.mode}
			_, err := applyLegacyWithDeps(lessons, []string{"process"}, inv, mapping, deps)
			if err == nil || MutationOutcomeOf(err) != tc.want {
				t.Fatalf("rollback err=%v outcome=%v", err, MutationOutcomeOf(err))
			}
		})
	}
	t.Run("rollback-prepublication-child", func(t *testing.T) {
		lessons, inv, mapping := legacyMatrixFixture(t)
		deps := defaultLegacyImportDeps()
		deps.fs = &legacyRollbackFS{lessonFS: osLessonFS{}, mode: "occurrence-remove-all"}
		_, err := applyLegacyWithDeps(lessons, []string{"process"}, inv, mapping, deps)
		if err == nil || MutationOutcomeOf(err) != MutationUncertain {
			t.Fatalf("rollback err=%v outcome=%v", err, MutationOutcomeOf(err))
		}
	})
}

func TestWriteImportedLessonResidualGenerationFailures(t *testing.T) {
	lessons, inv, mapping := legacyMatrixFixture(t)
	e, m := inv.Entries[0], mapping.Entries[0]
	target := filepath.Join(lessons, "rule", "README.md")
	if err := writeImportedLessonWithFS(target, "rule", "title", m, e, inv, &faultMatrixFS{lessonFS: osLessonFS{}, failAt: 1}); err == nil {
		t.Fatal("directory creation failure accepted")
	}
	if err := writeImportedLessonWithFS(target, "Bad", "title", m, e, inv, osLessonFS{}); err == nil {
		t.Fatal("scaffold failure accepted")
	}
	m.Lesson = "person@example.test"
	if err := writeImportedLessonWithFS(target, "rule", "title", m, e, inv, osLessonFS{}); err == nil {
		t.Fatal("unsafe generated Lesson accepted")
	}
}
