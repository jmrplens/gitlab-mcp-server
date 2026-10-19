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
// a JSON map keyed the way the instance spells it. The map is left out rather
// than written as null when there is none, since the output schema types it
// as an object and null fails it.
type GetOutput struct {
	toolutil.HintableOutput
	Settings map[string]any `json:"settings,omitempty"`
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

	values, err := readCapturedSettings(captured)
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
	Settings map[string]any `json:"settings,omitempty"`
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
// The same patch is half applied when GitLab drops a key rather than
// encoding/json, which is what happens to the settings the struct models and
// the route does not declare ([undeclaredSettingKeys]): client-go sends them,
// the route hands its handler the params it declares and no others, and the
// rest of the patch is applied and answered 200. Those keys are refused with
// the others, in the same refusal.
//
// The answer is read from the captured response for the reason [Get] is.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (output UpdateOutput, err error) {
	if unmodeled, undeclared := unsendableSettingKeys(input.Settings); len(unmodeled)+len(undeclared) > 0 {
		return output, unsendableSettingsError(len(input.Settings), unmodeled, undeclared)
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

	values, err := readCapturedSettings(captured)
	if err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", err)
	}

	output.Settings = values
	return output, nil
}

// readCapturedSettings is the reader Get and Update call. It is a test seam:
// since no answer through the transport can make [capturedSettings] fail (see
// below), the handlers' branch for a read that failed is driven by a test
// standing in for one here.
var readCapturedSettings = capturedSettings // test seam

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

// maxNamedUnmodeledKeys bounds how many keys of each kind a refusal spells
// out. A patch naming hundreds of unmodeled settings would otherwise answer
// with a wall of names a model pays to read, and the first twenty are enough
// to show what the refusal is about.
const maxNamedUnmodeledKeys = 20

// unsendableSettingsError is the refusal, naming what would have been dropped,
// under the reason each key would have been dropped for, and what to do about
// it.
func unsendableSettingsError(patched int, unmodeled, undeclared []string) error {
	var named strings.Builder
	if len(unmodeled) > 0 {
		fmt.Fprintf(&named, " Keys the GitLab API client this server sends with does not model: %s.", namedSettingKeys(unmodeled))
	}
	if len(undeclared) > 0 {
		fmt.Fprintf(&named, " Keys GitLab's update route does not take, which it drops although the client models them: %s.", namedSettingKeys(undeclared))
	}
	return fmt.Errorf("settings_update: %d of the %d keys in this patch would be dropped without a word, so nothing was sent: sending it would have applied the other keys and reported success for all of them.%s Remove them to apply the rest; a setting the client does not carry, or the update route does not take, cannot be set through this tool",
		len(unmodeled)+len(undeclared), patched, named.String())
}

// namedSettingKeys spells out up to [maxNamedUnmodeledKeys] of keys and counts
// the rest.
func namedSettingKeys(keys []string) string {
	named, tail := keys, ""
	if len(named) > maxNamedUnmodeledKeys {
		tail = fmt.Sprintf(" and %d more", len(named)-maxNamedUnmodeledKeys)
		named = named[:maxNamedUnmodeledKeys]
	}
	return strings.Join(named, ", ") + tail
}

// unsendableSettingKeys returns, each sorted, the keys of a patch that
// [gl.UpdateSettingsOptions] does not model, which a decode into it would
// discard, and the keys it models that the update route does not declare
// ([undeclaredSettingKeys]), which GitLab would discard.
func unsendableSettingKeys(patch map[string]any) (unmodeled, undeclared []string) {
	modeled := modeledSettingKeys()
	for key := range patch {
		name := strings.ToLower(key)
		if _, ok := modeled[name]; !ok {
			unmodeled = append(unmodeled, key)
			continue
		}
		if _, ok := undeclaredSettingKeys[name]; ok {
			undeclared = append(undeclared, key)
		}
	}
	slices.Sort(unmodeled)
	slices.Sort(undeclared)
	return unmodeled, undeclared
}

// undeclaredSettingKeys are the settings client-go's update options model and
// GitLab's PUT /application/settings does not declare, as the pinned live
// record of GitLab 19.5.0 and client-go v3.29.0 have them (upstream register
// row 135). GitLab's handler reads the declared params alone, so a patch
// naming one of these beside a setting it takes is applied without it and
// answered 200.
//
// The list is written out because the server does not carry the record, and
// TestUndeclaredSettingKeys_AreTheModeledKeysTheUpdateRouteDoesNotDeclare
// holds it to both: a client-go release that models another such setting, or
// a GitLab release that declares or retires one, fails that test until the
// list follows. It retires when client-go stops modeling them.
var undeclaredSettingKeys = map[string]struct{}{
	"autocomplete_users":                                   {},
	"autocomplete_users_unauthenticated":                   {},
	"ci_partitions_size_limit":                             {},
	"container_registry_import_created_before":             {},
	"container_registry_import_max_retries":                {},
	"container_registry_import_max_step_duration":          {},
	"container_registry_import_max_tags_count":             {},
	"container_registry_import_start_max_retries":          {},
	"container_registry_import_target_plan":                {},
	"delayed_group_deletion":                               {},
	"delayed_project_deletion":                             {},
	"enforce_pat_expiration":                               {},
	"enforce_ssh_key_expiration":                           {},
	"help_text":                                            {},
	"housekeeping_bitmaps_enabled":                         {},
	"in_product_marketing_emails_enabled":                  {},
	"pseudonymizer_enabled":                                {},
	"repository_storages":                                  {},
	"send_user_confirmation_email":                         {},
	"sign_in_text":                                         {},
	"suggest_pipeline_enabled":                             {},
	"throttle_unauthenticated_git_lfs_enabled":             {},
	"throttle_unauthenticated_git_lfs_period_in_seconds":   {},
	"throttle_unauthenticated_git_lfs_requests_per_period": {},
	"user_email_lookup_limit":                              {},
	"web_ide_clientside_preview_enabled":                   {},
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
