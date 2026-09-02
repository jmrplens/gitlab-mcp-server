package evaluator

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGradeCaseAssertions_PassesWhenRealOutputContainsEvidence verifies that a
// case-level output_contains assertion passes when every expected substring
// appears (case-insensitively) in the real tool output captured for the target
// step, and that a passing assertion is recorded without clearing FinalSuccess.
func TestGradeCaseAssertions_PassesWhenRealOutputContainsEvidence(t *testing.T) {
	runner := &modelRunner{}
	prepared := PreparedCase{
		Case: EvalCase{
			ID: "MS-ENV-DEP-1",
			Assertions: []CaseAssertion{
				{Type: CaseAssertionOutputContains, Step: 1, Name: "evidence", Inputs: []string{"{{ .Values.deployment_sha }}", "running"}},
			},
		},
		FixtureOutputs: FixtureOutput{"deployment_sha": "abc123def"},
	}
	state := &modelEvaluationState{realStepContent: map[int]string{
		1: `{"environment":{"last_deployment":{"sha":"ABC123DEF","ref":"main","status":"RUNNING"}}}`,
	}}
	result := taskResult{FinalSuccess: true}

	runner.gradeCaseAssertions(prepared, &result, state)

	if len(result.AssertionResults) != 1 {
		t.Fatalf("assertion results = %+v, want exactly one", result.AssertionResults)
	}
	graded := result.AssertionResults[0]
	if graded.Type != CaseAssertionOutputContains || !graded.Passed || graded.Step != 1 {
		t.Fatalf("graded = %+v, want passed output_contains for step 1", graded)
	}
	if !result.FinalSuccess {
		t.Fatalf("FinalSuccess = false, want true on passing assertion")
	}
}

// TestGradeCaseAssertions_FailsAndPenalizesScoreOnMissingEvidence verifies that a
// missing expected substring marks the assertion failed, clears FinalSuccess so
// the failure counts against scoring, and records a diagnostic note naming the
// missing evidence.
func TestGradeCaseAssertions_FailsAndPenalizesScoreOnMissingEvidence(t *testing.T) {
	runner := &modelRunner{}
	prepared := PreparedCase{
		Case: EvalCase{
			ID: "MS-ENV-DEP-2",
			Assertions: []CaseAssertion{
				{Type: CaseAssertionOutputContains, Step: 1, Name: "evidence", Inputs: []string{"main", "running"}},
			},
		},
	}
	state := &modelEvaluationState{realStepContent: map[int]string{
		1: `{"deployment":{"ref":"main","status":"success"}}`,
	}}
	result := taskResult{FinalSuccess: true}

	runner.gradeCaseAssertions(prepared, &result, state)

	if len(result.AssertionResults) != 1 || result.AssertionResults[0].Passed {
		t.Fatalf("assertion results = %+v, want a failed assertion", result.AssertionResults)
	}
	if result.FinalSuccess {
		t.Fatalf("FinalSuccess = true, want false on failing assertion")
	}
	if len(result.Notes) == 0 || !strings.Contains(strings.Join(result.Notes, "; "), "running") {
		t.Fatalf("notes = %v, want a note naming the missing evidence", result.Notes)
	}
}

// TestGradeCaseAssertions_SkipsWhenStepNotExecutedForReal verifies that
// output_contains assertions are skipped entirely (no assertion result, no score
// penalty) when the target step has no captured real content, which is the case
// in mock/simulation runs of docker-backend cases.
func TestGradeCaseAssertions_SkipsWhenStepNotExecutedForReal(t *testing.T) {
	runner := &modelRunner{}
	prepared := PreparedCase{
		Case: EvalCase{
			ID: "MS-ENV-DEP-1",
			Assertions: []CaseAssertion{
				{Type: CaseAssertionOutputContains, Step: 1, Name: "evidence", Inputs: []string{"running"}},
			},
		},
	}
	state := &modelEvaluationState{realStepContent: map[int]string{}}
	result := taskResult{FinalSuccess: true}

	runner.gradeCaseAssertions(prepared, &result, state)

	if len(result.AssertionResults) != 0 {
		t.Fatalf("assertion results = %+v, want none when step lacks real content", result.AssertionResults)
	}
	if !result.FinalSuccess {
		t.Fatalf("FinalSuccess = false, want true when assertion is skipped")
	}
}

// TestRecordRealStepContent_OnlyCapturesRealBackendOutput verifies that step
// output is captured only when a live MCP session is present and the step was
// not simulated, keeping simulated steps out of output_contains grading.
func TestRecordRealStepContent_OnlyCapturesRealBackendOutput(t *testing.T) {
	withSession := &modelRunner{mcpSession: &mcp.ClientSession{}}
	noSession := &modelRunner{}

	realState := &modelEvaluationState{realStepContent: map[int]string{}}
	withSession.recordRealStepContent(evalStep{}, realState, 1, simulationResult{Content: "real-output"})
	if realState.realStepContent[1] != "real-output" {
		t.Fatalf("real content = %q, want captured", realState.realStepContent[1])
	}

	simState := &modelEvaluationState{realStepContent: map[int]string{}}
	withSession.recordRealStepContent(evalStep{Simulation: "transient_error_once"}, simState, 1, simulationResult{Content: "sim-output"})
	if _, ok := simState.realStepContent[1]; ok {
		t.Fatalf("simulated step content captured = %v, want skipped", simState.realStepContent)
	}

	mockState := &modelEvaluationState{realStepContent: map[int]string{}}
	noSession.recordRealStepContent(evalStep{}, mockState, 1, simulationResult{Content: "mock-output"})
	if _, ok := mockState.realStepContent[1]; ok {
		t.Fatalf("mock-mode content captured = %v, want skipped", mockState.realStepContent)
	}
}

// TestGradeCaseAssertions_GradesOnlyRealOutputContainsSteps verifies the
// grader ignores a nil state, non output_contains assertions and steps that
// never ran for real, fails the case when evidence is missing from real
// output, and records a passing result when it is present.
func TestGradeCaseAssertions_GradesOnlyRealOutputContainsSteps(t *testing.T) {
	runner := &modelRunner{}
	prepared := PreparedCase{
		Case: EvalCase{ID: "MT-A", Assertions: []CaseAssertion{
			{Type: CaseAssertionExpectedAction, Step: 1},
			{Type: CaseAssertionOutputContains, Step: 2, Inputs: []string{"never ran"}},
			{Type: CaseAssertionOutputContains, Step: 1, Name: "sha evidence", Inputs: []string{"{{ .Values.deployment_sha }}", " "}},
		}},
		FixtureOutputs: FixtureOutput{"deployment_sha": "abc123"},
	}
	cases := []struct {
		name        string
		state       *modelEvaluationState
		wantResults int
		wantSuccess bool
		wantNote    string
	}{
		{name: "nil state", state: nil, wantResults: 0, wantSuccess: true},
		{name: "evidence present", state: &modelEvaluationState{realStepContent: map[int]string{1: "deployment ABC123 running"}}, wantResults: 1, wantSuccess: true},
		{name: "evidence missing", state: &modelEvaluationState{realStepContent: map[int]string{1: "no sha here"}}, wantResults: 1, wantSuccess: false, wantNote: "step 1 output missing expected evidence: abc123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := taskResult{FinalSuccess: true}
			runner.gradeCaseAssertions(prepared, &result, tc.state)
			if len(result.AssertionResults) != tc.wantResults || result.FinalSuccess != tc.wantSuccess {
				t.Fatalf("result = %+v, want %d assertion results and success %t", result, tc.wantResults, tc.wantSuccess)
			}
			if tc.wantNote != "" && (len(result.Notes) != 1 || result.Notes[0] != tc.wantNote) {
				t.Fatalf("notes = %v, want %q", result.Notes, tc.wantNote)
			}
			if tc.wantResults == 1 && (result.AssertionResults[0].Name != "sha evidence" || result.AssertionResults[0].Passed != tc.wantSuccess) {
				t.Fatalf("assertion result = %+v, want named result with passed=%t", result.AssertionResults[0], tc.wantSuccess)
			}
		})
	}
}

// TestGradeOutputContainsAssertion_DefaultsName verifies an unnamed assertion
// is reported under the generic "output contains" label.
func TestGradeOutputContainsAssertion_DefaultsName(t *testing.T) {
	result := taskResult{FinalSuccess: true}
	gradeOutputContainsAssertion(PreparedCase{}, &result, CaseAssertion{Type: CaseAssertionOutputContains, Step: 3, Inputs: []string{"ok"}}, "all OK")
	if len(result.AssertionResults) != 1 || result.AssertionResults[0].Name != "output contains" || !result.AssertionResults[0].Passed {
		t.Fatalf("assertion results = %+v, want passing default-named result", result.AssertionResults)
	}
}

// TestRenderAssertionInput_ReturnsInputOnTemplateErrors verifies plain text
// passes through, a template that fails to parse or execute is returned as
// written, and a valid template renders against fixture data.
func TestRenderAssertionInput_ReturnsInputOnTemplateErrors(t *testing.T) {
	data := promptTemplateDataMap(promptDataFromOutputs(FixtureOutput{"project_path": "my-org/app"}))
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text", input: "plain", want: "plain"},
		{name: "parse error", input: "{{ .Project.Path", want: "{{ .Project.Path"},
		{name: "missing key", input: "{{ .Nope.Field }}", want: "{{ .Nope.Field }}"},
		{name: "rendered", input: "{{ .Project.Path }}", want: "my-org/app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderAssertionInput("MT-A", tc.input, data); got != tc.want {
				t.Fatalf("renderAssertionInput(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestMissingOutputSubstrings_IgnoresBlanksAndCase verifies blank inputs are
// skipped and matching is case-insensitive.
func TestMissingOutputSubstrings_IgnoresBlanksAndCase(t *testing.T) {
	missing := missingOutputSubstrings("Pipeline SUCCESS on main", []string{"success", " ", "failed"})
	if strings.Join(missing, ",") != "failed" {
		t.Fatalf("missingOutputSubstrings() = %v, want [failed]", missing)
	}
}
