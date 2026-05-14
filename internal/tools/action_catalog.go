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
		route := withCatalogParameterGuidance(def.Name, actionName, def.Routes[actionName])
		group.SetAction(actioncatalog.Action{Name: actionName, Route: route})
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

func withCatalogParameterGuidance(toolName, actionName string, route toolutil.ActionRoute) toolutil.ActionRoute {
	actionID := actioncatalog.DomainFromToolName(toolName) + "." + actionName
	guidance := catalogParameterGuidance(actionID)
	if len(guidance) == 0 {
		return route
	}
	merged := make(map[string]toolutil.ParameterGuidance, len(route.ParameterGuidance)+len(guidance))
	for name, item := range route.ParameterGuidance {
		item.CommonConfusions = append([]string(nil), item.CommonConfusions...)
		merged[name] = item
	}
	for name, item := range guidance {
		item.CommonConfusions = append([]string(nil), item.CommonConfusions...)
		if existing, ok := merged[name]; ok {
			if existing.SemanticRole == "" {
				existing.SemanticRole = item.SemanticRole
			}
			if existing.ValueSource == "" {
				existing.ValueSource = item.ValueSource
			}
			if existing.ExampleBinding == "" {
				existing.ExampleBinding = item.ExampleBinding
			}
			existing.CommonConfusions = append(existing.CommonConfusions, item.CommonConfusions...)
			merged[name] = existing
			continue
		}
		merged[name] = item
	}
	route.ParameterGuidance = merged
	return route
}

func catalogParameterGuidance(actionID string) map[string]toolutil.ParameterGuidance {
	// Legacy migration bridge: remove this overlay in TASK-054 after all entries
	// move into canonical ActionSpec definitions owned by their domains.
	switch actionID {
	case "job.token_scope_remove_project":
		return map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_owner_project",
				ValueSource:      "Owning project whose CI job token allowlist is being changed.",
				CommonConfusions: []string{"Do not use the project being removed as project_id."},
				ExampleBinding:   "Remove project ID 51 from allowlist of project 1 => project_id=1.",
			},
			"target_project_id": {
				SemanticRole:     "target_project",
				ValueSource:      "Project being removed from or added to the allowlist.",
				CommonConfusions: []string{"Do not put the allowlist owner project here."},
				ExampleBinding:   "Remove project ID 51 from allowlist of project 1 => target_project_id=51.",
			},
		}
	case "issue.link_create":
		return map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "source_project",
				ValueSource:      "Project that owns the source issue.",
				CommonConfusions: []string{"Use target_project_id for the linked issue's project when it differs."},
			},
			"issue_iid": {
				SemanticRole:     "source_issue",
				ValueSource:      "IID of the source issue receiving the link.",
				CommonConfusions: []string{"Do not use the target issue IID here."},
			},
			"target_project_id": {
				SemanticRole:     "target_project",
				ValueSource:      "Project that owns the target issue.",
				CommonConfusions: []string{"For same-project links this may equal project_id; otherwise keep it distinct."},
			},
			"target_issue_iid": {
				SemanticRole:     "target_issue",
				ValueSource:      "IID of the issue being linked to.",
				CommonConfusions: []string{"Do not use the source issue IID here."},
			},
		}
	case "merge_request.create":
		return map[string]toolutil.ParameterGuidance{
			"source_branch": {
				SemanticRole:     "source_branch",
				ValueSource:      "Branch named after 'from'.",
				CommonConfusions: []string{"Do not use ref, tag_name, target_branch, or value for the source branch."},
				ExampleBinding:   "from feature/eval into main => source_branch=feature/eval.",
			},
			"target_branch": {
				SemanticRole:     "target_branch",
				ValueSource:      "Branch named after 'into' or the merge target.",
				CommonConfusions: []string{"Do not use source_branch, ref, tag_name, or to for the target branch."},
				ExampleBinding:   "from feature/eval into main => target_branch=main.",
			},
		}
	case "group.epic_issue_assign":
		return map[string]toolutil.ParameterGuidance{
			"full_path": {
				SemanticRole:     "parent_group_path",
				ValueSource:      "Group full path that owns the epic.",
				CommonConfusions: []string{"Do not use the child project path as full_path."},
			},
			"child_project_path": {
				SemanticRole:     "child_project_path",
				ValueSource:      "Project path that owns the issue being assigned to the epic.",
				CommonConfusions: []string{"Do not use project_id or target_full_path for this parameter."},
			},
			"child_iid": {
				SemanticRole:     "child_issue_iid",
				ValueSource:      "Issue IID in child_project_path.",
				CommonConfusions: []string{"Do not use epic_iid as child_iid."},
			},
		}
	case "access.deploy_token_delete_project":
		return map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole: "scope_owner_project",
				ValueSource:  "Project that owns the deploy token.",
			},
			"deploy_token_id": {
				SemanticRole:     "deploy_token",
				ValueSource:      "Deploy token ID, not a project, deploy key, personal token, or runner ID.",
				CommonConfusions: []string{"Do not use deploy_key_id or token_id for project deploy token deletion."},
			},
		}
	default:
		return nil
	}
}
