package actioncatalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// SurfaceKind classifies the runtime surface represented by a catalog group.
type SurfaceKind string

const (
	// SurfaceKindGitLabAction identifies ordinary GitLab API actions.
	SurfaceKindGitLabAction SurfaceKind = "gitlab-action"
	// SurfaceKindMetaGroup identifies visible domain meta-tool dispatchers.
	SurfaceKindMetaGroup SurfaceKind = "meta-group"
	// SurfaceKindDynamicController identifies Dynamic controller tools.
	SurfaceKindDynamicController SurfaceKind = "dynamic-controller"
	// SurfaceKindRuntimeUtility identifies non-GitLab runtime helper tools.
	SurfaceKindRuntimeUtility SurfaceKind = "runtime-utility"
	// SurfaceKindInteractiveUtility identifies tools that require MCP elicitation.
	SurfaceKindInteractiveUtility SurfaceKind = "interactive-utility"
	// SurfaceKindSamplingUtility identifies tools that require MCP sampling.
	SurfaceKindSamplingUtility SurfaceKind = "sampling-utility"
	// SurfaceKindServerMaintenance identifies server maintenance tools.
	SurfaceKindServerMaintenance SurfaceKind = "server-maintenance"
)

// CatalogGroupSpec is the canonical metadata contract for one catalog group.
type CatalogGroupSpec struct {
	ToolName               string
	Title                  string
	Description            string
	ReadOnly               bool
	Icons                  []mcp.Icon
	BaseDomain             string
	EnterpriseOnly         bool
	GitLabDotComOnly       bool
	CapabilityRequirements []string
	FormatResult           toolutil.FormatResultFunc
	Actions                []toolutil.ActionSpec
	OwnerPackage           string
	SurfaceKind            SurfaceKind
}

// CloneCatalogGroupSpec returns a defensive copy of group metadata.
func CloneCatalogGroupSpec(spec CatalogGroupSpec) CatalogGroupSpec {
	spec.ToolName = strings.TrimSpace(spec.ToolName)
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.BaseDomain = strings.TrimSpace(spec.BaseDomain)
	spec.OwnerPackage = strings.TrimSpace(spec.OwnerPackage)
	spec.Icons = append([]mcp.Icon(nil), spec.Icons...)
	spec.CapabilityRequirements = cloneNormalizedStrings(spec.CapabilityRequirements)
	spec.Actions = toolutil.CloneActionSpecs(spec.Actions)
	if spec.SurfaceKind == "" {
		spec.SurfaceKind = SurfaceKindMetaGroup
	}
	return spec
}

// GroupOptions returns catalog group options projected from the group spec.
func (spec CatalogGroupSpec) GroupOptions() GroupOptions {
	spec = CloneCatalogGroupSpec(spec)
	return GroupOptions{
		ToolName:               spec.ToolName,
		Title:                  spec.Title,
		Description:            spec.Description,
		Icons:                  spec.Icons,
		ReadOnly:               spec.ReadOnly,
		BaseDomain:             spec.BaseDomain,
		EnterpriseOnly:         spec.EnterpriseOnly,
		GitLabDotComOnly:       spec.GitLabDotComOnly,
		CapabilityRequirements: spec.CapabilityRequirements,
		OwnerPackage:           spec.OwnerPackage,
		SurfaceKind:            spec.SurfaceKind,
		FormatResult:           spec.FormatResult,
	}
}

// Validate verifies group-level catalog invariants before runtime projection.
func (spec CatalogGroupSpec) Validate() error {
	spec = CloneCatalogGroupSpec(spec)
	if spec.ToolName == "" {
		return errors.New(errToolNameRequired)
	}
	if spec.OwnerPackage == "" {
		return fmt.Errorf("catalog group %q owner package is required", spec.ToolName)
	}
	if !validSurfaceKind(spec.SurfaceKind) {
		return fmt.Errorf("catalog group %q has unsupported surface kind %q", spec.ToolName, spec.SurfaceKind)
	}
	if len(spec.Actions) == 0 {
		return fmt.Errorf("catalog group %q has no actions", spec.ToolName)
	}
	seenActionIDs := make(map[ActionID]struct{}, len(spec.Actions))
	seenActionNames := make(map[string]struct{}, len(spec.Actions))
	for _, actionSpec := range spec.Actions {
		if err := actionSpec.Validate(); err != nil {
			return fmt.Errorf("catalog group %q action %q: %w", spec.ToolName, actionSpec.Name, err)
		}
		actionName := strings.TrimSpace(actionSpec.Name)
		if actionName == "" {
			return fmt.Errorf("catalog group %q action name is required", spec.ToolName)
		}
		if _, exists := seenActionNames[actionName]; exists {
			return fmt.Errorf("catalog group %q duplicate action %q", spec.ToolName, actionName)
		}
		seenActionNames[actionName] = struct{}{}
		if actionSpec.OwnerPackage == "" {
			return fmt.Errorf("catalog group %q action %q owner package is required", spec.ToolName, actionName)
		}
		if actionSpec.Route.InputSchema == nil {
			return fmt.Errorf("catalog group %q action %q has nil input schema", spec.ToolName, actionName)
		}
		actionID := catalogGroupActionID(spec, actionName)
		if _, exists := seenActionIDs[actionID]; exists {
			return fmt.Errorf("catalog group %q duplicate action id %q", spec.ToolName, actionID)
		}
		seenActionIDs[actionID] = struct{}{}
	}
	return validateCatalogGroupAliases(spec)
}

func validSurfaceKind(kind SurfaceKind) bool {
	switch kind {
	case SurfaceKindGitLabAction,
		SurfaceKindMetaGroup,
		SurfaceKindDynamicController,
		SurfaceKindRuntimeUtility,
		SurfaceKindInteractiveUtility,
		SurfaceKindSamplingUtility,
		SurfaceKindServerMaintenance:
		return true
	default:
		return false
	}
}

func catalogGroupActionID(spec CatalogGroupSpec, actionName string) ActionID {
	domain := strings.TrimSpace(spec.BaseDomain)
	if domain == "" {
		domain = DomainFromToolName(spec.ToolName)
	}
	return ActionID(domain + "." + actionName)
}

func validateCatalogGroupAliases(spec CatalogGroupSpec) error {
	seenActionAliases := make(map[string]ActionID)
	seenParameterAliases := make(map[string]string)
	for _, actionSpec := range spec.Actions {
		actionID := catalogGroupActionID(spec, actionSpec.Name)
		for _, alias := range actionSpec.Compatibility.ActionAliases {
			aliasName := strings.TrimSpace(strings.ToLower(alias.Alias))
			if aliasName == "" {
				continue
			}
			if existing, ok := seenActionAliases[aliasName]; ok && existing != actionID {
				return fmt.Errorf("catalog group %q compatibility action alias %q maps to both %q and %q", spec.ToolName, aliasName, existing, actionID)
			}
			seenActionAliases[aliasName] = actionID
		}
		for _, alias := range actionSpec.Compatibility.ParameterAliases {
			aliasName := strings.TrimSpace(strings.ToLower(alias.Alias))
			if aliasName == "" {
				continue
			}
			aliasKey := string(actionID) + ":" + aliasName
			if existing, ok := seenParameterAliases[aliasKey]; ok && existing != alias.Target {
				return fmt.Errorf("catalog group %q action %q compatibility parameter alias %q maps to both %q and %q", spec.ToolName, actionSpec.Name, aliasName, existing, alias.Target)
			}
			seenParameterAliases[aliasKey] = alias.Target
		}
	}
	return nil
}

func cloneNormalizedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
