package evaluator

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type liveCaseFixtureEnsure func(context.Context, *liveFixturePreparer) error

var liveFixtureOutputs = newFixtureOutputCache()

var attemptNameFixtureOutputs = []string{
	"attempt_suffix",
	"subgroup_name",
	"subgroup_path",
	"mr_source_branch",
	"mr_title",
	"file_path",
	"milestone_title",
	"release_tag_name",
	"release_name",
	"ci_variable_key",
	"group_ci_variable_key",
	"instance_ci_variable_key",
	"package_release_name",
	"package_release_tag",
	"wiki_title",
	"wiki_title_v2",
	"feature_flag_user_list_name",
	"feature_flag_crud_name",
	"group_label_name",
	"group_label_name_v2",
}

type fixtureOutputCache struct {
	mu     sync.Mutex
	values map[string]FixtureOutput
}

func newFixtureOutputCache() *fixtureOutputCache {
	return &fixtureOutputCache{values: map[string]FixtureOutput{}}
}

func (cache *fixtureOutputCache) ensure(key string, create func() (FixtureOutput, error)) (FixtureOutput, error) {
	if key == "" {
		return create()
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if output, ok := cache.values[key]; ok {
		return maps.Clone(output), nil
	}
	output, err := create()
	if err != nil {
		return nil, err
	}
	cache.values[key] = maps.Clone(output)
	return maps.Clone(output), nil
}

func baseMutatingFixtureSpecs() []CaseFixtureSpec {
	return []CaseFixtureSpec{
		BootstrapProjectFixture,
		AttemptNamesFixture,
		BranchFixture,
		FileFixture,
		IssueFixture,
		MergeRequestFixture,
		ReleaseFixture,
		TagFixture,
		CIVariableFixture,
		HookFixture,
		BadgeFixture,
		WikiFixture,
		SnippetFixture,
		FeatureFlagFixture,
		DeployTokenFixture,
		DeployKeyFixture,
		PackageFixture,
		PackageReleaseFixture,
		PipelineTriggerFixture,
		PipelineScheduleFixture,
		MemberFixture,
	}
}

var (
	AttemptNamesFixture = CaseFixtureSpec{
		Name:                "attempt_names",
		Scope:               FixtureScopeAttempt,
		Timeout:             5 * time.Second,
		Retries:             0,
		Outputs:             attemptNameFixtureOutputs,
		IdempotencyKeyParts: []string{"attempt_names"},
		Ensure: func(_ context.Context, env FixtureContext) (FixtureOutput, error) {
			return attemptNameFixtureOutput(env), nil
		},
		Validate: validateAttemptNameFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
	BranchFixture       = liveCaseFixture("branch", FixtureScopeCase, []string{"project_id", "default_branch", "feature_branch"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureBranches(ctx) }, "branch")
	FileFixture         = liveCaseFixture("file", FixtureScopeCase, []string{"project_id", "feature_branch", "file_path"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureBranches(ctx) }, "file")
	IssueFixture        = liveCaseFixture("issue", FixtureScopeCase, []string{"project_id", "issue_iid"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureCoreIssues(ctx) }, "issue")
	MergeRequestFixture = liveCaseFixture("merge_request", FixtureScopeCase, []string{"project_id", "merge_request_iid", "feature_branch"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureMergeRequests(ctx)
	}, "merge_request")
	MergeRequestDiscussionFixture = liveCaseFixture("merge_request_discussion", FixtureScopeAttempt, []string{"project_id", "project_path", "merge_request_iid", "discussion_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		if err := preparer.ensureMergeRequests(ctx); err != nil {
			return err
		}
		return preparer.ensureDiscussions(ctx)
	}, "merge_request_discussion")
	PipelineJobFixture = liveCaseFixture("pipeline_job", FixtureScopeAttempt, []string{"project_id", "project_path", "pipeline_id", "job_id", "failed_job_id", "manual_job_id", "runner_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePipeline(ctx)
	}, "pipeline_job")
	ReleaseFixture = liveCaseFixture("release", FixtureScopeCase, []string{"project_id", "release_summary_tag"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureCleanupRelease(ctx)
	}, "release")
	TagFixture = liveCaseFixture("tag", FixtureScopeCase, []string{"project_id", "tag_name"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureTag(ctx, liveFixtureElicitationTag, preparer.defaultRef())
	}, "tag")
	CIVariableFixture      = liveCaseFixture("ci_variable", FixtureScopeCase, []string{"project_id", "group_id", "ci_variable_key", "group_ci_variable_key", "instance_ci_variable_key"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureCIVariables(ctx) }, "ci_variable")
	HookFixture            = liveCaseFixture("hook", FixtureScopeCase, []string{"project_id", "hook_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureHooks(ctx) }, "hook")
	BadgeFixture           = liveCaseFixture("badge", FixtureScopeCase, []string{"project_id", "badge_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureBadge(ctx) }, "badge")
	WikiFixture            = liveCaseFixture("wiki", FixtureScopeCase, []string{"project_id", "wiki_slug"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureWiki(ctx) }, "wiki")
	SnippetFixture         = liveCaseFixture("snippet", FixtureScopeCase, []string{"snippet_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureSnippet(ctx) }, "snippet")
	FeatureFlagFixture     = liveCaseFixture("feature_flag", FixtureScopeCase, []string{"project_id", "feature_flag_name"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureFeatureFlag(ctx) }, "feature_flag")
	DeployTokenFixture     = liveCaseFixture("deploy_token", FixtureScopeCase, []string{"project_id", "deploy_token_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureDeployToken(ctx) }, "deploy_token")
	DeployKeyFixture       = liveCaseFixture("deploy_key", FixtureScopeCase, []string{"project_id", "deploy_key_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensureDeployKey(ctx) }, "deploy_key")
	PackageFixture         = liveCaseFixture("package", FixtureScopeCase, []string{"project_id", "package_id", "package_name", "package_file"}, func(ctx context.Context, preparer *liveFixturePreparer) error { return preparer.ensurePackage(ctx) }, "package")
	PackageReleaseFixture  = liveCaseFixture("package_release", FixtureScopeCase, []string{"project_id", "package_release_name", "package_release_version", "package_release_tag", "package_release_dir", "package_release_files"}, ensurePackageReleaseFixture, "package_release")
	PipelineTriggerFixture = liveCaseFixture("pipeline_trigger", FixtureScopeCase, []string{"project_id", "pipeline_trigger_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePipelineTriggers(ctx)
	}, "pipeline_trigger")
	PipelineScheduleFixture = liveCaseFixture("pipeline_schedule", FixtureScopeCase, []string{"project_id", "pipeline_schedule_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensurePipelineSchedules(ctx)
	}, "pipeline_schedule")
	MemberFixture = liveCaseFixture("member", FixtureScopeCase, []string{"project_id", "user_id"}, func(ctx context.Context, preparer *liveFixturePreparer) error {
		return preparer.ensureDisposableUser(ctx)
	}, "member")
	MergeRequestSourceFixture = CaseFixtureSpec{
		Name:                "merge_request_source",
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             2,
		Outputs:             []string{"project_id", "project_path", "default_branch", "mr_source_branch", "mr_title"},
		IdempotencyKeyParts: []string{"merge_request_source"},
		Ensure: func(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
			return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
				preparer, err := newLiveCaseFixturePreparer(ctx, env)
				if err != nil {
					return nil, err
				}
				attemptOutput := attemptNameFixtureOutput(env)
				sourceBranch := attemptOutput["mr_source_branch"]
				if branchErr := preparer.ensureBranch(ctx, sourceBranch, preparer.defaultRef()); branchErr != nil {
					return nil, branchErr
				}
				filePath := "tmp/eval-mr-" + safeFixturePathPart(sourceBranch) + ".txt"
				if fileErr := preparer.ensureFile(ctx, filePath, sourceBranch, "evaluation merge request fixture\n", "Seed evaluation merge request fixture"); fileErr != nil {
					return nil, fileErr
				}
				if closeErr := preparer.closeOpenMergeRequestsForBranch(ctx, sourceBranch); closeErr != nil {
					return nil, closeErr
				}
				output := fixtureOutputFromLiveState(preparer.state)
				maps.Copy(output, attemptOutput)
				return output, nil
			})
		},
		Validate: validateLiveCaseFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
	ReleaseCreateSourceFixture = CaseFixtureSpec{
		Name:                "release_create_source",
		Scope:               FixtureScopeAttempt,
		Timeout:             2 * time.Minute,
		Retries:             2,
		Outputs:             []string{"project_id", "project_path", "default_branch", "release_tag_name", "release_name"},
		IdempotencyKeyParts: []string{"release_create_source"},
		Ensure: func(ctx context.Context, env FixtureContext) (FixtureOutput, error) {
			return liveFixtureOutputs.ensure(env.IdempotencyKey, func() (FixtureOutput, error) {
				preparer, err := newLiveCaseFixturePreparer(ctx, env)
				if err != nil {
					return nil, err
				}
				attemptOutput := attemptNameFixtureOutput(env)
				attemptOutput["release_tag_name"] = suffixEvaluationValue(liveFixtureElicitationTag, attemptOutput["attempt_suffix"])
				attemptOutput["release_name"] = suffixEvaluationValue("Evaluation elicitation release", attemptOutput["attempt_suffix"])
				if tagErr := preparer.ensureTag(ctx, attemptOutput["release_tag_name"], preparer.defaultRef()); tagErr != nil {
					return nil, tagErr
				}
				output := fixtureOutputFromLiveState(preparer.state)
				maps.Copy(output, attemptOutput)
				return output, nil
			})
		},
		Validate: validateLiveCaseFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
)

func attemptNameFixtureOutput(env FixtureContext) FixtureOutput {
	suffix := liveAttemptResourceSuffix(env.ModelName, firstPositiveInt(env.RunIndex, 1), env.RunSuffix)
	if casePart := attemptCaseSuffix(env.CaseID); suffix != "" && casePart != "" {
		suffix += "-" + casePart
	}
	return FixtureOutput{
		"attempt_suffix":           suffix,
		"subgroup_name":            suffixEvaluationValue("eval-temp", suffix),
		"subgroup_path":            suffixEvaluationValue("eval-temp", suffix),
		"mr_source_branch":         suffixEvaluationValue(liveFixtureFeatureRef, suffix),
		"mr_title":                 suffixEvaluationValue("Evaluation MR", suffix),
		"file_path":                suffixEvaluationValue("tmp/eval.txt", suffix),
		"milestone_title":          suffixEvaluationValue("Evaluation Sprint", suffix),
		"release_tag_name":         suffixEvaluationValue("v0.0.0-eval", suffix),
		"release_name":             suffixEvaluationValue("v0.0.0-eval", suffix),
		"release_link_name":        suffixEvaluationValue("eval-crud-link", suffix),
		"release_link_url":         "https://example.com/eval/" + suffix,
		"release_link_updated_url": "https://example.com/eval/updated/" + suffix,
		"ci_variable_key":          suffixEvaluationValue("EVAL_TOKEN", suffix),
		"group_ci_variable_key":    suffixEvaluationValue("GROUP_EVAL_TOKEN", suffix),
		"instance_ci_variable_key": suffixEvaluationValue("INSTANCE_EVAL_TOKEN", suffix),
		"package_release_name":     suffixEvaluationValue(liveFixturePackageReleaseName, suffix),
		"package_release_tag":      suffixEvaluationValue(liveFixturePackageReleaseTag, suffix),
		// Idempotent CRUD names for resources GitLab forces to be unique
		// (wiki title per project, feature flag name per project, group label
		// name per group). The per-attempt suffix prevents "already exists"
		// collisions when a preset is re-run without cleanup.
		"wiki_title":                  suffixEvaluationValue("Evaluation CRUD wiki", suffix),
		"wiki_title_v2":               suffixEvaluationValue("Evaluation CRUD wiki v2", suffix),
		"feature_flag_user_list_name": suffixEvaluationValue("eval-feature-list", suffix),
		"feature_flag_crud_name":      suffixEvaluationValue("eval-feature-flag-crud", suffix),
		"group_label_name":            suffixEvaluationValue("eval-group-label", suffix),
		"group_label_name_v2":         suffixEvaluationValue("eval-group-label-v2", suffix),
	}
}

func attemptCaseSuffix(caseID EvalCaseID) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(string(caseID)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			slug.WriteRune(r)
		}
	}
	return slug.String()
}

func validateAttemptNameFixtureOutput(_ context.Context, _ FixtureContext, output FixtureOutput) error {
	for _, key := range attemptNameFixtureOutputs {
		if strings.TrimSpace(output[key]) == "" {
			return fmt.Errorf("attempt names fixture missing output %q", key)
		}
	}
	return nil
}

func liveCaseFixture(name string, scope FixtureScope, outputs []string, ensure liveCaseFixtureEnsure, idempotencyKeyParts ...string) CaseFixtureSpec {
	return CaseFixtureSpec{
		Name:                name,
		Scope:               scope,
		Timeout:             2 * time.Minute,
		Retries:             2,
		Outputs:             outputs,
		IdempotencyKeyParts: idempotencyKeyParts,
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
				return fixtureOutputFromLiveState(preparer.state), nil
			})
		},
		Validate: validateLiveCaseFixtureOutput,
		Cleanup:  noopCaseFixtureCleanup,
	}
}

func newLiveCaseFixturePreparer(ctx context.Context, env FixtureContext) (*liveFixturePreparer, error) {
	if env.Client == nil {
		return nil, errors.New("typed live fixture requires GitLab client")
	}
	state := &liveFixtureState{
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		GitLabURL:             strings.TrimRight(firstNonEmpty(os.Getenv("GITLAB_URL"), defaultDockerGitLabURL), "/"),
		GroupPath:             liveFixtureGroupPath,
		ToolsGroupPath:        liveFixtureToolsPath,
		ProjectPath:           liveFixtureProjectPath,
		DefaultBranch:         liveFixtureDefaultRef,
		RemoteURL:             fixtureRemoteURL(strings.TrimRight(firstNonEmpty(os.Getenv("GITLAB_URL"), defaultDockerGitLabURL), "/"), liveFixtureProjectPath),
		FeatureFlagName:       liveFixtureFeatureFlag,
		WikiSlug:              liveFixtureWikiSlug,
		CleanupReleaseTag:     liveFixtureCleanupTag,
		ReleaseSummaryTag:     liveFixtureReleaseSummaryTag,
		ElicitationReleaseTag: liveFixtureElicitationTag,
	}
	preparer := &liveFixturePreparer{client: env.Client, state: state}
	topGroup, err := preparer.ensureGroup(ctx, "my-org", liveFixtureGroupPath, 0)
	if err != nil {
		return nil, err
	}
	state.GroupID = topGroup.ID
	toolsGroup, err := preparer.ensureGroup(ctx, "tools", liveFixtureToolsPath, topGroup.ID)
	if err != nil {
		return nil, err
	}
	state.ToolsGroupID = toolsGroup.ID
	project, err := preparer.ensureProject(ctx, toolsGroup.ID)
	if err != nil {
		return nil, err
	}
	state.ProjectID = project.ID
	if project.DefaultBranch != "" {
		state.DefaultBranch = project.DefaultBranch
	}
	return preparer, nil
}

func ensurePackageReleaseFixture(_ context.Context, preparer *liveFixturePreparer) error {
	return ensurePackageReleaseFixtureFiles(preparer.state, defaultFixtures)
}

func validateLiveCaseFixtureOutput(_ context.Context, _ FixtureContext, output FixtureOutput) error {
	if output["project_id"] == "" && output["snippet_id"] == "" {
		return errors.New("typed live fixture produced no project or standalone resource identifier")
	}
	return nil
}

func noopCaseFixtureCleanup(context.Context, FixtureContext, FixtureOutput) error {
	return nil
}

func fixtureOutputFromLiveState(state *liveFixtureState) FixtureOutput {
	if state == nil {
		return FixtureOutput{}
	}
	return FixtureOutput{
		"project_id":                    formatInt64(state.ProjectID),
		"project_path":                  state.ProjectPath,
		"group_path":                    state.GroupPath,
		"tools_group_path":              state.ToolsGroupPath,
		"default_branch":                state.DefaultBranch,
		"remote_url":                    state.RemoteURL,
		"group_id":                      formatInt64(state.GroupID),
		"tools_group_id":                formatInt64(state.ToolsGroupID),
		"feature_branch":                liveFixtureFeatureRef,
		"source_branch":                 liveFixtureFeatureRef,
		"target_branch":                 state.DefaultBranch,
		"obsolete_branch":               liveFixtureObsoleteRef,
		"file_path":                     "tmp/eval.txt",
		"issue_iid":                     formatInt64(state.IssueIID),
		"merge_request_iid":             formatInt64(state.MergeRequestIID),
		"merge_request_thread_id":       state.MergeRequestThreadID,
		"discussion_id":                 state.MergeRequestThreadID,
		"pipeline_id":                   formatInt64(state.PipelineID),
		"pipeline_iid":                  formatInt64(state.PipelineIID),
		"pipeline_ref":                  state.DefaultBranch,
		"job_id":                        formatInt64(state.FailedJobID),
		"failed_job_id":                 formatInt64(state.FailedJobID),
		"manual_job_id":                 formatInt64(state.ManualJobID),
		"runner_id":                     formatInt64(state.RunnerID),
		"tag_name":                      liveFixtureElicitationTag,
		"release_summary_tag":           state.ReleaseSummaryTag,
		"cleanup_release_tag":           state.CleanupReleaseTag,
		"ci_variable_key":               "EVAL_TOKEN",
		"group_ci_variable_key":         "GROUP_EVAL_TOKEN",
		"instance_ci_variable_key":      "INSTANCE_EVAL_TOKEN",
		"hook_id":                       formatInt64(state.HookDeleteID),
		"badge_id":                      formatInt64(state.BadgeDeleteID),
		"wiki_slug":                     state.WikiSlug,
		"snippet_id":                    formatInt64(state.SnippetID),
		"feature_flag_name":             state.FeatureFlagName,
		"deploy_token_id":               formatInt64(state.DeployTokenID),
		"deploy_key_id":                 formatInt64(state.DeployKeyID),
		"package_id":                    formatInt64(state.PackageID),
		"package_name":                  liveFixturePackageName,
		"package_file":                  liveFixturePackageFile,
		"package_release_name":          state.PackageReleaseName,
		"package_release_version":       state.PackageReleaseVersion,
		"package_release_tag":           state.PackageReleaseTag,
		"package_release_dir":           state.PackageReleaseDir,
		"package_release_files":         strings.Join(state.PackageReleaseFiles, ","),
		"package_release_files_display": strings.Join(state.PackageReleaseFiles, ", "),
		"pipeline_trigger_id":           formatInt64(state.PipelineTriggerID),
		"pipeline_schedule_id":          formatInt64(state.PipelineScheduleID),
		"user_id":                       formatInt64(state.UserID),
		"project_service_account_id":    formatInt64(state.ProjectServiceAccountID),
		"project_service_account_pat":   formatInt64(state.ProjectServiceAccountTokenID),
	}
}

func formatInt64(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func requireFixtureNames(fixtures []CaseFixtureSpec) map[string]CaseFixtureSpec {
	out := make(map[string]CaseFixtureSpec, len(fixtures))
	for _, fixture := range fixtures {
		out[fixture.Name] = fixture
	}
	return out
}

func fixtureNames(fixtures []CaseFixtureSpec) string {
	names := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		names = append(names, fixture.Name)
	}
	return fmt.Sprintf("%v", names)
}
