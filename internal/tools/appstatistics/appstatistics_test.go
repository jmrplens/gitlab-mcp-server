// appstatistics_test.go contains unit tests for the application statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package appstatistics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/statistics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"forks": 10, "issues": 200, "merge_requests": 50,
			"notes": 1000, "snippets": 5, "ssh_keys": 30,
			"milestones": 15, "users": 100, "groups": 8,
			"projects": 45, "active_users": 80
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ActiveUsers != 80 {
		t.Errorf("ActiveUsers = %d, want 80", out.ActiveUsers)
	}
	if out.Projects != 45 {
		t.Errorf("Projects = %d, want 45", out.Projects)
	}
	if out.Issues != 200 {
		t.Errorf("Issues = %d, want 200", out.Issues)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{ActiveUsers: 80, Projects: 45, Issues: 200}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "Application Statistics") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "80") {
		t.Error("missing active users")
	}
	if !strings.Contains(md, "45") {
		t.Error("missing projects")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const covStatsJSON = `{"forks":10,"issues":20,"merge_requests":30,"notes":40,"snippets":5,"ssh_keys":3,"milestones":7,"users":100,"groups":15,"projects":50,"active_users":80}`

// TestGet_APIError_Coverage verifies the behavior of cov get a p i error.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success_Coverage verifies the behavior of cov get success.
func TestGet_Success_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
	}))
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Projects != 50 || out.ActiveUsers != 80 {
		t.Errorf("unexpected: %+v", out)
	}
}

// TestFormatGetMarkdown_Cov_Coverage verifies the behavior of cov format get markdown.
func TestFormatGetMarkdown_Cov_Coverage(t *testing.T) {
	out := GetOutput{Projects: 50, ActiveUsers: 80, Users: 100, Issues: 20}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "50") || !strings.Contains(md, "80") {
		t.Error("expected stats in markdown")
	}
}

// TestRegisterTools_NoPanic_Coverage verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
	}))
	RegisterTools(server, client)
}

// TestMCPRound_Trip_Coverage verifies the behavior of cov m c p round trip.
func TestMCPRound_Trip_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_get_application_statistics",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestMCPRoundTrip_Error validates the register.go error path for the
// application statistics tool via MCP round-trip against a 500 backend.
func TestMCPRoundTrip_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_get_application_statistics",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true")
	}
}
