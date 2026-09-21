package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Get.

// GetInput is the input for getting application settings (no parameters needed).
type GetInput struct{}

// GetOutput contains the application settings object GitLab answered with, as
// a JSON map keyed the way the instance spells it.
type GetOutput struct {
	toolutil.HintableOutput
	Settings map[string]any `json:"settings"`
}

// Get retrieves the current application settings (admin-only).
//
// The map is decoded from the captured response (ADR-0021) rather than from
// the settings struct client-go decoded, because this is the one endpoint
// whose whole value is "tell me everything this instance has set" and that
// struct is a model of the settings rather than the settings. Measured against
// the pinned live record (GitLab 19.3.1-ee), the entity exposes 648 field names
// and client-go's Settings carries 424: re-encoding the struct dropped 253 of
// the keys the instance sent and, since one field of the struct has omitempty,
// added 29 keys it never mentioned, each at a plausible zero value. So a model
// asking "is abuse notification configured?" read an empty string GitLab had
// not said anything about, and a setting newer than the SDK's struct answered
// "GitLab does not have that setting".
//
// The SDK call is unchanged, so the route, the options and the retries stay
// what client-go makes them, and its own decode still refuses a body that is
// not the settings object before this reads the same bytes.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (output GetOutput, err error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	if _, _, err = client.GL().Settings.GetSettings(gl.WithContext(ctx)); err != nil {
		return output, toolutil.WrapErrWithStatusHint("settings_get", err, http.StatusForbidden,
			"requires administrator access; this is an instance-wide endpoint. Not available on GitLab.com SaaS")
	}

	values, err := capturedSettings(captured)
	if err != nil {
		return output, toolutil.WrapErr("settings_get", err)
	}

	output.Settings = values
	return output, nil
}

// Update.

// UpdateInput is the input for updating application settings.
// Settings is a map of setting keys to their new values,
// matching the JSON field names from the GitLab API (snake_case).
type UpdateInput struct {
	Settings map[string]any `json:"settings" jsonschema:"Map of setting_name to new value. Use snake_case keys matching GitLab API fields (e.g. signup_enabled, default_project_visibility, max_artifacts_size). A key the GitLab API client does not model is refused by name and nothing is sent, so the patch is all or nothing.,required"`
}

// UpdateOutput contains the application settings object GitLab answered the
// update with, keyed the way the instance spells it.
type UpdateOutput struct {
	toolutil.HintableOutput
	Settings map[string]any `json:"settings"`
}

// Update modifies application settings (admin-only).
//
// It accepts a map of setting keys and values and sends them through
// client-go's UpdateSettingsOptions. That struct models 422 of the 660 params
// GitLab's PUT route declares, and encoding/json drops a member the target
// does not carry without a word, so a patch naming one of the other 238 used
// to be sent with that key missing, answered 200, and reported to the model as
// a reconfiguration that never happened. The keys it does not model are
// therefore named and the whole patch refused before anything is sent: a
// half-applied patch reported as applied is the one outcome worth refusing
// for, which is the condition this repository states for preferring a refusal
// to a decode that fails open.
//
// The answer is read from the captured response for the reason [Get] is.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (output UpdateOutput, err error) {
	if unmodeled := unmodeledSettingKeys(input.Settings); len(unmodeled) > 0 {
		return output, unmodeledSettingsError(len(input.Settings), unmodeled)
	}

	raw, err := json.Marshal(input.Settings)
	if err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", fmt.Errorf("marshal input: %w", err))
	}

	var opts gl.UpdateSettingsOptions
	if err = json.Unmarshal(raw, &opts); err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", fmt.Errorf("unmarshal to options: %w", err))
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	if _, _, err = client.GL().Settings.UpdateSettings(&opts, gl.WithContext(ctx)); err != nil {
		return output, toolutil.WrapErrWithStatusHint("settings_update", err, http.StatusBadRequest,
			"settings keys must use snake_case matching the GitLab API (e.g. signup_enabled, default_project_visibility); use admin.settings_get first to inspect valid keys; requires administrator access")
	}

	values, err := capturedSettings(captured)
	if err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", err)
	}

	output.Settings = values
	return output, nil
}

// capturedSettings reads the settings object out of the answer GitLab sent,
// which is the whole published surface of both handlers and not a field beside
// it. A body that does not decode is reported rather than swallowed.
//
// No answer can actually reach that report, and the reason is worth writing
// down because it is not the usual one: client-go's Settings.UnmarshalJSON
// decodes the whole body into a map[string]any of its own before it fills its
// struct, so every body this map refuses the SDK has already refused, one
// decode earlier. The guard stays because a reader is answerable for what it
// decodes, not because a live exchange can trip it.
func capturedSettings(capture *gitlabclient.ResponseCapture) (map[string]any, error) {
	var values map[string]any
	if err := capture.Decode(&values); err != nil {
		return nil, err
	}
	return values, nil
}

// maxNamedUnmodeledKeys bounds how many keys a refusal spells out. A patch
// naming hundreds of unmodeled settings would otherwise answer with a wall of
// names a model pays to read, and the first twenty are enough to show what the
// refusal is about.
const maxNamedUnmodeledKeys = 20

// unmodeledSettingsError is the refusal, naming what would have been dropped
// and what to do about it.
func unmodeledSettingsError(patched int, unmodeled []string) error {
	named, tail := unmodeled, ""
	if len(named) > maxNamedUnmodeledKeys {
		tail = fmt.Sprintf(" and %d more", len(named)-maxNamedUnmodeledKeys)
		named = named[:maxNamedUnmodeledKeys]
	}
	return fmt.Errorf("settings_update: the GitLab API client this server sends with does not model %d of the %d keys in this patch, so nothing was sent: sending it would have applied the other keys and reported success for all of them. Keys it does not model: %s%s. Remove them to apply the rest; a setting the client does not carry cannot be set through this tool",
		len(unmodeled), patched, strings.Join(named, ", "), tail)
}

// unmodeledSettingKeys returns, sorted, the keys of a patch that
// [gl.UpdateSettingsOptions] does not model and that a decode into it would
// therefore discard.
func unmodeledSettingKeys(patch map[string]any) []string {
	modeled := modeledSettingKeys()
	var unmodeled []string
	for key := range patch {
		if _, ok := modeled[strings.ToLower(key)]; !ok {
			unmodeled = append(unmodeled, key)
		}
	}
	slices.Sort(unmodeled)
	return unmodeled
}

// modeledSettingKeys is the set of setting names client-go's update options
// carry, read from the struct rather than listed here so that a client-go
// release modeling more settings widens what this tool accepts on its own.
//
// The names are keyed in lower case because that is the question being asked:
// not "is this the json tag" but "would encoding/json bind this member to a
// field", and under v1 semantics it matches a member to a field
// case-insensitively when no exact match exists.
var modeledSettingKeys = sync.OnceValue(func() map[string]struct{} {
	keys := make(map[string]struct{})
	collectModeledKeys(reflect.TypeFor[gl.UpdateSettingsOptions](), keys)
	return keys
})

// collectModeledKeys adds the names one struct binds to keys, descending into
// an embedded struct whose fields encoding/json promotes rather than placing
// under a key of their own. Nothing in the options struct is embedded today;
// the descent is here so that a client-go release that factors a group of
// settings into an embedded struct widens the set instead of silently
// narrowing what this tool accepts.
func collectModeledKeys(t reflect.Type, keys map[string]struct{}) {
	for field := range t.Fields() {
		// The order below is encoding/json's own, and the two rules it puts
		// before the exported check are the ones easy to get backwards: an
		// embedded struct of an unexported type still contributes, since its
		// own fields may be exported, and it contributes them under a name
		// when its tag gives one rather than promoting them.
		embedded := field.Type
		if embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		embeddedStruct := field.Anonymous && embedded.Kind() == reflect.Struct
		if !field.IsExported() && !embeddedStruct {
			continue
		}
		tag, tagged := field.Tag.Lookup("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" && embeddedStruct {
			collectModeledKeys(embedded, keys)
			continue
		}
		if !tagged || name == "" {
			name = field.Name
		}
		keys[strings.ToLower(name)] = struct{}{}
	}
}
