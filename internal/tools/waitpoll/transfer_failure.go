package waitpoll

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// transferFailedAction is the to-do action GitLab gives the item it leaves a
// user whose transfer failed in the background (TodoService#transfer_failed),
// for which client-go declares no constant.
const transferFailedAction gl.TodoAction = "transfer_failed"

// TransferTarget names the object a transfer moves, as GitLab's to-do list
// is filtered by it: a project by its project_id with target type Project,
// and a group by its group_id with target type Namespace, the class GitLab
// records a group target under.
type TransferTarget struct {
	// ID is the project's or the group's numeric id.
	ID int64
	// Group says the object is a group rather than a project.
	Group bool
}

// AnsweredAt is when GitLab answered a transfer, read from the Date header of
// its response, so it is GitLab's clock and not this server's that a failure
// is dated against. It is zero when the response carries no date it can read.
func AnsweredAt(resp *gl.Response) time.Time {
	at, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return time.Time{}
	}
	return at
}

// TransferFailed reports whether GitLab has recorded that a transfer of
// target failed in the background, answering the destination the to-do item
// names. Since GitLab 19.4 that item is the only report of such a failure: the
// transfer's own answer came before the worker ran, and the object is left
// where it was.
//
// An item counts only when it was created no earlier than two seconds before
// answeredAt, because GitLab keeps one pending item per object and a user,
// and an item left by an earlier failure says nothing about this transfer.
// The two seconds are the Date header's whole-second precision and a worker
// that can fail before the web node writes that header. It follows that a
// failure is not seen while an earlier failure's item is still pending, since
// GitLab adds no second one, and that nothing is seen at all when answeredAt
// is zero: the item could not be told apart from an older one, and no
// request is made. Only the first page of 100 pending items is read.
func TransferFailed(ctx context.Context, client *gitlabclient.Client, target TransferTarget, answeredAt time.Time) (destination string, failed bool, err error) {
	if answeredAt.IsZero() {
		return "", false, nil
	}
	opts := &gl.ListTodosOptions{
		PerPage:   100,
		Action:    new(transferFailedAction),
		State:     new("pending"),
		Type:      new("Project"),
		ProjectID: new(target.ID),
	}
	if target.Group {
		opts.Type, opts.ProjectID, opts.GroupID = new("Namespace"), nil, new(target.ID)
	}
	todos, _, err := client.GL().Todos.ListTodos(opts, gl.WithContext(ctx))
	if err != nil {
		return "", false, err
	}
	earliest := answeredAt.Add(-2 * time.Second)
	for _, todo := range todos {
		if todo.CreatedAt != nil && !todo.CreatedAt.Before(earliest) && todoTargets(todo, target.ID) {
			return todo.Body, true, nil
		}
	}
	return "", false, nil
}

// todoTargets reports whether a to-do item is about the object with this id.
// A group's listing also carries the items of its descendants, so the filter
// alone does not settle it. The target's id arrives as a JSON number, which
// client-go decodes into an interface and so into a float64.
func todoTargets(todo *gl.Todo, id int64) bool {
	if todo.Target == nil {
		return false
	}
	targetID, ok := todo.Target.ID.(float64)
	return ok && targetID == float64(id)
}
