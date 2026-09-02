// case_fixture_engine_test.go covers [PrepareCaseAttempt] and the typed fixture
// preparation engine used by evaluation cases that opt into live fixture
// management.

package evaluator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPrepareCaseAttempt_EnsuresValidatesAndRendersOutput verifies that
// PrepareCaseAttempt calls the fixture's Ensure callback once, runs the
// optional Validate hook, and renders the case prompt template using the
// fixture outputs.
//
// The test wraps a tiny project fixture that records its idempotency key
// (asserting it contains the case ID) and returns a known project path. It
// then checks the prepared prompt, fixture outputs, and FixtureHealth slice
// for a single ready health entry. This guards the happy path used by every
// typed evaluation case.
func TestPrepareCaseAttempt_EnsuresValidatesAndRendersOutput(t *testing.T) {
	var ensureCalls int
	evalCase := EvalCase{
		ID:             "MT-FIXTURE-001",
		PromptTemplate: CasePromptTemplate{Text: "Get project {{ .Project.Path }}."},
		Steps:          []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
		Fixtures: []CaseFixtureSpec{{
			Name:    "project",
			Scope:   FixtureScopeAttempt,
			Outputs: []string{"project_path"},
			Ensure: func(_ context.Context, env FixtureContext) (FixtureOutput, error) {
				ensureCalls++
				if !strings.Contains(env.IdempotencyKey, "MT-FIXTURE-001") {
					t.Fatalf("idempotency key = %q, want case ID", env.IdempotencyKey)
				}
				return FixtureOutput{"project_path": "my-org/project"}, nil
			},
			Validate: func(_ context.Context, _ FixtureContext, output FixtureOutput) error {
				if output["project_path"] == "" {
					return errors.New("missing project path")
				}
				return nil
			},
		}},
	}
	prepared, err := PrepareCaseAttempt(context.Background(), FixtureContext{RuntimeEdition: EvalCaseEdition(editionCE), RunSuffix: "run"}, evalCase, "model", 1)
	if err != nil {
		t.Fatalf("PrepareCaseAttempt() error = %v", err)
	}
	if ensureCalls != 1 || prepared.Prompt != "Get project my-org/project." || prepared.FixtureOutputs["project_path"] != "my-org/project" {
		t.Fatalf("prepared = %+v ensureCalls=%d", prepared, ensureCalls)
	}
	if len(prepared.FixtureHealth) != 1 || !prepared.FixtureHealth[0].Ready {
		t.Fatalf("fixture health = %+v, want ready", prepared.FixtureHealth)
	}
}

// TestPrepareCaseAttempt_RetriesUntilSuccess verifies that PrepareCaseAttempt
// honors the fixture's Retries counter and reports every attempt in the
// returned FixtureHealth slice, marking the final attempt ready when the
// fixture eventually succeeds.
//
// The test configures a fixture that fails once before returning a valid
// output. It asserts that the ensure callback ran twice and that the second
// health entry is marked Ready. This protects the evaluator from treating a
// transient GitLab 5xx as a permanent failure.
func TestPrepareCaseAttempt_RetriesUntilSuccess(t *testing.T) {
	var ensureCalls int
	evalCase := EvalCase{ID: "MT-FIXTURE-002", Prompt: "ready", Steps: []ExpectedStep{{ExpectedTool: "gitlab_user"}}, Fixtures: []CaseFixtureSpec{{
		Name:    "flaky",
		Retries: 2,
		Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
			ensureCalls++
			if ensureCalls < 2 {
				return nil, errors.New("not ready")
			}
			return FixtureOutput{"ok": "yes"}, nil
		},
	}}}
	prepared, err := PrepareCaseAttempt(context.Background(), FixtureContext{}, evalCase, "model", 1)
	if err != nil {
		t.Fatalf("PrepareCaseAttempt() error = %v", err)
	}
	if ensureCalls != 2 || len(prepared.FixtureHealth) != 2 || !prepared.FixtureHealth[1].Ready {
		t.Fatalf("ensureCalls=%d health=%+v, want retry then success", ensureCalls, prepared.FixtureHealth)
	}
}

// TestPrepareCaseAttempt_FailsAfterRetryExhaustion verifies that
// PrepareCaseAttempt returns an error mentioning "failed after N attempts"
// when a fixture continues to fail past its Retries budget, and that
// FixtureHealth contains one failed entry per attempt.
//
// The test forces the fixture to always fail with the same error and asserts
// the wrapped message plus the resulting health slice length. This protects
// operators from silently losing fixture state when GitLab outages persist.
func TestPrepareCaseAttempt_FailsAfterRetryExhaustion(t *testing.T) {
	evalCase := EvalCase{ID: "MT-FIXTURE-003", Prompt: "never", Steps: []ExpectedStep{{ExpectedTool: "gitlab_user"}}, Fixtures: []CaseFixtureSpec{{
		Name:    "downstream",
		Retries: 1,
		Ensure:  func(context.Context, FixtureContext) (FixtureOutput, error) { return nil, errors.New("still down") },
	}}}
	prepared, err := PrepareCaseAttempt(context.Background(), FixtureContext{}, evalCase, "model", 1)
	if err == nil || !strings.Contains(err.Error(), "failed after 2 attempt") {
		t.Fatalf("PrepareCaseAttempt() error = %v, want retry exhaustion", err)
	}
	if len(prepared.FixtureHealth) != 2 {
		t.Fatalf("fixture health len = %d, want 2 failed attempts", len(prepared.FixtureHealth))
	}
}

// TestPrepareCaseAttempt_CleansPreparedFixturesWhenLaterFixtureFails
// verifies that PrepareCaseAttempt runs cleanup callbacks for any fixtures
// that succeeded before a later sibling failed, so the live GitLab instance
// is left in a clean state.
//
// The test prepares a successful "prepared" fixture and a failing "failing"
// fixture. It asserts that the failing fixture surfaces a wrapped error, the
// prepared case retains one cleanup function, and that cleanup function was
// invoked with the resource_id emitted by the prepared fixture. This guards
// the operator's GitLab from accumulating orphan resources after aborted
// evaluations.
func TestPrepareCaseAttempt_CleansPreparedFixturesWhenLaterFixtureFails(t *testing.T) {
	var cleaned []string
	evalCase := EvalCase{ID: "MT-FIXTURE-CLEANUP", Prompt: "cleanup", Steps: []ExpectedStep{{ExpectedTool: "gitlab_user"}}, Fixtures: []CaseFixtureSpec{
		{
			Name: "prepared",
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				return FixtureOutput{"resource_id": "created-resource"}, nil
			},
			Cleanup: func(_ context.Context, _ FixtureContext, output FixtureOutput) error {
				cleaned = append(cleaned, output["resource_id"])
				return nil
			},
		},
		{
			Name:   "failing",
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) { return nil, errors.New("boom") },
		},
	}}

	prepared, err := PrepareCaseAttempt(context.Background(), FixtureContext{}, evalCase, "model", 1)

	if err == nil || !strings.Contains(err.Error(), "fixture failing failed") {
		t.Fatalf("PrepareCaseAttempt() error = %v, want failing fixture error", err)
	}
	if len(prepared.Cleanup) != 1 || len(cleaned) != 1 || cleaned[0] != "created-resource" {
		t.Fatalf("prepared cleanup len=%d cleaned=%v, want prepared fixture cleanup", len(prepared.Cleanup), cleaned)
	}
}

// TestPrepareCaseAttempt_FailsValidationAndMissingOutput verifies that
// PrepareCaseAttempt rejects a fixture that omits an output key declared in
// the fixture's Outputs slice, surfacing a "missing output" error before
// the case prompt is rendered.
//
// The test runs a fixture that returns an empty FixtureOutput even though it
// declared project_path as a required output. The assertion guards against
// silently rendering an empty prompt template when live fixture preparation
// under-delivers.
func TestPrepareCaseAttempt_FailsValidationAndMissingOutput(t *testing.T) {
	evalCase := EvalCase{ID: "MT-FIXTURE-004", Prompt: "missing", Steps: []ExpectedStep{{ExpectedTool: "gitlab_user"}}, Fixtures: []CaseFixtureSpec{{
		Name:    "project",
		Outputs: []string{"project_path"},
		Ensure:  func(context.Context, FixtureContext) (FixtureOutput, error) { return FixtureOutput{}, nil },
	}}}
	_, err := PrepareCaseAttempt(context.Background(), FixtureContext{}, evalCase, "model", 1)
	if err == nil || !strings.Contains(err.Error(), "missing output") {
		t.Fatalf("PrepareCaseAttempt() error = %v, want missing output", err)
	}
}

// TestPrepareCaseAttempt_RespectsContextCancellation verifies that
// PrepareCaseAttempt returns a wrapped [context.Canceled] error before
// touching any fixture when the supplied context is already canceled.
//
// The test cancels a derived context immediately and asserts the returned
// error satisfies [errors.Is] for context.Canceled. This protects long
// evaluation runs from leaking GitLab resources when the user aborts the
// CLI.
func TestPrepareCaseAttempt_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evalCase := EvalCase{ID: "MT-FIXTURE-005", Prompt: "canceled", Steps: []ExpectedStep{{ExpectedTool: "gitlab_user"}}, Fixtures: []CaseFixtureSpec{{
		Name:   "project",
		Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) { return FixtureOutput{}, nil },
	}}}
	_, err := PrepareCaseAttempt(ctx, FixtureContext{}, evalCase, "model", 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PrepareCaseAttempt() error = %v, want context.Canceled", err)
	}
}

// TestPrepareTaskAttempt_UsesTypedFixtureEngineForTypedFixtureCases verifies
// that prepareTaskAttempt and prepareTaskAttemptValue delegate to the typed
// fixture engine for cases with declared fixtures and a prompt template,
// producing both a rendered task prompt and a wrapped [PreparedCase].
//
// The test opts into execute+use-fixtures, runs the helper on a tiny
// resource fixture, and asserts the task prompt and the wrapped Prepared
// value both contain the rendered template output. This protects the bridge
// between the runtime orchestrator and the typed fixture engine from
// regressing to the legacy "prompt only" path.
func TestPrepareTaskAttempt_UsesTypedFixtureEngineForTypedFixtureCases(t *testing.T) {
	evalCase := EvalCase{
		ID:             "MT-FIXTURE-006",
		PromptTemplate: CasePromptTemplate{Text: "Inspect {{ .Values.resource_name }}."},
		Steps:          []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
		Fixtures: []CaseFixtureSpec{{
			Name:    "resource",
			Outputs: []string{"resource_name"},
			Ensure: func(context.Context, FixtureContext) (FixtureOutput, error) {
				return FixtureOutput{"resource_name": "typed-fixture"}, nil
			},
		}},
	}
	task := taskFromCase(evalCase)
	prepared, err := prepareTaskAttempt(context.Background(), options{Execute: true, UseFixtures: true, Edition: editionCE}, modelSpec{Model: "model"}, 1, task, evaluationRuntime{}, "run")
	if err != nil {
		t.Fatalf("prepareTaskAttempt() error = %v", err)
	}
	if prepared.Prompt != "Inspect typed-fixture." {
		t.Fatalf("prepared prompt = %q, want typed fixture render", prepared.Prompt)
	}
	attempt, err := prepareTaskAttemptValue(context.Background(), options{Execute: true, UseFixtures: true, Edition: editionCE}, modelSpec{Model: "model"}, 1, task, evaluationRuntime{}, "run")
	if err != nil {
		t.Fatalf("prepareTaskAttemptValue() error = %v", err)
	}
	if attempt.Prepared == nil || attempt.Prepared.Prompt != "Inspect typed-fixture." || attempt.Task.Prompt != "Inspect typed-fixture." {
		t.Fatalf("attempt = %+v, want prepared case and rendered task prompt", attempt)
	}
}

// TestFixtureIdempotencyKey_EncodesScopeAndRuntime verifies the idempotency
// key changes with the fixture scope (bootstrap, run, case, attempt), uses the
// fixture's required runtime over the environment edition, and ends with the
// fixture's own key parts.
func TestFixtureIdempotencyKey_EncodesScopeAndRuntime(t *testing.T) {
	env := FixtureContext{RuntimeEdition: EvalCaseEdition(editionCE), RunSuffix: "abc", ModelName: "m", RunIndex: 2}
	evalCase := EvalCase{ID: "MT-KEY"}
	cases := []struct {
		name    string
		fixture CaseFixtureSpec
		want    string
	}{
		{name: "bootstrap", fixture: CaseFixtureSpec{Name: "project", Scope: FixtureScopeBootstrap}, want: "ce:bootstrap:project"},
		{name: "run", fixture: CaseFixtureSpec{Name: "project", Scope: FixtureScopeRun}, want: "ce:run:abc:project"},
		{name: "case", fixture: CaseFixtureSpec{Name: "project", Scope: FixtureScopeCase, IdempotencyKeyParts: []string{"x"}}, want: "ce:case:MT-KEY:project:x"},
		{name: "attempt", fixture: CaseFixtureSpec{Name: "project", Scope: FixtureScopeAttempt}, want: "ce:attempt:MT-KEY:m:2:abc:project"},
		{name: "required runtime wins", fixture: CaseFixtureSpec{Name: "project", Scope: FixtureScopeBootstrap, RequiredRuntime: EvalCaseEdition(editionEnterprise)}, want: "enterprise:bootstrap:project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fixtureIdempotencyKey(env, evalCase, tc.fixture); got != tc.want {
				t.Fatalf("fixtureIdempotencyKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFirstPositiveInt_PrefersPositiveValues verifies the first positive
// argument wins and the fallback is one.
func TestFirstPositiveInt_PrefersPositiveValues(t *testing.T) {
	cases := []struct {
		name   string
		first  int
		second int
		want   int
	}{
		{name: "first positive", first: 3, second: 2, want: 3},
		{name: "second positive", first: 0, second: 2, want: 2},
		{name: "neither", first: -1, second: 0, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstPositiveInt(tc.first, tc.second); got != tc.want {
				t.Fatalf("firstPositiveInt(%d, %d) = %d, want %d", tc.first, tc.second, got, tc.want)
			}
		})
	}
}

// TestCopyNonEmptyFixtureOutput_SkipsBlankValues verifies blank fixture
// outputs never overwrite values already collected from earlier fixtures.
func TestCopyNonEmptyFixtureOutput_SkipsBlankValues(t *testing.T) {
	dst := FixtureOutput{"project_id": "1"}
	copyNonEmptyFixtureOutput(dst, FixtureOutput{"project_id": " ", "issue_iid": "7"})
	if dst["project_id"] != "1" || dst["issue_iid"] != "7" {
		t.Fatalf("dst = %v, want project_id kept and issue_iid copied", dst)
	}
}

// TestCleanupPreparedFixtures_SkipsNilAndJoinsErrors verifies nil cleanup
// handles are ignored, cleanups run in reverse order, and every error is
// joined into the returned error.
func TestCleanupPreparedFixtures_SkipsNilAndJoinsErrors(t *testing.T) {
	var order []string
	cleanups := []PreparedFixtureCleanup{
		func(context.Context) error { order = append(order, "first"); return errors.New("first failed") },
		nil,
		func(context.Context) error { order = append(order, "second"); return nil },
	}
	err := cleanupPreparedFixtures(t.Context(), cleanups)
	if err == nil || !strings.Contains(err.Error(), "first failed") {
		t.Fatalf("cleanupPreparedFixtures() error = %v, want joined first failure", err)
	}
	if strings.Join(order, ",") != "second,first" {
		t.Fatalf("cleanup order = %v, want reverse order", order)
	}
}

// TestErrWithPreparedFixtureCleanup_WrapsCleanupFailure verifies a cleanup
// failure during error unwinding is appended to the original error.
func TestErrWithPreparedFixtureCleanup_WrapsCleanupFailure(t *testing.T) {
	original := errors.New("ensure failed")
	err := errWithPreparedFixtureCleanup(t.Context(), original, []PreparedFixtureCleanup{
		func(context.Context) error { return errors.New("cleanup failed") },
	})
	if !errors.Is(err, original) || !strings.Contains(err.Error(), "cleanup prepared fixtures: cleanup failed") {
		t.Fatalf("errWithPreparedFixtureCleanup() error = %v, want original plus cleanup failure", err)
	}
	if got := errWithPreparedFixtureCleanup(t.Context(), original, nil); !errors.Is(got, original) || got.Error() != original.Error() {
		t.Fatalf("errWithPreparedFixtureCleanup(no cleanups) = %v, want original", got)
	}
}

// TestCleanupHandle_NilCleanupAndFallbackContext verifies a fixture without a
// cleanup yields no handle, and a handle invoked with a nil context falls back
// to the preparation context.
func TestCleanupHandle_NilCleanupAndFallbackContext(t *testing.T) {
	if handle := cleanupHandle(t.Context(), FixtureContext{}, CaseFixtureSpec{}, nil); handle != nil {
		t.Fatal("cleanupHandle() = non-nil, want nil for fixture without cleanup")
	}
	type ctxKey struct{}
	prepareCtx := context.WithValue(t.Context(), ctxKey{}, "prepare")
	var seen any
	handle := cleanupHandle(prepareCtx, FixtureContext{}, CaseFixtureSpec{Cleanup: func(ctx context.Context, _ FixtureContext, _ FixtureOutput) error {
		seen = ctx.Value(ctxKey{})
		return nil
	}}, nil)
	// A nil context here exercises the documented fallback to the preparation context.
	if err := handle(nil); err != nil { //nolint:staticcheck,nolintlint // deliberate nil context
		t.Fatalf("handle(nil) error = %v", err)
	}
	if seen != "prepare" {
		t.Fatalf("cleanup context value = %v, want prepare context", seen)
	}
}

// TestPrepareCaseFixture_MissingEnsure_ReturnsError verifies a fixture spec
// without an Ensure callback is rejected before any attempt runs.
func TestPrepareCaseFixture_MissingEnsure_ReturnsError(t *testing.T) {
	_, _, _, err := prepareCaseFixture(t.Context(), FixtureContext{}, EvalCase{ID: "MT-X"}, CaseFixtureSpec{Name: "broken"})
	if err == nil || !strings.Contains(err.Error(), "fixture broken has no ensure function") {
		t.Fatalf("prepareCaseFixture() error = %v, want missing ensure error", err)
	}
}

// TestCaseUsesTypedFixtureEngine_DetectsFixturesOrTemplate verifies a task
// routes through the typed engine when its case declares fixtures or a prompt
// template, and not otherwise.
func TestCaseUsesTypedFixtureEngine_DetectsFixturesOrTemplate(t *testing.T) {
	cases := []struct {
		name string
		task evalTask
		want bool
	}{
		{name: "no case", want: false},
		{name: "plain case", task: evalTask{Case: &EvalCase{Prompt: "x"}}, want: false},
		{name: "fixtures", task: evalTask{Case: &EvalCase{Fixtures: []CaseFixtureSpec{{Name: "f"}}}}, want: true},
		{name: "template", task: evalTask{Case: &EvalCase{PromptTemplate: CasePromptTemplate{Text: "t"}}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := caseUsesTypedFixtureEngine(tc.task); got != tc.want {
				t.Fatalf("caseUsesTypedFixtureEngine() = %t, want %t", got, tc.want)
			}
		})
	}
}
