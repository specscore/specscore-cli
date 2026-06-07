---
captured_by: user
status: queued
---

# Linter to output supported prefixes for Related Ideas

## Context

Triggered while drafting an Idea in a sibling repo: used `depends-on:`
(dash) instead of `depends_on:` (underscore) and the linter rejected
it. The error message DOES include the valid list, but exposing the
supported set via a discoverable surface (e.g., `specscore spec lint
--list-relationships`, `specscore idea relationships`, or in
`--help`) would let authors look it up before guessing.
