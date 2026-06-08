package cases

func enterpriseReadEvalCases() []Case {
	return []Case{
		baseEnterpriseReadEvalCase("MT-070", "List attestations in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_attestation", "list", params("project_id"), params("subject_digest"))),
		baseEnterpriseReadEvalCase("MT-074", "List dependency inventory for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_dependency", "list", params("project_id"), params("package_manager", "per_page"))),
		baseEnterpriseReadEvalCase("MT-075", "Get deployment frequency DORA metrics for project `my-org/tools/gitlab-mcp-server` from `2026-01-01` to `2026-01-31`.", readStep("gitlab_dora_metrics", "project", params("project_id", "metric"), params("start_date", "end_date", "interval"))),
		baseEnterpriseReadEvalCase("MT-076", "List enterprise users in group `my-org`.", readStep("gitlab_enterprise_user", "list", params("group_id"), params("search", "active", "per_page"))),
		baseEnterpriseReadEvalCase("MT-078", "List Geo nodes.", readStep("gitlab_geo", "list", nil, nil)),
		baseEnterpriseReadEvalCase("MT-079", "List SCIM identities for group `my-org`.", readStep("gitlab_group_scim", "list", params("group_id"), nil)),
		baseEnterpriseReadEvalCase("MT-084", "List custom member roles in group `my-org`.", readStep("gitlab_member_role", "list_group", params("group_id"), nil)),
		baseEnterpriseReadEvalCase("MT-085", "List merge trains for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_merge_train", "list_project", params("project_id"), params("scope", "per_page"))),
		baseEnterpriseReadEvalCase("MT-086", "Download model registry file `model.onnx` from path `models` for model version ID `candidate:5` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_model_registry", "download", params("project_id", "model_version_id", "path", "filename"), nil)),
		baseEnterpriseReadEvalCase("MT-087", "List project aliases.", readStep("gitlab_project_alias", "list", nil, nil)),
		baseEnterpriseReadEvalCase("MT-088", "List security findings for pipeline IID `12345` in project path `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_security_finding", "list", params("project_path", "pipeline_iid"), params("severity", "report_type"))),
		baseEnterpriseReadEvalCase("MT-089", "Retrieve all project repository storage moves.", readStep("gitlab_storage_move", "retrieve_all_project", nil, params("per_page"))),
		baseEnterpriseReadEvalCase("MT-091", "List vulnerabilities for project path `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_vulnerability", "list", params("project_path"), params("state", "severity", "first"))),
		baseEnterpriseReadEvalCase("MT-117", "Download attestation IID `5` from project `my-org/tools/gitlab-mcp-server`; use the project-scoped attestation IID, not the database ID.", readStep("gitlab", "attestation.download", params("project_id", "attestation_iid"), nil)),
		baseEnterpriseReadEvalCase("MT-118", "Get instance audit event ID `77`.", readStep("gitlab", "audit_event.get_instance", params("event_id"), nil)),
		baseEnterpriseReadEvalCase("MT-119", "List project audit events for project `my-org/tools/gitlab-mcp-server` created during January 2026.", readStep("gitlab", "audit_event.list_project", params("project_id"), params("created_after", "created_before", "per_page"))),
		baseEnterpriseReadEvalCase("MT-122", "Download dependency list export ID `987`.", readStep("gitlab", "dependency.export_download", params("export_id"), nil)),
		baseEnterpriseReadEvalCase("MT-123", "Get group DORA lead time metrics for group `my-org` from `2026-01-01` to `2026-01-31`.", readStep("gitlab", "dora_metrics.group", params("group_id", "metric"), params("start_date", "end_date", "interval", "environment_tiers"))),
		baseEnterpriseReadEvalCase("MT-124", "Get enterprise user ID `55` in group `my-org`.", readStep("gitlab", "enterprise_user.get", params("group_id", "user_id"), nil)),
		baseEnterpriseReadEvalCase("MT-129", "Get Geo site ID `3`.", readStep("gitlab", "geo.get", params("id"), nil)),
		baseEnterpriseReadEvalCase("MT-132", "Count issues in group analytics for group path `my-org`.", readStep("gitlab", "group.analytics_issues_count", params("group_path"), nil)),
		baseEnterpriseReadEvalCase("MT-133", "List group personal access tokens for group `my-org`, filtering active tokens.", readStep("gitlab", "group.credential_list_pats", params("group_id"), params("state", "per_page"))),
		baseEnterpriseReadEvalCase("MT-135", "List epic boards for group `my-org`.", readStep("gitlab", "group.epic_board_list", params("group_id"), params("per_page"))),
		baseEnterpriseReadEvalCase("MT-136", "List epics in group full path `my-org` including descendant groups.", readStep("gitlab", "group.epic_list", params("full_path"), params("include_descendants", "state", "first"))),
		baseEnterpriseReadEvalCase("MT-159", "Get SCIM identity UID `external-123` for group `my-org`.", readStep("gitlab", "group_scim.get", params("group_id", "uid"), nil)),
		baseEnterpriseReadEvalCase("MT-161", "List group iterations for group `my-org`.", readStep("gitlab", "issue.iteration_list_group", params("group_id"), params("state", "per_page"))),
		baseEnterpriseReadEvalCase("MT-162", "List project iterations for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "issue.iteration_list_project", params("project_id"), params("state", "per_page"))),
		baseEnterpriseReadEvalCase("MT-166", "Get merge train entry for merge request IID `7` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "merge_train.get", params("project_id", "merge_request_iid"), nil)),
		baseEnterpriseReadEvalCase("MT-172", "Schedule a repository storage move for numeric project ID `123` to shard `default`.", readStep("gitlab", "storage_move.schedule_project", params("project_id"), params("destination_storage_name"))),
		baseEnterpriseReadEvalCase("MT-173", "Get group storage move ID `77` for numeric group ID `123`.", readStep("gitlab", "storage_move.get_group_for_group", params("group_id", "id"), nil)),
		baseEnterpriseReadEvalCase("MT-174", "Schedule a storage move for numeric snippet ID `44` to shard `default`.", readStep("gitlab", "storage_move.schedule_snippet", params("snippet_id"), params("destination_storage_name"))),
		baseEnterpriseReadEvalCase("MT-176", "Get vulnerability GID `gid://gitlab/Vulnerability/42`.", readStep("gitlab", "vulnerability.get", params("id"), nil)),
		baseEnterpriseReadEvalCase("MT-177", "Dismiss vulnerability GID `gid://gitlab/Vulnerability/42` as false positive with a comment.", readStep("gitlab", "vulnerability.dismiss", params("id"), params("dismissal_reason", "comment"))),
		baseEnterpriseReadEvalCase("MT-178", "Get the pipeline security summary for pipeline IID `12345` in project path `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "vulnerability.pipeline_security_summary", params("project_path", "pipeline_iid"), nil)),
		baseEnterpriseReadEvalCase("MT-180", "List project service accounts in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "project.service_account_list", params("project_id"), params("order_by", "sort", "per_page"))),
		baseEnterpriseReadEvalCase("MT-184", "List personal access tokens for project service account user ID `55` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab", "project.service_account_pat_list", params("project_id", "service_account_id"), params("state", "search", "per_page"))),
		baseEnterpriseReadEvalCase("MT-188", "Get project security settings for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_project", "security_settings_get", params("project_id"), nil)),
		baseEnterpriseReadEvalCase("MT-189", "List project service accounts in project `my-org/tools/gitlab-mcp-server` with one result per page.", readStep("gitlab_project", "service_account_list", params("project_id"), params("per_page"))),
		baseEnterpriseReadEvalCase("MT-190", "List group protected branch rules for group `my-org`.", readStep("gitlab_group", "protected_branch_list", params("group_id"), params("search", "per_page"))),
		baseEnterpriseReadEvalCase("MT-191", "List group protected environments for group `my-org`.", readStep("gitlab_group", "protected_env_list", params("group_id"), params("per_page"))),
		baseEnterpriseReadEvalCase(
			"MS-010", "Build a group compliance snapshot for group `my-org`: list top-level groups, get group `my-org`, list group audit events, then fetch the compliance policy configuration.",
			readStep("gitlab_group", "list", nil, params("top_level_only")),
			readStep("gitlab_group", "get", params("group_id"), nil),
			readStep("gitlab_audit_event", "list_group", params("group_id"), params("created_after", "created_before")),
			readStep("gitlab_compliance_policy", "get", nil, nil),
		),
		baseEnterpriseReadEvalCase(
			"MS-044", "Build an Enterprise read-only inventory for project `my-org/tools/gitlab-mcp-server` and group `my-org`: get project security settings, list project service accounts, list group protected branches, list group protected environments, then list group epic boards.",
			readStep("gitlab_project", "security_settings_get", params("project_id"), nil),
			readStep("gitlab_project", "service_account_list", params("project_id"), params("per_page")),
			readStep("gitlab_group", "protected_branch_list", params("group_id"), params("per_page")),
			readStep("gitlab_group", "protected_env_list", params("group_id"), params("per_page")),
			readStep("gitlab_group", "epic_board_list", params("group_id"), params("per_page")),
		),

		// MS-ENT-DYN-1..8 — Enterprise + dynamic surface cases. Each
		// stresses a distinct model capability on the GitLab EE
		// surface: multi-tool synthesis, project-id inference, complex
		// filtering, cross-namespace traversal, parameter
		// disambiguation, temporal reasoning, and capability-driven
		// discovery. All are read-only and resolved through
		// gitlab_find_action / gitlab_execute_action.
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-1",
			"Give me the security posture of project `my-org/tools/gitlab-mcp-server` in one go: confirm whether secret-push protection is on, count the project's service accounts, and pull the most recent two project audit events with their action and timestamp.",
			readStep("gitlab_project", "security_settings_get", params("project_id"), nil),
			readStep("gitlab_project", "service_account_list", params("project_id"), params("per_page")),
			readStep("gitlab_audit_event", "list_project", params("project_id"), params("per_page")),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-2",
			"First look up project `my-org/tools/gitlab-mcp-server` with project.get to confirm it exists, then pull the deployment-frequency DORA metric for that project over the last 30 days. Today is 2026-06-04, so compute the start_date and end_date as YYYY-MM-DD strings and pass them; interval is `daily`. Do not use a `days` or `days_back` parameter — only the computed start_date and end_date are accepted.",
			readStep("gitlab_project", "get", params("project_id"), nil),
			readStep("gitlab_dora_metrics", "project", params("project_id", "metric", "start_date", "end_date", "interval"), nil),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-3",
			"For group `my-org`, give me a quick governance view: how many custom member roles are defined at the instance level, the active enterprise users in the group, and the most recent group audit event. For member roles on self-managed GitLab, query the instance-level list (the group-level list is deprecated on self-managed 17+).",
			// gitlab_member_role.list_group returns 400 on self-managed
			// GitLab 17+ (group-level custom roles are deprecated; the
			// remaining API surface is list_instance). Switch the prompt
			// and expected step to list_instance so the case works on
			// the EE fixture without requiring a deprecation override.
			readStep("gitlab_member_role", "list_instance", nil, nil),
			readStep("gitlab_enterprise_user", "list", params("group_id"), params("active", "per_page")),
			readStep("gitlab_audit_event", "list_group", params("group_id"), params("per_page")),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-4",
			"For project `my-org/tools/gitlab-mcp-server`: first get the count of critical and high severity vulnerabilities (the dedicated severity-count endpoint, not a paginated list), then list the project's vulnerabilities using GraphQL-style pagination (pass `first=5` for the page size — do not pass `per_page`, that param is for REST-only endpoints).",
			readStep("gitlab_vulnerability", "severity_count", params("project_path"), nil),
			readStep("gitlab_vulnerability", "list", params("project_path"), params("state", "first")),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-5",
			"List every project alias on this GitLab instance, then pick the first alias from the list and fetch its full details with project_alias.get (passing the `name` you read from the list output). Report the alias name and the project_id it points to.",
			// gitlab_project_alias.list has no per_page in its schema
			// (the client-go v2 ListProjectAliases entry point takes
			// only RequestOptionFuncs and no ListOptions). Drop it from
			// the expected step until the action grows paginated input.
			readStep("gitlab_project_alias", "list", nil, nil),
			readStep("gitlab_project_alias", "get", params("name"), nil),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-6",
			"In project `my-org/tools/gitlab-mcp-server`, fetch the project audit events that occurred during January 2026 (use `2026-01-01T00:00:00Z` and `2026-02-01T00:00:00Z` for the date filters), paginated to 50 per page.",
			readStep("gitlab_audit_event", "list_project", params("project_id"), params("created_after", "created_before", "per_page")),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-7",
			"For group `my-org`: first list the open epics using GraphQL-style pagination (pass `first=5` for the page size — do not pass `per_page`, that param is for REST-only endpoints), then fetch the current group iterations in any state.",
			readStep("gitlab_group", "epic_list", params("full_path"), params("state", "first")),
			readStep("gitlab", "issue.iteration_list_group", params("group_id"), params("per_page")),
		),
		baseEnterpriseReadEvalCase(
			"MS-ENT-DYN-8",
			"List the Geo sites configured on this GitLab instance (Geo may be unconfigured in the fixture — explain the result either way), then list all snippet repository storage moves recorded on the instance.",
			// The Geo API on a single-node GitLab EE fixture has no
			// primary Geo site registered, so geo.get by id is
			// inherently 404. Use list (which returns an empty array
			// on an unconfigured instance, a perfectly valid response)
			// instead of fixing a specific id. Snippet storage moves
			// use retrieve_all_snippet (no snippet_id required) so the
			// case does not depend on a specific snippet fixture.
			readStep("gitlab_geo", "list", nil, nil),
			readStep("gitlab_storage_move", "retrieve_all_snippet", nil, params("per_page")),
		),
	}
}

func baseEnterpriseReadEvalCase(id, prompt string, steps ...Step) Case {
	// MT-188 through MT-198 are EnterpriseDockerFixture cases and need both presets
	presets := []string{presetSchemaEnterprise}
	hasDockerFixture := (id >= "MT-188" && id <= "MT-191") || id == "MS-044"
	// MS-ENT-DYN-* are Enterprise + dynamic-surface cases that
	// stress model discovery / multi-tool synthesis against the live
	// GitLab EE runtime, so they require the docker-enterprise-read
	// preset in addition to the schema-only run.
	if hasDockerFixture || isEnterpriseDynamicCase(id) {
		presets = append(presets, presetDockerEnterpriseRead)
	}
	return Case{
		ID:          id,
		Prompt:      prompt,
		Steps:       steps,
		Edition:     editionEnterprise,
		Presets:     presets,
		Partition:   partitionEnterpriseRead,
		ReportGroup: partitionEnterpriseRead,
	}
}
