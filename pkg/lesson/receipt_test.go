package lesson

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnforcementReceipt_NormalisesWhitespaceOnly(t *testing.T) {
	base := EnforcementReceipt(&Lesson{Control: "wb-hook", Verification: "go test ./x", Evidence: "https://e/1"})
	if len(base) != 64 {
		t.Fatalf("digest %q is not hex SHA-256", base)
	}
	if got := EnforcementReceipt(&Lesson{Control: " wb-hook ", Verification: "go  test\t./x", Evidence: "https://e/1"}); got != base {
		t.Errorf("whitespace-only change altered the receipt")
	}
	for _, changed := range []*Lesson{
		{Control: "ci-lesson-check", Verification: "go test ./x", Evidence: "https://e/1"},
		{Control: "wb-hook", Verification: "go test ./y", Evidence: "https://e/1"},
		{Control: "wb-hook", Verification: "go test ./x", Evidence: "https://e/2"},
		// A value moved between fields is a different receipt.
		{Control: "wb-hook go test ./x", Verification: "", Evidence: "https://e/1"},
	} {
		if EnforcementReceipt(changed) == base {
			t.Errorf("changed receipt %+v hashed equal to the original", *changed)
		}
	}
}

func TestWithCorrectionReceipt(t *testing.T) {
	if got, want := WithCorrectionReceipt("does not bind \n", "abc"), "does not bind\n\n"+CorrectionReceiptPrefix+"abc"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestParse_CorrectionReceiptLastInResolutionWins(t *testing.T) {
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	body := "# Lesson: X\n\n**Status:** Recorded\n\n## Enforcement\n\n" +
		CorrectionReceiptPrefix + strings.Repeat("c", 64) + "\n\n" + // outside Resolution: ignored
		"**Control:** wb-hook\n\n## Resolution\n\nfirst\n\n" + CorrectionReceiptPrefix + first + "\n\n" +
		"second\n\n  " + CorrectionReceiptPrefix + second + "  \n\n" +
		CorrectionReceiptPrefix + "not-hex\n"
	path := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.CorrectionReceipt != second {
		t.Errorf("CorrectionReceipt = %q, want the last Resolution receipt %q", l.CorrectionReceipt, second)
	}
	if l.Control != "wb-hook" {
		t.Errorf("receipt lines must not disturb field parsing: Control = %q", l.Control)
	}
}

func TestParse_NoCorrectionReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(path, []byte("# Lesson: X\n\n## Resolution\n\nAudited correction: returned to Recorded.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.CorrectionReceipt != "" {
		t.Errorf("CorrectionReceipt = %q, want empty", l.CorrectionReceipt)
	}
}
