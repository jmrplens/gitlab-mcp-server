package evaluator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

var ProjectServiceAccountFixture = enterpriseLiveCaseFixture("project_service_account", FixtureScopeCase, []string{"project_id", "project_path", "project_service_account_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
	return preparer.ensureProjectServiceAccount(ctx)
}, "project_service_account")

func enterpriseLiveCaseFixture(name string, scope FixtureScope, outputs []string, ensure liveCaseFixtureEnsure, idempotencyKeyParts ...string) CaseFixtureSpec {
	fixture := liveCaseFixture(name, scope, outputs, ensure, idempotencyKeyParts...)
	fixture.RequiredRuntime = EvalCaseEdition(editionEnterprise)
	return fixture
}

func EnterprisePushRuleProjectFixture(seedRule bool) CaseFixtureSpec {
	name := "enterprise_push_rule_project"
	seedPart := "empty"
	if seedRule {
		name = "enterprise_push_rule_project_seeded"
		seedPart = "seeded"
	}
	return CaseFixtureSpec{
		Name:                name,
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             2,
		RequiredRuntime:     EvalCaseEdition(editionEnterprise),
		Outputs:             []string{"project_id", "project_path", "default_branch"},
		IdempotencyKeyParts: []string{"enterprise_push_rule_project", seedPart},
		Ensure: func(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
			return ensureEnterprisePushRuleProjectFixture(ctx, env, seedRule)
		},
		Validate: validateEnterpriseCaseFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
}

func EnterpriseGroupServiceAccountFixture(withPAT bool) CaseFixtureSpec {
	name := "enterprise_group_service_account"
	outputs := []string{"group_id", "group_path", "service_account_id"}
	if withPAT {
		name = "enterprise_group_service_account_pat"
		outputs = append(outputs, "token_id")
	}
	return CaseFixtureSpec{
		Name:                name,
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             2,
		RequiredRuntime:     EvalCaseEdition(editionEnterprise),
		Outputs:             outputs,
		IdempotencyKeyParts: []string{name},
		Ensure: func(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
			return ensureEnterpriseGroupServiceAccountFixture(ctx, env, withPAT)
		},
		Validate: validateEnterpriseCaseFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
}

func ensureEnterprisePushRuleProjectFixture(ctx context.Context, env FixtureContext, seedRule bool) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		if env.Client == nil {
			return nil, errors.New("enterprise push rule fixture requires GitLab client")
		}
		setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		project, err := createLiveTemporaryProject(setupCtx, env.Client, "push-rule")
		if err != nil {
			return nil, fmt.Errorf("prepare Enterprise push rule project: %w", err)
		}
		if !seedRule {
			_, _ = env.Client.GL().Projects.DeleteProjectPushRule(project.PathWithNamespace, gl.WithContext(setupCtx))
		}
		if seedRule {
			commitMessageRegex := ".*"
			if _, _, err = env.Client.GL().Projects.AddProjectPushRule(project.PathWithNamespace, &gl.AddProjectPushRuleOptions{CommitMessageRegex: &commitMessageRegex}, gl.WithContext(setupCtx)); err != nil {
				return nil, fmt.Errorf("prepare Enterprise push rule: %w", err)
			}
		}
		projectID := firstNonEmpty(project.PathWithNamespace, strconv.FormatInt(project.ID, 10))
		return FixtureOutput{
			"project_id":     projectID,
			"project_path":   projectID,
			"default_branch": firstNonEmpty(project.DefaultBranch, liveFixtureDefaultRef),
		}, nil
	})
}

func ensureEnterpriseGroupServiceAccountFixture(ctx context.Context, env FixtureContext, withPAT bool) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		if env.Client == nil {
			return nil, errors.New("enterprise group service account fixture requires GitLab client")
		}
		accountID, tokenID, err := createEnterpriseGroupServiceAccountResource(ctx, env, withPAT)
		if err != nil {
			return nil, err
		}
		output := FixtureOutput{
			"group_id":           liveFixtureGroupPath,
			"group_path":         liveFixtureGroupPath,
			"service_account_id": strconv.FormatInt(accountID, 10),
		}
		if tokenID > 0 {
			output["token_id"] = strconv.FormatInt(tokenID, 10)
		}
		return output, nil
	})
}

func createEnterpriseGroupServiceAccountResource(ctx context.Context, env FixtureContext, withPAT bool) (accountID, tokenID int64, err error) {
	taskID := firstNonEmpty(string(env.CaseID), "enterprise_group_service_account")
	if withPAT {
		return createLiveGroupServiceAccountPAT(ctx, env.Client, taskID)
	}
	accountID, _, err = createLiveGroupServiceAccount(ctx, env.Client, taskID)
	return accountID, 0, err
}

func validateEnterpriseCaseFixtureOutput(_ context.Context, _ FixtureContext, output FixtureOutput) error {
	if output["project_id"] == "" && output["group_id"] == "" && output["service_account_id"] == "" {
		return errors.New("enterprise fixture produced no project, group, or service account identifier")
	}
	return nil
}
