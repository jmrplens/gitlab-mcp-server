// Package terraformstates implements MCP tools for GitLab Terraform state management.
//
// The package registers MCP tools and renders Markdown summaries for Terraform
// state and version responses.
package terraformstates

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// List.

// ListInput contains parameters for listing Terraform states.
type ListInput struct {
	ProjectPath string `json:"project_path" jsonschema:"Full project path (e.g. group/project),required"`
}

// StateItem represents a Terraform state.
type StateItem struct {
	toolutil.HintableOutput
	Name         string `json:"name"`
	LatestSerial uint64 `json:"latest_serial,omitempty"`
	DownloadPath string `json:"download_path,omitempty"`
}

// ListOutput contains a list of Terraform states.
type ListOutput struct {
	toolutil.HintableOutput
	States []StateItem `json:"states"`
}

// List retrieves all Terraform states for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	states, _, err := client.GL().TerraformStates.List(input.ProjectPath, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_terraform_states", err, http.StatusNotFound,
			"verify project_path with gitlab_project_get; uses GraphQL \u2014 Terraform states require Maintainer role to view")
	}
	items := make([]StateItem, 0, len(states))
	for _, s := range states {
		items = append(items, StateItem{
			Name:         s.Name,
			LatestSerial: s.LatestVersion.Serial,
			DownloadPath: s.LatestVersion.DownloadPath,
		})
	}
	return ListOutput{States: items}, nil
}

// Get.

// GetInput contains parameters for getting a single Terraform state.
type GetInput struct {
	ProjectPath string `json:"project_path" jsonschema:"Full project path (e.g. group/project),required"`
	Name        string `json:"name" jsonschema:"Terraform state name,required"`
}

// Get retrieves a specific Terraform state.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (StateItem, error) {
	s, _, err := client.GL().TerraformStates.Get(input.ProjectPath, input.Name, gl.WithContext(ctx))
	if err != nil {
		return StateItem{}, toolutil.WrapErrWithStatusHint("gitlab_get_terraform_state", err, http.StatusNotFound,
			"verify state name with gitlab_list_terraform_states; the state may not exist for this project")
	}
	return StateItem{
		Name:         s.Name,
		LatestSerial: s.LatestVersion.Serial,
		DownloadPath: s.LatestVersion.DownloadPath,
	}, nil
}

// Delete.

// DeleteInput contains parameters for deleting a Terraform state.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Terraform state name,required"`
}

// Delete removes a Terraform state.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	_, err := client.GL().TerraformStates.Delete(string(input.ProjectID), input.Name, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_delete_terraform_state", err, http.StatusForbidden,
			"deleting Terraform states requires Maintainer role; deletion is irreversible \u2014 all versions are removed")
	}
	return nil
}

// DeleteVersion.

// DeleteVersionInput contains parameters for deleting a Terraform state version.
type DeleteVersionInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Terraform state name,required"`
	Serial    uint64               `json:"serial" jsonschema:"State version serial number,required"`
}

// DeleteVersion removes a specific version of a Terraform state.
func DeleteVersion(ctx context.Context, client *gitlabclient.Client, input DeleteVersionInput) error {
	_, err := client.GL().TerraformStates.DeleteVersion(string(input.ProjectID), input.Name, input.Serial, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_delete_terraform_state_version", err, http.StatusNotFound,
			"verify serial with gitlab_list_terraform_states; cannot delete the latest version \u2014 use gitlab_delete_terraform_state to remove the entire state")
	}
	return nil
}

// Lock.

// LockInput contains parameters for locking a Terraform state.
type LockInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Terraform state name,required"`
}

// LockOutput represents the result of a lock operation.
type LockOutput struct {
	toolutil.HintableOutput
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Lock locks a Terraform state.
func Lock(ctx context.Context, client *gitlabclient.Client, input LockInput) (LockOutput, error) {
	_, err := client.GL().TerraformStates.Lock(string(input.ProjectID), input.Name, gl.WithContext(ctx))
	if err != nil {
		return LockOutput{}, toolutil.WrapErrWithMessage("gitlab_lock_terraform_state", err)
	}
	return LockOutput{Success: true, Message: fmt.Sprintf("State '%s' locked", input.Name)}, nil
}

// Unlock.

// Unlock unlocks a Terraform state.
func Unlock(ctx context.Context, client *gitlabclient.Client, input LockInput) (LockOutput, error) {
	_, err := client.GL().TerraformStates.Unlock(string(input.ProjectID), input.Name, gl.WithContext(ctx))
	if err != nil {
		return LockOutput{}, toolutil.WrapErrWithMessage("gitlab_unlock_terraform_state", err)
	}
	return LockOutput{Success: true, Message: fmt.Sprintf("State '%s' unlocked", input.Name)}, nil
}

// formatters.
