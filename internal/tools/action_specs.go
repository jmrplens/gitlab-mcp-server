package tools

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecGroup contains specs owned by one meta-tool group.
type ActionSpecGroup struct {
	ToolName string
	Specs    []toolutil.ActionSpec
}

type actionSpecGroupBuilder func(*gitlabclient.Client, bool) []ActionSpecGroup

// CollectActionSpecs gathers canonical specs from domain-local builders.
func CollectActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	groups := make([]ActionSpecGroup, 0)
	for _, build := range actionSpecGroupBuilders() {
		groups = append(groups, build(client, enterprise)...)
	}
	return cloneSortedActionSpecGroups(groups)
}

func actionSpecGroupBuilders() []actionSpecGroupBuilder {
	return nil
}

func actionSpecGroupsByTool(groups []ActionSpecGroup) (map[string][]toolutil.ActionSpec, error) {
	byTool := make(map[string][]toolutil.ActionSpec, len(groups))
	var errs []error
	for _, group := range groups {
		toolName := strings.TrimSpace(group.ToolName)
		if toolName == "" {
			errs = append(errs, errors.New("action spec group tool name is required"))
			continue
		}
		byTool[toolName] = append(byTool[toolName], cloneActionSpecs(group.Specs)...)
	}
	for toolName, specs := range byTool {
		seen := make(map[string]struct{}, len(specs))
		for _, spec := range specs {
			name := strings.TrimSpace(spec.Name)
			if name == "" {
				errs = append(errs, fmt.Errorf("%s: action spec name is required", toolName))
				continue
			}
			if _, exists := seen[name]; exists {
				errs = append(errs, fmt.Errorf("%s: duplicate action spec %q", toolName, name))
				continue
			}
			seen[name] = struct{}{}
		}
		sort.SliceStable(specs, func(left, right int) bool {
			return specs[left].Name < specs[right].Name
		})
		byTool[toolName] = specs
	}
	return byTool, errors.Join(errs...)
}

func cloneSortedActionSpecGroups(groups []ActionSpecGroup) []ActionSpecGroup {
	if len(groups) == 0 {
		return nil
	}
	out := make([]ActionSpecGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, ActionSpecGroup{ToolName: strings.TrimSpace(group.ToolName), Specs: cloneActionSpecs(group.Specs)})
	}
	sort.SliceStable(out, func(left, right int) bool {
		return out[left].ToolName < out[right].ToolName
	})
	return out
}

func cloneActionSpecs(specs []toolutil.ActionSpec) []toolutil.ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	out := make([]toolutil.ActionSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, toolutil.NewActionSpec(spec.Name, spec.Route, toolutil.ActionSpecOptions{
			Aliases:                spec.Aliases,
			Tags:                   spec.Tags,
			Usage:                  spec.Usage,
			RelatedActions:         spec.RelatedActions,
			ParameterGuidance:      spec.ParameterGuidance,
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
			RuntimeValidationNotes: spec.RuntimeValidationNotes,
		}))
	}
	return out
}
