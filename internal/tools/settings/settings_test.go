// settings_test.go contains unit tests for the application settings MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package settings

import (
	"context"
	"encoding/json"
	"maps"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// settingsJSON identifies the settings JSON constant used by this package.
const settingsJSON = `{
	"id": 1,
	"signup_enabled": true,
	"default_project_visibility": "private",
	"default_group_visibility": "private",
	"default_snippet_visibility": "internal",
	"can_create_group": true,
	"auto_devops_enabled": false,
	"shared_runners_enabled": true,
	"max_artifacts_size": 100,
	"default_branch_name": "main",
	"password_authentication_enabled_for_web": true,
	"require_two_factor_authentication": false,
	"throttle_authenticated_api_enabled": false
}`

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/settings" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Settings == nil {
		t.Fatal("expected settings map, got nil")
	}
	if val, ok := out.Settings["signup_enabled"]; !ok || val != true {
		t.Errorf("expected signup_enabled=true, got %v", val)
	}
	if val, ok := out.Settings["default_project_visibility"]; !ok || val != "private" {
		t.Errorf("expected default_project_visibility=private, got %v", val)
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
		if r.URL.Path == "/api/v4/application/settings" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{
			"signup_enabled":             false,
			"default_project_visibility": "internal",
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Settings == nil {
		t.Fatal("expected settings map, got nil")
	}
}

// TestUpdate_Error verifies Update when error.
func TestUpdate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{"signup_enabled": false},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_EmptySettings verifies Update when empty settings.
func TestUpdate_EmptySettings(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/settings" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Settings == nil {
		t.Fatal("expected settings map, got nil")
	}
}

// markdownText returns the Markdown a settings formatter wrote.
func markdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("result content = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

// TestFormatGetMarkdown_CuratedKeys_RendersOneSectionPerCategory verifies that
// the settings render as a card with a section per category that has anything
// to show, that a category GitLab sent none of opens no heading, and that the
// coverage sentence counts the rows the card actually wrote.
func TestFormatGetMarkdown_CuratedKeys_RendersOneSectionPerCategory(t *testing.T) {
	out := GetOutput{
		Settings: map[string]any{
			"signup_enabled":             true,
			"default_project_visibility": "private",
			"auto_devops_enabled":        false,
			"default_branch_name":        "main",
		},
	}

	want := "## Application Settings\n\n" +
		"### General\n\n" +
		"- **signup_enabled**: ✅\n" +
		"- **default_project_visibility**: private\n\n" +
		"### CI/CD\n\n" +
		"- **auto_devops_enabled**: ❌\n\n" +
		"### Repository\n\n" +
		"- **default_branch_name**: main\n\n" +
		"Showing all 4 settings GitLab returned.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.settings_update' to change one of these settings\n"

	if got := markdownText(t, FormatGetMarkdown(out)); got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatGetMarkdown_StructuredValues_StayInsideTheirRow verifies that an
// administrator-set value cannot leave the row it was written into.
//
// A setting is an `any` out of GitLab's application-settings JSON and used to be
// written through "%v" with no escaping into a table cell: a multi-line
// sign_in_text ended the row, a pipe split it, and an array or an object
// rendered as Go's container syntax rather than as the JSON it arrived as. The
// case fixes all four at once: the multi-line value collapses onto its row, the
// pipe is an entity, the array is a list a reader can read, and the object is a
// code span holding the document GitLab sent. A key whose value is null or an
// empty string writes no row, and neither counts as shown.
func TestFormatGetMarkdown_StructuredValues_StayInsideTheirRow(t *testing.T) {
	out := GetOutput{
		Settings: map[string]any{
			"sign_in_text":                 "line one\nline two|three",
			"restricted_visibility_levels": []any{"private", "internal"},
			"after_sign_out_path":          "",
			"can_create_group":             nil,
			"max_artifacts_size":           float64(100),
			"two_factor_grace_period":      float64(48),
			"default_branch_protection":    map[string]any{"allowed_to_push": []any{float64(30)}},
			"housekeeping_enabled":         true,
		},
	}

	want := "## Application Settings\n\n" +
		"### General\n\n" +
		"- **sign_in_text**: line one line two&#124;three\n" +
		"- **restricted_visibility_levels**: private, internal\n\n" +
		"### CI/CD\n\n" +
		"- **max_artifacts_size**: 100\n\n" +
		"### Authentication\n\n" +
		"- **two_factor_grace_period**: 48\n\n" +
		"### Repository\n\n" +
		"- **default_branch_protection**: `{\"allowed_to_push\":[30]}`\n\n" +
		"Showing 5 of 8 settings; the remaining keys are in the structured result.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.settings_update' to change one of these settings\n"

	if got := markdownText(t, FormatGetMarkdown(out)); got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatGetMarkdown_NoSettings_SaysSo verifies that a response carrying no
// settings renders the heading and one sentence rather than five empty
// category headings.
func TestFormatGetMarkdown_NoSettings_SaysSo(t *testing.T) {
	want := "## Application Settings\n\n" +
		"GitLab returned no settings.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.settings_update' to change one of these settings\n"

	if got := markdownText(t, FormatGetMarkdown(GetOutput{})); got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatUpdateMarkdown_ReturnsTheSameCard verifies that an update renders
// the object it returned as the card a read renders, under its own heading, and
// that an empty list is written as an answer rather than dropped.
func TestFormatUpdateMarkdown_ReturnsTheSameCard(t *testing.T) {
	out := UpdateOutput{
		Settings: map[string]any{
			"signup_enabled":               false,
			"restricted_visibility_levels": []any{},
		},
	}

	want := "## Application Settings Updated\n\n" +
		"### General\n\n" +
		"- **signup_enabled**: ❌\n" +
		"- **restricted_visibility_levels**: none\n\n" +
		"Showing all 2 settings GitLab returned.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.settings_get' to read the settings back\n"

	if got := markdownText(t, FormatUpdateMarkdown(out)); got != want {
		t.Errorf("FormatUpdateMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestSettingScalar_EveryJSONKind_RendersItsOwnForm verifies the leaf rendering
// each JSON kind gets inside a list value, including the fallback that encodes
// anything else as the JSON document it arrived as.
func TestSettingScalar_EveryJSONKind_RendersItsOwnForm(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "null", value: nil, want: "null"},
		{name: "true", value: true, want: "✅"},
		{name: "false", value: false, want: "❌"},
		{name: "string", value: "private", want: "private"},
		{name: "number", value: float64(48), want: "48"},
		{name: "fractional number", value: 1.5, want: "1.5"},
		{name: "json number", value: json.Number("900719925474099100"), want: "900719925474099100"},
		{name: "nested object", value: map[string]any{"a": float64(1)}, want: `{"a":1}`},
		{name: "nested list", value: []any{"a", "b"}, want: `["a","b"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := settingScalar(tt.value); got != tt.want {
				t.Errorf("settingScalar(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestCompactJSON_ValueJSONRefuses_FallsBackToTheGoForm verifies that a value
// encoding/json cannot encode still renders something rather than an empty row.
//
// Nothing decoded from a GitLab response reaches this branch, which is why it is
// driven with a channel: a settings map only ever holds what encoding/json
// produced, and the fallback exists so the formatter is total over the `any` its
// signature accepts.
func TestCompactJSON_ValueJSONRefuses_FallsBackToTheGoForm(t *testing.T) {
	if got := compactJSON(make(chan int)); got == "" {
		t.Error("compactJSON(chan) = \"\", want the value's Go form")
	}
}

// TestUpdate_MarshalInputError verifies that Update returns an error when the
// input settings map contains a value that cannot be marshaled to JSON (e.g. NaN).
func TestUpdate_MarshalInputError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	_, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{"bad_value": math.NaN()},
	})
	if err == nil {
		t.Fatal("expected error for unmarshalable input, got nil")
	}
	if !strings.Contains(err.Error(), "marshal input") {
		t.Errorf("expected 'marshal input' in error, got: %v", err)
	}
}

// TestUpdate_UnmarshalOptionsError verifies Update rejects values that marshal
// to JSON but cannot be decoded into GitLab update option field types.
func TestUpdate_UnmarshalOptionsError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	_, err := Update(t.Context(), client, UpdateInput{
		Settings: map[string]any{"signup_enabled": "not-a-bool"},
	})
	if err == nil {
		t.Fatal("expected error for invalid option field type")
	}
	if !strings.Contains(err.Error(), "unmarshal to options") {
		t.Errorf("expected 'unmarshal to options' in error, got: %v", err)
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for settings actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "settings" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_update_settings"].ParameterGuidance["settings"].SemanticRole == "" {
		t.Fatal("gitlab_update_settings should define settings parameter guidance")
	}
}

// TestActionSpecs_CallRoutes verifies that both settings canonical routes
// execute successfully through ActionSpecs.
func TestActionSpecs_CallRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})

	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_get_settings", nil},
		{"update", "gitlab_update_settings", map[string]any{"settings": map[string]any{"signup_enabled": false}}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if spec.OwnerPackage != "settings" || !spec.Idempotent || !spec.OpenWorld {
				t.Fatalf("unexpected ActionSpec semantics for %s: %+v", tt.tool, spec)
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

// TestGet_APIError verifies that Get returns a wrapped error when the API fails.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// TestGet_Success_FullRoundTrip verifies that Get handles a complete settings response.
func TestGet_Success_FullRoundTrip(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"signup_enabled":true,"default_project_visibility":"private"}`)
	}))
	out, err := Get(context.Background(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Settings == nil {
		t.Fatal("expected non-nil Settings map")
	}
}

// TestUpdate_APIError verifies that Update returns a wrapped error when the API fails.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// TestUpdate_BadRequest verifies Update includes the settings-key guidance for
// GitLab validation errors.
func TestUpdate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"unknown setting"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Settings: map[string]any{"unknown_setting": true}})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "snake_case") {
		t.Fatalf("error missing settings guidance: %v", err)
	}
}

// TestGet_UnmarshalResponseError documents the contract for Get when the
// API returns a body that the GitLab SDK cannot decode into the Settings
// struct (e.g. a bare number). The SDK rejects it before our json.Unmarshal
// runs, so the in-package json.Unmarshal error branch (settings.go:40-42)
// is defense-in-depth. We assert that an error is returned, which is the
// externally observable contract regardless of which layer surfaces it.
func TestGet_UnmarshalResponseError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A bare JSON number is invalid for the Settings object type.
		testutil.RespondJSON(w, http.StatusOK, `42`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for non-object response, got nil")
	}
}

// TestUpdate_UnmarshalResponseError documents the contract for Update when
// the API returns a body that cannot be decoded into map[string]any. The
// SDK rejects the non-object body before our json.Unmarshal runs, so the
// in-package json.Unmarshal error branch (settings.go:89-91) is
// defense-in-depth. We assert that an error is returned, satisfying the
// contract regardless of which layer surfaces the failure.
func TestUpdate_UnmarshalResponseError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `42`)
	}))
	_, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
	if err == nil {
		t.Fatal("expected error for non-object response, got nil")
	}
}

// TestUpdate_Request_CarriesTheCallersSettingsAsTheBody verifies that the map a
// caller passes is what GitLab receives, key for key and value for value.
//
// Every other Update case here drives the handler and reads its answer, so a
// handler sending an empty options struct satisfied all of them while changing
// nothing on the instance. The four values are deliberately all different, so a
// key that picked up a neighbour's value is visible too.
func TestUpdate_Request_CarriesTheCallersSettingsAsTheBody(t *testing.T) {
	var sent map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/settings" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode request body: %v", err)
			http.Error(w, "decode request body", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	}))

	if _, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{
		"signup_enabled":             false,
		"default_project_visibility": "internal",
		"default_branch_name":        "trunk",
		"max_artifacts_size":         250,
	}}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := []struct {
		key   string
		value any
	}{
		{"signup_enabled", false},
		{"default_project_visibility", "internal"},
		{"default_branch_name", "trunk"},
		{"max_artifacts_size", float64(250)},
	}
	for _, tt := range want {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := sent[tt.key]
			if !ok {
				t.Fatalf("request body has no %q; GitLab was sent %v", tt.key, sent)
			}
			if got != tt.value {
				t.Errorf("request body[%q] = %v, want %v", tt.key, got, tt.value)
			}
		})
	}
}

// TestGet_SettingsMap_IsTheWholeSettingsObject verifies that Get answers with
// the settings object re-keyed the way GitLab spells it: a false and a zero
// GitLab sent survive as themselves, and a key GitLab left out still arrives,
// at its zero value.
//
// That last part is also why the two error branches on this round trip cannot
// fire: what is marshaled is the SDK's struct, so it always encodes, and it
// always encodes as an object, which always decodes into a map.
func TestGet_SettingsMap_IsTheWholeSettingsObject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"signup_enabled":false,"max_artifacts_size":0}`)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := []struct {
		key   string
		value any
	}{
		{"signup_enabled", false},
		{"max_artifacts_size", float64(0)},
		{"default_branch_name", ""},
	}
	for _, tt := range want {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := out.Settings[tt.key]
			if !ok {
				t.Fatalf("Settings has no %q", tt.key)
			}
			if got != tt.value {
				t.Errorf("Settings[%q] = %v, want %v", tt.key, got, tt.value)
			}
		})
	}
	if len(out.Settings) <= len(want) {
		t.Errorf("len(Settings) = %d, want the whole settings object rather than the keys GitLab happened to send", len(out.Settings))
	}
}

// TestGet_Forbidden_CarriesTheAdministratorHint verifies that a 403 reaches the
// caller naming the action and carrying the hint about administrator access.
// The hint is the only thing that tells a model the read was refused for want
// of rights, and it is attached by the status it is keyed to.
func TestGet_Forbidden_CarriesTheAdministratorHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() = nil error, want the 403")
	}
	if !strings.HasPrefix(err.Error(), "settings_get: ") {
		t.Errorf("Get() error = %q, want it to name the settings_get action", err)
	}
	if !strings.Contains(err.Error(), "Suggestion: requires administrator access") {
		t.Errorf("Get() error = %q, want the administrator-access hint", err)
	}
}

// TestGet_NotFound_CarriesNoHint verifies that the administrator hint belongs to
// GitLab's 403 alone: another status still names the action and offers no advice
// about rights, since a hint a model acts on has to be one that applies.
func TestGet_NotFound_CarriesNoHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() = nil error, want the 404")
	}
	if !strings.HasPrefix(err.Error(), "settings_get: ") {
		t.Errorf("Get() error = %q, want it to name the settings_get action", err)
	}
	if strings.Contains(err.Error(), "Suggestion:") {
		t.Errorf("Get() error = %q, want no hint on a status the hint is not keyed to", err)
	}
}

// TestUpdate_Forbidden_CarriesNoSnakeCaseHint verifies that the key-spelling
// guidance belongs to GitLab's 400: a 403 names the action and says nothing
// about snake_case, which would send a model to rewrite a patch already right.
func TestUpdate_Forbidden_CarriesNoSnakeCaseHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
	if err == nil {
		t.Fatal("Update() = nil error, want the 403")
	}
	if !strings.HasPrefix(err.Error(), "settings_update: ") {
		t.Errorf("Update() error = %q, want it to name the settings_update action", err)
	}
	if strings.Contains(err.Error(), "snake_case") {
		t.Errorf("Update() error = %q, want the key-spelling guidance only on a 400", err)
	}
}

// TestSettings_CancelledContext_NeverReachesGitLab verifies that the caller's
// context travels with both requests. A handler that dropped it would keep
// asking GitLab on behalf of a caller who has already gone away, and would
// answer nobody.
func TestSettings_CancelledContext_NeverReachesGitLab(t *testing.T) {
	calls := []struct {
		name string
		call func(context.Context, *gitlabclient.Client) error
	}{
		{"get", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Get(ctx, client, GetInput{})
			return err
		}},
		{"update", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Update(ctx, client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
			return err
		}},
	}

	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			var served atomic.Bool
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				served.Store(true)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			if err := tt.call(ctx, client); err == nil {
				t.Error("call with a cancelled context returned no error")
			}
			if served.Load() {
				t.Error("call with a cancelled context reached GitLab")
			}
		})
	}
}

// TestFormatGetMarkdown_EveryCuratedKey_RendersUnderTheCategoryThatOwnsIt
// verifies that each curated key renders exactly once, under the heading named
// here for it, and that the five headings come out in the declared order.
//
// The expectation is written out rather than read from settingCategories: a key
// moved into the wrong list would move in both and the card would still be
// wrong. The cases above reach seven of the twenty-seven keys and no Rate Limits
// section at all.
func TestFormatGetMarkdown_EveryCuratedKey_RendersUnderTheCategoryThatOwnsIt(t *testing.T) {
	want := map[string]string{
		"signup_enabled":               "General",
		"sign_in_text":                 "General",
		"after_sign_out_path":          "General",
		"default_project_visibility":   "General",
		"default_group_visibility":     "General",
		"default_snippet_visibility":   "General",
		"restricted_visibility_levels": "General",
		"can_create_group":             "General",
		"user_default_external":        "General",

		"auto_devops_enabled":    "CI/CD",
		"auto_devops_domain":     "CI/CD",
		"shared_runners_enabled": "CI/CD",
		"max_artifacts_size":     "CI/CD",
		"default_ci_config_path": "CI/CD",
		"ci_max_includes":        "CI/CD",

		"password_authentication_enabled_for_web": "Authentication",
		"password_authentication_enabled_for_git": "Authentication",
		"two_factor_grace_period":                 "Authentication",
		"require_two_factor_authentication":       "Authentication",

		"default_branch_name":       "Repository",
		"default_branch_protection": "Repository",
		"max_attachment_size":       "Repository",
		"max_import_size":           "Repository",

		"throttle_authenticated_api_enabled":               "Rate Limits",
		"throttle_unauthenticated_api_enabled":             "Rate Limits",
		"throttle_authenticated_api_requests_per_period":   "Rate Limits",
		"throttle_unauthenticated_api_requests_per_period": "Rate Limits",
	}

	// A value derived from its own key, so a row reading a neighbour's value
	// shows up as plainly as a row under the wrong heading.
	values := make(map[string]any, len(want))
	for key := range want {
		values[key] = "value of " + key
	}

	got := markdownText(t, FormatGetMarkdown(GetOutput{Settings: values}))

	section := ""
	headings := []string{}
	rendered := make(map[string]string, len(want))
	for line := range strings.SplitSeq(got, "\n") {
		switch {
		case strings.HasPrefix(line, "### "):
			section = strings.TrimPrefix(line, "### ")
			headings = append(headings, section)
		case strings.HasPrefix(line, "- **"):
			key, value, _ := strings.Cut(strings.TrimPrefix(line, "- **"), "**: ")
			rendered[key] = section
			if value != values[key] {
				t.Errorf("row for %q reads %q, want %q", key, value, values[key])
			}
		}
	}

	if !maps.Equal(rendered, want) {
		t.Errorf("key sections =\n%v\nwant\n%v", rendered, want)
	}
	if wantHeadings := []string{"General", "CI/CD", "Authentication", "Repository", "Rate Limits"}; !slices.Equal(headings, wantHeadings) {
		t.Errorf("headings = %v, want %v", headings, wantHeadings)
	}
}
