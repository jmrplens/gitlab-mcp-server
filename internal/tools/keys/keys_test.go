// keys_test.go contains unit tests for the SSH key MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package keys

import (
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestGetKeyWithUser_Success verifies GetKeyWithUser when success.
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

// TestGetKeyWithUser_MissingID verifies GetKeyWithUser when missing ID.
func TestGetKeyWithUser_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{})
	if err == nil {
		t.Fatal("expected error for missing key_id")
	}
}

// TestGetKeyByFingerprint_Success verifies GetKeyByFingerprint when success.
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

// TestGetKeyByFingerprint_MissingFingerprint verifies GetKeyByFingerprint when missing fingerprint.
func TestGetKeyByFingerprint_MissingFingerprint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{})
	if err == nil {
		t.Fatal("expected error for missing fingerprint")
	}
}

// TestGetKeyWithUser_APIError verifies GetKeyWithUser when API error.
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

// TestFormatMarkdownString verifies FormatMarkdownString.
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

// TestGetKeyByFingerprint_APIError verifies GetKeyByFingerprint when API error.
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

// TestToOutput_WithCreatedAt verifies ToOutput when with created at.
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

// TestToOutput_NilCreatedAt verifies ToOutput when nil created at.
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

// TestFormatMarkdownString_WithCreatedAt verifies FormatMarkdownString when with created at.
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

// TestFormatMarkdownString_EmptyTitle verifies FormatMarkdownString when empty title.
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

// TestFormatMarkdownString_LongKey verifies FormatMarkdownString when long key.
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

// TestFormatMarkdownString_ShortKey verifies FormatMarkdownString when short key.
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

// TestFormatMarkdown_ReturnsResult verifies FormatMarkdown returns result.
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

// TestTruncateKey_LongKey verifies TruncateKey when long key.
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

// TestTruncateKey_ExactBoundary verifies TruncateKey when exact boundary.
func TestTruncateKey_ExactBoundary(t *testing.T) {
	exactly60 := strings.Repeat("Y", 60)
	got := truncateKey(exactly60)

	if got != exactly60 {
		t.Errorf("key of exactly 60 chars should not be truncated")
	}
}

// TestTruncateKey_ShortKey verifies TruncateKey when short key.
func TestTruncateKey_ShortKey(t *testing.T) {
	short := "ssh-rsa AAAA"
	got := truncateKey(short)

	if got != short {
		t.Errorf("short key should not be truncated, got %q", got)
	}
}

// TestTruncateKey_Empty verifies TruncateKey when empty.
func TestTruncateKey_Empty(t *testing.T) {
	got := truncateKey("")
	if got != "" {
		t.Errorf("empty key should stay empty, got %q", got)
	}
}

// TestActionSpecs_Metadata verifies key action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "keys" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates all key canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
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
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
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
			spec, ok := specByTool[tc.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.tool)
			}
		})
	}
}
