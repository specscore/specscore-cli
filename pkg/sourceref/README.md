# pkg/sourceref

`specscore:` annotation parsing and source-to-spec linking. Detects and parses references from source code comments.

## Outstanding Questions

1. `sourceref.go` hardcodes `"github.com"`, `"specscore"` as the "current" repo defaults used by `parseExpandedURL` to determine whether a cross-repo suffix is needed. Should these be configurable via a parameter or context struct so the library works for any project? For now, these match the original behavior.
