# Lessons for AI coding agents

Use canonical directory Lessons. The compact durable rule lives at `spec/lessons/<slug>/README.md`; each observation is a new immutable JSON object under `occurrences/`. Never put original/user/agent prompts, raw logs, raw diffs, credentials, personal contact data, or private absolute paths in committed Lesson data.

## Create and record

`specscore lesson new <slug>` requires `specscore.yaml` and a non-empty `lessons.classifications` vocabulary before it creates a path. Its bounded write set is the requested Lesson directory, a Git-preserved empty occurrence store (an empty `.gitkeep` marker until the first JSON occurrence), declared index rows, and durable prepared event/outbox. It rejects a sibling flat `<slug>.md` instead of retaining both layouts.

Use `specscore lesson occurrence add <slug> --summary "bounded factual statement" --context-json '<safe object>'`. Context is an opaque JSON object: arbitrary scalar, array, and nested members are preserved, while repository, git, worktree, and execution have recognized semantic shapes. JSON is strict at the occurrence envelope and inside those recognized objects; timestamps use UTC `Z`; path hints and path evidence are normalized repository-relative forward-slash paths or `redacted`. Recursive validation rejects oversized collections, newlines, duplicate redactions, credential/contact patterns, and raw prompt/log/diff properties without echoing unsafe values.

Inspect with `occurrence list` and `occurrence info`. `lesson recur` appends one child for a canonical Lesson, leaves its README byte-identical, and refreshes only its derived index row; it retains the flat-file compatibility writer only until migration. `lesson check --not-enforced --min-recurred N` is the opt-in process gate.

## Import and deduplicate without losing history

Run `specscore lesson import-legacy --source LESSONS-LEARNED.md --dry-run --format json` first. Inventory is write-free and records exact source bytes/hash, heading blocks, inline recurrence-marker blocks, duplicate numeric-ID collisions, and unmatched candidates. Its ordered entry-projection hash binds every key, parent, byte range, and block hash, so parser drift cannot hide behind unchanged counts. A marker can aggregate incidents, so never convert prose such as “at least twice” into an invented count.

Review a mapping where every source key chooses `new`, `occurrence`, or `manual`. Apply fails closed while any manual row remains. Each `new` row requires `status: Recorded`, at least one configured classification, and concise safe `lesson` plus `process_gap` text; missing, placeholder, unsafe, or unconfigured values fail before the prepared event. Source status stays in provenance and the result reports the conservative Recorded decision—apply never fabricates Enforced evidence. It requires an immutable committed repository/path/full-revision source identity. `new` creates the compact rule and one provider occurrence; each reviewed occurrence mapping creates one deterministic child. Apply publishes transactionally without overwriting, is byte-identical on retry, and stores only source identity, byte ranges, and hashes—not raw/base64 legacy prose.

Use `lesson relation add <from> <to> --type related|duplicates|supersedes` only after human review, then repeat it with the exact preview `--confirm` token. A duplicate is retained as `Superseded`, with `Duplicate Of` and `Superseded By` pointing to the unchanged canonical Lesson. Enforced duplicates, cycles, conflicting existing targets, malformed relation state, and overwrite races fail before mutation.

## Migrate a structured flat Lesson

Commit the exact `spec/lessons/<slug>.md` source, then run:

```sh
specscore lesson migrate-flat <recorded-slug> --classification process --format json
specscore lesson migrate-flat <enforced-slug> --classification validation --control "Run the durable check." --verification "go test ./pkg/event -run TestBoundary" --evidence "pkg/event/outbox_test.go" --format json
```

The command records immutable repository/path/revision/source+range hashes, creates one provider occurrence plus one child per structured `## Recurrences` bullet, refuses a `Recurred` mismatch, validates canonical output, updates only that exact index row, and durably commits the same prepared event. It atomically moves the flat source and completed marker—without replacement—into `.specscore/recovery/flat-migration/<event-uuid>/`, a private gitignored recovery namespace; it never unlinks a post-publication path and resumes only byte-verifiable interrupted state. An Enforced source additionally requires reviewed control, reproducible verification, and stable evidence flags; Git provenance is not enforcement proof. Raw incident and recurrence prose remains at its immutable Git provenance rather than being republished.

## Explicit Open Questions migration

Only `specscore spec lint --fix` rewrites the obsolete exact heading `## Outstanding Questions` to `## Open Questions`. Without `--fix`, lint diagnoses it and changes no byte. Scaffold/create commands never perform that repository-wide rewrite as a side effect; writers and help emit only `Open Questions`.

## Durable event delivery

Every changed Lesson mutation prepares a durable event before artifact publication unless the project explicitly configures an empty subscriber set. The immutable ledger retains a complete nonempty subscriber set; pending delivery can be reconstructed, and one successful sink never acknowledges another. The explicit empty set opts out without creating a recipientless ledger. Delivery is at-least-once, so subscribers deduplicate `event.uuid`.

Use `specscore event replay --subscriber <stable-name> [--from <event-uuid>]` to retry one sink in deterministic event-timestamp/UUID ledger order. `--from` is inclusive and remains valid after that event is acknowledged. Replay reports unresolved prepared records. Inspect the named artifact evidence, preview `specscore event reconcile <uuid> --decision commit|abort`, then repeat with its exact `--confirm` token. Path existence is evidence, never an automatic decision.
