// health_test.go contains unit tests for the server health MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// pathVersion identifies the path version constant used by this package.
	pathVersion = "/api/v4/version"
	// pathCurrentUser identifies the path current user constant used by this package.
	pathCurrentUser = "/api/v4/user"
	// fmtStatusCheckErr identifies the fmt status check err constant used by this package.
	fmtStatusCheckErr = "Check() unexpected error: %v"
	// fmtStatusWant identifies the fmt status want constant used by this package.
	fmtStatusWant = "Status = %q, want %q"
	// testGitLabVersion identifies the test GitLab version constant used by this package.
	testGitLabVersion = "17.5.0"

	// errPrefixConnectivity and errPrefixIdentity are the test's own copies of
	// the two prefixes Check writes, kept here so a rewrite of the handler
	// cannot move both sides of a comparison at once. They are the only part of
	// the output that says which of the two calls failed.
	errPrefixConnectivity = "connectivity check failed: "
	errPrefixIdentity     = "authenticated but user retrieval failed: "
)

// TestCheck_Healthy verifies the Check_Healthy handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCheck_Healthy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathVersion:
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc123"}`)
		case pathCurrentUser:
			testutil.RespondJSON(w, http.StatusOK, `{
"id":42,
"username":"jmrplens",
"name":"Jose Requena",
"email":"jmrplens@example.com",
"state":"active"
}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf(fmtStatusCheckErr, err)
	}
	if out.Status != "healthy" {
		t.Errorf(fmtStatusWant, out.Status, "healthy")
	}
	if out.GitLabVersion != testGitLabVersion {
		t.Errorf("GitLabVersion = %q, want %q", out.GitLabVersion, testGitLabVersion)
	}
	if out.GitLabRevision != "abc123" {
		t.Errorf("GitLabRevision = %q, want %q", out.GitLabRevision, "abc123")
	}
	if !out.Authenticated {
		t.Error("Authenticated = false, want true")
	}
	if out.Username != "jmrplens" {
		t.Errorf("Username = %q, want %q", out.Username, "jmrplens")
	}
	if out.UserID != 42 {
		t.Errorf("UserID = %d, want 42", out.UserID)
	}
	if out.ResponseTimeMS < 0 {
		t.Errorf("ResponseTimeMS = %d, should be >= 0", out.ResponseTimeMS)
	}
	if out.Error != "" {
		t.Errorf("Error = %q, want empty", out.Error)
	}
}

// TestCheck_SlowVersionCall_ReportsHowLongThatCallTook asserts the reported
// response time measures the version round trip rather than nothing at all.
//
// Every other test holds this field to ">= 0", which the zero value satisfies:
// the assignment could be dropped, or the clock started after the call it is
// meant to time, and none of them would fail. An instance that answers slowly
// is what tells a real measurement from a field nobody fills, and the figure is
// the only thing in the card a caller reads to decide the instance is slow
// rather than broken. The bound is one-sided on purpose: how much longer than
// the delay it takes is the machine's business, not the code's.
func TestCheck_SlowVersionCall_ReportsHowLongThatCallTook(t *testing.T) {
	const versionDelay = 50 * time.Millisecond

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathVersion {
			time.Sleep(versionDelay)
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc123"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"u","state":"active"}`)
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf(fmtStatusCheckErr, err)
	}
	if out.Status != "healthy" {
		t.Errorf(fmtStatusWant, out.Status, "healthy")
	}
	floor := (versionDelay - 10*time.Millisecond).Milliseconds()
	if out.ResponseTimeMS < floor {
		t.Errorf("ResponseTimeMS = %d, want at least %d for a version call that took %s", out.ResponseTimeMS, floor, versionDelay)
	}
}

// TestCheck_SlowIdentityCall_IsNotCountedInTheResponseTime asserts that the
// reported time covers the version round trip alone, which is where the
// measurement stops.
//
// Its sibling above holds only a floor, and a figure that timed the whole
// check would satisfy that just as well, so the clock could be stopped after
// the identity lookup instead and nothing would fail. Which call it measures
// decides what the number means: an instance answering at once while the
// identity lookup crawls is reachable, and a figure folding both in reports it
// as a slow instance. The ceiling is half the delay so the bound is about
// which call was timed rather than about how fast this machine is.
func TestCheck_SlowIdentityCall_IsNotCountedInTheResponseTime(t *testing.T) {
	const identityDelay = 200 * time.Millisecond

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathVersion {
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc123"}`)
			return
		}
		time.Sleep(identityDelay)
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"u","state":"active"}`)
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf(fmtStatusCheckErr, err)
	}
	if out.Status != "healthy" {
		t.Errorf(fmtStatusWant, out.Status, "healthy")
	}
	ceiling := (identityDelay / 2).Milliseconds()
	if out.ResponseTimeMS >= ceiling {
		t.Errorf("ResponseTimeMS = %d, want under %d: the identity call took %s and is not part of this figure", out.ResponseTimeMS, ceiling, identityDelay)
	}
}

// TestSetServerInfo_PopulatesCheckOutput verifies that calling SetServerInfo
// causes Check to include server metadata (version, author, department,
// repository) in the Output.
func TestSetServerInfo_PopulatesCheckOutput(t *testing.T) {
	// Save and restore global state.
	original := serverInfo
	t.Cleanup(func() { serverInfo = original })

	SetServerInfo(ServerInfo{
		Version:    "1.2.3",
		Author:     "Test Author",
		Department: "Test Dept",
		Repository: "https://example.com/repo",
	})

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathVersion:
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc"}`)
		case pathCurrentUser:
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"u","state":"active"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if out.MCPServerVersion != "1.2.3" {
		t.Errorf("MCPServerVersion = %q, want %q", out.MCPServerVersion, "1.2.3")
	}
	if out.Author != "Test Author" {
		t.Errorf("Author = %q, want %q", out.Author, "Test Author")
	}
	if out.Department != "Test Dept" {
		t.Errorf("Department = %q, want %q", out.Department, "Test Dept")
	}
	if out.Repository != "https://example.com/repo" {
		t.Errorf("Repository = %q, want %q", out.Repository, "https://example.com/repo")
	}
}

// TestSetServerInfo_DefaultsEmpty verifies the SetServerInfo_DefaultsEmpty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSetServerInfo_DefaultsEmpty(t *testing.T) {
	original := serverInfo
	t.Cleanup(func() { serverInfo = original })

	SetServerInfo(ServerInfo{})

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathVersion:
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc"}`)
		case pathCurrentUser:
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"u","state":"active"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if out.MCPServerVersion != "" {
		t.Errorf("MCPServerVersion = %q, want empty", out.MCPServerVersion)
	}
	if out.Author != "" {
		t.Errorf("Author = %q, want empty", out.Author)
	}
	if out.Department != "" {
		t.Errorf("Department = %q, want empty", out.Department)
	}
	if out.Repository != "" {
		t.Errorf("Repository = %q, want empty", out.Repository)
	}
}

// TestCheck_UnhealthyVersionFails asserts that an instance which does not
// answer the version call is reported unhealthy, unauthenticated, and with an
// error naming the connectivity half of the check.
//
// The prefix is asserted rather than the mere presence of an error, because it
// is the only thing in the output that says which call failed: held to "some
// error", this branch and the degraded one below could trade their messages
// and a caller would be told the credential was refused by an instance that
// never answered at all.
func TestCheck_UnhealthyVersionFails(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathVersion:
			testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf(fmtStatusCheckErr, err)
	}
	if out.Status != "unhealthy" {
		t.Errorf(fmtStatusWant, out.Status, "unhealthy")
	}
	if out.Authenticated {
		t.Error("Authenticated = true, want false")
	}
	if !strings.HasPrefix(out.Error, errPrefixConnectivity) {
		t.Errorf("Error = %q, want the %q prefix", out.Error, errPrefixConnectivity)
	}
}

// TestCheck_DegradedUserFails asserts that an instance which answers the
// version call but refuses the identity lookup is reported degraded, with the
// version it did send and an error naming the identity half.
//
// It is the other side of the pair described above: the prefix is what stops
// the two failure messages being interchangeable.
func TestCheck_DegradedUserFails(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathVersion:
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc123"}`)
		case pathCurrentUser:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Check(context.Background(), client, Input{})
	if err != nil {
		t.Fatalf(fmtStatusCheckErr, err)
	}
	if out.Status != "degraded" {
		t.Errorf(fmtStatusWant, out.Status, "degraded")
	}
	if out.GitLabVersion != testGitLabVersion {
		t.Errorf("GitLabVersion = %q, want %q", out.GitLabVersion, testGitLabVersion)
	}
	if out.Authenticated {
		t.Error("Authenticated = true, want false")
	}
	if !strings.HasPrefix(out.Error, errPrefixIdentity) {
		t.Errorf("Error = %q, want the %q prefix", out.Error, errPrefixIdentity)
	}
}

// TestCheck_CancelledContext asserts that a context already cancelled is
// refused before any request is made, and with the context's own error.
//
// The mock is testutil.ForbiddenHandler, which fails the test if a request
// arrives. That is the claim this test announced and did not hold: against a
// mock that answers, "an error came back" is satisfied just as well by a
// handler that calls GitLab and lets the transport refuse.
func TestCheck_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Check(testutil.CancelledCtx(t), client, Input{})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Check() error = %v, want context.Canceled", err)
	}
}

// TestOutput_JSONDocument_KeysEveryFieldUnderItsOwnName pins the JSON name of
// every field of Output, and pins the whole document rather than one key.
//
// Nothing else in this package ever decodes an Output: the card renders off the
// Go fields, so two `json` tags of the same type could trade places with every
// other test here green while a caller reading the JSON result got the version
// under gitlab_revision and the department under author. Each fixture value
// below names the field it belongs to, so a crossed tag puts a value somewhere
// a reader can see it, and the comparison is over the whole decoded document so
// that no key escapes by being the one nobody looked at.
//
// The zero half is the other side of the same tag: it says which fields carry
// omitempty, which is the part of a tag a populated document cannot show.
func TestOutput_JSONDocument_KeysEveryFieldUnderItsOwnName(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		out := Output{
			NextSteps:        []string{"next step for the caller"},
			Status:           "status of the check",
			MCPServerVersion: "3.1.0-mcp-server",
			Author:           "author of the binary",
			Department:       "department of the author",
			Repository:       "https://example.com/repository",
			GitLabURL:        "https://gitlab.example.com/instance",
			GitLabVersion:    "17.5.0-gitlab",
			GitLabRevision:   "revision0",
			Authenticated:    true,
			Username:         "username-of-the-credential",
			UserID:           4242,
			ResponseTimeMS:   1717,
			Error:            "error the check reported",
		}

		got := decodeOutputAsJSON(t, out)
		want := map[string]any{
			"next_steps":         []any{"next step for the caller"},
			"status":             "status of the check",
			"mcp_server_version": "3.1.0-mcp-server",
			"author":             "author of the binary",
			"department":         "department of the author",
			"repository":         "https://example.com/repository",
			"gitlab_url":         "https://gitlab.example.com/instance",
			"gitlab_version":     "17.5.0-gitlab",
			"gitlab_revision":    "revision0",
			"authenticated":      true,
			"username":           "username-of-the-credential",
			"user_id":            float64(4242),
			"response_time_ms":   float64(1717),
			"error":              "error the check reported",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Output marshaled to %#v, want %#v", got, want)
		}
	})

	t.Run("nothing filled in", func(t *testing.T) {
		got := decodeOutputAsJSON(t, Output{})
		want := map[string]any{
			"status":           "",
			"gitlab_url":       "",
			"authenticated":    false,
			"response_time_ms": float64(0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("zero Output marshaled to %#v, want %#v", got, want)
		}
	})
}

// decodeOutputAsJSON marshals out the way a tool result is marshaled and reads
// the document back as a map, so a comparison holds the keys rather than the
// order encoding/json happens to write the struct's fields in.
func decodeOutputAsJSON(t *testing.T, out Output) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal(Output) unexpected error: %v", err)
	}
	var got map[string]any
	if decodeErr := json.Unmarshal(encoded, &got); decodeErr != nil {
		t.Fatalf("json.Unmarshal(%s) unexpected error: %v", encoded, decodeErr)
	}
	return got
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatMarkdownString — healthy
// ---------------------------------------------------------------------------.

// healthHints is the guidance section every health card ends with.
const healthHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'project.list' to explore available projects\n" +
	"- Use action 'user.me' to see current user details\n"

// TestFormatMarkdownString_Healthy verifies the whole card a reachable,
// authenticated instance renders as: every field the check filled, the flag as
// a glyph rather than the word "true", and no Error row.
func TestFormatMarkdownString_Healthy(t *testing.T) {
	out := Output{
		Status:           "healthy",
		MCPServerVersion: "1.0.0",
		Author:           "Test Author",
		Department:       "Test Dept",
		Repository:       "https://example.com/repo",
		GitLabURL:        "https://gitlab.example.com",
		GitLabVersion:    "17.5.0",
		GitLabRevision:   "abc123",
		Authenticated:    true,
		Username:         "alice",
		UserID:           42,
		ResponseTimeMS:   15,
	}
	got := FormatMarkdownString(out)
	want := "## \u2705 GitLab Server Status: healthy\n\n" +
		"- **MCP Server Version**: 1.0.0\n" +
		"- **Author**: Test Author\n" +
		"- **Department**: Test Dept\n" +
		"- **Repository**: https://example.com/repo\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Version**: 17.5.0\n" +
		"- **Revision**: abc123\n" +
		"- **Authenticated**: \u2705\n" +
		"- **User**: @alice\n" +
		"- **User ID**: 42\n" +
		"- **Response Time**: 15 ms\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatMarkdownString_WithMetadata verifies the whole card when the
// binary was registered with its build metadata and GitLab has not answered
// yet: the four rows about this server, and nothing about the instance beyond
// its address.
func TestFormatMarkdownString_WithMetadata(t *testing.T) {
	out := Output{
		Status:           "healthy",
		MCPServerVersion: "2.3.4",
		Author:           "Test Author",
		Department:       "Test Department",
		Repository:       "https://github.com/jmrplens/gitlab-mcp-server",
		GitLabURL:        "https://gitlab.example.com",
	}
	got := FormatMarkdownString(out)
	want := "## ✅ GitLab Server Status: healthy\n\n" +
		"- **MCP Server Version**: 2.3.4\n" +
		"- **Author**: Test Author\n" +
		"- **Department**: Test Department\n" +
		"- **Repository**: https://github.com/jmrplens/gitlab-mcp-server\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Authenticated**: ❌\n" +
		"- **Response Time**: 0 ms\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatMarkdownString_WithoutMetadata verifies the whole card when the
// binary carries no build metadata: no label stands with an empty value after
// it.
func TestFormatMarkdownString_WithoutMetadata(t *testing.T) {
	out := Output{
		Status:    "healthy",
		GitLabURL: "https://gitlab.example.com",
	}
	got := FormatMarkdownString(out)
	want := "## ✅ GitLab Server Status: healthy\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Authenticated**: ❌\n" +
		"- **Response Time**: 0 ms\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — unhealthy
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_Unhealthy verifies the whole card an unreachable
// instance renders as: the cross in the heading, no version or identity rows,
// and the error on a line of its own.
func TestFormatMarkdownString_Unhealthy(t *testing.T) {
	out := Output{
		Status:         "unhealthy",
		GitLabURL:      "https://gitlab.example.com",
		ResponseTimeMS: 100,
		Error:          "connectivity check failed: connection refused",
	}
	got := FormatMarkdownString(out)
	want := "## \u274c GitLab Server Status: unhealthy\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Authenticated**: \u274c\n" +
		"- **Response Time**: 100 ms\n" +
		"- **Error**: connectivity check failed: connection refused\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatMarkdownString_UnhealthyHTMLError verifies that an error carrying
// a proxy's HTML page changes no structure. client-go puts the whole response
// body in its message when the body is not the JSON it expected, so the tags
// and the line breaks arrive here intact, and a body with a line break in it
// is quoted rather than left to open blocks of its own.
func TestFormatMarkdownString_UnhealthyHTMLError(t *testing.T) {
	out := Output{
		Status:    "unhealthy",
		GitLabURL: "https://gitlab.example.com",
		Error:     "<html>\n## Gateway Timeout\n</html>",
	}
	got := FormatMarkdownString(out)
	want := "## \u274c GitLab Server Status: unhealthy\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Authenticated**: \u274c\n" +
		"- **Response Time**: 0 ms\n" +
		"- **Error**:\n" +
		"  > <html>\n" +
		"  > ## Gateway Timeout\n" +
		"  > </html>\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — degraded
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_Degraded verifies the whole card an instance that
// answered but identified nobody renders as: the warning glyph in the heading,
// the version rows GitLab did send, and the error beneath them.
func TestFormatMarkdownString_Degraded(t *testing.T) {
	out := Output{
		Status:         "degraded",
		GitLabURL:      "https://gitlab.example.com",
		GitLabVersion:  "17.5.0",
		GitLabRevision: "abc123",
		Authenticated:  false,
		ResponseTimeMS: 50,
		Error:          "user retrieval failed",
	}
	got := FormatMarkdownString(out)
	want := "## \u26a0\ufe0f GitLab Server Status: degraded\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Version**: 17.5.0\n" +
		"- **Revision**: abc123\n" +
		"- **Authenticated**: \u274c\n" +
		"- **Response Time**: 50 ms\n" +
		"- **Error**: user retrieval failed\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — no username (empty)
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_NoUsername verifies that a check that identified
// nobody writes neither the handle row nor the id row, where a bare "@" and an
// id of zero would both read as an identity.
func TestFormatMarkdownString_NoUsername(t *testing.T) {
	out := Output{
		Status:        "healthy",
		GitLabURL:     "https://gitlab.example.com",
		Authenticated: true,
	}
	got := FormatMarkdownString(out)
	want := "## ✅ GitLab Server Status: healthy\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Authenticated**: ✅\n" +
		"- **Response Time**: 0 ms\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — no version (empty)
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_NoVersion verifies that a version GitLab sent with
// no revision beside it renders as one row, where the two joined as
// "%s (revision: %s)" printed an empty parenthesis, and that a check that
// learned no version at all writes neither row.
func TestFormatMarkdownString_NoVersion(t *testing.T) {
	got := FormatMarkdownString(Output{
		Status:    "unhealthy",
		GitLabURL: "https://gitlab.example.com",
		Error:     "failed",
	})
	want := "## ❌ GitLab Server Status: unhealthy\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Authenticated**: ❌\n" +
		"- **Response Time**: 0 ms\n" +
		"- **Error**: failed\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}

	got = FormatMarkdownString(Output{
		Status:        "healthy",
		GitLabURL:     "https://gitlab.example.com",
		GitLabVersion: "17.5.0",
		Authenticated: true,
	})
	want = "## ✅ GitLab Server Status: healthy\n\n" +
		"- **GitLab URL**: https://gitlab.example.com\n" +
		"- **Version**: 17.5.0\n" +
		"- **Authenticated**: ✅\n" +
		"- **Response Time**: 0 ms\n" +
		healthHints
	if got != want {
		t.Errorf("FormatMarkdownString() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown wrapper
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Wrapper asserts that the CallToolResult wrapper carries
// the card the string formatter renders, and carries it as the one text block
// a client reads.
//
// It reaches no GitLab: the wrapper is pure. Holding it to "content is not
// empty" is satisfied by a result built from any string at all, which is the
// one way this thin function can be wrong.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	out := Output{Status: "healthy", GitLabURL: "https://gitlab.example.com"}
	result := FormatMarkdown(out)
	if result == nil {
		t.Fatal("FormatMarkdown returned nil")
	}
	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
	}
	if want := toolutil.NormalizeResultMarkdown(FormatMarkdownString(out)); text.Text != want {
		t.Errorf("FormatMarkdown text =\n%q\nwant:\n%q", text.Text, want)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata asserts that the package publishes the two actions,
// both owned here, and that the status one carries the usage phrase, the alias
// and the Returns/See also guidance a model reads before calling it.
//
// It builds a client only because ActionSpecs takes one; no route is driven
// here and nothing reaches the mock.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc"}`)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "health" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
	statusSpec := healthSpecByTool(t, specs, "gitlab_server_status")
	if !strings.Contains(statusSpec.Usage, "connectivity") {
		t.Fatalf("status Usage = %q, want connectivity guidance", statusSpec.Usage)
	}
	if !slices.Contains(statusSpec.Aliases, "mcp server status") {
		t.Fatalf("status Aliases = %v, want mcp server status", statusSpec.Aliases)
	}
	if !strings.Contains(statusSpec.IndividualTool.Description, "Returns:") || !strings.Contains(statusSpec.IndividualTool.Description, "See also:") {
		t.Fatalf("status description = %q, want Returns/See also guidance", statusSpec.IndividualTool.Description)
	}
	healthCheckSpec := healthSpecByName(t, specs, "health_check")
	if !slices.Contains(healthCheckSpec.Aliases, "connectivity check") {
		t.Fatalf("health_check Aliases = %v, want connectivity check", healthCheckSpec.Aliases)
	}
}

// healthUsage is the one usage line both health actions carry, kept here as the
// test's own copy so a rewrite of the spec builder cannot move both sides of
// the comparison at once.
const healthUsage = "Verify MCP server connectivity to GitLab, authenticated identity, and response health before troubleshooting other tool failures."

// TestActionSpecs_EachAction_PublishesTheSurfaceAModelFindsItBy pins, per
// action, the metadata a model searches the catalog by.
//
// Both actions route to the one Check handler, so this metadata is the only
// thing that tells them apart: the aliases decide which phrase reaches which
// action, and the individual-tool projection decides which of the two becomes a
// tool of its own. That split used to be resolved by a switch on the action
// name inside the options builder, where one arm's aliases could be handed to
// the other, or both actions given one tool name, with nothing failing.
func TestActionSpecs_EachAction_PublishesTheSurfaceAModelFindsItBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc"}`)
	}))
	specs := ActionSpecs(client)

	cases := []struct {
		name       string
		aliases    []string
		individual toolutil.IndividualToolSpec
	}{
		{
			name:    "status",
			aliases: []string{"mcp server status", "gitlab server status", "gitlab connectivity status"},
			individual: toolutil.IndividualToolSpec{
				Name:        "gitlab_server_status",
				Title:       "Server Status",
				Description: "Check MCP server connectivity, GitLab reachability, and authenticated identity details. Returns: the current server and GitLab health diagnostics object. See also: gitlab_get_metadata, gitlab_user_current.",
			},
		},
		{
			name: "health_check",
			aliases: []string{
				"health check", "server health check", "connectivity check", "gitlab health check",
				"server diagnostics", "run diagnostics", "diagnostics", "server status check",
			},
			// No individual tool: a second tool over the same handler would be
			// the same call registered twice, under a name one of them would
			// have to share.
			individual: toolutil.IndividualToolSpec{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := healthSpecByName(t, specs, tc.name)
			if !slices.Equal(spec.Aliases, tc.aliases) {
				t.Errorf("Aliases = %v, want %v", spec.Aliases, tc.aliases)
			}
			if spec.IndividualTool != tc.individual {
				t.Errorf("IndividualTool = %+v, want %+v", spec.IndividualTool, tc.individual)
			}
			if !slices.Equal(spec.Tags, []string{"server", "health", "diagnostics", "connectivity"}) {
				t.Errorf("Tags = %v, want the four health tags", spec.Tags)
			}
			if !slices.Equal(spec.RelatedActions, []string{"admin.metadata_get", "user.me"}) {
				t.Errorf("RelatedActions = %v, want admin.metadata_get and user.me", spec.RelatedActions)
			}
			if spec.Usage != healthUsage {
				t.Errorf("Usage = %q, want %q", spec.Usage, healthUsage)
			}
			if spec.OwnerPackage != "health" {
				t.Errorf("OwnerPackage = %q, want %q", spec.OwnerPackage, "health")
			}
			if !spec.OpenWorld {
				t.Error("OpenWorld = false, want true: the answer comes from an instance this process does not own")
			}
		})
	}
}

// TestActionSpecs_BothActions_StayReadOnly asserts that neither health action
// is classified as a write.
//
// Both --read-only and a read_api token narrow the surface per action, so an
// action misclassified here vanishes from exactly the deployments that most
// need it: the check is what a caller reaches for when something is already
// wrong. Nothing else in the package held it, and the whole suite passed with
// both specs built as mutating creates.
func TestActionSpecs_BothActions_StayReadOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc"}`)
	}))

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if !spec.ReadOnly {
				t.Error("ReadOnly = false, want true: --read-only would withdraw the tool that diagnoses the failure")
			}
			if spec.Destructive {
				t.Error("Destructive = true, want false: the check writes nothing")
			}
			if !spec.Idempotent {
				t.Error("Idempotent = false, want true: asking twice asks the same question")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution — gitlab_server_status
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoute drives the status action through the route every
// surface dispatches to, and asserts the health output that reaches a caller.
//
// Asserting only that something non-nil came back would hold for a route bound
// to any handler in the tree; what makes this one the status action is that the
// answer carries the instance and the identity it just read.
func TestActionSpecs_CallRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc123"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":42,"username":"alice","name":"Alice","email":"alice@example.com","state":"active"
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	spec := healthSpecByTool(t, ActionSpecs(client), "gitlab_server_status")
	res, err := spec.Route.Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler gitlab_server_status: %v", err)
	}
	out, ok := res.(Output)
	if !ok {
		t.Fatalf("Route.Handler returned %T, want health.Output", res)
	}
	if out.Status != "healthy" {
		t.Errorf(fmtStatusWant, out.Status, "healthy")
	}
	if out.GitLabVersion != testGitLabVersion {
		t.Errorf("GitLabVersion = %q, want %q", out.GitLabVersion, testGitLabVersion)
	}
	if out.Username != "alice" {
		t.Errorf("Username = %q, want %q", out.Username, "alice")
	}
	if out.UserID != 42 {
		t.Errorf("UserID = %d, want 42", out.UserID)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution — gitlab_server_status unhealthy (API error)
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteUnhealthy asserts that an instance refusing the
// version call reaches a caller through the route as an unhealthy report
// rather than as a handler error.
//
// Both halves matter to a model: the route returns nil error, so the failure
// has to be readable in the output, and the error field has to name the
// connectivity half. The test was called Unhealthy while asserting nothing
// that a healthy answer would have failed.
func TestActionSpecs_CallRouteUnhealthy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))

	spec := healthSpecByTool(t, ActionSpecs(client), "gitlab_server_status")
	res, err := spec.Route.Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler gitlab_server_status: %v", err)
	}
	out, ok := res.(Output)
	if !ok {
		t.Fatalf("Route.Handler returned %T, want health.Output", res)
	}
	if out.Status != "unhealthy" {
		t.Errorf(fmtStatusWant, out.Status, "unhealthy")
	}
	if !strings.HasPrefix(out.Error, errPrefixConnectivity) {
		t.Errorf("Error = %q, want the %q prefix", out.Error, errPrefixConnectivity)
	}
}

// healthSpecByTool supports health spec by tool assertions in health tests.
func healthSpecByTool(t *testing.T, specs []toolutil.ActionSpec, tool string) toolutil.ActionSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.IndividualTool.Name == tool {
			return spec
		}
	}
	t.Fatalf("missing ActionSpec for %s", tool)
	return toolutil.ActionSpec{}
}

func healthSpecByName(t *testing.T, specs []toolutil.ActionSpec, name string) toolutil.ActionSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("missing ActionSpec for %s", name)
	return toolutil.ActionSpec{}
}

// ---------------------------------------------------------------------------
// Diagnostics never carry the secrets a base URL can hide
// ---------------------------------------------------------------------------.

// TestCheck_URLCarryingSecrets_KeepsOnlySchemeHostAndPath verifies that the
// reported instance URL is reduced to the parts that identify the instance.
// GITLAB_URL is validated for scheme and host alone, so userinfo, a query and a
// fragment are all configurable, and this value ends up in a tool result and in
// whatever the client logs. The rule now lives in toolutil.RedactURL, which
// this tool and the webhook audit prompt share.
func TestCheck_URLCarryingSecrets_KeepsOnlySchemeHostAndPath(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "https://gitlab.example.com/api/v4/", want: "https://gitlab.example.com/api/v4/"},
		{name: "userinfo with password", raw: "https://proxyuser:s3cret@gitlab.example.com/api/v4/", want: "https://gitlab.example.com/api/v4/"},
		{name: "username only", raw: "https://proxyuser@gitlab.example.com/api/v4/", want: "https://gitlab.example.com/api/v4/"},
		{name: "query and fragment", raw: "https://gitlab.example.com/api/v4/?token=s3cret#frag", want: "https://gitlab.example.com/api/v4/"},
		{name: "escaped path", raw: "https://gitlab.example.com/gitlab%20ce/api/v4/", want: "https://gitlab.example.com/gitlab%20ce/api/v4/"},
		{name: "no path", raw: "https://gitlab.example.com", want: "https://gitlab.example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tc.raw, err)
			}
			if got := toolutil.RedactURL(parsed); got != tc.want {
				t.Errorf("toolutil.RedactURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestWithoutUserinfo_EveryStandardRendering_RemovesTheCredential verifies that
// each of the three ways the standard library writes a URL credential into an
// error is stripped: verbatim (url.URL.String), password-masked (the *url.Error
// net/http returns) and Redacted. The username is the part none of them hides
// on its own, and an empty username must not turn into a stray "@".
func TestWithoutUserinfo_EveryStandardRendering_RemovesTheCredential(t *testing.T) {
	cases := []struct {
		name string
		user *url.Userinfo
		text string
		want string
	}{
		{
			name: "no userinfo leaves the text alone",
			user: nil,
			text: `Get "https://gitlab.example.com/api/v4/version": dial tcp: refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": dial tcp: refused`,
		},
		{
			name: "verbatim rendering",
			user: url.UserPassword("proxyuser", "s3cret"),
			text: `Get "https://proxyuser:s3cret@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
		{
			name: "net http password mask",
			user: url.UserPassword("proxyuser", "s3cret"),
			text: `Get "https://proxyuser:***@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
		{
			name: "redacted password mask",
			user: url.UserPassword("proxyuser", "s3cret"),
			text: `Get "https://proxyuser:xxxxx@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
		{
			name: "username only",
			user: url.User("proxyuser"),
			text: `Get "https://proxyuser@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
		{
			name: "empty username leaves at-signs alone",
			user: url.User(""),
			text: `Get "https://gitlab.example.com/api/v4/version": alice@example.com refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": alice@example.com refused`,
		},
		{
			name: "escaped username in the redacted mask",
			user: url.UserPassword("proxy@corp", "s3cret"),
			text: `Get "https://proxy%40corp:xxxxx@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
		{
			name: "decoded username in the net http mask",
			user: url.UserPassword("proxy@corp", "s3cret"),
			text: `Get "https://proxy@corp:***@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
		{
			name: "escaped username without a password",
			user: url.User("proxy@corp"),
			text: `Get "https://proxy%40corp@gitlab.example.com/api/v4/version": refused`,
			want: `Get "https://gitlab.example.com/api/v4/version": refused`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withoutUserinfo(tc.text, tc.user); got != tc.want {
				t.Errorf("withoutUserinfo() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCheck_BaseURLWithUserinfo_ReportsTheURLWithoutIt drives the handler
// against an instance whose configured URL carries a proxy credential and
// asserts the credential reaches neither the reported URL nor the error field.
func TestCheck_BaseURLWithUserinfo_ReportsTheURLWithoutIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathVersion {
			testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc123"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	t.Cleanup(srv.Close)

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", srv.URL, err)
	}
	parsed.User = url.UserPassword("proxyuser", "s3cret")

	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:      parsed.String(),
		GitLabToken:    "test-token",
		DisableRetries: true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	out, err := Check(t.Context(), client, Input{})
	if err != nil {
		t.Fatalf(fmtStatusCheckErr, err)
	}
	if out.Status != "degraded" {
		t.Errorf(fmtStatusWant, out.Status, "degraded")
	}
	for _, secret := range []string{"proxyuser", "s3cret"} {
		t.Run(secret, func(t *testing.T) {
			if strings.Contains(out.GitLabURL, secret) {
				t.Errorf("GitLabURL = %q, must not carry %q", out.GitLabURL, secret)
			}
			if strings.Contains(out.Error, secret) {
				t.Errorf("Error = %q, must not carry %q", out.Error, secret)
			}
		})
	}
	if !strings.HasPrefix(out.GitLabURL, "http://"+parsed.Host+"/") {
		t.Errorf("GitLabURL = %q, want the instance host %q", out.GitLabURL, parsed.Host)
	}
}
