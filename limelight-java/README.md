# limelight-java

Java SDK. **Structure only — no code yet.**

Java ships the tagging half already (`@WithSpan` + `@SpanAttribute`, honored by the
OpenTelemetry javaagent), so this SDK is the part that is actually the product:
the extractor registry, the identity-targeted TTL switch, and the enable endpoint.

- Build: Maven (`mvn package`) — requires a JDK 17+.
- Coordinates: `io.github.stuparm:limelight`
- Wire contract shared with every SDK: [`../spec/`](../spec/)
