// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build collectore2e

// example_test.go holds the worked telemetry example under examples/telemetry/
// to the server it documents. The telemetry page walks a reader through two
// committed files, a collector configuration and a Docker Compose file, and
// three authenticator overlays it carries itself; a reader who copies them
// expects them to work, and these tests are what keep them from rotting.
//
// The collector configuration is run the way a reader runs it: a real
// collector with the committed file mounted where the image reads it, the
// real server exporting into it, and a backend behind it. The backend is this
// module's own file-exporting collector, started under the host name the
// committed file forwards to, so what the example forwarded is read back
// rather than assumed. grafana/otel-lgtm itself is not started, as decided on
// issue 1106: the compose file is held to the same pipeline through `docker
// compose config`, which parses and normalizes it without pulling an image.
package collectore2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// The committed example and the names its two files share.
const (
	exampleConfigFile  = "otel-collector.yaml"
	exampleComposeFile = "compose.yaml"
	// exampleBackendHost is the host the committed configuration forwards to,
	// which is the backend's service name in the compose file.
	exampleBackendHost = "lgtm"
	// exampleCollectorService and exampleServerService are the compose
	// services that run the collector and this server.
	exampleCollectorService = "collector"
	exampleServerService    = "gitlab-mcp-server"
)

// exampleDir is the committed worked example the telemetry page imports.
func exampleDir() string {
	return filepath.Join(repoRoot(), "examples", "telemetry")
}

// exampleRig is the committed pipeline's surroundings: a network, and on it a
// backend standing in for grafana/otel-lgtm under the name the committed file
// forwards to.
type exampleRig struct {
	network string
	backend *collector
}

// startExampleRig starts the network and the backend.
func startExampleRig(t *testing.T) exampleRig {
	t.Helper()

	network := dockerNetwork(t)
	backend := runCollector(t, collectorSpec{
		configs: []string{writeCollectorConfig(t)},
		network: network,
		alias:   exampleBackendHost,
	})
	return exampleRig{network: network, backend: backend}
}

// startExample runs the committed configuration on the rig's network, with
// the overlays given merged onto it in order.
func (r exampleRig) startExample(t *testing.T, env map[string]string, probe http.Header, overlays ...string) *collector {
	t.Helper()

	committed := filepath.Join(exampleDir(), exampleConfigFile)
	if _, err := os.Stat(committed); err != nil {
		t.Fatalf("the committed example collector configuration is missing: %v", err)
	}
	return runCollector(t, collectorSpec{
		configs:     append([]string{committed}, overlays...),
		network:     r.network,
		env:         env,
		probeHeader: probe,
	})
}

// TestExample_CollectorConfig_CarriesEverySignalToTheBackend drives the real
// binary through the committed example configuration and reads back, on the
// far side of it, all three signals it forwarded.
//
// The page says three things of this file: that the collector forwards traces,
// metrics and logs to the backend, that the debug exporter prints a summary
// line per batch, and, by being a file a reader copies, that the collector
// takes it without complaint. Each is asserted. The last one is why the
// example's log is held to a stricter rule than this module's own collector:
// a warning there, such as the deprecation of a component name the file uses,
// is what a reader sees on their first start.
func TestExample_CollectorConfig_CarriesEverySignalToTheBackend(t *testing.T) {
	rig := startExampleRig(t)
	example := rig.startExample(t, nil, nil)
	srv := startServer(t, telemetryEnv(example), "--gitlab-url="+startFakeGitLab(t))

	const action = "issue.list"
	for i := range 5 {
		srv.callAction(t, i+1, action, "some-group/some-project")
	}

	t.Run("traces reach the backend", func(t *testing.T) {
		_, span, ok := rig.backend.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
			got, _ := attr(s.Attributes, "gitlab_mcp.action")
			return s.Kind == spanKindServer && got == action
		})
		if !ok {
			t.Fatalf("no %s span crossed the committed pipeline.\nExample collector:\n%s\nServer:\n%s",
				action, example.containerLogs(t), srv.logs())
		}
		assertServerSpan(t, span)
	})

	t.Run("metrics reach the backend", func(t *testing.T) {
		if _, _, ok := rig.backend.awaitMetric(t, exportDeadline, durationMetric); !ok {
			t.Fatalf("no %s crossed the committed pipeline.\nExample collector:\n%s\nServer:\n%s",
				durationMetric, example.containerLogs(t), srv.logs())
		}
	})

	t.Run("logs reach the backend", func(t *testing.T) {
		if _, ok := rig.backend.awaitLog(t, exportDeadline, func(r otlpLogRecord) bool {
			return strings.Contains(r.Body.StringValue, "tool call") && r.TraceID != ""
		}); !ok {
			t.Fatalf("no correlated tool-call record crossed the committed pipeline.\nExample collector:\n%s\nServer:\n%s",
				example.containerLogs(t), srv.logs())
		}
	})

	t.Run("the debug exporter summarizes every signal", func(t *testing.T) {
		if missing := awaitDebugSummaries(t, example, exportDeadline); len(missing) > 0 {
			t.Errorf("the debug exporter logged no batch summary for %v, which the page tells a reader to look for.\nExample collector:\n%s",
				missing, example.containerLogs(t))
		}
	})

	t.Run("nothing was refused, and the example started clean", func(t *testing.T) {
		assertCollectorLogClean(t, example)
		assertNothingWasRefused(t, rig.backend, srv)
	})
}

// debugSignals maps each message the debug exporter logs a batch under to the
// field that counts what the batch carried.
var debugSignals = map[string]string{
	"Traces":  "spans",
	"Metrics": "data points",
	"Logs":    "log records",
}

// debugSummary reports which signal a collector log line summarizes, when the
// line is the debug exporter's account of a batch that carried something.
//
// A line qualifies only when everything that makes it that account holds: the
// console encoding's last two fields are the message and a JSON object of
// fields, the message is a signal's name exactly, the fields name the debug
// exporter as the component and that signal as the one it handled, and the
// count is at least one. A startup line fails each of those, including the
// debug exporter's own ("Development component. May change in the future."),
// so no line written before any data moved can stand in for one written after.
func debugSummary(line string) (signal string, ok bool) {
	parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
	if len(parts) < 3 || parts[1] != "info" {
		return "", false
	}
	message := parts[len(parts)-2]
	countField, isSignal := debugSignals[message]
	if !isSignal {
		return "", false
	}
	var fields map[string]any
	if json.Unmarshal([]byte(parts[len(parts)-1]), &fields) != nil {
		return "", false
	}
	if fields["otelcol.component.id"] != "debug" || fields["otelcol.component.kind"] != "exporter" ||
		fields["otelcol.signal"] != strings.ToLower(message) {
		return "", false
	}
	if count, _ := fields[countField].(float64); count < 1 {
		return "", false
	}
	return message, true
}

// awaitDebugSummaries waits until the collector's log holds a batch summary
// for every signal, and returns the signals it gave up on, sorted.
func awaitDebugSummaries(t *testing.T, c *collector, within time.Duration) []string {
	t.Helper()

	var missing []string
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		seen := map[string]bool{}
		for line := range strings.SplitSeq(c.containerLogs(t), "\n") {
			if signal, ok := debugSummary(line); ok {
				seen[signal] = true
			}
		}
		missing = nil
		for _, signal := range slices.Sorted(maps.Keys(debugSignals)) {
			if !seen[signal] {
				missing = append(missing, signal)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return missing
}

// assertCollectorLogClean fails on any warning, error or fatal line in a
// collector's own log. The console encoding is tab-separated with the level
// second, so a delimited level cannot be matched by the word inside a message.
func assertCollectorLogClean(t *testing.T, c *collector) {
	t.Helper()

	for line := range strings.SplitSeq(c.containerLogs(t), "\n") {
		for _, level := range []string{"\twarn\t", "\terror\t", "\tfatal\t"} {
			if strings.Contains(line, level) {
				t.Errorf("the example collector logged %s, which a reader starting it would see:\n%s",
					strings.Trim(level, "\t"), line)
			}
		}
	}
}

// TestDebugSummary_Lines_AreTheBatchAccountAndNothingElse pins what the debug
// exporter's batch summary is told apart from, using lines of the shape the
// pinned image writes, so the matcher's strictness does not rest on whichever
// lines a run happens to produce.
func TestDebugSummary_Lines_AreTheBatchAccountAndNothingElse(t *testing.T) {
	const (
		stamp    = "2026-10-20T16:40:01.141Z"
		resource = `"resource": {"service.name": "otelcol-contrib"}, `
		debug    = `"otelcol.component.id": "debug", "otelcol.component.kind": "exporter", `
	)
	cases := []struct {
		name       string
		line       string
		wantSignal string
	}{
		{
			name:       "a traces batch",
			line:       stamp + "\tinfo\tTraces\t{" + resource + debug + `"otelcol.signal": "traces", "resource spans": 1, "spans": 3}`,
			wantSignal: "Traces",
		},
		{
			name:       "a metrics batch",
			line:       stamp + "\tinfo\tMetrics\t{" + resource + debug + `"otelcol.signal": "metrics", "resource metrics": 1, "metrics": 8, "data points": 24}`,
			wantSignal: "Metrics",
		},
		{
			name:       "a logs batch",
			line:       stamp + "\tinfo\tLogs\t{" + resource + debug + `"otelcol.signal": "logs", "resource logs": 1, "log records": 2}`,
			wantSignal: "Logs",
		},
		{
			name: "the collector's startup line",
			line: stamp + "\tinfo\tservice@v0.162.0/service.go:256\tEverything is ready. Begin running and processing data.\t{" + resource[:len(resource)-2] + "}",
		},
		{
			name: "the debug exporter's own startup line",
			line: stamp + "\tinfo\tbuilders/builders.go:26\tDevelopment component. May change in the future.\t{" + resource + debug + `"otelcol.signal": "logs"}`,
		},
		{
			name: "a pipeline's startup line naming a signal",
			line: stamp + "\tinfo\tLogs\t{" + resource + `"otelcol.component.id": "otlp", "otelcol.component.kind": "receiver", "otelcol.signal": "logs"}`,
		},
		{
			name: "an empty batch",
			line: stamp + "\tinfo\tTraces\t{" + resource + debug + `"otelcol.signal": "traces", "resource spans": 0, "spans": 0}`,
		},
		{
			name: "a signal that is not the one the line names",
			line: stamp + "\tinfo\tTraces\t{" + resource + debug + `"otelcol.signal": "metrics", "resource spans": 1, "spans": 3}`,
		},
		{
			name: "a summary at another level",
			line: stamp + "\twarn\tTraces\t{" + resource + debug + `"otelcol.signal": "traces", "resource spans": 1, "spans": 3}`,
		},
		{
			name: "fields that are not JSON",
			line: stamp + "\tinfo\tTraces\tspans=3",
		},
		{
			name: "too few fields",
			line: stamp + "\tinfo",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			signal, ok := debugSummary(testCase.line)
			if ok != (testCase.wantSignal != "") || signal != testCase.wantSignal {
				t.Errorf("debugSummary() = (%q, %t), want %q", signal, ok, testCase.wantSignal)
			}
		})
	}
}

// documentedOverlay matches an authenticator overlay on the telemetry page: a
// YAML block titled with the file name a reader saves it under.
var documentedOverlay = regexp.MustCompile("(?s)```yaml title=\"(auth-[a-z-]+\\.yaml)\"\n(.*?)\n```")

// documentedOverlays returns the authenticator overlays the English telemetry
// page carries, by title. The page is their only copy: read from it rather than
// kept here, a test of these blocks is a test of what a reader is given. The
// Spanish twin's blocks are held to these by the site's i18n check, which
// compares code blocks outside their comments.
func documentedOverlays(t *testing.T) map[string]string {
	t.Helper()

	page := readGuidePage(t, filepath.Join(repoRoot(), "site", "src", "content", "docs", "operations", "telemetry.mdx"))
	overlays := map[string]string{}
	for _, match := range documentedOverlay.FindAllStringSubmatch(page, -1) {
		overlays[match[1]] = match[2] + "\n"
	}
	return overlays
}

// exportProtocols are the two OTLP transports the committed receiver serves,
// each with the value OTEL_EXPORTER_OTLP_PROTOCOL takes for it, the endpoint
// form OTEL_EXPORTER_OTLP_ENDPOINT takes for it (a URL for both, as
// TestRealCollector_GRPCIsAnExportPathToo explains), and the way an export with
// no credential is sent over it.
var exportProtocols = []struct {
	name     string
	protocol string
	endpoint func(*collector) string
	refused  func(*testing.T, *collector)
}{
	{
		name:     "over HTTP",
		protocol: "http/protobuf",
		endpoint: func(c *collector) string { return c.endpoint },
		refused:  assertUnauthenticatedExportRefused,
	},
	{
		name:     "over gRPC",
		protocol: "grpc",
		endpoint: func(c *collector) string { return "http://" + c.grpcEndpoint },
		refused:  assertUnauthenticatedGRPCExportRefused,
	},
}

// TestExample_Authenticators_AdmitTheHeaderTheyPairWith holds each
// authenticator overlay on the telemetry page to the header form the page's
// table gives for the same scheme, over both protocols the receiver serves.
//
// Each overlay is merged onto the committed configuration the way the page
// tells a reader to merge it, with a second --config flag, and the collector
// reads its secret from the environment as the overlay says. Two things are
// asserted per scheme and protocol, because either alone passes vacuously: an
// export with no credential is refused, so the overlay really put that receiver
// behind the authenticator, and this server configured with the documented
// header form delivers its telemetry through it to the backend, so the pairing
// is right. Each overlay protects gRPC and HTTP alike, and the compose file
// publishes both ports, so an overlay that lost its grpc block would leave 4317
// open to anybody; the HTTP half alone could not see that.
func TestExample_Authenticators_AdmitTheHeaderTheyPairWith(t *testing.T) {
	overlays := documentedOverlays(t)

	const (
		token    = "s3cret-token"
		user     = "mcp"
		password = "s3cret" // "mcp:s3cret" encodes with padding, which the page says survives
		apiKey   = "s3cret-key"
	)
	basic := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))

	// The headers are the forms the page's authentication table gives, built
	// the way it says to build them: a space written %20, the basic credential
	// base64 with its padding left on.
	cases := []struct {
		overlay string
		env     map[string]string
		headers string
		probe   http.Header
	}{
		{
			overlay: "auth-bearer.yaml",
			env:     map[string]string{"COLLECTOR_TOKEN": token},
			headers: "Authorization=Bearer%20" + token,
			probe:   http.Header{"Authorization": {"Bearer " + token}},
		},
		{
			overlay: "auth-basic.yaml",
			env:     map[string]string{"COLLECTOR_USER": user, "COLLECTOR_PASSWORD": password},
			headers: "Authorization=Basic%20" + basic,
			probe:   http.Header{"Authorization": {"Basic " + basic}},
		},
		{
			overlay: "auth-api-key.yaml",
			env:     map[string]string{"COLLECTOR_API_KEY": apiKey},
			headers: "api-key=" + apiKey,
			probe:   http.Header{"Api-Key": {apiKey}},
		},
	}

	if got, want := slices.Sorted(maps.Keys(overlays)), []string{"auth-api-key.yaml", "auth-basic.yaml", "auth-bearer.yaml"}; !slices.Equal(got, want) {
		t.Fatalf("the telemetry page carries the overlays %v, want %v; a renamed or added block needs its case here", got, want)
	}

	rig := startExampleRig(t)
	fake := startFakeGitLab(t)

	for _, testCase := range cases {
		t.Run(testCase.overlay, func(t *testing.T) {
			overlay := filepath.Join(t.TempDir(), testCase.overlay)
			if err := os.WriteFile(overlay, []byte(overlays[testCase.overlay]), 0o644); err != nil { //#nosec G306 G703 -- a t.TempDir join holding a block of the committed page; read by a throwaway container
				t.Fatalf("writing the overlay: %v", err)
			}
			example := rig.startExample(t, testCase.env, testCase.probe, overlay)
			scheme := strings.TrimSuffix(strings.TrimPrefix(testCase.overlay, "auth-"), ".yaml")

			for _, transport := range exportProtocols {
				t.Run(transport.name, func(t *testing.T) {
					transport.refused(t, example)

					service := "gitlab-mcp-server-" + scheme + "-" + strings.TrimSuffix(transport.protocol, "/protobuf")
					env := telemetryEnv(example)
					env["OTEL_EXPORTER_OTLP_ENDPOINT"] = transport.endpoint(example)
					env["OTEL_EXPORTER_OTLP_PROTOCOL"] = transport.protocol
					env["OTEL_EXPORTER_OTLP_HEADERS"] = testCase.headers
					env["OTEL_SERVICE_NAME"] = service
					srv := startServer(t, env, "--gitlab-url="+fake)
					srv.callAction(t, 1, "issue.list", "some-group/some-project")

					if _, _, ok := rig.backend.awaitSpan(t, exportDeadline, func(rs otlpResourceSpans, _ otlpSpan) bool {
						name, _ := attr(rs.Resource.Attributes, "service.name")
						return name == service
					}); !ok {
						t.Fatalf("nothing from %s crossed the authenticated receiver %s.\nExample collector:\n%s\nServer:\n%s",
							service, transport.name, example.containerLogs(t), srv.logs())
					}
					assertCollectorLogClean(t, example)
					if logs := srv.logs(); strings.Contains(logs, "opentelemetry sdk error") {
						t.Errorf("the server reported an SDK error, so the authenticated receiver refused what it sent %s:\n%s",
							transport.name, logs)
					}
				})
			}
		})
	}
}

// assertUnauthenticatedExportRefused posts an empty export with no credential
// and requires the receiver to answer 401.
func assertUnauthenticatedExportRefused(t *testing.T, c *collector) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, c.endpoint+"/v1/traces", strings.NewReader(`{"resourceSpans":[]}`))
	if err != nil {
		t.Fatalf("building the unauthenticated export: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("posting the unauthenticated export: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("an export with no credential was answered %d, want 401: the overlay does not put the receiver behind its authenticator",
			resp.StatusCode)
	}
}

// assertUnauthenticatedGRPCExportRefused sends one span over gRPC with no
// credential and requires the receiver to answer Unauthenticated.
//
// The sender is the SDK's own gRPC trace exporter, the one this server builds
// for OTEL_EXPORTER_OTLP_PROTOCOL=grpc, rather than a gRPC client of this
// module's: the refusal under test is the collector's, so the sender only has
// to be a well-formed OTLP client, and this one is already a dependency of the
// server where a client of the module's own would make gRPC one of the tests.
// That is also why the status is read off the error's text, which is where the
// exporter surfaces it to a caller that does not import gRPC. Retries are off,
// so the refusal is the first answer rather than one a minute of backoff later,
// and no headers are set whatever this process's environment carries. The span
// is valid, so an open receiver accepts it and the authenticator is the one
// thing that can refuse it.
func assertUnauthenticatedGRPCExportRefused(t *testing.T, c *collector) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(c.grpcEndpoint),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithHeaders(map[string]string{}),
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{Enabled: false}),
	)
	if err != nil {
		t.Fatalf("building the unauthenticated gRPC export: %v", err)
	}
	defer func() { _ = exporter.Shutdown(context.WithoutCancel(ctx)) }()

	span := tracetest.SpanStub{
		Name: "unauthenticated export",
		SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: trace.TraceID{0x01}, SpanID: trace.SpanID{0x01}, TraceFlags: trace.FlagsSampled,
		}),
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}.Snapshot()
	err = exporter.ExportSpans(ctx, []sdktrace.ReadOnlySpan{span})
	if err == nil || !strings.Contains(err.Error(), "code = Unauthenticated") {
		t.Errorf("an export over gRPC with no credential was answered %v, want Unauthenticated: the overlay does not put the gRPC receiver behind its authenticator",
			err)
	}
}

// The parts of `docker compose config --format json` the example is held to.
type (
	composeProject struct {
		Services map[string]composeService `json:"services"`
	}

	composeService struct {
		Image       string              `json:"image"`
		Command     []string            `json:"command"`
		Environment map[string]*string  `json:"environment"`
		Volumes     []composeVolume     `json:"volumes"`
		Healthcheck *composeHealthcheck `json:"healthcheck"`
	}

	// composeHealthcheck is a health check as `docker compose config` prints
	// it: each duration in Go's spelling, the retries a number, and a field
	// the file does not state left out.
	composeHealthcheck struct {
		Test        []string `json:"test"`
		Interval    string   `json:"interval"`
		Timeout     string   `json:"timeout"`
		StartPeriod string   `json:"start_period"`
		Retries     *uint64  `json:"retries"`
	}

	composeVolume struct {
		Type     string `json:"type"`
		Source   string `json:"source"`
		Target   string `json:"target"`
		ReadOnly bool   `json:"read_only"`
	}
)

// TestExample_ComposeFile_RunsTheCommittedPipeline holds the committed compose
// file to the pipeline the configuration test proved.
//
// `docker compose config` parses, interpolates and normalizes the file and
// exits non-zero on a malformed service or an unknown key, without pulling an
// image or starting anything. What it prints is then held to the rest of the
// chain: the collector service runs the image this module pins, with the
// committed configuration mounted where this module mounts it and no command
// that would read another; the backend service is the host that configuration
// forwards to; and the server exports to the collector service, with telemetry
// on and the image's --probe health check the page shows, restated with the
// timing the Dockerfile gives it, since the file and the page both say so.
func TestExample_ComposeFile_RunsTheCommittedPipeline(t *testing.T) {
	requireDockerCompose(t)
	project := composeConfig(t, filepath.Join(exampleDir(), exampleComposeFile))

	t.Run("the collector runs the pinned image on the committed configuration", func(t *testing.T) {
		collectorService, ok := project.Services[exampleCollectorService]
		if !ok {
			t.Fatalf("no %q service; services are %v", exampleCollectorService, slices.Sorted(maps.Keys(project.Services)))
		}
		if collectorService.Image != collectorImage {
			t.Errorf("the collector service runs %q and this module tests %q: move collectorImage and the compose file together",
				collectorService.Image, collectorImage)
		}
		if len(collectorService.Command) != 0 {
			t.Errorf("the collector service passes %q, so it is not started the way this module starts the committed configuration",
				collectorService.Command)
		}
		want := composeVolume{Type: "bind", Source: filepath.Join(exampleDir(), exampleConfigFile), Target: collectorConfigPath, ReadOnly: true}
		if !slices.Contains(collectorService.Volumes, want) {
			t.Errorf("the collector service mounts %+v, want %+v among them", collectorService.Volumes, want)
		}
	})

	t.Run("the backend is the host the configuration forwards to", func(t *testing.T) {
		if _, ok := project.Services[exampleBackendHost]; !ok {
			t.Errorf("no %q service, which is where the committed configuration forwards; services are %v",
				exampleBackendHost, slices.Sorted(maps.Keys(project.Services)))
		}
	})

	t.Run("the server exports to the collector and is probed by --probe", func(t *testing.T) {
		serverService, ok := project.Services[exampleServerService]
		if !ok {
			t.Fatalf("no %q service; services are %v", exampleServerService, slices.Sorted(maps.Keys(project.Services)))
		}
		if !slices.Contains(serverService.Command, "--telemetry") {
			t.Errorf("the server service's command %q does not turn telemetry on", serverService.Command)
		}
		endpoint := serverService.Environment["OTEL_EXPORTER_OTLP_ENDPOINT"]
		if want := "http://" + exampleCollectorService + ":" + collectorOTLPPort; endpoint == nil || *endpoint != want {
			t.Errorf("the server service exports to %v, want %q", deref(endpoint), want)
		}
		if _, set := serverService.Environment["GITLAB_TOKEN"]; set {
			t.Error("the server service sets GITLAB_TOKEN, which HTTP mode never reads: each client sends its own")
		}
		if serverService.Healthcheck == nil || !slices.Equal(serverService.Healthcheck.Test, []string{"CMD", "gitlab-mcp-server", "--probe"}) {
			t.Errorf("the server service's health check is %+v, want the image's own --probe", serverService.Healthcheck)
		}
	})

	t.Run("the server's health check restates the image's own, timing included", func(t *testing.T) {
		assertRestatesImageHealthCheck(t, project.Services[exampleServerService])
	})
}

// assertRestatesImageHealthCheck holds a compose service's health check to the
// HEALTHCHECK of the Dockerfile the server image is built from, timing
// included.
func assertRestatesImageHealthCheck(t *testing.T, service composeService) {
	t.Helper()

	dockerfile := filepath.Join(repoRoot(), "Dockerfile")
	source, err := os.ReadFile(dockerfile) //#nosec G304 -- a path this test built from repoRoot
	if err != nil {
		t.Fatalf("reading the Dockerfile the server image is built from: %v", err)
	}
	image, err := imageHealthCheck(string(source))
	if err != nil {
		t.Fatalf("reading the HEALTHCHECK of %s: %v", dockerfile, err)
	}
	if service.Healthcheck == nil {
		t.Fatalf("the server service states no health check, and the image's is %v", image)
	}
	stated, err := service.Healthcheck.normalized()
	if err != nil {
		t.Fatalf("the server service's health check does not restate the image's (%v): %v", image, err)
	}
	if !reflect.DeepEqual(stated, image) {
		t.Errorf("the server service's health check is %v and the image's is %v: the compose file and the page say the example restates it",
			stated, image)
	}
}

// healthCheck is a container health check reduced to what decides how it
// runs, so a compose file and a Dockerfile that spell one check differently
// compare equal and two checks that differ in anything do not.
type healthCheck struct {
	test        []string
	interval    time.Duration
	timeout     time.Duration
	startPeriod time.Duration
	retries     uint64
}

// String renders a check for a failure message, its durations in Go's
// spelling rather than as the nanosecond counts the fields hold.
func (h healthCheck) String() string {
	return fmt.Sprintf("%q every %s, timeout %s, start period %s, %d retries",
		h.test, h.interval, h.timeout, h.startPeriod, h.retries)
}

// Docker's values for a HEALTHCHECK option the instruction leaves out.
const (
	dockerDefaultInterval = 30 * time.Second
	dockerDefaultTimeout  = 30 * time.Second
	dockerDefaultRetries  = 3
)

// imageHealthCheck reads the HEALTHCHECK instruction of a Dockerfile, the last
// one where there are several since that is the one Docker keeps, as the check
// a compose file would restate: its command in the exec form after CMD, and its
// timing, with Docker's default for an option it leaves out. An option the
// comparison does not know, a shell-form command and HEALTHCHECK NONE are
// errors rather than a partial reading, so a Dockerfile that gains one is
// looked at instead of passing on what this read of it happens to cover.
func imageHealthCheck(dockerfile string) (healthCheck, error) {
	var instruction string
	found := false
	for line := range strings.SplitSeq(strings.ReplaceAll(dockerfile, "\\\n", " "), "\n") {
		if word, rest := cutWord(line); strings.EqualFold(word, "HEALTHCHECK") {
			instruction, found = rest, true
		}
	}
	if !found {
		return healthCheck{}, errors.New("no HEALTHCHECK instruction")
	}

	check := healthCheck{interval: dockerDefaultInterval, timeout: dockerDefaultTimeout, retries: dockerDefaultRetries}
	durations := map[string]*time.Duration{
		"interval":     &check.interval,
		"timeout":      &check.timeout,
		"start-period": &check.startPeriod,
	}
	word, rest := cutWord(instruction)
	for ; strings.HasPrefix(word, "--"); word, rest = cutWord(rest) {
		name, value, hasValue := strings.Cut(strings.TrimPrefix(word, "--"), "=")
		if !hasValue {
			return healthCheck{}, fmt.Errorf("option %q has no value", word)
		}
		if into, isDuration := durations[name]; isDuration {
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return healthCheck{}, fmt.Errorf("option %q: %w", word, err)
			}
			*into = parsed
			continue
		}
		if name != "retries" {
			return healthCheck{}, fmt.Errorf("option %q is not one this comparison reads", word)
		}
		retries, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return healthCheck{}, fmt.Errorf("option %q: %w", word, err)
		}
		check.retries = retries
	}
	if !strings.EqualFold(word, "CMD") {
		return healthCheck{}, fmt.Errorf("the instruction runs %q rather than a CMD", word)
	}
	var argv []string
	if err := json.Unmarshal([]byte(rest), &argv); err != nil || len(argv) == 0 {
		return healthCheck{}, fmt.Errorf("the command %q is not in the exec form a compose test restates", rest)
	}
	check.test = append([]string{"CMD"}, argv...)
	return check, nil
}

// cutWord splits off the first whitespace-delimited word of s and returns it
// with the rest, both trimmed.
func cutWord(s string) (word, rest string) {
	s = strings.TrimSpace(s)
	if end := strings.IndexFunc(s, unicode.IsSpace); end >= 0 {
		return s[:end], strings.TrimSpace(s[end:])
	}
	return s, ""
}

// normalized reduces a compose health check to the shape imageHealthCheck
// reads. Every timing has to be stated: the file says it restates the image's,
// and a field it leaves out would be the image's value without the file
// showing it.
func (h composeHealthcheck) normalized() (healthCheck, error) {
	check := healthCheck{test: h.Test}
	for _, duration := range []struct {
		name  string
		value string
		into  *time.Duration
	}{
		{"interval", h.Interval, &check.interval},
		{"timeout", h.Timeout, &check.timeout},
		{"start_period", h.StartPeriod, &check.startPeriod},
	} {
		if duration.value == "" {
			return healthCheck{}, fmt.Errorf("%s is not stated", duration.name)
		}
		parsed, err := time.ParseDuration(duration.value)
		if err != nil {
			return healthCheck{}, fmt.Errorf("%s: %w", duration.name, err)
		}
		*duration.into = parsed
	}
	if h.Retries == nil {
		return healthCheck{}, errors.New("retries is not stated")
	}
	check.retries = *h.Retries
	return check, nil
}

// TestImageHealthCheck_Dockerfile_ReadsTheCheckDockerRuns pins how the
// Dockerfile's HEALTHCHECK is read, on instructions of the shapes Docker
// accepts, so the comparison above cannot pass by misreading the one the
// repository's Dockerfile carries.
func TestImageHealthCheck_Dockerfile_ReadsTheCheckDockerRuns(t *testing.T) {
	probe := []string{"CMD", "gitlab-mcp-server", "--probe"}
	cases := []struct {
		name       string
		dockerfile string
		want       healthCheck
		wantErr    string
	}{
		{
			name:       "the committed shape, continued onto a tab-indented line",
			dockerfile: "FROM scratch\nHEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \\\n\tCMD [\"gitlab-mcp-server\", \"--probe\"]\nUSER appuser\n",
			want:       healthCheck{test: probe, interval: 30 * time.Second, timeout: 5 * time.Second, startPeriod: 10 * time.Second, retries: 3},
		},
		{
			name:       "options left out take Docker's defaults",
			dockerfile: "healthcheck CMD [\"gitlab-mcp-server\", \"--probe\"]\n",
			want:       healthCheck{test: probe, interval: 30 * time.Second, timeout: 30 * time.Second, retries: 3},
		},
		{
			name:       "the last instruction is the one Docker keeps",
			dockerfile: "HEALTHCHECK --retries=9 CMD [\"old\"]\nHEALTHCHECK --interval=1m CMD [\"gitlab-mcp-server\", \"--probe\"]\n",
			want:       healthCheck{test: probe, interval: time.Minute, timeout: 30 * time.Second, retries: 3},
		},
		{
			name:       "no instruction",
			dockerfile: "FROM scratch\n# HEALTHCHECK is described here and never given\n",
			wantErr:    "no HEALTHCHECK instruction",
		},
		{
			name:       "HEALTHCHECK NONE",
			dockerfile: "HEALTHCHECK NONE\n",
			wantErr:    `runs "NONE"`,
		},
		{
			name:       "a shell-form command",
			dockerfile: "HEALTHCHECK CMD gitlab-mcp-server --probe\n",
			wantErr:    "not in the exec form",
		},
		{
			name:       "an empty exec form",
			dockerfile: "HEALTHCHECK CMD []\n",
			wantErr:    "not in the exec form",
		},
		{
			name:       "an option the comparison does not read",
			dockerfile: "HEALTHCHECK --start-interval=5s CMD [\"gitlab-mcp-server\", \"--probe\"]\n",
			wantErr:    `"--start-interval=5s" is not one this comparison reads`,
		},
		{
			name:       "an option with no value",
			dockerfile: "HEALTHCHECK --interval CMD [\"gitlab-mcp-server\", \"--probe\"]\n",
			wantErr:    `"--interval" has no value`,
		},
		{
			name:       "a duration Go cannot read",
			dockerfile: "HEALTHCHECK --timeout=soon CMD [\"gitlab-mcp-server\", \"--probe\"]\n",
			wantErr:    `"--timeout=soon"`,
		},
		{
			name:       "retries that are not a count",
			dockerfile: "HEALTHCHECK --retries=-1 CMD [\"gitlab-mcp-server\", \"--probe\"]\n",
			wantErr:    `"--retries=-1"`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := imageHealthCheck(testCase.dockerfile)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Errorf("imageHealthCheck() = %v, %v; want an error containing %q", got, err, testCase.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("imageHealthCheck() = %v, %v; want %v", got, err, testCase.want)
			}
		})
	}
}

// TestComposeHealthcheck_Normalized_RequiresEveryTimingStated pins that a
// compose health check is compared on every value it restates, and that one
// it leaves out fails rather than defaulting to the image's.
func TestComposeHealthcheck_Normalized_RequiresEveryTimingStated(t *testing.T) {
	probe := []string{"CMD", "gitlab-mcp-server", "--probe"}
	retries := uint64(3)
	complete := composeHealthcheck{Test: probe, Interval: "30s", Timeout: "5s", StartPeriod: "10s", Retries: &retries}
	edited := func(edit func(*composeHealthcheck)) composeHealthcheck {
		check := complete
		edit(&check)
		return check
	}
	cases := []struct {
		name    string
		check   composeHealthcheck
		want    healthCheck
		wantErr string
	}{
		{
			name:  "every value stated",
			check: complete,
			want:  healthCheck{test: probe, interval: 30 * time.Second, timeout: 5 * time.Second, startPeriod: 10 * time.Second, retries: 3},
		},
		{
			name:  "a duration in another spelling",
			check: edited(func(c *composeHealthcheck) { c.Interval = "0h0m30s" }),
			want:  healthCheck{test: probe, interval: 30 * time.Second, timeout: 5 * time.Second, startPeriod: 10 * time.Second, retries: 3},
		},
		{
			name:    "no interval",
			check:   edited(func(c *composeHealthcheck) { c.Interval = "" }),
			wantErr: "interval is not stated",
		},
		{
			name:    "no timeout",
			check:   edited(func(c *composeHealthcheck) { c.Timeout = "" }),
			wantErr: "timeout is not stated",
		},
		{
			name:    "no start period",
			check:   edited(func(c *composeHealthcheck) { c.StartPeriod = "" }),
			wantErr: "start_period is not stated",
		},
		{
			name:    "no retries",
			check:   edited(func(c *composeHealthcheck) { c.Retries = nil }),
			wantErr: "retries is not stated",
		},
		{
			name:    "a duration Go cannot read",
			check:   edited(func(c *composeHealthcheck) { c.Timeout = "soon" }),
			wantErr: "timeout: ",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.check.normalized()
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Errorf("normalized() = %v, %v; want an error containing %q", got, err, testCase.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("normalized() = %v, %v; want %v", got, err, testCase.want)
			}
		})
	}
}

// composeConfig returns what `docker compose config` makes of a compose file,
// and ends the test when the file is missing or the command refuses it.
func composeConfig(t *testing.T, compose string) composeProject {
	t.Helper()

	if _, err := os.Stat(compose); err != nil {
		t.Fatalf("the committed example compose file is missing: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	//#nosec G204 -- the only interpolated argument is a path this test built from repoRoot.
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", compose, "config", "--format", "json")
	// The defaults the file falls back to are what is under test, so a value a
	// developer exported for another purpose is not allowed to stand in.
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "GITLAB_URL=") || strings.HasPrefix(kv, "GITLAB_MCP_TELEMETRY_IDENTITY=")
	})
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker compose config rejected the committed example compose file: %v\n%s", err, exitStderr(err))
	}
	var project composeProject
	if decodeErr := json.Unmarshal(out, &project); decodeErr != nil {
		t.Fatalf("decoding docker compose config: %v\n%s", decodeErr, out)
	}
	return project
}

// deref renders an optional compose value for a failure message.
func deref(value *string) string {
	if value == nil {
		return "nothing"
	}
	return `"` + *value + `"`
}

// exitStderr returns what a failed command wrote to stderr, which Output keeps
// on the error rather than in what it returns.
func exitStderr(err error) string {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(exitErr.Stderr)
	}
	return ""
}

// requireDockerCompose skips unless the docker compose plugin is present.
//
// Unlike requireDocker this does not need a running daemon: `docker compose
// config` only parses, so the one thing it needs is the CLI plugin. A developer
// without it is told why the check did not run rather than handed a failure.
func requireDockerCompose(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available; the compose file is validated with `docker compose config`, skipped rather than modeled")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "compose", "version").Run(); err != nil {
		t.Skip("the docker compose plugin is not usable; skipping the compose-file validation")
	}
}
