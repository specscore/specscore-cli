---
type: sidekick-seed
slug: linter-to-output-supported-prefixes-for-related-ideas
captured_at: 2026-05-28T17:34:52Z
captured_by: user
captured_during: null
trigger: explicit
status: queued
synchestra_task: null
---

# Linter to output supported prefixes for Related Ideas

## Context

Triggered while drafting an Idea in a sibling repo: used `depends-on:`
(dash) instead of `depends_on:` (underscore) and the linter rejected
it. The error message DOES include the valid list, but exposing the
supported set via a discoverable surface (e.g., `specscore spec lint
--list-relationships`, `specscore idea relationships`, or in
`--help`) would let authors look it up before guessing.
