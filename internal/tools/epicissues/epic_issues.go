// Package epicissues implements GitLab epic-issue association operations
// including listing issues in an epic, assigning/removing issues, and
// reordering issue positions within an epic.
package epicissues

import (
	"context"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing issues assigned to an epic.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	toolutil.PaginationInput
}

// ListOutput holds a paginated list of issues in an epic.
type ListOutput struct {
	toolutil.HintableOutput
	Issues     []issues.Output           `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// AssignInput defines parameters for assigning an issue to an epic.
type AssignInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	IssueID int64                `json:"issue_id" jsonschema:"Global issue ID to assign to the epic,required"`
}

// RemoveInput defines parameters for removing an issue from an epic.
type RemoveInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID     int64                `json:"epic_iid"       jsonschema:"Epic internal ID within the group,required"`
	EpicIssueID int64                `json:"epic_issue_id"  jsonschema:"Epic-issue association ID to remove,required"`
}

// UpdateInput defines parameters for reordering an issue within an epic.
type UpdateInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"                  jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID      int64                `json:"epic_iid"                  jsonschema:"Epic internal ID within the group,required"`
	EpicIssueID  int64                `json:"epic_issue_id"             jsonschema:"Epic-issue association ID to reorder,required"`
	MoveBeforeID *int64               `json:"move_before_id,omitempty"  jsonschema:"ID of the epic-issue to move this issue before"`
	MoveAfterID  *int64               `json:"move_after_id,omitempty"   jsonschema:"ID of the epic-issue to move this issue after"`
}

// AssignOutput represents the result of assigning an issue to an epic.
type AssignOutput struct {
	toolutil.HintableOutput
	ID      int64 `json:"id"`
	EpicIID int64 `json:"epic_iid,omitempty"`
	IssueID int64 `json:"issue_id,omitempty"`
}

// toAssignOutput converts a GitLab EpicIssueAssignment to output format.
func toAssignOutput(a *gl.EpicIssueAssignment) AssignOutput {
	out := AssignOutput{ID: a.ID}
	if a.Epic != nil {
		out.EpicIID = a.Epic.IID
	}
	if a.Issue != nil {
		out.IssueID = a.Issue.ID
	}
	return out
}

// List retrieves issues assigned to an epic.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("epicIssueList: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.EpicIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueList", "epic_iid")
	}
	opts := &gl.ListOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	issueList, resp, err := client.GL().EpicIssues.ListEpicIssues(string(input.GroupID), input.EpicIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("epicIssueList", err)
	}
	out := make([]issues.Output, len(issueList))
	for i, issue := range issueList {
		out[i] = issues.ToOutput(issue)
	}
	return ListOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Assign links an existing issue to an epic.
func Assign(ctx context.Context, client *gitlabclient.Client, input AssignInput) (AssignOutput, error) {
	if err := ctx.Err(); err != nil {
		return AssignOutput{}, err
	}
	if input.GroupID == "" {
		return AssignOutput{}, errors.New("epicIssueAssign: group_id is required")
	}
	if input.EpicIID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueAssign", "epic_iid")
	}
	if input.IssueID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueAssign", "issue_id")
	}
	a, _, err := client.GL().EpicIssues.AssignEpicIssue(string(input.GroupID), input.EpicIID, input.IssueID, gl.WithContext(ctx))
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithMessage("epicIssueAssign", err)
	}
	return toAssignOutput(a), nil
}

// Remove unlinks an issue from an epic.
func Remove(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (AssignOutput, error) {
	if err := ctx.Err(); err != nil {
		return AssignOutput{}, err
	}
	if input.GroupID == "" {
		return AssignOutput{}, errors.New("epicIssueRemove: group_id is required")
	}
	if input.EpicIID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueRemove", "epic_iid")
	}
	if input.EpicIssueID <= 0 {
		return AssignOutput{}, toolutil.ErrRequiredInt64("epicIssueRemove", "epic_issue_id")
	}
	a, _, err := client.GL().EpicIssues.RemoveEpicIssue(string(input.GroupID), input.EpicIID, input.EpicIssueID, gl.WithContext(ctx))
	if err != nil {
		return AssignOutput{}, toolutil.WrapErrWithMessage("epicIssueRemove", err)
	}
	return toAssignOutput(a), nil
}

// UpdateOrder reorders an issue within an epic by moving it before or after another issue.
func UpdateOrder(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("epicIssueUpdate: group_id is required")
	}
	if input.EpicIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueUpdate", "epic_iid")
	}
	if input.EpicIssueID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicIssueUpdate", "epic_issue_id")
	}
	opts := &gl.UpdateEpicIssueAssignmentOptions{}
	if input.MoveBeforeID != nil {
		opts.MoveBeforeID = input.MoveBeforeID
	}
	if input.MoveAfterID != nil {
		opts.MoveAfterID = input.MoveAfterID
	}
	issueList, _, err := client.GL().EpicIssues.UpdateEpicIssueAssignment(string(input.GroupID), input.EpicIID, input.EpicIssueID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("epicIssueUpdate", err)
	}
	out := make([]issues.Output, len(issueList))
	for i, issue := range issueList {
		out[i] = issues.ToOutput(issue)
	}
	return ListOutput{Issues: out}, nil
}
