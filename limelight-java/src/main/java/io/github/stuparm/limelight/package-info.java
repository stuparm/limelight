/**
 * limelight Java SDK: the extractor registry, the TTL-bounded
 * identity-targeted switch, and the {@code @Limelight} annotation.
 *
 * <p>Java needs no codemod — {@code @WithSpan} and {@code @SpanAttribute} from
 * the OpenTelemetry javaagent already do the tagging half. So this SDK is
 * mostly the part that is actually the product: the switch. Note the agent's
 * AOP-proxy limit — annotations are honored on Spring-managed beans called
 * from outside, not on internal self-calls.
 *
 * <p>Context here is the OpenTelemetry {@code Context} / a ThreadLocal, not an
 * explicit parameter, which is exactly why the shared contract in
 * {@code spec/} is written in terms of field names rather than ctx keys.
 */
package io.github.stuparm.limelight;
