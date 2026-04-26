// Package errortracking implements MCP tools for GitLab Error Tracking operations.
package errortracking

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetSettings.

// GetSettingsInput contains parameters for getting error tracking settings.
type GetSettingsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// SettingsOutput contains error tracking settings.
type SettingsOutput struct {
	toolutil.HintableOutput
	Active            bool   `json:"active"`
	ProjectName       string `json:"project_name"`
	SentryExternalURL string `json:"sentry_external_url,omitempty"`
	APIURL            string `json:"api_url,omitempty"`
	Integrated        bool   `json:"integrated"`
}

// GetSettings retrieves error tracking settings for a project.
func GetSettings(ctx context.Context, client *gitlabclient.Client, input GetSettingsInput) (SettingsOutput, error) {
	s, _, err := client.GL().ErrorTracking.GetErrorTrackingSettings(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return SettingsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_error_tracking_settings", err, http.StatusForbidden,
			"requires Maintainer role on the project; verify project_id with gitlab_project_list; error tracking must be enabled at the instance level (Sentry integration or GitLab-integrated)")
	}
	return SettingsOutput{
		Active:            s.Active,
		ProjectName:       s.ProjectName,
		SentryExternalURL: s.SentryExternalURL,
		APIURL:            s.APIURL,
		Integrated:        s.Integrated,
	}, nil
}

// EnableDisable.

// EnableDisableInput contains parameters for enabling/disabling error tracking.
type EnableDisableInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Active     *bool                `json:"active" jsonschema:"Enable or disable error tracking"`
	Integrated *bool                `json:"integrated" jsonschema:"Use integrated error tracking"`
}

// EnableDisable enables or disables error tracking for a project.
func EnableDisable(ctx context.Context, client *gitlabclient.Client, input EnableDisableInput) (SettingsOutput, error) {
	opts := &gl.EnableDisableErrorTrackingOptions{
		Active:     input.Active,
		Integrated: input.Integrated,
	}
	s, _, err := client.GL().ErrorTracking.EnableDisableErrorTracking(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return SettingsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_enable_disable_error_tracking", err, http.StatusBadRequest,
			"requires Maintainer role; active=true requires error tracking to be configured (Sentry or GitLab-integrated); integrated_error_tracking flag toggles between Sentry and GitLab backend")
	}
	return SettingsOutput{
		Active:            s.Active,
		ProjectName:       s.ProjectName,
		SentryExternalURL: s.SentryExternalURL,
		APIURL:            s.APIURL,
		Integrated:        s.Integrated,
	}, nil
}

// ListClientKeys.

// ListClientKeysInput contains parameters for listing error tracking client keys.
type ListClientKeysInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int64                `json:"page" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page" jsonschema:"Number of items per page"`
}

// ClientKeyItem represents an error tracking client key.
type ClientKeyItem struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	Active    bool   `json:"active"`
	PublicKey string `json:"public_key"`
	SentryDsn string `json:"sentry_dsn"`
}

// ListClientKeysOutput contains a list of error tracking client keys.
type ListClientKeysOutput struct {
	toolutil.HintableOutput
	Keys       []ClientKeyItem           `json:"keys"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListClientKeys retrieves error tracking client keys for a project.
func ListClientKeys(ctx context.Context, client *gitlabclient.Client, input ListClientKeysInput) (ListClientKeysOutput, error) {
	opts := &gl.ListClientKeysOptions{}
	if input.Page > 0 || input.PerPage > 0 {
		opts.ListOptions = gl.ListOptions{Page: input.Page, PerPage: input.PerPage}
	}
	keys, resp, err := client.GL().ErrorTracking.ListClientKeys(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListClientKeysOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_error_tracking_client_keys", err, http.StatusForbidden,
			"requires Maintainer role; client keys are used by SDK clients to send events; only available with GitLab-integrated error tracking (not Sentry)")
	}
	items := make([]ClientKeyItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, ClientKeyItem{
			ID:        k.ID,
			Active:    k.Active,
			PublicKey: k.PublicKey,
			SentryDsn: k.SentryDsn,
		})
	}
	return ListClientKeysOutput{
		Keys:       items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// CreateClientKey.

// CreateClientKeyInput contains parameters for creating a client key.
type CreateClientKeyInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// CreateClientKey creates a new error tracking client key.
func CreateClientKey(ctx context.Context, client *gitlabclient.Client, input CreateClientKeyInput) (ClientKeyItem, error) {
	k, _, err := client.GL().ErrorTracking.CreateClientKey(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return ClientKeyItem{}, toolutil.WrapErrWithStatusHint("gitlab_create_error_tracking_client_key", err, http.StatusBadRequest,
			"requires Maintainer role; only available with GitLab-integrated error tracking; the returned public_key is used as the Sentry DSN by SDK clients")
	}
	return ClientKeyItem{
		ID:        k.ID,
		Active:    k.Active,
		PublicKey: k.PublicKey,
		SentryDsn: k.SentryDsn,
	}, nil
}

// DeleteClientKey.

// DeleteClientKeyInput contains parameters for deleting a client key.
type DeleteClientKeyInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	KeyID     int64                `json:"key_id" jsonschema:"Client key ID,required"`
}

// DeleteClientKey deletes an error tracking client key.
func DeleteClientKey(ctx context.Context, client *gitlabclient.Client, input DeleteClientKeyInput) error {
	if input.KeyID <= 0 {
		return toolutil.ErrRequiredInt64("delete_error_tracking_client_key", "key_id")
	}
	_, err := client.GL().ErrorTracking.DeleteClientKey(string(input.ProjectID), input.KeyID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_delete_error_tracking_client_key", err, http.StatusForbidden,
			"requires Maintainer role; verify key_id with gitlab_list_error_tracking_client_keys; deletion is irreversible \u2014 SDK clients using the key will stop receiving events")
	}
	return nil
}

// formatters.
