package consilium

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVotes writes body to <dir>/votes.yaml and returns the path. Used by the
// file-based tests so the parse path is exercised through real bytes on disk.
func writeVotes(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "votes.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write votes.yaml: %v", err)
	}
	return path
}

// validBundle is a two-vote YAML bundle every entry of which is in-enum with a
// short argument; used as the happy-path fixture.
const validBundle = `- verdict: should-implement
  confidence: high
  cost: 🟢
  complexity: 🟡
  argument: Clear user value and low risk.
  role: engineer
- verdict: abstain
  confidence: low
  cost: 🔴
  complexity: 🔴
  argument: Not my domain.
  role: ux
`

func TestParseVotes_ValidBundleParsesVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := writeVotes(t, dir, validBundle)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read votes.yaml: %v", err)
	}

	votes, err := ParseVotes(data)
	if err != nil {
		t.Fatalf("ParseVotes: %v", err)
	}
	if len(votes) != 2 {
		t.Fatalf("expected 2 votes, got %d", len(votes))
	}

	want := []Vote{
		{Verdict: "should-implement", Confidence: "high", Cost: "🟢", Complexity: "🟡", Argument: "Clear user value and low risk.", Role: "engineer"},
		{Verdict: "abstain", Confidence: "low", Cost: "🔴", Complexity: "🔴", Argument: "Not my domain.", Role: "ux"},
	}
	for i, w := range want {
		if votes[i] != w {
			t.Errorf("vote[%d] = %+v, want %+v", i, votes[i], w)
		}
	}
}

// TestParseVotes_AC_MalformedRejected covers the malformed-bundle AC: one vote
// with an out-of-enum verdict and another with a 281-char argument. Each
// offending case is asserted independently because zero-tolerance validation
// stops at the first violation.
func TestParseVotes_AC_MalformedRejected(t *testing.T) {
	t.Run("out_of_enum_verdict", func(t *testing.T) {
		bundle := `- verdict: maybe
  confidence: high
  cost: 🟢
  complexity: 🟢
  argument: ok
  role: skeptic
`
		_, err := ParseVotes([]byte(bundle))
		if err == nil {
			t.Fatal("expected error for out-of-enum verdict, got nil")
		}
		msg := err.Error()
		for _, want := range []string{"vote[0]", "skeptic", "verdict", "maybe", "should-implement"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error message missing %q; got: %s", want, msg)
			}
		}
	})

	t.Run("over_length_argument", func(t *testing.T) {
		bundle := "- verdict: should-implement\n" +
			"  confidence: high\n" +
			"  cost: 🟢\n" +
			"  complexity: 🟢\n" +
			"  role: pm\n" +
			"  argument: " + strings.Repeat("x", 281) + "\n"
		_, err := ParseVotes([]byte(bundle))
		if err == nil {
			t.Fatal("expected error for 281-char argument, got nil")
		}
		msg := err.Error()
		for _, want := range []string{"vote[0]", "pm", "argument", "280"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error message missing %q; got: %s", want, msg)
			}
		}
	})

	// A bundle containing both offending votes must fail (no partial result).
	t.Run("no_partial_verdict", func(t *testing.T) {
		bundle := "- verdict: maybe\n" +
			"  confidence: high\n" +
			"  cost: 🟢\n" +
			"  complexity: 🟢\n" +
			"  argument: ok\n" +
			"- verdict: should-implement\n" +
			"  confidence: high\n" +
			"  cost: 🟢\n" +
			"  complexity: 🟢\n" +
			"  argument: " + strings.Repeat("x", 281) + "\n"
		votes, err := ParseVotes([]byte(bundle))
		if err == nil {
			t.Fatal("expected error for malformed bundle, got nil")
		}
		if votes != nil {
			t.Fatalf("expected nil votes on failure, got %v", votes)
		}
	})
}

func TestValidateVote_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		vote      Vote
		wantErr   bool
		wantField string
	}{
		{
			name: "valid",
			vote: Vote{Verdict: "no-opinion", Confidence: "medium", Cost: "🟡", Complexity: "🟢", Argument: "fine"},
		},
		{
			name:      "out_of_enum_verdict",
			vote:      Vote{Verdict: "maybe", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "x"},
			wantErr:   true,
			wantField: "verdict",
		},
		{
			name:      "missing_verdict",
			vote:      Vote{Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "x"},
			wantErr:   true,
			wantField: "verdict",
		},
		{
			name:      "out_of_enum_confidence",
			vote:      Vote{Verdict: "abstain", Confidence: "certain", Cost: "🟢", Complexity: "🟢", Argument: "x"},
			wantErr:   true,
			wantField: "confidence",
		},
		{
			name:      "out_of_enum_cost",
			vote:      Vote{Verdict: "abstain", Confidence: "low", Cost: "blue", Complexity: "🟢", Argument: "x"},
			wantErr:   true,
			wantField: "cost",
		},
		{
			name:      "out_of_enum_complexity",
			vote:      Vote{Verdict: "abstain", Confidence: "low", Cost: "🟢", Complexity: "purple", Argument: "x"},
			wantErr:   true,
			wantField: "complexity",
		},
		{
			name:      "missing_argument",
			vote:      Vote{Verdict: "abstain", Confidence: "low", Cost: "🟢", Complexity: "🟢"},
			wantErr:   true,
			wantField: "argument",
		},
		{
			name:    "argument_exactly_280_ok",
			vote:    Vote{Verdict: "abstain", Confidence: "low", Cost: "🟢", Complexity: "🟢", Argument: strings.Repeat("a", 280)},
			wantErr: false,
		},
		{
			name:      "argument_281_rejected",
			vote:      Vote{Verdict: "abstain", Confidence: "low", Cost: "🟢", Complexity: "🟢", Argument: strings.Repeat("a", 281)},
			wantErr:   true,
			wantField: "argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVote(0, tt.vote)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				ve, ok := err.(*ValidationError)
				if !ok {
					t.Fatalf("expected *ValidationError, got %T", err)
				}
				if ve.Field != tt.wantField {
					t.Errorf("Field = %q, want %q", ve.Field, tt.wantField)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestParseVotes_YAMLParseFailure(t *testing.T) {
	_, err := ParseVotes([]byte("verdict: should-implement\n  bad: : indent"))
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse votes") {
		t.Errorf("error message missing 'parse votes'; got: %s", err.Error())
	}
}

// TestValidationError_ErrorFormat covers both the with-role and without-role
// branches of the message formatter.
func TestValidationError_ErrorFormat(t *testing.T) {
	withRole := (&ValidationError{Index: 2, Role: "qa", Field: "verdict", Rule: "bad"}).Error()
	if !strings.Contains(withRole, "vote[2]") || !strings.Contains(withRole, "role=qa") {
		t.Errorf("with-role format wrong: %s", withRole)
	}
	noRole := (&ValidationError{Index: 1, Field: "argument", Rule: "bad"}).Error()
	if !strings.Contains(noRole, "vote[1]") || strings.Contains(noRole, "role=") {
		t.Errorf("no-role format wrong: %s", noRole)
	}
}
