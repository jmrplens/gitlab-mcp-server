package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ffuserlists"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/instancevariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projecttemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedpackages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourcegroups"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerPipelineMeta registers the gitlab_pipeline meta-tool with actions:
// list, get, cancel, retry, and delete.
func registerPipelineMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                              routeAction(client, pipelines.List),
		"get":                               routeAction(client, pipelines.Get),
		"cancel":                            routeAction(client, pipelines.Cancel),
		"retry":                             routeAction(client, pipelines.Retry),
		"delete":                            destructiveVoidAction(client, pipelines.Delete),
		"variables":                         routeAction(client, pipelines.GetVariables),
		"test_report":                       routeAction(client, pipelines.GetTestReport),
		"test_report_summary":               routeAction(client, pipelines.GetTestReportSummary),
		"latest":                            routeAction(client, pipelines.GetLatest),
		"create":                            routeAction(client, pipelines.Create),
		"update_metadata":                   routeAction(client, pipelines.UpdateMetadata),
		"wait":                              routeActionWithRequest(client, pipelines.Wait),
		"trigger_list":                      routeAction(client, pipelinetriggers.ListTriggers),
		"trigger_get":                       routeAction(client, pipelinetriggers.GetTrigger),
		"trigger_create":                    routeAction(client, pipelinetriggers.CreateTrigger),
		"trigger_update":                    routeAction(client, pipelinetriggers.UpdateTrigger),
		"trigger_delete":                    destructiveVoidAction(client, pipelinetriggers.DeleteTrigger),
		"trigger_run":                       routeAction(client, pipelinetriggers.RunTrigger),
		"resource_group_list":               routeAction(client, resourcegroups.ListAll),
		"resource_group_get":                routeAction(client, resourcegroups.Get),
		"resource_group_edit":               routeAction(client, resourcegroups.Edit),
		"resource_group_upcoming_jobs":      routeAction(client, resourcegroups.ListUpcomingJobs),
		"schedule_list":                     routeAction(client, pipelineschedules.List),
		"schedule_get":                      routeAction(client, pipelineschedules.Get),
		"schedule_create":                   routeAction(client, pipelineschedules.Create),
		"schedule_update":                   routeAction(client, pipelineschedules.Update),
		"schedule_delete":                   destructiveVoidAction(client, pipelineschedules.Delete),
		"schedule_run":                      routeAction(client, pipelineschedules.Run),
		"schedule_take_ownership":           routeAction(client, pipelineschedules.TakeOwnership),
		"schedule_create_variable":          routeAction(client, pipelineschedules.CreateVariable),
		"schedule_edit_variable":            routeAction(client, pipelineschedules.EditVariable),
		"schedule_delete_variable":          destructiveVoidAction(client, pipelineschedules.DeleteVariable),
		"schedule_list_triggered_pipelines": routeAction(client, pipelineschedules.ListTriggeredPipelines),
	}

	addMetaTool(server, "gitlab_pipeline", `Manage GitLab CI/CD pipelines plus trigger tokens, resource groups (mutual-exclusion locks), JUnit test reports, and pipeline schedules. Delete permanently removes a pipeline and all its jobs.
When to use: pipeline CRUD on a project, retry/cancel a run, fetch CI variables and JUnit test reports, manage trigger tokens, resource groups (mutual-exclusion locks), scheduled pipelines and their variables.
NOT for: jobs, logs, artifacts, manual play actions (use gitlab_job), MR-specific pipelines (use gitlab_merge_request 'pipelines' / 'create_pipeline'), CI lint or includes (use gitlab_template).

Behavior:
- Idempotent reads: list / latest / get / variables / test_report / test_report_summary / trigger_list / trigger_get / resource_group_list / resource_group_get / resource_group_upcoming_jobs / schedule_list / schedule_get / schedule_list_triggered_pipelines.
- create / schedule_run / trigger_run start a NEW run on every call (NON-idempotent — produce a fresh pipeline_id). retry re-queues failed/canceled jobs on the existing pipeline (same pipeline_id; continue using it for subsequent get/wait calls). cancel is idempotent (no-op once final). update_metadata / trigger_update / resource_group_edit / schedule_update / schedule_edit_variable / schedule_take_ownership are idempotent (same input → same state).
- Side effects: create / retry / schedule_run / trigger_run queue runners, consume CI minutes, may trigger downstream pipelines, deployments and webhooks. trigger_create returns a secret token visible only ONCE — store it immediately. wait blocks server-side until terminal state or timeout.
- Destructive: delete permanently removes the pipeline and all its jobs, artifacts, logs and traces (irreversible). trigger_delete / schedule_delete / schedule_delete_variable are irreversible.

Returns:
- list / latest / variables / test_report / test_report_summary / trigger_list / resource_group_list / resource_group_upcoming_jobs / schedule_list / schedule_list_triggered_pipelines: array(s) or aggregated payloads with pagination where applicable.
- get / create / cancel / retry / update_metadata / wait / trigger_get / trigger_create / trigger_update / trigger_run / resource_group_get / resource_group_edit / schedule_get / schedule_create / schedule_update / schedule_run / schedule_take_ownership / schedule_create_variable / schedule_edit_variable: pipeline / trigger / resource group / schedule object.
- delete / trigger_delete / schedule_delete / schedule_delete_variable: {success, message}.
Errors: 404 (hint: pipeline_id and trigger/schedule IDs are project-scoped), 403 (hint: requires Maintainer+ to delete pipelines or manage triggers/schedules), 400 (hint: cron expressions must use 5 fields; cron_timezone must be a valid TZ name; create requires 'ref').

Param conventions: * = required. All pipeline actions need project_id*. List actions accept page, per_page.

Pipelines:
- list: project_id*, status (success/failed/running/pending/canceled), scope, source, ref, sha, username
- get / cancel / retry / variables / test_report / test_report_summary: project_id*, pipeline_id*
- delete: project_id*, pipeline_id*. PERMANENTLY removes pipeline and jobs.
- latest: project_id*, ref
- create: project_id*, ref*, variables (array of {key, value, variable_type})
- update_metadata: project_id*, pipeline_id*, name*
- wait: project_id*, pipeline_id*, interval_seconds (5-60, default 10), timeout_seconds (1-3600, default 300), fail_on_error (default true)

Triggers:
- trigger_list: project_id*
- trigger_get / trigger_delete: project_id*, trigger_id*
- trigger_create: project_id*, description*
- trigger_update: project_id*, trigger_id*, description
- trigger_run: project_id*, ref*, token*, variables (map)

Resource groups:
- resource_group_list: project_id*
- resource_group_get / resource_group_edit: project_id*, key*. Edit params: process_mode.
- resource_group_upcoming_jobs: project_id*, key*

Schedules:
- schedule_list: project_id*, scope (active/inactive)
- schedule_get / schedule_delete / schedule_run / schedule_take_ownership: project_id*, schedule_id*
- schedule_create: project_id*, description*, ref*, cron*, cron_timezone, active
- schedule_update: project_id*, schedule_id*, description, ref, cron, cron_timezone, active
- schedule_create_variable: project_id*, schedule_id*, key*, value*, variable_type (env_var/file)
- schedule_edit_variable: project_id*, schedule_id*, key*, value*, variable_type
- schedule_delete_variable: project_id*, schedule_id*, key*
- schedule_list_triggered_pipelines: project_id*, schedule_id*

See also: gitlab_job (job details/logs/artifacts), gitlab_merge_request, gitlab_ci_variable`, routes, toolutil.IconPipeline)
}

// registerJobMeta registers the gitlab_job meta-tool with actions:
// list, list_project, get, trace, cancel, retry, wait, list_bridges, artifacts, download_artifacts,
// download_single_artifact, download_single_artifact_by_ref, erase, keep_artifacts, play,
// delete_artifacts, delete_project_artifacts.
func registerJobMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                            routeAction(client, jobs.List),
		"list_project":                    routeAction(client, jobs.ListProject),
		"get":                             routeAction(client, jobs.Get),
		"trace":                           routeAction(client, jobs.Trace),
		"cancel":                          routeAction(client, jobs.Cancel),
		"retry":                           routeAction(client, jobs.Retry),
		"list_bridges":                    routeAction(client, jobs.ListBridges),
		"artifacts":                       routeAction(client, jobs.GetArtifacts),
		"download_artifacts":              routeAction(client, jobs.DownloadArtifacts),
		"download_single_artifact":        routeAction(client, jobs.DownloadSingleArtifact),
		"download_single_artifact_by_ref": routeAction(client, jobs.DownloadSingleArtifactByRef),
		"erase":                           destructiveAction(client, jobs.Erase),
		"keep_artifacts":                  routeAction(client, jobs.KeepArtifacts),
		"play":                            routeAction(client, jobs.Play),
		"delete_artifacts":                destructiveVoidAction(client, jobs.DeleteArtifacts),
		"delete_project_artifacts":        destructiveVoidAction(client, jobs.DeleteProjectArtifacts),
		"wait":                            routeActionWithRequest(client, jobs.Wait),
		"token_scope_get":                 routeAction(client, jobtokenscope.GetAccessSettings),
		"token_scope_patch":               routeAction(client, jobtokenscope.PatchAccessSettings),
		"token_scope_list_inbound":        routeAction(client, jobtokenscope.ListInboundAllowlist),
		"token_scope_add_project":         routeAction(client, jobtokenscope.AddProjectAllowlist),
		"token_scope_remove_project":      destructiveVoidAction(client, jobtokenscope.RemoveProjectAllowlist),
		"token_scope_list_groups":         routeAction(client, jobtokenscope.ListGroupAllowlist),
		"token_scope_add_group":           routeAction(client, jobtokenscope.AddGroupAllowlist),
		"token_scope_remove_group":        destructiveVoidAction(client, jobtokenscope.RemoveGroupAllowlist),
	}

	addMetaTool(server, "gitlab_job", `Manage GitLab CI/CD jobs and the CI/CD job token scope: lifecycle, manual play, log/artifact retrieval, and inbound trust boundaries. Erase/delete actions are destructive.
When to use: job details, logs, artifacts, retry/cancel jobs, job token scope. NOT for: pipeline-level operations (use gitlab_pipeline).

Behavior:
- Idempotent reads: list / list_project / get / trace / artifacts / download_artifacts / download_single_artifact / download_single_artifact_by_ref / list_bridges / token_scope_get / token_scope_list_inbound / token_scope_list_groups.
- retry starts a NEW job run on every call (NON-idempotent — returns a fresh job_id). play activates an existing manual job that has not yet run (same job_id; only manual jobs with rules.when=manual are eligible) and may pass new variables. cancel is idempotent (no-op once final). keep_artifacts / token_scope_patch / token_scope_add_project / token_scope_add_group are idempotent.
- Side effects: retry / play queue runners, consume CI minutes, and may trigger downstream pipelines and notifications. trace returns up to 100KB of log; download_artifacts streams up to 1MB inline (base64).
- Destructive: erase clears the job log and artifacts in place (irreversible); delete_artifacts removes a single job's artifacts; delete_project_artifacts wipes ALL artifacts across the project (irreversible). token_scope_remove_* tightens trust boundaries and may break running pipelines.

Param conventions: * = required. All job actions need project_id*. List actions accept page, per_page.

Jobs:
- list: project_id*, pipeline_id*, scope
- list_project: project_id*, scope, include_retried
- get: project_id*, job_id*
- trace: project_id*, job_id*. Returns job log (truncated to 100KB).
- cancel / retry / erase / keep_artifacts: project_id*, job_id*
- play: project_id*, job_id*, variables (array of {key, value, variable_type})
- wait: project_id*, job_id*, interval_seconds (5-60, default 10), timeout_seconds (1-3600, default 300), fail_on_error (default true)
- list_bridges: project_id*, pipeline_id*, scope
- delete_artifacts: project_id*, job_id*
- delete_project_artifacts: project_id*. Deletes ALL artifacts across project.

Artifact downloads (base64, max 1MB):
- artifacts: project_id*, job_id* — download the whole artifact archive from a known numeric job ID.
- download_artifacts: project_id*, ref_name*, job* — download the whole artifact archive by ref_name and job NAME (string). Never use with job_id.
- download_single_artifact: project_id*, job_id*, artifact_path* — use when the prompt gives a numeric job ID and one artifact file path such as coverage/report.xml. This is the single-file-by-job-id action.
- download_single_artifact_by_ref: project_id*, ref_name*, artifact_path*, job* — use when the prompt gives ref_name plus job NAME and one artifact file path. Never use with job_id.

Job token scope:
- token_scope_get / token_scope_patch: project_id*. Patch params: enabled.
- token_scope_list_inbound: project_id*
- token_scope_add_project / token_scope_remove_project: project_id*, target_project_id*
- token_scope_list_groups: project_id*
- token_scope_add_group / token_scope_remove_group: project_id*, target_group_id*

See also: gitlab_pipeline, gitlab_repository`, routes, toolutil.IconJob)
}

// registerEnvironmentMeta registers the gitlab_environment meta-tool with actions:
// list, get, create, update, delete, stop.
func registerEnvironmentMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                         routeAction(client, environments.List),
		"get":                          routeAction(client, environments.Get),
		"create":                       routeAction(client, environments.Create),
		"update":                       routeAction(client, environments.Update),
		"delete":                       destructiveVoidAction(client, environments.Delete),
		"stop":                         destructiveAction(client, environments.Stop),
		"protected_list":               routeAction(client, protectedenvs.List),
		"protected_get":                routeAction(client, protectedenvs.Get),
		"protected_protect":            routeAction(client, protectedenvs.Protect),
		"protected_update":             routeAction(client, protectedenvs.Update),
		"protected_unprotect":          destructiveVoidAction(client, protectedenvs.Unprotect),
		"freeze_list":                  routeAction(client, freezeperiods.List),
		"freeze_get":                   routeAction(client, freezeperiods.Get),
		"freeze_create":                routeAction(client, freezeperiods.Create),
		"freeze_update":                routeAction(client, freezeperiods.Update),
		"freeze_delete":                destructiveVoidAction(client, freezeperiods.Delete),
		"deployment_list":              routeAction(client, deployments.List),
		"deployment_get":               routeAction(client, deployments.Get),
		"deployment_create":            routeAction(client, deployments.Create),
		"deployment_update":            routeAction(client, deployments.Update),
		"deployment_delete":            destructiveVoidAction(client, deployments.Delete),
		"deployment_approve_or_reject": routeAction(client, deployments.ApproveOrReject),
		"deployment_merge_requests":    routeAction(client, deploymentmergerequests.List),
	}

	addMetaTool(server, "gitlab_environment", `Manage GitLab deployment environments, protected environments, freeze (deploy block) periods, and the deployment record audit trail. Delete and stop are destructive (stop terminates the running env; force=true skips on-stop jobs).
When to use: define/update environments (production, staging, review/*), restrict who can deploy via protected environments, schedule deploy freezes, audit deployment history, approve/reject deployments awaiting manual gate.
NOT for: CI/CD variables scoped to environments (use gitlab_ci_variable), pipelines/jobs (use gitlab_pipeline / gitlab_job), feature flag rollout strategies (use gitlab_feature_flags).

Behavior:
- Idempotent reads: list / get / protected_list / protected_get / freeze_list / freeze_get / deployment_list / deployment_get / deployment_merge_requests.
- update / protected_update / freeze_update / deployment_update are idempotent (same input → same state). create / protected_protect / freeze_create / deployment_create are NON-idempotent on duplicate (project_id, name) — return 409. deployment_approve_or_reject is single-shot per (deployment_id, user) and cannot be reversed.
- Side effects: stop runs the on-stop CI job (unless force=true) and terminates any review-app resources; deployment_approve_or_reject may release queued CI jobs awaiting a manual gate; freeze_create immediately blocks deploys that match the cron window.
- Destructive: delete and stop are destructive — stop cannot be reversed without re-deploying; deployment_delete removes the deployment audit record (history loss).

Returns: resource object (environment / protection / freeze / deployment) for *_get/*_create/*_update/*_protect; paginated array for *_list; updated deployment with approval state for deployment_approve_or_reject; MR list for deployment_merge_requests; {success, message} for *_delete/*_unprotect/stop.
Errors: 404 not found, 403 forbidden (hint: protect/unprotect require Maintainer+), 400 invalid params (hint: tier ∈ production/staging/testing/development/other; freeze cron timezone must be valid TZ name).

Param conventions: * = required. All actions need project_id*. environment_id is the numeric ID returned by list/create.

Environments:
- list: project_id*, name, search, states (available/stopped/stopping), page, per_page
- get / delete: project_id*, environment_id*
- create: project_id*, name*, description, external_url, tier (production/staging/testing/development/other)
- update: project_id*, environment_id*, name, description, external_url, tier
- stop: project_id*, environment_id*, force (bool) — force skips on-stop jobs

Protected environments:
- protected_list: project_id*, page, per_page
- protected_get / protected_unprotect: project_id*, environment* (environment name; name is accepted as an alias)
- protected_protect: project_id*, name*, deploy_access_levels, approval_rules
- protected_update: project_id*, environment*, name, deploy_access_levels, approval_rules

Freeze periods (cron expressions):
- freeze_list: project_id*, page, per_page
- freeze_get / freeze_delete: project_id*, freeze_period_id*
- freeze_create: project_id*, freeze_start* (cron, e.g. '0 23 * * 5'), freeze_end* (cron), cron_timezone
- freeze_update: project_id*, freeze_period_id*, freeze_start, freeze_end, cron_timezone

Deployments (immutable history records):
- deployment_list: project_id*, order_by, sort, environment, status, page, per_page
- deployment_get / deployment_delete: project_id*, deployment_id*
- deployment_create: project_id*, environment*, ref*, sha*, tag (bool), status (created/running/success/failed/canceled)
- deployment_update: project_id*, deployment_id*, status*
- deployment_approve_or_reject: project_id*, deployment_id*, status* (approved/rejected), comment
- deployment_merge_requests: project_id*, deployment_id*, state, order_by, sort, page, per_page

See also: gitlab_pipeline / gitlab_job (CI runs deploying to environments), gitlab_ci_variable (env-scoped variables), gitlab_feature_flags (env-scoped strategies).`, routes, toolutil.IconEnvironment)
}

// registerCIVariableMeta registers the gitlab_ci_variable meta-tool with actions:
// list, get, create, update, delete.
func registerCIVariableMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":            routeAction(client, civariables.List),
		"get":             routeAction(client, civariables.Get),
		"create":          routeAction(client, civariables.Create),
		"update":          routeAction(client, civariables.Update),
		"delete":          destructiveVoidAction(client, civariables.Delete),
		"group_list":      routeAction(client, groupvariables.List),
		"group_get":       routeAction(client, groupvariables.Get),
		"group_create":    routeAction(client, groupvariables.Create),
		"group_update":    routeAction(client, groupvariables.Update),
		"group_delete":    destructiveVoidAction(client, groupvariables.Delete),
		"instance_list":   routeAction(client, instancevariables.List),
		"instance_get":    routeAction(client, instancevariables.Get),
		"instance_create": routeAction(client, instancevariables.Create),
		"instance_update": routeAction(client, instancevariables.Update),
		"instance_delete": destructiveVoidAction(client, instancevariables.Delete),
	}

	addMetaTool(server, "gitlab_ci_variable", `Manage GitLab CI/CD variables at instance, group, and project scope. Delete actions are irreversible.
When to use: define / rotate / unmask / scope CI/CD variables at project, group, or instance level, both regular and secret (masked / masked_and_hidden), with environment scoping for per-env values.
NOT for: linting CI YAML or browsing CI templates (use gitlab_template), pipeline runs or schedules (use gitlab_pipeline), feature flags (use gitlab_feature_flags), per-deployment env metadata (use gitlab_environment), GitLab instance settings (use gitlab_admin).

Returns:
- list / group_list / instance_list: arrays of variable objects {key, value (or hidden), variable_type, protected, masked, raw, environment_scope, description} with pagination.
- get / create / update / group_get / group_create / group_update / instance_get / instance_create / instance_update: single variable object.
- delete / group_delete / instance_delete: {success, message}.
Errors: 404 (hint: a (key, environment_scope) pair must exist for get/update/delete — supply environment_scope when the variable is env-scoped), 403 (hint: project requires Maintainer+, group requires Owner, instance requires admin), 400 (hint: variable_type ∈ env_var/file; masked requires single-line non-empty value matching GitLab's masking rules).

Param conventions: * = required. Project-scoped actions need project_id*, group-scoped need group_id*, instance-scoped need no ID. Common optional params: variable_type, protected, masked, raw, environment_scope.

Project variables:
- list: project_id*
- get / delete: project_id*, key*, environment_scope
- create: project_id*, key*, value*, description, variable_type, protected, masked, masked_and_hidden, raw, environment_scope
- update: project_id*, key*, value, description, variable_type, protected, masked, raw, environment_scope

Group variables (group_*):
- group_list: group_id*
- group_get / group_delete: group_id*, key*
- group_create: group_id*, key*, value*, description, variable_type, protected, masked, raw, environment_scope
- group_update: group_id*, key*, value, description, variable_type, protected, masked, raw, environment_scope

Instance variables (instance_*):
- instance_list: (no params)
- instance_get / instance_delete: key*
- instance_create: key*, value*, description, variable_type, protected, masked, raw
- instance_update: key*, value, description, variable_type, protected, masked, raw

See also: gitlab_pipeline (pipeline operations), gitlab_template (CI lint)`, routes, toolutil.IconVariable)
}

// registerTemplateMeta registers the gitlab_template meta-tool with actions:
// lint, lint_project, ci_yml_list, ci_yml_get, dockerfile_list, dockerfile_get,
// gitignore_list, gitignore_get, license_list, license_get, project_template_list, project_template_get.
func registerTemplateMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"lint":                  routeAction(client, cilint.LintContent),
		"lint_project":          routeAction(client, cilint.LintProject),
		"ci_yml_list":           routeAction(client, ciyamltemplates.List),
		"ci_yml_get":            routeAction(client, ciyamltemplates.Get),
		"dockerfile_list":       routeAction(client, dockerfiletemplates.List),
		"dockerfile_get":        routeAction(client, dockerfiletemplates.Get),
		"gitignore_list":        routeAction(client, gitignoretemplates.List),
		"gitignore_get":         routeAction(client, gitignoretemplates.Get),
		"license_list":          routeAction(client, licensetemplates.List),
		"license_get":           routeAction(client, licensetemplates.Get),
		"project_template_list": routeAction(client, projecttemplates.List),
		"project_template_get":  routeAction(client, projecttemplates.Get),
	}

	addReadOnlyMetaTool(server, "gitlab_template", `Browse GitLab built-in templates (gitignore, CI/CD YAML, Dockerfile, license, project scaffolding) and lint CI configuration. Read-only; ci_lint may resolve `+"`include:`"+` directives that fetch remote URLs.
When to use: discover available built-in templates, fetch a template body to commit into a project, validate a .gitlab-ci.yml before pushing, or list project scaffolds.
NOT for: reusable Catalog components published by groups (use gitlab_ci_catalog), running pipelines (use gitlab_pipeline), CI/CD variables (use gitlab_ci_variable), repository files (use gitlab_repository).

Returns:
- *_list: [{key, name}] with pagination (page, per_page, total, next_page).
- *_get: {name, content} — paste `+"`content`"+` into the target file.
- lint / lint_project: {valid (bool), errors: [string], warnings: [string], merged_yaml (string), jobs: [...] when include_jobs=true}.
Errors: 404 not found (hint: check key or template_type), 403 forbidden, 400 invalid params (hint: content required for lint, project_id required for project_template_*).

Param conventions: * = required. template_type ∈ {dockerfiles, gitignores, gitlab_ci_ymls, licenses}.

CI lint:
- lint: project_id*, content*, dry_run (bool), include_jobs (bool), ref
- lint_project: project_id*, content_ref, dry_run (bool), dry_run_ref, include_jobs (bool), ref

Global templates:
- ci_yml_list / dockerfile_list / gitignore_list: page, per_page
- ci_yml_get / dockerfile_get / gitignore_get: key*
- license_list: page, per_page, popular (bool)
- license_get: key*, project, fullname

Project templates:
- project_template_list: project_id*, template_type*, page, per_page
- project_template_get: project_id*, template_type*, key*

See also: gitlab_ci_catalog (reusable Catalog components), gitlab_pipeline (run pipelines), gitlab_project (project membership/settings).`, routes, toolutil.IconTemplate)
}

// registerPackageMeta registers the gitlab_package meta-tool with actions from
// packages (publish, download, list, file_list, delete, file_delete, publish_and_link,
// publish_directory), container registry (registry_list_project, registry_list_group,
// registry_get, registry_delete, registry_tag_list, registry_tag_get, registry_tag_delete,
// registry_tag_delete_bulk, registry_rule_list, registry_rule_create, registry_rule_update,
// registry_rule_delete), and package protection rules (protection_rule_list, protection_rule_create,
// protection_rule_update, protection_rule_delete).
func registerPackageMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"publish":                  routeActionWithRequest(client, packages.Publish),
		"download":                 routeActionWithRequest(client, packages.Download),
		"list":                     routeAction(client, packages.List),
		"file_list":                routeAction(client, packages.FileList),
		"delete":                   destructiveVoidActionWithRequest(client, packages.Delete),
		"file_delete":              destructiveVoidActionWithRequest(client, packages.FileDelete),
		"publish_and_link":         routeActionWithRequest(client, packages.PublishAndLink),
		"publish_directory":        routeActionWithRequest(client, packages.PublishDirectory),
		"registry_list_project":    routeAction(client, containerregistry.ListProject),
		"registry_list_group":      routeAction(client, containerregistry.ListGroup),
		"registry_get":             routeAction(client, containerregistry.GetRepository),
		"registry_delete":          destructiveVoidAction(client, containerregistry.DeleteRepository),
		"registry_tag_list":        routeAction(client, containerregistry.ListTags),
		"registry_tag_get":         routeAction(client, containerregistry.GetTag),
		"registry_tag_delete":      destructiveVoidAction(client, containerregistry.DeleteTag),
		"registry_tag_delete_bulk": destructiveVoidAction(client, containerregistry.DeleteTagsBulk),
		"registry_rule_list":       routeAction(client, containerregistry.ListProtectionRules),
		"registry_rule_create":     routeAction(client, containerregistry.CreateProtectionRule),
		"registry_rule_update":     routeAction(client, containerregistry.UpdateProtectionRule),
		"registry_rule_delete":     destructiveVoidAction(client, containerregistry.DeleteProtectionRule),
		"protection_rule_list":     routeAction(client, protectedpackages.List),
		"protection_rule_create":   routeAction(client, protectedpackages.Create),
		"protection_rule_update":   routeAction(client, protectedpackages.Update),
		"protection_rule_delete":   destructiveVoidAction(client, protectedpackages.Delete),
	}

	addMetaTool(server, "gitlab_package", `Manage GitLab package registry, container registry, and protection rules. Upload/download generic packages, list/delete packages, browse container images/tags, and configure access policies. Delete actions are destructive.
When to use: publish / download / list / delete generic packages, browse npm/maven/conan/nuget/pypi/etc. metadata, browse and prune container images and tags, manage container and package protection rules.
NOT for: release asset links — these are managed by gitlab_release link_*; secure files (use gitlab_admin secure_file_*); ML model registry artifacts (use gitlab_model_registry); upload general project attachments (use gitlab_project upload).

Behavior:
- Idempotent reads: list / file_list / registry_list_project / registry_list_group / registry_get / registry_tag_list / registry_tag_get / registry_rule_list / protection_rule_list / download.
- publish / publish_directory / publish_and_link create a NEW package version (NON-idempotent — re-publishing the same (package_name, package_version, file_name) returns 400/409 or creates a duplicate file depending on package_type). registry_rule_update / protection_rule_update are idempotent; *_create are non-idempotent on duplicate keys.
- Side effects: publish_and_link also creates a release link visible to release subscribers; download streams files to the required output_path on disk; protection_rule_create / registry_rule_create take effect immediately and may block subsequent publish/delete calls.
- Destructive: delete (entire package), file_delete (single file), registry_delete (entire image repo), registry_tag_delete / registry_tag_delete_bulk (image tags — name_regex_delete may match many tags) and *_rule_delete are irreversible. Protection rules can return 403 ('forbidden by protection rule') instead of executing the delete.

Returns:
- list / file_list / registry_list_project / registry_list_group / registry_tag_list / registry_rule_list / protection_rule_list: arrays with pagination.
- publish / publish_and_link / publish_directory / registry_get / registry_tag_get / registry_rule_create / registry_rule_update / protection_rule_create / protection_rule_update: package / image / rule object. publish_and_link also returns the created release link.
- download: {output_path, size, sha256} — files are streamed to the required output_path on disk.
- delete / file_delete / registry_delete / registry_tag_delete / registry_tag_delete_bulk / registry_rule_delete / protection_rule_delete: {success, message}.
Errors: 404 (hint: package_id, repository_id and tag_name are project-scoped), 403 (hint: requires Maintainer+ to delete; protection rules may block delete with a 'forbidden by protection rule' message), 400 (hint: file_path must exist locally; content_base64 must be valid base64; package_type must be one of GitLab's supported types).

Param conventions: * = required. Most actions need project_id*. List actions accept page, per_page.

Packages:
- publish: project_id*, package_name*, package_version*, file_name*, file_path or content_base64 (one required), status (default/hidden)
- download: project_id*, package_name*, package_version*, file_name*, output_path*
- list: project_id*, package_name, package_version, package_type (generic/npm/maven/etc.), order_by, sort
- file_list: project_id*, package_id*
- delete: project_id*, package_id*. Deletes package and all files.
- file_delete: project_id*, package_id*, package_file_id*
- publish_and_link: publish + create release link. project_id*, package_name*, package_version*, file_name*, file_path or content_base64 (one required), tag_name*, link_name, link_type
- publish_directory: project_id*, package_name*, package_version*, directory_path*, include_pattern (glob), status

Container registry:
- registry_list_project: project_id*, tags, tags_count
- registry_list_group: group_id*
- registry_get: repository_id*, tags, tags_count
- registry_delete: project_id*, repository_id*
- registry_tag_list / registry_tag_get / registry_tag_delete: project_id*, repository_id*, tag_name* (for get/delete)
- registry_tag_delete_bulk: project_id*, repository_id*, name_regex_delete, name_regex_keep, keep_n, older_than

Container registry protection rules:
- registry_rule_list: project_id*
- registry_rule_create: project_id*, repository_path_pattern*, minimum_access_level_for_push, minimum_access_level_for_delete
- registry_rule_update: project_id*, rule_id*, repository_path_pattern, minimum_access_level_for_push, minimum_access_level_for_delete
- registry_rule_delete: project_id*, rule_id*

Package protection rules:
- protection_rule_list: project_id*
- protection_rule_create: project_id*, package_name_pattern*, package_type*, minimum_access_level_for_push, minimum_access_level_for_delete
- protection_rule_update: project_id*, rule_id*, package_name_pattern, package_type, minimum_access_level_for_push, minimum_access_level_for_delete
- protection_rule_delete: project_id*, rule_id*

See also: gitlab_release (release asset links), gitlab_project`, routes, toolutil.IconPackage)
}

// registerFeatureFlagsMeta registers the gitlab_feature_flags meta-tool with actions:
// feature_flag_list, feature_flag_get, feature_flag_create, feature_flag_update, feature_flag_delete,
// ff_user_list_list, ff_user_list_get, ff_user_list_create, ff_user_list_update, and ff_user_list_delete.
func registerFeatureFlagsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"feature_flag_list":   routeAction(client, featureflags.ListFeatureFlags),
		"feature_flag_get":    routeAction(client, featureflags.GetFeatureFlag),
		"feature_flag_create": routeAction(client, featureflags.CreateFeatureFlag),
		"feature_flag_update": routeAction(client, featureflags.UpdateFeatureFlag),
		"feature_flag_delete": destructiveVoidAction(client, featureflags.DeleteFeatureFlag),
		"ff_user_list_list":   routeAction(client, ffuserlists.ListUserLists),
		"ff_user_list_get":    routeAction(client, ffuserlists.GetUserList),
		"ff_user_list_create": routeAction(client, ffuserlists.CreateUserList),
		"ff_user_list_update": routeAction(client, ffuserlists.UpdateUserList),
		"ff_user_list_delete": destructiveVoidAction(client, ffuserlists.DeleteUserList),
	}
	addMetaTool(server, "gitlab_feature_flags", `Manage project feature flags and feature-flag user lists for gradual rollouts. Delete is destructive; setting active=false disables the flag but preserves history.
When to use: define rollout strategies (percentage, user-targeted, environment-scoped) for a project's feature flags, and manage the user lists referenced by `+"`gitlabUserList`"+` strategies.
NOT for: GitLab instance-level feature flags (admin only — use gitlab_admin), environment definitions or protection (use gitlab_environment), code branching (use gitlab_branch), CI/CD variables (use gitlab_ci_variable).

Returns:
- *_list: array with pagination (page, per_page, total, next_page).
- *_get / *_create / *_update: the resource object (flag includes strategies and scopes; user list includes user_xids).
- *_delete: {success: bool, message: string}.
Errors: 404 not found, 403 forbidden (hint: requires Developer+ role), 400 invalid params (hint: strategies/scopes JSON shape).

Param conventions: * = required. All actions need project_id*. version = `+"`new_version_flag`"+` (legacy `+"`legacy_flag`"+` deprecated).

strategies shape: [{name, parameters, scopes: [{environment_scope}]}] where name ∈ {default, gradualRolloutUserId, userWithId, flexibleRollout, gitlabUserList}. parameters per strategy: gradualRolloutUserId={groupId, percentage}; userWithId={userIds}; flexibleRollout={groupId, rollout, stickiness}; gitlabUserList={userListId}.

Feature flags (feature_flag_*):
- feature_flag_list: project_id*, scope (enabled/disabled), page, per_page
- feature_flag_get / feature_flag_delete: project_id*, name*
- feature_flag_create: project_id*, name*, version*, description, active (bool), strategies
- feature_flag_update: project_id*, name*, description, active (bool), strategies

User lists (ff_user_list_*) — named sets of user IDs referenced by gitlabUserList strategies:
- ff_user_list_list: project_id*, page, per_page
- ff_user_list_get / ff_user_list_delete: project_id*, user_list_iid*
- ff_user_list_create: project_id*, name*, user_xids* (comma-separated user IDs)
- ff_user_list_update: project_id*, user_list_iid*, name, user_xids

See also: gitlab_environment (environment scopes referenced by strategies), gitlab_admin (instance-level feature flags), gitlab_project (project membership and settings).`, routes, toolutil.IconConfig)
}
