package serverupdate

import (
	"context"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	checkUpdateToolName = "gitlab_server_check_update"
	applyUpdateToolName = "gitlab_server_apply_update"
)

// ActionSpecs returns canonical specs for visible server update tools. If
// updater is nil, auto-update is disabled and no update actions are available.
func ActionSpecs(updater *autoupdate.Updater) []toolutil.ActionSpec {
	if updater == nil {
		return nil
	}
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("check_update", toolutil.RouteFunc(func(ctx context.Context, input CheckInput) (CheckOutput, error) {
			return Check(ctx, updater, input)
		}), toolutil.ActionSpecOptions{
			Aliases: []string{checkUpdateToolName}, Usage: "Check whether a newer MCP server release is available.",
			OpenWorld:      true,
			OwnerPackage:   "serverupdate",
			IndividualTool: toolutil.IndividualToolSpec{Name: checkUpdateToolName, Title: toolutil.TitleFromName(checkUpdateToolName), Description: checkUpdateDescription},
		}),
		toolutil.NewDeleteActionSpec("apply_update", toolutil.DestructiveFunc(func(ctx context.Context, input ApplyInput) (ApplyOutput, error) {
			return Apply(ctx, updater, input)
		}), toolutil.ActionSpecOptions{
			Aliases: []string{applyUpdateToolName}, Usage: "Download and apply the latest MCP server update.",
			OpenWorld:      true,
			OwnerPackage:   "serverupdate",
			IndividualTool: toolutil.IndividualToolSpec{Name: applyUpdateToolName, Title: toolutil.TitleFromName(applyUpdateToolName), Description: applyUpdateDescription},
		}),
	}
}

const checkUpdateDescription = "Check if a newer version of the MCP server is available. Returns current version, latest version, release URL, and release notes.\n\nReturns: JSON with version comparison and release information.\n\nSee also: gitlab_server_apply_update, gitlab_server_status"

const applyUpdateDescription = "Download and apply the latest MCP server update. On Linux/macOS the binary is replaced atomically. On Windows the update is downloaded to a staging path with an update script (the running binary cannot be replaced). Use gitlab_server_check_update first to verify an update is available.\n\nReturns: JSON with update status and instructions.\n\nSee also: gitlab_server_check_update"
