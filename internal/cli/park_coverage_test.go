package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

func failPostMutationLint(t *testing.T) {
	t.Helper()
	original := lintLintFn
	lintLintFn = func(lint.Options) ([]lint.Violation, error) { return nil, errors.New("injected lint failure") }
	t.Cleanup(func() { lintLintFn = original })
}

func failPostMutationLintAfterRemoving(t *testing.T, path string) {
	t.Helper()
	original := lintLintFn
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
		return nil, errors.New("injected lint failure")
	}
	t.Cleanup(func() { lintLintFn = original })
}

func forceParkNoWrite(t *testing.T, feature bool) {
	t.Helper()
	if feature {
		originalSet, originalClear := setFeatureParkedFn, clearFeatureParkedFn
		setFeatureParkedFn = func(string, string) ([]byte, bool, error) { return nil, false, nil }
		clearFeatureParkedFn = func(string) ([]byte, bool, error) { return nil, false, nil }
		t.Cleanup(func() { setFeatureParkedFn, clearFeatureParkedFn = originalSet, originalClear })
		return
	}
	originalSet, originalClear := setIdeaParkedFn, clearIdeaParkedFn
	setIdeaParkedFn = func(string, string) ([]byte, bool, error) { return nil, false, nil }
	clearIdeaParkedFn = func(string) ([]byte, bool, error) { return nil, false, nil }
	t.Cleanup(func() { setIdeaParkedFn, clearIdeaParkedFn = originalSet, originalClear })
}

func TestFeatureParkAndUnpark_FailurePaths(t *testing.T) {
	t.Run("project resolution", func(t *testing.T) {
		setupFeatureSpec(t, "Draft")
		if _, _, err := runFeature(t, "park", "auth", "--reason=x", "--project=/definitely/missing"); err == nil {
			t.Fatal("park unexpectedly resolved a missing project")
		}
		if _, _, err := runFeature(t, "unpark", "auth", "--project=/definitely/missing"); err == nil {
			t.Fatal("unpark unexpectedly resolved a missing project")
		}
	})
	t.Run("missing status", func(t *testing.T) {
		root := setupFeatureSpec(t, "Draft")
		path := filepath.Join(root, "spec", "features", "auth", "README.md")
		if err := os.WriteFile(path, []byte("# Feature: Auth\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runFeature(t, "park", "auth", "--reason=x"); err == nil {
			t.Fatal("park accepted a Feature without Status")
		}
	})
	t.Run("post mutation lint rolls back park", func(t *testing.T) {
		root := setupFeatureSpec(t, "Draft")
		path := filepath.Join(root, "spec", "features", "auth", "README.md")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		failPostMutationLint(t)
		if _, _, err := runFeature(t, "park", "auth", "--reason=x"); err == nil {
			t.Fatal("park accepted an injected lint failure")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("park rollback = %q, read err=%v", after, err)
		}
	})
	t.Run("post mutation lint rolls back unpark", func(t *testing.T) {
		root := setupFeatureSpec(t, "Draft")
		if _, stderr, err := runFeature(t, "park", "auth", "--reason=x"); err != nil {
			t.Fatalf("park: %v\nstderr=%s", err, stderr)
		}
		path := filepath.Join(root, "spec", "features", "auth", "README.md")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		failPostMutationLint(t)
		if _, _, err := runFeature(t, "unpark", "auth"); err == nil {
			t.Fatal("unpark accepted an injected lint failure")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("unpark rollback = %q, read err=%v", after, err)
		}
	})
	t.Run("rollback failure is surfaced", func(t *testing.T) {
		root := setupFeatureSpec(t, "Draft")
		path := filepath.Join(root, "spec", "features", "auth", "README.md")
		failPostMutationLintAfterRemoving(t, path)
		if _, _, err := runFeature(t, "park", "auth", "--reason=x"); err == nil || !strings.Contains(err.Error(), "rollback also failed") {
			t.Fatalf("park error = %v, want rollback failure", err)
		}
	})
	t.Run("unpark rollback failure is surfaced", func(t *testing.T) {
		root := setupFeatureSpec(t, "Draft")
		if _, stderr, err := runFeature(t, "park", "auth", "--reason=x"); err != nil {
			t.Fatalf("park: %v\nstderr=%s", err, stderr)
		}
		path := filepath.Join(root, "spec", "features", "auth", "README.md")
		failPostMutationLintAfterRemoving(t, path)
		if _, _, err := runFeature(t, "unpark", "auth"); err == nil || !strings.Contains(err.Error(), "rollback also failed") {
			t.Fatalf("unpark error = %v, want rollback failure", err)
		}
	})
}

func TestFeatureList_UnparkedFieldDoesNotEmitTrueMarker(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	out, stderr, err := runFeature(t, "list", "--fields", "parked")
	if err != nil {
		t.Fatalf("list: %v\nstderr=%s", err, stderr)
	}
	if strings.Contains(out, "parked=true") {
		t.Fatalf("unparked Feature rendered as parked: %s", out)
	}
}

func TestParkCommands_SurfaceArtifactReadFailures(t *testing.T) {
	t.Run("feature", func(t *testing.T) {
		root := setupFeatureSpec(t, "Draft")
		path := filepath.Join(root, "spec", "features", "auth", "README.md")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runFeature(t, "park", "auth", "--reason=x"); err == nil {
			t.Fatal("park accepted a README directory")
		}
		if _, _, err := runFeature(t, "unpark", "auth"); err == nil {
			t.Fatal("unpark accepted a README directory")
		}
	})
	t.Run("idea", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		path := filepath.Join(root, "spec", "ideas", "auth.md")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runIdea(t, "park", "auth", "--reason=x"); err == nil {
			t.Fatal("park accepted an Idea directory")
		}
		if _, _, err := runIdea(t, "unpark", "auth"); err == nil {
			t.Fatal("unpark accepted an Idea directory")
		}
	})
}

func TestParkCommands_RejectUnexpectedNoWrite(t *testing.T) {
	t.Run("feature", func(t *testing.T) {
		setupFeatureSpec(t, "Draft")
		forceParkNoWrite(t, true)
		for _, args := range [][]string{{"park", "auth", "--reason=x"}, {"unpark", "auth"}} {
			if _, _, err := runFeature(t, args...); err == nil || !strings.Contains(err.Error(), "no changes written") {
				t.Fatalf("feature %v = %v, want no-write error", args, err)
			}
		}
	})
	t.Run("idea", func(t *testing.T) {
		stageActiveIdea(t, "auth", "Draft", "")
		forceParkNoWrite(t, false)
		for _, args := range [][]string{{"park", "auth", "--reason=x"}, {"unpark", "auth"}} {
			if _, _, err := runIdea(t, args...); err == nil || !strings.Contains(err.Error(), "no changes written") {
				t.Fatalf("idea %v = %v, want no-write error", args, err)
			}
		}
	})
}

func TestIdeaParkAndUnpark_FailurePaths(t *testing.T) {
	t.Run("invalid slug and missing project", func(t *testing.T) {
		stageActiveIdea(t, "auth", "Draft", "")
		if _, _, err := runIdea(t, "park", "Bad Slug", "--reason=x"); err == nil {
			t.Fatal("park accepted an invalid slug")
		}
		if _, _, err := runIdea(t, "park", "auth", "--reason=x", "--project=/definitely/missing"); err == nil {
			t.Fatal("park unexpectedly resolved a missing project")
		}
		if _, _, err := runIdea(t, "unpark", "auth", "--project=/definitely/missing"); err == nil {
			t.Fatal("unpark unexpectedly resolved a missing project")
		}
	})
	t.Run("stat failures", func(t *testing.T) {
		stageActiveIdea(t, "auth", "Draft", "")
		original := statIdeaForParking
		statIdeaForParking = func(string) (os.FileInfo, error) {
			return nil, errors.New("injected stat failure")
		}
		t.Cleanup(func() { statIdeaForParking = original })
		if _, _, err := runIdea(t, "park", "auth", "--reason=x"); err == nil || !strings.Contains(err.Error(), "stat ") {
			t.Fatalf("park stat error = %v", err)
		}
		if _, _, err := runIdea(t, "unpark", "auth"); err == nil || !strings.Contains(err.Error(), "stat ") {
			t.Fatalf("unpark stat error = %v", err)
		}
	})
	t.Run("missing status", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		path := filepath.Join(root, "spec", "ideas", "auth.md")
		if err := os.WriteFile(path, []byte("# Idea: Auth\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runIdea(t, "park", "auth", "--reason=x"); err == nil {
			t.Fatal("park accepted an Idea without Status")
		}
	})
	t.Run("post mutation lint rolls back", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		path := filepath.Join(root, "spec", "ideas", "auth.md")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		failPostMutationLint(t)
		if _, _, err := runIdea(t, "park", "auth", "--reason=x"); err == nil {
			t.Fatal("park accepted an injected lint failure")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("park rollback = %q, read err=%v", after, err)
		}
	})
	t.Run("unpark restores parked content on lint failure", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		if _, stderr, err := runIdea(t, "park", "auth", "--reason=x"); err != nil {
			t.Fatalf("park: %v\nstderr=%s", err, stderr)
		}
		path := filepath.Join(root, "spec", "ideas", "auth.md")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		failPostMutationLint(t)
		if _, _, err := runIdea(t, "unpark", "auth"); err == nil {
			t.Fatal("unpark accepted an injected lint failure")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) || !strings.Contains(string(after), "**Parked:** true") {
			t.Fatalf("unpark rollback = %q, read err=%v", after, err)
		}
	})
	t.Run("rollback failure is surfaced", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		path := filepath.Join(root, "spec", "ideas", "auth.md")
		failPostMutationLintAfterRemoving(t, path)
		if _, _, err := runIdea(t, "park", "auth", "--reason=x"); err == nil || !strings.Contains(err.Error(), "rollback also failed") {
			t.Fatalf("park error = %v, want rollback failure", err)
		}
	})
	t.Run("unpark rollback failure is surfaced", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		if _, stderr, err := runIdea(t, "park", "auth", "--reason=x"); err != nil {
			t.Fatalf("park: %v\nstderr=%s", err, stderr)
		}
		path := filepath.Join(root, "spec", "ideas", "auth.md")
		failPostMutationLintAfterRemoving(t, path)
		if _, _, err := runIdea(t, "unpark", "auth"); err == nil || !strings.Contains(err.Error(), "rollback also failed") {
			t.Fatalf("unpark error = %v, want rollback failure", err)
		}
	})
}
