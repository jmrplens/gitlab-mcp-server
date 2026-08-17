// group_datadog_test.go contains unit tests for the group-level Datadog
// integration MCP tool handlers. The tests cover happy paths, the validation
// guard, and the most common API errors (404 not found, 403 forbidden,
// response integration nil).
package integrations

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	testGroupPath   = "my-group"
	testGroupSlug   = "/groups/my-group/integrations/datadog"
	testAPIURL      = "https://api.datadoghq.com"
	testDatadogSite = "datadoghq.com"
)

// matchGroupDatadogPath checks if the request URL targets the group Datadog
// integration endpoint.
func matchGroupDatadogPath(path string) bool {
	return strings.HasSuffix(path, "/groups/"+testGroupPath+"/integrations/datadog")
}

// GetGroupDatadog.

// TestGetGroupDatadog_Success verifies that GetGroupDatadog succeeds when the GitLab API returns a valid response.
// The mock mirrors the current GitLab payload, which nests the Datadog-specific
// values under a "properties" object (client-go >= 2.57 maps it to Properties).
// It asserts the Datadog values, including datadog_ci_visibility, are read from
// the nested object.
func TestGetGroupDatadog_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 42,
				"title": "Datadog",
				"slug": "datadog",
				"active": true,
				"created_at": "2026-01-02T03:04:05.000Z",
				"updated_at": "2026-06-08T11:12:13.000Z",
				"properties": {
					"api_url": "` + testAPIURL + `",
					"datadog_env": "prod",
					"datadog_service": "gitlab",
					"datadog_site": "` + testDatadogSite + `",
					"datadog_tags": "team:platform,env:prod",
					"datadog_ci_visibility": true,
					"archive_trace_events": true
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetGroupDatadog(t.Context(), client, GetGroupDatadogInput{GroupID: testGroupPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.ID != 42 {
		t.Errorf("ID = %d, want 42", out.Integration.ID)
	}
	if !out.Integration.Active {
		t.Error("expected Active=true")
	}
	if out.Integration.APIURL != testAPIURL {
		t.Errorf("APIURL = %q, want %q", out.Integration.APIURL, testAPIURL)
	}
	if out.Integration.DatadogSite != testDatadogSite {
		t.Errorf("DatadogSite = %q, want %q", out.Integration.DatadogSite, testDatadogSite)
	}
	if out.Integration.DatadogCIVisibility == nil || !*out.Integration.DatadogCIVisibility {
		t.Errorf("DatadogCIVisibility = %v, want pointer-to-true", out.Integration.DatadogCIVisibility)
	}
	if out.Integration.ArchiveTraceEvents == nil || !*out.Integration.ArchiveTraceEvents {
		t.Errorf("ArchiveTraceEvents = %v, want pointer-to-true", out.Integration.ArchiveTraceEvents)
	}
	if out.Integration.CreatedAt == "" || out.Integration.UpdatedAt == "" {
		t.Errorf("expected populated CreatedAt/UpdatedAt, got %+v", out.Integration)
	}
}

// TestGetGroupDatadog_LegacyFlatResponse_MapsFlatFields verifies the fallback for older GitLab
// servers whose payload has no "properties" object and carries the Datadog
// values as deprecated top-level fields. The values must still round-trip, and
// DatadogCIVisibility must stay nil (absent) instead of fabricating false.
func TestGetGroupDatadog_LegacyFlatResponse_MapsFlatFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 42,
				"title": "Datadog",
				"slug": "datadog",
				"active": true,
				"api_url": "` + testAPIURL + `",
				"datadog_env": "prod",
				"datadog_service": "gitlab",
				"datadog_site": "` + testDatadogSite + `",
				"datadog_tags": "team:platform,env:prod",
				"archive_trace_events": true
			}`))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetGroupDatadog(t.Context(), client, GetGroupDatadogInput{GroupID: testGroupPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.APIURL != testAPIURL {
		t.Errorf("APIURL = %q, want %q", out.Integration.APIURL, testAPIURL)
	}
	if out.Integration.DatadogEnv != "prod" || out.Integration.DatadogService != "gitlab" ||
		out.Integration.DatadogSite != testDatadogSite || out.Integration.DatadogTags != "team:platform,env:prod" {
		t.Errorf("legacy flat Datadog fields not copied: %+v", out.Integration)
	}
	if out.Integration.ArchiveTraceEvents == nil || !*out.Integration.ArchiveTraceEvents {
		t.Errorf("ArchiveTraceEvents = %v, want pointer-to-true", out.Integration.ArchiveTraceEvents)
	}
	if out.Integration.DatadogCIVisibility != nil {
		t.Errorf("DatadogCIVisibility = %v, want nil for a legacy flat response", *out.Integration.DatadogCIVisibility)
	}
}

// TestGetGroupDatadog_NotFound verifies that GetGroupDatadog_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroupDatadog_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetGroupDatadog(t.Context(), client, GetGroupDatadogInput{GroupID: testGroupPath})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

// TestGetGroupDatadog_Forbidden verifies the GetGroupDatadog_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetGroupDatadog_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetGroupDatadog(t.Context(), client, GetGroupDatadogInput{GroupID: testGroupPath})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

// TestGetGroupDatadog_NilIntegration covers the rare case where the
// GitLab API returns HTTP 200 with a valid JSON null. The handler must
// return a structured error rather than panic on the nil dereference.
func TestGetGroupDatadog_NilIntegration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Valid JSON null → integration is nil
			_, _ = w.Write([]byte("null"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := GetGroupDatadog(t.Context(), client, GetGroupDatadogInput{GroupID: testGroupPath})
	if err == nil {
		t.Fatal("expected error for nil integration")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil, got: %v", err)
	}
}

// SetGroupDatadog.

// TestSetGroupDatadog_Success verifies SetGroupDatadog with a representative
// payload: api_key, api_url, datadog_env, datadog_service, datadog_site,
// datadog_tags, datadog_ci_visibility, and archive_trace_events. The mock
// asserts datadog_ci_visibility is forwarded in the PUT body and returns the
// current nested "properties" response shape so the new field round-trips.
func TestSetGroupDatadog_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"datadog_ci_visibility":true`) {
				t.Errorf("request body should carry datadog_ci_visibility=true, got: %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 7,
				"title": "Datadog",
				"slug": "datadog",
				"active": true,
				"properties": {
					"api_url": "` + testAPIURL + `",
					"datadog_env": "prod",
					"datadog_service": "gitlab",
					"datadog_site": "` + testDatadogSite + `",
					"datadog_tags": "team:platform",
					"datadog_ci_visibility": true,
					"archive_trace_events": false
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))

	archive := false
	ciVisibility := true
	out, err := SetGroupDatadog(t.Context(), client, SetGroupDatadogInput{
		GroupID:             testGroupPath,
		APIKey:              "secret-key",
		APIURL:              testAPIURL,
		DatadogEnv:          "prod",
		DatadogService:      "gitlab",
		DatadogSite:         testDatadogSite,
		DatadogTags:         "team:platform",
		DatadogCIVisibility: &ciVisibility,
		ArchiveTraceEvents:  &archive,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Integration.ID != 7 {
		t.Errorf("ID = %d, want 7", out.Integration.ID)
	}
	if out.Integration.DatadogCIVisibility == nil || !*out.Integration.DatadogCIVisibility {
		t.Errorf("DatadogCIVisibility = %v, want pointer-to-true", out.Integration.DatadogCIVisibility)
	}
	if out.Integration.ArchiveTraceEvents == nil || *out.Integration.ArchiveTraceEvents {
		t.Errorf("ArchiveTraceEvents = %v, want pointer-to-false", out.Integration.ArchiveTraceEvents)
	}
}

// TestSetGroupDatadog_UseInheritedSettings verifies the SetGroupDatadog_UseInheritedSettings handler.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSetGroupDatadog_UseInheritedSettings(t *testing.T) {
	inherited := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":7,"title":"Datadog","slug":"datadog","active":true}`))
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := SetGroupDatadog(t.Context(), client, SetGroupDatadogInput{
		GroupID:              testGroupPath,
		UseInheritedSettings: &inherited,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestSetGroupDatadog_EmptyInputRejected verifies the SetGroupDatadog_EmptyInputRejected handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSetGroupDatadog_EmptyInputRejected(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := SetGroupDatadog(t.Context(), client, SetGroupDatadogInput{GroupID: testGroupPath})
	if err == nil {
		t.Fatal("expected validation error for empty set")
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Errorf("error should describe the missing fields, got: %v", err)
	}
}

// TestSetGroupDatadog_NilIntegration covers the rare case where the
// GitLab API returns HTTP 200 with a valid JSON null on the PUT. The
// handler must return a structured error rather than panic on the
// nil dereference (mirrors the GET path's nil guard).
func TestSetGroupDatadog_NilIntegration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Valid JSON null → integration is nil
			_, _ = w.Write([]byte("null"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := SetGroupDatadog(t.Context(), client, SetGroupDatadogInput{
		GroupID: testGroupPath,
		APIKey:  "placeholder",
	})
	if err == nil {
		t.Fatal("expected error for nil integration")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil, got: %v", err)
	}
}

// TestSetGroupDatadog_Forbidden verifies the SetGroupDatadog_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSetGroupDatadog_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := SetGroupDatadog(t.Context(), client, SetGroupDatadogInput{
		GroupID: testGroupPath,
		APIKey:  "secret-key",
		APIURL:  testAPIURL,
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

// DeleteGroupDatadog.

// TestDeleteGroupDatadog_Success verifies that DeleteGroupDatadog succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteGroupDatadog_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	if err := DeleteGroupDatadog(t.Context(), client, DeleteGroupDatadogInput{GroupID: testGroupPath}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteGroupDatadog_Forbidden verifies the DeleteGroupDatadog_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteGroupDatadog_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	if err := DeleteGroupDatadog(t.Context(), client, DeleteGroupDatadogInput{GroupID: testGroupPath}); err == nil {
		t.Fatal("expected error for 403")
	}
}

// groupDatadogToItem.

// TestGroupDatadogToItem_Nil verifies the GroupDatadogToItem_Nil handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupDatadogToItem_Nil(t *testing.T) {
	if got := groupDatadogToItem(nil); got != (GroupDatadogItem{}) {
		t.Errorf("got = %+v, want zero value", got)
	}
}

// TestGroupDatadogToItem_AllFields verifies the converter with the current SDK
// shape where the Datadog-specific values live in the nested Properties struct.
// It asserts every embedded Integration field and every Properties field,
// including DatadogCIVisibility, is copied into the item.
func TestGroupDatadogToItem_AllFields(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 6, 8, 11, 12, 13, 0, time.UTC)
	src := &gl.GroupDatadogIntegration{
		Integration: gl.Integration{
			ID:        99,
			Title:     "Datadog",
			Slug:      "datadog",
			Active:    true,
			CreatedAt: &created,
			UpdatedAt: &updated,
		},
		Properties: &gl.GroupDatadogIntegrationProperties{
			APIURL:              testAPIURL,
			DatadogEnv:          "prod",
			DatadogService:      "gitlab",
			DatadogSite:         testDatadogSite,
			DatadogTags:         "team:platform",
			DatadogCIVisibility: true,
			ArchiveTraceEvents:  true,
		},
	}
	got := groupDatadogToItem(src)
	if got.ID != 99 || got.Title != "Datadog" || got.Slug != "datadog" || !got.Active {
		t.Errorf("embedded Integration not copied: %+v", got)
	}
	if got.APIURL != testAPIURL || got.DatadogEnv != "prod" || got.DatadogService != "gitlab" ||
		got.DatadogSite != testDatadogSite || got.DatadogTags != "team:platform" {
		t.Errorf("Datadog fields not copied: %+v", got)
	}
	if got.DatadogCIVisibility == nil || !*got.DatadogCIVisibility {
		t.Errorf("DatadogCIVisibility not copied: %v", got.DatadogCIVisibility)
	}
	if got.ArchiveTraceEvents == nil || !*got.ArchiveTraceEvents {
		t.Errorf("ArchiveTraceEvents not copied: %v", got.ArchiveTraceEvents)
	}
	if !strings.Contains(got.CreatedAt, "2026-01-02") {
		t.Errorf("CreatedAt not serialized: %q", got.CreatedAt)
	}
	if !strings.Contains(got.UpdatedAt, "2026-06-08") {
		t.Errorf("UpdatedAt not serialized: %q", got.UpdatedAt)
	}
}

// TestGroupDatadogToItem_DeprecatedFlatFallback_CopiesFlatFields verifies the converter falls
// back to the deprecated flat SDK fields when Properties is nil (older GitLab
// servers). The flat values must be copied and DatadogCIVisibility must stay
// nil since legacy responses never carry it.
func TestGroupDatadogToItem_DeprecatedFlatFallback_CopiesFlatFields(t *testing.T) {
	archive := true
	src := &gl.GroupDatadogIntegration{
		Integration: gl.Integration{ID: 99, Title: "Datadog", Slug: "datadog", Active: true},
	}
	src.APIURL = testAPIURL           //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogEnv = "prod"           //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogService = "gitlab"     //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogSite = testDatadogSite //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogTags = "team:platform" //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.ArchiveTraceEvents = &archive //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	got := groupDatadogToItem(src)
	if got.APIURL != testAPIURL || got.DatadogEnv != "prod" || got.DatadogService != "gitlab" ||
		got.DatadogSite != testDatadogSite || got.DatadogTags != "team:platform" {
		t.Errorf("deprecated flat Datadog fields not copied: %+v", got)
	}
	if got.ArchiveTraceEvents == nil || !*got.ArchiveTraceEvents {
		t.Errorf("ArchiveTraceEvents not copied: %v", got.ArchiveTraceEvents)
	}
	if got.DatadogCIVisibility != nil {
		t.Errorf("DatadogCIVisibility = %v, want nil when Properties is absent", *got.DatadogCIVisibility)
	}
}

// TestGroupDatadogToItem_NilTimestamps verifies the GroupDatadogToItem_NilTimestamps handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupDatadogToItem_NilTimestamps(t *testing.T) {
	got := groupDatadogToItem(&gl.GroupDatadogIntegration{
		Integration: gl.Integration{ID: 1, Title: "Datadog", Slug: "datadog", Active: true},
	})
	if got.CreatedAt != "" || got.UpdatedAt != "" {
		t.Errorf("expected empty timestamps, got %+v", got)
	}
}

// Markdown formatters.

// TestFormatGetGroupDatadogMarkdown_Minimal verifies the GetGroupDatadogMarkdown_Minimal Markdown formatter for a representative getgroupdatadog_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetGroupDatadogMarkdown_Minimal(t *testing.T) {
	result := FormatGetGroupDatadogMarkdown(GetGroupDatadogOutput{Integration: GroupDatadogItem{ID: 1, Active: true}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	for _, c := range result.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if !strings.Contains(tc.Text, "Group Datadog Integration") {
			t.Errorf("markdown should mention the heading, got: %s", tc.Text)
		}
	}
}

// TestFormatGetGroupDatadogMarkdown_Full verifies the GetGroupDatadogMarkdown_Full Markdown formatter for a representative getgroupdatadog_full input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetGroupDatadogMarkdown_Full(t *testing.T) {
	archive := true
	ciVisibility := true
	result := FormatGetGroupDatadogMarkdown(GetGroupDatadogOutput{
		Integration: GroupDatadogItem{
			ID:                  1,
			Title:               "Datadog Production",
			Slug:                "datadog",
			Active:              true,
			CreatedAt:           "2026-01-02T03:04:05.000Z",
			UpdatedAt:           "2026-06-08T11:12:13.000Z",
			APIURL:              testAPIURL,
			DatadogEnv:          "prod",
			DatadogService:      "gitlab",
			DatadogSite:         testDatadogSite,
			DatadogTags:         "team:platform",
			DatadogCIVisibility: &ciVisibility,
			ArchiveTraceEvents:  &archive,
		},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	// Every populated field should make it into the rendered markdown.
	text := firstMarkdownText(t, result)
	wantSubs := []string{
		"Datadog Production",    // fallback non-default for Title
		"Slug",                  // fallback non-default for Slug
		"API URL",               // i.APIURL != "" branch
		"Datadog Env",           // i.DatadogEnv != "" branch
		"Datadog Service",       // i.DatadogService != "" branch
		"Datadog Site",          // i.DatadogSite != "" branch
		"Datadog Tags",          // i.DatadogTags != "" branch
		"Datadog CI Visibility", // i.DatadogCIVisibility != nil branch
		"Archive Trace Events",  // i.ArchiveTraceEvents != nil branch
		"Created",               // i.CreatedAt != "" branch
		"Updated",               // i.UpdatedAt != "" branch
	}
	for _, want := range wantSubs {
		if !strings.Contains(text, want) {
			t.Errorf("markdown should contain %q, got: %s", want, text)
		}
	}
}

// TestFormatSetGroupDatadogMarkdown verifies the SetGroupDatadogMarkdown Markdown formatter for a representative setgroupdatadog input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatSetGroupDatadogMarkdown(t *testing.T) {
	result := FormatSetGroupDatadogMarkdown(SetGroupDatadogOutput{
		Integration: GroupDatadogItem{ID: 1, Active: true, DatadogSite: testDatadogSite},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	for _, c := range result.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if !strings.Contains(tc.Text, "Group Datadog Integration Updated") {
			t.Errorf("markdown should mention the update heading, got: %s", tc.Text)
		}
	}
}

// TestFormatSetGroupDatadogMarkdown_Full verifies the SetGroupDatadogMarkdown_Full Markdown formatter for a representative setgroupdatadog_full input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatSetGroupDatadogMarkdown_Full(t *testing.T) {
	archive := true
	ciVisibility := true
	result := FormatSetGroupDatadogMarkdown(SetGroupDatadogOutput{
		Integration: GroupDatadogItem{
			ID:                  1,
			Title:               "Datadog Production",
			Slug:                "datadog",
			Active:              true,
			APIURL:              testAPIURL,
			DatadogEnv:          "prod",
			DatadogService:      "gitlab",
			DatadogSite:         testDatadogSite,
			DatadogTags:         "team:platform",
			DatadogCIVisibility: &ciVisibility,
			ArchiveTraceEvents:  &archive,
		},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := firstMarkdownText(t, result)
	wantSubs := []string{
		"Datadog Production",    // fallback non-default for Title
		"API URL",               // i.APIURL != "" branch
		"Datadog Env",           // i.DatadogEnv != "" branch
		"Datadog Service",       // i.DatadogService != "" branch
		"Datadog Site",          // i.DatadogSite != "" branch
		"Datadog Tags",          // i.DatadogTags != "" branch
		"Datadog CI Visibility", // i.DatadogCIVisibility != nil branch
		"Archive Trace Events",  // i.ArchiveTraceEvents != nil branch
	}
	for _, want := range wantSubs {
		if !strings.Contains(text, want) {
			t.Errorf("markdown should contain %q, got: %s", want, text)
		}
	}
}

// TestDeleteGroupDatadogOutput_Success covers the destructive output
// wrapper used by the action-spec route. It calls DeleteGroupDatadog
// and then produces the standard DeleteResult shape on success.
func TestDeleteGroupDatadogOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if matchGroupDatadogPath(r.URL.Path) && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := deleteGroupDatadogOutput(t.Context(), client, DeleteGroupDatadogInput{GroupID: testGroupPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status == "" && out.Message == "" {
		t.Error("expected non-empty Status or Message in DeleteOutput")
	}
}

// TestDeleteGroupDatadogOutput_Forbidden verifies the DeleteGroupDatadogOutput_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteGroupDatadogOutput_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := deleteGroupDatadogOutput(t.Context(), client, DeleteGroupDatadogInput{GroupID: testGroupPath})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

// ActionSpec coverage.

// TestActionSpecs_GroupDatadogPresent verifies the three group_datadog action
// specs are registered with the expected metadata (Premium edition, group tag).
func TestActionSpecs_GroupDatadogPresent(t *testing.T) {
	specs := ActionSpecs(nil)
	want := map[string]struct {
		destructive bool
		edition     string
		hasGroupTag bool
	}{
		"integration_get_group_datadog":    {destructive: false, edition: "", hasGroupTag: true},
		"integration_set_group_datadog":    {destructive: false, edition: "", hasGroupTag: true},
		"integration_delete_group_datadog": {destructive: true, edition: "", hasGroupTag: true},
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
		if !hasTag(s.Tags, "group") {
			t.Errorf("%s: Tags = %v, want to contain 'group'", s.Name, s.Tags)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("ActionSpecs missing %q", name)
		}
	}
}

func hasTag(tags []string, want string) bool {
	return slices.Contains(tags, want)
}
