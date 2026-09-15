package evaluator

import "os"

// defaultModel is the provider:model label used when no explicit model
// flag or EVAL_MODELS environment variable is supplied. Override at repo
// level via the EVAL_DEFAULT_MODEL environment variable.
var defaultModel = func() string {
	if v := os.Getenv("EVAL_DEFAULT_MODEL"); v != "" {
		return v
	}
	return "anthropic:claude-haiku-4-5-20251001"
}()

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
	// metricSchemaLookupUseRate is the rendered label for the schema lookup
	// use rate metric.
	metricSchemaLookupUseRate = "Schema lookup use rate"
	// metricRepairSuccessRate is the rendered label for the rate at which a
	// model corrected its own call after the harness refused one.
	//
	// It was "Repair success rate" while the refusal handed back the call to
	// make, which made it a transcription rate; V08 cut the payload down to
	// the diagnostic a deployment produces, and the label says what is left.
	metricRepairSuccessRate = "Recovery from server diagnostics"
	// metricDestructiveSafety is the rendered label for the rate at which a
	// destructive call carried its confirmation.
	//
	// It was "Destructive safety" while every prompt told the model to send
	// confirm, which made it a restatement of the prompt rather than a
	// question about the surface. V07 deleted that clause, so the column
	// became a real measurement and takes a name that says so.
	metricDestructiveSafety = "Unaided confirmation"
	// modelToolCallingMode is the Mode a report carries when a model was
	// actually called, and the only one the publish gate accepts.
	modelToolCallingMode = "model tool-calling"
	// metricUnaidedCompletion is the rendered label for the rate at which a
	// task completed with no refusal along the way. It sits beside the final
	// success figure, which counts a task finished after correcting itself.
	metricUnaidedCompletion = "Unaided completion"
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
	// perRunMetricsHeading titles the table a repeated run writes to compare
	// one run with another.
	perRunMetricsHeading = "## Per-Run Metrics"
	// perModelMetricsHeading titles the table a multi-model run writes, and is
	// what the publisher looks for to read those rows back out of a report.
	perModelMetricsHeading = "## Per-Model Metrics"
	// metricStringValueTableRow formats an already-rendered metric value as
	// a markdown table row. Rates are written through it rather than through
	// a float verb, because a rate with no sample behind it renders as a
	// dash and not as a number.
	metricStringValueTableRow = "| %s | %s |\n"
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

// Report header keys, spelled once for the writer in report.go and for every
// reader of a written report. A report that does not say what it was a report
// of cannot be compared with another one, so each of these lines is written on
// every run, carrying [reportValueUnknown] where the value is unavailable
// rather than being left out: an omitted line and a line nothing could parse
// look the same to a reader, and a completeness check passes on a report with
// a hole in it.
const (
	// reportKeyGitBranch is the branch the evaluated tree was on.
	reportKeyGitBranch = "Git branch"
	// reportKeyGitCommit is the short commit the evaluated tree was at.
	reportKeyGitCommit = "Git commit"
	// reportKeyServerMode is the protective mode the catalog was built for:
	// default, read-only or safe-mode.
	reportKeyServerMode = "Server mode"
	// reportKeyTier is the licensing tier the catalog was built for, which
	// decides which actions exist at all.
	reportKeyTier = "Tier"
	// reportKeyMetaParamSchema is the meta-tool input-schema mode the catalog
	// was registered with, which decides how much schema a model was shown.
	reportKeyMetaParamSchema = "Meta param schema"
	// reportKeyTokenScopes is the credential's detected PAT scopes, which
	// narrow the catalog before registration.
	reportKeyTokenScopes = "Token scopes"
	// reportKeyGitLabVersion is the version the evaluated instance answered
	// with.
	reportKeyGitLabVersion = "GitLab version"
	// reportKeyTemperature is the sampling temperature every model request
	// was sent with.
	reportKeyTemperature = "Temperature"
	// reportKeyMaxOutputTokens is the output-token ceiling every model
	// request was sent with.
	reportKeyMaxOutputTokens = "Max output tokens"
	// reportKeyStimulus is the report header key that says what the model was
	// given. A run whose prompts name the expected tool, action or parameters
	// is measuring the prompt builder, not the model, so the header has to
	// carry the answer before any number taken from it may be published.
	reportKeyStimulus = "Stimulus"

	// stimulusCoached is what a run declares while the prompt builder still
	// hands the model the call the scorer checks for.
	stimulusCoached = "coached"
	// stimulusUncoached is the only Stimulus value publication accepts.
	stimulusUncoached = "uncoached"

	// reportValueUnknown is what a header line carries when its value is not
	// available to the run that wrote it.
	reportValueUnknown = "unknown"
)

// evalSamplingTemperature is the temperature every provider request is sent
// with. It is a constant rather than an option because nothing selects it, and
// it is named here so the header states the value the runner actually sends
// instead of a second copy of the number.
const evalSamplingTemperature float64 = 0

// evalElicitationReleaseTag stores the package-level eval elicitation release
// tag state.
