# Field contract (v0 draft)

A tag names a **registered extractor**, never a raw context key.

```go
limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
```

Team A stores a struct, team B a string, team C a metadata bag — none of it leaks into
the tool. This registry is what makes limelight portable team-to-team and
language-to-language.

## Mapping onto OTel semantic conventions

Emit `user.id`, not `userID`. semconv is at 1.44.0; `user.*` and `enduser.*` are in the
registry. Without this mapping the downstream join does not happen for free, and the
join is the whole point.

| Registered name | Suggested attribute key |
|---|---|
| `userID` | `user.id` |
| `projectID` | `project.id` (no semconv entry — namespace it) |
| `requestID` | `http.request.id` where it is one |
| `principalID` | `enduser.id` |

## PII

Per-field transforms — `hash`, `truncate`, `redact` — so regulated teams can adopt it.

## Cardinality

Fine as span attributes and log fields. **Never** as metric labels: a user id as a
Prometheus label is a cardinality bomb.
