package cli

import (
	"net"
	"os"
	"testing"
)

// TestMain makes the CLI test suite offline-by-default for template fetches.
//
// The `… new` scaffolders fetch their template from the gallery
// (cli-template-runtime-fetch); pointing SPECSCORE_TEMPLATE_BASE_URL at a
// closed local port makes every bare scaffold fail fast (connection refused)
// and fall back to the embedded scaffolder — the pre-fetch behaviour the
// existing tests assert. Tests that exercise the fetch path override
// SPECSCORE_TEMPLATE_BASE_URL themselves via t.Setenv.
func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("SPECSCORE_TEMPLATE_BASE_URL"); !ok {
		base := "http://127.0.0.1:0"
		// Reserve then immediately release a port so connections to it are
		// refused promptly rather than timing out.
		if l, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
			base = "http://" + l.Addr().String()
			_ = l.Close()
		}
		_ = os.Setenv("SPECSCORE_TEMPLATE_BASE_URL", base)
	}
	// Make `agent setup` skill-copying offline-by-default: stub the bundle
	// download to return nothing, so instruction-file tests don't hit GitHub.
	// Tests exercising skill copying override fetchSkillBundleFn themselves.
	fetchSkillBundleFn = func() ([]skillFile, error) { return nil, nil }
	os.Exit(m.Run())
}
