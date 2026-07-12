---
format: https://specscore.md/scenario-specification
---

# Rehearse: Facebook OAuth sign-in hardens the session and lands on the dashboard

**Status:** pending
**Verifies:** cli/rehearse/reusable-checks#ac:use-reused-across-scenarios

## Given a Facebook OAuth authorization code
```bash
echo 'code=facebook-auth-code-456' > callback.txt
echo 'provider=facebook' >> callback.txt
```

## When the Facebook OAuth callback exchanges the code for a session
```bash
# Simulate the server response to the Facebook OAuth callback.
printf 'HTTP/1.1 302 Found\r\nLocation: /dashboard\r\nSet-Cookie: session=fbook; HttpOnly; Secure\r\n\r\n' > out.txt
```

## Then the session is hardened
**Use:** [session-hardened](./_checks/session-hardened.check.md) with response_file=out.txt

## Then the user is redirected to the dashboard
**Use:** [redirected-to-dashboard](./_checks/redirected-to-dashboard.check.md) with response_file=out.txt location=/dashboard
