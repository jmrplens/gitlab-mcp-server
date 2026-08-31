package telemetry

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
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

// TestSlogHandler_ADerivedLoggerStillExports is the test the handler's own doc
// comment asks for and did not have.
//
// It says of WithAttrs and WithGroup: "Forgetting either would produce a handler
// that works until somebody calls slog.With, which is the ordinary way to build
// a component logger." Both methods were at zero coverage, so that sentence was
// a claim about untested code, which is the shape of defect this work has found
// three times already.
//
// It matters more here than the ordinary case for losing a method. Every
// component in this server builds its logger with slog.With, so a fan-out that
// dropped its OTLP leg there would leave the stderr stream complete and the
// exported stream missing almost everything, with the startup lines still
// arriving to prove telemetry was working.
func TestSlogHandler_ADerivedLoggerStillExports(t *testing.T) {
	tests := []struct {
		name   string
		derive func(*slog.Logger) *slog.Logger
		want   func(*testing.T, sdklog.Record)
	}{
		{
			name:   "the base logger",
			derive: func(l *slog.Logger) *slog.Logger { return l },
		},
		{
			name:   "one derived with attributes",
			derive: func(l *slog.Logger) *slog.Logger { return l.With("component", "telemetry") },
			want: func(t *testing.T, record sdklog.Record) {
				t.Helper()
				if !recordHasAttr(record, "component", "telemetry") {
					t.Error("the attribute added by slog.With did not reach the exported record")
				}
			},
		},
		{
			name:   "one derived twice",
			derive: func(l *slog.Logger) *slog.Logger { return l.With("component", "telemetry").With("phase", "startup") },
			want: func(t *testing.T, record sdklog.Record) {
				t.Helper()
				for key, value := range map[string]string{"component": "telemetry", "phase": "startup"} {
					if !recordHasAttr(record, key, value) {
						t.Errorf("%s=%s did not reach the exported record", key, value)
					}
				}
			},
		},
		{
			name:   "one derived with a group",
			derive: func(l *slog.Logger) *slog.Logger { return l.WithGroup("gitlab") },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, exp := newRecordingLogger(t)

			tp := sdktrace.NewTracerProvider()
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
			ctx, span := tp.Tracer("test").Start(context.Background(), "tools/call")

			tc.derive(logger).InfoContext(ctx, "tool call completed")
			span.End()

			records := exp.all()
			if len(records) != 1 {
				t.Fatalf("exported %d records, want 1: the derived handler lost its OTLP leg", len(records))
			}

			// The correlation has to survive derivation too. A component logger
			// that exports records with no span is the same defect one level
			// down.
			if records[0].TraceID() != span.SpanContext().TraceID() {
				t.Error("the derived handler exported a record that names no span")
			}
			if tc.want != nil {
				tc.want(t, records[0])
			}
		})
	}
}

// TestSlogHandler_ADerivedLoggerKeepsTheExportFloor pins the other half of
// derivation: the floor is carried, not reset.
//
// A derived handler that lost it would start exporting debug records, which is
// the failure the floor exists to prevent and the one nobody would notice until
// a collector bill arrived.
func TestSlogHandler_ADerivedLoggerKeepsTheExportFloor(t *testing.T) {
	exp := &recordingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(lp)
	t.Cleanup(func() { global.SetLoggerProvider(previous) })

	base := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(NewSlogHandler(base, slog.LevelInfo)).With("component", "telemetry")

	logger.DebugContext(context.Background(), "per-request detail")

	if got := len(exp.all()); got != 0 {
		t.Errorf("exported %d records, want 0: the derived handler reset the export floor", got)
	}
}

// recordHasAttr reports whether a record carries a string attribute.
func recordHasAttr(record sdklog.Record, key, value string) bool {
	found := false
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == key && kv.Value.AsString() == value {
			found = true
			return false
		}
		return true
	})
	return found
}
