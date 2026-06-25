// set_integration_test.go contains unit tests for the generic slug-dispatched
// project and group integration MCP tool handlers (set_integration and the
// group list/get/set/delete actions). The tests use httptest to assert that
// each handler targets the correct REST path and method, passes the config
// object through as the request body, and parses the response into the shared
// IntegrationItem shape.
package integrations

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	testProjectID = "42"
	testSlugSlack = "slack"
	testWebhook   = "https://hooks.example.com/abc"
)

// SetIntegration (generic project upsert).

// TestSetIntegration_Success verifies that SetIntegration PUTs the config body
// to the correct projects/{id}/integrations/{slug} path and parses the response.
func TestSetIntegration_Success(t *testing.T) {
	var capturedPath, capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && matchIntegrationPath(r.URL.Path, testSlugSlack) {
			capturedPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			testutil.RespondJSON(w, http.StatusOK, `{"id":5,"title":"Slack","slug":"slack","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetIntegration(t.Context(), client, SetIntegrationInput{
		ProjectID: testProjectID,
		Slug:      testSlugSlack,
		Config: map[string]any{
			"webhook":                 testWebhook,
			"push_events":             true,
			"branches_to_be_notified": "default",
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.ID != 5 || out.Integration.Slug != testSlugSlack || !out.Integration.Active {
		t.Errorf("unexpected output: %+v", out.Integration)
	}
	if !strings.HasSuffix(capturedPath, "/projects/"+testProjectID+"/integrations/"+testSlugSlack) {
		t.Errorf("unexpected path: %q", capturedPath)
	}
	// The config object must be passed through verbatim as the request body.
	var body map[string]any
	if jerr := json.Unmarshal([]byte(capturedBody), &body); jerr != nil {
		t.Fatalf("body is not valid JSON: %v (%q)", jerr, capturedBody)
	}
	if body["webhook"] != testWebhook {
		t.Errorf("config webhook not forwarded; body = %q", capturedBody)
	}
	if body["push_events"] != true {
		t.Errorf("config push_events not forwarded; body = %q", capturedBody)
	}
}

// TestSetIntegration_NilConfig verifies that a nil config map is tolerated and
// sent as an empty JSON object rather than null (no panic on missing config).
func TestSetIntegration_NilConfig(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"Harbor","slug":"harbor","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := SetIntegration(t.Context(), client, SetIntegrationInput{
		ProjectID: testProjectID,
		Slug:      "harbor",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if strings.TrimSpace(capturedBody) != "{}" {
		t.Errorf("expected empty object body, got %q", capturedBody)
	}
}

// TestSetIntegration_MissingSlug verifies the slug-required validation guard.
func TestSetIntegration_MissingSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("transport should not be called when slug is empty")
	}))
	if _, err := SetIntegration(t.Context(), client, SetIntegrationInput{ProjectID: testProjectID}); err == nil {
		t.Fatal("expected error for missing slug")
	}
}

// TestSetIntegration_Error verifies a wrapped error on a non-2xx response.
func TestSetIntegration_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if _, err := SetIntegration(t.Context(), client, SetIntegrationInput{
		ProjectID: testProjectID, Slug: testSlugSlack, Config: map[string]any{"webhook": testWebhook},
	}); err == nil {
		t.Fatal(errExpectedNil)
	}
}

// Group integrations.

// matchGroupIntegrationsPath returns true when the path targets the group
// integrations collection endpoint.
func matchGroupIntegrationsPath(path string) bool {
	return strings.HasSuffix(path, "/groups/"+testGroupPath+"/integrations")
}

// matchGroupIntegrationSlugPath returns true when the path targets a single
// group integration by slug.
func matchGroupIntegrationSlugPath(path, slug string) bool {
	return strings.HasSuffix(path, "/groups/"+testGroupPath+"/integrations/"+slug)
}

// TestListGroupIntegrations_Success verifies the list endpoint hits
// /groups/{id}/integrations and parses the array response.
func TestListGroupIntegrations_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && matchGroupIntegrationsPath(r.URL.Path) {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"Slack","slug":"slack","active":true},
				{"id":2,"title":"Jira","slug":"jira","active":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListGroupIntegrations(t.Context(), client, ListGroupIntegrationsInput{GroupID: testGroupPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Integrations) != 2 {
		t.Fatalf("expected 2 integrations, got %d", len(out.Integrations))
	}
	if out.Integrations[0].Slug != testSlugSlack || !out.Integrations[0].Active {
		t.Errorf("unexpected first integration: %+v", out.Integrations[0])
	}
}

// TestListGroupIntegrations_Error verifies a wrapped error on 403.
func TestListGroupIntegrations_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	if _, err := ListGroupIntegrations(t.Context(), client, ListGroupIntegrationsInput{GroupID: testGroupPath}); err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetGroupIntegration_Success verifies the get-by-slug endpoint.
func TestGetGroupIntegration_Success(t *testing.T) {
	var capturedPath string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && matchGroupIntegrationSlugPath(r.URL.Path, testSlugSlack) {
			capturedPath = r.URL.Path
			testutil.RespondJSON(w, http.StatusOK, `{"id":3,"title":"Slack","slug":"slack","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetGroupIntegration(t.Context(), client, GetGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.ID != 3 || out.Integration.Slug != testSlugSlack {
		t.Errorf("unexpected output: %+v", out.Integration)
	}
	if !strings.HasSuffix(capturedPath, "/integrations/"+testSlugSlack) {
		t.Errorf("unexpected path: %q", capturedPath)
	}
}

// TestGetGroupIntegration_MissingSlug verifies the slug-required guard.
func TestGetGroupIntegration_MissingSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("transport should not be called when slug is empty")
	}))
	if _, err := GetGroupIntegration(t.Context(), client, GetGroupIntegrationInput{GroupID: testGroupPath}); err == nil {
		t.Fatal("expected error for missing slug")
	}
}

// TestGetGroupIntegration_Error verifies a wrapped error on 404.
func TestGetGroupIntegration_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if _, err := GetGroupIntegration(t.Context(), client, GetGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack}); err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestSetGroupIntegration_Success verifies the PUT path and body passthrough.
func TestSetGroupIntegration_Success(t *testing.T) {
	var capturedPath, capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && matchGroupIntegrationSlugPath(r.URL.Path, testSlugSlack) {
			capturedPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			testutil.RespondJSON(w, http.StatusOK, `{"id":4,"title":"Slack","slug":"slack","active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetGroupIntegration(t.Context(), client, SetGroupIntegrationInput{
		GroupID: testGroupPath,
		Slug:    testSlugSlack,
		Config:  map[string]any{"webhook": testWebhook},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.ID != 4 {
		t.Errorf("unexpected output: %+v", out.Integration)
	}
	if !strings.HasSuffix(capturedPath, "/groups/"+testGroupPath+"/integrations/"+testSlugSlack) {
		t.Errorf("unexpected path: %q", capturedPath)
	}
	if !strings.Contains(capturedBody, testWebhook) {
		t.Errorf("config not forwarded; body = %q", capturedBody)
	}
}

// TestSetGroupIntegration_MissingSlug verifies the slug-required guard.
func TestSetGroupIntegration_MissingSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("transport should not be called when slug is empty")
	}))
	if _, err := SetGroupIntegration(t.Context(), client, SetGroupIntegrationInput{GroupID: testGroupPath}); err == nil {
		t.Fatal("expected error for missing slug")
	}
}

// TestSetGroupIntegration_Error verifies a wrapped error on 403.
func TestSetGroupIntegration_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	if _, err := SetGroupIntegration(t.Context(), client, SetGroupIntegrationInput{
		GroupID: testGroupPath, Slug: testSlugSlack, Config: map[string]any{"webhook": testWebhook},
	}); err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDeleteGroupIntegration_Success verifies the DELETE path.
func TestDeleteGroupIntegration_Success(t *testing.T) {
	var capturedPath string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && matchGroupIntegrationSlugPath(r.URL.Path, testSlugSlack) {
			capturedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	if err := DeleteGroupIntegration(t.Context(), client, DeleteGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.HasSuffix(capturedPath, "/integrations/"+testSlugSlack) {
		t.Errorf("unexpected path: %q", capturedPath)
	}
}

// TestDeleteGroupIntegration_MissingSlug verifies the slug-required guard.
func TestDeleteGroupIntegration_MissingSlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("transport should not be called when slug is empty")
	}))
	if err := DeleteGroupIntegration(t.Context(), client, DeleteGroupIntegrationInput{GroupID: testGroupPath}); err == nil {
		t.Fatal("expected error for missing slug")
	}
}

// TestDeleteGroupIntegration_Error verifies a wrapped error on 403.
func TestDeleteGroupIntegration_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	if err := DeleteGroupIntegration(t.Context(), client, DeleteGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack}); err == nil {
		t.Fatal(errExpectedNil)
	}
}

// deleteGroupIntegrationOutput wrapper.

// TestDeleteGroupIntegrationOutput_Success covers the destructive output wrapper.
func TestDeleteGroupIntegrationOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := deleteGroupIntegrationOutput(t.Context(), client, DeleteGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status == "" && out.Message == "" {
		t.Error("expected non-empty Status or Message in DeleteOutput")
	}
}

// TestDeleteGroupIntegrationOutput_Error covers the wrapper error path.
func TestDeleteGroupIntegrationOutput_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	if _, err := deleteGroupIntegrationOutput(t.Context(), client, DeleteGroupIntegrationInput{GroupID: testGroupPath, Slug: testSlugSlack}); err == nil {
		t.Fatal(errExpectedNil)
	}
}

// Markdown formatters.

// TestFormatSetIntegrationMarkdown verifies the generic project upsert formatter.
func TestFormatSetIntegrationMarkdown(t *testing.T) {
	result := FormatSetIntegrationMarkdown(SetIntegrationOutput{
		Integration: IntegrationItem{ID: 1, Title: "Slack", Slug: "slack", Active: true, CreatedAt: "2026-01-01", UpdatedAt: "2026-02-01"},
	})
	text := firstMarkdownText(t, result)
	for _, want := range []string{"Integration Updated", "Slack", "Slug", "Created", "Updated"} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown should contain %q, got: %s", want, text)
		}
	}
}

// TestFormatSetIntegrationMarkdown_TitleFallback verifies the slug fallback when Title is empty.
func TestFormatSetIntegrationMarkdown_TitleFallback(t *testing.T) {
	result := FormatSetIntegrationMarkdown(SetIntegrationOutput{
		Integration: IntegrationItem{ID: 1, Slug: "harbor", Active: false},
	})
	text := firstMarkdownText(t, result)
	if !strings.Contains(text, "harbor") {
		t.Errorf("expected slug fallback in heading, got: %s", text)
	}
}

// TestFormatListGroupIntegrationsMarkdown verifies the group list formatter (with data and empty).
func TestFormatListGroupIntegrationsMarkdown(t *testing.T) {
	withData := FormatListGroupIntegrationsMarkdown(ListGroupIntegrationsOutput{
		Integrations: []IntegrationItem{{ID: 1, Title: "Slack", Slug: "slack", Active: true}},
	})
	if !strings.Contains(firstMarkdownText(t, withData), "Group Integrations (1)") {
		t.Error("expected group integrations table heading")
	}
	empty := FormatListGroupIntegrationsMarkdown(ListGroupIntegrationsOutput{})
	if !strings.Contains(firstMarkdownText(t, empty), "No active integrations") {
		t.Error("expected empty-list message")
	}
}

// TestFormatGetGroupIntegrationMarkdown verifies the group get formatter.
func TestFormatGetGroupIntegrationMarkdown(t *testing.T) {
	result := FormatGetGroupIntegrationMarkdown(GetGroupIntegrationOutput{
		Integration: IntegrationItem{ID: 2, Title: "Jira", Slug: "jira", Active: true},
	})
	if !strings.Contains(firstMarkdownText(t, result), "Group Integration") {
		t.Error("expected group integration heading")
	}
}

// TestFormatSetGroupIntegrationMarkdown verifies the group upsert formatter.
func TestFormatSetGroupIntegrationMarkdown(t *testing.T) {
	result := FormatSetGroupIntegrationMarkdown(SetGroupIntegrationOutput{
		Integration: IntegrationItem{ID: 2, Title: "Jira", Slug: "jira", Active: true},
	})
	if !strings.Contains(firstMarkdownText(t, result), "Group Integration Updated") {
		t.Error("expected group integration update heading")
	}
}

// ActionSpec coverage.

// TestActionSpecs_GenericIntegrationsPresent verifies the new generic actions
// are registered with the expected destructive/edition/tag metadata.
func TestActionSpecs_GenericIntegrationsPresent(t *testing.T) {
	specs := ActionSpecs(nil)
	want := map[string]struct {
		destructive bool
		edition     string
		group       bool
	}{
		"integration_set":          {destructive: false, edition: "", group: false},
		"integration_list_group":   {destructive: false, edition: "premium", group: true},
		"integration_get_group":    {destructive: false, edition: "premium", group: true},
		"integration_set_group":    {destructive: false, edition: "premium", group: true},
		"integration_delete_group": {destructive: true, edition: "premium", group: true},
	}
	seen := map[string]bool{}
	for _, s := range specs {
		w, ok := want[s.Name]
		if !ok {
			continue
		}
		seen[s.Name] = true
		if s.Destructive != w.destructive {
			t.Errorf("%s: Destructive = %v, want %v", s.Name, s.Destructive, w.destructive)
		}
		if s.Edition != w.edition {
			t.Errorf("%s: Edition = %q, want %q", s.Name, s.Edition, w.edition)
		}
		if w.group && !hasTag(s.Tags, "group") {
			t.Errorf("%s: Tags = %v, want to contain 'group'", s.Name, s.Tags)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("ActionSpecs missing %q", name)
		}
	}
}
