package license

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Types.

// LicenseeItem represents the licensee of a GitLab license.
type LicenseeItem struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Email   string `json:"email"`
}

// AddOnsItem represents the add-ons of a GitLab license.
type AddOnsItem struct {
	GitLabAuditorUser int64 `json:"gitlab_auditor_user"`
	GitLabDeployBoard int64 `json:"gitlab_deploy_board"`
	GitLabFileLocks   int64 `json:"gitlab_file_locks"`
	GitLabGeo         int64 `json:"gitlab_geo"`
	GitLabServiceDesk int64 `json:"gitlab_service_desk"`
}

// Item represents a GitLab license.
type Item struct {
	ID               int64        `json:"id"`
	Plan             string       `json:"plan"`
	CreatedAt        string       `json:"created_at,omitempty"`
	StartsAt         string       `json:"starts_at,omitempty"`
	ExpiresAt        string       `json:"expires_at,omitempty"`
	HistoricalMax    int64        `json:"historical_max"`
	MaximumUserCount int64        `json:"maximum_user_count"`
	Expired          bool         `json:"expired"`
	Overage          int64        `json:"overage"`
	UserLimit        int64        `json:"user_limit"`
	ActiveUsers      int64        `json:"active_users"`
	Licensee         LicenseeItem `json:"licensee"`
	AddOns           AddOnsItem   `json:"add_ons"`
}

// GetInput is empty (no params needed).
type GetInput struct{}

// GetOutput wraps the license.
type GetOutput struct {
	toolutil.HintableOutput
	License Item `json:"license"`
}

// AddInput carries the Base64-encoded GitLab license payload to install.
type AddInput struct {
	License string `json:"license" jsonschema:"The license string (Base64-encoded),required"`
}

// AddOutput wraps the added license.
type AddOutput struct {
	toolutil.HintableOutput
	License Item `json:"license"`
}

// DeleteInput identifies the installed GitLab license to remove.
type DeleteInput struct {
	ID int64 `json:"id" jsonschema:"License ID to delete,required"`
}

// Helpers.

// errNoLicense is what an answer carrying no license means: the instance has
// none installed. GitLab answers the endpoint without one, so client-go leaves
// the pointer nil and no error beside it, and reading the fields off it would
// crash the handler. Rendering a zero license instead was worse than either:
// the card said "License #0", plan blank, expired false, which reads as a
// valid free-tier license that is not there.
var errNoLicense = errors.New("GitLab returned no license, so this instance has none installed")

// toItem converts a [gl.License] into the package's [Item] shape,
// flattening the embedded licensee and add-on structs.
func toItem(l *gl.License) Item {
	return Item{
		ID:               l.ID,
		Plan:             l.Plan,
		CreatedAt:        toolutil.RFC3339Ptr(l.CreatedAt),
		StartsAt:         toolutil.FormatISOTimePtr(l.StartsAt),
		ExpiresAt:        toolutil.FormatISOTimePtr(l.ExpiresAt),
		HistoricalMax:    l.HistoricalMax,
		MaximumUserCount: l.MaximumUserCount,
		Expired:          l.Expired,
		Overage:          l.Overage,
		UserLimit:        l.UserLimit,
		ActiveUsers:      l.ActiveUsers,
		Licensee: LicenseeItem{
			Name:    l.Licensee.Name,
			Company: l.Licensee.Company,
			Email:   l.Licensee.Email,
		},
		AddOns: AddOnsItem{
			GitLabAuditorUser: l.AddOns.GitLabAuditorUser,
			GitLabDeployBoard: l.AddOns.GitLabDeployBoard,
			GitLabFileLocks:   l.AddOns.GitLabFileLocks,
			GitLabGeo:         l.AddOns.GitLabGeo,
			GitLabServiceDesk: l.AddOns.GitLabServiceDesk,
		},
	}
}

// Handlers.

// Get retrieves the currently installed GitLab license via the GitLab
// License API (GET /license). Requires administrator access.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	lic, _, err := client.GL().License.GetLicense(gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("license_get", err, http.StatusForbidden, "license endpoints require administrator access")
	}
	if lic == nil {
		return GetOutput{}, toolutil.WrapErrWithHint("license_get", errNoLicense,
			"add one with the license_add action, or read the tier from the metadata_get action")
	}
	return GetOutput{License: toItem(lic)}, nil
}

// Add installs a new GitLab license via the GitLab License API
// (POST /license). The License field must be a base64-encoded
// license string. Requires administrator access.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (AddOutput, error) {
	opts := &gl.AddLicenseOptions{
		License: new(input.License),
	}
	lic, _, err := client.GL().License.AddLicense(opts, gl.WithContext(ctx))
	if err != nil {
		return AddOutput{}, toolutil.WrapErrWithStatusHint("license_add", err, http.StatusBadRequest, "verify the license key is valid. Requires administrator access")
	}
	if lic == nil {
		return AddOutput{}, toolutil.WrapErrWithHint("license_add", errNoLicense,
			"read the installed license back with the license_get action")
	}
	return AddOutput{License: toItem(lic)}, nil
}

// Delete removes a GitLab license by ID via the GitLab License API
// (DELETE /license/:id). Requires administrator access.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("license_delete", "id")
	}
	_, err := client.GL().License.DeleteLicense(input.ID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("license_delete", err, http.StatusNotFound, "verify license_id. Requires administrator access")
	}
	return nil
}

// Markdown formatters.
