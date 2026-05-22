package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
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
	} {
		if strings.HasPrefix(route, prefix) {
			return true
		}
	}
	return false
}

// taskRoutesAvailable reports whether task routes available.
func taskRoutesAvailable(task evalTask, routes map[string]toolutil.ActionMap, enterprise bool) bool {
	if taskUnavailableInLiveEvaluator(task.ID) {
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
	case "gitlab_discover_project",
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
	enterprise := taskHasEnterpriseStep(task)
	destructive := taskHasDestructiveStep(task)
	mutating := taskHasMutatingStep(task)
	readOnly := !mutating && !destructive
	special := strings.HasPrefix(task.ID, "MF-") || taskHasSimulation(task) || taskUsesCapabilityFallback(task)
	checks := map[string]bool{
		partitionBaseRead:              !enterprise && readOnly && !special,
		partitionBaseMutating:          !enterprise && mutating && !destructive && !special,
		partitionBaseDestructive:       !enterprise && destructive && !special,
		partitionEnterpriseRead:        enterprise && readOnly && !special,
		partitionEnterpriseMutating:    enterprise && mutating && !destructive && !special,
		partitionEnterpriseDestructive: enterprise && destructive && !special,
		partitionErrorRecovery:         strings.HasPrefix(task.ID, "MF-") || taskHasSimulation(task),
		partitionCapabilityFallback:    taskUsesCapabilityFallback(task),
	}
	return checks[partition]
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
	if preset != presetDockerDestructiveSafe {
		return tasks
	}
	return orderSharedFixtureDestructiveLast(tasks)
}

// orderSharedFixtureDestructiveLast moves destructive operations on shared
// Docker fixture resources after tasks that still need those resources intact.
func orderSharedFixtureDestructiveLast(tasks []evalTask) []evalTask {
	regular := make([]evalTask, 0, len(tasks))
	artifactDeletes := make([]evalTask, 0, 1)
	projectArchive := make([]evalTask, 0, 1)
	for _, task := range tasks {
		if taskArchivesSharedProject(task) {
			projectArchive = append(projectArchive, task)
			continue
		}
		if taskDeletesSharedJobArtifacts(task) {
			artifactDeletes = append(artifactDeletes, task)
			continue
		}
		regular = append(regular, task)
	}
	regular = append(regular, artifactDeletes...)
	return append(regular, projectArchive...)
}

// taskArchivesSharedProject reports whether task archives shared project.
func taskArchivesSharedProject(task evalTask) bool {
	for _, step := range taskSteps(task) {
		if step.ExpectedTool == "gitlab_project" && step.ExpectedAction == "archive" {
			return true
		}
		if step.ExpectedTool == dynamicExecuteTool && step.ExpectedAction == "project.archive" {
			return true
		}
	}
	return false
}

// taskDeletesSharedJobArtifacts reports whether a task removes artifacts from
// the shared failed-job fixture used by artifact download/read scenarios.
func taskDeletesSharedJobArtifacts(task evalTask) bool {
	for _, step := range taskSteps(task) {
		if step.ExpectedTool == "gitlab_job" && step.ExpectedAction == "delete_artifacts" {
			return true
		}
		if step.ExpectedTool == dynamicExecuteTool && step.ExpectedAction == "job.delete_artifacts" {
			return true
		}
	}
	return false
}

// taskMatchesPreset reports whether task matches preset.
func taskMatchesPreset(task evalTask, preset string) bool {
	enterprise := taskHasEnterpriseStep(task)
	destructive := taskHasDestructiveStep(task)
	mutating := taskHasMutatingStep(task)
	special := strings.HasPrefix(task.ID, "MF-") || taskHasSimulation(task) || taskUsesCapabilityFallback(task)
	switch preset {
	case presetSchemaEnterprise:
		return enterprise
	case presetDockerRead:
		return !enterprise && !mutating && !destructive && !special
	case presetDockerMutatingSafe:
		return !enterprise && mutating && !destructive && !special
	case presetDockerDestructiveSafe:
		return !enterprise && destructive && !special
	case presetDockerCapabilityDiscovery:
		return taskUsesCapabilityFallback(task)
	default:
		return false
	}
}

// taskHasEnterpriseStep reports whether task has enterprise step.
func taskHasEnterpriseStep(task evalTask) bool {
	for _, step := range taskSteps(task) {
		if routeLooksEnterprise(step.ExpectedTool, step.ExpectedAction) {
			return true
		}
	}
	return false
}

// routeLooksEnterprise reports whether route looks enterprise.
func routeLooksEnterprise(tool, action string) bool {
	domain := canonicalRouteID(tool, action)
	if domain == "" {
		domain = strings.TrimPrefix(tool, "gitlab_")
	}
	for _, prefix := range []string{
		"attestation.", "audit_event.", "compliance_policy.", "dependency.", "dora_metrics.", "enterprise_user.", "external_status_check.", "geo.", "group_analytics.", "group_credential.", "group_epic_board.", "group_iteration.", "group_ldap.", "group_protected_branch.", "group_protected_env.", "group_release.", "group_saml.", "group_scim.", "group_service_account.", "group_ssh_cert.", "group_wiki.", "member_role.", "merge_train.", "project_alias.", "project_iteration.", "security_finding.", "security_setting.", "storage_move.", "vulnerability.",
		"epic.", "epic_discussion.", "epic_issue.", "epic_note.",
		"project.mirror_", "project.push_rule_", "project.security_settings_",
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
	if tool != "gitlab" && tool != dynamicExecuteTool && action != "" {
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
func taskUnavailableInLiveEvaluator(id string) bool {
	switch id {
	case "MT-105", "MT-115":
		return true
	default:
		return false
	}
}

// taskHasMutatingStep reports whether task has mutating step.
func taskHasMutatingStep(task evalTask) bool {
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
		case "add", "approve", "archive", "assign", "bulk", "cancel", "clear", "close", "create", "delete", "disable", "enable", "fork", "keep", "lock", "merge", "move", "play", "protect", "publish", "reject", "remove", "reopen", "resolve", "retry", "revoke", "rotate", "run", "set", "star", "stop", "subscribe", "transfer", "trigger", "unarchive", "unassign", "unlock", "unprotect", "unsubscribe", "update", "upload":
			return true
		}
	}
	return false
}

// parseTasksFile keeps file I/O separate from markdown normalization so tests
// can exercise parser invariants without touching the filesystem.
func parseTasksFile(path string) ([]evalTask, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- task corpus path is an explicit evaluator input.
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	return parseTasksMarkdown(string(data))
}

// parseTasksMarkdown handles parse tasks markdown and returns [[]evalTask].
func parseTasksMarkdown(markdown string) ([]evalTask, error) {
	var tasks []evalTask
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if !isTaskRow(line) {
			continue
		}
		cols := splitMarkdownRow(line)
		if len(cols) < 7 {
			return nil, fmt.Errorf("task row has %d columns, want at least 7: %s", len(cols), line)
		}
		task, err := parseTaskRow(cols)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cols[0], err)
		}
		tasks = append(tasks, task)
	}
	if len(tasks) == 0 {
		return nil, errors.New("no MT-* or MS-* task rows found")
	}
	return tasks, nil
}

// isTaskRow reports whether a Markdown table row describes an evaluation task.
func isTaskRow(line string) bool {
	return strings.HasPrefix(line, "| MT-") || strings.HasPrefix(line, "| MS-") || strings.HasPrefix(line, "| MF-")
}

// parseTaskRow handles parse task row and returns [evalTask].
func parseTaskRow(cols []string) (evalTask, error) {
	steps, err := parseExpectedSteps(cols[2])
	if err != nil {
		return evalTask{}, err
	}
	requiredGroups, err := parseParamGroups(cols[3], len(steps))
	if err != nil {
		return evalTask{}, fmt.Errorf("required params: %w", err)
	}
	optionalGroups, err := parseParamGroups(cols[4], len(steps))
	if err != nil {
		return evalTask{}, fmt.Errorf("optional params: %w", err)
	}
	destructiveFlags, err := parseDestructiveSteps(cols[5], len(steps))
	if err != nil {
		return evalTask{}, fmt.Errorf("destructive steps: %w", err)
	}
	simulations, err := parseSimulationGroups(simulationColumn(cols), len(steps))
	if err != nil {
		return evalTask{}, fmt.Errorf("simulation: %w", err)
	}
	for i := range steps {
		steps[i].RequiredParams = requiredGroups[i]
		steps[i].OptionalParams = optionalGroups[i]
		steps[i].Destructive = destructiveFlags[i]
		steps[i].Simulation = simulations[i]
	}
	first := steps[0]
	return evalTask{
		ID:             cols[0],
		Prompt:         cols[1],
		ExpectedTool:   first.ExpectedTool,
		ExpectedAction: first.ExpectedAction,
		RequiredParams: first.RequiredParams,
		OptionalParams: first.OptionalParams,
		Destructive:    first.Destructive,
		Simulation:     first.Simulation,
		Steps:          steps,
	}, nil
}

// simulationColumn formats simulation column for report output.
func simulationColumn(cols []string) string {
	if len(cols) < 8 {
		return ""
	}
	return cols[6]
}

// validateTaskFixture validates task fixture for the main package.
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

// validateTaskFixtureAgainstRoutes validates task fixture against routes for the main package.
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
// gitlab_execute_tool envelope used by dynamic mode.
func normalizeTasksForDynamicRoutes(tasks []evalTask, routes map[string]toolutil.ActionMap) []evalTask {
	out := make([]evalTask, len(tasks))
	copy(out, tasks)
	for i := range out {
		out[i].ExpectedTool, out[i].ExpectedAction = normalizeExpectedDynamicRoute(out[i].ExpectedTool, out[i].ExpectedAction, routes)
		if len(out[i].Steps) == 0 {
			continue
		}
		out[i].Steps = slices.Clone(out[i].Steps)
		for j := range out[i].Steps {
			out[i].Steps[j].ExpectedTool, out[i].Steps[j].ExpectedAction = normalizeExpectedDynamicRoute(out[i].Steps[j].ExpectedTool, out[i].Steps[j].ExpectedAction, routes)
		}
	}
	return out
}

// normalizeExpectedDynamicRoute maps a fixture's catalog route expectation to
// gitlab_execute_tool when that route exists in the dynamic catalog.
func normalizeExpectedDynamicRoute(tool, action string, routes map[string]toolutil.ActionMap) (normalizedTool, normalizedAction string) {
	if action == "" {
		executeRoutes := routes[dynamicExecuteTool]
		for _, candidate := range standaloneDynamicActionCandidates(tool) {
			if _, ok := executeRoutes[candidate]; ok {
				return dynamicExecuteTool, candidate
			}
		}
		return tool, action
	}
	executeRoutes := routes[dynamicExecuteTool]
	for _, candidate := range dynamicActionCandidates(tool, action) {
		if _, ok := executeRoutes[candidate]; ok {
			return dynamicExecuteTool, candidate
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

// parseExpectedToolAction handles parse expected tool action and returns [string].
func parseExpectedToolAction(value string) (tool, action string, err error) {
	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		tool = strings.Trim(strings.TrimSpace(parts[0]), "`")
		if tool == "" {
			return "", "", fmt.Errorf("empty tool in %q", value)
		}
		return tool, "", nil
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected tool/action pair or standalone tool, got %q", value)
	}
	tool = strings.Trim(strings.TrimSpace(parts[0]), "`")
	action = strings.Trim(strings.TrimSpace(parts[1]), "`")
	if strings.EqualFold(action, "none") || action == "-" {
		action = ""
	}
	if tool == "" {
		return "", "", fmt.Errorf("empty tool/action in %q", value)
	}
	return tool, action, nil
}

// parseExpectedSteps handles parse expected steps and returns [[]evalStep].
func parseExpectedSteps(value string) ([]evalStep, error) {
	parts := strings.Split(value, "->")
	steps := make([]evalStep, 0, len(parts))
	for _, part := range parts {
		tool, action, err := parseExpectedToolAction(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		steps = append(steps, evalStep{ExpectedTool: tool, ExpectedAction: action})
	}
	if len(steps) == 0 {
		return nil, errors.New("empty expected sequence")
	}
	return steps, nil
}

// parseParamGroups handles parse param groups and returns [[][]string].
func parseParamGroups(value string, stepCount int) ([][]string, error) {
	if stepCount == 1 {
		return [][]string{parseParamList(value)}, nil
	}
	groups := strings.Split(value, ";")
	if len(groups) != stepCount {
		return nil, fmt.Errorf("got %d groups, want %d semicolon-separated groups", len(groups), stepCount)
	}
	out := make([][]string, 0, len(groups))
	for _, group := range groups {
		out = append(out, parseParamList(group))
	}
	return out, nil
}

// parseDestructiveSteps handles parse destructive steps and returns [[]bool].
func parseDestructiveSteps(value string, stepCount int) ([]bool, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	flags := make([]bool, stepCount)
	if value == "" || value == "none" || value == "no" {
		return flags, nil
	}
	if value == "yes" {
		if stepCount != 1 {
			return nil, errors.New("use 1-based step numbers or all for multi-step destructive scenarios")
		}
		flags[0] = true
		return flags, nil
	}
	if value == "all" {
		for i := range flags {
			flags[i] = true
		}
		return flags, nil
	}
	for rawPart := range strings.SplitSeq(value, ",") {
		part := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawPart), "step "))
		stepNumber, err := strconv.Atoi(part)
		if err != nil || stepNumber < 1 || stepNumber > stepCount {
			return nil, fmt.Errorf("invalid step number %q", rawPart)
		}
		flags[stepNumber-1] = true
	}
	return flags, nil
}

// parseSimulationGroups handles parse simulation groups and returns [[]string].
func parseSimulationGroups(value string, stepCount int) ([]string, error) {
	if strings.TrimSpace(value) == "" || strings.EqualFold(strings.TrimSpace(value), "none") {
		return make([]string, stepCount), nil
	}
	if stepCount == 1 {
		return []string{normalizeSimulation(value)}, nil
	}
	groups := strings.Split(value, ";")
	if len(groups) != stepCount {
		return nil, fmt.Errorf("got %d groups, want %d semicolon-separated groups", len(groups), stepCount)
	}
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		out = append(out, normalizeSimulation(group))
	}
	return out, nil
}

// normalizeSimulation normalizes simulation for stable comparisons.
func normalizeSimulation(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "`")
	if strings.EqualFold(value, "none") {
		return ""
	}
	return value
}

// parseParamList parses param list from evaluator input.
func parseParamList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return nil
	}
	params := make([]string, 0)
	for part := range strings.SplitSeq(value, ",") {
		name := strings.Trim(strings.TrimSpace(part), "`")
		if name != "" {
			params = append(params, name)
		}
	}
	return params
}

// newMockGitLabClient constructs mock GitLab client.
