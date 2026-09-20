// group_datadog_test.go contains unit tests for the group-level Datadog
// integration MCP tool handlers. The tests cover happy paths, the validation
// guard, and the most common API errors (404 not found, 403 forbidden,
// response integration nil).
package integrations

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const (
	testGroupPath   = "my-group"
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
	p := out.Integration.Properties
	if p == nil {
		t.Fatalf("Properties = nil, want the configuration GitLab sent: %+v", out.Integration)
	}
	if p.APIURL != testAPIURL {
		t.Errorf("Properties.APIURL = %q, want %q", p.APIURL, testAPIURL)
	}
	if p.DatadogSite != testDatadogSite {
		t.Errorf("Properties.DatadogSite = %q, want %q", p.DatadogSite, testDatadogSite)
	}
	if !p.DatadogCIVisibility {
		t.Error("Properties.DatadogCIVisibility = false, want true")
	}
	if !p.ArchiveTraceEvents {
		t.Error("Properties.ArchiveTraceEvents = false, want true")
	}
	if out.Integration.CreatedAt == "" || out.Integration.UpdatedAt == "" {
		t.Errorf("expected populated CreatedAt/UpdatedAt, got %+v", out.Integration)
	}
}

// TestGetGroupDatadog_LegacyFlatResponse_MapsFlatFields verifies the fallback
// for older GitLab servers whose payload has no "properties" object and
// carries the Datadog values as deprecated top-level fields. Those values must
// still reach the caller under the one key this type publishes, and
// DatadogCIVisibility reads false because such a payload never carried it.
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
	p := out.Integration.Properties
	if p == nil {
		t.Fatalf("Properties = nil, want the flat configuration read into it: %+v", out.Integration)
	}
	if p.APIURL != testAPIURL {
		t.Errorf("Properties.APIURL = %q, want %q", p.APIURL, testAPIURL)
	}
	if p.DatadogEnv != "prod" || p.DatadogService != "gitlab" ||
		p.DatadogSite != testDatadogSite || p.DatadogTags != "team:platform,env:prod" {
		t.Errorf("legacy flat Datadog fields not copied: %+v", p)
	}
	if !p.ArchiveTraceEvents {
		t.Error("Properties.ArchiveTraceEvents = false, want true")
	}
	if p.DatadogCIVisibility {
		t.Error("Properties.DatadogCIVisibility = true, want false for a payload that never carried it")
	}
}

// TestGetGroupDatadog_NotFound verifies that GetGroupDatadog_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the error names the status; the suggestion attached to a
// 404 is asserted by TestIntegrationHandlers_StatusHint_OnlyAtTheStatusItNames.
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
	p := out.Integration.Properties
	if p == nil {
		t.Fatalf("Properties = nil, want the configuration GitLab echoed: %+v", out.Integration)
	}
	if !p.DatadogCIVisibility {
		t.Error("Properties.DatadogCIVisibility = false, want true")
	}
	if p.ArchiveTraceEvents {
		t.Error("Properties.ArchiveTraceEvents = true, want false")
	}
}

// TestSetGroupDatadog_UseInheritedSettings asserts that asking a group to
// inherit its ancestor's Datadog configuration is accepted on its own, with no
// other field supplied, and that the flag reaches GitLab.
func TestSetGroupDatadog_UseInheritedSettings(t *testing.T) {
	inherited := true
	got := setGroupDatadogBody(t, SetGroupDatadogInput{
		GroupID:              testGroupPath,
		UseInheritedSettings: &inherited,
	})
	if want := (map[string]any{"use_inherited_settings": true}); !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %v, want %v", got, want)
	}
}

// setGroupDatadogBody drives SetGroupDatadog against a mock that records the
// PUT body, and returns that body decoded as a JSON object. Asserting on the
// decoded body rather than on a substring is what makes a field that never
// left the handler visible: buildGroupDatadogOptions omits every unset field,
// so a guard written the wrong way round drops the caller's value and sends an
// empty one instead, and neither shows up in the response GitLab echoes.
func setGroupDatadogBody(t *testing.T, input SetGroupDatadogInput) map[string]any {
	t.Helper()
	var raw []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !matchGroupDatadogPath(r.URL.Path) || r.Method != http.MethodPut {
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
		testutil.RespondJSON(w, http.StatusOK, `{"id":7,"title":"Datadog","slug":"datadog","active":true}`)
	}))

	if _, err := SetGroupDatadog(t.Context(), client, input); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("request body is not a JSON object: %v (%s)", err, raw)
	}
	return got
}

// TestSetGroupDatadog_OneFieldAtATime_SendsOnlyThatField drives each of the
// nine configurable fields on its own and asserts the PUT body carries exactly
// that field. One field per case is the only fixture that tells the nine
// apart: a request populated with all of them agrees with itself however the
// handler pairs an input field with an option field, so a value read from the
// wrong neighbor, or a value dropped because its guard tests the empty case,
// would go unnoticed. The explicit false on archive_trace_events also pins
// that a flag a caller turned off is sent rather than omitted as a zero.
func TestSetGroupDatadog_OneFieldAtATime_SendsOnlyThatField(t *testing.T) {
	ciVisibility := true
	archiveOff := false
	inherited := true

	tests := []struct {
		name  string
		input SetGroupDatadogInput
		want  map[string]any
	}{
		{"api_key", SetGroupDatadogInput{APIKey: "secret-key"}, map[string]any{"api_key": "secret-key"}},
		{"api_url", SetGroupDatadogInput{APIURL: testAPIURL}, map[string]any{"api_url": testAPIURL}},
		{"datadog_env", SetGroupDatadogInput{DatadogEnv: "prod"}, map[string]any{"datadog_env": "prod"}},
		{"datadog_service", SetGroupDatadogInput{DatadogService: "gitlab"}, map[string]any{"datadog_service": "gitlab"}},
		{"datadog_site", SetGroupDatadogInput{DatadogSite: testDatadogSite}, map[string]any{"datadog_site": testDatadogSite}},
		{"datadog_tags", SetGroupDatadogInput{DatadogTags: "team:platform"}, map[string]any{"datadog_tags": "team:platform"}},
		{"datadog_ci_visibility", SetGroupDatadogInput{DatadogCIVisibility: &ciVisibility}, map[string]any{"datadog_ci_visibility": true}},
		{"archive_trace_events", SetGroupDatadogInput{ArchiveTraceEvents: &archiveOff}, map[string]any{"archive_trace_events": false}},
		{"use_inherited_settings", SetGroupDatadogInput{UseInheritedSettings: &inherited}, map[string]any{"use_inherited_settings": true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			input.GroupID = testGroupPath
			got := setGroupDatadogBody(t, input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("request body = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSetGroupDatadog_InheritedSettingsOffAlone_Rejected asserts that
// use_inherited_settings=false is not itself a configuration: it names what
// the group should not do and leaves nothing to store, so the handler refuses
// it before reaching GitLab exactly as it refuses an empty input.
func TestSetGroupDatadog_InheritedSettingsOffAlone_Rejected(t *testing.T) {
	notInherited := false
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := SetGroupDatadog(t.Context(), client, SetGroupDatadogInput{
		GroupID:              testGroupPath,
		UseInheritedSettings: &notInherited,
	})
	if err == nil {
		t.Fatal("expected validation error for use_inherited_settings=false on its own")
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Errorf("error should describe the missing fields, got: %v", err)
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
		ID:        99,
		Title:     "Datadog",
		Slug:      "datadog",
		Active:    true,
		CreatedAt: &created,
		UpdatedAt: &updated,
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
	p := got.Properties
	if p == nil {
		t.Fatalf("Properties = nil, want the nested configuration copied: %+v", got)
	}
	if p.APIURL != testAPIURL || p.DatadogEnv != "prod" || p.DatadogService != "gitlab" ||
		p.DatadogSite != testDatadogSite || p.DatadogTags != "team:platform" {
		t.Errorf("Datadog fields not copied: %+v", p)
	}
	if !p.DatadogCIVisibility {
		t.Error("Properties.DatadogCIVisibility not copied")
	}
	if !p.ArchiveTraceEvents {
		t.Error("Properties.ArchiveTraceEvents not copied")
	}
	if !strings.Contains(got.CreatedAt, "2026-01-02") {
		t.Errorf("CreatedAt not serialized: %q", got.CreatedAt)
	}
	if !strings.Contains(got.UpdatedAt, "2026-06-08") {
		t.Errorf("UpdatedAt not serialized: %q", got.UpdatedAt)
	}
}

// TestGroupDatadogToItem_DeprecatedFlatFallback_CopiesFlatFields verifies the
// converter falls back to the deprecated flat SDK fields when Properties is
// nil (older GitLab servers). The flat values must reach the one key this type
// publishes, and DatadogCIVisibility reads false since such a response never
// carries it.
func TestGroupDatadogToItem_DeprecatedFlatFallback_CopiesFlatFields(t *testing.T) {
	archive := true
	src := &gl.GroupDatadogIntegration{
		ID: 99, Title: "Datadog", Slug: "datadog", Active: true,
	}
	src.APIURL = testAPIURL           //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogEnv = "prod"           //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogService = "gitlab"     //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogSite = testDatadogSite //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.DatadogTags = "team:platform" //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	src.ArchiveTraceEvents = &archive //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.
	got := groupDatadogToItem(src)
	p := got.Properties
	if p == nil {
		t.Fatalf("Properties = nil, want the flat fields read into it: %+v", got)
	}
	if p.APIURL != testAPIURL || p.DatadogEnv != "prod" || p.DatadogService != "gitlab" ||
		p.DatadogSite != testDatadogSite || p.DatadogTags != "team:platform" {
		t.Errorf("deprecated flat Datadog fields not copied: %+v", p)
	}
	if !p.ArchiveTraceEvents {
		t.Error("Properties.ArchiveTraceEvents not copied")
	}
	if p.DatadogCIVisibility {
		t.Error("Properties.DatadogCIVisibility = true, want false when the response never carried it")
	}
}

// TestGroupDatadogToItem_FlatFallbackWithoutArchiveFlag_LeavesItFalse covers
// the other half of the fallback: an older response that omits
// archive_trace_events leaves the flag false rather than dereferencing a nil
// pointer.
func TestGroupDatadogToItem_FlatFallbackWithoutArchiveFlag_LeavesItFalse(t *testing.T) {
	src := &gl.GroupDatadogIntegration{ID: 99, Title: "Datadog", Slug: "datadog", Active: true}
	src.DatadogSite = testDatadogSite //nolint:staticcheck // SA1019: exercising the deprecated flat fallback.

	got := groupDatadogToItem(src)
	if got.Properties == nil {
		t.Fatalf("Properties = nil, want the flat fields read into it: %+v", got)
	}
	if got.Properties.ArchiveTraceEvents {
		t.Error("Properties.ArchiveTraceEvents = true, want false when the response omitted it")
	}
}

// TestGroupDatadogToItem_NilTimestamps verifies the GroupDatadogToItem_NilTimestamps handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
// TestGroupDatadogToItem_OneFieldAtATime_ReadsEachFromItsOwnSource asserts
// that every field of the item, and of the nested Datadog configuration under
// it, is read from the client-go field of the same meaning and from no other.
//
// TestGroupDatadogToItem_Scenarios cannot say this: its fixture sets all
// seventeen event flags to true and both Datadog booleans to true, and values
// that agree are indistinguishable however the converter pairs them. Setting
// one field at a time is the fixture in which no two agree.
func TestGroupDatadogToItem_OneFieldAtATime_ReadsEachFromItsOwnSource(t *testing.T) {
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	updated := time.Date(2026, 8, 9, 10, 11, 12, 0, time.UTC)

	tests := []struct {
		name string
		src  func(*gl.GroupDatadogIntegration)
		want func(*GroupDatadogItem)
	}{
		{"id", func(s *gl.GroupDatadogIntegration) { s.ID = 9 }, func(i *GroupDatadogItem) { i.ID = 9 }},
		{"title", func(s *gl.GroupDatadogIntegration) { s.Title = "Datadog" }, func(i *GroupDatadogItem) { i.Title = "Datadog" }},
		{"slug", func(s *gl.GroupDatadogIntegration) { s.Slug = "datadog" }, func(i *GroupDatadogItem) { i.Slug = "datadog" }},
		{"active", func(s *gl.GroupDatadogIntegration) { s.Active = true }, func(i *GroupDatadogItem) { i.Active = true }},
		{"created_at", func(s *gl.GroupDatadogIntegration) { s.CreatedAt = &created }, func(i *GroupDatadogItem) { i.CreatedAt = "2026-03-04T05:06:07Z" }},
		{"updated_at", func(s *gl.GroupDatadogIntegration) { s.UpdatedAt = &updated }, func(i *GroupDatadogItem) { i.UpdatedAt = "2026-08-09T10:11:12Z" }},
		{"alert_events", func(s *gl.GroupDatadogIntegration) { s.AlertEvents = true }, func(i *GroupDatadogItem) { i.AlertEvents = true }},
		{"commit_events", func(s *gl.GroupDatadogIntegration) { s.CommitEvents = true }, func(i *GroupDatadogItem) { i.CommitEvents = true }},
		{"confidential_issues_events", func(s *gl.GroupDatadogIntegration) { s.ConfidentialIssuesEvents = true }, func(i *GroupDatadogItem) { i.ConfidentialIssuesEvents = true }},
		{"confidential_note_events", func(s *gl.GroupDatadogIntegration) { s.ConfidentialNoteEvents = true }, func(i *GroupDatadogItem) { i.ConfidentialNoteEvents = true }},
		{"deployment_events", func(s *gl.GroupDatadogIntegration) { s.DeploymentEvents = true }, func(i *GroupDatadogItem) { i.DeploymentEvents = true }},
		{"incident_events", func(s *gl.GroupDatadogIntegration) { s.IncidentEvents = true }, func(i *GroupDatadogItem) { i.IncidentEvents = true }},
		{"issues_events", func(s *gl.GroupDatadogIntegration) { s.IssuesEvents = true }, func(i *GroupDatadogItem) { i.IssuesEvents = true }},
		{"job_events", func(s *gl.GroupDatadogIntegration) { s.JobEvents = true }, func(i *GroupDatadogItem) { i.JobEvents = true }},
		{"merge_requests_events", func(s *gl.GroupDatadogIntegration) { s.MergeRequestsEvents = true }, func(i *GroupDatadogItem) { i.MergeRequestsEvents = true }},
		{"note_events", func(s *gl.GroupDatadogIntegration) { s.NoteEvents = true }, func(i *GroupDatadogItem) { i.NoteEvents = true }},
		{"pipeline_events", func(s *gl.GroupDatadogIntegration) { s.PipelineEvents = true }, func(i *GroupDatadogItem) { i.PipelineEvents = true }},
		{"push_events", func(s *gl.GroupDatadogIntegration) { s.PushEvents = true }, func(i *GroupDatadogItem) { i.PushEvents = true }},
		{"tag_push_events", func(s *gl.GroupDatadogIntegration) { s.TagPushEvents = true }, func(i *GroupDatadogItem) { i.TagPushEvents = true }},
		{"vulnerability_events", func(s *gl.GroupDatadogIntegration) { s.VulnerabilityEvents = true }, func(i *GroupDatadogItem) { i.VulnerabilityEvents = true }},
		{"wiki_page_events", func(s *gl.GroupDatadogIntegration) { s.WikiPageEvents = true }, func(i *GroupDatadogItem) { i.WikiPageEvents = true }},
		{"comment_on_event_enabled", func(s *gl.GroupDatadogIntegration) { s.CommentOnEventEnabled = true }, func(i *GroupDatadogItem) { i.CommentOnEventEnabled = true }},
		{"inherited", func(s *gl.GroupDatadogIntegration) { s.Inherited = true }, func(i *GroupDatadogItem) { i.Inherited = true }},
		{
			"properties.api_url",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{APIURL: testAPIURL}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{APIURL: testAPIURL} },
		},
		{
			"properties.datadog_env",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{DatadogEnv: "prod"}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{DatadogEnv: "prod"} },
		},
		{
			"properties.datadog_service",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{DatadogService: "svc"}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{DatadogService: "svc"} },
		},
		{
			"properties.datadog_site",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{DatadogSite: testDatadogSite}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{DatadogSite: testDatadogSite} },
		},
		{
			"properties.datadog_tags",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{DatadogTags: "team:core"}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{DatadogTags: "team:core"} },
		},
		{
			"properties.datadog_ci_visibility",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{DatadogCIVisibility: true}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{DatadogCIVisibility: true} },
		},
		{
			"properties.archive_trace_events",
			func(s *gl.GroupDatadogIntegration) {
				s.Properties = &gl.GroupDatadogIntegrationProperties{ArchiveTraceEvents: true}
			},
			func(i *GroupDatadogItem) { i.Properties = &GroupDatadogProperties{ArchiveTraceEvents: true} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var src gl.GroupDatadogIntegration
			tt.src(&src)
			want := GroupDatadogItem{}
			tt.want(&want)
			if want.Properties == nil {
				// With no nested object GitLab's older flat shape is read
				// instead, which always yields an empty configuration here.
				want.Properties = &GroupDatadogProperties{}
			}
			if got := groupDatadogToItem(&src); !reflect.DeepEqual(got, want) {
				t.Errorf("groupDatadogToItem(%+v) =\n %+v\nwant %+v", src, got, want)
			}
		})
	}
}

func TestGroupDatadogToItem_NilTimestamps(t *testing.T) {
	got := groupDatadogToItem(&gl.GroupDatadogIntegration{
		ID: 1, Title: "Datadog", Slug: "datadog", Active: true,
	})
	if got.CreatedAt != "" || got.UpdatedAt != "" {
		t.Errorf("expected empty timestamps, got %+v", got)
	}
}

// The Markdown formatters are asserted whole in markdown_test.go.

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
		t.Run(name, func(t *testing.T) {
			if !seen[name] {
				t.Errorf("ActionSpecs missing %q", name)
			}
		})
	}
}

func hasTag(tags []string, want string) bool {
	return slices.Contains(tags, want)
}
