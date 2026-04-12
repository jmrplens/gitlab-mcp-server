// projectdiscovery_test.go contains unit tests for the project discovery
// tool handlers defined in projectdiscovery.go.
//
// All tests use [httptest] to mock the GitLab REST API endpoints. Tests
// cover URL parsing (HTTPS, SSH shorthand, ssh:// protocol, edge cases),
// project resolution via the API, markdown formatting of results, and
// MCP round-trip registration via in-memory transports.
package projectdiscovery

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// Test constants shared across project discovery tests.
const (
	pathProject     = "/api/v4/projects/"
	fmtResolveErr   = "Resolve() unexpected error: %v"
	fmtWantField    = "%s = %v, want %v"
	testProjectJSON = `{
		"id": 42,
		"name": "my-project",
		"path": "my-project",
		"path_with_namespace": "group/subgroup/my-project",
		"web_url": "https://gitlab.example.com/group/subgroup/my-project",
		"default_branch": "main",
		"description": "A test project",
		"visibility": "private",
		"http_url_to_repo": "https://gitlab.example.com/group/subgroup/my-project.git",
		"ssh_url_to_repo": "git@gitlab.example.com:group/subgroup/my-project.git"
	}`
)

// TestParseRemoteURL_ValidURLs verifies that ParseRemoteURL correctly extracts
// the path_with_namespace from all supported git remote URL formats.
func TestParseRemoteURL_ValidURLs(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantPath string
	}{
		{
			name:     "HTTPS with .git suffix",
			url:      "https://gitlab.example.com/group/project.git",
			wantPath: "group/project",
		},
		{
			name:     "HTTPS without .git suffix",
			url:      "https://gitlab.example.com/group/project",
			wantPath: "group/project",
		},
		{
			name:     "HTTPS with subgroups",
			url:      "https://gitlab.example.com/group/subgroup/project.git",
			wantPath: "group/subgroup/project",
		},
		{
			name:     "SSH shorthand",
			url:      "git@gitlab.example.com:group/project.git",
			wantPath: "group/project",
		},
		{
			name:     "SSH shorthand without .git",
			url:      "git@gitlab.example.com:group/project",
			wantPath: "group/project",
		},
		{
			name:     "SSH shorthand with subgroups",
			url:      "git@gitlab.example.com:group/subgroup/project.git",
			wantPath: "group/subgroup/project",
		},
		{
			name:     "SSH protocol URL",
			url:      "ssh://git@gitlab.example.com/group/project.git",
			wantPath: "group/project",
		},
		{
			name:     "Git protocol URL",
			url:      "git://gitlab.example.com/group/project.git",
			wantPath: "group/project",
		},
		{
			name:     "HTTPS with trailing slash",
			url:      "https://gitlab.example.com/group/project/",
			wantPath: "group/project",
		},
		{
			name:     "URL with whitespace",
			url:      "  https://gitlab.example.com/group/project.git  ",
			wantPath: "group/project",
		},
		{
			name:     "HTTPS with port",
			url:      "https://gitlab.example.com:8443/group/project.git",
			wantPath: "group/project",
		},
		{
			name:     "SSH with custom user",
			url:      "deploy@gitlab.example.com:group/project.git",
			wantPath: "group/project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRemoteURL(tc.url)
			if err != nil {
				t.Fatalf("ParseRemoteURL(%q) unexpected error: %v", tc.url, err)
			}
			if got != tc.wantPath {
				t.Errorf("ParseRemoteURL(%q) = %q, want %q", tc.url, got, tc.wantPath)
			}
		})
	}
}

// TestParseRemoteURL_InvalidURLs verifies that ParseRemoteURL returns an error
// for malformed or unsupported remote URL formats.
func TestParseRemoteURL_InvalidURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "empty string", url: ""},
		{name: "whitespace only", url: "   "},
		{name: "no host", url: "/just/a/path"},
		{name: "relative path", url: "group/project"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRemoteURL(tc.url)
			if err == nil {
				t.Errorf("ParseRemoteURL(%q) expected error, got nil", tc.url)
			}
		})
	}
}

// TestParseRemoteURL_TruncatedSSH verifies that ParseRemoteURL returns an
// actionable error when an SSH URL is missing the user@ prefix (e.g.
// "host:group/project.git" instead of "git@host:group/project.git"). This is
// the most common LLM mistake when extracting URLs from git push output.
func TestParseRemoteURL_TruncatedSSH(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantMsg string
	}{
		{
			name:    "host:path without user prefix",
			url:     "gitlab.example.com:org/project.git",
			wantMsg: "truncated SSH remote missing the user prefix",
		},
		{
			name:    "host:path without .git suffix",
			url:     "gitlab.example.com:group/project",
			wantMsg: "truncated SSH remote missing the user prefix",
		},
		{
			name:    "host:nested/path",
			url:     "gitlab.example.com:org/mcp/gitlab-mcp-server.git",
			wantMsg: "git@gitlab.example.com:org/mcp/gitlab-mcp-server.git",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRemoteURL(tc.url)
			if err == nil {
				t.Fatalf("ParseRemoteURL(%q) expected error, got nil", tc.url)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("ParseRemoteURL(%q) error = %q, want it to contain %q", tc.url, err.Error(), tc.wantMsg)
			}
		})
	}
}

// TestResolve_Success verifies that Resolve parses the remote URL and returns
// the matching GitLab project from the API.
func TestResolve_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// GitLab API encodes path as URL-encoded: group%2Fsubgroup%2Fmy-project
		if r.URL.Path == pathProject+"group%2Fsubgroup%2Fmy-project" ||
			r.URL.Path == pathProject+"group/subgroup/my-project" {
			testutil.RespondJSON(w, http.StatusOK, testProjectJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Resolve(context.Background(), client, ResolveInput{
		RemoteURL: "https://gitlab.example.com/group/subgroup/my-project.git",
	})
	if err != nil {
		t.Fatalf(fmtResolveErr, err)
	}
	if out.ID != 42 {
		t.Errorf(fmtWantField, "ID", out.ID, 42)
	}
	if out.PathWithNamespace != "group/subgroup/my-project" {
		t.Errorf(fmtWantField, "PathWithNamespace", out.PathWithNamespace, "group/subgroup/my-project")
	}
	if out.DefaultBranch != "main" {
		t.Errorf(fmtWantField, "DefaultBranch", out.DefaultBranch, "main")
	}
	if out.ExtractedPath != "group/subgroup/my-project" {
		t.Errorf(fmtWantField, "ExtractedPath", out.ExtractedPath, "group/subgroup/my-project")
	}
}

// TestResolve_SSHRemote verifies that Resolve works with SSH shorthand URLs.
func TestResolve_SSHRemote(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, testProjectJSON)
	}))

	out, err := Resolve(context.Background(), client, ResolveInput{
		RemoteURL: "git@gitlab.example.com:group/subgroup/my-project.git",
	})
	if err != nil {
		t.Fatalf(fmtResolveErr, err)
	}
	if out.ID != 42 {
		t.Errorf(fmtWantField, "ID", out.ID, 42)
	}
	if out.ExtractedPath != "group/subgroup/my-project" {
		t.Errorf(fmtWantField, "ExtractedPath", out.ExtractedPath, "group/subgroup/my-project")
	}
}

// TestResolve_InvalidURL verifies that Resolve returns an error when the
// remote URL cannot be parsed.
func TestResolve_InvalidURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Resolve(context.Background(), client, ResolveInput{
		RemoteURL: "",
	})
	if err == nil {
		t.Error("Resolve() expected error for empty URL, got nil")
	}
}

// TestResolve_ProjectNotFound verifies that Resolve returns an actionable error
// when the GitLab API cannot find the project.
func TestResolve_ProjectNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := Resolve(context.Background(), client, ResolveInput{
		RemoteURL: "https://gitlab.example.com/nonexistent/project.git",
	})
	if err == nil {
		t.Error("Resolve() expected error for non-existent project, got nil")
	}
}

// TestResolve_CancelledContext verifies that Resolve respects context cancellation.
func TestResolve_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, testProjectJSON)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Resolve(ctx, client, ResolveInput{
		RemoteURL: "https://gitlab.example.com/group/project.git",
	})
	if err == nil {
		t.Error("Resolve() expected error for cancelled context, got nil")
	}
}

// TestFormatMarkdown_OutputContainsProjectInfo verifies that FormatMarkdown
// produces readable output with key project fields.
func TestFormatMarkdown_OutputContainsProjectInfo(t *testing.T) {
	out := ResolveOutput{
		ID:                42,
		Name:              "my-project",
		PathWithNamespace: "group/my-project",
		WebURL:            "https://gitlab.example.com/group/my-project",
		DefaultBranch:     "main",
		Description:       "Test desc",
		Visibility:        "private",
	}

	md := FormatMarkdown(out)

	checks := []string{
		"42",
		"my-project",
		"group/my-project",
		"https://gitlab.example.com/group/my-project",
		"main",
		"Test desc",
		"private",
	}
	for _, want := range checks {
		if !contains(md, want) {
			t.Errorf("FormatMarkdown() output missing %q", want)
		}
	}
}

// TestFormatMarkdown_NoDescription verifies that FormatMarkdown omits the
// description line when the project has no description.
func TestFormatMarkdown_NoDescription(t *testing.T) {
	out := ResolveOutput{
		ID:                1,
		Name:              "test",
		PathWithNamespace: "g/test",
		WebURL:            "https://example.com/g/test",
		DefaultBranch:     "main",
		Visibility:        "public",
	}

	md := FormatMarkdown(out)
	if contains(md, "Description") {
		t.Error("FormatMarkdown() should omit Description when empty")
	}
}

// contains reports whether substr is found within s.
// Uses a length pre-check before scanning.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

// containsStr performs a brute-force substring search of substr within s.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestParseRemoteURL_NoPath verifies that ParseRemoteURL returns an error
// for URLs that have a host but lack a path component (e.g. "https://gitlab.example.com").
func TestParseRemoteURL_NoPath(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "HTTPS host only", url: "https://gitlab.example.com"},
		{name: "HTTPS host with port only", url: "https://gitlab.example.com:8443"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRemoteURL(tc.url)
			if err == nil {
				t.Errorf("ParseRemoteURL(%q) expected error for URL with no path, got nil", tc.url)
			}
			if err != nil && !strings.Contains(err.Error(), "no path") {
				t.Errorf("ParseRemoteURL(%q) error = %q, want error containing 'no path'", tc.url, err.Error())
			}
		})
	}
}

// TestParseRemoteURL_InvalidURLParse verifies that ParseRemoteURL returns an error
// when url.Parse itself fails on the input (e.g. malformed scheme characters).
func TestParseRemoteURL_InvalidURLParse(t *testing.T) {
	// url.Parse rejects URLs with control characters in the host
	_, err := ParseRemoteURL("https://invalid\x7f host/group/project")
	if err == nil {
		t.Error("ParseRemoteURL() expected error for URL with invalid characters, got nil")
	}
}

// TestResolve_APIError verifies that Resolve wraps non-404 API errors
// (e.g. 401, 403, 500) with actionable context.
func TestResolve_APIError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"message":"401 Unauthorized"}`,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"message":"403 Forbidden"}`,
		},
		{
			name:       "500 internal server error",
			statusCode: http.StatusForbidden,
			body:       `{"message":"500 Internal Server Error"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.statusCode, tc.body)
			}))

			_, err := Resolve(context.Background(), client, ResolveInput{
				RemoteURL: "https://gitlab.example.com/group/project.git",
			})
			if err == nil {
				t.Fatalf("Resolve() expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "not found on GitLab") {
				t.Errorf("Resolve() error = %q, want it to contain 'not found on GitLab'", err.Error())
			}
		})
	}
}

// TestResolve_AllOutputFields verifies that every field in ResolveOutput
// is correctly populated from the GitLab API response.
func TestResolve_AllOutputFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, testProjectJSON)
	}))

	out, err := Resolve(context.Background(), client, ResolveInput{
		RemoteURL: "git@gitlab.example.com:group/subgroup/my-project.git",
	})
	if err != nil {
		t.Fatalf(fmtResolveErr, err)
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"ID", out.ID, int64(42)},
		{"Name", out.Name, "my-project"},
		{"Path", out.Path, "my-project"},
		{"PathWithNamespace", out.PathWithNamespace, "group/subgroup/my-project"},
		{"WebURL", out.WebURL, "https://gitlab.example.com/group/subgroup/my-project"},
		{"DefaultBranch", out.DefaultBranch, "main"},
		{"Description", out.Description, "A test project"},
		{"Visibility", out.Visibility, "private"},
		{"HTTPURLToRepo", out.HTTPURLToRepo, "https://gitlab.example.com/group/subgroup/my-project.git"},
		{"SSHURLToRepo", out.SSHURLToRepo, "git@gitlab.example.com:group/subgroup/my-project.git"},
		{"ExtractedPath", out.ExtractedPath, "group/subgroup/my-project"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf(fmtWantField, c.field, c.got, c.want)
		}
	}
}

// TestFormatMarkdown_ContainsHints verifies that FormatMarkdown includes
// the WriteHints guidance for subsequent tool calls.
func TestFormatMarkdown_ContainsHints(t *testing.T) {
	out := ResolveOutput{
		ID:                99,
		Name:              "hints-test",
		PathWithNamespace: "g/hints-test",
		WebURL:            "https://example.com/g/hints-test",
		DefaultBranch:     "develop",
		Visibility:        "internal",
	}

	md := FormatMarkdown(out)
	if !strings.Contains(md, "project_id") {
		t.Error("FormatMarkdown() output missing project_id guidance")
	}
	if !strings.Contains(md, "99") {
		t.Error("FormatMarkdown() output missing numeric project ID")
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools registers the tool
// on the MCP server without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// newMCPSession creates an in-memory MCP client session with the project
// discovery tool registered, backed by the provided HTTP handler.
func newMCPSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestRegisterTools_CallThroughMCP validates that the registered tool can be
// called through the MCP protocol and returns the expected project data.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, testProjectJSON)
	})

	session := newMCPSession(t, handler)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_resolve_project_from_remote",
		Arguments: map[string]any{
			"remote_url": "https://gitlab.example.com/group/subgroup/my-project.git",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() transport error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool() returned IsError=true")
	}
	// Verify output contains expected project data
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if !strings.Contains(tc.Text, "my-project") {
				t.Errorf("CallTool() response missing project name, got: %s", tc.Text)
			}
			return
		}
	}
	t.Error("CallTool() response has no TextContent")
}

// TestRegisterTools_CallThroughMCP_Error validates that the registered tool
// returns IsError=true when the GitLab API returns an error.
func TestRegisterTools_CallThroughMCP_Error(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})

	session := newMCPSession(t, handler)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_resolve_project_from_remote",
		Arguments: map[string]any{
			"remote_url": "https://gitlab.example.com/group/project.git",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() expected IsError=true for API error")
	}
}

// TestRegisterTools_CallThroughMCP_InvalidInput validates that the tool
// returns IsError=true when given an invalid (empty) remote URL.
func TestRegisterTools_CallThroughMCP_InvalidInput(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_resolve_project_from_remote",
		Arguments: map[string]any{
			"remote_url": "",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() expected IsError=true for empty URL input")
	}
}
