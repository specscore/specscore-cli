# specscore code deps

Scan source files for `specscore:` annotations and bare `https://specscore.md/...` URLs in comments, then list the SpecScore resources (features, plans, docs) those files depend on.

**CLI reference:** [`cli/code/deps`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/code/deps/README.md)

## When to use

- **"What spec does this code implement?"** — the canonical traceability query
- **Impact analysis:** Before changing a feature, list the source files that reference it
- **CI drift detection:** Diff `code deps` output between two commits to spot newly-reference / dropped specs
- **Onboarding:** A new contributor can follow code → spec without reading every comment by hand

## Command

```bash
specscore code deps \
  [--path <glob>] \
  [--type <feature|plan|doc>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--path` | No | Double-star glob filtering which source files to scan (e.g., `pkg/**/*.go`, `internal/cli/*.go`). Default `**/*` (every file). |
| `--type` | No | Filter results to one resource type: `feature`, `plan`, or `doc`. Omit to include all types. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Scan completed (zero or more references reported) | Parse the output. |
| `2` | Invalid `--path` or `--type` value | Check arguments. |
| `10+` | Unexpected I/O failure while scanning | Log and escalate. |

## Examples

### Scan the whole repo

```bash
specscore code deps
```

### Scan only Go files under `pkg/`

```bash
specscore code deps --path 'pkg/**/*.go'
```

### Filter to feature references only

```bash
specscore code deps --type feature
```

### Pipe to count references per file

```bash
specscore code deps --type feature | sort | uniq -c | sort -rn
```

### Diff between commits for drift

```bash
git stash
specscore code deps > /tmp/before.txt
git stash pop
specscore code deps > /tmp/after.txt
diff /tmp/before.txt /tmp/after.txt
```

## Notes

- **Read-only** — never mutates files or state.
- **Two reference forms** are detected and deduplicated per source file:
  1. `specscore:` annotations (defined by the [source-references](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/source-references/README.md) feature).
  2. Bare `https://specscore.md/...` URLs in comments.
  A reference appearing in both forms in the same file is reported exactly once.
- **Stable output:** files sorted, references within a file sorted — safe to diff in CI.
- **No reverse query yet.** "What source files reference *this feature*?" is not provided by `code deps`. Today, use `grep -r 'specscore:'` or [`feature refs`](../../feature/references/refs.md) for the spec-to-spec edge. A `code refs` command is [open question](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/code/README.md#outstanding-questions).
- **`--path` is a single glob** (no comma list) in the MVP — run the command twice if you need a union.
