package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// ValidateID accepts a slash-separated hierarchy of ordinary Plan slugs.
// Every segment is independently safe, so an ID can be joined to a plans root
// without traversal or platform-specific separator ambiguity.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("plan ID must not be empty")
	}
	if strings.Contains(id, "\\") || strings.HasPrefix(id, "/") || strings.HasSuffix(id, "/") {
		return fmt.Errorf("plan ID %q must be slash-separated lowercase slugs", id)
	}
	segments := strings.Split(id, "/")
	for _, segment := range segments {
		if err := ValidateSlug(segment); err != nil {
			return fmt.Errorf("invalid plan ID segment %q: %w", segment, err)
		}
	}
	return nil
}

// PathForID returns the canonical directory-form path for a Plan ID.
func PathForID(plansDir, id string) (string, error) {
	if err := ValidateID(id); err != nil {
		return "", err
	}
	parts := append([]string{plansDir}, strings.Split(id, "/")...)
	return filepath.Join(append(parts, "README.md")...), nil
}

// ValidateWritePath rejects a prospective Plan target whose existing ancestor
// resolves outside the already resolved Plans namespace.
func ValidateWritePath(plansDir, target string) error {
	root, err := filepath.EvalSymlinks(plansDir)
	if err != nil {
		return fmt.Errorf("resolve plans directory %s: %w", plansDir, err)
	}
	// Walk up from target looking for the first ancestor that actually
	// exists. filepath.Dir always converges to a fixed point ("/" for an
	// absolute target, "." for a relative one) in finitely many steps, and
	// that fixed point always exists on any real filesystem — so this loop
	// is guaranteed to terminate via the os.Lstat success case below; a
	// "walked off the top without finding anything" fallback is not
	// reachable for any real target path.
	ancestor := target
	for {
		_, statErr := os.Lstat(ancestor)
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}
		ancestor = filepath.Dir(ancestor)
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return exitcode.ConflictErrorf("plan write path escapes plans directory: %s", target)
	}
	return nil
}

// ResolveFile resolves the canonical directory form and the legacy flat form.
// Both forms for one logical ID are an error rather than an implicit winner.
func ResolveFile(plansDir, id string) (string, error) {
	canonical, err := PathForID(plansDir, id)
	if err != nil {
		return "", exitcode.InvalidArgsErrorf("invalid plan ID %q: %v", id, err)
	}
	canonicalExists, err := regularFileExists(canonical)
	if err != nil {
		return "", err
	}
	flat := ""
	flatExists := false
	if !strings.Contains(id, "/") {
		flat = filepath.Join(plansDir, id+".md")
		flatExists, err = regularFileExists(flat)
		if err != nil {
			return "", err
		}
	}
	if canonicalExists && flatExists {
		return "", exitcode.ConflictErrorf("plan %q is ambiguous: both %s and %s exist", id, flat, canonical)
	}
	if canonicalExists {
		if err := validateResolvedPlanPath(plansDir, canonical); err != nil {
			return "", err
		}
		return canonical, nil
	}
	if flatExists {
		// flat is always a direct child of plansDir (id already passed
		// ValidateID's no-"/" check via PathForID above), and regularFileExists
		// just proved it Lstats to a real, non-symlink regular file — so
		// unlike the canonical branch above (where an intermediate directory
		// segment CAN be a symlink pointing outside plansDir), there is no
		// intermediate path component here for an attacker or a stale
		// symlink to hide an escape behind. See validateResolvedPlanPathFn's
		// doc comment for why this is TOCTOU-only.
		if err := validateResolvedPlanPathFn(plansDir, flat); err != nil {
			return "", err
		}
		return flat, nil
	}
	return "", exitcode.NotFoundErrorf("plan not found: %s", id)
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, exitcode.UnexpectedErrorCause(fmt.Sprintf("stat %s: %v", path, err), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, exitcode.ConflictErrorf("plan path must not be a symbolic link: %s", path)
	}
	if !info.Mode().IsRegular() {
		return false, exitcode.ConflictErrorf("plan path is not a regular file: %s", path)
	}
	return true, nil
}

// validateResolvedPlanPathFn is an injectable seam over
// validateResolvedPlanPath for call sites where a failure can only occur via
// a genuine concurrent mutation of plansDir/path between an earlier
// existence check and this call — not something a deterministic,
// non-racy test can force by construction. The seam lets those branches
// still get a deterministic test.
var validateResolvedPlanPathFn = validateResolvedPlanPath

func validateResolvedPlanPath(plansDir, path string) error {
	root, err := filepath.EvalSymlinks(plansDir)
	if err != nil {
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("resolve plans directory %s: %v", plansDir, err), err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("resolve plan path %s: %v", path, err), err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return exitcode.ConflictErrorf("plan path escapes plans directory: %s", path)
	}
	return nil
}

func planIDFromPath(plansDir, path string) (string, bool) {
	rel, err := filepath.Rel(plansDir, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "/README.md") {
		id := strings.TrimSuffix(rel, "/README.md")
		return id, ValidateID(id) == nil
	}
	if !strings.Contains(rel, "/") && strings.HasSuffix(rel, ".md") && rel != "README.md" {
		id := strings.TrimSuffix(rel, ".md")
		return id, ValidateID(id) == nil
	}
	return "", false
}
