// sessions.go measures what a stateful session costs the server, and what the
// process does with more of them than it can hold: the measurement the
// stateful-session ceiling (register row HLD-010, issue 951) is sized from.
//
// A stateful session is what the --stateless=false transport keeps for a
// client once it has initialized: the SDK holds it, and everything serving
// it, until the client deletes it, the pool evicts its credential, or it has
// sat idle for --session-timeout, half an hour by default. An idle session
// holds no connection, so what it costs is memory and goroutines for as long
// as it is kept. Each may also open one standalone stream, a GET the server
// holds open for the session's life, which is a connection and so a
// descriptor.
//
// A step opens a known number of sessions at once, spread round robin across
// -sessions-credentials, each with its standalone stream when -sessions-stream
// asks, leaves them idle and samples the process. It then asks every session
// it opened whether the server still holds it, which is the SDK's session
// count as seen from outside, and closes them all before the next step.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// defaultSessionsRecord is where a -sessions run writes its document, under
// bench/ beside the held one, for the same reason: it measures a bound rather
// than the published matrix.
const defaultSessionsRecord = "bench/sessions.json"

// sessionsSchema versions the -sessions document.
const sessionsSchema = 1

// sessionsCounted is what a -sessions count counts, as its refusals name it.
const sessionsCounted = "session"

// sessionProtocol is the revision a session is opened with. The stateful
// transport refuses 2026-07-28, which has no sessions, so a session speaks the
// revision before it and carries none of the per-request _meta that revision
// added.
const sessionProtocol = "2025-11-25"

// sessionIDHeader names the session a request belongs to.
const sessionIDHeader = "Mcp-Session-Id"

// The methods a session's life takes, and the one message of them that is a
// notification, which is answered 202 with nothing rather than 200 with a
// response.
const (
	methodInitialize  = "initialize"
	methodInitialized = "notifications/initialized"
	methodPing        = "ping"
	methodStream      = "the standalone stream"
	methodDelete      = "DELETE"
)

// SessionsDoc is the document a -sessions run writes.
type SessionsDoc struct {
	Schema      int        `json:"schema"`
	GeneratedAt string     `json:"generated_at"`
	Host        HostInfo   `json:"host"`
	Server      ServerInfo `json:"server"`
	// Credentials is how many credentials the sessions were spread across,
	// round robin, and NoFile the descriptor limit the server was started
	// under, zero when it inherited this process's.
	Credentials int  `json:"credentials"`
	NoFile      int  `json:"nofile,omitempty"`
	Streams     bool `json:"streams"`
	// Idle is the process with every credential admitted and no session
	// open, which is what each step's per-session figures are measured over.
	Idle  HeldSample     `json:"idle"`
	Steps []SessionsStep `json:"steps"`
}

// SessionsStep is one count of sessions opened at once.
type SessionsStep struct {
	Offered int `json:"offered"`
	// Open is how many sessions the server had given an id when the step was
	// sampled, and Live how many of them still answered a ping on it right
	// after: the sessions the server holds, as seen from outside.
	Open int `json:"open"`
	Live int `json:"live"`
	// Settled is how the wait before the sample ended.
	Settled string `json:"settled"`
	// The outcome of every session offered once the step is over. Opened is a
	// session the server accepted, its stream too when one was asked for;
	// Refused a 503, the status the ceiling answers with, its first text kept;
	// Failed anything else.
	Opened       int        `json:"opened"`
	Refused      int        `json:"refused"`
	Failed       int        `json:"failed"`
	FirstRefusal string     `json:"first_refusal,omitempty"`
	FirstFailure string     `json:"first_failure,omitempty"`
	Sample       HeldSample `json:"sample"`
	// PerSession is what one open session cost over the idle process, absent
	// when none was open.
	PerSession *HeldCost `json:"per_session,omitempty"`
}

// validateSessions refuses a -sessions run the flags beside it make
// meaningless, and the counts and limits it cannot measure with. Like the held
// check it runs whatever else was asked for, since a -sessions run is a
// measurement.
func (o options) validateSessions() error {
	if o.sessions == "" {
		return nil
	}
	if o.fairness != "" || o.held != "" {
		return errors.New("-sessions, -held and -fairness are measurements with documents of their own: give one of them")
	}
	if o.render || o.check {
		return fmt.Errorf("-sessions measures one server and draws nothing, so it cannot be combined with %s, "+
			"which draws the committed artifacts and measures nothing", renderFlagName(o))
	}
	if _, err := parseCounts("-sessions", sessionsCounted, o.sessions); err != nil {
		return err
	}
	if o.sessionsCredentials <= 0 {
		return fmt.Errorf("-sessions-credentials must be positive, got %d", o.sessionsCredentials)
	}
	if o.sessionsNoFile < 0 {
		return fmt.Errorf("-sessions-nofile must not be negative, got %d", o.sessionsNoFile)
	}
	if o.sessionsNoFile > 0 && runtimeGOOS == "windows" {
		return errors.New("-sessions-nofile starts the server through /bin/sh and ulimit, which Windows has neither of")
	}
	return nil
}

// runSessions measures the session ladder and writes its own document.
//
// Like the held mode it returns before the record is read or a chart is drawn,
// so it cannot rewrite anything published.
func runSessions(opts options, root string) error {
	steps, err := parseCounts("-sessions", sessionsCounted, opts.sessions)
	if err != nil {
		return err
	}
	out := resolve(root, opts.sessionsJSON)
	for _, published := range []string{opts.record, defaultRecord} {
		if out == resolve(root, published) {
			return fmt.Errorf("-sessions-json names %s, which is the published record: "+
				"give the sessions run a path of its own", rel(root, out))
		}
	}

	r, cleanup, err := newHarness(opts, root)
	if err != nil {
		return err
	}
	defer cleanup()

	doc := &SessionsDoc{
		Schema:      sessionsSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Host:        hostInfo(),
		Credentials: opts.sessionsCredentials,
		NoFile:      opts.sessionsNoFile,
		Streams:     opts.sessionsStream,
	}
	fmt.Printf("sessions: %d credentials, descriptor limit %s, streams %t, counts %v\n",
		opts.sessionsCredentials, nofileLabel(opts.sessionsNoFile), opts.sessionsStream, steps)
	if runErr := r.runSessionsLadder(context.Background(), opts, steps, doc); runErr != nil {
		return runErr
	}
	if writeErr := writeJSON(out, doc, "the sessions record"); writeErr != nil {
		return writeErr
	}
	fmt.Printf("wrote %s\n", rel(root, out))
	return nil
}

// runSessionsLadder starts a stateful server, admits the credentials, samples
// the idle process and then every step in turn.
func (r *runner) runSessionsLadder(ctx context.Context, opts options, steps []int, doc *SessionsDoc) error {
	tgt := &httpTarget{
		binary: r.binary, stubURL: r.stub.url, otlpURL: r.otlp.url,
		plan:  scenarioPlan{ID: "sessions", Transport: transportHTTP, Surface: surfaceDynamic},
		pprof: true, maxClients: opts.sessionsCredentials, nofile: opts.sessionsNoFile, stateful: true,
	}
	defer tgt.close()
	if _, err := tgt.start(ctx); err != nil {
		return err
	}
	doc.Server = tgt.serverInfo()

	clients := make([]*sessionClient, opts.sessionsCredentials)
	for i := range clients {
		clients[i] = newSessionClient("http://"+tgt.addr+"/mcp", benchToken+strconv.Itoa(i))
	}
	if err := admitSessions(ctx, clients); err != nil {
		return err
	}
	in := heldInput{tgt: tgt, profiler: newPprofClient("http://" + tgt.pprofAddr)}
	doc.Idle = r.sampleHeld(ctx, in)
	fmt.Printf("  idle: %s\n", doc.Idle.summary())
	for _, offered := range steps {
		step := r.runSessionsStep(ctx, in, clients, offered, opts.sessionsStream)
		step.PerSession = costPer(step.Sample, doc.Idle, step.Open)
		doc.Steps = append(doc.Steps, step)
		fmt.Printf("  %s\n", step.summary())
	}
	return nil
}

// admitSessions opens one session per credential, lists its tools and closes
// it, so the pool holds a built entry for each before the idle process is
// sampled. A pooled entry is built on its credential's first request and
// registers its catalog in the background, which the listing waits for; an
// entry built inside a step would be priced as part of the sessions.
func admitSessions(ctx context.Context, clients []*sessionClient) error {
	for i, client := range clients {
		session, err := client.open(ctx, false)
		if err == nil {
			_, err = client.send(ctx, session.id, methodToolsList, callMessage(client.ids.Add(1), methodToolsList))
		}
		if session != nil {
			session.close(ctx)
		}
		client.close()
		if err != nil {
			return fmt.Errorf("admit credential %d: %w", i, err)
		}
	}
	return nil
}

// runSessionsStep opens one count of sessions at once, samples the process
// while they sit idle, asks each whether the server still holds it, closes
// them all and files how each open ended.
func (r *runner) runSessionsStep(ctx context.Context, in heldInput, clients []*sessionClient, offered int, streams bool) SessionsStep {
	var tally heldTally
	var opened sessionList
	var wg sync.WaitGroup
	for i := range offered {
		client := clients[i%len(clients)]
		wg.Go(func() {
			session, err := client.open(ctx, streams)
			opened.add(session)
			tally.record(err)
		})
	}
	step := SessionsStep{Offered: offered}
	step.Settled = awaitHeld(ctx, offered, tally.returned.Load)
	step.Sample = r.sampleHeld(ctx, in)
	open := opened.all()
	step.Open = len(open)
	step.Live = countLive(ctx, open)
	// The streams go first: an open still waiting for a connection a process
	// out of descriptors has not accepted gets one once they are gone, and
	// the step waits for every open to come back before it counts them.
	for _, session := range open {
		session.closeStream()
	}
	wg.Wait()
	for _, session := range opened.all() {
		session.close(ctx)
	}
	for _, client := range clients {
		client.close()
	}
	step.Opened = int(tally.served.Load())
	step.Refused = int(tally.refused.Load())
	step.Failed = int(tally.failed.Load())
	step.FirstRefusal, step.FirstFailure = tally.firstRefusal, tally.firstFailure
	return step
}

// countLive pings every session and counts those the server answered, which is
// how many of them it still holds.
//
// Each ping waits for its answer no longer than a /health probe does, and the
// count stops at the first ping that got no answer at all: a process that did
// not accept one connection will not accept the next, and asking each of a
// thousand sessions in turn would hold the step for as long as they all took
// to time out. A session the server answered for and no longer holds (a 404)
// is not live, and the count goes on.
func countLive(ctx context.Context, sessions []*benchSession) int {
	live := 0
	for _, session := range sessions {
		err := session.ping(ctx)
		if err == nil {
			live++
			continue
		}
		if _, answered := errors.AsType[*httpStatusError](err); !answered {
			break
		}
	}
	return live
}

// sessionList collects the sessions a step's opens were given an id for,
// whatever became of their stream, so every one of them is closed.
type sessionList struct {
	mu    sync.Mutex
	items []*benchSession
}

// add keeps a session, and nothing for an open that got none.
func (l *sessionList) add(session *benchSession) {
	if session == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, session)
}

// all returns the sessions kept so far.
func (l *sessionList) all() []*benchSession {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.items)
}

// sessionClient opens and drives stateful sessions for one credential.
type sessionClient struct {
	endpoint string
	token    string
	client   *http.Client
	ids      atomic.Int64
}

// newSessionClient builds a client for one credential.
//
// The client carries no timeout of its own, because a standalone stream is a
// response that never ends; each POST bounds itself with callTimeout, and the
// transport bounds the wait for any response's headers to the same, so a
// process out of descriptors cannot hold a step forever.
func newSessionClient(endpoint, token string) *sessionClient {
	return &sessionClient{
		endpoint: endpoint,
		token:    token,
		client: &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost:   8,
			ResponseHeaderTimeout: callTimeout,
		}},
	}
}

// close releases the client's idle connections, so a step starts from none: an
// idle keep-alive connection left over would be counted against it.
func (c *sessionClient) close() { c.client.CloseIdleConnections() }

// initializeMessage is the request that opens a session.
func initializeMessage(id int64) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{"protocolVersion":%q,`+
		`"capabilities":{},"clientInfo":{"name":"bench-resources","version":"1"}}}`, id, methodInitialize, sessionProtocol)
}

// initializedMessage is the notification that completes the handshake.
const initializedMessage = `{"jsonrpc":"2.0","method":"` + methodInitialized + `"}`

// callMessage is a request with no params: a ping, which asks a session
// whether the server still holds it, or a tools/list.
func callMessage(id int64, method string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`, id, method)
}

// request builds a request of this client's to the endpoint, carrying the
// credential, the revision and the session it belongs to, if any.
func (c *sessionClient) request(ctx context.Context, method, session, accept string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("MCP-Protocol-Version", sessionProtocol)
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if session != "" {
		req.Header.Set(sessionIDHeader, session)
	}
	return req, nil
}

// send posts one message on session, or on none to open one, and returns the
// session id the answer carried. A request is answered 200 with its response,
// a notification 202 with none; any other status is refused with it kept.
func (c *sessionClient) send(ctx context.Context, session, method, message string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	req, err := c.request(ctx, http.MethodPost, session, mediaJSON+", "+mediaEventStream, strings.NewReader(message))
	if err != nil {
		return "", err
	}
	req.Header.Set(headerContentType, mediaJSON)
	req.Header.Set("Mcp-Method", method)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s response: %w", method, err)
	}
	notification := strings.HasPrefix(method, "notifications/")
	want := http.StatusOK
	if notification {
		want = http.StatusAccepted
	}
	if resp.StatusCode != want {
		return "", &httpStatusError{Method: method, Status: resp.StatusCode, Snippet: firstLine(payload)}
	}
	if !notification {
		if answerErr := checkAnswer(resp, payload); answerErr != nil {
			return "", fmt.Errorf("%s: %w", method, answerErr)
		}
	}
	return resp.Header.Get(sessionIDHeader), nil
}

// checkAnswer reads the JSON-RPC response out of a 200, in the event stream the
// server answers with by default or as a bare body, and rejects an error.
func checkAnswer(resp *http.Response, payload []byte) error {
	message := payload
	if strings.HasPrefix(resp.Header.Get(headerContentType), mediaEventStream) {
		var err error
		if message, err = eventStreamPayload(payload); err != nil {
			return err
		}
	}
	return checkResponse(message)
}

// open initializes a session and, when stream asks, opens its standalone
// stream. A session the server gave an id is returned even when a later step
// of the opening failed, so the caller can close it.
func (c *sessionClient) open(ctx context.Context, stream bool) (*benchSession, error) {
	id, err := c.send(ctx, "", methodInitialize, initializeMessage(c.ids.Add(1)))
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, errors.New("initialize: the server answered with no session id")
	}
	session := &benchSession{client: c, id: id}
	if _, handshakeErr := c.send(ctx, id, methodInitialized, initializedMessage); handshakeErr != nil {
		return session, handshakeErr
	}
	if !stream {
		return session, nil
	}
	streamErr := session.openStream(ctx)
	return session, streamErr
}

// benchSession is one session the server gave an id.
type benchSession struct {
	client *sessionClient
	id     string

	// stop ends the standalone stream and done closes once its reader has
	// returned; both are nil for a session that opened none. once makes
	// ending it idempotent, since a step ends every stream before it waits
	// for the last opens and then closes every session.
	stop func()
	done chan struct{}
	once sync.Once
}

// openStream opens the session's standalone stream and keeps it open until the
// session is closed, reading what the server sends so its buffers never fill.
func (s *benchSession) openStream(ctx context.Context) error {
	streamCtx, stop := context.WithCancel(ctx)
	req, err := s.client.request(streamCtx, http.MethodGet, s.id, mediaEventStream, http.NoBody)
	if err != nil {
		stop()
		return err
	}
	resp, err := s.client.client.Do(req) //nolint:bodyclose // the stream's reader closes the body, once the session ends
	if err != nil {
		stop()
		return fmt.Errorf("%s: %w", methodStream, err)
	}
	if resp.StatusCode != http.StatusOK {
		// A refusal is a short body, but nothing promises one ends: read what
		// it says for no longer than the /health probe waits, so a response
		// that stays open cannot hold the step.
		abandon := time.AfterFunc(healthTimeout, stop)
		snippet, _ := io.ReadAll(resp.Body)
		abandon.Stop()
		_ = resp.Body.Close()
		stop()
		return &httpStatusError{Method: methodStream, Status: resp.StatusCode, Snippet: firstLine(snippet)}
	}
	s.stop, s.done = stop, make(chan struct{})
	go func() {
		defer close(s.done)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	return nil
}

// closeStream ends the standalone stream, if the session opened one, and waits
// for its reader to return.
func (s *benchSession) closeStream() {
	s.once.Do(func() {
		if s.stop != nil {
			s.stop()
			<-s.done
		}
	})
}

// ping asks the server whether it still holds the session, waiting no longer
// than a /health probe does for the answer.
func (s *benchSession) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	_, err := s.client.send(ctx, s.id, methodPing, callMessage(s.client.ids.Add(1), methodPing))
	return err
}

// close ends the stream and then the session. How the DELETE was answered is
// not read: a session the server no longer holds answers 404, and either way
// the session is gone, which is all the next step needs.
func (s *benchSession) close(ctx context.Context) {
	s.closeStream()
	_ = s.client.remove(ctx, s.id)
}

// remove deletes a session, reporting only whether the request could be made.
func (c *sessionClient) remove(ctx context.Context, session string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	req, err := c.request(ctx, http.MethodDelete, session, mediaJSON, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", methodDelete, err)
	}
	return resp.Body.Close()
}

// summary renders a step for the progress line.
func (s SessionsStep) summary() string {
	line := fmt.Sprintf("%d offered, %d open (%s): %s; %d live; then %d opened, %d refused, %d failed",
		s.Offered, s.Open, s.Settled, s.Sample.summary(), s.Live, s.Opened, s.Refused, s.Failed)
	if s.PerSession != nil {
		line += fmt.Sprintf("; per session %.2f descriptors, %.2f goroutines, %.1f KiB heap, %.1f KiB resident",
			s.PerSession.Descriptors, s.PerSession.Goroutines, s.PerSession.HeapKiB, s.PerSession.RSSKiB)
	}
	return line
}
