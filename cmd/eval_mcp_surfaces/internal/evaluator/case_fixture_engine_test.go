package evaluator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

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
