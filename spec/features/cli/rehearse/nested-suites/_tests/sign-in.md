---
format: https://specscore.md/scenario-specification
---

# Rehearse: sign-in behaviour (nested suite)

**Status:** pending
**Verifies:** cli/rehearse/nested-suites#ac:suite-two-cases

## Given a login endpoint stub (shared by every branch)
```bash
cat > login.sh <<'SH'
#!/bin/sh
if [ "$1" = "correct" ]; then echo "HTTP/1.1 200 OK"; else echo "HTTP/1.1 401 Unauthorized"; fi
SH
chmod +x login.sh
```

### When the password is correct
```bash
./login.sh correct > resp.txt
```
#### Then the response is 200 OK
```bash
grep -q '200 OK' resp.txt || { echo "expected 200"; cat resp.txt; exit 1; }
```

### When the password is wrong
```bash
./login.sh wrong > resp.txt
```
#### Then the response is 401 Unauthorized
```bash
grep -q '401' resp.txt || { echo "expected 401"; cat resp.txt; exit 1; }
```
