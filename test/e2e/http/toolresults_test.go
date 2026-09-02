//go:build httpe2e

package httpe2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// toolResultsProbeGitLab serves the two endpoints the server calls while it
// builds a pool entry, and hands every other path to extra.
//
// It is deliberately this package's own stand-in rather than the shared one:
// both tests here need a GitLab that answers a tool call with content nobody
// would put in a fixture, while the shared fakes exist to make credentials and
// scopes behave.
func toolResultsProbeGitLab(t *testing.T, userBody string, extra http.HandlerFunc) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userBody))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if extra != nil {
			extra(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// toolResultsCall drives one tools/call against the dynamic surface and returns
// the JSON-RPC payload, whether the transport wrote it as JSON or streamed it
// inside an SSE frame.
func toolResultsCall(t *testing.T, srv *server, action, params string) string {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action","arguments":{"action":"` +
		action + `","params":` + params +
		`},"_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion + `","io.modelcontextprotocol/clientCapabilities":{}}}}`

	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: body,
		headers: map[string]string{
			"PRIVATE-TOKEN": "glpat-whatever",
			// SEP-2243 makes the action a declared parameter of the dynamic
			// execute tool, and the transport refuses a call whose header does
			// not carry it. Omitting it fails with a header mismatch before
			// any handler runs, which is not the answer these tests want.
			"Mcp-Param-Action": action,
		},
	})
	if got.status != http.StatusOK {
		t.Fatalf("tools/call %s = %d, want 200: %s", action, got.status, toolResultsTruncate(got.body))
	}
	return jsonRPCPayload(t, got.body)
}

// toolResultsText returns the text of every content block in a tool result,
// concatenated, and whether the result was reported as a failure.
//
// The assertions here are about the text blocks specifically. That is the part
// of a result a client prints and a model reads as prose; the structured
// content beside it is JSON, whose encoder escapes a control byte on the wire
// whatever this server does with it, so asserting on the raw body would pass
// with the stripping removed and prove nothing.
func toolResultsText(t *testing.T, payload string) (string, bool) {
	t.Helper()

	var msg struct {
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		t.Fatalf("the response is not JSON-RPC: %v: %s", err, toolResultsTruncate(payload))
	}
	if msg.Result == nil {
		if msg.Error != nil {
			// A protocol-level error is still text the model reads, so it is
			// returned rather than failing here: the caller decides whether
			// that shape is the one it expected.
			return msg.Error.Message, true
		}
		t.Fatalf("the response carries neither a result nor an error: %s", toolResultsTruncate(payload))
	}
	var b strings.Builder
	for _, c := range msg.Result.Content {
		b.WriteString(c.Text)
	}
	return b.String(), msg.Result.IsError
}

// toolResultsControlRunes lists the control characters in s that have no place
// in rendered text, keeping the three Markdown actually uses: tab, newline and
// carriage return.
func toolResultsControlRunes(s string) []rune {
	var found []rune
	for _, r := range s {
		switch r {
		case '\t', '\n', '\r':
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			found = append(found, r)
		}
	}
	return found
}

// toolResultsLogPrefix is how the dispatcher opens the line it writes about a
// call, whether the call succeeded or failed.
const toolResultsLogPrefix = `"msg":"tool call `

// toolResultsToolCallLogLines waits for the dispatcher to write about a call
// and returns those lines, and only those.
//
// Waiting is necessary: the line is written before the response leaves the
// handler, but it travels to this process down a pipe, so reading the buffer
// the instant the response arrives sometimes finds it empty.
//
// Scoping is necessary too. The process output also holds the pool's startup
// probes, and the scope probe in internal/gitlab logs its error whole, so a
// proxy answering every path leaks the same page there. That is a separate leak
// in a separate layer; this test is about the funnel a tool call passes
// through, and asserting on the whole log would fail for something it does not
// pin.
func toolResultsToolCallLogLines(t *testing.T, srv *server) string {
	t.Helper()

	logs := awaitLog(t, srv, toolResultsLogPrefix)
	if logs == "" {
		t.Fatalf("the dispatcher never logged the call, so the log assertions would prove nothing: %s", toolResultsTruncate(srv.logs()))
	}
	var b strings.Builder
	for line := range strings.SplitSeq(logs, "\n") {
		if strings.Contains(line, toolResultsLogPrefix) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// toolResultsTruncate keeps a failure message readable.
func toolResultsTruncate(s string) string {
	const limit = 1500
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...[truncated]"
}

// The three controls the hostile profile carries. U+001B followed by "[2J"
// clears a terminal, U+0007 rings the bell, and U+009B is the C1 control
// sequence introducer that some terminals honor as a bare escape-bracket.
const (
	toolResultsESC = rune(0x1b)
	toolResultsBEL = rune(0x07)
	toolResultsCSI = rune(0x9b)
)

// toolResultsStrippedBio is what the bio below must look like once the control
// bytes are gone: dropping the escape leaves the "[2J" that followed it, which
// says plainly that the content tried something.
const toolResultsStrippedBio = "Reset[2Jscreenbellcsi"

// toolResultsHostileUser builds what GitLab answers with when somebody has put
// terminal escape sequences in their own profile.
//
// It is marshaled rather than written out because a raw control byte inside a
// JSON string is not JSON: client-go would refuse to decode the response and
// the tool call would fail for the wrong reason. Marshaling escapes them the
// way GitLab's own encoder does, so what arrives at the formatter is a Go
// string holding real control runes, which is the case under test.
func toolResultsHostileUser(t *testing.T) string {
	t.Helper()

	bio := "Reset" + string(toolResultsESC) + "[2Jscreen" +
		string(toolResultsBEL) + "bell" + string(toolResultsCSI) + "csi"

	body, err := json.Marshal(map[string]any{
		"id":       7,
		"username": "someone",
		"name":     "Some One",
		"state":    "active",
		"bio":      bio,
		"web_url":  "https://gitlab.example.com/someone",
	})
	if err != nil {
		t.Fatalf("building the hostile user body: %v", err)
	}
	return string(body)
}

// TestToolResults_ControlBytesFromGitLabNeverReachTheModel drives a real tool
// call whose GitLab response carries terminal escape sequences and asserts none
// of them survive into the result the client receives.
//
// GitLab content is untrusted. A bio, an issue title, a job log or a branch
// name is written by whoever can open an issue or push a branch, which on a
// public project is anybody, and it reaches the model as text and frequently a
// person's terminal unchanged. There an escape sequence is not text but an
// instruction. The result builders drop those bytes, and this pins that they
// still do.
//
// The bio is the field under test on purpose: the user formatter writes it
// straight into its builder with %s, so nothing but the strip inside the result
// builder stands between GitLab's bytes and the response. A unit test on the
// formatter cannot see that, because the formatter is not where the stripping
// happens; only a call driven through the assembled server shows what a client
// actually receives.
func TestToolResults_ControlBytesFromGitLabNeverReachTheModel(t *testing.T) {
	gitlab := toolResultsProbeGitLab(t, toolResultsHostileUser(t), nil)
	srv := startServer(t, nil, "--gitlab-url="+gitlab)

	payload := toolResultsCall(t, srv, "user.current", `{}`)
	body, isError := toolResultsText(t, payload)
	if isError {
		t.Fatalf("the call did not reach GitLab, so nothing was formatted: %s", toolResultsTruncate(body))
	}

	if found := toolResultsControlRunes(body); len(found) > 0 {
		t.Errorf("the tool result carries %d control character(s) %q that a terminal will execute: %q",
			len(found), string(found), toolResultsTruncate(body))
	}
	// The field has to have arrived, or the assertion above is vacuous: a
	// result that never carried the bio carries no escape sequence either.
	// The exact form is asserted rather than a fragment, because what must
	// survive is everything but the control bytes: dropping the escape leaves
	// the "[2J" behind it, which says plainly that the content tried something.
	if !strings.Contains(body, toolResultsStrippedBio) {
		t.Errorf("the bio did not arrive as %q, so the assertion above has nothing to prove: %q",
			toolResultsStrippedBio, toolResultsTruncate(body))
	}
}

// nginxBadGatewayPage is the page a proxy in front of GitLab answers with when
// the upstream is down. The hostname, the port and the version are the point:
// they name the operator's own infrastructure, and they used to be handed to
// the model.
const nginxBadGatewayPage = `<html>
<head><title>502 Bad Gateway</title></head>
<body>
<center><h1>502 Bad Gateway</h1></center>
<hr><center>nginx/1.24.0</center>
<!-- upstream: gitlab-web-03.internal.example:8181 -->
</body>
</html>
`

// TestToolResults_UpstreamGatewayPageIsNotReflectedToTheModel drives a tool
// call that a proxy answers with a 502 HTML page and asserts the tool error
// names the failure without echoing the body.
//
// On a deployment behind a proxy the thing that answers is not always GitLab:
// an nginx error page, a WAF block, a captive portal. client-go cannot parse
// such a body, so it puts the whole of it into the response message behind the
// "failed to parse unknown error format:" sentinel, and the %w verb that wraps
// the cause renders it. Internal hostnames and request identifiers therefore
// arrived uncapped in the text a model reads and in the server's own log line.
//
// This belongs over the wire rather than in a unit test on the wrapper, because
// what reaches the model is decided by the whole funnel: the handler wraps, the
// dispatcher sanitizes, and the SDK renders the handler error into the result.
// A test on any one of those layers passes while the assembled path leaks.
func TestToolResults_UpstreamGatewayPageIsNotReflectedToTheModel(t *testing.T) {
	const validUser = `{"id":7,"username":"someone","name":"Some One","state":"active"}`

	gitlab := toolResultsProbeGitLab(t, validUser, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(nginxBadGatewayPage))
	})
	srv := startServer(t, nil, "--gitlab-url="+gitlab)

	payload := toolResultsCall(t, srv, "project.get", `{"project_id":"acme/widgets"}`)
	body, isError := toolResultsText(t, payload)
	if !isError {
		t.Fatalf("a 502 from upstream was not reported as a failure: %s", toolResultsTruncate(body))
	}

	// The failure still has to be named, or the model is told nothing at all
	// and the safest possible response would be an empty string.
	if !strings.Contains(body, "502") && !strings.Contains(strings.ToLower(body), "bad gateway") {
		t.Errorf("the tool error names neither the status nor the failure: %s", toolResultsTruncate(body))
	}

	logs := toolResultsToolCallLogLines(t, srv)
	leaks := []struct {
		name string
		text string
	}{
		{name: "the client-go sentinel", text: "failed to parse unknown error format"},
		{name: "the proxy's internal upstream", text: "gitlab-web-03.internal.example"},
		{name: "the proxy's own version", text: "nginx/1.24.0"},
		{name: "the page's markup", text: "<html"},
	}
	for _, leak := range leaks {
		t.Run(leak.name, func(t *testing.T) {
			if strings.Contains(body, leak.text) {
				t.Errorf("the tool result echoes %s (%q) back to the model: %s", leak.name, leak.text, toolResultsTruncate(body))
			}
			if strings.Contains(logs, leak.text) {
				t.Errorf("the dispatcher's log line echoes %s (%q): %s", leak.name, leak.text, toolResultsTruncate(logs))
			}
		})
	}
}
