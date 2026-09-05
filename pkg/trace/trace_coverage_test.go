package trace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/feature"
)

func TestParseFeatureDirectoryAndFallbackPaths(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "checkout")
	if err := os.MkdirAll(filepath.Join(dir, "_tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Feature: Checkout\n\n**Status:** Draft\n\n## Acceptance Criteria\n\nNot defined yet.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseFeature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "checkout" || r.Source.StartLine != 1 {
		t.Fatalf("directory parse = %+v", r)
	}
	noTitle := filepath.Join(root, "untitled", "README.md")
	if err := os.MkdirAll(filepath.Dir(noTitle), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noTitle, []byte("**Status:** Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = ParseFeature(noTitle)
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.StartLine != 1 || r.Title != "" {
		t.Fatalf("untitled parse = %+v", r)
	}
}

func TestParseFeatureReadAndScenarioErrors(t *testing.T) {
	featurePath := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(featurePath, []byte("# Feature: Read\n**Status:** Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRead := readTraceFile
	oldStatus, oldTitle := parseTraceStatus, parseTraceTitle
	t.Cleanup(func() {
		readTraceFile = oldRead
		parseTraceStatus, parseTraceTitle = oldStatus, oldTitle
	})
	readTraceFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
	if _, err := ParseFeature(featurePath); err == nil {
		t.Fatal("expected feature read error")
	}
	parseTraceStatus = func(string) (string, error) { return "", errors.New("status failed") }
	if _, err := ParseFeature(featurePath); err == nil {
		t.Fatal("expected feature status error")
	}
	parseTraceStatus = func(string) (string, error) { return "Draft", nil }
	parseTraceTitle = func(string) (string, error) { return "", errors.New("title failed") }
	if _, err := ParseFeature(featurePath); err == nil {
		t.Fatal("expected feature title error")
	}
	parseTraceTitle = oldTitle
	readTraceFile = oldRead
	oldReadDir := readTraceDir
	t.Cleanup(func() { readTraceDir = oldReadDir })
	readTraceDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("scenario directory failed") }
	if _, err := ParseFeature(featurePath); err == nil {
		t.Fatal("expected feature scenario error")
	}
	if _, err := ParseScenario(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("expected scenario read error")
	}
}

func TestDiscoverAndSnapshotError(t *testing.T) {
	root := t.TempDir()
	featureDir := filepath.Join(root, "spec", "features", "one")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte("# Feature: One\n\n**Status:** Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	secondFeatureDir := filepath.Join(root, "spec", "features", "zero")
	if err := os.MkdirAll(secondFeatureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondFeatureDir, "README.md"), []byte("# Feature: Zero\n\n**Status:** Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	features, err := Discover(root)
	if err != nil || len(features) != 2 || features[0].ID != "one" || features[1].ID != "zero" {
		t.Fatalf("discover = %v %+v", err, features)
	}
	oldDiscover := discoverTrace
	oldFeatures := discoverTraceFeatures
	t.Cleanup(func() {
		discoverTrace = oldDiscover
		discoverTraceFeatures = oldFeatures
	})
	discoverTrace = func(string) ([]FeatureRecord, error) { return nil, errors.New("discover failed") }
	if _, err := SnapshotForRoot(root); err == nil {
		t.Fatal("expected snapshot discovery error")
	}
	discoverTraceFeatures = func(string) ([]feature.Feature, error) { return nil, errors.New("walk failed") }
	if _, err := Discover(root); err == nil {
		t.Fatal("expected feature discovery error")
	}
	discoverTraceFeatures = func(string) ([]feature.Feature, error) {
		return []feature.Feature{{ID: "missing"}}, nil
	}
	if _, err := Discover(root); err == nil {
		t.Fatal("expected feature parse error")
	}
}

func TestParseScenarioTitleAndFallbackIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "_tests", "login.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Scenario: Login\n\n**Validates:** auth#REQ:login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseScenario(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.FeatureID != "auth" || r.Title != "Login" || r.Validates[0] != "auth#req:login" {
		t.Fatalf("scenario = %+v", r)
	}
	if err := os.WriteFile(path, []byte("plain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = ParseScenario(path)
	if err != nil || r.Title != "login" || r.Source.StartLine != 1 {
		t.Fatalf("fallback scenario = %+v %v", r, err)
	}
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseScenario(empty); err == nil {
		t.Fatal("expected empty scenario error")
	}
}

func TestParseScenariosDirectoryBranchesAndError(t *testing.T) {
	featureDir := t.TempDir()
	dir := filepath.Join(featureDir, "_tests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Scenario: A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# Scenario: B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldReadDir := readTraceDir
	t.Cleanup(func() { readTraceDir = oldReadDir })
	readTraceDir = os.ReadDir
	got, err := parseScenarios(featureDir, "auth")
	if err != nil || len(got) != 2 {
		t.Fatalf("scenarios = %v %+v", err, got)
	}
	oldReadFile := readTraceFile
	t.Cleanup(func() { readTraceFile = oldReadFile })
	readTraceFile = func(string) ([]byte, error) { return nil, errors.New("scenario failed") }
	if _, err := parseScenarios(featureDir, "auth"); err == nil {
		t.Fatal("expected scenario parse error")
	}
	readTraceDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("directory failed") }
	if _, err := parseScenarios(featureDir, "auth"); err == nil {
		t.Fatal("expected directory read error")
	}
}

func TestTraceStructuralHelpersCoverFallbacks(t *testing.T) {
	lines := []string{
		"## Behavior", "### Topic", "#### REQ: one", "body", "## Next", "### AC: outside",
		"```md", "### AC: fake", "```", "## Acceptance Criteria", "### AC: no-metadata", "body",
	}
	m := make([]bool, len(lines))
	for i := range m {
		m[i] = true
	}
	m[6], m[7], m[8] = false, false, false
	record := &FeatureRecord{ID: "x", Path: "x.md"}
	parseFeatureChildren(record, lines)
	if len(record.Requirements) != 1 || len(record.AcceptanceCriteria) != 1 {
		t.Fatalf("children = %+v", record)
	}
	if nextHeadingEnd(lines, m, 4, 4) != 4 || nextHeadingEnd(lines, m, 6, 3) != 9 || nextHeadingEnd(lines, m, 12, 3) != len(lines) {
		t.Fatal("heading boundary fallback not exercised")
	}
	if lineEndColumn(lines, 0) != 1 || lineEndColumn(lines, len(lines)+1) != 1 {
		t.Fatal("line end invalid bounds")
	}
	if featureIDFromPath("/tmp/project/checkout/README.md") != "checkout" {
		t.Fatal("feature path fallback")
	}
	if featureIDFromScenarioPath("/tmp/project/checkout/_tests/a.md") != "checkout" {
		t.Fatal("scenario path fallback")
	}
	if firstMatchingLine(lines, func(string) bool { return false }) != 0 {
		t.Fatal("first matching line fallback")
	}
}

func TestParseRequirementsFieldMissingAndMasked(t *testing.T) {
	lines := []string{"### AC: x", "```", "**Requirements:** x#req:masked", "```", "body"}
	mask := []bool{true, false, false, false, true}
	if got := parseRequirementsField(lines, mask, 1, len(lines)); got != nil {
		t.Fatalf("masked requirements = %v", got)
	}
	if got := parseRequirementsField(lines, mask, 4, len(lines)); got != nil {
		t.Fatalf("missing requirements = %v", got)
	}
	if inSection([]string{"# Feature: x"}, []bool{true}, 0, "Acceptance Criteria") {
		t.Fatal("feature title should end section search")
	}
	if inSection([]string{"prose"}, []bool{true}, 0, "Acceptance Criteria") {
		t.Fatal("section search should return false at EOF")
	}
	if inSection([]string{"## Acceptance Criteria", "### AC: x"}, []bool{true, false}, 1, "Acceptance Criteria") != true {
		t.Fatal("masked section heading should be skipped")
	}
}
