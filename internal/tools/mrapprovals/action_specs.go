package mrapprovals

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request approval actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// Per-MR approval *rule* endpoints (/approval_state, /approval_rules) are
		// Premium/Ultimate; the basic approval-state flow (/approvals config,
		// reset_approvals) is Free. See the merge_request_approvals API docs.
		approvalPremiumSpec(approvalReadSpec("approval_state", toolutil.RouteAction(client, State), "gitlab_mr_approval_state")),
		approvalPremiumSpec(approvalReadSpec("approval_rules", toolutil.RouteAction(client, Rules), "gitlab_mr_approval_rules")),
		approvalReadSpec("approval_config", toolutil.RouteAction(client, Config), "gitlab_mr_approval_config"),
		approvalResetSpec(client),
		approvalPremiumSpec(approvalCreateSpec("approval_rule_create", toolutil.RouteAction(client, CreateRule), "gitlab_mr_approval_rule_create")),
		approvalPremiumSpec(approvalUpdateSpec("approval_rule_update", toolutil.RouteAction(client, UpdateRule), "gitlab_mr_approval_rule_update")),
		approvalPremiumSpec(approvalDeleteSpec("approval_rule_delete", toolutil.DestructiveAction(client, deleteRuleOutput), "gitlab_mr_approval_rule_delete")),
	}
}

func resetOutput(ctx context.Context, client *gitlabclient.Client, input ResetInput) (toolutil.DeleteOutput, error) {
	if err := Reset(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("MR approvals")
	return out, nil
}

func deleteRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("approval rule")
	return out, nil
}

// Canonical action IDs referenced as RelatedActions. MR approval actions are
// projected into the gitlab_merge_request catalog group, so their canonical IDs
// share the merge_request domain prefix.
const (
	actionApprovalState      = "merge_request.approval_state"
	actionApprovalRules      = "merge_request.approval_rules"
	actionApprovalConfig     = "merge_request.approval_config"
	actionApprovalReset      = "merge_request.approval_reset"
	actionApprovalRuleCreate = "merge_request.approval_rule_create"
	actionApprovalRuleUpdate = "merge_request.approval_rule_update"
	actionApprovalRuleDelete = "merge_request.approval_rule_delete"
	actionMRGet              = "merge_request.get"
	actionMRApprove          = "merge_request.approve"
	actionMRUnapprove        = "merge_request.unapprove"
)

// approvalPremiumSpec marks an MR approval action as Premium/Ultimate so the
// individual catalog hides it from CE clients. Merge-request approval *rules*
// (listing, per-rule approval state, create/update/delete) are a GitLab Premium
// feature; the basic approval-state config and reset endpoints remain Free.
func approvalPremiumSpec(spec toolutil.ActionSpec) toolutil.ActionSpec {
	spec.Edition = "premium"
	return spec
}

func approvalReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	decorateApprovalMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func approvalCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	decorateApprovalMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func approvalUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	decorateApprovalMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func approvalDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	decorateApprovalMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func approvalResetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := approvalOptions("gitlab_mr_approval_reset")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	decorateApprovalMeta(&options, "gitlab_mr_approval_reset")
	return toolutil.NewDeleteActionSpec("approval_reset", toolutil.DestructiveAction(client, resetOutput), options)
}

func approvalOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mrapprovals domain action.", Tags: []string{"merge_request", "approval"},
		OpenWorld:      true,
		OwnerPackage:   "mrapprovals",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// approvalActionMetaEntry is the discovery metadata for one MR approval action.
type approvalActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

// projectScopeGuidance is the shared parameter guidance for the project_id
// argument carried by every MR approval action.
var projectScopeGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "scope_project",
	ValueSource:      "Project ID or full namespace path that owns the merge request.",
	ExampleBinding:   `params.project_id:"group/project"`,
	CommonConfusions: []string{"Use the MR's parent project here, not a group path or the merge request's global ID."},
}

// mrIIDGuidance is the shared parameter guidance for the merge_request_iid
// argument carried by every MR approval action.
var mrIIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "merge_request_iid",
	ValueSource:      "Merge request number visible in the project, usually from the URL or prior MR list output.",
	ExampleBinding:   "params.merge_request_iid:42",
	CommonConfusions: []string{"Use the project-scoped merge_request_iid, not the merge request's global database ID."},
}

// ruleIDGuidance is the shared parameter guidance for the approval_rule_id
// argument carried by the rule update and delete actions.
var ruleIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "approval_rule_id",
	ValueSource:      "Approval rule ID from a prior gitlab_mr_approval_rules listing.",
	ExampleBinding:   "params.approval_rule_id:5",
	CommonConfusions: []string{"This is the MR-level approval rule ID, not the project-level approval rule ID or the merge request IID."},
}

// approvalActionMeta maps each individual MR approval tool to its discovery
// metadata (non-generic Usage, natural-language Aliases, canonical
// RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description). It satisfies the 1:1 audit R-META requirement.
var approvalActionMeta = map[string]approvalActionMetaEntry{
	"gitlab_mr_approval_state": {
		usage:   "Read the overall approval state of a merge request, including whether the project rules were overridden and the per-rule approval status. Use when checking whether an MR has met its approval requirements before merge.",
		aliases: []string{"merge request approval state", "is this mr approved", "show mr approval status"},
		related: []string{actionApprovalRules, actionApprovalConfig, actionMRApprove},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
		description: "Get the approval state of a merge request. Returns: whether project rules were overwritten and each applicable rule with its type, required count, approved flag, and approvers. See also: gitlab_mr_approval_rules, gitlab_mr_approval_config, gitlab_mr_approve.",
	},
	"gitlab_mr_approval_rules": {
		usage:   "List the approval rules configured on a merge request, including eligible approvers, assigned users, and groups. Use when inspecting or before editing the rules that gate an MR.",
		aliases: []string{"list mr approval rules", "show merge request approval rules", "merge request approvers"},
		related: []string{actionApprovalRuleCreate, actionApprovalRuleUpdate, actionApprovalState},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
		description: "List the approval rules of a merge request. Returns: each rule with its type, required count, approved flag, eligible approvers, users, groups, and source rule. See also: gitlab_mr_approval_rule_create, gitlab_mr_approval_rule_update, gitlab_mr_approval_state.",
	},
	"gitlab_mr_approval_config": {
		usage:   "Read the approval configuration of a merge request: approvals required and left, current approvers, suggested approvers, and approver groups. Use for a configuration overview rather than the per-rule state.",
		aliases: []string{"mr approval configuration", "merge request approvals required", "who has approved this mr"},
		related: []string{actionApprovalState, actionApprovalRules, actionMRApprove},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
		description: "Get the approval configuration of a merge request. Returns: approvals required and left, approved-by users with timestamps, suggested approvers, approver groups, and the remaining approval rules. See also: gitlab_mr_approval_state, gitlab_mr_approval_rules, gitlab_mr_approve.",
	},
	"gitlab_mr_approval_reset": {
		usage:   "Reset (clear) all existing approvals on a merge request. Requires a project or group access token (bot user); personal access tokens are rejected. Use when approvals must be re-collected after changes.",
		aliases: []string{"reset mr approvals", "clear merge request approvals", "remove all mr approvals"},
		related: []string{actionApprovalState, actionApprovalConfig, actionMRUnapprove},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
		description: "Reset all approvals on a merge request. Returns: a success confirmation. See also: gitlab_mr_approval_state, gitlab_mr_approval_config, gitlab_mr_unapprove.",
	},
	"gitlab_mr_approval_rule_create": {
		usage:   "Create a new approval rule on a merge request, naming the rule and the number of approvals required, optionally scoping it to specific users or groups. Requires Maintainer and Premium/Ultimate.",
		aliases: []string{"add mr approval rule", "create merge request approval rule", "require approvers on mr"},
		related: []string{actionApprovalRules, actionApprovalRuleUpdate, actionApprovalRuleDelete},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
			"name": {
				SemanticRole:   "approval_rule_name",
				ValueSource:    "Short descriptive name for the new approval rule.",
				ExampleBinding: `params.name:"Security review"`,
			},
			"approvals_required": {
				SemanticRole:     "approvals_required",
				ValueSource:      "Number of approvals the rule requires before the MR can merge.",
				ExampleBinding:   "params.approvals_required:2",
				CommonConfusions: []string{"This is the count for this single rule, not the project-wide minimum."},
			},
		},
		description: "Create an approval rule on a merge request. Returns: the created rule with its type, required count, eligible approvers, users, and groups. See also: gitlab_mr_approval_rules, gitlab_mr_approval_rule_update, gitlab_mr_approval_rule_delete.",
	},
	"gitlab_mr_approval_rule_update": {
		usage:   "Update an existing approval rule on a merge request: change its name, required approval count, or assigned users and groups. The rule_type cannot be changed after creation.",
		aliases: []string{"update mr approval rule", "edit merge request approval rule", "change required approvers"},
		related: []string{actionApprovalRules, actionApprovalRuleCreate, actionApprovalRuleDelete},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
			"approval_rule_id":  ruleIDGuidance,
		},
		description: "Update an approval rule on a merge request. Returns: the updated rule with its type, required count, eligible approvers, users, and groups. See also: gitlab_mr_approval_rules, gitlab_mr_approval_rule_create, gitlab_mr_approval_rule_delete.",
	},
	"gitlab_mr_approval_rule_delete": {
		usage:   "Delete an approval rule from a merge request. Destructive and irreversible; confirm the approval_rule_id with gitlab_mr_approval_rules before calling.",
		aliases: []string{"delete mr approval rule", "remove merge request approval rule", "destroy mr approval rule", "drop merge request approval rule"},
		related: []string{actionApprovalRules, actionApprovalRuleUpdate, actionApprovalRuleCreate},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectScopeGuidance,
			"merge_request_iid": mrIIDGuidance,
			"approval_rule_id":  ruleIDGuidance,
		},
		description: "Delete an approval rule from a merge request. Returns: a success confirmation. See also: gitlab_mr_approval_rules, gitlab_mr_approval_rule_update, gitlab_mr_approval_rule_create.",
	},
}

// decorateApprovalMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, ParameterGuidance, and the individual-tool
// description for an MR approval action from approvalActionMeta. It is a no-op
// for individual tools without a metadata entry.
func decorateApprovalMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := approvalActionMeta[individualTool]
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
