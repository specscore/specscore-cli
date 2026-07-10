---
format: https://specscore.md/scenario-specification
---

# Rehearse: cadences-in-help

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:cadences-in-help (REQ: freshness-cadences)

Scenario source: [../README.md](../README.md) → `### AC: cadences-in-help`.

Given the specscore CLI, when I run `specscore studio probe --help`, then the help text names a re-verification cadence for each of `verified-behavior`, `derived`, `declared`, `claimed`, and `attested`.

No seams are needed: help text is static.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:cadences-in-help
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# When I run `specscore studio probe --help`.
help="$("$SPECSCORE" studio probe --help 2>&1)"

# Then the help text names each evidence class paired with a cadence phrase.
require() { # require <needle> <label>
  case "$help" in
    *"$1"*) ;;
    *) echo "FAIL: help lacks $2 ($1)"; echo "$help"; exit 1 ;;
  esac
}

require 'verified-behavior' "the verified-behavior class"
require 'hours'             "the verified-behavior cadence"
require 'derived'           "the derived class"
require 'on push'           "the derived cadence"
require 'declared'          "the declared class"
require 'on repo change'    "the declared cadence"
require 'claimed'           "the claimed class"
require 'never'             "the claimed cadence"
require 'attested'          "the attested class"
require 'quarterly'         "the attested cadence"

echo "PASS: cadences-in-help"
```

---
*This document follows the https://specscore.md/scenario-specification*
