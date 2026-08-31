//go:build collectore2e

package collectore2e

import (
	"strings"
	"testing"
)

// TestRealCollector_SafeModeSaysWhyItRefused covers the protective mode from
// the only side that matters to an operator: what the telemetry says happened.
//
// A refusal travels as an error result, which is a successful JSON-RPC response
// carrying a failure meant for the model. From outside the handler it is
// indistinguishable from a call that ran and failed, so without the reason on
// the span a deployment refusing every third request looks exactly like one
// whose GitLab is erroring.
//
// gitlab_mcp.refusal_reason existed as a declared attribute key that nothing
// set, which is why this had to be built before it could be tested.
func TestRealCollector_SafeModeSaysWhyItRefused(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c),
		"--gitlab-url="+startFakeGitLab(t),
		"--safe-mode",
	)

	// A mutating action, because safe mode intercepts those and leaves reads
	// alone. Nothing is created: the point of the mode is that the call is
	// answered with a preview instead of being performed.
	for i := range 3 {
		srv.callToolTolerant(t, i+1, "gitlab_execute_action",
			`{"action":"issue.create","params":{"project_id":"some-group/some-project","title":"never created"}}`)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span under safe mode.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	reason, present := attr(span.Attributes, "gitlab_mcp.refusal_reason")
	if !present {
		t.Fatalf("gitlab_mcp.refusal_reason is absent; the span cannot say this server declined rather than failed. Recorded %v",
			keys(span.Attributes))
	}
	if reason != "safe_mode" {
		t.Errorf("gitlab_mcp.refusal_reason = %q, want %q", reason, "safe_mode")
	}

	t.Run("the reason is bounded, so it can be a metric dimension", func(t *testing.T) {
		// The set is closed on purpose: an operator computing a refusal rate
		// groups by this, and free text does not group. The assertion is that
		// the value is one of the five, not merely present.
		known := map[string]bool{
			"safe_mode": true, "needs_confirmation": true, "invalid_params": true,
			"unknown_action": true, "rate_limited": true,
		}
		if !known[reason] {
			t.Errorf("%q is not one of this server's five refusal reasons", reason)
		}
	})

	t.Run("the action is still recorded", func(t *testing.T) {
		// A refused call is still a call, and which action was refused is the
		// whole content of a report that safe mode is blocking something
		// somebody needs.
		if got, _ := attr(span.Attributes, "gitlab_mcp.action"); got != "issue.create" {
			t.Errorf("gitlab_mcp.action = %q, want issue.create", got)
		}
	})

	t.Run("the reason is a metric dimension, not only a span attribute", func(t *testing.T) {
		// The span answers "what happened to this one call". Counting is a
		// different question and needs a different signal: a deployment whose
		// safe mode is blocking something somebody needs shows up as a rate,
		// and a rate cannot be computed from an attribute that only exists on
		// individual traces, which are sampled.
		_, duration, found := c.awaitMetric(t, exportDeadline, durationMetric)
		if !found {
			t.Fatalf("%s never arrived.\nCollector:\n%s\nServer:\n%s",
				durationMetric, c.containerLogs(t), srv.logs())
		}

		points := dataPointAttributes(t, duration)
		if len(points) == 0 {
			t.Fatal("the instrument arrived with no data points")
		}

		refused := 0
		for _, point := range points {
			if value, carried := attr(point, "gitlab_mcp.refusal_reason"); carried {
				refused++
				if value != "safe_mode" {
					t.Errorf("gitlab_mcp.refusal_reason = %q on a data point, want safe_mode", value)
				}
			}
		}
		if refused == 0 {
			t.Errorf("no data point of %s carries the refusal reason, so a refusal rate cannot be computed",
				durationMetric)
		}
	})
}

// TestRealCollector_ReadOnlyRemovesTheToolRatherThanRefusingIt pins the
// difference between the two protective modes, which is easy to conflate and
// produces different telemetry.
//
// Read-only removes mutating operations from the catalog, so the tool is not
// there to call and the refusal comes from the SDK as an unknown tool. Safe
// mode keeps them and intercepts, which is what produces a refusal reason. An
// operator seeing no refusal_reason under read-only is looking at the mode
// working, not at a missing attribute.
func TestRealCollector_ReadOnlyRemovesTheToolRatherThanRefusingIt(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c),
		"--gitlab-url="+startFakeGitLab(t),
		"--read-only",
	)

	for i := range 3 {
		srv.callToolTolerant(t, i+1, "gitlab_execute_action",
			`{"action":"issue.create","params":{"project_id":"some-group/some-project","title":"never created"}}`)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span under read-only.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	// Whatever the mode answers, the span must not claim the server failed: a
	// caller asking for something this deployment does not offer is the
	// deployment working.
	if errorType, present := attr(span.Attributes, "error.type"); present {
		for _, serverFault := range []string{"-32603", "_OTHER"} {
			if errorType == serverFault {
				t.Errorf("error.type = %q under read-only; refusing a mutating call is not this server failing", errorType)
			}
		}
	}
}
