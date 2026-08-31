package telemetry

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// recordingExporter keeps every log record the SDK exports, so a test can read
// back what a collector would have received.
//
// It is written here rather than pulled from sdk/log/logtest because that is a
// separate module: one interface with three methods is cheaper than a
// dependency, and this way the test exercises the same Exporter contract the
// real OTLP exporter implements.
type recordingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *recordingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, records...)
	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error   { return nil }
func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// newRecordingLogger installs a logger provider that records, and returns a
// slog.Logger writing through the fan-out handler plus the recorder.
//
// The stderr leg is io.Discard: this test is about what reaches the collector,
// and a test that also printed to the terminal would bury its own output.
func newRecordingLogger(t *testing.T) (*slog.Logger, *recordingExporter) {
	t.Helper()

	exp := &recordingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(lp)
	t.Cleanup(func() { global.SetLoggerProvider(previous) })

	handler := NewSlogHandler(slog.NewJSONHandler(io.Discard, nil), slog.LevelInfo)
	return slog.New(handler), exp
}

// TestSlogHandler_RecordInsideASpan_CarriesTheTraceID is the regression for a
// defect the unit tests could not see and a live deployment could.
//
// The OTLP leg of this handler exists for one reason, stated in its own doc
// comment: it "correlates a log record with the span it happened inside, which
// stderr cannot do at all". That correlation is carried by the trace and span
// IDs on the exported record, and otelslog reads them from the context it is
// handed. So a server that logs through the context-free slog.Info family
// exports records with no trace ID at all, and the whole reason for the leg
// evaporates while every test still passes.
//
// This is what a collector receiving real traffic showed: 235 records, none of
// them correlated. The assertion is therefore on the exported record rather
// than on the handler's plumbing, because the plumbing was already correct.
func TestSlogHandler_RecordInsideASpan_CarriesTheTraceID(t *testing.T) {
	logger, exp := newRecordingLogger(t)

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "tools/call")
	logger.InfoContext(ctx, "tool call completed")
	span.End()

	records := exp.all()
	if len(records) != 1 {
		t.Fatalf("exported %d records, want 1", len(records))
	}

	want := span.SpanContext()
	if got := records[0].TraceID(); got != want.TraceID() {
		t.Errorf("record trace ID = %v, want %v: the record cannot be joined to its span", got, want.TraceID())
	}
	if got := records[0].SpanID(); got != want.SpanID() {
		t.Errorf("record span ID = %v, want %v", got, want.SpanID())
	}
}

// TestSlogHandler_RecordWithoutAContext_HasNoTraceID pins the other half of the
// same fact, so the cause stays documented rather than becoming folklore.
//
// slog.Info and friends pass context.Background(), which carries no span. The
// record is still exported, which is why the defect was invisible: the log
// pipeline looked healthy from every angle except the one that mattered. A
// future rewrite that reintroduces the context-free form will fail the test
// above and this one explains why.
func TestSlogHandler_RecordWithoutAContext_HasNoTraceID(t *testing.T) {
	logger, exp := newRecordingLogger(t)

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "tools/call")
	logger.Info("tool call completed")
	span.End()

	records := exp.all()
	if len(records) != 1 {
		t.Fatalf("exported %d records, want 1", len(records))
	}
	if records[0].TraceID().IsValid() {
		t.Errorf("record carries trace ID %v; a context-free log call cannot know a span", records[0].TraceID())
	}
}

// TestSlogHandler_BelowTheFloor_IsNotExported asserts the export floor still
// applies to the context-carrying form, so the rewrite that added contexts did
// not also widen what leaves the process.
func TestSlogHandler_BelowTheFloor_IsNotExported(t *testing.T) {
	exp := &recordingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(lp)
	t.Cleanup(func() { global.SetLoggerProvider(previous) })

	// The stderr leg is set to debug so it accepts the record: the point is that
	// the OTLP leg declines it on its own floor, not that nobody logged it.
	base := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(NewSlogHandler(base, slog.LevelInfo))

	logger.DebugContext(context.Background(), "per-request detail")

	if got := len(exp.all()); got != 0 {
		t.Errorf("exported %d records, want 0: debug is below the export floor", got)
	}
}
