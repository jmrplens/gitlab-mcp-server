//go:build httpe2e

package httpe2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// export is one OTLP payload the server sent us.
type export struct {
	path    string
	headers http.Header
	body    []byte
}

// collector is an OTLP receiver that keeps what it was sent.
//
// # Why a stub rather than a real collector
//
// A real one in a container would prove that a genuine receiver accepts our
// protobuf, which is worth having and belongs in the Docker-mode suite. It
// would prove less about the two things these tests exist for. The credential
// assertion needs the raw Authorization header, which a collector consumes and
// does not report. The leak assertion needs the payload bytes themselves, and a
// collector forwards them onward rather than handing them back.
//
// This also keeps the httpe2e module what it is: a suite that needs no GitLab,
// no credentials and no daemon, and therefore runs on every push.
//
// # Why searching raw bytes is legitimate
//
// The leak assertions below look for substrings in the protobuf payload rather
// than decoding it. That is not laziness: protobuf encodes strings as UTF-8
// literals with no framing inside them, so a project path that reached any
// attribute, any span name or any log body appears verbatim in these bytes.
// Searching them proves the negative across every field at once, including
// fields nobody thought to check, which a decoder driven by a field list cannot.
type collector struct {
	URL string

	mu      sync.Mutex
	exports []export
}

// startCollector runs an OTLP receiver for the duration of the test.
func startCollector(t *testing.T) *collector {
	t.Helper()

	c := &collector{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		c.mu.Lock()
		c.exports = append(c.exports, export{
			path:    r.URL.Path,
			headers: r.Header.Clone(),
			body:    body,
		})
		c.mu.Unlock()

		// An empty ExportTraceServiceResponse is zero bytes of protobuf, which
		// is what a successful export looks like. Answering anything else makes
		// the exporter retry and the test slower for no reason.
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c.URL = srv.URL
	return c
}

// awaitExport blocks until at least one payload has arrived, or fails.
//
// The batch processors export on a schedule, so a test that read immediately
// would assert against an empty slice and pass for the wrong reason. This is
// the difference between proving the credential was sent and proving nothing
// was sent at all.
func (c *collector) awaitExport(t *testing.T, within time.Duration) []export {
	t.Helper()

	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if got := c.received(); len(got) > 0 {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no OTLP export arrived within %s; nothing was verified", within)
	return nil
}

// received returns what has arrived so far.
func (c *collector) received() []export {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]export(nil), c.exports...)
}

// assertNoPayloadContains fails when any exported payload carries a value that
// must never leave the process.
func (c *collector) assertNoPayloadContains(t *testing.T, forbidden ...string) {
	t.Helper()

	for _, e := range c.received() {
		text := string(e.body)
		for _, secret := range forbidden {
			if strings.Contains(text, secret) {
				t.Errorf("%q reached the collector in a %s payload; it must never leave this process",
					secret, e.path)
			}
		}
	}
}

// collectorEnv points a server at this collector, exporting promptly.
//
// The durations are integers because the specification defines every OTEL_
// timeout as an integer number of milliseconds. Writing "200ms" would parse as
// nothing and silently keep the ten-second default, which here would mean every
// test timing out with no export to inspect.
func collectorEnv(c *collector) map[string]string {
	return map[string]string{
		"GITLAB_MCP_TELEMETRY":        "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": c.URL,
		"OTEL_EXPORTER_OTLP_TIMEOUT":  "2000",
		"OTEL_BSP_SCHEDULE_DELAY":     "100",
		"OTEL_METRIC_EXPORT_INTERVAL": "100",
	}
}
