// license_test.go contains unit tests for the GitLab license MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package license

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

const licenseJSON = `{
	"id": 1,
	"plan": "premium",
	"created_at": "2024-01-01T00:00:00Z",
	"starts_at": "2024-01-01",
	"expires_at": "2025-01-01",
	"historical_max": 100,
	"maximum_user_count": 50,
	"expired": false,
	"overage": 0,
	"user_limit": 100,
	"active_users": 42,
	"licensee": {"Name":"John","Company":"Acme","Email":"john@acme.com"},
	"add_ons": {"GitLab_Auditor_User":1,"GitLab_DeployBoard":0,"GitLab_FileLocks":1,"GitLab_Geo":0,"GitLab_ServiceDesk":1}
}`

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/license" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, licenseJSON)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.License.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.License.ID)
	}
	if out.License.Plan != "premium" {
		t.Errorf("expected premium, got %s", out.License.Plan)
	}
	if out.License.ActiveUsers != 42 {
		t.Errorf("expected 42 active users, got %d", out.License.ActiveUsers)
	}
	if out.License.Licensee.Name != "John" {
		t.Errorf("expected John, got %s", out.License.Licensee.Name)
	}
}

// TestGet_Error verifies the behavior of get error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestAdd_Success verifies the behavior of add success.
func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, licenseJSON)
	}))

	out, err := Add(t.Context(), client, AddInput{License: "base64encodedlicense"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.License.Plan != "premium" {
		t.Errorf("expected premium, got %s", out.License.Plan)
	}
}

// TestDelete_Success verifies the behavior of delete success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/license/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_InvalidID verifies the behavior of delete invalid i d.
func TestDelete_InvalidID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, id := range []int64{0, -1} {
		err := Delete(t.Context(), client, DeleteInput{ID: id})
		if err == nil {
			t.Errorf("expected error for ID %d", id)
		}
	}
}

// TestDelete_Error verifies the behavior of delete error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatLicenseMarkdown verifies the behavior of format license markdown.
func TestFormatLicenseMarkdown(t *testing.T) {
	result := FormatLicenseMarkdown(Item{
		ID:          1,
		Plan:        "premium",
		ActiveUsers: 42,
		UserLimit:   100,
		Expired:     false,
		Licensee:    LicenseeItem{Name: "John", Company: "Acme", Email: "john@acme.com"},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "premium") {
		t.Errorf("expected premium in output, got: %s", text)
	}
	if !strings.Contains(text, "John") {
		t.Errorf("expected John in output, got: %s", text)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Add — API error
// ---------------------------------------------------------------------------.

// TestAdd_Error verifies the behavior of add error.
func TestAdd_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"invalid license"}`)
	}))
	_, err := Add(t.Context(), client, AddInput{License: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatLicenseMarkdown — with dates
// ---------------------------------------------------------------------------.

// TestFormatLicenseMarkdown_WithDates verifies the behavior of format license markdown with dates.
func TestFormatLicenseMarkdown_WithDates(t *testing.T) {
	item := Item{
		ID:               2,
		Plan:             "ultimate",
		StartsAt:         "2024-01-01",
		ExpiresAt:        "2025-12-31",
		CreatedAt:        "2024-01-01T00:00:00Z",
		ActiveUsers:      100,
		UserLimit:        200,
		MaximumUserCount: 150,
		HistoricalMax:    120,
		Overage:          5,
		Expired:          true,
		Licensee:         LicenseeItem{Name: "Jane", Company: "Corp", Email: "jane@corp.com"},
	}
	result := FormatLicenseMarkdown(item)
	text := result.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"ultimate", "1 Jan 2024", "31 Dec 2025", "Jane", "Corp", "true"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in markdown", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown / FormatAddMarkdown — wrappers
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_Coverage verifies the behavior of format get markdown coverage.
func TestFormatGetMarkdown_Coverage(t *testing.T) {
	out := GetOutput{License: Item{ID: 1, Plan: "premium", Licensee: LicenseeItem{Name: "A", Company: "B", Email: "c@d.com"}}}
	result := FormatGetMarkdown(out)
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "premium") {
		t.Error("missing plan in output")
	}
}

// TestFormatAddMarkdown_Coverage verifies the behavior of format add markdown coverage.
func TestFormatAddMarkdown_Coverage(t *testing.T) {
	out := AddOutput{License: Item{ID: 3, Plan: "gold", Licensee: LicenseeItem{Name: "X", Company: "Y", Email: "x@y.com"}}}
	result := FormatAddMarkdown(out)
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "gold") {
		t.Error("missing plan in output")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip validates m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	session := newLicenseMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_get_license", map[string]any{}},
		{"add", "gitlab_add_license", map[string]any{"license": "base64data"}},
		{"delete", "gitlab_delete_license", map[string]any{"id": float64(1)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// newLicenseMCPSession is an internal helper for the license package.
func newLicenseMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/license", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, licenseJSON)
	})
	handler.HandleFunc("POST /api/v4/license", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, licenseJSON)
	})
	handler.HandleFunc("DELETE /api/v4/license/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

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

// TestToItem_NilDates verifies that toItem produces empty date strings when the
// GitLab license has nil CreatedAt, StartsAt, and ExpiresAt fields.
func TestToItem_NilDates(t *testing.T) {
	lic := &gl.License{
		ID:   99,
		Plan: "free",
		Licensee: gl.LicenseLicensee{
			Name:    "Test",
			Company: "Co",
			Email:   "t@co.com",
		},
	}
	item := toItem(lic)
	if item.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", item.CreatedAt)
	}
	if item.StartsAt != "" {
		t.Errorf("expected empty StartsAt, got %q", item.StartsAt)
	}
	if item.ExpiresAt != "" {
		t.Errorf("expected empty ExpiresAt, got %q", item.ExpiresAt)
	}
}

// TestToItem_WithDates verifies that toItem correctly formats non-nil date
// pointers from the GitLab license struct.
func TestToItem_WithDates(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	isoStart := gl.ISOTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	isoEnd := gl.ISOTime(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC))

	lic := &gl.License{
		ID:        1,
		Plan:      "premium",
		CreatedAt: &now,
		StartsAt:  &isoStart,
		ExpiresAt: &isoEnd,
		Licensee: gl.LicenseLicensee{
			Name:    "Name",
			Company: "Corp",
			Email:   "e@corp.com",
		},
	}
	item := toItem(lic)
	if item.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if !strings.Contains(item.CreatedAt, "2024") {
		t.Errorf("expected year 2024 in CreatedAt, got %q", item.CreatedAt)
	}
	if item.StartsAt == "" {
		t.Error("expected non-empty StartsAt")
	}
	if item.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
}
