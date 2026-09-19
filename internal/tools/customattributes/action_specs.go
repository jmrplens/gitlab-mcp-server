// action_specs.go holds what this package contributes to the canonical action
// catalog. The specs themselves are declared in internal/tools/adminspecs,
// which is the one place the gitlab_admin group is assembled; what stays here
// is the handler adaptation a spec routes to.
//
// This package used to declare a full set of specs of its own that nothing
// ever aggregated, so they reached no surface and no validation.
package customattributes

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// DeleteOutput deletes a custom attribute and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted custom_attribute."}, nil
}
