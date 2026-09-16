package modelcorpus

// The read partition: cases whose every step reads, so an attempt changes
// nothing and the shared world serves any number of them at once.
//
// The texts are the ones a person would write, ported from the corpus as the
// rewrite of issue 778 left it, with the values the old text spelled out
// replaced by the fact its recipe produces. Four kinds of change were made
// while porting, each for a reason the corpus gate states:
//
//   - A value the prompt does not spell is not an argument truth. The old key
//     required the parameter name and compared nothing, which is how a case
//     came to expect a value the model was never given.
//   - A phrase that is an argument name read as English ("snippet ID",
//     "deployment ID", "group path") is rewritten where the sentence is as
//     natural without it, and declared in declarations.go where it is not.
//   - The five cases that called an evaluator-built bridge tool are gone:
//     asking the MCP client for a resource, a prompt or a completion is not a
//     request to this server, and the end-to-end suite already asks whether
//     the server answers those on every push. R03 writes them down as retired.
//   - Nothing carries a fixture value any more: a prompt interpolates the
//     fact, so the same case is about the same thing on a world it did not
//     build.
//
// The maintainability index is waived below because what it measures here is
// how many cases the corpus has, which is the thing this file is for: the
// function branches nowhere, and splitting it would put each case in one half
// or the other by a rule chosen to satisfy a number.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1; see above.
func readCases() []Case {
	return []Case{
		{
			ID:     "MT-001",
			Prompt: "Show the current authenticated GitLab user.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("user.current")}},
		},
		{
			ID:     "MT-002",
			Prompt: "Find project `{{ .Facts.project_path }}` and give me its ID and default branch.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("project.get", project())}},
		},
		{
			ID:     "MT-003",
			Prompt: "List the 10 most recently updated projects I can access.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("project.list", opt("per_page", literal("10")))}},
		},
		{
			ID:     "MT-005",
			Prompt: "List members of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("project.members", project())}},
		},
		{
			ID:     "MT-006",
			Prompt: "List top-level groups only.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("group.list")}},
		},
		{
			ID:     "MT-009",
			Prompt: "List open issues in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("issue.list", project())}},
		},
		{
			ID:     "MT-014",
			Prompt: "List merge requests opened against `{{ .Facts.default_branch }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("merge_request.list", project(), opt("target_branch", fact(FactDefaultBranch))),
			}},
		},
		{
			ID:     "MT-018",
			Prompt: "List the latest pipelines on branch `{{ .Facts.default_branch }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("pipeline.list", project(), opt("ref", fact(FactDefaultBranch))),
			}},
		},
		{
			ID:     "MT-021",
			Prompt: "List failed jobs in pipeline `{{ .Facts.pipeline_id }}` for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeFailedJob,
			Needs:  Needs{Runner: true},
			key: Key{Steps: []Step{
				step("job.list", project(), req("pipeline_id", fact(FactPipelineID)), opt("scope", literal("failed"))),
			}},
		},
		{
			ID:     "MT-022",
			Prompt: "Get the trace for job `{{ .Facts.job_id }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeFailedJob,
			Needs:  Needs{Runner: true},
			key:    Key{Steps: []Step{step("job.trace", project(), req("job_id", fact(FactJobID)))}},
		},
		{
			ID:     "MT-025",
			Prompt: "List CI variables in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("ci_variable.list", project())}},
		},
		{
			ID:     "MT-029",
			Prompt: "Get file `README.md` from ref `{{ .Facts.default_branch }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("repository.file_get", project(), req("file_path", literal("README.md")), req("ref", fact(FactDefaultBranch))),
			}},
		},
		{
			ID:     "MT-032",
			Prompt: "Search the code in project `{{ .Facts.project_path }}` for `RegisterMCPMeta`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("search.code", project(), req("query", literal("RegisterMCPMeta")))}},
		},
		{
			ID:     "MT-033",
			Prompt: "Search all projects for `gitlab-mcp-server`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("search.projects", req("query", literal("gitlab-mcp-server")))}},
		},
		{
			ID:     "MT-038",
			Prompt: "List deploy keys for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("access.deploy_key_list_project", project())}},
		},
		{
			ID:     "MT-040",
			Prompt: "Check that the MCP server can reach GitLab.",
			Recipe: RecipeWorld,
			Surfaces: Restrict{
				Only: []Surface{SurfaceDynamic, SurfaceMeta},
				Reason: "server.health_check declares no individual tool of its own: it shares its " +
					"handler with server.status, which is the action the individual surface publishes",
			},
			key: Key{Steps: []Step{step("server.health_check")}},
		},
		{
			ID:     "MT-043",
			Prompt: "List generic packages in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("package.list", project(), opt("package_type", literal("generic"))),
			}},
		},
		{
			ID:     "MT-045",
			Prompt: "List online project runners for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("runner.list_project", project(), opt("status", literal("online"))),
			}},
		},
		{
			ID:     "MT-048",
			Prompt: "List available environments in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("environment.list", project())}},
		},
		{
			ID:     "MT-050",
			Prompt: "Get the raw content of personal snippet `{{ .Facts.snippet_id }}`.",
			Recipe: RecipeSnippet,
			key:    Key{Steps: []Step{step("snippet.content", req("snippet_id", fact(FactSnippetID)))}},
		},
		{
			ID:     "MT-052",
			Prompt: "Show instance application settings.",
			Recipe: RecipeWorld,
			Needs:  Needs{Admin: true},
			key:    Key{Steps: []Step{step("admin.settings_get")}},
		},
		{
			ID:     "MT-065",
			Prompt: "Download artifact `{{ .Facts.artifact_path }}` from job `{{ .Facts.job_id }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeFailedJob,
			Needs:  Needs{Runner: true},
			key: Key{Steps: []Step{
				step("job.download_single_artifact", project(),
					req("job_id", fact(FactJobID)), req("artifact_path", fact(FactArtifactPath))),
			}},
		},
		{
			ID: "MS-ENV-DEP-1",
			Prompt: "Get environment `{{ .Facts.environment_name }}` (ID `{{ .Facts.environment_id }}`) in " +
				"project `{{ .Facts.project_path }}` and report its last deployment's commit SHA and status.",
			Recipe: RecipeEnvironment,
			key: Key{Steps: []Step{
				step("environment.get", project(), req("environment_id", fact(FactEnvironmentID))),
			}},
		},
		{
			ID:     "MS-ENV-DEP-2",
			Prompt: "Get deployment `{{ .Facts.deployment_id }}` in project `{{ .Facts.project_path }}` and report its ref and status.",
			Recipe: RecipeEnvironment,
			key: Key{Steps: []Step{
				step("environment.deployment_get", project(), req("deployment_id", fact(FactDeploymentID))),
			}},
		},
		{
			ID:     "MT-071",
			Prompt: "List branches in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("branch.list", project())}},
		},
		{
			ID:     "MT-072",
			Prompt: "List CI/CD catalog resources.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("ci_catalog.list")}},
		},
		{
			ID:     "MT-073",
			Prompt: "List custom emoji for group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("custom_emoji.list", req("group_path", fact(FactGroupPath)))}},
		},
		{
			ID:     "MT-077",
			Prompt: "List feature flags in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("feature_flags.feature_flag_list", project())}},
		},
		{
			ID:     "MT-090",
			Prompt: "List available Dockerfile templates.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("template.dockerfile_list")}},
		},
		{
			ID:     "MT-092",
			Prompt: "List wiki pages in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("wiki.list", project())}},
		},
		{
			ID: "MT-179",
			Prompt: "Inspect the changes of merge request `{{ .Facts.merge_request_iid }}` " +
				"in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeMergeRequest,
			key: Key{Steps: []Step{
				step("mr_review.changes_get", project(), req("merge_request_iid", fact(FactMergeRequestIID))),
			}},
		},
		{
			ID: "MT-199",
			Prompt: "List merged merge requests in project `{{ .Facts.project_path }}` ordered by updated date, " +
				"with 5 results per page.",
			Recipe: RecipeMergeRequest,
			key: Key{Steps: []Step{
				step("merge_request.list", project(), opt("state", literal("merged")), opt("per_page", literal("5"))),
			}},
		},
		{
			ID:     "MT-200",
			Prompt: "Get all pending to-do items for the current user.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("user.todo_list", opt("state", literal("pending")))}},
		},
		{
			ID: "MT-204",
			Prompt: "List issues with status `closed` created in the last 60 days in project " +
				"`{{ .Facts.project_path }}`, ordered by creation date.",
			Recipe: RecipeIssue,
			key:    Key{Steps: []Step{step("issue.list", project(), opt("state", literal("closed")))}},
		},
		{
			ID: "MT-205",
			Prompt: "For project `{{ .Facts.project_path }}`, list the pipelines on branch " +
				"`{{ .Facts.default_branch }}`, then list the jobs of pipeline `{{ .Facts.pipeline_id }}` " +
				"with their statuses.",
			Recipe: RecipePipelineJob,
			Needs:  Needs{Runner: true},
			key: Key{Steps: []Step{
				step("pipeline.list", project(), opt("ref", fact(FactDefaultBranch))),
				step("job.list", project(), req("pipeline_id", fact(FactPipelineID))),
			}},
		},
		{
			ID:     "MT-206",
			Prompt: "Find issues labeled `bug` and in milestone `v2.0` for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeIssue,
			key: Key{Steps: []Step{
				step("issue.list", project(), opt("labels", literal("bug")), opt("milestone", literal("v2.0"))),
			}},
		},
		{
			ID:     "MT-207",
			Prompt: "Get the second page of releases (100 per page) for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeRelease,
			key:    Key{Steps: []Step{step("release.list", project(), opt("per_page", literal("100")))}},
		},
		{
			ID:     "MT-208",
			Prompt: "Find the branches of project `{{ .Facts.project_path }}` whose name starts with `feat`.",
			Recipe: RecipeBranch,
			key:    Key{Steps: []Step{step("branch.list", project(), opt("search", literal("feat")))}},
		},
		{
			ID:     "MT-209",
			Prompt: "List the members of project `{{ .Facts.project_path }}` with their roles.",
			Recipe: RecipeMember,
			key:    Key{Steps: []Step{step("project.members", project())}},
		},
		{
			ID: "MS-001",
			Prompt: "Resolve remote URL `{{ .Facts.remote_url }}` for project `{{ .Facts.project_path }}`, " +
				"verify the project metadata, then read `README.md` from `{{ .Facts.default_branch }}`.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				standalone("gitlab_discover_project", req("remote_url", fact(FactRemoteURL))),
				step("project.get", project()),
				step("repository.file_get", project(),
					req("file_path", literal("README.md")), req("ref", fact(FactDefaultBranch))),
			}},
		},
		{
			ID: "MS-002",
			Prompt: "Investigate failed pipeline `{{ .Facts.pipeline_id }}` for project " +
				"`{{ .Facts.project_path }}` and remote URL `{{ .Facts.remote_url }}`: first resolve that exact " +
				"remote URL to the project, inspect the pipeline, list the failed jobs, then fetch the trace of " +
				"job `{{ .Facts.job_id }}`.",
			Recipe: RecipeFailedJob,
			Needs:  Needs{Runner: true},
			key: Key{Steps: []Step{
				standalone("gitlab_discover_project", req("remote_url", fact(FactRemoteURL))),
				step("pipeline.get", project(), req("pipeline_id", fact(FactPipelineID))),
				step("job.list", project(), req("pipeline_id", fact(FactPipelineID)), opt("scope", literal("failed"))),
				step("job.trace", project(), req("job_id", fact(FactJobID))),
			}},
		},
		{
			ID: "MS-037",
			Prompt: "Build a broad read-only inventory of project `{{ .Facts.project_path }}`: get the project, " +
				"list branches, list tags, list releases, list the repository tree at " +
				"`{{ .Facts.default_branch }}`, list the project CI variables, list deploy keys, list deploy " +
				"tokens, then list generic packages.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("project.get", project()),
				step("branch.list", project()),
				step("tag.list", project()),
				step("release.list", project()),
				step("repository.tree", project(), opt("ref", fact(FactDefaultBranch))),
				step("ci_variable.list", project()),
				step("access.deploy_key_list_project", project()),
				step("access.deploy_token_list_project", project()),
				step("package.list", project(), opt("package_type", literal("generic"))),
			}},
		},
	}
}
