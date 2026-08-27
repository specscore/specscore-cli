package lint

import "testing"

func TestReconciliation_String(t *testing.T) {
	r := Reconciliation{
		Rule:     "task-index-row-sync",
		Artifact: "fleet-core-modules-bump",
		Changes: []FieldChange{
			{Field: "status", IndexValue: "planning", FileValue: "queued"},
		},
	}
	want := `task-index-row-sync: fleet-core-modules-bump: status: index had "planning", file has "queued"`
	if got := r.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestReconciliation_StringMultipleChanges(t *testing.T) {
	r := Reconciliation{
		Rule:     "feature-index-row-sync",
		Artifact: "auth",
		Changes: []FieldChange{
			{Field: "title", IndexValue: "Old Title", FileValue: "New Title"},
			{Field: "status", IndexValue: "Draft", FileValue: "Approved"},
		},
	}
	want := `feature-index-row-sync: auth: title: index had "Old Title", file has "New Title"; status: index had "Draft", file has "Approved"`
	if got := r.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
