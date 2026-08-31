package mcpotel

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// driveSession runs one real MCP session end to end over the SDK's in-memory
// transports and closes it.
//
// Real sessions rather than fakes, because the whole instrument hangs off
// ServerSession.Wait: a fake would let this test pass while the goroutine that
// records the measurement never unblocks in production. The client side is
// closed rather than the server side, which is what a departing client does.
func driveSession(t *testing.T, transport string) {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddReceivingMiddleware(Middleware(Options{Transport: transport}))

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	// One request, so the middleware sees the session and starts the clock.
	if _, listErr := clientSession.ListTools(ctx, nil); listErr != nil {
		t.Fatalf("tools/list: %v", listErr)
	}

	if closeErr := clientSession.Close(); closeErr != nil {
		t.Fatalf("client close: %v", closeErr)
	}
	_ = serverSession.Wait()
}

// awaitMetric polls for an instrument, because the measurement is recorded on a
// goroutine that unblocks when the session ends rather than inside the request.
//
// A single Collect right after Close is a race this test would lose most of the
// time and win occasionally, which is the worst kind of flake: it would pass in
// development and fail in CI.
func awaitMetric(t *testing.T, reader *metric.ManualReader, name string) (metricdata.Metrics, bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for {
		var collected metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &collected); err != nil {
			t.Fatalf("collecting metrics: %v", err)
		}
		for _, scope := range collected.ScopeMetrics {
			for _, m := range scope.Metrics {
				if m.Name == name {
					return m, true
				}
			}
		}
		if time.Now().After(deadline) {
			return metricdata.Metrics{}, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSessionDuration_IsRecordedOnStdio covers the convention's second
// server-side instrument, which this server did not emit at all.
//
// On stdio the session is the process lifetime, which is the case where the
// measurement says something no other instrument does: how long a client stayed
// connected. The SDK offers no session-lifecycle hook, so the value of the test
// is that it drives a real session and asserts the goroutine parked on Wait
// actually fires.
func TestSessionDuration_IsRecordedOnStdio(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	driveSession(t, TransportPipe)

	recorded, ok := awaitMetric(t, reader, "mcp.server.session.duration")
	if !ok {
		t.Fatal("mcp.server.session.duration was never recorded; the convention defines it as the second server-side instrument")
	}
	if recorded.Unit != "s" {
		t.Errorf("unit = %q, want %q", recorded.Unit, "s")
	}

	histogram, ok := recorded.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("data is %T, want a float64 histogram", recorded.Data)
	}
	if len(histogram.DataPoints) != 1 {
		t.Fatalf("recorded %d data points, want 1 per session", len(histogram.DataPoints))
	}

	// mcp.session.id is deliberately not among the metric's attributes: the
	// convention's instrument table omits it, and it is one value per client.
	for _, kv := range histogram.DataPoints[0].Attributes.ToSlice() {
		if kv.Key == AttrMCPSessionID {
			t.Errorf("mcp.session.id is a metric dimension; it is one time series per connected client")
		}
	}
}

// TestSessionDuration_IsSkippedForAStatelessPost is the other half, and the
// reason the instrument is not simply always on.
//
// Under the default stateless HTTP transport every POST is its own session that
// ends with its response. Recording those would produce a histogram of request
// durations under a name that promises session durations, duplicating
// mcp.server.operation.duration and misleading anyone who plotted it. A
// stateless session has no id, which is exactly how it is recognized.
func TestSessionDuration_IsSkippedForAStatelessPost(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	// TransportTCP with an in-memory session, which carries no session id: the
	// same shape a stateless POST presents to the middleware.
	driveSession(t, TransportTCP)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "mcp.server.session.duration" {
				t.Error("a stateless POST was recorded as a session; this duplicates mcp.server.operation.duration under a misleading name")
			}
		}
	}
}
