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

// The bodies GitLab's API helpers render for a permission refusal, which carry
// a message and no error code: unauthorized! with a reason (the fork
// relation's target namespace check), forbidden! bare, and a service's refusal
// handed to render_api_error!, whose message is a list (the external status
// check update).
const (
	targetNamespaceBody = `{"message":"401 Unauthorized - Target Namespace"}`
	plainForbiddenBody  = `{"message":"403 Forbidden"}`
	notAllowedListBody  = `{"message":["Not allowed"]}`
)

// The bodies the API guard answers with when it refuses the credential rather
// than the call, each carrying the RFC 6750 code rack-oauth2 renders.
const (
	insufficientScopeBody        = `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"api"}`
	insufficientGranularBody     = `{"error":"insufficient_granular_scope","error_description":"Access denied"}`
	dpopErrorBody                = `{"error":"dpop_error","error_description":"DPoP validation error"}`
	restrictedLanguageClientBody = `{"error":"restricted_language_server_client_error","error_description":"Language server client not allowed"}`
)

// accountRefusalBodies are the plain 403s the API guard answers an account the
// API will not serve with, one per reason
// lib/gitlab/auth/user_access_denied_reason.rb gives, each written the way
// forbidden! renders it, with the username and URLs an instance fills in.
var accountRefusalBodies = map[string]string{
	"an internal user":            `{"message":"403 Forbidden - This action cannot be performed by internal users"}`,
	"an account pending approval": `{"message":"403 Forbidden - Your account is pending approval from your administrator and hence blocked."}`,
	"the Terms of Service":        `{"message":"403 Forbidden - You (@alice) must accept the Terms of Service in order to perform this action. To accept these terms, please access GitLab from a web browser at https://gitlab.example.com."}`,
	"a deactivated account":       `{"message":"403 Forbidden - Your account has been deactivated by your administrator. Please log back in from a web browser to reactivate your account at https://gitlab.example.com"}`,
	"an unconfirmed email":        `{"message":"403 Forbidden - Your primary email address is not confirmed. Please check your inbox for the confirmation instructions. In case the link is expired, you can request a new confirmation email at https://gitlab.example.com/users/confirmation/new"}`,
	"a blocked account":           `{"message":"403 Forbidden - Your account has been blocked."}`,
	"an expired password":         `{"message":"403 Forbidden - Your password expired. Please access GitLab from a web browser to update your password."}`,
}

// TestRefusalMayBePermission_PlainRESTRefusal_MayBeAPermission verifies the
// answers a permission hint may follow: a REST 401 or 403 whose body carries
// no error code, whatever else it carries, including no body at all and a
// body that is not a JSON object, since neither says the credential was the
// problem. The request-free row is an error built by hand: client-go never
// answers without one.
func TestRefusalMayBePermission_PlainRESTRefusal_MayBeAPermission(t *testing.T) {
	tests := []struct {
		name   string
		status int
		url    string
		body   string
	}{
		{name: "unauthorized!", status: http.StatusUnauthorized, url: restURL, body: plainUnauthorizedBody},
		{name: "unauthorized! with a reason", status: http.StatusUnauthorized, url: restURL, body: targetNamespaceBody},
		{name: "a service refusal rendered as a list", status: http.StatusUnauthorized, url: restURL, body: notAllowedListBody},
		{name: "forbidden!", status: http.StatusForbidden, url: restURL, body: plainForbiddenBody},
		{name: "no body", status: http.StatusUnauthorized, url: restURL, body: ""},
		{name: "a body that is not JSON", status: http.StatusForbidden, url: restURL, body: "<html><body>403 Forbidden</body></html>"},
		{name: "an empty code", status: http.StatusUnauthorized, url: restURL, body: `{"error":""}`},
		{
			name:   "a REST path parameter that decodes to api/graphql",
			status: http.StatusUnauthorized,
			url:    "https://gitlab.example.com/api/v4/projects/1/repository/files/api%2Fgraphql",
			body:   plainUnauthorizedBody,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !RefusalMayBePermission(tt.status, requestTo(t, tt.url), []byte(tt.body)) {
				t.Errorf("RefusalMayBePermission(%d, %s, %q) = false, want true", tt.status, tt.url, tt.body)
			}
		})
	}
	t.Run("no request", func(t *testing.T) {
		if !RefusalMayBePermission(http.StatusUnauthorized, nil, []byte(plainUnauthorizedBody)) {
			t.Error("RefusalMayBePermission(401, nil, plain) = false, want true: the body alone decides")
		}
	})
}

// TestRefusalMayBePermission_CredentialOrOtherAnswers_AreNot is the negative
// half. Every code the API guard writes is a refusal of the credential, on
// either status, the default one it maps a missing token to included although
// nothing raises it; the guard's plain 403 about an account the API will not
// serve is one too, and is told apart by its message; the GraphQL endpoint
// answers neither status for a permission; and no other status is a refusal of
// the kind a permission hint is for, the 404 GitLab answers a resource the
// caller may not see included, since that one carries a hint of its own.
func TestRefusalMayBePermission_CredentialOrOtherAnswers_AreNot(t *testing.T) {
	tests := []struct {
		name   string
		status int
		url    string
		body   string
	}{
		{name: "an expired token", status: http.StatusUnauthorized, url: restURL, body: expiredTokenBody},
		{name: "a revoked token", status: http.StatusUnauthorized, url: restURL, body: revokedTokenBody},
		{name: "a DPoP refusal", status: http.StatusUnauthorized, url: restURL, body: dpopErrorBody},
		{name: "a restricted language server client", status: http.StatusUnauthorized, url: restURL, body: restrictedLanguageClientBody},
		{name: "the guard's default code, which nothing raises", status: http.StatusUnauthorized, url: restURL, body: `{"error":"unauthorized"}`},
		{name: "a missing scope", status: http.StatusForbidden, url: restURL, body: insufficientScopeBody},
		{name: "a missing granular scope", status: http.StatusForbidden, url: restURL, body: insufficientGranularBody},
		{name: "GraphQL 401", status: http.StatusUnauthorized, url: graphQLURL, body: graphQLInvalidTokenBody},
		{name: "GraphQL 403", status: http.StatusForbidden, url: graphQLURL, body: `{"errors":[{"message":"API not accessible for user"}]}`},
		{name: "a 404", status: http.StatusNotFound, url: restURL, body: `{"message":"404 Project Not Found"}`},
		{name: "a 400", status: http.StatusBadRequest, url: restURL, body: `{"message":"400 Bad request"}`},
		{name: "a 405", status: http.StatusMethodNotAllowed, url: restURL, body: `{"message":"405 Method Not Allowed"}`},
		{name: "a 500", status: http.StatusInternalServerError, url: restURL, body: `{"message":"500 Internal Server Error"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if RefusalMayBePermission(tt.status, requestTo(t, tt.url), []byte(tt.body)) {
				t.Errorf("RefusalMayBePermission(%d, %s, %q) = true, want false", tt.status, tt.url, tt.body)
			}
		})
	}
	for name, body := range accountRefusalBodies {
		t.Run(name, func(t *testing.T) {
			if RefusalMayBePermission(http.StatusForbidden, requestTo(t, restURL), []byte(body)) {
				t.Errorf("RefusalMayBePermission(403, %s, %q) = true, want false: the guard refused the account", restURL, body)
			}
		})
	}
}

// TestRefusalMayBePermission_CredentialVerdict_NeverAPermission holds the
// two rules of this file to the one direction they must agree in: every 401
// [UnauthorizedNamesCredential] says names the credential is one no permission
// hint may follow, so a handler's hint never contradicts the description in
// front of it.
func TestRefusalMayBePermission_CredentialVerdict_NeverAPermission(t *testing.T) {
	answers := []struct {
		name string
		url  string
		body string
	}{
		{name: "an expired token", url: restURL, body: expiredTokenBody},
		{name: "an impersonation token", url: restURL, body: impersonationDisabledTokenBody},
		{name: "GraphQL", url: graphQLURL, body: graphQLInvalidTokenBody},
		{name: "GraphQL with no body", url: graphQLURL, body: ""},
	}
	for _, tt := range answers {
		t.Run(tt.name, func(t *testing.T) {
			req := requestTo(t, tt.url)
			if !UnauthorizedNamesCredential(req, []byte(tt.body)) {
				t.Fatalf("UnauthorizedNamesCredential(%s, %q) = false, want the credential verdict this row is about", tt.url, tt.body)
			}
			if RefusalMayBePermission(http.StatusUnauthorized, req, []byte(tt.body)) {
				t.Errorf("RefusalMayBePermission(401, %s, %q) = true after the credential verdict", tt.url, tt.body)
			}
		})
	}
}
