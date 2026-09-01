//go:build collectore2e

package collectore2e

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The pinned inventories.
//
// # Why an exact set rather than a handful of assertions
//
// Every other assertion in this module names an attribute and checks it is
// there, which catches a removal and is blind to an addition. An addition is the
// dangerous direction: a new attribute is a new metric dimension, a new thing a
// client can influence, and possibly a new thing that should never have left the
// process. Nothing failed when gen_ai.prompt.name started being emitted, and
// nothing would have failed if a project path had started being emitted beside
// it.
//
// So this is a closed set, and adding an attribute is meant to fail here. The
// failure is the review: whoever adds one states, by editing this list, that
// they considered its cardinality and whether it is safe to export.
//
// # Why this module and not a unit test
//
// A unit test asserts what the code passes to the SDK. This asserts what a real
// collector parsed out of a real OTLP export, which is the same thing an
// operator's backend sees and the same thing this project verifies by hand
// against the hosted endpoint. That correspondence is the point: when the two
// disagree, one of them is wrong and the question is which, rather than a shrug.
var (
	// wantToolCallSpanAttributes is what a tools/call span carries on the
	// default dynamic surface with the default identity policy, which records
	// nobody. The identity keys are absent because of that default, and
	// neverOnAnyMetric asserts they stay off metrics under every policy.
	wantToolCallSpanAttributes = []string{
		"gen_ai.operation.name",
		"gen_ai.tool.name",
		"gitlab_mcp.action",
		"gitlab_mcp.tool_surface",
		"mcp.method.name",
		"mcp.protocol.version",
		"network.transport",
	}

	// wantDurationMetricDimensions is the label set of the convention's server
	// instrument. Every entry here multiplies the number of stored time series,
	// so the list is short by design and each addition is a cost decision.
	wantDurationMetricDimensions = []string{
		"gen_ai.operation.name",
		"gen_ai.tool.name",
		"gitlab_mcp.action",
		"gitlab_mcp.tool_surface",
		"mcp.method.name",
		"mcp.protocol.version",
		"network.transport",
	}

	// neverOnAnyMetric are keys that may appear on a span and must never become
	// a metric dimension.
	//
	// mcp.session.id and jsonrpc.request.id are unbounded by construction: one
	// value per connected client, one per request. The user keys are unbounded
	// by the number of people using a deployment, which is exactly the number an
	// operator cannot predict. The SDK does not refuse an exhausted series
	// budget, it collapses the overflow into a single otel.metric.overflow
	// bucket on a first-come-wins basis under cumulative temporality, so the
	// failure mode is silent data destruction rather than an error anyone sees.
	// conditionallyAllowed are keys the convention defines as Conditionally
	// Required, so they are present exactly when their condition holds and
	// absent otherwise. They belong in the inventory as permitted rather than as
	// expected: asserting error.type is always there would demand that every
	// call fail, and asserting it is never there would demand that none does.
	//
	// The traffic this test drives runs against the harness's fake GitLab and
	// fails, which is why error.type shows up here rather than in theory.
	conditionallyAllowed = []string{
		"error.type",
		"rpc.response.status_code",
	}

	neverOnAnyMetric = []string{
		"user.id",
		"user.name",
		"user.hash",
		"mcp.session.id",
		"jsonrpc.request.id",
		"mcp.resource.uri",
		// Added after this list failed to catch the thing it was written for.
		// The resource attribute reached a metric on the hosted endpoint while
		// this test passed, because the list named the convention's key and not
		// the one this server invented for the digest a few commits later. A
		// closed list only closes over what somebody remembered to write in it,
		// which is an argument for keeping it short and reviewing it whenever
		// an attribute is added rather than for trusting it.
		"gitlab_mcp.resource.ref",
		"client.address",
		"client.port",
	}
)

// TestRealCollector_TheExportedInventoryIsExactlyThis pins what leaves this
// process, in both directions, as parsed by a real collector.
func TestRealCollector_TheExportedInventoryIsExactlyThis(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	const action = "issue.list"
	for i := range 5 {
		srv.callAction(t, i+1, action, "some-group/some-project")
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("the collector parsed no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	_, duration, ok := c.awaitMetric(t, exportDeadline, durationMetric)
	if !ok {
		t.Fatalf("the collector parsed no %s metric.\nCollector:\n%s\nServer:\n%s", durationMetric, c.containerLogs(t), srv.logs())
	}

	t.Run("a tool-call span carries exactly the pinned attributes", func(t *testing.T) {
		assertInventory(t, "tools/call span", keys(span.Attributes), wantToolCallSpanAttributes)
	})

	t.Run("the duration metric carries exactly the pinned dimensions", func(t *testing.T) {
		points := dataPointAttributes(t, duration)
		if len(points) == 0 {
			t.Fatalf("%s arrived with no data points", durationMetric)
		}
		for _, point := range points {
			assertInventory(t, durationMetric+" data point", keys(point), wantDurationMetricDimensions)
		}
	})

	t.Run("no unbounded key is a dimension of any metric", func(t *testing.T) {
		for _, forbidden := range neverOnAnyMetric {
			if found := metricDimensionExists(t, c, forbidden); found != "" {
				t.Errorf("%q is a dimension of %s; it is unbounded and mints one time series per distinct value",
					forbidden, found)
			}
		}
	})

	t.Run("the negotiated protocol version is recorded and is one this build serves", func(t *testing.T) {
		version, present := attr(span.Attributes, "mcp.protocol.version")
		if !present {
			t.Fatal("mcp.protocol.version is absent; the convention marks it Recommended and the key was declared long before it was set")
		}
		// Any admitted revision is acceptable: which one the SDK client
		// negotiates is its business, and pinning one here would make this test
		// fail on an SDK bump rather than on a defect of ours.
		if !strings.HasPrefix(version, "202") || len(version) != len("2026-07-28") {
			t.Errorf("mcp.protocol.version = %q, which is not a revision string; an unvalidated caller value reached a metric dimension", version)
		}
	})

	t.Run("a stateless deployment reports no session", func(t *testing.T) {
		// The default HTTP transport is stateless: each POST is its own session
		// with no id, so the convention's condition ("part of a session") is not
		// met and the attribute must be absent rather than invented per POST.
		if value, present := attr(span.Attributes, "mcp.session.id"); present {
			t.Errorf("mcp.session.id = %q on a stateless deployment; there is no session to name", value)
		}
		// Same reasoning for the instrument: a session that is one POST would
		// make this histogram a copy of the operation one under a name that
		// promises something else.
		if _, _, found := c.awaitMetric(t, 2*exportDeadline/10, "mcp.server.session.duration"); found {
			t.Error("mcp.server.session.duration was recorded for stateless POSTs; it duplicates mcp.server.operation.duration")
		}
	})
}

// assertInventory checks both directions, which mean opposite things: a missing
// key is a regression in what gets recorded, an unexpected one is a dimension or
// a disclosure nobody reviewed.
//
// Required keys must all be present. Anything present must be either required or
// conditionally allowed, so a key that arrives from nowhere fails even though a
// conditional one does not.
func assertInventory(t *testing.T, what string, got, want []string) {
	t.Helper()

	gotSorted := slices.Clone(got)
	slices.Sort(gotSorted)
	gotSorted = slices.Compact(gotSorted)

	for _, key := range want {
		if !slices.Contains(gotSorted, key) {
			t.Errorf("%s is missing %q; recorded %v", what, key, gotSorted)
		}
	}
	for _, key := range gotSorted {
		if !slices.Contains(want, key) && !slices.Contains(conditionallyAllowed, key) {
			t.Errorf("%s carries %q, which is not in the pinned inventory. If it belongs there, add it and say why: it becomes a metric dimension or an exported value that nobody has reviewed",
				what, key)
		}
	}
}

// dataPointAttributes decodes the attribute set of each data point.
//
// The harness leaves data points as raw JSON because nothing needed them until
// now, and decoding only the attributes keeps that restraint: the values are
// bucket counts and sums, which this test has no opinion about.
func dataPointAttributes(t *testing.T, m otlpMetric) [][]otlpAttr {
	t.Helper()

	var raws []json.RawMessage
	for _, body := range []*otlpMetricBody{m.Histogram, m.Sum, m.Gauge, m.ExponentialHistogram} {
		if body != nil {
			raws = append(raws, body.DataPoints...)
		}
	}

	out := make([][]otlpAttr, 0, len(raws))
	for _, raw := range raws {
		var point struct {
			Attributes []otlpAttr `json:"attributes"`
		}
		if err := json.Unmarshal(raw, &point); err != nil {
			t.Fatalf("decoding a data point of %s: %v", m.Name, err)
		}
		out = append(out, point.Attributes)
	}
	return out
}

// metricDimensionExists reports the first instrument carrying a key, or "".
//
// It sweeps every metric the collector parsed rather than the one instrument
// under test: an assertion about what must never be a dimension has to look
// everywhere, because the instrument that acquires a forbidden key by accident
// is by definition not the one anybody suspected.
func metricDimensionExists(t *testing.T, c *collector, key string) string {
	t.Helper()

	for _, doc := range documents[metricDocument](t, filepath.Join(c.outDir, metricsFile)) {
		for _, resourceMetrics := range doc.ResourceMetrics {
			for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
				for _, m := range scopeMetrics.Metrics {
					for _, point := range dataPointAttributes(t, m) {
						if _, present := attr(point, key); present {
							return m.Name
						}
					}
				}
			}
		}
	}
	return ""
}

// TestRealCollector_IdentityReachesSpansAndNeverMetrics closes the last
// difference between this module and the hosted endpoint.
//
// The inventory above is recorded under the default identity policy, which
// records nobody, so it says nothing about the policies that do. The hosted
// deployment runs pseudonymous, and comparing its exported keys against the
// pinned set leaves user.hash unaccounted for: explained by the policy, but
// explained in prose rather than by a test. This is the test.
//
// The asymmetry is the point. Identity on a span is bounded by the trace it
// belongs to and is what makes a report of "this user's calls are slow"
// answerable. Identity on a metric is a dimension whose cardinality is the
// number of people using the deployment, which is the number an operator cannot
// predict, and the SDK responds to an exhausted series budget by collapsing the
// overflow into one bucket rather than by complaining.
func TestRealCollector_IdentityReachesSpansAndNeverMetrics(t *testing.T) {
	c := startCollector(t)

	env := telemetryEnv(c)
	env["GITLAB_MCP_TELEMETRY_IDENTITY"] = "pseudonymous"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 5 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("the collector parsed no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span carries the pseudonym and neither of the readable keys", func(t *testing.T) {
		digest, present := attr(span.Attributes, "user.hash")
		if !present {
			t.Fatalf("user.hash is absent under the pseudonymous policy; recorded %v", keys(span.Attributes))
		}
		if digest == "" {
			t.Error("user.hash is present but empty, which correlates nothing while still claiming to")
		}
		for _, readable := range []string{"user.id", "user.name"} {
			if value, leaked := attr(span.Attributes, readable); leaked {
				t.Errorf("%s = %q under the pseudonymous policy; the whole point of the policy is that this key is not exported", readable, value)
			}
		}
	})

	t.Run("no identity key is a dimension of any metric", func(t *testing.T) {
		// awaitMetric first, so the sweep runs against a file the exporter has
		// actually written to rather than an empty one, which would pass for the
		// wrong reason.
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("the collector parsed no %s metric, so this assertion would pass vacuously", durationMetric)
		}
		for _, key := range []string{"user.hash", "user.id", "user.name"} {
			if instrument := metricDimensionExists(t, c, key); instrument != "" {
				t.Errorf("%q is a dimension of %s; identity must never reach a metric under any policy", key, instrument)
			}
		}
	})
}
