// health_test.go contains unit tests for the server health MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

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

// TestCheck_UnhealthyVersionFails verifies the Check_UnhealthyVersionFails handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
	if out.Error == "" {
		t.Error("Error should not be empty for unhealthy status")
	}
}

// TestCheck_DegradedUserFails verifies the Check_DegradedUserFails handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
	if out.Error == "" {
		t.Error("Error should not be empty for degraded status")
	}
}

// TestCheck_CancelledContext verifies the Check_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCheck_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Check(ctx, client, Input{})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatMarkdownString — healthy
// ---------------------------------------------------------------------------.

// healthHints is the guidance section every health card ends with.
const healthHints = "\n---\n💡 **Next steps:**\n" +
	"- Use gitlab_project action 'list' to explore available projects\n" +
	"- Use gitlab_user action 'me' to see current user details\n"

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

// TestFormatMarkdownString_Unhealthy verifies the MarkdownString_Unhealthy Markdown formatter for a representative string_unhealthy input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatMarkdownString_Degraded verifies the MarkdownString_Degraded Markdown formatter for a representative string_degraded input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatMarkdown_Wrapper verifies the Markdown_Wrapper Markdown formatter for a representative _wrapper input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	out := Output{Status: "healthy", GitLabURL: "https://gitlab.example.com"}
	result := FormatMarkdown(out)
	if result == nil {
		t.Fatal("FormatMarkdown returned nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("FormatMarkdown returned empty content")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// ---------------------------------------------------------------------------
// ActionSpec route execution — gitlab_server_status
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoute validates the CallRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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
	if res == nil {
		t.Fatal("nil result")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution — gitlab_server_status unhealthy (API error)
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteUnhealthy validates the CallRouteUnhealthy route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRouteUnhealthy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))

	spec := healthSpecByTool(t, ActionSpecs(client), "gitlab_server_status")
	res, err := spec.Route.Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler gitlab_server_status: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
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
