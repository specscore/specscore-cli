# specscore spec lint

Validate the spec tree against structural conventions: README presence, Outstanding Questions sections, heading levels, feature references, internal links, index completeness, adherence footers, and Idea-specific rules. `--fix` applies autofixes for safe rules.

**CLI reference:** [`cli/spec/lint`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/spec/lint/README.md)

## When to use

- **Before committing spec edits:** Catch convention drift before the diff goes up
- **After [`feature new`](../../feature/references/new.md) / [`task new`](../../task/references/new.md) / [`idea new`](../../idea/references/new.md):** Confirm the scaffolded artifact is clean (it should be — but verify)
- **CI gating:** `exit 1` on violations is the contract CI relies on
- **Periodic audit:** Catch silent drift in long-lived spec trees
- **Targeted check:** Use `--rules a,b` to run only specific checks during fast iteration

## Command

```bash
specscore spec lint \
  [--fix] \
  [--severity <error|warning|info>] \
  [--rules <list>] \
  [--ignore <list>] \
  [--format <text|json|yaml>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--fix` | No | Apply autofixes for rules that declare themselves safe to autofix (e.g., adherence footer insertion). Idempotent. |
| `--severity` | No | Minimum severity reported: `error` (default), `warning`, `info`. Filter is `info ⊂ warning ⊂ error` (wider includes narrower). |
| `--rules` | No | Comma-separated allowlist. Run only these rules. Mutually exclusive with `--ignore`. |
| `--ignore` | No | Comma-separated denylist. Skip these rules. Mutually exclusive with `--rules`. |
| `--format` | No | Output format: `text` (default), `json`, `yaml`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | No violations at or above `--severity` | Proceed. |
| `1` | One or more violations at or above `--severity` | **This is the gate signal.** Read the report, fix or `--fix`, retry. |
| `2` | Invalid `--rules` / `--ignore` name, invalid `--severity` / `--format` value, or `--rules` and `--ignore` both supplied | Check arguments. |
| `10+` | Unexpected I/O failure while scanning | Log and escalate. |

## Examples

### Quick check

```bash
specscore spec lint
# 0 violations found
```

### Include warnings

```bash
specscore spec lint --severity warning
```

### Run a single rule

```bash
specscore spec lint --rules adherence-footer
```

### Run all rules except one

```bash
specscore spec lint --ignore source-references --severity warning
```

### JSON for tooling

```bash
specscore spec lint --format json --severity warning | jq '.[] | select(.severity=="error")'
```

### Autofix safe rules and verify idempotence

```bash
specscore spec lint --fix
specscore spec lint --fix    # second run is a no-op (exit 0, no changes)
```

### CI gate

```yaml
# .github/workflows/dogfood.yml
- name: Lint spec tree
  run: specscore spec lint
```

## Notes

- **Exit 1 ≠ failure to run.** It is the *successful* signal that violations exist. CI scripts treat it as a hard gate.
- **`--fix` is conservative:** it only mutates what each rule declares safe (e.g., inserts a missing adherence footer). Rules that need human judgment — like a wrong-URL footer that *might* mean the document is mis-classified — are never autofixed.
- **`--fix` is idempotent:** a second consecutive run yields no changes and exits `0`.
- **`--rules` and `--ignore` are mutually exclusive.** Supplying both exits `2`.
- **Unknown rule names are hard errors** in either flag — the lint run is not started until names validate.
- **Default severity is `error`.** Warnings and info-level findings are silent unless explicitly widened.
- **Path argument** is not currently accepted — the full tree is always scanned. Tracked as an [open question](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/spec/lint/README.md#outstanding-questions).
- For per-feature inspection (rather than tree-wide validation), use [`feature info`](../../feature/references/info.md).
