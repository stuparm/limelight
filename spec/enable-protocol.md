# Enable protocol (v0 draft)

Turn emission on for a *targeted* subset of traffic, for a *bounded* time.

```http
POST /debug/limelight/enable
Content-Type: application/json

{
  "version": 0,
  "ttl": "10m",
  "match": { "projectID": "abc-123" },
  "methods": ["billing.*"]
}
```

| Field | Meaning |
|---|---|
| `version` | protocol version; reject unknown values. Read it **before** validating anything else, so a client speaking a later version is told which version you speak rather than which of its fields you failed to recognise. |
| `ttl` | duration string; server caps it (suggested max 1h) |
| `match` | field name → value. Evaluated by the **same registered extractors** that produce output, so targeting and emission can never disagree. |
| `match_all` | emit for every call, whatever identity it carries — the firehose. Mutually exclusive with `match`; exactly one of the two is required. |
| `methods` | optional glob over tagged method names; absent means all tagged methods |

An empty `match` is **not** the way to ask for everything. An empty map is what a
dropped field marshals to, so the cheapest bug upstream would produce the most expensive
outcome available. The firehose gets its own named field that someone had to type, and a
request carrying neither is rejected.

Because `match_all` emits user identifiers for every request the process serves, it is
capped far more tightly than a targeted activation — **suggested max 60s** against the
hour allowed for `match`. Long enough to answer "is anything coming through at all",
short enough to be uninteresting to a log bill.

Response names the scope it actually achieved, and says which kind of targeting is live
so an audit log of enable responses shows firehoses plainly:

```json
{
  "enabled": true,
  "scope": "pod",
  "instance": "api-server-7d9f-x2k4",
  "targeting": "matched",
  "expires_at": "..."
}
```

`targeting` is `"matched"` or `"all"`.

## Naming

Protocol field names are **snake_case** — `match_all`, `expires_at`, `trace_id`. The keys
*inside* `match` are not protocol names at all: they are the field names the service
registered, so they look however that codebase spells them (`projectID` above). Do not
normalise them.

## Open in v0

- **Multi-replica is the #1 gotcha.** A POST behind a load balancer enables one pod;
  20 replicas means 5% of traffic and a confused user. v0 is honest per-pod scope and
  says so in the response. Options later: a shared store polled every few seconds, or
  control-plane fan-out ([OpAMP](https://opentelemetry.io/docs/specs/opamp/)).
- `POST /debug/limelight/disable` and a `GET` for current state.
- **Security is not optional here**: this endpoint turns on emission of user
  identifiers and can flood a logging bill. Authenticate it, rate-limit it, cap the
  TTL server-side, audit who enabled what.
