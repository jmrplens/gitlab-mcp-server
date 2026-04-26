// Package epicissues implements GitLab epic-issue hierarchy operations using
// the Work Items GraphQL API. Child issues are managed through the hierarchy
// widget, supporting listing, assigning, removing, and reordering.
//
// This package was migrated from the deprecated Epics REST API (deprecated
// GitLab 17.0, removal planned 19.0) to the Work Items GraphQL API per
// ADR-0009 (progressive GraphQL migration).
package epicissues

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GraphQL queries and mutations for work item hierarchy operations.

const queryListChildren = `
query($fullPath: ID!, $iid: String!, $first: Int, $after: String) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
      widgets {
        ... on WorkItemWidgetHierarchy {
          children(first: $first, after: $after) {
            pageInfo {
              hasNextPage
              hasPreviousPage
              endCursor
              startCursor
            }
            nodes {
              id
              iid
              title
              state
              webUrl
              createdAt
              updatedAt
              author { username }
              widgets {
                ... on WorkItemWidgetLabels {
                  labels { nodes { title } }
                }
              }
            }
          }
        }
      }
    }
  }
}
`

const queryResolveWorkItemGID = `
query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
    }
  }
}
`

const mutationAddChild = `
mutation($id: WorkItemID!, $childrenIds: [WorkItemID!]!) {
  workItemUpdate(input: {
    id: $id
    hierarchyWidget: {
      childrenIds: $childrenIds
    }
  }) {
    workItem { id }
    errors
  }
}
`

const mutationRemoveParent = `
mutation($id: WorkItemID!) {
  workItemUpdate(input: {
    id: $id
    hierarchyWidget: {
      parentId: null
    }
  }) {
    workItem { id }
    errors
  }
}
`

const mutationReorderChild = `
mutation($id: WorkItemID!, $childrenIds: [WorkItemID!]!, $adjacentWorkItemId: WorkItemID!, $relativePosition: RelativePosition!) {
  workItemUpdate(input: {
    id: $id
    hierarchyWidget: {
      childrenIds: $childrenIds
      adjacentWorkItemId: $adjacentWorkItemId
      relativePosition: $relativePosition
    }
  }) {
    workItem {
      id
      widgets {
        ... on WorkItemWidgetHierarchy {
          children(first: 100) {
            nodes {
              id
              iid
              title
              state
              webUrl
              createdAt
              updatedAt
              author { username }
              widgets {
                ... on WorkItemWidgetLabels {
                  labels { nodes { title } }
                }
              }
            }
          }
        }
      }
    }
    errors
  }
}
`

// gqlChildNode represents a child work item from the GraphQL hierarchy widget.
type gqlChildNode struct {
	ID        string           `json:"id"`
	IID       string           `json:"iid"`
	Title     string           `json:"title"`
	State     string           `json:"state"`
	WebURL    string           `json:"webUrl"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
	Author    gqlAuthor        `json:"author"`
	Widgets   []gqlLabelWidget `json:"widgets"`
}

// gqlAuthor represents a user author in GraphQL responses.
type gqlAuthor struct {
	Username string `json:"username"`
}

// gqlLabelTitle represents a single label title.
type gqlLabelTitle struct {
	Title string `json:"title"`
}

// gqlLabelsConnection holds a list of label titles.
type gqlLabelsConnection struct {
	Nodes []gqlLabelTitle `json:"nodes"`
}

// gqlLabelWidget is a work item widget containing label data.
type gqlLabelWidget struct {
	Labels *gqlLabelsConnection `json:"labels"`
}

// gqlChildrenConnection holds a paginated list of child nodes.
type gqlChildrenConnection struct {
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
	Nodes    []gqlChildNode              `json:"nodes"`
}

// gqlChildrenWidget is a work item widget containing children data.
type gqlChildrenWidget struct {
	Children *gqlChildrenConnection `json:"children"`
}

// gqlListWorkItem represents a work item with children widgets for listing.
type gqlListWorkItem struct {
	ID      string              `json:"id"`
	Widgets []gqlChildrenWidget `json:"widgets"`
}

// gqlNamespaceWorkItem wraps a work item inside a namespace.
type gqlNamespaceWorkItem struct {
	WorkItem *gqlListWorkItem `json:"workItem"`
}

// gqlChildrenResponse is the response for the list children query.
type gqlChildrenResponse struct {
	Data struct {
		Namespace *gqlNamespaceWorkItem `json:"namespace"`
	} `json:"data"`
}

// gqlMutationChildrenNodes holds a non-paginated list of child nodes.
type gqlMutationChildrenNodes struct {
	Nodes []gqlChildNode `json:"nodes"`
}

// gqlMutationWidget is a work item widget for mutation responses.
type gqlMutationWidget struct {
	Children *gqlMutationChildrenNodes `json:"children"`
}

// gqlMutationWorkItem represents a work item in mutation responses.
type gqlMutationWorkItem struct {
	ID      string              `json:"id"`
	Widgets []gqlMutationWidget `json:"widgets"`
}

// gqlWorkItemUpdatePayload is the response payload for workItemUpdate mutations.
type gqlWorkItemUpdatePayload struct {
	WorkItem *gqlMutationWorkItem `json:"workItem"`
	Errors   []string             `json:"errors"`
}

// gqlMutationResponse is the response for workItemUpdate mutations.
type gqlMutationResponse struct {
	Data struct {
		WorkItemUpdate gqlWorkItemUpdatePayload `json:"workItemUpdate"`
	} `json:"data"`
}

// normalizeState maps GraphQL work item states (OPEN, CLOSED) to
// REST-compatible lowercase forms (opened, closed).
func normalizeState(state string) string {
	switch strings.ToUpper(state) {
	case "OPEN":
		return "opened"
	case "CLOSED":
		return "closed"
	default:
		return strings.ToLower(state)
	}
}

// nodeToChildOutput converts a GraphQL child node to the MCP output format.
func nodeToChildOutput(n gqlChildNode) ChildOutput {
	out := ChildOutput{
		ID:        n.ID,
		Title:     n.Title,
		State:     normalizeState(n.State),
		WebURL:    n.WebURL,
		Author:    n.Author.Username,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
	if iid, err := strconv.ParseInt(n.IID, 10, 64); err == nil {
		out.IID = iid
	}
	for _, w := range n.Widgets {
		if w.Labels != nil {
			for _, l := range w.Labels.Nodes {
				out.Labels = append(out.Labels, l.Title)
			}
		}
	}
	return out
}

// gqlWorkItemID holds a resolved work item GID.
type gqlWorkItemID struct {
	ID string `json:"id"`
}

// gqlNamespaceWorkItemID wraps a work item ID inside a namespace.
type gqlNamespaceWorkItemID struct {
	WorkItem *gqlWorkItemID `json:"workItem"`
}

// resolveWorkItemGID resolves the GraphQL GID for a work item by namespace path and IID.
func resolveWorkItemGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	var resp struct {
		Data struct {
			Namespace *gqlNamespaceWorkItemID `json:"namespace"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryResolveWorkItemGID,
		Variables: map[string]any{
			"fullPath": fullPath,
			"iid":      strconv.FormatInt(iid, 10),
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return "", err
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return "", fmt.Errorf("work item not found in %q with IID %d", fullPath, iid)
	}

	return resp.Data.Namespace.WorkItem.ID, nil
}

// ListInput defines parameters for listing child issues of an epic.
type ListInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group or my-group/sub-group),required"`
	IID      int64  `json:"iid"       jsonschema:"Epic IID within the group,required"`
	toolutil.GraphQLPaginationInput
}

// AssignInput defines parameters for assigning an issue to an epic.
type AssignInput struct {
	FullPath         string `json:"full_path"          jsonschema:"Full path of the group that contains the epic,required"`
	IID              int64  `json:"iid"                jsonschema:"Epic IID within the group,required"`
	ChildProjectPath string `json:"child_project_path" jsonschema:"Full project path of the issue to assign (e.g. my-group/my-project),required"`
	ChildIID         int64  `json:"child_iid"          jsonschema:"IID of the issue to assign to the epic,required"`
}

// RemoveInput defines parameters for removing an issue from an epic.
type RemoveInput struct {
	FullPath         string `json:"full_path"          jsonschema:"Full path of the group that contains the epic,required"`
	IID              int64  `json:"iid"                jsonschema:"Epic IID within the group,required"`
	ChildProjectPath string `json:"child_project_path" jsonschema:"Full project path of the issue to remove,required"`
	ChildIID         int64  `json:"child_iid"          jsonschema:"IID of the issue to remove from the epic,required"`
}

// UpdateInput defines parameters for reordering an issue within an epic.
type UpdateInput struct {
	FullPath         string `json:"full_path"                   jsonschema:"Full path of the group that contains the epic,required"`
	IID              int64  `json:"iid"                         jsonschema:"Epic IID within the group,required"`
	ChildID          string `json:"child_id"                    jsonschema:"Work item GID of the issue to reorder (from list output id field),required"`
	AdjacentID       string `json:"adjacent_id,omitempty"       jsonschema:"Work item GID of the reference issue to position relative to"`
	RelativePosition string `json:"relative_position,omitempty" jsonschema:"Position relative to adjacent item: BEFORE or AFTER"`
}

// ChildOutput represents a child work item (issue) within an epic.
type ChildOutput struct {
	ID        string   `json:"id"`
	IID       int64    `json:"iid"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	WebURL    string   `json:"web_url,omitempty"`
	Author    string   `json:"author,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// ListOutput holds a paginated list of child issues in an epic.
type ListOutput struct {
	toolutil.HintableOutput
	Issues     []ChildOutput                    `json:"issues"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// AssignOutput represents the result of assigning or removing an issue from an epic.
type AssignOutput struct {
	toolutil.HintableOutput
	EpicGID  string `json:"epic_gid"`
	ChildGID string `json:"child_gid"`
}

// List retrieves child issues of an epic via the Work Items GraphQL hierarchy widget.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicIssueList: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueList", "iid")
	}

	vars := input.GraphQLPaginationInput.Variables()
	vars["fullPath"] = input.FullPath
	vars["iid"] = strconv.FormatInt(input.IID, 10)

	var resp gqlChildrenResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListChildren,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicIssueList", err,
			"verify full_path (group path) with gitlab_group_get and iid with gitlab_epic_list; epics require GitLab Premium or Ultimate")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return ListOutput{}, fmt.Errorf("epicIssueList: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	var children []ChildOutput
	var pageInfo toolutil.GraphQLRawPageInfo
	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Children == nil {
			continue
		}
		pageInfo = w.Children.PageInfo
		for _, n := range w.Children.Nodes {
			children = append(children, nodeToChildOutput(n))
		}
	}

	return ListOutput{
		Issues:     children,
		Pagination: toolutil.PageInfoToOutput(pageInfo),
	}, nil
}

// Assign links an existing issue to an epic via the Work Items GraphQL hierarchy widget.
func Assign(ctx context.Context, client *gitlabclient.Client, input AssignInput) (AssignOutput, error) {
	if err := ctx.Err(); err != nil {
		return AssignOutput{}, err
	}
	if input.FullPath == "" {
		return AssignOutput{}, errors.New("epicIssueAssign: full_path is required")
	}
	if input.IID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueAssign", "iid")
	}
	if input.ChildProjectPath == "" {
		return AssignOutput{}, errors.New("epicIssueAssign: child_project_path is required")
	}
	if input.ChildIID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueAssign", "child_iid")
	}

	epicGID, err := resolveWorkItemGID(ctx, client, input.FullPath, input.IID)
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueAssign", err,
			"could not resolve epic GID; verify full_path with gitlab_group_get and iid with gitlab_epic_list")
	}

	childGID, err := resolveWorkItemGID(ctx, client, input.ChildProjectPath, input.ChildIID)
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueAssign", err,
			"could not resolve child issue GID; verify child_project_path with gitlab_project_get and child_iid with gitlab_issue_list")
	}

	var resp gqlMutationResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationAddChild,
		Variables: map[string]any{
			"id":          epicGID,
			"childrenIds": []string{childGID},
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueAssign", err,
			"the child issue may already be linked to another epic, or you lack Reporter role on the group; epics require GitLab Premium or Ultimate")
	}

	if len(resp.Data.WorkItemUpdate.Errors) > 0 {
		return AssignOutput{}, fmt.Errorf("epicIssueAssign: %s", resp.Data.WorkItemUpdate.Errors[0])
	}

	return AssignOutput{EpicGID: epicGID, ChildGID: childGID}, nil
}

// Remove unlinks an issue from an epic by clearing the child's parent reference.
func Remove(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (AssignOutput, error) {
	if err := ctx.Err(); err != nil {
		return AssignOutput{}, err
	}
	if input.FullPath == "" {
		return AssignOutput{}, errors.New("epicIssueRemove: full_path is required")
	}
	if input.IID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueRemove", "iid")
	}
	if input.ChildProjectPath == "" {
		return AssignOutput{}, errors.New("epicIssueRemove: child_project_path is required")
	}
	if input.ChildIID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueRemove", "child_iid")
	}

	epicGID, err := resolveWorkItemGID(ctx, client, input.FullPath, input.IID)
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueRemove", err,
			"could not resolve epic GID; verify full_path with gitlab_group_get and iid with gitlab_epic_list")
	}

	childGID, err := resolveWorkItemGID(ctx, client, input.ChildProjectPath, input.ChildIID)
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueRemove", err,
			"could not resolve child issue GID; verify child_project_path with gitlab_project_get and child_iid with gitlab_issue_list")
	}

	var resp gqlMutationResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationRemoveParent,
		Variables: map[string]any{
			"id": childGID,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueRemove", err,
			"the issue may not be linked to this epic; verify with gitlab_epic_issues_list; removing requires Reporter role")
	}

	if len(resp.Data.WorkItemUpdate.Errors) > 0 {
		return AssignOutput{}, fmt.Errorf("epicIssueRemove: %s", resp.Data.WorkItemUpdate.Errors[0])
	}

	return AssignOutput{EpicGID: epicGID, ChildGID: childGID}, nil
}

// UpdateOrder reorders an issue within an epic by moving it relative to another issue.
func UpdateOrder(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicIssueUpdate: full_path is required")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueUpdate", "iid")
	}
	if input.ChildID == "" {
		return ListOutput{}, errors.New("epicIssueUpdate: child_id is required")
	}
	if input.AdjacentID == "" {
		return ListOutput{}, errors.New("epicIssueUpdate: adjacent_id is required for reordering")
	}
	if input.RelativePosition == "" {
		return ListOutput{}, errors.New("epicIssueUpdate: relative_position is required (BEFORE or AFTER)")
	}

	pos := strings.ToUpper(input.RelativePosition)
	if pos != "BEFORE" && pos != "AFTER" {
		return ListOutput{}, fmt.Errorf("epicIssueUpdate: relative_position must be BEFORE or AFTER, got %q", input.RelativePosition)
	}

	epicGID, err := resolveWorkItemGID(ctx, client, input.FullPath, input.IID)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicIssueUpdate", err,
			"could not resolve epic GID; verify full_path with gitlab_group_get and iid with gitlab_epic_list")
	}

	var resp gqlMutationResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationReorderChild,
		Variables: map[string]any{
			"id":                 epicGID,
			"childrenIds":        []string{input.ChildID},
			"adjacentWorkItemId": input.AdjacentID,
			"relativePosition":   pos,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicIssueUpdate", err,
			"child_id and adjacent_id must both be GIDs of issues already linked to this epic; relative_position must be BEFORE or AFTER; reordering requires Reporter role")
	}

	if len(resp.Data.WorkItemUpdate.Errors) > 0 {
		return ListOutput{}, fmt.Errorf("epicIssueUpdate: %s", resp.Data.WorkItemUpdate.Errors[0])
	}

	var children []ChildOutput
	if resp.Data.WorkItemUpdate.WorkItem != nil {
		for _, w := range resp.Data.WorkItemUpdate.WorkItem.Widgets {
			if w.Children != nil {
				for _, n := range w.Children.Nodes {
					children = append(children, nodeToChildOutput(n))
				}
			}
		}
	}

	return ListOutput{Issues: children}, nil
}
