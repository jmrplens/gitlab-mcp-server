package cases

const (
	evalMT192 = "MT-192"
	evalMT195 = "MT-195"
	evalMS054 = "MS-054"
)

func enterpriseMutatingEvalCases() []Case {
	return []Case{
		baseEnterpriseMutatingEvalCase(
			"MS-006", "Check deployment gate state for project `my-org/tools/gitlab-mcp-server` and remote URL `https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git`: resolve the project, list available environments, inspect protected environment `production`, list production deployments, then approve deployment ID `77`. Do not call deployment approval until after the deployment list step completes.",
			readStep("gitlab_discover_project", "", params("remote_url"), nil),
			readStep("gitlab_environment", "list", params("project_id"), params("states")),
			readStep("gitlab_environment", "protected_get", params("project_id", "environment"), nil),
			readStep("gitlab_environment", "deployment_list", params("project_id"), params("environment")),
			readStep("gitlab_environment", "deployment_approve_or_reject", params("project_id", "deployment_id", "status"), params("comment")),
		),
		baseEnterpriseMutatingEvalCase("MT-120", "Update the admin compliance policy settings to use namespace ID `123`.", readStep("gitlab", "compliance_policy.update", params("csp_namespace_id"), nil)),
		baseEnterpriseMutatingEvalCase("MT-121", "Create a dependency list export for pipeline ID `12345`.", readStep("gitlab", "dependency.export_create", params("pipeline_id"), params("export_type"))),
		baseEnterpriseMutatingEvalCase("MT-126", "Create external project status check `Eval Gate` on project `my-org/tools/gitlab-mcp-server` pointing at `https://example.com/check`.", readStep("gitlab", "external_status_check.create_project", params("project_id", "name", "external_url"), params("shared_secret", "protected_branch_ids"))),
		baseEnterpriseMutatingEvalCase("MT-127", "Mark external status check ID `8` as passed for merge request IID `7` at SHA `abc123` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "external_status_check.set_project_mr_status", params("project_id", "merge_request_iid", "sha", "external_status_check_id", "status"), nil)),
		baseEnterpriseMutatingEvalCase("MT-130", "Create a disabled Geo secondary site named `eval-geo` with URL `https://geo.example.com`.", readStep("gitlab", "geo.create", params("name", "url"), params("enabled", "primary"))),
		baseEnterpriseMutatingEvalCase("MT-137", "Create an epic titled `Evaluation Epic` in group full path `my-org`.", readStep("gitlab", "group.epic_create", params("full_path", "title"), params("description", "start_date", "due_date"))),
		baseEnterpriseMutatingEvalCase("MT-138", "Update epic IID `12` in group full path `my-org` to close it.", readStep("gitlab", "group.epic_update", params("full_path", "epic_iid"), params("state_event", "title"))),
		baseEnterpriseMutatingEvalCase("MT-140", "Assign issue IID `99` from child project path `my-org/tools/gitlab-mcp-server` to epic IID `12` in group full path `my-org`.", readStep("gitlab", "group.epic_issue_assign", params("full_path", "epic_iid", "child_project_path", "child_iid"), nil)),
		baseEnterpriseMutatingEvalCase("MT-142", "Create note `Please update roadmap` on epic IID `12` in group full path `my-org`.", readStep("gitlab", "group.epic_note_create", params("full_path", "epic_iid", "body"), nil)),
		baseEnterpriseMutatingEvalCase("MT-144", "Add LDAP link for group `my-org` using provider `ldapmain`, CN `developers`, and Maintainer access.", readStep("gitlab", "group.ldap_link_add", params("group_id", "group_access", "provider"), params("cn", "filter", "member_role_id"))),
		baseEnterpriseMutatingEvalCase("MT-146", "Protect branch pattern `release/*` for group `my-org` with Maintainer merge access.", readStep("gitlab", "group.protected_branch_protect", params("group_id", "name"), params("merge_access_level", "push_access_level", "allowed_to_merge"))),
		baseEnterpriseMutatingEvalCase("MT-148", "Protect group environment `production` for group `my-org` requiring one approval.", readStep("gitlab", "group.protected_env_protect", params("group_id", "name", "deploy_access_levels"), params("approval_rules"))),
		baseEnterpriseMutatingEvalCase("MT-150", "Add SAML group link `Engineering` to group `my-org` with Developer access.", readStep("gitlab", "group.saml_link_add", params("group_id", "saml_group_name", "access_level"), params("provider", "member_role_id"))),
		baseEnterpriseMutatingEvalCase("MT-152", "Update group security settings for group `my-org` to enable secret push protection.", readStep("gitlab", "group.security_settings_update", params("group_id", "secret_push_protection_enabled"), nil)),
		baseEnterpriseMutatingEvalCase("MT-153", "Create group service account `eval-bot` in top-level group `my-org`.", readStep("gitlab", "group.service_account_create", params("group_id"), params("name", "username", "email"))),
		baseEnterpriseMutatingEvalCase("MT-155", "Create SSH certificate `Eval CA` for group `my-org`.", readStep("gitlab", "group.ssh_cert_create", params("group_id", "key", "title"), nil)),
		baseEnterpriseMutatingEvalCase("MT-157", "Create group wiki page `Evaluation Group Wiki` in group `my-org`.", readStep("gitlab", "group.wiki_create", params("group_id", "title", "content"), params("format"))),
		baseEnterpriseMutatingEvalCase("MT-160", "Update SCIM identity UID `external-123` in group `my-org` to external UID `external-456`.", readStep("gitlab", "group_scim.update", params("group_id", "uid", "extern_uid"), nil)),
		baseEnterpriseMutatingEvalCase("MT-163", "Create custom member role `Eval Auditor` in group `my-org` with Guest base access.", readStep("gitlab", "member_role.create_group", params("group_id", "name", "base_access_level"), params("read_code", "read_vulnerability"))),
		baseEnterpriseMutatingEvalCase("MT-165", "Add merge request IID `7` to the merge train in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "merge_train.add", params("project_id", "merge_request_iid"), params("auto_merge", "sha", "squash"))),
		baseEnterpriseMutatingEvalCase("MT-167", "Add a project push rule to project `my-org/tools/gitlab-mcp-server` that rejects unsigned commits.", readStep("gitlab", "project.push_rule_add", params("project_id"), params("reject_unsigned_commits", "commit_message_regex"))),
		baseEnterpriseMutatingEvalCase("MT-169", "Update project security settings for project `my-org/tools/gitlab-mcp-server` to enable secret push protection.", readStep("gitlab", "project.security_settings_update", params("project_id", "secret_push_protection_enabled"), nil)),
		baseEnterpriseMutatingEvalCase("MT-170", "Create project alias `eval-alias` for numeric project ID `123`; do not use a project path for project_id.", readStep("gitlab", "project_alias.create", params("name", "project_id"), nil)),
		baseEnterpriseMutatingEvalCase("MT-175", "Create an instance service account named `eval-service-account` with username `eval-service-account`.", readStep("gitlab", "user.create_service_account", params("name", "username"), params("email"))),
		baseEnterpriseMutatingEvalCase("MT-181", "Create project service account `eval-project-bot` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "project.service_account_create", params("project_id"), params("name", "username", "email"))),
		baseEnterpriseMutatingEvalCase("MT-182", "Update project service account user ID `55` in project `my-org/tools/gitlab-mcp-server` to name `eval-project-bot-v2`.", readStep("gitlab", "project.service_account_update", params("project_id", "service_account_id"), params("name", "username", "email"))),
		baseEnterpriseMutatingEvalCase("MT-185", "Create personal access token `eval-project-bot-token` with scope `api` for project service account user ID `55` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "project.service_account_pat_create", params("project_id", "service_account_id", "name", "scopes"), params("description"))),
		baseEnterpriseMutatingEvalCase("MT-186", "Rotate project service account PAT ID `66` for project service account user ID `55` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "project.service_account_pat_rotate", params("project_id", "service_account_id", "token_id"), nil)),
		baseEnterpriseMutatingEvalCase(evalMT192, "Add a project push rule to project `my-org/tools/gitlab-mcp-server` with commit message regex `^EVAL-`.", readStep("gitlab_project", "push_rule_add", params("project_id"), params("commit_message_regex", "reject_unsigned_commits"))),
		baseEnterpriseMutatingEvalCase("MT-193", "Edit the project push rule in project `my-org/tools/gitlab-mcp-server` to reject unsigned commits.", readStep("gitlab_project", "push_rule_edit", params("project_id"), params("reject_unsigned_commits", "commit_message_regex"))),
		baseEnterpriseMutatingEvalCase("MT-194", "Update project security settings for project `my-org/tools/gitlab-mcp-server` to set `secret_push_protection_enabled` to true.", readStep("gitlab_project", "security_settings_update", params("project_id", "secret_push_protection_enabled"), nil)),
		baseEnterpriseMutatingEvalCase(evalMT195, "Update project service account user ID `55` in project `my-org/tools/gitlab-mcp-server` to name `eval-project-bot-live`.", readStep("gitlab_project", "service_account_update", params("project_id", "service_account_id"), params("name", "username", "email"))),
		baseEnterpriseMutatingEvalCase(
			evalMS054, "Exercise Enterprise project mutating settings in project `my-org/tools/gitlab-mcp-server`: get project security settings, update `secret_push_protection_enabled` to true, list project service accounts, then update project service account user ID `55` to name `eval-project-bot-workflow`.",
			readStep("gitlab_project", "security_settings_get", params("project_id"), nil),
			readStep("gitlab_project", "security_settings_update", params("project_id", "secret_push_protection_enabled"), nil),
			readStep("gitlab_project", "service_account_list", params("project_id"), params("per_page")),
			readStep("gitlab_project", "service_account_update", params("project_id", "service_account_id"), params("name")),
		),
	}
}

func baseEnterpriseMutatingEvalCase(id, prompt string, steps ...Step) Case {
	presets := []string{presetSchemaEnterprise}
	if (id >= evalMT192 && id <= evalMT195) || id == evalMS054 {
		presets = append(presets, presetDockerEnterpriseMutatingSafe)
	}
	promptTemplate, fixtures := enterpriseMutatingPromptTemplateAndFixtures(id, prompt)
	return Case{
		ID:             id,
		Prompt:         prompt,
		PromptTemplate: promptTemplate,
		Steps:          steps,
		Fixtures:       fixtures,
		Edition:        editionEnterprise,
		Presets:        presets,
		Partition:      partitionEnterpriseMutating,
		Mutating:       true,
		ReportGroup:    partitionEnterpriseMutating,
	}
}

func enterpriseMutatingPromptTemplateAndFixtures(id, prompt string) (promptTemplate PromptTemplate, fixtures []string) {
	switch id {
	case evalMT192:
		return PromptTemplate{Text: "Add a project push rule to project `{{.Project.Path}}` with commit message regex `^EVAL-`. Only add the rule; do not edit or delete it."}, []string{fixtureEnterprisePushRuleProject}
	case "MT-193":
		return PromptTemplate{Text: "Edit the project push rule in project `{{.Project.Path}}` to reject unsigned commits."}, []string{fixtureEnterprisePushRuleProjectSeeded}
	case evalMT195:
		return PromptTemplate{Text: "Update project service account user ID `{{.Values.project_service_account_id}}` in project `{{.Project.Path}}` to name `eval-project-bot-live`."}, []string{fixtureProjectServiceAccount}
	case evalMS054:
		return PromptTemplate{Text: "Exercise Enterprise project mutating settings in project `{{.Project.Path}}`: get project security settings, update `secret_push_protection_enabled` to true, list project service accounts, then update project service account user ID `{{.Values.project_service_account_id}}` to name `eval-project-bot-workflow`."}, []string{fixtureProjectServiceAccount}
	default:
		return PromptTemplate{Text: prompt}, nil
	}
}
