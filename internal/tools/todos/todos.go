// Package todos implements MCP tool handlers for GitLab to-do item operations
// including list, mark as done, and mark all as done.
// It wraps the Todos service from client-go v2.
package todos

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing to-do items.
type ListInput struct {
	toolutil.PaginationInput
	Action    string `json:"action,omitempty"     jsonschema:"Filter by action: assigned, mentioned, build_failed, marked, approval_required, directly_addressed"`
	AuthorID  int64  `json:"author_id,omitempty"  jsonschema:"Filter by author user ID"`
	ProjectID int64  `json:"project_id,omitempty" jsonschema:"Filter by project ID"`
	GroupID   int64  `json:"group_id,omitempty"   jsonschema:"Filter by group ID"`
	State     string `json:"state,omitempty"      jsonschema:"Filter by state: pending or done (default: pending)"`
	Type      string `json:"type,omitempty"       jsonschema:"Filter by target type: Issue, MergeRequest, DesignManagement::Design, AlertManagement::Alert"`
}

// MarkDoneInput defines parameters for marking a single to-do item as done.
type MarkDoneInput struct {
	ID int64 `json:"id" jsonschema:"ID of the to-do item to mark as done,required"`
}

// MarkAllDoneInput defines parameters for marking all to-do items as done.
type MarkAllDoneInput struct{}

// Output represents a single to-do item.
type Output struct {
	ID          int64  `json:"id"`
	ActionName  string `json:"action_name"`
	TargetType  string `json:"target_type"`
	TargetTitle string `json:"target_title"`
	TargetURL   string `json:"target_url"`
	Body        string `json:"body,omitempty"`
	State       string `json:"state"`
	ProjectName string `json:"project_name,omitempty"`
	AuthorName  string `json:"author_name,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
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
		ActionName: string(t.ActionName),
		TargetType: string(t.TargetType),
		TargetURL:  t.TargetURL,
		Body:       t.Body,
		State:      t.State,
	}
	if t.Target != nil {
		out.TargetTitle = t.Target.Title
	}
	if t.Project != nil {
		out.ProjectName = t.Project.Name
	}
	if t.Author != nil {
		out.AuthorName = t.Author.Username
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
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
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
