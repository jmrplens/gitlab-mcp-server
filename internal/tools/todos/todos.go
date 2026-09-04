package todos

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListInput defines parameters for listing to-do items.
type ListInput struct {
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
	Action    string `json:"action,omitempty"     jsonschema:"Filter by action: assigned, mentioned, build_failed, marked, approval_required, unmergeable, directly_addressed, merge_train_removed, member_access_requested"`
	AuthorID  int64  `json:"author_id,omitempty"  jsonschema:"Filter by author user ID"`
	ProjectID int64  `json:"project_id,omitempty" jsonschema:"Filter by project ID"`
	GroupID   int64  `json:"group_id,omitempty"   jsonschema:"Filter by group ID"`
	State     string `json:"state,omitempty"      jsonschema:"Filter by state: pending or done (default: pending)"`
	Type      string `json:"type,omitempty"       jsonschema:"Filter by target type: Issue, MergeRequest, Commit, Epic, DesignManagement::Design, AlertManagement::Alert, Project, Namespace, Vulnerability, WikiPage::Meta"`
	OrderBy   string `json:"order_by,omitempty"   jsonschema:"Order results by field (e.g. id, created_at). Combine with sort."`
	Sort      string `json:"sort,omitempty"       jsonschema:"Sort direction: asc or desc."`
}

// MarkDoneInput defines parameters for marking a single to-do item as done.
type MarkDoneInput struct {
	ID int64 `json:"id" jsonschema:"ID of the to-do item to mark as done,required"`
}

// MarkAllDoneInput defines parameters for marking all to-do items as done.
type MarkAllDoneInput struct{}

// Output represents a single to-do item. It mirrors gl.Todo: action_name and
// target_type carry the SDK enum types, and the project/author/target
// sub-objects are surfaced in full per the 1:1 audit policy (no flattened
// scalar duplicates).
type Output struct {
	ID         int64             `json:"id"`
	Project    *BasicProjectOut  `json:"project,omitempty"`
	Author     *BasicUserOut     `json:"author,omitempty"`
	ActionName gl.TodoAction     `json:"action_name"`
	TargetType gl.TodoTargetType `json:"target_type"`
	Target     *TodoTargetOut    `json:"target,omitempty"`
	TargetURL  string            `json:"target_url"`
	Body       string            `json:"body,omitempty"`
	State      string            `json:"state"`
	CreatedAt  string            `json:"created_at,omitempty"`
}

// ListOutput holds a paginated list of to-do items.
type ListOutput struct {
	toolutil.HintableOutput
	Todos      []Output                  `json:"todos"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// MarkDoneOutput represents the result of marking a to-do as done.
type MarkDoneOutput struct {
	toolutil.HintableOutput
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// MarkAllDoneOutput represents the result of marking all to-dos as done.
type MarkAllDoneOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// toOutput converts a GitLab API [gl.Todo] to MCP output format.
func toOutput(t *gl.Todo) Output {
	out := Output{
		ID:         t.ID,
		Project:    basicProjectOut(t.Project),
		Author:     basicUserOut(t.Author),
		ActionName: t.ActionName,
		TargetType: t.TargetType,
		Target:     todoTargetOut(t.Target),
		TargetURL:  t.TargetURL,
		Body:       t.Body,
		State:      t.State,
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}

// List retrieves to-do items for the authenticated user with optional filters.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := &gl.ListTodosOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Action != "" {
		action := gl.TodoAction(input.Action)
		opts.Action = &action
	}
	if input.AuthorID != 0 {
		opts.AuthorID = new(input.AuthorID)
	}
	if input.ProjectID != 0 {
		opts.ProjectID = new(input.ProjectID)
	}
	if input.GroupID != 0 {
		opts.GroupID = new(input.GroupID)
	}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Type != "" {
		opts.Type = new(input.Type)
	}

	todos, resp, err := client.GL().Todos.ListTodos(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("todoList", err, http.StatusForbidden, "verify your token has read_api scope")
	}

	out := make([]Output, len(todos))
	for i, t := range todos {
		out[i] = toOutput(t)
	}
	return ListOutput{
		Todos:      out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// MarkDone marks a single pending to-do item as done.
func MarkDone(ctx context.Context, client *gitlabclient.Client, input MarkDoneInput) (MarkDoneOutput, error) {
	if err := ctx.Err(); err != nil {
		return MarkDoneOutput{}, err
	}
	if input.ID == 0 {
		return MarkDoneOutput{}, errors.New("todoMarkDone: id is required. Use gitlab_todo_list to find to-do item IDs")
	}

	_, err := client.GL().Todos.MarkTodoAsDone(input.ID, gl.WithContext(ctx))
	if err != nil {
		return MarkDoneOutput{}, toolutil.WrapErrWithStatusHint("todoMarkDone", err, http.StatusNotFound, "verify todo_id with gitlab_todo_list")
	}
	return MarkDoneOutput{
		ID:      input.ID,
		Message: fmt.Sprintf("To-do %d marked as done", input.ID),
	}, nil
}

// MarkAllDone marks all pending to-do items as done for the current user.
func MarkAllDone(ctx context.Context, client *gitlabclient.Client, _ MarkAllDoneInput) (MarkAllDoneOutput, error) {
	if err := ctx.Err(); err != nil {
		return MarkAllDoneOutput{}, err
	}

	_, err := client.GL().Todos.MarkAllTodosAsDone(gl.WithContext(ctx))
	if err != nil {
		return MarkAllDoneOutput{}, toolutil.WrapErrWithStatusHint("todoMarkAllDone", err, http.StatusForbidden, "verify your token has api scope")
	}
	return MarkAllDoneOutput{
		Message: "All pending to-do items marked as done",
	}, nil
}

// Markdown formatting.
