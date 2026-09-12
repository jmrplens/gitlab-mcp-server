//go:build e2e

// baseline_recorder_test.go writes down what this suite asked the server to
// do and what the server dispatched, so the suite that replaces it has a
// baseline to be held to.
//
// The suite is frozen apart from this instrumentation. Its coverage used to be
// credited by cmd/audit_e2e_gaps from mentions in the source, which counts an
// action nothing runs; the rebuild credits only a call the server dispatched,
// in a test that passed, on a named runtime, surface and mode. Before the old
// suite can be deleted, the new one has to reach every credit the old one
// reached, and that comparison needs the old suite recorded on the same
// terms. This file is that record, written through internal/testutil/e2ecalls
// whenever GITLAB_MCP_TEST_E2E_CALLS_DIR is set and silent otherwise.
//
// Two halves make one line. On the server side, every in-process server gets
// the same telemetry middleware the binary installs, with a tracer provider
// that keeps each span in memory instead of exporting it: the dispatched
// action is then what production telemetry says it is, computed by the same
// code, and not a second reading of the dispatch made here. On the client
// side, a sending middleware stamps a trace into every call, attributes it to
// the test that made it, and joins the span back on that trace. There is no
// Env here to carry the attribution, so the test is read off the goroutine's
// stack: the closure a subtest runs in is named after the function that
// declared it, and the file and line of that frame say which t.Run literal it
// sits inside. When that function is a helper rather than the Test function,
// which is how the pipeline lifecycles are written, the rest of the name is
// on the goroutine that created this one, parked in t.Run, and the walk
// follows the creators up to it. The ledger's cleanups run with no test
// frame on the stack at all, so they carry their test on the context instead.
//
// Nothing here buffers per test: a line is written as soon as its call
// answers, without a test status, and the gotestsum JSON the Docker targets
// already write supplies the verdict when cmd/audit_e2e_coverage joins the
// two with -results. The status a line would carry at write time is the
// status of a test still running, which is no status at all.

package suite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The MCP methods the recorder writes a line for. Spelled here because the
// SDK keeps its constants unexported, and the method is part of the record's
// contract with cmd/audit_e2e_coverage.
const (
	baselineMethodCallTool     = "tools/call"
	baselineMethodReadResource = "resources/read"
	baselineMethodGetPrompt    = "prompts/get"
	baselineMethodComplete     = "completion/complete"
)

// The protective modes, in the vocabulary the coverage audit reads off every
// session and call line.
const (
	baselineModeDefault  = "default"
	baselineModeReadOnly = "read-only"
	baselineModeSafe     = "safe"
)

// baselineTransport names how this suite reaches its servers. The audit does
// not classify on it; it is written so a reader of a shard can tell these
// lines from the rebuilt suite's, which drives the binary over stdio.
const baselineTransport = "in-memory"

// baselinePackage names this package in the run line. The rebuilt suite
// writes the directory name of each of its packages there, and this is the
// old suite's.
const baselinePackage = "suite"

// baselineTraceParentKey is the _meta key W3C trace context travels in. The
// server's telemetry middleware reads it there, which is the whole of the
// client side of the join.
const baselineTraceParentKey = "traceparent"

// baselineShape is what a session is, in the vocabulary the record carries on
// every line: which surface it serves, in which protective mode, with which
// resource surface.
type baselineShape struct {
	label        string
	surface      string
	mode         string
	capabilities string
}

// baselineDispatch is what one server span said about the call it served:
// the same five attributes and status the rebuilt harness reads out of the
// binary's OTLP export, read here off the in-process span.
type baselineDispatch struct {
	tool          string
	action        string
	domain        string
	refusalReason string
	errorType     string
	status        string
}

// empty reports whether the span carried none of the attributes the record
// is made of, which is every span that is not a tool call.
func (d baselineDispatch) empty() bool {
	return d.tool == "" && d.action == "" && d.domain == "" && d.refusalReason == "" && d.errorType == ""
}

// baselineSpans is the span processor every in-process server's tracer ends
// its spans in. It keeps what each server span said, keyed by the trace the
// client stamped into the call.
//
// It is a processor rather than an exporter because a processor sees the
// span synchronously, on the server goroutine, before the response is
// written: by the time the client middleware reads the answer, the span is
// already here.
type baselineSpans struct {
	mu      sync.Mutex
	byTrace map[string]baselineDispatch
}

// OnStart is part of the processor interface and has nothing to do.
func (s *baselineSpans) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

// OnEnd keeps what a server span said about a tool call, and ignores every
// other span.
func (s *baselineSpans) OnEnd(span sdktrace.ReadOnlySpan) {
	if span.SpanKind() != trace.SpanKindServer {
		return
	}
	facts := baselineSpanFacts(span)
	if facts.empty() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byTrace[span.SpanContext().TraceID().String()] = facts
}

// Shutdown is part of the processor interface and has nothing to release.
func (s *baselineSpans) Shutdown(context.Context) error { return nil }

// ForceFlush is part of the processor interface; nothing here is buffered.
func (s *baselineSpans) ForceFlush(context.Context) error { return nil }

// lookup returns what the server said about the call carrying traceID.
func (s *baselineSpans) lookup(traceID string) (baselineDispatch, bool) {
	if traceID == "" {
		return baselineDispatch{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, seen := s.byTrace[traceID]
	return facts, seen
}

// baselineSpanFacts reads the attributes the record is made of off one span.
//
// The keys come from internal/mcpotel rather than being spelled again here:
// they are the server's own, and a second spelling would be a record that
// quietly emptied itself the day one was renamed. The status is spelled the
// way the OTLP protocol spells it, which is what the rebuilt harness records,
// so the two records agree on the one field the SDK and the wire name
// differently.
func baselineSpanFacts(span sdktrace.ReadOnlySpan) baselineDispatch {
	var facts baselineDispatch
	into := map[attribute.Key]*string{
		mcpotel.AttrGenAIToolName: &facts.tool,
		mcpotel.AttrActionID:      &facts.action,
		mcpotel.AttrDomain:        &facts.domain,
		mcpotel.AttrRefusalReason: &facts.refusalReason,
		mcpotel.AttrErrorType:     &facts.errorType,
	}
	for _, kv := range span.Attributes() {
		if target, wanted := into[kv.Key]; wanted {
			*target = kv.Value.AsString()
		}
	}
	if facts.empty() {
		return facts
	}
	facts.status = "STATUS_CODE_" + strings.ToUpper(span.Status().Code.String())
	return facts
}

// baselineRecorder is the process-wide state of the record: the spans every
// server produced, the shard writer, the sessions to describe at exit, and
// whether writing has failed.
type baselineRecorder struct {
	spans baselineSpans
	// writer is nil when recording is off, and a nil writer writes nothing.
	writer *e2ecalls.Writer
	// failed is set once a shard could not be written, which turns the run's
	// exit status into a failure: a baseline with holes in it is worse than
	// no baseline, and the operator who set the variable asked for one.
	failed atomic.Bool

	mu        sync.Mutex
	sessions  []*baselineSession
	observers map[int]func(*e2ecalls.Call)
	nextID    int
}

// baseline is the one recorder of this process.
var baseline = &baselineRecorder{spans: baselineSpans{byTrace: map[string]baselineDispatch{}}}

// baselineReporter is where a broken shard is reported. There is no test to
// fail from inside a middleware that many tests share, so it fails the run.
type baselineReporter struct{}

// Errorf logs the failure and marks the run.
func (baselineReporter) Errorf(format string, args ...any) {
	log.Printf("e2e baseline: "+format, args...)
	baseline.failed.Store(true)
}

// installBaselineTracing points the process's tracer at the in-memory span
// processor and installs the W3C propagator, so a traceparent stamped into a
// call's _meta becomes the parent of the server span that serves it.
//
// It runs once, before any server is built, because the telemetry middleware
// takes its tracer from the global provider at construction. The propagator
// matters as much as the provider: the SDK's default is a no-op that
// extracts nothing, and without it every server span would carry a fresh
// trace id nothing could join.
func installBaselineTracing() {
	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(&baseline.spans),
	))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	baseline.writer = e2ecalls.Open()
	if baseline.writer != nil {
		log.Printf("e2e baseline: recording calls into the directory %s names", e2ecalls.DirEnv)
	}
}

// telemetry is the server half: the production telemetry middleware, with
// the identifier the binary would build for the same catalog and surface, so
// the span names the route that ran. The identifier is the one the client
// half names the requested action with, so both halves read one catalog.
func (s *baselineSession) telemetry() mcp.Middleware {
	return mcpotel.Middleware(mcpotel.Options{Identifier: s.identifier, Surface: s.shape.surface})
}

// observe registers a function that sees every call line the recorder
// builds, and returns the function that unregisters it. It is how the
// recorder's own tests read what it wrote without opening the shard.
func (r *baselineRecorder) observe(fn func(*e2ecalls.Call)) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observers == nil {
		r.observers = map[int]func(*e2ecalls.Call){}
	}
	id := r.nextID
	r.nextID++
	r.observers[id] = fn
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.observers, id)
	}
}

// notify hands one finished call line to every observer.
func (r *baselineRecorder) notify(call *e2ecalls.Call) {
	r.mu.Lock()
	observers := slices.Collect(maps.Values(r.observers))
	r.mu.Unlock()
	for _, fn := range observers {
		fn(call)
	}
}

// addSession remembers a session so its line can be written at exit, once
// its dispatch-observed flag has settled.
func (r *baselineRecorder) addSession(s *baselineSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, s)
}

// write appends lines to the shard, or nothing when recording is off.
func (r *baselineRecorder) write(lines ...e2ecalls.Line) {
	r.writer.Write(baselineReporter{}, lines...)
}

// baselineFacts is what the run line says about the instance, probed once
// before the sessions start.
type baselineFacts struct {
	version    string
	enterprise bool
	licensed   bool
}

// probeBaselineFacts asks the instance what it is, without changing the
// client it is asked through.
//
// The edition flag is read off the captured /version response, since
// client-go's Version struct does not carry it. The license is read with
// DetectTier, which sets the client's tier as a side effect; the previous
// value is put back so this probe changes nothing about how the sessions are
// built, and TestMain's own tier detection runs afterwards exactly as it did.
func probeBaselineFacts(glClient *gitlabclient.Client) baselineFacts {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var facts baselineFacts
	captureCtx, capture := gitlabclient.WithResponseCapture(ctx)
	if version, _, err := glClient.GL().Version.GetVersion(gl.WithContext(captureCtx)); err == nil && version != nil {
		facts.version = version.Version
	}
	var reported struct {
		Enterprise *bool `json:"enterprise"`
	}
	if err := capture.Decode(&reported); err == nil && reported.Enterprise != nil {
		facts.enterprise = *reported.Enterprise
	}

	previous := glClient.Tier()
	facts.licensed = glClient.DetectTier(ctx).IsEnterprise()
	glClient.SetTier(previous)
	return facts
}

// The two requirements this suite can state, in the rebuilt harness's
// vocabulary: the enterprise-tagged binary asks for a license, the plain one
// runs anywhere.
const (
	baselineRequirementAny      = "any"
	baselineRequirementLicensed = "licensed"
)

// baselineRequirement is what the build asked of the runtime, copied onto
// every call line so a line can be read on its own. TestMain settles it
// before any session starts, and nothing writes it afterwards.
var baselineRequirement = baselineRequirementAny

// baselineRunLine records what this run found and what it asked for.
//
// The requirement is what the build asked for, which is what GITLAB_ENTERPRISE
// tells the suite. The tier is the one the sessions built their catalogs at,
// and it is confirmed only when the license said so: an EE image with no
// license serves a Free catalog and says so here.
func baselineRunLine(facts baselineFacts, enterprise bool, glClient *gitlabclient.Client) *e2ecalls.Run {
	editionToken := "community"
	if facts.enterprise {
		editionToken = "enterprise"
	}
	return &e2ecalls.Run{
		Package:       baselinePackage,
		Requirement:   baselineRequirement,
		Edition:       editionToken,
		Tier:          edition.TierForEnterprise(enterprise).String(),
		TierConfirmed: facts.licensed,
		GitLabVersion: facts.version,
		RunID:         e2eRunID,
		Commit:        os.Getenv("E2E_COMMIT"),
		Filter:        baselineRunFilter(),
		Fixtures: e2ecalls.FixtureProfile{
			Runner:         hasRunner(glClient),
			FixtureService: os.Getenv("E2E_FIXTURE_URL") != "",
			Bitbucket:      os.Getenv("BITBUCKET_SERVER_URL") != "",
			GHToken:        os.Getenv("GH_TOKEN") != "",
			Seeds:          baselineSeeds(os.Getenv("E2E_SEEDS")),
		},
		Status: e2ecalls.RunStarted,
	}
}

// baselineRunFilter returns the -run pattern this binary was given, so a
// partial run is not read as a claim about the tests it did not run.
func baselineRunFilter() string {
	if f := flag.Lookup("test.run"); f != nil {
		return f.Value.String()
	}
	return ""
}

// baselineSeeds splits the seed list the provisioning script publishes.
func baselineSeeds(configured string) []string {
	var seeds []string
	for seed := range strings.SplitSeq(configured, ",") {
		if trimmed := strings.TrimSpace(seed); trimmed != "" {
			seeds = append(seeds, trimmed)
		}
	}
	slices.Sort(seeds)
	return slices.Compact(seeds)
}

// finishBaseline writes the lines that belong to the run rather than to any
// one test, and turns a recording failure into the exit status.
//
// It runs after the last test, because that is when every session's
// dispatch-observed flag has settled and every ad hoc session has been
// started.
func finishBaseline(code int, run *e2ecalls.Run) int {
	if baseline.writer == nil {
		return code
	}
	lines := []e2ecalls.Line{run}
	baseline.mu.Lock()
	sessions := slices.Clone(baseline.sessions)
	baseline.mu.Unlock()
	for _, s := range sessions {
		lines = append(lines, s.line())
	}
	baseline.write(lines...)
	if baseline.failed.Load() && code == 0 {
		log.Printf("e2e baseline: the call record could not be written; failing the run so the missing shard is noticed")
		return 1
	}
	return code
}

// baselineSession is the client half of one session: its shape, the
// identifier that names what a call asks for, and what it listed when it
// started.
type baselineSession struct {
	shape      baselineShape
	identifier mcpotel.CallIdentifier
	served     e2ecalls.Session
	// observed is set once a span has been joined to a call of this
	// session, which is what says the dispatched actions are real.
	observed atomic.Bool
}

// newBaselineSession builds the client half for one server.
func newBaselineSession(shape baselineShape, catalog *actioncatalog.Catalog) *baselineSession {
	return &baselineSession{shape: shape, identifier: tools.NewCallIdentifier(catalog, shape.surface)}
}

// describeServed lists what the session serves, which is the denominator the
// coverage audit divides by.
//
// A listing that fails is logged and leaves that list empty rather than
// failing the run: the old suite's meta and dynamic servers register no
// resources or prompts, and what matters here is the tools.
func (s *baselineSession) describeServed(ctx context.Context, session *mcp.ClientSession) {
	s.served = e2ecalls.Session{
		Label:        s.shape.label,
		Surface:      s.shape.surface,
		Mode:         s.shape.mode,
		Capabilities: s.shape.capabilities,
		Transport:    baselineTransport,
	}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			log.Printf("e2e baseline: %s session: tools/list: %v", s.shape.label, err)
			break
		}
		s.served.Tools = append(s.served.Tools, tool.Name)
	}
	for resource, err := range session.Resources(ctx, nil) {
		if err != nil {
			log.Printf("e2e baseline: %s session: resources/list: %v", s.shape.label, err)
			break
		}
		s.served.Resources = append(s.served.Resources, resource.URI)
	}
	for template, err := range session.ResourceTemplates(ctx, nil) {
		if err != nil {
			log.Printf("e2e baseline: %s session: resources/templates/list: %v", s.shape.label, err)
			break
		}
		s.served.ResourceTemplates = append(s.served.ResourceTemplates, template.URITemplate)
	}
	for prompt, err := range session.Prompts(ctx, nil) {
		if err != nil {
			log.Printf("e2e baseline: %s session: prompts/list: %v", s.shape.label, err)
			break
		}
		s.served.Prompts = append(s.served.Prompts, prompt.Name)
	}
	slices.Sort(s.served.Tools)
	slices.Sort(s.served.Resources)
	slices.Sort(s.served.ResourceTemplates)
	slices.Sort(s.served.Prompts)
}

// line is the session line as it stands at exit.
func (s *baselineSession) line() *e2ecalls.Session {
	line := s.served
	line.DispatchObserved = s.observed.Load()
	return &line
}

// sending is the client middleware every request of the session leaves
// through. It records the four methods that are coverage and passes every
// other one through untouched.
func (s *baselineSession) sending() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if !baselineRecordsMethod(method) {
				return next(ctx, method, req)
			}
			origin := baselineOriginFor(ctx)
			traceID, traceParent := newBaselineTrace()
			if !stampBaselineTrace(req, traceParent) {
				traceID = ""
			}
			started := time.Now()
			result, err := next(ctx, method, req)
			elapsed := time.Since(started)

			line := s.callLine(origin, method, req, result, err, elapsed, traceID)
			lines := []e2ecalls.Line{line}
			if dispatch, seen := baseline.spans.lookup(traceID); seen {
				s.observed.Store(true)
				line.Dispatched = dispatch.action
				lines = append(lines, &e2ecalls.Dispatch{
					TraceID:       traceID,
					Tool:          dispatch.tool,
					Action:        dispatch.action,
					Domain:        dispatch.domain,
					RefusalReason: dispatch.refusalReason,
					ErrorType:     dispatch.errorType,
					Status:        dispatch.status,
				})
			}
			baseline.write(lines...)
			baseline.notify(line)
			return result, err
		}
	}
}

// baselineRecordsMethod reports whether a method is one the record holds.
func baselineRecordsMethod(method string) bool {
	switch method {
	case baselineMethodCallTool, baselineMethodReadResource, baselineMethodGetPrompt, baselineMethodComplete:
		return true
	default:
		return false
	}
}

// callLine builds the client half of one call line.
func (s *baselineSession) callLine(origin baselineOrigin, method string, req mcp.Request, result mcp.Result, err error, elapsed time.Duration, traceID string) *e2ecalls.Call {
	line := &e2ecalls.Call{
		Test:         origin.test,
		Purpose:      origin.purpose,
		Expectation:  e2ecalls.ExpectationAny,
		Session:      s.shape.label,
		Surface:      s.shape.surface,
		Mode:         s.shape.mode,
		Capabilities: s.shape.capabilities,
		Requirement:  baselineRequirement,
		Method:       method,
		Outcome:      baselineOutcome(method, result, err),
		DurationMS:   float64(elapsed.Microseconds()) / 1000,
		TraceID:      traceID,
	}
	switch params := req.GetParams().(type) {
	case *mcp.CallToolParams:
		if params == nil {
			break
		}
		line.Tool = params.Name
		raw, names := baselineArguments(params.Arguments)
		line.Arguments = names
		if identity, ok := s.identifier.Identify(params.Name, raw); ok {
			line.Action = identity.ActionID
		}
	case *mcp.ReadResourceParams:
		if params != nil {
			line.Target = params.URI
		}
	case *mcp.GetPromptParams:
		if params != nil {
			line.Target = params.Name
		}
	case *mcp.CompleteParams:
		if params != nil && params.Ref != nil {
			line.Target = params.Ref.Name
			if line.Target == "" {
				line.Target = params.Ref.URI
			}
		}
	}
	return line
}

// baselineArguments renders a call's arguments the two ways the record needs
// them: as the JSON the server will read the action out of, and as the sorted
// top-level names.
//
// The old suite passes typed inputs as often as maps, so the names come from
// the JSON encoding rather than from a type switch: a struct's json tags are
// what the server sees, and what a coverage report compares with a schema.
func baselineArguments(arguments any) (raw json.RawMessage, names []string) {
	if arguments == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, nil
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return encoded, nil
	}
	return encoded, slices.Sorted(maps.Keys(object))
}

// baselineOutcome classifies one answer in the record's vocabulary.
//
// The classes and the markers mirror the rebuilt harness's classify, on
// purpose and by hand: the baseline is compared with the new record credit
// by credit, and a refusal the new suite spells refused:not_found must be the
// same refusal here, or the comparison would report a loss where nothing was
// lost. The markers are the server's own wording, which is the only thing a
// client holds when it has to classify.
func baselineOutcome(method string, result mcp.Result, err error) string {
	if err != nil {
		if baselineTransportError(err) {
			return e2ecalls.OutcomeTransportError
		}
		return e2ecalls.OutcomeProtocolError
	}
	if method != baselineMethodCallTool {
		return e2ecalls.OutcomeOK
	}
	toolResult, _ := result.(*mcp.CallToolResult)
	if toolResult == nil {
		return e2ecalls.OutcomeProtocolError
	}
	text := baselineResultText(toolResult)
	if baselineIsPreview(toolResult, text) {
		return e2ecalls.OutcomePreview
	}
	if !toolResult.IsError {
		return e2ecalls.OutcomeOK
	}
	lowered := strings.ToLower(text)
	switch {
	case strings.Contains(lowered, "re-send with confirm=true"):
		return e2ecalls.RefusedOutcome(toolutil.RefusalNeedsConfirmation)
	case strings.Contains(lowered, "unknown action"):
		return e2ecalls.RefusedOutcome(toolutil.RefusalUnknownAction)
	case strings.Contains(lowered, "is required for this action"),
		strings.Contains(lowered, "missing required params"),
		strings.Contains(lowered, "'action' is required"):
		return e2ecalls.RefusedOutcome(toolutil.RefusalInvalidParams)
	case strings.Contains(lowered, "404"), strings.Contains(lowered, "not found"):
		return e2ecalls.RefusedOutcome("not_found")
	case strings.Contains(lowered, "403"), strings.Contains(lowered, "401"),
		strings.Contains(lowered, "forbidden"), strings.Contains(lowered, "unauthorized"):
		return e2ecalls.RefusedOutcome("forbidden")
	default:
		return e2ecalls.OutcomeToolError
	}
}

// baselineTransportError reports whether a call never reached a handler.
func baselineTransportError(err error) bool {
	message := err.Error()
	for _, transient := range []string{"EOF", "connection reset by peer", "broken pipe", "connection refused", "file already closed"} {
		if strings.Contains(message, transient) {
			return true
		}
	}
	return false
}

// baselineResultText returns the first text block of a result.
func baselineResultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			return text.Text
		}
	}
	return ""
}

// baselineIsPreview reports whether a result is a safe-mode preview, read the
// two ways the server writes one.
func baselineIsPreview(result *mcp.CallToolResult, text string) bool {
	if result.StructuredContent != nil {
		if encoded, err := json.Marshal(result.StructuredContent); err == nil {
			var preview toolutil.SafeModePreview
			if json.Unmarshal(encoded, &preview) == nil && preview.Status == "blocked" && preview.Mode == "safe" {
				return true
			}
		}
	}
	_, isPreview := toolutil.ParseSafeModePreview(text)
	return isPreview
}

// newBaselineTrace mints a trace and returns its id with the header value
// that carries it. crypto/rand, because the W3C format forbids an all-zero id
// and the ids must not collide across the processes of one run.
func newBaselineTrace() (traceID, traceParent string) {
	var ids [24]byte
	rand.Read(ids[:])
	traceID = hex.EncodeToString(ids[:16])
	return traceID, "00-" + traceID + "-" + hex.EncodeToString(ids[16:]) + "-01"
}

// stampBaselineTrace writes the trace context into a request's _meta,
// reporting whether it could. The meta map is cloned rather than mutated,
// since a caller may reuse one params value across calls.
func stampBaselineTrace(req mcp.Request, traceParent string) bool {
	params := req.GetParams()
	if params == nil {
		return false
	}
	// A typed nil pointer in a non-nil interface is what the SDK hands a
	// middleware for a method whose params may be missing.
	if value := reflect.ValueOf(params); value.Kind() == reflect.Pointer && value.IsNil() {
		return false
	}
	meta := maps.Clone(params.GetMeta())
	if meta == nil {
		meta = map[string]any{}
	}
	meta[baselineTraceParentKey] = traceParent
	params.SetMeta(meta)
	return true
}

// baselineOrigin is what only the caller knows about a call: which test made
// it and what for.
type baselineOrigin struct {
	test    string
	purpose string
}

// baselineOriginKey is the context key an origin travels under, for the one
// path that has no test frame on its stack: the ledger's cleanups.
type baselineOriginKey struct{}

// withBaselineOrigin returns a context carrying who is making the calls
// under it.
func withBaselineOrigin(ctx context.Context, test, purpose string) context.Context {
	return context.WithValue(ctx, baselineOriginKey{}, baselineOrigin{test: test, purpose: purpose})
}

// baselineOriginFor reads the origin off the context when a caller set one,
// and off the goroutine's stack otherwise.
func baselineOriginFor(ctx context.Context) baselineOrigin {
	if origin, carried := ctx.Value(baselineOriginKey{}).(baselineOrigin); carried {
		return origin
	}
	return baselineOriginFromStack()
}

// baselineStackDepth bounds the stack walk. The SDK, the retry helper and a
// few fixture helpers sit between a test and this middleware; the margin is
// for a goroutine a test started from a helper of a helper, not for an
// arbitrarily deep stack.
const baselineStackDepth = 128

// baselineCleanupFrame is the testing-package frame every cleanup runs under.
const baselineCleanupFrame = "testing.(*common).runCleanup"

// baselineCreatorHops bounds how far up the chain of creating goroutines the
// walk follows before giving up: a subtest under a subtest under a helper's
// subtest is three, and nothing in this suite nests deeper.
const baselineCreatorHops = 8

// suitePackagePath is this package's import path as the runtime spells it in
// a function name, read off one of this file's own functions so a module
// rename needs no edit here. The function it reads is one the walk never
// calls, or the initializer would refer to itself.
var suitePackagePath = func() string {
	name := runtime.FuncForPC(reflect.ValueOf(rewriteSubtestName).Pointer()).Name()
	return name[:strings.LastIndex(name, ".")]
}()

// baselineFrame is one stack frame as the walk reads it.
type baselineFrame struct {
	function string
	file     string
	line     int
}

// baselineOriginFromStack attributes a call to the test whose goroutine it
// runs on.
//
// A subtest runs in a closure the runtime names after the function that
// declared it, and the file and line of that frame say which t.Run literals
// enclose it, which is how the subtest's name is recovered from the source.
// When the declaring function is the Test function the goroutine's own frames
// name the test in full. When it is a helper, as the pipeline lifecycles are
// written, the goroutine holds the leaf of the name and nothing above it: the
// Test function is parked in t.Run on the goroutine that created this one.
// The walk then reads the whole process's stacks, follows the "created by"
// chain up to the goroutine holding a Test frame, and joins the parts. A
// cleanup is told by the testing-package frame it runs under.
func baselineOriginFromStack() baselineOrigin {
	counters := make([]uintptr, baselineStackDepth)
	depth := runtime.Callers(2, counters)
	iterator := runtime.CallersFrames(counters[:depth])
	var frames []baselineFrame
	for {
		frame, more := iterator.Next()
		frames = append(frames, baselineFrame{function: frame.Function, file: frame.File, line: frame.Line})
		if !more {
			break
		}
	}

	origin := baselineOrigin{purpose: e2ecalls.PurposeTest}
	if baselineIsCleanup(frames) {
		origin.purpose = e2ecalls.PurposeCleanup
	}
	parts, rooted := baselineNameParts(frames)
	if !rooted {
		parts, rooted = baselineNamePartsFromCreators(parts)
	}
	if rooted {
		origin.test = joinNameParts(parts)
	}
	return origin
}

// baselineIsCleanup reports whether a goroutine is running a test's cleanups.
func baselineIsCleanup(frames []baselineFrame) bool {
	for _, frame := range frames {
		if frame.function == baselineCleanupFrame {
			return true
		}
	}
	return false
}

// baselineNamePart is one part of a test name: the Test function, or a t.Run
// literal identified by where it is declared, so that two goroutines whose
// frames sit inside the same literal contribute it once.
type baselineNamePart struct {
	file  string
	start int
	name  string
}

// joinNameParts spells the parts as the testing package spells the name.
func joinNameParts(parts []baselineNamePart) string {
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		names = append(names, part.name)
	}
	return strings.Join(names, "/")
}

// baselineNameParts reads the parts of a test name off one goroutine's
// frames, innermost first, and reports whether the goroutine is rooted in
// the Test function.
//
// Each frame of this package contributes the t.Run literals enclosing its
// line, in the function that declared it, and every frame is read: a helper
// closure declared at the top of a test and called inside a subtest is a
// frame of the Test function that sits in no literal, while the subtest's
// own frame, further out on the same stack, sits in one. What decides
// whether the goroutine is rooted is its outermost frame of this package,
// the one the testing package or the go statement started it in: the Test
// function itself or a closure declared in it says the whole name is here,
// and a helper's closure says the rest of the name is on the goroutine that
// created this one. A Test-named closure called from deeper in the stack
// decides nothing, since a helper's subtest can call one.
func baselineNameParts(frames []baselineFrame) (parts []baselineNamePart, rooted bool) {
	outermost := ""
	for _, frame := range frames {
		base, inPackage := baselineFrameFunction(frame.function)
		if !inPackage {
			continue
		}
		parts = mergeNameParts(baselineSubtests.literalsAt(frame.file, frame.line, base), parts)
		outermost = base
	}
	if outermost == "" || outermost == "TestMain" || !isTestFunctionName(outermost) {
		return parts, false
	}
	return append([]baselineNamePart{{name: outermost}}, parts...), true
}

// mergeNameParts puts outer parts in front of inner ones, dropping an inner
// part the outer list already holds.
//
// The overlap is real and not a defect of either side: a goroutine parked in
// t.Run sits on the line of the literal its subtest runs, and a goroutine a
// subtest started with go sits inside the same literal as the subtest's own
// frame. Both would spell that literal twice if the lists were only joined.
func mergeNameParts(outer, inner []baselineNamePart) []baselineNamePart {
	merged := slices.Clone(outer)
	for _, part := range inner {
		if !slices.Contains(merged, part) {
			merged = append(merged, part)
		}
	}
	return merged
}

// baselineNamePartsFromCreators reads every goroutine of the process and
// follows the current one's creators until a goroutine holds the Test
// function, merging the parts of each on the way, starting from what the
// current goroutine's frames already said.
//
// This is the slow path, taken only when the calling goroutine names no Test
// function of its own, because it stops the world to read every stack. It is
// what attributes a subtest a helper function declared: the goroutine holds
// the leaf of the name, and the Test function is parked in t.Run on the one
// that created it.
func baselineNamePartsFromCreators(own []baselineNamePart) ([]baselineNamePart, bool) {
	stacks, current := parseGoroutineDump(baselineGoroutineDump())
	chain := own
	stack, known := stacks[current]
	for hops := 0; known && hops < baselineCreatorHops; hops++ {
		stack, known = stacks[stack.createdBy]
		if !known {
			break
		}
		parts, rooted := baselineNameParts(stack.frames)
		chain = mergeNameParts(parts, chain)
		if rooted {
			return chain, true
		}
	}
	return nil, false
}

// baselineGoroutineDump returns the stacks of every goroutine, grown until
// the whole dump fits.
func baselineGoroutineDump() []byte {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

// baselineGoroutine is one goroutine of a dump: its frames, innermost first,
// and the goroutine that created it, zero for the first one.
type baselineGoroutine struct {
	frames    []baselineFrame
	createdBy int
}

// parseGoroutineDump reads a runtime.Stack dump of every goroutine into
// stacks keyed by goroutine id, and returns the id of the goroutine that
// asked for it, which the runtime prints first.
//
// A block is a "goroutine N [state]:" header, then frames as a function line
// followed by a tab-indented location line, then possibly "created by F in
// goroutine M" with its own location. Only the shapes the walk reads are
// parsed; anything else in a block is skipped.
func parseGoroutineDump(dump []byte) (stacks map[int]baselineGoroutine, current int) {
	stacks = map[int]baselineGoroutine{}
	id := 0
	var stack baselineGoroutine
	flush := func() {
		if id != 0 {
			stacks[id] = stack
		}
		id, stack = 0, baselineGoroutine{}
	}
	lines := strings.Split(string(dump), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "goroutine "):
			flush()
			if _, err := fmt.Sscanf(line, "goroutine %d ", &id); err != nil {
				id = 0
			}
			if current == 0 {
				current = id
			}
		case strings.HasPrefix(line, "created by "):
			stack.createdBy = createdByGoroutine(line)
			i++
		case line == "":
			flush()
		case strings.HasPrefix(line, "\t"):
			// A location line with no function line before it, which the
			// walk cannot use.
		default:
			frame := baselineFrame{function: dumpFunctionName(line)}
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\t") {
				frame.file, frame.line = dumpLocation(lines[i+1])
				i++
			}
			stack.frames = append(stack.frames, frame)
		}
	}
	flush()
	return stacks, current
}

// dumpFunctionName strips the argument list off a dump's function line.
func dumpFunctionName(line string) string {
	if open := strings.LastIndex(line, "("); open >= 0 && strings.HasSuffix(line, ")") {
		return line[:open]
	}
	return line
}

// dumpLocation reads the file and line off a dump's location line.
func dumpLocation(line string) (string, int) {
	location := strings.TrimSpace(line)
	if space := strings.Index(location, " "); space >= 0 {
		location = location[:space]
	}
	colon := strings.LastIndex(location, ":")
	if colon < 0 {
		return location, 0
	}
	number, err := strconv.Atoi(location[colon+1:])
	if err != nil {
		return location[:colon], 0
	}
	return location[:colon], number
}

// createdByGoroutine reads the creating goroutine off a "created by" line,
// or zero when the line names none.
func createdByGoroutine(line string) int {
	marker := " in goroutine "
	at := strings.LastIndex(line, marker)
	if at < 0 {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimSpace(line[at+len(marker):]))
	if err != nil {
		return 0
	}
	return id
}

// baselineFrameFunction names the function of this package that declared a
// frame's code, as the source index keys it: the function itself for a
// plain function, "(*T).M" or "T.M" for a method, with the closure, defer,
// go and range suffixes the runtime appends stripped off. It reports false
// for a frame of any other package.
func baselineFrameFunction(function string) (string, bool) {
	local, inPackage := strings.CutPrefix(function, suitePackagePath+".")
	if !inPackage {
		return "", false
	}
	return declaredFunctionName(local), true
}

// declaredFunctionName strips what the runtime appends to a declared
// function's name for the code nested in it: ".funcN" and ".N" for closures,
// ".deferwrapN" and ".gowrapN" for the wrappers of defer and go statements,
// "-rangeN" for range-over-func bodies, and "[...]" for an instantiation.
func declaredFunctionName(local string) string {
	for {
		trimmed := local
		if at := strings.Index(trimmed, "["); at >= 0 {
			trimmed = trimmed[:at] + trimmed[strings.LastIndex(trimmed, "]")+1:]
		}
		if at := strings.LastIndexAny(trimmed, ".-"); at >= 0 && isRuntimeSuffix(trimmed[at+1:]) {
			trimmed = trimmed[:at]
		}
		if trimmed == local {
			return local
		}
		local = trimmed
	}
}

// isRuntimeSuffix reports whether one dot- or dash-separated part of a
// function name is a suffix the runtime appended rather than an identifier
// the source declared.
func isRuntimeSuffix(part string) bool {
	for _, prefix := range []string{"func", "deferwrap", "gowrap", "range"} {
		if digits, found := strings.CutPrefix(part, prefix); found && digits != "" && isDigits(digits) {
			return true
		}
	}
	return isDigits(part)
}

// isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isTestFunctionName applies the testing package's rule: Test followed by a
// character that is not a lowercase letter.
func isTestFunctionName(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) == len("Test") {
		return true
	}
	rest := []rune(name[len("Test"):])
	return !unicode.IsLower(rest[0])
}

// baselineRunLiteral is one t.Run call with a literal name, as its closure's
// line range in the file and the subtest name the runtime gives it.
type baselineRunLiteral struct {
	start int
	end   int
	name  string
}

// baselineSubtestIndex maps a frame's file and line to the t.Run literals
// whose closures enclose it, parsed once per file from the source, for every
// function of the file.
//
// The source is parsed rather than the closure numbered: the runtime names a
// subtest's closure TestX.funcN, and turning N back into a name would mean
// reproducing the compiler's numbering of every closure in the function. A
// line is inside a closure or it is not, whatever the compiler called it.
type baselineSubtestIndex struct {
	mu    sync.Mutex
	files map[string]map[string][]baselineRunLiteral
}

// baselineSubtests is the one index of this process.
var baselineSubtests = &baselineSubtestIndex{files: map[string]map[string][]baselineRunLiteral{}}

// literalsAt returns the t.Run literals of function in file whose closures
// enclose line, outermost first, as name parts identified by the file and
// the line the literal starts on.
func (i *baselineSubtestIndex) literalsAt(file string, line int, function string) []baselineNamePart {
	var parts []baselineNamePart
	for _, literal := range i.entries(file)[function] {
		if literal.start <= line && line <= literal.end {
			parts = append(parts, baselineNamePart{file: file, start: literal.start, name: literal.name})
		}
	}
	return parts
}

// entries returns the parsed literals of one file, parsing it on first use.
//
// The frame's path is tried as given and then by its base name in the
// working directory, which is the package directory under go test and is
// where the file is when a build trimmed its paths. A file that cannot be
// parsed indexes as empty, so its calls resolve to their Test function and
// never to nothing.
func (i *baselineSubtestIndex) entries(file string) map[string][]baselineRunLiteral {
	i.mu.Lock()
	defer i.mu.Unlock()
	if parsed, seen := i.files[file]; seen {
		return parsed
	}
	parsed, err := parseBaselineSubtests(baselineSourcePath(file))
	if err != nil {
		log.Printf("e2e baseline: %s: subtests resolve to their Test function only: %v", filepath.Base(file), err)
		parsed = map[string][]baselineRunLiteral{}
	}
	i.files[file] = parsed
	return parsed
}

// baselineSourcePath finds a frame's source file on disk.
func baselineSourcePath(file string) string {
	if _, err := os.Stat(file); err == nil {
		return file
	}
	return filepath.Base(file)
}

// parseBaselineSubtests reads every function of one file and, for each, the
// t.Run calls whose name is a string literal and whose body is a function
// literal, in source order, keyed by the function's name as the runtime
// spells it.
//
// Every function and not only the Test functions, because a helper declares
// subtests too: the pipeline lifecycles are t.Run calls in helpers, and a
// call made inside one has the helper's closure on its stack rather than the
// test's.
func parseBaselineSubtests(path string) (map[string][]baselineRunLiteral, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	byFunction := map[string][]baselineRunLiteral{}
	for _, decl := range parsed.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		byFunction[runtimeFunctionName(fn)] = runLiteralsOf(fset, fn.Body)
	}
	return byFunction, nil
}

// runtimeFunctionName spells a declared function the way the walk keys a
// frame's function: the name itself, or "(*T).M" and "T.M" for a method,
// with a generic receiver's type arguments left out, since the walk strips
// the runtime's "[...]" off the frame's name too.
func runtimeFunctionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := fn.Recv.List[0].Type
	pointer := false
	if star, isStar := receiver.(*ast.StarExpr); isStar {
		pointer = true
		receiver = star.X
	}
	var base string
	switch typed := receiver.(type) {
	case *ast.Ident:
		base = typed.Name
	case *ast.IndexExpr:
		if ident, isIdent := typed.X.(*ast.Ident); isIdent {
			base = ident.Name
		}
	case *ast.IndexListExpr:
		if ident, isIdent := typed.X.(*ast.Ident); isIdent {
			base = ident.Name
		}
	}
	if base == "" {
		return fn.Name.Name
	}
	if pointer {
		return "(*" + base + ")." + fn.Name.Name
	}
	return base + "." + fn.Name.Name
}

// runLiteralsOf collects the t.Run literals of one function body, with the
// names the testing package will give them.
func runLiteralsOf(fset *token.FileSet, body *ast.BlockStmt) []baselineRunLiteral {
	var literals []baselineRunLiteral
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		name, lit, isRun := runLiteralCall(call)
		if !isRun {
			return true
		}
		literals = append(literals, baselineRunLiteral{
			start: fset.Position(lit.Pos()).Line,
			end:   fset.Position(lit.End()).Line,
			name:  name,
		})
		return true
	})
	return uniqueRunNames(literals)
}

// runLiteralCall reads a t.Run("name", func(t *testing.T) {...}) call, and
// reports false for any other call.
func runLiteralCall(call *ast.CallExpr) (name string, lit *ast.FuncLit, ok bool) {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Run" || len(call.Args) != 2 {
		return "", nil, false
	}
	basic, isLiteral := call.Args[0].(*ast.BasicLit)
	if !isLiteral || basic.Kind != token.STRING {
		return "", nil, false
	}
	lit, isFunc := call.Args[1].(*ast.FuncLit)
	if !isFunc {
		return "", nil, false
	}
	unquoted, err := strconv.Unquote(basic.Value)
	if err != nil {
		return "", nil, false
	}
	return rewriteSubtestName(unquoted), lit, true
}

// uniqueRunNames gives repeated names under one parent the #NN suffix the
// testing package gives them, in source order, which is execution order for
// literals written one after another.
func uniqueRunNames(literals []baselineRunLiteral) []baselineRunLiteral {
	seen := map[string]int{}
	for i := range literals {
		key := strconv.Itoa(enclosingLiteral(literals, i)) + "\x00" + literals[i].name
		if count := seen[key]; count > 0 {
			literals[i].name = fmt.Sprintf("%s#%02d", literals[i].name, count)
		}
		seen[key]++
	}
	return literals
}

// enclosingLiteral returns the index of the innermost other literal whose
// closure encloses literal i, or -1 when it sits directly in the Test
// function.
func enclosingLiteral(literals []baselineRunLiteral, i int) int {
	parent := -1
	for j, other := range literals {
		if j == i || other.start > literals[i].start || other.end < literals[i].end {
			continue
		}
		if parent < 0 || other.start >= literals[parent].start {
			parent = j
		}
	}
	return parent
}

// rewriteSubtestName spells a name the way the testing package does: a space
// becomes an underscore and an unprintable rune its quoted form.
func rewriteSubtestName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsSpace(r):
			b.WriteByte('_')
		case !strconv.IsPrint(r):
			quoted := strconv.QuoteRune(r)
			b.WriteString(quoted[1 : len(quoted)-1])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
