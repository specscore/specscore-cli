package cli

// Features implemented: coordination-branch enforcement for plan-mutating
// verbs (cli/plan/change-status, cli/plan/reconcile, cli/task/change-status
// --plan). See spec/features/plan/README.md#coordination-branch (specscore)
// for the field's syntax and intended meaning; this file owns the CLI-side
// enforcement the upstream doc explicitly defers to `specscore-cli`.

import (
	"fmt"
	"io"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/gitremote"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// coordinationForceFlagName is the shared `--force-coordination` flag name
// every plan-mutating verb registers. Bypassing the coordination-branch check
// is an exceptional, per-invocation override (mirroring `--force-tasks` on
// `plan reconcile`), never a persistent setting.
const coordinationForceFlagName = "force-coordination"

// coordinationForceFlagUsage is the shared help text for --force-coordination.
const coordinationForceFlagUsage = "bypass the plan's **Coordination:** repo/branch check for this invocation (prints a warning; does not modify the plan's Coordination field)"

// enforceCoordinationBranch checks, for a Plan that declares a
// **Coordination:** header field, that the CURRENT git repository and branch
// (resolved from projectRoot) match the declared <owner>/<repo>@<branch>. A
// plan with no **Coordination:** field is unrestricted and always passes
// (nil). A malformed value is treated the same as a mismatch — it is already
// a `specscore spec lint` (P-010) violation, and this verb refuses rather
// than guess intent.
//
// force (--force-coordination) bypasses BOTH the malformed-value case and a
// genuine mismatch: it prints a warning naming what was bypassed to warn and
// lets the mutation proceed, exactly like other force-an-exception flags in
// this CLI (e.g. `plan reconcile --force-tasks`).
func enforceCoordinationBranch(p *plan.Plan, projectRoot string, force bool, warn io.Writer) error {
	if p.CoordinationLine == 0 {
		return nil // no Coordination field: unrestricted
	}

	owner, repo, branch, ok := lint.ParseCoordinationRef(p.Coordination)
	var reason string
	matched := false
	if !ok {
		reason = fmt.Sprintf(
			"declares a malformed **Coordination:** value %q (run `specscore spec lint` to see the P-010 violation)",
			p.Coordination)
	} else {
		var actual string
		matched, actual = coordinationCheck(projectRoot, owner, repo, branch)
		if !matched {
			reason = fmt.Sprintf(
				"is coordinated on %s/%s@%s, but this invocation is running from %s",
				owner, repo, branch, actual)
		}
	}
	if matched {
		return nil
	}
	if force {
		_, _ = fmt.Fprintf(warn,
			"warning: --%s bypassed the coordination-branch check for plan %s (%s)\n",
			coordinationForceFlagName, p.Slug, reason)
		return nil
	}
	return exitcode.ConflictErrorf(
		"plan %s %s; mutating it here is a process violation (spec/features/plan#coordination-branch) — "+
			"switch to the declared repo/branch, or pass --%s to bypass (prints a warning and proceeds)",
		p.Slug, reason, coordinationForceFlagName)
}

// coordinationCheck resolves the ambient git identity at projectRoot and
// compares it against the declared owner/repo/branch. matched is false
// whenever it cannot be POSITIVELY confirmed that the invocation is running
// on the declared coordination branch — an unresolvable git remote, a
// non-GitHub remote, or a detached HEAD are all treated as a mismatch (fail
// closed), never silently accepted. actual is a human-readable description of
// what was found, for the refusal/warning message.
func coordinationCheck(projectRoot, wantOwner, wantRepo, wantBranch string) (matched bool, actual string) {
	originURL, err := gitremote.OriginURL(projectRoot)
	if err != nil {
		return false, "a directory with no resolvable git origin remote"
	}
	remote, ok := gitremote.Parse(originURL)
	if !ok {
		return false, fmt.Sprintf("origin remote %q (not a recognized GitHub owner/repo)", originURL)
	}
	branch, err := gitremote.CurrentBranch(projectRoot)
	if err != nil {
		return false, fmt.Sprintf("%s/%s with a detached HEAD (no current branch)", remote.Owner, remote.Repo)
	}
	actual = remote.Owner + "/" + remote.Repo + "@" + branch
	if !strings.EqualFold(remote.Owner, wantOwner) || !strings.EqualFold(remote.Repo, wantRepo) || branch != wantBranch {
		return false, actual
	}
	return true, actual
}
