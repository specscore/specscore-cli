package consilium

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGateYAML writes the canonical schema-header line followed by `body` to
// <dir>/specscore.yaml, mirroring the helper used by the events config tests.
func writeGateYAML(t *testing.T, dir, body string) {
	t.Helper()
	header := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\n"
	path := filepath.Join(dir, "specscore.yaml")
	if err := os.WriteFile(path, []byte(header+body), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
}

func wantBaseline() GateKnobs {
	return GateKnobs{
		AdversaryVetoConfidence: "high",
		CostCeiling:             "medium",
		ComplexityCeiling:       "medium",
		MinMedianConfidence:     "medium",
		RequireAllBuilders:      true,
		RequireAllCustomers:     true,
	}
}

// AC: gate-knobs-default-to-strict-baseline — no consilium.gate mapping.
func TestLoadGate_DefaultsToStrictBaseline(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "project:\n  title: Test\n")

	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected strict baseline %+v, got %+v", wantBaseline(), got)
	}
}

// No specscore.yaml at all → strict baseline (file absent == block absent).
func TestLoadGate_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected strict baseline, got %+v", got)
	}
}

// consilium.gate present but empty → still the strict baseline.
func TestLoadGate_EmptyGateMapping(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate: {}\n")

	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected strict baseline, got %+v", got)
	}
}

// A partial override applies knob-by-knob over the baseline.
func TestLoadGate_PartialOverrideMergesKnobByKnob(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    require_all_builders: false\n    cost_ceiling: low\n")

	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	want := wantBaseline()
	want.RequireAllBuilders = false
	want.CostCeiling = "low"
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

// AC: gate-knob-invalid-enum-rejected — cost_ceiling: extreme.
func TestLoadGate_InvalidEnumRejected(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    cost_ceiling: extreme\n")

	_, err := LoadGate(dir)
	if err == nil {
		t.Fatalf("expected error for out-of-enum cost_ceiling, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"consilium.gate.cost_ceiling", "extreme", "low", "medium"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

// An unknown knob key is rejected, naming the key path.
func TestLoadGate_UnknownKnobRejected(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    bogus_knob: high\n")

	_, err := LoadGate(dir)
	if err == nil {
		t.Fatalf("expected error for unknown knob, got nil")
	}
	if !strings.Contains(err.Error(), "consilium.gate.bogus_knob") {
		t.Fatalf("error %q missing key path consilium.gate.bogus_knob", err.Error())
	}
}

// ResolveGate with no override file behaves exactly like LoadGate.
func TestResolveGate_NoOverrideFile(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    cost_ceiling: low\n")

	got, err := ResolveGate(dir, "")
	if err != nil {
		t.Fatalf("ResolveGate: %v", err)
	}
	want := wantBaseline()
	want.CostCeiling = "low"
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

// ResolveGate merges a `--gate` override file (top-level `gate:` mapping) over
// the specscore.yaml-resolved knobs, knob-by-knob.
func TestResolveGate_OverrideFileMergesOverBaseline(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    cost_ceiling: low\n")

	overridePath := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(overridePath, []byte("gate:\n  require_all_customers: false\n"), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	got, err := ResolveGate(dir, overridePath)
	if err != nil {
		t.Fatalf("ResolveGate: %v", err)
	}
	want := wantBaseline()
	want.CostCeiling = "low"         // from specscore.yaml
	want.RequireAllCustomers = false // from override file
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

// The config --print-gate output shape (a top-level `gate:` mapping with every
// knob) MUST be acceptable verbatim as a `--gate` override file.
func TestResolveGate_AcceptsPrintGateOutputVerbatim(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "project:\n  title: Test\n")

	printGate := "gate:\n" +
		"  adversary_veto_confidence: high\n" +
		"  cost_ceiling: medium\n" +
		"  complexity_ceiling: medium\n" +
		"  min_median_confidence: medium\n" +
		"  require_all_builders: true\n" +
		"  require_all_customers: true\n"
	overridePath := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(overridePath, []byte(printGate), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	got, err := ResolveGate(dir, overridePath)
	if err != nil {
		t.Fatalf("ResolveGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected baseline, got %+v", got)
	}
}

// An out-of-enum value in the override file is rejected, naming the override
// key path and the accepted enum.
func TestResolveGate_OverrideInvalidEnumRejected(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "project:\n  title: Test\n")

	overridePath := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(overridePath, []byte("gate:\n  complexity_ceiling: huge\n"), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	_, err := ResolveGate(dir, overridePath)
	if err == nil {
		t.Fatalf("expected error for out-of-enum override, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"gate.complexity_ceiling", "huge", "low", "medium"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

// An unreadable specscore.yaml (a directory at that path) is a non-ErrNotExist
// read error and must propagate.
func TestLoadGate_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// Make specscore.yaml a directory so os.ReadFile fails with a non-NotExist error.
	if err := os.Mkdir(filepath.Join(dir, "specscore.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := LoadGate(dir); err == nil {
		t.Fatalf("expected read error for directory-as-file, got nil")
	}
}

// Malformed YAML in specscore.yaml is a parse error.
func TestLoadGate_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate: {bad: [unterminated\n")
	if _, err := LoadGate(dir); err == nil {
		t.Fatalf("expected parse error for malformed YAML, got nil")
	}
}

// A non-mapping document root yields the strict baseline (rootMapping returns
// nil, extractGateNode reports not-present).
func TestLoadGate_NonMappingRoot(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "- just\n- a\n- list\n")
	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected baseline for non-mapping root, got %+v", got)
	}
}

// An empty document (no content) yields the strict baseline.
func TestLoadGate_EmptyDocument(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "\n")
	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected baseline for empty document, got %+v", got)
	}
}

// A consilium block present but without a gate key yields the baseline.
func TestLoadGate_ConsiliumWithoutGate(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  roster:\n    exclude: [pm]\n")
	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected baseline when consilium has no gate, got %+v", got)
	}
}

// consilium.gate: null is a no-op merge yielding the baseline.
func TestLoadGate_NullGateMapping(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n")
	got, err := LoadGate(dir)
	if err != nil {
		t.Fatalf("LoadGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected baseline for null gate, got %+v", got)
	}
}

// A gate node that is a scalar (not a mapping, not null) is rejected.
func TestLoadGate_GateNotAMapping(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate: nonsense\n")
	if _, err := LoadGate(dir); err == nil {
		t.Fatalf("expected error for scalar gate node, got nil")
	}
}

// A bool knob given a non-bool scalar value is rejected, naming the path and
// the accepted true|false values.
func TestLoadGate_InvalidBoolKnob(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    require_all_builders: maybe\n")
	_, err := LoadGate(dir)
	if err == nil {
		t.Fatalf("expected error for invalid bool knob, got nil")
	}
	for _, want := range []string{"consilium.gate.require_all_builders", "true", "false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// An enum knob given a non-scalar (mapping) value is rejected.
func TestLoadGate_EnumKnobNonScalar(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    cost_ceiling:\n      nested: value\n")
	_, err := LoadGate(dir)
	if err == nil {
		t.Fatalf("expected error for non-scalar enum knob, got nil")
	}
	if !strings.Contains(err.Error(), "cost_ceiling") {
		t.Errorf("error %q missing cost_ceiling", err.Error())
	}
}

// ResolveGate propagates a LoadGate error (malformed specscore.yaml).
func TestResolveGate_LoadGateError(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    cost_ceiling: extreme\n")
	if _, err := ResolveGate(dir, filepath.Join(dir, "gate.yaml")); err == nil {
		t.Fatalf("expected LoadGate error to propagate, got nil")
	}
}

// ResolveGate with a missing override file is a read error.
func TestResolveGate_MissingOverrideFile(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "project:\n  title: Test\n")
	if _, err := ResolveGate(dir, filepath.Join(dir, "does-not-exist.yaml")); err == nil {
		t.Fatalf("expected read error for missing override file, got nil")
	}
}

// ResolveGate with a malformed override file is a parse error.
func TestResolveGate_MalformedOverrideFile(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "project:\n  title: Test\n")
	overridePath := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(overridePath, []byte("gate: {bad: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}
	if _, err := ResolveGate(dir, overridePath); err == nil {
		t.Fatalf("expected parse error for malformed override, got nil")
	}
}

// ResolveGate with an override file lacking a top-level gate key leaves the
// baseline untouched (extractTopLevelGateNode reports not-present).
func TestResolveGate_OverrideWithoutGateKey(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "consilium:\n  gate:\n    cost_ceiling: low\n")
	overridePath := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(overridePath, []byte("other: stuff\n"), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}
	got, err := ResolveGate(dir, overridePath)
	if err != nil {
		t.Fatalf("ResolveGate: %v", err)
	}
	want := wantBaseline()
	want.CostCeiling = "low"
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

// ResolveGate override file with a non-mapping root leaves baseline untouched.
func TestResolveGate_OverrideNonMappingRoot(t *testing.T) {
	dir := t.TempDir()
	writeGateYAML(t, dir, "project:\n  title: Test\n")
	overridePath := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(overridePath, []byte("- a\n- b\n"), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}
	got, err := ResolveGate(dir, overridePath)
	if err != nil {
		t.Fatalf("ResolveGate: %v", err)
	}
	if got != wantBaseline() {
		t.Fatalf("expected baseline, got %+v", got)
	}
}

func TestStrictBaseline_Values(t *testing.T) {
	if StrictBaseline() != wantBaseline() {
		t.Fatalf("StrictBaseline() = %+v, want %+v", StrictBaseline(), wantBaseline())
	}
}
