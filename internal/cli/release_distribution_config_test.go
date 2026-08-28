package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseDistributionMacOSNotarizationContract(t *testing.T) {
	root := repositoryRoot(t)
	goreleaser := readRepositoryFile(t, root, ".goreleaser.yml")
	workflow := readRepositoryFile(t, root, ".github/workflows/release.yml")

	for _, want := range []string{
		"notarize:\n  macos:\n    - enabled: '{{ isEnvSet \"MACOS_SIGN_P12\" }}'",
		"ids: [specscore]",
		"certificate: \"{{ .Env.MACOS_SIGN_P12 }}\"",
		"password: \"{{ .Env.MACOS_SIGN_PASSWORD }}\"",
		"issuer_id: \"{{ .Env.NOTARIZE_ISSUER_ID }}\"",
		"key_id: \"{{ .Env.NOTARIZE_KEY_ID }}\"",
		"key: \"{{ .Env.NOTARIZE_KEY }}\"",
		"wait: true",
		"timeout: 20m",
	} {
		if !strings.Contains(goreleaser, want) {
			t.Errorf(".goreleaser.yml missing notarization contract %q", want)
		}
	}

	// require_notarized_macos: true is deliberately NOT asserted here yet.
	// spec/features/cli/release-distribution/README.md#req:fail-closed-macos-release-gate
	// permits merging that flip only after an operator confirms all five
	// secrets below exist as real repository secrets — that confirmation
	// hasn't happened. TestReleaseCallerDefersCaskInstallSmokeUntilNotarization
	// (workflow_contract_test.go) is the test that enforces it stays absent
	// until then; do not duplicate or contradict that guard here.
	for _, secret := range []string{
		"MACOS_SIGN_P12",
		"MACOS_SIGN_PASSWORD",
		"NOTARIZE_ISSUER_ID",
		"NOTARIZE_KEY_ID",
		"NOTARIZE_KEY",
	} {
		want := secret + ": ${{ secrets." + secret + " }}"
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow must forward %s unchanged", secret)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepositoryFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
