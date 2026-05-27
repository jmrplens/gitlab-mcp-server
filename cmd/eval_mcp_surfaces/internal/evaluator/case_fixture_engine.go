package evaluator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// PreparedCase contains a rendered case and the fixture state created for it.
type PreparedCase struct {
	Case           EvalCase
	Prompt         string
	Steps          []ExpectedStep
	FixtureOutputs FixtureOutput
	FixtureHealth  []FixtureHealth
	Cleanup        []PreparedFixtureCleanup
}

// PreparedFixtureCleanup removes fixture state owned by a prepared case.
type PreparedFixtureCleanup func(context.Context) error

// PrepareCaseAttempt prepares fixtures and renders a typed evaluation case.
func PrepareCaseAttempt(ctx context.Context, env FixtureContext, evalCase EvalCase, model string, runIndex int) (PreparedCase, error) {
	if err := ctx.Err(); err != nil {
		return PreparedCase{}, err
	}
	env.ModelName = firstNonEmpty(model, env.ModelName)
	env.RunIndex = firstPositiveInt(runIndex, env.RunIndex)
	env.CaseID = evalCase.ID
	prepared := PreparedCase{
		Case:           evalCase,
		Steps:          cloneExpectedSteps(evalCase.Steps),
		FixtureOutputs: FixtureOutput{},
	}
	for _, fixture := range evalCase.Fixtures {
		output, health, cleanup, err := prepareCaseFixture(ctx, env, evalCase, fixture)
		prepared.FixtureHealth = append(prepared.FixtureHealth, health...)
		if err != nil {
			return prepared, errWithPreparedFixtureCleanup(ctx, err, prepared.Cleanup)
		}
		copyNonEmptyFixtureOutput(prepared.FixtureOutputs, output)
		if cleanup != nil {
			prepared.Cleanup = append(prepared.Cleanup, cleanup)
		}
	}
	prompt, err := RenderCasePrompt(evalCase, prepared.FixtureOutputs)
	if err != nil {
		return prepared, errWithPreparedFixtureCleanup(ctx, err, prepared.Cleanup)
	}
	prepared.Prompt = prompt
	return prepared, nil
}

func errWithPreparedFixtureCleanup(ctx context.Context, err error, cleanups []PreparedFixtureCleanup) error {
	if cleanupErr := cleanupPreparedFixtures(ctx, cleanups); cleanupErr != nil {
		return fmt.Errorf("%w; cleanup prepared fixtures: %w", err, cleanupErr)
	}
	return err
}

func cleanupPreparedFixtures(ctx context.Context, cleanups []PreparedFixtureCleanup) error {
	ctx = context.WithoutCancel(ctx)
	var cleanupErrs []error
	for _, v := range slices.Backward(cleanups) {
		cleanup := v
		if cleanup == nil {
			continue
		}
		if err := cleanup(ctx); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	return errors.Join(cleanupErrs...)
}

func copyNonEmptyFixtureOutput(dst, src FixtureOutput) {
	for key, value := range src {
		if strings.TrimSpace(value) == "" {
			continue
		}
		dst[key] = value
	}
}

func prepareCaseFixture(ctx context.Context, env FixtureContext, evalCase EvalCase, fixture CaseFixtureSpec) (FixtureOutput, []FixtureHealth, PreparedFixtureCleanup, error) {
	if fixture.Ensure == nil {
		return nil, nil, nil, fmt.Errorf("fixture %s has no ensure function", fixture.Name)
	}
	fixtureEnv := env
	fixtureEnv.FixtureName = fixture.Name
	fixtureEnv.IdempotencyKey = fixtureIdempotencyKey(env, evalCase, fixture)
	health := []FixtureHealth{}
	attempts := max(fixture.Retries+1, 1)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, health, nil, err
		}
		output, err := fixture.Ensure(ctx, fixtureEnv)
		if err == nil {
			err = validateFixtureOutput(fixture, output)
		}
		if err == nil && fixture.Validate != nil {
			err = fixture.Validate(ctx, fixtureEnv, output)
		}
		if err == nil {
			health = append(health, FixtureHealth{Name: fixture.Name, Ready: true, Message: fmt.Sprintf("prepared on attempt %d", attempt), Outputs: output})
			return output, health, cleanupHandle(ctx, fixtureEnv, fixture, output), nil
		}
		lastErr = err
		health = append(health, FixtureHealth{Name: fixture.Name, Ready: false, Message: fmt.Sprintf("attempt %d: %v", attempt, err)})
	}
	return nil, health, nil, fmt.Errorf("fixture %s failed after %d attempt(s): %w", fixture.Name, attempts, lastErr)
}

func validateFixtureOutput(fixture CaseFixtureSpec, output FixtureOutput) error {
	for _, key := range fixture.Outputs {
		if strings.TrimSpace(output[key]) == "" {
			return fmt.Errorf("fixture %s missing output %q", fixture.Name, key)
		}
	}
	return nil
}

func cleanupHandle(ctx context.Context, env FixtureContext, fixture CaseFixtureSpec, output FixtureOutput) PreparedFixtureCleanup {
	if fixture.Cleanup == nil {
		return nil
	}
	return func(cleanupCtx context.Context) error {
		if cleanupCtx == nil {
			cleanupCtx = ctx
		}
		return fixture.Cleanup(cleanupCtx, env, output)
	}
}

func fixtureIdempotencyKey(env FixtureContext, evalCase EvalCase, fixture CaseFixtureSpec) string {
	parts := []string{firstNonEmpty(string(fixture.RequiredRuntime), string(env.RuntimeEdition))}
	switch fixture.Scope {
	case FixtureScopeBootstrap:
		parts = append(parts, "bootstrap")
	case FixtureScopeRun:
		parts = append(parts, "run", env.RunSuffix)
	case FixtureScopeCase:
		parts = append(parts, "case", string(evalCase.ID))
	default:
		parts = append(parts, "attempt", string(evalCase.ID), env.ModelName, strconv.Itoa(env.RunIndex), env.RunSuffix)
	}
	parts = append(parts, fixture.Name)
	parts = append(parts, fixture.IdempotencyKeyParts...)
	return strings.Join(parts, ":")
}

func firstNonEmpty(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

func firstPositiveInt(first, second int) int {
	if first > 0 {
		return first
	}
	if second > 0 {
		return second
	}
	return 1
}

func caseUsesTypedFixtureEngine(task evalTask) bool {
	return task.Case != nil && (len(task.Case.Fixtures) > 0 || task.Case.PromptTemplate.Text != "")
}
