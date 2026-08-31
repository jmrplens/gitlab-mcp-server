//go:build collectore2e

package collectore2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// documentedButNotDrivenHere are names the guide documents that this module
// cannot produce, each with the reason.
//
// The list is the review. Anything on it is a claim in the documentation that
// nothing here checks, so it should be short and every entry should be
// uncomfortable to write. Adding a name to silence a failure is the failure.
var documentedButNotDrivenHere = map[string]string{}

// TestRealCollector_EveryDocumentedNameIsReallyEmitted turns the operator guide
// into an artifact this suite checks.
//
// The reason is a defect that has now appeared three times in this work, each
// time as something that read as implemented and was not: an attribute key
// declared and set by nothing, a second one the same, and a safe-mode path that
// accepted a context and discarded it. None produced a compiler warning,
// because Go says nothing about an unused declaration, and none produced a test
// failure, because nothing asserted the claim.
//
// Documentation is where that class is most expensive. A reader has no way to
// tell a documented attribute from an emitted one, and an operator building a
// dashboard on a name that is never exported gets an empty panel and no reason.
// So the guide's own tables are read here and every name in them has to appear
// in something a real collector parsed.
func TestRealCollector_EveryDocumentedNameIsReallyEmitted(t *testing.T) {
	guide := readGuide(t)
	documented := documentedNames(t, guide)
	if len(documented) < 8 {
		t.Fatalf("only %d names were parsed out of the guide; the tables have probably changed shape and this test is now checking nothing",
			len(documented))
	}

	// One collector, two servers. Some documented names exist only under a
	// stateful transport and some only under a stateless one, so a sweep that
	// ran a single server would have to excuse the other half, and an exclusion
	// list that fills up with bookkeeping stops being a review. Pointing both
	// at one collector makes the union happen where the names are read.
	c := startCollector(t)
	driveStatelessNames(t, c)
	driveStatefulNames(t, c)

	emitted := emittedNames(t, c)

	for _, name := range documented {
		if emitted[name] {
			continue
		}
		if reason, excused := documentedButNotDrivenHere[name]; excused {
			t.Logf("not driven here: %s (%s)", name, reason)
			continue
		}
		t.Errorf("the guide documents %q and nothing exported it. Either it is not wired, or this module does not drive the path that emits it and the exclusion list should say so",
			name)
	}
}

// readGuide returns the operator guide's text.
func readGuide(t *testing.T) string {
	t.Helper()

	path := filepath.Join(repoRoot(), "docs", "guides", "telemetry.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the telemetry guide: %v", err)
	}
	return string(body)
}

// documentedNameRow matches a table row whose first cell is a backticked name.
var documentedNameRow = regexp.MustCompile(`(?m)^\|\s*` + "`" + `([a-z][a-z0-9_.]+)` + "`" + `\s*\|`)

// documentedNames extracts the attribute and instrument names the guide's
// tables list.
//
// Read out of the tables rather than kept in a second list here, so the guide
// stays the single statement of what this server records. A list in the test
// would be a copy, and a copy is the thing that drifts.
func documentedNames(t *testing.T, guide string) []string {
	t.Helper()

	seen := map[string]bool{}
	var names []string
	for _, match := range documentedNameRow.FindAllStringSubmatch(guide, -1) {
		name := match[1]
		// Only dotted names: the tables also carry environment variables and
		// flag names, which are not telemetry keys.
		if !strings.Contains(name, ".") || strings.HasPrefix(name, "otel_") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// emittedNames collects every attribute key and instrument name the collector
// parsed, across all three signals.
func emittedNames(t *testing.T, c *collector) map[string]bool {
	t.Helper()

	found := map[string]bool{}
	collectSpanNames(t, c, found)
	collectMetricNames(t, c, found)
	return found
}

// collectSpanNames adds every resource and span attribute key to found.
func collectSpanNames(t *testing.T, c *collector, found map[string]bool) {
	t.Helper()

	for _, doc := range documents[traceDocument](t, filepath.Join(c.outDir, tracesFile)) {
		for _, rs := range doc.ResourceSpans {
			addKeys(found, rs.Resource.Attributes)
			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					addKeys(found, span.Attributes)
				}
			}
		}
	}
}

// collectMetricNames adds every instrument name and dimension key to found.
func collectMetricNames(t *testing.T, c *collector, found map[string]bool) {
	t.Helper()

	for _, doc := range documents[metricDocument](t, metricsPath(c)) {
		for _, rm := range doc.ResourceMetrics {
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					found[m.Name] = true
					for _, point := range dataPointAttributes(t, m) {
						addKeys(found, point)
					}
				}
			}
		}
	}
}

// addKeys records the keys of one attribute set.
func addKeys(found map[string]bool, attrs []otlpAttr) {
	for _, kv := range attrs {
		found[kv.Key] = true
	}
}

// driveStatelessNames exercises everything the default transport emits.
//
// The identity policy that records the most, so a name missing afterwards is
// missing because nothing emits it rather than because a policy suppressed it.
func driveStatelessNames(t *testing.T, c *collector) {
	t.Helper()

	env := telemetryEnv(c)
	env["GITLAB_MCP_TELEMETRY_IDENTITY"] = "full"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	// A tool call, a prompt fetch and a resource read each carry attributes the
	// others do not.
	for i := range 3 {
		srv.callAction(t, i*4+1, "issue.list", "some-group/some-project")
		srv.readResource(t, i*4+2, resourceURI)
		srv.getPromptExpectingRefusal(t, i*4+3, "not-a-prompt-this-server-has")
		// A GitLab failure, which is the only thing that produces error.type:
		// a caller fault deliberately does not, since a model naming an action
		// that does not exist is an ordinary event rather than a malfunction.
		srv.callToolTolerant(t, i*4+4, "gitlab_execute_action",
			`{"action":"issue.list","params":{"project_id":"`+failingProject+`"}}`)
	}

	if _, _, ok := c.awaitMetric(t, exportDeadline, durationMetric); !ok {
		t.Fatalf("no metric arrived from the stateless server.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}
}

// driveStatefulNames exercises what only exists when a deployment has sessions:
// the session identifier, the session instrument, and the client span a
// server-initiated notification produces.
//
// This is the slow half, and unavoidably so: the notification comes from a
// subscription noticing a change, and the watcher's base cadence is fifteen
// seconds.
func driveStatefulNames(t *testing.T, c *collector) {
	t.Helper()

	fake := startMutableFakeGitLab(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+fake.URL(), "--stateless=false")

	sess := srv.openSession(t)
	sess.call(t, 2, "resources/subscribe", `{"uri":"`+resourceURI+`"}`)
	fake.change("changed so the watcher has something to notice")

	if _, _, ok := c.awaitSpan(t, 90*time.Second, func(_ otlpResourceSpans, s otlpSpan) bool {
		return s.Kind == spanKindClient && strings.HasPrefix(s.Name, "notifications/")
	}); !ok {
		t.Fatalf("no server-initiated notification arrived.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	// Ending the session is what records its duration.
	sess.close(t)
	if _, _, ok := c.awaitMetric(t, exportDeadline, "mcp.server.session.duration"); !ok {
		t.Fatalf("no session duration arrived.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}
}
