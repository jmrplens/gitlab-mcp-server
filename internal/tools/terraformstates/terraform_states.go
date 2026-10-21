package terraformstates

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// List.

// ListInput contains parameters for listing Terraform states.
//
// The list offers forward paging only, first and after, as branch.rule_list
// does, and selects only the forward half of pageInfo. Project.terraformStates
// takes last and before as well; the list does not offer them, so it never
// advertises a backward page it does not select the page info for.
type ListInput struct {
	toolutil.GraphQLPaginationInput
	ProjectPath string `json:"project_path" jsonschema:"Full project path (e.g. group/project),required"`
}

// StateItem represents a Terraform state.
//
// The four times are what client-go's document already selects and decodes,
// and each is written the way the pinned schema types it on TerraformState:
// createdAt and updatedAt are Time!, so they are always written, while
// lockedAt and deletedAt are nullable, null for a state that is unlocked and
// not scheduled for deletion, and client-go decodes a null into the zero time,
// so they are left out then rather than written as the year one. The latest
// version's serial and download path are nullable too, and its two times,
// which client-go's documents select and decode as well, are Time! on the
// version and written whenever there is one: the schema lets latestVersion
// itself be null, for a state nothing has written yet, and client-go decodes
// that into a version whose times are zero, so they are left out then. The
// package's tests hold each field to the schema.
type StateItem struct {
	toolutil.HintableOutput
	Name                   string `json:"name"`
	LatestSerial           uint64 `json:"latest_serial,omitempty"`
	DownloadPath           string `json:"download_path,omitempty"`
	LatestVersionCreatedAt string `json:"latest_version_created_at,omitempty"`
	LatestVersionUpdatedAt string `json:"latest_version_updated_at,omitempty"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
	LockedAt               string `json:"locked_at,omitempty"`
	DeletedAt              string `json:"deleted_at,omitempty"`
}

// toStateItem converts a state as client-go decodes it, which the list and
// the get both answer with.
func toStateItem(s gl.TerraformState) StateItem {
	return StateItem{
		Name:                   s.Name,
		LatestSerial:           s.LatestVersion.Serial,
		DownloadPath:           s.LatestVersion.DownloadPath,
		LatestVersionCreatedAt: toolutil.RFC3339(s.LatestVersion.CreatedAt),
		LatestVersionUpdatedAt: toolutil.RFC3339(s.LatestVersion.UpdatedAt),
		CreatedAt:              toolutil.RFC3339(s.CreatedAt),
		UpdatedAt:              toolutil.RFC3339(s.UpdatedAt),
		LockedAt:               toolutil.RFC3339(s.LockedAt),
		DeletedAt:              toolutil.RFC3339(s.DeletedAt),
	}
}

// ListOutput contains one page of a project's Terraform states, how many
// states the project holds and where the next page starts.
type ListOutput struct {
	toolutil.HintableOutput
	States     []StateItem                             `json:"states"`
	Count      int                                     `json:"count"`
	Pagination toolutil.GraphQLForwardPaginationOutput `json:"pagination"`
}

// listOperation labels every error the list reports.
const listOperation = "gitlab_list_terraform_states"

// listNotFoundHint is what a caller can check when GitLab finds no project or
// shows the caller none of its states.
const listNotFoundHint = "verify project_path with project.get; uses GraphQL. Terraform states require Maintainer role to view"

// listStatesQuery is the list's own document rather than client-go's, which
// asks for the connection with no first, no after, no count and no pageInfo:
// GitLab caps a connection at 100 nodes, so a project holding more states
// read as holding 100 and no more, with nothing to ask for the rest by
// (upstream-bugs row 104 records it: the paged List client-go owes goes out
// in a small merge request now that gitlab-org/api/client-go!3063, which it
// was to join, is closed unmerged). The
// nodes select exactly what client-go's list and get select, so a state reads
// the same through the list as through the get, which is still client-go's.
const listStatesQuery = `
query($fullPath: ID!, $first: Int, $after: String) {
  project(fullPath: $fullPath) {
    terraformStates(first: $first, after: $after) {
      count
      nodes {
        name
        createdAt
        deletedAt
        latestVersion {
          createdAt
          updatedAt
          downloadPath
          serial
        }
        updatedAt
        lockedAt
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}`

// listStatesResponse is the answer to [listStatesQuery]. The project is a
// pointer because GitLab answers null for a project it does not find or does
// not show the caller, which is client-go's not-found too.
type listStatesResponse struct {
	Data struct {
		Project *statesProject `json:"project"`
	} `json:"data"`
	Errors []toolutil.GraphQLError `json:"errors"`
}

// statesProject is the project [listStatesQuery] reads the states of.
type statesProject struct {
	TerraformStates statesPage `json:"terraformStates"`
}

// statesPage is one page of a project's states as [listStatesQuery] selects
// it, with how many states the project has in all.
type statesPage struct {
	Count    int                                `json:"count"`
	Nodes    []stateNode                        `json:"nodes"`
	PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
}

// stateNode is one state as [listStatesQuery] selects it. It mirrors client-go's
// TerraformState field for field, and decodes a null the way it does, into the
// zero value, so [stateNode.state] hands [toStateItem] what the get hands it.
// It is declared here rather than borrowed from client-go so the document and
// the struct that decodes it are this package's, and what the document leaves
// out is answered under this package.
type stateNode struct {
	Name          string           `json:"name"`
	LatestVersion stateVersionNode `json:"latestVersion"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
	DeletedAt     time.Time        `json:"deletedAt"`
	LockedAt      time.Time        `json:"lockedAt"`
}

// stateVersionNode is a state's latest version as [listStatesQuery] selects it.
type stateVersionNode struct {
	Serial       uint64    `json:"serial"`
	DownloadPath string    `json:"downloadPath"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// state is the node as client-go's TerraformState.
func (n stateNode) state() gl.TerraformState {
	return gl.TerraformState{
		Name:          n.Name,
		LatestVersion: gl.TerraformStateVersion(n.LatestVersion),
		CreatedAt:     n.CreatedAt,
		UpdatedAt:     n.UpdatedAt,
		DeletedAt:     n.DeletedAt,
		LockedAt:      n.LockedAt,
	}
}

// List retrieves one page of a project's Terraform states.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	return listPage(ctx, client, listStatesQuery, input)
}

// listPage sends document, which is [listStatesQuery] for every caller but the
// test that hands it a document declaring no page variables, since that is
// the one way the page variables can be refused.
func listPage(ctx context.Context, client *gitlabclient.Client, document string, input ListInput) (ListOutput, error) {
	pageVars, err := input.Variables(document)
	if err != nil {
		return ListOutput{}, toolutil.WrapErr(listOperation, err)
	}
	var resp listStatesResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     document,
		Variables: toolutil.MergeVariables(pageVars, map[string]any{"fullPath": input.ProjectPath}),
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(listOperation, err, http.StatusNotFound, listNotFoundHint)
	}
	return listOutput(resp)
}

// listOutput reads the answer to [listStatesQuery]. GitLab answers a document
// it refused with HTTP 200 and a top-level errors array, which client-go does
// not turn into an error, and a project it does not find or does not show the
// caller with a null project, which is reported the way a 404 is, hint
// included, since that is what client-go's list reported it as.
func listOutput(resp listStatesResponse) (ListOutput, error) {
	if err := toolutil.GraphQLTopLevelError(listOperation, resp.Errors); err != nil {
		return ListOutput{}, err
	}
	project := resp.Data.Project
	if project == nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(listOperation, gl.ErrNotFound, http.StatusNotFound, listNotFoundHint)
	}
	items := make([]StateItem, 0, len(project.TerraformStates.Nodes))
	for _, node := range project.TerraformStates.Nodes {
		items = append(items, toStateItem(node.state()))
	}
	return ListOutput{
		States:     items,
		Count:      project.TerraformStates.Count,
		Pagination: toolutil.ForwardPageInfoToOutput(project.TerraformStates.PageInfo),
	}, nil
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
			"verify state name with admin.terraform_state_list; the state may not exist for this project")
	}
	return toStateItem(*s), nil
}

// Delete.

// DeleteInput contains parameters for deleting a Terraform state.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Terraform state name,required"`
}

// deleteOperation labels every error the delete reports.
const deleteOperation = "gitlab_delete_terraform_state"

// Delete removes a Terraform state, after reading it to refuse a locked one.
//
// GitLab answers the REST delete of a locked state with 200 and deletes
// nothing (upstream-bugs row 118), so sending it reported a delete that never
// happened. Only the GraphQL state carries the lock, and its project field
// takes a full path, so a numeric project_id is looked up first.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	path, err := stateProjectPath(ctx, client, input.ProjectID)
	if err != nil {
		return toolutil.WrapErrWithStatusHint(deleteOperation, err, http.StatusNotFound,
			"verify project_id with project.get")
	}
	//gitlab:request mandatory: the state is read before the delete on every call, since only its GraphQL fields say whether it is locked, and GitLab answers the delete of a locked state as done without doing it
	state, _, err := client.GL().TerraformStates.Get(path, input.Name, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint(deleteOperation, err, http.StatusNotFound,
			"verify the state name with admin.terraform_state_list; the state may not exist for this project")
	}
	if !state.LockedAt.IsZero() {
		locked := fmt.Errorf("state %q has been locked since %s, and GitLab answers the delete of a locked state as done without deleting it",
			input.Name, toolutil.RFC3339(state.LockedAt))
		return toolutil.WrapErrWithHint(deleteOperation, locked, "release the lock with admin.terraform_state_unlock, then delete the state")
	}
	//gitlab:request mandatory: the delete itself, sent once the read found the state unlocked
	_, err = client.GL().TerraformStates.Delete(string(input.ProjectID), input.Name, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint(deleteOperation, err, http.StatusForbidden,
			"deleting Terraform states requires Maintainer role; deletion is irreversible. All versions are removed")
	}
	return nil
}

// stateProjectPath is the full path GitLab's GraphQL project field needs for
// project_id: the path itself, URL-encoded or not, or for a numeric ID the
// path GitLab answers the project with.
func stateProjectPath(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt) (string, error) {
	if _, err := projectID.Int64(); err != nil {
		path, unescapeErr := url.PathUnescape(string(projectID))
		if unescapeErr != nil {
			return "", fmt.Errorf("project_id %q is neither an ID nor a path: %w", projectID, unescapeErr)
		}
		return path, nil
	}
	//gitlab:request mandatory: a numeric project_id is looked up on every call that names one, since the GraphQL project field takes no ID
	project, _, err := client.GL().Projects.GetProject(string(projectID), nil, gl.WithContext(ctx))
	if err != nil {
		return "", err
	}
	return project.PathWithNamespace, nil
}

// DeleteVersion.

// DeleteVersionInput contains parameters for deleting a Terraform state version.
type DeleteVersionInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Terraform state name,required"`
	Serial    uint64               `json:"serial" jsonschema:"State version serial number,required"`
}

// DeleteVersion removes a specific version of a Terraform state.
//
// A 404 means the project has no state by that name or the state no version
// with that serial, and the hint says that and nothing more. It used to add
// that the latest version cannot be deleted, which GitLab does not enforce:
// 19.4.2 answers a delete of the latest version 204 and deletes it, whether or
// not the state is locked.
func DeleteVersion(ctx context.Context, client *gitlabclient.Client, input DeleteVersionInput) error {
	_, err := client.GL().TerraformStates.DeleteVersion(string(input.ProjectID), input.Name, input.Serial, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_delete_terraform_state_version", err, http.StatusNotFound,
			"the project has no state by that name, or the state no version with that serial; verify the state name with admin.terraform_state_list, which shows each state's latest serial")
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
		// client-go sends no Terraform lock-info body, so GitLab always
		// rejects this call with 400; point to the working alternatives.
		return LockOutput{}, toolutil.WrapErrWithStatusHint("gitlab_lock_terraform_state", err, http.StatusBadRequest,
			"the Go client sends no Terraform lock-info body, so GitLab always rejects this call with 400. Run the terraform CLI against the GitLab HTTP backend for real locking; admin.terraform_state_unlock still works to clear stale locks")
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
