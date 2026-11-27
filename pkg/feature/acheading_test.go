package feature

import "testing"

func TestACHeadingID(t *testing.T) {
	tests := []struct {
		line   string
		wantID string
		wantOK bool
	}{
		{"### AC: total-visible", "total-visible", true},
		{"### AC: total-visible (verifies REQ:totals)", "total-visible", true},
		{"### AC: a1-b2 (verifies cli/init#req:plan-routing-hint-appended)", "a1-b2", true},
		{"  ### AC: indented  ", "indented", true}, // surrounding whitespace ignored, as lint ignores it
		{"### AC: total-visible\r", "total-visible", true},
		{"###  AC: doubled-space", "", false},
		{"### AC:  doubled-space", "", false},
		{"### AC: Upper-Case", "", false},
		{"### AC: under_score", "", false},
		{"### AC: trailing text", "", false},
		{"### AC: id  (verifies REQ:x)", "", false},
		{"### AC: id (verifies (nested))", "", false},
		{"### AC: id (see REQ:x)", "", false},
		{"### AC:", "", false},
		{"#### AC: too-deep", "", false},
		{"#### REQ: not-an-ac", "", false},
	}
	for _, tt := range tests {
		gotID, gotOK := ACHeadingID(tt.line)
		if gotID != tt.wantID || gotOK != tt.wantOK {
			t.Errorf("ACHeadingID(%q) = (%q, %v), want (%q, %v)", tt.line, gotID, gotOK, tt.wantID, tt.wantOK)
		}
	}
}
