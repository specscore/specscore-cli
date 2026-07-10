package bash

// In-package tests exercising the failure paths behind the test seams.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// swap replaces a package-level seam for one test.
func swap[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	saved := *target
	*target = replacement
	t.Cleanup(func() { *target = saved })
}

func TestRun_CreateCapturesFileFailureIsStepFail(t *testing.T) {
	swap(t, &createTempFn, func(string, string) (*os.File, error) {
		return nil, errors.New("disk full")
	})
	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "true"})
	if res.Status != blocks.StatusFail {
		t.Fatalf("status = %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "creating $REHEARSE_CAPTURES file: disk full") {
		t.Errorf("detail = %q", res.Detail)
	}
}

func TestRun_ReadCapturesFileFailureIsStepFail(t *testing.T) {
	swap(t, &readFileFn, func(string) ([]byte, error) {
		return nil, errors.New("gone")
	})
	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "true"})
	if res.Status != blocks.StatusFail {
		t.Fatalf("status = %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "reading $REHEARSE_CAPTURES file: gone") {
		t.Errorf("detail = %q", res.Detail)
	}
}
