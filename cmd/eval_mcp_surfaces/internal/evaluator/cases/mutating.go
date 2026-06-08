package cases

func mutatingEvalCases() []Case {
	return []Case{
		baseMutatingEvalCase("MT-004", "Star project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_project", "star", params("project_id"), nil)),
		baseMutatingEvalCase("MT-007", "Create a subgroup named `eval-temp` with path `eval-temp` under group ID `123` (`my-org`).", readStep("gitlab_group", "create", params("name", "path", "parent_id"), params("visibility"))),
		baseMutatingEvalCase("MT-010", "Create an issue titled `Evaluate schema discovery` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_issue", "create", params("project_id", "title"), params("description", "labels"))),
		baseMutatingEvalCase("MT-011", "Update issue `42` in project `my-org/tools/gitlab-mcp-server` to add label `evaluation`.", readStep("gitlab_issue", "update", params("project_id", "issue_iid", "labels"), nil)),
		baseMutatingEvalCase("MT-012", "Close issue `42` in project `my-org/tools/gitlab-mcp-server` by setting `state_event` to `close`.", readStep("gitlab_issue", "update", params("project_id", "issue_iid", "state_event"), nil)),
		baseMutatingEvalCase("MT-015", "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval` into `main` titled `Evaluation MR`.", readStep("gitlab_merge_request", "create", params("project_id", "source_branch", "target_branch", "title"), params("description", "remove_source_branch"))),
		baseMutatingEvalCase("MT-016", "Add a note saying `Can we add coverage?` to merge request `7` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_mr_review", "note_create", params("project_id", "merge_request_iid", "body"), nil)),
		baseMutatingEvalCase("MT-019", "Create a new pipeline on branch `main` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_pipeline", "create", params("project_id", "ref"), params("variables"))),
		baseMutatingEvalCase("MT-020", "Cancel pipeline `12345` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_pipeline", "cancel", params("project_id", "pipeline_id"), nil)),
		baseMutatingEvalCase("MT-023", "Retry job `999` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_job", "retry", params("project_id", "job_id"), nil)),
		baseMutatingEvalCase("MT-026", "Create masked CI variable `EVAL_TOKEN` with value `masked-value-123` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_ci_variable", "create", params("project_id", "key", "value"), params("masked", "protected"))),
		baseMutatingEvalCase("MT-027", "Update CI variable `EVAL_TOKEN` to value `masked-value-456` with environment_scope `production` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_ci_variable", "update", params("project_id", "key", "value", "environment_scope"), nil)),
		baseMutatingEvalCase("MT-030", "Create file `tmp/eval.txt` with content `evaluation file` and commit_message `Create evaluation file` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_repository", "file_create", params("project_id", "file_path", "branch", "content", "commit_message"), nil)),
		baseMutatingEvalCase("MT-034", "Create milestone with title `Evaluation Sprint` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_project", "milestone_create", params("project_id", "title"), params("due_date", "description"))),
		baseMutatingEvalCase("MT-036", "Create release with tag_name `v0.0.0-eval`, ref `main`, and name `v0.0.0-eval` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_release", "create", params("project_id", "tag_name", "ref"), params("name", "description"))),
		baseMutatingEvalCase("MT-041", "Create project access token `eval-token` for project `my-org/tools/gitlab-mcp-server` with `read_api` scope expiring `2026-12-31`.", readStep("gitlab_access", "token_project_create", params("project_id", "name", "scopes"), params("expires_at"))),
		baseMutatingEvalCase("MT-046", "Set paused=true on runner ID `99`.", readStep("gitlab_runner", "update", params("runner_id", "paused"), nil)),
		baseMutatingEvalCase("MT-053", "Create a banner broadcast message saying `Evaluation maintenance` from `2026-01-01T00:00:00Z` to `2026-01-01T01:00:00Z`.", readStep("gitlab_admin", "broadcast_message_create", params("message"), params("starts_at", "ends_at", "broadcast_type", "dismissable"))),
		baseMutatingEvalCase("MT-056", "Add webhook `https://example.com/gitlab-hook` to project `my-org/tools/gitlab-mcp-server` for push events.", readStep("gitlab_project", "hook_add", params("project_id", "url"), params("push_events", "enable_ssl_verification"))),
		baseMutatingEvalCase("MT-058", "Add a coverage badge to project `my-org/tools/gitlab-mcp-server` with link_url `https://example.com/coverage` and image_url `https://example.com/badge.svg`.", readStep("gitlab_project", "badge_add", params("project_id", "link_url", "image_url"), nil)),
		baseMutatingEvalCase("MT-060", "Create a merge request discussion on MR `7` in project `my-org/tools/gitlab-mcp-server` asking `Can we add coverage?`.", readStep("gitlab_mr_review", "discussion_create", params("project_id", "merge_request_iid", "body"), params("position"))),
		baseMutatingEvalCase("MT-061", "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7` in project `my-org/tools/gitlab-mcp-server` (project_id `my-org/tools/gitlab-mcp-server`) by setting `resolved` to true.", readStep("gitlab_mr_review", "discussion_resolve", params("project_id", "merge_request_iid", "discussion_id", "resolved"), nil)),
		baseMutatingEvalCase("MT-062", "Create a draft review note on MR `7` in project `my-org/tools/gitlab-mcp-server` saying `Please add a regression test`.", readStep("gitlab_mr_review", "draft_note_create", params("project_id", "merge_request_iid", "note"), params("position"))),
		baseMutatingEvalCase("MT-064", "Play manual job `999` in project `my-org/tools/gitlab-mcp-server` with variable `DEPLOY_ENV=staging`.", readStep("gitlab_job", "play", params("project_id", "job_id"), params("variables"))),
		baseMutatingEvalCase("MT-067", "Create group CI variable `GROUP_EVAL_TOKEN` in group `my-org` with value `masked-value-123`.", readStep("gitlab_ci_variable", "group_create", params("group_id", "key", "value"), params("masked", "environment_scope"))),
		baseMutatingEvalCase("MT-068", "Create instance CI variable `INSTANCE_EVAL_TOKEN` with value `masked-value-123`.", readStep("gitlab_ci_variable", "instance_create", params("key", "value"), params("masked", "protected"))),
		baseMutatingEvalCase("MT-080", "Start the guided issue creation flow for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_interactive_issue_create", "", params("project_id"), nil)),
		baseMutatingEvalCase("MT-081", "Start the guided merge request creation flow for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_interactive_mr_create", "", params("project_id"), nil)),
		baseMutatingEvalCase("MT-082", "Start the guided project creation flow.", readStep("gitlab_interactive_project_create", "", nil, params("project_id"))),
		baseMutatingEvalCase("MT-083", "Start the guided release creation flow for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_interactive_release_create", "", params("project_id"), nil)),
		baseMutatingEvalCase(
			"MS-008", "Troubleshoot runner ID `99` for project `my-org/tools/gitlab-mcp-server`: list project runners, inspect runner jobs, fetch trace for job `999`, then set paused=true on the runner.",
			readStep("gitlab_runner", "list_project", params("project_id"), params("status")),
			readStep("gitlab_runner", "jobs", params("runner_id"), params("status")),
			readStep("gitlab_job", "trace", params("project_id", "job_id"), nil),
			readStep("gitlab_runner", "update", params("runner_id", "paused"), nil),
		),
		baseMutatingEvalCase(
			"MS-011", "Resolve remote URL `https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git`, then start guided issue creation for the resolved project `my-org/tools/gitlab-mcp-server`.",
			readStep("gitlab_discover_project", "", params("remote_url"), nil),
			readStep("gitlab_interactive_issue_create", "", params("project_id"), nil),
		),
		baseMutatingEvalCase(
			"MS-038", "Publish the local fixture files `__PACKAGE_RELEASE_FILES__` from directory `__PACKAGE_RELEASE_DIR__` to GitLab Generic Packages in project `my-org/tools/gitlab-mcp-server` as package `__PACKAGE_RELEASE_PACKAGE__` version `__PACKAGE_RELEASE_VERSION__`, then create release `__PACKAGE_RELEASE_TAG__` from ref `main` named `Evaluation package release`, and link each uploaded package file to that release as a package asset. Upload the package files first, then generate the release, then link the returned package URLs; do not construct package URLs manually.",
			readStep("gitlab_package", "publish_directory", params("project_id", "package_name", "package_version", "directory_path"), params("include_pattern")),
			readStep("gitlab_release", "create", params("project_id", "tag_name", "ref"), params("name", "description")),
			readStep("gitlab_release", "link_create_batch", params("project_id", "tag_name", "links"), nil),
		),
	}
}

func baseMutatingEvalCase(id, prompt string, steps ...Step) Case {
	evalCase := Case{
		ID:          id,
		Prompt:      prompt,
		Steps:       steps,
		Edition:     editionCE,
		Presets:     []string{presetDockerMutatingSafe},
		Partition:   partitionBaseMutating,
		Mutating:    true,
		ReportGroup: partitionBaseMutating,
	}
	if template, fixtures := baseMutatingPromptTemplateAndFixtures(id); template != "" {
		evalCase.PromptTemplate = PromptTemplate{Text: template}
		evalCase.Fixtures = fixtures
	}
	return evalCase
}

//nolint:gocyclo // Keeping the ID-to-fixture mapping in one switch makes the migration table auditable.
func baseMutatingPromptTemplateAndFixtures(id string) (template string, fixtures []string) {
	switch id {
	case "MT-007":
		return "Create a subgroup named `{{ .Values.subgroup_name }}` with path `{{ .Values.subgroup_path }}` under group ID `{{ .Group.ID }}` (`my-org`).", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MT-011":
		return "Update issue `{{ .Issue.IID }}` in project `{{ .Project.Path }}` to add label `evaluation`.", []string{fixtureIssue}
	case "MT-012":
		return "Close issue `{{ .Issue.IID }}` in project `{{ .Project.Path }}` by setting `state_event` to `close`.", []string{fixtureIssue}
	case "MT-015":
		return "Create a merge request in project `{{ .Project.Path }}` from `{{ .Values.mr_source_branch }}` into `{{ .Branch.Default }}` titled `{{ .Values.mr_title }}`.", []string{fixtureMergeRequestSource}
	case "MT-016":
		return "Add a note saying `Can we add coverage?` to merge request `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}`.", []string{fixtureMergeRequest}
	case "MT-020":
		return "Cancel pipeline `{{ .Pipeline.ID }}` in project `{{ .Project.Path }}`.", []string{fixturePipelineJob}
	case "MT-023":
		return "Retry job `{{ .Job.ID }}` in project `{{ .Project.Path }}`.", []string{fixtureFailedJobArtifact}
	case "MT-026":
		return "Create masked CI variable `{{ .Values.ci_variable_key }}` with value `masked-value-123` in project `{{ .Project.Path }}`.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case taskFileCreateID:
		return "Create file `{{ .Values.file_path }}` with content `evaluation file` and commit_message `Create evaluation file` on branch `{{ .Values.feature_branch }}` in project `{{ .Project.Path }}`.", []string{fixtureBranch, fixtureAttemptNames}
	case "MT-034":
		return "Create milestone with title `{{ .Values.milestone_title }}` in project `{{ .Project.Path }}`.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MT-036":
		return "Create release with tag_name `{{ .Release.TagName }}`, ref `{{ .Branch.Default }}`, and name `{{ .Release.Name }}` in project `{{ .Project.Path }}`.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MT-046":
		return "Set paused=true on runner ID `{{ .Runner.ID }}`.", []string{fixtureRunnerRemove}
	case "MT-060":
		return "Create a merge request discussion on MR `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}` asking `Can we add coverage?`.", []string{fixtureMergeRequest}
	case "MT-061":
		return "Resolve merge request discussion with discussion_id `{{ .Values.discussion_id }}` on merge_request_iid `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}` by setting `resolved` to true.", []string{fixtureMergeRequestDiscussion}
	case "MT-062":
		return "Create a draft review note on MR `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}` saying `Please add a regression test`.", []string{fixtureMergeRequest}
	case "MT-064":
		return "Play manual job `{{ .Values.manual_job_id }}` in project `{{ .Project.Path }}` with variable `DEPLOY_ENV=staging`.", []string{fixturePipelineJob}
	case "MT-067":
		return "Create group CI variable `{{ .Values.group_ci_variable_key }}` in group `{{ .Group.Path }}` with value `masked-value-123`.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MT-068":
		return "Create instance CI variable `{{ .Values.instance_ci_variable_key }}` with value `masked-value-123`.", []string{fixtureAttemptNames}
	case "MT-081":
		return "Start the guided merge request creation flow for project `{{ .Project.Path }}`. Do not pass `source_branch`, `target_branch`, or `title` as tool parameters; the guided prompts will use source branch `{{ .Values.mr_source_branch }}`, target branch `{{ .Branch.Default }}`, and title `{{ .Values.mr_title }}`.", []string{fixtureMergeRequestSource}
	case "MT-083":
		return "Start the guided release creation flow for project `{{ .Project.Path }}`. Do not pass `tag_name` or `name` as tool parameters; the guided prompts will use tag `{{ .Release.TagName }}` and release name `{{ .Release.Name }}`.", []string{fixtureReleaseCreateSource}
	case "MS-008":
		return "Troubleshoot runner ID `{{ .Runner.ID }}` for project `{{ .Project.Path }}`: list project runners, inspect runner jobs, fetch trace for job `{{ .Job.ID }}`, then set paused=true on the runner.", []string{fixtureFailedJobArtifact, fixtureRunnerRemove}
	case taskPackageReleaseID:
		return "Publish the local fixture files `{{ .Values.package_release_files_display }}` from directory `{{ .Values.package_release_dir }}` to GitLab Generic Packages in project `{{ .Project.Path }}` as package `{{ .Values.package_release_name }}` version `{{ .Values.package_release_version }}`, then create release `{{ .Values.package_release_tag }}` from ref `{{ .Branch.Default }}` named `Evaluation package release`, and link each uploaded package file to that release as a package asset. Upload the package files first, then generate the release, then link the returned package URLs; do not construct package URLs manually.", []string{fixturePackageRelease, fixtureAttemptNames}
	default:
		return "", nil
	}
}
