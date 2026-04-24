// avatar_test.go contains unit tests for the avatar MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package avatar

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/avatar" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://example.com/avatar.png"}`)
	}))
	out, err := Get(t.Context(), client, GetInput{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AvatarURL != "https://example.com/avatar.png" {
		t.Errorf("unexpected avatar URL: %s", out.AvatarURL)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{Email: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatMarkdown verifies the behavior of format markdown.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown(GetOutput{AvatarURL: "https://example.com/avatar.png"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestGet_APIError_Coverage verifies the behavior of cov get a p i error.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{Email: "a@b.c"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success_Coverage verifies the behavior of cov get success.
func TestGet_Success_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://img.example.com/a.png"}`)
	}))
	out, err := Get(t.Context(), client, GetInput{Email: "a@b.c", Size: 100})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.AvatarURL != "https://img.example.com/a.png" {
		t.Errorf("unexpected URL: %s", out.AvatarURL)
	}
}

// TestFormatMarkdown_Coverage verifies the behavior of cov format markdown.
func TestFormatMarkdown_Coverage(t *testing.T) {
	md := FormatMarkdown(GetOutput{AvatarURL: "https://img.example.com/a.png"})
	if !strings.Contains(md, "https://img.example.com/a.png") {
		t.Error("expected avatar URL in markdown")
	}
}

// TestRegisterTools_NoPanic_Coverage verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"x"}`)
	}))
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic_Coverage verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"x"}`)
	}))
	RegisterMeta(server, client)
}

// TestMCPRound_Trip_Coverage verifies the behavior of cov m c p round trip.
func TestMCPRound_Trip_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://x.com/a.png"}`)
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
		Name:      "gitlab_get_avatar",
		Arguments: map[string]any{"email": "a@b.c", "size": float64(100)},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestMCPRoundTrip_ErrorPath covers the error return path in register.go
// when the GitLab API returns an error.
func TestMCPRoundTrip_ErrorPath(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_get_avatar",
		Arguments: map[string]any{"email": "a@b.c"},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for 500 backend")
	}
}
