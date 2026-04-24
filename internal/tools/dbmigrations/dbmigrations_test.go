// dbmigrations_test.go contains unit tests for the database migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package dbmigrations

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestMark verifies the behavior of mark.
func TestMark(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/admin/migrations/20240115100000/mark" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Mark(t.Context(), client, MarkInput{
		Version:  20240115100000,
		Database: "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "marked" {
		t.Errorf("Status = %q, want marked", out.Status)
	}
	if out.Version != 20240115100000 {
		t.Errorf("Version = %d, want 20240115100000", out.Version)
	}
}

// TestMark_Error verifies the behavior of mark error.
func TestMark_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Mark(t.Context(), client, MarkInput{Version: 99999})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestMark_VersionValidation validates mark version validation across multiple scenarios using table-driven subtests.
func TestMark_VersionValidation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when version is invalid")
	}))
	ctx := context.Background()

	tests := []struct {
		name    string
		version int64
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Mark(ctx, client, MarkInput{Version: tt.version})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "version") {
				t.Errorf("error %q does not contain %q", err.Error(), "version")
			}
		})
	}
}

// TestFormatMarkMarkdown verifies the behavior of format mark markdown.
func TestFormatMarkMarkdown(t *testing.T) {
	md := FormatMarkMarkdown(MarkOutput{Status: "marked", Version: 20240115100000})
	if !strings.Contains(md, "marked") {
		t.Error("missing status")
	}
	if !strings.Contains(md, "20240115100000") {
		t.Error("missing version")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestRegisterTools_NoPanic_Coverage verifies the behavior of cov register tools no panic (from coverage_test.go).
func TestRegisterTools_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	RegisterTools(server, client)
}

// TestMCPRound_Trip_Coverage verifies the behavior of cov m c p round trip (from coverage_test.go).
func TestMCPRound_Trip_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
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
		Name:      "gitlab_mark_migration",
		Arguments: map[string]any{"version": float64(20240115100000)},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestMCPRoundTrip_MarkError validates the register.go error path for
// gitlab_mark_migration when the GitLab API returns 500.
func TestMCPRoundTrip_MarkError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
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
		Name:      "gitlab_mark_migration",
		Arguments: map[string]any{"version": float64(99999)},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for API error")
	}
}
