package evaluator

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// gradeCaseAssertions evaluates case-level output_contains assertions against the
// real MCP tool output captured for their target step. It only grades when the
// target step executed for real against a live MCP session (state.realStepContent
// holds the step's genuine output); in simulation mode it skips, because those
// cases are docker-backend cases whose evidence assertions are meaningless
// against synthetic content.
//
// Each graded assertion appends a CaseAssertionResult so it appears in the
// report, and a failing assertion clears FinalSuccess and records a note so the
// failure counts against the case's scoring.
func (r *modelRunner) gradeCaseAssertions(prepared PreparedCase, result *taskResult, state *modelEvaluationState) {
	if state == nil {
		return
	}
	for _, assertion := range prepared.Case.Assertions {
		if assertion.Type != CaseAssertionOutputContains {
			continue
		}
		content, ok := state.realStepContent[assertion.Step]
		if !ok {
			// The step was simulated or never executed for real; skip so we do
			// not penalize mock/simulation runs of docker-backend cases.
			continue
		}
		gradeOutputContainsAssertion(prepared, result, assertion, content)
	}
}

// gradeOutputContainsAssertion checks that every rendered input substring is
// present (case-insensitive) in the step's real tool output content, then
// records the typed assertion result and updates scoring on failure.
func gradeOutputContainsAssertion(prepared PreparedCase, result *taskResult, assertion CaseAssertion, content string) {
	wanted := renderAssertionInputs(prepared, assertion.Inputs)
	missing := missingOutputSubstrings(content, wanted)
	passed := len(missing) == 0
	name := assertion.Name
	if name == "" {
		name = "output contains"
	}
	message := fmt.Sprintf("all expected evidence present in step %d output", assertion.Step)
	if !passed {
		message = fmt.Sprintf("step %d output missing expected evidence: %s", assertion.Step, strings.Join(missing, ", "))
	}
	result.AssertionResults = append(result.AssertionResults, CaseAssertionResult{
		Type:    CaseAssertionOutputContains,
		Step:    assertion.Step,
		Name:    name,
		Passed:  passed,
		Message: message,
	})
	if passed {
		return
	}
	result.FinalSuccess = false
	result.Notes = append(result.Notes, message)
}

// missingOutputSubstrings returns the rendered expected substrings that do not
// appear (case-insensitive) in content.
func missingOutputSubstrings(content string, wanted []string) []string {
	lowerContent := strings.ToLower(content)
	var missing []string
	for _, want := range wanted {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if !strings.Contains(lowerContent, strings.ToLower(want)) {
			missing = append(missing, want)
		}
	}
	return missing
}

// renderAssertionInputs renders each assertion input against the case fixture
// outputs using the same template machinery as the case prompt, so assertions
// can reference dynamic fixture values (for example {{ .Values.deployment_sha }}).
// Inputs without template syntax pass through unchanged.
func renderAssertionInputs(prepared PreparedCase, inputs []string) []string {
	out := make([]string, 0, len(inputs))
	data := promptTemplateDataMap(promptDataFromOutputs(prepared.FixtureOutputs))
	for _, input := range inputs {
		out = append(out, renderAssertionInput(string(prepared.Case.ID), input, data))
	}
	return out
}

func renderAssertionInput(caseID, input string, data map[string]any) string {
	if !strings.Contains(input, "{{") {
		return input
	}
	tmpl, err := template.New(caseID + ":assertion").Option("missingkey=error").Parse(input)
	if err != nil {
		return input
	}
	var rendered bytes.Buffer
	if execErr := tmpl.Execute(&rendered, data); execErr != nil {
		return input
	}
	return rendered.String()
}
