package lint

import "testing"

func TestCountFailing_ExcludesAdvisory(t *testing.T) {
	vs := []Violation{
		{Rule: "a", Severity: "info"},
		{Rule: "b", Severity: "info", Advisory: true},
		{Rule: "c", Severity: "error"},
	}
	if got := CountFailing(vs); got != 2 {
		t.Fatalf("CountFailing = %d, want 2", got)
	}
	if CountFailing(nil) != 0 {
		t.Fatal("no violations, none failing")
	}
}

func TestGraphRules_OnlyTheSpellingNoticeIsAdvisory(t *testing.T) {
	for _, r := range GraphRules() {
		if want := r.ID == "graph-model-deprecated-spelling"; r.Advisory != want {
			t.Errorf("%s: Advisory = %v, want %v", r.ID, r.Advisory, want)
		}
	}
}
