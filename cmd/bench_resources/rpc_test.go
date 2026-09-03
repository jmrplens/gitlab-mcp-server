// rpc_test.go covers the JSON-RPC client the measurements are taken with: the
// request shape protocol 2026-07-28 requires, the two response framings the
// server uses, and the demultiplexing that lets parallel requests share one
// stdio pipe.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
		if got.Get(header) != value {
			t.Errorf("%s = %q, want %q", header, got.Get(header), value)
		}
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
