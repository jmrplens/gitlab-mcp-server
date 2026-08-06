package evaluator

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestTaskPromptForSurface_DynamicBridgeGuidance verifies dynamic prompts expose
// capability bridge tools without telling models to wrap those calls in execute.
func TestTaskPromptForSurface_DynamicBridgeGuidance(t *testing.T) {
	task := evalTask{ID: "MS-039", Prompt: "Read `gitlab://tools`.", Steps: []evalStep{{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	for _, want := range []string{"Use MCP capability bridge tools directly", "do not use bridge tools as a substitute for a required catalog action", "gitlab://tools"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, got)
		}
	}
}

// TestTaskPromptForSurface_DynamicRemoteURLDiscoveryGuidance verifies that
// dynamic prompts for tasks anchored on a remote URL expose the discovery
// guidance without leaking the exact discovery action name into the prompt.
//
// The test renders the prompt for a task with discover_project.resolve and
// pipeline.get steps and asserts the prompt contains the expected discovery
// guidance text and does not expose the literal action name. This protects
// the runner from leaking catalog action names that should be discovered.
func TestTaskPromptForSurface_DynamicRemoteURLDiscoveryGuidance(t *testing.T) {
	task := evalTask{ID: "MS-002", Prompt: "Resolve remote URL `https://gitlab.example.com/group/project.git` then inspect pipeline `1`.", Steps: []evalStep{{ExpectedTool: "gitlab_execute_action", ExpectedAction: "discover_project.resolve", RequiredParams: []string{"remote_url"}}, {ExpectedTool: "gitlab_execute_action", ExpectedAction: "pipeline.get", RequiredParams: []string{"project_id", "pipeline_id"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	for _, want := range []string{"first gitlab_find_action query for that discovery step must explicitly describe resolving the provided remote URL", "must use the project-discovery action with params.remote_url set to that exact URL"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "discover_project.resolve") {
		t.Fatalf("dynamic prompt leaked exact discovery action:\n%s", got)
	}
}

// TestTaskPromptForSurface_DynamicRemoteURLGuidanceIsScoped verifies that
// the remote-URL discovery guidance is only emitted for tasks whose prompt
// actually contains a remote URL.
//
// The test renders a prompt for a project.get task that does not mention a
// remote URL and asserts the discovery guidance is absent and the literal
// discover_project.resolve action is not leaked. This protects the dynamic
// prompt builder from injecting unrelated guidance for tasks that already
// know the project path.
func TestTaskPromptForSurface_DynamicRemoteURLGuidanceIsScoped(t *testing.T) {
	task := evalTask{ID: "MT-002", Prompt: "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.", Steps: []evalStep{{ExpectedTool: "gitlab_execute_action", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	forbidden := []string{"first gitlab_find_action query for that discovery step must explicitly describe resolving the provided remote URL", "must use the project-discovery action with params.remote_url set to that exact URL"}
	for _, text := range forbidden {
		if strings.Contains(got, text) {
			t.Fatalf("dynamic prompt unexpectedly included remote URL guidance %q:\n%s", text, got)
		}
	}
	if strings.Contains(got, "discover_project.resolve") {
		t.Fatalf("dynamic prompt unexpectedly leaked exact discovery action:\n%s", got)
	}
}

// TestDynamicExampleParamValue_CompareRefsExtractsFromAndTo verifies that
// dynamicExampleParamValue pulls the correct ref values for repository.compare
// actions from a prompt that names both refs in
// backticks.
//
// The test invokes the helper with the from/to parameters for each action
// and asserts the extracted values match the prompt's refs. This protects
// the prompt builder from binding the wrong ref to the wrong action when a
// workflow depends on consistent from/to values.
func TestDynamicExampleParamValue_CompareRefsExtractsFromAndTo(t *testing.T) {
	prompt := "Prepare an LLM-assisted release summary for project `my-org/tools/gitlab-mcp-server`: inspect releases, compare refs `main` and `v0.0.0-eval-ms`, then generate release notes."
	if got := dynamicExampleParamValue("repository.compare", "from", prompt); got != "main" {
		t.Fatalf("dynamicExampleParamValue(from) = %v, want main", got)
	}
	if got := dynamicExampleParamValue("repository.compare", "to", prompt); got != "v0.0.0-eval-ms" {
		t.Fatalf("dynamicExampleParamValue(to) = %v, want v0.0.0-eval-ms", got)
	}
}

// TestTaskForSurface_RewritesToolDetailResourceIDs verifies capability tasks use
// detail resource IDs from the active surface instead of dynamic-only IDs.
func TestTaskForSurface_RewritesToolDetailResourceIDs(t *testing.T) {
	evalCase, ok := CaseByID("MS-040")
	if !ok {
		t.Fatal("CaseByID(MS-040) = false")
	}
	task := taskFromCase(evalCase)

	metaTask := taskForSurface(task, config.ToolSurfaceMeta)
	if !strings.Contains(metaTask.Prompt, "`gitlab://tools/gitlab_project.get`") {
		t.Fatalf("meta prompt = %q, want meta project detail URI", metaTask.Prompt)
	}
	if strings.Contains(metaTask.Prompt, dynamicProjectGetToolDetailURI) {
		t.Fatalf("meta prompt kept dynamic project detail URI: %q", metaTask.Prompt)
	}

	dynamicTask := taskForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(dynamicTask.Prompt, "`"+dynamicProjectGetToolDetailURI+"`") {
		t.Fatalf("dynamic prompt = %q, want dynamic project detail URI", dynamicTask.Prompt)
	}
}

// TestJoinNonEmpty_TrimAndSkipBlanks verifies prompt fragments are composed
// without introducing empty paragraphs.
func TestJoinNonEmpty_TrimAndSkipBlanks(t *testing.T) {
	if got := joinNonEmpty("|", " first ", " ", "second"); got != "first|second" {
		t.Fatalf("joinNonEmpty() = %q, want first|second", got)
	}
}

// TestDynamicExampleParamValue_UsesPromptMarkers verifies exact-call guidance
// binds role-sensitive parameters from natural-language prompts.
func TestDynamicExampleParamValue_UsesPromptMarkers(t *testing.T) {
	if got := dynamicExampleParamValue("repository.file_create", "file_path", "create file `docs/eval.md`"); got != "docs/eval.md" {
		t.Fatalf("dynamicExampleParamValue(file_path) = %v, want docs/eval.md", got)
	}
	if got := dynamicExampleParamValue("pipeline.schedule_create", "active", "create inactive schedule `nightly`"); got != false {
		t.Fatalf("dynamicExampleParamValue(active) = %v, want false", got)
	}
}

// TestTaskPrompt_IssueLinkConfirmationStaysSurfaceSpecific verifies shared
// prompts use params.confirm until dynamic rewriting changes the call shape.
func TestTaskPrompt_IssueLinkConfirmationStaysSurfaceSpecific(t *testing.T) {
	task := evalTask{ID: "MS-link", Prompt: "Run issue link CRUD.", Steps: []evalStep{
		{ExpectedTool: "gitlab_issue", ExpectedAction: actionIssueCreate},
		{ExpectedTool: "gitlab_issue", ExpectedAction: "link_create"},
	}}
	metaPrompt := taskPromptForSurface(task, config.ToolSurfaceMeta)
	if !strings.Contains(metaPrompt, "with params.confirm=true") || strings.Contains(metaPrompt, "gitlab_execute_action") {
		t.Fatalf("meta prompt = %s", metaPrompt)
	}
	dynamicPrompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(dynamicPrompt, "first call gitlab_find_action") {
		t.Fatalf("dynamic prompt = %s", dynamicPrompt)
	}
	if strings.Contains(dynamicPrompt, "params.confirm") {
		t.Fatalf("dynamic prompt kept params.confirm guidance: %s", dynamicPrompt)
	}
}

// TestCompactExactTaskPrompt_UsesExpectedToolName verifies compact exact prompts
// do not force unified gitlab when a split meta-tool is expected.
func TestCompactExactTaskPrompt_UsesExpectedToolName(t *testing.T) {
	task := evalTask{ID: "MT-job", Prompt: "Download attestation for project `1` job `2`."}
	step := evalStep{ExpectedTool: "gitlab_attestation", ExpectedAction: "attestation.download", RequiredParams: []string{"project_id", "job_id"}}
	got := compactExactTaskPrompt(task, "No", step)
	if !strings.Contains(got, "Use the gitlab_attestation tool once") {
		t.Fatalf("compact prompt = %s", got)
	}
}

// TestSchemaFirstTaskPrompt_RendersFallbackGuidance verifies unresolved exact
// params produce schema-first instructions instead of placeholder examples.
func TestSchemaFirstTaskPrompt_RendersFallbackGuidance(t *testing.T) {
	got := schemaFirstTaskPrompt(evalTask{ID: "MT-999", Prompt: "Find the thing."}, "no", evalStep{ExpectedTool: "", ExpectedAction: "project.get"})
	for _, want := range []string{"Task MT-999", "Do not use placeholder values", "call gitlab with action project.get"} {
		if !strings.Contains(got, want) {
			t.Fatalf("schemaFirstTaskPrompt() missing %q:\n%s", want, got)
		}
	}
}

// requireContainsAll returns contains all test data or fails the test.
func requireContainsAll(t *testing.T, name, content string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Fatalf("%s = %q, want content containing %q", name, content, want)
		}
	}
}

// TestDynamicPrompt_RequiresFindBeforeUncertainExecute verifies that dynamic
// prompts instruct models to find actions before uncertain execution.
func TestDynamicPrompt_RequiresFindBeforeUncertainExecute(t *testing.T) {
	task := evalTask{ID: "MS-002", Prompt: "Investigate a pipeline failure for git remote `git@gitlab.example.com:group/project.git` and summarize the failing job."}

	system := systemPromptForTask(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "systemPromptForTask()", system, []string{
		"GitLab catalog operations are executed through a find-then-execute workflow",
		"MCP capability bridge tools",
		"expects gitlab_find_action before every gitlab_execute_action call",
		"Destructive actions require top-level confirm:true on gitlab_execute_action",
	})

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"Dynamic workflow:",
		"first call gitlab_find_action",
		"Do not use action IDs from memory",
		"Use MCP capability bridge tools directly",
		"Return tool calls only",
	})
}

// TestDynamicTaskPrompt_MultiStepUsesFindFirst verifies multi-step Dynamic prompts require find before execute.
func TestDynamicTaskPrompt_MultiStepUsesFindFirst(t *testing.T) {
	task := evalTask{ID: "MS-PLAN", Prompt: "Create an issue and then list it.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.list", RequiredParams: []string{"project_id"}},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"For each of the 2 GitLab catalog operations",
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"Do not use action IDs from memory",
	})
	for _, unwanted := range []string{"Dynamic workflow plan:", "action=issue.create", "do not call gitlab_find_action for these planned actions"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPromptForSurface() = %q, want no exact dynamic plan content %q", prompt, unwanted)
		}
	}
}

// TestDynamicTaskPrompt_ProviderConfusionCasesUseFindFirst verifies previously
// brittle Dynamic workflows now receive generic find-first guidance without
// leaking expected action IDs.
func TestDynamicTaskPrompt_ProviderConfusionCasesUseFindFirst(t *testing.T) {
	tests := []struct {
		name   string
		task   evalTask
		absent []string
	}{
		{
			name: "failed pipeline investigation workflow",
			task: evalTask{ID: "MS-002", Prompt: "Investigate failed pipeline `339` for project `my-org/tools/gitlab-mcp-server` and remote URL `http://localhost:8929/my-org/tools/gitlab-mcp-server.git`: resolve the project, inspect the pipeline, list failed jobs, fetch job `677` trace, then call the pipeline failure analyzer for pipeline `339`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "discover_project.resolve", RequiredParams: []string{"remote_url"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.get", RequiredParams: []string{"project_id", "pipeline_id"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.list", RequiredParams: []string{"project_id", "pipeline_id"}},
			}},
		},
		{
			name: "settings broadcast workflow",
			task: evalTask{ID: "MS-009", Prompt: "Read current instance settings, create a broadcast message, then delete it.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.settings_get"},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.broadcast_message_create"},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.broadcast_message_delete"},
			}},
		},
		{
			name: "release cleanup workflow",
			task: evalTask{ID: "MS-004", Prompt: "Verify a tag and release, list release asset links, delete release and tag.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "tag.get"},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.get"},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.link_list"},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.delete"},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "tag.delete"},
			}},
		},
		{
			name: "feature flag user list workflow",
			task: evalTask{ID: "MS-029", Prompt: "Exercise feature flag and user-list lifecycle in project `my-org/tools/gitlab-mcp-server`: create feature flag user list `eval-feature-list` with user IDs `u1,u2`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "feature_flags.ff_user_list_create", RequiredParams: []string{"project_id", "name", "user_xids"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "feature_flags.ff_user_list_get", RequiredParams: []string{"project_id", "user_list_iid"}},
			}},
		},
		{
			name: "issue time tracking workflow",
			task: evalTask{ID: "MS-032", Prompt: "Exercise issue time tracking in project `my-org/tools/gitlab-mcp-server`: create issue `eval-time-issue`, set estimate `2h`, add spent time `30m`, reset spent time, reset the estimate, then delete the issue.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.time_estimate_set", RequiredParams: []string{"project_id", "issue_iid", "duration"}},
			}},
		},
		{
			name: "issue link workflow",
			task: evalTask{ID: "MS-016", Prompt: "Exercise issue link CRUD in project `my-org/tools/gitlab-mcp-server`: create source issue `eval-link-source`, create target issue `eval-link-target`, link source to target as `relates_to`, list source issue links, delete the returned issue link, then delete both issues.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.link_create", RequiredParams: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.link_list", RequiredParams: []string{"project_id", "issue_iid"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.link_delete", RequiredParams: []string{"project_id", "issue_iid", "issue_link_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "issue note workflow",
			task: evalTask{ID: "MS-015", Prompt: "Exercise issue note CRUD in project `my-org/tools/gitlab-mcp-server`: create issue `eval-note-issue`, add a note saying `first note`, fetch that note with note get using the returned note ID, update the note to `updated note`, delete the note, then delete the issue.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.note_create", RequiredParams: []string{"project_id", "issue_iid", "body"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.note_get", RequiredParams: []string{"project_id", "issue_iid", "note_id"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.note_update", RequiredParams: []string{"project_id", "issue_iid", "note_id", "body"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.note_delete", RequiredParams: []string{"project_id", "issue_iid", "note_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "merge request award workflow",
			task: evalTask{ID: "MS-033", Prompt: "Exercise merge request time tracking and emoji in project `my-org/tools/gitlab-mcp-server`: set estimate `1h` on merge request `1`, add spent time `15m`, add award emoji `eyes`, list MR awards, delete the returned award emoji, reset spent time, then reset the estimate.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.time_estimate_set", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.spent_time_add", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.emoji_mr_create", RequiredParams: []string{"project_id", "merge_request_iid", "name"}},
			}},
		},
		{
			name: "epic discussion workflow",
			task: evalTask{ID: "MS-049", Prompt: "Exercise epic discussion lifecycle in group full path `my-org`: create epic `Evaluation Enterprise Discussion Epic`, create discussion `first enterprise discussion`, list discussions, fetch the created discussion, add reply note `enterprise reply`, update that reply to `enterprise reply updated`, delete the reply note, then delete the epic.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_create", RequiredParams: []string{"full_path", "title"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_discussion_create", RequiredParams: []string{"full_path", "epic_iid", "body"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_discussion_list", RequiredParams: []string{"full_path", "epic_iid"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_discussion_get", RequiredParams: []string{"full_path", "epic_iid", "discussion_id"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_discussion_add_note", RequiredParams: []string{"full_path", "epic_iid", "discussion_id", "body"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_discussion_update_note", RequiredParams: []string{"full_path", "epic_iid", "note_id", "body"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_discussion_delete_note", RequiredParams: []string{"full_path", "epic_iid", "note_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "group protected environment workflow",
			task: evalTask{ID: "MS-052", Prompt: "Exercise group protected environment lifecycle with a temporary group: create group `eval-enterprise-protected-env`, protect environment `staging`, list group protected environments, fetch environment `staging`, update it to require one approval, unprotect environment `staging`, then delete the temporary group.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.create", RequiredParams: []string{"name", "path"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.protected_env_protect", RequiredParams: []string{"group_id", "name", "deploy_access_levels"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.protected_env_update", RequiredParams: []string{"group_id", "environment"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.protected_env_unprotect", RequiredParams: []string{"group_id", "environment"}, OptionalParams: []string{"confirm"}, Destructive: true},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.delete", RequiredParams: []string{"group_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
		},
		{
			name: "project push rule add",
			task: evalTask{ID: "MT-192", Prompt: "Add a project push rule to project `my-org/tools/eval-push-rule` with commit message regex `^EVAL-`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.push_rule_add", RequiredParams: []string{"project_id"}, OptionalParams: []string{"commit_message_regex", "reject_unsigned_commits"}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic)
			requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
				"first call gitlab_find_action",
				"Use the returned result ID, input_schema, required_params, and example",
				"Do not use action IDs from memory",
			})
			for _, step := range taskSteps(tt.task) {
				if step.ExpectedAction != "" && strings.Contains(prompt, step.ExpectedAction) {
					t.Fatalf("taskPromptForSurface() leaked expected action %q in prompt %q", step.ExpectedAction, prompt)
				}
			}
		})
	}
}

// TestDynamicTaskPrompt_MultiStepOmitsExactActionPlan verifies Dynamic prompts do not leak planned action IDs.
func TestDynamicTaskPrompt_MultiStepOmitsExactActionPlan(t *testing.T) {
	task := evalTask{ID: "MS-020", Prompt: "Exercise pipeline schedule CRUD in project `my-org/tools/gitlab-mcp-server`: create inactive schedule `eval-crud-schedule` on `main`, get it, update its cron, create variable `SCHEDULE_CRUD_TOKEN`, update that variable, delete the variable, then delete the schedule.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.schedule_create", RequiredParams: []string{"project_id", "description", "ref", "cron"}, OptionalParams: []string{"active"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.schedule_get", RequiredParams: []string{"project_id", "schedule_id"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.schedule_update", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"cron"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.schedule_delete_variable", RequiredParams: []string{"project_id", "schedule_id", "key"}, Destructive: true},
	}}

	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
		"For each of the 4 GitLab catalog operations",
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"Do not use action IDs from memory",
	})
	for _, unwanted := range []string{"Dynamic first-step exact call", "pipeline.schedule_create", "do not call gitlab_find_action for these planned actions"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPromptForSurface() = %q, want no exact dynamic plan content %q", prompt, unwanted)
		}
	}
}

// TestDynamicTaskPrompt_OmitsRoleSensitiveExactCallContent verifies role-sensitive
// examples are no longer injected into Dynamic prompts.
func TestDynamicTaskPrompt_OmitsRoleSensitiveExactCallContent(t *testing.T) {
	tests := []struct {
		name string
		task evalTask
		want []string
	}{
		{
			name: "allowlist source and target projects",
			task: evalTask{ID: "MT-066", Prompt: "Remove project ID `51` from the CI job token allowlist of project `1`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{`"action":"job.token_scope_remove_project"`, `"confirm":true`, `"params":{"project_id":1,"target_project_id":51}`},
		},
		{
			name: "issue link source and target",
			task: evalTask{ID: "MT-LINK", Prompt: "Link source issue IID `5` in project `my-org/source` to target issue IID `9` in target project ID `77`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.link_create", RequiredParams: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
			}},
			want: []string{`"action":"issue.link_create"`, `"issue_iid":5`, `"project_id":"my-org/source"`, `"target_issue_iid":9`, `"target_project_id":77`},
		},
		{
			name: "merge request branches",
			task: evalTask{ID: "MT-MR", Prompt: "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval` into `main` titled `Evaluation MR`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.create", RequiredParams: []string{"project_id", "source_branch", "target_branch", "title"}},
			}},
			want: []string{`"action":"merge_request.create"`, `"project_id":"my-org/tools/gitlab-mcp-server"`, `"source_branch":"feature/eval"`, `"target_branch":"main"`, `"title":"Evaluation MR"`},
		},
		{
			name: "group epic child issue",
			task: evalTask{ID: "MT-140", Prompt: "Assign issue IID `99` from child project path `my-org/tools/gitlab-mcp-server` to epic IID `12` in group full path `my-org`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.epic_issue_assign", RequiredParams: []string{"full_path", "epic_iid", "child_project_path", "child_iid"}},
			}},
			want: []string{`"action":"group.epic_issue_assign"`, `"child_iid":99`, `"child_project_path":"my-org/tools/gitlab-mcp-server"`, `"epic_iid":12`, `"full_path":"my-org"`},
		},
		{
			name: "project deploy token delete",
			task: evalTask{ID: "MT-112", Prompt: "Delete project deploy token ID `66` from project `my-org/tools/gitlab-mcp-server`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "access.deploy_token_delete_project", RequiredParams: []string{"project_id", "deploy_token_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			want: []string{`"action":"access.deploy_token_delete_project"`, `"confirm":true`, `"deploy_token_id":66`, `"project_id":"my-org/tools/gitlab-mcp-server"`},
		},
		{
			name: "group ci variable environment scope",
			task: evalTask{ID: "MS-026", Prompt: "Exercise scoped group CI variable CRUD in group `my-org`: create variable `GROUP_EVAL_CRUD_TOKEN` with value `group-crud-value-1` and environment scope `review/eval`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "ci_variable.group_create", RequiredParams: []string{"group_id", "key", "value"}, OptionalParams: []string{"environment_scope", "masked"}},
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "ci_variable.group_get", RequiredParams: []string{"group_id", "key"}, OptionalParams: []string{"environment_scope"}},
			}},
			want: []string{`"action":"ci_variable.group_create"`, `"environment_scope":"review/eval"`, `"group_id":"my-org"`, `"key":"GROUP_EVAL_CRUD_TOKEN"`, `"value":"group-crud-value-1"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic)
			requireContainsAll(t, "taskPromptForSurface()", prompt, []string{
				"first call gitlab_find_action",
				"Use the returned result ID, input_schema, required_params, and example",
				"Do not use action IDs from memory",
			})
			for _, unwanted := range tt.want {
				if strings.Contains(prompt, unwanted) {
					t.Fatalf("taskPromptForSurface() = %q, want no exact-call content %q", prompt, unwanted)
				}
			}
		})
	}
}

// TestDynamicTaskPrompt_UnresolvedRoleSensitiveParamsStayFindFirst verifies
// unresolved role-sensitive values keep Dynamic prompts on the find-first path.
func TestDynamicTaskPrompt_UnresolvedRoleSensitiveParamsStayFindFirst(t *testing.T) {
	tests := []struct {
		name   string
		task   evalTask
		absent []string
	}{
		{
			name: "missing target project",
			task: evalTask{ID: "MT-066", Prompt: "Remove a project from the CI job token allowlist of project `1`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			absent: []string{"Dynamic first-step exact call", `"target_project_id":123`, "<target_project_id>"},
		},
		{
			name: "non numeric target project",
			task: evalTask{ID: "MT-066", Prompt: "Remove project ID `not-a-number` from the CI job token allowlist of project `1`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			}},
			absent: []string{"Dynamic first-step exact call", `"target_project_id":123`},
		},
		{
			name: "missing target branch",
			task: evalTask{ID: "MT-MR", Prompt: "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval` titled `Evaluation MR`.", Steps: []evalStep{
				{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.create", RequiredParams: []string{"project_id", "source_branch", "target_branch", "title"}},
			}},
			absent: []string{"Dynamic first-step exact call", `"target_branch":"main"`, "<target_branch>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPromptForSurface(tt.task, config.ToolSurfaceDynamic)
			for _, unwanted := range tt.absent {
				if strings.Contains(prompt, unwanted) {
					t.Fatalf("taskPromptForSurface() = %q, want no unsafe exact-call content %q", prompt, unwanted)
				}
			}
			if !strings.Contains(prompt, "Required parameters for action") && !strings.Contains(prompt, "gitlab_find_action") {
				t.Fatalf("taskPromptForSurface() = %q, want schema-first or dynamic discovery guidance", prompt)
			}
		})
	}
}

// TestTaskPrompt_ClarifiesTransientRetry verifies TaskPrompt when clarifies transient retry.
func TestTaskPrompt_ClarifiesTransientRetry(t *testing.T) {
	task := evalTask{
		ID:             "MF-001",
		Prompt:         "Inspect pipeline `12345`, retrying once if GitLab temporarily returns a server error.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "pipeline.get",
		Simulation:     "transient_error_once",
	}
	prompt := taskPrompt(task)
	if !strings.Contains(prompt, "repeat the same validated operation once") {
		t.Fatalf("taskPrompt() = %q, want transient retry guidance", prompt)
	}
	if !strings.Contains(prompt, "do not use GitLab CI retry actions") {
		t.Fatalf("taskPrompt() = %q, want CI retry disambiguation", prompt)
	}
}

// TestTaskPrompt_SingleOperationPrefersOneClearToolCall verifies TaskPrompt when single operation prefers one clear tool call.
func TestTaskPrompt_SingleOperationPrefersOneClearToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-003",
		Prompt:         "List the 10 most recently updated projects I can access.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "project.list",
	}
	prompt := taskPrompt(task)
	assertTaskPromptContains(
		t, prompt,
		"exactly one tool call",
		"A schema lookup before the task call is a failure",
		"Do not look up schemas for ordinary parameter names already supplied by the task prompt",
		"do not add any params that the task did not ask for",
		"Use gitlab_interactive_* only if this task explicitly asks for a guided interactive flow",
		"When the selected action requires project_id, a value like group/project is params.project_id, not params.full_path, params.path, or remote_url",
		"never call gitlab without an input object containing action and params",
		"server diagnostics or a GitLab connectivity check, call gitlab_server with action health_check",
		"For subgroup creation with group.create, use params.name, params.path, and params.parent_id",
		"For merge request creation, from is params.source_branch, into is params.target_branch, and titled is params.title",
		"For merge request notes or comments, use mr_review.note_create",
		"Use mr_review.discussion_create only when the task explicitly asks for a threaded discussion or discussion",
		"For personal snippets, snippet ID is params.snippet_id",
		"or file_path",
		"For custom emoji group operations, use custom_emoji.list with params.group_path",
		"For project access tokens, scope names go in params.scopes as an array",
		"expiring dates go in params.expires_at",
		"For broadcast messages, saying maps to params.message",
		"For job.play variables, use params.job_variables_attributes as an array",
		"For project CI variables in a project, use ci_variable.list/get/create/update/delete with params.project_id",
		"for group CI variables, use ci_variable.group_list/group_get/group_create/group_update/group_delete with params.group_id",
		"use ci_variable.instance_* only for instance-level variables when no project_id or group_id is supplied",
		"For runner.list_project, use params.project_id by default",
		"Do not send params.paused, params.type, params.tag_list",
		"For repository file create/update/delete, use params.branch, params.file_path, and params.commit_message",
		"For CI variables, variable name maps to params.key, value maps to params.value, and environment_scope or production scope maps to params.environment_scope",
		"linking to a URL means params.link_url and image means params.image_url",
		"latest pipelines plural means pipeline.list",
		"do not send empty arrays or objects",
		"call the selected action with params:{}",
	)
}

func assertTaskPromptContains(t *testing.T, prompt string, snippets ...string) {
	t.Helper()
	for _, snippet := range snippets {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("taskPrompt() = %q, want %q", prompt, snippet)
		}
	}
}

// TestTaskPrompt_MultiStepAvoidsImplicitPagination verifies TaskPrompt when multi step avoids implicit pagination.
func TestTaskPrompt_MultiStepAvoidsImplicitPagination(t *testing.T) {
	task := evalTask{
		ID:     "MS-037",
		Prompt: "Build a broad read-only Docker inventory for project `my-org/tools/gitlab-mcp-server`: list project CI variables, list deploy keys, then list generic packages.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_ci_variable", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_key_list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_package", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"one successful list response completes a list step",
		"do not fetch additional pagination pages",
		"unless the task explicitly asks for every page",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pagination guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_BroadInventoryUsesExactOrderAndSmallPages verifies TaskPrompt when broad inventory uses exact order and small pages.
func TestTaskPrompt_BroadInventoryUsesExactOrderAndSmallPages(t *testing.T) {
	task := evalTask{
		ID:     "MS-037",
		Prompt: "Build a broad read-only Docker inventory for project `my-org/tools/gitlab-mcp-server`: get the project, list branches, list tags, list releases, list the repository tree at `main`, list project CI variables, list deploy keys, list deploy tokens, then list generic packages.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_branch", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_tag", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_release", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "tree", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_ci_variable", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_key_list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_package", ExpectedAction: "list", RequiredParams: []string{"project_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"follow exactly this order",
		"gitlab_release/list before repository tree",
		"call repository tree with params.ref=\"main\"",
		"Use params.per_page=1 on list/tree/package steps",
		"one page is enough for this evaluation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want broad inventory guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_PackageReleaseWorkflowUsesExactOrder verifies package publishing and release linking guidance.
func TestTaskPrompt_PackageReleaseWorkflowUsesExactOrder(t *testing.T) {
	task := evalTask{
		ID:     taskPackageReleaseID,
		Prompt: "Publish local fixture files to Generic Packages, then create a release, and link each uploaded package file to that release as a package asset.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_package", ExpectedAction: "publish_directory", RequiredParams: []string{"project_id", "package_name", "package_version", "directory_path"}},
			{ExpectedTool: "gitlab_release", ExpectedAction: "create", RequiredParams: []string{"project_id", "tag_name", "ref"}},
			{ExpectedTool: "gitlab_release", ExpectedAction: "link_create_batch", RequiredParams: []string{"project_id", "tag_name", "links"}},
		},
	}

	prompt := taskPrompt(task)
	requireContainsAll(t, "taskPrompt()", prompt, []string{
		"follow exactly this order: gitlab_package/publish_directory, gitlab_release/create, gitlab_release/link_create_batch",
		"Omit params.include_pattern for this task",
		"never a comma-separated file list",
		"Use the returned published[].url values as links[].url",
		"set each links[].link_type to \"package\"",
		"do not construct package URLs manually",
		"Create the release from params.ref=\"main\" before link_create_batch",
		"do not send direct_asset_path or filepath",
	})
}

// TestTaskPrompt_PackageReleaseWorkflowUsesExactOrderDynamic verifies package
// release guidance is preserved after dynamic action normalization.
func TestTaskPrompt_PackageReleaseWorkflowUsesExactOrderDynamic(t *testing.T) {
	task := evalTask{
		ID:     taskPackageReleaseID,
		Prompt: "Publish local fixture files to Generic Packages, then create a release, and link each uploaded package file to that release as a package asset.",
		Steps: []evalStep{
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "package.publish_directory", RequiredParams: []string{"project_id", "package_name", "package_version", "directory_path"}},
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.create", RequiredParams: []string{"project_id", "tag_name", "ref"}},
			{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "release.link_create_batch", RequiredParams: []string{"project_id", "tag_name", "links"}},
		},
	}

	prompt := taskPromptForSurface(task, "dynamic")
	requireContainsAll(t, "taskPromptForSurface(dynamic)", prompt, []string{
		"For each of the 3 GitLab catalog operations",
		"first call gitlab_find_action",
		"Use the returned result ID, input_schema, required_params, and example",
		"Do not use action IDs from memory",
	})
	for _, unwanted := range []string{"Dynamic workflow plan:", "package.publish_directory", "release.link_create_batch"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPromptForSurface(dynamic) = %q, want no exact action guidance %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_MergeRequestTimeEmojiUsesExactOrder verifies TaskPrompt when merge request time emoji uses exact order.
func TestTaskPrompt_MergeRequestTimeEmojiUsesExactOrder(t *testing.T) {
	task := evalTask{
		ID:     "MS-033",
		Prompt: "Exercise merge request time tracking and emoji in project `my-org/tools/gitlab-mcp-server`: set estimate `1h` on MR `7`, add spent time `15m`, add award emoji `eyes`, list MR awards, delete the returned award emoji, reset spent time, then reset the estimate.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "time_estimate_set", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "spent_time_add", RequiredParams: []string{"project_id", "merge_request_iid", "duration"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_create", RequiredParams: []string{"project_id", "merge_request_iid", "name"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_list", RequiredParams: []string{"project_id", "merge_request_iid"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_delete", RequiredParams: []string{"project_id", "merge_request_iid", "award_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "spent_time_reset", RequiredParams: []string{"project_id", "merge_request_iid"}},
			{ExpectedTool: "gitlab_merge_request", ExpectedAction: "time_estimate_reset", RequiredParams: []string{"project_id", "merge_request_iid"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"follow exactly this order: time_estimate_set, spent_time_add, emoji_mr_create, emoji_mr_list, emoji_mr_delete, spent_time_reset, time_estimate_reset",
		"After emoji_mr_create, call emoji_mr_list next",
		"using the returned award emoji id as params.award_id with params.confirm=true",
		"After emoji_mr_delete, call spent_time_reset before time_estimate_reset",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR time/emoji guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_MergeRequestNoteCRUDUsesExactOrder verifies TaskPrompt when merge request note CRUD uses exact order.
func TestTaskPrompt_MergeRequestNoteCRUDUsesExactOrder(t *testing.T) {
	task := evalTask{
		ID:     "MS-027",
		Prompt: "Exercise merge request note CRUD in project `my-org/tools/gitlab-mcp-server`: add note `eval-mr-note` to MR `7`, fetch the created note using the returned note ID, update it to `eval-mr-note-updated`, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_create", RequiredParams: []string{"project_id", "merge_request_iid", "body"}},
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_get", RequiredParams: []string{"project_id", "merge_request_iid", "note_id"}},
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_update", RequiredParams: []string{"project_id", "merge_request_iid", "note_id", "body"}},
			{ExpectedTool: "gitlab_mr_review", ExpectedAction: "note_delete", RequiredParams: []string{"project_id", "merge_request_iid", "note_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"follow exactly this order: note_create, note_get, note_update, note_delete",
		"After note_create, call note_get next",
		"call note_update with params.body set to the updated note text and without params.confirm",
		"Only note_delete is destructive; call note_delete last with params.confirm=true",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR note CRUD guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_MergeRequestNotePrefersNoteCreate verifies TaskPrompt when merge request note prefers note create.
func TestTaskPrompt_MergeRequestNotePrefersNoteCreate(t *testing.T) {
	task := evalTask{
		ID:             "MT-016",
		Prompt:         "Add a note saying `Can we add coverage?` to merge request `7` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_mr_review",
		ExpectedAction: "note_create",
		RequiredParams: []string{"project_id", "merge_request_iid", "body"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`call gitlab_mr_review with {"action":"note_create"`,
		`"merge_request_iid":<merge_request_iid>`,
		`"body":"<body>"`,
		"Do not use discussion_create unless the task explicitly says threaded discussion or discussion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR note guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_RunnerListProjectAvoidsImplicitFilters verifies TaskPrompt when runner list project avoids implicit filters.
func TestTaskPrompt_RunnerListProjectAvoidsImplicitFilters(t *testing.T) {
	task := evalTask{
		ID:     "MS-008",
		Prompt: "Troubleshoot runner ID `99` for project `my-org/tools/gitlab-mcp-server`: list project runners, inspect runner jobs, fetch trace for job `999`, then set paused=true on the runner.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_runner", ExpectedAction: "list_project", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_runner", ExpectedAction: "jobs", RequiredParams: []string{"runner_id"}},
			{ExpectedTool: "gitlab_job", ExpectedAction: "trace", RequiredParams: []string{"project_id", "job_id"}},
			{ExpectedTool: "gitlab_runner", ExpectedAction: "update", RequiredParams: []string{"runner_id", "paused"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`call gitlab_runner with {"action":"list_project","params":{"project_id":"<project_id>"}}`,
		"unless the task explicitly asks for an online, offline, stale, or never_contacted status filter",
		"Do not send params.paused, params.type, params.tag_list, status all, status active, or empty filter strings for runner.list_project",
		"For runner jobs, use runner.jobs with params.runner_id only",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want runner filter guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_PipelineTriggerCreateOmitsRef verifies TaskPrompt when pipeline trigger create omits ref.
func TestTaskPrompt_PipelineTriggerCreateOmitsRef(t *testing.T) {
	task := evalTask{
		ID:     "MS-019",
		Prompt: "Exercise pipeline trigger CRUD in project `my-org/tools/gitlab-mcp-server`: create trigger `eval-crud-trigger`, fetch it with trigger get using the returned trigger ID, update the description, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_create", RequiredParams: []string{"project_id", "description"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_get", RequiredParams: []string{"project_id", "trigger_id"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_update", RequiredParams: []string{"project_id", "trigger_id"}, OptionalParams: []string{"description"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "trigger_delete", RequiredParams: []string{"project_id", "trigger_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"trigger_create accepts only params.project_id and params.description",
		"never send params.ref for trigger_create",
		"Ref belongs to trigger_run or pipeline.create, not trigger_create",
		"Use the returned trigger_id for trigger_get, trigger_update, and trigger_delete",
		"trigger_delete also requires params.confirm=true",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline trigger guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_RepositoryFileCRUDUsesRefAndDeletesAfterUpdate verifies TaskPrompt when repository file CRUD uses ref and deletes after update.
func TestTaskPrompt_RepositoryFileCRUDUsesRefAndDeletesAfterUpdate(t *testing.T) {
	task := evalTask{
		ID:     "MS-017",
		Prompt: "Exercise repository file CRUD in project `my-org/tools/gitlab-mcp-server`: create file `tmp/eval-crud.txt` on branch `feature/eval`, read it, update its content, then delete it from the same branch.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_create", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_update", RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_delete", RequiredParams: []string{"project_id", "file_path", "branch", "commit_message"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"read the created file with file_get using params.ref set to the branch name",
		"never send params.branch to file_get",
		"After file_update succeeds, call file_delete next",
		"confirm must be inside params, never a top-level field",
		`"action":"file_delete","params":{"project_id":"<project_id>","file_path":"<file_path>","branch":"<branch>","commit_message":"<commit_message>","confirm":true}`,
		"Do not call file_get again after the update",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want repository file CRUD guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SingleFileCreateUsesExactToolCall verifies TaskPrompt when single file create uses exact tool call.
func TestTaskPrompt_SingleFileCreateUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-030",
		Prompt:         "Create file `tmp/eval.txt` with content `evaluation file` and commit_message `Create evaluation file` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_repository",
		ExpectedAction: "file_create",
		RequiredParams: []string{"project_id", "file_path", "branch", "content", "commit_message"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call: use the gitlab_repository tool once with input",
		`"action":"file_create"`,
		`"file_path":"tmp/eval.txt"`,
		`"content":"evaluation file"`,
		`"branch":"feature/eval"`,
		`"commit_message":"Create evaluation file"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want exact file_create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ProjectGetUsesExactToolCall verifies exact project path
// lookups do not drift into project search in meta-surface evaluations.
func TestTaskPrompt_ProjectGetUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-002",
		Prompt:         "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.",
		ExpectedTool:   "gitlab_project",
		ExpectedAction: "get",
		RequiredParams: []string{"project_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call: use the gitlab_project tool once with input",
		`"action":"get"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		"do not call gitlab_discover_project",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want exact project_get guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_InstanceVariableCreateUsesExactToolCall verifies TaskPrompt when instance variable create uses exact tool call.
func TestTaskPrompt_InstanceVariableCreateUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-068",
		Prompt:         "Create instance CI variable `INSTANCE_EVAL_TOKEN` with value `masked-value-123`.",
		ExpectedTool:   "gitlab_ci_variable",
		ExpectedAction: "instance_create",
		RequiredParams: []string{"key", "value"},
		OptionalParams: []string{"masked", "protected"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call: use the gitlab_ci_variable tool once with input",
		`"action":"instance_create"`,
		`"key":"INSTANCE_EVAL_TOKEN"`,
		`"value":"masked-value-123"`,
		"Return exactly one tool call and no text answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want exact instance_create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_PipelineScheduleCRUDAvoidsProjectPrefetchAndConfirmsDeletes verifies TaskPrompt when pipeline schedule CRUD avoids project prefetch and confirms deletes.
func TestTaskPrompt_PipelineScheduleCRUDAvoidsProjectPrefetchAndConfirmsDeletes(t *testing.T) {
	task := evalTask{
		ID:     "MS-020",
		Prompt: "Exercise pipeline schedule CRUD in project `my-org/tools/gitlab-mcp-server`: create inactive schedule `eval-crud-schedule` on `main`, get it, update its cron, create variable `SCHEDULE_CRUD_TOKEN`, update that variable, delete the variable, then delete the schedule.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_create", RequiredParams: []string{"project_id", "description", "ref", "cron"}, OptionalParams: []string{"active"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_get", RequiredParams: []string{"project_id", "schedule_id"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_update", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"cron"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_create_variable", RequiredParams: []string{"project_id", "schedule_id", "key", "value"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_edit_variable", RequiredParams: []string{"project_id", "schedule_id", "key", "value"}},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_delete_variable", RequiredParams: []string{"project_id", "schedule_id", "key"}, OptionalParams: []string{"confirm"}, Destructive: true},
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "schedule_delete", RequiredParams: []string{"project_id", "schedule_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"include confirm:true in params for each destructive tool call",
		"the first call is gitlab_pipeline with action schedule_create",
		"do not call gitlab_discover_project or gitlab_project first",
		"Use description, not name, for the schedule display label",
		"never send masked or protected",
		`use params.value="schedule-value-1" for schedule_create_variable`,
		`params.value="schedule-value-2" for schedule_edit_variable`,
		"Use the returned id as params.schedule_id",
		"Both schedule_delete_variable and schedule_delete are destructive and require confirm:true according to the active tool surface",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline schedule guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DiscoverProjectUsesStandaloneInput verifies TaskPrompt when discover project uses standalone input.
func TestTaskPrompt_DiscoverProjectUsesStandaloneInput(t *testing.T) {
	task := evalTask{
		ID:     "MS-001",
		Prompt: "Resolve remote URL `https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git` for project `my-org/tools/gitlab-mcp-server`, verify the project metadata, then read `README.md` from `main`.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`call the standalone tool with top-level remote_url only`,
		`{"remote_url":"<remote_url>"}`,
		"do not send action, params, project_id, or ref to gitlab_discover_project",
		"call gitlab_project/get to verify metadata before calling gitlab_repository/file_get",
		"do not skip the project metadata verification step",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want discover_project guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_FeatureFlagLifecycleOmitsArrayStrategies verifies TaskPrompt when feature flag lifecycle omits array strategies.
func TestTaskPrompt_FeatureFlagLifecycleOmitsArrayStrategies(t *testing.T) {
	task := evalTask{
		ID:     "MS-029",
		Prompt: "Exercise feature flag and user-list lifecycle in project `my-org/tools/gitlab-mcp-server`: create feature flag user list `eval-feature-list` with user IDs `u1,u2`, fetch it, update the user IDs to `u2,u3`, create feature flag `eval-feature-flag-crud` using version `new_version_flag`, fetch the flag, update it inactive, delete the flag, then delete the user list.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_create", RequiredParams: []string{"project_id", "name", "user_xids"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_get", RequiredParams: []string{"project_id", "user_list_iid"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_update", RequiredParams: []string{"project_id", "user_list_iid"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_create", RequiredParams: []string{"project_id", "name", "version"}, OptionalParams: []string{"strategies"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_get", RequiredParams: []string{"project_id", "name"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_update", RequiredParams: []string{"project_id", "name"}, OptionalParams: []string{"strategies"}},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "feature_flag_delete", RequiredParams: []string{"project_id", "name"}, OptionalParams: []string{"confirm"}, Destructive: true},
			{ExpectedTool: "gitlab_feature_flags", ExpectedAction: "ff_user_list_delete", RequiredParams: []string{"project_id", "user_list_iid"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`params.user_xids is a comma-separated string such as "u1,u2", not an array`,
		"Use the returned iid as params.user_list_iid",
		"do not use the user-list name for those lookup/delete actions",
		"omit params.strategies unless the task gives an exact strategies JSON string",
		`must be a JSON string such as "[{\"name\":\"default\"}]", never an array or object`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want feature flag lifecycle guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DeployTokenLifecycleAvoidsInventedTimestamp verifies TaskPrompt when deploy token lifecycle avoids invented timestamp.
func TestTaskPrompt_DeployTokenLifecycleAvoidsInventedTimestamp(t *testing.T) {
	task := evalTask{
		ID:     "MS-030",
		Prompt: "Exercise project deploy token lifecycle in project `my-org/tools/gitlab-mcp-server`: create deploy token `eval-deploy-token` with scope `read_repository`, fetch it with the returned deploy token ID, list project deploy tokens, then delete that deploy token.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_create_project", RequiredParams: []string{"project_id", "name", "scopes"}, OptionalParams: []string{"expires_at", "username"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_get_project", RequiredParams: []string{"project_id", "deploy_token_id"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_list_project", RequiredParams: []string{"project_id"}, OptionalParams: []string{"page", "per_page"}},
			{ExpectedTool: "gitlab_access", ExpectedAction: "deploy_token_delete_project", RequiredParams: []string{"project_id", "deploy_token_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"deploy_token_create_project requires params.project_id, params.name, and params.scopes",
		"Do not add params.expires_at unless the task gives an explicit expiry date",
		"must be YYYY-MM-DD only, never a timestamp",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want deploy token lifecycle guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DestructiveScenarioWarningGuidance covers TaskPrompt with table-driven subtests for destructive scenario warning guidance.
func TestTaskPrompt_DestructiveScenarioWarningGuidance(t *testing.T) {
	tests := []struct {
		name  string
		task  evalTask
		wants []string
	}{
		{
			name: "broadcast theme",
			task: evalTask{
				ID:     "MS-009",
				Prompt: "Schedule and then remove an instance maintenance banner: read current instance settings, immediately create broadcast message `Evaluation maintenance`, then delete the broadcast message created in the previous step using the returned ID.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_admin", ExpectedAction: "settings_get"},
					{ExpectedTool: "gitlab_admin", ExpectedAction: "broadcast_message_create", RequiredParams: []string{"message"}, OptionalParams: []string{"starts_at", "ends_at", "broadcast_type"}},
					{ExpectedTool: "gitlab_admin", ExpectedAction: "broadcast_message_delete", RequiredParams: []string{"id"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"omit params.theme unless explicitly requested", "use a GitLab theme name such as indigo, never a hex color"},
		},
		{
			name: "issue link delete",
			task: evalTask{
				ID:     "MS-016",
				Prompt: "Exercise issue link CRUD in project `my-org/tools/gitlab-mcp-server`: create source issue `eval-link-source`, create target issue `eval-link-target`, link source to target as `relates_to`, list source issue links, delete the returned issue link, then delete both issues.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_issue", ExpectedAction: "create", RequiredParams: []string{"project_id", "title"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "create", RequiredParams: []string{"project_id", "title"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "link_create", RequiredParams: []string{"project_id", "issue_iid", "target_project_id", "target_issue_iid"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "link_list", RequiredParams: []string{"project_id", "issue_iid"}},
					{ExpectedTool: "gitlab_issue", ExpectedAction: "link_delete", RequiredParams: []string{"project_id", "issue_iid", "issue_link_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"keep the source issue IID from the first create call", "params.issue_iid set to the source issue IID", "params.issue_link_id from the returned link"},
		},
		{
			name: "project badge URLs",
			task: evalTask{
				ID:     "MS-022",
				Prompt: "Exercise project badge CRUD in project `my-org/tools/gitlab-mcp-server`: add badge `eval-crud-badge`, fetch it with badge get using the returned badge ID, edit the badge name to `Evaluation CRUD badge link`, then delete it.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_add", RequiredParams: []string{"project_id", "link_url", "image_url"}},
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_get", RequiredParams: []string{"project_id", "badge_id"}},
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_edit", RequiredParams: []string{"project_id", "badge_id"}},
					{ExpectedTool: "gitlab_project", ExpectedAction: "badge_delete", RequiredParams: []string{"project_id", "badge_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"badge_add requires valid absolute params.link_url and params.image_url", "https://example.com/eval-badge", "https://example.com/eval-badge.svg", "badge_edit uses params.name", "never send new_name"},
		},
		{
			name: "branch unprotect",
			task: evalTask{
				ID:     "MS-028",
				Prompt: "Exercise branch protection lifecycle in project `my-org/tools/gitlab-mcp-server`: create branch `eval-protect-branch` from `main`, protect it with Maintainer push and merge access, fetch the protected branch, update it to allow force push, unprotect it, then delete the branch.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_branch", ExpectedAction: "create", RequiredParams: []string{"project_id", "branch_name", "ref"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "protect", RequiredParams: []string{"project_id", "branch_name"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "get_protected", RequiredParams: []string{"project_id", "branch_name"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "update_protected", RequiredParams: []string{"project_id", "branch_name"}},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "unprotect", RequiredParams: []string{"project_id", "branch_name"}, OptionalParams: []string{"confirm"}, Destructive: true},
					{ExpectedTool: "gitlab_branch", ExpectedAction: "delete", RequiredParams: []string{"project_id", "branch_name"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"params.push_access_level=40", "params.merge_access_level=40", "After protect succeeds, call get_protected next", "unprotect only uses params.project_id, params.branch_name, and params.confirm=true", "never send allow_force_push to unprotect", "For direct gitlab_branch meta-tool calls", `"action":"unprotect","params":{"project_id":"<project_id>","branch_name":"<branch_name>","confirm":true}`, `"action":"delete","params":{"project_id":"<project_id>","branch_name":"<branch_name>","confirm":true}`, "For dynamic mode with gitlab_execute_action", "top-level confirm:true"},
		},
		{
			name: "group milestone",
			task: evalTask{
				ID:     "MS-036",
				Prompt: "Exercise group milestone lifecycle in group `my-org`: create milestone `Evaluation Group Milestone` with due date `2026-12-31`, fetch it using the returned milestone IID, update title to `Evaluation Group Milestone v2`, then delete it.",
				Steps: []evalStep{
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_create", RequiredParams: []string{"group_id", "title"}, OptionalParams: []string{"description", "due_date"}},
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_get", RequiredParams: []string{"group_id", "milestone_iid"}},
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_update", RequiredParams: []string{"group_id", "milestone_iid"}},
					{ExpectedTool: "gitlab_group", ExpectedAction: "group_milestone_delete", RequiredParams: []string{"group_id", "milestone_iid"}, OptionalParams: []string{"confirm"}, Destructive: true},
				},
			},
			wants: []string{"Do not invent params.start_date unless the task provides an earlier start date", "call group_milestone_get with the returned milestone_iid before any update"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPrompt(tt.task)
			for _, want := range tt.wants {
				if !strings.Contains(prompt, want) {
					t.Fatalf("taskPrompt() = %q, want guidance containing %q", prompt, want)
				}
			}
		})
	}
}

// TestTaskPrompt_ProjectSnippetCRUDAvoidsProjectPrefetch verifies TaskPrompt when project snippet CRUD avoids project prefetch.
func TestTaskPrompt_ProjectSnippetCRUDAvoidsProjectPrefetch(t *testing.T) {
	task := evalTask{
		ID:     "MS-024",
		Prompt: "Exercise project snippet CRUD in project `my-org/tools/gitlab-mcp-server`: create project snippet `eval-crud-snippet` titled `Evaluation CRUD snippet`, fetch it with project snippet get using the returned snippet ID, update its content with a `files` entry using action `update` and `file_path` set to the returned file path, not `previous_path`, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_create", RequiredParams: []string{"project_id", "title", "file_name", "content"}},
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_get", RequiredParams: []string{"project_id", "snippet_id"}},
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_update", RequiredParams: []string{"project_id", "snippet_id"}},
			{ExpectedTool: "gitlab_snippet", ExpectedAction: "project_delete", RequiredParams: []string{"project_id", "snippet_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"the first call is gitlab_snippet with action project_create",
		"do not call gitlab_project first",
		"project_create requires params.project_id, params.title, params.file_name, and params.content",
		"Use the returned snippet_id for project_get, project_update, and project_delete",
		"project_update params should contain project_id, snippet_id, and files",
		"never send params.file_path or params.content at top level when using files[]",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want project snippet CRUD guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ProjectHookCRUDAvoidsGroupHooks verifies TaskPrompt when project hook CRUD avoids group hooks.
func TestTaskPrompt_ProjectHookCRUDAvoidsGroupHooks(t *testing.T) {
	task := evalTask{
		ID:     "MS-021",
		Prompt: "Exercise project hook CRUD in project `my-org/tools/gitlab-mcp-server`: add a hook, fetch it with hook get, edit it, then delete it.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_add", RequiredParams: []string{"project_id", "url"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_get", RequiredParams: []string{"project_id", "hook_id"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_edit", RequiredParams: []string{"project_id", "hook_id"}},
			{ExpectedTool: "gitlab_project", ExpectedAction: "hook_delete", RequiredParams: []string{"project_id", "hook_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"For project hook CRUD, use gitlab_project actions hook_add, hook_get, hook_edit, and hook_delete with params.project_id",
		"Do not use gitlab_group hook actions for a project hook workflow",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want project hook guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DiscussionResolveIncludesQuotedEnvelopeGuidance verifies TaskPrompt when discussion resolve includes quoted envelope guidance.
func TestTaskPrompt_DiscussionResolveIncludesQuotedEnvelopeGuidance(t *testing.T) {
	task := evalTask{
		ID:             "MT-061",
		Prompt:         "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "mr_review.discussion_resolve",
	}

	prompt := taskPrompt(task)
	if !strings.Contains(prompt, "emit tool gitlab_mr_review") ||
		!strings.Contains(prompt, `"action":"discussion_resolve"`) ||
		!strings.Contains(prompt, `action "mr_review.discussion_resolve"`) ||
		!strings.Contains(prompt, `"discussion_id":"<discussion_id>"`) {
		t.Fatalf("taskPrompt() = %q, want quoted discussion_resolve envelope guidance", prompt)
	}
}

// TestTaskPrompt_SplitDiscussionResolveUsesExactToolCall verifies TaskPrompt when split discussion resolve uses exact tool call.
func TestTaskPrompt_SplitDiscussionResolveUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-061",
		Prompt:         "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_mr_review",
		ExpectedAction: "discussion_resolve",
		RequiredParams: []string{"project_id", "merge_request_iid", "discussion_id"},
		OptionalParams: []string{"resolved"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call",
		"use the gitlab_mr_review tool once",
		`"action":"discussion_resolve"`,
		`"discussion_id":"abc123"`,
		`"merge_request_iid":7`,
		`"resolved":true`,
		"Return exactly one tool call and no text answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want split discussion_resolve guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SearchCodeAvoidsProjectDiscovery verifies TaskPrompt when search code avoids project discovery.
func TestTaskPrompt_SearchCodeAvoidsProjectDiscovery(t *testing.T) {
	task := evalTask{
		ID:             "MT-032",
		Prompt:         "Search code for `func RegisterMCPMeta` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "search.code",
	}

	prompt := taskPrompt(task)
	if !strings.Contains(prompt, `"action":"search.code"`) || !strings.Contains(prompt, "never remote_url") {
		t.Fatalf("taskPrompt() = %q, want search.code direct project_id guidance", prompt)
	}
}

// TestTaskPrompt_ReleaseCreateMapsFromRef verifies TaskPrompt when release create maps from ref.
func TestTaskPrompt_ReleaseCreateMapsFromRef(t *testing.T) {
	task := evalTask{
		ID:             "MT-036",
		Prompt:         "Create release `v0.0.0-eval` for tag `v0.0.0-eval` from ref `main` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_release",
		ExpectedAction: "create",
	}
	prompt := taskPrompt(task)

	if !strings.Contains(prompt, `For release.create, "from ref X" maps to params.ref`) {
		t.Fatalf("taskPrompt() = %q, want release ref guidance", prompt)
	}
}

// TestTaskPrompt_AdminSettingsUsesDispatcherDirectly verifies TaskPrompt when admin settings uses dispatcher directly.
func TestTaskPrompt_AdminSettingsUsesDispatcherDirectly(t *testing.T) {
	task := evalTask{
		ID:             "MT-052",
		Prompt:         "Show instance application settings.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "admin.settings_get",
	}

	prompt := taskPrompt(task)
	if !strings.Contains(prompt, `gitlab_admin with {"action":"settings_get","params":{}}`) || !strings.Contains(prompt, "gitlab_server") || !strings.Contains(prompt, "schema lookup") {
		t.Fatalf("taskPrompt() = %q, want direct admin.settings_get guidance", prompt)
	}
}

// TestTaskPrompt_ArtifactFromNumericJobUsesSingleArtifact verifies TaskPrompt when artifact from numeric job uses single artifact.
func TestTaskPrompt_ArtifactFromNumericJobUsesSingleArtifact(t *testing.T) {
	task := evalTask{
		ID:             "MT-065",
		Prompt:         "Download artifact `coverage/report.xml` from job `999` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_job",
		ExpectedAction: "download_single_artifact",
		RequiredParams: []string{"project_id", "job_id", "artifact_path"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{"Exact required call", "use the gitlab_job tool once", `"action":"download_single_artifact"`, `"job_id":999`, `"artifact_path":"coverage/report.xml"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want artifact guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_FailedPipelineJobsUseJobList verifies TaskPrompt when failed pipeline jobs use job list.
func TestTaskPrompt_FailedPipelineJobsUseJobList(t *testing.T) {
	task := evalTask{
		ID:     "MS-002",
		Prompt: "Investigate failed pipeline `12345` for project `my-org/tools/gitlab-mcp-server`: inspect the pipeline, list failed jobs, fetch job `999` trace.",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_pipeline", ExpectedAction: "get", RequiredParams: []string{"project_id", "pipeline_id"}},
			{ExpectedTool: "gitlab_job", ExpectedAction: "list", RequiredParams: []string{"project_id", "pipeline_id"}, OptionalParams: []string{"scope"}},
			{ExpectedTool: "gitlab_job", ExpectedAction: "trace", RequiredParams: []string{"project_id", "job_id"}},
		},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{"gitlab_job", `"action":"list"`, `"scope":"failed"`, "do not call gitlab_pipeline list with pipeline_id"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want failed-job guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_SingleFailedPipelineJobsUsesExactToolCall verifies TaskPrompt when single failed pipeline jobs uses exact tool call.
func TestTaskPrompt_SingleFailedPipelineJobsUsesExactToolCall(t *testing.T) {
	task := evalTask{
		ID:             "MT-021",
		Prompt:         "List failed jobs in pipeline `1323` for project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_job",
		ExpectedAction: "list",
		RequiredParams: []string{"project_id", "pipeline_id"},
		OptionalParams: []string{"scope"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		"Exact required call",
		"use the gitlab_job tool once",
		`"action":"list"`,
		`"pipeline_id":1323`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"scope":"failed"`,
		"Return exactly one tool call and no text answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want single failed-job guidance containing %q", prompt, want)
		}
	}

	system := systemPromptForTask(task, config.ToolSurfaceMeta)
	if !strings.Contains(system, "Return tool calls only") || strings.Contains(system, "runner.list_project") {
		t.Fatalf("systemPromptForTask() = %q, want compact exact-call system prompt", system)
	}
}

// TestTaskPrompt_SingleDestructiveSplitActionsUseExactToolCalls covers TaskPrompt with table-driven subtests for single destructive split actions use exact tool calls.
func TestTaskPrompt_SingleDestructiveSplitActionsUseExactToolCalls(t *testing.T) {
	tests := []struct {
		name   string
		task   evalTask
		wants  []string
		absent []string
	}{
		{
			name:  "job artifacts",
			task:  evalTask{ID: "MT-024", Prompt: "Delete artifacts for job `999` in project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_job", ExpectedAction: "delete_artifacts", RequiredParams: []string{"project_id", "job_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_job tool once", `"action":"delete_artifacts"`, `"job_id":999`, `"confirm":true`},
		},
		{
			name:  "wiki delete",
			task:  evalTask{ID: "MT-108", Prompt: "Delete wiki page `obsolete-eval` from project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_wiki", ExpectedAction: "delete", RequiredParams: []string{"project_id", "slug"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_wiki tool once", `"action":"delete"`, `"slug":"obsolete-eval"`, `"confirm":true`},
		},
		{
			name:  "mr emoji",
			task:  evalTask{ID: "MT-109", Prompt: "Remove award emoji ID `12` from merge request `7` in project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_merge_request", ExpectedAction: "emoji_mr_delete", RequiredParams: []string{"project_id", "merge_request_iid", "award_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_merge_request tool once", `"action":"emoji_mr_delete"`, `"award_id":12`, `"merge_request_iid":7`, "do not use gitlab_mr_review"},
		},
		{
			name:  "commit discussion note",
			task:  evalTask{ID: "MT-113", Prompt: "Delete commit discussion note `999` from discussion `abc123` on commit `abc1234` in project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_repository", ExpectedAction: "commit_discussion_delete_note", RequiredParams: []string{"project_id", "commit_sha", "discussion_id", "note_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
			wants: []string{"use the gitlab_repository tool once", `"action":"commit_discussion_delete_note"`, `"commit_sha":"abc1234"`, `"discussion_id":"abc123"`, `"note_id":999`},
		},
		{
			name:   "archive",
			task:   evalTask{ID: "MT-055", Prompt: "Archive project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_project", ExpectedAction: "archive", RequiredParams: []string{"project_id"}},
			wants:  []string{"use the gitlab_project tool once", `"action":"archive"`, `"project_id":"my-org/tools/gitlab-mcp-server"`},
			absent: []string{`"action":"delete"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := taskPrompt(tt.task)
			for _, want := range tt.wants {
				if !strings.Contains(prompt, want) {
					t.Fatalf("taskPrompt() = %q, want exact guidance containing %q", prompt, want)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(prompt, absent) {
					t.Fatalf("taskPrompt() = %q, want exact guidance without %q", prompt, absent)
				}
			}
		})
	}
}

// TestTaskPrompt_PipelineTriggerDeleteUsesTriggerID verifies TaskPrompt when pipeline trigger delete uses trigger ID.
func TestTaskPrompt_PipelineTriggerDeleteUsesTriggerID(t *testing.T) {
	task := evalTask{
		ID:             "MT-102",
		Prompt:         "Delete pipeline trigger token ID `77` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "pipeline.trigger_delete",
		RequiredParams: []string{"project_id", "trigger_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"pipeline.trigger_delete"`,
		`"trigger_id":77`,
		"Exact required call",
		"The supplied ID maps to the matching *_id param",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline trigger delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact pipeline trigger delete guidance without %q", prompt, unwanted)
		}
	}
	system := systemPromptForTask(task, config.ToolSurfaceMeta)
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(system, unwanted) {
			t.Fatalf("systemPromptForTask() = %q, want compact pipeline trigger delete system prompt without %q", system, unwanted)
		}
	}
}

// TestTaskPrompt_PipelineScheduleDeleteUsesScheduleID verifies TaskPrompt when pipeline schedule delete uses schedule ID.
func TestTaskPrompt_PipelineScheduleDeleteUsesScheduleID(t *testing.T) {
	task := evalTask{
		ID:             "MT-103",
		Prompt:         "Delete pipeline schedule ID `49` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "pipeline.schedule_delete",
		RequiredParams: []string{"project_id", "schedule_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"pipeline.schedule_delete"`,
		`"schedule_id":49`,
		"Exact required call",
		"The supplied ID maps to the matching *_id param",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want pipeline schedule delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact pipeline schedule delete guidance without %q", prompt, unwanted)
		}
	}
	system := systemPromptForTask(task, config.ToolSurfaceMeta)
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(system, unwanted) {
			t.Fatalf("systemPromptForTask() = %q, want compact pipeline schedule delete system prompt without %q", system, unwanted)
		}
	}
}

// TestTaskPrompt_UserBlockUsesUserID verifies TaskPrompt when user block uses user ID.
func TestTaskPrompt_UserBlockUsesUserID(t *testing.T) {
	task := evalTask{
		ID:             "MT-104",
		Prompt:         "Block user ID `69`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "user.block",
		RequiredParams: []string{"user_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"user.block"`,
		`"user_id":69`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want user block guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"runner_id", "target_branch", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact user block guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_FeatureFlagDeleteUsesName verifies TaskPrompt when feature flag delete uses name.
func TestTaskPrompt_FeatureFlagDeleteUsesName(t *testing.T) {
	task := evalTask{
		ID:             "MT-106",
		Prompt:         "Delete feature flag `eval_flag` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "feature_flags.feature_flag_delete",
		RequiredParams: []string{"project_id", "name"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"feature_flags.feature_flag_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"name":"eval_flag"`,
		"Exact required call",
		"The supplied values map to the matching params",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want feature flag delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact feature flag delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_WikiDeleteUsesSlug verifies TaskPrompt when wiki delete uses slug.
func TestTaskPrompt_WikiDeleteUsesSlug(t *testing.T) {
	task := evalTask{
		ID:             "MT-108",
		Prompt:         "Delete wiki page `obsolete-eval` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "wiki.delete",
		RequiredParams: []string{"project_id", "slug"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"wiki.delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"slug":"obsolete-eval"`,
		"Exact required call",
		"The supplied values map to the matching params",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want wiki delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"target_branch", "tag_name", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact wiki delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_MRAwardDeleteUsesAwardID verifies TaskPrompt when MR award delete uses award ID.
func TestTaskPrompt_MRAwardDeleteUsesAwardID(t *testing.T) {
	task := evalTask{
		ID:             "MT-109",
		Prompt:         "Remove award emoji ID `21` from merge request `1` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "merge_request.emoji_mr_delete",
		RequiredParams: []string{"project_id", "merge_request_iid", "award_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"merge_request.emoji_mr_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"merge_request_iid":1`,
		`"award_id":21`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want MR award delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"mr_review.emoji_mr_note_delete", "note_id", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact MR award delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_IssueAwardDeleteUsesAwardID verifies TaskPrompt when issue award delete uses award ID.
func TestTaskPrompt_IssueAwardDeleteUsesAwardID(t *testing.T) {
	task := evalTask{
		ID:             "MT-110",
		Prompt:         "Remove award emoji ID `22` from issue `42` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "issue.emoji_issue_delete",
		RequiredParams: []string{"project_id", "issue_iid", "award_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"issue.emoji_issue_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"issue_iid":42`,
		`"award_id":22`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want issue award delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"note_id", "target_branch", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact issue award delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_DeployKeyDeleteUsesDeployKeyID verifies TaskPrompt when deploy key delete uses deploy key ID.
func TestTaskPrompt_DeployKeyDeleteUsesDeployKeyID(t *testing.T) {
	task := evalTask{
		ID:             "MT-111",
		Prompt:         "Delete deploy key ID `32` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "access.deploy_key_delete",
		RequiredParams: []string{"project_id", "deploy_key_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"access.deploy_key_delete"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"deploy_key_id":32`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want deploy key delete guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DeployTokenDeleteUsesDeployTokenID verifies TaskPrompt when deploy token delete uses deploy token ID.
func TestTaskPrompt_DeployTokenDeleteUsesDeployTokenID(t *testing.T) {
	task := evalTask{
		ID:             "MT-112",
		Prompt:         "Delete project deploy token ID `66` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "access.deploy_token_delete_project",
		RequiredParams: []string{"project_id", "deploy_token_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"access.deploy_token_delete_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"deploy_token_id":66`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want deploy token delete guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_CommitDiscussionDeleteUsesDiscussionAndNote verifies TaskPrompt when commit discussion delete uses discussion and note.
func TestTaskPrompt_CommitDiscussionDeleteUsesDiscussionAndNote(t *testing.T) {
	task := evalTask{
		ID:             "MT-113",
		Prompt:         "Delete commit discussion note `999` from discussion `abc123` on commit `abc1234` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "repository.commit_discussion_delete_note",
		RequiredParams: []string{"project_id", "commit_sha", "discussion_id", "note_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"repository.commit_discussion_delete_note"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"commit_sha":"abc1234"`,
		`"discussion_id":"abc123"`,
		`"note_id":999`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want commit discussion delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"issue.discussion_delete_note", "merge_request_iid", "params.variables"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want compact commit discussion delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_AttestationDownloadUsesAttestationIID verifies TaskPrompt when attestation download uses attestation IID.
func TestTaskPrompt_AttestationDownloadUsesAttestationIID(t *testing.T) {
	task := evalTask{
		ID:             "MT-117",
		Prompt:         "Download attestation IID `5` from project `my-org/tools/gitlab-mcp-server`; use the project-scoped attestation IID, not the database ID.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "attestation.download",
		RequiredParams: []string{"project_id", "attestation_iid"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"attestation.download"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"attestation_iid":5`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want attestation guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_AuditEventGetUsesEventID verifies TaskPrompt when audit event get uses event ID.
func TestTaskPrompt_AuditEventGetUsesEventID(t *testing.T) {
	task := evalTask{
		ID:             "MT-118",
		Prompt:         "Get instance audit event ID `77`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "audit_event.get_instance",
		RequiredParams: []string{"event_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"audit_event.get_instance"`,
		`"event_id":77`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want audit event get guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "user_id") {
		t.Fatalf("taskPrompt() = %q, want event_id guidance without user_id", prompt)
	}
}

// TestTaskPrompt_AuditEventListUsesCreatedRange verifies TaskPrompt when audit event list uses created range.
func TestTaskPrompt_AuditEventListUsesCreatedRange(t *testing.T) {
	task := evalTask{
		ID:             "MT-119",
		Prompt:         "List project audit events for project `my-org/tools/gitlab-mcp-server` created during January 2026.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "audit_event.list_project",
		RequiredParams: []string{"project_id"},
		OptionalParams: []string{"created_after", "created_before", "per_page"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"audit_event.list_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"created_after":"2026-01-01"`,
		`"created_before":"2026-02-01"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want audit event list guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_CompliancePolicyUpdateUsesNamespaceID verifies TaskPrompt when compliance policy update uses namespace ID.
func TestTaskPrompt_CompliancePolicyUpdateUsesNamespaceID(t *testing.T) {
	task := evalTask{
		ID:             "MT-120",
		Prompt:         "Update the admin compliance policy settings to use namespace ID `123`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "compliance_policy.update",
		RequiredParams: []string{"csp_namespace_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"compliance_policy.update"`,
		`"csp_namespace_id":123`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want compliance policy guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "issue_iid") {
		t.Fatalf("taskPrompt() = %q, want csp_namespace_id guidance without issue_iid", prompt)
	}
}

// TestTaskPrompt_DependencyExportCreateUsesPipelineID verifies TaskPrompt when dependency export create uses pipeline ID.
func TestTaskPrompt_DependencyExportCreateUsesPipelineID(t *testing.T) {
	task := evalTask{
		ID:             "MT-121",
		Prompt:         "Create a dependency list export for pipeline ID `12345`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "dependency.export_create",
		RequiredParams: []string{"pipeline_id"},
		OptionalParams: []string{"export_type"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"dependency.export_create"`,
		`"pipeline_id":12345`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want dependency export create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_DependencyExportDownloadUsesExportID verifies TaskPrompt when dependency export download uses export ID.
func TestTaskPrompt_DependencyExportDownloadUsesExportID(t *testing.T) {
	task := evalTask{
		ID:             "MT-122",
		Prompt:         "Download dependency list export ID `987`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "dependency.export_download",
		RequiredParams: []string{"export_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"dependency.export_download"`,
		`"export_id":987`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want dependency export download guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"project_id", "attestation_iid"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want export_id guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_DORAMetricsGroupUsesMetric verifies TaskPrompt when dora metrics group uses metric.
func TestTaskPrompt_DORAMetricsGroupUsesMetric(t *testing.T) {
	task := evalTask{
		ID:             "MT-123",
		Prompt:         "Get group DORA lead time metrics for group `my-org` from `2026-01-01` to `2026-01-31`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "dora_metrics.group",
		RequiredParams: []string{"group_id", "metric"},
		OptionalParams: []string{"start_date", "end_date", "interval", "environment_tiers"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"dora_metrics.group"`,
		`"group_id":"my-org"`,
		`"metric":"lead_time_for_changes"`,
		`"start_date":"2026-01-01"`,
		`"end_date":"2026-01-31"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want DORA guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_EnterpriseUserGetUsesGroupAndUserID verifies TaskPrompt when enterprise user get uses group and user ID.
func TestTaskPrompt_EnterpriseUserGetUsesGroupAndUserID(t *testing.T) {
	task := evalTask{
		ID:             "MT-124",
		Prompt:         "Get enterprise user ID `55` in group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "enterprise_user.get",
		RequiredParams: []string{"group_id", "user_id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"enterprise_user.get"`,
		`"group_id":"my-org"`,
		`"user_id":55`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want enterprise user guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_EnterpriseUserDisable2FAUsesEnterpriseAction verifies TaskPrompt uses enterprise action for enterprise user disable 2FA.
func TestTaskPrompt_EnterpriseUserDisable2FAUsesEnterpriseAction(t *testing.T) {
	task := evalTask{
		ID:             "MT-125",
		Prompt:         "Disable two-factor authentication for enterprise user ID `55` in group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "enterprise_user.disable_2fa",
		RequiredParams: []string{"group_id", "user_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"enterprise_user.disable_2fa"`,
		`"group_id":"my-org"`,
		`"user_id":55`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want enterprise 2FA guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "user.disable_two_factor") {
		t.Fatalf("taskPrompt() = %q, want enterprise action guidance without base user 2FA action", prompt)
	}
}

// TestTaskPrompt_ExternalStatusCheckCreateUsesExternalURL verifies TaskPrompt when external status check create uses external URL.
func TestTaskPrompt_ExternalStatusCheckCreateUsesExternalURL(t *testing.T) {
	task := evalTask{
		ID:             "MT-126",
		Prompt:         "Create external project status check `Eval Gate` on project `my-org/tools/gitlab-mcp-server` pointing at `https://example.com/check`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "external_status_check.create_project",
		RequiredParams: []string{"project_id", "name", "external_url"},
		OptionalParams: []string{"shared_secret", "protected_branch_ids"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"external_status_check.create_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"name":"Eval Gate"`,
		`"external_url":"https://example.com/check"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want external check create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ExternalStatusCheckStatusUsesCheckID verifies TaskPrompt when external status check status uses check ID.
func TestTaskPrompt_ExternalStatusCheckStatusUsesCheckID(t *testing.T) {
	task := evalTask{
		ID:             "MT-127",
		Prompt:         "Mark external status check ID `8` as passed for merge request IID `7` at SHA `abc123` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "external_status_check.set_project_mr_status",
		RequiredParams: []string{"project_id", "merge_request_iid", "sha", "external_status_check_id", "status"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"external_status_check.set_project_mr_status"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"merge_request_iid":7`,
		`"sha":"abc123"`,
		`"external_status_check_id":8`,
		`"status":"passed"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want external check status guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_ExternalStatusCheckDeleteUsesCheckID verifies TaskPrompt when external status check delete uses check ID.
func TestTaskPrompt_ExternalStatusCheckDeleteUsesCheckID(t *testing.T) {
	task := evalTask{
		ID:             "MT-128",
		Prompt:         "Delete external project status check ID `8` from project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "external_status_check.delete_project",
		RequiredParams: []string{"project_id", "check_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"external_status_check.delete_project"`,
		`"project_id":"my-org/tools/gitlab-mcp-server"`,
		`"check_id":8`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want external check delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"rule_id", "deploy_key_id"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want check_id guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_GeoGetUsesID verifies TaskPrompt when geo get uses ID.
func TestTaskPrompt_GeoGetUsesID(t *testing.T) {
	task := evalTask{
		ID:             "MT-129",
		Prompt:         "Get Geo site ID `3`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "geo.get",
		RequiredParams: []string{"id"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"geo.get"`,
		`"id":3`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want Geo get guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GeoCreateUsesEnabledAndPrimary verifies TaskPrompt when geo create uses enabled and primary.
func TestTaskPrompt_GeoCreateUsesEnabledAndPrimary(t *testing.T) {
	task := evalTask{
		ID:             "MT-130",
		Prompt:         "Create a disabled Geo secondary site named `eval-geo` with URL `https://geo.example.com`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "geo.create",
		RequiredParams: []string{"name", "url"},
		OptionalParams: []string{"enabled", "primary"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"geo.create"`,
		`"name":"eval-geo"`,
		`"url":"https://geo.example.com"`,
		`"enabled":false`,
		`"primary":false`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want Geo create guidance containing %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "paused") {
		t.Fatalf("taskPrompt() = %q, want Geo create guidance without paused", prompt)
	}
}

// TestTaskPrompt_GeoDeleteUsesID verifies TaskPrompt when geo delete uses ID.
func TestTaskPrompt_GeoDeleteUsesID(t *testing.T) {
	task := evalTask{
		ID:             "MT-131",
		Prompt:         "Delete Geo site ID `3`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "geo.delete",
		RequiredParams: []string{"id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"geo.delete"`,
		`"id":3`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want Geo delete guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"geo_node_id", "site_id", "path"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want Geo delete guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_GroupCredentialListUsesCredentialAction verifies TaskPrompt when group credential list uses credential action.
func TestTaskPrompt_GroupCredentialListUsesCredentialAction(t *testing.T) {
	task := evalTask{
		ID:             "MT-133",
		Prompt:         "List group personal access tokens for group `my-org`, filtering active tokens.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.credential_list_pats",
		RequiredParams: []string{"group_id"},
		OptionalParams: []string{"state", "per_page"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.credential_list_pats"`,
		`"group_id":"my-org"`,
		`"state":"active"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group credential list guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupCredentialRevokeUsesTokenID verifies TaskPrompt when group credential revoke uses token ID.
func TestTaskPrompt_GroupCredentialRevokeUsesTokenID(t *testing.T) {
	task := evalTask{
		ID:             "MT-134",
		Prompt:         "Revoke group personal access token ID `77` in group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.credential_revoke_pat",
		RequiredParams: []string{"group_id", "token_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.credential_revoke_pat"`,
		`"group_id":"my-org"`,
		`"token_id":77`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group credential revoke guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicBoardListUsesEpicBoardAction verifies TaskPrompt when group epic board list uses epic board action.
func TestTaskPrompt_GroupEpicBoardListUsesEpicBoardAction(t *testing.T) {
	task := evalTask{
		ID:             "MT-135",
		Prompt:         "List epic boards for group `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_board_list",
		RequiredParams: []string{"group_id"},
		OptionalParams: []string{"per_page"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_board_list"`,
		`"group_id":"my-org"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic board list guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicListUsesFullPath verifies TaskPrompt when group epic list uses full path.
func TestTaskPrompt_GroupEpicListUsesFullPath(t *testing.T) {
	task := evalTask{
		ID:             "MT-136",
		Prompt:         "List epics in group full path `my-org` including descendant groups.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_list",
		RequiredParams: []string{"full_path"},
		OptionalParams: []string{"include_descendants", "state", "first"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_list"`,
		`"full_path":"my-org"`,
		`"include_descendants":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic list guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"group_path", "group_id", "include_descendant_groups"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want group epic list guidance without %q", prompt, unwanted)
		}
	}
}

// TestTaskPrompt_GroupEpicCreateUsesFullPathAndTitle verifies TaskPrompt when group epic create uses full path and title.
func TestTaskPrompt_GroupEpicCreateUsesFullPathAndTitle(t *testing.T) {
	task := evalTask{
		ID:             "MT-137",
		Prompt:         "Create an epic titled `Evaluation Epic` in group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_create",
		RequiredParams: []string{"full_path", "title"},
		OptionalParams: []string{"description", "start_date", "due_date"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_create"`,
		`"full_path":"my-org"`,
		`"title":"Evaluation Epic"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic create guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicUpdateUsesEpicIID verifies TaskPrompt when group epic update uses epic IID.
func TestTaskPrompt_GroupEpicUpdateUsesEpicIID(t *testing.T) {
	task := evalTask{
		ID:             "MT-138",
		Prompt:         "Update epic IID `12` in group full path `my-org` to close it.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_update",
		RequiredParams: []string{"full_path", "epic_iid"},
		OptionalParams: []string{"state_event", "title"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_update"`,
		`"full_path":"my-org"`,
		`"epic_iid":12`,
		`"state_event":"close"`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic update guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicDeleteUsesEpicIID verifies TaskPrompt when group epic delete uses epic IID.
func TestTaskPrompt_GroupEpicDeleteUsesEpicIID(t *testing.T) {
	task := evalTask{
		ID:             "MT-139",
		Prompt:         "Delete epic IID `12` from group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_delete",
		RequiredParams: []string{"full_path", "epic_iid"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_delete"`,
		`"full_path":"my-org"`,
		`"epic_iid":12`,
		`"confirm":true`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic delete guidance containing %q", prompt, want)
		}
	}
}

// TestTaskPrompt_GroupEpicIssueAssignUsesChildParams verifies TaskPrompt when group epic issue assign uses child params.
func TestTaskPrompt_GroupEpicIssueAssignUsesChildParams(t *testing.T) {
	task := evalTask{
		ID:             "MT-140",
		Prompt:         "Assign issue IID `99` from child project path `my-org/tools/gitlab-mcp-server` to epic IID `12` in group full path `my-org`.",
		ExpectedTool:   "gitlab",
		ExpectedAction: "group.epic_issue_assign",
		RequiredParams: []string{"full_path", "epic_iid", "child_project_path", "child_iid"},
	}

	prompt := taskPrompt(task)
	for _, want := range []string{
		`"action":"group.epic_issue_assign"`,
		`"full_path":"my-org"`,
		`"epic_iid":12`,
		`"child_project_path":"my-org/tools/gitlab-mcp-server"`,
		`"child_iid":99`,
		"Exact required call",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("taskPrompt() = %q, want group epic issue assign guidance containing %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"project_id", "issue_iid", "target_full_path"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("taskPrompt() = %q, want group epic issue assign guidance without %q", prompt, unwanted)
		}
	}
}
