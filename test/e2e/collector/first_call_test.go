//go:build collectore2e

package collectore2e

import (
	"strings"
	"testing"
)

// TestRealCollector_TheFirstCallOfAProcess_AlreadyNamesItsAction holds the one
// call that used to arrive anonymous.
//
// The readiness gate is installed as the innermost middleware on purpose, so a
// request that waits for the tool catalog waits inside its own span and the
// latency an operator reads is the one the client saw. The cost, unnoticed
// until a real collector was put in front of the server, is that the span is
// described BEFORE that wait: the first tools/call of a process was described
// while registration was still building the catalog its action is resolved
// against, so nothing could name it. The span said a tool had been called and
// not what it did, and the duration metric carried the same hole.
//
// One call and no more, because the defect is invisible from the second
// onwards: every sibling test here makes three or five, so all of them saw the
// attribute and only the first of their calls lacked it. A test that makes one
// call is the only shape that fails on this.
//
// It matters past a tidy trace. On the dynamic surface gen_ai.tool.name is
// gitlab_execute_action for listing issues and for deleting a branch alike, so
// the action is the entire content of the span; and the end-to-end coverage
// audit joins its own record to that attribute, so a harness starting a server
// per test loses one action per test.
func TestRealCollector_TheFirstCallOfAProcess_AlreadyNamesItsAction(t *testing.T) {
	tests := []struct {
		name      string
		surface   string
		tool      string
		arguments string
	}{
		{
			name:      "dynamic, where the action is all the span says",
			surface:   "dynamic",
			tool:      "gitlab_execute_action",
			arguments: `{"action":"issue.list","params":{"project_id":"some-group/some-project"}}`,
		},
		{
			name:      "meta, where the tool names the domain and the argument the operation",
			surface:   "meta",
			tool:      "gitlab_issue",
			arguments: `{"action":"list","params":{"project_id":"some-group/some-project"}}`,
		},
		{
			name:      "individual, where the tool name is the operation",
			surface:   "individual",
			tool:      "gitlab_issue_list",
			arguments: `{"project_id":"some-group/some-project"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := startCollector(t)
			env := telemetryEnv(c)
			env["GITLAB_MCP_TOOL_SURFACE"] = tc.surface
			// Pinned, so the individual case asks about the resolver rather
			// than about the cardinality policy's decision to drop the tool
			// name on that surface.
			env["GITLAB_MCP_TELEMETRY_TOOL_NAME"] = "on"
			srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

			// Exactly one. See the doc comment: a second would hide the defect.
			srv.callTool(t, 1, tc.tool, tc.arguments)

			_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
				return strings.HasPrefix(s.Name, toolCallMethod)
			})
			if !ok {
				t.Fatalf("no %s span on the %s surface.\nCollector:\n%s\nServer:\n%s",
					toolCallMethod, tc.surface, c.containerLogs(t), srv.logs())
			}

			t.Run("the span names the action and its domain", func(t *testing.T) {
				assertNamesIssueList(t, "span", span.Attributes)
			})

			// The metric is asserted beside the span rather than left to the
			// span alone, because they are filled from one attribute list and a
			// repair that reached only the span would leave every dashboard
			// built on the metric exactly as blind as before.
			t.Run("the duration metric names them too", func(t *testing.T) {
				point, _, found := awaitDurationPoint(t, c, exportDeadline, func(p []otlpAttr) bool {
					method, _ := attr(p, "mcp.method.name")
					return method == toolCallMethod
				})
				if !found {
					t.Fatalf("no %s data point named %s on the %s surface.\nCollector:\n%s\nServer:\n%s",
						durationMetric, toolCallMethod, tc.surface, c.containerLogs(t), srv.logs())
				}
				assertNamesIssueList(t, durationMetric+" data point", point)
			})
		})
	}
}

// assertNamesIssueList holds one attribute set to the action the call made and
// the domain it belongs to.
//
// Both, rather than the action alone: the action is what an operator groups by
// on the dynamic surface and the domain is what survives on the individual one,
// where the cardinality policy drops the tool name, so a repair that produced
// only the first would still leave that surface unable to say what was touched.
func assertNamesIssueList(t *testing.T, what string, attributes []otlpAttr) {
	t.Helper()

	if got, present := attr(attributes, "gitlab_mcp.action"); !present || got != "issue.list" {
		t.Errorf("%s: gitlab_mcp.action = %q (present=%v), want issue.list on the first call of the process; recorded %v",
			what, got, present, keys(attributes))
	}
	if got, present := attr(attributes, "gitlab_mcp.domain"); !present || got != "issue" {
		t.Errorf("%s: gitlab_mcp.domain = %q (present=%v), want issue on the first call of the process; recorded %v",
			what, got, present, keys(attributes))
	}
}
