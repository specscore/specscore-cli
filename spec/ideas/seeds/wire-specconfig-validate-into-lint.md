---
captured_by: claude
status: queued
---
# Wire SpecConfig.Validate() into the lint pipeline

`projectdef.SpecConfig.Validate()` (module-paths uniqueness/non-nesting, repositories shape/roles, and the new `path_overrides.ideas_path` absolute/`../` rejection) is **never called in production** — only in `pkg/projectdef` unit tests. `ReadSpecConfig` deserializes but does not validate, and no lint rule calls `Validate()`. `config_scope.go` even assumes "malformed specscore.yaml is surfaced by other rules" — but no such rule exists.

Consequence: `configurable-ideas-path#ac:invalid-path-rejected` holds at the `Validate()` contract (unit-tested) but `specscore spec lint` does not surface an absolute/escaping `ideas_path` (nor module-path or repositories violations).

## Task

Add a `config-valid` lint checker that loads `specscore.yaml` and emits each `SpecConfig.Validate()` error as a hard `error` violation (file = `specscore.yaml`). Register it, cover it 100%, and add fixtures for `ideas_path: /abs` and `ideas_path: ../x`.

## Caution

This newly enforces ALL `Validate()` rules at lint time — audit existing repos/test fixtures for latent invalid configs before enabling, so the change doesn't unexpectedly fail currently-green repos.
