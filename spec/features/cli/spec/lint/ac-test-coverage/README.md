---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: AC Test Coverage Lint Rule

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-test-coverage?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-test-coverage?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-test-coverage?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-test-coverage?op=request-change) |

**Status:** Draft
**Date:** 2026-11-27
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

Adds one opt-in lint rule, `feature-ac-test-coverage` (error), that requires every acceptance criterion of an Implementing or Stable Feature to be named by an annotation in a test file, or to be explicitly deferred. A repository turns it on with `ac_test_coverage.required: true` in `specscore.yaml`; repositories that do not are unaffected.

## Problem

Green tests do not prove that a Feature's acceptance criteria are covered: a suite can pass while no test exercises a given AC, and nothing connects the two. The same gap has been recorded repeatedly (backstage lesson `l51-passing-tests-do-not-prove-a-feature-implements-the-acceptance-criteria-the-tests-never-name`). A repository whose Features and tests live together should be able to require, mechanically, that every AC of a built Feature is named by at least one test, or is deferred on purpose with a reason.

The rule is opt-in because in some repositories (for example a product backstage repository) Features are implemented in other repositories, so lint there cannot see the tests.

## Behavior

### Opt-in and selection

#### REQ: rules-registered

The rule MUST be registered in the lint rule registry as `feature-ac-test-coverage`, family `feature`, severity `error`, and MUST be accepted by `--rules`, `--ignore` and `lint.AllRuleNames()`.

#### REQ: opt-in

The rule MUST report nothing unless the project's `specscore.yaml` carries:

```yaml
ac_test_coverage:
  required: true
  test_files:                       # optional; replaces the defaults
    - "**/*_test.go"
```

A missing block, `required: false`, or an unreadable `specscore.yaml` MUST leave the rule off, so a repository's default `specscore spec lint` result does not change when the CLI gains it. Naming the rule in `--rules` MUST run it even without the block, so a repository can measure its gap before opting in. Glob entries starting with `*` must be quoted in YAML.

#### REQ: config-shape

A present `ac_test_coverage` block that is malformed MUST be reported as one `feature-ac-test-coverage` finding citing `specscore.yaml`, and the rule MUST NOT check Features while it stands. The finding is an error, except a warning when the block itself sets `required: false`. Malformed means: not a mapping; `required` not a YAML boolean; `test_files` not a non-empty list of non-empty strings, or an entry that is not a valid glob; or an unknown key (unknown keys are still preserved in the parsed config). A malformed block MUST NOT make the rest of `specscore.yaml` unreadable.

### What is checked

#### REQ: scope

The rule applies to every Feature README (`spec/features/**/README.md` recognized by its `# Feature: <title>` H1, outside `_`-prefixed directories) whose `**Status:**` is `Implementing` or `Stable` (case-insensitive). Every other status is out of scope. Its acceptance criteria are the `### AC: <id>` headings under `## Acceptance Criteria` (the section runs to the next level-1 or level-2 heading; a `---` thematic break inside it does not end it); headings inside fenced code blocks or HTML comments do not count. Each AC is addressed as `<feature-path>#ac:<id>`, where `<feature-path>` is the Feature's directory relative to `spec/features` (for example `cli/code/deps#ac:path-filter-works`).

#### REQ: test-files

Test files are the regular files under the project root whose slash-separated project-relative path matches one of the `ac_test_coverage.test_files` globs (doublestar syntax, `{a,b}` alternatives allowed). The default set is `**/*_test.go`, `**/*.test.{ts,tsx,js,mjs}`, `**/*.spec.{ts,tsx,js,mjs}`, `**/test_*.py` and `**/*_test.py`. Directories named `.git` and `node_modules` are not searched. A project root that is itself a symlink is resolved before the search; symlinked files and directories inside the tree are not followed. Annotations in files that match no glob MUST NOT count.

#### REQ: annotation-names-ac

An AC is covered when at least one line of a test file carries a source annotation that names exactly that AC. Lines are parsed with the same parser as `specscore code deps` (the [source-references](../../../../source-references/README.md) syntax: typed `specscore:verifies` / `specscore:references` directives, untyped `specscore:` annotations, and the canonical `https://specscore.org/...` URL form, each behind a comment marker). The target MUST be a Feature reference whose fragment is `ac:<id>` (the `ac` prefix case-insensitive, the id exact) and whose path is the Feature's path. A directive that fails relation validation — such as `specscore:implements`, which must target a REQ — MUST NOT count. A cross-repository target (authority form or URL) counts only when its `host/org/repo` equals this project's `project.host`, `project.org` and `project.repo`. The rule reads comments only: it MUST NOT run tests or read coverage profiles.

#### REQ: deferral

An AC that no test names is still satisfied when it is explicitly deferred, by either:

1. an entry for it in the existing `## Deferred AC Coverage` list (`- <feature-path>#ac:<id> — <reason>`, see [plan-rules](../plan-rules/README.md)) of a Plan that is not retired. A retired Plan — `Rejected`, `Withdrawn`, `Superseded` or `Deprecated`, the dispositions `P-001`/`P-002` also skip — is a historical record and defers nothing; or
2. a line `**Deferred:** <reason>` inside the AC's own block (between its `### AC:` heading and the next heading or `---` break).

In both forms the reason MUST be non-empty: an entry or line with no reason, or only a dash, is not a deferral. For the AC-level line the finding says so.

Plan discovery follows the lint run's Plan routing; when Plan routing is configured but broken, the rule reports nothing (the run already fails on `plan-route-unresolved`). A missing Plans directory defers nothing; any other error reading the Plan tree (an unreadable, symlinked or ambiguous Plan) fails the run rather than being treated as "no deferrals".

#### REQ: violation-message

Each AC that is neither covered nor deferred MUST be one `feature-ac-test-coverage` error citing the Feature README and the AC heading line, with a single-line message naming the Feature status, the AC as `<feature-path>#ac:<id>`, and the fix:

> Feature is Stable but `cli/code/deps#ac:path-filter-works` is named by no test; add `specscore:verifies feature/cli/code/deps#ac:path-filter-works` to a test file (ac_test_coverage.test_files), or defer it with `**Deferred:** <reason>` under the AC or a `## Deferred AC Coverage` entry in a Plan.

Findings are sorted by file, then line.

## Acceptance Criteria

### AC: off-without-opt-in

**Requirements:** cli/spec/lint/ac-test-coverage#req:opt-in, cli/spec/lint/ac-test-coverage#req:rules-registered

**Given** a project without an `ac_test_coverage` block (or with `required: false`, or with no readable `specscore.yaml`) and a Stable Feature whose AC no test names,
**When** `specscore spec lint` runs,
**Then** no `feature-ac-test-coverage` finding is reported; and `specscore spec lint --rules feature-ac-test-coverage` on the same project reports the AC.

### AC: malformed-config-visible

**Requirements:** cli/spec/lint/ac-test-coverage#req:config-shape

**Given** an `ac_test_coverage` block that is a scalar, has `required: "yes"`, `test_files: []`, a non-string or empty `test_files` entry, an invalid glob, or an unknown key, and separately `required: false` with an unknown key,
**When** `specscore spec lint --severity warning` runs,
**Then** each of the first gives one `feature-ac-test-coverage` error citing `specscore.yaml`, the last gives a warning, and the rest of `specscore.yaml` still reads.

### AC: only-implementing-and-stable

**Requirements:** cli/spec/lint/ac-test-coverage#req:scope

**Given** an opted-in project and Features at Draft, Approved, Implementing, Stable and Deprecated, each with one AC no test names, plus an AC heading inside a code fence,
**When** `specscore spec lint` runs,
**Then** only the Implementing and Stable Features' ACs are reported, and the fenced heading is not.

### AC: test-annotation-covers

**Requirements:** cli/spec/lint/ac-test-coverage#req:annotation-names-ac, cli/spec/lint/ac-test-coverage#req:test-files

**Given** an opted-in project with a Stable Feature `orders` with ACs `a` to `f`, and test files carrying `// specscore:verifies feature/orders#ac:a`, `# specscore:feature/orders#AC:b`, a `specscore:verifies` `specscore.org` URL naming `c` under this project's identity, a bare `// https://specscore.org/...` URL naming `h`, a `specscore://<host>/<org>/<repo>/feature/orders#ac:i` authority form under this project's identity, and `specscore:references feature/orders#ac:j`, plus a URL naming `d` and an authority form naming `k` under another project's identity, while `e` is named only in a non-test source file and `f` only by `specscore:implements`,
**When** `specscore spec lint` runs (also through a symlink to the project root),
**Then** `a`, `b`, `c`, `h`, `i` and `j` are covered and `d`, `e`, `f` and `k` are reported.

### AC: custom-test-globs

**Requirements:** cli/spec/lint/ac-test-coverage#req:test-files

**Given** an opted-in project with `test_files: ["e2e/**/*.feature"]`, one AC named in `e2e/x.feature` and one named only in a `_test.go` file, a third named only in a test file under `node_modules`, and a fourth named only in a non-test file that a symlink matching the glob points to,
**When** `specscore spec lint` runs,
**Then** the first is covered and the other three are reported.

### AC: deferral-counts

**Requirements:** cli/spec/lint/ac-test-coverage#req:deferral

**Given** an opted-in project with a Stable Feature whose AC `x` is listed with a reason in a Draft Plan's `## Deferred AC Coverage`, whose AC `y` carries `**Deferred:** needs a staging account`, whose AC `z` carries an empty `**Deferred:**` line, whose ACs `v` and `u` are listed in that Plan with no reason (`- orders#ac:v`, `- orders#ac:u —`), and whose ACs `r` and `s` are deferred only by a Rejected and a Superseded Plan,
**When** `specscore spec lint` runs,
**Then** `x` and `y` are not reported, `z` is reported with a message saying the deferral has no reason, and `v`, `u`, `r` and `s` are reported.

### AC: message-names-feature-ac-and-fix

**Requirements:** cli/spec/lint/ac-test-coverage#req:violation-message

**Given** an opted-in project with an Implementing Feature `orders` whose AC `total` no test names,
**When** `specscore spec lint` runs,
**Then** one error cites `features/orders/README.md` at the AC heading line, and its one-line message contains `Feature is Implementing`, `orders#ac:total` and `specscore:verifies feature/orders#ac:total`, and mentions `**Deferred:**`.

## Open Questions

- **Require the named test to run and pass in CI.** This rule proves only that a test file names the AC in a comment (decision 2026-11-27, option "a"). A stronger future option would require the named test to actually run and pass in CI, for example by matching annotations against a test report; it is not part of this rule.
- **Config key shape.** The opt-in lives in a top-level `ac_test_coverage:` block, following the `storage_layout:` precedent. A general `lint:` block for opt-in rules remains the alternative noted there.

---
*This document follows the https://specscore.md/feature-specification*
