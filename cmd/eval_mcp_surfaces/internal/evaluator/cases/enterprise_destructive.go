package cases

const (
	evalMS053             = "MS-053"
	actionGroupEpicDelete = "group.epic_delete"
	evalMT196             = "MT-196"
	evalMT198             = "MT-198"
	evalMS045             = "MS-045"
	actionGroupEpicCreate = "group.epic_create"
)

func enterpriseDestructiveEvalCases() []Case {
	return []Case{
		baseEnterpriseDestructiveEvalCase(
			"MS-005", "Review external integration risk in project `my-org/tools/gitlab-mcp-server`: list project hooks, list project status checks, inspect CI job-token inbound allowlist, then remove target project ID `123` from that allowlist.",
			readStep("gitlab_project", "hook_list", params("project_id"), nil),
			readStep("gitlab_external_status_check", "list_project", params("project_id"), nil),
			readStep("gitlab_job", "token_scope_list_inbound", params("project_id"), nil),
			destructiveStep("gitlab_job", "token_scope_remove_project", params("project_id", "target_project_id"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase("MT-116", "Force-push remote mirror ID `9` for project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_project", "mirror_force_push", params("project_id", "mirror_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-125", "Disable two-factor authentication for enterprise user ID `55` in group `my-org`.", destructiveStep("gitlab", "enterprise_user.disable_2fa", params("group_id", "user_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-128", "Delete external project status check ID `8` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab", "external_status_check.delete_project", params("project_id", "check_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-131", "Delete Geo site ID `3`.", destructiveStep("gitlab", "geo.delete", params("id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-134", "Revoke group personal access token ID `77` in group `my-org`.", destructiveStep("gitlab", "group.credential_revoke_pat", params("group_id", "token_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-139", "Delete epic IID `12` from group full path `my-org`.", destructiveStep("gitlab", actionGroupEpicDelete, params("full_path", "epic_iid"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-141", "Remove issue IID `99` from child project path `my-org/tools/gitlab-mcp-server` from epic IID `12` in group full path `my-org`.", destructiveStep("gitlab", "group.epic_issue_remove", params("full_path", "epic_iid", "child_project_path", "child_iid"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-143", "Delete note ID `44` from epic IID `12` in group full path `my-org`.", destructiveStep("gitlab", "group.epic_note_delete", params("full_path", "epic_iid", "note_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-145", "Delete LDAP link for provider `ldapmain` in group `my-org`.", destructiveStep("gitlab", "group.ldap_link_delete_for_provider", params("group_id", "provider"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-147", "Unprotect branch pattern `release/*` for group `my-org`; pass the branch name as `branch`.", destructiveStep("gitlab", "group.protected_branch_unprotect", params("group_id", "branch"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-149", "Unprotect group environment `production` for group `my-org`; pass the environment name as `environment`.", destructiveStep("gitlab", "group.protected_env_unprotect", params("group_id", "environment"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-151", "Delete SAML group link `Engineering` from group `my-org`.", destructiveStep("gitlab", "group.saml_link_delete", params("group_id", "saml_group_name"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-154", "Revoke service account PAT ID `66` for service account user ID `55` in group `my-org`.", destructiveStep("gitlab", "group.service_account_pat_revoke", params("group_id", "service_account_id", "token_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-156", "Delete SSH certificate ID `44` from group `my-org`.", destructiveStep("gitlab", "group.ssh_cert_delete", params("group_id", "certificate_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-158", "Delete group wiki page slug `evaluation-group-wiki` from group `my-org`.", destructiveStep("gitlab", "group.wiki_delete", params("group_id", "slug"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-164", "Delete instance member role ID `44`.", destructiveStep("gitlab", "member_role.delete_instance", params("member_role_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-168", "Delete the project push rule from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab", "project.push_rule_delete", params("project_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-171", "Delete project alias `eval-alias`.", destructiveStep("gitlab", "project_alias.delete", params("name"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-183", "Delete project service account user ID `55` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab", "project.service_account_delete", params("project_id", "service_account_id"), params("hard_delete", "confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-187", "Revoke project service account PAT ID `66` for project service account user ID `55` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab", "project.service_account_pat_revoke", params("project_id", "service_account_id", "token_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase(evalMT196, "Delete the project push rule from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_project", "push_rule_delete", params("project_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase("MT-197", "Revoke group service account PAT ID `66` for service account user ID `55` in group `my-org`.", destructiveStep("gitlab_group", "service_account_pat_revoke", params("group_id", "service_account_id", "token_id"), params("confirm"))),
		baseEnterpriseDestructiveEvalCase(evalMT198, "Delete group service account user ID `55` in group `my-org`.", destructiveStep("gitlab_group", "service_account_delete", params("group_id", "service_account_id"), params("hard_delete", "confirm"))),
		baseEnterpriseDestructiveEvalCase(
			"MS-043", "Exercise project service account lifecycle in project `my-org/tools/gitlab-mcp-server`: create service account `eval-project-service-account`, list project service accounts, update the created service account name to `eval-project-service-account-v2`, create personal access token `eval-project-service-token` with scope `api`, list that service account's tokens, rotate the token, revoke the rotated token, then delete the service account.",
			readStep("gitlab_project", "service_account_create", params("project_id"), params("name", "username")),
			readStep("gitlab_project", "service_account_list", params("project_id"), params("per_page")),
			readStep("gitlab_project", "service_account_update", params("project_id", "service_account_id"), params("name")),
			readStep("gitlab_project", "service_account_pat_create", params("project_id", "service_account_id", "name", "scopes"), params("description")),
			readStep("gitlab_project", "service_account_pat_list", params("project_id", "service_account_id"), params("state")),
			readStep("gitlab_project", "service_account_pat_rotate", params("project_id", "service_account_id", "token_id"), nil),
			destructiveStep("gitlab_project", "service_account_pat_revoke", params("project_id", "service_account_id", "token_id"), params("confirm")),
			destructiveStep("gitlab_project", "service_account_delete", params("project_id", "service_account_id"), params("hard_delete", "confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			evalMS045, "Exercise project push rule lifecycle: add a push rule to project `my-org/tools/gitlab-mcp-server` with commit message regex `^EVAL-`, fetch the project push rule, edit it to reject unsigned commits, then delete the project push rule.",
			readStep("gitlab_project", "push_rule_add", params("project_id"), params("commit_message_regex")),
			readStep("gitlab_project", "push_rule_get", params("project_id"), nil),
			readStep("gitlab_project", "push_rule_edit", params("project_id"), params("reject_unsigned_commits", "commit_message_regex")),
			destructiveStep("gitlab_project", "push_rule_delete", params("project_id"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-046", "Exercise group service account lifecycle in group `my-org`: list group service accounts, create service account `eval-group-service-account`, update the created service account name to `eval-group-service-account-v2`, create personal access token `eval-group-service-token` with scope `api`, list that service account's tokens, revoke the created token, then delete the service account.",
			readStep("gitlab_group", "service_account_list", params("group_id"), params("per_page")),
			readStep("gitlab_group", "service_account_create", params("group_id"), params("name", "username")),
			readStep("gitlab_group", "service_account_update", params("group_id", "service_account_id"), params("name")),
			readStep("gitlab_group", "service_account_pat_create", params("group_id", "service_account_id", "name", "scopes"), params("description")),
			readStep("gitlab_group", "service_account_pat_list", params("group_id", "service_account_id"), params("per_page")),
			destructiveStep("gitlab_group", "service_account_pat_revoke", params("group_id", "service_account_id", "token_id"), params("confirm")),
			destructiveStep("gitlab_group", "service_account_delete", params("group_id", "service_account_id"), params("hard_delete", "confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-047", "Exercise epic CRUD in group full path `my-org`: create epic `Evaluation Enterprise Epic`, list epics, fetch the created epic by IID, update its title to `Evaluation Enterprise Epic v2`, then delete the epic.",
			readStep("gitlab", actionGroupEpicCreate, params("full_path", "title"), params("description")),
			readStep("gitlab", "group.epic_list", params("full_path"), params("first")),
			readStep("gitlab", "group.epic_get", params("full_path", "epic_iid"), nil),
			readStep("gitlab", "group.epic_update", params("full_path", "epic_iid"), params("title")),
			destructiveStep("gitlab", actionGroupEpicDelete, params("full_path", "epic_iid"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-048", "Exercise epic note lifecycle in group full path `my-org`: create epic `Evaluation Enterprise Note Epic`, create note `first enterprise note`, list epic notes, fetch the created note by note ID, update it to `updated enterprise note`, delete the note, then delete the epic.",
			readStep("gitlab", actionGroupEpicCreate, params("full_path", "title"), params("description")),
			readStep("gitlab", "group.epic_note_create", params("full_path", "epic_iid", "body"), nil),
			readStep("gitlab", "group.epic_note_list", params("full_path", "epic_iid"), nil),
			readStep("gitlab", "group.epic_note_get", params("full_path", "epic_iid", "note_id"), nil),
			readStep("gitlab", "group.epic_note_update", params("full_path", "epic_iid", "note_id", "body"), nil),
			destructiveStep("gitlab", "group.epic_note_delete", params("full_path", "epic_iid", "note_id"), params("confirm")),
			destructiveStep("gitlab", actionGroupEpicDelete, params("full_path", "epic_iid"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-049", "Exercise epic discussion lifecycle in group full path `my-org`: create epic `Evaluation Enterprise Discussion Epic`, create discussion `first enterprise discussion`, list discussions, fetch the created discussion, add reply note `enterprise reply`, update that reply to `enterprise reply updated`, delete the reply note, then delete the epic.",
			readStep("gitlab", actionGroupEpicCreate, params("full_path", "title"), params("description")),
			readStep("gitlab", "group.epic_discussion_create", params("full_path", "epic_iid", "body"), nil),
			readStep("gitlab", "group.epic_discussion_list", params("full_path", "epic_iid"), nil),
			readStep("gitlab", "group.epic_discussion_get", params("full_path", "epic_iid", "discussion_id"), nil),
			readStep("gitlab", "group.epic_discussion_add_note", params("full_path", "epic_iid", "discussion_id", "body"), nil),
			readStep("gitlab", "group.epic_discussion_update_note", params("full_path", "epic_iid", "note_id", "body"), nil),
			destructiveStep("gitlab", "group.epic_discussion_delete_note", params("full_path", "epic_iid", "note_id"), params("confirm")),
			destructiveStep("gitlab", actionGroupEpicDelete, params("full_path", "epic_iid"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-050", "Exercise epic issue assignment in group full path `my-org` using project `my-org/tools/gitlab-mcp-server`: create issue `eval-enterprise-epic-child`, create epic `Evaluation Enterprise Issue Epic`, assign the issue to the epic, list epic issues, remove the issue from the epic, delete the issue, then delete the epic.",
			readStep("gitlab_issue", "create", params("project_id", "title"), params("description")),
			readStep("gitlab", actionGroupEpicCreate, params("full_path", "title"), params("description")),
			readStep("gitlab", "group.epic_issue_assign", params("full_path", "epic_iid", "child_project_path", "child_iid"), nil),
			readStep("gitlab", "group.epic_issue_list", params("full_path", "epic_iid"), nil),
			destructiveStep("gitlab", "group.epic_issue_remove", params("full_path", "epic_iid", "child_project_path", "child_iid"), params("confirm")),
			destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm")),
			destructiveStep("gitlab", actionGroupEpicDelete, params("full_path", "epic_iid"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-051", "Exercise group protected branch lifecycle in group `my-org`: protect branch pattern `eval-enterprise/*` with Maintainer push and merge access, list group protected branches, fetch that protected branch, update it to allow force push, then unprotect it.",
			readStep("gitlab_group", "protected_branch_protect", params("group_id", "name"), params("push_access_level", "merge_access_level")),
			readStep("gitlab_group", "protected_branch_list", params("group_id"), params("per_page")),
			readStep("gitlab_group", "protected_branch_get", params("group_id", "branch"), nil),
			readStep("gitlab_group", "protected_branch_update", params("group_id", "branch"), params("allow_force_push")),
			destructiveStep("gitlab_group", "protected_branch_unprotect", params("group_id", "branch"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-052", "Exercise group protected environment lifecycle with a temporary group: create group `eval-enterprise-protected-env`, protect environment `staging`, list group protected environments, fetch environment `staging`, update it to require one approval, unprotect environment `staging`, then delete the temporary group.",
			readStep("gitlab_group", "create", params("name", "path"), params("parent_id", "visibility")),
			readStep("gitlab_group", "protected_env_protect", params("group_id", "name", "deploy_access_levels"), nil),
			readStep("gitlab_group", "protected_env_list", params("group_id"), params("per_page")),
			readStep("gitlab_group", "protected_env_get", params("group_id", "environment"), nil),
			readStep("gitlab_group", "protected_env_update", params("group_id", "environment"), params("approval_rules")),
			destructiveStep("gitlab_group", "protected_env_unprotect", params("group_id", "environment"), params("confirm")),
			destructiveStep("gitlab_group", "delete", params("group_id"), params("confirm")),
		),
		baseEnterpriseDestructiveEvalCase(
			"MS-053", "Exercise project protected environment lifecycle with a temporary project: create project `eval-enterprise-protected-env-project`, protect environment `staging`, list protected environments, fetch environment `staging`, unprotect environment `staging`, then delete the temporary project.",
			readStep("gitlab_project", "create", params("name"), params("path", "namespace_id", "initialize_with_readme", "visibility")),
			readStep("gitlab_environment", "protected_protect", params("project_id", "name", "deploy_access_levels"), nil),
			readStep("gitlab_environment", "protected_list", params("project_id"), params("per_page")),
			readStep("gitlab_environment", "protected_get", params("project_id", "environment"), nil),
			destructiveStep("gitlab_environment", "protected_unprotect", params("project_id", "environment"), params("confirm")),
			destructiveStep("gitlab_project", "delete", params("project_id"), params("permanently_remove", "full_path", "confirm")),
		),
	}
}

func baseEnterpriseDestructiveEvalCase(id, prompt string, steps ...Step) Case {
	presets := []string{presetSchemaEnterprise}
	if (id >= evalMT196 && id <= evalMT198) || id == "MS-043" || (id >= evalMS045 && id <= evalMS053) {
		presets = append(presets, presetDockerEnterpriseDestructiveSafe)
	}
	promptTemplate, fixtures := enterpriseDestructivePromptTemplateAndFixtures(id, prompt)
	return Case{
		ID:             id,
		Prompt:         prompt,
		PromptTemplate: promptTemplate,
		Steps:          steps,
		Fixtures:       fixtures,
		Edition:        editionEnterprise,
		Presets:        presets,
		Partition:      partitionEnterpriseDestructive,
		Mutating:       true,
		Destructive:    true,
		ReportGroup:    partitionEnterpriseDestructive,
	}
}

func enterpriseDestructivePromptTemplateAndFixtures(id, prompt string) (promptTemplate PromptTemplate, fixtures []string) {
	switch id {
	case "MS-005":
		return PromptTemplate{Text: "Review external integration risk in project `{{.Project.Path}}`: list project hooks, list project status checks, inspect CI job-token inbound allowlist, then remove target project ID `{{.Values.target_project_id}}` from that allowlist."}, []string{fixtureJobTokenScopeProject}
	case evalMT196:
		return PromptTemplate{Text: "Delete the project push rule from project `{{.Project.Path}}`."}, []string{fixtureEnterprisePushRuleProjectSeeded}
	case "MT-197":
		return PromptTemplate{Text: "Revoke group service account PAT ID `{{.Token.ID}}` for service account user ID `{{.Values.service_account_id}}` in group `my-org`."}, []string{fixtureEnterpriseGroupServiceAccountPAT}
	case evalMT198:
		return PromptTemplate{Text: "Delete group service account user ID `{{.Values.service_account_id}}` in group `my-org`."}, []string{fixtureEnterpriseGroupServiceAccount}
	case evalMS045:
		return PromptTemplate{Text: "Exercise project push rule lifecycle: add a push rule to project `{{.Project.Path}}` with commit message regex `^EVAL-`, fetch the project push rule, edit it to reject unsigned commits, then delete the project push rule."}, []string{fixtureEnterprisePushRuleProject}
	case "MS-052":
		return PromptTemplate{Text: "Exercise group protected environment lifecycle with a temporary group: create group `{{ .Values.subgroup_name }}` with path `{{ .Values.subgroup_path }}`, protect environment `staging` with Maintainer deploy access, list group protected environments, fetch environment `staging`, update it to require one approval, unprotect environment `staging`, then delete the temporary group."}, []string{fixtureAttemptNames}
	case evalMS053:
		return PromptTemplate{Text: "Exercise project protected environment lifecycle with a temporary project: create project `{{ .Values.subgroup_name }}` with path `{{ .Values.subgroup_path }}`, protect environment `staging` with Maintainer deploy access, list protected environments, fetch environment `staging`, unprotect environment `staging`, then delete the temporary project."}, []string{fixtureAttemptNames}
	default:
		return PromptTemplate{Text: prompt}, nil
	}
}
