// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package todos

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput defines parameters for listing to-do items.
type ListInput struct {
	toolutil.PaginationInput
	Action    string `json:"action,omitempty"     jsonschema:"Filter by action, for example assigned, mentioned, review_requested, build_failed or transfer_failed (a project or group transfer GitLab failed in the background)"`
	AuthorID  int64  `json:"author_id,omitempty"  jsonschema:"Filter by author user ID"`
	ProjectID int64  `json:"project_id,omitempty" jsonschema:"Filter by project ID"`
	GroupID   int64  `json:"group_id,omitempty"   jsonschema:"Filter by group ID"`
	State     string `json:"state,omitempty"      jsonschema:"Filter by state: pending or done (default: pending)"`
	Type      string `json:"type,omitempty"       jsonschema:"Filter by target type, for example Issue, MergeRequest, Commit, Epic, Project or Namespace (a group)"`
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
	UpdatedAt  string            `json:"updated_at,omitempty"`
	// Group is present on a to-do raised in a group rather than a project.
	Group *toolutil.NamespaceBasicOutput `json:"group,omitempty"`
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

// toOutput converts a GitLab API [gl.Todo] to MCP output format, the
// target's weight taken from what the capture read at the to-do's position.
func toOutput(t *gl.Todo, extra todoExtra) Output {
	out := Output{
		ID:         t.ID,
		Project:    basicProjectOut(t.Project),
		Author:     basicUserOut(t.Author),
		ActionName: t.ActionName,
		TargetType: t.TargetType,
		Target:     todoTargetOut(t.Target, extra.Target.Weight),
		TargetURL:  t.TargetURL,
		Body:       t.Body,
		State:      t.State,
		UpdatedAt:  toolutil.FormatTimePtr(t.UpdatedAt),
		Group:      namespaceBasicOut(t.Group),
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}

// todoExtra is what a to-do's target carries that client-go's TodoTarget
// reads in a way the output cannot use, read off the captured answer
// (ADR-0021). The weight is modeled as an int64, which reads GitLab's null,
// an issue without a weight, as the weight 0, a weight of its own (issue
// 1390), for the reason of register row 116 ("A measure GitLab sends as null
// decodes as 0").
type todoExtra struct {
	Target struct {
		Weight *int64 `json:"weight"`
	} `json:"target"`
}

// readCapturedTodos is the reader [List] calls, a seam because no answer the
// SDK decodes can fail it: the one key it reads is one the SDK reads too, so
// a body that fails one fails both, and the refusal is reached only by a
// test that replaces it.
var readCapturedTodos = capturedTodos

// capturedTodos reads [todoExtra] off the answer to the to-do list, one per
// to-do in the list's order. The count is held to what the SDK decoded: the
// two read the same bytes, so a difference is a fault in this reader rather
// than in the answer.
func capturedTodos(capture *gitlabclient.ResponseCapture, decoded int) ([]todoExtra, error) {
	var extras []todoExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d to-dos and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}

// namespaceBasicOut converts the group a to-do raised in a group carries, the
// NamespaceBasic entity client-go decodes into a [gl.ProjectNamespace], and
// returns nil for a to-do raised in a project, which carries none.
func namespaceBasicOut(g *gl.ProjectNamespace) *toolutil.NamespaceBasicOutput {
	if g == nil {
		return nil
	}
	return &toolutil.NamespaceBasicOutput{
		ID:        g.ID,
		Name:      g.Name,
		Path:      g.Path,
		Kind:      g.Kind,
		FullPath:  g.FullPath,
		ParentID:  g.ParentID,
		AvatarURL: g.AvatarURL,
		WebURL:    g.WebURL,
	}
}

// List retrieves to-do items for the authenticated user with optional filters.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := &gl.ListTodosOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, toolutil.KeysetPaginationInput{})
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	todos, resp, err := client.GL().Todos.ListTodos(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("todoList", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	extras, err := readCapturedTodos(captured, len(todos))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("todoList", err)
	}

	out := make([]Output, len(todos))
	for i, t := range todos {
		out[i] = toOutput(t, extras[i])
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
		return MarkDoneOutput{}, errors.New("todoMarkDone: id is required. Use user.todo_list to find to-do item IDs")
	}

	_, err := client.GL().Todos.MarkTodoAsDone(input.ID, gl.WithContext(ctx))
	if err != nil {
		return MarkDoneOutput{}, toolutil.WrapErrWithStatusHint("todoMarkDone", err, http.StatusNotFound, "verify todo_id with user.todo_list")
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
