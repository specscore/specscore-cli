---
format: https://specscore.md/scenario-specification
---

# Rehearse: rehearse acs generates the AC summary

**Status:** pending
**Verifies:** cli/rehearse/thin-acs#ac:summary-generated

## Given a feature with a thin AC in _acs/
```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p spec/features/demo/_acs
cat > spec/features/demo/_acs/session-hardened.ac.md <<'MD'
# AC: session-hardened

**Status:** accepted

## Statement

The session cookie is HttpOnly and Secure.
MD
```

## When I run specscore rehearse acs on the feature
```bash
SPECSCORE="${SPECSCORE:-specscore}"
"$SPECSCORE" rehearse acs spec/features/demo > out.txt
```

## Then the generated summary embeds the AC's statement inline
```bash
grep -q '## Acceptance Criteria' out.txt || { echo "no summary heading"; cat out.txt; exit 1; }
grep -q '\[session-hardened\](_acs/session-hardened.ac.md)' out.txt || { echo "slug not linked"; cat out.txt; exit 1; }
grep -qi 'HttpOnly and Secure' out.txt || { echo "statement not embedded"; cat out.txt; exit 1; }
echo "AC summary generated with inline statement"
```
