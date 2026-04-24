// appearance_test.go contains unit tests for the GitLab instance appearance MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package appearance

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const appearanceJSON = `{
	"title": "GitLab CE",
	"description": "Open source self-hosted Git management",
	"pwa_name": "GitLab",
	"pwa_short_name": "GL",
	"pwa_description": "Code hosting",
	"pwa_icon": "",
	"logo": "/uploads/logo.png",
	"header_logo": "/uploads/header.png",
	"favicon": "/uploads/favicon.ico",
	"member_guidelines": "Be nice",
	"new_project_guidelines": "Follow naming conventions",
	"profile_image_guidelines": "Use a real photo",
	"header_message": "Welcome",
	"footer_message": "Goodbye",
	"message_background_color": "#e75e40",
	"message_font_color": "#ffffff",
	"email_header_and_footer_enabled": true
}`

// TestGet_Success verifies that Get handles the success scenario correctly.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/appearance" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Appearance.Title != "GitLab CE" {
		t.Errorf("expected title 'GitLab CE', got %q", out.Appearance.Title)
	}
	if !out.Appearance.EmailHeaderAndFooterEnabled {
		t.Error("expected email_header_and_footer_enabled=true")
	}
	if out.Appearance.HeaderMessage != "Welcome" {
		t.Errorf("expected header_message 'Welcome', got %q", out.Appearance.HeaderMessage)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_Success verifies that Update handles the success scenario correctly.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/appearance" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
			return
		}
		http.NotFound(w, r)
	}))

	enabled := true
	out, err := Update(t.Context(), client, UpdateInput{
		Title:                       "New Title",
		HeaderMessage:               "New Header",
		EmailHeaderAndFooterEnabled: &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Appearance.Title != "GitLab CE" {
		t.Errorf("expected title from response, got %q", out.Appearance.Title)
	}
}

// TestUpdate_Error verifies that Update handles the error scenario correctly.
func TestUpdate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(t.Context(), client, UpdateInput{Title: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		Appearance: Item{
			Title:                       "GitLab CE",
			Description:                 "Test instance",
			HeaderMessage:               "Welcome",
			EmailHeaderAndFooterEnabled: true,
		},
	}
	result := FormatGetMarkdown(out)
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "Application Appearance") {
		t.Error("expected 'Application Appearance' header")
	}
	if !strings.Contains(content, "GitLab CE") {
		t.Error("expected title in markdown")
	}
	if !strings.Contains(content, "Welcome") {
		t.Error("expected header message in markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Update — all optional fields populated
// ---------------------------------------------------------------------------.

// TestUpdate_AllFields verifies the behavior of update all fields.
func TestUpdate_AllFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/appearance" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
			return
		}
		http.NotFound(w, r)
	}))

	enabled := true
	out, err := Update(t.Context(), client, UpdateInput{
		Title:                       "New Title",
		Description:                 "New Desc",
		PWAName:                     "MyApp",
		PWAShortName:                "MA",
		PWADescription:              "Progressive",
		HeaderMessage:               "Header",
		FooterMessage:               "Footer",
		MessageBackgroundColor:      "#000000",
		MessageFontColor:            "#ffffff",
		EmailHeaderAndFooterEnabled: &enabled,
		MemberGuidelines:            "Be kind",
		NewProjectGuidelines:        "Name it well",
		ProfileImageGuidelines:      "Use a face",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Appearance.Title != "GitLab CE" {
		t.Errorf("expected response title, got %q", out.Appearance.Title)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — with PWA fields
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_WithPWA verifies the behavior of format get markdown with p w a.
func TestFormatGetMarkdown_WithPWA(t *testing.T) {
	out := GetOutput{
		Appearance: Item{
			Title:         "Test",
			PWAName:       "TestPWA",
			PWAShortName:  "TP",
			FooterMessage: "bye",
		},
	}
	result := FormatGetMarkdown(out)
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "PWA Name") {
		t.Error("expected PWA Name in markdown")
	}
	if !strings.Contains(text, "PWA Short Name") {
		t.Error("expected PWA Short Name in markdown")
	}
	if !strings.Contains(text, "Footer Message") {
		t.Error("expected Footer Message in markdown")
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — empty fields (no optional PWA/messages)
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_Minimal verifies the behavior of format get markdown minimal.
func TestFormatGetMarkdown_Minimal(t *testing.T) {
	out := GetOutput{
		Appearance: Item{
			Title: "Minimal",
		},
	}
	result := FormatGetMarkdown(out)
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Minimal") {
		t.Error("expected title in markdown")
	}
	if strings.Contains(text, "PWA Name") {
		t.Error("should not contain PWA Name when empty")
	}
	if strings.Contains(text, "Header Message") {
		t.Error("should not contain Header Message when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatUpdateMarkdown
// ---------------------------------------------------------------------------.

// TestFormatUpdateMarkdown_Coverage verifies the behavior of format update markdown coverage.
func TestFormatUpdateMarkdown_Coverage(t *testing.T) {
	out := UpdateOutput{
		Appearance: Item{
			Title:       "Updated",
			Description: "Updated desc",
		},
	}
	result := FormatUpdateMarkdown(out)
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Updated") {
		t.Error("expected title in markdown")
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
	session := newAppearanceMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_appearance", "gitlab_get_appearance", map[string]any{}},
		{"update_appearance", "gitlab_update_appearance", map[string]any{
			"title": "New Title",
		}},
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

// newAppearanceMCPSession is an internal helper for the appearance package.
func newAppearanceMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/appearance", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/appearance", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
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
