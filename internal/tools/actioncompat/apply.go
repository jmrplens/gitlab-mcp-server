package actioncompat

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ApplyToGroupSpecs projects compatibility policies into matching ActionSpecs.
func ApplyToGroupSpecs(groups []actioncatalog.CatalogGroupSpec) []actioncatalog.CatalogGroupSpec {
	if len(groups) == 0 {
		return nil
	}
	out := make([]actioncatalog.CatalogGroupSpec, 0, len(groups))
	for _, group := range groups {
		out = append(out, ApplyToGroupSpec(group))
	}
	return out
}

// ApplyToGroupSpec projects compatibility policies into one catalog group spec.
func ApplyToGroupSpec(group actioncatalog.CatalogGroupSpec) actioncatalog.CatalogGroupSpec {
	group = actioncatalog.CloneCatalogGroupSpec(group)
	group.Actions = ApplyToActionSpecs(group.ToolName, group.BaseDomain, group.Actions)
	return group
}

// ApplyToActionSpecs projects compatibility policies into specs for one group.
func ApplyToActionSpecs(toolName, baseDomain string, specs []toolutil.ActionSpec) []toolutil.ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	actionAliases := actionAliasesByCanonicalID()
	parameterAliases := parameterAliasesByActionID()
	out := make([]toolutil.ActionSpec, 0, len(specs))
	for _, spec := range specs {
		actionID := groupActionID(toolName, baseDomain, spec.Name)
		compatibility := toolutil.CloneCompatibilityPolicy(spec.Compatibility)
		compatibility.ActionAliases = append(compatibility.ActionAliases, actionAliasSpecsForAction(spec.Name, actionAliases[actionID])...)
		compatibility.ParameterAliases = append(compatibility.ParameterAliases, parameterAliasSpecs(parameterAliases[actionID])...)
		out = append(out, cloneSpecWithCompatibility(spec, compatibility))
	}
	return out
}

func groupActionID(toolName, baseDomain, actionName string) string {
	domain := strings.TrimSpace(baseDomain)
	if domain == "" {
		domain = actioncatalog.DomainFromToolName(toolName)
	}
	return strings.ToLower(strings.TrimSpace(domain + "." + actionName))
}

func actionAliasesByCanonicalID() map[string][]ActionAlias {
	aliases := make(map[string][]ActionAlias)
	for _, alias := range ActionAliases() {
		aliases[alias.Canonical] = append(aliases[alias.Canonical], alias)
	}
	return aliases
}

func actionAliasSpecsForAction(actionName string, aliases []ActionAlias) []toolutil.ActionAliasSpec {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]toolutil.ActionAliasSpec, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, toolutil.ActionAliasSpec{
			Alias:          alias.Alias,
			Target:         actionName,
			Source:         alias.Source,
			Searchable:     alias.Searchable,
			Deprecated:     alias.Deprecated,
			RemovalVersion: alias.RemovalVersion,
			Reason:         alias.Reason,
		})
	}
	return out
}

func parameterAliasesByActionID() map[string][]ParameterAlias {
	aliases := make(map[string][]ParameterAlias)
	for _, alias := range ParameterAliases() {
		if alias.SpecMetadata {
			aliases[alias.ActionID] = append(aliases[alias.ActionID], alias)
		}
	}
	return aliases
}

func parameterAliasSpecs(aliases []ParameterAlias) []toolutil.ParameterAliasSpec {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]toolutil.ParameterAliasSpec, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, toolutil.ParameterAliasSpec{
			Alias:          alias.Alias,
			Target:         alias.Target,
			Source:         alias.Source,
			Searchable:     alias.Searchable,
			Deprecated:     alias.Deprecated,
			RemovalVersion: alias.RemovalVersion,
			Reason:         alias.Reason,
		})
	}
	return out
}

func cloneSpecWithCompatibility(spec toolutil.ActionSpec, compatibility toolutil.CompatibilityPolicy) toolutil.ActionSpec {
	return toolutil.NewActionSpec(spec.Name, spec.Route, toolutil.ActionSpecOptions{
		Aliases:                spec.Aliases,
		Tags:                   spec.Tags,
		Usage:                  spec.Usage,
		RelatedActions:         spec.RelatedActions,
		Compatibility:          compatibility,
		ParameterGuidance:      spec.ParameterGuidance,
		InputSchemaOverrides:   spec.InputSchemaOverrides,
		ReadOnly:               spec.ReadOnly,
		Destructive:            spec.Destructive,
		Idempotent:             spec.Idempotent,
		OpenWorld:              spec.OpenWorld,
		Edition:                spec.Edition,
		GitLabDotComOnly:       spec.GitLabDotComOnly,
		OwnerPackage:           spec.OwnerPackage,
		IndividualTool:         spec.IndividualTool,
		ContentKind:            spec.ContentKind,
		NotFoundPolicy:         spec.NotFoundPolicy,
		EmbeddedResourcePolicy: spec.EmbeddedResourcePolicy,
		RichResultPolicy:       spec.RichResultPolicy,
		SchemaValidationNotes:  append([]string(nil), spec.SchemaValidationNotes...),
		RuntimeValidationNotes: append([]string(nil), spec.RuntimeValidationNotes...),
	})
}
