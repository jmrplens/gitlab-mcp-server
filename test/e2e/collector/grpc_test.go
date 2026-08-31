//go:build collectore2e

package collectore2e

import (
	"strings"
	"testing"
)

// TestRealCollector_GRPCIsAnExportPathToo drives the protocol nothing here had
// driven.
//
// resolveProtocol builds a gRPC exporter for each signal when the environment
// asks for one, and every case in this module and every unit test speaks
// http/protobuf. So the whole gRPC path could be broken and the suite would
// stay green: the exporters are constructed, the configuration is honored, and
// nothing ever put a byte through them.
//
// It is not a variation on a theme. A different transport means different
// framing, different error surfaces and a different port, and the endpoint is a
// trap in the other direction from the obvious one: the programmatic option
// takes a bare host and port, and this environment variable requires a URL for
// every protocol, so an endpoint written the way the Go API accepts it is
// rejected before a single byte is sent.
func TestRealCollector_GRPCIsAnExportPathToo(t *testing.T) {
	c := startCollector(t)

	env := telemetryEnv(c)
	// A URL, with the scheme deciding whether the connection is encrypted.
	//
	// The programmatic option takes a bare host and port, and this variable
	// does not: the configuration specification requires a URL here for every
	// protocol, and the SDK refuses "127.0.0.1:4317" outright with "first path
	// segment in URL cannot contain colon". Measured rather than assumed, after
	// writing it the other way round and watching the exporter reject it.
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://" + c.grpcEndpoint
	env["OTEL_EXPORTER_OTLP_PROTOCOL"] = "grpc"

	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 5 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	t.Run("spans arrive", func(t *testing.T) {
		_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
			return strings.HasPrefix(s.Name, "tools/call")
		})
		if !ok {
			t.Fatalf("no span arrived over gRPC.\nCollector:\n%s\nServer:\n%s",
				c.containerLogs(t), srv.logs())
		}
		// The same attributes as over HTTP. A transport that delivered spans
		// with a different shape would be worse than one that delivered none.
		if got, _ := attr(span.Attributes, "gitlab_mcp.action"); got != "issue.list" {
			t.Errorf("gitlab_mcp.action = %q over gRPC, want issue.list", got)
		}
	})

	t.Run("metrics arrive", func(t *testing.T) {
		if _, duration, ok := c.awaitMetric(t, exportDeadline, durationMetric); !ok {
			t.Fatalf("no metric arrived over gRPC.\nCollector:\n%s", c.containerLogs(t))
		} else if duration.Unit != "s" {
			t.Errorf("unit = %q over gRPC, want %q", duration.Unit, "s")
		}
	})

	t.Run("the collector accepted every export", func(t *testing.T) {
		assertNothingWasRefused(t, c, srv)
	})
}
