// health_test.go contains unit tests for the server health MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package health

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pathVersion       = "/api/v4/version"
	pathCurrentUser   = "/api/v4/user"
	fmtStatusCheckErr = "Check() unexpected error: %v"
	fmtStatusWant     = "Status = %q, want %q"
	testGitLabVersion = "17.5.0"
)

// TestCheck_Healthy verifies the behavior of check healthy.
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

// TestSetServerInfo_DefaultsEmpty verifies that when SetServerInfo is not called
// (or called with zero value), the metadata fields remain empty.
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

// TestCheck_UnhealthyVersionFails verifies the behavior of check unhealthy version fails.
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

// TestCheck_DegradedUserFails verifies the behavior of check degraded user fails.
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

// TestCheck_CancelledContext verifies the behavior of check cancelled context.
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

// TestFormatMarkdownString_Healthy validates cov format markdown string healthy across multiple scenarios using table-driven subtests.
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
	md := FormatMarkdownString(out)

	checks := []struct {
		name, want string
	}{
		{"status emoji", "\u2705"},
		{"status text", "healthy"},
		{"mcp version", "1.0.0"},
		{"author", "Test Author"},
		{"department", "Test Dept"},
		{"repository", "https://example.com/repo"},
		{"url", "https://gitlab.example.com"},
		{"version", "17.5.0"},
		{"revision", "abc123"},
		{"auth", "true"},
		{"username", "alice"},
		{"user id", "42"},
		{"response time", "15 ms"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("FormatMarkdownString healthy missing %s: want substring %q", c.name, c.want)
		}
	}
	if strings.Contains(md, "Error") {
		t.Error("healthy status should not contain Error section")
	}
}

// TestFormatMarkdownString_WithMetadata verifies that metadata fields
// (MCP Server Version, Author, Department, Repository) appear when set.
func TestFormatMarkdownString_WithMetadata(t *testing.T) {
	out := Output{
		Status:           "healthy",
		MCPServerVersion: "2.3.4",
		Author:           "Test Author",
		Department:       "Test Department",
		Repository:       "https://github.com/jmrplens/gitlab-mcp-server",
		GitLabURL:        "https://gitlab.example.com",
	}
	md := FormatMarkdownString(out)

	for _, want := range []string{
		"**MCP Server Version**: 2.3.4",
		"**Author**: Test Author",
		"**Department**: Test Department",
		"**Repository**: https://github.com/jmrplens/gitlab-mcp-server",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown", want)
		}
	}
}

// TestFormatMarkdownString_WithoutMetadata verifies that metadata labels
// are omitted when the fields are empty.
func TestFormatMarkdownString_WithoutMetadata(t *testing.T) {
	out := Output{
		Status:    "healthy",
		GitLabURL: "https://gitlab.example.com",
	}
	md := FormatMarkdownString(out)

	for _, unwanted := range []string{
		"MCP Server Version",
		"**Author**",
		"**Department**",
		"**Repository**",
	} {
		if strings.Contains(md, unwanted) {
			t.Errorf("should not contain %q when field is empty", unwanted)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — unhealthy
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_Unhealthy verifies the behavior of cov format markdown string unhealthy.
func TestFormatMarkdownString_Unhealthy(t *testing.T) {
	out := Output{
		Status:         "unhealthy",
		GitLabURL:      "https://gitlab.example.com",
		ResponseTimeMS: 100,
		Error:          "connectivity check failed: connection refused",
	}
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "\u274c") {
		t.Error("unhealthy should have cross mark emoji")
	}
	if !strings.Contains(md, "unhealthy") {
		t.Error("missing 'unhealthy' status text")
	}
	if !strings.Contains(md, "connectivity check failed") {
		t.Error("missing error message")
	}
	if strings.Contains(md, "Version") {
		t.Error("unhealthy should not show version")
	}
	if strings.Contains(md, "User") {
		t.Error("unhealthy should not show user")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — degraded
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_Degraded verifies the behavior of cov format markdown string degraded.
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
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "\u26a0\ufe0f") {
		t.Error("degraded should have warning emoji")
	}
	if !strings.Contains(md, "degraded") {
		t.Error("missing 'degraded' status text")
	}
	if !strings.Contains(md, "user retrieval failed") {
		t.Error("missing error message")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — no username (empty)
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_NoUsername verifies the behavior of cov format markdown string no username.
func TestFormatMarkdownString_NoUsername(t *testing.T) {
	out := Output{
		Status:    "healthy",
		GitLabURL: "https://gitlab.example.com",
	}
	md := FormatMarkdownString(out)
	if strings.Contains(md, "**User**") {
		t.Error("should not show User when username is empty")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — no version (empty)
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_NoVersion verifies the behavior of cov format markdown string no version.
func TestFormatMarkdownString_NoVersion(t *testing.T) {
	out := Output{
		Status:    "unhealthy",
		GitLabURL: "https://gitlab.example.com",
		Error:     "failed",
	}
	md := FormatMarkdownString(out)
	if strings.Contains(md, "**Version**") {
		t.Error("should not show Version when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown wrapper
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Wrapper verifies the behavior of cov format markdown wrapper.
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
// RegisterTools no-panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"version":"17.5.0","revision":"abc"}`)
	}))
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — gitlab_server_status
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip verifies the behavior of cov m c p round trip.
func TestMCPRound_Trip(t *testing.T) {
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

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_server_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool gitlab_server_status: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.IsError {
		t.Error("expected no error")
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — gitlab_server_status unhealthy (API error)
// ---------------------------------------------------------------------------.

// TestMCPRound_TripUnhealthy verifies the behavior of cov m c p round trip unhealthy.
func TestMCPRound_TripUnhealthy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_server_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool gitlab_server_status: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}
