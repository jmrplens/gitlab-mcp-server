package pipelinetriggers

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for pipeline trigger actions. The catalog projects
// these actions under the gitlab_pipeline group, so the canonical domain is
// "pipeline" and the action names carry the "trigger_" prefix.
const (
	actionTriggerList   = "pipeline.trigger_list"
	actionTriggerGet    = "pipeline.trigger_get"
	actionTriggerCreate = "pipeline.trigger_create"
	actionTriggerUpdate = "pipeline.trigger_update"
	actionTriggerDelete = "pipeline.trigger_delete"
	actionTriggerRun    = "pipeline.trigger_run"
	actionPipelineGet   = "pipeline.get"
)

// ActionSpecs returns canonical specs for pipeline trigger actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineTriggerReadSpec("trigger_list", toolutil.RouteAction(client, ListTriggers), "gitlab_pipeline_trigger_list"),
		pipelineTriggerReadSpec("trigger_get", toolutil.RouteAction(client, GetTrigger), "gitlab_pipeline_trigger_get"),
		pipelineTriggerCreateSpec("trigger_create", toolutil.RouteAction(client, CreateTrigger), "gitlab_pipeline_trigger_create"),
		pipelineTriggerUpdateSpec("trigger_update", toolutil.RouteAction(client, UpdateTrigger), "gitlab_pipeline_trigger_update"),
		pipelineTriggerDeleteSpec("trigger_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_pipeline_trigger_delete"),
		pipelineTriggerCreateSpec("trigger_run", toolutil.RouteAction(client, RunTrigger), "gitlab_pipeline_trigger_run"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTrigger(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pipeline trigger")
	return out, nil
}

func pipelineTriggerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineTriggerOptions(individualTool)
	decoratePipelineTriggerMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func pipelineTriggerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineTriggerOptions(individualTool)
	decoratePipelineTriggerMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func pipelineTriggerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineTriggerOptions(individualTool)
	decoratePipelineTriggerMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func pipelineTriggerDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineTriggerOptions(individualTool)
	decoratePipelineTriggerMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func pipelineTriggerOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute pipelinetriggers domain action.", Tags: []string{"ci", "pipeline", "trigger"},
		OpenWorld:      true,
		OwnerPackage:   "pipelinetriggers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// pipelineTriggerActionMetaEntry is the discovery metadata for one pipeline
// trigger action (R-META: action-specific Usage, natural-language Aliases,
// canonical RelatedActions, optional ParameterGuidance, and the
// "Returns: … See also: …" individual-tool description).
type pipelineTriggerActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

// pipelineTriggerActionMeta maps each individual pipeline trigger tool to its
// discovery metadata, replacing the generic placeholder Usage/Aliases.
var pipelineTriggerActionMeta = map[string]pipelineTriggerActionMetaEntry{
	"gitlab_pipeline_trigger_list": {
		usage:   "List the trigger tokens defined on a project. Use when the prompt asks which trigger tokens exist, or to find a trigger_id or token to run or edit. Requires Maintainer+ to read tokens; supports order_by, sort, offset and keyset pagination.",
		aliases: []string{"list pipeline triggers", "show trigger tokens", "list trigger tokens for project"},
		related: []string{actionTriggerGet, actionTriggerCreate, actionTriggerRun},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or full namespace path whose trigger tokens should be listed.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Trigger tokens are project-scoped; do not pass a group path."},
			},
		},
		description: "List a project's pipeline trigger tokens. Returns: trigger tokens with id, description, owner, last-used time, and pagination metadata. See also: gitlab_pipeline_trigger_get, gitlab_pipeline_trigger_create, gitlab_pipeline_trigger_run.",
	},
	"gitlab_pipeline_trigger_get": {
		usage:   "Get one pipeline trigger token by project_id plus trigger_id. Use after a list result or when the trigger_id is already known.",
		aliases: []string{"get pipeline trigger", "show trigger token details", "fetch trigger token"},
		related: []string{actionTriggerList, actionTriggerUpdate, actionTriggerDelete},
		guidance: map[string]toolutil.ParameterGuidance{
			"trigger_id": {
				SemanticRole:     "trigger_id",
				ValueSource:      "Numeric trigger token id, usually from a prior pipeline_trigger.list result.",
				ExampleBinding:   "params.trigger_id:10",
				CommonConfusions: []string{"trigger_id is the token's numeric id, not the token secret string."},
			},
		},
		description: "Get a single pipeline trigger token. Returns: the trigger with id, description, token, owner, and timestamps. See also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_update, gitlab_pipeline_trigger_delete.",
	},
	"gitlab_pipeline_trigger_create": {
		usage:   "Create a new pipeline trigger token on a project so external systems can start pipelines. Provide project_id and a clear description. Requires Maintainer+.",
		aliases: []string{"create pipeline trigger", "add trigger token", "generate trigger token"},
		related: []string{actionTriggerList, actionTriggerRun, actionTriggerDelete},
		guidance: map[string]toolutil.ParameterGuidance{
			"description": {
				SemanticRole:   "trigger_description",
				ValueSource:    "Human-readable label describing what the token is for.",
				ExampleBinding: `params.description:"nightly deploy"`,
			},
		},
		description: "Create a pipeline trigger token. Returns: the created trigger with id, description, token secret, and owner. See also: gitlab_pipeline_trigger_run, gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_delete.",
	},
	"gitlab_pipeline_trigger_update": {
		usage:       "Update a pipeline trigger token's description. Use when renaming or re-labeling an existing token; the token secret is unchanged.",
		aliases:     []string{"update pipeline trigger", "rename trigger token", "edit trigger description"},
		related:     []string{actionTriggerGet, actionTriggerList, actionTriggerDelete},
		description: "Update a pipeline trigger token's description. Returns: the updated trigger with id, description, token, and owner. See also: gitlab_pipeline_trigger_get, gitlab_pipeline_trigger_delete.",
	},
	"gitlab_pipeline_trigger_delete": {
		usage:       "Permanently delete a pipeline trigger token. Destructive and irreversible; the token is invalidated immediately. Confirm project_id and trigger_id before calling.",
		aliases:     []string{"delete pipeline trigger", "revoke trigger token", "remove trigger token"},
		related:     []string{actionTriggerGet, actionTriggerList, actionTriggerCreate},
		description: "Delete a pipeline trigger token. Returns: a success confirmation. See also: gitlab_pipeline_trigger_get, gitlab_pipeline_trigger_create.",
	},
	"gitlab_pipeline_trigger_run": {
		usage:   "Trigger a pipeline on a ref using a trigger token. Provide project_id, ref, and token; optionally pass variables (CI/CD variables) or inputs (pipeline spec inputs).",
		aliases: []string{"run pipeline trigger", "trigger a pipeline with a token", "start pipeline via trigger"},
		related: []string{actionTriggerList, actionTriggerGet, actionPipelineGet},
		guidance: map[string]toolutil.ParameterGuidance{
			"ref": {
				SemanticRole:     "git_ref",
				ValueSource:      "Branch or tag name the pipeline should run on.",
				ExampleBinding:   `params.ref:"main"`,
				CommonConfusions: []string{"ref must be an existing branch or tag, not a commit SHA or MR id."},
			},
			"token": {
				SemanticRole:     "trigger_token",
				ValueSource:      "Trigger token secret string from pipeline_trigger.create or pipeline_trigger.list.",
				ExampleBinding:   `params.token:"glptt-..."`,
				CommonConfusions: []string{"token is the secret string, not the numeric trigger_id."},
			},
		},
		description: "Trigger a pipeline using a trigger token. Returns: the created pipeline with id, status, ref, sha, source, user, detailed_status, and web URL. See also: gitlab_pipeline_trigger_list, gitlab_pipeline_get.",
	},
}

// decoratePipelineTriggerMeta fills non-generic Usage, natural-language
// Aliases, canonical RelatedActions, ParameterGuidance, and the
// "Returns: … See also: …" individual-tool description for a pipeline trigger
// action from pipelineTriggerActionMeta.
func decoratePipelineTriggerMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := pipelineTriggerActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if len(meta.guidance) > 0 {
		options.ParameterGuidance = meta.guidance
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}
