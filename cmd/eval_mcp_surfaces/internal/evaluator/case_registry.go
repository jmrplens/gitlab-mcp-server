package evaluator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// AllEvalCases returns the typed evaluation case registry.
func AllEvalCases() []EvalCase {
	cases := readEvalCases()
	cases = append(cases, mutatingEvalCases()...)
	cases = append(cases, destructiveEvalCases()...)
	cases = append(cases, capabilityDiscoveryEvalCases()...)
	cases = append(cases, enterpriseReadEvalCases()...)
	cases = append(cases, enterpriseMutatingEvalCases()...)
	cases = append(cases, enterpriseDestructiveEvalCases()...)
	return cloneEvalCases(cases)
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
		evalCase.Steps = cloneExpectedSteps(evalCase.Steps)
		evalCase.Fixtures = slices.Clone(evalCase.Fixtures)
		evalCase.Assertions = slices.Clone(evalCase.Assertions)
		evalCase.Presets = slices.Clone(evalCase.Presets)
		evalCase.Tags = slices.Clone(evalCase.Tags)
		evalCase.SkipReasons = slices.Clone(evalCase.SkipReasons)
		out = append(out, evalCase)
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
