---
format: https://specscore.md/scenario-specification
---

# Rehearse: Email sign-in hardens the session

**Status:** pending
**Verifies:** cli/rehearse/reusable-checks#ac:use-reused-across-scenarios

## Given the email form returns a hardened session cookie
```bash
printf 'HTTP/1.1 302 Found\r\nSet-Cookie: session=abc; HttpOnly; Secure\r\n\r\n' > out.txt
```

## Then the session is hardened
**Use:** [session-hardened](./_checks/session-hardened.check.md) with response_file=out.txt
