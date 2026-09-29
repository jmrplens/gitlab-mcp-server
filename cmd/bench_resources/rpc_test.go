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
	"fmt"
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
		// JSON-RPC 2.0 puts exactly one of result and error in every response,
		// so an envelope carrying neither answered nothing and its timing
		// measures nothing. Every method this client calls carries an id and
		// is a request, so there is no answer without a body to protect.
		{name: "neither result nor error", payload: `{"jsonrpc":"2.0","id":1}`, wantErr: true},
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

// TestCheckResponse_KeepsTheTextARefusalIsIdentifiedBy verifies a failed tool
// result carries its message forward.
//
// On this wire a refused tools/call is a successful HTTP 200 response with a
// well formed result and no code anywhere: its text is the only thing that
// separates "the bound said no" from "the tool broke", and the fairness
// scenario cannot tell them apart without it. The second decode happens only
// on this path, because a successful result on the individual surface is
// megabytes of content and decoding all of it into strings on every call would
// put the driver's own cost into every latency this command publishes.
func TestCheckResponse_KeepsTheTextARefusalIsIdentifiedBy(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text",` +
		`"text":"rate limit exceeded for gitlab_find_action; retry after a short backoff"}]}}`
	err := checkResponse([]byte(payload))
	var toolErr *toolResultError
	if !errors.As(err, &toolErr) {
		t.Fatalf("checkResponse = %v, want a typed tool-result failure", err)
	}
	if !strings.HasPrefix(toolErr.Text, "rate limit exceeded for ") {
		t.Errorf("text = %q, want the message the server sent", toolErr.Text)
	}
	if !strings.Contains(toolErr.Error(), "rate limit exceeded") {
		t.Errorf("Error = %q, want it to name the text", toolErr.Error())
	}
	if got := (&toolResultError{}).Error(); got != "the tool returned an error result" {
		t.Errorf("Error with no text = %q, want the plain sentence", got)
	}
}

// TestFirstResultText_AnswersEmptyForAPayloadWithNothingToRead verifies the
// second decode is safe on every shape a failed result can arrive in.
func TestFirstResultText_AnswersEmptyForAPayloadWithNothingToRead(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "a payload that is not JSON", payload: `<html>`, want: ""},
		{name: "a result with no content", payload: `{"result":{"isError":true}}`, want: ""},
		{name: "content with no text in it", payload: `{"result":{"content":[{"type":"image"}]}}`, want: ""},
		{name: "the first text block", payload: `{"result":{"content":[{"text":""},{"text":"second"}]}}`, want: "second"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstResultText([]byte(tc.payload)); got != tc.want {
				t.Errorf("firstResultText = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHTTPStatusError_KeepsTheStatusAndReadsAsItAlwaysDid verifies a refused
// response carries its status without changing the message anything
// downstream prints.
//
// The status is what separates the per-credential rate limit from the
// per-address authentication lockout: both answer JSON-RPC -42900, one at HTTP
// 200 and one at 429, and counting the lockout as the bound's refusal would
// turn a throttled run into an apparent fairness result.
func TestHTTPStatusError_KeepsTheStatusAndReadsAsItAlwaysDid(t *testing.T) {
	err := error(&httpStatusError{Method: "tools/list", Status: 429, Snippet: "too many"})
	if got, want := err.Error(), "tools/list: HTTP 429: too many"; got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
	if got := responseStatus(fmt.Errorf("wrapped: %w", err)); got != 429 {
		t.Errorf("responseStatus = %d, want the status through the wrapping", got)
	}
	if got := responseStatus(errors.New("no status here")); got != httpOK {
		t.Errorf("responseStatus = %d, want a response the transport accepted to read as 200", got)
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

// TestHTTPRPC_NameHeader_ComesFromTheFieldTheMethodNamesIt verifies Mcp-Name
// is derived from the request body for each of the three methods the
// specification sources it for, and sent for nothing else.
//
// Mcp-Name does not mean "the tool this call runs": SEP-2243 requires it for
// tools/call, prompts/get and resources/read alike, taking params.name for the
// first two and params.uri for the third, so a gateway can route without
// parsing a body. Omitting it on a prompts/get is what a server validating its
// headers refuses, which is the opposite of what this test used to assert.
func TestHTTPRPC_NameHeader_ComesFromTheFieldTheMethodNamesIt(t *testing.T) {
	tests := []struct {
		name   string
		method string
		params map[string]any
		want   string
	}{
		{
			name:   "a tools/call names its tool",
			method: methodToolsCall,
			params: map[string]any{"name": "gitlab_issue"},
			want:   "gitlab_issue",
		},
		{
			name:   "a prompts/get names its prompt",
			method: methodPromptsGet,
			params: map[string]any{"name": "review_mr"},
			want:   "review_mr",
		},
		{
			name:   "a resources/read names its uri",
			method: methodResourcesRead,
			params: map[string]any{"uri": "gitlab://tools"},
			want:   "gitlab://tools",
		},
		{
			name:   "a listing names nothing, whatever it carries",
			method: methodResourcesList,
			params: map[string]any{"name": "not a name of this call"},
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.Header().Set(headerContentType, mediaJSON)
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
			}))
			defer server.Close()

			client := newHTTPRPC(server.URL, "token")
			defer client.close()

			if _, err := client.call(context.Background(), tc.method, tc.params); err != nil {
				t.Fatalf("call: %v", err)
			}
			if name := got.Get("Mcp-Name"); name != tc.want {
				t.Errorf("Mcp-Name = %q on a %s, want %q", name, tc.method, tc.want)
			}
			if method := got.Get("Mcp-Method"); method != tc.method {
				t.Errorf("Mcp-Method = %q, want %q", method, tc.method)
			}
		})
	}
}

// TestHTTPRPC_ParamHeader_MirrorsTheExecuteAction verifies the one parameter
// header this harness sends: gitlab_execute_action's action, which the tool's
// schema marks for mirroring and the 2026-07-28 transport refuses a call
// without. A held run calls that tool and nothing else did, so every call it
// made was refused with a 400 until the header was sent.
//
// The header is sent only where the value exists: a call that names no action,
// or names it as something other than a string, sends none, so the server
// refuses it the way it would refuse a real client that left it out.
func TestHTTPRPC_ParamHeader_MirrorsTheExecuteAction(t *testing.T) {
	tests := []struct {
		name   string
		method string
		params map[string]any
		want   string
	}{
		{
			name:   "an execute call carries its action",
			method: methodToolsCall,
			params: map[string]any{"name": executeTool, "arguments": map[string]any{"action": "project.get"}},
			want:   "project.get",
		},
		{
			name:   "another tool carries nothing",
			method: methodToolsCall,
			params: map[string]any{"name": "gitlab_find_action", "arguments": map[string]any{"action": "project.get"}},
		},
		{
			name:   "an execute call with no action carries nothing",
			method: methodToolsCall,
			params: map[string]any{"name": executeTool, "arguments": map[string]any{}},
		},
		{
			name:   "an execute call with no arguments carries nothing",
			method: methodToolsCall,
			params: map[string]any{"name": executeTool},
		},
		{
			name:   "an action that is not a string carries nothing",
			method: methodToolsCall,
			params: map[string]any{"name": executeTool, "arguments": map[string]any{"action": 7}},
		},
		{
			name:   "another method naming the tool carries nothing",
			method: methodPromptsGet,
			params: map[string]any{"name": executeTool, "arguments": map[string]any{"action": "project.get"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.Header().Set(headerContentType, mediaJSON)
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
			}))
			defer server.Close()

			client := newHTTPRPC(server.URL, "token")
			defer client.close()

			if _, err := client.call(context.Background(), tc.method, tc.params); err != nil {
				t.Fatalf("call: %v", err)
			}
			values, sent := got[http.CanonicalHeaderKey(executeActionHeader)]
			if tc.want == "" {
				if sent {
					t.Errorf("%s = %q, want no header", executeActionHeader, values)
				}
				return
			}
			if got.Get(executeActionHeader) != tc.want {
				t.Errorf("%s = %q, want %q", executeActionHeader, got.Get(executeActionHeader), tc.want)
			}
		})
	}
}

// TestHTTPRPC_AcceptedResponse_IsNotAnAnswer verifies a 202 is a failed
// measurement even when something that parses arrives alongside it.
//
// The streamable transport answers 202, with no body, to a notification or a
// response. Every call this client makes carries an id and is a request, which
// a server answers with 200 and a body, so a 202 here means the server did not
// answer the thing that was timed. Taking whatever came with it as the answer
// would put that timing into a published percentile, which is the failure this
// client exists to avoid rather than one to be lenient about.
func TestHTTPRPC_AcceptedResponse_IsNotAnAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, mediaJSON)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"resources":[]}}`)
	}))
	defer server.Close()

	client := newHTTPRPC(server.URL, "token")
	defer client.close()

	payload, err := client.call(context.Background(), methodResourcesList, nil)
	if err == nil {
		t.Fatalf("call() = %q with no error, want a 202 to a request reported as a failure", payload)
	}
	var status *httpStatusError
	if !errors.As(err, &status) || status.Status != http.StatusAccepted {
		t.Errorf("call() error = %v, want an httpStatusError naming 202", err)
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

// TestStdioRPC_ResponseNobodyWaitsFor_IsDropped verifies a response carrying an
// id no caller is waiting for is discarded, and the caller that does have a
// request in flight still gets its own answer.
//
// A server that answers a request the client gave up on, or repeats one it has
// already answered, puts a line on the pipe with nobody behind it. Handing it
// on is not a possibility here, it is a send to a channel that does not exist:
// the reader would block forever on it and every later response, including the
// one this caller is waiting for, would stay unread behind it.
func TestStdioRPC_ResponseNobodyWaitsFor_IsDropped(t *testing.T) {
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	defer func() { _ = toServer.Close() }()

	go func() {
		// Sent before any call, so nothing is registered under this id and
		// nothing ever will be: the client numbers its own requests from one.
		_, _ = io.WriteString(fromServer, `{"jsonrpc":"2.0","id":9999,"result":{"stray":true}}`+"\n")
		var request struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(toServer).Decode(&request); err != nil {
			return
		}
		_, _ = io.WriteString(fromServer, `{"jsonrpc":"2.0","id":`+itoa(request.ID)+`,"result":{"stray":false}}`+"\n")
	}()

	client := newStdioRPC(fromClient, toClient)
	defer client.close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	payload, err := client.call(ctx, methodToolsList, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var decoded struct {
		ID     int64 `json:"id"`
		Result struct {
			Stray bool `json:"stray"`
		} `json:"result"`
	}
	if unmarshalErr := json.Unmarshal(payload, &decoded); unmarshalErr != nil {
		t.Fatalf("decoding the response: %v", unmarshalErr)
	}
	if decoded.Result.Stray || decoded.ID == 9999 {
		t.Errorf("the caller was handed the unmatched response %s, want its own answer", payload)
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

// TestEventStreamPayload_ALargeMessage_IsReadWhole verifies a data line far
// longer than the scanner's first buffer is read whole.
//
// A tools/list on the individual surface is megabytes on one line, which is
// exactly the response the benchmark times, so the ceiling above has to be a
// ceiling on a runaway body and never on the answer being measured.
func TestEventStreamPayload_ALargeMessage_IsReadWhole(t *testing.T) {
	message := `{"jsonrpc":"2.0","id":1,"result":{"blob":"` + strings.Repeat("x", 1<<20) + `"}}`
	got, err := eventStreamPayload([]byte("event: message\ndata: " + message + "\n\n"))
	if err != nil {
		t.Fatalf("eventStreamPayload over a one-megabyte message: %v", err)
	}
	if string(got) != message {
		t.Errorf("payload is %d bytes, want the %d-byte message whole", len(got), len(message))
	}
}

// TestStdioRPC_ALargeResponse_IsReadWhole verifies a response line far longer
// than the reader's first buffer reaches its caller whole, as the megabytes a
// tools/list writes on one line have to.
func TestStdioRPC_ALargeResponse_IsReadWhole(t *testing.T) {
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	defer func() { _ = toServer.Close() }()

	blob := strings.Repeat("y", 1<<20)
	go func() {
		var request struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(toServer).Decode(&request); err != nil {
			return
		}
		_, _ = io.WriteString(fromServer, `{"jsonrpc":"2.0","id":`+itoa(request.ID)+`,"result":{"blob":"`+blob+`"}}`+"\n")
	}()

	client := newStdioRPC(fromClient, toClient)
	defer client.close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	got, err := client.call(ctx, methodToolsList, nil)
	if err != nil {
		t.Fatalf("call over a one-megabyte response: %v", err)
	}
	if !strings.Contains(string(got), blob) {
		t.Errorf("the response is %d bytes, want the megabyte result whole", len(got))
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

// recordingServer answers every POST with an empty result and keeps the
// headers and the peer address of the last one.
type recordingServer struct {
	*httptest.Server
	mu     sync.Mutex
	header http.Header
	peer   string
}

func newRecordingServer(t *testing.T) *recordingServer {
	t.Helper()
	s := &recordingServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.header, s.peer = r.Header.Clone(), r.RemoteAddr
		s.mu.Unlock()
		w.Header().Set(headerContentType, mediaJSON)
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	}))
	t.Cleanup(s.Close)
	return s
}

// last is the headers and the peer of the last request.
func (s *recordingServer) last() (http.Header, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.header, s.peer
}

// TestHTTPRPC_CallAs_PresentsTheCredentialItIsGiven verifies a request carries
// the credential it was handed rather than the client's own, as a bearer token
// to a server in OAuth mode and with the forwarded address it names, and that
// an ordinary call goes out exactly as it did before.
func TestHTTPRPC_CallAs_PresentsTheCredentialItIsGiven(t *testing.T) {
	server := newRecordingServer(t)
	client := newHTTPRPC(server.URL, "bench-token-0")
	defer client.close()

	if _, err := client.call(t.Context(), methodToolsList, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	header, _ := server.last()
	if header.Get("PRIVATE-TOKEN") != "bench-token-0" || header.Get("Authorization") != "" || header.Get(headerForwardedFor) != "" {
		t.Errorf("headers = %v, want the client's own personal access token and nothing else", header)
	}

	client.bearer = true
	if _, err := client.callAs(t.Context(), methodToolsList, nil, credential{token: "fresh", forwardedFor: "10.0.0.7"}); err != nil {
		t.Fatalf("callAs: %v", err)
	}
	header, _ = server.last()
	if header.Get("Authorization") != "Bearer fresh" || header.Get("PRIVATE-TOKEN") != "" || header.Get(headerForwardedFor) != "10.0.0.7" {
		t.Errorf("headers = %v, want the credential given, as a bearer, from the address it names", header)
	}
}

// TestNewSourcedHTTPRPC_LeavesFromItsSource verifies a sourced client's
// requests leave from the local address it names, which is the transport
// source the server charges, and present their credential as a bearer when
// asked to.
func TestNewSourcedHTTPRPC_LeavesFromItsSource(t *testing.T) {
	server := newRecordingServer(t)
	client := newSourcedHTTPRPC(server.URL, "127.0.0.1", true)
	defer client.close()
	if _, err := client.callAs(t.Context(), methodToolsList, nil, credential{token: "x"}); err != nil {
		t.Fatalf("callAs: %v", err)
	}
	header, peer := server.last()
	if !strings.HasPrefix(peer, "127.0.0.1:") || header.Get("Authorization") != "Bearer x" {
		t.Errorf("peer %q and headers %v, want a bearer request from 127.0.0.1", peer, header)
	}
}

// TestStatusError_KeepsTheJSONRPCErrorItCarried verifies a refused response
// keeps the code and words its body carried, which is what tells the
// verification ceiling's 503 apart from any other 503 in front of the server,
// and that a body carrying none leaves nothing to unwrap.
func TestStatusError_KeepsTheJSONRPCErrorItCarried(t *testing.T) {
	carried := statusError(methodToolsList, 503, []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-50300,"message":"busy"}}`))
	var rpc rpcError
	if !errors.As(error(carried), &rpc) || rpc.Code != -50300 || rpc.Message != "busy" {
		t.Errorf("statusError = %+v, want the JSON-RPC error reachable through it", carried)
	}
	if carried.Status != 503 || carried.Snippet == "" {
		t.Errorf("statusError = %+v, want the status and the snippet kept", carried)
	}
	for name, body := range map[string]string{
		"a body that is not JSON":       "service unavailable",
		"a JSON body carrying no error": `{"message":"down"}`,
	} {
		t.Run(name, func(t *testing.T) {
			bare := statusError(methodToolsList, 503, []byte(body))
			if bare.RPC != nil || bare.Unwrap() != nil {
				t.Errorf("statusError = %+v, want nothing to unwrap", bare)
			}
		})
	}
}
