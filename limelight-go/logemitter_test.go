package limelight

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func jsonLogger(b *strings.Builder, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(b, &slog.HandlerOptions{Level: level}))
}

func sampleEvent() Event {
	return Event{
		Version: EventVersion,
		Method:  "main.Service.CreateThing",
		Fields:  map[string]string{"user.id": "u-42", "project.id": "abc-123"},
	}
}

func TestLogEmitterShape(t *testing.T) {
	var buf strings.Builder
	NewLogEmitter(WithLogger(jsonLogger(&buf, slog.LevelDebug))).Emit(context.Background(), sampleEvent())

	var got struct {
		Msg     string            `json:"msg"`
		Version int               `json:"version"`
		Method  string            `json:"method"`
		Fields  map[string]string `json:"fields"`
		TraceID string            `json:"trace_id"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &got); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if got.Msg != "limelight" || got.Method != "main.Service.CreateThing" || got.Version != EventVersion {
		t.Errorf("got %+v", got)
	}
	if got.Fields["user.id"] != "u-42" || got.Fields["project.id"] != "abc-123" {
		t.Errorf("fields = %v", got.Fields)
	}
	// Empty trace ids are omitted rather than emitted as "", so a consumer can
	// tell "no span" from "a span with a blank id".
	if strings.Contains(buf.String(), "trace_id") || strings.Contains(buf.String(), "span_id") {
		t.Errorf("empty trace ids were emitted: %s", buf.String())
	}
}

func TestLogEmitterFieldsAreSorted(t *testing.T) {
	var buf strings.Builder
	NewLogEmitter(WithLogger(jsonLogger(&buf, slog.LevelDebug))).Emit(context.Background(), sampleEvent())
	line := buf.String()
	if strings.Index(line, "project.id") > strings.Index(line, "user.id") {
		t.Errorf("fields are not sorted by key: %s", line)
	}
}

func TestLogEmitterLevel(t *testing.T) {
	var buf strings.Builder
	// Default is Info, so a handler admitting only Warn drops it.
	NewLogEmitter(WithLogger(jsonLogger(&buf, slog.LevelWarn))).Emit(context.Background(), sampleEvent())
	if buf.Len() != 0 {
		t.Errorf("default level should be Info, got output: %s", buf.String())
	}

	buf.Reset()
	NewLogEmitter(
		WithLogger(jsonLogger(&buf, slog.LevelWarn)),
		WithLevel(slog.LevelError),
	).Emit(context.Background(), sampleEvent())
	if !strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Errorf("WithLevel ignored: %s", buf.String())
	}
}

// Without a logger the emitter must resolve slog.Default() at emit time, so a
// process that calls slog.SetDefault after start-up is still honoured.
func TestLogEmitterFollowsTheDefaultLogger(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })

	e := NewLogEmitter() // built before the default is replaced
	var buf strings.Builder
	slog.SetDefault(jsonLogger(&buf, slog.LevelDebug))
	e.Emit(context.Background(), sampleEvent())

	if !strings.Contains(buf.String(), "main.Service.CreateThing") {
		t.Errorf("emitter latched the old default logger: %q", buf.String())
	}
}

func TestDiscardEmitterAndNilSetEmitter(t *testing.T) {
	var buf strings.Builder
	slog.SetDefault(jsonLogger(&buf, slog.LevelDebug))
	DiscardEmitter.Emit(context.Background(), sampleEvent())
	if buf.Len() != 0 {
		t.Errorf("DiscardEmitter wrote: %s", buf.String())
	}

	c := reset(t)
	SetEmitter(nil) // must fall back to discarding, not panic on a nil interface
	registerBoth()
	if _, err := Enable(Config{TTL: time.Minute, Match: map[string]string{"projectID": "abc-123"}}); err != nil {
		t.Fatal(err)
	}
	Emit(ctxWith("abc-123", "u-42"), "p.S.M", "projectID")
	if got := c.drain(); len(got) != 0 {
		t.Errorf("collector saw events after SetEmitter(nil): %+v", got)
	}
}
