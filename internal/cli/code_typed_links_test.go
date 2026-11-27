package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// specscore:verifies https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#ac:typed-implementation-and-test-links-checked
func TestCodeDeps_CheckTypedRequirementAndAcceptanceCriterionCitations(t *testing.T) {
	root := t.TempDir()
	config := projectdef.SpecConfig{Project: &projectdef.ProjectConfig{Host: "github.com", Org: "acme", Repo: "checkout"}}
	if err := projectdef.WriteSpecConfig(root, config); err != nil {
		t.Fatal(err)
	}
	featureDir := filepath.Join(root, "spec", "features", "checkout")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	feature := "# Feature: Checkout\n\n#### REQ: totals\n\n## Acceptance Criteria\n\n### AC: total-visible\n"
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte(feature), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "package checkout\n\n// specscore:" + "implements https://specscore.org/github.com/acme/checkout/spec/features/checkout#req:totals\n" +
		"func Calculate() {}\n\n// specscore:" + "verifies https://specscore.org/github.com/acme/checkout/spec/features/checkout#ac:total-visible\n" +
		"func TestCalculate() {}\n"
	if err := os.WriteFile(filepath.Join(root, "checkout.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	out, _, err := runCode(t, "deps", "--path=checkout.go", "--check")
	if err != nil {
		t.Fatalf("valid typed REQ and AC citations: %v", err)
	}
	if !strings.Contains(out, "#req:totals") || !strings.Contains(out, "#ac:total-visible") {
		t.Fatalf("typed directive targets were not listed: %q", out)
	}

	bad := strings.ReplaceAll(source, "ac:total-visible", "ac:removed")
	if err := os.WriteFile(filepath.Join(root, "checkout.go"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = runCode(t, "deps", "--path=checkout.go", "--check")
	if err == nil || !strings.Contains(err.Error(), "ac:removed") || !strings.Contains(err.Error(), "missing exact heading") {
		t.Fatalf("missing AC must fail with its source target: %v", err)
	}
}

// specscore:verifies https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#ac:typed-implementation-and-test-links-checked
func TestCodeDeps_CheckRejectsInvalidTypedRelationTarget(t *testing.T) {
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatal(err)
	}
	content := "// specscore:" + "implements feature/checkout#ac:visible\n"
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	_, _, err := runCode(t, "deps", "--path=source.go", "--check")
	if err == nil || !strings.Contains(err.Error(), "implements target must be a REQ") {
		t.Fatalf("invalid typed relation target must fail: %v", err)
	}
}

// specscore:verifies https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#ac:ac-citations-accept-lint-heading-forms — runs spec lint (ac-heading-format) and code deps --check on the same tree per heading form and asserts they agree
func TestCodeDeps_CheckAgreesWithLintOnACHeadingForms(t *testing.T) {
	tests := []struct {
		heading  string
		accepted bool
	}{
		{"### AC: total-visible", true},
		{"### AC: total-visible (verifies REQ:totals)", true},
		{"### AC: total-visible (verifies checkout#req:totals)", true},
		{"###  AC: total-visible", false},
		{"### AC: total-visible  (verifies REQ:totals)", false},
		{"### AC: total-visible trailing", false},
	}
	for _, tt := range tests {
		t.Run(tt.heading, func(t *testing.T) {
			root := t.TempDir()
			config := projectdef.SpecConfig{Project: &projectdef.ProjectConfig{Host: "github.com", Org: "acme", Repo: "checkout"}}
			if err := projectdef.WriteSpecConfig(root, config); err != nil {
				t.Fatal(err)
			}
			featureDir := filepath.Join(root, "spec", "features", "checkout")
			if err := os.MkdirAll(featureDir, 0o755); err != nil {
				t.Fatal(err)
			}
			readme := "# Feature: Checkout\n\n**Status:** Draft\n\n#### REQ: totals\n\n## Acceptance Criteria\n\n" + tt.heading + "\n"
			if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte(readme), 0o644); err != nil {
				t.Fatal(err)
			}
			source := "package checkout\n\n// specscore:" + "verifies https://specscore.org/github.com/acme/checkout/spec/features/checkout#ac:total-visible\nfunc TestTotal() {}\n"
			if err := os.WriteFile(filepath.Join(root, "checkout_test.go"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			withCwd(t, root)

			lintOut, _, lintErr := runSpecLintCmd(t, "--project", root, "--rules=ac-heading-format")
			if lintAccepted := lintErr == nil; lintAccepted != tt.accepted {
				t.Fatalf("spec lint accepted=%v (err=%v, out=%q), want %v", lintAccepted, lintErr, lintOut, tt.accepted)
			}

			_, _, checkErr := runCode(t, "deps", "--path=checkout_test.go", "--check")
			if tt.accepted {
				if checkErr != nil {
					t.Fatalf("lint accepts %q but code deps --check rejects its citation: %v", tt.heading, checkErr)
				}
				return
			}
			if got := exitCodeOf(checkErr); got != exitcode.InvalidState {
				t.Fatalf("lint rejects %q but code deps --check exit = %d (err=%v), want %d", tt.heading, got, checkErr, exitcode.InvalidState)
			}
			if !strings.Contains(checkErr.Error(), "#ac:total-visible") {
				t.Fatalf("diagnostic does not name the citation: %v", checkErr)
			}
		})
	}
}
