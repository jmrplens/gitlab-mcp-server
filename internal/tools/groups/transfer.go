package groups

import (
	"context"
	"errors"
	"net/http"
	"strconv"

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

// The hints a refused group transfer is answered with. GitLab refuses a
// transfer it cannot start, one already under way, a path collision and a
// missing permission all with 400, and each needs a different next step, so
// the hint is chosen by what GitLab said rather than by the status alone.
const (
	hintSubGroupTransferPermission = "transferring a group requires the Owner role on the group and on the destination parent group. Use group.transfer_locations to list the parents it can be moved under"
	hintSubGroupTransferNotFound   = "verify group_id (and parent_id) with group.get"
	hintSubGroupTransferUnderWay   = "GitLab is already moving this group, or has moved it: since GitLab 19.4 a transfer is applied in the background. Wait, then read the group back with group.get to see its parent before transferring it again"
	hintSubGroupTransferCollision  = "the destination parent already has a subgroup or project with this group's path. Change the group's path with group.update, or choose another parent from group.transfer_locations"
	hintSubGroupTransferRefused    = "the destination parent is invalid for this group, for example one of its own subgroups, or GitLab cannot move what the group holds (container images, npm packages, a subscription). Use group.transfer_locations to find valid parents"
)

// TransferSubGroupInput defines parameters for transferring a group under a new
// parent group or to the top level.
type TransferSubGroupInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id"  jsonschema:"Group ID or URL-encoded path of the group to move,required"`
	ParentID *int64               `json:"parent_id,omitempty" jsonschema:"ID of the new parent group. Omit to turn the subgroup into a top-level group"`
}

// TransferSubGroupOutput is the group a transfer answers with. GitLab 19.4 and
// later move a group in the background, so the handler reads it back until it
// sits under the parent the transfer named. TransferQueued says the wait ended
// first, and the group is then described where it still was.
//
// The flag is this server's own and GitLab sends nothing like it: no entity
// exposes the transfer state of a group, so whether a move has landed can only
// be told by comparing the group's parent with the one it was sent to.
type TransferSubGroupOutput struct {
	DetailOutput
	TransferQueued bool `json:"transfer_queued,omitempty" jsonschema:"True when GitLab accepted the transfer but had not applied it when the wait ended. The group fields then show where it still is. Read it back with group.get before relying on its new path"`
}

// TransferSubGroup moves a group under a new parent group, or promotes a
// subgroup to a top-level group when parent_id is omitted, and waits for the
// move.
//
// GitLab 19.4 and later accept the transfer, answer with the group where it
// still is and move it in a background worker, so an answer that does not yet
// show the new parent is read back until it does, for up to
// [waitpoll.TransferBound]. An older GitLab answers after the move, and its
// answer is returned as it is, with no read at all. A move that has not landed
// when the wait ends is answered with the group from the transfer's own answer
// and TransferQueued set, never as an error: GitLab accepted it, and a model
// told the transfer failed would send it again, which GitLab refuses while the
// first is under way.
func TransferSubGroup(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input TransferSubGroupInput) (TransferSubGroupOutput, error) {
	if err := ctx.Err(); err != nil {
		return TransferSubGroupOutput{}, err
	}
	if input.GroupID == "" {
		return TransferSubGroupOutput{}, errors.New("groupTransferSubGroup: group_id is required")
	}
	opts := &gl.TransferSubGroupOptions{}
	if input.ParentID != nil {
		opts.GroupID = input.ParentID
	}
	transferCtx, captured := gitlabclient.WithResponseCapture(ctx)
	g, _, err := client.GL().Groups.TransferSubGroup(string(input.GroupID), opts, gl.WithContext(transferCtx))
	if err != nil {
		return TransferSubGroupOutput{}, transferSubGroupError(err)
	}
	answered, err := groupDetail("TransferSubGroup", g, captured)
	if err != nil {
		return TransferSubGroupOutput{}, err
	}
	if underParent(answered, input.ParentID) {
		return TransferSubGroupOutput{DetailOutput: answered}, nil
	}

	moved, landed, err := waitpoll.Until(ctx, waitpoll.UntilOptions[DetailOutput]{
		Request:  req,
		Interval: transferInterval,
		Bound:    transferBound,
		Message:  "Waiting for GitLab to move the group",
		Read: func(readCtx context.Context) (DetailOutput, error) {
			// The transfer's own answer carries no projects, and a read that
			// asked for them would page through a group's projects on every
			// attempt only to change the shape of the answer.
			return Get(readCtx, client, GetInput{
				GroupID:      toolutil.StringOrInt(strconv.FormatInt(answered.ID, 10)),
				WithProjects: new(false),
			})
		},
		Landed: func(current DetailOutput) bool { return underParent(current, input.ParentID) },
	})
	if err != nil {
		return TransferSubGroupOutput{}, err
	}
	if landed {
		return TransferSubGroupOutput{DetailOutput: moved}, nil
	}
	return TransferSubGroupOutput{DetailOutput: answered, TransferQueued: true}, nil
}

// underParent reports whether a group sits where a transfer put it: under the
// parent it named, or at the top level, where GitLab reports no parent, when
// it named none.
func underParent(g DetailOutput, parentID *int64) bool {
	if parentID == nil {
		return g.ParentID == 0
	}
	return g.ParentID == *parentID
}

// transferSubGroupError wraps a refused group transfer with the hint its
// refusal calls for. The messages are GitLab's own
// (app/services/groups/transfer_service.rb): a transfer already under way, a
// group already under that parent and a group already at the top level all
// mean the move is happening or has happened, which reading the group back
// settles, while only a path collision is fixed by renaming.
func transferSubGroupError(err error) error {
	const op = "groupTransferSubGroup"
	switch {
	case toolutil.IsPermissionRefusal(err):
		return toolutil.WrapErrWithHint(op, err, hintSubGroupTransferPermission)
	case toolutil.IsHTTPStatus(err, http.StatusNotFound):
		return toolutil.WrapErrWithHint(op, err, hintSubGroupTransferNotFound)
	case !toolutil.IsHTTPStatus(err, http.StatusBadRequest):
		return toolutil.WrapErrWithMessage(op, err)
	case toolutil.ContainsAny(err, "transfer in progress", "already associated to the parent group", "already a root group"):
		return toolutil.WrapErrWithHint(op, err, hintSubGroupTransferUnderWay)
	case toolutil.ContainsAny(err, "with the same path"):
		return toolutil.WrapErrWithHint(op, err, hintSubGroupTransferCollision)
	case toolutil.ContainsAny(err, "enough permissions"):
		return toolutil.WrapErrWithHint(op, err, hintSubGroupTransferPermission)
	default:
		return toolutil.WrapErrWithHint(op, err, hintSubGroupTransferRefused)
	}
}
