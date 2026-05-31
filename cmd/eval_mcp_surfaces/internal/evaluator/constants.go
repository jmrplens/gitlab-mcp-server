package evaluator

const (
	// defaultEvalDir identifies the default eval dir constant used by this package.
	defaultEvalDir = "dist/evaluation/mcp-surfaces"
	// defaultFixtures identifies the default fixtures constant used by this package.
	defaultFixtures = "dist/evaluation/mcp-surfaces/e2e-fixtures.json"
	// defaultModel identifies the default model constant used by this package.
	defaultModel = "anthropic:claude-haiku-4-5-20251001"
	// backendMock identifies the backend mock constant used by this package.
	backendMock = "mock"
	// backendGitLab identifies the backend GitLab constant used by this package.
	backendGitLab = "gitlab"
	// editionAll identifies the edition selector that keeps every task.
	editionAll = "all"
	// editionCE identifies the GitLab CE/base task selector.
	editionCE = "ce"
	// editionEnterprise identifies the GitLab Enterprise/Premium task selector.
	editionEnterprise = "enterprise"
	// anthropicAPI identifies the anthropic API constant used by this package.
	anthropicAPI = "https://api.anthropic.com/v1/messages"
	// anthropicVersion identifies the anthropic version constant used by this package.
	anthropicVersion = "2023-06-01"
	// toolCallLimit identifies the tool call limit constant used by this package.
	toolCallLimit = 12
	// maxResponseBytes identifies the max response bytes constant used by this package.
	maxResponseBytes = 1 << 20
	// maxToolResultLen identifies the max tool result len constant used by this package.
	maxToolResultLen = 20_000
	// dynamicFindTool identifies the dynamic find tool constant used by this package.
	dynamicFindTool = "gitlab_find_action"
	// dynamicExecuteActionTool identifies the dynamic execute-action MCP tool name used by this package.
	dynamicExecuteActionTool = "gitlab_execute_action"
	// resourceListTool identifies the evaluator resource-list bridge tool.
	resourceListTool = "gitlab_list_resources"
	// resourceReadTool identifies the evaluator resource-read bridge tool.
	resourceReadTool = "gitlab_read_resource"
	// capabilityListTool identifies the evaluator MCP capability bridge tool.
	capabilityListTool = "gitlab_list_capabilities"
	// promptListTool identifies the evaluator prompt-list bridge tool.
	promptListTool = "gitlab_list_prompts"
	// promptGetTool identifies the evaluator prompt-get bridge tool.
	promptGetTool = "gitlab_get_prompt"
	// completionTool identifies the evaluator completion bridge tool.
	completionTool = "gitlab_complete"
	// defaultDockerComposeFile identifies the default Docker Compose file for GitLab evaluator runs.
	defaultDockerComposeFile = "test/e2e/docker-compose.yml"
	// defaultDockerGitLabURL identifies the default host URL exposed by the Docker GitLab fixture stack.
	defaultDockerGitLabURL = "http://localhost:8929"
	// defaultDockerGitLabEEImage identifies the default GitLab EE image used for Enterprise Docker runtimes.
	defaultDockerGitLabEEImage = "gitlab/gitlab-ee:latest"
)

const (
	// presetSchemaEnterprise identifies the preset schema enterprise constant used by this package.
	presetSchemaEnterprise = "schema-enterprise"
	// presetDockerRead identifies the preset docker read constant used by this package.
	presetDockerRead = "docker-read"
	// presetDockerMutatingSafe identifies the preset docker mutating safe constant used by this package.
	presetDockerMutatingSafe = "docker-mutating-safe"
	// presetDockerDestructiveSafe identifies the preset docker destructive safe constant used by this package.
	presetDockerDestructiveSafe = "docker-destructive-safe"
	// presetDockerEnterpriseRead identifies the Docker-backed Enterprise read-only preset.
	presetDockerEnterpriseRead = "docker-enterprise-read"
	// presetDockerEnterpriseMutatingSafe identifies the Docker-backed Enterprise safe mutation preset.
	presetDockerEnterpriseMutatingSafe = "docker-enterprise-mutating-safe"
	// presetDockerEnterpriseDestructiveSafe identifies the Docker-backed Enterprise destructive preset.
	presetDockerEnterpriseDestructiveSafe = "docker-enterprise-destructive-safe"
	// presetDockerCapabilityDiscovery identifies the Docker-backed MCP capability discovery preset.
	presetDockerCapabilityDiscovery = "docker-capability-discovery"
	// presetDockerErrorRecovery identifies the Docker-backed fault-injection/error-recovery preset.
	presetDockerErrorRecovery = "docker-error-recovery"

	// partitionBaseRead identifies the partition base read constant used by this package.
	partitionBaseRead = "base-read"
	// partitionBaseMutating identifies the partition base mutating constant used by this package.
	partitionBaseMutating = "base-mutating"
	// partitionBaseDestructive identifies the partition base destructive constant used by this package.
	partitionBaseDestructive = "base-destructive"
	// partitionEnterpriseRead identifies the partition enterprise read constant used by this package.
	partitionEnterpriseRead = "enterprise-read"
	// partitionEnterpriseMutating identifies the partition enterprise mutating constant used by this package.
	partitionEnterpriseMutating = "enterprise-mutating"
	// partitionEnterpriseDestructive identifies the partition enterprise destructive constant used by this package.
	partitionEnterpriseDestructive = "enterprise-destructive"
	// partitionErrorRecovery identifies the partition error recovery constant used by this package.
	partitionErrorRecovery = "error-recovery"
	// partitionCapabilityFallback identifies the partition capability fallback constant used by this package.
	partitionCapabilityFallback = "capability-fallback"
	// flagSkipDestructive identifies the flag skip destructive constant used by this package.
	flagSkipDestructive = "skip-destructive"
	// flagSkipMutating identifies the flag skip mutating constant used by this package.
	flagSkipMutating = "skip-mutating"
	// flagOnlyDestructive identifies the flag only destructive constant used by this package.
	flagOnlyDestructive = "only-destructive"
	// flagOnlyMutating identifies the flag only mutating constant used by this package.
	flagOnlyMutating = "only-mutating"
	// flagSkipUnavailable identifies the flag skip unavailable constant used by this package.
	flagSkipUnavailable = "skip-unavailable"
	// promptMarkerIssue identifies the prompt marker issue constant used by this package.
	promptMarkerIssue = "issue "
	// promptMarkerMergeRequest identifies the prompt marker merge request constant used by this package.
	promptMarkerMergeRequest = "merge request "
	// promptMarkerBranch identifies the prompt marker branch constant used by this package.
	promptMarkerBranch = "branch "
	// promptMarkerProject identifies the prompt marker project constant used by this package.
	promptMarkerProject = "project "
	// promptMarkerAllowlistProject identifies allowlist project prompt markers.
	promptMarkerAllowlistProject = "allowlist of project "
	// promptMarkerIssueIID identifies issue IID prompt markers.
	promptMarkerIssueIID = "issue IID "
	// promptMarkerGroupPath identifies group path prompt markers.
	promptMarkerGroupPath = "group path "
	// promptMarkerAwardEmojiID identifies the prompt marker award emoji ID constant used by this package.
	promptMarkerAwardEmojiID = "award emoji ID "
	// promptMarkerFrom identifies the prompt marker from constant used by this package.
	promptMarkerFrom = " from "
	// promptPhraseFailedJobs identifies the prompt phrase failed jobs constant used by this package.
	promptPhraseFailedJobs = "failed jobs"
	// metricToolSelection identifies the metric tool selection constant used by this package.
	metricToolSelection = "Tool-selection accuracy"
	// metricActionSelection identifies the metric action selection constant used by this package.
	metricActionSelection = "Action-selection accuracy"
	// metricFirstCallValidationPassRate identifies the metric first call validation pass rate constant used by this package.
	metricFirstCallValidationPassRate = "First-call validation pass rate"
	// metricRepairSuccessRate identifies the metric repair success rate constant used by this package.
	metricRepairSuccessRate = "Repair success rate"
	// metricDestructiveSafety identifies the metric destructive safety constant used by this package.
	metricDestructiveSafety = "Destructive safety"
	// metricFinalTaskSuccess identifies the metric final task success constant used by this package.
	metricFinalTaskSuccess = "Final task success proxy"
	// metricEstimatedTokens identifies the metric estimated tokens constant used by this package.
	metricEstimatedTokens = "Estimated tokens"
	// metricValueTableHeader identifies the metric value table header constant used by this package.
	metricValueTableHeader = "| Metric | Value |\n| --- | ---: |\n"
	// metricIntegerValueTableRow identifies the metric integer value table row constant used by this package.
	metricIntegerValueTableRow = "| %s | %d |\n"
	// timestampLayout identifies the UTC timestamp layout used for generated evaluator artifacts.
	timestampLayout = "20060102-150405"

	// actionDiscoverProjectResolve identifies the action discover project resolve constant used by this package.
	actionDiscoverProjectResolve = "discover_project.resolve"
	// actionSearchProjects identifies the action search projects constant used by this package.
	actionSearchProjects = "search.projects"
	// actionProjectGet identifies the action project get constant used by this package.
	actionProjectGet = "project.get"
	// actionProjectList identifies the action project list constant used by this package.
	actionProjectList = "project.list"
	// actionEnvironmentProtectedList identifies the action environment protected list constant used by this package.
	actionEnvironmentProtectedList = "environment.protected_list"
	// actionPipelineGet identifies the action pipeline get constant used by this package.
	actionPipelineGet = "pipeline.get"
	// actionIssueCreate identifies the action issue create constant used by this package.
	actionIssueCreate = "issue.create"
	// actionIssueLinkCreate identifies the action issue link create constant used by this package.
	actionIssueLinkCreate = "issue.link_create"
	// errBuildActionCatalog identifies the err build action catalog constant used by this package.
	errBuildActionCatalog = "build action catalog: %w"
	// diagnosticUnknownParams identifies the diagnostic unknown params constant used by this package.
	diagnosticUnknownParams = "unknown params"
	// diagnosticMissingRequiredParams identifies missing required params diagnostics.
	diagnosticMissingRequiredParams = "missing required params"
	// diagnosticMissingRequiredStandalone identifies standalone missing required field diagnostics.
	diagnosticMissingRequiredStandalone = "missing required "
	// diagnosticNotFound identifies the diagnostic not found constant used by this package.
	diagnosticNotFound = "not found"
	// diagnosticExpectedAction identifies the diagnostic expected action constant used by this package.
	diagnosticExpectedAction = "expected action"
	// diagnosticUnexpectedTopLevelParameter identifies invalid dynamic envelopes.
	diagnosticUnexpectedTopLevelParameter = "unexpected top-level parameter"
)

// evalElicitationReleaseTag stores the package-level eval elicitation release tag state.
