// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package projects

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/waitpoll"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// transferBound and transferInterval are waitpoll's transfer timing, held in
// variables so a test waits milliseconds rather than the whole bound.
var (
	transferBound    = waitpoll.TransferBound
	transferInterval = waitpoll.TransferInterval
)

// The hints a refused or failed transfer is answered with. GitLab refuses a
// transfer it cannot start, one whose project is already in place and a name
// collision all with 400, and each needs a different next step, so the hint
// is chosen by what GitLab said rather than by the status alone.
const (
	hintTransferPermission      = "transferring a project requires the Owner role on the project and permission to create projects in the target namespace"
	hintTransferNamespace       = "verify the target namespace exists. Use group.list or user.get, and pass the namespace as a numeric ID or a full path"
	hintTransferNotFound        = "verify project_id with project.get, or find the project with project.list"
	hintTransferUnderWay        = "GitLab is applying a transfer of this project right now (since GitLab 19.4 a transfer runs in the background), or the project is marked for deletion, which GitLab refuses a transfer with in the same words. Wait, then read the project back with project.get to see which namespace it is in and whether it is marked for deletion, before transferring it again"
	hintTransferInPlace         = "the project is already in the target namespace, either from the start or because an earlier transfer has moved it. Read it back with project.get to confirm its path: there is nothing left to transfer"
	hintTransferCollision       = "a project with this name or path already exists in the target namespace. Rename the project with project.update (both name and path), or remove the other project, then transfer again"
	hintTransferPendingDeletion = "a project with this name or path was deleted in the target namespace and is still pending deletion. Wait for the deletion to complete or delete that project permanently (project.delete with permanently_remove and full_path set to its path_with_namespace), or rename this project with project.update (both name and path), then transfer again"
	hintTransferRefused         = "GitLab refused the transfer for the reason quoted above. Use project.get to confirm where the project is now"
	hintTransferFailed          = "GitLab keeps the reason of a failed background transfer only from 19.5, on this to-do item, and none could be read here. The checks its worker runs are a project in the target namespace still pending deletion with the same name or path, container registry images, and npm packages scoped to the old top-level namespace when the move changes it. It also refuses a name or path the target namespace already holds, which GitLab 19.5 and later refuse with the request itself unless the other project appeared after it. Look for a project with this name or path in the target namespace with project.list, rename the project with project.update (both name and path) or remove the other one, then transfer again. The item stays pending in user.todo_list until a transfer of this project succeeds"
	hintTransferFailedReason    = "read the project back with project.get first. If GitLab's reason says the transfer failed unexpectedly, GitLab retries it by itself a few more times over the next minutes, so wait and read the project again rather than sending the transfer a second time. Otherwise fix what the reason names, then transfer again. A name or path the target namespace already holds, or holds for a project still pending deletion, is freed by renaming this project with project.update (both name and path) or by removing the other one, which project.list finds. The item stays pending in user.todo_list until a transfer of this project succeeds"
)

// TransferInput defines parameters for transferring a project to another namespace.
type TransferInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Namespace string               `json:"namespace" jsonschema:"Target namespace ID or path,required"`
}

// TransferOutput is the project a transfer answers with. GitLab 19.4 and
// later move a project in the background, so the handler reads it back until
// it sits in the namespace the transfer named. TransferQueued says the wait
// ended first, with no failure reported either, and the project is then
// described where it still was.
//
// The flag is this server's own and GitLab sends nothing like it: no entity
// exposes the transfer state of a namespace, so whether a move has landed can
// only be told by comparing where the project is with where it was sent.
type TransferOutput struct {
	Output
	TransferQueued bool `json:"transfer_queued,omitempty" jsonschema:"True when GitLab accepted the transfer but had neither applied it nor reported it failed when the wait ended. The project fields then show where it still is. Read it back with project.get before relying on its new path"`
}

// transferRead is one read of a project a transfer is moving: where the
// project is, and whether GitLab has reported the move failed, with the to-do
// item its report is.
type transferRead struct {
	project Output
	failed  bool
	failure waitpoll.TransferFailure
}

// Transfer moves a project to a different namespace and waits for the move.
//
// GitLab 19.4 and later accept the transfer, answer with the project where
// it still is and move it in a background worker, so an answer that does not
// yet show the destination is read back until it does, for up to
// [waitpoll.TransferBound]. An older GitLab answers after the move, and its
// answer is returned as it is, with no read at all; so does one from 19.0 to
// 19.3 unless the groups_and_projects_async_transfer flag is on, and the
// handler tells the two apart by the answer rather than by the version.
//
// Some checks run only in that worker (a project pending deletion with the
// same name or path, container registry images, npm packages scoped to the
// old top-level namespace, and on 19.4 the name and path collision, which
// 19.5 refuses with the request too), and a failure is reported only as a
// to-do item for the caller, so each read that does not find the project
// moved also looks for that item, and one dated after the transfer ends the
// wait with an error. From 19.5 the item carries GitLab's reason, which the
// error quotes ([transferFailedError]).
// A move that has neither landed nor been reported failed when the wait ends
// is answered with the project from the transfer's own answer and
// TransferQueued set, never as an error: GitLab accepted it, and a model told
// the transfer failed would send it again, which GitLab runs a second time
// while the first is queued and refuses while it runs.
func Transfer(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input TransferInput) (TransferOutput, error) {
	if err := ctx.Err(); err != nil {
		return TransferOutput{}, err
	}
	if input.ProjectID == "" {
		return TransferOutput{}, errors.New("projectTransfer: project_id is required. Use project.list to find the ID, then pass it as project_id")
	}
	if input.Namespace == "" {
		return TransferOutput{}, errors.New("projectTransfer: namespace is required. Provide the target namespace ID or path (e.g. 'my-group' or '42')")
	}
	opts := &gl.TransferProjectOptions{
		Namespace: input.Namespace,
	}
	transferCtx, captured := gitlabclient.WithResponseCapture(ctx)
	p, resp, err := client.GL().Projects.TransferProject(string(input.ProjectID), opts, gl.WithContext(transferCtx))
	if err != nil {
		return TransferOutput{}, transferError(err, captured)
	}
	answered, err := projectOutput("projectTransfer", p, captured)
	if err != nil {
		return TransferOutput{}, err
	}
	if inNamespace(answered, input.Namespace) {
		return TransferOutput{Output: answered}, nil
	}

	answeredAt := waitpoll.AnsweredAt(resp)
	target := waitpoll.TransferTarget{ID: answered.ID}
	settled, done, err := waitpoll.Until(ctx, waitpoll.UntilOptions[transferRead]{
		Request:  req,
		Interval: transferInterval,
		Bound:    transferBound,
		Message:  i18n.T(ctx, i18n.ProgressProjectTransferWait, i18n.S("namespace", input.Namespace)),
		Read: func(readCtx context.Context) (transferRead, error) {
			current, readErr := Get(readCtx, client, GetInput{ProjectID: toolutil.StringOrInt(strconv.FormatInt(answered.ID, 10))})
			if readErr != nil || inNamespace(current, input.Namespace) {
				return transferRead{project: current}, readErr
			}
			failure, failed, readErr := waitpoll.TransferFailed(readCtx, client, target, answeredAt)
			return transferRead{project: current, failed: failed, failure: failure}, readErr
		},
		Landed: func(current transferRead) bool {
			return current.failed || inNamespace(current.project, input.Namespace)
		},
	})
	if err != nil {
		return TransferOutput{}, err
	}
	switch {
	case !done:
		return TransferOutput{Output: answered, TransferQueued: true}, nil
	case settled.failed:
		return TransferOutput{}, transferFailedError(ctx, client, settled.failure)
	default:
		return TransferOutput{Output: settled.project}, nil
	}
}

// transferFailedError is the error a transfer GitLab failed in the background
// is answered with: the destination the to-do item names and, from GitLab
// 19.5, the reason the item carries, quoted ahead of the hint so a caller
// reads what GitLab said before this server's advice. Without a reason the
// hint lists the checks the worker runs.
func transferFailedError(ctx context.Context, client *gitlabclient.Client, failure waitpoll.TransferFailure) error {
	reason := waitpoll.TransferFailureReason(ctx, client, failure.TodoID)
	if reason == "" {
		return fmt.Errorf("projectTransfer: GitLab accepted the transfer and then failed it in the background, leaving a to-do item that says the transfer to %s failed. Suggestion: %s", failure.Destination, hintTransferFailed)
	}
	return fmt.Errorf("projectTransfer: GitLab accepted the transfer and then failed it in the background, leaving a to-do item that says the transfer to %s failed, for this reason: %q. Suggestion: %s", failure.Destination, reason, hintTransferFailedReason)
}

// inNamespace reports whether a project sits in the namespace a transfer
// named, reading the name the way GitLab's find_namespace does: a number is
// an id, and anything else is a full path, which GitLab looks up without
// regard to case. A project whose answer carries no namespace sits nowhere
// this can confirm.
func inNamespace(p Output, namespace string) bool {
	if p.Namespace == nil {
		return false
	}
	if id, err := strconv.ParseInt(namespace, 10, 64); err == nil {
		return p.Namespace.ID == id
	}
	return strings.EqualFold(p.Namespace.FullPath, namespace)
}

// transferError wraps a refused transfer with the hint its refusal calls for.
// The messages are GitLab's own (app/services/projects/transfer_service.rb
// and the route in lib/api/projects.rb): a transfer that cannot start means
// one is running or the project is marked for deletion, and a project already
// in the namespace needs nothing more, which reading the project back settles
// in both cases, while a name or path collision is fixed by renaming. A name
// or path a project pending deletion still holds is a refusal of its own,
// answered with the request only by an instance that transfers inline (from
// 19.4 its worker reports it), and is fixed by waiting for that deletion as
// well; its words contain the collision's, so it is matched first. The route
// looks the project up before the namespace, so a 404 names which of the two
// it could not find, and [notFoundHint] reads which from the captured answer.
func transferError(err error, captured *gitlabclient.ResponseCapture) error {
	const op = "projectTransfer"
	switch {
	case toolutil.IsPermissionRefusal(err):
		return toolutil.WrapErrWithHint(op, err, hintTransferPermission)
	case toolutil.IsHTTPStatus(err, http.StatusNotFound):
		return toolutil.WrapErrWithHint(op, err, notFoundHint(captured))
	case !toolutil.IsHTTPStatus(err, http.StatusBadRequest):
		return toolutil.WrapErrWithMessage(op, err)
	case toolutil.ContainsAny(err, "transfer in progress"):
		return toolutil.WrapErrWithHint(op, err, hintTransferUnderWay)
	case toolutil.ContainsAny(err, "already in this namespace"):
		return toolutil.WrapErrWithHint(op, err, hintTransferInPlace)
	case toolutil.ContainsAny(err, "recently deleted"):
		return toolutil.WrapErrWithHint(op, err, hintTransferPendingDeletion)
	case toolutil.ContainsAny(err, "same name or path"):
		return toolutil.WrapErrWithHint(op, err, hintTransferCollision)
	case toolutil.ContainsAny(err, "don't have permission"):
		return toolutil.WrapErrWithHint(op, err, hintTransferPermission)
	default:
		return toolutil.WrapErrWithHint(op, err, hintTransferRefused)
	}
}

// notFoundHint is the hint for a transfer's 404, chosen by which lookup of
// the route failed. client-go answers every 404 with one sentinel that drops
// GitLab's message, so the message is read from the captured body: "404
// Project Not Found" from the route's project lookup, and "404 Namespace Not
// Found" from its namespace lookup. A body that does not decode names
// neither, and is read as the namespace, the lookup a caller most often gets
// wrong.
func notFoundHint(captured *gitlabclient.ResponseCapture) string {
	var body struct {
		Message string `json:"message"`
	}
	if err := captured.Decode(&body); err != nil {
		return hintTransferNamespace
	}
	if strings.Contains(body.Message, "Project Not Found") {
		return hintTransferNotFound
	}
	return hintTransferNamespace
}
