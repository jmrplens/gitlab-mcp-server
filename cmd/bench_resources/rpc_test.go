// rpc_test.go covers the JSON-RPC client the measurements are taken with: the
// request shape protocol 2026-07-28 requires, the two response framings the
// server uses, and the demultiplexing that lets parallel requests share one
// stdio pipe.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRequestBody_CarriesTheRequiredMeta verifies every request carries the
// per-request _meta a 2026-07-28 client sends, alongside the caller's own
// parameters. A request whose _meta is missing or disagrees with the header is
// refused by the transport before any handler runs, so this is what stands
// between the harness and a run of nothing but errors.
func TestRequestBody_CarriesTheRequiredMeta(t *testing.T) {
	body, err := requestBody(7, "tools/call", map[string]any{"name": "gitlab_find_action"})
	if err != nil {
		t.Fatalf("requestBody: %v", err)
	}

	var decoded struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name string `json:"name"`
			Meta struct {
				ProtocolVersion string `json:"io.modelcontextprotocol/protocolVersion"`
			} `json:"_meta"`
		} `json:"params"`
	}
	if unmarshalErr := json.Unmarshal(body, &decoded); unmarshalErr != nil {
		t.Fatalf("the request is not valid JSON: %v", unmarshalErr)
	}
	if decoded.JSONRPC != "2.0" || decoded.ID != 7 || decoded.Method != "tools/call" {
		t.Errorf("envelope = %+v, want jsonrpc 2.0 id 7 tools/call", decoded)
	}
	if decoded.Params.Name != "gitlab_find_action" {
		t.Errorf("the caller's parameters were lost: %+v", decoded.Params)
	}
	if decoded.Params.Meta.ProtocolVersion != protocolVersion {
		t.Errorf("_meta protocol version = %q, want %q", decoded.Params.Meta.ProtocolVersion, protocolVersion)
	}
}

// TestCheckResponse_ClassifiesResults verifies a JSON-RPC error and a tool
// result that reports failure are both treated as failed measurements, while
// an ordinary result is not.
func TestCheckResponse_ClassifiesResults(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{name: "result", payload: `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`, wantErr: false},
		{name: "rpc error", payload: `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`, wantErr: true},
		{name: "tool error", payload: `{"jsonrpc":"2.0","id":1,"result":{"isError":true}}`, wantErr: true},
		{name: "not json", payload: `<html>`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkResponse([]byte(tc.payload))
			if tc.wantErr != (err != nil) {
				t.Errorf("checkResponse(%s) error = %v, want error %v", tc.payload, err, tc.wantErr)
			}
		})
	}
}

// TestEventStreamPayload_ExtractsTheMessage verifies the JSON message is
// pulled out of the SSE framing the server answers with by default, and that
// a stream carrying no message is reported rather than parsed as empty.
func TestEventStreamPayload_ExtractsTheMessage(t *testing.T) {
	body := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"
	got, err := eventStreamPayload([]byte(body))
	if err != nil {
		t.Fatalf("eventStreamPayload: %v", err)
	}
	if !strings.HasPrefix(string(got), `{"jsonrpc"`) {
		t.Errorf("payload = %q, want the JSON message", got)
	}
	if _, emptyErr := eventStreamPayload([]byte("event: ping\n\n")); emptyErr == nil {
		t.Error("eventStreamPayload accepted a stream with no data line")
	}
}

// TestHTTPRPC_SendsTheProtocolHeaders verifies the client sends what a
// 2026-07-28 HTTP client must: the credential that keys its pool entry, the
// protocol version, and the method name in the header the transport requires,
// with the tool name added for a tools/call.
func TestHTTPRPC_SendsTheProtocolHeaders(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n")
	}))
	defer server.Close()

	client := newHTTPRPC(server.URL, "bench-token-3")
	defer client.close()

	if _, err := client.call(context.Background(), "tools/call", map[string]any{"name": "gitlab_server_status"}); err != nil {
		t.Fatalf("call: %v", err)
	}
	want := map[string]string{
		"Private-Token":        "bench-token-3",
		"Mcp-Protocol-Version": protocolVersion,
		"Mcp-Method":           "tools/call",
		"Mcp-Name":             "gitlab_server_status",
		"Content-Type":         "application/json",
	}
	for header, value := range want {
		t.Run(header, func(t *testing.T) {
			if got.Get(header) != value {
				t.Errorf("%s = %q, want %q", header, got.Get(header), value)
			}
		})
	}
}

// TestHTTPRPC_FailureModes verifies a refused request is a failed measurement
// with the reason attached, whether the refusal arrives as a status code or as
// a JSON-RPC error inside a 200.
func TestHTTPRPC_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "status code",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "rate limited", http.StatusTooManyRequests)
			},
			want: "429",
		},
		{
			name: "rpc error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`)
			},
			want: "method not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			client := newHTTPRPC(server.URL, "token")
			defer client.close()

			_, err := client.call(context.Background(), "tools/list", nil)
			if err == nil {
				t.Fatal("call reported success for a refused request")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// TestHTTPRPC_PlainJSONResponse verifies the client also reads the
// application/json framing a server started with --json-response uses.
func TestHTTPRPC_PlainJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"resources":[]}}`)
	}))
	defer server.Close()

	client := newHTTPRPC(server.URL, "token")
	defer client.close()

	payload, err := client.call(context.Background(), "resources/list", nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(payload) == 0 {
		t.Error("the client returned no payload, so no response size could be measured")
	}
}

// TestStdioRPC_ParallelRequests_AreMatchedByID verifies several requests in
// flight on one pipe each get their own answer, out of order.
//
// This is what the parallelism axis rests on: a client that read responses in
// order would hand one request's timing to another and quietly report the
// wrong distribution.
func TestStdioRPC_ParallelRequests_AreMatchedByID(t *testing.T) {
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	defer func() { _ = toServer.Close() }()

	// A server that answers every request, deliberately in reverse order of
	// arrival: it collects three, then replies from the last to the first.
	go func() {
		decoder := json.NewDecoder(toServer)
		var ids []int64
		for len(ids) < 3 {
			var request struct {
				ID int64 `json:"id"`
			}
			if err := decoder.Decode(&request); err != nil {
				return
			}
			ids = append(ids, request.ID)
		}
		for _, id := range slices.Backward(ids) {
			_, _ = io.WriteString(fromServer, `{"jsonrpc":"2.0","id":`+itoa(id)+`,"result":{"seen":`+itoa(id)+`}}`+"\n")
		}
	}()

	client := newStdioRPC(fromClient, toClient)
	defer client.close()

	var wg sync.WaitGroup
	results := make([][]byte, 3)
	errs := make([]error, 3)
	for i := range 3 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			results[index], errs[index] = client.call(ctx, "tools/list", nil)
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		var decoded struct {
			ID     int64 `json:"id"`
			Result struct {
				Seen int64 `json:"seen"`
			} `json:"result"`
		}
		if err := json.Unmarshal(results[i], &decoded); err != nil {
			t.Fatalf("decoding response %d: %v", i, err)
		}
		if decoded.ID != decoded.Result.Seen {
			t.Errorf("response for id %d carries the answer to %d", decoded.ID, decoded.Result.Seen)
		}
		if seen[itoa(decoded.ID)] {
			t.Errorf("id %d was handed to two callers", decoded.ID)
		}
		seen[itoa(decoded.ID)] = true
	}
}

// TestStdioRPC_ServerExits_ReportsRatherThanHangs verifies a call whose server
// died comes back as an error, since a benchmark that hung on a crashed
// process would look like a very slow one.
func TestStdioRPC_ServerExits_ReportsRatherThanHangs(t *testing.T) {
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()

	go func() {
		// Read the request, then close the output the way a dying process
		// does.
		buffer := make([]byte, 4096)
		_, _ = toServer.Read(buffer)
		_ = fromServer.Close()
	}()

	client := newStdioRPC(fromClient, toClient)
	defer client.close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.call(ctx, "tools/list", nil); err == nil {
		t.Error("the call reported success after the server closed its output")
	}
}

// TestStdioRPC_ContextCancelled_ReturnsPromptly verifies a caller's timeout is
// honored rather than waiting for a response that is not coming.
func TestStdioRPC_ContextCancelled_ReturnsPromptly(t *testing.T) {
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	defer func() { _ = fromServer.Close() }()
	go func() {
		buffer := make([]byte, 4096)
		for {
			if _, err := toServer.Read(buffer); err != nil {
				return
			}
		}
	}()

	client := newStdioRPC(fromClient, toClient)
	defer client.close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := client.call(ctx, "tools/list", nil); err == nil {
		t.Error("the call reported success though nothing answered it")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("the call took %s to honor a 50 ms deadline", elapsed)
	}
}

// itoa renders an id for the fixtures above.
func itoa(value int64) string { return strconv.FormatInt(value, 10) }

// TestRequestBody_UnencodableParams_IsReported verifies a parameter the
// encoder cannot serialize fails at the request rather than at the wire,
// which is the one failure requestBody has.
func TestRequestBody_UnencodableParams_IsReported(t *testing.T) {
	_, err := requestBody(1, "tools/call", map[string]any{"bad": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "encode tools/call request") {
		t.Errorf("requestBody = %v, want the encoding failure", err)
	}
}

// TestHTTPRPC_EveryFailureIsNamed covers the failures between building a
// request and reading a usable answer: parameters that cannot be encoded, an
// endpoint that is not a URL, nothing listening, a body cut short, and an
// event stream with no message in it.
func TestHTTPRPC_EveryFailureIsNamed(t *testing.T) {
	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short")
	}))
	defer truncated.Close()
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: ping\n\n")
	}))
	defer empty.Close()

	cases := []struct {
		name     string
		endpoint string
		params   map[string]any
		want     string
	}{
		{name: "unencodable params", endpoint: truncated.URL, params: map[string]any{"bad": make(chan int)}, want: "encode"},
		{name: "endpoint is not a url", endpoint: "http://bad host", want: "build tools/list request"},
		{name: "nothing listening", endpoint: "http://127.0.0.1:1", want: "tools/list:"},
		{name: "body cut short", endpoint: truncated.URL, want: "read tools/list response"},
		{name: "event stream with no message", endpoint: empty.URL, want: "no data line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newHTTPRPC(tc.endpoint, "token")
			defer client.close()
			_, err := client.call(context.Background(), "tools/list", tc.params)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("call = %v, want an error saying %q", err, tc.want)
			}
		})
	}
}

// TestEventStreamPayload_LineTooLong_IsReported verifies a line longer than
// the scanner's ceiling is reported rather than read as an empty stream: the
// ceiling exists so a runaway response cannot take the driver's memory, and
// the report is what tells the operator that is what happened.
func TestEventStreamPayload_LineTooLong_IsReported(t *testing.T) {
	body := append([]byte("data: "), bytes.Repeat([]byte("x"), 33<<20)...)
	if _, err := eventStreamPayload(body); err == nil || !strings.Contains(err.Error(), "read event stream") {
		t.Errorf("eventStreamPayload = %v, want the scanner's refusal", err)
	}
}

// failingReader is a server output that fails on the first read, which is
// what a broken pipe looks like to the demultiplexer.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("pipe broken") }

// TestStdioRPC_Failures covers what the stdio client does when the
// server's side goes wrong: lines that are not responses are skipped, a
// read error is remembered and every later call gets it, a closed input
// fails the write, and a response carrying an error is a failed call.
func TestStdioRPC_Failures(t *testing.T) {
	t.Run("skips what is not a response and remembers the read error", func(t *testing.T) {
		toClient, fromServer := io.Pipe()
		toServer, fromClient := io.Pipe()
		go func() {
			buffer := make([]byte, 4096)
			_, _ = toServer.Read(buffer)
			_, _ = io.WriteString(fromServer, "not json at all\n")
			_, _ = io.WriteString(fromServer, `{"jsonrpc":"2.0","method":"notifications/progress"}`+"\n")
			_, _ = io.WriteString(fromServer, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"refused"}}`+"\n")
			_ = fromServer.Close()
		}()
		client := newStdioRPC(fromClient, toClient)
		defer client.close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := client.call(ctx, "tools/list", nil); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("call = %v, want the server's refusal", err)
		}
		<-client.done
		if _, err := client.call(ctx, "tools/list", nil); err == nil || !strings.Contains(err.Error(), "closed its output") {
			t.Errorf("a call after the server left = %v, want the remembered failure", err)
		}
	})

	t.Run("a read error is the failure", func(t *testing.T) {
		_, fromClient := io.Pipe()
		client := newStdioRPC(fromClient, failingReader{})
		defer client.close()
		<-client.done
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := client.call(ctx, "tools/list", nil); err == nil || !strings.Contains(err.Error(), "pipe broken") {
			t.Errorf("call = %v, want the read error", err)
		}
	})

	t.Run("a closed input fails the write", func(t *testing.T) {
		toClient, _ := io.Pipe()
		_, fromClient := io.Pipe()
		client := newStdioRPC(fromClient, toClient)
		client.close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := client.call(ctx, "tools/list", nil); err == nil || !strings.Contains(err.Error(), "write to the server") {
			t.Errorf("call = %v, want the write failure", err)
		}
	})

	t.Run("unencodable params", func(t *testing.T) {
		toClient, _ := io.Pipe()
		_, fromClient := io.Pipe()
		client := newStdioRPC(fromClient, toClient)
		defer client.close()
		if _, err := client.call(context.Background(), "tools/call", map[string]any{"bad": make(chan int)}); err == nil {
			t.Error("call accepted parameters it cannot encode")
		}
	})
}

// TestCommandProcess_RefusesPipesAlreadyWired covers the two ways exec.Cmd
// declines to hand out a pipe: a stream the caller already assigned.
func TestCommandProcess_RefusesPipesAlreadyWired(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*exec.Cmd)
		wantErr string
	}{
		{name: "stdin already set", prepare: func(c *exec.Cmd) { c.Stdin = strings.NewReader("") }, wantErr: "stdin pipe"},
		{name: "stdout already set", prepare: func(c *exec.Cmd) { c.Stdout = io.Discard }, wantErr: "stdout pipe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "go", "version")
			tc.prepare(cmd)
			_, _, err := commandProcess(cmd)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("commandProcess = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestFirstLine_TrimsForAnErrorMessage checks a body is cut at its first line
// and at two hundred bytes, since the message it goes into is one line of a
// note.
func TestFirstLine_TrimsForAnErrorMessage(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{name: "multi-line", body: "  first\nsecond\n", want: "first"},
		{name: "long", body: strings.Repeat("x", 250), want: strings.Repeat("x", 200)},
		{name: "blank", body: "\n\n", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstLine([]byte(tc.body)); got != tc.want {
				t.Errorf("firstLine(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestStdioRPC_Await_AResponseThatArrivedWins pins the ordering the reader
// cannot promise: it delivers a response and, when the pipe closes right
// behind it, closes done, so a call's select finds both ready and would pick
// at random. A response that has arrived must win over the server exiting
// and over the caller's own cancellation. The channels are prepared by hand
// because no pipe timing reaches this state on purpose.
func TestStdioRPC_Await_AResponseThatArrivedWins(t *testing.T) {
	response := []byte(`{"jsonrpc":"2.0","id":7,"result":{"tools":[]}}`)
	cases := []struct {
		name string
		end  func(c *stdioRPC) context.Context
	}{
		{name: "the server exited", end: func(c *stdioRPC) context.Context {
			close(c.done)
			return context.Background()
		}},
		{name: "the caller cancelled", end: func(*stdioRPC) context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Both cases are ready on every iteration and select picks between
			// them at random, so one pass proves nothing: the property is that
			// the response wins every time. Without the fix this fails within
			// the first few iterations.
			for attempt := range 64 {
				c := &stdioRPC{waiting: map[int64]chan []byte{}, done: make(chan struct{})}
				waiter := make(chan []byte, 1)
				waiter <- response
				ctx := tc.end(c)

				got, err := c.await(ctx, "tools/list", 7, waiter)
				if err != nil {
					t.Fatalf("attempt %d: await = %v, want the response that had already arrived", attempt, err)
				}
				if !bytes.Equal(got, response) {
					t.Fatalf("attempt %d: await = %s, want the delivered response", attempt, got)
				}
			}
		})
	}
}

// TestDelivered_ReportsWhatIsInTheWaiter covers the non-blocking read on its
// own: a payload that is there is returned, and an empty waiter answers at
// once rather than waiting for one.
func TestDelivered_ReportsWhatIsInTheWaiter(t *testing.T) {
	waiter := make(chan []byte, 1)
	if _, ok := delivered(waiter); ok {
		t.Error("delivered reported a payload from an empty waiter")
	}
	waiter <- []byte("x")
	if got, ok := delivered(waiter); !ok || string(got) != "x" {
		t.Errorf("delivered = %q, %v, want the payload and true", got, ok)
	}
}

// TestStdioRPC_Await_NothingArrived_ReportsWhatEndedTheWait is the other
// half: with no response in the waiter, the wait ends with the reason it
// ended and the waiter is forgotten.
func TestStdioRPC_Await_NothingArrived_ReportsWhatEndedTheWait(t *testing.T) {
	c := &stdioRPC{waiting: map[int64]chan []byte{}, done: make(chan struct{})}
	waiter := make(chan []byte, 1)
	c.waiting[7] = waiter
	close(c.done)

	if _, err := c.await(context.Background(), "tools/list", 7, waiter); err == nil || !strings.Contains(err.Error(), "the server exited") {
		t.Errorf("await = %v, want the server's exit", err)
	}
	if _, still := c.waiting[7]; still {
		t.Error("await left the waiter registered after the server exited")
	}
}
