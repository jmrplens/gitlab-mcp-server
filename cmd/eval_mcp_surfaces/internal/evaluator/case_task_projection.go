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
	return cloneExpectedSteps(evalCase.Steps)
}

func casePrompt(evalCase EvalCase) string {
	if evalCase.Prompt != "" {
		return evalCase.Prompt
	}
	return evalCase.PromptTemplate.Text
}

func stepsFromTask(task evalTask) []ExpectedStep {
	return cloneExpectedSteps(taskSteps(task))
}
