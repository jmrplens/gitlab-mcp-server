//go:build e2e

// otlp.go is the half of the record that comes from the server rather than
// from the client.
//
// A client knows what it asked for. It does not know what ran, and the two
// differ on purpose: a meta or dynamic call naming an environment by name
// dispatches protected_get rather than get, and an individual safe-mode call
// answers with a preview of a mutation nobody made. Crediting the requested
// action would credit an action that never ran, which is the defect the suite
// this replaces shipped for years.
//
// So every child runs with telemetry on, pointed at a receiver the harness
// serves on loopback, and the harness stamps a W3C traceparent into each call
// it makes. The server reads that traceparent out of _meta, which it already
// does for any instrumented caller, so its span carries the harness's trace id
// and the route the dispatcher actually chose. Joining the two is one map
// lookup on a trace id.
//
// Only the traces are read. Metrics and logs are accepted and dropped, because
// refusing them would make the child's exporter retry for the whole run.

package harness

import (
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
)

// maxExportBytes bounds one OTLP payload the receiver reads.
//
// A child of this suite exports its own spans and nothing else, so a payload
// this large is a bug rather than a busy run; the bound is here so that a
// runaway exporter cannot take the test process's memory with it.
const maxExportBytes = 16 << 20

// receiverReadHeaderTimeout bounds how long the receiver waits for a child's
// request headers. It exists because an HTTP server with no timeout is a
// finding in every scanner, and because a child that hangs mid-header should
// not hold a connection for the life of the run.
const receiverReadHeaderTimeout = 30 * time.Second

// dispatchRecord is what the server's own span said about one call.
//
// Five attributes and a status, which is the whole of what the coverage audit
// asks of the server side: what ran, in which domain, under which tool, and
// whether the server declined it and why.
type dispatchRecord struct {
	// tool is gen_ai.tool.name: the tool the client named.
	tool string
	// action is gitlab_mcp.action: the route the dispatcher chose, after every
	// alias rewrite.
	action string
	// domain is gitlab_mcp.domain.
	domain string
	// refusalReason is gitlab_mcp.refusal_reason, set when the server declined
	// to run what it was asked for.
	refusalReason string
	// errorType is error.type.
	errorType string
	// status is the span status code, as the protocol spells it.
	status string
}

// carriesFacts reports whether a span carried any of the five dispatch
// attributes.
//
// It is one of the two ways [isServerSpan] recognizes the server's own span,
// and it is no longer the arrival test. It used to be both, and the span of a
// successful resource read, prompt, completion or subscribe carries none of
// these: such a call is spanned with mcp.method.name and the attributes of
// what it addressed, and only a call of any method that failed on the
// server's side carries error.type: a JSON-RPC code the server counts as its
// own failure, or an error carrying no code, which reads _OTHER. So the span of a
// call that succeeded, or that failed through the caller's fault, was dropped
// as saying nothing, its trace never arrived, and every flush that held one
// waited the whole budget for a span that had come and gone.
func (d dispatchRecord) carriesFacts() bool {
	return d.tool != "" || d.action != "" || d.domain != "" || d.refusalReason != "" || d.errorType != ""
}

// namesCall reports whether the server's span named the tool or the action it
// ran, which is what a dispatch line is written for.
//
// A line naming an action is what both readers of the record join. A line
// naming only a tool is kept too, as the server's own word that a call reached
// the find tool, which dispatches no action, even though neither reader joins
// it today: both skip a line naming no action, so the coverage command's
// dispatch_lines diagnostic counts these lines and its join passes over them.
// A line naming neither is dropped because it identifies nothing. That is the
// span of a method other than tools/call, such as a resource read or a
// completion, whose record says at most that the call failed, and the span of
// a tools/call that named no tool, which the server refused before it had
// anything to name.
func (d dispatchRecord) namesCall() bool {
	return d.tool != "" || d.action != ""
}

// traceSpans is everything the receiver kept about one trace: whether the
// server's own span arrived and what it said, and how many GitLab requests the
// handler made under it.
//
// The three are held in one entry rather than in several maps so that a reader
// can never take a count from one moment and a dispatch from another, and the
// count is a field of its own rather than part of the merged record because it
// arrives from different spans and answers a different question.
type traceSpans struct {
	// served is whether the MCP server span of this trace has arrived, whatever
	// its method. It is the arrival test, and it is a field of its own because
	// what the span said can be nothing at all: a resource read that succeeded
	// carries none of the dispatch facts.
	served bool
	// dispatch is what the MCP server span said.
	dispatch dispatchRecord
	// requests counts the GitLab client spans of this trace.
	requests int
}

// merge fills this record's empty fields from another, first non-empty
// winning, so several spans of one trace add up rather than replace.
func (d dispatchRecord) merge(other dispatchRecord) dispatchRecord {
	fields := []struct {
		into *string
		from string
	}{
		{&d.tool, other.tool},
		{&d.action, other.action},
		{&d.domain, other.domain},
		{&d.refusalReason, other.refusalReason},
		{&d.errorType, other.errorType},
		{&d.status, other.status},
	}
	for _, field := range fields {
		if *field.into == "" {
			*field.into = field.from
		}
	}
	return d
}

// spanReceiver is the OTLP/HTTP endpoint every child exports to.
//
// It keeps only the traces the harness issued. A trace id it never stamped
// belongs to work no test asked for, such as the server's own startup, and
// keeping those would grow a map nothing ever reads.
type spanReceiver struct {
	// url is the base endpoint a child is pointed at.
	url string

	mu sync.Mutex
	// issued is every trace the harness stamped into a call, with the session
	// the call went to, which the server's span marks dispatch-observed the
	// moment it lands. The session is nil for a trace issued by a test that
	// has no session to name.
	issued map[string]*sessionConn
	seen   map[string]traceSpans

	// observed is set the first time any server span arrives, so a run whose
	// telemetry never worked can stop waiting for spans that are not coming.
	// Any method's span sets it: a run whose first calls were all successful
	// resource reads has working telemetry, and a flag that waited for a span
	// carrying the dispatch facts, which only a tools/call span or the span of
	// a call that failed on the server's side carries, gave up on it and told
	// the log that nothing had arrived.
	observed atomic.Bool

	server   *http.Server
	listener net.Listener
}

// The receiver is started once per test process, by whichever session starts
// first, because its endpoint has to be in the child's environment before that
// child is launched.
var (
	receiverOnce sync.Once
	receiver     *spanReceiver
	errReceiver  error
	// startedSpans is the receiver once it is running, readable from any
	// goroutine without starting one. The recorder needs exactly that: a test
	// that made no call must not bind a listener on its way out, and reading
	// the variable above while another test is starting a session would be a
	// race.
	startedSpans atomic.Pointer[spanReceiver]
)

// spans returns this process's receiver, starting it on the first call.
func spans() (*spanReceiver, error) {
	receiverOnce.Do(func() {
		receiver, errReceiver = startSpanReceiver()
		if errReceiver == nil {
			startedSpans.Store(receiver)
		}
	})
	return receiver, errReceiver
}

// receiverIfStarted returns the running receiver, or nil when none was ever
// started. It never starts one.
func receiverIfStarted() *spanReceiver { return startedSpans.Load() }

// startSpanReceiver binds a loopback listener and serves OTLP over it.
//
// Loopback because the children are local processes and nothing else has any
// business reading this suite's spans; port zero because several test binaries
// of one run are alive at once and a fixed port would make the second refuse
// to start.
func startSpanReceiver() (*spanReceiver, error) {
	var config net.ListenConfig
	listener, err := config.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("binding the loopback OTLP receiver: %w", err)
	}

	received := &spanReceiver{
		url:      "http://" + listener.Addr().String(),
		issued:   map[string]*sessionConn{},
		seen:     map[string]traceSpans{},
		listener: listener,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", received.receiveTraces)
	// Metrics and logs are exported to the same endpoint and are of no use
	// here. They are acknowledged rather than refused: a 404 makes the child's
	// exporter retry the same payload for the rest of the run, which costs the
	// server under test a background goroutine and the log a warning per batch.
	mux.HandleFunc("/", acknowledgeExport)

	received.server = &http.Server{Handler: mux, ReadHeaderTimeout: receiverReadHeaderTimeout}
	go func() {
		if serveErr := received.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("e2e: the OTLP receiver stopped: %v", serveErr)
		}
	}()
	return received, nil
}

// closeSpanReceiver stops the receiver, if one was started.
//
// Main calls it after the last record is written. Nothing depends on it in a
// run that ends normally, since the process is about to exit; it is here so
// that the listener is released in the order the rest of the shutdown happens
// in rather than by the operating system.
func closeSpanReceiver() {
	if receiver == nil || receiver.server == nil {
		return
	}
	_ = receiver.server.Close()
}

// issue records a trace id the harness stamped into a call on one session, so
// the spans of that call are the ones the receiver keeps and the session is
// the one its server span marks.
func (r *spanReceiver) issue(traceID string, conn *sessionConn) {
	if traceID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issued[traceID] = conn
}

// lookup returns what the receiver kept about one trace, and whether the
// server's own span has arrived.
//
// Arrival is the server span, of whatever method, and nothing else. An entry
// made by a GitLab request span alone says the handler called out and not
// that the server has reported on the call, and a caller told it had would
// flush a tools/call with no dispatched action and no assertion about what ran.
func (r *spanReceiver) lookup(traceID string) (traceSpans, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.seen[traceID]
	return kept, kept.served
}

// all returns everything the receiver has kept, keyed by trace id.
func (r *spanReceiver) all() map[string]traceSpans {
	r.mu.Lock()
	defer r.mu.Unlock()

	return maps.Clone(r.seen)
}

// receiveTraces decodes one export and keeps what it says about the calls this
// harness made.
func (r *spanReceiver) receiveTraces(w http.ResponseWriter, req *http.Request) {
	body, err := readExportBody(req)
	if err != nil {
		http.Error(w, "the export could not be read", http.StatusBadRequest)
		return
	}

	var export coltracepb.ExportTraceServiceRequest
	if err = proto.Unmarshal(body, &export); err != nil {
		http.Error(w, "the export is not an OTLP trace request", http.StatusBadRequest)
		return
	}

	r.absorb(&export)
	acknowledgeExport(w, req)
}

// absorb folds every span of an export into the traces the harness issued.
func (r *spanReceiver) absorb(export *coltracepb.ExportTraceServiceRequest) {
	for _, resourceSpans := range export.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				r.absorbSpan(span)
			}
		}
	}
}

// absorbSpan keeps one span, if the harness issued its trace and the span is
// one of the two kinds this record is made of.
//
// The two are kept apart. The MCP server span says the server reported on the
// call and, for a tool call, what ran; a GitLab client span says the handler
// called out, and is counted rather than merged. Counting it is the whole of
// the per-action observation this record exists to make possible, and keeping
// it out of the merge is what stops a failed GitLab call from writing its own
// error.type over the server's.
//
// The server span's arrival is also where the session the call went to is
// marked dispatch-observed, at the moment the span lands rather than at the
// flush of the test that made the call: a span that came after that flush
// still says the session's telemetry works, and its dispatch line is joined
// to the call all the same.
func (r *spanReceiver) absorbSpan(span *tracepb.Span) {
	traceID := hex.EncodeToString(span.GetTraceId())
	if traceID == "" {
		return
	}
	request := isGitLabRequest(span)
	facts := spanFacts(span)
	if !request && !isServerSpan(span, facts) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	conn, issued := r.issued[traceID]
	if !issued {
		return
	}
	kept := r.seen[traceID]
	if request {
		kept.requests++
		r.seen[traceID] = kept
		return
	}
	kept.served = true
	kept.dispatch = kept.dispatch.merge(facts)
	r.seen[traceID] = kept
	r.observed.Store(true)
	if conn != nil {
		conn.dispatchObserved.Store(true)
	}
}

// isServerSpan reports whether a span is the MCP server's own account of one
// call.
//
// The server opens one for every request it handles, of every method, and
// every one carries mcp.method.name: the convention marks it Required, and
// the middleware writes it before it knows anything else about the request.
// That attribute is what makes a resource read's span recognizable at all,
// since it carries none of the dispatch facts. A span carrying those facts is
// taken too, which is what a span built by hand in this package's tests is,
// and what keeps a tool call's span arriving should the method attribute ever
// go missing.
//
// A client span is never the server span, whatever it carries, and the rule
// is the span kind for a reason the GitLab case makes visible only by
// accident. The outbound transport is not the server's only producer of client
// spans: [mcpotel.SendingMiddleware] opens one per server-initiated request
// (an elicitation, a sampling call, a progress notification), and it carries
// mcp.method.name on the trace of the tools/call that provoked it, plus
// error.type when it failed. Taken for the server span, it would mark the trace
// arrived before the tools/call's own span landed, the flush would write that
// call with no dispatched action, and the assertion about what ran would be
// skipped. Asking for SPAN_KIND_SERVER outright is the cleaner spelling and is
// not what this does, because a span built by hand leaves Kind unset.
func isServerSpan(span *tracepb.Span, facts dispatchRecord) bool {
	if span.GetKind() == tracepb.Span_SPAN_KIND_CLIENT {
		return false
	}
	return facts.carriesFacts() || hasAttribute(span, mcpotel.AttrMCPMethodName)
}

// hasAttribute reports whether a span carries an attribute under this key,
// whatever its value.
func hasAttribute(span *tracepb.Span, key attribute.Key) bool {
	for _, kv := range span.GetAttributes() {
		if kv.GetKey() == string(key) {
			return true
		}
	}
	return false
}

// isGitLabRequest reports whether a span is one outbound GitLab call.
//
// Both halves of the test are needed. The attribute is what
// internal/mcpotel.NewTransport puts on every round trip, and it is also on
// the span the HTTP server middleware opens for an inbound POST, which shares
// a trace with nothing here only because the harness stamps its traceparent
// into _meta rather than into a header. The span kind is what separates them
// for good, and it is the kind the round tripper asks for by name.
func isGitLabRequest(span *tracepb.Span) bool {
	return span.GetKind() == tracepb.Span_SPAN_KIND_CLIENT && hasAttribute(span, mcpotel.AttrHTTPRequestMethod)
}

// spanFacts reads the attributes the record is made of off one span, and its
// status.
//
// The attribute keys come from internal/mcpotel rather than being spelled
// again here: they are the server's own, and a second spelling of them would
// be a record that quietly emptied itself the day one was renamed.
//
// The status is read off every server span, whatever else it carries, so a
// span that arrived always says how the server classified the call. It is not
// the refusal: a tools/call with an empty name gives the middleware no tool to
// record, the server refuses it with -32602, and since the convention counts
// that code as the caller's fault rather than the server's failure the span's
// status stays STATUS_CODE_UNSET, the value a success carries. The code itself
// is on rpc.response.status_code, which this record does not read.
func spanFacts(span *tracepb.Span) dispatchRecord {
	facts := dispatchRecord{status: span.GetStatus().GetCode().String()}
	into := map[string]*string{
		string(mcpotel.AttrGenAIToolName): &facts.tool,
		string(mcpotel.AttrActionID):      &facts.action,
		string(mcpotel.AttrDomain):        &facts.domain,
		string(mcpotel.AttrRefusalReason): &facts.refusalReason,
		string(mcpotel.AttrErrorType):     &facts.errorType,
	}
	for _, kv := range span.GetAttributes() {
		if target, wanted := into[kv.GetKey()]; wanted {
			*target = kv.GetValue().GetStringValue()
		}
	}
	return facts
}

// readExportBody reads one request body, decompressing it when the exporter
// compressed it.
//
// The Go OTLP/HTTP exporter sends uncompressed protobuf unless it is told
// otherwise, and this suite does not tell it otherwise. Handling gzip anyway
// costs eight lines and removes a way for the whole record to go silently
// empty if that default ever changes.
func readExportBody(req *http.Request) ([]byte, error) {
	reader := io.LimitReader(req.Body, maxExportBytes)
	if req.Header.Get("Content-Encoding") == "gzip" {
		decompressed, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("decompressing the export: %w", err)
		}
		defer decompressed.Close()
		reader = io.LimitReader(decompressed, maxExportBytes)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading the export: %w", err)
	}
	return body, nil
}

// acknowledgeExport answers one export the way a collector does.
//
// An empty ExportServiceResponse is zero bytes of protobuf, which is what a
// successful export looks like on the wire. Answering anything else makes the
// exporter treat it as a partial success and log about it.
func acknowledgeExport(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

// otlpEndpointVariable is the variable that points a child's exporters at
// this process's receiver, and so the one that says a child's spans can
// arrive here at all.
const otlpEndpointVariable = "OTEL_EXPORTER_OTLP_ENDPOINT"

// telemetryVariables returns the environment that points one child at this
// process's receiver.
//
// It is called once per session, from the one place a child's environment is
// built, and it is what makes the dispatch half of the record exist at all. A
// receiver that cannot start is logged and the child runs without telemetry:
// the suite is still a suite, the sessions it starts are marked
// dispatch-unobserved, and every call of theirs is recorded as a claim about
// what was asked for rather than about what ran.
func telemetryVariables() map[string]string {
	received, err := spans()
	if err != nil {
		log.Printf("e2e: no OTLP receiver, so no session can say what it dispatched: %v", err)
		return nil
	}
	return map[string]string{
		"GITLAB_MCP_TELEMETRY":        "true",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
		otlpEndpointVariable:          received.url,
		// Milliseconds, as an integer: the specification defines every OTEL_
		// duration that way, and "100ms" parses as nothing and silently keeps
		// the five-second default, which is longer than a test's whole flush.
		"OTEL_BSP_SCHEDULE_DELAY": "100",
	}
}
