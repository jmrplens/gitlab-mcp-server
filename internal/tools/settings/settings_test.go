// settings_test.go contains unit tests for the application settings MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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
		Settings: map[string]any{"max_artifacts_size": math.NaN()},
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

// TestSettings_EachHandlerReachesItsOwnEndpoint drives both handlers against
// the two endpoints GitLab serves the application settings on, so a handler
// sending the wrong method or path fails rather than being answered by a
// catch-all.
func TestSettings_EachHandlerReachesItsOwnEndpoint(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})
	client := testutil.NewTestClient(t, handler)

	t.Run("get", func(t *testing.T) {
		if _, err := Get(t.Context(), client, GetInput{}); err != nil {
			t.Fatalf("Get() error = %v, want nil", err)
		}
	})
	t.Run("update", func(t *testing.T) {
		if _, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}}); err != nil {
			t.Fatalf("Update() error = %v, want nil", err)
		}
	})
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
//
// The patch names a setting the client models, so the refusal below is not in
// the way and GitLab is the one saying no: a key the client does not model
// never reaches the instance at all.
func TestUpdate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"unknown setting"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Settings: map[string]any{"default_branch_name": "trunk"}})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "snake_case") {
		t.Fatalf("error missing settings guidance: %v", err)
	}
}

// TestGet_UnmarshalResponseError documents the contract for Get when the
// API returns a body that is not the settings object (e.g. a bare number).
// The SDK decodes the same bytes into its own struct first and rejects them
// there, so capturedSettings never sees such a body through the transport and
// is driven directly by TestCapturedSettings_BodyThatIsNotAnObject_IsAnError.
// What is asserted here is the externally observable contract, whichever layer
// surfaces it.
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

// TestUpdate_UnmarshalResponseError documents the same contract for Update:
// a body that is not the settings object is an error, and the SDK's own decode
// is what refuses it before capturedSettings reads the same bytes.
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

// TestGet_SettingsMap_IsWhatGitLabSent verifies that Get answers with the
// object the instance sent and with nothing else: a false and a zero survive as
// themselves, a key GitLab sent that client-go's Settings struct does not model
// arrives, and a key that struct carries and GitLab did not send is absent.
//
// All three used to be wrong in the same place. The map was built by marshaling
// *gl.Settings and unmarshaling the result, so its key set was the struct's:
// measured against the pinned live record (GitLab 19.3.1-ee) the entity exposes
// 648 names and the struct carries 424, so 253 of what the instance said was
// dropped and 29 names the instance never mentioned were emitted at their zero
// values, since one field in the whole struct has omitempty. A model asking
// whether a setting was configured read a convincing empty string for a setting
// GitLab had said nothing about.
func TestGet_SettingsMap_IsWhatGitLabSent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"signup_enabled":false,"max_artifacts_size":0,"allow_possible_spam":true}`)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	sent := []struct {
		key   string
		value any
	}{
		{"signup_enabled", false},
		{"max_artifacts_size", float64(0)},
		// A setting GitLab exposes that client-go's struct does not model, so
		// the round trip this replaced could not carry it at all.
		{"allow_possible_spam", true},
	}
	for _, tt := range sent {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := out.Settings[tt.key]
			if !ok {
				t.Fatalf("Settings has no %q; GitLab sent it", tt.key)
			}
			if got != tt.value {
				t.Errorf("Settings[%q] = %v, want %v", tt.key, got, tt.value)
			}
		})
	}

	// The first is a setting the SDK models and this instance did not send; the
	// second is one the SDK carries that no GitLab sends at all.
	for _, key := range []string{"default_branch_name", "admin_notification_email"} {
		t.Run("absent "+key, func(t *testing.T) {
			if value, ok := out.Settings[key]; ok {
				t.Errorf("Settings[%q] = %v, want it absent: GitLab said nothing about it", key, value)
			}
		})
	}

	if len(out.Settings) != len(sent) {
		t.Errorf("len(Settings) = %d, want %d: the keys GitLab sent and no others", len(out.Settings), len(sent))
	}
}

// TestGet_CoverageNote_CountsTheKeysGitLabSent verifies that the sentence under
// the card is about the answer rather than about the client.
//
// The note reads "Showing %d of %d settings; the remaining keys are in the
// structured result", and its total is len(GetOutput.Settings). While that map
// was the re-encoded SDK struct the total was 424 on every instance, every
// version and every tier, and the promise about the remaining keys was false of
// the 253 the round trip had dropped. Taking the map from the answer is what
// makes the sentence true, so the guard belongs on the whole path rather than
// on the formatter, which was always honest about what it was given.
func TestGet_CoverageNote_CountsTheKeysGitLabSent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"signup_enabled":true,"default_branch_name":"main","allow_possible_spam":false,"autocomplete_users_limit":300}`)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	const want = "Showing 2 of 4 settings; the remaining keys are in the structured result."
	if got := markdownText(t, FormatGetMarkdown(out)); !strings.Contains(got, want) {
		t.Errorf("card =\n%s\nwant it to contain %q", got, want)
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

// TestUpdate_KeyTheClientDoesNotModel_RefusedBeforeAnythingIsSent verifies that
// a patch naming a setting client-go's options do not carry is refused by name,
// and that GitLab is not asked at all.
//
// This is the write-side half of the read-side gap, and the more dangerous one.
// encoding/json drops a member the target struct does not declare without a
// word, so the patch used to be sent with that key missing, GitLab answered 200
// for the keys that survived, and the tool rendered a settings card: nothing in
// the exchange said the change had not been made, and the model reported the
// instance reconfigured. Measured against the pinned live record, the PUT route
// declares 660 params and the options struct models 422 of them.
func TestUpdate_KeyTheClientDoesNotModel_RefusedBeforeAnythingIsSent(t *testing.T) {
	var asked atomic.Bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Store(true)
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	}))

	_, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{
		"signup_enabled":      false,
		"allow_possible_spam": true,
	}})
	if err == nil {
		t.Fatal("Update() = nil error, want a refusal naming the key the client does not model")
	}
	if asked.Load() {
		t.Error("Update() reached GitLab; a patch that cannot be sent whole is refused before the request")
	}

	for _, want := range []string{
		"settings_update: ",
		"allow_possible_spam",
		"nothing was sent",
		"1 of the 2 keys",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Update() error = %q, want it to contain %q", err, want)
			}
		})
	}
	if strings.Contains(err.Error(), "signup_enabled") {
		t.Errorf("Update() error = %q, want it to name only the keys that could not be sent", err)
	}
}

// TestUpdate_ManyKeysTheClientDoesNotModel_NamesTwentyAndCountsTheRest verifies
// that the refusal stays readable when a patch names far more keys the client
// cannot send than a reader needs to see, that the count still covers all of
// them, and that a patch sitting exactly on the bound is spelled out whole.
//
// 238 of GitLab's settable params are outside the client's options today, so
// "all of them" is a wall of names a model pays tokens to read before learning
// the one thing it has to do, which is drop them. The bound case is here
// because a cap written with the wrong comparison passes every test that only
// ever exceeds it, and answers "and 0 more" to a patch it named in full.
func TestUpdate_ManyKeysTheClientDoesNotModel_NamesTwentyAndCountsTheRest(t *testing.T) {
	tests := []struct {
		name  string
		keys  int
		named int
		tail  string
	}{
		{name: "exactly the bound", keys: maxNamedUnmodeledKeys, named: maxNamedUnmodeledKeys},
		{name: "over the bound", keys: maxNamedUnmodeledKeys + 5, named: maxNamedUnmodeledKeys, tail: "and 5 more"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.NotFoundHandler())

			patch := make(map[string]any, tt.keys)
			for i := range tt.keys {
				patch[fmt.Sprintf("not_a_gitlab_setting_%02d", i)] = true
			}

			_, err := Update(t.Context(), client, UpdateInput{Settings: patch})
			if err == nil {
				t.Fatal("Update() = nil error, want a refusal")
			}

			if named := strings.Count(err.Error(), "not_a_gitlab_setting_"); named != tt.named {
				t.Errorf("Update() error names %d keys, want %d", named, tt.named)
			}
			switch {
			case tt.tail == "" && strings.Contains(err.Error(), " more"):
				t.Errorf("Update() error = %q, want no tail: every key was named", err)
			case tt.tail != "" && !strings.Contains(err.Error(), tt.tail):
				t.Errorf("Update() error = %q, want it to count the keys it did not name as %q", err, tt.tail)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d of the %d keys", tt.keys, tt.keys)) {
				t.Errorf("Update() error = %q, want it to say the whole patch was outside what the client models", err)
			}
		})
	}
}

// TestUpdate_SettingsMap_IsWhatGitLabSent verifies that the object an update
// answers with is read from GitLab's answer, like the read's.
//
// Both handlers used to re-encode the SDK struct, so an update's card and its
// structured result were as partial and as invented as a read's, and the
// coverage sentence under the card counted the same constant.
func TestUpdate_SettingsMap_IsWhatGitLabSent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/settings" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"signup_enabled":false,"allow_possible_spam":true}`)
	}))

	out, err := Update(t.Context(), client, UpdateInput{Settings: map[string]any{"signup_enabled": false}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got, ok := out.Settings["allow_possible_spam"]; !ok || got != true {
		t.Errorf("Settings[allow_possible_spam] = %v (present %t), want true: GitLab sent it", got, ok)
	}
	if len(out.Settings) != 2 {
		t.Errorf("len(Settings) = %d, want the 2 keys GitLab sent", len(out.Settings))
	}
}

// TestSettings_UnreadableCapturedAnswer_IsTheSDKsOwnRefusal documents why the
// handlers' capture-decode branches cannot be driven through the transport, and
// pins the fact they rest on.
//
// Elsewhere a captured read is failed by poisoning a field client-go does not
// model, since the SDK skips that member and the capture's own type does not.
// That cannot work here twice over: the capture's type is a map, which holds
// every object GitLab could send but for a number outside float64, and
// client-go's Settings.UnmarshalJSON decodes the whole body into a
// map[string]any of its own before it touches its struct, so the SDK refuses
// exactly the bodies this reader would. The guards stay because the reader is
// answerable for what it decodes; nothing on the wire can reach them.
func TestSettings_UnreadableCapturedAnswer_IsTheSDKsOwnRefusal(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"signup_enabled":true,"allow_possible_spam":1e999}`)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() = nil error, want the body refused")
	}
	if strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("Get() error = %q, want the SDK's own refusal: it decodes through a map before its struct", err)
	}
}

// TestCapturedSettings_NoResponse_IsAnError verifies that a capture nothing was
// recorded into is reported rather than read as an instance with no settings.
//
// No handler can reach this through the transport, since a request that
// succeeded was recorded by definition, which is why the reader is driven
// directly here.
func TestCapturedSettings_NoResponse_IsAnError(t *testing.T) {
	_, untouched := gitlabclient.WithResponseCapture(t.Context())

	values, err := capturedSettings(untouched)
	if err == nil {
		t.Fatalf("capturedSettings() = %v, nil error; want the no-response error", values)
	}
	if !errors.Is(err, gitlabclient.ErrNoResponseCaptured) {
		t.Errorf("capturedSettings() error = %v, want ErrNoResponseCaptured", err)
	}
}

// TestCapturedSettings_BodyThatIsNotAnObject_IsAnError verifies that a body the
// settings map cannot hold is reported rather than answered as an empty map.
//
// The SDK decodes the same bytes into its own struct first and refuses such a
// body there, so this branch is defense in depth and is driven from a capture
// built by hand.
func TestCapturedSettings_BodyThatIsNotAnObject_IsAnError(t *testing.T) {
	values, err := capturedSettings(gitlabclient.CapturedBody([]byte("42")))
	if err == nil {
		t.Fatalf("capturedSettings() = %v, nil error; want the decode to be reported", values)
	}
}

// TestModeledSettingKeys_ReadTheOptionsStructRatherThanAList verifies that the
// accepted key set is the one client-go's update options actually model.
//
// The set is read from the struct so that a client-go release modeling more
// settings widens what this tool accepts with no edit here, which is the whole
// reason it is not a literal list: a list would have to be maintained against
// every bump, and a stale one refuses settings the client can send.
func TestModeledSettingKeys_ReadTheOptionsStructRatherThanAList(t *testing.T) {
	modeled := modeledSettingKeys()

	if len(modeled) < 400 {
		t.Errorf("modeledSettingKeys() holds %d names, want the several hundred the options struct carries", len(modeled))
	}
	for _, key := range []string{"signup_enabled", "default_branch_name", "max_artifacts_size"} {
		t.Run(key, func(t *testing.T) {
			if _, ok := modeled[key]; !ok {
				t.Errorf("modeledSettingKeys() has no %q, which client-go models", key)
			}
		})
	}
	if _, ok := modeled["allow_possible_spam"]; ok {
		t.Error("modeledSettingKeys() holds allow_possible_spam, which client-go does not model")
	}
}

// TestCollectModeledKeys_EveryFieldShape_BindsTheNameEncodingJSONWould verifies
// the walk over one options struct: an embedded struct and an embedded pointer
// to one contribute their promoted members, a tagged field its tag, an untagged
// field and a tag carrying only options their Go names, and `json:"-"` and an
// unexported field nothing at all.
//
// The walk is exercised on a fixture rather than only on client-go's struct
// because that struct has no embedded field today: the descent exists so that a
// release which factors settings into one widens the accepted set instead of
// quietly refusing settings the client can send, and a branch nothing drives is
// a branch nothing holds to that promise.
func TestCollectModeledKeys_EveryFieldShape_BindsTheNameEncodingJSONWould(t *testing.T) {
	// A group of settings a client-go release could factor into an embedded
	// struct, and the same thing embedded by pointer.
	type promotedGroup struct {
		Promoted *bool `json:"promoted,omitempty"`
	}
	type deepGroup struct {
		Deep *bool `json:"deep,omitempty"`
	}
	// An embedded struct carrying a json name, which encoding/json places
	// under that name instead of promoting, and an embedded non-struct type,
	// which it binds by the type's own name.
	type namedGroup struct {
		Inner *bool `json:"inner,omitempty"`
	}
	type EmbeddedScalar string
	type keyShapes struct {
		promotedGroup
		*deepGroup
		namedGroup `json:"asgroup"`
		EmbeddedScalar
		Tagged      *bool `json:"tagged,omitempty"`
		Untagged    *bool
		OptionsOnly *bool          `json:",omitempty"`
		Skipped     *bool          `json:"-"`
		Nested      *promotedGroup `json:"nested,omitempty"`
		unexported  *bool
	}
	// A field encoding/json never writes and whose name it never binds; read
	// here because it is the fixture's reason for existing.
	_ = keyShapes{}.unexported

	keys := make(map[string]struct{})
	collectModeledKeys(reflect.TypeFor[keyShapes](), keys)

	got := make([]string, 0, len(keys))
	for key := range keys {
		got = append(got, key)
	}
	slices.Sort(got)

	want := []string{"asgroup", "deep", "embeddedscalar", "nested", "optionsonly", "promoted", "tagged", "untagged"}
	if !slices.Equal(got, want) {
		t.Errorf("collectModeledKeys() = %v, want %v", got, want)
	}
}
