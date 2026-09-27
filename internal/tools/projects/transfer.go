package projects

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/waitpoll"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// transferBound and transferInterval are waitpoll's transfer timing, held in
// variables so a test waits milliseconds rather than the whole bound.
var (
	transferBound    = waitpoll.TransferBound
	transferInterval = waitpoll.TransferInterval
)

// The hints a refused transfer is answered with. GitLab refuses a transfer
// it cannot start, one already under way and a name collision all with 400,
// and each needs a different next step, so the hint is chosen by what GitLab
// said rather than by the status alone.
const (
	hintTransferPermission = "transferring a project requires the Owner role on the project and permission to create projects in the target namespace"
	hintTransferNamespace  = "verify the target namespace exists. Use group.list or user.get, and pass the namespace as a numeric ID or a full path"
	hintTransferUnderWay   = "GitLab is already moving this project, or has moved it: since GitLab 19.4 a transfer is applied in the background. Wait, then read the project back with project.get to see which namespace it is in before transferring it again"
	hintTransferCollision  = "a project with this name or path already exists in the target namespace, or was deleted there recently and is still pending deletion. Rename the project with project.update (both name and path), or remove the other project, then transfer again"
	hintTransferRefused    = "GitLab refused the transfer for the reason quoted above. Use project.get to confirm where the project is now"
)

// TransferInput defines parameters for transferring a project to another namespace.
type TransferInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Namespace string               `json:"namespace" jsonschema:"Target namespace ID or path,required"`
}

// TransferOutput is the project a transfer answers with. GitLab 19.4 and
// later move a project in the background, so the handler reads it back until
// it sits in the namespace the transfer named. TransferQueued says the wait
// ended first, and the project is then described where it still was.
//
// The flag is this server's own and GitLab sends nothing like it: no entity
// exposes the transfer state of a namespace, so whether a move has landed can
// only be told by comparing where the project is with where it was sent.
type TransferOutput struct {
	Output
	TransferQueued bool `json:"transfer_queued,omitempty" jsonschema:"True when GitLab accepted the transfer but had not applied it when the wait ended. The project fields then show where it still is. Read it back with project.get before relying on its new path"`
}

// Transfer moves a project to a different namespace and waits for the move.
//
// GitLab 19.4 and later accept the transfer, answer with the project where
// it still is and move it in a background worker, so an answer that does not
// yet show the destination is read back until it does, for up to
// [waitpoll.TransferBound]. An older GitLab answers after the move, and its
// answer is returned as it is, with no read at all. A move that has not landed
// when the wait ends is answered with the project from the transfer's own
// answer and TransferQueued set, never as an error: GitLab accepted it, and a
// model told the transfer failed would send it again, which GitLab refuses
// while the first is under way.
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
	p, _, err := client.GL().Projects.TransferProject(string(input.ProjectID), opts, gl.WithContext(transferCtx))
	if err != nil {
		return TransferOutput{}, transferError(err)
	}
	answered, err := projectOutput("projectTransfer", p, captured)
	if err != nil {
		return TransferOutput{}, err
	}
	if inNamespace(answered, input.Namespace) {
		return TransferOutput{Output: answered}, nil
	}

	moved, landed, err := waitpoll.Until(ctx, waitpoll.UntilOptions[Output]{
		Request:  req,
		Interval: transferInterval,
		Bound:    transferBound,
		Message:  "Waiting for GitLab to move the project to " + input.Namespace,
		Read: func(readCtx context.Context) (Output, error) {
			return Get(readCtx, client, GetInput{ProjectID: toolutil.StringOrInt(strconv.FormatInt(answered.ID, 10))})
		},
		Landed: func(current Output) bool { return inNamespace(current, input.Namespace) },
	})
	if err != nil {
		return TransferOutput{}, err
	}
	if landed {
		return TransferOutput{Output: moved}, nil
	}
	return TransferOutput{Output: answered, TransferQueued: true}, nil
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
// The messages are GitLab's own (app/services/projects/transfer_service.rb):
// a transfer already under way and a project already in the namespace both
// mean the move is happening or has happened, which reading the project back
// settles, while only a name or path collision is fixed by renaming.
func transferError(err error) error {
	const op = "projectTransfer"
	switch {
	case toolutil.IsPermissionRefusal(err):
		return toolutil.WrapErrWithHint(op, err, hintTransferPermission)
	case toolutil.IsHTTPStatus(err, http.StatusNotFound):
		return toolutil.WrapErrWithHint(op, err, hintTransferNamespace)
	case !toolutil.IsHTTPStatus(err, http.StatusBadRequest):
		return toolutil.WrapErrWithMessage(op, err)
	case toolutil.ContainsAny(err, "transfer in progress", "already in this namespace"):
		return toolutil.WrapErrWithHint(op, err, hintTransferUnderWay)
	case toolutil.ContainsAny(err, "same name or path"):
		return toolutil.WrapErrWithHint(op, err, hintTransferCollision)
	case toolutil.ContainsAny(err, "don't have permission"):
		return toolutil.WrapErrWithHint(op, err, hintTransferPermission)
	default:
		return toolutil.WrapErrWithHint(op, err, hintTransferRefused)
	}
}
