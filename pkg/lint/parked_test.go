package lint

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func parkedViolations(t *testing.T, specRoot string) []Violation {
	t.Helper()
	v, err := newParkedChecker().check(specRoot)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	return v
}

// Absent **Parked:** axis is valid — no violations.
func TestParked_AbsentIsValid(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", featHdr+"**Status:** Approved\n\n## Summary\n\nx\n")
	if vs := parkedViolations(t, sr); len(vs) != 0 {
		t.Fatalf("expected no violations, got %v", vs)
	}
}

// A well-formed, recent park is valid.
func TestParked_WellFormedRecentIsValid(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	today := time.Now().UTC().Format(dateLayout)
	writeMD(t, sr, "features/foo/README.md", featHdr+
		"**Status:** Approved\n**Parked:** true\n**Parked Reason:** not v1\n**Parked Date:** "+today+"\n\n## Summary\n\nx\n")
	if vs := parkedViolations(t, sr); len(vs) != 0 {
		t.Fatalf("expected no violations, got %v", vs)
	}
}

// **Parked:** true with no **Parked Reason:** is a shape error.
func TestParked_MissingReasonIsShapeError(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	today := time.Now().UTC().Format(dateLayout)
	writeMD(t, sr, "features/foo/README.md", featHdr+
		"**Status:** Approved\n**Parked:** true\n**Parked Date:** "+today+"\n\n## Summary\n\nx\n")
	vs := parkedViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "parked-shape" {
		t.Fatalf("expected one parked-shape violation, got %v", vs)
	}
	if !strings.Contains(vs[0].Message, "Parked Reason") {
		t.Fatalf("message should name the missing field: %s", vs[0].Message)
	}
}

// **Parked:** true with no **Parked Date:** is a shape error.
func TestParked_MissingDateIsShapeError(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", featHdr+
		"**Status:** Approved\n**Parked:** true\n**Parked Reason:** not v1\n\n## Summary\n\nx\n")
	vs := parkedViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "parked-shape" {
		t.Fatalf("expected one parked-shape violation, got %v", vs)
	}
	if !strings.Contains(vs[0].Message, "Parked Date") {
		t.Fatalf("message should name the missing field: %s", vs[0].Message)
	}
}

// Both missing → two distinct shape violations.
func TestParked_MissingBothIsTwoShapeErrors(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", featHdr+"**Status:** Approved\n**Parked:** true\n\n## Summary\n\nx\n")
	vs := parkedViolations(t, sr)
	if len(vs) != 2 {
		t.Fatalf("expected two parked-shape violations, got %v", vs)
	}
	for _, v := range vs {
		if v.Rule != "parked-shape" {
			t.Errorf("expected parked-shape, got %s", v.Rule)
		}
	}
}

// A malformed date is a shape error, not a staleness warning.
func TestParked_MalformedDateIsShapeError(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", featHdr+
		"**Status:** Approved\n**Parked:** true\n**Parked Reason:** not v1\n**Parked Date:** not-a-date\n\n## Summary\n\nx\n")
	vs := parkedViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "parked-shape" {
		t.Fatalf("expected one parked-shape violation, got %v", vs)
	}
}

// **Parked:** false (or any non-"true" value) is treated as not parked — no
// shape or staleness checks apply, even with a dangling reason/date line.
func TestParked_FalseValueIsNotParked(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", featHdr+"**Status:** Approved\n**Parked:** false\n\n## Summary\n\nx\n")
	if vs := parkedViolations(t, sr); len(vs) != 0 {
		t.Fatalf("expected no violations, got %v", vs)
	}
}

// Exactly at the boundary (age == staleDays) does not warn; one day past does.
func TestParked_StaleBoundary(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\nparked:\n  stale_days: 10\n")

	atBoundary := time.Now().UTC().AddDate(0, 0, -10).Format(dateLayout)
	pastBoundary := time.Now().UTC().AddDate(0, 0, -11).Format(dateLayout)

	writeMD(t, sr, "features/at-boundary/README.md", featHdr+
		fmt.Sprintf("**Status:** Approved\n**Parked:** true\n**Parked Reason:** r\n**Parked Date:** %s\n\n## Summary\n\nx\n", atBoundary))
	writeMD(t, sr, "features/past-boundary/README.md", featHdr+
		fmt.Sprintf("**Status:** Approved\n**Parked:** true\n**Parked Reason:** r\n**Parked Date:** %s\n\n## Summary\n\nx\n", pastBoundary))

	vs := parkedViolations(t, sr)
	if len(vs) != 1 {
		t.Fatalf("expected exactly one parked-stale violation, got %v", vs)
	}
	if vs[0].Rule != "parked-stale" || vs[0].Severity != "warning" {
		t.Fatalf("expected a parked-stale warning, got %+v", vs[0])
	}
	if !strings.Contains(vs[0].File, "past-boundary") {
		t.Fatalf("expected the violation on the past-boundary file, got %s", vs[0].File)
	}
}

// The default 90-day window applies when no `parked:` config block is set.
func TestParked_StaleDefaultWindow(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	old := time.Now().UTC().AddDate(0, 0, -91).Format(dateLayout)
	writeMD(t, sr, "features/foo/README.md", featHdr+
		fmt.Sprintf("**Status:** Approved\n**Parked:** true\n**Parked Reason:** r\n**Parked Date:** %s\n\n## Summary\n\nx\n", old))
	vs := parkedViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "parked-stale" {
		t.Fatalf("expected one parked-stale violation, got %v", vs)
	}
}

// The checker is registered under both rule IDs in the shared registry.
func TestParked_RuleNamesRegistered(t *testing.T) {
	l := newLinter(Options{})
	for _, n := range parkedRuleNames {
		if _, ok := l.ruleSet[n]; !ok {
			t.Errorf("rule %q not registered in linter", n)
		}
	}
}

func TestParkedCheckerMetadata(t *testing.T) {
	checker := newParkedChecker()
	if checker.name() != "parked-shape" || checker.severity() != "error" {
		t.Fatalf("checker metadata = %q/%q", checker.name(), checker.severity())
	}
}
