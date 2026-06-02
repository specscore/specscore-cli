// Package journal implements the SpecScore activity journal: an append-only,
// date-sharded event store plus on-demand day/week/month summary rollups.
package journal

import (
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/gitremote"
)

// originURLFn is injectable for tests.
var originURLFn = gitremote.OriginURL

// ResolveStream resolves the journal `stream` for a source repo at write time
// (Phase 1): the git origin org, lowercased, when the origin is parseable;
// otherwise the lowercased basename of the repo directory. Every event gets a
// populated stream even with no user configuration. Phase 2 (an explicit
// journal.stream config value, or an explicit null opt-out) layers on top of
// this and is gated on the layered-config Feature.
func ResolveStream(repoRoot string) string {
	if url, err := originURLFn(repoRoot); err == nil {
		if r, ok := gitremote.Parse(url); ok {
			return strings.ToLower(r.Owner)
		}
	}
	return strings.ToLower(filepath.Base(repoRoot))
}
