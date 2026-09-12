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

// carriesFacts reports whether a span said anything this record is for.
//
// A trace holds more than the MCP server span: every GitLab request the
// handler made is a child span of it, and none of those carries any of these
// attributes. Merging one would overwrite the facts with emptiness, so a span
// that carries none of them is dropped instead.
func (d dispatchRecord) carriesFacts() bool {
	return d.tool != "" || d.action != "" || d.domain != "" || d.refusalReason != "" || d.errorType != ""
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

	mu     sync.Mutex
	issued map[string]struct{}
	seen   map[string]dispatchRecord

	// observed is set the first time any dispatch arrives, so a run whose
	// telemetry never worked can stop waiting for spans that are not coming.
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
		issued:   map[string]struct{}{},
		seen:     map[string]dispatchRecord{},
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

// issue records a trace id the harness stamped into a call, so the spans of
// that call are the ones the receiver keeps.
func (r *spanReceiver) issue(traceID string) {
	if traceID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issued[traceID] = struct{}{}
}

// lookup returns what the server said about one trace, and whether any span of
// it has arrived.
func (r *spanReceiver) lookup(traceID string) (dispatchRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, found := r.seen[traceID]
	return record, found
}

// all returns every dispatch the receiver has kept, keyed by trace id.
func (r *spanReceiver) all() map[string]dispatchRecord {
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

// absorbSpan keeps one span's facts, if the harness issued its trace and the
// span carries any.
func (r *spanReceiver) absorbSpan(span *tracepb.Span) {
	traceID := hex.EncodeToString(span.GetTraceId())
	if traceID == "" {
		return
	}
	facts := spanFacts(span)
	if !facts.carriesFacts() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, issued := r.issued[traceID]; !issued {
		return
	}
	r.seen[traceID] = r.seen[traceID].merge(facts)
	r.observed.Store(true)
}

// spanFacts reads the attributes the record is made of off one span.
//
// The attribute keys come from internal/mcpotel rather than being spelled
// again here: they are the server's own, and a second spelling of them would
// be a record that quietly emptied itself the day one was renamed.
func spanFacts(span *tracepb.Span) dispatchRecord {
	var facts dispatchRecord
	into := map[string]*string{
		string(mcpotel.AttrGenAIToolName): &facts.tool,
		string(mcpotel.AttrActionID):      &facts.action,
		string(mcpotel.AttrDomain):        &facts.domain,
		string(mcpotel.AttrRefusalReason): &facts.refusalReason,
		string(mcpotel.AttrErrorType):     &facts.errorType,
	}
	for _, attribute := range span.GetAttributes() {
		if target, wanted := into[attribute.GetKey()]; wanted {
			*target = attribute.GetValue().GetStringValue()
		}
	}
	if !facts.carriesFacts() {
		return facts
	}
	facts.status = span.GetStatus().GetCode().String()
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
		"OTEL_EXPORTER_OTLP_ENDPOINT": received.url,
		// Milliseconds, as an integer: the specification defines every OTEL_
		// duration that way, and "100ms" parses as nothing and silently keeps
		// the five-second default, which is longer than a test's whole flush.
		"OTEL_BSP_SCHEDULE_DELAY": "100",
	}
}
