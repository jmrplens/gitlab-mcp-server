package evaluator

import (
	"context"
	"time"
)

// EvalCaseID identifies one model-evaluation case.
type EvalCaseID string

// EvalCaseEdition identifies the GitLab edition a case targets.
type EvalCaseEdition string

// EvalPreset identifies an evaluator preset that may run a case.
type EvalPreset string

// EvalPartition identifies the logical evaluation partition for a case.
type EvalPartition string

// FixtureScope controls how fixture resources are shared across attempts.
type FixtureScope string

// CaseAssertionType identifies the typed validation rule a case assertion uses.
type CaseAssertionType string

const (
	// FixtureScopeBootstrap allows a fixture to be reused across unrelated cases.
	FixtureScopeBootstrap FixtureScope = "bootstrap"
	// FixtureScopeRun creates fixture state for one evaluator run suffix.
	FixtureScopeRun FixtureScope = "run"
	// FixtureScopeCase creates fixture state for one case.
	FixtureScopeCase FixtureScope = "case"
	// FixtureScopeAttempt creates fixture state for one model/run attempt.
	FixtureScopeAttempt FixtureScope = "attempt"
)

const (
	// CaseAssertionExpectedAction verifies the model selected the expected tool action.
	CaseAssertionExpectedAction CaseAssertionType = "expected_action"
	// CaseAssertionRequiredParams verifies all required parameters are present.
	CaseAssertionRequiredParams CaseAssertionType = "required_params"
	// CaseAssertionOptionalParams verifies optional parameters are accepted when present.
	CaseAssertionOptionalParams CaseAssertionType = "optional_params"
	// CaseAssertionForbiddenParams verifies forbidden parameters are not present.
	CaseAssertionForbiddenParams CaseAssertionType = "forbidden_params"
	// CaseAssertionDestructiveConfirm verifies destructive calls include confirmation.
	CaseAssertionDestructiveConfirm CaseAssertionType = "destructive_confirm"
	// CaseAssertionOutputContains verifies tool output contains expected evidence.
	CaseAssertionOutputContains CaseAssertionType = "output_contains"
	// CaseAssertionProducedValue verifies a step produced a value used later.
	CaseAssertionProducedValue CaseAssertionType = "produced_value"
	// CaseAssertionNoExtraToolCall verifies the model did not call extra tools.
	CaseAssertionNoExtraToolCall CaseAssertionType = "no_extra_tool_call"
	// CaseAssertionAllowRepair verifies a known repair path is allowed.
	CaseAssertionAllowRepair CaseAssertionType = "allow_repair"
)

// EvalCase is the typed source of truth for one evaluator task.
type EvalCase struct {
	ID               EvalCaseID
	Title            string
	Prompt           string
	PromptTemplate   CasePromptTemplate
	Steps            []ExpectedStep
	Fixtures         []CaseFixtureSpec
	Assertions       []CaseAssertion
	Metrics          CaseMetricsSpec
	Edition          EvalCaseEdition
	Presets          []EvalPreset
	Partition        EvalPartition
	Tags             []string
	Mutating         bool
	Destructive      bool
	CapabilityBridge bool
	SkipReasons      []string
	ReportGroup      string
}

// ExpectedStep describes one expected MCP tool or action call.
type ExpectedStep struct {
	ExpectedTool    string
	ExpectedAction  string
	RequiredParams  []string
	OptionalParams  []string
	ForbiddenParams []string
	OptionalStep    bool
	Destructive     bool
	Simulation      string
	AllowedRepairs  []string
	ProducedValues  []string
}

// CaseFixtureSpec declares the GitLab state needed by a case.
type CaseFixtureSpec struct {
	Name                string
	Scope               FixtureScope
	Timeout             time.Duration
	Retries             int
	RequiredRuntime     EvalCaseEdition
	Ensure              CaseFixtureEnsureFunc
	Validate            CaseFixtureValidateFunc
	Cleanup             CaseFixtureCleanupFunc
	Outputs             []string
	IdempotencyKeyParts []string
}

// CaseAssertion describes a post-call assertion for a case.
type CaseAssertion struct {
	Type        CaseAssertionType
	Step        int
	Name        string
	Description string
	Required    bool
	Inputs      []string
	Expected    map[string]string
}

// CaseAssertionResult records the outcome of one typed case assertion.
type CaseAssertionResult struct {
	Type    CaseAssertionType
	Step    int
	Name    string
	Passed  bool
	Message string
}

// CaseMetricsSpec customizes how a case contributes to evaluator metrics.
type CaseMetricsSpec struct {
	ExpectedModelCalls int
	ExpectedToolCalls  int
	FinalSuccess       bool
}

// CasePromptTemplate captures prompt text plus fixture variables.
type CasePromptTemplate struct {
	Text      string
	Variables []string
}

// FixtureOutput stores named values emitted by fixture builders.
type FixtureOutput map[string]string

// FixtureHealth records fixture readiness for report and trace diagnostics.
type FixtureHealth struct {
	Name    string
	Ready   bool
	Message string
	Outputs FixtureOutput
}

// CaseFixtureEnsureFunc creates or repairs fixture state.
type CaseFixtureEnsureFunc func(context.Context, FixtureContext) (FixtureOutput, error)

// CaseFixtureValidateFunc verifies fixture state before prompt rendering.
type CaseFixtureValidateFunc func(context.Context, FixtureContext, FixtureOutput) error

// CaseFixtureCleanupFunc removes fixture state owned by a case attempt.
type CaseFixtureCleanupFunc func(context.Context, FixtureContext, FixtureOutput) error
