package tools

import (
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionCatalogOptions controls which action groups are included in the
// canonical catalog.
type ActionCatalogOptions struct {
	Enterprise bool
	IncludeMCP bool
	Updater    *autoupdate.Updater
	SpecGroups []ActionSpecGroup
}

// BuildActionCatalog builds the canonical action catalog for catalog-backed
// GitLab action surfaces without constructing an MCP server.
func BuildActionCatalog(client *gitlabclient.Client, opts ActionCatalogOptions) (*actioncatalog.Catalog, error) {
	definitions := toolutil.CaptureMetaToolDefinitions(func() {
		registerAllMetaGroups(nil, client, opts.Enterprise)
	})
	specGroups := append(CollectActionSpecs(client, opts.Enterprise), opts.SpecGroups...)
	specsByTool, specErr := actionSpecGroupsByTool(specGroups)
	if specErr != nil {
		return nil, fmt.Errorf("collect action specs: %w", specErr)
	}
	catalog := actioncatalog.NewCatalog()
	for _, definition := range definitions {
		group, groupErr := groupFromMetaToolDefinition(definition, specsByTool[definition.Name])
		if groupErr != nil {
			return nil, fmt.Errorf("build meta tool group %q: %w", definition.Name, groupErr)
		}
		if addErr := catalog.AddGroup(group); addErr != nil {
			return nil, fmt.Errorf("add meta tool group %q: %w", definition.Name, addErr)
		}
	}
	if opts.IncludeMCP {
		if addErr := catalog.AddGroup(BuildMCPActionGroup(client, opts.Updater)); addErr != nil {
			return nil, fmt.Errorf("add MCP action group: %w", addErr)
		}
	}
	if validateErr := catalog.Validate(); validateErr != nil {
		return nil, fmt.Errorf("validate action catalog: %w", validateErr)
	}
	return catalog, nil
}

// groupFromMetaToolDefinition converts a captured meta-tool definition into an
// actioncatalog.Group built with actioncatalog.NewGroup and populated with
// group.SetAction. Route names are sorted first so catalog action ordering is
// deterministic across map iterations.
func groupFromMetaToolDefinition(def toolutil.MetaToolDefinition, specs []toolutil.ActionSpec) (actioncatalog.Group, error) {
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{
		ToolName:     def.Name,
		Description:  def.Description,
		Icons:        def.Icons,
		ReadOnly:     def.ReadOnly,
		FormatResult: def.FormatResult,
	})
	specActions, err := actioncatalog.ActionsFromSpecs(specs)
	if err != nil {
		return actioncatalog.Group{}, err
	}
	specActionByName := make(map[string]actioncatalog.Action, len(specActions))
	for _, action := range specActions {
		specActionByName[action.Name] = action
	}
	actionNames := make([]string, 0, len(def.Routes))
	for actionName := range def.Routes {
		actionNames = append(actionNames, actionName)
	}
	sort.Strings(actionNames)
	for _, actionName := range actionNames {
		if action, ok := specActionByName[actionName]; ok {
			group.SetAction(action)
			delete(specActionByName, actionName)
			continue
		}
		group.SetAction(actioncatalog.Action{Name: actionName, Route: def.Routes[actionName]})
	}
	if len(specActionByName) > 0 {
		missing := make([]string, 0, len(specActionByName))
		for actionName := range specActionByName {
			missing = append(missing, actionName)
		}
		sort.Strings(missing)
		return actioncatalog.Group{}, fmt.Errorf("spec actions missing captured routes: %v", missing)
	}
	return group, nil
}
