package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	specGroups := mergeActionSpecGroupOverrides(CollectActionSpecs(client, opts.Enterprise), opts.SpecGroups)
	catalog := actioncatalog.NewCatalog()
	for _, specGroup := range specGroups {
		group, groupErr := groupFromActionSpecGroup(specGroup)
		if groupErr != nil {
			return nil, fmt.Errorf("build catalog group %q: %w", specGroup.ToolName, groupErr)
		}
		if addErr := catalog.AddGroup(group); addErr != nil {
			return nil, fmt.Errorf("add catalog group %q: %w", group.ToolName, addErr)
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

func mergeActionSpecGroupOverrides(baseGroups, overrideGroups []ActionSpecGroup) []ActionSpecGroup {
	if len(overrideGroups) == 0 {
		return baseGroups
	}
	mergedByTool := make(map[string]ActionSpecGroup, len(baseGroups)+len(overrideGroups))
	invalidGroups := make([]ActionSpecGroup, 0)
	for _, group := range baseGroups {
		toolName := strings.TrimSpace(group.ToolName)
		if toolName == "" {
			invalidGroups = append(invalidGroups, group)
			continue
		}
		mergedByTool[toolName] = actioncatalog.CloneCatalogGroupSpec(group)
	}
	for _, override := range overrideGroups {
		toolName := strings.TrimSpace(override.ToolName)
		if toolName == "" {
			invalidGroups = append(invalidGroups, override)
			continue
		}
		base := mergedByTool[toolName]
		mergedByTool[toolName] = mergeActionSpecGroup(base, override)
	}
	toolNames := make([]string, 0, len(mergedByTool))
	for toolName := range mergedByTool {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)
	merged := make([]ActionSpecGroup, 0, len(invalidGroups)+len(toolNames))
	merged = append(merged, invalidGroups...)
	for _, toolName := range toolNames {
		merged = append(merged, mergedByTool[toolName])
	}
	return merged
}

func mergeActionSpecGroup(base, override ActionSpecGroup) ActionSpecGroup {
	merged := actioncatalog.CloneCatalogGroupSpec(base)
	override = actioncatalog.CloneCatalogGroupSpec(override)
	if merged.ToolName == "" {
		merged.ToolName = strings.TrimSpace(override.ToolName)
	}
	if strings.TrimSpace(override.Title) != "" {
		merged.Title = override.Title
	}
	if strings.TrimSpace(override.Description) != "" {
		merged.Description = override.Description
	}
	if len(override.Icons) > 0 {
		merged.Icons = override.Icons
	}
	if override.ReadOnly {
		merged.ReadOnly = true
	}
	if strings.TrimSpace(override.BaseDomain) != "" {
		merged.BaseDomain = override.BaseDomain
	}
	if override.EnterpriseOnly {
		merged.EnterpriseOnly = true
	}
	if override.GitLabDotComOnly {
		merged.GitLabDotComOnly = true
	}
	if len(override.CapabilityRequirements) > 0 {
		merged.CapabilityRequirements = override.CapabilityRequirements
	}
	if override.FormatResult != nil {
		merged.FormatResult = override.FormatResult
	}
	if strings.TrimSpace(override.OwnerPackage) != "" {
		merged.OwnerPackage = override.OwnerPackage
	}
	if override.SurfaceKind != "" {
		merged.SurfaceKind = override.SurfaceKind
	}
	merged.Actions = mergeActionSpecOverrides(merged.Actions, override.Actions)
	return merged
}

func mergeActionSpecOverrides(baseSpecs, overrideSpecs []toolutil.ActionSpec) []toolutil.ActionSpec {
	if len(overrideSpecs) == 0 {
		return baseSpecs
	}
	overrideNames := make(map[string]struct{}, len(overrideSpecs))
	for _, spec := range overrideSpecs {
		name := strings.TrimSpace(spec.Name)
		if name != "" {
			overrideNames[name] = struct{}{}
		}
	}
	merged := make([]toolutil.ActionSpec, 0, len(baseSpecs)+len(overrideSpecs))
	for _, spec := range baseSpecs {
		if _, overridden := overrideNames[strings.TrimSpace(spec.Name)]; overridden {
			continue
		}
		merged = append(merged, spec)
	}
	merged = append(merged, overrideSpecs...)
	return merged
}

func groupFromActionSpecGroup(specGroup ActionSpecGroup) (actioncatalog.Group, error) {
	specGroup = actioncatalog.CloneCatalogGroupSpec(specGroup)
	if specGroup.OwnerPackage == "" {
		specGroup.OwnerPackage = "tools"
	}
	specGroup.Actions = ensureActionSpecOwners(specGroup.Actions, specGroup.OwnerPackage)
	if specGroup.SurfaceKind == "" {
		specGroup.SurfaceKind = actioncatalog.SurfaceKindMetaGroup
	}
	if len(specGroup.Icons) == 0 {
		specGroup.Icons = catalogGroupIcons(specGroup.ToolName)
	}
	if specGroup.FormatResult == nil {
		specGroup.FormatResult = catalogGroupFormatResult(specGroup.ToolName)
	}
	if !specGroup.ReadOnly {
		specGroup.ReadOnly = catalogGroupReadOnly(specGroup.Actions)
	}
	if specGroup.Description == "" {
		routes, err := toolutil.ActionSpecsToMapWithError(specGroup.Actions)
		if err != nil {
			return actioncatalog.Group{}, err
		}
		specGroup.Description = catalogGroupDescription(specGroup.ToolName, routes)
	}
	if err := specGroup.Validate(); err != nil {
		return actioncatalog.Group{}, err
	}
	return actioncatalog.GroupFromSpecs(specGroup.GroupOptions(), specGroup.Actions)
}

func ensureActionSpecOwners(specs []toolutil.ActionSpec, ownerPackage string) []toolutil.ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	out := toolutil.CloneActionSpecs(specs)
	for index := range out {
		if out[index].OwnerPackage == "" {
			out[index].OwnerPackage = ownerPackage
		}
	}
	return out
}
