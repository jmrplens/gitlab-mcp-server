package actioncatalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// SurfaceToolSpec is the canonical metadata contract for visible MCP tools that
// are not ordinary GitLab API meta-tool groups.
type SurfaceToolSpec struct {
	Name                   string
	Title                  string
	Description            string
	GroupToolName          string
	BaseDomain             string
	ActionName             string
	SurfaceKind            SurfaceKind
	Route                  toolutil.ActionRoute
	Aliases                []string
	Tags                   []string
	RelatedActions         []string
	Compatibility          toolutil.CompatibilityPolicy
	Icons                  []mcp.Icon
	CapabilityRequirements []string
	FormatResult           toolutil.FormatResultFunc
	SafeModePolicy         string
	ReadOnlyPolicy         string
	OwnerPackage           string
	ReadOnly               bool
	Destructive            bool
	Idempotent             bool
	OpenWorld              bool
}

// Validate verifies that the surface spec has enough metadata for runtime
// registration and catalog projection.
func (spec SurfaceToolSpec) Validate() error {
	spec = CloneSurfaceToolSpec(spec)
	if spec.Name == "" {
		return errors.New("surface tool name is required")
	}
	if spec.Description == "" {
		return fmt.Errorf("surface tool %q description is required", spec.Name)
	}
	if spec.GroupToolName == "" {
		return fmt.Errorf("surface tool %q group tool name is required", spec.Name)
	}
	if spec.BaseDomain == "" {
		return fmt.Errorf("surface tool %q base domain is required", spec.Name)
	}
	if spec.ActionName == "" {
		return fmt.Errorf("surface tool %q action name is required", spec.Name)
	}
	if spec.OwnerPackage == "" {
		return fmt.Errorf("surface tool %q owner package is required", spec.Name)
	}
	if !validSurfaceKind(spec.SurfaceKind) {
		return fmt.Errorf("surface tool %q has unsupported surface kind %q", spec.Name, spec.SurfaceKind)
	}
	if spec.Route.Handler == nil {
		return fmt.Errorf("surface tool %q route handler is required", spec.Name)
	}
	if spec.Route.InputSchema == nil {
		return fmt.Errorf("surface tool %q input schema is required", spec.Name)
	}
	if spec.Route.OutputSchema == nil {
		return fmt.Errorf("surface tool %q output schema is required", spec.Name)
	}
	return nil
}

// ActionSpec projects a surface tool spec into the shared ActionSpec model.
func (spec SurfaceToolSpec) ActionSpec() (toolutil.ActionSpec, error) {
	if err := spec.Validate(); err != nil {
		return toolutil.ActionSpec{}, err
	}
	spec = CloneSurfaceToolSpec(spec)
	return toolutil.NewActionSpec(spec.ActionName, spec.Route, toolutil.ActionSpecOptions{
		Aliases:        spec.Aliases,
		Tags:           spec.Tags,
		Usage:          spec.Description,
		RelatedActions: spec.RelatedActions,
		Compatibility:  spec.Compatibility,
		ReadOnly:       spec.ReadOnly,
		Destructive:    spec.Destructive,
		Idempotent:     spec.Idempotent,
		OpenWorld:      spec.OpenWorld,
		OwnerPackage:   spec.OwnerPackage,
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        spec.Name,
			Title:       spec.Title,
			Description: spec.Description,
		},
	}), nil
}

// CloneSurfaceToolSpec returns a defensive copy of surface tool metadata.
func CloneSurfaceToolSpec(spec SurfaceToolSpec) SurfaceToolSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.GroupToolName = strings.TrimSpace(spec.GroupToolName)
	spec.BaseDomain = strings.TrimSpace(spec.BaseDomain)
	spec.ActionName = strings.TrimSpace(spec.ActionName)
	spec.OwnerPackage = strings.TrimSpace(spec.OwnerPackage)
	spec.SafeModePolicy = strings.TrimSpace(spec.SafeModePolicy)
	spec.ReadOnlyPolicy = strings.TrimSpace(spec.ReadOnlyPolicy)
	spec.Aliases = cloneNormalizedStrings(spec.Aliases)
	spec.Tags = cloneNormalizedStrings(spec.Tags)
	spec.RelatedActions = cloneNormalizedStrings(spec.RelatedActions)
	spec.Compatibility = toolutil.CloneCompatibilityPolicy(spec.Compatibility)
	spec.Icons = append([]mcp.Icon(nil), spec.Icons...)
	spec.CapabilityRequirements = cloneNormalizedStrings(spec.CapabilityRequirements)
	return spec
}
