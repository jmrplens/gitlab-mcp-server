package serverupdate

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers the gitlab_server_check_update and
// gitlab_server_apply_update tools. If updater is nil the tools are not
// registered (auto-update disabled).
func RegisterTools(server *mcp.Server, updater *autoupdate.Updater) {
	for _, spec := range ActionSpecs(updater) {
		toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Icons: toolutil.IconServer})
	}
}
