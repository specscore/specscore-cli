---
format: https://specscore.md/scenario-specification
---

# Rehearse: OAuth sign-in (nested suite reusing shared checks)

**Status:** pending
**Verifies:** cli/rehearse/nested-suites#ac:suite-two-cases

## Given a Google identity mapped to a user
```bash
echo 'sub=10432' > identity.txt
```

### When the OAuth callback succeeds
```bash
printf 'HTTP/1.1 302 Found\r\nLocation: /dashboard\r\nSet-Cookie: session=g0og; HttpOnly; Secure\r\n\r\n' > out.txt
```
#### Then the session is hardened
**Use:** [session-hardened](./_checks/session-hardened.check.md) with response_file=out.txt
#### Then the user is redirected to the dashboard
**Use:** [redirected-to-dashboard](./_checks/redirected-to-dashboard.check.md) with response_file=out.txt location=/dashboard

### When the user denies consent
```bash
printf 'HTTP/1.1 302 Found\r\nLocation: /login?error=access_denied\r\n\r\n' > out.txt
```
#### Then no session is issued and the user returns to login
```bash
grep -qi '^set-cookie: session=' out.txt && { echo "session should not be set"; exit 1; }
grep -qi '^Location: /login' out.txt || { echo "not sent back to login"; exit 1; }
echo "correctly refused"
```
