package limelightzap_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/limelightzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func sampleEvent() limelight.Event {
	return limelight.Event{
		Version: limelight.EventVersion,
		Method:  "main.Service.CreateThing",
		Fields:  map[string]string{"user.id": "u-42", "project.id": "abc-123"},
	}
}

func observed(t *testing.T, level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(level)
	return zap.New(core), logs
}

func TestEventShape(t *testing.T) {
	logger, logs := observed(t, zapcore.DebugLevel)
	limelightzap.New(logger).Emit(context.Background(), sampleEvent())

	all := logs.All()
	if len(all) != 1 {
		t.Fatalf("got %d entries, want 1", len(all))
	}
	entry := all[0]
	if entry.Message != "limelight" {
		t.Errorf("message = %q, want %q", entry.Message, "limelight")
	}
	if entry.Level != zapcore.InfoLevel {
		t.Errorf("level = %v, want Info (the default)", entry.Level)
	}

	ctx := entry.ContextMap()
	if ctx["method"] != "main.Service.CreateThing" {
		t.Errorf("method = %v", ctx["method"])
	}
	if ctx["version"] != int64(limelight.EventVersion) {
		t.Errorf("version = %v (%T), want %d", ctx["version"], ctx["version"], limelight.EventVersion)
	}
	// Per spec/event-schema.md the extracted fields are a nested object, not
	// flattened into the top level where they could collide with method or
	// version.
	fields, ok := ctx["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields = %v (%T), want a nested object", ctx["fields"], ctx["fields"])
	}
	if fields["project.id"] != "abc-123" || fields["user.id"] != "u-42" {
		t.Errorf("fields = %v", fields)
	}
	// Empty trace ids are omitted, so a consumer can tell "no span" from "a
	// span with a blank id".
	if _, present := ctx["trace_id"]; present {
		t.Error("empty trace_id was emitted")
	}
	if _, present := ctx["span_id"]; present {
		t.Error("empty span_id was emitted")
	}
}

func TestTraceIDsAreEmittedWhenSet(t *testing.T) {
	logger, logs := observed(t, zapcore.DebugLevel)
	ev := sampleEvent()
	ev.TraceID, ev.SpanID = "4bf92f", "00f067"
	limelightzap.New(logger).Emit(context.Background(), ev)

	ctx := logs.All()[0].ContextMap()
	if ctx["trace_id"] != "4bf92f" || ctx["span_id"] != "00f067" {
		t.Errorf("trace ids = %v / %v", ctx["trace_id"], ctx["span_id"])
	}
}

// The same determinism guarantee limelight.NewLogEmitter gives: sorted keys, so
// two runs of the same code produce identical lines.
func TestFieldsAreSorted(t *testing.T) {
	var buf bytes.Buffer
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(&buf), zapcore.DebugLevel)
	limelightzap.New(zap.New(core)).Emit(context.Background(), sampleEvent())

	line := buf.String()
	if strings.Index(line, "project.id") > strings.Index(line, "user.id") {
		t.Errorf("fields are not sorted by key: %s", line)
	}
}

func TestWithLevel(t *testing.T) {
	logger, logs := observed(t, zapcore.DebugLevel)
	limelightzap.New(logger, limelightzap.WithLevel(zapcore.DebugLevel)).
		Emit(context.Background(), sampleEvent())
	if got := logs.All()[0].Level; got != zapcore.DebugLevel {
		t.Errorf("level = %v, want Debug", got)
	}
}

// A core that admits only Warn must drop a default-level event, and the emitter
// must not do the work of building it.
func TestLevelGating(t *testing.T) {
	logger, logs := observed(t, zapcore.WarnLevel)
	limelightzap.New(logger).Emit(context.Background(), sampleEvent())
	if logs.Len() != 0 {
		t.Errorf("emitted %d entries below the core's level", logs.Len())
	}
}

func TestNilLoggerFollowsTheZapGlobal(t *testing.T) {
	logger, logs := observed(t, zapcore.DebugLevel)
	restore := zap.ReplaceGlobals(logger)
	t.Cleanup(restore)

	limelightzap.New(nil).Emit(context.Background(), sampleEvent())
	if logs.Len() != 1 {
		t.Fatalf("got %d entries, want 1 — a nil logger did not reach zap.L()", logs.Len())
	}
}

// End to end through the runtime: the switch, the targeting and the emitter.
func TestThroughTheRuntime(t *testing.T) {
	type projectKey struct{}
	logger, logs := observed(t, zapcore.DebugLevel)

	limelight.Register("projectID", func(ctx context.Context) (string, bool) {
		v, ok := ctx.Value(projectKey{}).(string)
		return v, ok
	}, limelight.As("project.id"))
	limelight.SetEmitter(limelightzap.New(logger))
	t.Cleanup(func() { limelight.Disable() })

	if _, err := limelight.Enable(limelight.Config{
		TTL:   time.Minute,
		Match: map[string]string{"projectID": "abc-123"},
	}); err != nil {
		t.Fatal(err)
	}

	limelight.Emit(context.WithValue(context.Background(), projectKey{}, "abc-123"), "p.S.M", "projectID")
	limelight.Emit(context.WithValue(context.Background(), projectKey{}, "zzz-999"), "p.S.M", "projectID")

	if logs.Len() != 1 {
		t.Fatalf("got %d entries, want 1 — targeting did not reach the zap emitter", logs.Len())
	}
	fields := logs.All()[0].ContextMap()["fields"].(map[string]any)
	if fields["project.id"] != "abc-123" {
		t.Errorf("fields = %v", fields)
	}
}
