package publication

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// withUserConfigDir overrides the userConfigDir seam for the duration of a
// test and restores it afterward.
func withUserConfigDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := userConfigDir
	userConfigDir = fn
	t.Cleanup(func() { userConfigDir = orig })
}

// writeProjectConfig creates a valid specscore.yaml in dir with the given
// extras merged in.
func writeProjectConfig(t *testing.T, dir string, extras map[string]any) {
	t.Helper()
	cfg := projectdef.SpecConfig{Extras: extras}
	if err := projectdef.WriteSpecConfig(dir, cfg); err != nil {
		t.Fatalf("WriteSpecConfig: %v", err)
	}
}

// initGitRepo initializes a git repo in dir on the given branch with one
// commit so rev-parse resolves.
func initGitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
}

func TestNormalizeActions(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{"empty", nil, nil, false},
		{"blank tokens", []string{" , "}, nil, false},
		{"just-edit", []string{"just-edit"}, []string{}, false},
		{"edit", []string{"edit"}, []string{}, false},
		{"none", []string{"none"}, []string{}, false},
		{"stage shorthand", []string{"stage"}, []string{"stage"}, false},
		{"commit shorthand", []string{"commit"}, []string{"stage", "commit"}, false},
		{"commit-and-push", []string{"commit-and-push"}, []string{"stage", "commit", "push"}, false},
		{"commit+push", []string{"commit+push"}, []string{"stage", "commit", "push"}, false},
		{"explicit sequence", []string{"stage", "commit"}, []string{"stage", "commit"}, false},
		{"comma split", []string{"stage,commit,push"}, []string{"stage", "commit", "push"}, false},
		{"unknown token", []string{"stage", "deploy"}, nil, true},
		{"single unknown not shorthand", []string{"deploy"}, nil, true},
		{"invalid sequence order", []string{"commit", "stage"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeActions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateActions(t *testing.T) {
	valid := [][]string{
		{},
		{"stage"},
		{"stage", "commit"},
		{"stage", "commit", "push"},
	}
	for _, v := range valid {
		if err := ValidateActions(v); err != nil {
			t.Errorf("ValidateActions(%v) = %v, want nil", v, err)
		}
	}
	invalid := [][]string{
		{"commit"},
		{"push"},
		{"commit", "stage"},
		{"stage", "push"},
	}
	for _, v := range invalid {
		if err := ValidateActions(v); err == nil {
			t.Errorf("ValidateActions(%v) = nil, want error", v)
		}
	}
}

func TestTargetKey(t *testing.T) {
	tests := []struct {
		name    string
		opts    SetOptions
		want    []string
		wantErr bool
	}{
		{"default", SetOptions{Default: true}, []string{"publication", "default"}, false},
		{"default conflict", SetOptions{Default: true, Command: "c"}, nil, true},
		{"command+event", SetOptions{Command: "c", Event: "e"}, []string{"publication", "commands", "c", "events", "e"}, false},
		{"command+milestone", SetOptions{Command: "c", Milestone: "m"}, []string{"publication", "commands", "c", "milestones", "m"}, false},
		{"command only", SetOptions{Command: "c"}, []string{"publication", "commands", "c", "default"}, false},
		{"event only", SetOptions{Event: "e"}, []string{"publication", "events", "e"}, false},
		{"missing target", SetOptions{}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := targetKey(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUserConfigPath(t *testing.T) {
	withUserConfigDir(t, func() (string, error) { return "/cfg", nil })
	got, err := UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath: %v", err)
	}
	want := filepath.Join("/cfg", "specscore", UserConfigFile)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	withUserConfigDir(t, func() (string, error) { return "", errors.New("no config dir") })
	if _, err := UserConfigPath(); err == nil {
		t.Fatal("expected error from UserConfigPath")
	}
}

func TestSetPolicyProject(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, nil)

	res, err := SetPolicy(SetOptions{
		Scope:       "project",
		ProjectRoot: dir,
		Command:     "ship",
		Event:       "done",
		Actions:     []string{"commit"},
	})
	if err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if res.Scope != "project" {
		t.Errorf("scope = %q", res.Scope)
	}
	if res.Key != "publication.commands.ship.events.done" {
		t.Errorf("key = %q", res.Key)
	}
	if !reflect.DeepEqual(res.Actions, []string{"stage", "commit"}) {
		t.Errorf("actions = %v", res.Actions)
	}
	wantPath := filepath.Join(dir, projectdef.SpecConfigFile)
	if res.Path != wantPath || len(res.TouchedPaths) != 1 || res.TouchedPaths[0] != wantPath {
		t.Errorf("path = %q touched = %v", res.Path, res.TouchedPaths)
	}

	// Round-trip: the policy must be resolvable.
	resolved, err := Resolve(ResolveOptions{
		ProjectRoot: dir,
		Command:     "ship",
		Event:       "done",
		Branch:      "feature/x",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(resolved.ActionsResolved, []string{"stage", "commit"}) {
		t.Errorf("resolved actions = %v", resolved.ActionsResolved)
	}
}

func TestSetPolicyProjectExistingExtras(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, map[string]any{"other": "kept"})
	if _, err := SetPolicy(SetOptions{Scope: "project", ProjectRoot: dir, Default: true, Actions: []string{"stage"}}); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	cfg, err := projectdef.ReadSpecConfig(dir)
	if err != nil {
		t.Fatalf("ReadSpecConfig: %v", err)
	}
	if cfg.Extras["other"] != "kept" {
		t.Errorf("existing extras lost: %v", cfg.Extras)
	}
}

func TestSetPolicyProjectReadError(t *testing.T) {
	// No specscore.yaml present -> ReadSpecConfig fails.
	dir := t.TempDir()
	_, err := SetPolicy(SetOptions{Scope: "project", ProjectRoot: dir, Default: true, Actions: []string{"stage"}})
	if err == nil {
		t.Fatal("expected error for missing project config")
	}
}

func TestSetPolicyUser(t *testing.T) {
	cfgDir := t.TempDir()
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })

	res, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"commit-and-push"}})
	if err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if res.Scope != "user" || res.Key != "publication.default" {
		t.Errorf("scope = %q key = %q", res.Scope, res.Key)
	}
	if !reflect.DeepEqual(res.Actions, []string{"stage", "commit", "push"}) {
		t.Errorf("actions = %v", res.Actions)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("config not written: %v", err)
	}

	// Write a second key to exercise the read-existing-config branch.
	if _, err := SetPolicy(SetOptions{Scope: "user", Event: "e", Actions: []string{"stage"}}); err != nil {
		t.Fatalf("second SetPolicy: %v", err)
	}
	cfg, path, err := readUserConfig(res.Path)
	if err != nil {
		t.Fatalf("readUserConfig: %v", err)
	}
	if path != res.Path {
		t.Errorf("path = %q want %q", path, res.Path)
	}
	pub, _ := cfg["publication"].(map[string]any)
	if pub["default"] == nil || pub["events"] == nil {
		t.Errorf("expected both keys present: %v", cfg)
	}
}

func TestSetPolicyUserConfigPathError(t *testing.T) {
	withUserConfigDir(t, func() (string, error) { return "", errors.New("boom") })
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected error when UserConfigPath fails")
	}
}

func TestSetUserPolicyReadExistingError(t *testing.T) {
	// Existing user config is malformed -> readUserConfig (within
	// setUserPolicy) returns an error.
	cfgDir := t.TempDir()
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
	dir := filepath.Join(cfgDir, "specscore")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, UserConfigFile), []byte("::bad::\n  - ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected read error for malformed existing config")
	}
}

func TestSetUserPolicyMkdirError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits do not block MkdirAll")
	}
	// The config dir does not yet contain the specscore/ subdir, so
	// readUserConfig sees ErrNotExist and returns an empty config; the
	// dir is read-only so MkdirAll of the specscore/ subdir then fails.
	cfgDir := t.TempDir()
	if err := os.Chmod(cfgDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o755) })
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected MkdirAll error")
	}
}

func TestSetUserPolicyWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits do not block WriteFile")
	}
	// Pre-create the specscore/ dir (so MkdirAll succeeds) but make it
	// read-only so the final WriteFile of config.yaml fails. The config
	// file itself is absent, so readUserConfig returns empty cleanly.
	cfgDir := t.TempDir()
	specDir := filepath.Join(cfgDir, "specscore")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(specDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(specDir, 0o755) })
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected WriteFile error on read-only dir")
	}
}

func TestSetProjectPolicyWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits do not block WriteFile")
	}
	// Read a valid config, then make the specscore.yaml file read-only so
	// the write-back (which truncates the existing file) fails.
	dir := t.TempDir()
	writeProjectConfig(t, dir, nil)
	cfgPath := filepath.Join(dir, projectdef.SpecConfigFile)
	if err := os.Chmod(cfgPath, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgPath, 0o644) })
	_, err := SetPolicy(SetOptions{Scope: "project", ProjectRoot: dir, Default: true, Actions: []string{"stage"}})
	if err == nil {
		t.Fatal("expected WriteSpecConfig error on read-only file")
	}
}

func TestResolveCommandMilestone(t *testing.T) {
	projDir := t.TempDir()
	writeProjectConfig(t, projDir, nil)
	if _, err := SetPolicy(SetOptions{Scope: "project", ProjectRoot: projDir, Command: "ship", Milestone: "rc", Actions: []string{"commit"}}); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	res, err := Resolve(ResolveOptions{ProjectRoot: projDir, Command: "ship", Milestone: "rc", Branch: "feature/x"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(res.ActionsResolved, []string{"stage", "commit"}) {
		t.Errorf("resolved = %v", res.ActionsResolved)
	}
}

func TestSetUserPolicyMarshalError(t *testing.T) {
	cfgDir := t.TempDir()
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
	orig := yamlMarshal
	yamlMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { yamlMarshal = orig })
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestSetPolicyInvalidScope(t *testing.T) {
	if _, err := SetPolicy(SetOptions{Scope: "bogus", Default: true, Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected invalid scope error")
	}
}

func TestSetPolicyNormalizeError(t *testing.T) {
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"deploy"}}); err == nil {
		t.Fatal("expected normalize error")
	}
}

func TestSetPolicyTargetKeyError(t *testing.T) {
	if _, err := SetPolicy(SetOptions{Scope: "user", Actions: []string{"stage"}}); err == nil {
		t.Fatal("expected target key error (no target)")
	}
}

func TestReadUserConfig(t *testing.T) {
	t.Run("missing file returns empty", func(t *testing.T) {
		cfg, path, err := readUserConfig(filepath.Join(t.TempDir(), "nope.yaml"))
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(cfg) != 0 || path == "" {
			t.Errorf("cfg = %v path = %q", cfg, path)
		}
	})

	t.Run("override empty uses UserConfigPath", func(t *testing.T) {
		cfgDir := t.TempDir()
		withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
		_, path, err := readUserConfig("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := filepath.Join(cfgDir, "specscore", UserConfigFile)
		if path != want {
			t.Errorf("path = %q want %q", path, want)
		}
	})

	t.Run("override empty UserConfigPath error", func(t *testing.T) {
		withUserConfigDir(t, func() (string, error) { return "", errors.New("boom") })
		if _, _, err := readUserConfig(""); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "bad.yaml")
		if err := os.WriteFile(p, []byte("::not yaml::\n  - ["), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readUserConfig(p); err == nil {
			t.Fatal("expected yaml error")
		}
	})

	t.Run("empty file yields empty map", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "empty.yaml")
		if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, _, err := readUserConfig(p)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if cfg == nil || len(cfg) != 0 {
			t.Errorf("cfg = %v", cfg)
		}
	})

	t.Run("read error other than not-exist", func(t *testing.T) {
		// A directory path produces a non-ErrNotExist read error.
		d := t.TempDir()
		if _, _, err := readUserConfig(d); err == nil {
			t.Fatal("expected read error for directory path")
		}
	})
}

func TestCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, "feature/cb")
	got, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if got != "feature/cb" {
		t.Errorf("branch = %q", got)
	}

	// Non-git directory -> error.
	if _, err := CurrentBranch(filepath.Join(t.TempDir(), "not-a-repo")); err == nil {
		t.Fatal("expected error for non-git dir")
	}
}

func TestCheckBranch(t *testing.T) {
	tests := []struct {
		name        string
		branch      string
		policy      BranchPolicy
		wantAllowed bool
		reasonHas   string
	}{
		{"empty branch", "", BranchPolicy{}, false, "missing branch"},
		{"detached head", "HEAD", BranchPolicy{}, false, "detached HEAD"},
		{"deny match", "main", BranchPolicy{DenyBranches: []string{"main"}}, false, "denied by pattern"},
		{"deny pattern", "release/1", BranchPolicy{DenyBranches: []string{"release/*"}}, false, "denied by pattern"},
		{"allow match", "feature/x", BranchPolicy{AllowBranches: []string{"feature/*"}}, true, ""},
		{"allow no match", "hotfix/x", BranchPolicy{AllowBranches: []string{"feature/*"}}, false, "does not match allowed"},
		{"no policy allowed", "anything", BranchPolicy{}, true, ""},
		{"deny then allow precedence", "main", BranchPolicy{DenyBranches: []string{"main"}, AllowBranches: []string{"main"}}, false, "denied by pattern"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := CheckBranch(tt.branch, tt.policy)
			if res.Allowed != tt.wantAllowed {
				t.Errorf("allowed = %v want %v (reason %q)", res.Allowed, tt.wantAllowed, res.Reason)
			}
			if tt.reasonHas != "" && !strings.Contains(res.Reason, tt.reasonHas) {
				t.Errorf("reason = %q want contains %q", res.Reason, tt.reasonHas)
			}
		})
	}
}

func TestResolveTaskAndSessionPolicy(t *testing.T) {
	// Session policy has highest priority and should win.
	res, err := Resolve(ResolveOptions{
		TaskPolicy:    []string{"commit"},
		SessionPolicy: []string{"stage"},
		Branch:        "feature/x",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(res.ActionsResolved, []string{"stage"}) {
		t.Errorf("resolved = %v, want session policy to win", res.ActionsResolved)
	}
	if len(res.PolicySources) == 0 || res.PolicySources[0].Scope != "session" {
		t.Errorf("expected session source first: %v", res.PolicySources)
	}
}

func TestResolveTaskPolicyError(t *testing.T) {
	if _, err := Resolve(ResolveOptions{TaskPolicy: []string{"deploy"}}); err == nil {
		t.Fatal("expected task policy error")
	}
}

func TestResolveSessionPolicyError(t *testing.T) {
	if _, err := Resolve(ResolveOptions{SessionPolicy: []string{"deploy"}}); err == nil {
		t.Fatal("expected session policy error")
	}
}

func TestResolvePushBlockedOnDeniedBranch(t *testing.T) {
	res, err := Resolve(ResolveOptions{
		SessionPolicy: []string{"commit-and-push"},
		Branch:        "main", // denied by builtin defaults
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if containsAction(res.ActionsAllowed, "push") {
		t.Errorf("push should be blocked, allowed = %v", res.ActionsAllowed)
	}
	if len(res.ActionsBlocked) != 1 || res.ActionsBlocked[0].Action != "push" {
		t.Errorf("expected push blocked entry, got %v", res.ActionsBlocked)
	}
	if res.BranchPushAllowed {
		t.Errorf("BranchPushAllowed should be false for main")
	}
}

func TestResolvePushAllowedOnFeatureBranch(t *testing.T) {
	res, err := Resolve(ResolveOptions{
		SessionPolicy: []string{"commit-and-push"},
		Branch:        "feature/x",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !containsAction(res.ActionsAllowed, "push") {
		t.Errorf("push should be allowed, allowed = %v", res.ActionsAllowed)
	}
	if len(res.ActionsBlocked) != 0 {
		t.Errorf("no blocked actions expected, got %v", res.ActionsBlocked)
	}
}

func TestResolveBranchFromGit(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, nil)
	initGitRepo(t, dir, "feature/from-git")
	res, err := Resolve(ResolveOptions{ProjectRoot: dir})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Branch != "feature/from-git" {
		t.Errorf("branch = %q, want feature/from-git", res.Branch)
	}
}

func TestResolveUserAndProjectConfigPolicies(t *testing.T) {
	// User config: default policy. Project config: more specific command
	// default which should outrank the user default.
	cfgDir := t.TempDir()
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
	if _, err := SetPolicy(SetOptions{Scope: "user", Default: true, Actions: []string{"stage"}}); err != nil {
		t.Fatalf("user SetPolicy: %v", err)
	}

	projDir := t.TempDir()
	writeProjectConfig(t, projDir, nil)
	if _, err := SetPolicy(SetOptions{Scope: "project", ProjectRoot: projDir, Command: "ship", Actions: []string{"commit"}}); err != nil {
		t.Fatalf("project SetPolicy: %v", err)
	}

	res, err := Resolve(ResolveOptions{ProjectRoot: projDir, Command: "ship", Branch: "feature/x"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(res.ActionsResolved, []string{"stage", "commit"}) {
		t.Errorf("resolved = %v, want project command policy", res.ActionsResolved)
	}
}

func TestResolveBranchPolicyFromConfigs(t *testing.T) {
	// Project deny_branches replaces builtin defaults; user allow_branches
	// applies. main is no longer denied; only "trunk" is.
	projDir := t.TempDir()
	writeProjectConfig(t, projDir, map[string]any{
		"publication": map[string]any{
			"push": map[string]any{
				"deny_branches": []any{"trunk"},
			},
		},
	})
	cfgDir := t.TempDir()
	withUserConfigDir(t, func() (string, error) { return cfgDir, nil })
	if err := os.MkdirAll(filepath.Join(cfgDir, "specscore"), 0o755); err != nil {
		t.Fatal(err)
	}
	userPath := filepath.Join(cfgDir, "specscore", UserConfigFile)
	body := "publication:\n  push:\n    allow_branches:\n      - feature/*\n"
	if err := os.WriteFile(userPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// main allowed now (not denied, and matches no allow pattern -> blocked
	// by allow list actually). feature/x allowed.
	resFeature, err := Resolve(ResolveOptions{ProjectRoot: projDir, SessionPolicy: []string{"commit-and-push"}, Branch: "feature/x"})
	if err != nil {
		t.Fatalf("Resolve feature: %v", err)
	}
	if !resFeature.BranchPushAllowed {
		t.Errorf("feature/x should be push-allowed")
	}

	// trunk denied by project policy.
	resTrunk, err := Resolve(ResolveOptions{ProjectRoot: projDir, SessionPolicy: []string{"commit-and-push"}, Branch: "trunk"})
	if err != nil {
		t.Fatalf("Resolve trunk: %v", err)
	}
	if resTrunk.BranchPushAllowed {
		t.Errorf("trunk should be denied")
	}

	// main not in deny list but not in allow list -> blocked by allow gate.
	resMain, err := Resolve(ResolveOptions{ProjectRoot: projDir, SessionPolicy: []string{"commit-and-push"}, Branch: "main"})
	if err != nil {
		t.Fatalf("Resolve main: %v", err)
	}
	if resMain.BranchPushAllowed {
		t.Errorf("main should be blocked by allow gate")
	}
}

func TestResolveUserConfigError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(p, []byte("::bad::\n  - ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(ResolveOptions{UserConfigPath: p}); err == nil {
		t.Fatal("expected user config read error")
	}
}

func TestResolveProjectConfigError(t *testing.T) {
	// A specscore.yaml with a missing schema header makes ReadSpecConfig
	// fail with a non-ErrNotExist error.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, projectdef.SpecConfigFile), []byte("project:\n  title: T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(ResolveOptions{ProjectRoot: dir}); err == nil {
		t.Fatal("expected project config read error")
	}
}

func TestReadProjectConfig(t *testing.T) {
	t.Run("empty root", func(t *testing.T) {
		cfg, path, err := readProjectConfig("")
		if err != nil || path != "" || len(cfg) != 0 {
			t.Fatalf("cfg=%v path=%q err=%v", cfg, path, err)
		}
	})

	t.Run("missing file returns empty", func(t *testing.T) {
		dir := t.TempDir()
		cfg, path, err := readProjectConfig(dir)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(cfg) != 0 || path == "" {
			t.Errorf("cfg=%v path=%q", cfg, path)
		}
	})

	t.Run("with extras", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectConfig(t, dir, map[string]any{"k": "v"})
		cfg, _, err := readProjectConfig(dir)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if cfg["k"] != "v" {
			t.Errorf("extras not propagated: %v", cfg)
		}
	})

	t.Run("read error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, projectdef.SpecConfigFile), []byte("bad: header\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readProjectConfig(dir); err == nil {
			t.Fatal("expected read error")
		}
	})
}

func TestStringSlice(t *testing.T) {
	if got, ok := stringSlice([]string{"a"}); !ok || !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("[]string case: %v %v", got, ok)
	}
	if got, ok := stringSlice([]any{"a", "b"}); !ok || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("[]any case: %v %v", got, ok)
	}
	if _, ok := stringSlice([]any{"a", 1}); ok {
		t.Error("[]any with non-string should fail")
	}
	if _, ok := stringSlice(42); ok {
		t.Error("non-slice should fail")
	}
}

func TestPolicyActionsAt(t *testing.T) {
	root := map[string]any{
		"publication": map[string]any{
			"default":    map[string]any{"actions": []any{"stage"}},
			"noactions":  map[string]any{},
			"badactions": map[string]any{"actions": []any{"deploy"}},
			"wrongtype":  map[string]any{"actions": 5},
			"notamap":    "x",
		},
	}
	if got, ok := policyActionsAt(root, []string{"publication", "default"}); !ok || !reflect.DeepEqual(got, []string{"stage"}) {
		t.Errorf("default: %v %v", got, ok)
	}
	if got, ok := policyActionsAt(root, []string{"publication", "noactions"}); !ok || len(got) != 0 {
		t.Errorf("noactions should be empty actions: %v %v", got, ok)
	}
	if _, ok := policyActionsAt(root, []string{"publication", "badactions"}); ok {
		t.Error("badactions should fail ValidateActions")
	}
	if _, ok := policyActionsAt(root, []string{"publication", "wrongtype"}); ok {
		t.Error("wrongtype actions should fail")
	}
	if _, ok := policyActionsAt(root, []string{"publication", "notamap"}); ok {
		t.Error("leaf not a map should fail")
	}
	if _, ok := policyActionsAt(root, []string{"missing", "key"}); ok {
		t.Error("missing intermediate key should fail")
	}
	if _, ok := policyActionsAt(root, []string{"publication", "notamap", "deeper"}); ok {
		t.Error("descending into a non-map intermediate should fail")
	}
}

func TestMapAt(t *testing.T) {
	root := map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}}
	if m, ok := mapAt(root, []string{"a", "b"}); !ok || m["c"] != 1 {
		t.Errorf("nested map: %v %v", m, ok)
	}
	if _, ok := mapAt(root, []string{"a", "missing"}); ok {
		t.Error("missing key should fail")
	}
	if _, ok := mapAt(root, []string{"a", "b", "c"}); ok {
		t.Error("leaf not map should fail")
	}
	if _, ok := mapAt(root, []string{"a", "b", "c", "d"}); ok {
		t.Error("descend past non-map should fail")
	}
}

func TestBranchPatternMatch(t *testing.T) {
	tests := []struct {
		pattern, branch string
		want            bool
	}{
		{"main", "main", true},
		{"main", "dev", false},
		{"release/*", "release/1.0", true},
		{"release/*", "feature/1.0", false},
		{"release/*", "release/1/0", false}, // different segment count
		{"a/b", "a/b/c", false},
		{"*", "anything", true},
	}
	for _, tt := range tests {
		if got := branchPatternMatch(tt.pattern, tt.branch); got != tt.want {
			t.Errorf("branchPatternMatch(%q,%q)=%v want %v", tt.pattern, tt.branch, got, tt.want)
		}
	}
}

func TestContainsAndRemoveAction(t *testing.T) {
	if !containsAction([]string{"a", "push"}, "push") {
		t.Error("containsAction should be true")
	}
	if containsAction([]string{"a"}, "push") {
		t.Error("containsAction should be false")
	}
	got := removeAction([]string{"stage", "push", "commit"}, "push")
	if reflect.DeepEqual(got, []string{"stage", "commit"}) == false {
		t.Errorf("removeAction = %v", got)
	}
}

func TestSetNestedPolicy(t *testing.T) {
	root := map[string]any{"publication": map[string]any{"existing": "keep"}}
	setNestedPolicy(root, []string{"publication", "default"}, []string{"stage"})
	pub := root["publication"].(map[string]any)
	if pub["existing"] != "keep" {
		t.Error("existing key should be preserved")
	}
	leaf := pub["default"].(map[string]any)
	if !reflect.DeepEqual(leaf["actions"], []string{"stage"}) {
		t.Errorf("leaf actions = %v", leaf["actions"])
	}

	// Creates intermediate maps when absent.
	root2 := map[string]any{}
	setNestedPolicy(root2, []string{"a", "b", "c"}, []string{})
	a := root2["a"].(map[string]any)
	b := a["b"].(map[string]any)
	c := b["c"].(map[string]any)
	if _, ok := c["actions"]; !ok {
		t.Error("actions not set on deep leaf")
	}
}

func TestBranchPolicyFromConfigsDefaults(t *testing.T) {
	// No config -> builtin deny defaults present.
	policy := branchPolicyFromConfigs(map[string]any{}, map[string]any{})
	if !reflect.DeepEqual(policy.DenyBranches, defaultDenyBranches) {
		t.Errorf("deny = %v want builtin defaults", policy.DenyBranches)
	}
	found := false
	for _, s := range policy.Sources {
		if s.Scope == "builtin" {
			found = true
		}
	}
	if !found {
		t.Error("expected builtin source")
	}
}
