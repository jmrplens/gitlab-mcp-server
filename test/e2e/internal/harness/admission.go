//go:build e2e

// admission.go starts a server on a credential the binary admits to nothing,
// one GitLab accepts that carries neither read_api nor api (issue 952), and
// reports how it refused it.
//
// [Env.Session] cannot: a session there is one whose served set matched what
// the assemblers say it should be, and a credential below the admission
// minimum is served no set at all. The refusal differs by transport, which is
// the point of asking both: a stdio process keeps answering the handshake and
// refuses each catalog request in-band, since its token is the operator's and
// an exit would read to a client as a crash, while an HTTP server's gate
// refuses the request that carries the credential, the handshake included.

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AdmissionRefusal is how a server refused a credential below the admission
// minimum: the JSON-RPC error the first catalog request met, and over HTTP the
// status the gate gave it.
type AdmissionRefusal struct {
	// Code is the JSON-RPC error code.
	Code int64
	// Message is the error's message.
	Message string
	// Status is the HTTP status of the refused request, and 0 over stdio,
	// which has none.
	Status int
	// HandshakeAnswered is whether initialize was answered, which a stdio
	// process does and an HTTP gate refusing the credential does not.
	HandshakeAnswered bool
}

// admissionHandshake is the initialize an HTTP refusal is asked for with, as a
// 2025-11-25 client sends it: the gate judges the credential before the body,
// so what it carries only has to be a request the endpoint would serve.
const admissionHandshake = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
	`{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"gitlab-mcp-e2e-harness","version":"1"}}}`

// RefusedAdmission starts a server for cfg on cfg.Token, which a test minted
// below the admission minimum, and returns how the server refused it: over
// stdio the handshake answered and tools/list refused, over HTTP the handshake
// refused by the gate. A server that served the catalog, or that could not be
// started, fails the test. The server is the test's alone and is stopped when
// the test ends.
func (e *Env) RefusedAdmission(cfg ServerConfig) AdmissionRefusal {
	e.T.Helper()

	cfg.Private = true
	normalized := cfg.normalized()
	if err := normalized.validate(); err != nil {
		e.T.Fatalf("a refused admission's configuration: %v", err)
	}
	if normalized.Token == "" || normalized.Token == e.inst.settings.get(envGitLabToken) {
		e.T.Fatal("a refused admission needs a credential of its own: the run's own token is admitted")
	}

	label := "admission-" + string(normalized.Transport) + "-" + normalized.label(privateSessions.Add(1))
	proc, _, err := newChild(e.inst, normalized, normalized.Token, label)
	if err != nil {
		e.T.Fatalf("describing the %s server: %v", label, err)
	}
	lifetime, stop := context.WithCancel(sessionLifetime())
	// The child is this test's alone, so it ends with the test and is waited
	// for: it is in no session the run closes at its end, and a child still
	// exiting when the package removes its directory and binary loses the
	// coverage counters it writes on the way out.
	e.T.Cleanup(func() {
		stop()
		proc.waitForExit(time.Now().Add(childTerminateDelay + childStopTimeout))
	})
	ctx, cancel := context.WithTimeout(lifetime, sessionStartTimeout)
	defer cancel()

	var refusal AdmissionRefusal
	if normalized.Transport == TransportHTTP {
		refusal, err = refusedOverHTTP(ctx, lifetime, proc)
	} else {
		refusal, err = refusedOverStdio(ctx, lifetime, proc)
	}
	if err != nil {
		e.T.Fatalf("the %s server: %v\nserver stderr:\n%s", label, err, proc.stderrTail())
	}
	return refusal
}

// refusedOverStdio starts the child over its pipes, completes the handshake
// and asks tools/list, which a process whose token is below the minimum
// refuses in-band.
func refusedOverStdio(ctx, lifetime context.Context, proc *serverProcess) (AdmissionRefusal, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "gitlab-mcp-e2e-harness", Version: "1"}, nil)
	session, err := client.Connect(ctx, proc.transport(lifetime), nil)
	if err != nil {
		return AdmissionRefusal{}, fmt.Errorf("the handshake was not answered: %w", err)
	}
	defer func() { _ = session.Close() }()

	if _, err = session.ListTools(ctx, nil); err == nil {
		return AdmissionRefusal{}, errors.New("tools/list was served")
	}
	wire, isWire := errors.AsType[*jsonrpc.Error](err)
	if !isWire {
		return AdmissionRefusal{}, fmt.Errorf("tools/list failed without a JSON-RPC error: %w", err)
	}
	return AdmissionRefusal{Code: wire.Code, Message: wire.Message, HandshakeAnswered: true}, nil
}

// refusedOverHTTP starts the child listening on a loopback address and posts
// the handshake with the credential in the header a deployment's client sends
// it in, reading the gate's answer as it is on the wire rather than through a
// client that would report only the status.
func refusedOverHTTP(ctx, lifetime context.Context, proc *serverProcess) (AdmissionRefusal, error) {
	addr, err := freeLoopbackAddr(lifetime)
	if err != nil {
		return AdmissionRefusal{}, err
	}
	transport, err := proc.httpTransport(lifetime, addr)
	if err != nil {
		return AdmissionRefusal{}, err
	}
	streamable, isStreamable := transport.(*mcp.StreamableClientTransport)
	if !isStreamable {
		return AdmissionRefusal{}, fmt.Errorf("the HTTP transport is a %T, not a streamable client", transport)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, streamable.Endpoint, bytes.NewBufferString(admissionHandshake))
	if err != nil {
		return AdmissionRefusal{}, fmt.Errorf("building the handshake: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := streamable.HTTPClient.Do(req)
	if err != nil {
		return AdmissionRefusal{}, fmt.Errorf("posting the handshake: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, gateAnswerBytes))
	if err != nil {
		return AdmissionRefusal{}, fmt.Errorf("reading the gate's answer: %w", err)
	}
	return gateRefusal(resp.StatusCode, body)
}

// gateAnswerBytes bounds what is read of a gate's answer, which is one short
// JSON-RPC error.
const gateAnswerBytes = 64 << 10

// gateRefusal reads the answer an HTTP gate gave a handshake carrying a
// credential below the minimum: a refusal is a status that is not 2xx and a
// body carrying a JSON-RPC error, which every refusal this server's gate
// writes carries. A 2xx is the credential admitted.
func gateRefusal(status int, body []byte) (AdmissionRefusal, error) {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return AdmissionRefusal{}, fmt.Errorf("the handshake was admitted with %d", status)
	}
	var answer struct {
		Error *struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &answer); err != nil || answer.Error == nil {
		return AdmissionRefusal{}, fmt.Errorf("the %d answer carries no JSON-RPC error: %q", status, body)
	}
	return AdmissionRefusal{Status: status, Code: answer.Error.Code, Message: answer.Error.Message}, nil
}
