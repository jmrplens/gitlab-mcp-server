// credential_refusal_test.go contains unit tests for the one rule that says
// whether a 401 GitLab answered names the credential.
package gitlab

import (
	"net/http"
	"testing"
)

// The bodies GitLab's API guard answers a credential it cannot use with, as
// rack-oauth2 renders them, and the one its GraphQL endpoint answers with.
const (
	revokedTokenBody               = `{"error":"invalid_token","error_description":"Token was revoked. You have to re-authorize from the user."}`
	impersonationDisabledTokenBody = `{"error":"invalid_token","error_description":"Token is an impersonation token but impersonation was disabled."}`
	graphQLInvalidTokenBody        = `{"errors":[{"message":"Invalid token"}]}`
)

// requestTo builds the request a 401 would have answered, the way client-go
// sends it: the URL parsed from its escaped form, so a %2F-encoded path
// parameter keeps its escaping.
func requestTo(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest(%q): %v", rawURL, err)
	}
	return req
}

// TestUnauthorizedNamesCredential_InvalidTokenOrGraphQL_NamesTheCredential
// verifies the two answers that say GitLab refused the credential itself: a
// REST body carrying invalid_token, which only the API guard writes, for each
// of the three causes it writes it for; and any 401 from the GraphQL endpoint,
// including one served under a relative URL root and one with no body at all,
// since the endpoint alone decides.
func TestUnauthorizedNamesCredential_InvalidTokenOrGraphQL_NamesTheCredential(t *testing.T) {
	tests := []struct {
		name string
		url  string
		body string
	}{
		{name: "an expired token", url: restURL, body: expiredTokenBody},
		{name: "a revoked token", url: restURL, body: revokedTokenBody},
		{name: "an impersonation token with impersonation disabled", url: restURL, body: impersonationDisabledTokenBody},
		{name: "GraphQL", url: graphQLURL, body: graphQLInvalidTokenBody},
		{name: "GraphQL under a relative URL root", url: "https://gitlab.example.com/gitlab/api/graphql", body: graphQLInvalidTokenBody},
		{name: "GraphQL with no body", url: graphQLURL, body: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !UnauthorizedNamesCredential(requestTo(t, tt.url), []byte(tt.body)) {
				t.Errorf("UnauthorizedNamesCredential(%s, %q) = false, want true", tt.url, tt.body)
			}
		})
	}
	t.Run("the code with no request to go on", func(t *testing.T) {
		if !UnauthorizedNamesCredential(nil, []byte(expiredTokenBody)) {
			t.Error("UnauthorizedNamesCredential(nil, invalid_token) = false, want true: the body alone decides")
		}
	})
}

// TestUnauthorizedNamesCredential_PlainOrUnrelatedAnswers_NameNothing is the
// negative half, which keeps the moved rule from widening.
//
// Grape's refusal body is what a permission refusal gets, and what a token
// GitLab has no record of gets too, so it cannot name the credential. Neither
// can the guard's default code or another code it writes, a body that is not a
// JSON object, no body, or a REST path parameter that decodes to api/graphql,
// which is not the GraphQL endpoint however its decoded path reads. The last
// rows are the guards around the request: client-go never answers without
// one, so only a caller building the pair by hand reaches them.
func TestUnauthorizedNamesCredential_PlainOrUnrelatedAnswers_NameNothing(t *testing.T) {
	tests := []struct {
		name string
		url  string
		body string
	}{
		{name: "Grape's refusal", url: restURL, body: plainUnauthorizedBody},
		{name: "no body", url: restURL, body: ""},
		{name: "a body that is not JSON", url: restURL, body: "<html><body>401 Authorization Required</body></html>"},
		{name: "a JSON array", url: restURL, body: `["invalid_token"]`},
		{name: "the guard's default code", url: restURL, body: `{"error":"unauthorized"}`},
		{name: "another API guard code", url: restURL, body: `{"error":"dpop_error","error_description":"DPoP validation error"}`},
		{
			name: "a REST path parameter that decodes to api/graphql",
			url:  "https://gitlab.example.com/api/v4/projects/1/repository/files/api%2Fgraphql",
			body: plainUnauthorizedBody,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if UnauthorizedNamesCredential(requestTo(t, tt.url), []byte(tt.body)) {
				t.Errorf("UnauthorizedNamesCredential(%s, %q) = true, want false", tt.url, tt.body)
			}
		})
	}
	t.Run("no request", func(t *testing.T) {
		if UnauthorizedNamesCredential(nil, []byte(plainUnauthorizedBody)) {
			t.Error("UnauthorizedNamesCredential(nil, plain) = true, want false")
		}
	})
	t.Run("a request with no URL", func(t *testing.T) {
		if UnauthorizedNamesCredential(&http.Request{Method: http.MethodPost}, []byte(plainUnauthorizedBody)) {
			t.Error("UnauthorizedNamesCredential(no URL, plain) = true, want false")
		}
	})
}
