# Git merge drivers

Some SpecScore-managed files are safe to text-merge with git's default
algorithm; two others are not, for opposite reasons:

- **The JSONL event ledger** (`.specscore/events.jsonl` by default, or
  wherever `specscore.yaml`'s `events:` block points it — see
  [events.md](events.md)) is an append-only log. Two branches that each add a
  new line at the end of the same file are, by construction, non-conflicting
  — but git's default line-based merge frequently disagrees, because it
  cannot tell that "insert a line after line N" on both sides means "keep
  both, in some order" rather than "pick one." This is exactly what happened
  when two lanes concurrently ran `specscore lesson new` (which emits an
  event) against a shared repository's committed ledger: a plain
  fast-forward-incompatible merge produced a real textual conflict on a file
  where both sides had done nothing but append.
- **Generated index README files** under `spec/` (the tables listing every
  feature, idea, plan, task, lesson, and decision) are wholly *derived* from
  the individual artifact files they list. A textual conflict on one of these
  is never a real content disagreement to reconcile line by line — the
  correct row is always whatever `specscore spec lint --fix` would write from
  the artifact file itself.

Both cases have the same shape: git's default merge is the wrong tool
because it operates on lines with no knowledge of the file's actual
semantics. `specscore merge-driver install` registers two git [custom merge
drivers](https://git-scm.com/docs/gitattributes#_defining_a_custom_merge_driver)
that replace line-based merging with a resolution matching each file's
actual structure.

> **Note on the default convention.** SpecScore's own default is that
> `.specscore/events.jsonl` is git-ignored — a local audit log, not a
> checked-in artifact (see [events.md](events.md#default-behavior)). The
> events merge driver only matters for a project that has deliberately
> chosen to commit the ledger anyway (as a shared cross-agent audit trail,
> for example); if your project keeps the default `.gitignore`d ledger, only
> the index driver applies to you.

## Install once per clone

```bash
specscore merge-driver install
```

This writes two things, both scoped to the current repository (never your
global git config):

1. `.gitattributes` entries mapping the configured event ledger path and the
   eight known generated index README paths to the two driver names (existing
   `.gitattributes` content and any lines already present are left alone —
   the command is idempotent).
2. Repo-local `git config` entries (`merge.<name>.name` and
   `merge.<name>.driver`) that tell git which command to run for each driver
   name. Both entries invoke `specscore` as found on `PATH` at merge time —
   not a path baked in at install time — so upgrading the CLI later (e.g.
   `brew upgrade specscore`) does not require re-running `install`.

Run it again after every fresh `git clone` or `git worktree add`:
`.gitattributes` travels with the repository, but `git config` is per-clone
and does not.

## `specscore event merge-driver` — the events-ledger driver

Git invokes this with the three placeholders from `gitattributes(5)`:
`specscore event merge-driver %O %A %B` (base, ours, theirs — three temporary
file paths, not the real working-tree file). It is not meant to be run by
hand.

The merge is the same deterministic union `specscore event merge` already
implements (see [events.md](events.md#merging-branch-ledgers)): `<ours>`
keeps its exact bytes and order, and every event present only in `<theirs>`
is appended afterward, sorted by UUID. `<base>` is accepted for the driver
contract but is not needed to compute the result — an append-only ledger's
union does not depend on the common ancestor.

Two UUIDs are reconciled automatically only when their canonical content is
byte-identical on both sides (an honest concurrent append, or the same
emission reaching both branches through a shared dependency). When the same
event UUID carries **different** content on the two sides — which should not
happen from normal emission, since UUIDs are randomly generated per event,
but could result from a hand-edited or rewritten ledger — the driver refuses
to guess. It exits non-zero, leaves `<ours>` untouched, and git reports an
ordinary merge conflict on the file for a human to resolve. Nothing is ever
silently dropped or overwritten.

## `specscore merge-driver index` — the generated-index driver

Git invokes this one with a fourth placeholder, `%P` (the path of the
attributed file, relative to the repository root):
`specscore merge-driver index %O %A %B %P`. One driver command handles every
generated index kind, dispatching on `%P` only to know which real file to
read back after regenerating.

Rather than merge the three git-supplied versions of the file at all, this
driver **regenerates it** — the equivalent of running
`specscore spec lint --fix` scoped to the project containing `%P` — from
whatever artifact files (feature/idea/plan/task/lesson/decision READMEs) are
currently checked out, then copies the freshly regenerated bytes over
`<ours>`. See the `*-index-row-sync` rule family in
[lint-rules.md](lint-rules.md) for exactly what each index kind derives from.

The eight paths `merge-driver install` attributes to this driver:

| Path                                | Regenerated from                          |
| ------------------------------------ | ------------------------------------------ |
| `spec/features/README.md`            | every `spec/features/<slug>/README.md`     |
| `spec/ideas/README.md`               | every `spec/ideas/<slug>.md`               |
| `spec/ideas/archived/README.md`      | every `spec/ideas/archived/<slug>.md`      |
| `spec/plans/README.md`               | every `spec/plans/<slug>.md`               |
| `spec/tasks/README.md`               | every `spec/tasks/<slug>/README.md`        |
| `spec/lessons/README.md`             | every `spec/lessons/<slug>/README.md`      |
| `spec/decisions/README.md`           | every `spec/decisions/<NNN>-<slug>.md`     |
| `spec/decisions/archived/README.md`  | every `spec/decisions/archived/<NNN>-<slug>.md` |

**Known limitation.** Regeneration reads sibling artifact files as they exist
on disk at the moment git invokes the driver. If one of those sibling files
is *itself* still conflict-marked from the same merge (git resolves
conflicted files one at a time, in no particular order), the regenerated
index may be built from a momentarily inconsistent tree. This driver does
not attempt to detect that case — it only fails closed (leaves `<ours>`
untouched, exits non-zero, so git still reports a conflict) when
regeneration itself errors, e.g. a source file fails to parse. When a merge
has conflicts on both an index and one of its source files, resolve the
source file first, then re-run `specscore spec lint --fix` by hand — it is
idempotent, so a redundant run changes nothing.

This is a deliberate scope decision, not an oversight: making the driver
merge-order-aware would require it to coordinate across multiple git-invoked
subprocesses that don't otherwise know about each other, which git's merge
driver contract does not provide a hook for.
