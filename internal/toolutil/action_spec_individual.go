package toolutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// IndividualToolProjectionOptions contains surface-level metadata that is shared
// by an individual tool projection but not owned by the action spec itself.
type IndividualToolProjectionOptions struct {
	Description string
	Icons       []mcp.Icon
}

// IndividualToolFromSpecs projects the spec that owns an individual tool name.
func IndividualToolFromSpecs(specs []ActionSpec, individualName string, opts IndividualToolProjectionOptions) (*mcp.Tool, error) {
	name := strings.TrimSpace(individualName)
	if name == "" {
		return nil, errors.New("individual tool name is required")
	}
	var found *ActionSpec
	for index := range specs {
		if strings.TrimSpace(specs[index].IndividualTool.Name) != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("individual tool %q has multiple action specs", name)
		}
		found = &specs[index]
	}
	if found == nil {
		return nil, fmt.Errorf("individual tool %q action spec not found", name)
	}
	return IndividualToolFromActionSpec(*found, opts)
}

// MustIndividualToolFromSpecs projects an individual tool or panics on invalid
// registration metadata. Use it from catalog-backed startup paths where a
// missing spec is a programming error.
func MustIndividualToolFromSpecs(specs []ActionSpec, individualName string, opts IndividualToolProjectionOptions) *mcp.Tool {
	tool, err := IndividualToolFromSpecs(specs, individualName, opts)
	if err != nil {
		panic(err)
	}
	return tool
}

// IndividualToolFromActionSpec projects canonical action metadata into an MCP
// tool definition for the individual-tool surface.
func IndividualToolFromActionSpec(spec ActionSpec, opts IndividualToolProjectionOptions) (*mcp.Tool, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(spec.IndividualTool.Name)
	if name == "" {
		return nil, errors.New("individual tool name is required")
	}
	route := cloneActionRoute(spec.Route)
	if route.InputSchema == nil {
		return nil, fmt.Errorf("individual tool %q input schema is required", name)
	}
	if route.OutputSchema == nil {
		return nil, fmt.Errorf("individual tool %q output schema is required", name)
	}
	applyIndividualRequiredFields(route)
	route.InputSchema = enrichDestructiveSchema(route.InputSchema, route.Destructive)
	normalizeSchemaDescriptions(route.InputSchema)
	lockdownSchemaNode(route.InputSchema)
	title := strings.TrimSpace(spec.IndividualTool.Title)
	if title == "" {
		title = TitleFromName(name)
	}
	description := strings.TrimSpace(spec.IndividualTool.Description)
	if description == "" {
		description = strings.TrimSpace(opts.Description)
	}
	if description == "" {
		return nil, fmt.Errorf("individual tool %q description is required", name)
	}

	return &mcp.Tool{
		Name:         name,
		Title:        title,
		Description:  description,
		Annotations:  annotationsFromActionSpec(spec),
		InputSchema:  route.InputSchema,
		OutputSchema: route.OutputSchema,
		Icons:        append([]mcp.Icon(nil), opts.Icons...),
	}, nil
}

func applyIndividualRequiredFields(route ActionRoute) {
	if route.InputType == nil || route.InputSchema == nil {
		return
	}
	schema := schemaForType(route.InputType)
	required, ok := schema["required"]
	if !ok {
		delete(route.InputSchema, "required")
		return
	}
	route.InputSchema["required"] = cloneSchemaValue(required)
}

func annotationsFromActionSpec(spec ActionSpec) *mcp.ToolAnnotations {
	readOnly := spec.ReadOnly
	destructive := spec.Destructive
	idempotent := spec.Idempotent
	openWorld := spec.OpenWorld
	overrides := spec.IndividualTool.AnnotationOverrides
	if overrides.ReadOnly != nil {
		readOnly = *overrides.ReadOnly
	}
	if overrides.Destructive != nil {
		destructive = *overrides.Destructive
	}
	if overrides.Idempotent != nil {
		idempotent = *overrides.Idempotent
	}
	if overrides.OpenWorld != nil {
		openWorld = *overrides.OpenWorld
	}
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   &openWorld,
	}
}
