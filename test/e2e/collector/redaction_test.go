//go:build collectore2e

package collectore2e

import (
	"strings"
	"testing"
)

// resourceURI is a resource this server exposes whose identifier is the whole
// point: it embeds a project path.
const resourceURI = "gitlab://project/some-group%2Fsome-project"

// TestRealCollector_ResourceURIsFollowTheIdentityPolicy covers the rule a live
// deployment broke while every test passed.
//
// A subscription poll span carried gitlab://project/82077663 verbatim, which
// contradicted this server's documented position that resource URIs are not
// exported, and did so on the worst span for it: a poll repeats for the life of
// the watch, so one subscription wrote a project id into a backend hundreds of
// times.
//
// What replaced it is not a removal. Without something naming the resource, two
// watchers of one kind are indistinguishable and "one subscription is failing"
// and "all of them are" look the same. So the identity policy decides: a keyed
// digest by default, the URI itself under full, and never either on a metric.
func TestRealCollector_ResourceURIsFollowTheIdentityPolicy(t *testing.T) {
	tests := []struct {
		name      string
		identity  string
		wantKey   string
		absentKey string
	}{
		{
			name:      "the default policy names the resource indirectly",
			identity:  "",
			wantKey:   "gitlab_mcp.resource.ref",
			absentKey: "mcp.resource.uri",
		},
		{
			// The same decision the operator already made about the caller's
			// name. A URI says what somebody is working on, which is the same
			// class of disclosure as saying who they are.
			name:      "full exports the convention's own attribute",
			identity:  "full",
			wantKey:   "mcp.resource.uri",
			absentKey: "gitlab_mcp.resource.ref",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, span := readOneResource(t, tc.identity)

			assertResourceAttribute(t, span, tc.wantKey, tc.absentKey, tc.identity)
			assertNoResourceMetricDimension(t, c)

			// The span name never carries it, under either policy: a name built
			// per resource is one span name per project, which the convention
			// calls out for this attribute specifically.
			if span.Name != "resources/read" {
				t.Errorf("span name = %q, want %q", span.Name, "resources/read")
			}
		})
	}
}

// readOneResource starts a stack under an identity policy, reads a resource,
// and returns the span it produced.
func readOneResource(t *testing.T, identity string) (*collector, otlpSpan) {
	t.Helper()

	c := startCollector(t)
	env := telemetryEnv(c)
	if identity != "" {
		env["GITLAB_MCP_TELEMETRY_IDENTITY"] = identity
	}
	srv := startServer(t, env, "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.readResource(t, i+1, resourceURI)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return s.Name == "resources/read"
	})
	if !ok {
		t.Fatalf("no resources/read span.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}
	return c, span
}

// assertResourceAttribute checks that exactly one of the two keys is present
// and that its value is what the policy allows.
func assertResourceAttribute(t *testing.T, span otlpSpan, wantKey, absentKey, identity string) {
	t.Helper()

	value, present := attr(span.Attributes, wantKey)
	if !present {
		t.Fatalf("%s is absent; recorded %v", wantKey, keys(span.Attributes))
	}
	if _, leaked := attr(span.Attributes, absentKey); leaked {
		t.Errorf("%s is recorded as well, which says the same thing twice", absentKey)
	}

	if identity == "full" {
		if value != resourceURI {
			t.Errorf("mcp.resource.uri = %q, want the URI the client asked for", value)
		}
		return
	}
	if strings.Contains(value, "some-group") {
		t.Errorf("the digest %q contains the project path it stands for", value)
	}
}

// assertNoResourceMetricDimension checks both keys against every instrument.
func assertNoResourceMetricDimension(t *testing.T, c *collector) {
	t.Helper()

	if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
		t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
	}
	for _, key := range []string{"mcp.resource.uri", "gitlab_mcp.resource.ref"} {
		if instrument := metricDimensionExists(t, c, key); instrument != "" {
			t.Errorf("%s is a dimension of %s; it is one series per resource a client touches",
				key, instrument)
		}
	}
}

// TestRealCollector_UnknownNamesDoNotBecomeDimensionValues covers the bound that
// stops a caller choosing how many time series this process stores.
//
// gen_ai.tool.name and gen_ai.prompt.name are copied off the request and the
// convention puts both on the duration metric, and nothing checked that either
// named anything. A prompts/get for a prompt that does not exist recorded the
// invented name as a dimension value, which was found by driving the hosted
// deployment and reading its collector.
//
// The span keeps the name, because that is where an operator finds out what a
// misbehaving client is actually sending.
func TestRealCollector_UnknownNamesDoNotBecomeDimensionValues(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	const invented = "not-a-prompt-this-server-has"
	for i := range 3 {
		srv.getPromptExpectingRefusal(t, i+1, invented)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "prompts/get")
	})
	if !ok {
		t.Fatalf("no prompts/get span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the span records what the client sent", func(t *testing.T) {
		if got, _ := attr(span.Attributes, "gen_ai.prompt.name"); got != invented {
			t.Errorf("gen_ai.prompt.name = %q on the span, want the name the client sent", got)
		}
	})

	t.Run("the metric records the bucket instead", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, durationMetric); !found {
			t.Fatalf("no %s metric, so this would pass vacuously", durationMetric)
		}
		if instrument, key := metricValueExists(t, c, invented); instrument != "" {
			t.Errorf("%s carries the invented name on %s; a caller then chooses the label space",
				key, instrument)
		}
	})
}

// metricValueExists reports the first instrument and key recording a value.
func metricValueExists(t *testing.T, c *collector, value string) (instrument, key string) {
	t.Helper()

	for _, doc := range documents[metricDocument](t, metricsPath(c)) {
		for _, resourceMetrics := range doc.ResourceMetrics {
			for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
				if name, found := metricInScopeCarrying(t, scopeMetrics, value); name != "" {
					return name, found
				}
			}
		}
	}
	return "", ""
}

// metricInScopeCarrying searches one scope's instruments for a value.
func metricInScopeCarrying(t *testing.T, scope otlpScopeMetrics, value string) (instrument, key string) {
	t.Helper()

	for _, m := range scope.Metrics {
		for _, point := range dataPointAttributes(t, m) {
			for _, kv := range point {
				if kv.Value.StringValue == value {
					return m.Name, kv.Key
				}
			}
		}
	}
	return "", ""
}
