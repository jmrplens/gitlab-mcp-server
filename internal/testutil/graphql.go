package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
)

// graphqlRequest is the JSON body shape of a GitLab GraphQL POST request. It
// mirrors the fields consumed by the [gitlab.com/gitlab-org/api/client-go]
// GraphQL transport without depending on that package's private types.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// RespondGraphQL writes a GraphQL JSON envelope response with the given data
// payload. It wraps data in the standard {"data": ...} envelope expected by
// the GitLab GraphQL API client. For example:
//
//	testutil.RespondGraphQL(w, http.StatusOK, `{"project":{"name":"foo"}}`)
//
// produces the body {"data":{"project":{"name":"foo"}}}. Callers remain
// responsible for escaping any quotes embedded in data.
func RespondGraphQL(w http.ResponseWriter, status int, data string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"data":` + data + `}`))
}

// RespondGraphQLError writes a GraphQL error envelope with the given message.
// It sets data to null and emits a single error entry, mirroring the shape
// returned by GitLab when a query partially or fully fails. For example:
//
//	testutil.RespondGraphQLError(w, http.StatusOK, "not found")
//
// produces {"data":null,"errors":[{"message":"not found"}]}. The message is
// interpolated directly into JSON — callers must escape any embedded quotes.
func RespondGraphQLError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"` + message + `"}]}`))
}

// GraphQLHandler returns an [http.Handler] that routes GraphQL POST requests
// by matching the request's query string against handler keys. The first key
// that appears as a substring of the query wins; keys are therefore evaluated
// longest-first so specific mutation names take precedence over shorter
// operation roots (e.g. "vulnerabilityDismiss" matches before "vulnerability").
//
// The request body is parsed into a [graphqlRequest], then reattached to r so
// downstream handlers can read it again. Non-POST requests are rejected with
// 405 Method Not Allowed, malformed JSON with 400 Bad Request, and queries
// that match no key with 400 Bad Request.
//
// Keys should be GraphQL operation identifiers (field names, type names, or
// mutation names) that uniquely identify the operation, for example:
//
//	testutil.GraphQLHandler(map[string]http.HandlerFunc{
//	    "vulnerabilities":      handleListVulnerabilities,
//	    "vulnerabilityDismiss": handleDismissVulnerability,
//	})
func GraphQLHandler(handlers map[string]http.HandlerFunc) http.Handler {
	// Sort keys longest-first so the most specific key matches first,
	// avoiding non-deterministic map iteration when multiple keys match.
	keys := make([]string, 0, len(handlers))
	for k := range handlers {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

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

		for _, key := range keys {
			if strings.Contains(req.Query, key) {
				handlers[key](w, r)
				return
			}
		}

		http.Error(w, "no matching GraphQL handler for query", http.StatusBadRequest)
	})
}

// ParseGraphQLVariables reads r's body and returns the Variables map from
// the GraphQL request, or an error if the body cannot be read or is not
// valid JSON. The body is restored on r so subsequent handlers can re-read
// it. The returned map is nil when the request omits a variables field.
//
// ParseGraphQLVariables is intended for assertions inside test handlers —
// production code should rely on the client-go GraphQL transport.
func ParseGraphQLVariables(r *http.Request) (map[string]any, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	// Restore body for potential re-reads.
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	var req graphqlRequest
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return req.Variables, nil
}
