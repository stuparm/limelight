# Enable protocol (v0 draft)

Turn emission on for a *targeted* subset of traffic, for a *bounded* time.

```http
POST /debug/limelight/enable
Content-Type: application/json

{
  "version": 0,
  "ttl": "10m",
  "match": { "projectID": "abc-123" },
  "methods": ["vnet.*"]
}
```

| Field | Meaning |
|---|---|
| `version` | protocol version; reject unknown values |
| `ttl` | duration string; server caps it (suggested max 1h) |
| `match` | field name → value. Evaluated by the **same registered extractors** that produce output, so targeting and emission can never disagree. Empty `match` means everything — a firehose; require it to be explicit. |
| `methods` | optional glob over tagged method names; absent means all tagged methods |

Response names the scope it actually achieved:

```json
{ "enabled": true, "scope": "pod", "instance": "vnet-rpc-server-7d9f-x2k4", "expiresAt": "..." }
```

## Open in v0

- **Multi-replica is the #1 gotcha.** A POST behind a load balancer enables one pod;
  20 replicas means 5% of traffic and a confused user. v0 is honest per-pod scope and
  says so in the response. Options later: a shared store polled every few seconds, or
  control-plane fan-out ([OpAMP](https://opentelemetry.io/docs/specs/opamp/)).
- `POST /debug/limelight/disable` and a `GET` for current state.
- **Security is not optional here**: this endpoint turns on emission of user
  identifiers and can flood a logging bill. Authenticate it, rate-limit it, cap the
  TTL server-side, audit who enabled what.
