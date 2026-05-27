// appearance_test.go contains unit tests for the GitLab instance appearance MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package appearance

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// appearanceJSON identifies the appearance JSON constant used by this package.
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

// TestGet_Success verifies Get when success.
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

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_Success verifies Update when success.
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

// TestUpdate_Error verifies Update when error.
func TestUpdate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(t.Context(), client, UpdateInput{Title: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown.
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

// TestUpdate_AllFields verifies Update when all fields.
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

// TestFormatGetMarkdown_WithPWA verifies FormatGetMarkdown when with pwa.
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

// TestFormatGetMarkdown_Minimal verifies FormatGetMarkdown when minimal.
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

// TestFormatUpdateMarkdown_Coverage verifies FormatUpdateMarkdown when coverage.
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
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes covers ActionSpecs with table-driven subtests for call routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := newAppearanceRouteClient(t)
	specs := ActionSpecs(client)
	getSpec := appearanceSpecByName(t, specs, "appearance_get")
	if !strings.Contains(getSpec.Usage, "branding") {
		t.Fatalf("appearance_get Usage = %q, want branding guidance", getSpec.Usage)
	}
	if !slices.Contains(getSpec.Aliases, "branding settings") {
		t.Fatalf("appearance_get Aliases = %v, want branding settings alias", getSpec.Aliases)
	}
	updateSpec := appearanceSpecByName(t, specs, "appearance_update")
	if guidance := updateSpec.ParameterGuidance["message_background_color"]; guidance.SemanticRole != "hex_color" {
		t.Fatalf("appearance_update guidance = %+v, want hex_color", guidance)
	}
	if guidance := updateSpec.ParameterGuidance["title"]; guidance.SemanticRole != "instance_brand_title" {
		t.Fatalf("appearance_update title guidance = %+v, want instance_brand_title", guidance)
	}
	if !strings.Contains(updateSpec.IndividualTool.Description, "Returns:") || !strings.Contains(updateSpec.IndividualTool.Description, "See also:") {
		t.Fatalf("appearance_update description = %q, want Returns/See also guidance", updateSpec.IndividualTool.Description)
	}
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

func appearanceSpecByName(t *testing.T, specs []toolutil.ActionSpec, name string) toolutil.ActionSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("missing ActionSpec %s", name)
	return toolutil.ActionSpec{}
}

// newAppearanceRouteClient returns a client backed by mock appearance endpoints.
func newAppearanceRouteClient(t *testing.T) *gitlabclient.Client {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/appearance", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/appearance", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, appearanceJSON)
	})

	return testutil.NewTestClient(t, handler)
}
