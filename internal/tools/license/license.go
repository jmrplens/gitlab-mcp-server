// Package license implements MCP tools for GitLab License API.
package license

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

// AddInput represents the input for adding a license.
type AddInput struct {
	License string `json:"license" jsonschema:"The license string (Base64-encoded),required"`
}

// AddOutput wraps the added license.
type AddOutput struct {
	toolutil.HintableOutput
	License Item `json:"license"`
}

// DeleteInput represents the input for deleting a license.
type DeleteInput struct {
	ID int64 `json:"id" jsonschema:"License ID to delete,required"`
}

// Helpers.

// formatTime renders the result as a formatted string.
func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatISOTime renders the result as a formatted string.
func formatISOTime(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return t.String()
}

// toItem converts the GitLab API response to the tool output format.
func toItem(l *gl.License) Item {
	return Item{
		ID:               l.ID,
		Plan:             l.Plan,
		CreatedAt:        formatTime(l.CreatedAt),
		StartsAt:         formatISOTime(l.StartsAt),
		ExpiresAt:        formatISOTime(l.ExpiresAt),
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

// Get retrieves the current license information.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	lic, _, err := client.GL().License.GetLicense(gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("license_get", err)
	}
	return GetOutput{License: toItem(lic)}, nil
}

// Add adds a new license.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (AddOutput, error) {
	opts := &gl.AddLicenseOptions{
		License: new(input.License),
	}
	lic, _, err := client.GL().License.AddLicense(opts, gl.WithContext(ctx))
	if err != nil {
		return AddOutput{}, toolutil.WrapErrWithMessage("license_add", err)
	}
	return AddOutput{License: toItem(lic)}, nil
}

// Delete removes a license by ID.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ID <= 0 {
		return toolutil.ErrRequiredInt64("license_delete", "id")
	}
	_, err := client.GL().License.DeleteLicense(input.ID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("license_delete", err)
	}
	return nil
}

// Markdown formatters.
