package evaluator

// Default directory, model, and backend identifiers used to bootstrap the
// evaluator when explicit flags are absent. All values are mirrored on the
// command line so flag documentation should stay in sync with this block.
const (
	// defaultEvalDir is the directory under which generated reports,
	// fixtures, and terminal logs are written.
	defaultEvalDir = "dist/evaluation/mcp-surfaces"
	// defaultFixtures is the JSON file used to persist live fixture state
	// between prepare-fixtures and use-fixtures invocations.
	defaultFixtures = "dist/evaluation/mcp-surfaces/e2e-fixtures.json"
	// defaultModel is the provider:model label used when no explicit model
	// flag or EVAL_MODELS environment variable is supplied.
	defaultModel = "anthropic:claude-haiku-4-5-20251001"
	// backendMock keeps the catalog offline; no live GitLab calls execute.
	backendMock = "mock"
	// backendGitLab switches the catalog backend to a real GitLab instance.
	backendGitLab = "gitlab"
	// editionAll keeps every case regardless of edition tag.
	editionAll = "all"
	// editionCE selects cases targeted at GitLab CE/base.
	editionCE = "ce"
	// editionEnterprise selects cases that require GitLab Enterprise/Premium.
	editionEnterprise = "enterprise"
	// anthropicAPI is the Anthropic Messages endpoint used by the
	// first-party provider implementation.
	anthropicAPI = "https://api.anthropic.com/v1/messages"
	// anthropicVersion is the Anthropic API version header sent with every
	// first-party request.
	anthropicVersion = "2023-06-01"
	// toolCallLimit is the floor applied to per-task tool-call budgets.
	toolCallLimit = 12
	// maxResponseBytes caps the size of provider response bodies that the
	// runner keeps in memory.
	maxResponseBytes = 1 << 20
	// maxToolResultLen is the character limit applied to simulated and live
	// tool results before they are returned to the model.
	maxToolResultLen = 20_000
	// dynamicFindTool is the dynamic-surface discovery tool name.
	dynamicFindTool = "gitlab_find_action"
	// dynamicExecuteActionTool is the dynamic-surface dispatcher tool name.
	dynamicExecuteActionTool = "gitlab_execute_action"
	// resourceListTool is the evaluator bridge tool that lists MCP resources.
	resourceListTool = "gitlab_list_resources"
	// resourceReadTool is the evaluator bridge tool that reads MCP resources.
	resourceReadTool = "gitlab_read_resource"
	// capabilityListTool is the evaluator bridge tool that reports MCP
	// server capabilities.
	capabilityListTool = "gitlab_list_capabilities"
	// promptListTool is the evaluator bridge tool that lists MCP prompts.
	promptListTool = "gitlab_list_prompts"
	// promptGetTool is the evaluator bridge tool that renders one MCP prompt.
	promptGetTool = "gitlab_get_prompt"
	// completionTool is the evaluator bridge tool that requests MCP argument
	// completions.
	completionTool = "gitlab_complete"
	// defaultDockerComposeFile is the Compose file used by Docker-backed
	// presets when --docker-compose-file is not supplied.
	defaultDockerComposeFile = "test/e2e/docker-compose.yml"
	// defaultDockerGitLabURL is the host URL exposed by the Docker GitLab
	// fixture stack.
	defaultDockerGitLabURL = "http://localhost:8929"
	// defaultDockerGitLabEEImage is the GitLab EE image used by Enterprise
	// Docker runtimes when no override is supplied.
	defaultDockerGitLabEEImage = "gitlab/gitlab-ee:latest"
)

// Preset, partition, and diagnostic identifiers shared between the CLI and the
// evaluator runtime. Presets compose edition, partition, and Docker flags; the
// constants below double as the canonical names referenced by docs and tests.
const (
	// presetSchemaEnterprise runs the typed registry against the schema
	// validator without live GitLab calls.
	presetSchemaEnterprise = "schema-enterprise"
	// presetDockerRead runs the read-only subset of the CE registry against a
	// Docker GitLab instance.
	presetDockerRead = "docker-read"
	// presetDockerMutatingSafe runs CE cases that mutate state but skip
	// destructive steps.
	presetDockerMutatingSafe = "docker-mutating-safe"
	// presetDockerDestructiveSafe runs destructive CE cases with confirm.
	presetDockerDestructiveSafe = "docker-destructive-safe"
	// presetDockerEnterpriseRead runs Enterprise read-only cases against a
	// Docker GitLab instance.
	presetDockerEnterpriseRead = "docker-enterprise-read"
	// presetDockerEnterpriseMutatingSafe runs Enterprise mutating cases.
	presetDockerEnterpriseMutatingSafe = "docker-enterprise-mutating-safe"
	// presetDockerEnterpriseDestructiveSafe runs Enterprise destructive cases.
	presetDockerEnterpriseDestructiveSafe = "docker-enterprise-destructive-safe"
	// presetDockerCapabilityDiscovery runs MCP capability discovery cases.
	presetDockerCapabilityDiscovery = "docker-capability-discovery"
	// presetDockerErrorRecovery runs the fault-injection/error-recovery cases.
	presetDockerErrorRecovery = "docker-error-recovery"

	// partitionBaseRead groups read-only CE cases.
	partitionBaseRead = "base-read"
	// partitionBaseMutating groups CE cases that mutate state but are not
	// destructive.
	partitionBaseMutating = "base-mutating"
	// partitionBaseDestructive groups destructive CE cases.
	partitionBaseDestructive = "base-destructive"
	// partitionEnterpriseRead groups read-only Enterprise cases.
	partitionEnterpriseRead = "enterprise-read"
	// partitionEnterpriseMutating groups Enterprise cases that mutate state.
	partitionEnterpriseMutating = "enterprise-mutating"
	// partitionEnterpriseDestructive groups destructive Enterprise cases.
	partitionEnterpriseDestructive = "enterprise-destructive"
	// partitionErrorRecovery groups error-recovery/fault-injection cases.
	partitionErrorRecovery = "error-recovery"
	// partitionCapabilityFallback groups cases that depend on the MCP
	// capability bridge rather than the action catalog.
	partitionCapabilityFallback = "capability-fallback"
	// flagSkipDestructive is the canonical name of the destructive-skip flag.
	flagSkipDestructive = "skip-destructive"
	// flagSkipMutating is the canonical name of the mutation-skip flag.
	flagSkipMutating = "skip-mutating"
	// flagOnlyDestructive is the canonical name of the destructive-only flag.
	flagOnlyDestructive = "only-destructive"
	// flagOnlyMutating is the canonical name of the mutation-only flag.
	flagOnlyMutating = "only-mutating"
	// flagSkipUnavailable is the canonical name of the unavailable-skip flag.
	flagSkipUnavailable = "skip-unavailable"

	// promptMarkerIssue is the natural-language marker that the prompt parser
	// recognizes when extracting issue identifiers.
	promptMarkerIssue = "issue "
	// promptMarkerMergeRequest is the natural-language marker recognized when
	// extracting merge-request identifiers.
	promptMarkerMergeRequest = "merge request "
	// promptMarkerBranch is the natural-language marker recognized when
	// extracting branch names.
	promptMarkerBranch = "branch "
	// promptMarkerProject is the natural-language marker recognized when
	// extracting project paths.
	promptMarkerProject = "project "
	// promptMarkerAllowlistProject is the marker recognized around CI job
	// token allowlist project identifiers.
	promptMarkerAllowlistProject = "allowlist of project "
	// promptMarkerIssueIID is the marker recognized around an issue IID.
	promptMarkerIssueIID = "issue IID "
	// promptMarkerGroupPath is the marker recognized around a group path.
	promptMarkerGroupPath = "group path "
	// promptMarkerAwardEmojiID is the marker recognized around an award
	// emoji identifier.
	promptMarkerAwardEmojiID = "award emoji ID "
	// promptMarkerFrom is the shared connector marker used by compare-style
	// prompts (e.g. "from `main` to `feature/x`").
	promptMarkerFrom = " from "
	// promptPhraseFailedJobs is the literal phrase that selects the failed
	// jobs pipeline workflow.
	promptPhraseFailedJobs = "failed jobs"

	// metricToolSelection is the rendered label for the tool-selection metric.
	metricToolSelection = "Tool-selection accuracy"
	// metricActionSelection is the rendered label for the action-selection
	// metric.
	metricActionSelection = "Action-selection accuracy"
	// metricFirstCallValidationPassRate is the rendered label for the
	// first-call validation pass rate metric.
	metricFirstCallValidationPassRate = "First-call validation pass rate"
	// metricRepairSuccessRate is the rendered label for the repair success
	// rate metric.
	metricRepairSuccessRate = "Repair success rate"
	// metricDestructiveSafety is the rendered label for the destructive
	// safety metric.
	metricDestructiveSafety = "Destructive safety"
	// metricFinalTaskSuccess is the rendered label for the final task
	// success proxy metric.
	metricFinalTaskSuccess = "Final task success proxy"
	// metricEstimatedTokens is the rendered label for the estimated tokens
	// metric.
	metricEstimatedTokens = "Estimated tokens"
	// metricValueTableHeader is the markdown header for a metric value row.
	metricValueTableHeader = "| Metric | Value |\n| --- | ---: |\n"
	// metricIntegerValueTableRow formats a single metric integer as a
	// markdown table row.
	metricIntegerValueTableRow = "| %s | %d |\n"
	// timestampLayout is the UTC timestamp layout used for generated
	// evaluator artifacts.
	timestampLayout = "20060102-150405"

	// actionDiscoverProjectResolve is the dynamic action that resolves a
	// remote URL to a GitLab project.
	actionDiscoverProjectResolve = "discover_project.resolve"
	// actionSearchProjects is the dynamic action that searches projects.
	actionSearchProjects = "search.projects"
	// actionProjectGet is the dynamic action that fetches a project.
	actionProjectGet = "project.get"
	// actionProjectList is the dynamic action that lists projects.
	actionProjectList = "project.list"
	// actionEnvironmentProtectedList is the dynamic action that lists
	// protected environments.
	actionEnvironmentProtectedList = "environment.protected_list"
	// actionPipelineGet is the dynamic action that fetches a pipeline.
	actionPipelineGet = "pipeline.get"
	// actionIssueCreate is the dynamic action that creates an issue.
	actionIssueCreate = "issue.create"
	// actionIssueLinkCreate is the dynamic action that links two issues.
	actionIssueLinkCreate = "issue.link_create"
	// errBuildActionCatalog formats the wrapped error returned when the
	// action catalog cannot be loaded.
	errBuildActionCatalog = "build action catalog: %w"
	// diagnosticUnknownParams is the substring emitted when the model passes
	// parameters that the schema does not list.
	diagnosticUnknownParams = "unknown params"
	// diagnosticMissingRequiredParams is the substring emitted when required
	// parameters are missing from the call.
	diagnosticMissingRequiredParams = "missing required params"
	// diagnosticMissingRequiredStandalone is the standalone-prefixed variant
	// of the missing-required diagnostic.
	diagnosticMissingRequiredStandalone = "missing required "
	// diagnosticNotFound is the substring emitted when a tool returns a
	// not-found error that should be reflected in repair feedback.
	diagnosticNotFound = "not found"
	// diagnosticExpectedAction is the substring emitted when the model
	// selected the wrong action for a step.
	diagnosticExpectedAction = "expected action"
	// diagnosticUnexpectedTopLevelParameter is the substring emitted when a
	// dynamic call wraps its arguments in an invalid top-level parameter.
	diagnosticUnexpectedTopLevelParameter = "unexpected top-level parameter"
)

// evalElicitationReleaseTag stores the package-level eval elicitation release
// tag state.
