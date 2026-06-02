// Package publication implements durable publication policy config helpers.
package publication

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/projectdef"
	"gopkg.in/yaml.v3"
)

const UserConfigFile = "config.yaml"

var userConfigDir = os.UserConfigDir

var yamlMarshal = yaml.Marshal

var defaultDenyBranches = []string{"main", "master", "release/*"}

type Policy struct {
	Actions []string `json:"actions" yaml:"actions"`
}

type BlockedAction struct {
	Action string `json:"action" yaml:"action"`
	Reason string `json:"reason" yaml:"reason"`
}

type PolicySource struct {
	Scope   string   `json:"scope" yaml:"scope"`
	Path    string   `json:"path" yaml:"path"`
	Key     string   `json:"key" yaml:"key"`
	Actions []string `json:"actions,omitempty" yaml:"actions,omitempty"`
}

type ResolveResult struct {
	ActionsResolved   []string        `json:"actions_resolved" yaml:"actions_resolved"`
	ActionsAllowed    []string        `json:"actions_allowed" yaml:"actions_allowed"`
	ActionsBlocked    []BlockedAction `json:"actions_blocked" yaml:"actions_blocked"`
	PolicySources     []PolicySource  `json:"policy_sources" yaml:"policy_sources"`
	Branch            string          `json:"branch" yaml:"branch"`
	BranchPushAllowed bool            `json:"branch_push_allowed" yaml:"branch_push_allowed"`
	BranchBlockReason string          `json:"branch_block_reason" yaml:"branch_block_reason"`
}

type SetOptions struct {
	Scope       string
	ProjectRoot string
	Command     string
	Event       string
	Milestone   string
	Default     bool
	Actions     []string
}

type SetResult struct {
	Scope        string   `json:"scope" yaml:"scope"`
	Path         string   `json:"path" yaml:"path"`
	Key          string   `json:"key" yaml:"key"`
	Actions      []string `json:"actions" yaml:"actions"`
	TouchedPaths []string `json:"touched_paths" yaml:"touched_paths"`
}

type ResolveOptions struct {
	ProjectRoot    string
	UserConfigPath string
	Command        string
	Event          string
	Milestone      string
	TaskPolicy     []string
	SessionPolicy  []string
	Branch         string
}

func NormalizeActions(input []string) ([]string, error) {
	tokens := splitActionTokens(input)
	if len(tokens) == 0 {
		return nil, nil
	}
	if len(tokens) == 1 {
		switch strings.ToLower(tokens[0]) {
		case "just-edit", "just edit", "edit", "none":
			return []string{}, nil
		case "stage":
			return []string{"stage"}, nil
		case "commit":
			return []string{"stage", "commit"}, nil
		case "commit-and-push", "commit & push", "commit+push":
			return []string{"stage", "commit", "push"}, nil
		}
	}
	for _, tok := range tokens {
		if tok != "stage" && tok != "commit" && tok != "push" {
			return nil, fmt.Errorf("unknown publication action %q; expected stage, commit, push, or shorthand just-edit|stage|commit|commit-and-push", tok)
		}
	}
	if err := ValidateActions(tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

func ValidateActions(actions []string) error {
	want := [][]string{
		{},
		{"stage"},
		{"stage", "commit"},
		{"stage", "commit", "push"},
	}
	for _, valid := range want {
		if reflect.DeepEqual(actions, valid) {
			return nil
		}
	}
	return fmt.Errorf("invalid publication action sequence %v; commit requires stage and push requires stage, commit", actions)
}

func SetPolicy(opts SetOptions) (SetResult, error) {
	actions, err := NormalizeActions(opts.Actions)
	if err != nil {
		return SetResult{}, err
	}
	key, err := targetKey(opts)
	if err != nil {
		return SetResult{}, err
	}
	switch opts.Scope {
	case "project":
		return setProjectPolicy(opts.ProjectRoot, key, actions)
	case "user":
		return setUserPolicy(key, actions)
	default:
		return SetResult{}, fmt.Errorf("invalid scope %q; expected user or project", opts.Scope)
	}
}

func Resolve(opts ResolveOptions) (ResolveResult, error) {
	result := ResolveResult{
		ActionsResolved: []string{},
		ActionsAllowed:  []string{},
		ActionsBlocked:  []BlockedAction{},
		PolicySources:   []PolicySource{},
	}
	userConfig, userPath, err := readUserConfig(opts.UserConfigPath)
	if err != nil {
		return result, err
	}
	projectConfig, projectPath, err := readProjectConfig(opts.ProjectRoot)
	if err != nil {
		return result, err
	}

	candidates := []policyCandidate{}
	candidates = append(candidates, candidatesFromConfig("user", userPath, userConfig, opts)...)
	candidates = append(candidates, candidatesFromConfig("project", projectPath, projectConfig, opts)...)
	if len(opts.TaskPolicy) > 0 {
		actions, err := NormalizeActions(opts.TaskPolicy)
		if err != nil {
			return result, fmt.Errorf("task policy: %w", err)
		}
		candidates = append(candidates, policyCandidate{source: PolicySource{Scope: "task", Key: "task", Actions: actions}, actions: actions, priority: 300})
	}
	if len(opts.SessionPolicy) > 0 {
		actions, err := NormalizeActions(opts.SessionPolicy)
		if err != nil {
			return result, fmt.Errorf("session policy: %w", err)
		}
		candidates = append(candidates, policyCandidate{source: PolicySource{Scope: "session", Key: "session", Actions: actions}, actions: actions, priority: 400})
	}

	if len(candidates) > 0 {
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].priority > candidates[j].priority })
		result.ActionsResolved = append([]string(nil), candidates[0].actions...)
		result.PolicySources = append(result.PolicySources, candidates[0].source)
	}

	branch := strings.TrimSpace(opts.Branch)
	if branch == "" && opts.ProjectRoot != "" {
		branch, _ = CurrentBranch(opts.ProjectRoot)
	}
	result.Branch = branch
	branchCheck := CheckBranch(branch, branchPolicyFromConfigs(userConfig, projectConfig))
	result.BranchPushAllowed = branchCheck.Allowed
	result.BranchBlockReason = branchCheck.Reason
	result.PolicySources = append(result.PolicySources, branchCheck.Sources...)

	result.ActionsAllowed = append([]string(nil), result.ActionsResolved...)
	if containsAction(result.ActionsResolved, "push") && !branchCheck.Allowed {
		result.ActionsAllowed = removeAction(result.ActionsAllowed, "push")
		result.ActionsBlocked = append(result.ActionsBlocked, BlockedAction{Action: "push", Reason: branchCheck.Reason})
	}
	return result, nil
}

type BranchPolicy struct {
	AllowBranches []string
	DenyBranches  []string
	Sources       []PolicySource
}

type BranchCheckResult struct {
	Branch  string         `json:"branch" yaml:"branch"`
	Allowed bool           `json:"branch_push_allowed" yaml:"branch_push_allowed"`
	Reason  string         `json:"branch_block_reason" yaml:"branch_block_reason"`
	Sources []PolicySource `json:"policy_sources" yaml:"policy_sources"`
}

func CheckBranch(branch string, policy BranchPolicy) BranchCheckResult {
	result := BranchCheckResult{Branch: branch, Allowed: true, Sources: policy.Sources}
	if strings.TrimSpace(branch) == "" {
		result.Allowed = false
		result.Reason = "missing branch"
		return result
	}
	if branch == "HEAD" {
		result.Allowed = false
		result.Reason = "detached HEAD"
		return result
	}
	for _, pattern := range policy.DenyBranches {
		if branchPatternMatch(pattern, branch) {
			result.Allowed = false
			result.Reason = fmt.Sprintf("branch %q denied by pattern %q", branch, pattern)
			return result
		}
	}
	if len(policy.AllowBranches) > 0 {
		for _, pattern := range policy.AllowBranches {
			if branchPatternMatch(pattern, branch) {
				return result
			}
		}
		result.Allowed = false
		result.Reason = fmt.Sprintf("branch %q does not match allowed branch patterns", branch)
	}
	return result
}

func CurrentBranch(projectRoot string) (string, error) {
	cmd := exec.Command("git", "-C", projectRoot, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func UserConfigPath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "specscore", UserConfigFile), nil
}

type policyCandidate struct {
	source   PolicySource
	actions  []string
	priority int
}

func splitActionTokens(input []string) []string {
	var out []string
	for _, raw := range input {
		for _, part := range strings.Split(raw, ",") {
			tok := strings.TrimSpace(part)
			if tok != "" {
				out = append(out, strings.ToLower(tok))
			}
		}
	}
	return out
}

func targetKey(opts SetOptions) ([]string, error) {
	switch {
	case opts.Default:
		if opts.Command != "" || opts.Event != "" || opts.Milestone != "" {
			return nil, errors.New("--default cannot be combined with --command, --event, or --milestone")
		}
		return []string{"publication", "default"}, nil
	case opts.Command != "" && opts.Event != "":
		return []string{"publication", "commands", opts.Command, "events", opts.Event}, nil
	case opts.Command != "" && opts.Milestone != "":
		return []string{"publication", "commands", opts.Command, "milestones", opts.Milestone}, nil
	case opts.Command != "":
		return []string{"publication", "commands", opts.Command, "default"}, nil
	case opts.Event != "":
		return []string{"publication", "events", opts.Event}, nil
	default:
		return nil, errors.New("missing policy target; pass --default, --event, or --command")
	}
}

func setProjectPolicy(projectRoot string, key []string, actions []string) (SetResult, error) {
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		return SetResult{}, err
	}
	if cfg.Extras == nil {
		cfg.Extras = map[string]any{}
	}
	setNestedPolicy(cfg.Extras, key, actions)
	if err := projectdef.WriteSpecConfig(projectRoot, cfg); err != nil {
		return SetResult{}, err
	}
	path := filepath.Join(projectRoot, projectdef.SpecConfigFile)
	return SetResult{Scope: "project", Path: path, Key: strings.Join(key, "."), Actions: actions, TouchedPaths: []string{path}}, nil
}

func setUserPolicy(key []string, actions []string) (SetResult, error) {
	path, err := UserConfigPath()
	if err != nil {
		return SetResult{}, err
	}
	cfg, _, err := readUserConfig(path)
	if err != nil {
		return SetResult{}, err
	}
	setNestedPolicy(cfg, key, actions)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SetResult{}, err
	}
	body, err := yamlMarshal(cfg)
	if err != nil {
		return SetResult{}, err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return SetResult{}, err
	}
	return SetResult{Scope: "user", Path: path, Key: strings.Join(key, "."), Actions: actions, TouchedPaths: []string{path}}, nil
}

func setNestedPolicy(root map[string]any, key []string, actions []string) {
	current := root
	for _, part := range key[:len(key)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[key[len(key)-1]] = map[string]any{"actions": actions}
}

func readUserConfig(override string) (map[string]any, string, error) {
	path := override
	if path == "" {
		var err error
		path, err = UserConfigPath()
		if err != nil {
			return nil, "", err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, path, nil
		}
		return nil, path, err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, path, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, path, nil
}

func readProjectConfig(projectRoot string) (map[string]any, string, error) {
	if projectRoot == "" {
		return map[string]any{}, "", nil
	}
	path := filepath.Join(projectRoot, projectdef.SpecConfigFile)
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, path, nil
		}
		return nil, path, err
	}
	root := map[string]any{}
	if cfg.Extras != nil {
		for k, v := range cfg.Extras {
			root[k] = v
		}
	}
	return root, path, nil
}

func candidatesFromConfig(scope, path string, cfg map[string]any, opts ResolveOptions) []policyCandidate {
	priorBase := 0
	if scope == "project" {
		priorBase = 100
	}
	paths := candidatePaths(opts)
	var out []policyCandidate
	for _, candidate := range paths {
		if actions, ok := policyActionsAt(cfg, candidate.parts); ok {
			out = append(out, policyCandidate{
				source:   PolicySource{Scope: scope, Path: path, Key: strings.Join(candidate.parts, "."), Actions: actions},
				actions:  actions,
				priority: priorBase + candidate.priority,
			})
		}
	}
	return out
}

type pathCandidate struct {
	parts    []string
	priority int
}

func candidatePaths(opts ResolveOptions) []pathCandidate {
	var paths []pathCandidate
	if opts.Command != "" && opts.Milestone != "" {
		paths = append(paths, pathCandidate{[]string{"publication", "commands", opts.Command, "milestones", opts.Milestone}, 50})
	}
	if opts.Command != "" && opts.Event != "" {
		paths = append(paths, pathCandidate{[]string{"publication", "commands", opts.Command, "events", opts.Event}, 40})
	}
	if opts.Command != "" {
		paths = append(paths, pathCandidate{[]string{"publication", "commands", opts.Command, "default"}, 30})
	}
	if opts.Event != "" {
		paths = append(paths, pathCandidate{[]string{"publication", "events", opts.Event}, 20})
	}
	paths = append(paths, pathCandidate{[]string{"publication", "default"}, 10})
	return paths
}

func policyActionsAt(root map[string]any, parts []string) ([]string, bool) {
	var cur any = root
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := m["actions"]
	if !ok {
		return []string{}, true
	}
	actions, ok := stringSlice(raw)
	if !ok {
		return nil, false
	}
	if err := ValidateActions(actions); err != nil {
		return nil, false
	}
	return actions, true
}

func stringSlice(v any) ([]string, bool) {
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func branchPolicyFromConfigs(userConfig, projectConfig map[string]any) BranchPolicy {
	var policy BranchPolicy
	projectDenyConfigured := false
	appendConfig := func(scope string, cfg map[string]any) {
		push, ok := mapAt(cfg, []string{"publication", "push"})
		if !ok {
			return
		}
		if allow, ok := stringSlice(push["allow_branches"]); ok && len(allow) > 0 {
			policy.AllowBranches = append(policy.AllowBranches, allow...)
			policy.Sources = append(policy.Sources, PolicySource{Scope: scope, Key: "publication.push.allow_branches"})
		}
		if _, exists := push["deny_branches"]; scope == "project" && exists {
			projectDenyConfigured = true
		}
		if deny, ok := stringSlice(push["deny_branches"]); ok {
			policy.DenyBranches = append(policy.DenyBranches, deny...)
			policy.Sources = append(policy.Sources, PolicySource{Scope: scope, Key: "publication.push.deny_branches"})
		}
	}
	appendConfig("user", userConfig)
	appendConfig("project", projectConfig)
	if !projectDenyConfigured {
		policy.DenyBranches = append(policy.DenyBranches, defaultDenyBranches...)
		policy.Sources = append(policy.Sources, PolicySource{Scope: "builtin", Key: "publication.push.deny_branches.default"})
	}
	return policy
}

func mapAt(root map[string]any, parts []string) (map[string]any, bool) {
	var cur any = root
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	m, ok := cur.(map[string]any)
	return m, ok
}

func containsAction(actions []string, action string) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}

func removeAction(actions []string, action string) []string {
	out := actions[:0]
	for _, a := range actions {
		if a != action {
			out = append(out, a)
		}
	}
	return out
}

func branchPatternMatch(pattern, branch string) bool {
	patternParts := strings.Split(pattern, "/")
	branchParts := strings.Split(branch, "/")
	if len(patternParts) != len(branchParts) {
		return false
	}
	for i := range patternParts {
		if patternParts[i] == "*" {
			continue
		}
		if patternParts[i] != branchParts[i] {
			return false
		}
	}
	return true
}
