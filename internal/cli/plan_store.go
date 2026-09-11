package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/planstore"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// planRouteUnavailableSentinel is passed as a plansDir argument (in place of
// the empty string) when Plan routing is configured for the project but
// failed to resolve, and the caller must report Plan context as unavailable
// rather than fall back to searching the local spec/plans tree — which may
// be exactly the stale artifact routing was configured to route away from
// (finding 2, `feature info`'s counterpart to spec lint's finding 1).
//
// It is a syntactically invalid path: an embedded NUL byte, which every
// os.Stat-family call rejects with an error before it ever reaches the
// filesystem (Go's os package fails converting such a string to a C string).
// Passing it as a non-empty plansDir routes call sites like
// feature.FindLinkedPlansDir past their own ""-means-"search locally"
// fallback, into their "plansDir doesn't exist" branch, which already
// returns cleanly empty rather than erroring.
const planRouteUnavailableSentinel = "\x00"

func resolvePlanStore(projectFlag string, mode planstore.AccessMode) (planstore.Resolution, error) {
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return planstore.Resolution{}, err
	}
	store, err := planstore.Resolve(root, mode)
	if err != nil {
		// exitcode.Wrap (not InvalidStateErrorf, which drops the cause via
		// %v) preserves err as the Unwrap() chain's cause so callers that
		// need to distinguish "no route configured" (planstore.ErrNoRoute)
		// from "a route is configured but broken" can still do so via
		// errors.Is on the error this function returns (spec lint / feature
		// info — findings 1 and 2).
		return planstore.Resolution{}, exitcode.Wrap(exitcode.InvalidState,
			fmt.Sprintf("resolving plans repository: %v", err), err)
	}
	return store, nil
}

func ensurePlansNamespace(plansDir string) error {
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return err
	}
	index := filepath.Join(plansDir, "README.md")
	if _, err := os.Stat(index); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	// Use the same canonical plans-index template `specscore init` scaffolds
	// (frontmatter + Contents/Recently Closed/Open Questions sections +
	// adherence footer) so a namespace materialized lazily by `plan new`
	// (same-repo or external) is exactly as lint-clean as one an `init` run
	// would have produced.
	content := plansIndexContent(projectdef.SpecConfig{})
	if err := publishFileExclusive(index, []byte(content), 0o644); err != nil && !os.IsExist(err) {
		return fmt.Errorf("publishing plans namespace index: %w", err)
	}
	return nil
}
