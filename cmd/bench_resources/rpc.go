// rpc.go speaks JSON-RPC to the server directly, on both transports.
//
// The Go SDK's client is deliberately not used here, and the reason is a
// measurement bug it would cause rather than a preference. From protocol
// 2026-07-28 a client may serve tools/list from its own cache (SEP-2549), and
// the SDK does: a second ListTools on the same session returns in microseconds
// without a request ever leaving the process. A harness built on it therefore
// published a tools/list latency of 0.002 ms for a call that costs the server
// hundreds of milliseconds, which is worse than not measuring it.
//
// What an operator sizes for is the work the server does. So this client sends
// the request every time, and the documentation says plainly that a caching
// client pays it less often than these figures suggest.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// protocolVersion is the revision this harness speaks. Pinned rather than
// negotiated down: the caching behavior above, the required per-request
// headers and the absence of a handshake are all properties of this revision.
//
// 2026-07-28 removed initialize. Every request is self-contained and carries
// the client's identity in its own _meta, so there is no handshake to time and
// the first thing a client waits for is its first real call. That is why the
// startup figures published here are anchored on the first tools/list rather
// than on a connection ceremony that no longer exists.
const protocolVersion = "2026-07-28"

// Header name and media types shared by the client here and the stand-in
// servers in stubs.go, which have to agree for a request to be understood.
const (
	headerContentType = "Content-Type"
	mediaJSON         = "application/json"
	mediaEventStream  = "text/event-stream"
	mediaProtobuf     = "application/x-protobuf"
)

// rpcClient is one client's connection to the server.
type rpcClient interface {
	// call issues one request and returns the raw response payload, whose
	// size is itself a published measurement for tools/list.
	call(ctx context.Context, method string, params map[string]any) ([]byte, error)
	// close releases the connection.
	close()
}

// requestBody builds a JSON-RPC request carrying the per-request _meta a
// 2026-07-28 client sends. A request whose header and _meta disagree is
// refused by the transport before any handler runs.
func requestBody(id int64, method string, params map[string]any) ([]byte, error) {
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    protocolVersion,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"name": "bench-resources", "version": "1",
		},
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", method, err)
	}
	return body, nil
}

// rpcError is a JSON-RPC error result, which is a failed measurement rather
// than a transport failure and is reported as such.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error renders the server's refusal.
func (e rpcError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// checkResponse rejects a payload carrying a JSON-RPC error, and a tool result
// that reports failure inside a successful response.
func checkResponse(payload []byte) error {
	var envelope struct {
		Error  *rpcError `json:"error"`
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if envelope.Error != nil {
		return *envelope.Error
	}
	if envelope.Result != nil && envelope.Result.IsError {
		return errors.New("the tool returned an error result")
	}
	return nil
}

// httpRPC talks to a server in HTTP mode, one credential per client.
type httpRPC struct {
	endpoint string
	token    string
	client   *http.Client
	ids      atomic.Int64
}

// newHTTPRPC builds a client for one credential.
func newHTTPRPC(endpoint, token string) *httpRPC {
	return &httpRPC{
		endpoint: endpoint,
		token:    token,
		client: &http.Client{
			Timeout: callTimeout,
			// Every credential gets its own connection pool, so a scenario's
			// clients contend for sockets the way separate callers would
			// rather than sharing one keep-alive connection.
			Transport: &http.Transport{MaxIdleConnsPerHost: 8},
		},
	}
}

// call posts one request and reads the answer.
func (c *httpRPC) call(ctx context.Context, method string, params map[string]any) ([]byte, error) {
	body, err := requestBody(c.ids.Add(1), method, params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set(headerContentType, mediaJSON)
	req.Header.Set("Accept", mediaJSON+", "+mediaEventStream)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	// Protocol 2026-07-28 makes Mcp-Method required: without it the transport
	// refuses the POST before any handler runs.
	req.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok && method == methodToolsCall {
		req.Header.Set("Mcp-Name", name)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("%s: HTTP %d: %s", method, resp.StatusCode, firstLine(payload))
	}
	message := payload
	if strings.HasPrefix(resp.Header.Get(headerContentType), mediaEventStream) {
		message, err = eventStreamPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", method, err)
		}
	}
	if checkErr := checkResponse(message); checkErr != nil {
		return nil, fmt.Errorf("%s: %w", method, checkErr)
	}
	return payload, nil
}

// close releases idle connections.
func (c *httpRPC) close() { c.client.CloseIdleConnections() }

// eventStreamPayload pulls the JSON message out of an SSE body. The server
// answers in SSE by default, which is the shape a real client receives.
func eventStreamPayload(body []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			return []byte(data), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read event stream: %w", err)
	}
	return nil, errors.New("event stream carried no data line")
}

// firstLine trims a body for an error message.
func firstLine(body []byte) string {
	text := strings.TrimSpace(string(body))
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		text = text[:index]
	}
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

// stdioRPC talks to one server process over its pipes.
//
// Responses are demultiplexed by id rather than read in order, because the
// parallelism axis puts several requests in flight on the one connection,
// which is exactly what stdio clients do.
type stdioRPC struct {
	stdin  io.WriteCloser
	writeM sync.Mutex
	ids    atomic.Int64

	mu      sync.Mutex
	waiting map[int64]chan []byte
	failure error

	done chan struct{}
}

// newStdioRPC wires a client to an already-started process's pipes.
func newStdioRPC(stdin io.WriteCloser, stdout io.Reader) *stdioRPC {
	c := &stdioRPC{
		stdin:   stdin,
		waiting: map[int64]chan []byte{},
		done:    make(chan struct{}),
	}
	go c.read(stdout)
	return c
}

// read demultiplexes the server's output until the pipe closes.
func (c *stdioRPC) read(stdout io.Reader) {
	defer close(c.done)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var envelope struct {
			ID *int64 `json:"id"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil || envelope.ID == nil {
			// Notifications carry no id and nothing here waits for one.
			continue
		}
		c.mu.Lock()
		waiter, ok := c.waiting[*envelope.ID]
		delete(c.waiting, *envelope.ID)
		c.mu.Unlock()
		if ok {
			waiter <- line
		}
	}
	c.mu.Lock()
	c.failure = errors.New("the server closed its output")
	if err := scanner.Err(); err != nil {
		c.failure = fmt.Errorf("read from the server: %w", err)
	}
	c.mu.Unlock()
}

// call sends one request and waits for the matching response.
func (c *stdioRPC) call(ctx context.Context, method string, params map[string]any) ([]byte, error) {
	id := c.ids.Add(1)
	body, err := requestBody(id, method, params)
	if err != nil {
		return nil, err
	}

	waiter := make(chan []byte, 1)
	c.mu.Lock()
	if c.failure != nil {
		defer c.mu.Unlock()
		return nil, c.failure
	}
	c.waiting[id] = waiter
	c.mu.Unlock()

	if writeErr := c.write(body); writeErr != nil {
		c.forget(id)
		return nil, writeErr
	}

	select {
	case payload := <-waiter:
		if checkErr := checkResponse(payload); checkErr != nil {
			return nil, fmt.Errorf("%s: %w", method, checkErr)
		}
		return payload, nil
	case <-c.done:
		c.forget(id)
		return nil, fmt.Errorf("%s: the server exited", method)
	case <-ctx.Done():
		c.forget(id)
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// write puts one message on the pipe, serialized against the other clients'
// in-flight requests.
func (c *stdioRPC) write(body []byte) error {
	c.writeM.Lock()
	defer c.writeM.Unlock()
	if _, err := c.stdin.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("write to the server: %w", err)
	}
	return nil
}

// forget drops a waiter whose request failed or timed out.
func (c *stdioRPC) forget(id int64) {
	c.mu.Lock()
	delete(c.waiting, id)
	c.mu.Unlock()
}

// close shuts the input pipe, which is how a stdio client says goodbye.
func (c *stdioRPC) close() { _ = c.stdin.Close() }

// commandProcess is the subset of exec.Cmd the stdio target needs, kept small
// so the pipe wiring can be read in one place.
func commandProcess(cmd *exec.Cmd) (io.WriteCloser, io.Reader, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	return stdin, stdout, nil
}
