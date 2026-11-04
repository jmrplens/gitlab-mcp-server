// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package required

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

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
		reasonLabelSelector = "the route under /projects/:id/labels/:name requires the label in its path; " +
			"labels.labelSelector fills that segment from label_id when the caller gives it and from name otherwise, " +
			"and the handler refuses a call with neither, as the anyOf of the schema says"
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
		reasonLockInfo = "POST .../terraform/state/:name/lock requires every key of terraform's lock info and keeps " +
			"only the ID: lib/api/terraform/state.rb answers Operation, Info, Version and Path back as sent, and Who " +
			"and Created from the lock it records; the handler sends the key empty when the caller gives none, " +
			"which Grape's requires takes as present"
		// actionTerraformStateLock keys the six lock info declarations below.
		actionTerraformStateLock = "admin.terraform_state_lock"
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

		// A null client-go writes in place of a key left out.
		{action: "pipeline.schedule_edit_variable", field: "value"}: {
			DirectionGitLabOptional, categorySDKSendsNull,
			"PUT .../pipeline_schedules/:pipeline_schedule_id/variables/:key declares value optional, and " +
				"client-go's EditPipelineScheduleVariableOptions writes it with no omitempty, so a call without " +
				"one sends null, which lib/api/ci/pipeline_schedules.rb passes on through declared_params and the " +
				"save service assigns over the stored value; the handler therefore always sends a value " +
				"(docs/development/upstream-bugs.md, entry 54)",
		},

		// Requirements GitLab states inside a given block.
		{action: "repository.commit_comment_create", field: "line"}:      {DirectionGitLabRequires, categoryConditional, reasonCommitCommentLine},
		{action: "repository.commit_comment_create", field: "line_type"}: {DirectionGitLabRequires, categoryConditional, reasonCommitCommentLine},
		{action: "snippet.create", field: "file_name"}:                   {DirectionGitLabRequires, categoryConditional, reasonSnippetFileName},
		{action: "snippet.project_create", field: "file_name"}:           {DirectionGitLabRequires, categoryConditional, reasonSnippetFileName},
		{action: "user.create_runner", field: "group_id"}:                {DirectionGitLabRequires, categoryConditional, reasonRunnerScope},
		{action: "user.create_runner", field: "project_id"}:              {DirectionGitLabRequires, categoryConditional, reasonRunnerScope},
		{action: "audit_event.list_instance", field: "entity_type"}: {
			DirectionGitLabRequires, categoryConditional,
			"ee/lib/api/audit_events.rb requires entity_type only in a given :entity_id block; the record writes it as " +
				"an unconditional requirement, and a list of every entity takes neither (the handler refuses an " +
				"entity_id without one, as GitLab would)",
		},

		// Requirements the handler meets for a caller who leaves the field out.
		{action: "snippet.project_create", field: "visibility"}: {
			DirectionGitLabRequires, categoryHandlerDefault,
			"lib/api/project_snippets.rb requires visibility; the handler sends private when the caller gives none, as the schema says",
		},
		{action: "repository.list_submodules", field: "ref"}:     {DirectionGitLabRequires, categoryHandlerDefault, reasonSubmoduleRef},
		{action: "repository.read_submodule_file", field: "ref"}: {DirectionGitLabRequires, categoryHandlerDefault, reasonSubmoduleRef},
		{action: "project.label_update", field: "name"}:          {DirectionGitLabRequires, categoryHandlerDefault, reasonLabelSelector},
		{action: "project.label_delete", field: "name"}:          {DirectionGitLabRequires, categoryHandlerDefault, reasonLabelSelector},

		// The lock info, of which GitLab keeps the ID alone (issue 1597).
		{action: actionTerraformStateLock, field: "operation"}: {DirectionGitLabRequires, categoryHandlerDefault, reasonLockInfo},
		{action: actionTerraformStateLock, field: "info"}:      {DirectionGitLabRequires, categoryHandlerDefault, reasonLockInfo},
		{action: actionTerraformStateLock, field: "who"}:       {DirectionGitLabRequires, categoryHandlerDefault, reasonLockInfo},
		{action: actionTerraformStateLock, field: "version"}:   {DirectionGitLabRequires, categoryHandlerDefault, reasonLockInfo},
		{action: actionTerraformStateLock, field: "created"}:   {DirectionGitLabRequires, categoryHandlerDefault, reasonLockInfo},
		{action: actionTerraformStateLock, field: "path"}:      {DirectionGitLabRequires, categoryHandlerDefault, reasonLockInfo},
	}
}

// workhorseBody is what GitLab reads out of the body of a request Workhorse
// turned into one file param: each param, with whether GitLab refuses a
// request without it, and where lib/api says so.
type workhorseBody struct {
	Params map[string]bool
	Source string
}

// answers reports whether a record route still leaves its body to one
// WorkhorseFile param: a required param of that type, and none of the params
// the declaration says the body carries, which a record that declared them
// again would answer itself. A param the declaration does not name, beside the
// file, leaves the answer as it is, since the record answers for that one.
func (b workhorseBody) answers(route *apilive.Route) bool {
	for name := range b.Params {
		if _, declared := route.Params[name]; declared {
			return false
		}
	}
	for _, param := range route.Params {
		if param.Type == apilive.WorkhorseFile && param.Required {
			return true
		}
	}
	return false
}

// declaredWorkhorseBodies holds, by route as the request record names it, the
// body of each route this server sends whose endpoint reads its params out of
// the file Workhorse writes the whole body to. Workhorse does that before Rails
// sees the request, and the endpoint parses the file and validates the params
// it holds by hand
// (file_params_from_body_upload, lib/api/helpers/commits_body_uploader_helper.rb:20
// at v19.5.0-ee), so the record, which reads Grape's declarations, holds none
// of them: without this table every field these inputs publish beside the
// path was unplaced, and a schema could drop `,required` from a commit
// message GitLab refuses to commit without. The params of a body are placed
// as if the record declared them. The table names its routes because the
// record cannot tell one of them from a real upload: PUT /user/avatar declares
// one required WorkhorseFile param and nothing else as well, and its client
// sends a multipart form whose file is all the endpoint reads, and a generic
// package's body is the file itself. A declaration for a route no action
// sends, or whose record route declares a param of its body again or stops
// requiring the file, answers nothing.
func declaredWorkhorseBodies() map[string]workhorseBody {
	const filesSource = "lib/api/files.rb:167-210 at v19.5.0-ee (validate_file_params! and filter_file_params!): " +
		"branch and commit_message are refused blank and content absent, and start_branch, author_email, " +
		"author_name, encoding, last_commit_id and execute_filemode are read when given"
	files := map[string]bool{
		"branch": true, "commit_message": true, "content": true,
		"start_branch": false, "author_email": false, "author_name": false,
		"encoding": false, "last_commit_id": false, "execute_filemode": false,
	}
	return map[string]workhorseBody{
		"POST /projects/:id/repository/commits": {
			Params: map[string]bool{
				"branch": true, "commit_message": true, "actions": true,
				"start_branch": false, "start_sha": false, "start_project": false,
				"author_email": false, "author_name": false, "stats": false, "force": false, "allow_empty": false,
			},
			Source: "lib/api/commits.rb:91-190 at v19.5.0-ee (validate_commits_attrs!, validate_actions_allow_empty! and " +
				"filter_commits_attrs!): branch and commit_message are refused blank and actions absent or empty unless " +
				"allow_empty is true, which client-go's options cannot send, and start_branch, start_sha, start_project, " +
				"author_email, author_name, stats and force are read when given",
		},
		"POST /projects/:id/repository/files/:file_path": {Params: files, Source: filesSource},
		"PUT /projects/:id/repository/files/:file_path":  {Params: files, Source: filesSource},
	}
}

// FieldAliases returns, by action and then by GitLab parameter, the input
// field each declared alias fills the parameter from, so another rule that
// looks a route's params up among an input's fields (R-PARAM) finds them
// where this scope does rather than keeping a second table.
func FieldAliases() map[string]map[string]string {
	return aliasesByAction(declaredAliases())
}

// aliasKey names the GitLab parameter of an action an alias places.
type aliasKey struct {
	action string
	param  string
}

// paramAlias names the field an action fills a GitLab parameter from, where the
// placement rules ([placements]) would find another.
type paramAlias struct {
	// Field is the input field that fills the parameter.
	Field string
	// Reason says why the rules cannot find it.
	Reason string
}

// declaredAliases holds every GitLab parameter an action fills from a field
// the placement rules do not find it under: a param of the body or the query
// that GitLab names otherwise than the input does. A path parameter needs
// none, since the request record names the field each call fills it from
// (the request's placeholders), and an alias for one is read only where the
// record names none. Without the alias the join holds the param's
// requiredness to a field of its name, or to no field at all. The body
// group_id of /groups/:id/share and /groups/:id/transfer is the case the
// placeholders alone cannot answer: the input's group_id fills the :id beside
// it, so the placement passes over a param of that name, and the field that
// does fill it is named here. A path parameter is here only when the call
// fills it from a value GitLab answered, which the record cannot name a field
// for.
func declaredAliases() map[aliasKey]paramAlias {
	const groupShared = "the body param group_id of POST /groups/:id/share is the group shared with, which the input names %s, " +
		"its group_id being the :id of the path"
	const lockInfoKey = "GitLab spells the keys of terraform's lock info as terraform's LockInfo does, " +
		"capitalized; the input writes %s in lower case, as every input here writes a parameter"
	const lockID = "GitLab's ID is the lock's, beside the project's id of the path; the input names it lock_id, " +
		"since a bare id would read as the project or the state, and the unlock reads an id as the state's name"
	// lockAction keys the seven aliases of the lock's parameters below.
	const lockAction = "admin.terraform_state_lock"
	aliased := map[aliasKey]paramAlias{
		{action: lockAction, param: "ID"}:                     {Field: "lock_id", Reason: lockID},
		{action: "admin.terraform_state_unlock", param: "ID"}: {Field: "lock_id", Reason: lockID},
		{action: lockAction, param: "Operation"}:              {Field: "operation", Reason: fmt.Sprintf(lockInfoKey, "Operation")},
		{action: lockAction, param: "Info"}:                   {Field: "info", Reason: fmt.Sprintf(lockInfoKey, "Info")},
		{action: lockAction, param: "Who"}:                    {Field: "who", Reason: fmt.Sprintf(lockInfoKey, "Who")},
		{action: lockAction, param: "Version"}:                {Field: "version", Reason: fmt.Sprintf(lockInfoKey, "Version")},
		{action: lockAction, param: "Created"}:                {Field: "created", Reason: fmt.Sprintf(lockInfoKey, "Created")},
		{action: lockAction, param: "Path"}:                   {Field: "path", Reason: fmt.Sprintf(lockInfoKey, "Path")},
		{action: "project.milestone_promote", param: "milestone_id"}: {
			Field: "milestone_iid",
			Reason: "the :milestone_id of POST /projects/:id/milestones/:milestone_id/promote is the ID in the answer of a " +
				"lookup of milestone_iid, a value GitLab answered, so the record names no field for it",
		},
		{action: "branch.create", param: "branch"}: {
			Field:  "branch_name",
			Reason: "the body param branch of POST /projects/:id/repository/branches is the new branch, which the input names branch_name",
		},
		{action: "tag.protect", param: "name"}: {
			Field:  "tag_name",
			Reason: "the body param name of POST /projects/:id/protected_tags is the tag or wildcard to protect, which the input names tag_name",
		},
		{action: "group.group_member_share", param: "group_id"}: {
			Field:  "share_group_id",
			Reason: fmt.Sprintf(groupShared, "share_group_id"),
		},
		{action: "group.share_with_group", param: "group_id"}: {
			Field:  "shared_group_id",
			Reason: fmt.Sprintf(groupShared, "shared_group_id"),
		},
		{action: "group.transfer", param: "group_id"}: {
			Field: "parent_id",
			Reason: "the body param group_id of POST /groups/:id/transfer is the new parent, which the input names parent_id, " +
				"its group_id being the :id of the path",
		},
		{action: "branch.protect", param: "name"}: {
			Field: "branch_name",
			Reason: "the body param name of POST /projects/:id/protected_branches is the branch or wildcard to protect, " +
				"which the input names branch_name",
		},
		{action: "feature_flags.feature_flag_update", param: "name"}: {
			Field:  "new_name",
			Reason: "the new name GitLab calls name is the input's new_name, its name being the flag the path addresses",
		},
	}
	for _, scope := range []string{"code", "commits", "issues", "merge_requests", "milestones", "notes", "projects", "snippets", "users", "wiki"} {
		aliased[aliasKey{action: "search." + scope, param: "search"}] = paramAlias{
			Field:  "query",
			Reason: "the query param search of every search route is what the input names query, its scope being the action itself",
		}
	}
	for _, action := range []string{"list", "get", "set", "delete"} {
		aliased[aliasKey{action: "admin.custom_attr_" + action, param: "id"}] = paramAlias{
			Field: "resource_id",
			Reason: "the :id of a custom attribute route is resource_id, or the ID in the answer of a lookup of it, and the lookup " +
				"is handed resource_type as well, which says which read to send, so the record names no one field for it",
		}
	}
	return aliased
}
