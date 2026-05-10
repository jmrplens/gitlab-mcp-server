package tools

import (
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionCatalogOptions controls which action groups are included in a catalog.
type ActionCatalogOptions struct {
	Enterprise bool
	IncludeMCP bool
	Updater    *autoupdate.Updater
}

// BuildActionCatalog builds the canonical action catalog for meta-style GitLab
// actions without constructing an MCP server.
func BuildActionCatalog(client *gitlabclient.Client, opts ActionCatalogOptions) (*actionregistry.Catalog, error) {
	definitions := toolutil.CaptureMetaToolDefinitions(func() {
		registerAllMetaGroups(nil, client, opts.Enterprise)
	})
	catalog := actionregistry.NewCatalog()
	for _, definition := range definitions {
		if err := catalog.AddGroup(groupFromMetaToolDefinition(definition)); err != nil {
			return nil, err
		}
	}
	if opts.IncludeMCP {
		if err := catalog.AddGroup(BuildMCPActionGroup(client, opts.Updater)); err != nil {
			return nil, err
		}
	}
	return catalog, nil
}

// groupFromMetaToolDefinition converts a captured meta-tool definition into an
// actionregistry.Group built with actionregistry.NewGroup and populated with
// group.SetAction. Route names are sorted first so catalog action ordering is
// deterministic across map iterations.
func groupFromMetaToolDefinition(def toolutil.MetaToolDefinition) actionregistry.Group {
	group := actionregistry.NewGroup(actionregistry.GroupOptions{
		ToolName:     def.Name,
		Description:  def.Description,
		Icons:        def.Icons,
		ReadOnly:     def.ReadOnly,
		FormatResult: def.FormatResult,
	})
	actionNames := make([]string, 0, len(def.Routes))
	for actionName := range def.Routes {
		actionNames = append(actionNames, actionName)
	}
	sort.Strings(actionNames)
	for _, actionName := range actionNames {
		group.SetAction(actionregistry.Action{Name: actionName, Route: def.Routes[actionName]})
	}
	return group
}
