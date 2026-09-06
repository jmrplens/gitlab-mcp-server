package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// UnattributedRequestMessage is what a caller is told when this server could
// not decide which credential a request belongs to.
//
// It says what happened and asks for a report, because nothing the caller sent
// is wrong and nothing they can change will help: on a server shared by a
// configuration shape every handler resolves the caller's client from the
// request, and a handler that resolved none has run without one. The wording
// matches the refusal the subscription path already gives for the same cause,
// so an operator meeting both sees one fact rather than two symptoms.
//
// What it deliberately does not say is "not found" or "your token lacks
// access". Both were what a caller used to see, and both send someone to check
// permissions that are perfectly fine.
const UnattributedRequestMessage = "this request could not be attributed to a credential and was not sent to GitLab; " +
	"retry, and report it if it persists"

// UnattributedRequestError is [UnattributedRequestMessage] as a JSON-RPC
// internal error, for the surfaces that answer with an error value rather than
// a classified string.
//
// Internal rather than invalid-request for the reason the message gives: the
// request was well formed and this server failed to route it.
func UnattributedRequestError() error {
	return &jsonrpc.Error{
		Code:    jsonrpc.CodeInternalError,
		Message: UnattributedRequestMessage,
	}
}

// UnattributedRequestErrorFor is [UnattributedRequestError] for a request that
// is still live, and the reason it ended for one that is not.
//
// Not every unattributed request is a wiring defect, which is what the message
// says it is. A POST the client abandoned takes its carrier with it, and the
// carrier is where the credential is read from, so the binding finds nothing
// and the handler resolves the credential-less client: a legitimate cause,
// answered with a sentence asking the caller to report a bug. The tools path
// never had this problem, because [ClassifyError] checks cancellation first and
// that check is what it hits.
//
// Consulting the context is what tells them apart. A cancelled request is over
// and nobody is reading the answer, so what matters is only that it is not
// blamed on the wiring in a log an operator does read.
//
// One cause it cannot distinguish, and does not claim to: the pool evicting the
// entry between the gate resolving it and the gate looking up its state. The
// request is alive and unattributable, and the honest thing to tell that caller
// is exactly what the message already says, since retrying rebuilds the entry.
func UnattributedRequestErrorFor(ctx context.Context) error {
	if ctx != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
	}
	return UnattributedRequestError()
}

// ToolError represents a structured error from a tool handler.
type ToolError struct {
	Tool       string `json:"tool"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}

// Error returns a human-readable representation of the tool error.
// When StatusCode is set, it is appended as "(HTTP <code>)".
func (e *ToolError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Tool, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.Tool, e.Message)
}

// WrapErr classifies the error, enriches it with a semantic message, and
// wraps it with the operation name. All tool handlers funnel through here
// so connectivity and auth problems are reported consistently.
func WrapErr(operation string, err error) error {
	if unattributed := wrapUnattributed(operation, err); unattributed != nil {
		return unattributed
	}
	semantic := ClassifyError(err)
	return fmt.Errorf("%s: %s: %w", operation, semantic, sanitize(err))
}

// wrapUnattributed returns the whole message for a request that never reached
// GitLab because no credential was bound to it, or nil when that is not the
// cause.
//
// The cause is deliberately not appended, which makes this the one wrapping in
// this file that does not end with the error it wraps. Everywhere else that
// tail is GitLab's own words and the useful half of the message; here it is
// net/http's, and it reads "Get \"https://gitlab.invalid/api/v4/...\"" — a
// synthetic host the shared catalog is registered against, for a request that
// never left this process. So the model was handed the attribution sentence
// followed by a DNS wild-goose chase, which is the exact hunt the sentence was
// written to prevent. [ClassifyError] alone could not fix that: it decides the
// sentence, not the composition.
//
// A hint goes the same way, for the same reason. It advises about GitLab state
// ("use gitlab_branch_unprotect first"), and nothing was asked of GitLab.
//
// The chain is kept, so errors.Is and errors.As behave exactly as before.
func wrapUnattributed(operation string, err error) error {
	if !errors.Is(err, gitlabclient.ErrUnboundClient) {
		return nil
	}
	return &sanitizedCauseError{
		text:  operation + ": " + UnattributedRequestMessage,
		cause: err,
	}
}

// ClassifyError inspects the error chain and returns a short, human-friendly
// diagnostic message explaining what went wrong at a high level.
func ClassifyError(err error) string {
	if err == nil {
		return "unknown error"
	}

	// The caller went away. Checked before anything else because a canceled
	// request often surfaces as a transport failure underneath — a closed
	// connection, an aborted round trip — and every classification below would
	// describe that symptom as though GitLab had done something wrong. Saying
	// "unexpected error" about a client pressing stop is both untrue and
	// unhelpful: nothing is wrong and there is nothing to check.
	if errors.Is(err, context.Canceled) {
		return "the request was canceled by the client"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "the request exceeded its deadline and was canceled"
	}

	// The request never reached GitLab, because this server could not say which
	// credential it belonged to. Checked here rather than left to the network
	// branches below, which would otherwise describe it as a host being
	// unreachable and name the synthetic host the shared catalog is registered
	// against, sending an operator to check DNS for a hostname that does not
	// exist.
	if errors.Is(err, gitlabclient.ErrUnboundClient) {
		return UnattributedRequestMessage
	}

	// GitLab API returned an HTTP error response
	var glErr *gl.ErrorResponse
	if errors.As(err, &glErr) && glErr.Response != nil {
		return ClassifyHTTPStatus(glErr.Response.StatusCode)
	}

	// Network-level errors (connection refused, DNS, timeout, TLS)
	if isConnectionRefused(err) {
		return "GitLab server is unreachable (connection refused). Check GITLAB_URL and whether the server is running"
	}
	if isDNSError(err) {
		return "GitLab server hostname could not be resolved (DNS error). Check GITLAB_URL"
	}
	if isTimeout(err) {
		return "Request to GitLab timed out. The server may be overloaded or unreachable"
	}
	if isTLSError(err) {
		return "TLS/SSL handshake failed. If using self-signed certificates, set GITLAB_MCP_SKIP_TLS_VERIFY=true"
	}

	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return fmt.Sprintf("network error reaching GitLab (%s)", urlErr.Op)
	}

	return "unexpected error"
}

// httpStatusDescriptions maps HTTP status codes to semantic descriptions.
var httpStatusDescriptions = map[int]string{
	400: "bad request: check your input parameters",
	401: "authentication failed: GITLAB_TOKEN may be invalid or expired",
	403: "access denied: your token lacks the required permissions. This can mean: (1) missing API scope on the token, (2) insufficient project role (some operations require Maintainer or Owner), or (3) the feature is restricted by instance admin settings",
	404: "not found: the requested resource does not exist, you lack access, or the feature requires a higher GitLab tier. Verify the ID/path is correct",
	405: "method not allowed: the action cannot be performed on this resource in its current state",
	409: "conflict: the resource already exists or there is a state conflict",
	422: "validation failed: GitLab rejected the request due to invalid data",
	429: "rate limited: too many requests, please wait before retrying",
	500: "GitLab internal server error: the server encountered an unexpected condition",
	502: "GitLab is temporarily unavailable (bad gateway): try again shortly",
	503: "GitLab is under maintenance or overloaded (service unavailable): try again shortly",
}

// ClassifyHTTPStatus returns a semantic description for common HTTP status codes.
func ClassifyHTTPStatus(code int) string {
	if desc, ok := httpStatusDescriptions[code]; ok {
		return desc
	}
	return fmt.Sprintf("GitLab returned HTTP %d", code)
}

// isConnectionRefused checks for ECONNREFUSED at any depth in the error chain.
func isConnectionRefused(err error) bool {
	if opErr, ok := errors.AsType[*net.OpError](err); ok {
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return true
		}
	}
	return ContainsAny(err, "connection refused", "connectex:")
}

// isDNSError checks for DNS resolution failures.
func isDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// isTimeout checks for context deadline or network timeout errors.
func isTimeout(err error) bool {
	type timeouter interface{ Timeout() bool }
	var t timeouter
	return errors.As(err, &t) && t.Timeout()
}

// isTLSError detects TLS handshake failures from error message patterns.
func isTLSError(err error) bool {
	return ContainsAny(err, "tls:", "certificate", "x509:")
}

// ContainsAny returns true if err.Error() contains any of the substrings.
func ContainsAny(err error, substrs ...string) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range substrs {
		if strings.Contains(msg, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

// DetailedError represents a rich, structured error with domain context for
// diagnostic output. It extends ToolError with additional fields useful for
// automated issue creation and Markdown error reporting.
type DetailedError struct {
	Domain       string `json:"domain"`
	Action       string `json:"action"`
	Message      string `json:"message"`
	Details      string `json:"details,omitempty"`
	GitLabStatus int    `json:"gitlab_status,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
}

// Error returns a concise representation: "domain/action: message".
func (e *DetailedError) Error() string {
	base := fmt.Sprintf("%s/%s: %s", e.Domain, e.Action, e.Message)
	if e.GitLabStatus > 0 {
		return fmt.Sprintf("%s (HTTP %d)", base, e.GitLabStatus)
	}
	return base
}

// Markdown renders the error as a Markdown block suitable for display in MCP
// tool results. Includes all available context for diagnostics.
func (e *DetailedError) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## "+EmojiCross+" Error: %s/%s\n\n", e.Domain, e.Action)
	fmt.Fprintf(&b, "**Message**: %s\n", e.Message)
	if e.GitLabStatus > 0 {
		fmt.Fprintf(&b, "**HTTP Status**: %d (%s)\n", e.GitLabStatus, ClassifyHTTPStatus(e.GitLabStatus))
	}
	if e.Details != "" {
		fmt.Fprintf(&b, "**Details**: %s\n", e.Details)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, "**Request ID**: `%s`\n", e.RequestID)
	}
	return b.String()
}

// NewDetailedError creates a DetailedError from a GitLab API error, extracting
// HTTP status and request ID when available.
func NewDetailedError(domain, action string, err error) *DetailedError {
	de := &DetailedError{
		Domain:  domain,
		Action:  action,
		Message: ClassifyError(err),
	}

	// Safely extract details — gl.ErrorResponse.Error() can panic with nil Body.
	// Sanitized for the same reason the wrappers are: Details is rendered into
	// the Markdown of an error result, which is model-facing text, so it carries
	// the request line and status plus GitLab's own message when the body
	// actually parsed as one — and never the body itself.
	func() {
		defer func() { recover() }() //nolint:errcheck // intentional panic recovery
		de.Details = sanitize(err).Error()
	}()
	if msg := ExtractGitLabMessage(err); msg != "" {
		if de.Details == "" {
			de.Details = msg
		} else {
			de.Details += " (" + msg + ")"
		}
	}

	var glErr *gl.ErrorResponse
	if errors.As(err, &glErr) && glErr.Response != nil {
		de.GitLabStatus = glErr.Response.StatusCode
		de.RequestID = EscapeMdTableCell(glErr.Response.Header.Get("X-Request-Id"))
	}

	return de
}

// ErrFieldRequired returns a validation error indicating that a required field
// is missing or empty. It produces the message "<field> is required", which is
// the standard validation pattern used across all tool handlers.
func ErrFieldRequired(field string) error {
	return fmt.Errorf("%s is required", field)
}

// ErrRequiredInt64 returns a formatted error when a required int64 field is
// missing or has its zero value. This catches silent deserialization failures
// in meta-tool dispatch, where a misnamed JSON parameter (e.g. "mr_iid"
// instead of "merge_request_iid") is silently ignored and the field defaults to 0.
func ErrRequiredInt64(operation, field string) error {
	return fmt.Errorf("%s: %s is required (must be > 0). Ensure you use the exact parameter name '%s' as documented in the tool description", operation, field, field)
}

// ErrRequiredString returns a formatted error when a required string field is
// missing or empty. Like ErrRequiredInt64, this guides LLMs to use the exact
// parameter name when silent deserialization failures occur.
func ErrRequiredString(operation, field string) error {
	return fmt.Errorf("%s: %s is required (must be non-empty). Ensure you use the exact parameter name '%s' as documented in the tool description", operation, field, field)
}

// ErrInvalidEnum returns a validation error indicating that a field value
// is not one of the allowed options. The error message lists the valid values
// to guide LLMs toward correct parameter usage.
func ErrInvalidEnum(field, value string, validValues []string) error {
	return fmt.Errorf("invalid %s %q, must be one of: %s", field, value, strings.Join(validValues, ", "))
}

// IsHTTPStatus reports whether err wraps a GitLab ErrorResponse with the
// given HTTP status code. Useful for handling specific API responses like
// 404 (feature not available on CE) or 403 (insufficient permissions).
func IsHTTPStatus(err error, code int) bool {
	if code == http.StatusNotFound && errors.Is(err, gl.ErrNotFound) {
		return true
	}
	var glErr *gl.ErrorResponse
	return errors.As(err, &glErr) && glErr.Response != nil && glErr.Response.StatusCode == code
}

// IsNotFound reports whether err represents a 404 Not Found, either via a
// structured GitLab ErrorResponse status code or via a plain-text error
// message from client-go (which may contain "404 Not Found" as text).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if IsHTTPStatus(err, http.StatusNotFound) {
		return true
	}
	return ContainsAny(err, "404 Not Found")
}

// unparsedBodyPrefix is what client-go puts in ErrorResponse.Message when the
// upstream body is not JSON it recognizes: the prefix, then the whole body.
//
// The body is whatever answered between this server and GitLab. On a pinned
// deployment that is still not necessarily GitLab — an nginx 502, a WAF block
// page, a captive portal — and those carry internal hostnames, request
// identifiers and the operator's own error detail. None of it is a message for
// a model to read, and none of it should be in a tenant's context or in the
// server's logs, so a message wearing this prefix is dropped rather than
// truncated.
const unparsedBodyPrefix = "failed to parse unknown error format:"

// gitLabErrorBodyKeys is the whole of GitLab's documented API error body: a
// "message", an "error", and the "error_description" the OAuth endpoints add
// beside it.
var gitLabErrorBodyKeys = map[string]bool{"message": true, "error": true, "error_description": true}

// gitLabAuthoredMessage returns the response message when the body it was
// parsed from is GitLab's own error shape, and the empty string otherwise.
//
// [unparsedBodyPrefix] only catches an interloper whose body is not JSON.
// An API gateway, a JSON-speaking WAF or an ingress error page answers in JSON
// too, and client-go flattens any object into Message, so a body like
// {"error":"upstream timeout","upstream":"gitlab-web-03.internal:8181"} was
// reflected as though GitLab had written it. GitLab's own error body carries
// nothing but the keys above, so a top-level key outside that set is the
// evidence that something else composed the body; the status and the
// classification still describe what happened.
//
// A response with no body at all is trusted, because client-go fills Message
// and Body together: an ErrorResponse carrying a message and no body was
// composed by something other than CheckResponse.
func gitLabAuthoredMessage(glErr *gl.ErrorResponse) string {
	if len(glErr.Body) == 0 {
		return glErr.Message
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(glErr.Body, &body); err != nil {
		return ""
	}
	named := false
	for key := range body {
		if !gitLabErrorBodyKeys[key] {
			return ""
		}
		if key != "error_description" {
			named = true
		}
	}
	if !named {
		return ""
	}
	return glErr.Message
}

// ExtractGitLabMessage extracts the specific error message from a GitLab
// ErrorResponse in the error chain. Returns empty string if not found, if the
// message only repeats the HTTP status text (e.g. "405 Method Not Allowed"), or
// if it is an unparsed upstream response body rather than a GitLab message.
//
// What survives is flattened onto one line and truncated to 300 characters.
// GitLab's messages routinely quote input an attacker chose — a branch name, a
// path, a title — so the span has to stay a span: with its newlines intact it
// could add structure to the error text a model reads.
func ExtractGitLabMessage(err error) string {
	var glErr *gl.ErrorResponse
	if !errors.As(err, &glErr) {
		return ""
	}
	msg := gitLabAuthoredMessage(glErr)
	if msg == "" || strings.HasPrefix(strings.TrimSpace(msg), unparsedBodyPrefix) {
		return ""
	}
	// Filter out messages that are just the HTTP status text — they add no information
	// beyond what ClassifyHTTPStatus already provides.
	if glErr.Response != nil {
		statusText := strconv.Itoa(glErr.Response.StatusCode)
		normalized := strings.TrimSpace(msg)
		if normalized == statusText || strings.HasPrefix(normalized, statusText+" ") {
			return ""
		}
		// Also filter wrapped status messages like "{message: 405 Method Not Allowed}"
		if strings.Contains(normalized, statusText+" ") && !strings.ContainsAny(normalized, "[]") {
			inner := normalized
			inner = strings.TrimPrefix(inner, "{message: ")
			inner = strings.TrimSuffix(inner, "}")
			inner = strings.TrimSpace(inner)
			if strings.HasPrefix(inner, statusText+" ") {
				return ""
			}
		}
	}
	return boundedGitLabMessage(msg)
}

// maxGitLabMessageLen caps how much of GitLab's own message is reflected.
const maxGitLabMessageLen = 300

// boundedGitLabMessage renders ErrorResponse.Message for a reader: an unparsed
// upstream body is dropped entirely, what remains is flattened onto one line
// and capped. It is the only path by which any part of an upstream response
// body reaches a model or a log line.
func boundedGitLabMessage(msg string) string {
	if msg == "" || strings.HasPrefix(strings.TrimSpace(msg), unparsedBodyPrefix) {
		return ""
	}
	msg = flattenErrorText(msg)
	if len(msg) > maxGitLabMessageLen {
		msg = strings.ToValidUTF8(msg[:maxGitLabMessageLen], "") + "..."
	}
	return msg
}

// flattenErrorText renders error text as a single line with no control
// characters: runs of whitespace collapse to one space, and everything else in
// the C0 and C1 ranges is dropped.
func flattenErrorText(s string) string {
	return strings.Join(strings.Fields(StripControlBytes(s)), " ")
}

// sanitizedCauseError is an error that renders a bounded, control-free summary of
// the error it wraps while leaving the original reachable through
// [errors.As] and [errors.Is].
//
// It exists because the %w verb renders as well as wraps. Every tool error in
// this package ends with the cause, and for a GitLab failure that cause is
// *gl.ErrorResponse, whose Error() appends Message — which, for a body
// client-go could not parse, is the entire body. So the raw upstream bytes
// arrived in the text a model reads and in the slog "tool call failed" line,
// uncapped, even though ExtractGitLabMessage was capping its own copy of them
// at 300 characters a few lines earlier.
type sanitizedCauseError struct {
	text  string
	cause error
}

// Error returns the sanitized rendering.
func (e *sanitizedCauseError) Error() string { return e.text }

// Unwrap returns the original error, so status checks and sentinel comparisons
// behave exactly as they did before the wrapper existed.
func (e *sanitizedCauseError) Unwrap() error { return e.cause }

// sanitize returns err with a rendering that reflects no upstream response
// body.
//
// The GitLab response's own rendering is swapped for the bounded one inside the
// text rather than replacing the text wholesale, because the handlers wrap
// before they hand the error over — "could not read .gitmodules from project
// 42: %w" — and that context is the useful half of the message. Only the
// dangerous substring changes. If the swap does not land, which would mean a
// wrapper rendered the cause some way other than %w or %v, the safe rendering
// replaces the lot rather than letting the body through.
func sanitize(err error) error {
	if err == nil {
		return nil
	}
	// Formatted through fmt rather than by calling Error() directly: fmt
	// recovers a panic inside an Error method and renders a placeholder, and
	// client-go's dereferences the request without checking it. Calling it here
	// would turn an error into a crash.
	original := fmt.Sprintf("%v", err) //nolint:perfsprint // err.Error() is exactly what must not be called here
	text := original
	if glErr, ok := errors.AsType[*gl.ErrorResponse](err); ok {
		raw, safe := renderGitLabResponse(glErr)
		if raw != safe {
			text = strings.ReplaceAll(text, raw, safe)
			if unsafe := unreflectableMessage(glErr); unsafe != "" && strings.Contains(text, unsafe) {
				text = safe
			}
		}
	}
	text = replaceUnboundRendering(text, err)
	text = flattenErrorText(text)
	if text == original {
		return err
	}
	return &sanitizedCauseError{text: text, cause: err}
}

// replaceUnboundRendering swaps the part of text that describes a request made
// through the credential-less client for the sentence that explains it.
//
// The unbound client refuses at the transport, so net/http hands back a
// *url.Error naming the synthetic host the shared catalog is registered
// against. To a model that reads as a DNS failure against a hostname that does
// not exist, for a request that never left this process.
//
// The four wrapping helpers answer this for themselves ([wrapUnattributed]) and
// forty-eight handlers do not reach them: they wrap their GitLab error with a
// plain fmt.Errorf, so their message carried the synthetic host and no
// explanation at all. Every action's error passes [SanitizeError] at its
// dispatcher, which is why the swap is made here too, and why those handlers
// need no change to stop sending anyone hunting for a DNS record.
//
// The substring is replaced rather than the whole text, for the reason
// [sanitize] gives: the handler's own context ("listing group service accounts")
// is the useful half. The fallback replaces the sentinel's own words, for a
// chain that never went through an HTTP round trip.
func replaceUnboundRendering(text string, err error) string {
	if !errors.Is(err, gitlabclient.ErrUnboundClient) {
		return text
	}
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		// Through fmt for the reason [sanitize] gives: url.Error.Error()
		// delegates to whatever it wraps, and fmt recovers a panic in there
		// where a direct call would crash the process.
		if raw := fmt.Sprintf("%v", urlErr); strings.Contains(text, raw) { //nolint:perfsprint // Error() is exactly what must not be called here
			return strings.ReplaceAll(text, raw, UnattributedRequestMessage)
		}
	}
	return strings.ReplaceAll(text, gitlabclient.ErrUnboundClient.Error(), UnattributedRequestMessage)
}

// SanitizeError returns err with a rendering that reflects no upstream response
// body, leaving the chain intact for [errors.As] and [errors.Is].
//
// The four wrapping helpers in this file already do this to what they wrap, but
// a handler is free to wrap a client-go error itself with %w, and a good many
// do, so for those actions nothing between the handler and the SDK ever bounded
// the body. Calling this at the dispatchers, where every action's error passes
// on its way to the model and to the "tool call failed" log line, makes the
// wrapping helpers defense in depth rather than the only gate.
func SanitizeError(err error) error { return sanitize(err) }

// renderGitLabResponse returns client-go's own rendering of a GitLab error
// response and the bounded rendering that replaces it. The first is recovered
// rather than trusted: ErrorResponse.Error() dereferences the request without
// checking it.
func renderGitLabResponse(glErr *gl.ErrorResponse) (raw, safe string) {
	safe = describeGitLabResponse(glErr)
	func() {
		defer func() { recover() }() //nolint:errcheck // intentional panic recovery
		raw = glErr.Error()
	}()
	if raw == "" {
		raw = safe
	}
	return raw, safe
}

// unreflectableMessage returns the response message when no part of it may be
// reflected verbatim: an unparsed upstream body, a body GitLab did not compose,
// or one long enough that [boundedGitLabMessage] would have to cut it.
func unreflectableMessage(glErr *gl.ErrorResponse) string {
	msg := strings.TrimSpace(glErr.Message)
	if msg == "" {
		return ""
	}
	if gitLabAuthoredMessage(glErr) == "" {
		return msg
	}
	if boundedGitLabMessage(msg) == flattenErrorText(msg) {
		return ""
	}
	return msg
}

// describeGitLabResponse renders a GitLab error response the way client-go's
// own Error() does — request line, status, message — with the message put
// through [gitLabAuthoredMessage] and [boundedGitLabMessage] first. That is the
// whole difference: a body GitLab did not compose is dropped instead of being
// pasted in full, and a message it did is flattened and capped.
func describeGitLabResponse(glErr *gl.ErrorResponse) string {
	status := glErr.StatusCode
	if glErr.Response != nil {
		status = glErr.Response.StatusCode
	}
	msg := boundedGitLabMessage(gitLabAuthoredMessage(glErr))
	if glErr.Response == nil || glErr.Response.Request == nil || glErr.Response.Request.URL == nil {
		if msg == "" {
			return fmt.Sprintf("HTTP %d", status)
		}
		return fmt.Sprintf("%d %s", status, msg)
	}
	req := glErr.Response.Request
	path := req.URL.RawPath
	if path == "" {
		path = req.URL.Path
	}
	line := fmt.Sprintf("%s %s://%s%s: %d", req.Method, req.URL.Scheme, req.URL.Host, path, status)
	if msg == "" {
		return line
	}
	return line + " " + msg
}

// WrapErrWithMessage works like WrapErr but also includes the specific GitLab
// error message (from ErrorResponse.Message) when available. This produces
// richer errors like:
//
//	"fileCreate: bad request (A file with this name already exists): POST .../files: 400"
//
// Use WrapErrWithMessage for mutating operations where the specific GitLab
// error detail helps the LLM understand what went wrong. Use WrapErr for
// read-only operations where the generic classification suffices.
func WrapErrWithMessage(operation string, err error) error {
	if unattributed := wrapUnattributed(operation, err); unattributed != nil {
		return unattributed
	}
	semantic := ClassifyError(err)
	glMsg := ExtractGitLabMessage(err)
	if glMsg != "" {
		return fmt.Errorf("%s: %s (%s): %w", operation, semantic, glMsg, sanitize(err))
	}
	return fmt.Errorf("%s: %s: %w", operation, semantic, sanitize(err))
}

// WrapErrWithHint works like WrapErrWithMessage but appends an actionable hint
// that tells the LLM what to do next. Example:
//
//	"branchDelete: bad request (Cannot delete: protected branch).
//	 Suggestion: use gitlab_branch_unprotect first, then retry deletion: <original>"
//
// The hint should be a concise suggestion starting with a verb (e.g., "use
// gitlab_branch_list to verify the branch name").
func WrapErrWithHint(operation string, err error, hint string) error {
	if unattributed := wrapUnattributed(operation, err); unattributed != nil {
		return unattributed
	}
	semantic := ClassifyError(err)
	glMsg := ExtractGitLabMessage(err)
	if glMsg != "" {
		return fmt.Errorf("%s: %s (%s). Suggestion: %s: %w", operation, semantic, glMsg, hint, sanitize(err))
	}
	return fmt.Errorf("%s: %s. Suggestion: %s: %w", operation, semantic, hint, sanitize(err))
}

// WrapErrWithStatusHint returns WrapErrWithHint(operation, err, hint) when err
// matches the given HTTP status code, otherwise falls back to
// WrapErrWithMessage(operation, err). It compresses the common pattern:
//
//	if toolutil.IsHTTPStatus(err, 404) {
//	    return ..., toolutil.WrapErrWithHint(op, err, hint)
//	}
//	return ..., toolutil.WrapErrWithMessage(op, err)
//
// into a single call. For handlers that need different hints per status, use
// a switch over IsHTTPStatus checks; this helper covers the dominant single-
// status case.
func WrapErrWithStatusHint(operation string, err error, code int, hint string) error {
	if IsHTTPStatus(err, code) {
		return WrapErrWithHint(operation, err, hint)
	}
	return WrapErrWithMessage(operation, err)
}

// ErrorResultMarkdown creates an MCP tool error result with Markdown formatting.
// The result has IsError = true for MCP clients that distinguish error results.
func ErrorResultMarkdown(domain, action string, err error) *mcp.CallToolResult {
	de := NewDetailedError(domain, action, err)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: de.Markdown()},
		},
		IsError: true,
	}
}
