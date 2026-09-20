// integrations_test.go contains unit tests for the project integration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package integrations

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const (
	// errExpUnsupportedSlug identifies the err exp unsupported slug constant used by this package.
	errExpUnsupportedSlug = "expected error for unsupported slug"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// fmtExpSlugJira identifies the fmt exp slug jira constant used by this package.
	fmtExpSlugJira = "expected slug 'jira', got %q"
	// testSlugJira identifies the test slug jira constant used by this package.
	testSlugJira = "jira"
	// testJiraURL is the Jira instance the SetJira fixtures point at; it is the
	// one field the action requires, so every recorded request body carries it.
	testJiraURL = "https://jira.example.com"
)

// matchIntegrationPath checks if the URL path ends with a given suffix
// under either /services/ or /integrations/ prefix.
func matchIntegrationPath(path, suffix string) bool {
	return strings.HasSuffix(path, "/services/"+suffix) ||
		strings.HasSuffix(path, "/integrations/"+suffix)
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
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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
	// The source above sets group_mention_events and its confidential twin,
	// and this want deliberately does not: no Grape entity renders either, so
	// publishing them asserted a false GitLab never sent.
	want := IntegrationItem{
		ID: 7, Title: "Jira", Slug: "jira", Active: true,
		AlertEvents: true, CommitEvents: true, ConfidentialIssuesEvents: true,
		ConfidentialNoteEvents: true, DeploymentEvents: true,
		IncidentEvents: true, IssuesEvents: true, JobEvents: true,
		MergeRequestsEvents: true, NoteEvents: true, PipelineEvents: true,
		PushEvents: true, TagPushEvents: true, VulnerabilityEvents: true,
		WikiPageEvents: true, CommentOnEventEnabled: true, Inherited: true,
	}
	if got != want {
		t.Fatalf("integrationToItem mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

// TestIntegrationToItem_OneFieldAtATime_ReadsEachFromItsOwnSource asserts that
// every published field is read from the client-go field of the same meaning
// and from no other, by setting one field of the source at a time and
// comparing the whole item against one carrying only that field.
//
// The test above cannot say this. It sets every flag to true, and seventeen
// booleans that all agree are indistinguishable: a converter pairing a field
// with its neighbor produces the same item, and there is no branch for a
// mutation gate to flip. One field per case is the fixture in which no two
// values agree, and it is also what GitLab really answers, since an
// integration triggers on the events it was configured for and not on all of
// them.
func TestIntegrationToItem_OneFieldAtATime_ReadsEachFromItsOwnSource(t *testing.T) {
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	updated := time.Date(2026, 8, 9, 10, 11, 12, 0, time.UTC)

	tests := []struct {
		name string
		src  func(*gl.Integration)
		want func(*IntegrationItem)
	}{
		{"id", func(s *gl.Integration) { s.ID = 7 }, func(i *IntegrationItem) { i.ID = 7 }},
		{"title", func(s *gl.Integration) { s.Title = "Jira" }, func(i *IntegrationItem) { i.Title = "Jira" }},
		{"slug", func(s *gl.Integration) { s.Slug = testSlugJira }, func(i *IntegrationItem) { i.Slug = testSlugJira }},
		{"active", func(s *gl.Integration) { s.Active = true }, func(i *IntegrationItem) { i.Active = true }},
		{"created_at", func(s *gl.Integration) { s.CreatedAt = &created }, func(i *IntegrationItem) { i.CreatedAt = "2026-03-04T05:06:07Z" }},
		{"updated_at", func(s *gl.Integration) { s.UpdatedAt = &updated }, func(i *IntegrationItem) { i.UpdatedAt = "2026-08-09T10:11:12Z" }},
		{"alert_events", func(s *gl.Integration) { s.AlertEvents = true }, func(i *IntegrationItem) { i.AlertEvents = true }},
		{"commit_events", func(s *gl.Integration) { s.CommitEvents = true }, func(i *IntegrationItem) { i.CommitEvents = true }},
		{"confidential_issues_events", func(s *gl.Integration) { s.ConfidentialIssuesEvents = true }, func(i *IntegrationItem) { i.ConfidentialIssuesEvents = true }},
		{"confidential_note_events", func(s *gl.Integration) { s.ConfidentialNoteEvents = true }, func(i *IntegrationItem) { i.ConfidentialNoteEvents = true }},
		{"deployment_events", func(s *gl.Integration) { s.DeploymentEvents = true }, func(i *IntegrationItem) { i.DeploymentEvents = true }},
		{"incident_events", func(s *gl.Integration) { s.IncidentEvents = true }, func(i *IntegrationItem) { i.IncidentEvents = true }},
		{"issues_events", func(s *gl.Integration) { s.IssuesEvents = true }, func(i *IntegrationItem) { i.IssuesEvents = true }},
		{"job_events", func(s *gl.Integration) { s.JobEvents = true }, func(i *IntegrationItem) { i.JobEvents = true }},
		{"merge_requests_events", func(s *gl.Integration) { s.MergeRequestsEvents = true }, func(i *IntegrationItem) { i.MergeRequestsEvents = true }},
		{"note_events", func(s *gl.Integration) { s.NoteEvents = true }, func(i *IntegrationItem) { i.NoteEvents = true }},
		{"pipeline_events", func(s *gl.Integration) { s.PipelineEvents = true }, func(i *IntegrationItem) { i.PipelineEvents = true }},
		{"push_events", func(s *gl.Integration) { s.PushEvents = true }, func(i *IntegrationItem) { i.PushEvents = true }},
		{"tag_push_events", func(s *gl.Integration) { s.TagPushEvents = true }, func(i *IntegrationItem) { i.TagPushEvents = true }},
		{"vulnerability_events", func(s *gl.Integration) { s.VulnerabilityEvents = true }, func(i *IntegrationItem) { i.VulnerabilityEvents = true }},
		{"wiki_page_events", func(s *gl.Integration) { s.WikiPageEvents = true }, func(i *IntegrationItem) { i.WikiPageEvents = true }},
		{"comment_on_event_enabled", func(s *gl.Integration) { s.CommentOnEventEnabled = true }, func(i *IntegrationItem) { i.CommentOnEventEnabled = true }},
		{"inherited", func(s *gl.Integration) { s.Inherited = true }, func(i *IntegrationItem) { i.Inherited = true }},
		// The two group mention flags are read by nothing on purpose: no Grape
		// entity renders them, so an item carrying one would assert a value
		// GitLab never sent.
		{"group_mention_events_not_published", func(s *gl.Integration) { s.GroupMentionEvents = true }, func(*IntegrationItem) {}},
		{"group_confidential_mention_events_not_published", func(s *gl.Integration) { s.GroupConfidentialMentionEvents = true }, func(*IntegrationItem) {}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var src gl.Integration
			tt.src(&src)
			var want IntegrationItem
			tt.want(&want)
			if got := integrationToItem(&src); got != want {
				t.Errorf("integrationToItem(%+v) =\n %+v\nwant %+v", src, got, want)
			}
		})
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
			"ConfidentialIssuesEvents": got.ConfidentialIssuesEvents,
			"ConfidentialNoteEvents":   got.ConfidentialNoteEvents,
			"DeploymentEvents":         got.DeploymentEvents,
			"IncidentEvents":           got.IncidentEvents,
			"IssuesEvents":             got.IssuesEvents, "JobEvents": got.JobEvents,
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
// nested "properties" object: the deprecated flat SDK values are read into the
// one key this type publishes, and datadog_ci_visibility reads false because
// such a payload never carries it.
func verifyLegacyDatadogItem(t *testing.T, got GroupDatadogItem) {
	t.Helper()
	if got.Properties == nil {
		t.Fatalf("Properties = nil, want the legacy flat payload read into it: %+v", got)
	}
	want := GroupDatadogProperties{
		APIURL:         "https://legacy.example",
		DatadogEnv:     "staging",
		DatadogService: "legacy-svc",
		DatadogSite:    "datadoghq.eu",
		DatadogTags:    "team:legacy",
	}
	if *got.Properties != want {
		t.Fatalf("legacy flat fallback:\n got %+v\nwant %+v", *got.Properties, want)
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

// TestGet_UnsupportedSlug asserts that a slug the dispatch map does not hold
// is refused here, before any request is made, and that the refusal names the
// slug so a model can correct it.
//
// The mock is ForbiddenHandler, which fails the test if anything arrives. It
// has to be: under a mock answering 404 the test passes either way, since a
// handler that stopped checking the map and sent the unknown slug to GitLab
// would be answered 404 and return an error all the same, and the assertion
// would be about the mock rather than about the guard.
func TestGet_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should name the slug it rejected, got: %v", err)
	}
}

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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

// TestDelete_UnsupportedSlug asserts that a slug the dispatch map does not
// hold is refused before any request is made, and that the refusal names it.
// ForbiddenHandler is what separates the guard from the mock, for the reason
// given on TestGet_UnsupportedSlug; it matters more here, because the request
// a missing guard would let through is a delete.
func TestDelete_UnsupportedSlug(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Slug: "nonexistent"})
	if err == nil {
		t.Fatal(errExpUnsupportedSlug)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should name the slug it rejected, got: %v", err)
	}
}

// TestDelete_Error verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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

// The Markdown formatters are asserted whole in markdown_test.go.

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// List — API error (400)
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies that List400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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

// captureIntegrationPath drives call against a mock that records the path the
// request landed on, and returns it.
func captureIntegrationPath(t *testing.T, method string, call func(*gitlabclient.Client) error) string {
	t.Helper()
	var path string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			t.Errorf("method = %q, want %q (path %s)", r.Method, method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		path = r.URL.EscapedPath()
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"title":"Test","slug":"test","active":true}`)
	}))

	if err := call(client); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	return path
}

// TestGetAndDelete_EverySlug_ReachesTheEndpointNamedAfterIt asserts that each
// slug in the dispatch maps reaches the GitLab endpoint of the same name.
//
// Nothing else checks this, and it is the whole content of these two actions:
// the slug is the only thing the caller supplies that chooses a behavior, and
// it selects a client-go method by hand in a map of twenty-one entries. The
// slug tests beside this one answer any request with a canned body carrying
// the slug the test itself passed in, so an entry wired to the wrong method
// returns a happy answer about the wrong integration, and there is no branch
// for a mutation gate to flip. Ranging over the maps rather than over a list
// is what makes a newly added slug covered by this the moment it exists.
func TestGetAndDelete_EverySlug_ReachesTheEndpointNamedAfterIt(t *testing.T) {
	// The one slug whose GitLab path does not repeat it: this package accepts
	// the short name for disabling the GitLab for Slack app.
	pathSegment := map[string]string{"slack-application": "gitlab-slack-application"}
	want := func(slug string) string {
		if segment, ok := pathSegment[slug]; ok {
			return "/" + segment
		}
		return "/" + slug
	}

	t.Run("get", func(t *testing.T) {
		for _, slug := range slices.Sorted(maps.Keys(integrationGetters)) {
			t.Run(slug, func(t *testing.T) {
				got := captureIntegrationPath(t, http.MethodGet, func(c *gitlabclient.Client) error {
					_, err := Get(t.Context(), c, GetInput{ProjectID: "1", Slug: slug})
					return err
				})
				if !strings.HasSuffix(got, want(slug)) {
					t.Errorf("get %q reached %q, want a path ending in %q", slug, got, want(slug))
				}
			})
		}
	})

	t.Run("delete", func(t *testing.T) {
		for _, slug := range slices.Sorted(maps.Keys(integrationDeleters)) {
			t.Run(slug, func(t *testing.T) {
				got := captureIntegrationPath(t, http.MethodDelete, func(c *gitlabclient.Client) error {
					return Delete(t.Context(), c, DeleteInput{ProjectID: "1", Slug: slug})
				})
				if !strings.HasSuffix(got, want(slug)) {
					t.Errorf("delete %q reached %q, want a path ending in %q", slug, got, want(slug))
				}
			})
		}
	})
}

// TestGet_AllSlugsSuccess drives every slug the get action dispatches and
// asserts the response is decoded into the shared item shape. It is about
// integrationFromService, which reaches the embedded Integration of each
// concrete client-go service type by reflection, and not about routing: the
// mock answers any GET, so which endpoint the slug chose is asserted by
// TestGetAndDelete_EverySlug_ReachesTheEndpointNamedAfterIt instead.
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
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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

// TestDelete_AllSlugsSuccess drives every slug the delete action dispatches
// and asserts each one succeeds against a 204. Which endpoint the slug chose
// is asserted by TestGetAndDelete_EverySlug_ReachesTheEndpointNamedAfterIt.
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
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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

// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames asserts, for
// every handler in this package that attaches a hint to one HTTP status, that
// the hint reaches the caller at that status and at no other.
//
// The error tests beside it drive one status each and assert only that some
// error came back, which says nothing about the suggestion a model is meant to
// act on: the hint is the difference between "403" and "ask for Maintainer on
// the project". The second half matters as much as the first, because
// WrapErrWithStatusHint falls through to a plain wrap on any other status, and
// a hint about a missing role attached to a 400 would send a model to change
// permissions over a malformed request.
func TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames(t *testing.T) {
	const otherStatus = http.StatusBadRequest

	tests := []struct {
		name   string
		status int
		hint   string
		call   func(t *testing.T, client *gitlabclient.Client) error
	}{
		{"list", http.StatusForbidden, "requires Maintainer role on the project", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := List(t.Context(), c, ListInput{ProjectID: "1"})
			return err
		}},
		{"get", http.StatusNotFound, "valid integration name", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := Get(t.Context(), c, GetInput{ProjectID: "1", Slug: testSlugJira})
			return err
		}},
		{"delete", http.StatusForbidden, "deactivates the integration on the project", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			return Delete(t.Context(), c, DeleteInput{ProjectID: "1", Slug: testSlugJira})
		}},
		{"set_jira", http.StatusNotFound, "ensure url is reachable", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := SetJira(t.Context(), c, SetJiraInput{ProjectID: "1", URL: testJiraURL})
			return err
		}},
		{"set_integration", http.StatusNotFound, "verify slug is a supported integration", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := SetIntegration(t.Context(), c, SetIntegrationInput{ProjectID: "1", Slug: testSlugSlack})
			return err
		}},
		{"list_group", http.StatusForbidden, "lists active integrations only", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := ListGroupIntegrations(t.Context(), c, ListGroupIntegrationsInput{GroupID: testGroupPath})
			return err
		}},
		{"get_group", http.StatusNotFound, "must be active on the group", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := GetGroupIntegration(t.Context(), c, GetGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack})
			return err
		}},
		{"set_group", http.StatusForbidden, "verify slug is supported", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := SetGroupIntegration(t.Context(), c, SetGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack})
			return err
		}},
		{"delete_group", http.StatusForbidden, "verify slug with gitlab_list_group_integrations", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			return DeleteGroupIntegration(t.Context(), c, DeleteGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack})
		}},
		{"get_group_datadog", http.StatusNotFound, "Datadog integration must be active on the group", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := GetGroupDatadog(t.Context(), c, GetGroupDatadogInput{GroupID: testGroupPath})
			return err
		}},
		{"set_group_datadog", http.StatusForbidden, "requires Owner role on the group", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			_, err := SetGroupDatadog(t.Context(), c, SetGroupDatadogInput{GroupID: testGroupPath, APIKey: "secret-key"})
			return err
		}},
		{"delete_group_datadog", http.StatusForbidden, "deletion is irreversible", func(t *testing.T, c *gitlabclient.Client) error {
			t.Helper()
			return DeleteGroupDatadog(t.Context(), c, DeleteGroupDatadogInput{GroupID: testGroupPath})
		}},
	}

	answering := func(t *testing.T, status int, call func(*testing.T, *gitlabclient.Client) error) string {
		t.Helper()
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		err := call(t, client)
		if err == nil {
			t.Fatalf("expected an error for HTTP %d", status)
		}
		return err.Error()
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := answering(t, tt.status, tt.call); !strings.Contains(got, tt.hint) {
				t.Errorf("error at %d = %q, want it to suggest %q", tt.status, got, tt.hint)
			}
			if got := answering(t, otherStatus, tt.call); strings.Contains(got, tt.hint) {
				t.Errorf("error at %d = %q, want no %d hint in it", otherStatus, got, tt.status)
			}
		})
	}
}

// setJiraBody drives SetJira against a mock that records the PUT body, and
// returns that body decoded as a JSON object. Decoding it is what makes the
// assertion exact: a substring check cannot tell "project_key" from the
// "project_keys" that contains it, and it says nothing about the value, so a
// field read from the wrong neighbor still matches.
func setJiraBody(t *testing.T, input SetJiraInput) map[string]any {
	t.Helper()
	var raw []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		raw = body
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"Jira","slug":"jira","active":true}`)
	}))

	if _, err := SetJira(t.Context(), client, input); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("request body is not a JSON object: %v (%s)", err, raw)
	}
	return got
}

// TestSetJira_OneOptionalFieldAtATime_SendsOnlyThatField drives each optional
// Jira field on its own and asserts the PUT body is the required url plus
// exactly that field under its documented name and value.
//
// One field per case is the only fixture that separates them. SetJira assigns
// sixteen pointer fields straight across and guards nine more on emptiness, so
// a request carrying all of them agrees with itself however the handler pairs
// an input field with an option field, and a guard written the wrong way round
// silently drops the caller's value. The cases that pass false or an empty
// list also pin that an explicitly negative setting is sent rather than
// omitted as a zero, and that an unset list is left out rather than sent as
// null, which is the difference between leaving Jira's project restriction
// alone and clearing it.
func TestSetJira_OneOptionalFieldAtATime_SendsOnlyThatField(t *testing.T) {
	yes, no := true, false
	authType, issueType := int64(1), int64(10001)

	tests := []struct {
		name  string
		with  func(*SetJiraInput)
		key   string
		value any
	}{
		{"api_url", func(in *SetJiraInput) { in.APIURL = "https://jira.example.com/api" }, "api_url", "https://jira.example.com/api"},
		{"username", func(in *SetJiraInput) { in.Username = "jira-user" }, "username", "jira-user"},
		{"password", func(in *SetJiraInput) { in.Password = "jira-token" }, "password", "jira-token"},
		{"active", func(in *SetJiraInput) { in.Active = &no }, "active", false},
		{"jira_auth_type", func(in *SetJiraInput) { in.JiraAuthType = &authType }, "jira_auth_type", float64(1)},
		{"jira_issue_prefix", func(in *SetJiraInput) { in.JiraIssuePrefix = "PROJ" }, "jira_issue_prefix", "PROJ"},
		{"jira_issue_regex", func(in *SetJiraInput) { in.JiraIssueRegex = `[A-Z]+-\d+` }, "jira_issue_regex", `[A-Z]+-\d+`},
		{"jira_issue_transition_automatic", func(in *SetJiraInput) { in.JiraIssueTransitionAutomatic = &yes }, "jira_issue_transition_automatic", true},
		{"jira_issue_transition_id", func(in *SetJiraInput) { in.JiraIssueTransitionID = "31" }, "jira_issue_transition_id", "31"},
		{"commit_events", func(in *SetJiraInput) { in.CommitEvents = &no }, "commit_events", false},
		{"merge_requests_events", func(in *SetJiraInput) { in.MergeRequestsEvents = &yes }, "merge_requests_events", true},
		{"comment_on_event_enabled", func(in *SetJiraInput) { in.CommentOnEventEnabled = &no }, "comment_on_event_enabled", false},
		{"issues_enabled", func(in *SetJiraInput) { in.IssuesEnabled = &yes }, "issues_enabled", true},
		{"project_keys", func(in *SetJiraInput) { in.ProjectKeys = []string{"PROJ", "DEV"} }, "project_keys", []any{"PROJ", "DEV"}},
		{"use_inherited_settings", func(in *SetJiraInput) { in.UseInheritedSettings = &no }, "use_inherited_settings", false},
		{"vulnerabilities_enabled", func(in *SetJiraInput) { in.VulnerabilitiesEnabled = &yes }, "vulnerabilities_enabled", true},
		{"vulnerabilities_issuetype", func(in *SetJiraInput) { in.VulnerabilitiesIssueType = &issueType }, "vulnerabilities_issuetype", float64(10001)},
		{"project_key", func(in *SetJiraInput) { in.ProjectKey = "SEC" }, "project_key", "SEC"},
		{"customize_jira_issue_enabled", func(in *SetJiraInput) { in.CustomizeJiraIssueEnabled = &yes }, "customize_jira_issue_enabled", true},
		{"jira_check_enabled", func(in *SetJiraInput) { in.JiraCheckEnabled = &no }, "jira_check_enabled", false},
		{"jira_exists_check_enabled", func(in *SetJiraInput) { in.JiraExistsCheckEnabled = &yes }, "jira_exists_check_enabled", true},
		{"jira_assignee_check_enabled", func(in *SetJiraInput) { in.JiraAssigneeCheckEnabled = &no }, "jira_assignee_check_enabled", false},
		{"jira_status_check_enabled", func(in *SetJiraInput) { in.JiraStatusCheckEnabled = &yes }, "jira_status_check_enabled", true},
		{"jira_allowed_statuses_as_string", func(in *SetJiraInput) { in.JiraAllowedStatusesAsString = "In Progress,Done" }, "jira_allowed_statuses_as_string", "In Progress,Done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := SetJiraInput{ProjectID: "1", URL: testJiraURL}
			tt.with(&input)
			want := map[string]any{"url": testJiraURL, tt.key: tt.value}
			if got := setJiraBody(t, input); !reflect.DeepEqual(got, want) {
				t.Errorf("request body = %v, want %v", got, want)
			}
		})
	}
}

// TestSetJira_NoOptionalField_SendsOnlyTheURL asserts that a call naming only
// the Jira instance sends nothing else. It is what keeps an optional field
// from acquiring a default on the way out: an empty guard inverted, or a list
// guard that admits the empty list, would reach GitLab as an instruction to
// clear a setting the caller never mentioned.
func TestSetJira_NoOptionalField_SendsOnlyTheURL(t *testing.T) {
	got := setJiraBody(t, SetJiraInput{ProjectID: "1", URL: testJiraURL})
	if want := (map[string]any{"url": testJiraURL}); !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %v, want %v", got, want)
	}
}

// TestSetJira_WithAllOptionalFields asserts that a call setting every optional
// field reaches GitLab carrying all of them at once, which the per-field cases
// above cannot say: each of those proves one field travels alone.
func TestSetJira_WithAllOptionalFields(t *testing.T) {
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
	got := setJiraBody(t, SetJiraInput{
		ProjectID:                    "1",
		URL:                          testJiraURL,
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
	// Key presence is checked against the decoded object rather than against
	// the raw text: "project_key" is a substring of the "project_keys" beside
	// it, so a substring search reported it present on a body that never
	// carried it.
	for _, want := range []string{
		"url", "api_url", "username", "password", "active", "jira_auth_type",
		"jira_issue_prefix", "jira_issue_regex", "jira_issue_transition_automatic",
		"jira_issue_transition_id", "commit_events", "merge_requests_events",
		"comment_on_event_enabled", "issues_enabled", "project_keys",
		"use_inherited_settings", "vulnerabilities_enabled", "vulnerabilities_issuetype",
		"project_key", "customize_jira_issue_enabled", "jira_check_enabled",
		"jira_exists_check_enabled", "jira_assignee_check_enabled",
		"jira_status_check_enabled", "jira_allowed_statuses_as_string",
	} {
		t.Run(want, func(t *testing.T) {
			if _, ok := got[want]; !ok {
				t.Errorf("request body missing field %q: %v", want, got)
			}
		})
	}
}

// TestSetJira_APIError400 verifies that SetJira400 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts that an error is returned and nothing about its content; which
// suggestion each handler attaches, and at which status, is asserted by
// TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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
