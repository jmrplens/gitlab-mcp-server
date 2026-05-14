package tools

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projecttemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
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
	return []actionSpecGroupBuilder{
		buildAccessActionSpecs,
		buildBranchActionSpecs,
		buildCustomEmojiActionSpecs,
		buildGroupActionSpecs,
		buildIssueActionSpecs,
		buildJobActionSpecs,
		buildMergeRequestActionSpecs,
		buildTagActionSpecs,
		buildTemplateActionSpecs,
		buildWikiActionSpecs,
	}
}

func buildAccessActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_access", deploytokens.ActionSpecs(client))
}

func buildBranchActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := append(branches.ActionSpecs(client), branchrules.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_branch", specs)
}

func buildCustomEmojiActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_custom_emoji", customemoji.ActionSpecs(client))
}

func buildGroupActionSpecs(client *gitlabclient.Client, enterprise bool) []ActionSpecGroup {
	if !enterprise {
		return nil
	}
	return actionSpecGroup("gitlab_group", epicissues.ActionSpecs(client))
}

func buildIssueActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_issue", issuelinks.ActionSpecs(client))
}

func buildJobActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_job", jobtokenscope.ActionSpecs(client))
}

func buildMergeRequestActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_merge_request", mergerequests.ActionSpecs(client))
}

func buildTagActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_tag", tags.ActionSpecs(client))
}

func buildTemplateActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	specs := make([]toolutil.ActionSpec, 0, 12)
	specs = append(specs, cilint.ActionSpecs(client)...)
	specs = append(specs, ciyamltemplates.ActionSpecs(client)...)
	specs = append(specs, dockerfiletemplates.ActionSpecs(client)...)
	specs = append(specs, gitignoretemplates.ActionSpecs(client)...)
	specs = append(specs, licensetemplates.ActionSpecs(client)...)
	specs = append(specs, projecttemplates.ActionSpecs(client)...)
	return actionSpecGroup("gitlab_template", specs)
}

func buildWikiActionSpecs(client *gitlabclient.Client, _ bool) []ActionSpecGroup {
	return actionSpecGroup("gitlab_wiki", wikis.ActionSpecs(client))
}

func actionSpecGroup(toolName string, specs []toolutil.ActionSpec) []ActionSpecGroup {
	if len(specs) == 0 {
		return nil
	}
	return []ActionSpecGroup{{ToolName: toolName, Specs: specs}}
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
