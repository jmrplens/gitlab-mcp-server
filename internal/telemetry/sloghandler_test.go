package telemetry

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
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

// TestSlogHandler_ExportedRecordsCarryNoResourceURI is the regression for a
// hole that no care in this repository could have closed.
//
// It was found by subscribing on the hosted deployment and reading the
// collector: an exported record carried uri = gitlab://project/82077663, while
// the span for the same subscribe carried the digest. The redaction was
// working, and the log stream went around it.
//
// The record is not ours. The Go SDK writes it, with the URI, through the
// logger this server installs as the default:
//
//	s.opts.Logger.Info("resource subscribed", "uri", req.Params.URI, ...)
//
// That is not a defect in the SDK. A library logging what it did, to the logger
// it was handed, at INFO, is a library behaving correctly. What was ours is
// building a policy for the records we write and then giving a third party the
// pen, so this is enforced where every exported record passes, and the fixture
// is that exact call rather than an invented one.
func TestSlogHandler_ExportedRecordsCarryNoResourceURI(t *testing.T) {
	tests := []struct {
		name    string
		log     func(*slog.Logger)
		wantOut string
	}{
		{
			name: "the SDK's own subscribe record",
			log: func(l *slog.Logger) {
				l.Info("resource subscribed", "uri", "gitlab://project/82077663",
					"session_id", "ABC", "request_id", 20)
			},
			wantOut: "gitlab://[redacted]",
		},
		{
			name: "a URI in the message rather than an attribute",
			log: func(l *slog.Logger) {
				l.Info("could not read gitlab://project/1/mr/2")
			},
			wantOut: "gitlab://[redacted]",
		},
		{
			name: "a URI inside a group",
			log: func(l *slog.Logger) {
				l.Info("watcher stopped", slog.Group("subscription",
					slog.String("uri", "gitlab://project/1")))
			},
			wantOut: "gitlab://[redacted]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, exp := newRecordingLogger(t)
			tc.log(logger)

			records := exp.all()
			if len(records) != 1 {
				t.Fatalf("exported %d records, want 1", len(records))
			}

			rendered := renderRecord(records[0])
			if strings.Contains(rendered, "82077663") || strings.Contains(rendered, "project/1") {
				t.Errorf("the exported record names a project: %s", rendered)
			}
			if !strings.Contains(rendered, tc.wantOut) {
				t.Errorf("the exported record does not carry the marker; got %s", rendered)
			}
		})
	}
}

// TestSlogHandler_StderrKeepsTheResourceURI pins the other half, which is the
// reason this is a redaction on one leg and not a removal.
//
// stderr is the operator's own terminal and their container platform's log.
// Blanking it there would take the diagnosis with it: "could not read
// gitlab://[redacted]" tells somebody debugging their own deployment nothing
// they did not already know.
func TestSlogHandler_StderrKeepsTheResourceURI(t *testing.T) {
	var out bytes.Buffer
	base := slog.NewJSONHandler(&out, nil)

	exp := &recordingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })
	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(lp)
	t.Cleanup(func() { global.SetLoggerProvider(previous) })

	slog.New(NewSlogHandler(base, slog.LevelInfo)).
		Info("resource subscribed", "uri", "gitlab://project/82077663")

	if !strings.Contains(out.String(), "gitlab://project/82077663") {
		t.Errorf("stderr lost the URI, which is where an operator needs it: %s", out.String())
	}
}

// TestRedactResourceURIs_LeavesEverythingElseAlone keeps the substitution from
// becoming a blunt instrument.
func TestRedactResourceURIs_LeavesEverythingElseAlone(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "no URI is untouched", in: "rate limit exceeded", want: "rate limit exceeded"},
		{name: "a bare URI", in: "read gitlab://project/1", want: "read gitlab://[redacted]"},
		{
			name: "two in one string",
			in:   "gitlab://project/1 and gitlab://group/2",
			want: "gitlab://[redacted] and gitlab://[redacted]",
		},
		{
			name: "a quoted URI stops at the quote",
			in:   `read "gitlab://project/1" failed`,
			want: `read "gitlab://[redacted]" failed`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactResourceURIs(tc.in); got != tc.want {
				t.Errorf("RedactResourceURIs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// renderRecord flattens a record into one string, so an assertion about what
// must not appear looks at the message and every attribute rather than at
// whichever one the author remembered.
func renderRecord(record sdklog.Record) string {
	parts := []string{record.Body().AsString()}
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		parts = append(parts, string(kv.Key)+"="+kv.Value.String())
		return true
	})
	return strings.Join(parts, " ")
}
