package epicissues

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicworkitems"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintEpicGIDResolution is the shared hint when epic GID resolution fails.
const hintEpicGIDResolution = "could not resolve epic GID; verify full_path with group.get and iid with group.epic_list"

// GraphQL queries and mutations for work item hierarchy operations.

const queryListChildren = `
query($fullPath: ID!, $iid: String!, $first: Int, $after: String, $last: Int, $before: String) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
      widgets {
        ... on WorkItemWidgetHierarchy {
          children(first: $first, after: $after, last: $last, before: $before) {
            pageInfo {
              hasNextPage
              hasPreviousPage
              endCursor
              startCursor
            }
            nodes {
              id
              iid
              reference(full: true)
              workItemType { name }
              title
              description
              state
              confidential
              hidden
              archived
              imported
              externalAuthor
              userDiscussionsCount
              webUrl
              createdAt
              updatedAt
              closedAt
              movedToWorkItemUrl
              duplicatedToWorkItemUrl
              promotedToEpicUrl
              author { username }
              widgets {
                ... on WorkItemWidgetLabels {
                  labels { nodes { id title color description descriptionHtml textColor } }
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

// mutationReorderChild moves one child among its siblings. The item updated
// is the child, with the epic as parentId: GitLab refuses relativePosition
// combined with childrenIds ("Relative position cannot be combined with
// childrenIds"), so the parent cannot be told which child moves. The mutation
// answers with the child alone; the epic's children are listed afterwards.
const mutationReorderChild = `
mutation($id: WorkItemID!, $parentId: WorkItemID!, $adjacentWorkItemId: WorkItemID!, $relativePosition: RelativePositionType!) {
  workItemUpdate(input: {
    id: $id
    hierarchyWidget: {
      parentId: $parentId
      adjacentWorkItemId: $adjacentWorkItemId
      relativePosition: $relativePosition
    }
  }) {
    workItem {
      id
    }
    errors
  }
}
`

// gqlChildNode represents a child work item from the GraphQL hierarchy widget.
type gqlChildNode struct {
	ID                      string           `json:"id"`
	IID                     string           `json:"iid"`
	Reference               string           `json:"reference"`
	WorkItemType            gqlWorkItemType  `json:"workItemType"`
	Title                   string           `json:"title"`
	Description             string           `json:"description"`
	State                   string           `json:"state"`
	Confidential            bool             `json:"confidential"`
	Hidden                  bool             `json:"hidden"`
	Archived                bool             `json:"archived"`
	Imported                bool             `json:"imported"`
	ExternalAuthor          string           `json:"externalAuthor"`
	UserDiscussionsCount    int64            `json:"userDiscussionsCount"`
	WebURL                  string           `json:"webUrl"`
	CreatedAt               string           `json:"createdAt"`
	UpdatedAt               string           `json:"updatedAt"`
	ClosedAt                string           `json:"closedAt"`
	MovedToWorkItemURL      string           `json:"movedToWorkItemUrl"`
	DuplicatedToWorkItemURL string           `json:"duplicatedToWorkItemUrl"`
	PromotedToEpicURL       string           `json:"promotedToEpicUrl"`
	Author                  gqlAuthor        `json:"author"`
	Widgets                 []gqlLabelWidget `json:"widgets"`
}

// gqlWorkItemType names the type of a child: an issue, a task, or a child
// epic, all of which the hierarchy widget lists.
type gqlWorkItemType struct {
	Name string `json:"name"`
}

// gqlAuthor represents a user author in GraphQL responses.
type gqlAuthor struct {
	Username string `json:"username"`
}

// gqlLabel represents a label on a child, with the fields the shared label
// details object publishes.
type gqlLabel struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Color           string `json:"color"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"descriptionHtml"`
	TextColor       string `json:"textColor"`
}

// gqlLabelsConnection holds the labels of a child.
type gqlLabelsConnection struct {
	Nodes []gqlLabel `json:"nodes"`
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
	Errors []toolutil.GraphQLError `json:"errors"`
}

// gqlMutationWorkItem represents a work item in mutation responses, which
// select its id alone: the three mutations here answer with the item they
// updated and nothing about its children, which are listed afterwards.
type gqlMutationWorkItem struct {
	ID string `json:"id"`
}

// gqlWorkItemUpdatePayload is the response payload for workItemUpdate mutations.
type gqlWorkItemUpdatePayload struct {
	WorkItem *gqlMutationWorkItem `json:"workItem"`
	Errors   []string             `json:"errors"`
}

// gqlMutationResponse is the response for workItemUpdate mutations. The
// payload is a pointer and the top-level errors are decoded beside it, so that
// a mutation GitLab refused is told apart from one that ran
// ([workItemUpdateError]).
type gqlMutationResponse struct {
	Data struct {
		WorkItemUpdate *gqlWorkItemUpdatePayload `json:"workItemUpdate"`
	} `json:"data"`
	Errors []toolutil.GraphQLError `json:"errors"`
}

// workItemUpdateError is the error a workItemUpdate mutation answered with,
// nil when it ran: GitLab's reason when it answered with no payload, an
// error naming the missing payload when it gave no reason either, and the
// first payload error otherwise.
//
// GitLab answers a mutation it refuses with HTTP 200, the field null and the
// reason as one top-level errors[] entry, which client-go does not turn into
// an error: a token without the role through authorized_find!, and a
// fine-grained token whose grant lacks Work Item: Update through
// authorize_granular_token (app/graphql/mutations/work_items/update.rb at
// v19.4.1-ee), whose sentence names the permission. The payload used to be
// read as a value, which decodes null as its zero, so a refused link, unlink
// or reorder was reported as done.
func workItemUpdateError(operation string, resp gqlMutationResponse) error {
	payload := resp.Data.WorkItemUpdate
	if payload == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError(operation, resp.Errors); graphQLErr != nil {
			return graphQLErr
		}
		return errors.New(operation + ": GitLab answered with no workItemUpdate payload")
	}
	if len(payload.Errors) > 0 {
		return fmt.Errorf("%s: %s", operation, payload.Errors[0])
	}
	return nil
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
		ID:                      n.ID,
		Reference:               n.Reference,
		Type:                    n.WorkItemType.Name,
		Title:                   n.Title,
		Description:             n.Description,
		State:                   normalizeState(n.State),
		Confidential:            n.Confidential,
		Hidden:                  n.Hidden,
		Archived:                n.Archived,
		Imported:                n.Imported,
		ExternalAuthor:          n.ExternalAuthor,
		UserDiscussionsCount:    n.UserDiscussionsCount,
		WebURL:                  n.WebURL,
		Author:                  n.Author.Username,
		CreatedAt:               n.CreatedAt,
		UpdatedAt:               n.UpdatedAt,
		ClosedAt:                n.ClosedAt,
		MovedToWorkItemURL:      n.MovedToWorkItemURL,
		DuplicatedToWorkItemURL: n.DuplicatedToWorkItemURL,
		PromotedToEpicURL:       n.PromotedToEpicURL,
	}
	if iid, err := strconv.ParseInt(n.IID, 10, 64); err == nil {
		out.IID = iid
	}
	for _, w := range n.Widgets {
		if w.Labels != nil {
			for _, l := range w.Labels.Nodes {
				out.Labels = append(out.Labels, l.Title)
				out.LabelDetails = append(out.LabelDetails, labelDetails(l))
			}
		}
	}
	return out
}

// labelDetails converts a label into the shared label details object, the
// numeric id read from the label's global ID.
func labelDetails(l gqlLabel) *toolutil.LabelDetailsOutput {
	details := &toolutil.LabelDetailsOutput{
		Name:            l.Title,
		Color:           l.Color,
		Description:     l.Description,
		DescriptionHTML: l.DescriptionHTML,
		TextColor:       l.TextColor,
	}
	if _, id, err := toolutil.ParseGID(l.ID); err == nil {
		details.ID = id
	}
	return details
}

// resolveWorkItemGID resolves the GraphQL GID for a work item by namespace path and IID.
func resolveWorkItemGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	return epicworkitems.ResolveWorkItemGID(ctx, client, fullPath, iid)
}

// ListInput defines parameters for listing child issues of an epic.
type ListInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group or my-group/sub-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	toolutil.GraphQLCursorPaginationInput
}

// AssignInput defines parameters for assigning an issue to an epic.
type AssignInput struct {
	FullPath         string `json:"full_path"          jsonschema:"Full path of the group that contains the epic,required"`
	IID              int64  `json:"epic_iid"                jsonschema:"Epic IID within the group,required"`
	ChildProjectPath string `json:"child_project_path" jsonschema:"Full project path of the issue to assign (e.g. my-group/my-project),required"`
	ChildIID         int64  `json:"child_iid"          jsonschema:"IID of the issue to assign to the epic,required"`
}

// RemoveInput defines parameters for removing an issue from an epic.
type RemoveInput struct {
	FullPath         string `json:"full_path"          jsonschema:"Full path of the group that contains the epic,required"`
	IID              int64  `json:"epic_iid"                jsonschema:"Epic IID within the group,required"`
	ChildProjectPath string `json:"child_project_path" jsonschema:"Full project path of the issue to remove,required"`
	ChildIID         int64  `json:"child_iid"          jsonschema:"IID of the issue to remove from the epic,required"`
}

// UpdateInput defines parameters for reordering an issue within an epic.
type UpdateInput struct {
	FullPath         string `json:"full_path"                   jsonschema:"Full path of the group that contains the epic,required"`
	IID              int64  `json:"epic_iid"                         jsonschema:"Epic IID within the group,required"`
	ChildID          string `json:"child_id"                    jsonschema:"Work item GID of the issue to reorder (from list output id field),required"`
	AdjacentID       string `json:"adjacent_id,omitempty"       jsonschema:"Work item GID of the reference issue to position relative to"`
	RelativePosition string `json:"relative_position,omitempty" jsonschema:"Position relative to adjacent item: BEFORE or AFTER"`
}

// ChildOutput represents a child work item (issue) within an epic.
//
// reference is the child's full reference (group/project#iid for an issue),
// which names the project child_project_path takes without the caller parsing
// web_url.
// type is the work item type's name, since the hierarchy lists tasks and
// child epics beside issues. labels keeps the names every issue output
// carries and label_details the rest of each label, as the epic output does.
// The three URL keys say where the item went when it was moved, closed as a
// duplicate or promoted to an epic, and are absent otherwise.
type ChildOutput struct {
	ID                      string                         `json:"id"`
	IID                     int64                          `json:"iid"`
	Reference               string                         `json:"reference,omitempty"`
	Type                    string                         `json:"type,omitempty"`
	Title                   string                         `json:"title"`
	Description             string                         `json:"description,omitempty"`
	State                   string                         `json:"state"`
	Confidential            bool                           `json:"confidential,omitempty"`
	Hidden                  bool                           `json:"hidden,omitempty"`
	Archived                bool                           `json:"archived,omitempty"`
	Imported                bool                           `json:"imported,omitempty"`
	ExternalAuthor          string                         `json:"external_author,omitempty"`
	UserDiscussionsCount    int64                          `json:"user_discussions_count"`
	WebURL                  string                         `json:"web_url,omitempty"`
	Author                  string                         `json:"author,omitempty"`
	Labels                  []string                       `json:"labels,omitempty"`
	LabelDetails            []*toolutil.LabelDetailsOutput `json:"label_details,omitempty"`
	CreatedAt               string                         `json:"created_at,omitempty"`
	UpdatedAt               string                         `json:"updated_at,omitempty"`
	ClosedAt                string                         `json:"closed_at,omitempty"`
	MovedToWorkItemURL      string                         `json:"moved_to_work_item_url,omitempty"`
	DuplicatedToWorkItemURL string                         `json:"duplicated_to_work_item_url,omitempty"`
	PromotedToEpicURL       string                         `json:"promoted_to_epic_url,omitempty"`
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
		return ListOutput{}, errors.New("epicIssueList: full_path is required. Use group.list to find the group path first")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueList", "epic_iid")
	}

	vars, err := input.Variables(queryListChildren)
	if err != nil {
		return ListOutput{}, fmt.Errorf("epicIssueList: %w", err)
	}
	vars["fullPath"] = input.FullPath
	vars["iid"] = strconv.FormatInt(input.IID, 10)

	var resp gqlChildrenResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListChildren,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicIssueList", err,
			"verify full_path (group path) with group.get and iid with group.epic_list; epics require GitLab Premium or Ultimate")
	}

	// GitLab answers a rejected document with HTTP 200 and a top-level errors
	// array, which client-go does not turn into an error, so a query the
	// instance refused would otherwise be reported as a missing epic.
	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError("epicIssueList", resp.Errors); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
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
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueAssign", "epic_iid")
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
			hintEpicGIDResolution)
	}

	childGID, err := resolveWorkItemGID(ctx, client, input.ChildProjectPath, input.ChildIID)
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueAssign", err,
			"could not resolve child issue GID; verify child_project_path with project.get and child_iid with issue.list")
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

	if refused := workItemUpdateError("epicIssueAssign", resp); refused != nil {
		return AssignOutput{}, refused
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
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueRemove", "epic_iid")
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
			hintEpicGIDResolution)
	}

	childGID, err := resolveWorkItemGID(ctx, client, input.ChildProjectPath, input.ChildIID)
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithHint("epicIssueRemove", err,
			"could not resolve child issue GID; verify child_project_path with project.get and child_iid with issue.list")
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
			"the issue may not be linked to this epic; verify with group.epic_issue_list; removing requires Reporter role")
	}

	if refused := workItemUpdateError("epicIssueRemove", resp); refused != nil {
		return AssignOutput{}, refused
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
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueUpdate", "epic_iid")
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
			hintEpicGIDResolution)
	}

	// The child is the item updated and the epic is its parentId; see the
	// comment on mutationReorderChild for why the parent cannot be the target.
	var resp gqlMutationResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationReorderChild,
		Variables: map[string]any{
			"id":                 input.ChildID,
			"parentId":           epicGID,
			"adjacentWorkItemId": input.AdjacentID,
			"relativePosition":   pos,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicIssueUpdate", err,
			"child_id and adjacent_id must both be GIDs of issues already linked to this epic; relative_position must be BEFORE or AFTER; reordering requires Reporter role")
	}

	if refused := workItemUpdateError("epicIssueUpdate", resp); refused != nil {
		return ListOutput{}, refused
	}

	// The mutation answers with the child that moved, not with the epic's
	// children, so the order the caller asked about is read back from the epic.
	return List(ctx, client, ListInput{FullPath: input.FullPath, IID: input.IID})
}
