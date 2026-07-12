# Reusable checks

Reusable, parameterized verification units (`<slug>.check.md`) invoked from the sibling scenarios via `**Use:**` — Feature: [cli/rehearse/reusable-checks](../../README.md). These are not scenarios and are never run directly (the runner excludes `.check.md` from discovery).

- `session-hardened.check.md` — asserts the response sets an `HttpOnly` session cookie.
- `redirected-to-dashboard.check.md` — asserts the response is a `302` redirect to the given `location`.

Both are shared unchanged by the email, Google, and Facebook sign-in scenarios.

## Open Questions

None at this time.
