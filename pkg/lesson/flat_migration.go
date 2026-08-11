package lesson

// Deterministic migration of one compatibility-era flat Lesson into the
// canonical directory layout. The old bytes remain available through an
// immutable Git reference; committed migration artifacts never copy them.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type FlatMigrationOptions struct {
	LessonsDir      string
	Slug            string
	Classifications []string
	Control         string
	Verification    string
	Evidence        string
	EventUUID       string
}

type FlatMigrationResult struct {
	CanonicalPath      string   `json:"canonical_path"`
	ManifestPath       string   `json:"manifest_path"`
	CreatedOccurrences []string `json:"created_occurrences"`
	RedactedSections   []string `json:"redacted_sections,omitempty"`
	AlreadyMigrated    bool     `json:"already_migrated"`
	PendingFinalize    bool     `json:"pending_finalize"`
}

type FlatMigrationPreflight struct {
	Source             LegacySourceRef
	EventUUID          string
	Classifications    []string
	AlreadyMigrated    bool
	PendingTransaction bool
}

type flatObservation struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	ID          string `json:"occurrence_id"`
	StartByte   int    `json:"start_byte"`
	EndByte     int    `json:"end_byte"`
	BytesSHA256 string `json:"bytes_sha256"`
}

type flatMigrationManifest struct {
	SchemaVersion int                `json:"schema_version"`
	Kind          string             `json:"kind"`
	Source        LegacySourceRef    `json:"source"`
	Slug          string             `json:"slug"`
	SourceStatus  string             `json:"source_status"`
	SourceRange   persistedByteRange `json:"source_range"`
	Observations  []flatObservation  `json:"observations"`
	Redactions    []string           `json:"redactions,omitempty"`
	Enforcement   *flatEnforcement   `json:"reviewed_enforcement,omitempty"`
}

type flatEnforcement struct {
	Control      string `json:"control"`
	Verification string `json:"verification"`
	Evidence     string `json:"evidence"`
}

type persistedByteRange struct {
	StartByte   int    `json:"start_byte"`
	EndByte     int    `json:"end_byte"`
	BytesSHA256 string `json:"bytes_sha256"`
}

type flatMigrationMarker struct {
	SchemaVersion   int                `json:"schema_version"`
	Source          LegacySourceRef    `json:"source"`
	Slug            string             `json:"slug"`
	Classifications []string           `json:"classifications"`
	EventUUID       string             `json:"event_uuid"`
	Files           []flatExpectedFile `json:"files"`
}

type flatExpectedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type flatSection struct {
	start int
	end   int
	text  string
}

// flatMigrationDeps is private and per operation. It replaces the old
// process-global test hooks without changing the exported lifecycle surface.
type flatMigrationDeps struct {
	fs             lessonFS
	sourceIdentity func(string, []byte) (LegacySourceRef, error)
}

func defaultFlatMigrationDeps() flatMigrationDeps {
	return flatMigrationDeps{fs: osLessonFS{}, sourceIdentity: resolveLegacySourceIdentity}
}

var flatStatus = map[string]bool{
	"Recorded": true, "Stated": true, "Enforced": true,
	"Withdrawn": true, "Superseded": true,
}

var flatRecurrenceDate = regexp.MustCompile(`\b[0-9]{4}-[0-9]{2}-[0-9]{2}\b`)

func validateFlatMigrationOptions(opts FlatMigrationOptions) error {
	if err := ValidateSlug(opts.Slug); err != nil {
		return err
	}
	if len(opts.Classifications) == 0 {
		return fmt.Errorf("at least one reviewed classification is required")
	}
	seenClassifications := map[string]bool{}
	for _, classification := range opts.Classifications {
		if err := ValidateSlug(classification); err != nil || seenClassifications[classification] {
			return fmt.Errorf("classifications must be unique valid slugs")
		}
		seenClassifications[classification] = true
	}
	return nil
}

// PreflightFlatMigration validates source identity and the complete generated
// transaction without publishing committed artifacts. A retry reads the
// durable marker so the CLI reuses the original event UUID and timestamp.
func PreflightFlatMigration(opts FlatMigrationOptions) (FlatMigrationPreflight, error) {
	return preflightFlatMigrationWithDeps(opts, defaultFlatMigrationDeps())
}

func preflightFlatMigrationWithDeps(opts FlatMigrationOptions, deps flatMigrationDeps) (FlatMigrationPreflight, error) {
	if err := validateFlatMigrationOptions(opts); err != nil {
		return FlatMigrationPreflight{}, err
	}
	flatPath := filepath.Join(opts.LessonsDir, opts.Slug+".md")
	canonicalPath := filepath.Join(opts.LessonsDir, opts.Slug, "README.md")
	markerPath := filepath.Join(opts.LessonsDir, ".flat-migration-"+opts.Slug+".json")
	if markerBytes, err := os.ReadFile(markerPath); err == nil {
		var marker flatMigrationMarker
		if err := decodeStrictJSON(markerBytes, &marker); err != nil {
			return FlatMigrationPreflight{}, fmt.Errorf("migration marker is malformed: %w", err)
		}
		if marker.SchemaVersion != 1 || marker.Slug != opts.Slug || validateLegacySourceRef(marker.Source) != nil || !occurrenceUUID.MatchString(marker.EventUUID) {
			return FlatMigrationPreflight{}, fmt.Errorf("migration marker does not describe a valid transaction")
		}
		if strings.Join(marker.Classifications, "\x00") != strings.Join(opts.Classifications, "\x00") {
			return FlatMigrationPreflight{}, fmt.Errorf("migration retry classifications differ from the durable transaction")
		}
		return FlatMigrationPreflight{Source: marker.Source, EventUUID: marker.EventUUID, Classifications: append([]string(nil), marker.Classifications...), PendingTransaction: true}, nil
	} else if !os.IsNotExist(err) {
		return FlatMigrationPreflight{}, err
	}
	flatBytes, err := os.ReadFile(flatPath)
	if os.IsNotExist(err) {
		if err := validateCompletedFlatMigrationProof(opts.LessonsDir, canonicalPath, opts.Slug); err != nil {
			return FlatMigrationPreflight{}, fmt.Errorf("legacy flat Lesson is absent and canonical migration is not verifiable: %w", err)
		}
		return FlatMigrationPreflight{AlreadyMigrated: true}, nil
	}
	if err != nil {
		return FlatMigrationPreflight{}, err
	}
	source, err := deps.sourceIdentity(flatPath, flatBytes)
	if err != nil {
		return FlatMigrationPreflight{}, fmt.Errorf("flat migration requires bytes committed at the current immutable Git revision: %w", err)
	}
	if err := validateLegacySourceRef(source); err != nil {
		return FlatMigrationPreflight{}, err
	}
	stage, _, _, err := stageFlatMigrationWithDeps(opts, flatBytes, flatPath, source, deps.fs)
	if err != nil {
		return FlatMigrationPreflight{}, err
	}
	defer func() { _ = deps.fs.RemoveAll(stage) }()
	manifestPath := flatManifestPath(opts.LessonsDir, source, opts.Slug)
	expected, err := collectFlatExpectedFiles(stage, opts.LessonsDir, opts.Slug, manifestPath)
	if err != nil {
		return FlatMigrationPreflight{}, err
	}
	if err := ensureFreshMigrationTargetsAbsent(opts.LessonsDir, expected); err != nil {
		return FlatMigrationPreflight{}, err
	}
	return FlatMigrationPreflight{Source: source, EventUUID: FlatMigrationEventUUID(source.SHA256, opts.Slug), Classifications: append([]string(nil), opts.Classifications...)}, nil
}

func FlatMigrationEventUUID(sourceSHA, slug string) string {
	return legacyOccurrenceID(sourceSHA, "flat#migration-event", slug)
}

// MigrateFlat migrates exactly one spec/lessons/<slug>.md artifact. A
// repository-scoped marker makes an interrupted publication inspectable and
// resumable. All writes are exclusive and every existing path must be either
// absent or byte-identical before any missing path is published.
func MigrateFlat(opts FlatMigrationOptions) (FlatMigrationResult, error) {
	return migrateFlatWithDeps(opts, defaultFlatMigrationDeps())
}

func migrateFlatWithDeps(opts FlatMigrationOptions, deps flatMigrationDeps) (FlatMigrationResult, error) {
	if err := validateFlatMigrationOptions(opts); err != nil {
		return FlatMigrationResult{}, err
	}

	flatPath := filepath.Join(opts.LessonsDir, opts.Slug+".md")
	canonicalDir := filepath.Join(opts.LessonsDir, opts.Slug)
	canonicalPath := filepath.Join(canonicalDir, "README.md")
	markerPath := filepath.Join(opts.LessonsDir, ".flat-migration-"+opts.Slug+".json")
	result := FlatMigrationResult{CanonicalPath: canonicalPath}

	flatBytes, flatErr := os.ReadFile(flatPath)
	if flatErr != nil && !os.IsNotExist(flatErr) {
		return result, flatErr
	}
	markerBytes, markerErr := os.ReadFile(markerPath)
	if markerErr != nil && !os.IsNotExist(markerErr) {
		return result, markerErr
	}
	if os.IsNotExist(flatErr) {
		if os.IsNotExist(markerErr) {
			if err := validateCompletedFlatMigrationProof(opts.LessonsDir, canonicalPath, opts.Slug); err != nil {
				return result, fmt.Errorf("legacy flat Lesson is absent and canonical migration is not verifiable: %w", err)
			}
			result.AlreadyMigrated = true
			return result, nil
		}
		var marker flatMigrationMarker
		if err := decodeStrictJSON(markerBytes, &marker); err != nil {
			return result, fmt.Errorf("migration marker is malformed: %w", err)
		}
		if marker.SchemaVersion != 1 || marker.Slug != opts.Slug || validateLegacySourceRef(marker.Source) != nil || !occurrenceUUID.MatchString(marker.EventUUID) {
			return result, fmt.Errorf("migration marker does not describe this Lesson")
		}
		if strings.Join(marker.Classifications, "\x00") != strings.Join(opts.Classifications, "\x00") {
			return result, fmt.Errorf("migration retry classifications differ from the durable transaction")
		}
		if opts.EventUUID != "" && opts.EventUUID != marker.EventUUID {
			return result, fmt.Errorf("migration retry event UUID differs from the durable transaction")
		}
		if err := verifyExpectedFiles(opts.LessonsDir, marker.Files, false); err != nil {
			return result, fmt.Errorf("interrupted migration cannot be finalized: %w", err)
		}
		if err := validateCompletedFlatMigration(canonicalPath); err != nil {
			return result, err
		}
		result.ManifestPath = flatManifestPath(opts.LessonsDir, marker.Source, opts.Slug)
		for _, expected := range marker.Files {
			if strings.HasPrefix(expected.Path, opts.Slug+"/occurrences/") {
				result.CreatedOccurrences = append(result.CreatedOccurrences, strings.TrimSuffix(filepath.Base(expected.Path), ".json"))
			}
		}
		result.PendingFinalize = true
		return result, nil
	}

	if info, err := os.Stat(canonicalDir); err == nil && !info.IsDir() {
		return result, fmt.Errorf("canonical target is not a directory")
	} else if err == nil && os.IsNotExist(markerErr) {
		return result, fmt.Errorf("both legacy flat and canonical directory forms exist for %q", opts.Slug)
	} else if err != nil && !os.IsNotExist(err) {
		return result, err
	}

	source, err := deps.sourceIdentity(flatPath, flatBytes)
	if err != nil {
		return result, fmt.Errorf("flat migration requires bytes committed at the current immutable Git revision: %w", err)
	}
	if err := validateLegacySourceRef(source); err != nil {
		return result, err
	}
	if source.SHA256 != shaString(flatBytes) || source.ByteCount != len(flatBytes) {
		return result, fmt.Errorf("resolved source identity does not match flat Lesson bytes")
	}
	if opts.EventUUID == "" {
		opts.EventUUID = FlatMigrationEventUUID(source.SHA256, opts.Slug)
	}
	if opts.EventUUID != FlatMigrationEventUUID(source.SHA256, opts.Slug) {
		return result, fmt.Errorf("flat migration event UUID does not match the deterministic source transaction")
	}

	stageDir, manifest, redacted, err := stageFlatMigrationWithDeps(opts, flatBytes, flatPath, source, deps.fs)
	if err != nil {
		return result, err
	}
	defer func() { _ = deps.fs.RemoveAll(stageDir) }()
	result.RedactedSections = redacted
	manifestPath := flatManifestPath(opts.LessonsDir, source, opts.Slug)
	result.ManifestPath = manifestPath

	expected, err := collectFlatExpectedFiles(stageDir, opts.LessonsDir, opts.Slug, manifestPath)
	if err != nil {
		return result, err
	}
	marker := flatMigrationMarker{SchemaVersion: 1, Source: source, Slug: opts.Slug, Classifications: append([]string(nil), opts.Classifications...), EventUUID: opts.EventUUID, Files: expected}
	wantMarker, err := marshalSafeJSON("flat migration marker", marker)
	if err != nil {
		return result, err
	}
	if !os.IsNotExist(markerErr) {
		if !bytes.Equal(markerBytes, wantMarker) {
			return result, fmt.Errorf("migration marker collision: existing transaction differs")
		}
	} else if _, statErr := os.Stat(markerPath); statErr == nil {
		return result, fmt.Errorf("migration marker appeared during preflight")
	} else if !os.IsNotExist(statErr) {
		return result, statErr
	}

	// Preflight every final file, including the manifest, before publishing a
	// missing one. A fresh migration permits no target at all; only a durable
	// marker authorizes byte-identical partial files from an interrupted write.
	if os.IsNotExist(markerErr) {
		if err := ensureFreshMigrationTargetsAbsent(opts.LessonsDir, expected); err != nil {
			return result, err
		}
	} else if err := verifyExpectedFiles(opts.LessonsDir, expected, true); err != nil {
		return result, err
	}

	createdMarker := false
	createdCanonical := false
	createdManifest := false
	rollback := func() {
		if createdCanonical {
			_ = deps.fs.RemoveAll(canonicalDir)
		}
		if createdManifest {
			_ = os.Remove(manifestPath)
		}
		if createdMarker {
			_ = os.Remove(markerPath)
		}
		_ = syncDirectoryWithFS(opts.LessonsDir, deps.fs)
	}
	if os.IsNotExist(markerErr) {
		markerStage := filepath.Join(stageDir, "transaction.json")
		if err := writeDurableStageFile(markerStage, wantMarker); err != nil {
			return result, err
		}
		if err := deps.fs.Link(markerStage, markerPath); err != nil {
			return result, err
		}
		createdMarker = true
		if err := syncDirectoryWithFS(opts.LessonsDir, deps.fs); err != nil {
			rollback()
			return result, err
		}
	}

	if _, err := os.Stat(canonicalDir); os.IsNotExist(err) {
		if err := os.Mkdir(canonicalDir, 0o755); err != nil {
			rollback()
			return result, err
		}
		createdCanonical = true
		if err := os.Mkdir(filepath.Join(canonicalDir, "occurrences"), 0o755); err != nil {
			rollback()
			return result, err
		}
	} else if err != nil {
		rollback()
		return result, err
	}

	for _, expectedFile := range expected {
		if !strings.HasPrefix(expectedFile.Path, opts.Slug+"/") {
			continue
		}
		final := filepath.Join(opts.LessonsDir, filepath.FromSlash(expectedFile.Path))
		if _, err := os.Stat(final); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			rollback()
			return result, err
		}
		stage := filepath.Join(stageDir, filepath.FromSlash(strings.TrimPrefix(expectedFile.Path, opts.Slug+"/")))
		if err := deps.fs.Link(stage, final); err != nil {
			rollback()
			return result, err
		}
	}
	if err := syncDirectoryWithFS(filepath.Join(canonicalDir, "occurrences"), deps.fs); err != nil {
		rollback()
		return result, err
	}
	if err := syncDirectoryWithFS(canonicalDir, deps.fs); err != nil {
		rollback()
		return result, err
	}

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			rollback()
			return result, err
		}
		if err := deps.fs.Link(filepath.Join(stageDir, "manifest.json"), manifestPath); err != nil {
			rollback()
			return result, err
		}
		createdManifest = true
		if err := syncDirectoryWithFS(filepath.Dir(manifestPath), deps.fs); err != nil {
			rollback()
			return result, err
		}
	} else if err != nil {
		rollback()
		return result, err
	}

	if err := validateCompletedFlatMigration(canonicalPath); err != nil {
		rollback()
		return result, err
	}
	current, err := os.ReadFile(flatPath)
	if err != nil || !bytes.Equal(current, flatBytes) {
		rollback()
		return result, fmt.Errorf("legacy flat Lesson changed during migration")
	}
	if err := os.Remove(flatPath); err != nil {
		rollback()
		return result, err
	}
	if err := syncDirectoryWithFS(opts.LessonsDir, deps.fs); err != nil {
		return result, err
	}
	result.PendingFinalize = true
	for _, observation := range manifest.Observations {
		result.CreatedOccurrences = append(result.CreatedOccurrences, observation.ID)
	}
	return result, nil
}

// FinalizeFlatMigration retires the durable transaction marker only after the
// caller has durably committed the prepared event. It independently verifies
// the canonical artifacts, manifest, exact index projection, and removal of
// the flat source. A crash before this call leaves a marker that makes the CLI
// replay the same deterministic event and narrow index upsert.
func FinalizeFlatMigration(opts FlatMigrationOptions, eventUUID string) error {
	return finalizeFlatMigrationWithDeps(opts, eventUUID, defaultFlatMigrationDeps())
}

func finalizeFlatMigrationWithDeps(opts FlatMigrationOptions, eventUUID string, deps flatMigrationDeps) error {
	if err := validateFlatMigrationOptions(opts); err != nil {
		return err
	}
	markerPath := filepath.Join(opts.LessonsDir, ".flat-migration-"+opts.Slug+".json")
	b, err := os.ReadFile(markerPath)
	if err != nil {
		return err
	}
	var marker flatMigrationMarker
	if err := decodeStrictJSON(b, &marker); err != nil {
		return err
	}
	if marker.SchemaVersion != 1 || marker.Slug != opts.Slug || marker.EventUUID != eventUUID || !occurrenceUUID.MatchString(eventUUID) {
		return fmt.Errorf("migration finalization does not match the durable transaction")
	}
	if strings.Join(marker.Classifications, "\x00") != strings.Join(opts.Classifications, "\x00") {
		return fmt.Errorf("migration finalization classifications differ")
	}
	if _, err := os.Stat(filepath.Join(opts.LessonsDir, opts.Slug+".md")); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("legacy flat Lesson still exists")
		}
		return err
	}
	if err := verifyExpectedFiles(opts.LessonsDir, marker.Files, false); err != nil {
		return err
	}
	canonicalPath := filepath.Join(opts.LessonsDir, opts.Slug, "README.md")
	if err := validateCompletedFlatMigrationProof(opts.LessonsDir, canonicalPath, opts.Slug); err != nil {
		return err
	}
	if err := os.Remove(markerPath); err != nil {
		return err
	}
	return syncDirectoryWithFS(opts.LessonsDir, deps.fs)
}

func validateFlatMigrationIndexRow(lessonsDir, canonicalPath string) error {
	l, err := Parse(canonicalPath)
	if err != nil {
		return err
	}
	occurrences, err := DiscoverOccurrences(canonicalPath)
	if err != nil {
		return err
	}
	last := ""
	if len(occurrences) > 0 {
		last = occurrences[len(occurrences)-1].OccurredAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	enforcement := strings.TrimSpace(l.Control)
	if enforcement == "" {
		enforcement = "—"
	}
	want := fmt.Sprintf("| [%s](%s/README.md) | %s | %s | %d | %s | %s |", l.Slug, l.Slug, l.Status, strings.Join(l.Classifications, ", "), len(occurrences), last, enforcement)
	indexBytes, err := os.ReadFile(filepath.Join(lessonsDir, "README.md"))
	if err != nil {
		return err
	}
	count := 0
	for _, line := range strings.Split(string(indexBytes), "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("lessons index lacks the exact canonical row for %q", l.Slug)
	}
	return nil
}

func stageFlatMigration(opts FlatMigrationOptions, sourceBytes []byte, flatPath string, source LegacySourceRef) (string, flatMigrationManifest, []string, error) {
	return stageFlatMigrationWithDeps(opts, sourceBytes, flatPath, source, osLessonFS{})
}

func stageFlatMigrationWithDeps(opts FlatMigrationOptions, sourceBytes []byte, flatPath string, source LegacySourceRef, fs lessonFS) (string, flatMigrationManifest, []string, error) {
	legacy, err := Parse(flatPath)
	if err != nil {
		return "", flatMigrationManifest{}, nil, err
	}
	if legacy.Canonical || !legacy.HasLessonTitle || strings.TrimSpace(legacy.Title) == "" {
		return "", flatMigrationManifest{}, nil, fmt.Errorf("source is not a structured flat Lesson")
	}
	if !flatStatus[legacy.Status] || legacy.FrontmatterStatus != legacy.Status {
		return "", flatMigrationManifest{}, nil, fmt.Errorf("flat Lesson status is invalid or frontmatter/body statuses differ")
	}
	if !legacy.RecurredValid {
		return "", flatMigrationManifest{}, nil, fmt.Errorf("flat Lesson requires a valid Recurred count")
	}
	date, err := time.Parse("2006-01-02", legacy.Date)
	if err != nil {
		return "", flatMigrationManifest{}, nil, fmt.Errorf("flat Lesson date must be YYYY-MM-DD")
	}
	sections := splitFlatSections(sourceBytes)
	recurrences := flatRecurrenceObservations(sourceBytes, sections["Recurrences"])
	if len(recurrences) != legacy.Recurred {
		return "", flatMigrationManifest{}, nil, fmt.Errorf("recurred is %d but Recurrences contains %d structured entries; refusing to invent or drop history", legacy.Recurred, len(recurrences))
	}

	redacted := []string{}
	title := strings.TrimSpace(legacy.Title)
	if ValidateSafeContent("legacy Lesson title", title) != nil {
		title = titleCaseFromSlug(opts.Slug)
		redacted = append(redacted, "title")
	}
	owner := strings.TrimSpace(legacy.Owner)
	if owner == "" || ValidateSafeContent("legacy Lesson owner", owner) != nil {
		owner = "legacy-migration"
		redacted = append(redacted, "owner")
	}
	lessonText := safeFlatSection("Check", sections["Check"].text, "Apply the control documented by the immutable legacy source.", &redacted)
	processGap := safeFlatSection("Process Gap", sections["Process gap"].text, "The detailed process gap remains in the immutable legacy source.", &redacted)

	readme, err := ScaffoldCanonical(ScaffoldOptions{Slug: opts.Slug, Title: title, Owner: owner, Date: legacy.Date}, opts.Classifications)
	if err != nil {
		return "", flatMigrationManifest{}, nil, err
	}
	provenance := fmt.Sprintf("%s@%s:%s#bytes=0-%d;sha256=%s", source.Repository, source.Revision, source.Path, len(sourceBytes), source.SHA256)
	text := string(readme)
	text = strings.Replace(text, "status: Recorded", "status: "+legacy.Status, 1)
	text = strings.Replace(text, "**Status:** Recorded", "**Status:** "+legacy.Status, 1)
	text = strings.Replace(text, "**Legacy Provenance:** —", "**Legacy Provenance:** "+provenance, 1)
	text = replaceCanonicalSection(text, "Lesson", lessonText)
	text = replaceCanonicalSection(text, "Process Gap", processGap)
	var reviewedEnforcement *flatEnforcement
	if legacy.Status == "Enforced" {
		if strings.TrimSpace(opts.Control) == "" || strings.TrimSpace(opts.Verification) == "" || strings.TrimSpace(opts.Evidence) == "" {
			return "", flatMigrationManifest{}, nil, fmt.Errorf("enforced flat Lesson requires reviewed control, verification, and evidence mappings; provenance alone does not prove enforcement")
		}
		for field, value := range map[string]string{
			"control": opts.Control, "verification": opts.Verification, "evidence": opts.Evidence,
		} {
			if err := ValidateSafeContent("reviewed enforcement "+field, value); err != nil {
				return "", flatMigrationManifest{}, nil, err
			}
		}
		reviewedEnforcement = &flatEnforcement{
			Control:      strings.TrimSpace(opts.Control),
			Verification: strings.TrimSpace(opts.Verification),
			Evidence:     strings.TrimSpace(opts.Evidence),
		}
		text = strings.Replace(text, "**Control:** —", "**Control:** "+reviewedEnforcement.Control, 1)
		text = strings.Replace(text, "**Verification:** —", "**Verification:** "+reviewedEnforcement.Verification, 1)
		text = strings.Replace(text, "**Evidence:** —", "**Evidence:** "+reviewedEnforcement.Evidence, 1)
	} else if strings.TrimSpace(opts.Control) != "" || strings.TrimSpace(opts.Verification) != "" || strings.TrimSpace(opts.Evidence) != "" {
		return "", flatMigrationManifest{}, nil, fmt.Errorf("reviewed enforcement mappings are accepted only when the source Lesson status is Enforced")
	}
	if err := ValidateSafeContent("canonical migrated Lesson", text); err != nil {
		return "", flatMigrationManifest{}, nil, err
	}

	stageDir, err := os.MkdirTemp(opts.LessonsDir, ".flat-migration-stage-")
	if err != nil {
		return "", flatMigrationManifest{}, nil, err
	}
	cleanupOnError := func(cause error) (string, flatMigrationManifest, []string, error) {
		_ = fs.RemoveAll(stageDir)
		return "", flatMigrationManifest{}, nil, cause
	}
	if err := fs.Mkdir(filepath.Join(stageDir, "occurrences"), 0o755); err != nil {
		return cleanupOnError(err)
	}
	if err := writeDurableStageFileWithFS(filepath.Join(stageDir, "README.md"), []byte(text), fs); err != nil {
		return cleanupOnError(err)
	}

	manifest := flatMigrationManifest{
		SchemaVersion: 1,
		Kind:          "specscore.flat-lesson-migration/v1",
		Source:        source,
		Slug:          opts.Slug,
		SourceStatus:  legacy.Status,
		SourceRange:   persistedByteRange{StartByte: 0, EndByte: len(sourceBytes), BytesSHA256: source.SHA256},
		Redactions:    redacted,
		Enforcement:   reviewedEnforcement,
	}
	providerID := legacyOccurrenceID(source.SHA256, "flat#provider", opts.Slug)
	manifest.Observations = append(manifest.Observations, flatObservation{Key: "provider", Kind: "lesson-provider", ID: providerID, StartByte: 0, EndByte: len(sourceBytes), BytesSHA256: source.SHA256})
	for i, recurrence := range recurrences {
		id := legacyOccurrenceID(source.SHA256, fmt.Sprintf("flat#recurrence#%d", i+1), opts.Slug)
		manifest.Observations = append(manifest.Observations, flatObservation{Key: fmt.Sprintf("recurrence#%d", i+1), Kind: "structured-recurrence", ID: id, StartByte: recurrence.start, EndByte: recurrence.end, BytesSHA256: shaString(sourceBytes[recurrence.start:recurrence.end])})
	}
	manifestBytes, err := marshalSafeJSON("flat migration manifest", manifest)
	if err != nil {
		return cleanupOnError(err)
	}
	if err := writeDurableStageFileWithFS(filepath.Join(stageDir, "manifest.json"), manifestBytes, fs); err != nil {
		return cleanupOnError(err)
	}
	manifestRef := filepath.ToSlash(filepath.Join("spec", "lessons", ".legacy-import", "flat-"+source.SHA256+"-"+opts.Slug+".json"))
	for i, observation := range manifest.Observations {
		now := date.UTC().Add(time.Duration(i) * time.Nanosecond)
		if i > 0 {
			if match := flatRecurrenceDate.FindString(recurrences[i-1].text); match != "" {
				if parsed, parseErr := time.Parse("2006-01-02", match); parseErr == nil {
					now = parsed.UTC().Add(time.Duration(i) * time.Nanosecond)
				}
			}
		}
		summary := "Imported structured legacy Lesson provider observation."
		if i > 0 {
			summary = fmt.Sprintf("Imported structured legacy recurrence %d.", i)
		}
		ref := manifestRef
		_, err := AddOccurrence(AddOccurrenceOptions{
			LessonPath: filepath.Join(stageDir, "README.md"),
			ID:         observation.ID,
			Summary:    summary,
			Context: map[string]any{
				"repository": source.Repository,
				"git":        map[string]any{"commit": source.Revision, "branch": nil},
				"execution":  map[string]any{"kind": "unknown", "id": "flat-migration:" + source.SHA256 + "#" + observation.Key},
			},
			Evidence:   Evidence{Kind: "path", Ref: &ref},
			Redactions: []string{"legacy-unstructured-context"},
			Now:        now,
		})
		if err != nil {
			return cleanupOnError(err)
		}
	}
	return stageDir, manifest, redacted, nil
}

func splitFlatSections(source []byte) map[string]flatSection {
	lines := strings.SplitAfter(string(source), "\n")
	offset := 0
	type heading struct {
		name                    string
		lineStart, contentStart int
	}
	var headings []heading
	for _, line := range lines {
		plain := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
		if strings.HasPrefix(plain, "## ") {
			headings = append(headings, heading{name: strings.TrimSpace(strings.TrimPrefix(plain, "## ")), lineStart: offset, contentStart: offset + len(line)})
		}
		offset += len(line)
	}
	out := map[string]flatSection{}
	for i, heading := range headings {
		end := len(source)
		if i+1 < len(headings) {
			end = headings[i+1].lineStart
		}
		raw := source[heading.contentStart:end]
		if footer := bytes.Index(raw, []byte("*This document follows the ")); footer >= 0 {
			end = heading.contentStart + footer
			raw = source[heading.contentStart:end]
		}
		text := strings.TrimSpace(string(raw))
		out[heading.name] = flatSection{start: heading.contentStart, end: end, text: text}
	}
	return out
}

func flatRecurrenceObservations(source []byte, section flatSection) []flatSection {
	if section.end <= section.start {
		return nil
	}
	raw := source[section.start:section.end]
	if separator := bytes.Index(raw, []byte("\n---\n")); separator >= 0 {
		raw = raw[:separator]
	}
	lines := strings.SplitAfter(string(raw), "\n")
	offset := 0
	var starts []int
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(strings.TrimRight(line, "\r\n")), "- ") {
			starts = append(starts, offset)
		}
		offset += len(line)
	}
	var out []flatSection
	for i, start := range starts {
		end := len(raw)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		block := strings.TrimSpace(string(raw[start:end]))
		if footer := strings.Index(block, "*This document follows the "); footer >= 0 {
			block = strings.TrimSpace(block[:footer])
			end = start + len(block)
		}
		out = append(out, flatSection{start: section.start + start, end: section.start + end, text: block})
	}
	return out
}

func safeFlatSection(name, value, fallback string, redacted *[]string) string {
	value = strings.TrimSpace(value)
	if value == "" || ValidateSafeContent("legacy "+name, value) != nil {
		*redacted = append(*redacted, name)
		return fallback
	}
	return value
}

func replaceCanonicalSection(document, name, body string) string {
	startToken := "## " + name + "\n"
	start := strings.Index(document, startToken)
	if start < 0 {
		return document
	}
	bodyStart := start + len(startToken)
	nextRel := strings.Index(document[bodyStart:], "\n## ")
	if nextRel < 0 {
		return document
	}
	next := bodyStart + nextRel + 1
	return document[:bodyStart] + "\n" + strings.TrimSpace(body) + "\n\n" + document[next:]
}

func flatManifestPath(lessonsDir string, source LegacySourceRef, slug string) string {
	return filepath.Join(lessonsDir, ".legacy-import", "flat-"+source.SHA256+"-"+slug+".json")
}

func collectFlatExpectedFiles(stageDir, lessonsDir, slug, manifestPath string) ([]flatExpectedFile, error) {
	var out []flatExpectedFile
	for _, path := range []string{filepath.Join(stageDir, "README.md"), filepath.Join(stageDir, "manifest.json")} {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rel := filepath.ToSlash(filepath.Join(slug, "README.md"))
		if filepath.Base(path) == "manifest.json" {
			rel, err = filepath.Rel(lessonsDir, manifestPath)
			if err != nil {
				return nil, err
			}
			rel = filepath.ToSlash(rel)
		}
		out = append(out, flatExpectedFile{Path: rel, SHA256: shaString(b)})
	}
	entries, err := os.ReadDir(filepath.Join(stageDir, "occurrences"))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("staged occurrence is a directory")
		}
		b, err := os.ReadFile(filepath.Join(stageDir, "occurrences", entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, flatExpectedFile{Path: filepath.ToSlash(filepath.Join(slug, "occurrences", entry.Name())), SHA256: shaString(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func verifyExpectedFiles(lessonsDir string, files []flatExpectedFile, allowMissing bool) error {
	for _, expected := range files {
		if err := validateRepoRelativePath(expected.Path); err != nil || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(expected.SHA256) {
			return fmt.Errorf("migration marker contains invalid expected-file metadata")
		}
		path := filepath.Join(lessonsDir, filepath.FromSlash(expected.Path))
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) && allowMissing {
			continue
		}
		if err != nil {
			return fmt.Errorf("expected migration file %s is unavailable", expected.Path)
		}
		if shaString(b) != expected.SHA256 {
			return fmt.Errorf("migration file collision at %s", expected.Path)
		}
	}
	return nil
}

func ensureFreshMigrationTargetsAbsent(lessonsDir string, files []flatExpectedFile) error {
	for _, expected := range files {
		path := filepath.Join(lessonsDir, filepath.FromSlash(expected.Path))
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("migration target collision at %s", expected.Path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func validateCompletedFlatMigration(canonicalPath string) error {
	l, err := Parse(canonicalPath)
	if err != nil {
		return err
	}
	if !l.Canonical || l.LegacyProvenance == "" || l.LegacyProvenance == "—" || len(l.MissingRequiredSectionsForLayout()) != 0 {
		return fmt.Errorf("canonical Lesson is incomplete or lacks immutable legacy provenance")
	}
	_, err = DiscoverOccurrences(canonicalPath)
	return err
}

// validateCompletedFlatMigrationProof is intentionally stronger than parsing
// the canonical README.  Once the old flat source and transaction marker are
// gone, declaring a migration already complete is safe only when the immutable
// manifest/source proof and the exact denormalized index row still agree.
func validateCompletedFlatMigrationProof(lessonsDir, canonicalPath, slug string) error {
	if err := validateCompletedFlatMigration(canonicalPath); err != nil {
		return err
	}
	l, err := Parse(canonicalPath)
	if err != nil {
		return err
	}
	match := regexp.MustCompile(`^[^@]+@([0-9a-f]{40}):[^#]+#bytes=0-([0-9]+);sha256=([0-9a-f]{64})$`).FindStringSubmatch(l.LegacyProvenance)
	if len(match) != 4 {
		return fmt.Errorf("canonical Lesson has malformed immutable legacy provenance")
	}
	manifestPath := filepath.Join(lessonsDir, ".legacy-import", "flat-"+match[3]+"-"+slug+".json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("immutable migration manifest is unavailable: %w", err)
	}
	var manifest flatMigrationManifest
	if err := decodeStrictJSON(b, &manifest); err != nil {
		return fmt.Errorf("immutable migration manifest is malformed: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Kind != "specscore.flat-lesson-migration/v1" || manifest.Slug != slug || validateLegacySourceRef(manifest.Source) != nil {
		return fmt.Errorf("immutable migration manifest does not describe this canonical Lesson")
	}
	if manifest.Source.Revision != match[1] || manifest.Source.SHA256 != match[3] || fmt.Sprintf("%d", manifest.Source.ByteCount) != match[2] || manifest.SourceRange.StartByte != 0 || manifest.SourceRange.EndByte != manifest.Source.ByteCount || manifest.SourceRange.BytesSHA256 != manifest.Source.SHA256 {
		return fmt.Errorf("immutable migration manifest/source provenance differs from canonical Lesson")
	}
	wantProvenance := fmt.Sprintf("%s@%s:%s#bytes=0-%d;sha256=%s", manifest.Source.Repository, manifest.Source.Revision, manifest.Source.Path, manifest.Source.ByteCount, manifest.Source.SHA256)
	if l.LegacyProvenance != wantProvenance || l.Status != manifest.SourceStatus {
		return fmt.Errorf("canonical Lesson and immutable migration manifest disagree")
	}
	return validateFlatMigrationIndexRow(lessonsDir, canonicalPath)
}

func marshalSafeJSON(field string, value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if err := ValidateSafeContent(field, string(b)); err != nil {
		return nil, err
	}
	return b, nil
}

func decodeStrictJSON(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return fmt.Errorf("trailing JSON")
	} else if err != io.EOF {
		return err
	}
	return nil
}
