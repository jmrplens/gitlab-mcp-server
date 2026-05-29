package evaluator

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func filterTasks(tasks []evalTask, onlyIDs string) []evalTask {
	if strings.TrimSpace(onlyIDs) == "" {
		return tasks
	}
	selected := make(map[string]struct{})
	for id := range strings.SplitSeq(onlyIDs, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	filtered := make([]evalTask, 0, len(selected))
	for _, task := range tasks {
		if _, ok := selected[task.ID]; ok {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

// filterTasksByEdition filters tasks by GitLab edition coverage.
func filterTasksByEdition(tasks []evalTask, edition string) ([]evalTask, error) {
	edition = strings.ToLower(strings.TrimSpace(edition))
	if edition == "" || edition == editionAll {
		return tasks, nil
	}
	if edition != editionCE && edition != editionEnterprise {
		return nil, fmt.Errorf("unknown --edition %q", edition)
	}
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		enterprise := taskHasEnterpriseStep(task)
		if edition == editionCE && enterprise {
			continue
		}
		if edition == editionEnterprise && !enterprise {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

// filterTasksByDestructive handles filter tasks by destructive and returns [[]evalTask].
func filterTasksByDestructive(tasks []evalTask, skipDestructive, onlyDestructive bool) ([]evalTask, error) {
	if skipDestructive && onlyDestructive {
		return nil, errors.New("--skip-destructive and --only-destructive cannot be used together")
	}
	if !skipDestructive && !onlyDestructive {
		return tasks, nil
	}
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		destructive := taskHasDestructiveStep(task)
		if skipDestructive && destructive {
			continue
		}
		if onlyDestructive && !destructive {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

// taskHasDestructiveStep reports whether task has destructive step.
func taskHasDestructiveStep(task evalTask) bool {
	if task.Case != nil {
		return task.Case.Destructive
	}
	if task.Destructive {
		return true
	}
	for _, step := range taskSteps(task) {
		if step.Destructive || routeLooksDestructive(step.ExpectedAction) {
			return true
		}
	}
	return false
}

// routeLooksDestructive reports whether route looks destructive.
func routeLooksDestructive(action string) bool {
	action = strings.TrimPrefix(action, "gitlab_")
	for _, token := range strings.FieldsFunc(action, func(r rune) bool { return r == '.' || r == '_' || r == '-' }) {
		switch token {
		case "archive", "delete", "destroy", "purge", "remove", "revoke", "terminate":
			return true
		}
	}
	return strings.Contains(action, "publish_all")
}

// filterTasksByMutation handles filter tasks by mutation and returns [[]evalTask].
func filterTasksByMutation(tasks []evalTask, skipMutating, onlyMutating bool) ([]evalTask, error) {
	if skipMutating && onlyMutating {
		return nil, errors.New("--skip-mutating and --only-mutating cannot be used together")
	}
	if !skipMutating && !onlyMutating {
		return tasks, nil
	}
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		mutating := taskHasMutatingStep(task)
		if skipMutating && mutating {
			continue
		}
		if onlyMutating && !mutating {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

// filterTasksByAvailableRoutes filters tasks by available routes using evaluator options.
func filterTasksByAvailableRoutes(tasks []evalTask, routes map[string]toolutil.ActionMap, enterprise bool) []evalTask {
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		if taskRoutesAvailable(task, routes, enterprise) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func catalogHasEnterpriseRoutes(routes map[string]toolutil.ActionMap) bool {
	for tool, actions := range routes {
		for action := range actions {
			if routeSignalsEnterpriseCatalog(tool, action) {
				return true
			}
		}
	}
	return false
}

func routeSignalsEnterpriseCatalog(tool, action string) bool {
	route := canonicalRouteID(tool, action)
	for _, prefix := range []string{
		"attestation.", "audit_event.", "compliance_policy.", "dependency.", "dora_metrics.", "enterprise_user.", "external_status_check.", "geo.", "member_role.", "merge_train.", "project_alias.", "security_finding.", "security_setting.", "storage_move.", "vulnerability.",
		"project.service_account_",
	} {
		if strings.HasPrefix(route, prefix) {
			return true
		}
	}
	return false
}

// taskRoutesAvailable reports whether task routes available.
func taskRoutesAvailable(task evalTask, routes map[string]toolutil.ActionMap, enterprise bool) bool {
	if taskUnavailableInLiveEvaluator(task) {
		return false
	}
	for _, step := range taskSteps(task) {
		if step.ExpectedAction == "" {
			if !standaloneToolAvailableInLiveEvaluator(step.ExpectedTool) {
				return false
			}
			continue
		}
		if !catalogHasRoute(routes, step.ExpectedTool, step.ExpectedAction) {
			return false
		}
		if !enterprise && routeUnavailableOnCE(step.ExpectedTool, step.ExpectedAction) {
			return false
		}
	}
	return true
}

// standaloneToolAvailableInLiveEvaluator reports whether standalone tool available in live evaluator.
func standaloneToolAvailableInLiveEvaluator(tool string) bool {
	switch tool {
	case dynamicFindTool,
		"gitlab_discover_project",
		"gitlab_interactive_issue_create",
		"gitlab_interactive_mr_create",
		"gitlab_interactive_project_create",
		"gitlab_interactive_release_create",
		capabilityListTool,
		resourceListTool,
		resourceReadTool,
		promptListTool,
		promptGetTool,
		completionTool:
		return true
	default:
		return false
	}
}

// filterTasksByPartition handles filter tasks by partition and returns [[]evalTask].
func filterTasksByPartition(tasks []evalTask, partition string) ([]evalTask, error) {
	partition = strings.TrimSpace(partition)
	if partition == "" {
		return tasks, nil
	}
	if !validPartition(partition) {
		return nil, fmt.Errorf("unknown --partition %q", partition)
	}
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		if taskMatchesPartition(task, partition) {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

// validPartition reports whether valid partition.
func validPartition(partition string) bool {
	switch partition {
	case partitionBaseRead, partitionBaseMutating, partitionBaseDestructive, partitionEnterpriseRead, partitionEnterpriseMutating, partitionEnterpriseDestructive, partitionErrorRecovery, partitionCapabilityFallback:
		return true
	default:
		return false
	}
}

// taskMatchesPartition reports whether task matches partition.
func taskMatchesPartition(task evalTask, partition string) bool {
	if task.Case != nil && task.Case.Partition != "" {
		return string(task.Case.Partition) == partition
	}
	return string(inferTaskPartition(task)) == partition
}

func inferTaskPartition(task evalTask) EvalPartition {
	if taskUsesCapabilityFallback(task) {
		return EvalPartition(partitionCapabilityFallback)
	}
	if strings.HasPrefix(task.ID, "MF-") || taskHasSimulation(task) {
		return EvalPartition(partitionErrorRecovery)
	}
	enterprise := taskHasEnterpriseStep(task)
	destructive := taskHasDestructiveStep(task)
	mutating := taskHasMutatingStep(task)
	switch {
	case enterprise && destructive:
		return EvalPartition(partitionEnterpriseDestructive)
	case enterprise && mutating:
		return EvalPartition(partitionEnterpriseMutating)
	case enterprise:
		return EvalPartition(partitionEnterpriseRead)
	case destructive:
		return EvalPartition(partitionBaseDestructive)
	case mutating:
		return EvalPartition(partitionBaseMutating)
	default:
		return EvalPartition(partitionBaseRead)
	}
}

// filterTasksByPreset handles filter tasks by preset and returns [[]evalTask].
func filterTasksByPreset(tasks []evalTask, preset string) ([]evalTask, error) {
	if !validPreset(preset) {
		return nil, fmt.Errorf("unknown --preset %q", preset)
	}
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		if taskMatchesPreset(task, preset) {
			filtered = append(filtered, task)
		}
	}
	return orderTasksForPreset(filtered, preset), nil
}

// orderTasksForPreset orders tasks for preset deterministically.
func orderTasksForPreset(tasks []evalTask, preset string) []evalTask {
	if preset != presetDockerDestructiveSafe && preset != presetDockerEnterpriseDestructiveSafe {
		return tasks
	}
	return orderSharedFixtureDestructiveLast(tasks)
}

// orderSharedFixtureDestructiveLast moves destructive operations on shared
// Docker fixture resources after tasks that still need those resources intact.
func orderSharedFixtureDestructiveLast(tasks []evalTask) []evalTask {
	regular := make([]evalTask, 0, len(tasks))
	artifactDeletes := make([]evalTask, 0, 1)
	projectServiceAccountDeletes := make([]evalTask, 0, 1)
	projectArchive := make([]evalTask, 0, 1)
	for _, task := range tasks {
		if taskArchivesSharedProject(task) {
			projectArchive = append(projectArchive, task)
			continue
		}
		if taskDeletesProjectServiceAccount(task) {
			projectServiceAccountDeletes = append(projectServiceAccountDeletes, task)
			continue
		}
		if taskDeletesSharedJobArtifacts(task) {
			artifactDeletes = append(artifactDeletes, task)
			continue
		}
		regular = append(regular, task)
	}
	regular = append(regular, projectServiceAccountDeletes...)
	regular = append(regular, artifactDeletes...)
	return append(regular, projectArchive...)
}

// taskArchivesSharedProject reports whether task archives shared project.
func taskArchivesSharedProject(task evalTask) bool {
	for _, step := range taskSteps(task) {
		if step.ExpectedTool == "gitlab_project" && step.ExpectedAction == "archive" {
			return true
		}
		if step.ExpectedTool == dynamicExecuteActionTool && step.ExpectedAction == "project.archive" {
			return true
		}
	}
	return false
}

// taskDeletesSharedJobArtifacts reports whether a task removes artifacts from
// the shared failed-job fixture used by artifact download/read scenarios.
func taskDeletesSharedJobArtifacts(task evalTask) bool {
	if taskHasAttemptScopedFixture(task, "failed_job_artifact") {
		return false
	}
	for _, step := range taskSteps(task) {
		if step.ExpectedTool == "gitlab_job" && step.ExpectedAction == "delete_artifacts" {
			return true
		}
		if step.ExpectedTool == dynamicExecuteActionTool && step.ExpectedAction == "job.delete_artifacts" {
			return true
		}
	}
	return false
}

// taskDeletesProjectServiceAccount reports whether a task removes the shared
// project service-account fixture used by Enterprise PAT scenarios.
func taskDeletesProjectServiceAccount(task evalTask) bool {
	if taskCreatesProjectServiceAccount(task) {
		return false
	}
	for _, step := range taskSteps(task) {
		if step.ExpectedTool == "gitlab_project" && step.ExpectedAction == "service_account_delete" {
			return true
		}
		if step.ExpectedTool == dynamicExecuteActionTool && step.ExpectedAction == "project.service_account_delete" {
			return true
		}
		if step.ExpectedTool == "gitlab" && step.ExpectedAction == "project.service_account_delete" {
			return true
		}
	}
	return false
}

func taskHasAttemptScopedFixture(task evalTask, fixtureName string) bool {
	if task.Case == nil {
		return false
	}
	for _, fixture := range task.Case.Fixtures {
		if fixture.Name == fixtureName && fixture.Scope == FixtureScopeAttempt {
			return true
		}
	}
	return false
}

func taskCreatesProjectServiceAccount(task evalTask) bool {
	for _, step := range taskSteps(task) {
		if step.ExpectedTool == "gitlab_project" && step.ExpectedAction == "service_account_create" {
			return true
		}
		if step.ExpectedTool == dynamicExecuteActionTool && step.ExpectedAction == "project.service_account_create" {
			return true
		}
		if step.ExpectedTool == "gitlab" && step.ExpectedAction == "project.service_account_create" {
			return true
		}
	}
	return false
}

// taskMatchesPreset reports whether task matches preset.
func taskMatchesPreset(task evalTask, preset string) bool {
	if task.Case != nil && len(task.Case.Presets) > 0 {
		return evalCaseMatchesPreset(*task.Case, preset)
	}
	capabilityFallback := taskUsesCapabilityFallback(task)
	traits := taskPresetTraits{
		Enterprise:              taskHasEnterpriseStep(task),
		EnterpriseDockerFixture: taskIsEnterpriseDockerFixture(task),
		Destructive:             taskHasDestructiveStep(task),
		Mutating:                taskHasMutatingStep(task),
		Special:                 strings.HasPrefix(task.ID, "MF-") || taskHasSimulation(task) || capabilityFallback,
		CapabilityFallback:      capabilityFallback,
	}
	return taskPresetMatchesTraits(traits, preset)
}

type taskPresetTraits struct {
	Enterprise              bool
	EnterpriseDockerFixture bool
	Destructive             bool
	Mutating                bool
	Special                 bool
	CapabilityFallback      bool
}

type taskPresetPredicate func(taskPresetTraits) bool

var taskPresetPredicates = map[string]taskPresetPredicate{
	presetSchemaEnterprise: func(traits taskPresetTraits) bool {
		return traits.Enterprise
	},
	presetDockerRead: func(traits taskPresetTraits) bool {
		return !traits.Enterprise && !traits.Mutating && !traits.Destructive && !traits.Special
	},
	presetDockerMutatingSafe: func(traits taskPresetTraits) bool {
		return !traits.Enterprise && traits.Mutating && !traits.Destructive && !traits.Special
	},
	presetDockerDestructiveSafe: func(traits taskPresetTraits) bool {
		return !traits.Enterprise && traits.Destructive && !traits.Special
	},
	presetDockerEnterpriseRead: func(traits taskPresetTraits) bool {
		return traits.Enterprise && traits.EnterpriseDockerFixture && !traits.Mutating && !traits.Destructive && !traits.Special
	},
	presetDockerEnterpriseMutatingSafe: func(traits taskPresetTraits) bool {
		return traits.Enterprise && traits.EnterpriseDockerFixture && traits.Mutating && !traits.Destructive && !traits.Special
	},
	presetDockerEnterpriseDestructiveSafe: func(traits taskPresetTraits) bool {
		return traits.Enterprise && traits.EnterpriseDockerFixture && traits.Destructive && !traits.Special
	},
	presetDockerCapabilityDiscovery: func(traits taskPresetTraits) bool {
		return traits.CapabilityFallback
	},
}

func taskPresetMatchesTraits(traits taskPresetTraits, preset string) bool {
	predicate, ok := taskPresetPredicates[preset]
	return ok && predicate(traits)
}

func taskIsEnterpriseDockerFixture(task evalTask) bool {
	if task.Case != nil {
		return evalCaseMatchesPreset(*task.Case, presetDockerEnterpriseRead) ||
			evalCaseMatchesPreset(*task.Case, presetDockerEnterpriseMutatingSafe) ||
			evalCaseMatchesPreset(*task.Case, presetDockerEnterpriseDestructiveSafe)
	}
	return false
}

// taskHasEnterpriseStep reports whether task has enterprise step.
func taskHasEnterpriseStep(task evalTask) bool {
	if task.Case != nil && task.Case.Edition != "" {
		return task.Case.Edition == EvalCaseEdition(editionEnterprise)
	}
	for _, step := range taskSteps(task) {
		if routeLooksEnterprise(step.ExpectedTool, step.ExpectedAction) {
			return true
		}
	}
	return false
}

// routeLooksEnterprise reports whether route looks enterprise.
func routeLooksEnterprise(tool, action string) bool {
	if routeUnavailableOnCE(tool, action) {
		return true
	}
	domain := canonicalRouteID(tool, action)
	if domain == "" {
		domain = strings.TrimPrefix(tool, "gitlab_")
	}
	for _, prefix := range []string{
		"attestation.", "audit_event.", "compliance_policy.", "dependency.", "dora_metrics.", "enterprise_user.", "external_status_check.", "geo.", "group_analytics.", "group_credential.", "group_epic_board.", "group_iteration.", "group_ldap.", "group_protected_branch.", "group_protected_env.", "group_release.", "group_saml.", "group_scim.", "group_service_account.", "group_ssh_cert.", "group_wiki.", "member_role.", "merge_train.", "project_alias.", "project_iteration.", "security_finding.", "security_setting.", "storage_move.", "vulnerability.",
		"epic.", "epic_discussion.", "epic_issue.", "epic_note.",
		"environment.protected_",
		"project.mirror_", "project.push_rule_", "project.security_settings_", "project.service_account_",
		"group.analytics_", "group.credential_", "group.epic_", "group.iteration_", "group.ldap_", "group.protected_branch_", "group.protected_env_", "group.release_", "group.saml_", "group.security_settings_", "group.service_account_", "group.ssh_cert_", "group.wiki_",
		"issue.iteration_",
		"user.create_service_account", "user.list_service_accounts",
	} {
		if strings.HasPrefix(domain, prefix) {
			return true
		}
	}
	return false
}

// taskHasSimulation reports whether task has simulation.
func taskHasSimulation(task evalTask) bool {
	for _, step := range taskSteps(task) {
		if step.Simulation != "" {
			return true
		}
	}
	return false
}

// taskUsesCapabilityFallback reports whether task uses capability fallback.
func taskUsesCapabilityFallback(task evalTask) bool {
	hasExpectedRoute := false
	for _, step := range taskSteps(task) {
		if isCapabilityBridgeName(step.ExpectedTool) {
			return true
		}
		if strings.Contains(step.ExpectedAction, "schema") {
			return true
		}
		if step.ExpectedTool != "" || step.ExpectedAction != "" {
			hasExpectedRoute = true
		}
	}
	if hasExpectedRoute {
		return false
	}
	prompt := strings.ToLower(task.Prompt)
	return strings.Contains(prompt, "schema") || strings.Contains(prompt, "capability") || strings.Contains(prompt, "fallback")
}

// catalogHasRoute reports whether catalog has route.
func catalogHasRoute(routes map[string]toolutil.ActionMap, tool, action string) bool {
	toolRoutes, ok := routes[tool]
	if !ok {
		return false
	}
	_, ok = toolRoutes[action]
	return ok
}

// canonicalRouteID returns the meta-tool route ID represented by a tool/action pair.
func canonicalRouteID(tool, action string) string {
	if tool != "gitlab" && tool != dynamicExecuteActionTool && action != "" {
		return strings.TrimPrefix(tool, "gitlab_") + "." + action
	}
	return action
}

// routeUnavailableOnCE reports whether route unavailable on ce.
func routeUnavailableOnCE(tool, action string) bool {
	route := canonicalRouteID(tool, action)
	switch route {
	case "environment.deployment_approve_or_reject", "model_registry.download":
		return true
	default:
		return false
	}
}

// taskUnavailableInLiveEvaluator reports whether task unavailable in live evaluator.
func taskUnavailableInLiveEvaluator(task evalTask) bool {
	if task.Case != nil {
		return len(task.Case.SkipReasons) > 0
	}
	switch task.ID {
	case "MT-105", "MT-115":
		return true
	default:
		return false
	}
}

// taskHasMutatingStep reports whether task has mutating step.
func taskHasMutatingStep(task evalTask) bool {
	if task.Case != nil {
		return task.Case.Mutating
	}
	for _, step := range taskSteps(task) {
		if step.Destructive || routeLooksMutating(step.ExpectedTool, step.ExpectedAction) {
			return true
		}
	}
	return false
}

// routeLooksMutating reports whether route looks mutating.
func routeLooksMutating(tool, action string) bool {
	if action == "" {
		return strings.HasPrefix(tool, "gitlab_interactive_")
	}
	action = strings.TrimPrefix(action, "gitlab_")
	if dot := strings.LastIndex(action, "."); dot >= 0 {
		action = action[dot+1:]
	}
	for _, token := range strings.FieldsFunc(action, func(r rune) bool { return r == '.' || r == '_' || r == '-' }) {
		switch token {
		case "add", "approve", "archive", "assign", "bulk", "cancel", "clear", "close", "create", "delete", "disable", "edit", "enable", "fork", "keep", "lock", "merge", "move", "play", "protect", "publish", "reject", "remove", "reopen", "resolve", "retry", "revoke", "rotate", "run", "set", "star", "stop", "subscribe", "transfer", "trigger", "unarchive", "unassign", "unlock", "unprotect", "unsubscribe", "update", "upload":
			return true
		}
	}
	return false
}

// validateTaskFixture validates task fixture for the evaluator package.
func validateTaskFixture(tasks []evalTask) []string {
	var problems []string
	for _, task := range tasks {
		steps := taskSteps(task)
		for stepIndex, step := range steps {
			stepLabel := task.ID
			if len(steps) > 1 {
				stepLabel = fmt.Sprintf("%s step %d", task.ID, stepIndex+1)
			}
			if hasParam(step.RequiredParams, "project_id") && !promptNamesEntity(task.Prompt, "project") {
				problems = append(problems, stepLabel+" requires project_id but prompt does not name a project")
			}
			if hasParam(step.RequiredParams, "group_id") && !promptNamesEntity(task.Prompt, "group") {
				problems = append(problems, stepLabel+" requires group_id but prompt does not name a group")
			}
			if step.Destructive && !hasParam(step.OptionalParams, "confirm") && !hasParam(step.RequiredParams, "confirm") {
				problems = append(problems, stepLabel+" is destructive but does not list confirm as a parameter")
			}
		}
	}
	return problems
}

// validateTaskFixtureAgainstRoutes validates task fixture against routes for the evaluator package.
func validateTaskFixtureAgainstRoutes(tasks []evalTask, routes map[string]toolutil.ActionMap) []string {
	var problems []string
	for _, task := range tasks {
		steps := taskSteps(task)
		for stepIndex, step := range steps {
			stepLabel := task.ID
			if len(steps) > 1 {
				stepLabel = fmt.Sprintf("%s step %d", task.ID, stepIndex+1)
			}
			if step.ExpectedAction == "" {
				continue
			}
			route, ok := routes[step.ExpectedTool][step.ExpectedAction]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s expected route %s/%s is not registered", stepLabel, step.ExpectedTool, step.ExpectedAction))
				continue
			}
			if step.Destructive != route.Destructive {
				problems = append(problems, fmt.Sprintf("%s destructive flag = %t, route metadata = %t", stepLabel, step.Destructive, route.Destructive))
			}
			for _, param := range append(slices.Clone(step.RequiredParams), step.OptionalParams...) {
				if !schemaAllowsParam(route.InputSchema, param) {
					problems = append(problems, fmt.Sprintf("%s lists param %q but %s/%s schema does not expose it", stepLabel, param, step.ExpectedTool, step.ExpectedAction))
				}
			}
		}
	}
	return problems
}

// normalizeTasksForCatalog normalizes fixture expectations for the selected
// model-facing tool catalog.
func normalizeTasksForCatalog(tasks []evalTask, routes map[string]toolutil.ActionMap, toolSurface string) []evalTask {
	if isDynamicEvalSurface(toolSurface) {
		return normalizeTasksForDynamicRoutes(tasks, routes)
	}
	return normalizeTasksForRoutes(tasks, routes)
}

// normalizeTasksForDynamicRoutes rewrites action-based expectations to the
// gitlab_execute_action envelope used by dynamic mode.
func normalizeTasksForDynamicRoutes(tasks []evalTask, routes map[string]toolutil.ActionMap) []evalTask {
	out := make([]evalTask, len(tasks))
	copy(out, tasks)
	for i := range out {
		steps := taskSteps(out[i])
		normalized := make([]evalStep, 0, len(steps))
		for _, step := range steps {
			step.ExpectedTool, step.ExpectedAction = normalizeExpectedDynamicRoute(step.ExpectedTool, step.ExpectedAction, routes)
			normalized = append(normalized, step)
		}
		out[i].Steps = expandDynamicFindFirstSteps(normalized)
		if len(out[i].Steps) == 0 {
			out[i].ExpectedTool, out[i].ExpectedAction = "", ""
			continue
		}
		first := out[i].Steps[0]
		out[i].ExpectedTool = first.ExpectedTool
		out[i].ExpectedAction = first.ExpectedAction
		out[i].RequiredParams = slices.Clone(first.RequiredParams)
		out[i].OptionalParams = slices.Clone(first.OptionalParams)
		out[i].Destructive = first.Destructive
		out[i].Simulation = first.Simulation
	}
	return out
}

func expandDynamicFindFirstSteps(steps []evalStep) []evalStep {
	expanded := make([]evalStep, 0, len(steps)*2)
	for _, step := range steps {
		if dynamicExecuteStep(step) {
			expanded = append(expanded, dynamicFindStep())
		}
		expanded = append(expanded, step)
	}
	return expanded
}

func dynamicExecuteStep(step evalStep) bool {
	return step.ExpectedTool == dynamicExecuteActionTool && step.ExpectedAction != ""
}

func dynamicFindStep() evalStep {
	return evalStep{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}}
}

// normalizeExpectedDynamicRoute maps a fixture's catalog route expectation to
// gitlab_execute_action when that route exists in the dynamic catalog.
func normalizeExpectedDynamicRoute(tool, action string, routes map[string]toolutil.ActionMap) (normalizedTool, normalizedAction string) {
	if action == "" {
		executeRoutes := routes[dynamicExecuteActionTool]
		for _, candidate := range standaloneDynamicActionCandidates(tool) {
			if _, ok := executeRoutes[candidate]; ok {
				return dynamicExecuteActionTool, candidate
			}
		}
		return tool, action
	}
	executeRoutes := routes[dynamicExecuteActionTool]
	for _, candidate := range dynamicActionCandidates(tool, action) {
		if _, ok := executeRoutes[candidate]; ok {
			return dynamicExecuteActionTool, candidate
		}
	}
	return tool, action
}

// standaloneDynamicActionCandidates returns dynamic fallback action candidates for standalone tools.
func standaloneDynamicActionCandidates(tool string) []string {
	switch tool {
	case "gitlab_discover_project":
		return []string{actionDiscoverProjectResolve}
	case "gitlab_interactive_issue_create":
		return []string{"interactive.issue_create"}
	case "gitlab_interactive_mr_create":
		return []string{"interactive.mr_create"}
	case "gitlab_interactive_project_create":
		return []string{"interactive.project_create"}
	case "gitlab_interactive_release_create":
		return []string{"interactive.release_create"}
	default:
		return nil
	}
}

// dynamicActionCandidates returns likely dynamic action IDs for a fixture route.
func dynamicActionCandidates(tool, action string) []string {
	candidates := []string{action}
	if tool != "" && tool != "gitlab" && strings.HasPrefix(tool, "gitlab_") {
		candidates = append(candidates, dynamicActionID(tool, action))
	}
	return candidates
}

// normalizeTasksForRoutes normalizes tasks for routes for stable comparisons.
func normalizeTasksForRoutes(tasks []evalTask, routes map[string]toolutil.ActionMap) []evalTask {
	out := make([]evalTask, len(tasks))
	copy(out, tasks)
	for i := range out {
		out[i].ExpectedTool, out[i].ExpectedAction = normalizeExpectedRoute(out[i].ExpectedTool, out[i].ExpectedAction, routes)
		if len(out[i].Steps) == 0 {
			continue
		}
		out[i].Steps = slices.Clone(out[i].Steps)
		for j := range out[i].Steps {
			out[i].Steps[j].ExpectedTool, out[i].Steps[j].ExpectedAction = normalizeExpectedRoute(out[i].Steps[j].ExpectedTool, out[i].Steps[j].ExpectedAction, routes)
		}
	}
	return out
}

// normalizeExpectedRoute normalizes expected route for stable comparisons.
func normalizeExpectedRoute(tool, action string, routes map[string]toolutil.ActionMap) (normalizedTool, normalizedAction string) {
	if action == "" || tool == "gitlab_server" || !strings.HasPrefix(tool, "gitlab") {
		return tool, action
	}
	if tool == "gitlab" {
		if _, ok := routes["gitlab"][action]; ok {
			return tool, action
		}
		if standaloneTool, ok := standaloneMetaToolForAction(action); ok {
			return standaloneTool, ""
		}
		if metaTool, metaAction, ok := metaToolRouteForAction(action, routes); ok {
			return metaTool, metaAction
		}
		return tool, action
	}
	superAction := superDispatcherAction(tool, action)
	if _, ok := routes["gitlab"][superAction]; ok {
		return "gitlab", superAction
	}
	return tool, action
}

// standaloneMetaToolForAction handles standalone meta tool for action and returns [string].
func standaloneMetaToolForAction(action string) (string, bool) {
	switch action {
	case actionDiscoverProjectResolve:
		return "gitlab_discover_project", true
	case "interactive.issue_create":
		return "gitlab_interactive_issue_create", true
	case "interactive.mr_create":
		return "gitlab_interactive_mr_create", true
	case "interactive.project_create":
		return "gitlab_interactive_project_create", true
	case "interactive.release_create":
		return "gitlab_interactive_release_create", true
	default:
		return "", false
	}
}

// metaToolRouteForAction handles meta tool route for action and returns [string].
func metaToolRouteForAction(action string, routes map[string]toolutil.ActionMap) (toolName, actionName string, ok bool) {
	domain, routeAction, found := strings.Cut(action, ".")
	if !found || domain == "" || routeAction == "" {
		return "", "", false
	}
	toolName = "gitlab_" + domain
	if _, exists := routes[toolName][routeAction]; exists {
		return toolName, routeAction, true
	}
	return "", "", false
}

// superDispatcherAction returns the meta-tool dispatcher action for a task step.
func superDispatcherAction(tool, action string) string {
	return strings.TrimPrefix(tool, "gitlab_") + "." + action
}

// taskSteps returns expected tool steps for an evaluation task.
func taskSteps(task evalTask) []evalStep {
	if len(task.Steps) > 0 {
		return task.Steps
	}
	return []evalStep{{
		ExpectedTool:   task.ExpectedTool,
		ExpectedAction: task.ExpectedAction,
		RequiredParams: task.RequiredParams,
		OptionalParams: task.OptionalParams,
		Destructive:    task.Destructive,
		Simulation:     task.Simulation,
	}}
}

// hasParam reports whether has param.
func hasParam(params []string, needle string) bool {
	return slices.Contains(params, needle)
}

// promptNamesEntity reports whether a prompt names the target entity.
func promptNamesEntity(prompt, entity string) bool {
	lowerPrompt := strings.ToLower(prompt)
	lowerEntity := strings.ToLower(entity)
	return strings.Contains(lowerPrompt, lowerEntity+" `") ||
		strings.Contains(lowerPrompt, lowerEntity+" id `") ||
		strings.Contains(lowerPrompt, lowerEntity+" id ") ||
		strings.Contains(lowerPrompt, lowerEntity+" path `")
}

// splitMarkdownRow splits markdown row into parsed fields.
func splitMarkdownRow(line string) []string {
	parts := make([]string, 0)
	var current strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			if r != '|' {
				current.WriteRune('\\')
			}
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '|' {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	parts = append(parts, strings.TrimSpace(current.String()))
	if len(parts) > 0 && parts[0] == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

// newMockGitLabClient constructs mock GitLab client.
