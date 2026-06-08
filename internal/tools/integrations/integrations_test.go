// integrations_test.go contains unit tests for the project integration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package integrations

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	// errExpNonNilResult identifies the err exp non nil result constant used by this package.
	errExpNonNilResult = "expected non-nil result"
	// errExpUnsupportedSlug identifies the err exp unsupported slug constant used by this package.
	errExpUnsupportedSlug = "expected error for unsupported slug"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// fmtExpSlugJira identifies the fmt exp slug jira constant used by this package.
	fmtExpSlugJira = "expected slug 'jira', got %q"
	// testSlugJira identifies the test slug jira constant used by this package.
	testSlugJira = "jira"
	// testTitleJira identifies the test title jira constant used by this package.
	testTitleJira = "Jira"
)

// matchIntegrationPath checks if the URL path ends with a given suffix
// under either /services/ or /integrations/ prefix.
func matchIntegrationPath(path, suffix string) bool {
	return strings.HasSuffix(path, "/services/"+suffix) ||
		strings.HasSuffix(path, "/integrations/"+suffix)
}

// List.

// TestList_Success verifies List when success.
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

// TestList_Empty verifies List when empty.
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

// TestList_Error verifies List when error.
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

// TestGet_JiraSuccess verifies Get when jira success.
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

// TestGet_SlackSuccess verifies Get when slack success.
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

// TestGet_UnsupportedSlug verifies Get when unsupported slug.
func TestGet_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
}

// TestGet_APIError verifies Get when API error.
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

// TestDelete_JiraSuccess verifies Delete when jira success.
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

// TestDelete_SlackApplicationSuccess verifies Delete when slack application success.
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

// TestDelete_UnsupportedSlug verifies Delete when unsupported slug.
func TestDelete_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
}

// TestDelete_Error verifies Delete when error.
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

// TestSetJira_Success verifies SetJira when success.
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

// TestSetJira_Error verifies SetJira when error.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
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

// TestFormatGetMarkdown verifies FormatGetMarkdown.
func TestFormatGetMarkdown(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{
		Integration: IntegrationItem{ID: 1, Title: testTitleJira, Slug: testSlugJira, Active: true, CreatedAt: "2026-01-01"},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// List — API error (400)
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies List when API error 400.
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

// TestGet_AllSlugsSuccess verifies Get when all slugs success.
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

// TestGet_APIError400 verifies Get when API error 400.
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

// TestDelete_AllSlugsSuccess verifies Delete when all slugs success.
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

// TestDelete_APIError400 verifies Delete when API error 400.
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

// TestSetJira_WithAllOptionalFields verifies SetJira when with all optional fields.
func TestSetJira_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
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
	for _, want := range []string{"api_url", "jira_issue_prefix", "jira_issue_regex", "project_keys", "comment_on_event_enabled", "jira_issue_transition_automatic"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// TestSetJira_APIError400 verifies SetJira when API error 400.
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

// TestFormatGetMarkdown_Inactive verifies FormatGetMarkdown when inactive.
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

// TestFormatGetMarkdown_WithUpdatedAt verifies FormatGetMarkdown when with updated at.
func TestFormatGetMarkdown_WithUpdatedAt(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{
		Integration: IntegrationItem{
			ID: 1, Title: "Jira", Slug: "jira", Active: true,
			CreatedAt: "2026-01-01", UpdatedAt: "2026-06-01",
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

// TestGet_WithTimestamps covers the CreatedAt/UpdatedAt != nil branches
// in integrationToItem by including timestamps in the API response.
func TestGet_WithTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, testSlugJira) && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"Jira","slug":"jira","active":true,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-02-20T15:30:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: testSlugJira})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.Integration.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// TestGet_NilResult covers the result == nil guard in Get().
func TestGet_NilResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchIntegrationPath(r.URL.Path, testSlugJira) && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("null"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: testSlugJira})
	if err == nil {
		t.Fatal("expected error for nil result")
	}
}

// ----- branch coverage -----

// TestIntegrationFromService_Branches verifies the reflection-based
// integrationFromService function returns the expected nil-result for the
// edge cases that protect against malformed GitLab SDK responses. Each
// branch must propagate the error from the GitLab SDK without panicking.
// The function is intentionally tolerant: any unexpected shape yields a
// nil *gl.Integration alongside the caller-supplied error so that the
// upper layers can still surface a meaningful error message to the user.
func TestIntegrationFromService_Branches(t *testing.T) {
	// Define a local type without an embedded Service field. The reflection
	// lookup fails because FieldByName("Service") returns an invalid Value.
	type noService struct {
		Other int
	}

	// Define a local type whose "Service" field has a different concrete
	// type than *gl.Integration, simulating the !ok branch in the type
	// assertion performed inside integrationFromService.
	type mismatchedService struct {
		Service string
	}

	// nil pointer whose type still embeds gl.Integration (via Service alias).
	var nilJira *gl.JiraService
	// Pointer to a struct that does not have a Service field.
	noServicePtr := &noService{Other: 7}
	// Pointer to a struct whose Service field has a non-*gl.Integration
	// type. FieldByName finds the field, the field is addressable, but the
	// type assertion to *gl.Integration fails.
	mismatchPtr := &mismatchedService{Service: "not-an-integration"}
	// Struct value (not pointer) that has an embedded Service aliasing
	// *gl.Integration. The field is valid but not addressable because the
	// receiver of FieldByName is not a pointer, exercising the !CanAddr()
	// branch in integrationFromService.
	type validServiceStruct struct {
		Service gl.Integration
	}
	validByValue := validServiceStruct{}

	sentinelErr := errors.New("sdk error")

	tests := []struct {
		name    string
		service any
		err     error
	}{
		{
			name:    "nil interface value",
			service: nil,
			err:     sentinelErr,
		},
		{
			name:    "nil pointer to GitLab service",
			service: nilJira,
			err:     sentinelErr,
		},
		{
			name:    "pointer to struct missing Service field",
			service: noServicePtr,
			err:     sentinelErr,
		},
		{
			name:    "pointer to struct with mismatched Service field type",
			service: mismatchPtr,
			err:     sentinelErr,
		},
		{
			name:    "value with valid but unaddressable Service field",
			service: validByValue,
			err:     sentinelErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := integrationFromService(tt.service, tt.err)
			if got != nil {
				t.Fatalf("expected nil integration, got %+v", got)
			}
			if !errors.Is(gotErr, sentinelErr) {
				t.Fatalf("expected error %v, got %v", sentinelErr, gotErr)
			}
		})
	}
}
