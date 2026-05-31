package cases

import (
	"maps"
	"slices"
)

// All returns every typed evaluation case definition.
func All() []Case {
	cases := readEvalCases()
	cases = append(cases, mutatingEvalCases()...)
	cases = append(cases, destructiveEvalCases()...)
	cases = append(cases, capabilityDiscoveryEvalCases()...)
	cases = append(cases, enterpriseReadEvalCases()...)
	cases = append(cases, enterpriseMutatingEvalCases()...)
	cases = append(cases, enterpriseDestructiveEvalCases()...)
	cases = append(cases, errorRecoveryEvalCases()...)
	return cloneCases(cases)
}

func cloneCases(cases []Case) []Case {
	out := make([]Case, 0, len(cases))
	for _, evalCase := range cases {
		evalCase.PromptTemplate.Variables = slices.Clone(evalCase.PromptTemplate.Variables)
		evalCase.Steps = cloneSteps(evalCase.Steps)
		evalCase.Fixtures = slices.Clone(evalCase.Fixtures)
		evalCase.Assertions = cloneAssertions(evalCase.Assertions)
		evalCase.Presets = slices.Clone(evalCase.Presets)
		evalCase.Tags = slices.Clone(evalCase.Tags)
		evalCase.SkipReasons = slices.Clone(evalCase.SkipReasons)
		out = append(out, evalCase)
	}
	return out
}

func cloneSteps(steps []Step) []Step {
	out := make([]Step, 0, len(steps))
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

func cloneAssertions(assertions []Assertion) []Assertion {
	out := make([]Assertion, 0, len(assertions))
	for _, assertion := range assertions {
		assertion.Inputs = slices.Clone(assertion.Inputs)
		assertion.Expected = maps.Clone(assertion.Expected)
		out = append(out, assertion)
	}
	return out
}
