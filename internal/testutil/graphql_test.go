package testutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRespondGraphQL verifies the GraphQL response envelope helper.
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

// TestRespondGraphQLError verifies the GraphQL error response helper.
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

// TestGraphQLHandler_Routing verifies query-based handler dispatch.
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
		req := httptest.NewRequest(http.MethodPost, "/api/graphql",
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
		req := httptest.NewRequest(http.MethodPost, "/api/graphql",
			strings.NewReader(`{"query":"mutation { vulnerabilityDismiss(input: {id: \"1\"}) { vulnerability { id } } }"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if called != "dismiss" {
			t.Errorf("called = %q, want %q", called, "dismiss")
		}
	})

	t.Run("returns 400 for non-matching query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/graphql",
			strings.NewReader(`{"query":"query { unknownField { id } }"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("rejects non-POST methods", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/graphql", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

// TestParseGraphQLVariables verifies variable extraction from request body.
func TestParseGraphQLVariables(t *testing.T) {
	body := `{"query":"query($id: ID!) { vulnerability(id: $id) { title } }","variables":{"id":"gid://gitlab/Vulnerability/42","severity":"HIGH"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(body))
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
}

// TestParseGraphQLVariables_NoVariables verifies handling of requests without variables.
func TestParseGraphQLVariables_NoVariables(t *testing.T) {
	body := `{"query":"query { currentUser { username } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	vars, err := ParseGraphQLVariables(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vars) != 0 {
		t.Errorf("expected empty variables, got %v", vars)
	}
}

// TestGraphQLHandler_InvalidJSON verifies that GraphQLHandler returns 400
// when the request body is not valid JSON.
func TestGraphQLHandler_InvalidJSON(t *testing.T) {
	handler := GraphQLHandler(map[string]http.HandlerFunc{
		"test": func(w http.ResponseWriter, _ *http.Request) {
			RespondGraphQL(w, http.StatusOK, `{}`)
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/graphql",
		strings.NewReader(`not valid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for invalid JSON", w.Code, http.StatusBadRequest)
	}
}

// TestParseGraphQLVariables_InvalidJSON verifies that ParseGraphQLVariables
// returns an error when the request body is not valid JSON.
func TestParseGraphQLVariables_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/graphql",
		strings.NewReader(`not json at all`))
	req.Header.Set("Content-Type", "application/json")

	_, err := ParseGraphQLVariables(req)
	if err == nil {
		t.Fatal("expected error for invalid JSON body")
	}
}
