# limelight spec

The three things nobody standardizes, and the only assets every SDK reuses:

| Doc | What it pins down |
|---|---|
| [field-contract.md](field-contract.md) | registered field names → OTel semconv attribute keys |
| [enable-protocol.md](enable-protocol.md) | the targeting predicate + TTL, one wire format for all SDKs |
| [event-schema.md](event-schema.md) | what actually gets emitted, and what makes it joinable |

Context models diverge per language (Go explicit parameter · Java ThreadLocal ·
Rust task-local · Python contextvars). These three documents do not — they are
written in terms of **field names**, never context keys.

**Versioning.** Every payload carries a `version` field. Breaking changes bump it;
SDKs reject a version they do not understand rather than guessing.

Status: **v0 draft.** Nothing here is frozen until the first SDK ships.
