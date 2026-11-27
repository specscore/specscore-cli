package lesson

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

const receiptFixtureEnforcement = "## Enforcement\n\n**Control:** wb-hook\n**Verification:** go test ./x\n**Evidence:** https://e/1\n\n"

func receiptOf(t *testing.T, content string) ReceiptState {
	t.Helper()
	return ReadReceiptState([]byte(content))
}

func TestEnforcementReceipt_NormalisesWhitespaceOnly(t *testing.T) {
	base := enforcementReceipt("wb-hook", "go test ./x", "https://e/1")
	if len(base) != 64 {
		t.Fatalf("digest %q is not hex SHA-256", base)
	}
	if enforcementReceipt(" wb-hook ", "go  test\t./x", "https://e/1") != base {
		t.Errorf("whitespace-only change altered the receipt")
	}
	for _, changed := range [][3]string{
		{"ci-lesson-check", "go test ./x", "https://e/1"},
		{"wb-hook", "go test ./y", "https://e/1"},
		{"wb-hook", "go test ./x", "https://e/2"},
		{"wb-hook go test ./x", "", "https://e/1"}, // moved between fields
	} {
		if enforcementReceipt(changed[0], changed[1], changed[2]) == base {
			t.Errorf("changed receipt %v hashed equal to the original", changed)
		}
	}
	if got := enforcementReceipt(" ", "", "\t"); got != "" {
		t.Errorf("an empty field set must have no receipt, got %q", got)
	}
}

func TestReadReceiptState_FieldsOnlyFromFirstEnforcementSection(t *testing.T) {
	base := receiptOf(t, "# Lesson: X\n\n"+receiptFixtureEnforcement).Current
	for name, content := range map[string]string{
		"field before Enforcement": "# Lesson: X\n\n**Control:** other\n\n" + receiptFixtureEnforcement,
		"field in Resolution":      "# Lesson: X\n\n" + receiptFixtureEnforcement + "## Resolution\n\nDoes not bind:\n\n**Control:** wb-hook (never runs)\n",
		"second Enforcement":       "# Lesson: X\n\n" + receiptFixtureEnforcement + "## Enforcement\n\n**Control:** other\n",
		"duplicate field":          "# Lesson: X\n\n" + strings.Replace(receiptFixtureEnforcement, "\n\n**Control:** wb-hook\n", "\n\n**Control:** wb-hook\n**Control:** other\n", 1),
		"fenced field":             "# Lesson: X\n\n" + strings.Replace(receiptFixtureEnforcement, "**Evidence:** https://e/1\n", "**Evidence:** https://e/1\n\n```\n**Control:** other\n```\n", 1),
		"crlf and non-field lines": strings.ReplaceAll("# Lesson: X\n\n"+strings.Replace(receiptFixtureEnforcement, "\n\n**Control:**", "\n\nprose\n\n**Control:**", 1), "\n", "\r\n"),
	} {
		if got := receiptOf(t, content).Current; got != base {
			t.Errorf("%s: receipt %q, want %q", name, got, base)
		}
	}
	wrapped := "# Lesson: X\n\n## Enforcement\n\n**Control:** wb-hook\n**Verification:** go\n  test   ./x\n**Evidence:** https://e/1\n"
	if got := receiptOf(t, wrapped).Current; got != base {
		t.Errorf("a re-wrapped field must keep its receipt")
	}
	if got := receiptOf(t, "# Lesson: X\n\n## Enforcement\n\nProse only, no fields.\n").Current; got != "" {
		t.Errorf("a prose-only Enforcement section must have no receipt, got %q", got)
	}
}

func TestReadReceiptState_CountsOnlyStructuralResolutionReceipts(t *testing.T) {
	a, b, c := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	content := "---\nnote: " + CorrectionReceiptPrefix + c + "\n---\n# Lesson: X\n\n" + receiptFixtureEnforcement +
		CorrectionReceiptPrefix + c + "\n\n" + // in Enforcement, not Resolution
		"## Resolution\n\nfirst\n\n" + CorrectionReceiptPrefix + a + "  \n\n" +
		"```markdown\n## Enforcement\n" + CorrectionReceiptPrefix + c + "\n```\n\n" + // fenced heading and receipt
		"    " + CorrectionReceiptPrefix + c + "\n\n" + // indented code
		" " + CorrectionReceiptPrefix + c + "\n\n" + // indented, not where the tool writes
		"<!--\n" + CorrectionReceiptPrefix + c + "\n-->\n\n" + // HTML comment
		CorrectionReceiptPrefix + "not-hex\n\n" +
		"## Open Questions\n\n" + CorrectionReceiptPrefix + c + "\n\n" +
		"## Resolution\n\n" + CorrectionReceiptPrefix + b + "\n"
	got := receiptOf(t, content).Recorded
	if strings.Join(got, ",") != a+","+b {
		t.Fatalf("Recorded = %v, want [a b]", got)
	}
}

func TestReceiptState_DisprovedMatchesAnyRecordedReceipt(t *testing.T) {
	for _, tc := range []struct {
		state ReceiptState
		want  bool
	}{
		{ReceiptState{Current: "r1", Recorded: []string{"r1", "r2"}}, true},
		{ReceiptState{Current: "r2", Recorded: []string{"r1", "r2"}}, true},
		{ReceiptState{Current: "r3", Recorded: []string{"r1", "r2"}}, false},
		{ReceiptState{Current: "", Recorded: []string{""}}, false},
	} {
		if got := tc.state.Disproved(); got != tc.want {
			t.Errorf("%+v.Disproved() = %v, want %v", tc.state, got, tc.want)
		}
	}
}

func TestWithCorrectionReceipt(t *testing.T) {
	if got, want := WithCorrectionReceipt("does not bind \n", "abc"), "does not bind\n\n"+CorrectionReceiptPrefix+"abc"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got, want := WithCorrectionReceipt(" \n", "abc"), CorrectionReceiptPrefix+"abc"; got != want {
		t.Errorf("empty note: got %q want %q", got, want)
	}
}

func TestValidateTransitionNote(t *testing.T) {
	for _, ok := range []string{"", "plain reason", "---\nruled off\n---", "### subheading\n\n```\ncode\n```", "see the Control line"} {
		if err := ValidateTransitionNote(ok); err != nil {
			t.Errorf("ValidateTransitionNote(%q) = %v, want nil", ok, err)
		}
	}
	for bad, want := range map[string]string{
		"x\n  " + CorrectionReceiptPrefix + strings.Repeat("0", 64): "receipt",
		"quoted: enforcement RECEIPT at correction: whatever":       "receipt",
		"Does not bind.\n\n## Details\nSee CI log.":                 "heading",
		"##":                                 "heading",
		"Bad line:\n\n**Control:** ci-check": "Control",
		"> x\n **Verification:** y":          "Verification",
		"**Evidence:** z":                    "Evidence",
		"open fence:\n```\ncode":             "open",
		"open comment <!-- never closed":     "open",
	} {
		err := ValidateTransitionNote(bad)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateTransitionNote(%q) = %v, want error mentioning %q", bad, err, want)
		}
	}
}

func receiptLesson(status string) string {
	return strings.Replace(lessonBody(status), "## Enforcement\n\nRecorded only.\n\n", receiptFixtureEnforcement, 1)
}

func TestCheckReEnforcement(t *testing.T) {
	_, path := stageLesson(t, "kinder-fake", "Recorded")
	if err := CheckReEnforcement("kinder-fake", path); err != nil {
		t.Fatalf("never-demoted lesson refused: %v", err)
	}
	digest := ReadReceiptState([]byte(receiptLesson("Recorded"))).Current
	body := strings.Replace(receiptLesson("Recorded"), "---\n*This", "## Resolution\n\n"+CorrectionReceiptPrefix+digest+"\n\n---\n*This", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckReEnforcement("kinder-fake", path)
	if got := codeOf(t, err); got != exitcode.InvalidState || !strings.Contains(err.Error(), "replace the disproved receipt") {
		t.Fatalf("exit %d err %v, want InvalidState refusal", got, err)
	}
	if got := codeOf(t, CheckReEnforcement("kinder-fake", path+".missing")); got != exitcode.Unexpected {
		t.Fatalf("read error exit = %d, want Unexpected", got)
	}
}

func TestChangeStatus_RejectsUnsafeNote(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Stated")
	before, _ := os.ReadFile(path)
	_, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonRecorded, Note: "## Details", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidArgs {
		t.Fatalf("exit = %d, want InvalidArgs", got)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("rejected note changed the lesson")
	}
}

// The exported API stamps and refuses under the per-Lesson lock, with no CLI
// pre-check involved.
func TestChangeStatus_StampsAndRefusesUnderLock(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Enforced")
	if err := os.WriteFile(path, []byte(receiptLesson("Enforced")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonRecorded, Note: "does not bind", PostMutation: okHook}); err != nil {
		t.Fatalf("demote: %v", err)
	}
	demoted, _ := os.ReadFile(path)
	state := ReadReceiptState(demoted)
	if !state.Disproved() || !strings.Contains(string(demoted), "does not bind\n\n"+CorrectionReceiptPrefix+state.Current+"\n") {
		t.Fatalf("demotion must stamp the receipt after the note:\n%s", demoted)
	}
	_, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonEnforced, PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("exit %d outcome %v, want pre-publication InvalidState", got, MutationOutcomeOf(err))
	}
	if after, _ := os.ReadFile(path); string(after) != string(demoted) {
		t.Fatal("refused re-enforcement changed the lesson")
	}
}

// AC: prose-only-enforcement-has-no-receipt — a compatibility Lesson with prose-only Enforcement has no receipt: the
// demotion stamps none and re-enforcement is never refused forever.
func TestChangeStatus_ProseOnlyEnforcementHasNoReceipt(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Enforced")
	for _, to := range []lifecycle.Status{lifecycle.LessonRecorded, lifecycle.LessonEnforced} {
		if _, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "kinder-fake", To: to, Note: "audited", PostMutation: okHook}); err != nil {
			t.Fatalf("-> %s: %v", to, err)
		}
	}
	if body, _ := os.ReadFile(path); strings.Contains(string(body), CorrectionReceiptPrefix) {
		t.Fatalf("prose-only Enforcement must not be stamped:\n%s", body)
	}
}

func TestChangeStatus_ReceiptReadFailureIsPrePublication(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Stated")
	before, _ := os.ReadFile(path)
	orig := readLessonFn
	readLessonFn = func(string) ([]byte, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { readLessonFn = orig })
	for _, to := range []lifecycle.Status{lifecycle.LessonRecorded, lifecycle.LessonEnforced} {
		_, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "kinder-fake", To: to, Note: "x", PostMutation: okHook})
		if got := codeOf(t, err); got != exitcode.Unexpected || MutationOutcomeOf(err) != MutationPrePublication {
			t.Fatalf("-> %s: exit %d outcome %v, want pre-publication Unexpected", to, got, MutationOutcomeOf(err))
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("failed receipt read changed the lesson")
	}
}
