package evaluator

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var (
	MergeableMergeRequestFixture = CaseFixtureSpec{
		Name:                "mergeable_merge_request",
		Scope:               FixtureScopeAttempt,
		Timeout:             8 * time.Minute,
		Retries:             1,
		Outputs:             []string{"project_id", "project_path", "default_branch", "merge_request_iid"},
		IdempotencyKeyParts: []string{"mergeable_merge_request"},
		Ensure:              ensureMergeableMergeRequestFixture,
		Validate:            validateLiveCaseFixtureOutput,
		Cleanup:             noopCaseFixtureCleanup,
	}
	JobTokenScopeProjectFixture = CaseFixtureSpec{
		Name:                "job_token_scope_project",
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             2,
		Outputs:             []string{"project_id", "project_path", "target_project_id"},
		IdempotencyKeyParts: []string{"job_token_scope_project"},
		Ensure:              ensureJobTokenScopeProjectFixture,
		Validate:            validateLiveCaseFixtureOutput,
		Cleanup:             noopCaseFixtureCleanup,
	}
	FailedJobArtifactFixture = CaseFixtureSpec{
		Name:                "failed_job_artifact",
		Scope:               FixtureScopeAttempt,
		Timeout:             4 * time.Minute,
		Retries:             1,
		Outputs:             []string{"project_id", "project_path", "job_id", "artifact_path"},
		IdempotencyKeyParts: []string{"failed_job_artifact"},
		Ensure:              ensureFailedJobArtifactFixture,
		Validate:            validateLiveCaseFixtureOutput,
		Cleanup:             noopCaseFixtureCleanup,
	}
	MergeRequestAwardEmojiFixture = CaseFixtureSpec{
		Name:                "merge_request_award_emoji",
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             1,
		Outputs:             []string{"project_id", "project_path", "merge_request_iid", "award_id"},
		IdempotencyKeyParts: []string{"merge_request_award_emoji"},
		Ensure:              ensureMergeRequestAwardEmojiFixture,
		Validate:            validateLiveCaseFixtureOutput,
		Cleanup:             noopCaseFixtureCleanup,
	}
	IssueAwardEmojiFixture = CaseFixtureSpec{
		Name:                "issue_award_emoji",
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             1,
		Outputs:             []string{"project_id", "project_path", "issue_iid", "award_id"},
		IdempotencyKeyParts: []string{"issue_award_emoji"},
		Ensure:              ensureIssueAwardEmojiFixture,
		Validate:            validateLiveCaseFixtureOutput,
		Cleanup:             noopCaseFixtureCleanup,
	}
	GroupDeleteFixture = destructiveAttemptFixture("group_delete", []string{"group_id", "group_path"}, nil, func(ctx context.Context, env FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
		attemptOutput := attemptNameFixtureOutput(env)
		groupPath := preparer.state.GroupPath + "/" + attemptOutput["subgroup_path"]
		group, err := preparer.ensureGroup(ctx, attemptOutput["subgroup_name"], groupPath, preparer.state.GroupID)
		if err != nil {
			return nil, err
		}
		output["group_id"] = strconv.FormatInt(group.ID, 10)
		output["group_path"] = group.FullPath
		output["group_name"] = group.Name
		return output, nil
	})
	IssueDeleteFixture = destructiveAttemptFixture("issue_delete", []string{"project_id", "project_path", "issue_iid"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureCoreIssues(ctx)
	}, func(_ context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
		output["issue_iid"] = strconv.FormatInt(preparer.state.IssueDeleteIID, 10)
		output["issue_title"] = "Fixture issue safe to delete"
		return output, nil
	})
	ProjectCIVariableDeleteFixture = destructiveAttemptFixture("project_ci_variable_delete", []string{"project_id", "project_path", "ci_variable_key"}, nil, ensureProjectCIVariableDeleteFixture)
	RepositoryFileDeleteFixture    = destructiveAttemptFixture("repository_file_delete", []string{"project_id", "project_path", "branch_name", "file_path"}, nil, ensureRepositoryFileDeleteFixture)
	MilestoneDeleteFixture         = destructiveAttemptFixture("milestone_delete", []string{"project_id", "project_path", "milestone_iid"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureMilestone(ctx)
	}, func(_ context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
		output["milestone_iid"] = strconv.FormatInt(preparer.state.MilestoneDeleteIID, 10)
		return output, nil
	})
	ReleaseDeleteFixture            = destructiveAttemptFixture("release_delete", []string{"project_id", "project_path", "release_tag_name", "release_name"}, nil, ensureReleaseDeleteFixture)
	ProjectAccessTokenRevokeFixture = destructiveAttemptFixture("project_access_token_revoke", []string{"project_id", "project_path", "token_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureProjectAccessToken(ctx)
	}, func(_ context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
		output["token_id"] = strconv.FormatInt(preparer.state.ProjectTokenID, 10)
		return output, nil
	})
	ProjectArchiveFixture = destructiveAttemptFixture("project_archive", []string{"project_id", "project_path"}, nil, ensureProjectArchiveFixture)
	PackageDeleteFixture  = destructiveAttemptFixture("package_delete", []string{"project_id", "project_path", "package_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePackage(ctx)
	}, nil)
	PipelineDeleteFixture = destructiveAttemptFixture("pipeline_delete", []string{"project_id", "project_path", "pipeline_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePipeline(ctx)
	}, nil)
	PipelineTriggerDeleteFixture = destructiveAttemptFixture("pipeline_trigger_delete", []string{"project_id", "project_path", "pipeline_trigger_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePipelineTriggers(ctx)
	}, nil)
	PipelineScheduleDeleteFixture = destructiveAttemptFixture("pipeline_schedule_delete", []string{"project_id", "project_path", "pipeline_schedule_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePipelineSchedules(ctx)
	}, nil)
	RunnerRemoveFixture = destructiveAttemptFixture("runner_remove", []string{"runner_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureDisposableRunner(ctx)
	}, nil)
	EnvironmentStopFixture          = destructiveAttemptFixture("environment_stop", []string{"project_id", "project_path", "environment_id", "environment_name"}, nil, ensureEnvironmentStopFixture)
	SnippetDeleteFixture            = destructiveAttemptFixture("snippet_delete", []string{"snippet_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureSnippet(ctx) }, nil)
	BroadcastMessageDeleteFixture   = destructiveAttemptFixture("broadcast_message_delete", []string{"id"}, nil, ensureBroadcastMessageDeleteFixture)
	ProjectHookDeleteFixture        = destructiveAttemptFixture("project_hook_delete", []string{"project_id", "project_path", "hook_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureHooks(ctx) }, nil)
	ProjectBadgeDeleteFixture       = destructiveAttemptFixture("project_badge_delete", []string{"project_id", "project_path", "badge_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureBadge(ctx) }, nil)
	DraftNotePublishAllFixture      = destructiveAttemptFixture("draft_note_publish_all", []string{"project_id", "project_path", "merge_request_iid"}, ensureDraftNotePublishAllFixture, nil)
	InstanceCIVariableDeleteFixture = destructiveAttemptFixture("instance_ci_variable_delete", []string{"instance_ci_variable_key"}, nil, ensureInstanceCIVariableDeleteFixture)
	BranchDeleteFixture             = destructiveAttemptFixture("branch_delete", []string{"project_id", "project_path", "branch_name"}, nil, ensureBranchDeleteFixture)
	TagDeleteFixture                = destructiveAttemptFixture("tag_delete", []string{"project_id", "project_path", "tag_name"}, nil, ensureTagDeleteFixture)
	UserBlockFixture                = destructiveAttemptFixture("user_block", []string{"user_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureDisposableUser(ctx)
	}, nil)
	FeatureFlagDeleteFixture          = destructiveAttemptFixture("feature_flag_delete", []string{"project_id", "project_path", "feature_flag_name"}, nil, ensureFeatureFlagDeleteFixture)
	WikiDeleteFixture                 = destructiveAttemptFixture("wiki_delete", []string{"project_id", "project_path", "wiki_slug"}, nil, ensureWikiDeleteFixture)
	DeployKeyLifecycleFixture         = destructiveAttemptFixture("deploy_key_lifecycle", []string{"project_id", "project_path", "deploy_key_title", "deploy_key_updated_title", "deploy_key_key"}, nil, ensureDeployKeyLifecycleFixture)
	DeployKeyDeleteFixture            = destructiveAttemptFixture("deploy_key_delete", []string{"project_id", "project_path", "deploy_key_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureDeployKey(ctx) }, nil)
	DeployTokenDeleteFixture          = destructiveAttemptFixture("deploy_token_delete", []string{"project_id", "project_path", "deploy_token_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureDeployToken(ctx) }, nil)
	CommitDiscussionDeleteNoteFixture = destructiveAttemptFixture("commit_discussion_delete_note", []string{"project_id", "project_path", "commit_sha", "discussion_id", "note_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureCommitDiscussion(ctx)
	}, func(_ context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
		output["commit_sha"] = preparer.state.CommitSHA
		output["discussion_id"] = preparer.state.CommitDiscussionID
		output["note_id"] = strconv.FormatInt(preparer.state.CommitDiscussionNoteID, 10)
		return output, nil
	})
	BranchProtectionLifecycleFixture = CaseFixtureSpec{
		Name:                "branch_protection_lifecycle",
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             1,
		Outputs:             []string{"project_id", "project_path", "default_branch", "branch_name"},
		IdempotencyKeyParts: []string{"branch_protection_lifecycle"},
		Ensure:              ensureBranchProtectionLifecycleFixture,
		Validate:            validateLiveCaseFixtureOutput,
		Cleanup:             noopCaseFixtureCleanup,
	}
)

type destructiveAttemptFixtureMutator func(context.Context, FixtureContext, *liveFixturePreparer, FixtureOutput) (FixtureOutput, error)

func destructiveAttemptFixture(name string, outputs []string, ensure liveCaseFixtureEnsure, mutate destructiveAttemptFixtureMutator) CaseFixtureSpec {
	return CaseFixtureSpec{
		Name:                name,
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             2,
		Outputs:             outputs,
		IdempotencyKeyParts: []string{name},
		Ensure: func(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
			return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
				preparer, err := newLiveCaseFixturePreparer(ctx, env)
				if err != nil {
					return nil, err
				}
				if ensure != nil {
					if ensureErr := ensure(ctx, preparer); ensureErr != nil {
						return nil, ensureErr
					}
				}
				output := fixtureOutputFromLiveState(preparer.state)
				maps.Copy(output, attemptNameFixtureOutput(env))
				if mutate != nil {
					return mutate(ctx, env, preparer, output)
				}
				return output, nil
			})
		},
		Validate: validateLiveCaseFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
}

func ensureProjectCIVariableDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	key := output["ci_variable_key"]
	_, _, err := preparer.client.GL().ProjectVariables.CreateVariable(preparer.state.ProjectID, &gl.CreateProjectVariableOptions{
		Key:              &key,
		Value:            new("masked-value-123"),
		EnvironmentScope: new("production"),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return output, nil
}

func ensureRepositoryFileDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	branchName := suffixEvaluationValue("feature/eval-delete", output["attempt_suffix"])
	filePath := output["file_path"]
	if err := preparer.ensureBranch(ctx, branchName, preparer.defaultRef()); err != nil {
		return nil, err
	}
	if err := preparer.ensureFile(ctx, filePath, branchName, "evaluation file delete fixture\n", "Seed file delete fixture"); err != nil {
		return nil, err
	}
	output["branch_name"] = branchName
	return output, nil
}

func ensureReleaseDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	tag := output["release_tag_name"]
	if err := preparer.ensureTag(ctx, tag, preparer.defaultRef()); err != nil {
		return nil, err
	}
	_, _, err := preparer.client.GL().Releases.CreateRelease(preparer.state.ProjectID, &gl.CreateReleaseOptions{
		Name:        &tag,
		TagName:     &tag,
		Description: new("Fixture release safe to delete."),
	}, gl.WithContext(ctx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusConflict) && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return nil, err
	}
	output["tag_name"] = tag
	output["release_name"] = tag
	return output, nil
}

func ensureProjectArchiveFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	path := "eval-archive-" + liveUniqueSuffix()
	visibility := gl.PrivateVisibility
	initializeWithReadme := true
	project, _, err := preparer.client.GL().Projects.CreateProject(&gl.CreateProjectOptions{
		Name:                 &path,
		Path:                 &path,
		NamespaceID:          &preparer.state.ToolsGroupID,
		InitializeWithReadme: &initializeWithReadme,
		DefaultBranch:        &preparer.state.DefaultBranch,
		Visibility:           &visibility,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	output["project_id"] = strconv.FormatInt(project.ID, 10)
	output["project_path"] = firstNonEmpty(project.PathWithNamespace, path)
	return output, nil
}

func ensureEnvironmentStopFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	name := "env-" + liveUniqueSuffix()
	environment, _, err := preparer.client.GL().Environments.CreateEnvironment(preparer.state.ProjectID, &gl.CreateEnvironmentOptions{
		Name:        &name,
		Description: new("Evaluation environment stop fixture"),
		Tier:        new("production"),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	output["environment_id"] = strconv.FormatInt(environment.ID, 10)
	output["environment_name"] = name
	return output, nil
}

func ensureBroadcastMessageDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	message := suffixEvaluationValue("Evaluation maintenance", output["attempt_suffix"])
	broadcast, _, err := preparer.client.GL().BroadcastMessage.CreateBroadcastMessage(&gl.CreateBroadcastMessageOptions{
		Message: &message,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	output["id"] = strconv.FormatInt(broadcast.ID, 10)
	return output, nil
}

func ensureDraftNotePublishAllFixture(ctx context.Context, preparer *liveFixturePreparer) error {
	if err := preparer.ensureMergeRequests(ctx); err != nil {
		return err
	}
	_, _, err := preparer.client.GL().DraftNotes.CreateDraftNote(preparer.state.ProjectID, preparer.state.MergeRequestIID, &gl.CreateDraftNoteOptions{
		Note: new("Evaluation draft note fixture."),
	}, gl.WithContext(ctx))
	return err
}

func ensureInstanceCIVariableDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	key := output["instance_ci_variable_key"]
	_, _, err := preparer.client.GL().InstanceVariables.CreateVariable(&gl.CreateInstanceVariableOptions{
		Key:   &key,
		Value: new("masked-value-123"),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return output, nil
}

func ensureBranchDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	branchName := suffixEvaluationValue("obsolete/eval", output["attempt_suffix"])
	if err := preparer.ensureBranch(ctx, branchName, preparer.defaultRef()); err != nil {
		return nil, err
	}
	output["branch_name"] = branchName
	return output, nil
}

func ensureTagDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	tagName := output["release_tag_name"]
	if err := preparer.ensureTag(ctx, tagName, preparer.defaultRef()); err != nil {
		return nil, err
	}
	output["tag_name"] = tagName
	return output, nil
}

func ensureFeatureFlagDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	name := strings.ReplaceAll("eval_flag_"+liveUniqueSuffix(), "-", "_")
	_, _, err := preparer.client.GL().ProjectFeatureFlags.CreateProjectFeatureFlag(preparer.state.ProjectID, &gl.CreateProjectFeatureFlagOptions{
		Name: &name,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	output["feature_flag_name"] = name
	return output, nil
}

func ensureWikiDeleteFixture(ctx context.Context, _ FixtureContext, preparer *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	slug := "obsolete-" + liveUniqueSuffix()
	format := gl.WikiFormatValue("markdown")
	_, _, err := preparer.client.GL().Wikis.CreateWikiPage(preparer.state.ProjectID, &gl.CreateWikiPageOptions{
		Title:   &slug,
		Content: new("evaluation wiki delete fixture"),
		Format:  &format,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	output["wiki_slug"] = slug
	return output, nil
}

func ensureDeployKeyLifecycleFixture(_ context.Context, _ FixtureContext, _ *liveFixturePreparer, output FixtureOutput) (FixtureOutput, error) {
	key, err := newAuthorizedSSHKey()
	if err != nil {
		return nil, err
	}
	output["deploy_key_title"] = suffixEvaluationValue("eval-deploy-key", output["attempt_suffix"])
	output["deploy_key_updated_title"] = suffixEvaluationValue("eval-deploy-key-updated", output["attempt_suffix"])
	output["deploy_key_key"] = key
	return output, nil
}

func ensureMergeableMergeRequestFixture(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		if env.Client == nil {
			return nil, errors.New("mergeable merge request fixture requires GitLab client")
		}
		setupCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
		defer cancel()
		project, err := createMergeableMRTemporaryProject(setupCtx, env.Client)
		if err != nil {
			return nil, err
		}
		projectID := project.PathWithNamespace
		if projectID == "" {
			projectID = strconv.FormatInt(project.ID, 10)
		}
		targetBranch := firstNonEmpty(project.DefaultBranch, liveFixtureDefaultRef)
		sourceBranch := "eval-merge-" + safeFixturePathPart(liveAttemptResourceSuffix(env.ModelName, firstPositiveInt(env.RunIndex, 1), env.RunSuffix))
		if branchErr := ensureLiveBranchExists(setupCtx, env.Client, projectID, sourceBranch, targetBranch); branchErr != nil {
			return nil, fmt.Errorf("prepare mergeable MR branch: %w", branchErr)
		}
		if seedErr := seedMergeRequestFixture(setupCtx, env.Client, projectID, sourceBranch); seedErr != nil {
			return nil, seedErr
		}
		removeSource := false
		mergeTitle := "Evaluation merge target"
		mergeRequest, _, err := env.Client.GL().MergeRequests.CreateMergeRequest(projectID, &gl.CreateMergeRequestOptions{
			SourceBranch:       &sourceBranch,
			TargetBranch:       &targetBranch,
			Title:              &mergeTitle,
			RemoveSourceBranch: &removeSource,
		}, gl.WithContext(setupCtx))
		if err != nil {
			return nil, fmt.Errorf("prepare mergeable MR: %w", err)
		}
		if approvalsErr := ensureLiveMergeRequestApprovalless(setupCtx, env.Client, projectID, mergeRequest.IID); approvalsErr != nil {
			return nil, approvalsErr
		}
		if waitErr := waitForLiveMergeRequestReady(setupCtx, env.Client, projectID, mergeRequest.IID); waitErr != nil {
			return nil, waitErr
		}
		return FixtureOutput{
			"project_id":        projectID,
			"project_path":      projectID,
			"default_branch":    targetBranch,
			"source_branch":     sourceBranch,
			"target_branch":     targetBranch,
			"merge_request_iid": strconv.FormatInt(mergeRequest.IID, 10),
		}, nil
	})
}

func ensureLiveMergeRequestApprovalless(ctx context.Context, client *gitlabclient.Client, projectID string, mergeRequestIID int64) error {
	approvalsRequired := int64(0)
	_, _, err := client.GL().MergeRequestApprovals.ChangeApprovalConfiguration(projectID, mergeRequestIID, &gl.ChangeMergeRequestApprovalConfigurationOptions{ //nolint:staticcheck // GitLab EE still relies on this endpoint to clear MR-level approval requirements in fixture projects.
		ApprovalsRequired: &approvalsRequired,
	}, gl.WithContext(ctx))
	if err != nil && !canIgnoreApprovalConfigurationError(err) {
		return fmt.Errorf("prepare mergeable MR approval configuration: %w", err)
	}
	rules, _, err := client.GL().MergeRequestApprovals.GetApprovalRules(projectID, mergeRequestIID, gl.WithContext(ctx))
	if err != nil {
		if canIgnoreApprovalConfigurationError(err) {
			return nil
		}
		return fmt.Errorf("prepare mergeable MR approval rules: %w", err)
	}
	for _, rule := range rules {
		if rule == nil || rule.ID == 0 || rule.ApprovalsRequired == 0 {
			continue
		}
		_, _, updateErr := client.GL().MergeRequestApprovals.UpdateApprovalRule(projectID, mergeRequestIID, rule.ID, &gl.UpdateMergeRequestApprovalRuleOptions{
			ApprovalsRequired: &approvalsRequired,
		}, gl.WithContext(ctx))
		if updateErr != nil && !canIgnoreApprovalConfigurationError(updateErr) {
			return fmt.Errorf("prepare mergeable MR approval rule %d: %w", rule.ID, updateErr)
		}
	}
	return nil
}

func canIgnoreApprovalConfigurationError(err error) bool {
	return toolutil.IsHTTPStatus(err, http.StatusBadRequest) ||
		toolutil.IsHTTPStatus(err, http.StatusForbidden) ||
		toolutil.IsHTTPStatus(err, http.StatusNotFound)
}

func createMergeableMRTemporaryProject(ctx context.Context, client *gitlabclient.Client) (*gl.Project, error) {
	visibility := gl.PrivateVisibility
	approvalsBeforeMerge := int64(0)
	mergePipelinesEnabled := false
	onlyAllowMergeIfAllDiscussionsAreResolved := false
	onlyAllowMergeIfAllStatusChecksPassed := false
	allowMergeOnSkippedPipeline := true
	var lastErr error
	for range 5 {
		path := "eval-merge-mr-" + liveUniqueSuffix()
		initializeWithReadme := true
		project, _, createErr := client.GL().Projects.CreateProject(&gl.CreateProjectOptions{
			Name:                  &path,
			Path:                  &path,
			InitializeWithReadme:  &initializeWithReadme,
			Visibility:            &visibility,
			ApprovalsBeforeMerge:  &approvalsBeforeMerge,
			MergePipelinesEnabled: &mergePipelinesEnabled,
			OnlyAllowMergeIfAllDiscussionsAreResolved: &onlyAllowMergeIfAllDiscussionsAreResolved,
			OnlyAllowMergeIfAllStatusChecksPassed:     &onlyAllowMergeIfAllStatusChecksPassed,
			AllowMergeOnSkippedPipeline:               &allowMergeOnSkippedPipeline,
		}, gl.WithContext(ctx))
		if createErr == nil {
			return project, nil
		}
		lastErr = fmt.Errorf("create standalone project %s: %w", path, createErr)
		if !temporaryProjectNameTaken(createErr) {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func seedMergeRequestFixture(ctx context.Context, client *gitlabclient.Client, projectID, sourceBranch string) error {
	filePath := fmt.Sprintf("tmp/eval-merge-%s.txt", safeFixturePathPart(sourceBranch))
	fileContent := "evaluation merge request fixture\n"
	fileCommitMessage := "Seed merge request evaluation fixture"
	_, _, err := client.GL().RepositoryFiles.CreateFile(projectID, filePath, &gl.CreateFileOptions{
		Branch:        &sourceBranch,
		Content:       &fileContent,
		CommitMessage: &fileCommitMessage,
	}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("prepare mergeable MR file: %w", err)
	}
	return nil
}

func ensureJobTokenScopeProjectFixture(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		preparer, err := newLiveCaseFixturePreparer(ctx, env)
		if err != nil {
			return nil, err
		}
		setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		source, _, err := env.Client.GL().Projects.GetProject(preparer.state.ProjectPath, nil, gl.WithContext(setupCtx))
		if err != nil {
			return nil, fmt.Errorf("prepare job token source project: %w", err)
		}
		target, err := createLiveTemporaryProject(setupCtx, env.Client, "token-scope")
		if err != nil {
			return nil, fmt.Errorf("prepare job token target project: %w", err)
		}
		_, err = env.Client.GL().JobTokenScope.PatchProjectJobTokenAccessSettings(source.ID, &gl.PatchProjectJobTokenAccessSettingsOptions{Enabled: true}, gl.WithContext(setupCtx))
		if err != nil {
			return nil, fmt.Errorf("prepare job token scope settings: %w", err)
		}
		targetProjectID := target.ID
		_, _, err = env.Client.GL().JobTokenScope.AddProjectToJobScopeAllowList(source.ID, &gl.JobTokenInboundAllowOptions{TargetProjectID: &targetProjectID}, gl.WithContext(setupCtx))
		if err != nil && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return nil, fmt.Errorf("prepare job token allowlist project: %w", err)
		}
		if validateErr := validateJobTokenScopeAllowlistTarget(setupCtx, env.Client, source.ID, target.ID); validateErr != nil {
			return nil, validateErr
		}
		output := fixtureOutputFromLiveState(preparer.state)
		output["project_id"] = strconv.FormatInt(source.ID, 10)
		output["project_path"] = firstNonEmpty(source.PathWithNamespace, preparer.state.ProjectPath)
		output["target_project_id"] = strconv.FormatInt(target.ID, 10)
		return output, nil
	})
}

func validateJobTokenScopeAllowlistTarget(ctx context.Context, client *gitlabclient.Client, sourceProjectID, targetProjectID int64) error {
	projects, _, err := client.GL().JobTokenScope.GetProjectJobTokenInboundAllowList(sourceProjectID, &gl.GetJobTokenInboundAllowListOptions{
		ListOptions: gl.ListOptions{PerPage: 100},
	}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("validate job token allowlist project: %w", err)
	}
	for _, project := range projects {
		if project.ID == targetProjectID {
			return nil
		}
	}
	return fmt.Errorf("validate job token allowlist project: target project %d is not in source project %d allowlist", targetProjectID, sourceProjectID)
}

func ensureFailedJobArtifactFixture(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		preparer, err := newLiveCaseFixturePreparer(ctx, env)
		if err != nil {
			return nil, err
		}
		setupCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		defer cancel()
		ref := preparer.defaultRef()
		pipeline, _, err := env.Client.GL().Pipelines.CreatePipeline(preparer.state.ProjectPath, &gl.CreatePipelineOptions{Ref: &ref}, gl.WithContext(setupCtx))
		if err != nil {
			return nil, fmt.Errorf("prepare failed job pipeline: %w", err)
		}
		jobID, err := waitForFailedJob(setupCtx, env.Client, preparer.state.ProjectPath, pipeline.ID)
		if err != nil {
			return nil, err
		}
		output := fixtureOutputFromLiveState(preparer.state)
		output["pipeline_id"] = strconv.FormatInt(pipeline.ID, 10)
		output["job_id"] = strconv.FormatInt(jobID, 10)
		output["failed_job_id"] = strconv.FormatInt(jobID, 10)
		output["artifact_path"] = "coverage/report.xml"
		return output, nil
	})
}

func ensureMergeRequestAwardEmojiFixture(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		preparer, err := newLiveCaseFixturePreparer(ctx, env)
		if err != nil {
			return nil, err
		}
		if ensureErr := preparer.ensureMergeRequests(ctx); ensureErr != nil {
			return nil, ensureErr
		}
		setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		awardID, err := createLiveMRAwardEmoji(setupCtx, env.Client, preparer.state.ProjectPath, preparer.state.MergeRequestIID)
		if err != nil {
			return nil, fmt.Errorf("prepare MR award emoji: %w", err)
		}
		output := fixtureOutputFromLiveState(preparer.state)
		output["award_id"] = strconv.FormatInt(awardID, 10)
		output["award_name"] = "eyes"
		return output, nil
	})
}

func ensureIssueAwardEmojiFixture(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		preparer, err := newLiveCaseFixturePreparer(ctx, env)
		if err != nil {
			return nil, err
		}
		if ensureErr := preparer.ensureCoreIssues(ctx); ensureErr != nil {
			return nil, ensureErr
		}
		setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		awardID, err := createLiveIssueAwardEmoji(setupCtx, env.Client, preparer.state.ProjectPath, preparer.state.IssueIID)
		if err != nil {
			return nil, fmt.Errorf("prepare issue award emoji: %w", err)
		}
		output := fixtureOutputFromLiveState(preparer.state)
		output["award_id"] = strconv.FormatInt(awardID, 10)
		output["award_name"] = "eyes"
		return output, nil
	})
}

func ensureBranchProtectionLifecycleFixture(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
	return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
		preparer, err := newLiveCaseFixturePreparer(ctx, env)
		if err != nil {
			return nil, err
		}
		branchName := suffixEvaluationValue("eval-protect-branch", liveAttemptResourceSuffix(env.ModelName, firstPositiveInt(env.RunIndex, 1), env.RunSuffix))
		setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		_, unprotectErr := env.Client.GL().ProtectedBranches.UnprotectRepositoryBranches(preparer.state.ProjectPath, branchName, gl.WithContext(setupCtx))
		if unprotectErr != nil && !toolutil.IsHTTPStatus(unprotectErr, http.StatusNotFound) {
			return nil, fmt.Errorf("prepare branch protection unprotect cleanup: %w", unprotectErr)
		}
		_, deleteErr := env.Client.GL().Branches.DeleteBranch(preparer.state.ProjectPath, branchName, gl.WithContext(setupCtx))
		if deleteErr != nil && !toolutil.IsHTTPStatus(deleteErr, http.StatusNotFound) {
			return nil, fmt.Errorf("prepare branch protection branch cleanup: %w", deleteErr)
		}
		output := fixtureOutputFromLiveState(preparer.state)
		output["branch_name"] = branchName
		return output, nil
	})
}
