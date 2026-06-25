package tools

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionCatalogOptions controls which action groups are included in the
// canonical catalog.
//
// Tier is the effective instance licensing tier; an action is included only
// when its per-action minimum tier (derived from [toolutil.ActionSpec.Edition]
// via [edition.TierFromEdition]) is at most Tier. Enterprise is a deprecated
// back-compat selector retained for callers and tests that predate the 3-tier
// model: when Tier is the zero value ([edition.Free]) and Enterprise is true,
// the effective tier is [edition.Ultimate] (all tools), preserving the historical
// "enterprise = show everything" behavior. IncludeMCP adds the gitlab_server
// maintenance group; Updater enables the apply_update/check_update actions
// inside that group; SpecGroups injects additional [ActionSpecGroup] overrides
// that merge on top of the collected domain specs.
type ActionCatalogOptions struct {
	Tier       edition.Tier
	Enterprise bool
	IncludeMCP bool
	Updater    *autoupdate.Updater
	SpecGroups []ActionSpecGroup
}

// effectiveTier resolves the licensing tier used to gate catalog actions.
// An explicit Tier wins; otherwise the deprecated Enterprise flag maps true to
// [edition.Ultimate] and false to [edition.Free].
func (opts ActionCatalogOptions) effectiveTier() edition.Tier {
	if opts.Tier != edition.Free {
		return opts.Tier
	}
	if opts.Enterprise {
		return edition.Ultimate
	}
	return edition.Free
}

// BuildActionCatalog builds the canonical action catalog for catalog-backed
// GitLab action surfaces without constructing an MCP server.
//
// It collects domain-local [ActionSpecGroup]s via [CollectActionSpecs],
// merges any opts.SpecGroups overrides into them, materializes each group
// through [actioncatalog.GroupFromSpecs], adds the MCP maintenance group
// when opts.IncludeMCP is set, and finally calls
// [actioncatalog.Catalog.Validate] to surface ownership, schema, and
// surface classification errors before any runtime surface registers
// from the result.
//
// The returned catalog is the source of truth for [RegisterMetaCatalog],
// [RegisterIndividualCatalogTools], and the dynamic find/execute registry.
// Returns a wrapped error for any group construction, addition, or
// validation failure so callers can present a clear "build catalog
// group" / "add catalog group" / "validate action catalog" message.
func BuildActionCatalog(client *gitlabclient.Client, opts ActionCatalogOptions) (*actioncatalog.Catalog, error) {
	tier := opts.effectiveTier()
	specGroups := mergeActionSpecGroupOverrides(CollectActionSpecs(client, tier.IsEnterprise()), opts.SpecGroups)
	specGroups = filterActionSpecGroupsByTier(specGroups, tier)
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

// filterActionSpecGroupsByTier drops every action whose minimum required tier
// (from [toolutil.ActionSpec.Edition] via [edition.TierFromEdition]) exceeds the
// instance tier, and drops any group left with no actions. This is the single
// central tier gate: an instance at a given tier sees exactly the actions
// available at that tier or below (Free ⊂ Premium ⊂ Ultimate). Input groups are
// cloned so the shared spec slices returned by [CollectActionSpecs] are not
// mutated.
func filterActionSpecGroupsByTier(groups []ActionSpecGroup, tier edition.Tier) []ActionSpecGroup {
	out := make([]ActionSpecGroup, 0, len(groups))
	for _, group := range groups {
		// Pass through groups that were already empty so catalog validation can
		// still report them (e.g. malformed explicit-group overrides); only the
		// tier filter is allowed to remove a group here.
		if len(group.Actions) == 0 {
			out = append(out, group)
			continue
		}
		kept := make([]toolutil.ActionSpec, 0, len(group.Actions))
		for _, spec := range group.Actions {
			if edition.TierFromEdition(spec.Edition) <= tier {
				kept = append(kept, pruneSpecFieldsByTier(spec, tier))
			}
		}
		if len(kept) == 0 {
			continue
		}
		group = actioncatalog.CloneCatalogGroupSpec(group)
		group.Actions = kept
		out = append(out, group)
	}
	return out
}

// pruneSpecFieldsByTier returns spec with its input-schema properties that
// require a higher tier than the instance tier removed. Field tiers come from
// the input type's `tier:"premium|ultimate"` struct tags via
// [toolutil.FieldTiers]. The cached shared input schema is never mutated: a
// clone is produced only when at least one field must be dropped, so a Free
// instance never advertises Premium/Ultimate-only parameters.
func pruneSpecFieldsByTier(spec toolutil.ActionSpec, tier edition.Tier) toolutil.ActionSpec {
	fieldTiers := toolutil.FieldTiers(spec.Route.InputType)
	if len(fieldTiers) == 0 || spec.Route.InputSchema == nil {
		return spec
	}
	drop := make(map[string]bool, len(fieldTiers))
	for name, fieldTier := range fieldTiers {
		if edition.TierFromEdition(fieldTier) > tier {
			drop[name] = true
		}
	}
	if len(drop) == 0 {
		return spec
	}
	spec.Route.InputSchema = pruneSchemaProperties(spec.Route.InputSchema, drop)
	return spec
}

// pruneSchemaProperties returns a shallow clone of a JSON Schema object with the
// named properties removed from "properties" and "required". The input map is
// not mutated.
func pruneSchemaProperties(schema map[string]any, drop map[string]bool) map[string]any {
	cloned := make(map[string]any, len(schema))
	maps.Copy(cloned, schema)
	if props, ok := schema["properties"].(map[string]any); ok {
		newProps := make(map[string]any, len(props))
		for name, value := range props {
			if !drop[name] {
				newProps[name] = value
			}
		}
		cloned["properties"] = newProps
	}
	switch req := schema["required"].(type) {
	case []any:
		newReq := make([]any, 0, len(req))
		for _, item := range req {
			if name, _ := item.(string); !drop[name] {
				newReq = append(newReq, item)
			}
		}
		cloned["required"] = newReq
	case []string:
		newReq := make([]string, 0, len(req))
		for _, name := range req {
			if !drop[name] {
				newReq = append(newReq, name)
			}
		}
		cloned["required"] = newReq
	}
	return cloned
}

// mergeActionSpecGroupOverrides folds overrideGroups into baseGroups by tool
// name. Groups with a blank ToolName on either side are passed through (and
// later rejected by catalog validation) so the merge order stays stable and
// validation errors keep their original group context.
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

// mergeActionSpecGroup returns a clone of base with non-empty fields from
// override applied on top. Override actions fully replace base actions
// that share an action name; remaining base actions are preserved so
// overrides can add new actions without re-declaring the whole group.
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

// mergeActionSpecOverrides combines baseSpecs and overrideSpecs by name.
// Override actions with a non-blank Name replace any base spec of the same
// name; blank-named overrides are preserved as-is so the catalog validator
// can report them with the originating source.
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

// groupFromActionSpecGroup materializes one [ActionSpecGroup] into the
// canonical [actioncatalog.Group] consumed by every catalog-backed surface.
// It fills in default OwnerPackage, SurfaceKind, icons, formatter, ReadOnly,
// and description when the caller left them empty, then validates the group
// before returning it.
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

// ensureActionSpecOwners returns clones of specs with OwnerPackage filled
// in for any spec that did not declare one. The clones are defensive:
// callers may freely mutate the returned slice and individual specs
// without affecting the input. Returns nil when specs is empty.
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
