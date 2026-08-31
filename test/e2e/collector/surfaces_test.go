//go:build collectore2e

package collectore2e

import (
	"strings"
	"testing"
)

// TestRealCollector_EverySurfaceResolvesTheAction covers the one attribute that
// cannot be derived from the request on any surface but the individual one.
//
// gitlab_mcp.action is what makes a metric readable, and how it is resolved
// differs completely between the three: on the dynamic surface it is an
// argument, on the meta surface it is the domain tool plus an argument, and on
// the individual surface it is looked up from a declared tool name. A resolver
// that silently returns nothing produces spans that are structurally perfect
// and say only that some tool was called, which is what shipped once already:
// the individual surface recorded no action at all because the catalog handed
// to the identifier was nil.
//
// The tool name matters as much. It is declared per surface, never derived, so
// a call that names the wrong tool is refused rather than mislabelled, and the
// harness fails on a refusal.
func TestRealCollector_EverySurfaceResolvesTheAction(t *testing.T) {
	tests := []struct {
		name       string
		surface    string
		tool       string
		arguments  string
		wantTool   string
		wantAction string
	}{
		{
			name:      "dynamic names two tools and carries the action as an argument",
			surface:   "dynamic",
			tool:      "gitlab_execute_action",
			arguments: `{"action":"issue.list","params":{"project_id":"some-group/some-project"}}`,
			wantTool:  "gitlab_execute_action",
			// The reason this attribute exists. On this surface the tool name
			// is the same for listing issues and for deleting a branch.
			wantAction: "issue.list",
		},
		{
			name:       "meta names the domain and carries the operation as an argument",
			surface:    "meta",
			tool:       "gitlab_issue",
			arguments:  `{"action":"list","params":{"project_id":"some-group/some-project"}}`,
			wantTool:   "gitlab_issue",
			wantAction: "issue.list",
		},
		{
			name:       "individual declares the tool and the action is looked up",
			surface:    "individual",
			tool:       "gitlab_issue_list",
			arguments:  `{"project_id":"some-group/some-project"}`,
			wantTool:   "gitlab_issue_list",
			wantAction: "issue.list",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := startCollector(t)
			env := telemetryEnv(c)
			env["TOOL_SURFACE"] = tc.surface
			// Pinned, so the individual case tests the resolver rather than the
			// auto policy's decision to drop the attribute on that surface.
			env["GITLAB_MCP_TELEMETRY_TOOL_NAME"] = "on"
			srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

			for i := range 3 {
				srv.callTool(t, i+1, tc.tool, tc.arguments)
			}

			_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
				return strings.HasPrefix(s.Name, "tools/call")
			})
			if !ok {
				t.Fatalf("no tools/call span on the %s surface.\nCollector:\n%s\nServer:\n%s",
					tc.surface, c.containerLogs(t), srv.logs())
			}

			if got, _ := attr(span.Attributes, "gitlab_mcp.tool_surface"); got != tc.surface {
				t.Errorf("gitlab_mcp.tool_surface = %q, want %q", got, tc.surface)
			}
			if got, _ := attr(span.Attributes, "gen_ai.tool.name"); got != tc.wantTool {
				t.Errorf("gen_ai.tool.name = %q, want %q", got, tc.wantTool)
			}
			action, present := attr(span.Attributes, "gitlab_mcp.action")
			if !present {
				t.Fatalf("gitlab_mcp.action is absent on the %s surface; the span says a tool was called and not what it did. Recorded %v",
					tc.surface, keys(span.Attributes))
			}
			if action != tc.wantAction {
				t.Errorf("gitlab_mcp.action = %q, want %q", action, tc.wantAction)
			}

			// The span name follows the tool, not the action, which is what the
			// convention asks for and is worth pinning because the action is
			// the more informative of the two and the temptation is real.
			if want := "tools/call " + tc.wantTool; span.Name != want {
				t.Errorf("span name = %q, want %q", span.Name, want)
			}
		})
	}
}

// TestRealCollector_IndividualSurfaceDropsTheNameFromMetricsByDefault is the
// auto policy, on the surface it exists for.
//
// About a thousand tools, one visible per catalog action, against an SDK limit
// of 2000 series per instrument. The policy's answer is to drop both keys from
// metrics and keep them on spans, and this is the only place that decision is
// exercised end to end: every other case here runs the dynamic surface, where
// auto keeps them.
func TestRealCollector_IndividualSurfaceDropsTheNameFromMetricsByDefault(t *testing.T) {
	c := startCollector(t)
	env := telemetryEnv(c)
	env["TOOL_SURFACE"] = "individual"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callTool(t, i+1, "gitlab_issue_list", `{"project_id":"some-group/some-project"}`)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span still says what was called", func(t *testing.T) {
		for _, key := range []string{"gen_ai.tool.name", "gitlab_mcp.action"} {
			if _, present := attr(span.Attributes, key); !present {
				t.Errorf("%s is absent from the span; the policy bounds series and does not hide values", key)
			}
		}
	})

	t.Run("the metric carries neither", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
		}
		for _, key := range []string{"gen_ai.tool.name", "gitlab_mcp.action"} {
			if instrument := metricDimensionExists(t, c, key); instrument != "" {
				t.Errorf("%s is a dimension of %s on the individual surface; about a thousand values against a 2000-series budget",
					key, instrument)
			}
		}
	})
}
