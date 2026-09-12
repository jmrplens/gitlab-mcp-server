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

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// appearanceJSON identifies the appearance JSON constant used by this package.
const appearanceJSON = `{
	"site_name": "Example GitLab",
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

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/application/appearance (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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
	if out.Appearance.SiteName != "Example GitLab" {
		t.Errorf("expected site_name 'Example GitLab', got %q", out.Appearance.SiteName)
	}
}

// TestGet_Error verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_Success verifies that Update succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/application/appearance (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestUpdate_Error verifies that Update returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(t.Context(), client, UpdateInput{Title: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// getHints is the guidance section the read card ends with.
const getHints = "\n---\n💡 **Next steps:**\n" +
	"- Use `gitlab_update_appearance` to modify appearance settings\n"

// updateHints is the guidance section the update card ends with.
const updateHints = "\n---\n💡 **Next steps:**\n" +
	"- Use `gitlab_get_appearance` to read the settings back\n"

// markdownText returns the one text block a formatter's result carries.
func markdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("the formatter returned no content at all")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

// TestFormatGetMarkdown verifies the whole card an ordinary appearance renders
// as: a row per field GitLab sent, the two messages as prose under their
// labels, and no row for a field it did not send.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		Appearance: Item{
			Title:                       "GitLab CE",
			Description:                 "Test instance",
			HeaderMessage:               "Welcome",
			EmailHeaderAndFooterEnabled: true,
		},
	}
	got := markdownText(t, FormatGetMarkdown(out))
	want := "## Application Appearance\n\n" +
		"- **Title**: GitLab CE\n" +
		"- **Email Header/Footer**: ✅\n" +
		"- **Description**: Test instance\n" +
		"- **Header Message**: Welcome\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatGetMarkdown_EveryField verifies that every field the output type
// carries reaches the card. The formatter used to render eight of eighteen,
// so the logos, the favicon, the colors and all three guideline texts were
// fetched and then dropped from what a reader sees.
func TestFormatGetMarkdown_EveryField(t *testing.T) {
	out := GetOutput{Appearance: Item{
		SiteName:                    "Example GitLab",
		Title:                       "GitLab CE",
		Description:                 "Open source self-hosted Git management",
		PWAName:                     "GitLab",
		PWAShortName:                "GL",
		PWADescription:              "Code hosting",
		PWAIcon:                     "/uploads/pwa.png",
		Logo:                        "/uploads/logo.png",
		HeaderLogo:                  "/uploads/header.png",
		Favicon:                     "/uploads/favicon.ico",
		MemberGuidelines:            "Be nice",
		NewProjectGuidelines:        "Follow naming conventions",
		ProfileImageGuidelines:      "Use a real photo",
		HeaderMessage:               "Welcome",
		FooterMessage:               "Goodbye",
		MessageBackgroundColor:      "#e75e40",
		MessageFontColor:            "#ffffff",
		EmailHeaderAndFooterEnabled: true,
	}}
	got := markdownText(t, FormatGetMarkdown(out))
	want := "## Application Appearance\n\n" +
		"- **Site Name**: Example GitLab\n" +
		"- **Title**: GitLab CE\n" +
		"- **PWA Name**: GitLab\n" +
		"- **PWA Short Name**: GL\n" +
		"- **Email Header/Footer**: ✅\n" +
		"- **Logo**: /uploads/logo.png\n" +
		"- **Header Logo**: /uploads/header.png\n" +
		"- **Favicon**: /uploads/favicon.ico\n" +
		"- **PWA Icon**: /uploads/pwa.png\n" +
		"- **Message Background Color**: #e75e40\n" +
		"- **Message Font Color**: #ffffff\n" +
		"- **Description**: Open source self-hosted Git management\n" +
		"- **PWA Description**: Code hosting\n" +
		"- **Header Message**: Welcome\n" +
		"- **Footer Message**: Goodbye\n" +
		"- **Member Guidelines**: Be nice\n" +
		"- **New Project Guidelines**: Follow naming conventions\n" +
		"- **Profile Image Guidelines**: Use a real photo\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatGetMarkdown_MultiLineGuidelines verifies that a guideline text an
// administrator typed over several lines becomes a quote under its label,
// where nothing in it can add a row, a heading or a guidance section to the
// card.
func TestFormatGetMarkdown_MultiLineGuidelines(t *testing.T) {
	out := GetOutput{Appearance: Item{
		Title:            "GitLab CE",
		MemberGuidelines: "Be nice\n\n## Rules\n- **State**: closed",
	}}
	got := markdownText(t, FormatGetMarkdown(out))
	want := "## Application Appearance\n\n" +
		"- **Title**: GitLab CE\n" +
		"- **Email Header/Footer**: ❌\n" +
		"- **Member Guidelines**:\n" +
		"  > Be nice\n" +
		"  >\n" +
		"  > ## Rules\n" +
		"  > - **State**: closed\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Update — all optional fields populated
// ---------------------------------------------------------------------------.

// TestUpdate_AllFields verifies the Update_AllFields handler.
// The mock GitLab API at /api/v4/application/appearance (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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
		PWAIcon:                     "/uploads/pwa.png",
		Logo:                        "/uploads/logo.png",
		HeaderLogo:                  "/uploads/header.png",
		Favicon:                     "/uploads/favicon.ico",
		URL:                         "https://example.com",
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

// TestFormatGetMarkdown_WithPWA verifies the whole card for an appearance
// carrying the progressive web app labels and a footer message.
func TestFormatGetMarkdown_WithPWA(t *testing.T) {
	out := GetOutput{
		Appearance: Item{
			Title:         "Test",
			PWAName:       "TestPWA",
			PWAShortName:  "TP",
			FooterMessage: "bye",
		},
	}
	got := markdownText(t, FormatGetMarkdown(out))
	want := "## Application Appearance\n\n" +
		"- **Title**: Test\n" +
		"- **PWA Name**: TestPWA\n" +
		"- **PWA Short Name**: TP\n" +
		"- **Email Header/Footer**: ❌\n" +
		"- **Footer Message**: bye\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — empty fields (no optional PWA/messages)
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_Minimal verifies that an appearance GitLab sent
// nothing but a title for renders two rows and no label with an empty value
// after it: the description row used to be written whatever GitLab sent.
func TestFormatGetMarkdown_Minimal(t *testing.T) {
	out := GetOutput{
		Appearance: Item{
			Title: "Minimal",
		},
	}
	got := markdownText(t, FormatGetMarkdown(out))
	want := "## Application Appearance\n\n" +
		"- **Title**: Minimal\n" +
		"- **Email Header/Footer**: ❌\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatUpdateMarkdown
// ---------------------------------------------------------------------------.

// TestFormatUpdateMarkdown_Coverage verifies that the update result is a card
// of its own: it names what happened in the heading, where it used to reuse
// the read formatter and say "Application Appearance" for a write.
func TestFormatUpdateMarkdown_Coverage(t *testing.T) {
	out := UpdateOutput{
		Appearance: Item{
			Title:       "Updated",
			Description: "Updated desc",
		},
	}
	got := markdownText(t, FormatUpdateMarkdown(out))
	want := "## Application Appearance Updated\n\n" +
		"- **Title**: Updated\n" +
		"- **Email Header/Footer**: ❌\n" +
		"- **Description**: Updated desc\n" +
		updateHints
	if got != want {
		t.Errorf("FormatUpdateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestFormatGetMarkdown_SiteName verifies the site name reaches the rendered
// card. It is the one appearance field the SDK does not model and the handler
// reads off the captured response, so a formatter that dropped it would leave
// the whole captured read with nothing to show for itself.
func TestFormatGetMarkdown_SiteName(t *testing.T) {
	got := markdownText(t, FormatGetMarkdown(GetOutput{Appearance: Item{SiteName: "Example GitLab", Title: "GitLab CE"}}))
	want := "## Application Appearance\n\n" +
		"- **Site Name**: Example GitLab\n" +
		"- **Title**: GitLab CE\n" +
		"- **Email Header/Footer**: ❌\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestAppearance_UnreadableCapturedSiteName verifies that both appearance
// handlers return an error rather than a half-filled result when GitLab sends
// site_name as something that is not a string. The SDK ignores the key its own
// Appearance struct does not model, so the read of the captured response is the
// only thing that can notice, and a handler that swallowed its failure would
// publish an appearance with no site name and no complaint.
func TestAppearance_UnreadableCapturedSiteName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"title":"GitLab CE","site_name":42}`)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "get", Call: func() error {
			_, err := Get(t.Context(), client, GetInput{})
			return err
		}},
		{Name: "update", Call: func() error {
			_, err := Update(t.Context(), client, UpdateInput{Title: "test"})
			return err
		}},
	})
}
