//go:build collectore2e

package collectore2e

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// driveDynamic starts a stack with the given extra environment and makes a few
// dynamic-surface calls through it.
//
// One collector and one server per configuration, because these differ in what
// the process was started with and there is no way to change that afterwards:
// the identity policy, the tool-name policy and the surface are all fixed
// before the first request, which is exactly why they are worth covering here
// rather than in a unit test that can set them per case.
func driveDynamic(t *testing.T, extra map[string]string) (*collector, *server) {
	t.Helper()

	c := startCollector(t)
	env := telemetryEnv(c)
	maps.Copy(env, extra)
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}
	return c, srv
}

// TestRealCollector_ToolNamePolicyOff covers the View that exists to stop the
// individual surface exhausting the SDK's series budget.
//
// Both keys, because filtering one and keeping the other is what the View did
// until recently: the individual surface projects one visible tool per catalog
// action, so gen_ai.tool.name and gitlab_mcp.action carry the same eleven
// hundred values and dropping either alone sheds nothing. The span keeps both,
// which is the point of doing this with a View rather than by not recording
// them.
func TestRealCollector_ToolNamePolicyOff(t *testing.T) {
	c, srv := driveDynamic(t, map[string]string{"GITLAB_MCP_TELEMETRY_TOOL_NAME": "off"})

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span keeps both", func(t *testing.T) {
		for _, key := range []string{"gen_ai.tool.name", "gitlab_mcp.action"} {
			t.Run(key, func(t *testing.T) {
				if _, present := attr(span.Attributes, key); !present {
					t.Errorf("%s is absent from the span; the policy is about metric series, not about hiding the value", key)
				}
			})
		}
	})

	t.Run("the metric keeps neither", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
		}
		for _, key := range []string{"gen_ai.tool.name", "gitlab_mcp.action"} {
			t.Run(key, func(t *testing.T) {
				if instrument := metricDimensionExists(t, c, key); instrument != "" {
					t.Errorf("%s is still a dimension of %s under the off policy", key, instrument)
				}
			})
		}
	})
}

// TestRealCollector_IdentityFullExportsTheReadableUser is the third identity
// policy, and the one an operator reaches for when they have decided to record
// who calls.
//
// The assertion that matters is the same under every policy: whatever reaches a
// span, nothing about the caller reaches a metric. A per-user dimension is
// unbounded by the number of people using the deployment, which is the number
// an operator cannot predict.
func TestRealCollector_IdentityFullExportsTheReadableUser(t *testing.T) {
	c, srv := driveDynamic(t, map[string]string{"GITLAB_MCP_TELEMETRY_IDENTITY": "full"})

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span names the user", func(t *testing.T) {
		if name, present := attr(span.Attributes, "user.name"); !present || name == "" {
			t.Errorf("user.name is absent under the full policy; recorded %v", keys(span.Attributes))
		}
		if _, present := attr(span.Attributes, "user.hash"); present {
			t.Error("the pseudonym is recorded alongside the readable name, which says the same thing twice")
		}
	})

	t.Run("no identity key is a metric dimension", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
		}
		for _, key := range []string{"user.id", "user.name", "user.hash"} {
			t.Run(key, func(t *testing.T) {
				if instrument := metricDimensionExists(t, c, key); instrument != "" {
					t.Errorf("%q is a dimension of %s; identity must never reach a metric under any policy", key, instrument)
				}
			})
		}
	})
}

// TestRealCollector_TelemetryOffExportsNothing covers the default, which is the
// configuration almost every deployment runs.
//
// Nothing else asserts it. Every other case here turns telemetry on, so a
// change that started an exporter regardless of the flag would be invisible to
// this module while being the most serious thing it could miss: a server that
// exports without being asked to.
func TestRealCollector_TelemetryOffExportsNothing(t *testing.T) {
	c := startCollector(t)

	// The endpoint is configured and the switch is not, which is the shape that
	// catches a provider started from the environment alone.
	env := telemetryEnv(c)
	env["GITLAB_MCP_TELEMETRY"] = "false"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	assertNothingExported(t, c, srv)
}

// TestRealCollector_SDKDisabledVetoesAnEnabledDeployment covers the standard
// variable that overrides this server's own switch.
//
// It is a veto rather than a second on switch, and the direction is easy to get
// backwards: OTEL_SDK_DISABLED defaults to "enabled" while telemetry here
// defaults to off, so adopting it as the switch would invert its meaning. This
// asserts the composition, with the operator explicitly asking for telemetry.
func TestRealCollector_SDKDisabledVetoesAnEnabledDeployment(t *testing.T) {
	c := startCollector(t)

	env := telemetryEnv(c)
	env["OTEL_SDK_DISABLED"] = "true"
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	assertNothingExported(t, c, srv)
}

// assertNothingExported waits out the export schedules and then fails if any
// file has content.
//
// The wait is what makes this an assertion rather than a race: the exporters
// batch, so a check that ran immediately would pass against a server that was
// about to export.
func assertNothingExported(t *testing.T, c *collector, srv *server) {
	t.Helper()

	// Comfortably past the batch schedule and the metric interval this module
	// configures, both of which are 100ms.
	time.Sleep(5 * time.Second)

	for _, name := range []string{tracesFile, metricsFile, logsFile} {
		path := filepath.Join(c.outDir, name)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue // never created, which is the expected outcome
		}
		if err != nil {
			// Any other failure means the output cannot be inspected, and a
			// test that treats that as "nothing was exported" passes blind.
			t.Fatalf("inspecting %s: %v", name, err)
		}
		if info.Size() > 0 {
			t.Errorf("%s holds %d bytes; the deployment exported telemetry it was not asked for.\nServer:\n%s",
				name, info.Size(), srv.logs())
		}
	}
}

// TestRealCollector_TheIdentityPolicyGovernsEverySignal is the end-to-end form
// of a gap two of the three signals hid.
//
// A hosted deployment ran with the policy on pseudonymous, which by definition
// names nobody. Its spans carried user.hash and its metrics carried no identity
// at all, exactly as designed, while 48 exported log records carried user_id
// and 38 carried the username in the clear. The policy had been applied where
// it was written and never to the log bridge.
//
// So the assertion is deliberately about the whole log stream rather than one
// record: what must hold is that nothing exported names the caller under a
// policy that promises not to, not that a particular line was fixed. This is
// the same shape as the resource-URI leak this module already guards, and it is
// the second time the log signal has been the one that leaked.
func TestRealCollector_TheIdentityPolicyGovernsEverySignal(t *testing.T) {
	for _, tc := range []struct {
		policy  string
		forbids []string
	}{
		{policy: "none", forbids: []string{"user.id", "user.name", "user.hash", "user_id"}},
		{policy: "pseudonymous", forbids: []string{"user.id", "user.name", "user_id"}},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			c, srv := driveDynamic(t, map[string]string{"GITLAB_MCP_TELEMETRY_IDENTITY": tc.policy})

			// A log record has to have arrived, or an empty stream passes for a
			// clean one. The tool call the driver makes writes one.
			if _, ok := c.awaitLog(t, exportDeadline, func(r otlpLogRecord) bool {
				return strings.Contains(r.Body.StringValue, "tool call")
			}); !ok {
				t.Fatalf("no tool call log record arrived.\nCollector:\n%s\nServer:\n%s",
					c.containerLogs(t), srv.logs())
			}

			for _, record := range allLogRecords(t, c) {
				rendered := renderLogRecord(record)
				for _, forbidden := range tc.forbids {
					if strings.Contains(rendered, forbidden) {
						t.Errorf("an exported log record carries %q under policy %q: %s",
							forbidden, tc.policy, rendered)
					}
				}
			}
		})
	}
}

// TestRealCollector_AConfiguredKeyMakesTwoProcessesAgree is the replica
// scenario, run as two processes rather than reasoned about.
//
// A unit test can show that two keyrings built from one secret compute the same
// digest. What it cannot show is that two independently started servers,
// exporting into the same collector, put the same value on the wire: that
// depends on the environment reaching the keyring, on the keyring reaching both
// redactors, and on nothing in between regenerating a key. Each of those has
// been wrong at least once in this package's history.
//
// Both halves are asserted, because the interesting failure is one-sided. A
// build that ignored the key entirely would fail the first; a build that
// somehow shared state between processes would fail the second, and would be a
// far stranger defect.
func TestRealCollector_AConfiguredKeyMakesTwoProcessesAgree(t *testing.T) {
	t.Run("with a key, two processes agree", func(t *testing.T) {
		digests := digestsFromTwoServers(t, map[string]string{
			"GITLAB_MCP_TELEMETRY_IDENTITY":     "pseudonymous",
			"GITLAB_MCP_TELEMETRY_IDENTITY_KEY": "a deployment-wide secret",
		})
		if len(digests) != 1 {
			t.Errorf("two replicas sharing a key produced %d digests (%v); one caller must read as one person",
				len(digests), digests)
		}
	})

	t.Run("without a key, they do not", func(t *testing.T) {
		digests := digestsFromTwoServers(t, map[string]string{
			"GITLAB_MCP_TELEMETRY_IDENTITY": "pseudonymous",
		})
		if len(digests) != 2 {
			t.Errorf("two replicas without a shared key produced %d digests (%v); the default is a per-process key",
				len(digests), digests)
		}
	})
}

// digestsFromTwoServers runs two servers into one collector and returns the set
// of user.hash values they exported.
func digestsFromTwoServers(t *testing.T, extra map[string]string) []string {
	t.Helper()

	c := startCollector(t)
	gitlab := startFakeGitLab(t)

	for range 2 {
		env := telemetryEnv(c)
		maps.Copy(env, extra)
		srv := startServer(t, env, "--gitlab-url="+gitlab)
		for i := range 2 {
			srv.callAction(t, i+1, "issue.list", "some-group/some-project")
		}
	}

	// Both processes have to have reported before the digests mean anything:
	// reading after only the faster one exported would find one digest and
	// call that agreement. The instance id is what distinguishes them, and it
	// is independent of the digest being asked about.
	reported := map[string]bool{}
	if _, _, ok := c.awaitSpan(t, exportDeadline, func(rs otlpResourceSpans, s otlpSpan) bool {
		if _, present := attr(s.Attributes, "user.hash"); !present {
			return false
		}
		if instance, present := attr(rs.Resource.Attributes, "service.instance.id"); present {
			reported[instance] = true
		}
		return len(reported) == 2
	}); !ok {
		t.Fatalf("only %d of 2 processes exported a user.hash.\nCollector:\n%s",
			len(reported), c.containerLogs(t))
	}

	seen := map[string]bool{}
	for _, doc := range documents[traceDocument](t, filepath.Join(c.outDir, tracesFile)) {
		for _, rs := range doc.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					if digest, present := attr(span.Attributes, "user.hash"); present {
						seen[digest] = true
					}
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for digest := range seen {
		out = append(out, digest)
	}
	sort.Strings(out)
	return out
}

// TestRealCollector_TheKeyChoiceIsVisibleToAnOperator covers the line an
// operator reads to confirm a setting took effect, in the place they read it.
//
// The hosted deployment showed the setting working, two processes agreeing on a
// digest, while the line announcing it appeared in no exported log record. A
// setting that works and cannot be confirmed is a setting somebody will
// configure twice, or will believe is on when a typo turned it off.
//
// Both legs are checked, because they can disagree and did: stderr is where the
// line is written and the collector is where an operator running three replicas
// actually looks.
func TestRealCollector_TheKeyChoiceIsVisibleToAnOperator(t *testing.T) {
	c, srv := driveDynamic(t, map[string]string{
		"GITLAB_MCP_TELEMETRY_IDENTITY":     "pseudonymous",
		"GITLAB_MCP_TELEMETRY_IDENTITY_KEY": "a deployment-wide secret",
	})

	const line = "telemetry pseudonyms use the configured key"

	t.Run("the server writes it", func(t *testing.T) {
		if logs := srv.logs(); !strings.Contains(logs, line) {
			t.Errorf("the server never announced the configured key.\nServer:\n%s", logs)
		}
	})

	t.Run("the collector receives it", func(t *testing.T) {
		if _, ok := c.awaitLog(t, exportDeadline, func(r otlpLogRecord) bool {
			return strings.Contains(r.Body.StringValue, line)
		}); !ok {
			t.Errorf("the line reached stderr and not the collector, which is where an operator running replicas looks.\nCollector:\n%s",
				c.containerLogs(t))
		}
	})
}
