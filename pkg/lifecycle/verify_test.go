package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeArtifact(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return path
}

const mirroredArtifact = "---\nformat: https://specscore.md/idea-specification\nstatus: %s\n---\n\n# Idea: X\n\n**Status:** %s\n**Owner:** tester\n\n## Problem Statement\n\nWhy.\n"

func TestVerifyPersistedStatus_AgreeingSurfacesPass(t *testing.T) {
	path := writeArtifact(t, strings.ReplaceAll(mirroredArtifact, "%s", "Approved"))
	if err := VerifyPersistedStatus(path, IdeaApproved); err != nil {
		t.Fatalf("VerifyPersistedStatus = %v; want nil", err)
	}
}

// The reported defect: the transition wrote Specifying, the post-mutation
// index sync rewrote both surfaces back to Approved, and the verb still
// printed its success line. Verification MUST catch that.
func TestVerifyPersistedStatus_RevertedBodyIsCaught(t *testing.T) {
	path := writeArtifact(t, strings.ReplaceAll(mirroredArtifact, "%s", "Approved"))

	err := VerifyPersistedStatus(path, IdeaSpecifying)
	var notPersisted *StatusNotPersistedError
	if !errors.As(err, &notPersisted) {
		t.Fatalf("VerifyPersistedStatus = %v; want *StatusNotPersistedError", err)
	}
	if notPersisted.Want != IdeaSpecifying || notPersisted.GotBody != IdeaApproved {
		t.Errorf("error = %+v; want Want=Specifying GotBody=Approved", notPersisted)
	}
	for _, want := range []string{"did not persist", "Specifying", "Approved"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q does not mention %q", err.Error(), want)
		}
	}
}

// A half-written transition — body advanced, frontmatter mirror left stale —
// is the same defect class and MUST also fail, naming the mirror.
func TestVerifyPersistedStatus_StaleFrontmatterMirrorIsCaught(t *testing.T) {
	content := strings.Replace(strings.ReplaceAll(mirroredArtifact, "%s", "Approved"),
		"**Status:** Approved", "**Status:** Specifying", 1)
	path := writeArtifact(t, content)

	err := VerifyPersistedStatus(path, IdeaSpecifying)
	var notPersisted *StatusNotPersistedError
	if !errors.As(err, &notPersisted) {
		t.Fatalf("VerifyPersistedStatus = %v; want *StatusNotPersistedError", err)
	}
	if notPersisted.GotFrontmatter != "Approved" {
		t.Errorf("GotFrontmatter = %q; want %q", notPersisted.GotFrontmatter, "Approved")
	}
	if !strings.Contains(err.Error(), "frontmatter status") {
		t.Errorf("message %q does not name the frontmatter mirror", err.Error())
	}
}

// An artifact with no frontmatter block is verified on its body alone — the
// mirror half of the check MUST NOT invent a failure.
func TestVerifyPersistedStatus_NoFrontmatterUsesBodyOnly(t *testing.T) {
	path := writeArtifact(t, "# Feature: X\n\n**Status:** Stable\n\n## Summary\n\nY.\n")
	if err := VerifyPersistedStatus(path, FeatureStable); err != nil {
		t.Fatalf("VerifyPersistedStatus = %v; want nil", err)
	}
	body, frontmatter, has, err := PersistedStatus(path)
	if err != nil {
		t.Fatalf("PersistedStatus: %v", err)
	}
	if body != FeatureStable || has || frontmatter != "" {
		t.Errorf("PersistedStatus = (%q, %q, %v); want (Stable, \"\", false)", body, frontmatter, has)
	}
}

func TestVerifyPersistedStatus_MissingStatusLine(t *testing.T) {
	path := writeArtifact(t, "# Feature: X\n\n## Summary\n\nNo status here.\n")
	if err := VerifyPersistedStatus(path, FeatureStable); !errors.Is(err, ErrStatusLineNotFound) {
		t.Fatalf("VerifyPersistedStatus = %v; want ErrStatusLineNotFound", err)
	}
}

func TestVerifyPersistedStatus_UnreadableArtifact(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	if err := VerifyPersistedStatus(missing, FeatureStable); !os.IsNotExist(err) {
		t.Fatalf("VerifyPersistedStatus = %v; want a not-exist error", err)
	}
}
