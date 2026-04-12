// integrations_test.go contains unit tests for the project integration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package integrations

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	errExpNonNilResult    = "expected non-nil result"
	errExpUnsupportedSlug = "expected error for unsupported slug"
	fmtUnexpErr           = "unexpected error: %v"
	fmtExpSlugJira        = "expected slug 'jira', got %q"
	testSlugJira          = "jira"
	testTitleJira         = "Jira"
)

// matchIntegrationPath checks if the URL path ends with a given suffix
// under either /services/ or /integrations/ prefix.
func matchIntegrationPath(path, suffix string) bool {
	return strings.HasSuffix(path, "/services/"+suffix) ||
		strings.HasSuffix(path, "/integrations/"+suffix)
}

// List.

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/services") && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"Jira","slug":"jira","active":true},
				{"id":2,"title":"Slack","slug":"slack","active":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Integrations) != 2 {
		t.Fatalf("expected 2 integrations, got %d", len(out.Integrations))
	}
	if out.Integrations[0].Slug != testSlugJira {
		t.Errorf(fmtExpSlugJira, out.Integrations[0].Slug)
	}
	if !out.Integrations[0].Active {
		t.Error("expected jira to be active")
	}
	if out.Integrations[1].Active {
		t.Error("expected slack to be inactive")
	}
}

// TestList_Empty verifies the behavior of list empty.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Integrations) != 0 {
		t.Fatalf("expected 0 integrations, got %d", len(out.Integrations))
	}
}

// TestList_Error verifies the behavior of list error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// Get.

// TestGet_JiraSuccess verifies the behavior of get jira success.
func TestGet_JiraSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, testSlugJira) && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"Jira","slug":"jira","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: testSlugJira})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.Slug != testSlugJira {
		t.Errorf(fmtExpSlugJira, out.Integration.Slug)
	}
	if !out.Integration.Active {
		t.Error("expected jira to be active")
	}
}

// TestGet_SlackSuccess verifies the behavior of get slack success.
func TestGet_SlackSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, "slack") && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":2,"title":"Slack notifications","slug":"slack","active":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: "slack"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.Slug != "slack" {
		t.Errorf("expected slug 'slack', got %q", out.Integration.Slug)
	}
}

// TestGet_UnsupportedSlug verifies the behavior of get unsupported slug.
func TestGet_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
}

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: testSlugJira})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// Delete.

// TestDelete_JiraSuccess verifies the behavior of delete jira success.
func TestDelete_JiraSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, testSlugJira) && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: testSlugJira})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_SlackApplicationSuccess verifies the behavior of delete slack application success.
func TestDelete_SlackApplicationSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, "gitlab-slack-application") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: "slack-application"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_UnsupportedSlug verifies the behavior of delete unsupported slug.
func TestDelete_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
}

// TestDelete_Error verifies the behavior of delete error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: testSlugJira})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// SetJira.

// TestSetJira_Success verifies the behavior of set jira success.
func TestSetJira_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, testSlugJira) && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"Jira","slug":"jira","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetJira(t.Context(), client, SetJiraInput{
		ProjectID: "1",
		URL:       "https://jira.example.com",
		Username:  "user",
		Password:  "pass",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.Slug != testSlugJira {
		t.Errorf(fmtExpSlugJira, out.Integration.Slug)
	}
	if !out.Integration.Active {
		t.Error("expected jira to be active after set")
	}
}

// TestSetJira_Error verifies the behavior of set jira error.
func TestSetJira_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := SetJira(t.Context(), client, SetJiraInput{
		ProjectID: "1",
		URL:       "https://jira.example.com",
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// Markdown Formatters.

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Integrations: []IntegrationItem{
			{ID: 1, Title: testTitleJira, Slug: testSlugJira, Active: true},
			{ID: 2, Title: "Slack", Slug: "slack", Active: false},
		},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{
		Integration: IntegrationItem{ID: 1, Title: testTitleJira, Slug: testSlugJira, Active: true, CreatedAt: "2024-01-01"},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// List — API error (400)
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies the behavior of list a p i error400.
func TestList_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Get — all slug dispatches, API error 400
// ---------------------------------------------------------------------------.

// TestGet_AllSlugsSuccess verifies the behavior of get all slugs success.
func TestGet_AllSlugsSuccess(t *testing.T) {
	slugs := []string{
		"discord", "mattermost", "microsoft-teams", "telegram",
		"datadog", "jenkins", "emails-on-push", "pipelines-email",
		"external-wiki", "custom-issue-tracker", "drone-ci", "github",
		"harbor", "matrix", "redmine", "youtrack",
		"slack-slash-commands", "mattermost-slash-commands",
	}
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					testutil.RespondJSON(w, http.StatusOK, `{"id":10,"title":"Test","slug":"`+slug+`","active":true}`)
					return
				}
				http.NotFound(w, r)
			}))
			out, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: slug})
			if err != nil {
				t.Fatalf("unexpected error for slug %s: %v", slug, err)
			}
			if out.Integration.Slug != slug {
				t.Errorf("expected slug %q, got %q", slug, out.Integration.Slug)
			}
		})
	}
}

// TestGet_APIError400 verifies the behavior of get a p i error400.
func TestGet_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: "slack"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Delete — all slug dispatches, API error 400
// ---------------------------------------------------------------------------.

// TestDelete_AllSlugsSuccess verifies the behavior of delete all slugs success.
func TestDelete_AllSlugsSuccess(t *testing.T) {
	slugs := []string{
		"jira", "slack", "discord", "mattermost", "microsoft-teams", "telegram",
		"datadog", "jenkins", "emails-on-push", "pipelines-email",
		"external-wiki", "custom-issue-tracker", "drone-ci", "github",
		"harbor", "matrix", "redmine", "youtrack",
		"slack-slash-commands", "mattermost-slash-commands", "slack-application",
	}
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			}))
			err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: slug})
			if err != nil {
				t.Fatalf("unexpected error for slug %s: %v", slug, err)
			}
		})
	}
}

// TestDelete_APIError400 verifies the behavior of delete a p i error400.
func TestDelete_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: "jira"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// SetJira — optional fields, API error 400
// ---------------------------------------------------------------------------.

// TestSetJira_WithAllOptionalFields verifies the behavior of set jira with all optional fields.
func TestSetJira_WithAllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"Jira","slug":"jira","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))
	active := true
	autoTransition := true
	commitEvents := true
	mrEvents := true
	commentEnabled := true
	issuesEnabled := true
	useInherited := false
	authType := int64(1)
	out, err := SetJira(t.Context(), client, SetJiraInput{
		ProjectID:                    "1",
		URL:                          "https://jira.example.com",
		Username:                     "user",
		Password:                     "pass",
		Active:                       &active,
		APIURL:                       "https://jira.example.com/api",
		JiraAuthType:                 &authType,
		JiraIssuePrefix:              "PROJ",
		JiraIssueRegex:               "[A-Z]+-\\d+",
		JiraIssueTransitionAutomatic: &autoTransition,
		JiraIssueTransitionID:        "31",
		CommitEvents:                 &commitEvents,
		MergeRequestsEvents:          &mrEvents,
		CommentOnEventEnabled:        &commentEnabled,
		IssuesEnabled:                &issuesEnabled,
		ProjectKeys:                  []string{"PROJ", "DEV"},
		UseInheritedSettings:         &useInherited,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Integration.Slug != "jira" {
		t.Errorf("expected slug 'jira', got %q", out.Integration.Slug)
	}
}

// TestSetJira_APIError400 verifies the behavior of set jira a p i error400.
func TestSetJira_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := SetJira(t.Context(), client, SetJiraInput{
		ProjectID: "1",
		URL:       "https://jira.example.com",
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional branches
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_Inactive verifies the behavior of format get markdown inactive.
func TestFormatGetMarkdown_Inactive(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{
		Integration: IntegrationItem{ID: 2, Title: "Slack", Slug: "slack", Active: false},
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No") {
		t.Errorf("expected 'No' for inactive, got %q", text)
	}
}

// TestFormatGetMarkdown_WithUpdatedAt verifies the behavior of format get markdown with updated at.
func TestFormatGetMarkdown_WithUpdatedAt(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{
		Integration: IntegrationItem{
			ID: 1, Title: "Jira", Slug: "jira", Active: true,
			CreatedAt: "2024-01-01", UpdatedAt: "2024-06-01",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Updated") {
		t.Errorf("expected 'Updated' in output, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllTools validates m c p round trip all tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	session := newIntegrationsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_integrations", map[string]any{"project_id": "1"}},
		{"get_jira", "gitlab_get_integration", map[string]any{"project_id": "1", "slug": "jira"}},
		{"delete_jira", "gitlab_delete_integration", map[string]any{"project_id": "1", "slug": "jira"}},
		{"set_jira", "gitlab_set_jira_integration", map[string]any{"project_id": "1", "url": "https://jira.example.com"}},
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
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// newIntegrationsMCPSession is an internal helper for the integrations package.
func newIntegrationsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	integrationJSON := `{"id":1,"title":"Jira","slug":"jira","active":true}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/services", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+integrationJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/integrations/jira", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, integrationJSON)
	})
	handler.HandleFunc("GET /api/v4/projects/1/services/jira", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, integrationJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/services/jira", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/integrations/jira", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/services/jira", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, integrationJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/integrations/jira", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, integrationJSON)
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
