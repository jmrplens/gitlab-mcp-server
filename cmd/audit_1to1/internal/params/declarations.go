// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"fmt"
	"slices"
	"strings"
)

// Declaration categories. Each says why a param a route declares is not one
// this server sends, and so which findings the declaration may answer.
const (
	// categoryUpstream is a param client-go does not model, recorded in the
	// upstream register on the row the declaration names. It answers an
	// unmodeled finding alone, so the declaration goes stale the day
	// client-go models the param.
	categoryUpstream = "upstream"
	// categoryDeclaredNotApplied is a param GitLab declares on the route and
	// drops before it acts, or cannot apply, so offering it would offer
	// nothing. It answers a param no input offers alone.
	categoryDeclaredNotApplied = "declared-not-applied"
	// categoryMapInput is a route whose action takes its params as a free
	// map the caller fills, so every param GitLab declares is offered by
	// the map and none by name.
	categoryMapInput = "map-input"
	// categoryWebhookPayload is a param of a payload another service posts
	// to the route, which no caller of an action chooses. It answers a param
	// no input offers alone.
	categoryWebhookPayload = "webhook-payload"
	// categoryBranchNotTaken is a param of a route the client-go method
	// sends only for an argument the handler never passes, so the action
	// never sends that route, and what its input offers goes to the route
	// the method takes instead.
	categoryBranchNotTaken = "branch-not-taken"
)

// anySegment and anyParam are the wildcards a declaration's route and param
// are written with: one segment of the path, and every param of the route.
const (
	anySegment = "*"
	anyParam   = "*"
)

// paramDeclaration answers the findings on the params it names with a
// category and a reason.
type paramDeclaration struct {
	// Route is the route as the request record names it ("PUT
	// /projects/:id/integrations/slack"), a segment of "*" standing for any
	// one segment.
	Route string
	// Param is the param as the live record spells it, or "*" for every
	// param of the route.
	Param    string
	Category string
	Reason   string
	// Row is the upstream register row an upstream declaration waits on.
	Row int
}

// answeredClasses names the classes a category may answer, where it may not
// answer all of them. An upstream declaration answers an unmodeled param
// alone, so it goes stale when client-go models the param. A map input
// offers what the struct it is decoded into carries, or, sent as it is,
// whatever the caller writes, so it answers a param an option struct models
// and a param whose options are a free map, and never one neither can send.
// A param GitLab drops, or a payload another service posts, is one no input
// should offer, so those two answer a param no input offers and never an
// offered one: an input that starts offering a key GitLab drops is a finding
// again rather than an offer the declaration hides. A branch not taken
// answers any class, since what its route's input offers goes to the route
// the method takes instead.
var answeredClasses = map[string][]string{
	categoryUpstream:           {ClassUnmodeled},
	categoryMapInput:           {ClassNotOffered, ClassUnjudged},
	categoryDeclaredNotApplied: {ClassUnmodeled, ClassNotOffered, ClassUnjudged},
	categoryWebhookPayload:     {ClassUnmodeled, ClassNotOffered, ClassUnjudged},
}

// answers reports whether the declaration answers a finding: on its route,
// on its param, and of a class its category may answer.
func (d paramDeclaration) answers(finding Finding) bool {
	if classes, narrowed := answeredClasses[d.Category]; narrowed && !slices.Contains(classes, finding.Class) {
		return false
	}
	return (d.Param == anyParam || d.Param == finding.Param) && routeMatches(d.Route, finding.Route)
}

// routeMatches reports whether a declared route names a route: the same verb
// and, segment by segment, the same text or a wildcard.
func routeMatches(declared, route string) bool {
	declaredVerb, declaredPath, _ := strings.Cut(declared, " ")
	verb, path, _ := strings.Cut(route, " ")
	want, got := strings.Split(declaredPath, "/"), strings.Split(path, "/")
	if declaredVerb != verb || len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != anySegment && want[i] != got[i] {
			return false
		}
	}
	return true
}

// answer returns the index of the first declaration that answers a finding,
// and false when none does.
func answer(declared []paramDeclaration, finding Finding) (int, bool) {
	index := slices.IndexFunc(declared, func(d paramDeclaration) bool { return d.answers(finding) })
	return index, index >= 0
}

// knownCategories is every category a declaration may name.
var knownCategories = []string{
	categoryUpstream, categoryDeclaredNotApplied, categoryMapInput, categoryWebhookPayload, categoryBranchNotTaken,
}

// staleDeclarations names, sorted, every declaration that answers no
// finding, names no known category or no reason, or, for an upstream one,
// names a row the register does not hold.
func staleDeclarations(declared []paramDeclaration, used []bool, rows map[int]string) []string {
	stale := []string{}
	for i, d := range declared {
		if reason := staleReason(d, used[i], rows); reason != "" {
			stale = append(stale, fmt.Sprintf("%s %s (%s): %s", d.Route, d.Param, d.Category, reason))
		}
	}
	slices.Sort(stale)
	return stale
}

// staleReason says why one declaration fails the gate, and is empty for one
// that holds. Ifs rather than a tagless switch, so the mutation tool, which
// cannot see a case expression, measures each condition.
func staleReason(d paramDeclaration, used bool, rows map[int]string) string {
	if !slices.Contains(knownCategories, d.Category) {
		return "names no category this table defines"
	}
	if strings.TrimSpace(d.Reason) == "" {
		return "gives no reason"
	}
	if d.Category == categoryUpstream && rows[d.Row] == "" {
		return fmt.Sprintf("names row %d, which the upstream register does not hold", d.Row)
	}
	if !used {
		return "answers no finding"
	}
	return ""
}

// paramDeclarations is every finding this server answers rather than sends,
// in the order a finding is matched against them. A declaration that answers
// nothing fails the gate, so the table cannot outlive what it answers.
func paramDeclarations() []paramDeclaration {
	// Reasons shared by more than one declaration. They are local so the
	// coverage profile, which marks function bodies only, sees them.
	const (
		reasonSettingsMap = "admin.settings_update takes the settings as one map (settings.UpdateInput.Settings), " +
			"refuses a key client-go's UpdateSettingsOptions does not model, and decodes the rest into it, " +
			"so every param that struct models is offered by the map and none by name"
		reasonIntegrationMap = "the integration set actions send the input's config map verbatim as the body of " +
			"PUT .../integrations/<slug>, the slug named by the input (integrations.SetIntegration and SetGroupIntegration), " +
			"so every param of every integration is offered by the map and none by name"
		reasonDuoOnCreate = "ee/lib/ee/api/helpers/projects_helpers.rb filter_duo_vulnerability_attributes! deletes " +
			"duo_sast_fp_detection_enabled, duo_sast_vr_workflow_enabled, duo_secret_detection_fp_enabled and " +
			"duo_dependency_bump_breaking_changes_enabled unless params[:id] is present, so a create applies none of them"
		reasonDeploymentMRs = "lib/api/deployments.rb takes merge_requests_base_params on GET :id/deployments/:deployment_id/merge_requests " +
			"and presents Entities::MergeRequestBasic with current_user alone: view, with_labels_details and " +
			"with_merge_status_recheck become presenter options only through serializer_options_for in lib/api/merge_requests.rb, " +
			"which this route does not call, and MergeRequestsFinder reads none of them, so the route applies none"
		reasonProjectChecksums = "ee/lib/ee/api/helpers/projects_helpers.rb:38-42 at v19.5.0-ee declares repository_checksum_failed " +
			"and wiki_checksum_failed among the optional_filter_params_ee of every route that takes the project list filters, " +
			"and nothing reads them: the finder params the routes build carry neither (EE::API::Helpers#project_finder_params_ee, " +
			"ee/lib/ee/api/helpers.rb:119-127) and no finder, model or service names them, so a list applies neither"
		reasonImportNotRestored = "the import's tree restorer merges override_params into the archive's project attributes " +
			"and keeps only those lib/gitlab/import_export/project/import_export.yml permits for a project " +
			"(Gitlab::ImportExport::AttributesPermitter, through Group::RelationTreeRestorer#filter_attributes), " +
			"and this key is not among them, so the import drops it"
		reasonImportNotAssignable = "lib/gitlab/import_export/project/import_export.yml permits this key for a project, " +
			"and Group::RelationTreeRestorer#update_params! hands it to Project#assign_attributes, which cannot take it: " +
			"topics is the has_many :topics association with Projects::Topic (app/models/project.rb), which a list of names " +
			"does not fill, and the project has no jobs_enabled writer (lib/api/projects.rb renames it to builds_enabled for " +
			"the create and the edit alone), so either one raises, the restore fails and the import with it"
		reasonMirrorWebhook = "ee/lib/api/project_mirror.rb declares action and the pull_request.* keys for the GitHub " +
			"pull request event a webhook posts to the route (ProcessGithubEventService), which no caller of an action writes"
		reasonPlanLimits    = "ChangePlanLimitOptions models eight of the twenty-nine limits PUT /application/plan_limits takes"
		reasonRow133Groups  = "UpdateGroupOptions and CreateGroupOptions lack the group params GitLab 19.5 added"
		reasonRow133Duo     = "the project create, create-for-user and edit options lack duo_code_review_decisions_enabled"
		reasonRow133Account = "CreateServiceAccountUserOptions and UpdateServiceAccountOptions lack external"
		reasonRow133Search  = "SearchOptions lacks the merge request filters the group and project search routes take"
		reasonLabelRoutes   = "client-go's UpdateLabel, DeleteLabel, UpdateGroupLabel and DeleteGroupLabel send the " +
			"collection route, which selects the label by the label_id or name of the body or the query, only for a " +
			"nil label argument; labels.labelSelector always hands them label_id or the name, and grouplabels always " +
			"hands them label_id, so each action sends the route that names the label in its path alone"
	)
	declared := []paramDeclaration{
		{Route: "PUT /projects/:id/labels", Param: anyParam, Category: categoryBranchNotTaken, Reason: reasonLabelRoutes},
		{Route: "DELETE /projects/:id/labels", Param: anyParam, Category: categoryBranchNotTaken, Reason: reasonLabelRoutes},
		{Route: "PUT /groups/:id/labels", Param: anyParam, Category: categoryBranchNotTaken, Reason: reasonLabelRoutes},
		{Route: "DELETE /groups/:id/labels", Param: anyParam, Category: categoryBranchNotTaken, Reason: reasonLabelRoutes},
		{Route: "PUT /application/settings", Param: anyParam, Category: categoryMapInput, Reason: reasonSettingsMap},
		{Route: "PUT /projects/:id/integrations/*", Param: anyParam, Category: categoryMapInput, Reason: reasonIntegrationMap},
		{Route: "PUT /groups/:id/integrations/*", Param: anyParam, Category: categoryMapInput, Reason: reasonIntegrationMap},
	}
	for _, route := range []string{"POST /projects", "POST /projects/user/:user_id"} {
		for _, param := range []string{
			"duo_dependency_bump_breaking_changes_enabled", "duo_sast_fp_detection_enabled",
			"duo_sast_vr_workflow_enabled", "duo_secret_detection_fp_enabled",
		} {
			declared = append(declared, paramDeclaration{Route: route, Param: param, Category: categoryDeclaredNotApplied, Reason: reasonDuoOnCreate})
		}
	}
	for _, param := range []string{"view", "with_labels_details", "with_merge_status_recheck"} {
		declared = append(declared, paramDeclaration{
			Route: "GET /projects/:id/deployments/:deployment_id/merge_requests", Param: param,
			Category: categoryDeclaredNotApplied, Reason: reasonDeploymentMRs,
		})
	}
	for _, route := range []string{
		"GET /projects", "GET /projects/:id/forks", "GET /users/:user_id/projects", "GET /users/:user_id/starred_projects",
	} {
		for _, param := range []string{"repository_checksum_failed", "wiki_checksum_failed"} {
			declared = append(declared, paramDeclaration{Route: route, Param: param, Category: categoryDeclaredNotApplied, Reason: reasonProjectChecksums})
		}
	}
	for _, key := range []string{
		"auto_duo_code_review_enabled", "avatar", "ci_config_path", "cicd_catalog_enabled",
		"container_expiration_policy_attributes", "duo_code_review_decisions_enabled",
		"duo_dependency_bump_breaking_changes_enabled", "duo_remote_flows_enabled", "duo_sast_fp_detection_enabled",
		"duo_sast_vr_workflow_enabled", "duo_secret_detection_fp_enabled", "emails_disabled", "emails_enabled",
		"enforce_auth_checks_on_uploads", "group_runners_enabled", "issue_branch_template", "lfs_enabled",
		"merge_request_title_regex", "merge_request_title_regex_description", "mirror", "mirror_trigger_builds",
		"mr_default_target_self", "mr_default_title_template", "only_allow_merge_if_all_status_checks_passed",
		"packages_enabled", "prevent_merge_without_jira_issue", "public_jobs", "repository_storage",
		"resource_group_default_process_mode", "show_default_award_emojis", "show_diff_preview_in_email",
		"spp_repository_pipeline_access", "tag_list", "warn_about_potentially_unwanted_characters",
	} {
		declared = append(declared, paramDeclaration{
			Route: "POST /projects/import", Param: "override_params[" + key + "]",
			Category: categoryDeclaredNotApplied, Reason: reasonImportNotRestored,
		})
	}
	for _, key := range []string{"jobs_enabled", "topics"} {
		declared = append(declared, paramDeclaration{
			Route: "POST /projects/import", Param: "override_params[" + key + "]",
			Category: categoryDeclaredNotApplied, Reason: reasonImportNotAssignable,
		})
	}
	for _, param := range []string{
		"action", "pull_request.base.ref", "pull_request.base.repo.full_name", "pull_request.base.sha",
		"pull_request.head.ref", "pull_request.head.repo.full_name", "pull_request.head.sha", "pull_request.number",
	} {
		declared = append(declared, paramDeclaration{Route: "POST /projects/:id/mirror/pull", Param: param, Category: categoryWebhookPayload, Reason: reasonMirrorWebhook})
	}
	for _, param := range []string{
		"cargo_max_file_size", "ci_active_jobs", "ci_instance_level_variables", "ci_needs_size_limit",
		"ci_pipeline_schedules", "ci_pipeline_size", "ci_project_subscriptions", "ci_registered_group_runners",
		"ci_registered_project_runners", "dotenv_size", "dotenv_variables", "enforcement_limit",
		"max_pipelines_per_merge_train", "notification_limit", "pipeline_hierarchy_size",
		"service_desk_outbound_emails_per_day", "service_desk_outbound_emails_per_hour", "storage_size_limit",
		"web_hook_calls", "web_hook_calls_low", "web_hook_calls_mid",
	} {
		declared = append(declared, paramDeclaration{Route: "PUT /application/plan_limits", Param: param, Category: categoryUpstream, Reason: reasonPlanLimits, Row: 59})
	}
	upstream133 := func(route, reason string, params ...string) {
		for _, param := range params {
			declared = append(declared, paramDeclaration{Route: route, Param: param, Category: categoryUpstream, Reason: reason, Row: 133})
		}
	}
	upstream133("PUT /groups/:id", reasonRow133Groups, "duo_auto_mode_availability", "duo_code_review_decisions_enabled", "lock_require_sha_for_merge", "require_sha_for_merge")
	upstream133("POST /groups", reasonRow133Groups, "duo_auto_mode_availability")
	for _, route := range []string{"POST /projects", "POST /projects/user/:user_id", "PUT /projects/:id"} {
		upstream133(route, reasonRow133Duo, "duo_code_review_decisions_enabled")
	}
	upstream133("POST /service_accounts", reasonRow133Account, "external")
	upstream133("PATCH /service_accounts/:user_id", reasonRow133Account, "external")
	for _, route := range []string{"GET /groups/:id/(-/)search", "GET /projects/:id/(-/)search"} {
		// The negations stand as the one container they sit under.
		upstream133(route, reasonRow133Search, "author_username", "label_name", "not", "source_branch", "target_branch")
	}
	return declared
}
