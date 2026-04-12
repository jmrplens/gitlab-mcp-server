package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// graphqlRequest represents the JSON body of a GraphQL POST request.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// RespondGraphQL writes a GraphQL JSON envelope response with the given
// data payload. It wraps the data in {"data": ...} as expected by the
// GitLab GraphQL API client.
//
//	testutil.RespondGraphQL(w, http.StatusOK, `{"project":{"name":"foo"}}`)
//
// produces: {"data":{"project":{"name":"foo"}}}
func RespondGraphQL(w http.ResponseWriter, status int, data string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"data":` + data + `}`))
}

// RespondGraphQLError writes a GraphQL error response with the given message.
//
//	testutil.RespondGraphQLError(w, http.StatusOK, "not found")
//
// produces: {"data":null,"errors":[{"message":"not found"}]}
func RespondGraphQLError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"` + message + `"}]}`))
}

// GraphQLHandler creates an http.Handler that routes GraphQL POST requests
// by matching the query body against handler keys. It reads the request body,
// checks if the query contains each key string, and dispatches to the first
// matching handler.
//
// Keys should be GraphQL operation identifiers (type names, field names, or
// mutation names) that uniquely identify the query. For example:
//
//	testutil.GraphQLHandler(map[string]http.HandlerFunc{
//	    "vulnerabilities":    handleListVulnerabilities,
//	    "vulnerabilityDismiss": handleDismissVulnerability,
//	})
//
// If no handler matches, it responds with 400 Bad Request.
func GraphQLHandler(handlers map[string]http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "GraphQL requires POST", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req graphqlRequest
		if err = json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid GraphQL JSON", http.StatusBadRequest)
			return
		}

		// Restore body for handlers that may need to re-read it.
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		for key, handler := range handlers {
			if strings.Contains(req.Query, key) {
				handler(w, r)
				return
			}
		}

		http.Error(w, "no matching GraphQL handler for query", http.StatusBadRequest)
	})
}

// ParseGraphQLVariables reads the request body and returns the Variables
// map from the GraphQL request. Useful for asserting input parameters
// in test handlers.
func ParseGraphQLVariables(r *http.Request) (map[string]any, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	// Restore body for potential re-reads.
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	var req graphqlRequest
	if err = json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return req.Variables, nil
}
