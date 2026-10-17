package waitpoll

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TransferFailure is the to-do item GitLab left the caller when a transfer
// failed in the background.
type TransferFailure struct {
	// TodoID is the item's id, which [TransferFailureReason] reads the reason
	// of.
	TodoID int64
	// Destination is the path the item names, where the transfer was to put
	// the object.
	Destination string
}

// TransferFailed reports whether GitLab has recorded that a transfer of
// target failed in the background, answering the to-do item that records it.
// Since GitLab 19.4 that item is the only report of such a failure: the
// transfer's own answer came before the worker ran, and the object is left
// where it was. From 19.5 the item also carries the reason, which the REST
// to-do entity leaves out and [TransferFailureReason] reads over GraphQL.
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
func TransferFailed(ctx context.Context, client *gitlabclient.Client, target TransferTarget, answeredAt time.Time) (failure TransferFailure, failed bool, err error) {
	if answeredAt.IsZero() {
		return TransferFailure{}, false, nil
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
		return TransferFailure{}, false, err
	}
	earliest := answeredAt.Add(-2 * time.Second)
	for _, todo := range todos {
		if todo.CreatedAt != nil && !todo.CreatedAt.Before(earliest) && todoTargets(todo, target.ID) {
			return TransferFailure{TodoID: todo.ID, Destination: todo.Body}, true, nil
		}
	}
	return TransferFailure{}, false, nil
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

// transferFailureReasonQuery reads the reason GitLab keeps on a
// transfer_failed to-do item. The field is Todo.transferFailureReason, which
// GitLab 19.5 added (gitlab-org/gitlab!251614) and the REST to-do entity does
// not carry.
const transferFailureReasonQuery = `query transferFailureReason($id: TodoID!) {
  todo(id: $id) {
    transferFailureReason
  }
}`

// The release that added Todo.transferFailureReason. An older one refuses a
// document naming it as a field it does not have.
const (
	transferReasonMajor = 19
	transferReasonMinor = 5
)

// transferReasonMaxRunes bounds the reason quoted to a caller. GitLab cuts the
// reason to 500 characters when it stores it (TRANSFER_ERROR_MAX_LENGTH in
// Namespaces::Stateful::TransitionCallbacks), so every reason GitLab writes is
// quoted whole, and only a longer one, which no GitLab sends, is cut.
const transferReasonMaxRunes = 500

// TransferFailureReason returns the reason GitLab gives for the failed
// transfer the to-do item todoID records, cleaned to one line of at most 500
// characters, or "" when there is none to give.
//
// GitLab records the reason from 19.5, on the namespace the transfer moved,
// and serves it only over GraphQL. An instance known to run an older release
// is not asked, since it refuses the document. One whose release is not known
// is asked anyway: its version endpoint did not answer, it refused the token
// Metadata: Read (a fine-grained token without it), or it named something
// that is not a version, and a refusal from it costs nothing but the reason.
//
// The read is the last word on a transfer the caller is already being told
// failed, so every way it can fail gives "" rather than an error that would
// take the failure's place: GitLab failing the request, a release that does
// not know the field, a fine-grained token refused the item, and an item
// GitLab gives no reason for.
//
// The worker creates the item and stores the reason a moment later, in two
// writes (Projects::TransferWorker and Namespaces::Groups::TransferWorker),
// so a read between them finds the item without the reason. An item answered
// with no reason is therefore read once more after [transferReasonRetry], and
// no more: a reason still missing then is one GitLab did not store, or one it
// cleared because the transfer was scheduled again, which a retry of the
// worker does. A caller that goes away during the pause gets no reason and no
// second read.
func TransferFailureReason(ctx context.Context, client *gitlabclient.Client, todoID int64) string {
	if !client.AtLeast(ctx, transferReasonMajor, transferReasonMinor) && client.Version() != "" {
		return ""
	}
	//gitlab:request optional: the reason is read only once a transfer is found failed, and the failure is reported without it when the read gives none
	reason, item, err := queryTransferFailureReason(ctx, client, todoID)
	if err != nil || !item || reason != "" {
		return reason
	}
	pause := time.NewTimer(transferReasonRetry)
	defer pause.Stop()
	select {
	case <-ctx.Done():
		return ""
	case <-pause.C:
	}
	//gitlab:request optional: the item is read once more only when GitLab answered it without the reason it stores a moment after creating it
	reason, _, _ = queryTransferFailureReason(ctx, client, todoID)
	return reason
}

// transferReasonRetry is how long [TransferFailureReason] waits before it
// reads an item that held no reason once more, a second. A variable so a test
// waits a millisecond.
var transferReasonRetry time.Duration = 1e9

// queryTransferFailureReason sends [transferFailureReasonQuery] for the item
// todoID with the caller's context and returns the reason cleaned, whether
// GitLab answered the item, and the error of a request GitLab did not answer.
// The reason is "" when GitLab answers no item, or the item with no reason.
func queryTransferFailureReason(ctx context.Context, client *gitlabclient.Client, todoID int64) (reason string, item bool, err error) {
	var resp struct {
		Data struct {
			Todo *struct {
				TransferFailureReason string `json:"transferFailureReason"`
			} `json:"todo"`
		} `json:"data"`
	}
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     transferFailureReasonQuery,
		Variables: map[string]any{"id": toolutil.FormatGID("Todo", todoID)},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return "", false, err
	}
	if resp.Data.Todo == nil {
		return "", false, nil
	}
	return cleanTransferReason(resp.Data.Todo.TransferFailureReason), true, nil
}

// cleanTransferReason returns GitLab's reason as one line a caller is quoted:
// every run of white space and control characters is one space, the ends are
// trimmed, and the text is cut at [transferReasonMaxRunes] characters. The
// reason is the instance's text, which under --allow-any-gitlab-url is an
// instance a caller named, and it is quoted inside an error a model reads.
func cleanTransferReason(reason string) string {
	runes := []rune(strings.Join(strings.FieldsFunc(reason, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " "))
	// The cut can end on the space between two words, which is trimmed; text
	// that is not cut ends on a word already.
	return strings.TrimRight(string(runes[:min(len(runes), transferReasonMaxRunes)]), " ")
}
