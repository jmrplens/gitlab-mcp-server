package modelcorpus

// The mutating partition: cases that create or change something and destroy
// nothing. Whether a case mutates is read from the catalog at its steps'
// actions rather than declared here, which is why no field says so.
//
// Almost every one of them runs against state of the attempt's own rather than
// against the shared world, and that is what lets a prompt name a literal. The
// evaluator this replaces shared one project between attempts, so every name a
// case created had to come from a fixture that made it unique; here the world
// is the attempt's, so "Evaluation Sprint" is a title two attempts can both
// use and a reader can see what the case asks for. The two exceptions are the
// two scopes an attempt cannot have to itself: a group, which the group cases
// build for themselves, and the instance, where the CI variable name must
// still come from a fact.
//
// Where the prompt states a value as a verb rather than as a value ("pause the
// runner", "resolve the discussion"), no argument truth is declared: a Literal
// the prompt never spells is a value the model was never given, and the corpus
// gate refuses one. What the model did with that argument is checked by the
// recipe's own verification against GitLab after the attempt, which is the
// only oracle that can see it.
//
// Two texts changed beyond the four kinds of rewrite read.go lists, and both
// are changes to what the model is asked:
//
//   - MT-007 gives the model the parent group's numeric ID and not its path.
//     group.create takes parent_id as an integer, so a path is a value the
//     schema refuses, and the case would have been completable only through a
//     lookup call the key does not name. It is why [FactGroupID] exists: the
//     spellings a fact accepts are the scorer's business, and what the prompt
//     hands the model is the one value the fact renders as.
//   - MT-053 drops the window the old text stated, from 2026-01-01T00:00:00Z
//     to one hour later. The key binds neither starts_at nor ends_at, so those
//     two timestamps were a value the model was asked to transcribe and
//     nothing compared; what is left is the message and the banner type, which
//     the key does compare.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: see readCases.
func mutatingCases() []Case {
	return []Case{
		{
			ID:     "MT-004",
			Prompt: "Star project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key:    Key{Steps: []Step{step("project.star", project())}},
		},
		{
			ID: "MT-007",
			Prompt: "Create a subgroup named `eval-temp`, with the same path, under the group " +
				"whose ID is `{{ .Facts.group_id }}`.",
			Recipe: RecipeGroup,
			key: Key{Steps: []Step{
				step("group.create",
					req("name", literal("eval-temp")),
					req("path", literal("eval-temp")),
					req("parent_id", fact(FactGroupID))),
			}},
		},
		{
			ID:     "MT-010",
			Prompt: "Create an issue titled `Evaluate schema discovery` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("issue.create", project(), req("title", literal("Evaluate schema discovery"))),
			}},
		},
		{
			ID: "MT-011",
			Prompt: "Update issue `{{ .Facts.issue_iid }}` in project `{{ .Facts.project_path }}` to add the " +
				"label `evaluation`.",
			Recipe: RecipeIssue,
			key: Key{Steps: []Step{
				step("issue.update", project(),
					req("issue_iid", fact(FactIssueIID)), req("labels", literal("evaluation"))),
			}},
		},
		{
			ID:     "MT-012",
			Prompt: "Close issue `{{ .Facts.issue_iid }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeIssue,
			key: Key{Steps: []Step{
				step("issue.update", project(),
					req("issue_iid", fact(FactIssueIID)), req("state_event", literal("close"))),
			}},
		},
		{
			ID: "MT-015",
			Prompt: "Create a merge request in project `{{ .Facts.project_path }}` from " +
				"`{{ .Facts.mr_source_branch }}` into `{{ .Facts.default_branch }}` titled `Evaluation MR`.",
			Recipe: RecipeMergeRequestSource,
			key: Key{Steps: []Step{
				step("merge_request.create", project(),
					req("source_branch", fact(FactMergeRequestSource)),
					req("target_branch", fact(FactDefaultBranch)),
					req("title", literal("Evaluation MR"))),
			}},
		},
		{
			ID: "MT-016",
			Prompt: "Add a note saying `Can we add coverage?` to merge request " +
				"`{{ .Facts.merge_request_iid }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeMergeRequest,
			key: Key{Steps: []Step{
				step("mr_review.note_create", project(),
					req("merge_request_iid", fact(FactMergeRequestIID)),
					req("body", literal("Can we add coverage?"))),
			}},
		},
		{
			ID:     "MT-019",
			Prompt: "Create a new pipeline on branch `{{ .Facts.default_branch }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("pipeline.create", project(), req("ref", fact(FactDefaultBranch))),
			}},
		},
		{
			ID:     "MT-020",
			Prompt: "Cancel pipeline `{{ .Facts.pipeline_id }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipePipelineJob,
			key: Key{Steps: []Step{
				step("pipeline.cancel", project(), req("pipeline_id", fact(FactPipelineID))),
			}},
		},
		{
			ID:     "MT-023",
			Prompt: "Retry job `{{ .Facts.job_id }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeFailedJob,
			Needs:  Needs{Runner: true},
			key:    Key{Steps: []Step{step("job.retry", project(), req("job_id", fact(FactJobID)))}},
		},
		{
			ID: "MT-026",
			Prompt: "Create a masked CI variable `EVAL_TOKEN` with value `masked-value-123` in project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("ci_variable.create", project(),
					req("key", literal("EVAL_TOKEN")), req("value", literal("masked-value-123"))),
			}},
		},
		{
			ID: "MT-027",
			Prompt: "Update CI variable `{{ .Facts.ci_variable_key }}` scoped to environment `production` in " +
				"project `{{ .Facts.project_path }}` to value `masked-value-456`.",
			Recipe: RecipeCIVariable,
			key: Key{Steps: []Step{
				step("ci_variable.update", project(),
					req("key", fact(FactCIVariableKey)),
					req("value", literal("masked-value-456")),
					req("environment_scope", literal("production"))),
			}},
		},
		{
			ID: "MT-030",
			Prompt: "Create file `tmp/eval.txt` containing `evaluation file` on branch " +
				"`{{ .Facts.branch_name }}` in project `{{ .Facts.project_path }}`, with commit message " +
				"`Create evaluation file`.",
			Recipe: RecipeBranch,
			key: Key{Steps: []Step{
				step("repository.file_create", project(),
					req("file_path", literal("tmp/eval.txt")),
					req("branch", fact(FactBranchName)),
					req("content", literal("evaluation file")),
					req("commit_message", literal("Create evaluation file"))),
			}},
		},
		{
			ID:     "MT-034",
			Prompt: "Create a milestone titled `Evaluation Sprint` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("project.milestone_create", project(), req("title", literal("Evaluation Sprint"))),
			}},
		},
		{
			ID: "MT-036",
			Prompt: "Create a release named `v0.0.0-eval` for tag `v0.0.0-eval` from " +
				"`{{ .Facts.default_branch }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("release.create", project(),
					req("tag_name", literal("v0.0.0-eval")),
					req("ref", fact(FactDefaultBranch)),
					opt("name", literal("v0.0.0-eval"))),
			}},
		},
		{
			ID: "MT-041",
			Prompt: "Create a project access token `eval-token` for project `{{ .Facts.project_path }}` with " +
				"the `read_api` scope.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("access.token_project_create", project(),
					req("name", literal("eval-token")), req("scopes", literal("read_api"))),
			}},
		},
		{
			ID:     "MT-046",
			Prompt: "Pause runner `{{ .Facts.runner_id }}`.",
			Recipe: RecipeRunner,
			Needs:  Needs{Runner: true},
			key:    Key{Steps: []Step{step("runner.update", req("runner_id", fact(FactRunnerID)))}},
		},
		{
			ID:     "MT-053",
			Prompt: "Create a banner broadcast message saying `Evaluation maintenance`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Admin: true},
			key: Key{Steps: []Step{
				step("admin.broadcast_message_create",
					req("message", literal("Evaluation maintenance")), opt("broadcast_type", literal("banner"))),
			}},
		},
		{
			ID: "MT-056",
			Prompt: "Add webhook `https://example.com/gitlab-hook` to project `{{ .Facts.project_path }}`, " +
				"triggered when commits are pushed.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("project.hook_add", project(), req("url", literal("https://example.com/gitlab-hook"))),
			}},
		},
		{
			ID: "MT-058",
			Prompt: "Add a coverage badge to project `{{ .Facts.project_path }}` that links to " +
				"`https://example.com/coverage` and shows the image at `https://example.com/badge.svg`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("project.badge_add", project(),
					req("link_url", literal("https://example.com/coverage")),
					req("image_url", literal("https://example.com/badge.svg"))),
			}},
		},
		{
			ID: "MT-060",
			Prompt: "Start a discussion on merge request `{{ .Facts.merge_request_iid }}` in project " +
				"`{{ .Facts.project_path }}` asking `Can we add coverage?`.",
			Recipe: RecipeMergeRequest,
			key: Key{Steps: []Step{
				step("mr_review.discussion_create", project(),
					req("merge_request_iid", fact(FactMergeRequestIID)),
					req("body", literal("Can we add coverage?"))),
			}},
		},
		{
			ID: "MT-061",
			Prompt: "Resolve discussion `{{ .Facts.discussion_id }}` on merge request " +
				"`{{ .Facts.merge_request_iid }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeMergeRequestDiscussion,
			key: Key{Steps: []Step{
				step("mr_review.discussion_resolve", project(),
					req("merge_request_iid", fact(FactMergeRequestIID)),
					req("discussion_id", fact(FactDiscussionID))),
			}},
		},
		{
			ID: "MT-062",
			Prompt: "Create a draft review note on merge request `{{ .Facts.merge_request_iid }}` in project " +
				"`{{ .Facts.project_path }}` saying `Please add a regression test`.",
			Recipe: RecipeMergeRequest,
			key: Key{Steps: []Step{
				step("mr_review.draft_note_create", project(),
					req("merge_request_iid", fact(FactMergeRequestIID)),
					req("note", literal("Please add a regression test"))),
			}},
		},
		{
			ID: "MT-064",
			Prompt: "Play manual job `{{ .Facts.job_id }}` in project `{{ .Facts.project_path }}` with the " +
				"variable `DEPLOY_ENV=staging`.",
			Recipe: RecipePipelineJob,
			Needs:  Needs{Runner: true},
			key: Key{Steps: []Step{
				step("job.play", project(),
					req("job_id", fact(FactJobID)),
					// The value is a structure the model composes from the
					// prompt rather than a value the prompt states, so it is
					// reported as authored: that it must be sent is the truth
					// here, and what reached GitLab is the recipe's to verify.
					req("job_variables_attributes", authored())),
			}},
		},
		{
			ID: "MT-067",
			Prompt: "Create a group CI variable `GROUP_EVAL_TOKEN` with value `masked-value-123` in group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroup,
			key: Key{Steps: []Step{
				step("ci_variable.group_create",
					req("group_id", fact(FactGroupPath)),
					req("key", literal("GROUP_EVAL_TOKEN")),
					req("value", literal("masked-value-123"))),
			}},
		},
		{
			ID: "MT-068",
			Prompt: "Create an instance CI variable `{{ .Facts.instance_ci_variable_key }}` with value " +
				"`masked-value-123`.",
			Recipe: RecipeInstanceVariable,
			Needs:  Needs{Admin: true},
			key: Key{Steps: []Step{
				step("ci_variable.instance_create",
					req("key", fact(FactInstanceCIVariableKey)),
					req("value", literal("masked-value-123"))),
			}},
		},
		{
			ID:     "MT-080",
			Prompt: "Start the guided issue creation flow for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				standalone("gitlab_interactive_issue_create", project()),
			}},
		},
		{
			ID:     "MT-081",
			Prompt: "Start the guided merge request creation flow for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeMergeRequestSource,
			key: Key{Steps: []Step{
				standalone("gitlab_interactive_mr_create", project()),
			}},
		},
		{
			ID:     "MT-082",
			Prompt: "Start the guided project creation flow.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{standalone("gitlab_interactive_project_create")}},
		},
		{
			ID:     "MT-083",
			Prompt: "Start the guided release creation flow for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				standalone("gitlab_interactive_release_create", project()),
			}},
		},
		{
			ID: "MS-008",
			Prompt: "Troubleshoot runner `{{ .Facts.runner_id }}` for project `{{ .Facts.project_path }}`: " +
				"list the project runners, inspect what that runner has run, fetch the trace of job " +
				"`{{ .Facts.job_id }}`, then pause the runner.",
			Recipe: RecipeRunner,
			Needs:  Needs{Runner: true},
			key: Key{Steps: []Step{
				step("runner.list_project", project()),
				step("runner.jobs", req("runner_id", fact(FactRunnerID))),
				step("job.trace", project(), req("job_id", fact(FactJobID))),
				step("runner.update", req("runner_id", fact(FactRunnerID))),
			}},
		},
		{
			ID: "MS-011",
			Prompt: "Resolve remote URL `{{ .Facts.remote_url }}`, then start the guided issue creation flow " +
				"for the resolved project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				standalone("gitlab_discover_project", req("remote_url", fact(FactRemoteURL))),
				standalone("gitlab_interactive_issue_create", project()),
			}},
		},
		{
			ID: "MS-038",
			Prompt: "Publish the local fixture files `{{ .Facts.package_files_display }}` from directory " +
				"`{{ .Facts.package_dir }}` to the GitLab generic package registry of project " +
				"`{{ .Facts.project_path }}`, as package `{{ .Facts.package_name }}` version " +
				"`{{ .Facts.package_version }}`. Then create release `{{ .Facts.package_tag }}` from " +
				"`{{ .Facts.default_branch }}`, named `Evaluation package release`, and attach every file you " +
				"uploaded to that release in a single call rather than one call per file. Upload first, then " +
				"create the release, then attach what the upload returned; do not build the addresses yourself.",
			Recipe: RecipePackageFiles,
			key: Key{Steps: []Step{
				step("package.publish_directory", project(),
					req("package_name", fact(FactPackageName)),
					req("package_version", fact(FactPackageVersion)),
					req("directory_path", fact(FactPackageDir))),
				step("release.create", project(),
					req("tag_name", fact(FactPackageTag)),
					req("ref", fact(FactDefaultBranch)),
					opt("name", literal("Evaluation package release"))),
				// One batch call rather than one call per file is measured by
				// the step itself: the per-file action is a different action,
				// so an attempt that makes it never reaches this step.
				step("release.link_create_batch", project(),
					req("tag_name", fact(FactPackageTag)), req("links", authored())),
			}},
		},
	}
}
