// action_specs.go holds what this package contributes to the canonical action
// catalog. The specs themselves are declared in internal/tools/adminspecs,
// which is the one place the gitlab_admin group is assembled; what stays here
// is the handler adaptation a spec routes to.
//
// This package used to declare a full set of specs of its own. Nothing ever
// aggregated them, so they reached no surface: the served metadata had drifted
// from this copy word for word, and a maintainer correcting the text here
// changed nothing a model reads.
package broadcastmessages

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// DeleteOutput deletes a broadcast message and returns the legacy success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted broadcast_message."}, nil
}
