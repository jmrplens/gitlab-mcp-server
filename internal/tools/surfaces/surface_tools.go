package surfaces

import (
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/serverupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	surfaceSafeModeGlobalWrapper = "global-safe-mode-wrapper"
	surfaceReadOnlyGlobalFilter  = "global-read-only-filter"
)

// CatalogOptions controls projection of standalone surface tools into an action
// catalog.
type CatalogOptions struct {
	ReadOnlyOnly     bool
	ExcludeToolNames []string
}

// StandaloneToolSpecs returns visible utility tools that remain outside
// ordinary GitLab API meta-tool dispatchers.
func StandaloneToolSpecs(client *gitlabclient.Client) []actioncatalog.SurfaceToolSpec {
	specs := make([]actioncatalog.SurfaceToolSpec, 0, 5)
	specs = append(specs, surfaceToolSpecsFromActions(surfaceToolGroupOptions{
		GroupToolName:         "gitlab_discover_project",
		BaseDomain:            "discover_project",
		SurfaceKind:           actioncatalog.SurfaceKindRuntimeUtility,
		Icons:                 toolutil.IconProject,
		FormatResult:          toolutil.MarkdownForResult,
		OwnerPackage:          "projectdiscovery",
		Description:           "Resolve a full git remote URL to a GitLab project and return its project_id and metadata. Read-only; use only for complete git remote URLs from .git/config or git remote -v.",
		CompatibilityToolName: "gitlab_discover_project",
	}, projectdiscovery.ActionSpecs(client))...)
	specs = append(specs, surfaceToolSpecsFromActions(surfaceToolGroupOptions{
		GroupToolName:          "gitlab_interactive",
		BaseDomain:             "interactive",
		SurfaceKind:            actioncatalog.SurfaceKindInteractiveUtility,
		Icons:                  toolutil.IconConfig,
		CapabilityRequirements: []string{"elicitation"},
		FormatResult:           elicitationtools.FormatResult,
		OwnerPackage:           "elicitationtools",
		Description:            "Guided interactive creation flows for issues, merge requests, projects, and releases. Mutating; use only when the task explicitly asks for a guided flow.",
		CompatibilityToolName:  "gitlab_interactive",
	}, elicitationtools.ActionSpecs(client))...)
	return specs
}

// ServerMaintenanceToolSpecs returns updater-backed visible server maintenance
// tools.
func ServerMaintenanceToolSpecs(updater *autoupdate.Updater) []actioncatalog.SurfaceToolSpec {
	return surfaceToolSpecsFromActions(surfaceToolGroupOptions{
		GroupToolName:         "gitlab_server",
		BaseDomain:            "server",
		SurfaceKind:           actioncatalog.SurfaceKindServerMaintenance,
		Icons:                 toolutil.IconServer,
		FormatResult:          toolutil.MarkdownForResult,
		OwnerPackage:          "serverupdate",
		Description:           "MCP server maintenance tools for update checks and manual update application.",
		CompatibilityToolName: "gitlab_server",
	}, serverupdate.ActionSpecs(updater))
}

// AddToolCatalog projects surface specs into a catalog used by Dynamic
// discovery/execution.
func AddToolCatalog(catalog *actioncatalog.Catalog, specs []actioncatalog.SurfaceToolSpec, opts CatalogOptions) (*actioncatalog.Catalog, error) {
	if catalog == nil {
		catalog = actioncatalog.NewCatalog()
	}
	groups := ToolGroupSpecs(filterToolSpecs(specs, opts))
	for _, groupSpec := range groups {
		group, err := actioncatalog.GroupFromSpecs(groupSpec.GroupOptions(), groupSpec.Actions)
		if err != nil {
			return nil, fmt.Errorf("build surface tool group %s: %w", groupSpec.ToolName, err)
		}
		if addErr := catalog.AddGroup(group); addErr != nil {
			return nil, fmt.Errorf("add surface tool group %s: %w", surfaceGroupActionLabel(group), addErr)
		}
	}
	return catalog, nil
}

func surfaceGroupActionLabel(group actioncatalog.Group) string {
	actions := group.ActionsInOrder()
	if len(actions) == 0 {
		return group.ToolName
	}
	return group.ToolName + "." + actions[0].Name
}

// ToolGroupSpecs groups surface specs into catalog group specs.
func ToolGroupSpecs(specs []actioncatalog.SurfaceToolSpec) []actioncatalog.CatalogGroupSpec {
	if len(specs) == 0 {
		return nil
	}
	type groupedSurface struct {
		options surfaceToolGroupOptions
		actions []toolutil.ActionSpec
	}
	groups := make(map[string]groupedSurface)
	for _, spec := range specs {
		spec = actioncatalog.CloneSurfaceToolSpec(spec)
		actionSpec, err := spec.ActionSpec()
		if err != nil {
			panic(fmt.Errorf("project surface tool %s: %w", spec.Name, err))
		}
		group := groups[spec.GroupToolName]
		if group.options.GroupToolName == "" {
			group.options = surfaceToolGroupOptions{
				GroupToolName:          spec.GroupToolName,
				BaseDomain:             spec.BaseDomain,
				SurfaceKind:            spec.SurfaceKind,
				Icons:                  spec.Icons,
				CapabilityRequirements: spec.CapabilityRequirements,
				FormatResult:           spec.FormatResult,
				OwnerPackage:           spec.OwnerPackage,
				Description:            spec.Description,
			}
		}
		group.actions = append(group.actions, actionSpec)
		groups[spec.GroupToolName] = group
	}
	toolNames := make([]string, 0, len(groups))
	for toolName := range groups {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)
	out := make([]actioncatalog.CatalogGroupSpec, 0, len(toolNames))
	for _, toolName := range toolNames {
		group := groups[toolName]
		out = append(out, surfaceActionSpecGroup(group.options, group.actions))
	}
	return out
}

type surfaceToolGroupOptions struct {
	GroupToolName          string
	BaseDomain             string
	SurfaceKind            actioncatalog.SurfaceKind
	Icons                  []mcp.Icon
	CapabilityRequirements []string
	FormatResult           toolutil.FormatResultFunc
	OwnerPackage           string
	Description            string
	CompatibilityToolName  string
}

func surfaceToolSpecsFromActions(opts surfaceToolGroupOptions, specs []toolutil.ActionSpec) []actioncatalog.SurfaceToolSpec {
	specs = actioncompat.ApplyToActionSpecs(opts.CompatibilityToolName, opts.BaseDomain, specs)
	out := make([]actioncatalog.SurfaceToolSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, actioncatalog.SurfaceToolSpec{
			Name:                   spec.IndividualTool.Name,
			Title:                  spec.IndividualTool.Title,
			Description:            spec.IndividualTool.Description,
			GroupToolName:          opts.GroupToolName,
			BaseDomain:             opts.BaseDomain,
			ActionName:             spec.Name,
			SurfaceKind:            opts.SurfaceKind,
			Route:                  spec.Route,
			Aliases:                spec.Aliases,
			Tags:                   spec.Tags,
			RelatedActions:         spec.RelatedActions,
			Compatibility:          spec.Compatibility,
			Icons:                  opts.Icons,
			CapabilityRequirements: opts.CapabilityRequirements,
			FormatResult:           opts.FormatResult,
			SafeModePolicy:         surfaceSafeModeGlobalWrapper,
			ReadOnlyPolicy:         surfaceReadOnlyGlobalFilter,
			OwnerPackage:           opts.OwnerPackage,
			ReadOnly:               spec.ReadOnly,
			Destructive:            spec.Destructive,
			Idempotent:             spec.Idempotent,
			OpenWorld:              spec.OpenWorld,
		})
	}
	return out
}

func surfaceActionSpecGroup(opts surfaceToolGroupOptions, specs []toolutil.ActionSpec) actioncatalog.CatalogGroupSpec {
	return actioncatalog.CatalogGroupSpec{
		ToolName:               opts.GroupToolName,
		Description:            opts.Description,
		ReadOnly:               readOnlyGroup(specs),
		Icons:                  opts.Icons,
		BaseDomain:             opts.BaseDomain,
		CapabilityRequirements: opts.CapabilityRequirements,
		FormatResult:           opts.FormatResult,
		Actions:                specs,
		OwnerPackage:           opts.OwnerPackage,
		SurfaceKind:            opts.SurfaceKind,
	}
}

func filterToolSpecs(specs []actioncatalog.SurfaceToolSpec, opts CatalogOptions) []actioncatalog.SurfaceToolSpec {
	excluded := stringSet(opts.ExcludeToolNames)
	out := make([]actioncatalog.SurfaceToolSpec, 0, len(specs))
	for _, spec := range specs {
		if opts.ReadOnlyOnly && !spec.ReadOnly {
			continue
		}
		if _, ok := excluded[strings.TrimSpace(spec.Name)]; ok {
			continue
		}
		if _, ok := excluded[strings.TrimSpace(spec.GroupToolName)]; ok {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func readOnlyGroup(specs []toolutil.ActionSpec) bool {
	if len(specs) == 0 {
		return false
	}
	for _, spec := range specs {
		if !spec.ReadOnly {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}
