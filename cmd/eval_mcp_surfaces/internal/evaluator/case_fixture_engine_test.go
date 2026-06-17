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
