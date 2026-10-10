package required

import "fmt"

// declaration answers one disagreement this scope finds, which the schema
// keeps on purpose.
type declaration struct {
	// Direction is the disagreement it answers, so a declaration written for a
	// field the schema required cannot quietly answer the field the day the
	// schema stops requiring it and GitLab starts to.
	Direction string
	// Category says what kind of answer this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it
	// still holds.
	Reason string
}

// Declaration categories. Each names why the schema may say something about a
// field that GitLab's declaration of the parameter does not.
const (
	// categoryConditional is a parameter GitLab requires only under a
	// condition, in a Grape `given` block, which the record writes as an
	// unconditional requirement.
	categoryConditional = "conditional"
	// categoryHandlerDefault is a parameter GitLab requires and the handler
	// supplies when the caller leaves it out.
	categoryHandlerDefault = "handler-default"
	// categoryOneAlternative is a parameter GitLab takes as one of several
	// alternatives, of which this action can send only this one.
	categoryOneAlternative = "one-alternative"
	// categorySoleAttribute is a parameter GitLab leaves optional on a route
	// whose every other attribute the action does not send, so a call
	// without it changes nothing.
	categorySoleAttribute = "sole-attribute"
	// categorySDKPositional is a parameter client-go takes as a positional
	// argument and writes into the path whatever its value, so the call
	// cannot leave it out.
	categorySDKPositional = "sdk-positional"
	// categoryModelRequires is a parameter Grape leaves optional and the
	// model the route builds refuses to save without, so a call without it is
	// refused all the same, one step later.
	categoryModelRequires = "model-requires"
	// categorySDKSendsNull is a parameter GitLab leaves optional that
	// client-go writes as null when the handler leaves it unset, on a route
	// that reads the null otherwise than the key left out, so the handler
	// always sends a value and the schema asks for one.
	categorySDKSendsNull = "sdk-sends-null"
)

// declaredRequiredness holds every field whose requiredness the schema keeps
// apart from GitLab's on purpose, keyed by action and field. Every entry is a
// claim about the handler or about GitLab that the record cannot see, which
// is what a reviewer has to check; a declaration that answers nothing is
// itself a finding, so the table cannot outlive the disagreement it answers.
func declaredRequiredness() map[declarationKey]declaration {
	// Reasons shared by more than one declaration. They are local so the
	// coverage profile, which marks function bodies only, sees them.
	const (
		reasonNoteBody = "PUT .../notes/:note_id declares body optional beside confidential, " +
			"which lib/api/notes.rb describes as no longer allowed to update a note, so body is the one " +
			"attribute the update changes and a call without it changes nothing"
		reasonDiscussionNoteBody = "PUT .../discussions/:discussion_id/notes/:note_id takes exactly one of " +
			"body and resolved (lib/api/discussions.rb); client-go's update options for this route carry " +
			"body and no resolved, so body is the one this action sends"
		reasonLabelPath = "client-go's label update and delete put any non-nil label argument in the path and " +
			"take the body route only for nil; this handler always passes label_id, so it sends " +
			"PUT /groups/:id/labels/:name alone, on which GitLab requires the label, and label_id " +
			"already takes a name as well as an ID"
		reasonSnippetFileName = "lib/api/helpers/snippets_helpers.rb requires file_name only in a given :content " +
			"block; the record writes it as an unconditional requirement, and a snippet created " +
			"from files needs none"
		reasonRunnerScope = "lib/api/user_runners.rb requires group_id only when runner_type is group_type and " +
			"project_id only when it is project_type, in given blocks the record writes as " +
			"unconditional requirements; an instance runner needs neither"
		reasonCommitCommentLine = "lib/api/commits.rb requires line and line_type only in a given :path block, " +
			"and line_type then defaults to new; a comment on the whole commit takes neither"
		reasonSubmoduleRef = "GET .../repository/files/:file_path requires ref; the handler sends HEAD when the " +
			"caller gives none, which is the default branch the schema promises"
	)
	return map[declarationKey]declaration{
		// Notes and discussion notes: the one attribute the update changes.
		{action: "issue.note_update", field: "body"}:                        {DirectionGitLabOptional, categorySoleAttribute, reasonNoteBody},
		{action: "mr_review.note_update", field: "body"}:                    {DirectionGitLabOptional, categorySoleAttribute, reasonNoteBody},
		{action: "snippet.note_update", field: "body"}:                      {DirectionGitLabOptional, categorySoleAttribute, reasonNoteBody},
		{action: "issue.discussion_update_note", field: "body"}:             {DirectionGitLabOptional, categoryOneAlternative, reasonDiscussionNoteBody},
		{action: "repository.commit_discussion_update_note", field: "body"}: {DirectionGitLabOptional, categoryOneAlternative, reasonDiscussionNoteBody},
		{action: "snippet.discussion_update_note", field: "body"}:           {DirectionGitLabOptional, categoryOneAlternative, reasonDiscussionNoteBody},

		// Settings routes whose one attribute this action sends.
		{action: "pipeline.resource_group_edit", field: "process_mode"}: {
			DirectionGitLabOptional, categorySoleAttribute,
			"PUT /projects/:id/resource_groups/:key declares process_mode as its only attribute, so a call without it changes nothing",
		},
		{action: "project.security_settings_update", field: "secret_push_protection_enabled"}: {
			DirectionGitLabOptional, categoryOneAlternative,
			"ee/lib/api/project_security_settings.rb takes at least one of secret_push_protection_enabled, " +
				"pre_receive_secret_detection_enabled (its older spelling) and fast_dependency_paths_enabled; " +
				"client-go's options carry the first alone, so it is the one this action sends",
		},

		// Alternatives of which client-go carries one.
		{action: "group.group_board_create_list", field: "label_id"}: {
			DirectionGitLabOptional, categoryOneAlternative,
			"CE requires label_id and EE takes exactly one of label_id, milestone_id, iteration_id and assignee_id " +
				"(ee/lib/ee/api/boards_responses.rb); client-go's group board list options carry label_id alone",
		},
		{action: "user.create_current_user_pat", field: "scopes"}: {
			DirectionGitLabOptional, categoryOneAlternative,
			"POST /user/personal_access_tokens takes exactly one of scopes and granular_scopes, the second behind the " +
				"granular_personal_access_tokens feature flag; client-go has no granular_scopes, so scopes is the one this action sends",
		},
		{action: "user.create_impersonation_token", field: "scopes"}: {
			DirectionGitLabOptional, categoryModelRequires,
			"POST /users/:user_id/impersonation_tokens declares scopes optional beside granular_scopes, which client-go does not " +
				"carry, and PersonalAccessToken validates the presence of scopes, so a token created without them is refused",
		},
		{action: "group.group_label_update", field: "label_id"}: {DirectionGitLabOptional, categorySDKPositional, reasonLabelPath},
		{action: "merge_request.dependency_create", field: "blocking_merge_request_id"}: {
			DirectionGitLabOptional, categoryOneAlternative,
			"POST /projects/:id/merge_requests/:merge_request_iid/blocks takes exactly one of blocking_merge_request_id and " +
				"blocking_merge_request_iid (ee/lib/api/merge_request_dependencies.rb); client-go's " +
				"CreateMergeRequestDependencyOptions carries the first alone, so it is the one this action sends",
		},

		// A null client-go writes in place of a key left out.
		{action: "pipeline.schedule_edit_variable", field: "value"}: {
			DirectionGitLabOptional, categorySDKSendsNull,
			"PUT .../pipeline_schedules/:pipeline_schedule_id/variables/:key declares value optional, and " +
				"client-go's EditPipelineScheduleVariableOptions writes it with no omitempty, so a call without " +
				"one sends null, which lib/api/ci/pipeline_schedules.rb passes on through declared_params and the " +
				"save service assigns over the stored value; the handler therefore always sends a value " +
				"(docs/development/upstream-bugs.md, entry 54)",
		},

		// A path segment client-go always writes.
		{action: "model_registry.download", field: "path"}: {
			DirectionGitLabOptional, categorySDKPositional,
			"GitLab's route writes the path as an optional group, (*path/):file_name, and client-go's " +
				"DownloadMachineLearningModelPackage writes the path argument between two slashes whatever its value, " +
				"so the action cannot spell the route without it",
		},

		// Requirements GitLab states inside a given block.
		{action: "repository.commit_comment_create", field: "line"}:      {DirectionGitLabRequires, categoryConditional, reasonCommitCommentLine},
		{action: "repository.commit_comment_create", field: "line_type"}: {DirectionGitLabRequires, categoryConditional, reasonCommitCommentLine},
		{action: "snippet.create", field: "file_name"}:                   {DirectionGitLabRequires, categoryConditional, reasonSnippetFileName},
		{action: "snippet.project_create", field: "file_name"}:           {DirectionGitLabRequires, categoryConditional, reasonSnippetFileName},
		{action: "user.create_runner", field: "group_id"}:                {DirectionGitLabRequires, categoryConditional, reasonRunnerScope},
		{action: "user.create_runner", field: "project_id"}:              {DirectionGitLabRequires, categoryConditional, reasonRunnerScope},

		// Requirements the handler meets for a caller who leaves the field out.
		{action: "snippet.project_create", field: "visibility"}: {
			DirectionGitLabRequires, categoryHandlerDefault,
			"lib/api/project_snippets.rb requires visibility; the handler sends private when the caller gives none, as the schema says",
		},
		{action: "repository.list_submodules", field: "ref"}:     {DirectionGitLabRequires, categoryHandlerDefault, reasonSubmoduleRef},
		{action: "repository.read_submodule_file", field: "ref"}: {DirectionGitLabRequires, categoryHandlerDefault, reasonSubmoduleRef},
	}
}

// aliasKey names the GitLab parameter of an action an alias places.
type aliasKey struct {
	action string
	param  string
}

// pathAlias names the field an action fills a GitLab parameter from, where the
// placement rules ([placements]) would find another.
type pathAlias struct {
	// Field is the input field that fills the parameter.
	Field string
	// Reason says why the rules cannot find it.
	Reason string
}

// declaredAliases holds every GitLab parameter an action fills from a field
// the placement rules do not find it under. The usual case is a path
// parameter whose GitLab name is also the name of a separate option the input
// publishes: GitLab writes the protected branch of
// /protected_branches/:name as `name`, and the input fills it from `branch`
// because its own `name` is the rename client-go's options carry. Without
// the alias the join holds the rename to the path parameter's requiredness.
func declaredAliases() map[aliasKey]pathAlias {
	const renameBesidePath = "GitLab's path parameter :name is the object the update addresses; the input " +
		"names it %s because its own name field carries client-go's option of that name"
	return map[aliasKey]pathAlias{
		{action: "branch.update_protected", param: "name"}: {
			Field:  "branch_name",
			Reason: fmt.Sprintf(renameBesidePath, "branch_name"),
		},
		{action: "group.protected_branch_update", param: "name"}: {
			Field:  "branch",
			Reason: fmt.Sprintf(renameBesidePath, "branch"),
		},
		{action: "environment.protected_update", param: "name"}: {
			Field:  "environment",
			Reason: fmt.Sprintf(renameBesidePath, "environment"),
		},
		{action: "group.protected_env_update", param: "name"}: {
			Field:  "environment",
			Reason: fmt.Sprintf(renameBesidePath, "environment"),
		},
		{action: "feature_flags.feature_flag_update", param: "feature_flag_name"}: {
			Field:  "name",
			Reason: "GitLab names the flag the path addresses feature_flag_name and its new name name; the input calls them name and new_name",
		},
		{action: "feature_flags.feature_flag_update", param: "name"}: {
			Field:  "new_name",
			Reason: "the new name GitLab calls name is the input's new_name, its name being the flag the path addresses",
		},
		{action: "group.service_account_pat_list", param: "user_id"}: {
			Field:  "service_account_id",
			Reason: "GitLab's path parameter :user_id is the service account, which the input names service_account_id",
		},
		{action: "project.service_account_pat_list", param: "user_id"}: {
			Field:  "service_account_id",
			Reason: "GitLab's path parameter :user_id is the service account, which the input names service_account_id",
		},
		{action: "template.project_template_list", param: "type"}: {
			Field:  "template_type",
			Reason: "GitLab's path parameter :type is the template family, which the input names template_type",
		},
		{action: "merge_request.dependency_delete", param: "block_id"}: {
			Field: "blocking_merge_request_id",
			Reason: "GitLab's path parameter :block_id is the dependency to remove, which the input names " +
				"blocking_merge_request_id after client-go's argument",
		},
	}
}
