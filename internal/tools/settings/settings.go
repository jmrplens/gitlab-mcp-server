// Package settings implements MCP tool handlers for GitLab application settings.
// It wraps the SettingsService from client-go v2.
// These are admin-only endpoints requiring administrator access.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Get.

// GetInput is the input for getting application settings (no parameters needed).
type GetInput struct{}

// GetOutput contains the full application settings as a JSON map.
type GetOutput struct {
	toolutil.HintableOutput
	Settings map[string]any `json:"settings"`
}

// Get retrieves the current application settings (admin-only).
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (output GetOutput, err error) {
	settings, _, err := client.GL().Settings.GetSettings(gl.WithContext(ctx))
	if err != nil {
		return output, toolutil.WrapErrWithStatusHint("settings_get", err, http.StatusForbidden,
			"requires administrator access; this is an instance-wide endpoint \u2014 not available on GitLab.com SaaS")
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return output, toolutil.WrapErrWithMessage("settings_get", fmt.Errorf("marshal settings: %w", err))
	}

	var m map[string]any
	if err = json.Unmarshal(raw, &m); err != nil {
		return output, toolutil.WrapErrWithMessage("settings_get", fmt.Errorf("unmarshal settings: %w", err))
	}

	output.Settings = m
	return output, nil
}

// Update.

// UpdateInput is the input for updating application settings.
// Settings is a map of setting keys to their new values,
// matching the JSON field names from the GitLab API (snake_case).
type UpdateInput struct {
	Settings map[string]any `json:"settings" jsonschema:"Map of setting_name to new value. Use snake_case keys matching GitLab API fields (e.g. signup_enabled, default_project_visibility, max_artifacts_size).,required"`
}

// UpdateOutput contains the updated application settings.
type UpdateOutput struct {
	toolutil.HintableOutput
	Settings map[string]any `json:"settings"`
}

// Update modifies application settings (admin-only).
// It accepts a map of setting keys and values, JSON-round-trips them
// into UpdateSettingsOptions, and sends to the GitLab API.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (output UpdateOutput, err error) {
	raw, err := json.Marshal(input.Settings)
	if err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", fmt.Errorf("marshal input: %w", err))
	}

	var opts gl.UpdateSettingsOptions
	if err = json.Unmarshal(raw, &opts); err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", fmt.Errorf("unmarshal to options: %w", err))
	}

	settings, _, err := client.GL().Settings.UpdateSettings(&opts, gl.WithContext(ctx))
	if err != nil {
		return output, toolutil.WrapErrWithStatusHint("settings_update", err, http.StatusBadRequest,
			"settings keys must use snake_case matching the GitLab API (e.g. signup_enabled, default_project_visibility); use gitlab_settings_get first to inspect valid keys; requires administrator access")
	}

	settingsRaw, err := json.Marshal(settings)
	if err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", fmt.Errorf("marshal response: %w", err))
	}

	var m map[string]any
	if err = json.Unmarshal(settingsRaw, &m); err != nil {
		return output, toolutil.WrapErrWithMessage("settings_update", fmt.Errorf("unmarshal response: %w", err))
	}

	output.Settings = m
	return output, nil
}
