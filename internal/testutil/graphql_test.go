// graphql_test.go contains unit tests for the GraphQL test helpers in
// graphql.go. Each test exercises a public helper ([RespondGraphQL],
// [RespondGraphQLError], [GraphQLHandler], [ParseGraphQLVariables]) in
// isolation using [httptest.NewRecorder] or in-memory POST requests.
package testutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRespondGraphQL verifies that [RespondGraphQL] produces a response
// whose body is wrapped in the standard {"data": ...} envelope.
//
// The test writes a payload containing a "project" object and asserts that
// the resulting body contains both the "data" envelope key and the "project"
// field. It protects the contract that MCP-tool tests rely on when
// parsing GraphQL responses.
func TestRespondGraphQL(t *testing.T) {
	w := httptest.NewRecorder()
	RespondGraphQL(w, http.StatusOK, `{"project":{"name":"test"}}`)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) {
		t.Errorf("response missing data envelope: %s", body)
	}
	if !strings.Contains(body, `"project"`) {
		t.Errorf("response missing project data: %s", body)
	}
}

// TestRespondGraphQLError verifies that [RespondGraphQLError] emits a
// {"data":null,"errors":[…]} envelope with the supplied message.
//
// The test asserts both the presence of the "errors" field and the literal
// error message in the response body. It guards the error-path shape that
// tool code consumes when checking [gl.IsGraphQLNotFound] and similar
// GitLab GraphQL error helpers.
func TestRespondGraphQLError(t *testing.T) {
	w := httptest.NewRecorder()
	RespondGraphQLError(w, http.StatusOK, "something went wrong")

	body := w.Body.String()
	if !strings.Contains(body, `"errors"`) {
		t.Errorf("response missing errors field: %s", body)
	}
	if !strings.Contains(body, "something went wrong") {
		t.Errorf("response missing error message: %s", body)
	}
}

// TestGraphQLHandler_Routing is a table-driven check that [GraphQLHandler]
// dispatches POST requests to the correct handler based on substring matches
// in the query body. Subtests cover:
//
//   - "routes to vulnerabilities handler": a query whose body contains
//     "vulnerabilities" is dispatched to the matching handler and the
//     response status is 200 OK.
//   - "routes to dismiss handler": a mutation whose body contains
//     "vulnerabilityDismiss" is dispatched to the mutation-specific
//     handler.
//   - "returns 400 for non-matching query": an unknown operation yields
//     400 Bad Request instead of a silent 200.
//   - "rejects non-POST methods": GET requests are rejected with 405
//     Method Not Allowed, matching GitLab's GraphQL endpoint behavior.
func TestGraphQLHandler_Routing(t *testing.T) {
	var called string
	handler := GraphQLHandler(map[string]http.HandlerFunc{
		"vulnerabilities": func(w http.ResponseWriter, _ *http.Request) {
			called = "vulnerabilities"
			RespondGraphQL(w, http.StatusOK, `{"project":{"vulnerabilities":{"nodes":[]}}}`)
		},
		"vulnerabilityDismiss": func(w http.ResponseWriter, _ *http.Request) {
			called = "dismiss"
			RespondGraphQL(w, http.StatusOK, `{"vulnerabilityDismiss":{"vulnerability":{"id":"1"}}}`)
		},
	})

	t.Run("routes to vulnerabilities handler", func(t *testing.T) {
		called = ""
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql",
			strings.NewReader(`{"query":"query { project(fullPath: \"test\") { vulnerabilities { nodes { id } } } }"}`))

		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if called != "vulnerabilities" {
			t.Errorf("called = %q, want %q", called, "vulnerabilities")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("routes to dismiss handler", func(t *testing.T) {
		called = ""
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql",
			strings.NewReader(`{"query":"mutation { vulnerabilityDismiss(input: {id: \"1\"}) { vulnerability { id } } }"}`))

		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if called != "dismiss" {
			t.Errorf("called = %q, want %q", called, "dismiss")
		}
	})

	t.Run("returns 400 for non-matching query", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql",
			strings.NewReader(`{"query":"query { unknownField { id } }"}`))

		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("rejects non-POST methods", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/graphql", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

// TestGraphQLHandler_LongestKeyWinsAndRestoresBody verifies two contract
// guarantees of [GraphQLHandler]: longest-key-wins dispatch and body
// restoration for downstream handlers.
//
// The test registers both "vulnerability" and "vulnerabilityDismiss" as
// keys and posts a mutation that matches both substrings. It asserts that
// the longer key wins, the dispatched handler is invoked, and that the
// handler can read the original request body verbatim. Without longest-key
// ordering, non-deterministic map iteration would flake the assertion.
func TestGraphQLHandler_LongestKeyWinsAndRestoresBody(t *testing.T) {
	body := `{"query":"mutation { vulnerabilityDismiss(input: {id: \"1\"}) { vulnerability { id } } }"}`
	var called string
	handler := GraphQLHandler(map[string]http.HandlerFunc{
		"vulnerability": func(w http.ResponseWriter, _ *http.Request) {
			called = "short"
			RespondGraphQL(w, http.StatusOK, `{}`)
		},
		"vulnerabilityDismiss": func(w http.ResponseWriter, r *http.Request) {
			called = "long"
			restored, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll(restored body) error = %v", err)
			}
			if string(restored) != body {
				t.Fatalf("restored body = %q, want %q", string(restored), body)
			}
			RespondGraphQL(w, http.StatusOK, `{"vulnerabilityDismiss":{"vulnerability":{"id":"1"}}}`)
		},
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if called != "long" {
		t.Fatalf("called = %q, want long", called)
	}
}

// graphqlFailReader is an [io.ReadCloser] whose Read method always returns an
// error. It is used to simulate a client that disconnects mid-request when
// validating the failure-handling paths of [GraphQLHandler] and
// [ParseGraphQLVariables].
type graphqlFailReader struct{}

// Read always fails with a synthetic "read failed" error. It satisfies
// [io.Reader] but never returns any bytes.
func (graphqlFailReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// Close is a no-op that always succeeds, satisfying [io.Closer].
func (graphqlFailReader) Close() error             { return nil }

// TestGraphQLHandler_ReadError verifies that [GraphQLHandler] responds with
// 400 Bad Request when the request body cannot be read.
//
// The test wires a [graphqlFailReader] as the request body and asserts the
// returned status. This protects against regressions where a transport error
// would propagate as a 500 or hang the test goroutine.
func TestGraphQLHandler_ReadError(t *testing.T) {
	handler := GraphQLHandler(map[string]http.HandlerFunc{
		"query": func(w http.ResponseWriter, _ *http.Request) { RespondGraphQL(w, http.StatusOK, `{}`) },
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql", nil)
	req.Body = graphqlFailReader{}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestParseGraphQLVariables verifies that [ParseGraphQLVariables] extracts
// the variables map from a GraphQL request body and restores the body so
// downstream reads see the original payload.
//
// The test constructs a query with id and severity variables, calls the
// helper, and asserts the values are extracted correctly. It then reads the
// restored body to confirm it equals the original byte slice — a regression
// here would break the contract used by [GraphQLHandler] and downstream
// mock handlers that need to inspect the same request.
func TestParseGraphQLVariables(t *testing.T) {
	body := `{"query":"query($id: ID!) { vulnerability(id: $id) { title } }","variables":{"id":"gid://gitlab/Vulnerability/42","severity":"HIGH"}}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	vars, err := ParseGraphQLVariables(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vars["id"] != "gid://gitlab/Vulnerability/42" {
		t.Errorf("id = %v, want gid://gitlab/Vulnerability/42", vars["id"])
	}
	if vars["severity"] != "HIGH" {
		t.Errorf("severity = %v, want HIGH", vars["severity"])
	}
	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll(restored body) error = %v", err)
	}
	if string(restored) != body {
		t.Fatalf("restored body = %q, want original body", string(restored))
	}
}

// TestParseGraphQLVariables_NoVariables verifies that
// [ParseGraphQLVariables] returns an empty map (not an error) when the
// GraphQL request omits the variables field entirely.
//
// The test uses a minimal query with no variables block and asserts that
// the returned map has length zero. This protects callers that range over
// the result without nil-checking.
func TestParseGraphQLVariables_NoVariables(t *testing.T) {
	body := `{"query":"query { currentUser { username } }"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	vars, err := ParseGraphQLVariables(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vars) != 0 {
		t.Errorf("expected empty variables, got %v", vars)
	}
}

// TestGraphQLHandler_InvalidJSON verifies that [GraphQLHandler] returns 400
// Bad Request when the request body cannot be unmarshaled as JSON.
//
// The test posts the literal string "not valid json" and asserts the
// response code. This guards the input-validation contract relied on by
// callers — without it, a malformed body would silently 200 instead of
// surfacing the parse error.
func TestGraphQLHandler_InvalidJSON(t *testing.T) {
	handler := GraphQLHandler(map[string]http.HandlerFunc{
		"test": func(w http.ResponseWriter, _ *http.Request) {
			RespondGraphQL(w, http.StatusOK, `{}`)
		},
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql",
		strings.NewReader(`not valid json`))

	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for invalid JSON", w.Code, http.StatusBadRequest)
	}
}

// TestParseGraphQLVariables_InvalidJSON verifies that
// [ParseGraphQLVariables] propagates the JSON parse error when the body is
// not valid JSON.
//
// The test feeds the literal string "not json at all" and asserts that the
// helper returns a non-nil error. Handlers should treat this as a 400-class
// condition in production code.
func TestParseGraphQLVariables_InvalidJSON(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql",
		strings.NewReader(`not json at all`))

	req.Header.Set("Content-Type", "application/json")

	_, err := ParseGraphQLVariables(req)
	if err == nil {
		t.Fatal("expected error for invalid JSON body")
	}
}

// TestParseGraphQLVariables_ReadError verifies that [ParseGraphQLVariables]
// returns the underlying error when reading the request body fails.
//
// The test wires a [graphqlFailReader] as the request body and asserts a
// non-nil error. This protects the helper against silently swallowing
// transport errors, which would otherwise hide client disconnects from the
// caller.
func TestParseGraphQLVariables_ReadError(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/graphql", nil)
	req.Body = graphqlFailReader{}

	_, err := ParseGraphQLVariables(req)
	if err == nil {
		t.Fatal("expected error for failing request body")
	}
}

// TestGraphQLHandler_NonPostMethod verifies that [GraphQLHandler] rejects
// non-POST requests with 405 Method Not Allowed, mirroring GitLab's own
// behavior on the /api/graphql endpoint.
//
// The test issues a GET request with no body and asserts the response code.
// This guards against regressions where the handler would accept arbitrary
// methods or surface an unhelpful 200.
func TestGraphQLHandler_NonPostMethod(t *testing.T) {
	handler := GraphQLHandler(map[string]http.HandlerFunc{
		"someQuery": func(w http.ResponseWriter, _ *http.Request) {
			RespondGraphQL(w, http.StatusOK, `{"data":{"ok":true}}`)
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/graphql", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
