package evaluator

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	casecatalog "github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/evaluator/cases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// AllEvalCases returns the typed evaluation case registry.
func AllEvalCases() []EvalCase {
	return cloneEvalCases(evalCasesFromDefinitions(casecatalog.All()))
}

// CaseByID looks up one typed evaluation case by ID.
func CaseByID(id string) (EvalCase, bool) {
	for _, evalCase := range AllEvalCases() {
		if string(evalCase.ID) == id {
			return evalCase, true
		}
	}
	return EvalCase{}, false
}

// CasesByPreset returns typed evaluation cases matching an evaluator preset.
func CasesByPreset(preset string) []EvalCase {
	var out []EvalCase
	for _, evalCase := range AllEvalCases() {
		if evalCaseMatchesPreset(evalCase, preset) {
			out = append(out, evalCase)
		}
	}
	return out
}

// ValidateEvalCaseRegistry validates the migrated typed evaluation registry.
func ValidateEvalCaseRegistry(routes map[string]toolutil.ActionMap) []string {
	return validateEvalCaseRegistry(AllEvalCases(), routes)
}

func loadEvalCases(opts options) ([]EvalCase, error) {
	if customTasksPath(opts.TasksPath) {
		return nil, fmt.Errorf("custom --tasks Markdown files are deprecated; add typed EvalCase definitions instead: %s", opts.TasksPath)
	}
	return AllEvalCases(), nil
}

func evalTasksFromCases(cases []EvalCase) []evalTask {
	tasks := make([]evalTask, 0, len(cases))
	for _, evalCase := range cases {
		tasks = append(tasks, taskFromCase(evalCase))
	}
	return tasks
}

func evalCasesFromDefinitions(definitions []casecatalog.Case) []EvalCase {
	cases := make([]EvalCase, 0, len(definitions))
	for _, definition := range definitions {
		cases = append(cases, evalCaseFromDefinition(definition))
	}
	return cases
}

func evalCaseFromDefinition(definition casecatalog.Case) EvalCase {
	return EvalCase{
		ID:               EvalCaseID(definition.ID),
		Title:            definition.Title,
		Prompt:           definition.Prompt,
		PromptTemplate:   promptTemplateFromDefinition(definition.PromptTemplate),
		Steps:            expectedStepsFromDefinitions(definition.Steps),
		Fixtures:         caseFixturesFromNames(definition.Fixtures),
		Assertions:       caseAssertionsFromDefinitions(definition.Assertions),
		Metrics:          caseMetricsFromDefinition(definition.Metrics),
		Edition:          EvalCaseEdition(definition.Edition),
		Presets:          evalPresetsFromDefinitions(definition.Presets),
		Partition:        EvalPartition(definition.Partition),
		Tags:             slices.Clone(definition.Tags),
		Mutating:         definition.Mutating,
		Destructive:      definition.Destructive,
		CapabilityBridge: definition.CapabilityBridge,
		SkipReasons:      slices.Clone(definition.SkipReasons),
		ReportGroup:      definition.ReportGroup,
	}
}

func promptTemplateFromDefinition(definition casecatalog.PromptTemplate) CasePromptTemplate {
	return CasePromptTemplate{
		Text:      definition.Text,
		Variables: slices.Clone(definition.Variables),
	}
}

func expectedStepsFromDefinitions(definitions []casecatalog.Step) []ExpectedStep {
	steps := make([]ExpectedStep, 0, len(definitions))
	for _, definition := range definitions {
		steps = append(steps, ExpectedStep{
			ExpectedTool:    definition.ExpectedTool,
			ExpectedAction:  definition.ExpectedAction,
			RequiredParams:  slices.Clone(definition.RequiredParams),
			OptionalParams:  slices.Clone(definition.OptionalParams),
			ForbiddenParams: slices.Clone(definition.ForbiddenParams),
			OptionalStep:    definition.OptionalStep,
			Destructive:     definition.Destructive,
			Simulation:      definition.Simulation,
			AllowedRepairs:  slices.Clone(definition.AllowedRepairs),
			ProducedValues:  slices.Clone(definition.ProducedValues),
		})
	}
	return steps
}

func caseAssertionsFromDefinitions(definitions []casecatalog.Assertion) []CaseAssertion {
	assertions := make([]CaseAssertion, 0, len(definitions))
	for _, definition := range definitions {
		assertions = append(assertions, CaseAssertion{
			Type:        CaseAssertionType(definition.Type),
			Step:        definition.Step,
			Name:        definition.Name,
			Description: definition.Description,
			Required:    definition.Required,
			Inputs:      slices.Clone(definition.Inputs),
			Expected:    maps.Clone(definition.Expected),
		})
	}
	return assertions
}

func caseMetricsFromDefinition(definition casecatalog.MetricsSpec) CaseMetricsSpec {
	return CaseMetricsSpec{
		ExpectedModelCalls: definition.ExpectedModelCalls,
		ExpectedToolCalls:  definition.ExpectedToolCalls,
		FinalSuccess:       definition.FinalSuccess,
	}
}

func evalPresetsFromDefinitions(definitions []string) []EvalPreset {
	presets := make([]EvalPreset, 0, len(definitions))
	for _, definition := range definitions {
		presets = append(presets, EvalPreset(definition))
	}
	return presets
}

func caseFixturesFromNames(names []string) []CaseFixtureSpec {
	if len(names) == 0 {
		return nil
	}
	fixtures := make([]CaseFixtureSpec, 0, len(names))
	for _, name := range names {
		fixture, ok := caseFixtureSpecsByName[name]
		if !ok {
			panic(fmt.Sprintf("unknown evaluator case fixture %q", name))
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

var caseFixtureSpecsByName = buildCaseFixtureSpecsByName()

func buildCaseFixtureSpecsByName() map[string]CaseFixtureSpec {
	fixtures := baseMutatingFixtureSpecs()
	fixtures = append(
		fixtures,
		PipelineJobFixture,
		MergeRequestDiscussionFixture,
		MergeRequestSourceFixture,
		ReleaseCreateSourceFixture,
		MergeableMergeRequestFixture,
		JobTokenScopeProjectFixture,
		FailedJobArtifactFixture,
		MergeRequestAwardEmojiFixture,
		IssueAwardEmojiFixture,
		GroupDeleteFixture,
		IssueDeleteFixture,
		ProjectCIVariableDeleteFixture,
		RepositoryFileDeleteFixture,
		MilestoneDeleteFixture,
		ReleaseDeleteFixture,
		ProjectAccessTokenRevokeFixture,
		ProjectArchiveFixture,
		PackageDeleteFixture,
		PipelineDeleteFixture,
		PipelineTriggerDeleteFixture,
		PipelineScheduleDeleteFixture,
		RunnerRemoveFixture,
		EnvironmentStopFixture,
		SnippetDeleteFixture,
		BroadcastMessageDeleteFixture,
		ProjectHookDeleteFixture,
		ProjectBadgeDeleteFixture,
		DraftNotePublishAllFixture,
		InstanceCIVariableDeleteFixture,
		BranchDeleteFixture,
		TagDeleteFixture,
		UserBlockFixture,
		FeatureFlagDeleteFixture,
		WikiDeleteFixture,
		DeployKeyLifecycleFixture,
		DeployKeyDeleteFixture,
		DeployTokenDeleteFixture,
		CommitDiscussionDeleteNoteFixture,
		BranchProtectionLifecycleFixture,
		ProjectServiceAccountFixture,
		EnterprisePushRuleProjectFixture(false),
		EnterprisePushRuleProjectFixture(true),
		EnterpriseGroupServiceAccountFixture(false),
		EnterpriseGroupServiceAccountFixture(true),
	)

	byName := make(map[string]CaseFixtureSpec, len(fixtures))
	for _, fixture := range fixtures {
		byName[fixture.Name] = fixture
	}
	return byName
}

func validateEvalCaseRegistry(cases []EvalCase, routes map[string]toolutil.ActionMap) []string {
	var problems []string
	seen := map[EvalCaseID]struct{}{}
	for _, evalCase := range cases {
		label := string(evalCase.ID)
		if strings.TrimSpace(label) == "" {
			label = "<empty>"
			problems = append(problems, "case has empty ID")
		}
		if _, ok := seen[evalCase.ID]; ok {
			problems = append(problems, label+" has duplicate ID")
		}
		seen[evalCase.ID] = struct{}{}
		if strings.TrimSpace(casePrompt(evalCase)) == "" {
			problems = append(problems, label+" has empty prompt")
		}
		if len(evalCase.Steps) == 0 {
			problems = append(problems, label+" has no expected steps")
		}
		for _, preset := range evalCase.Presets {
			if !validPreset(string(preset)) {
				problems = append(problems, fmt.Sprintf("%s uses unknown preset %q", label, preset))
			}
		}
		if evalCase.Partition != "" && !validPartition(string(evalCase.Partition)) {
			problems = append(problems, fmt.Sprintf("%s uses unknown partition %q", label, evalCase.Partition))
		}
		problems = append(problems, validateEvalCaseSteps(evalCase, routes)...)
	}
	return problems
}

func validateEvalCaseSteps(evalCase EvalCase, routes map[string]toolutil.ActionMap) []string {
	var problems []string
	label := string(evalCase.ID)
	for stepIndex, step := range evalCase.Steps {
		stepLabel := label
		if len(evalCase.Steps) > 1 {
			stepLabel = fmt.Sprintf("%s step %d", label, stepIndex+1)
		}
		if strings.TrimSpace(step.ExpectedTool) == "" {
			problems = append(problems, stepLabel+" has empty expected tool")
		}
		if step.Destructive && !hasParam(step.OptionalParams, "confirm") && !hasParam(step.RequiredParams, "confirm") {
			problems = append(problems, stepLabel+" is destructive but does not list confirm as a parameter")
		}
		problems = append(problems, validateOptionalStepScope(evalCase, stepIndex, stepLabel)...)
		if routes == nil || step.ExpectedAction == "" {
			continue
		}
		if _, ok := routes[step.ExpectedTool][step.ExpectedAction]; !ok {
			problems = append(problems, fmt.Sprintf("%s expected route %s/%s is not registered", stepLabel, step.ExpectedTool, step.ExpectedAction))
		}
	}
	return problems
}

func validateOptionalStepScope(evalCase EvalCase, stepIndex int, stepLabel string) []string {
	step := evalCase.Steps[stepIndex]
	if !step.OptionalStep {
		return nil
	}
	if !expectedCapabilityBridgeStep(step) {
		return []string{stepLabel + " marks a non-capability bridge step as optional"}
	}
	nextIndex := stepIndex + 1
	if nextIndex >= len(evalCase.Steps) || !expectedCapabilityBridgeStep(evalCase.Steps[nextIndex]) {
		return []string{stepLabel + " optional capability bridge step must be followed by another capability bridge step"}
	}
	return nil
}

func evalCaseMatchesPreset(evalCase EvalCase, preset string) bool {
	for _, candidate := range evalCase.Presets {
		if string(candidate) == preset {
			return true
		}
	}
	return false
}

func customTasksPath(path string) bool {
	path = strings.TrimSpace(path)
	return path != ""
}

func cloneEvalCases(cases []EvalCase) []EvalCase {
	out := make([]EvalCase, 0, len(cases))
	for _, evalCase := range cases {
		evalCase.PromptTemplate.Variables = slices.Clone(evalCase.PromptTemplate.Variables)
		evalCase.Steps = cloneExpectedSteps(evalCase.Steps)
		evalCase.Fixtures = slices.Clone(evalCase.Fixtures)
		evalCase.Assertions = cloneCaseAssertions(evalCase.Assertions)
		evalCase.Presets = slices.Clone(evalCase.Presets)
		evalCase.Tags = slices.Clone(evalCase.Tags)
		evalCase.SkipReasons = slices.Clone(evalCase.SkipReasons)
		out = append(out, evalCase)
	}
	return out
}

func cloneCaseAssertions(assertions []CaseAssertion) []CaseAssertion {
	out := make([]CaseAssertion, 0, len(assertions))
	for _, assertion := range assertions {
		assertion.Inputs = slices.Clone(assertion.Inputs)
		assertion.Expected = maps.Clone(assertion.Expected)
		out = append(out, assertion)
	}
	return out
}

func cloneExpectedSteps(steps []ExpectedStep) []ExpectedStep {
	out := make([]ExpectedStep, 0, len(steps))
	for _, step := range steps {
		step.RequiredParams = slices.Clone(step.RequiredParams)
		step.OptionalParams = slices.Clone(step.OptionalParams)
		step.ForbiddenParams = slices.Clone(step.ForbiddenParams)
		step.AllowedRepairs = slices.Clone(step.AllowedRepairs)
		step.ProducedValues = slices.Clone(step.ProducedValues)
		out = append(out, step)
	}
	return out
}
