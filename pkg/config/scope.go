package config

import (
	"path/filepath"
	"strings"
)

// UserScopedKeys are dotted config keys that carry per-user or per-machine
// values (filesystem paths, identities). They MUST NOT appear in the committed
// project file (specscore.yaml); they are accepted only from specscore.local.yaml
// or ~/.specscore.yaml. Owning Features register their keys here.
var UserScopedKeys = []string{
	"recaps.repo",
	"recaps.user",
	"journal.repo",
	"journal.stream",
}

// CommittedScopeViolation reports a user-scoped key found in the committed
// project file where it is not allowed.
type CommittedScopeViolation struct {
	Key string
}

// CheckCommittedScope reads the committed project layer (specscore.yaml) at
// repoRoot and returns a violation for each user-scoped key present in it.
// The local and home layers are not inspected — user-scoped keys are allowed
// there. A missing project file yields no violations; a malformed one errors.
func CheckCommittedScope(repoRoot string) ([]CommittedScopeViolation, error) {
	m, err := loadLayer(filepath.Join(repoRoot, ProjectFile))
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	var violations []CommittedScopeViolation
	for _, key := range UserScopedKeys {
		if hasPath(m, key) {
			violations = append(violations, CommittedScopeViolation{Key: key})
		}
	}
	return violations, nil
}

// hasPath reports whether the dotted key resolves to a present entry in m.
func hasPath(m map[string]any, dotted string) bool {
	parts := strings.Split(dotted, ".")
	cur := m
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			return false
		}
		cur = next
	}
	_, ok := cur[parts[len(parts)-1]]
	return ok
}
