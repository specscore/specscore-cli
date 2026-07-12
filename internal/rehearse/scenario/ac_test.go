package scenario_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

func TestParseAC(t *testing.T) {
	src := "# AC: session-hardened\n\n" +
		"**Verifies:** ../README.md#req:converged-session\n" +
		"**Status:** accepted\n" +
		"**Applies-to:** email, google, facebook\n\n" +
		"## Statement\n\n" +
		"After any successful sign-in, the session cookie is HttpOnly and Secure.\n"
	ac, err := scenario.ParseACBytes("session-hardened.ac.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseACBytes: %v", err)
	}
	if ac.Slug != "session-hardened" {
		t.Errorf("Slug = %q", ac.Slug)
	}
	if ac.Verifies != "../README.md#req:converged-session" {
		t.Errorf("Verifies = %q", ac.Verifies)
	}
	if ac.Status != "accepted" {
		t.Errorf("Status = %q", ac.Status)
	}
	if strings.Join(ac.AppliesTo, ",") != "email,google,facebook" {
		t.Errorf("AppliesTo = %v", ac.AppliesTo)
	}
	if !strings.Contains(ac.Statement, "HttpOnly and Secure") {
		t.Errorf("Statement = %q", ac.Statement)
	}
}

func TestParseAC_MissingHeadingErrors(t *testing.T) {
	_, err := scenario.ParseACBytes("x.ac.md", []byte("**Status:** accepted\n"))
	if err == nil {
		t.Fatal("want error for a file with no `# AC:` heading")
	}
}

func TestParseAC_File(t *testing.T) {
	path := write(t, "# AC: f\n\n## Statement\n\nSomething true.\n")
	ac, err := scenario.ParseAC(path)
	if err != nil {
		t.Fatalf("ParseAC: %v", err)
	}
	if ac.Slug != "f" || ac.Statement != "Something true." {
		t.Errorf("ac = %+v", ac)
	}
}

func TestParseAC_MissingFile(t *testing.T) {
	if _, err := scenario.ParseAC(filepath.Join(t.TempDir(), "nope.ac.md")); err == nil {
		t.Fatal("want error for a missing AC file")
	}
}

func TestGenerateACSummary(t *testing.T) {
	acs := []scenario.AC{
		{Slug: "zeta", Statement: "Z must hold.", Verifies: "#req:z", Status: "accepted"},
		{Slug: "alpha", Statement: "A\nmust  hold | across lines.", Verifies: "", Status: ""},
	}
	out := scenario.GenerateACSummary(acs)
	// Sorted by slug: alpha before zeta.
	ai, zi := strings.Index(out, "alpha"), strings.Index(out, "zeta")
	if ai < 0 || zi < 0 || ai > zi {
		t.Errorf("rows not sorted by slug:\n%s", out)
	}
	// One-lined + pipe-escaped statement, em-dash for empty fields.
	if !strings.Contains(out, "A must hold \\| across lines.") {
		t.Errorf("statement not collapsed/escaped:\n%s", out)
	}
	if !strings.Contains(out, "| — | — |") {
		t.Errorf("empty verifies/status not dashed:\n%s", out)
	}
	if !strings.Contains(out, "[alpha](_acs/alpha.ac.md)") {
		t.Errorf("slug not linked:\n%s", out)
	}
}
