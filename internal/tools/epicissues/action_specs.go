package epicissues

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionEpicIssueList     = "group.epic_issue_list"
	actionEpicIssueAssign   = "group.epic_issue_assign"
	actionEpicIssueRemove   = "group.epic_issue_remove"
	actionEpicGet           = "group.epic_get"
	actionIssueGet          = "issue.get"
	paramFullPath           = "full_path"
	paramChildProjectPath   = "child_project_path"
	roleParentGroupPath     = "parent_group_path"
	hintGroupFullPath       = "Group full path that owns the epic."
	hintNotChildProjectPath = "Do not use the child project path as full_path."
)

// ActionSpecs returns canonical specs for epic issue hierarchy actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicIssueListSpec(client),
		epicIssueAssignSpec(client),
		epicIssueRemoveSpec(client),
		epicIssueUpdateSpec(client),
	}
}

// epicIssueListSpec builds the read-only spec for listing issues linked to an epic.
func epicIssueListSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := epicIssueBaseOptions("gitlab_epic_issue_list")
	options.Usage = "Use to list issues assigned to an epic. Send full_path for the epic group and epic_iid from the epic_create or epic_get result."
	options.Aliases = []string{"list epic issues", "show issues in epic", "epic child issues", "issues linked to epic"}
	options.RelatedActions = []string{actionEpicIssueAssign, actionEpicIssueRemove, "group.epic_issue_update", actionEpicGet, actionIssueGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramFullPath: {
			SemanticRole:     roleParentGroupPath,
			ValueSource:      hintGroupFullPath,
			CommonConfusions: []string{"Do not use a child project path as full_path. Epics live on groups."},
		},
		"epic_iid": {
			SemanticRole:   "epic_iid",
			ValueSource:    "Epic IID within the group, from epic_create or epic_get.",
			ExampleBinding: `params.epic_iid:5`,
		},
	}
	options.IndividualTool.Description = "List the issues linked to a group epic via the Work Items hierarchy. Returns: child issues with IID, title, state, author, labels, web URL, timestamps, and cursor pagination metadata. See also: gitlab_epic_issue_assign, gitlab_epic_issue_remove, gitlab_epic_get."
	return toolutil.NewReadActionSpec("epic_issue_list", toolutil.RouteAction(client, List), options)
}

// epicIssueAssignSpec builds the create spec for linking a project issue to an epic.
func epicIssueAssignSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := epicIssueBaseOptions("gitlab_epic_issue_assign")
	options.Usage = "Use to assign a project issue as a child of an epic owned by a group path. Send full_path for the epic group. Copy epic_iid from the epic_create or epic_get result and child_iid from the child issue result. Do not omit full_path or epic_iid after creating the epic."
	options.Aliases = []string{"assign issue to epic", "add issue to epic", "link issue to epic", "attach issue to epic"}
	options.RelatedActions = []string{actionEpicIssueList, actionEpicIssueRemove, actionEpicGet, actionIssueGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramFullPath: {
			SemanticRole:     roleParentGroupPath,
			ValueSource:      hintGroupFullPath,
			CommonConfusions: []string{hintNotChildProjectPath},
		},
		paramChildProjectPath: {
			SemanticRole:     paramChildProjectPath,
			ValueSource:      "Project path that owns the issue being assigned to the epic.",
			CommonConfusions: []string{"Do not use project_id or target_full_path for this parameter."},
		},
		"child_iid": {
			SemanticRole:     "child_issue_iid",
			ValueSource:      "Issue IID in child_project_path.",
			CommonConfusions: []string{"Do not use epic_iid as child_iid."},
		},
	}
	options.IndividualTool.Description = "Assign an existing project issue as a child of a group epic. Returns: the resolved epic and issue work item GIDs confirming the link. See also: gitlab_epic_issue_list, gitlab_epic_issue_remove, gitlab_epic_get."
	return toolutil.NewCreateActionSpec("epic_issue_assign", toolutil.RouteAction(client, Assign), options)
}

// epicIssueRemoveSpec builds the destructive spec for unlinking an issue from an epic.
func epicIssueRemoveSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := epicIssueBaseOptions("gitlab_epic_issue_remove")
	options.Usage = "Use to unlink a child issue from an epic. Send full_path for the epic group. Copy epic_iid from the epic_create or epic_get result and child_iid from the child issue result. Removal is destructive and requires confirmation."
	options.Aliases = []string{"remove issue from epic", "unlink issue from epic", "detach issue from epic"}
	options.RelatedActions = []string{actionEpicIssueList, actionEpicIssueAssign, actionEpicGet, actionIssueGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramFullPath: {
			SemanticRole:     roleParentGroupPath,
			ValueSource:      hintGroupFullPath,
			CommonConfusions: []string{hintNotChildProjectPath},
		},
		paramChildProjectPath: {
			SemanticRole:     paramChildProjectPath,
			ValueSource:      "Project path that owns the issue being removed from the epic.",
			CommonConfusions: []string{"Do not use project_id or target_full_path for this parameter."},
		},
		"child_iid": {
			SemanticRole:     "child_issue_iid",
			ValueSource:      "Issue IID in child_project_path.",
			CommonConfusions: []string{"Do not use epic_iid as child_iid."},
		},
	}
	options.IndividualTool.Description = "Unlink a child issue from a group epic (destructive. Requires confirmation). Returns: the resolved epic and issue work item GIDs confirming the removal. See also: gitlab_epic_issue_list, gitlab_epic_issue_assign, gitlab_epic_get."
	return toolutil.NewDeleteActionSpec("epic_issue_remove", toolutil.DestructiveAction(client, Remove), options)
}

// epicIssueUpdateSpec builds the update spec for reordering an issue within an epic.
func epicIssueUpdateSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := epicIssueBaseOptions("gitlab_epic_issue_update")
	options.Usage = "Use to reorder an issue within an epic by moving it before or after another linked issue. Send full_path for the epic group and epic_iid from the epic_create or epic_get result. child_id and adjacent_id are work item GIDs from the epic_issue_list output id field, and relative_position is BEFORE or AFTER."
	options.Aliases = []string{"reorder epic issue", "move issue within epic", "reposition epic issue", "change epic issue order"}
	options.RelatedActions = []string{actionEpicIssueList, actionEpicIssueAssign, actionEpicIssueRemove, actionEpicGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramFullPath: {
			SemanticRole:     roleParentGroupPath,
			ValueSource:      hintGroupFullPath,
			CommonConfusions: []string{hintNotChildProjectPath},
		},
		"child_id": {
			SemanticRole:     "child_work_item_gid",
			ValueSource:      "Work item GID of the issue to reorder, from the epic_issue_list output id field.",
			CommonConfusions: []string{"Do not use a numeric issue IID here. This must be a gid:// work item identifier."},
		},
		"adjacent_id": {
			SemanticRole:     "reference_work_item_gid",
			ValueSource:      "Work item GID of the issue to position relative to, from the epic_issue_list output id field.",
			CommonConfusions: []string{"Do not use a numeric issue IID. This must be a gid:// work item identifier already linked to the epic."},
		},
		"relative_position": {
			SemanticRole:   "relative_position",
			ValueSource:    "BEFORE or AFTER, relative to adjacent_id.",
			ExampleBinding: `params.relative_position:"BEFORE"`,
		},
	}
	// The GraphQL RelativePositionType enum behind the reorder mutation has
	// exactly these two values, and the handler refuses anything else.
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaEnumOverride("relative_position", "BEFORE", "AFTER"),
	}
	options.IndividualTool.Description = "Reorder an issue within a group epic by moving it before or after another linked issue. Returns: the epic's child issues in their updated order. See also: gitlab_epic_issue_list, gitlab_epic_issue_assign, gitlab_epic_issue_remove."
	return toolutil.NewUpdateActionSpec("epic_issue_update", toolutil.RouteAction(client, UpdateOrder), options)
}

// epicIssueBaseOptions returns the shared option scaffold for an epic-issue
// individual tool, leaving per-action Usage, Aliases, RelatedActions,
// ParameterGuidance, and IndividualTool.Description to the caller.
func epicIssueBaseOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "epic", "issue"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicissues",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
