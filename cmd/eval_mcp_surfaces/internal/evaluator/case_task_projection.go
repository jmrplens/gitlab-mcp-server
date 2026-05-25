package evaluator

import "slices"

func taskFromCase(evalCase EvalCase) evalTask {
	steps := stepsFromCase(evalCase)
	task := evalTask{
		ID:     string(evalCase.ID),
		Prompt: casePrompt(evalCase),
		Steps:  steps,
		Case:   cloneEvalCasePtr(evalCase),
	}
	if len(steps) == 0 {
		return task
	}
	first := steps[0]
	task.ExpectedTool = first.ExpectedTool
	task.ExpectedAction = first.ExpectedAction
	task.RequiredParams = slices.Clone(first.RequiredParams)
	task.OptionalParams = slices.Clone(first.OptionalParams)
	task.Destructive = first.Destructive
	task.Simulation = first.Simulation
	return task
}

func cloneEvalCasePtr(evalCase EvalCase) *EvalCase {
	cloned := cloneEvalCases([]EvalCase{evalCase})[0]
	return &cloned
}

func stepsFromCase(evalCase EvalCase) []evalStep {
	steps := make([]evalStep, 0, len(evalCase.Steps))
	for _, step := range evalCase.Steps {
		steps = append(steps, evalStep{
			ExpectedTool:    step.ExpectedTool,
			ExpectedAction:  step.ExpectedAction,
			RequiredParams:  slices.Clone(step.RequiredParams),
			OptionalParams:  slices.Clone(step.OptionalParams),
			ForbiddenParams: slices.Clone(step.ForbiddenParams),
			OptionalStep:    step.OptionalStep,
			Destructive:     step.Destructive,
			Simulation:      step.Simulation,
			AllowedRepairs:  slices.Clone(step.AllowedRepairs),
			ProducedValues:  slices.Clone(step.ProducedValues),
		})
	}
	return steps
}

func casePrompt(evalCase EvalCase) string {
	if evalCase.Prompt != "" {
		return evalCase.Prompt
	}
	return evalCase.PromptTemplate.Text
}

func stepsFromTask(task evalTask) []ExpectedStep {
	steps := taskSteps(task)
	out := make([]ExpectedStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, ExpectedStep{
			ExpectedTool:    step.ExpectedTool,
			ExpectedAction:  step.ExpectedAction,
			RequiredParams:  slices.Clone(step.RequiredParams),
			OptionalParams:  slices.Clone(step.OptionalParams),
			ForbiddenParams: slices.Clone(step.ForbiddenParams),
			OptionalStep:    step.OptionalStep,
			Destructive:     step.Destructive,
			Simulation:      step.Simulation,
			AllowedRepairs:  slices.Clone(step.AllowedRepairs),
			ProducedValues:  slices.Clone(step.ProducedValues),
		})
	}
	return out
}
