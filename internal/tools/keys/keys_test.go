// keys_test.go contains unit tests for the SSH key MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package keys

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestGetKeyWithUser_Success verifies that GetKeyWithUser handles the success scenario correctly.
func TestGetKeyWithUser_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/keys/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":42,"title":"My Key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z","user":{"id":1,"username":"admin","name":"Admin"}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.User.Username != "admin" {
		t.Errorf("user = %q, want %q", out.User.Username, "admin")
	}
}

// TestGetKeyWithUser_MissingID verifies that GetKeyWithUser handles the missing i d scenario correctly.
func TestGetKeyWithUser_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{})
	if err == nil {
		t.Fatal("expected error for missing key_id")
	}
}

// TestGetKeyByFingerprint_Success verifies that GetKeyByFingerprint handles the success scenario correctly.
func TestGetKeyByFingerprint_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fingerprint") != "SHA256:abc123" {
			t.Errorf("unexpected fingerprint param: %s", r.URL.Query().Get("fingerprint"))
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":10,"title":"Deploy Key","key":"ssh-rsa BBBB...","user":{"id":2,"username":"deploy","name":"Deploy"}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{Fingerprint: "SHA256:abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
}

// TestGetKeyByFingerprint_MissingFingerprint verifies that GetKeyByFingerprint handles the missing fingerprint scenario correctly.
func TestGetKeyByFingerprint_MissingFingerprint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{})
	if err == nil {
		t.Fatal("expected error for missing fingerprint")
	}
}

// TestGetKeyWithUser_APIError verifies that GetKeyWithUser handles the a p i error scenario correctly.
func TestGetKeyWithUser_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 99})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestFormatMarkdownString verifies the behavior of format markdown string.
func TestFormatMarkdownString(t *testing.T) {
	out := Output{
		ID:    1,
		Title: "Test Key",
		Key:   "ssh-rsa AAAA...",
		User:  UserOutput{ID: 1, Username: "user", Name: "User"},
	}
	md := FormatMarkdownString(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// GetKeyByFingerprint — API error
// ---------------------------------------------------------------------------.

// TestGetKeyByFingerprint_APIError verifies the behavior of get key by fingerprint a p i error.
func TestGetKeyByFingerprint_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{Fingerprint: "SHA256:abc123"})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// ---------------------------------------------------------------------------
// toOutput — CreatedAt populated and nil
// ---------------------------------------------------------------------------.

// TestToOutput_WithCreatedAt verifies the behavior of to output with created at.
func TestToOutput_WithCreatedAt(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	key := &gl.Key{
		ID:        1,
		Title:     "Test",
		Key:       "ssh-rsa AAAA...",
		CreatedAt: &now,
		User: gl.User{
			ID:       10,
			Username: "tester",
			Name:     "Test User",
		},
	}

	out := toOutput(key)

	if out.CreatedAt == "" {
		t.Fatal("expected non-empty CreatedAt")
	}
	if !strings.Contains(out.CreatedAt, "2026") {
		t.Errorf("CreatedAt = %q, expected to contain 2026", out.CreatedAt)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.User.Username != "tester" {
		t.Errorf("User.Username = %q, want %q", out.User.Username, "tester")
	}
}

// TestToOutput_NilCreatedAt verifies the behavior of to output nil created at.
func TestToOutput_NilCreatedAt(t *testing.T) {
	key := &gl.Key{
		ID:    2,
		Title: "No Date",
		Key:   "ssh-ed25519 AAAA",
		User: gl.User{
			ID:       20,
			Username: "nodate",
			Name:     "No Date User",
		},
	}

	out := toOutput(key)

	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — branch coverage
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_WithCreatedAt verifies the behavior of format markdown string with created at.
func TestFormatMarkdownString_WithCreatedAt(t *testing.T) {
	out := Output{
		ID:        1,
		Title:     "My Key",
		Key:       "ssh-rsa short",
		CreatedAt: "2026-01-01T00:00:00Z",
		User:      UserOutput{ID: 1, Username: "admin", Name: "Admin"},
	}

	md := FormatMarkdownString(out)

	if !strings.Contains(md, "**Created**") {
		t.Error("expected markdown to contain Created field")
	}
	if !strings.Contains(md, "1 Jan 2026 00:00 UTC") {
		t.Error("expected markdown to contain the date value")
	}
}

// TestFormatMarkdownString_EmptyTitle verifies the behavior of format markdown string empty title.
func TestFormatMarkdownString_EmptyTitle(t *testing.T) {
	out := Output{
		ID:   3,
		Key:  "ssh-rsa short",
		User: UserOutput{ID: 1, Username: "u", Name: "U"},
	}

	md := FormatMarkdownString(out)

	if strings.Contains(md, "**Title**") {
		t.Error("expected no Title line when title is empty")
	}
}

// TestFormatMarkdownString_LongKey verifies the behavior of format markdown string long key.
func TestFormatMarkdownString_LongKey(t *testing.T) {
	longKey := strings.Repeat("A", 100)
	out := Output{
		ID:    4,
		Title: "Long",
		Key:   longKey,
		User:  UserOutput{ID: 1, Username: "u", Name: "U"},
	}

	md := FormatMarkdownString(out)

	if !strings.Contains(md, "...") {
		t.Error("expected truncated key with ellipsis in markdown")
	}
	if strings.Contains(md, longKey) {
		t.Error("expected key to be truncated, but found full key")
	}
}

// TestFormatMarkdownString_ShortKey verifies the behavior of format markdown string short key.
func TestFormatMarkdownString_ShortKey(t *testing.T) {
	shortKey := "ssh-rsa AAAA"
	out := Output{
		ID:    5,
		Title: "Short",
		Key:   shortKey,
		User:  UserOutput{ID: 1, Username: "u", Name: "U"},
	}

	md := FormatMarkdownString(out)

	if !strings.Contains(md, shortKey) {
		t.Error("expected full short key in markdown")
	}
	if strings.Contains(md, "...") {
		t.Error("short key should not be truncated")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — returns non-nil CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_ReturnsResult verifies the behavior of format markdown returns result.
func TestFormatMarkdown_ReturnsResult(t *testing.T) {
	out := Output{
		ID:    1,
		Title: "Test",
		Key:   "ssh-rsa AAAA",
		User:  UserOutput{ID: 1, Username: "u", Name: "U"},
	}

	result := FormatMarkdown(out)

	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
}

// ---------------------------------------------------------------------------
// truncateKey — direct tests
// ---------------------------------------------------------------------------.

// TestTruncateKey_LongKey verifies the behavior of truncate key long key.
func TestTruncateKey_LongKey(t *testing.T) {
	long := strings.Repeat("X", 80)
	got := truncateKey(long)

	if len(got) != 60 {
		t.Errorf("truncated length = %d, want 60", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected ellipsis suffix")
	}
	if got[:57] != long[:57] {
		t.Error("expected first 57 chars to match")
	}
}

// TestTruncateKey_ExactBoundary verifies the behavior of truncate key exact boundary.
func TestTruncateKey_ExactBoundary(t *testing.T) {
	exactly60 := strings.Repeat("Y", 60)
	got := truncateKey(exactly60)

	if got != exactly60 {
		t.Errorf("key of exactly 60 chars should not be truncated")
	}
}

// TestTruncateKey_ShortKey verifies the behavior of truncate key short key.
func TestTruncateKey_ShortKey(t *testing.T) {
	short := "ssh-rsa AAAA"
	got := truncateKey(short)

	if got != short {
		t.Errorf("short key should not be truncated, got %q", got)
	}
}

// TestTruncateKey_Empty verifies the behavior of truncate key empty.
func TestTruncateKey_Empty(t *testing.T) {
	got := truncateKey("")
	if got != "" {
		t.Errorf("empty key should stay empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)

	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)

	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterTools — call all tools through MCP
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v4/keys/"):
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":42,"title":"MCP Key","key":"ssh-rsa AAAA...","created_at":"2026-06-01T12:00:00Z","user":{"id":1,"username":"admin","name":"Admin"}}`)
		case r.URL.Path == "/api/v4/keys" && r.URL.Query().Get("fingerprint") != "":
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":99,"title":"FP Key","key":"ssh-ed25519 BBBB...","user":{"id":2,"username":"deploy","name":"Deploy"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, handler)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(context.Background(), st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{
			name: "get_key_with_user",
			tool: "gitlab_get_key_with_user",
			args: map[string]any{"key_id": 42},
		},
		{
			name: "get_key_by_fingerprint",
			tool: "gitlab_get_key_by_fingerprint",
			args: map[string]any{"fingerprint": "SHA256:abc123"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.tool,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tc.tool, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tc.tool)
			}
			if len(result.Content) == 0 {
				t.Errorf("CallTool(%s) returned empty content", tc.tool)
			}
		})
	}
}
