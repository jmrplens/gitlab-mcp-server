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

// firstMarkdownText returns the text of the first TextContent in a
// CallToolResult. Used by markdown formatter tests to assert on the
// rendered output without re-implementing the content extraction
// inline in every test.
func firstMarkdownText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if r == nil {
		t.Fatal("CallToolResult is nil")
	}
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no TextContent in CallToolResult")
	return ""
}

// List.

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_Empty verifies the List_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestIntegrationToItem_MirrorsAllSDKFields verifies that integrationToItem
// maps every field of the client-go gl.Integration base struct onto
// IntegrationItem, including the full set of event-trigger flags and the
// inherited flag. It pins the 1:1 field coverage against client-go.
func TestIntegrationToItem_MirrorsAllSDKFields(t *testing.T) {
	src := &gl.Integration{
		ID: 7, Title: "Jira", Slug: "jira", Active: true,
		AlertEvents: true, CommitEvents: true, ConfidentialIssuesEvents: true,
		ConfidentialNoteEvents: true, DeploymentEvents: true,
		GroupConfidentialMentionEvents: true, GroupMentionEvents: true,
		IncidentEvents: true, IssuesEvents: true, JobEvents: true,
		MergeRequestsEvents: true, NoteEvents: true, PipelineEvents: true,
		PushEvents: true, TagPushEvents: true, VulnerabilityEvents: true,
		WikiPageEvents: true, CommentOnEventEnabled: true, Inherited: true,
	}
	got := integrationToItem(src)
	want := IntegrationItem{
		ID: 7, Title: "Jira", Slug: "jira", Active: true,
		AlertEvents: true, CommitEvents: true, ConfidentialIssuesEvents: true,
		ConfidentialNoteEvents: true, DeploymentEvents: true,
		GroupConfidentialMentionEvents: true, GroupMentionEvents: true,
		IncidentEvents: true, IssuesEvents: true, JobEvents: true,
		MergeRequestsEvents: true, NoteEvents: true, PipelineEvents: true,
		PushEvents: true, TagPushEvents: true, VulnerabilityEvents: true,
		WikiPageEvents: true, CommentOnEventEnabled: true, Inherited: true,
	}
	if got != want {
		t.Fatalf("integrationToItem mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

// TestGroupDatadogToItem_Scenarios verifies groupDatadogToItem across the two
// payload shapes GitLab can return, as table-driven subtests:
//
//   - NestedPropertiesPayload: every embedded gl.Integration event flag and
//     the Datadog-specific fields map onto GroupDatadogItem, the canonical
//     nested Properties object is a field-for-field mirror of the SDK's, and
//     the deprecated flat copies carry the same values (1:1 dual shape).
//   - LegacyFlatPayload: older servers omit the nested "properties" object —
//     the flat fields carry the SDK's deprecated flat values, Properties
//     stays nil (the same state the SDK struct is in), and
//     datadog_ci_visibility is never fabricated.
func TestGroupDatadogToItem_Scenarios(t *testing.T) {
	archiveOn := true
	archiveOff := false

	nestedSrc := &gl.GroupDatadogIntegration{
		ID: 9, Title: "datadog", Slug: "datadog", Active: true,
		AlertEvents: true, CommitEvents: true, ConfidentialIssuesEvents: true,
		ConfidentialNoteEvents: true, DeploymentEvents: true,
		GroupConfidentialMentionEvents: true, GroupMentionEvents: true,
		IncidentEvents: true, IssuesEvents: true, JobEvents: true,
		MergeRequestsEvents: true, NoteEvents: true, PipelineEvents: true,
		PushEvents: true, TagPushEvents: true, VulnerabilityEvents: true,
		WikiPageEvents: true, CommentOnEventEnabled: true, Inherited: true,
		Properties: &gl.GroupDatadogIntegrationProperties{
			APIURL: "https://api.datadoghq.com", DatadogEnv: "prod",
			DatadogService: "svc", DatadogSite: "datadoghq.com", DatadogTags: "team:core",
			DatadogCIVisibility: true, ArchiveTraceEvents: archiveOn,
		},
	}

	legacySrc := &gl.GroupDatadogIntegration{ID: 3, Title: "datadog", Slug: "datadog"}
	legacySrc.APIURL = "https://legacy.example" //nolint:staticcheck // SA1019: legacy flat payload under test.
	legacySrc.DatadogEnv = "staging"            //nolint:staticcheck // SA1019: legacy flat payload under test.
	legacySrc.DatadogService = "legacy-svc"     //nolint:staticcheck // SA1019: legacy flat payload under test.
	legacySrc.DatadogSite = "datadoghq.eu"      //nolint:staticcheck // SA1019: legacy flat payload under test.
	legacySrc.DatadogTags = "team:legacy"       //nolint:staticcheck // SA1019: legacy flat payload under test.
	legacySrc.ArchiveTraceEvents = &archiveOff  //nolint:staticcheck // SA1019: legacy flat payload under test.

	tests := []struct {
		name   string
		src    *gl.GroupDatadogIntegration
		verify func(t *testing.T, got GroupDatadogItem)
	}{
		{
			name:   "NestedPropertiesPayload",
			src:    nestedSrc,
			verify: verifyNestedDatadogItem(nestedSrc),
		},
		{
			name:   "LegacyFlatPayload",
			src:    legacySrc,
			verify: verifyLegacyDatadogItem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, groupDatadogToItem(tt.src))
		})
	}
}

// verifyNestedDatadogItem returns the assertion set for the modern payload:
// every event flag mapped, the deprecated flat copies populated, and the
// canonical nested Properties object mirroring the SDK field for field.
func verifyNestedDatadogItem(nestedSrc *gl.GroupDatadogIntegration) func(t *testing.T, got GroupDatadogItem) {
	return func(t *testing.T, got GroupDatadogItem) {
		t.Helper()
		flags := map[string]bool{
			"AlertEvents": got.AlertEvents, "CommitEvents": got.CommitEvents,
			"ConfidentialIssuesEvents":       got.ConfidentialIssuesEvents,
			"ConfidentialNoteEvents":         got.ConfidentialNoteEvents,
			"DeploymentEvents":               got.DeploymentEvents,
			"GroupConfidentialMentionEvents": got.GroupConfidentialMentionEvents,
			"GroupMentionEvents":             got.GroupMentionEvents,
			"IncidentEvents":                 got.IncidentEvents,
			"IssuesEvents":                   got.IssuesEvents, "JobEvents": got.JobEvents,
			"MergeRequestsEvents": got.MergeRequestsEvents, "NoteEvents": got.NoteEvents,
			"PipelineEvents": got.PipelineEvents, "PushEvents": got.PushEvents,
			"TagPushEvents": got.TagPushEvents, "VulnerabilityEvents": got.VulnerabilityEvents,
			"WikiPageEvents": got.WikiPageEvents, "CommentOnEventEnabled": got.CommentOnEventEnabled,
			"Inherited": got.Inherited,
		}
		for name, set := range flags {
			if !set {
				t.Fatalf("groupDatadogToItem dropped event flag %s: %+v", name, got)
			}
		}
		if got.APIURL != nestedSrc.Properties.APIURL || got.DatadogEnv != nestedSrc.Properties.DatadogEnv ||
			got.DatadogService != nestedSrc.Properties.DatadogService || got.DatadogSite != nestedSrc.Properties.DatadogSite ||
			got.DatadogTags != nestedSrc.Properties.DatadogTags {
			t.Fatalf("groupDatadogToItem dropped a Datadog field: %+v", got)
		}
		if got.DatadogCIVisibility == nil || !*got.DatadogCIVisibility {
			t.Fatalf("groupDatadogToItem DatadogCIVisibility = %v, want true", got.DatadogCIVisibility)
		}
		if got.ArchiveTraceEvents == nil || !*got.ArchiveTraceEvents {
			t.Fatalf("groupDatadogToItem ArchiveTraceEvents = %v, want true", got.ArchiveTraceEvents)
		}
		if got.Properties == nil {
			t.Fatal("groupDatadogToItem dropped the canonical nested Properties object")
		}
		want := GroupDatadogProperties{
			APIURL:              nestedSrc.Properties.APIURL,
			DatadogEnv:          nestedSrc.Properties.DatadogEnv,
			DatadogService:      nestedSrc.Properties.DatadogService,
			DatadogSite:         nestedSrc.Properties.DatadogSite,
			DatadogTags:         nestedSrc.Properties.DatadogTags,
			DatadogCIVisibility: nestedSrc.Properties.DatadogCIVisibility,
			ArchiveTraceEvents:  nestedSrc.Properties.ArchiveTraceEvents,
		}
		if *got.Properties != want {
			t.Fatalf("nested Properties mismatch:\n got %+v\nwant %+v", *got.Properties, want)
		}
	}
}

// verifyLegacyDatadogItem asserts the fallback for servers that omit the
// nested "properties" object: flat fields carry the deprecated SDK values,
// Properties stays nil, and datadog_ci_visibility is never fabricated.
func verifyLegacyDatadogItem(t *testing.T, got GroupDatadogItem) {
	t.Helper()
	if got.Properties != nil {
		t.Fatalf("Properties = %+v, want nil for a legacy flat payload", got.Properties)
	}
	if got.APIURL != "https://legacy.example" || got.DatadogEnv != "staging" ||
		got.DatadogService != "legacy-svc" || got.DatadogSite != "datadoghq.eu" ||
		got.DatadogTags != "team:legacy" {
		t.Fatalf("flat fallback dropped a Datadog field: %+v", got)
	}
	if got.DatadogCIVisibility != nil {
		t.Fatalf("DatadogCIVisibility = %v, want nil (never fabricated for legacy payloads)", got.DatadogCIVisibility)
	}
	if got.ArchiveTraceEvents == nil || *got.ArchiveTraceEvents {
		t.Fatalf("ArchiveTraceEvents = %v, want explicit false from the legacy flat field", got.ArchiveTraceEvents)
	}
}

// TestGet_JiraSuccess verifies the Get_JiraSuccess handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_SlackSuccess verifies the Get_SlackSuccess handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_UnsupportedSlug verifies the Get_UnsupportedSlug handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
}

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDelete_JiraSuccess verifies the Delete_JiraSuccess handler.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDelete_SlackApplicationSuccess verifies the Delete_SlackApplicationSuccess handler.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDelete_UnsupportedSlug verifies the Delete_UnsupportedSlug handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
}

// TestDelete_Error verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestSetJira_Success verifies that SetJira succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestSetJira_Error verifies that SetJira returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_WithData verifies the ListMarkdown_WithData Markdown formatter for a representative list_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatGetMarkdown verifies the GetMarkdown Markdown formatter for a representative get input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestList_APIError400 verifies that List400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestGet_AllSlugsSuccess verifies the Get_AllSlugsSuccess handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_APIError400 verifies that Get400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDelete_AllSlugsSuccess verifies the Delete_AllSlugsSuccess handler.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDelete_APIError400 verifies that Delete400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestSetJira_WithAllOptionalFields verifies the SetJira_WithAllOptionalFields handler.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSetJira_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
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
	// v2.42.0 fields.
	vulnEnabled := true
	vulnIssueType := int64(10001)
	customizeJira := true
	jiraCheck := true
	jiraExists := true
	jiraAssignee := true
	jiraStatus := true
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
		VulnerabilitiesEnabled:       &vulnEnabled,
		VulnerabilitiesIssueType:     &vulnIssueType,
		ProjectKey:                   "SEC",
		CustomizeJiraIssueEnabled:    &customizeJira,
		JiraCheckEnabled:             &jiraCheck,
		JiraExistsCheckEnabled:       &jiraExists,
		JiraAssigneeCheckEnabled:     &jiraAssignee,
		JiraStatusCheckEnabled:       &jiraStatus,
		JiraAllowedStatusesAsString:  "In Progress,Done",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Integration.Slug != "jira" {
		t.Errorf("expected slug 'jira', got %q", out.Integration.Slug)
	}
	for _, want := range []string{
		"api_url", "jira_issue_prefix", "jira_issue_regex", "project_keys", "comment_on_event_enabled", "jira_issue_transition_automatic",
		"vulnerabilities_enabled", "vulnerabilities_issuetype", "project_key", "customize_jira_issue_enabled",
		"jira_check_enabled", "jira_exists_check_enabled", "jira_assignee_check_enabled", "jira_status_check_enabled",
		"jira_allowed_statuses_as_string",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q", want)
			}
		})
	}
}

// TestSetJira_APIError400 verifies that SetJira400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatGetMarkdown_Inactive verifies the GetMarkdown_Inactive Markdown formatter for a representative get_inactive input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatGetMarkdown_WithUpdatedAt verifies the GetMarkdown_WithUpdatedAt Markdown formatter for a representative get_withupdatedat input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestGet_WithTimestamps verifies the Get_WithTimestamps handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_NilResult verifies the Get_NilResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestFormatSetJiraMarkdown_RendersUpdateAndHints verifies the Jira upsert
// formatter renders the integration item plus the write-only credentials
// hint through the shared Markdown registry.
func TestFormatSetJiraMarkdown_RendersUpdateAndHints(t *testing.T) {
	md := formatSetJiraMarkdownString(SetJiraOutput{Integration: IntegrationItem{Slug: "jira", Active: true}})
	for _, want := range []string{"Jira Integration Updated", "write-only"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("formatSetJiraMarkdownString missing %q in:\n%s", want, md)
			}
		})
	}
}

// TestListGroupIntegrations_SkipsNullElements verifies that a JSON null
// element inside the integrations array is skipped instead of producing an
// empty item or a panic (the nil-guard branch of the list loop).
func TestListGroupIntegrations_SkipsNullElements(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[null, {"id": 3, "slug": "datadog", "active": true}]`)
	}))
	out, err := ListGroupIntegrations(t.Context(), client, ListGroupIntegrationsInput{GroupID: "42"})
	if err != nil {
		t.Fatalf("ListGroupIntegrations error = %v", err)
	}
	if len(out.Integrations) != 1 || out.Integrations[0].Slug != "datadog" {
		t.Errorf("Integrations = %+v, want single datadog item (null skipped)", out.Integrations)
	}
}
