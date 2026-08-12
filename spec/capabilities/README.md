# CLI capability delivery

`specscore.json` is the machine-readable capability × Runtime/Help/AI Skill/Tests manifest for the `specscore` binary. It conforms to the pinned SpecScore core `cli-capability-delivery` schema under `schemas/`; the same directory pins the occurrence schema consumed by Lesson validation. Executable tests validate both schema digests/provenance plus public command and flag coverage, help anchors, AI skill markers/examples, test symbols, and the generated matrix.

See the generated human projection at [`docs/capabilities/specscore.md`](../../docs/capabilities/specscore.md).

## Open Questions

None at this time.
