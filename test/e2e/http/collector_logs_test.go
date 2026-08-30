//go:build httpe2e

package httpe2e

import (
	"strings"
	"testing"
	"time"
)

// TestCollectorLogs_TheAnnouncedSignalActuallyProducesSomething is the
// regression for a defect that no unit test could have caught and that every
// other telemetry test passed straight through.
//
// The server started a logger provider, logged "signals: traces, metrics,
// logs" at startup, and published the same list on its server card. Nothing
// bridged slog into that provider, so the logs signal was announced and always
// empty. Every assertion about traces and metrics passed, the card looked
// right, and an operator would have gone looking for records that were never
// going to arrive.
//
// A real OTLP receiver is the only thing that can see this: the gap is between
// what the process says it does and what reaches a collector, and both halves
// look correct from inside.
func TestCollectorLogs_TheAnnouncedSignalActuallyProducesSomething(t *testing.T) {
	c := startCollector(t)
	env := collectorEnv(c)
	env["OTEL_BLRP_SCHEDULE_DELAY"] = "100"
	srv := startServer(t, env)

	// Any traffic at all: the server logs a record per request, and startup
	// alone already produces several.
	for range 3 {
		srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range c.received() {
			if e.path == "/v1/logs" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	var paths []string
	for _, e := range c.received() {
		paths = append(paths, e.path)
	}
	t.Errorf("no log record reached the collector, though the server advertises the logs signal; it exported to %s",
		strings.Join(paths, ", "))
}

// TestCollectorLogs_DebugRecordsAreNotExported pins the severity floor.
//
// The stderr leg and the collector leg are filtered differently on purpose. An
// operator running at debug wants everything on their terminal, and almost
// certainly does not want a record per GitLab round trip on their collector on
// top of a span describing the same call. The specification's one passage
// addressed to end users says as much: "Logging could consume much memory by
// default if the end user application emits too many logs".
//
// The assertion is indirect because a protobuf payload is searched rather than
// decoded: a debug-only message that appears on stderr must not appear in any
// exported payload.
func TestCollectorLogs_DebugRecordsAreNotExported(t *testing.T) {
	c := startCollector(t)
	env := collectorEnv(c)
	env["LOG_LEVEL"] = "debug"
	env["OTEL_BLRP_SCHEDULE_DELAY"] = "100"
	srv := startServer(t, env)

	for range 3 {
		srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
	}
	time.Sleep(1500 * time.Millisecond)

	logs := srv.logs()
	if !strings.Contains(logs, `"level":"DEBUG"`) {
		t.Skip("the server emitted no debug record, so there is nothing to check the floor against")
	}

	for _, e := range c.received() {
		if e.path != "/v1/logs" {
			continue
		}
		if strings.Contains(string(e.body), "DEBUG") {
			t.Errorf("a debug record reached the collector; the export floor is not being applied")
		}
	}
}
