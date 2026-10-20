//go:build collectore2e

// collector_test.go runs a genuine OpenTelemetry Collector and reads back what
// it parsed out of this server's exports.
//
// The receiver is the thing under test here, which is why the pipeline ends in
// a file exporter: the collector writes OTLP JSON only after decoding the
// protobuf, routing it through a pipeline and re-encoding it, so a document
// appearing in that file is evidence that a real implementation understood what
// we sent. Reading raw bytes off a socket, which is what the in-process stub in
// test/e2e/http does, proves the bytes were delivered and nothing about whether
// they mean anything.
package collectore2e

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// collectorImage pins the receiver under test.
//
// Never latest. A floating tag makes the suite's meaning change without a
// commit, so a collector release that tightened validation would either fail a
// run nobody had changed anything in, or, worse, relax one and let a defect
// through unannounced. Bumping this is a deliberate act with a diff, and it
// moves the collector in examples/telemetry/compose.yaml with it, which
// TestExample_ComposeFile_RunsTheCommittedPipeline holds to this tag.
const collectorImage = "otel/opentelemetry-collector-contrib:0.163.1"

// The container-side paths. The receiver binds 0.0.0.0 rather than the image
// default of localhost, because a published port reaches the container from
// outside its loopback and a receiver on localhost would simply never be
// reached, which presents as a timeout with a perfectly healthy collector.
//
// Three file exporters rather than one shared instance: the collector builds a
// separate exporter per pipeline, so a single path would have three writers
// appending to one file and the interleaving would be ours to untangle.
const (
	collectorOTLPPort = "4318"
	// The gRPC receiver's port, published alongside the HTTP one so a single
	// collector serves both protocols and a test picks by endpoint.
	collectorGRPCPort = "4317"
	tracesFile        = "traces.json"
	metricsFile       = "metrics.json"
	logsFile          = "logs.json"
)

// collectorConfig is the pipeline the container runs.
//
// All three signal types are wired even though only traces and metrics are
// asserted on, and the third one is insurance rather than need. This server
// starts a logs provider and publishes "logs" among the signals on its server
// card, but nothing in it currently bridges slog to that provider, so no log
// record is ever exported: the suite passes today with the logs pipeline
// removed. The day a bridge is added, a receiver without a logs pipeline
// answers /v1/logs with 404 (measured against this image, not assumed), the
// server's SDK error handler records the refusal, and the acceptance subtest
// below fails, reporting a defect in this file as though the server had emitted
// something invalid. Wiring the pipeline costs three lines and removes that.
//
// The debug exporter is at basic verbosity, not detailed. It prints one counted
// line per batch, which is a readable trace of what traversed the pipeline and
// leaves the container log small enough that "no error line appears in it" is
// an assertion a human can also check by eye.
const collectorConfig = `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:` + collectorOTLPPort + `
      grpc:
        endpoint: 0.0.0.0:` + collectorGRPCPort + `

exporters:
  file/traces:
    path: /out/` + tracesFile + `
  file/metrics:
    path: /out/` + metricsFile + `
  file/logs:
    path: /out/` + logsFile + `
  debug:
    verbosity: basic

service:
  telemetry:
    logs:
      level: info
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [file/traces, debug]
    metrics:
      receivers: [otlp]
      exporters: [file/metrics, debug]
    logs:
      receivers: [otlp]
      exporters: [file/logs, debug]
`

// collectorConfigPath is where the image reads its configuration when it is
// started with no --config flag. Every collector here is started that way, and
// so is the one examples/telemetry/compose.yaml runs, which mounts the
// committed configuration at the same path; TestExample_ComposeFile_RunsTheCommittedPipeline
// holds the two to it. collectorConfigDir is the directory, where an overlay
// is mounted beside it.
const (
	collectorConfigDir  = "/etc/otelcol-contrib"
	collectorConfigPath = collectorConfigDir + "/config.yaml"
)

// collectorSpec is what a collector container is started with beyond what
// every one here shares (the image, the published ports, the user, the output
// directory).
type collectorSpec struct {
	// configs are host paths of configuration files, in the order the
	// collector merges them. The first is mounted at collectorConfigPath; each
	// further one is an overlay, mounted beside it and named with a --config
	// flag of its own after the first, which is how a reader adds one of the
	// authenticators the telemetry page documents to the committed example.
	configs []string
	// network is a Docker network the container joins, and alias the name the
	// other containers on it reach it by. Both are empty for a collector the
	// server reaches through its published port alone.
	network string
	alias   string
	// env is set in the container, for a configuration that reads ${env:NAME}.
	env map[string]string
	// probeHeader is sent with the readiness probe, for a receiver that
	// refuses an export without a credential.
	probeHeader http.Header
}

// collector is a running OpenTelemetry Collector container.
type collector struct {
	// endpoint is what OTEL_EXPORTER_OTLP_ENDPOINT is set to.
	endpoint string
	// grpcEndpoint is the same collector reached over the other protocol,
	// which nothing exercised until it existed.
	grpcEndpoint string
	// outDir is the host side of the bind mount the file exporters write into.
	outDir string
	name   string
	// probeHeader is what waitReceiving sends, from collectorSpec.
	probeHeader http.Header
}

// startCollector runs the collector for the duration of the test.
//
// Docker unavailability is a skip and never a failure: a developer without a
// daemon should be told why this suite did not run, not handed a red test they
// cannot act on. A container that starts and then refuses to serve is the
// opposite, and fails: at that point the configuration above is wrong, which is
// this module's own defect rather than the environment's.
func startCollector(t *testing.T) *collector {
	t.Helper()
	return runCollector(t, collectorSpec{configs: []string{writeCollectorConfig(t)}})
}

// writeCollectorConfig writes collectorConfig to a file of the test's own and
// returns its path, for a spec that runs this module's pipeline.
func writeCollectorConfig(t *testing.T) string {
	t.Helper()

	confPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(confPath, []byte(collectorConfig), 0o644); err != nil { //#nosec G306 -- read by a throwaway container
		t.Fatalf("writing the collector config: %v", err)
	}
	return confPath
}

// runCollector runs a collector described by spec for the duration of the
// test, through the start, wait and cleanup path every collector here shares.
// startCollector is the common case: this module's own pipeline, reached
// through its published ports and nothing else.
func runCollector(t *testing.T, spec collectorSpec) *collector {
	t.Helper()

	requireDocker(t)
	if len(spec.configs) == 0 {
		t.Fatal("a collector needs at least one configuration file")
	}

	hostPort := freePort(t)
	grpcPort := freePort(t)
	outDir := filepath.Join(t.TempDir(), "out")
	// World-writable, because a throwaway container writes here as its own
	// user. MkdirAll applies the process umask, which on most machines clears
	// the group and other write bits that user needs, so the mode is set
	// again afterwards with Chmod, which does not.
	if err := os.MkdirAll(outDir, 0o777); err != nil { //#nosec G301 -- a throwaway container writes here as its own user
		t.Fatalf("creating the collector output directory: %v", err)
	}
	if err := os.Chmod(outDir, 0o777); err != nil { //#nosec G302 -- the same directory, with the bits the umask took back
		t.Fatalf("opening the collector output directory to the container: %v", err)
	}

	name := "gitlab-mcp-collectore2e-" + strconv.Itoa(hostPort)
	args := []string{
		"run", "-d", "--name", name,
		"-p", "127.0.0.1:" + strconv.Itoa(hostPort) + ":" + collectorOTLPPort,
		"-p", "127.0.0.1:" + strconv.Itoa(grpcPort) + ":" + collectorGRPCPort,
	}
	// Run as the invoking user so the exported files are readable here and
	// removable by t.TempDir's cleanup. Without this the image's own uid 10001
	// owns them, and the cleanup of a suite run by an ordinary user fails on
	// files it may not delete.
	if uid := os.Getuid(); uid >= 0 {
		args = append(args, "--user", strconv.Itoa(uid)+":"+strconv.Itoa(os.Getgid()))
	}
	if spec.network != "" {
		args = append(args, "--network", spec.network, "--network-alias", spec.alias)
	}
	for _, key := range slices.Sorted(maps.Keys(spec.env)) {
		args = append(args, "-e", key+"="+spec.env[key])
	}
	args = append(args, "-v", outDir+":/out")
	containerConfigs := make([]string, len(spec.configs))
	for i, hostPath := range spec.configs {
		containerConfigs[i] = collectorConfigPath
		if i > 0 {
			containerConfigs[i] = collectorConfigDir + "/overlay-" + strconv.Itoa(i) + ".yaml"
		}
		args = append(args, "-v", hostPath+":"+containerConfigs[i]+":ro")
	}
	args = append(args, collectorImage)
	// One file is started the way the image starts by default, which reads
	// collectorConfigPath; an overlay needs every file named, since a --config
	// flag replaces the default rather than adding to it.
	if len(containerConfigs) > 1 {
		for _, path := range containerConfigs {
			args = append(args, "--config="+path)
		}
	}

	// Generous, because this may include pulling the image on a cold machine.
	runCtx, runCancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer runCancel()
	if out, err := exec.CommandContext(runCtx, "docker", args...).CombinedOutput(); err != nil {
		// requireDocker has already established that a daemon answers, so a
		// failure here is usually this module's own: a configuration the
		// collector rejects, a flag it does not know, a bind mount it cannot
		// make. Reporting those as an environmental skip is how a broken
		// harness stays green, which is the failure this module exists to
		// prevent in the server and should not commit itself.
		//
		// The exception is reaching the registry. A machine with a working
		// daemon and no route to the image cannot run this and has nothing to
		// fix in the code, so that stays a skip.
		if isRegistryFailure(out) {
			t.Skipf("could not pull %s (%v):\n%s", collectorImage, err, out)
		}
		t.Fatalf("could not start %s (%v):\n%s", collectorImage, err, out)
	}

	c := &collector{
		endpoint:     "http://127.0.0.1:" + strconv.Itoa(hostPort),
		grpcEndpoint: "127.0.0.1:" + strconv.Itoa(grpcPort),
		outDir:       outDir,
		name:         name,
		probeHeader:  spec.probeHeader,
	}
	t.Cleanup(func() {
		// Not t.Context: cleanup runs after the test context is cancelled, and
		// a container left behind would collide with the next run by name.
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		//#nosec G204 -- the container name is this file's own literal plus a port number
		_ = exec.CommandContext(rmCtx, "docker", "rm", "-f", c.name).Run()
	})

	c.waitReceiving(t)
	return c
}

// requireDocker skips unless a usable daemon is present.
func requireDocker(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("this module bind-mounts POSIX paths into a Linux container; run it from WSL or a Linux machine")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available; a real collector is the whole point here, so this is skipped rather than modeled")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skip("docker is installed but not usable; skipping the real-collector suite")
	}
}

// waitReceiving polls the OTLP endpoint until it accepts an export.
//
// The probe is an empty but well-formed OTLP JSON document rather than a health
// endpoint, because what the tests need to know is that the receiver is taking
// exports, not that the process is up. Those differ for several seconds while
// the collector builds its pipelines, and a server that exported into that
// window would have its batch refused for reasons that are nobody's defect.
func (c *collector) waitReceiving(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
			c.endpoint+"/v1/traces", strings.NewReader(`{"resourceSpans":[]}`))
		if err != nil {
			t.Fatalf("building the collector readiness probe: %v", err)
		}
		maps.Copy(req.Header, c.probeHeader)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the collector never accepted an export. Container output:\n%s", c.containerLogs(t))
}

// dockerNetwork creates a Docker network for the duration of the test and
// returns its name, so containers started on it reach each other by alias.
//
// Created before the containers that join it, so its cleanup, registered
// first, runs after theirs: Docker refuses to remove a network a container is
// still attached to.
func dockerNetwork(t *testing.T) string {
	t.Helper()

	requireDocker(t)
	name := "gitlab-mcp-collectore2e-net-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	//#nosec G204 -- the network name is this file's own literal plus a pid and a clock reading
	if out, err := exec.CommandContext(ctx, "docker", "network", "create", name).CombinedOutput(); err != nil {
		t.Fatalf("could not create the network %s (%v):\n%s", name, err, out)
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		//#nosec G204 -- the same name
		_ = exec.CommandContext(rmCtx, "docker", "network", "rm", name).Run()
	})
	return name
}

// containerLogs returns everything the collector has written to its own log.
func (c *collector) containerLogs(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	//#nosec G204 -- the container name is this file's own literal plus a port number
	out, err := exec.CommandContext(ctx, "docker", "logs", c.name).CombinedOutput()
	if err != nil {
		return "could not read the container log: " + err.Error()
	}
	return string(out)
}

// documents decodes the OTLP JSON the file exporter has written so far.
//
// A line that will not parse is skipped rather than reported. The exporter
// appends whole documents, so the only way to see a broken one is to read the
// file while a line is mid-write; every caller polls, so the next read has it
// intact. Failing on it would turn a timing artifact into a test failure, and
// nothing is hidden by the tolerance, since a document that never becomes
// parseable is a document the caller times out waiting for.
func documents[T any](t *testing.T, path string) []T {
	t.Helper()

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var out []T
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var doc T
		if json.Unmarshal([]byte(line), &doc) != nil {
			continue
		}
		out = append(out, doc)
	}
	return out
}

// awaitSpan blocks until the collector has parsed a span the predicate accepts,
// and reports whether one arrived.
//
// Waiting is not optional. The batch processor exports on a schedule and the
// file exporter flushes on another, so a test that read immediately would find
// an empty file and could only assert emptiness, which every broken server also
// satisfies.
//
// The outcome is returned rather than fataled on, because the interesting
// failure is diagnosed from two logs this type can only see one of. A collector
// that refuses the export produces exactly this timeout, and the sentence that
// explains it is in the server's log, not in the collector's.
func (c *collector) awaitSpan(t *testing.T, within time.Duration, match func(otlpResourceSpans, otlpSpan) bool) (rs otlpResourceSpans, found otlpSpan, ok bool) {
	t.Helper()

	var seen int
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		seen = 0
		for _, doc := range documents[traceDocument](t, filepath.Join(c.outDir, tracesFile)) {
			for _, resourceSpans := range doc.ResourceSpans {
				for _, scopeSpans := range resourceSpans.ScopeSpans {
					for _, span := range scopeSpans.Spans {
						seen++
						if match(resourceSpans, span) {
							return resourceSpans, span, true
						}
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Logged rather than returned: it separates "the collector parsed nothing"
	// from "it parsed spans and none was the one asked for", which are
	// different defects, and the caller's message is about neither.
	t.Logf("waited %s for a matching span; the collector parsed %d span(s) in total", within, seen)
	return otlpResourceSpans{}, otlpSpan{}, false
}

// awaitMetric blocks until the collector has parsed a metric with this name,
// and reports whether one arrived. It returns rather than fatals for the reason
// [collector.awaitSpan] gives.
func (c *collector) awaitMetric(t *testing.T, within time.Duration, name string) (rm otlpResourceMetrics, found otlpMetric, ok bool) {
	t.Helper()

	var seen []string
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		seen = nil
		for _, doc := range documents[metricDocument](t, filepath.Join(c.outDir, metricsFile)) {
			for _, resourceMetrics := range doc.ResourceMetrics {
				for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
					for _, metric := range scopeMetrics.Metrics {
						seen = append(seen, metric.Name)
						if metric.Name == name {
							return resourceMetrics, metric, true
						}
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("waited %s for a metric named %q; the collector parsed %v", within, name, seen)
	return otlpResourceMetrics{}, otlpMetric{}, false
}

// The OTLP JSON the file exporter writes, decoded down to the fields under
// assertion and no further.
//
// These are hand-written rather than taken from a pdata or proto module on
// purpose. Importing the collector's own types to check the collector's own
// output would make both halves agree by construction, and this suite exists
// precisely to compare what we emit against an independent reading of it.
type (
	// otlpAttr is one key-value attribute. Only the string case is decoded:
	// every attribute asserted on here is a string, and a non-string one still
	// shows up by key, which is what a presence check needs.
	otlpAttr struct {
		Key   string `json:"key"`
		Value struct {
			StringValue string `json:"stringValue"`
		} `json:"value"`
	}

	otlpResource struct {
		Attributes []otlpAttr `json:"attributes"`
	}

	otlpSpan struct {
		Name string `json:"name"`
		// Kind is the SpanKind enum. The exporter writes it as a number, so
		// SPAN_KIND_SERVER arrives as 2 rather than by name.
		Kind       int        `json:"kind"`
		Attributes []otlpAttr `json:"attributes"`
		// The identifiers, which are what makes a tree a tree. An empty
		// parentSpanId is how the exporter writes a root, so the zero value
		// carries meaning here rather than being an absence.
		TraceID      string `json:"traceId"`
		SpanID       string `json:"spanId"`
		ParentSpanID string `json:"parentSpanId"`
		Links        []struct {
			TraceID string `json:"traceId"`
			SpanID  string `json:"spanId"`
		} `json:"links"`
	}

	otlpScopeSpans struct {
		Scope otlpScope  `json:"scope"`
		Spans []otlpSpan `json:"spans"`
	}

	otlpResourceSpans struct {
		Resource   otlpResource     `json:"resource"`
		ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
	}

	traceDocument struct {
		ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
	}

	otlpScope struct {
		Name string `json:"name"`
	}

	otlpMetricBody struct {
		DataPoints []json.RawMessage `json:"dataPoints"`
	}

	otlpMetric struct {
		Name                 string          `json:"name"`
		Unit                 string          `json:"unit"`
		Histogram            *otlpMetricBody `json:"histogram"`
		Sum                  *otlpMetricBody `json:"sum"`
		Gauge                *otlpMetricBody `json:"gauge"`
		ExponentialHistogram *otlpMetricBody `json:"exponentialHistogram"`
	}

	otlpScopeMetrics struct {
		Scope   otlpScope    `json:"scope"`
		Metrics []otlpMetric `json:"metrics"`
	}

	otlpResourceMetrics struct {
		Resource     otlpResource       `json:"resource"`
		ScopeMetrics []otlpScopeMetrics `json:"scopeMetrics"`
	}

	metricDocument struct {
		ResourceMetrics []otlpResourceMetrics `json:"resourceMetrics"`
	}
)

// spanKindServer is SPAN_KIND_SERVER in the OTLP enum.
const spanKindServer = 2

// attr returns the string value recorded under a key, and whether the key was
// present at all. The two are separate answers: an attribute set to the empty
// string and an attribute nobody set are different defects.
// awaitDurationPoint waits for a data point of the duration instrument that
// match accepts, and reports what it read when it gives up.
//
// [collector.awaitMetric] answers with the first export carrying the
// instrument, which is not the same question. A process that has just served
// one call exports the handshake's series first, so a test that waits for the
// instrument and then reads whatever that export happened to hold asserts
// about export timing rather than about the server: it passes alone and fails
// under load, which is how two assertions here came to be believed.
// It returns the whole instrument the point was found in as well, because a
// caller asserting about the inventory has to read the export that carries the
// series it waited for rather than an earlier one.
func awaitDurationPoint(t *testing.T, c *collector, within time.Duration, match func([]otlpAttr) bool) (point []otlpAttr, in otlpMetric, ok bool) {
	t.Helper()

	var seen []string
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		seen = nil
		for _, candidate := range durationPoints(t, c) {
			method, _ := attr(candidate.attributes, "mcp.method.name")
			seen = append(seen, method)
			if match(candidate.attributes) {
				return candidate.attributes, candidate.in, true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("waited %s for a matching %s data point; the methods exported were %v", within, durationMetric, seen)
	return nil, otlpMetric{}, false
}

// durationPoint is one data point of the duration instrument and the export it
// arrived in, which a caller asserting about the whole inventory needs.
type durationPoint struct {
	attributes []otlpAttr
	in         otlpMetric
}

// durationPoints flattens every duration data point the collector has parsed
// so far, across every document, resource and scope.
func durationPoints(t *testing.T, c *collector) []durationPoint {
	t.Helper()

	var out []durationPoint
	for _, doc := range documents[metricDocument](t, filepath.Join(c.outDir, metricsFile)) {
		for _, resourceMetrics := range doc.ResourceMetrics {
			for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
				for _, metric := range scopeMetrics.Metrics {
					if metric.Name != durationMetric {
						continue
					}
					for _, attributes := range dataPointAttributes(t, metric) {
						out = append(out, durationPoint{attributes: attributes, in: metric})
					}
				}
			}
		}
	}
	return out
}

func attr(attrs []otlpAttr, key string) (value string, present bool) {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.StringValue, true
		}
	}
	return "", false
}

// keys lists what was recorded, for a failure message that says what arrived
// instead of only what did not.
func keys(attrs []otlpAttr) []string {
	out := make([]string, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, a.Key)
	}
	return out
}

// isRegistryFailure reports whether docker failed because it could not obtain
// the image rather than because it could not run it.
//
// Matching on the message is coarse, and it is the only signal available:
// docker exits 125 for both "cannot pull" and "bad configuration", so the exit
// status cannot separate them. The list is deliberately narrow, because the
// cost of a wrong match in this direction is a skipped test that should have
// failed.
func isRegistryFailure(output []byte) bool {
	text := strings.ToLower(string(output))
	for _, marker := range []string{
		"pull access denied",
		"manifest unknown",
		"no such host",
		"connection refused while trying to connect",
		"timeout exceeded while awaiting headers",
		"error pulling image",
		"failed to resolve reference",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
