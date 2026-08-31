package toolutil_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// capturingExporter keeps the log records the SDK exports so the test can read
// what a collector would have received.
type capturingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *capturingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, records...)
	return nil
}

func (e *capturingExporter) Shutdown(context.Context) error   { return nil }
func (e *capturingExporter) ForceFlush(context.Context) error { return nil }

func (e *capturingExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// TestLogToolCallAll_InsideASpan_ExportsACorrelatedRecord is the regression that
// a handler-level test could not be.
//
// The fan-out handler was always correct: hand it a context carrying a span and
// it exports a correlated record. What was wrong was every caller, because the
// server logged through the context-free slog.Info family, and
// LogToolCallAll in particular accepted a context and then dropped it on the
// floor by delegating to helpers that took none.
//
// Nothing in the package's own tests could see that. The bridge passed, the
// records were exported, the log stream on stderr was unchanged, and a
// collector receiving real traffic showed 235 records with not one trace ID
// among them. So the assertion here deliberately goes through the exported
// entry point a tool handler actually calls, rather than through the handler
// the previous test covers.
func TestLogToolCallAll_InsideASpan_ExportsACorrelatedRecord(t *testing.T) {
	exp := &capturingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(lp)
	t.Cleanup(func() { global.SetLoggerProvider(previous) })

	handler := telemetry.NewSlogHandler(slog.NewJSONHandler(io.Discard, nil), slog.LevelInfo)
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previousDefault) })

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "tools/call gitlab_execute_action")
	toolutil.LogToolCallAll(ctx, nil, "gitlab_execute_action", time.Now(), nil, nil)
	span.End()

	records := exp.all()
	if len(records) != 1 {
		t.Fatalf("exported %d records, want 1", len(records))
	}

	want := span.SpanContext().TraceID()
	if got := records[0].TraceID(); got != want {
		t.Errorf("record trace ID = %v, want %v: a tool call log cannot be joined to the span it happened inside", got, want)
	}
}
