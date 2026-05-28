package cases

func readEvalCases() []Case {
	return []Case{
		baseReadEvalCase("MT-001", "Show the current authenticated GitLab user.", readStep("gitlab_user", "current", nil, nil)),
		baseReadEvalCase("MT-002", "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.", readStep("gitlab_project", "get", params("project_id"), nil)),
		baseReadEvalCase("MT-003", "List the 10 most recently updated projects I can access.", readStep("gitlab_project", "list", nil, params("order_by", "sort", "per_page"))),
		baseReadEvalCase("MT-005", "List members of project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_project", "members", params("project_id"), params("per_page"))),
		baseReadEvalCase("MT-006", "List top-level groups only.", readStep("gitlab_group", "list", nil, params("top_level_only", "per_page"))),
		baseReadEvalCase("MT-009", "List open issues in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_issue", "list", params("project_id"), params("state", "per_page"))),
		baseReadEvalCase("MT-014", "List merge requests opened against `main` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_merge_request", "list", params("project_id"), params("target_branch", "state", "per_page"))),
		baseReadEvalCase("MT-018", "List the latest pipelines on branch `main` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_pipeline", "list", params("project_id"), params("ref", "per_page"))),
		baseReadEvalCase("MT-021", "List failed jobs in pipeline `12345` for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_job", "list", params("project_id", "pipeline_id"), params("scope"))),
		baseReadEvalCase("MT-022", "Get the trace for job `999` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_job", "trace", params("project_id", "job_id"), nil)),
		baseReadEvalCase("MT-025", "List CI variables in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_ci_variable", "list", params("project_id"), params("page", "per_page"))),
		baseReadEvalCase("MT-029", "Get file `README.md` from ref `main` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_repository", "file_get", params("project_id", "file_path", "ref"), nil)),
		baseReadEvalCase("MT-032", "Search code inside project `my-org/tools/gitlab-mcp-server` for `func RegisterMCPMeta` using that project's `project_id`.", readStep("gitlab_search", "code", params("query", "project_id"), nil)),
		baseReadEvalCase("MT-033", "Search all projects for `gitlab-mcp-server`.", readStep("gitlab_search", "projects", params("query"), nil)),
		baseReadEvalCase("MT-038", "List deploy keys for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_access", "deploy_key_list_project", params("project_id"), params("page", "per_page"))),
		baseReadEvalCase("MT-039", "Analyze why pipeline `12345` failed in project `my-org/tools/gitlab-mcp-server` using the LLM-assisted pipeline failure analyzer.", readStep("gitlab_analyze", "pipeline_failure", params("project_id", "pipeline_id"), nil)),
		baseReadEvalCase("MT-040", "First use gitlab_find_action to locate the server health check action, then execute the GitLab connectivity check for the MCP server.", readStep("gitlab_server", "health_check", nil, nil)),
		baseReadEvalCase("MT-043", "List generic packages in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_package", "list", params("project_id"), params("package_type", "per_page"))),
		baseReadEvalCase("MT-045", "List online project runners for project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_runner", "list_project", params("project_id"), params("status"))),
		baseReadEvalCase("MT-048", "List available environments in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_environment", "list", params("project_id"), params("states"))),
		baseReadEvalCase("MT-050", "Get raw content of personal snippet ID `33`.", readStep("gitlab_snippet", "content", params("snippet_id"), nil)),
		baseReadEvalCase("MT-052", "Show instance application settings.", readStep("gitlab_admin", "settings_get", nil, nil)),
		artifactDownloadEvalCase(),
		baseReadEvalCase("MT-071", "List branches in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_branch", "list", params("project_id"), params("search", "per_page"))),
		baseReadEvalCase("MT-072", "List CI/CD catalog resources.", readStep("gitlab_ci_catalog", "list", nil, params("search", "scope", "sort"))),
		baseReadEvalCase("MT-073", "List custom emoji for group path `my-org`.", readStep("gitlab_custom_emoji", "list", params("group_path"), params("first", "after"))),
		baseReadEvalCase("MT-077", "List feature flags in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_feature_flags", "feature_flag_list", params("project_id"), params("scope", "per_page"))),
		baseReadEvalCase("MT-090", "List available Dockerfile templates.", readStep("gitlab_template", "dockerfile_list", nil, nil)),
		baseReadEvalCase("MT-092", "List wiki pages in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_wiki", "list", params("project_id"), params("with_content"))),
		baseReadEvalCase("MT-093", "Analyze merge request `7` code changes in project `my-org/tools/gitlab-mcp-server` with the LLM-assisted code review analyzer.", readStep("gitlab_analyze", "mr_changes", params("project_id", "merge_request_iid"), nil)),
		baseReadEvalCase("MT-094", "In project `my-org/tools/gitlab-mcp-server`, summarize issue `42` discussion using the `analyze.issue_summary` catalog action.", readStep("gitlab_analyze", "issue_summary", params("project_id", "issue_iid"), nil)),
		baseReadEvalCase("MT-095", "Generate release notes for project `my-org/tools/gitlab-mcp-server` from `main` to `v0.0.0-eval-ms`.", readStep("gitlab_analyze", "release_notes", params("project_id", "from", "to"), nil)),
		baseReadEvalCase("MT-096", "Run a security review of merge request `7` in project `my-org/tools/gitlab-mcp-server` using the LLM-assisted security review analyzer.", readStep("gitlab_analyze", "mr_security", params("project_id", "merge_request_iid"), nil)),
		baseReadEvalCase("MT-097", "Analyze the CI configuration for project `my-org/tools/gitlab-mcp-server` using content_ref `main`.", readStep("gitlab_analyze", "ci_config", params("project_id"), params("content_ref"))),
		baseReadEvalCase("MT-098", "Analyze technical-debt markers on branch `main` in project `my-org/tools/gitlab-mcp-server` with the LLM-assisted technical debt analyzer.", readStep("gitlab_analyze", "technical_debt", params("project_id"), params("ref"))),
		baseReadEvalCase("MT-179", "Inspect merge request `7` changes in project `my-org/tools/gitlab-mcp-server` without running an LLM analyzer.", readStep("gitlab_mr_review", "changes_get", params("project_id", "merge_request_iid"), nil)),
		baseReadEvalCase("MS-001", "Resolve remote URL `https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git` for project `my-org/tools/gitlab-mcp-server`, verify the project metadata, then read `README.md` from `main`.",
			readStep("gitlab_discover_project", "", params("remote_url"), nil),
			readStep("gitlab_project", "get", params("project_id"), nil),
			readStep("gitlab_repository", "file_get", params("project_id", "file_path", "ref"), nil),
		),
		baseReadEvalCase("MS-002", "Investigate failed pipeline `12345` for project `my-org/tools/gitlab-mcp-server` and remote URL `https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git`: first resolve that exact remote URL to the project, inspect the pipeline, list failed jobs, fetch job `999` trace, then call the pipeline failure analyzer using that same pipeline ID.",
			readStep("gitlab_discover_project", "", params("remote_url"), nil),
			readStep("gitlab_pipeline", "get", params("project_id", "pipeline_id"), nil),
			readStep("gitlab_job", "list", params("project_id", "pipeline_id"), params("scope")),
			readStep("gitlab_job", "trace", params("project_id", "job_id"), nil),
			readStep("gitlab_analyze", "pipeline_failure", params("project_id", "pipeline_id"), nil),
		),
		baseReadEvalCase("MS-012", "Prepare an LLM-assisted release summary for project `my-org/tools/gitlab-mcp-server`: inspect releases, compare refs `main` and `v0.0.0-eval-ms`, then generate release notes.",
			readStep("gitlab_release", "list", params("project_id"), params("per_page")),
			readStep("gitlab_repository", "compare", params("project_id", "from", "to"), nil),
			readStep("gitlab_analyze", "release_notes", params("project_id", "from", "to"), nil),
		),
		baseReadEvalCase("MS-037", "Build a broad read-only Docker inventory for project `my-org/tools/gitlab-mcp-server`: get the project, list branches, list tags, list releases, list the repository tree at `main`, list project CI variables, list deploy keys, list deploy tokens, then list generic packages.",
			readStep("gitlab_project", "get", params("project_id"), nil),
			readStep("gitlab_branch", "list", params("project_id"), params("per_page")),
			readStep("gitlab_tag", "list", params("project_id"), params("per_page")),
			readStep("gitlab_release", "list", params("project_id"), params("per_page")),
			readStep("gitlab_repository", "tree", params("project_id"), params("ref", "path", "per_page")),
			readStep("gitlab_ci_variable", "list", params("project_id"), params("page", "per_page")),
			readStep("gitlab_access", "deploy_key_list_project", params("project_id"), params("page", "per_page")),
			readStep("gitlab_access", "deploy_token_list_project", params("project_id"), params("page", "per_page")),
			readStep("gitlab_package", "list", params("project_id"), params("package_type", "per_page")),
		),
	}
}

func artifactDownloadEvalCase() Case {
	evalCase := baseReadEvalCase("MT-065", "Download artifact `coverage/report.xml` from job `999` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_job", "download_single_artifact", params("project_id", "job_id", "artifact_path"), nil))
	evalCase.PromptTemplate = PromptTemplate{Text: "Download artifact `{{ .Values.artifact_path }}` from job `{{ .Job.ID }}` in project `{{ .Project.Path }}`."}
	evalCase.Fixtures = []string{fixtureFailedJobArtifact}
	return evalCase
}

func baseReadEvalCase(id, prompt string, steps ...Step) Case {
	evalCase := Case{
		ID:          id,
		Prompt:      prompt,
		Steps:       steps,
		Edition:     editionCE,
		Presets:     []string{presetDockerRead},
		Partition:   partitionBaseRead,
		ReportGroup: partitionBaseRead,
	}
	if template, fixtures := baseReadPromptTemplateAndFixtures(id); template != "" {
		evalCase.PromptTemplate = PromptTemplate{Text: template}
		evalCase.Fixtures = fixtures
	}
	return evalCase
}

func baseReadPromptTemplateAndFixtures(id string) (template string, fixtures []string) {
	switch id {
	case "MT-021":
		return "List failed jobs in pipeline `{{ .Pipeline.ID }}` for project `{{ .Project.Path }}`.", []string{fixtureFailedJobArtifact}
	case "MT-022":
		return "Get the trace for job `{{ .Job.ID }}` in project `{{ .Project.Path }}`.", []string{fixtureFailedJobArtifact}
	case "MT-039":
		return "Analyze why pipeline `{{ .Pipeline.ID }}` failed in project `{{ .Project.Path }}` using the LLM-assisted pipeline failure analyzer.", []string{fixtureFailedJobArtifact}
	case "MT-050":
		return "Get raw content of personal snippet ID `{{ .Values.snippet_id }}`.", []string{fixtureSnippet}
	case "MT-093":
		return "Analyze merge request `{{ .MergeRequest.IID }}` code changes in project `{{ .Project.Path }}` with the LLM-assisted code review analyzer.", []string{fixtureMergeRequest}
	case "MT-094":
		return "In project `{{ .Project.Path }}`, summarize issue `{{ .Issue.IID }}` discussion using the `analyze.issue_summary` catalog action.", []string{fixtureIssue}
	case "MT-096":
		return "Run a security review of merge request `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}` using the LLM-assisted security review analyzer.", []string{fixtureMergeRequest}
	case "MT-179":
		return "Inspect merge request `{{ .MergeRequest.IID }}` changes in project `{{ .Project.Path }}` without running an LLM analyzer.", []string{fixtureMergeRequest}
	case "MS-002":
		return "Investigate failed pipeline `{{ .Pipeline.ID }}` for project `{{ .Project.Path }}` and remote URL `{{ .Values.remote_url }}`: first resolve that exact remote URL to the project, inspect the pipeline, list failed jobs, fetch job `{{ .Job.ID }}` trace, then call the pipeline failure analyzer using that same pipeline ID.", []string{fixtureFailedJobArtifact}
	default:
		return "", nil
	}
}

func readStep(tool, action string, requiredParams, optionalParams []string) Step {
	return Step{
		ExpectedTool:   tool,
		ExpectedAction: action,
		RequiredParams: requiredParams,
		OptionalParams: optionalParams,
	}
}

func params(names ...string) []string {
	if len(names) == 0 {
		return nil
	}
	return names
}
