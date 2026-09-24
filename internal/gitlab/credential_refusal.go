package gitlab

import (
	"encoding/json"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// UnauthorizedAnswer is what a 401 GitLab answered says about the credential
// it refused.
//
// GitLab answers 401 for two different things, and they must not be acted on
// alike. Its API guard answers it for a credential it cannot use, and at a
// family of REST routes Grape's unauthorized! answers it for a valid credential
// that lacks a permission (entry 55 of docs/development/upstream-bugs.md). The
// first is a reason to stop serving the credential; the second is an ordinary
// refusal of one call, and ending the caller's subscriptions over it tells them
// to re-authenticate a token that works.
type UnauthorizedAnswer uint8

const (
	// UnauthorizedUnexplained is a 401 that names no cause. GitLab answers a
	// permission refusal and a token it no longer finds with the same bytes,
	// so the answer alone cannot say which it was, and whoever acts on it has
	// to ask again. It is the zero value on purpose: an answer nobody
	// classified is read the way that asks rather than the way that acts.
	UnauthorizedUnexplained UnauthorizedAnswer = iota
	// UnauthorizedCredential is a 401 GitLab said was about the credential
	// itself, which [UnauthorizedNamesCredential] decides.
	UnauthorizedCredential
)

// invalidTokenCode is the RFC 6750 error code GitLab's REST API guard writes
// into a 401 about the credential itself.
const invalidTokenCode = "invalid_token"

// UnauthorizedNamesCredential reports whether a 401 GitLab answered says the
// credential itself was refused, as opposed to a valid credential refused a
// permission. req is the request GitLab answered and body the answer's body,
// either of which may be nil. The caller has already established that the
// status is 401: neither signal means anything on another status.
//
// Two answers say so:
//
//   - A REST body carrying the RFC 6750 code invalid_token. GitLab's API guard
//     answers an expired, revoked or impersonation-disabled token with
//     rack-oauth2's body carrying that code (lib/api/api_guard.rb, pinned by
//     spec/requests/api/api_guard_spec.rb), and nothing else in the REST API
//     writes it. Grape's unauthorized!, which the permission refusals call,
//     renders {"message":"401 Unauthorized"} instead.
//   - Any 401 from the GraphQL endpoint. It answers 401 only from its
//     authentication checks (GraphqlController#authorize_access_api! renders
//     {"errors":[{"message":"Invalid token"}]} for a token it could not use,
//     and the two authentication errors the controller rescues are the
//     others), and it refuses a field the caller may not see with a 200
//     carrying errors, never with a 401. So the permission refusal a REST 401
//     can be has no GraphQL counterpart to be confused with.
//
// The absence of both decides nothing. A token GitLab cannot find at all is
// answered through unauthorized! too, byte for byte like a permission refusal
// (auth_finders.rb raises UnauthorizedError, which the API helpers turn into
// unauthorized!), so a plain 401 may be either.
//
// It is one rule with two readers, and they must not disagree. The transport
// reads it to decide whether a refused call ends a pooled credential at once or
// is confirmed first, and internal/toolutil reads it to decide what a 401 is
// described as to a model. A copy in either would let the server end a
// credential while telling the model a permission was missing, or the reverse.
//
// It is not [IsCredentialRejection], whose rule is the opposite and is right
// where it is used: that judges the answer to GET /version, a route with no
// permission to refuse, where any 401 or 403 is about the credential; the
// credential probe's GET /user is read by the same status-only rule in
// [credentialVerdictFor].
func UnauthorizedNamesCredential(req *http.Request, body []byte) bool {
	return answeredByGraphQL(req) || carriesInvalidToken(body)
}

// RefusalMayBePermission reports whether a refusal GitLab answered can be a
// valid credential refused a permission, which is the one refusal a handler's
// hint about a role, a license or an owner is written for. status is the
// answered status, req the request GitLab answered and body the answer's body,
// either of which may be nil.
//
// Only a REST 401 or 403 whose body carries no RFC 6750 error code, and whose
// message does not refuse the account, qualifies. GitLab's API helpers
// unauthorized!, forbidden! and render_api_error!, which every permission
// refusal goes through (the 401 ones are entry 55 of
// docs/development/upstream-bugs.md), render {"message": ...} and nothing
// else. GitLab's API guard refuses the credential rather than the call in two
// ways, and neither qualifies. Its token and scope refusals carry a code:
// invalid_token, dpop_error and restricted_language_server_client_error on a
// 401, insufficient_scope and insufficient_granular_scope on a 403
// (lib/api/api_guard.rb). An account the API will not serve (blocked,
// deactivated, pending approval, an internal user, the Terms of Service not
// accepted, the primary email unconfirmed, the password expired) it refuses
// through those same helpers, with a plain 403 forbidden! whose reason is one
// of the fixed sentences of lib/gitlab/auth/user_access_denied_reason.rb, so
// that answer is told apart by its message, [accountRefusalReasons]. A role or
// license hint after either sends the reader to fix something that is not
// wrong. The GraphQL endpoint answers 401 and 403 only from its own
// authentication and access checks, and refuses a field the caller may not
// see with a 200, so none of its answers qualifies.
//
// A request carrying no token at all does qualify, and it is harmless here.
// The guard maps the error for a missing token to rack-oauth2's default code
// unauthorized, but nothing in GitLab raises that error: such a request is
// answered by authenticate!'s plain 401, byte for byte a permission refusal,
// and this server never sends a request without a token.
//
// The answer is "may" and not "is": a token GitLab has no record of is
// answered through unauthorized! with the same bytes as a permission refusal,
// which is why the description in front of a hint names both causes. What it
// guarantees is the other direction. Every 401 [UnauthorizedNamesCredential]
// says names the credential carries a code or came from GraphQL, so a hint
// keyed on this never follows the verdict that the token itself was refused.
func RefusalMayBePermission(status int, req *http.Request, body []byte) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	return !answeredByGraphQL(req) && errorCode(body) == "" && !refusesAccount(body)
}

// accountRefusalReasons are the fixed parts of the reasons GitLab's API guard
// gives when it refuses an account the API will not serve, one per rejection
// type of lib/gitlab/auth/user_access_denied_reason.rb, which has no EE
// extension. Each is a part no instance changes: the sentences around some of
// them carry the username or the instance's own URL. The reason for an
// unknown rejection is left out because it cannot be given: the rejection
// type falls back to blocked.
var accountRefusalReasons = []string{
	"cannot be performed by internal users",
	"pending approval from your administrator",
	"must accept the Terms of Service",
	"Your account has been deactivated",
	"primary email address is not confirmed",
	"Your account has been blocked",
	"Your password expired",
}

// refusesAccount reports whether body is a JSON object whose message is one of
// the API guard's refusals of the account itself, [accountRefusalReasons]. A
// message that is not a string, as a service's refusal rendered as a list is,
// is never one: the guard renders its reason into the string forbidden!
// builds.
func refusesAccount(body []byte) bool {
	var answer struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return false
	}
	for _, reason := range accountRefusalReasons {
		if strings.Contains(answer.Message, reason) {
			return true
		}
	}
	return false
}

// answeredByGraphQL reports whether req was sent to GitLab's GraphQL endpoint.
//
// The suffix rather than the whole path is compared because an instance served
// under a relative URL root puts its own prefix in front. The path compared is
// the one sent, escaped, and not the decoded Path: client-go escapes a path
// parameter, so a REST read of the repository file api/graphql goes out as
// .../files/api%2Fgraphql, and its decoded Path ends in /api/graphql like the
// GraphQL endpoint's does. Escaped, a %2F-encoded parameter never matches,
// while the GraphQL request, whose Path client-go rewrites without an escaped
// form to disagree with, still does.
//
// A request that is missing, or carries no URL, names no endpoint and is not
// taken for the GraphQL one. client-go never answers a 401 without one; the
// guard is for an error built by hand.
func answeredByGraphQL(req *http.Request) bool {
	return req != nil && req.URL != nil && strings.HasSuffix(req.URL.EscapedPath(), gl.GraphQLAPIEndpoint)
}

// carriesInvalidToken reports whether body is a JSON object whose error code
// is invalid_token.
func carriesInvalidToken(body []byte) bool {
	return errorCode(body) == invalidTokenCode
}

// errorCode returns the RFC 6750 error code body carries: the string member
// error of a JSON object, and the empty string for a body that is not one or
// carries none.
func errorCode(body []byte) string {
	var answer struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return ""
	}
	return answer.Error
}
