package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
	"golang.org/x/crypto/ssh"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	liveFixtureGroupPath         = "my-org"
	liveFixtureToolsPath         = "my-org/tools"
	liveFixtureProjectPath       = "my-org/tools/gitlab-mcp-server"
	liveFixtureDefaultRef        = "main"
	liveFixtureFeatureRef        = "feature/eval"
	liveFixtureObsoleteRef       = "obsolete/eval"
	liveFixtureFailureTag        = "v0.0.0-eval"
	liveFixtureCleanupTag        = "v0.0.0-eval-ms"
	liveFixtureElicitationTag    = "v0.0.0-eval-elicit"
	liveFixtureInteractiveMRFile = "interactive/eval-mr.txt"
	liveFixtureFeatureFlag       = "eval_flag"
	liveFixturePackageName       = "eval-package"
	liveFixturePackageVer        = "0.0.1"
	liveFixturePackageFile       = "artifact.txt"
	liveFixtureWikiSlug          = "obsolete-eval"
	liveFixtureReviewBranch      = "feature/eval-review-fixture"
	liveFixtureMergeBranch       = "feature/eval-merge-fixture"
	liveFixtureAwardBranchPrefix = "feature/eval-award-fixture-"
	liveDeleteFixtureFormat      = "delete-fixture-%d"
	taskFileCreateID             = "MT-030"
	taskPipelineScheduleID       = "MT-103"
)

// liveFixtureState holds data for main operations.
type liveFixtureState struct {
	GeneratedAt            string   `json:"generated_at"`
	GitLabURL              string   `json:"gitlab_url"`
	GroupPath              string   `json:"group_path"`
	GroupID                int64    `json:"group_id"`
	ToolsGroupPath         string   `json:"tools_group_path"`
	ToolsGroupID           int64    `json:"tools_group_id"`
	ProjectPath            string   `json:"project_path"`
	ProjectID              int64    `json:"project_id"`
	DefaultBranch          string   `json:"default_branch"`
	RemoteURL              string   `json:"remote_url"`
	IssueIID               int64    `json:"issue_iid"`
	IssueDeleteIID         int64    `json:"issue_delete_iid"`
	MergeRequestIID        int64    `json:"merge_request_iid"`
	MergeRequestMergeIID   int64    `json:"merge_request_merge_iid"`
	MergeRequestAwardIID   int64    `json:"merge_request_award_iid,omitempty"`
	MergeRequestThreadID   string   `json:"merge_request_thread_id,omitempty"`
	PipelineID             int64    `json:"pipeline_id"`
	PipelineIID            int64    `json:"pipeline_iid"`
	FailedJobID            int64    `json:"failed_job_id"`
	ManualJobID            int64    `json:"manual_job_id"`
	RunnerID               int64    `json:"runner_id"`
	MilestoneDeleteIID     int64    `json:"milestone_delete_iid"`
	HookDeleteID           int64    `json:"hook_delete_id"`
	BadgeDeleteID          int64    `json:"badge_delete_id"`
	SnippetID              int64    `json:"snippet_id"`
	EnvironmentID          int64    `json:"environment_id"`
	ProjectTokenID         int64    `json:"project_token_id"`
	PackageID              int64    `json:"package_id"`
	DeployKeyID            int64    `json:"deploy_key_id"`
	DeployKeyCreateKey     string   `json:"deploy_key_create_key,omitempty"`
	DeployTokenID          int64    `json:"deploy_token_id"`
	PipelineTriggerID      int64    `json:"pipeline_trigger_id"`
	PipelineTriggerRunID   int64    `json:"pipeline_trigger_run_id"`
	PipelineScheduleID     int64    `json:"pipeline_schedule_id"`
	PipelineSchedulePlayID int64    `json:"pipeline_schedule_play_id"`
	UserID                 int64    `json:"user_id"`
	IssueAwardID           int64    `json:"issue_award_id"`
	MergeRequestAwardID    int64    `json:"merge_request_award_id"`
	CommitSHA              string   `json:"commit_sha,omitempty"`
	CommitDiscussionID     string   `json:"commit_discussion_id,omitempty"`
	CommitDiscussionNoteID int64    `json:"commit_discussion_note_id,omitempty"`
	FeatureFlagName        string   `json:"feature_flag_name"`
	WikiSlug               string   `json:"wiki_slug"`
	CleanupReleaseTag      string   `json:"cleanup_release_tag"`
	ReleaseSummaryTag      string   `json:"release_summary_tag,omitempty"`
	ElicitationReleaseTag  string   `json:"elicitation_release_tag,omitempty"`
	Notes                  []string `json:"notes,omitempty"`
}

// liveFixturePreparer holds data for main operations.
type liveFixturePreparer struct {
	client *gitlabclient.Client
	state  *liveFixtureState
}

// prepareLiveFixtures performs the prepare live fixtures operation using the GitLab API and returns [*liveFixtureState].
func prepareLiveFixtures(opts options) (*liveFixtureState, error) {
	if err := validateFixtureOptions(opts); err != nil {
		return nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load GitLab config: %w", err)
	}
	client, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	deployKeyCreateKey, err := newAuthorizedSSHKey()
	if err != nil {
		return nil, fmt.Errorf("create deploy key fixture public key: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	state := &liveFixtureState{
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		GitLabURL:             cfg.GitLabURL,
		GroupPath:             liveFixtureGroupPath,
		ToolsGroupPath:        liveFixtureToolsPath,
		ProjectPath:           liveFixtureProjectPath,
		DefaultBranch:         liveFixtureDefaultRef,
		RemoteURL:             fixtureRemoteURL(cfg.GitLabURL, liveFixtureProjectPath),
		DeployKeyCreateKey:    deployKeyCreateKey,
		FeatureFlagName:       liveFixtureFeatureFlag,
		WikiSlug:              liveFixtureWikiSlug,
		CleanupReleaseTag:     liveFixtureCleanupTag,
		ReleaseSummaryTag:     liveFixtureCleanupTag,
		ElicitationReleaseTag: liveFixtureElicitationTag,
	}
	preparer := &liveFixturePreparer{client: client, state: state}
	if prepareErr := preparer.prepare(ctx); prepareErr != nil {
		return nil, prepareErr
	}
	return state, nil
}

// validateFixtureOptions is an internal helper for the main package.
func validateFixtureOptions(opts options) error {
	if opts.ToolsFile != "" {
		return errors.New("--prepare-fixtures requires a live catalog, not --tools-file")
	}
	if normalizedBackend(opts.Backend) != backendGitLab {
		return errors.New("--prepare-fixtures requires --backend=gitlab")
	}
	if !opts.AllowLive && !strings.EqualFold(os.Getenv("E2E_MODE"), "docker") {
		return errors.New("--prepare-fixtures requires E2E_MODE=docker unless --allow-live-mutations is set")
	}
	return nil
}

// writeLiveFixtures is an internal helper for the main package.
func writeLiveFixtures(path string, state *liveFixtureState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create fixture state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fixture state: %w", err)
	}
	data = append(data, '\n')
	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("write fixture state %s: %w", path, writeErr)
	}
	return nil
}

// readLiveFixtures performs the read live fixtures operation using the GitLab API and returns [*liveFixtureState].
func readLiveFixtures(path string) (*liveFixtureState, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- fixture state path is an explicit evaluator input.
	if err != nil {
		return nil, fmt.Errorf("read fixture state %s: %w", path, err)
	}
	var state liveFixtureState
	if parseErr := json.Unmarshal(data, &state); parseErr != nil {
		return nil, fmt.Errorf("parse fixture state %s: %w", path, parseErr)
	}
	if state.ProjectPath == "" || state.ProjectID == 0 {
		return nil, fmt.Errorf("fixture state %s is missing project identity", path)
	}
	if state.CleanupReleaseTag == "" {
		state.CleanupReleaseTag = liveFixtureCleanupTag
	}
	if state.ReleaseSummaryTag == "" {
		state.ReleaseSummaryTag = liveFixtureCleanupTag
	}
	if state.ElicitationReleaseTag == "" {
		state.ElicitationReleaseTag = liveFixtureElicitationTag
	}
	return &state, nil
}

// prepare performs the prepare operation on *liveFixturePreparer.
func (p *liveFixturePreparer) prepare(ctx context.Context) error {
	topGroup, err := p.ensureGroup(ctx, "my-org", liveFixtureGroupPath, 0)
	if err != nil {
		return err
	}
	p.state.GroupID = topGroup.ID
	toolsGroup, err := p.ensureGroup(ctx, "tools", liveFixtureToolsPath, topGroup.ID)
	if err != nil {
		return err
	}
	p.state.ToolsGroupID = toolsGroup.ID
	project, err := p.ensureProject(ctx, toolsGroup.ID)
	if err != nil {
		return err
	}
	p.state.ProjectID = project.ID
	if project.DefaultBranch != "" {
		p.state.DefaultBranch = project.DefaultBranch
	}
	p.bestEffort(ctx, "CI variables", p.ensureCIVariables)

	if ensureErr := p.ensureRepository(ctx); ensureErr != nil {
		return ensureErr
	}
	if ensureErr := p.ensureLabels(ctx); ensureErr != nil {
		return ensureErr
	}
	if ensureErr := p.ensureBranches(ctx); ensureErr != nil {
		return ensureErr
	}
	if ensureErr := p.ensureInteractiveResources(ctx); ensureErr != nil {
		return ensureErr
	}
	if ensureErr := p.ensureCoreIssues(ctx); ensureErr != nil {
		return ensureErr
	}
	if ensureErr := p.ensureMergeRequests(ctx); ensureErr != nil {
		return ensureErr
	}
	if ensureErr := p.ensurePipeline(ctx); ensureErr != nil {
		p.notef("pipeline fixture unavailable: %v", ensureErr)
	}
	p.bestEffort(ctx, "milestone", p.ensureMilestone)
	p.bestEffort(ctx, "cleanup release", p.ensureCleanupRelease)
	p.bestEffort(ctx, "hooks", p.ensureHooks)
	p.bestEffort(ctx, "badges", p.ensureBadge)
	p.bestEffort(ctx, "snippet", p.ensureSnippet)
	p.bestEffort(ctx, "environment", p.ensureEnvironment)
	p.bestEffort(ctx, "project access token", p.ensureProjectAccessToken)
	p.bestEffort(ctx, "package", p.ensurePackage)
	p.bestEffort(ctx, "deploy key", p.ensureDeployKey)
	p.bestEffort(ctx, "deploy token", p.ensureDeployToken)
	p.bestEffort(ctx, "pipeline triggers", p.ensurePipelineTriggers)
	p.bestEffort(ctx, "pipeline schedules", p.ensurePipelineSchedules)
	p.bestEffort(ctx, "test runner", p.ensureDisposableRunner)
	p.bestEffort(ctx, "admin user", p.ensureDisposableUser)
	p.bestEffort(ctx, "feature flag", p.ensureFeatureFlag)
	p.bestEffort(ctx, "wiki", p.ensureWiki)
	p.bestEffort(ctx, "award emojis", p.ensureAwardEmoji)
	p.bestEffort(ctx, "discussions", p.ensureDiscussions)
	p.bestEffort(ctx, "commit discussion", p.ensureCommitDiscussion)
	return nil
}

// bestEffort performs the best effort operation on *liveFixturePreparer.
func (p *liveFixturePreparer) bestEffort(ctx context.Context, name string, fn func(context.Context) error) {
	if err := fn(ctx); err != nil {
		p.notef("%s fixture unavailable: %v", name, err)
	}
}

// notef performs the notef operation on *liveFixturePreparer.
func (p *liveFixturePreparer) notef(format string, args ...any) {
	p.state.Notes = append(p.state.Notes, fmt.Sprintf(format, args...))
}

// defaultRef returns the detected project default branch or the fixture fallback.
func (p *liveFixturePreparer) defaultRef() string {
	if p != nil && p.state != nil && p.state.DefaultBranch != "" {
		return p.state.DefaultBranch
	}
	return liveFixtureDefaultRef
}

// ensureGroup performs the ensure group operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureGroup(ctx context.Context, name, fullPath string, parentID int64) (*gl.Group, error) {
	group, _, err := p.client.GL().Groups.GetGroup(fullPath, nil, gl.WithContext(ctx))
	if err == nil {
		return group, nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return nil, fmt.Errorf("get group %s: %w", fullPath, err)
	}
	visibility := gl.PrivateVisibility
	opts := &gl.CreateGroupOptions{
		Name:       new(name),
		Path:       new(pathBase(fullPath)),
		Visibility: &visibility,
	}
	if parentID > 0 {
		opts.ParentID = new(parentID)
	}
	group, _, err = p.client.GL().Groups.CreateGroup(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create group %s: %w", fullPath, err)
	}
	return group, nil
}

// ensureProject performs the ensure project operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureProject(ctx context.Context, namespaceID int64) (*gl.Project, error) {
	project, _, err := p.client.GL().Projects.GetProject(liveFixtureProjectPath, nil, gl.WithContext(ctx))
	if err == nil {
		if project.Archived {
			unarchived, _, unarchiveErr := p.client.GL().Projects.UnarchiveProject(project.ID, gl.WithContext(ctx))
			if unarchiveErr != nil {
				return nil, fmt.Errorf("unarchive project %s: %w", liveFixtureProjectPath, unarchiveErr)
			}
			project = unarchived
		}
		return project, nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return nil, fmt.Errorf("get project %s: %w", liveFixtureProjectPath, err)
	}
	visibility := gl.PrivateVisibility
	project, _, err = p.client.GL().Projects.CreateProject(&gl.CreateProjectOptions{
		Name:                 new("gitlab-mcp-server"),
		Path:                 new("gitlab-mcp-server"),
		NamespaceID:          new(namespaceID),
		InitializeWithReadme: new(true),
		DefaultBranch:        new(liveFixtureDefaultRef),
		Visibility:           &visibility,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create project %s: %w", liveFixtureProjectPath, err)
	}
	return project, nil
}

// ensureRepository performs the ensure repository operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureRepository(ctx context.Context) error {
	defaultRef := p.defaultRef()
	if err := p.waitForBranch(ctx, defaultRef); err != nil {
		return err
	}
	if err := p.ensureFile(ctx, "README.md", defaultRef, fixtureReadme(), "Seed evaluation README"); err != nil {
		return err
	}
	return p.ensureFile(ctx, ".gitlab-ci.yml", defaultRef, fixtureCI(), "Seed evaluation CI")
}

// ensureLabels performs the ensure labels operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureLabels(ctx context.Context) error {
	labels := []struct {
		name  string
		color string
	}{
		{name: "evaluation", color: "#1f75cb"},
		{name: "bug", color: "#d73a4a"},
	}
	for _, label := range labels {
		_, _, err := p.client.GL().Labels.GetLabel(p.state.ProjectID, label.name, gl.WithContext(ctx))
		if err == nil {
			continue
		}
		if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return fmt.Errorf("get label %s: %w", label.name, err)
		}
		_, _, err = p.client.GL().Labels.CreateLabel(p.state.ProjectID, &gl.CreateLabelOptions{
			Name:  new(label.name),
			Color: new(label.color),
		}, gl.WithContext(ctx))
		if err != nil && !toolutil.IsHTTPStatus(err, http.StatusConflict) && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return fmt.Errorf("create label %s: %w", label.name, err)
		}
	}
	return nil
}

// ensureBranches performs the ensure branches operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureBranches(ctx context.Context) error {
	defaultRef := p.defaultRef()
	if err := p.ensureBranch(ctx, liveFixtureFeatureRef, defaultRef); err != nil {
		return err
	}
	if err := p.ensureBranch(ctx, liveFixtureObsoleteRef, defaultRef); err != nil {
		return err
	}
	if err := p.ensureFile(ctx, "feature/eval.txt", liveFixtureFeatureRef, "feature fixture\n", "Seed feature evaluation file"); err != nil {
		return err
	}
	if err := p.ensureFile(ctx, "tmp/eval.txt", liveFixtureFeatureRef, "temporary evaluation fixture\n", "Seed temporary evaluation file"); err != nil {
		return err
	}
	return p.closeOpenMergeRequestsForBranch(ctx, liveFixtureFeatureRef)
}

// ensureInteractiveResources seeds resources used by MCP elicitation evaluation flows.
func (p *liveFixturePreparer) ensureInteractiveResources(ctx context.Context) error {
	defaultRef := p.defaultRef()
	if err := p.ensureFile(ctx, liveFixtureInteractiveMRFile, liveFixtureFeatureRef, "interactive merge request fixture\n", "Seed interactive merge request evaluation file"); err != nil {
		return err
	}
	if err := p.ensureTag(ctx, liveFixtureElicitationTag, defaultRef); err != nil {
		return err
	}
	if p.state.ElicitationReleaseTag == "" {
		p.state.ElicitationReleaseTag = liveFixtureElicitationTag
	}
	return nil
}

// ensureCoreIssues performs the ensure core issues operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureCoreIssues(ctx context.Context) error {
	issue, err := p.createIssue(ctx, "Fixture issue for evaluation reads", "Used by read, update, close, note, emoji, and analyzer cases.", []string{"evaluation"})
	if err != nil {
		return err
	}
	p.state.IssueIID = issue.IID
	deleteIssue, err := p.createIssue(ctx, "Fixture issue safe to delete", "Used only by destructive delete evaluation cases.", []string{"evaluation"})
	if err != nil {
		return err
	}
	p.state.IssueDeleteIID = deleteIssue.IID
	return nil
}

// ensureMergeRequests performs the ensure merge requests operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureMergeRequests(ctx context.Context) error {
	mr, err := p.ensureFixtureMergeRequest(ctx, liveFixtureReviewBranch, "Evaluation review fixture MR", false)
	if err != nil {
		return err
	}
	p.state.MergeRequestIID = mr.IID
	mergeMR, err := p.ensureFixtureMergeRequest(ctx, liveFixtureMergeBranch, "Evaluation merge fixture MR", true)
	if err != nil {
		return err
	}
	if mergeableErr := p.waitForMergeRequestMergeable(ctx, mergeMR.IID); mergeableErr != nil {
		p.notef("merge fixture not mergeable: %v", mergeableErr)
	}
	p.state.MergeRequestMergeIID = mergeMR.IID
	awardMR, err := p.ensureFixtureMergeRequest(ctx, liveFixtureAwardBranchPrefix+"stable", "Evaluation time and award fixture MR", false)
	if err != nil {
		return err
	}
	p.state.MergeRequestAwardIID = awardMR.IID
	return nil
}

// ensurePipeline performs the ensure pipeline operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensurePipeline(ctx context.Context) error {
	pipeline, _, err := p.client.GL().Pipelines.CreatePipeline(p.state.ProjectID, &gl.CreatePipelineOptions{
		Ref: new(p.defaultRef()),
	}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}
	p.state.PipelineID = pipeline.ID
	p.state.PipelineIID = pipeline.IID
	return p.waitForPipelineJobs(ctx, pipeline.ID)
}

// ensureMilestone performs the ensure milestone operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureMilestone(ctx context.Context) error {
	m, _, err := p.client.GL().Milestones.CreateMilestone(p.state.ProjectID, &gl.CreateMilestoneOptions{
		Title:       new(fmt.Sprintf("Evaluation Sprint Delete %d", time.Now().Unix())),
		Description: new("Fixture milestone safe to delete."),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.MilestoneDeleteIID = m.IID
	return nil
}

// ensureCleanupRelease performs the ensure cleanup release operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureCleanupRelease(ctx context.Context) error {
	if err := p.ensureTag(ctx, liveFixtureCleanupTag, p.defaultRef()); err != nil {
		return err
	}
	_, _, err := p.client.GL().Releases.GetRelease(p.state.ProjectID, liveFixtureCleanupTag, gl.WithContext(ctx))
	if err != nil {
		if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return err
		}
		_, _, err = p.client.GL().Releases.CreateRelease(p.state.ProjectID, &gl.CreateReleaseOptions{
			Name:        new("Evaluation cleanup release"),
			TagName:     new(liveFixtureCleanupTag),
			Description: new("Fixture release for cleanup workflow."),
		}, gl.WithContext(ctx))
		if err != nil {
			return err
		}
	}
	_, _, err = p.client.GL().ReleaseLinks.CreateReleaseLink(p.state.ProjectID, liveFixtureCleanupTag, &gl.CreateReleaseLinkOptions{
		Name: new(fmt.Sprintf("docs-%d", time.Now().UnixNano())),
		URL:  new("https://example.com/eval-release-notes"),
	}, gl.WithContext(ctx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return err
	}
	return nil
}

// ensureHooks performs the ensure hooks operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureHooks(ctx context.Context) error {
	if err := p.cleanupProjectHooks(ctx); err != nil {
		return err
	}
	hook, _, err := p.client.GL().Projects.AddProjectHook(p.state.ProjectID, &gl.AddProjectHookOptions{
		Name:                  new(fmt.Sprintf(liveDeleteFixtureFormat, time.Now().UnixNano())),
		URL:                   new("https://example.com/gitlab-hook-delete"),
		PushEvents:            new(true),
		EnableSSLVerification: new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.HookDeleteID = hook.ID
	return nil
}

// cleanupProjectHooks performs the cleanup project hooks operation on *liveFixturePreparer.
func (p *liveFixturePreparer) cleanupProjectHooks(ctx context.Context) error {
	for range 3 {
		deleted, err := p.deleteEvaluationProjectHooks(ctx)
		if err != nil {
			return err
		}
		if deleted == 0 {
			return nil
		}
	}
	return nil
}

// deleteEvaluationProjectHooks performs the delete evaluation project hooks operation on *liveFixturePreparer.
func (p *liveFixturePreparer) deleteEvaluationProjectHooks(ctx context.Context) (int, error) {
	deleted := 0
	for page := int64(1); ; {
		hooks, resp, err := p.client.GL().Projects.ListProjectHooks(p.state.ProjectID, &gl.ListProjectHooksOptions{
			ListOptions: gl.ListOptions{Page: page, PerPage: 100},
		}, gl.WithContext(ctx))
		if err != nil {
			return deleted, err
		}
		for _, hook := range hooks {
			if !isEvaluationProjectHook(hook) {
				continue
			}
			_, deleteErr := p.client.GL().Projects.DeleteProjectHook(p.state.ProjectID, hook.ID, gl.WithContext(ctx))
			if deleteErr != nil && !toolutil.IsHTTPStatus(deleteErr, http.StatusNotFound) {
				return deleted, deleteErr
			}
			deleted++
		}
		if resp == nil || resp.NextPage == 0 {
			return deleted, nil
		}
		page = resp.NextPage
	}
}

// isEvaluationProjectHook is an internal helper for the main package.
func isEvaluationProjectHook(hook *gl.ProjectHook) bool {
	if hook == nil {
		return false
	}
	name := strings.ToLower(hook.Name)
	url := strings.ToLower(hook.URL)
	return strings.HasPrefix(name, "delete-fixture-") ||
		strings.Contains(name, "ms-021") ||
		strings.Contains(name, "eval-crud-hook") ||
		strings.Contains(url, "example.com/gitlab-hook") ||
		strings.Contains(url, "example.com/eval-crud-hook")
}

// ensureBadge performs the ensure badge operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureBadge(ctx context.Context) error {
	badge, _, err := p.client.GL().ProjectBadges.AddProjectBadge(p.state.ProjectID, &gl.AddProjectBadgeOptions{
		LinkURL:  new("https://example.com/coverage"),
		ImageURL: new("https://example.com/badge.svg"),
		Name:     new(fmt.Sprintf(liveDeleteFixtureFormat, time.Now().UnixNano())),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.BadgeDeleteID = badge.ID
	return nil
}

// ensureSnippet performs the ensure snippet operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureSnippet(ctx context.Context) error {
	visibility := gl.PrivateVisibility
	snippet, _, err := p.client.GL().Snippets.CreateSnippet(&gl.CreateSnippetOptions{
		Title:      new(fmt.Sprintf("Evaluation snippet %d", time.Now().UnixNano())),
		FileName:   new("eval.txt"),
		Content:    new("evaluation snippet content\n"),
		Visibility: &visibility,
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.SnippetID = snippet.ID
	return nil
}

// ensureEnvironment performs the ensure environment operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureEnvironment(ctx context.Context) error {
	environments, _, err := p.client.GL().Environments.ListEnvironments(p.state.ProjectID, &gl.ListEnvironmentsOptions{
		Name: new("production"),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	if len(environments) > 0 {
		p.state.EnvironmentID = environments[0].ID
		return nil
	}
	env, _, err := p.client.GL().Environments.CreateEnvironment(p.state.ProjectID, &gl.CreateEnvironmentOptions{
		Name:        new("production"),
		Description: new("Evaluation production environment"),
		Tier:        new("production"),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.EnvironmentID = env.ID
	return nil
}

// ensureProjectAccessToken performs the ensure project access token operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureProjectAccessToken(ctx context.Context) error {
	expires := gl.ISOTime(time.Now().UTC().AddDate(0, 1, 0))
	accessLevel := gl.DeveloperPermissions
	token, _, err := p.client.GL().ProjectAccessTokens.CreateProjectAccessToken(p.state.ProjectID, &gl.CreateProjectAccessTokenOptions{
		Name:        new(fmt.Sprintf("eval-revoke-%d", time.Now().UnixNano())),
		Scopes:      &[]string{"read_api"},
		AccessLevel: &accessLevel,
		ExpiresAt:   &expires,
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.ProjectTokenID = token.ID
	return nil
}

// ensureCIVariables performs the ensure c i variables operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureCIVariables(ctx context.Context) error {
	const (
		projectKey  = "EVAL_TOKEN"
		groupKey    = "GROUP_EVAL_TOKEN"
		instanceKey = "INSTANCE_EVAL_TOKEN"
		value       = "masked-value-123"
	)
	for _, scope := range []string{"*", "production"} {
		p.ignoreNotFound(p.client.GL().ProjectVariables.RemoveVariable(p.state.ProjectID, projectKey, &gl.RemoveProjectVariableOptions{
			Filter: &gl.VariableFilter{EnvironmentScope: scope},
		}, gl.WithContext(ctx)))
		p.ignoreNotFound(p.client.GL().GroupVariables.RemoveVariable(p.state.GroupID, groupKey, &gl.RemoveGroupVariableOptions{
			Filter: &gl.VariableFilter{EnvironmentScope: scope},
		}, gl.WithContext(ctx)))
	}
	p.ignoreNotFound(p.client.GL().InstanceVariables.RemoveVariable(instanceKey, gl.WithContext(ctx)))

	_, _, err := p.client.GL().ProjectVariables.CreateVariable(p.state.ProjectID, &gl.CreateProjectVariableOptions{
		Key:              new(projectKey),
		Value:            new(value),
		EnvironmentScope: new("production"),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	_, _, err = p.client.GL().GroupVariables.CreateVariable(p.state.GroupID, &gl.CreateGroupVariableOptions{
		Key:              new(groupKey),
		Value:            new(value),
		EnvironmentScope: new("production"),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	_, _, err = p.client.GL().InstanceVariables.CreateVariable(&gl.CreateInstanceVariableOptions{
		Key:   new(instanceKey),
		Value: new(value),
	}, gl.WithContext(ctx))
	return err
}

// ignoreNotFound performs the ignore not found operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ignoreNotFound(_ *gl.Response, err error) {
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		p.notef("cleanup warning: %v", err)
	}
}

// ensurePackage performs the ensure package operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensurePackage(ctx context.Context) error {
	_, _, err := p.client.GL().GenericPackages.PublishPackageFile(
		p.state.ProjectID,
		liveFixturePackageName,
		fmt.Sprintf("%s-%d", liveFixturePackageVer, time.Now().UnixNano()),
		liveFixturePackageFile,
		bytes.NewBufferString("evaluation package\n"),
		nil,
		gl.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	packages, _, err := p.client.GL().Packages.ListProjectPackages(p.state.ProjectID, &gl.ListProjectPackagesOptions{
		PackageType: new("generic"),
		PackageName: new(liveFixturePackageName),
		OrderBy:     new("created_at"),
		Sort:        new("desc"),
		ListOptions: gl.ListOptions{PerPage: 1},
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return errors.New("published generic package was not listed")
	}
	p.state.PackageID = packages[0].ID
	return nil
}

// ensureDeployKey performs the ensure deploy key operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureDeployKey(ctx context.Context) error {
	key, err := newAuthorizedSSHKey()
	if err != nil {
		return err
	}
	deployKey, _, err := p.client.GL().DeployKeys.AddDeployKey(p.state.ProjectID, &gl.AddDeployKeyOptions{
		Title:   new(fmt.Sprintf("eval-key-%d", time.Now().UnixNano())),
		Key:     new(key),
		CanPush: new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.DeployKeyID = deployKey.ID
	return nil
}

// ensureDeployToken performs the ensure deploy token operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureDeployToken(ctx context.Context) error {
	expiresAt := time.Now().UTC().AddDate(0, 1, 0)
	token, _, err := p.client.GL().DeployTokens.CreateProjectDeployToken(p.state.ProjectID, &gl.CreateProjectDeployTokenOptions{
		Name:      new(fmt.Sprintf("eval-deploy-token-%d", time.Now().UnixNano())),
		ExpiresAt: &expiresAt,
		Scopes:    &[]string{"read_repository"},
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.DeployTokenID = token.ID
	return nil
}

// ensurePipelineTriggers performs the ensure pipeline triggers operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensurePipelineTriggers(ctx context.Context) error {
	deleteTrigger, _, err := p.client.GL().PipelineTriggers.AddPipelineTrigger(p.state.ProjectID, &gl.AddPipelineTriggerOptions{
		Description: new(fmt.Sprintf(liveDeleteFixtureFormat, time.Now().UnixNano())),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	runTrigger, _, err := p.client.GL().PipelineTriggers.AddPipelineTrigger(p.state.ProjectID, &gl.AddPipelineTriggerOptions{
		Description: new(fmt.Sprintf("run-fixture-%d", time.Now().UnixNano())),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.PipelineTriggerID = deleteTrigger.ID
	p.state.PipelineTriggerRunID = runTrigger.ID
	return nil
}

// ensurePipelineSchedules performs the ensure pipeline schedules operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensurePipelineSchedules(ctx context.Context) error {
	if err := p.cleanupPipelineSchedules(ctx); err != nil {
		return err
	}
	deleteSchedule, _, err := p.client.GL().PipelineSchedules.CreatePipelineSchedule(p.state.ProjectID, &gl.CreatePipelineScheduleOptions{
		Description:  new(fmt.Sprintf(liveDeleteFixtureFormat, time.Now().UnixNano())),
		Ref:          new(p.defaultRef()),
		Cron:         new("0 3 * * *"),
		CronTimezone: new("UTC"),
		Active:       new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	playSchedule, _, err := p.client.GL().PipelineSchedules.CreatePipelineSchedule(p.state.ProjectID, &gl.CreatePipelineScheduleOptions{
		Description:  new(fmt.Sprintf("play-fixture-%d", time.Now().UnixNano())),
		Ref:          new(p.defaultRef()),
		Cron:         new("30 3 * * *"),
		CronTimezone: new("UTC"),
		Active:       new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.PipelineScheduleID = deleteSchedule.ID
	p.state.PipelineSchedulePlayID = playSchedule.ID
	return nil
}

// cleanupPipelineSchedules performs the cleanup pipeline schedules operation on *liveFixturePreparer.
func (p *liveFixturePreparer) cleanupPipelineSchedules(ctx context.Context) error {
	schedules, _, err := p.client.GL().PipelineSchedules.ListPipelineSchedules(p.state.ProjectID, &gl.ListPipelineSchedulesOptions{
		ListOptions: gl.ListOptions{PerPage: 100},
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	for _, schedule := range schedules {
		if !strings.HasPrefix(schedule.Description, "delete-fixture-") && !strings.HasPrefix(schedule.Description, "play-fixture-") {
			continue
		}
		_, deleteErr := p.client.GL().PipelineSchedules.DeletePipelineSchedule(p.state.ProjectID, schedule.ID, gl.WithContext(ctx))
		if deleteErr != nil && !toolutil.IsHTTPStatus(deleteErr, http.StatusNotFound) {
			return deleteErr
		}
	}
	return nil
}

// ensureDisposableRunner performs the ensure disposable runner operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureDisposableRunner(ctx context.Context) error {
	runner, _, err := p.client.GL().Users.CreateUserRunner(&gl.CreateUserRunnerOptions{
		RunnerType:  new("project_type"),
		ProjectID:   new(p.state.ProjectID),
		Description: new(fmt.Sprintf("eval-disposable-runner-%d", time.Now().UnixNano())),
		Paused:      new(false),
		Locked:      new(false),
		RunUntagged: new(true),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.RunnerID = runner.ID
	return nil
}

// ensureDisposableUser performs the ensure disposable user operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureDisposableUser(ctx context.Context) error {
	username := fmt.Sprintf("eval-user-%d", time.Now().UnixNano())
	user, _, err := p.client.GL().Users.CreateUser(&gl.CreateUserOptions{
		Name:                new("Evaluation User"),
		Username:            new(username),
		Email:               new(username + "@example.com"),
		ForceRandomPassword: new(true),
		SkipConfirmation:    new(true),
		ProjectsLimit:       new(int64(0)),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.UserID = user.ID
	return nil
}

// ensureFeatureFlag performs the ensure feature flag operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureFeatureFlag(ctx context.Context) error {
	_, _, err := p.client.GL().ProjectFeatureFlags.GetProjectFeatureFlag(p.state.ProjectID, liveFixtureFeatureFlag, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return err
	}
	_, _, err = p.client.GL().ProjectFeatureFlags.CreateProjectFeatureFlag(p.state.ProjectID, &gl.CreateProjectFeatureFlagOptions{
		Name:        new(liveFixtureFeatureFlag),
		Description: new("Evaluation feature flag"),
		Active:      new(true),
	}, gl.WithContext(ctx))
	return err
}

// ensureWiki performs the ensure wiki operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureWiki(ctx context.Context) error {
	_, _, err := p.client.GL().Wikis.GetWikiPage(p.state.ProjectID, liveFixtureWikiSlug, nil, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return err
	}
	_, _, err = p.client.GL().Wikis.CreateWikiPage(p.state.ProjectID, &gl.CreateWikiPageOptions{
		Title:   new(liveFixtureWikiSlug),
		Content: new("Obsolete evaluation wiki page.\n"),
	}, gl.WithContext(ctx))
	return err
}

// ensureAwardEmoji performs the ensure award emoji operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureAwardEmoji(ctx context.Context) error {
	if p.state.IssueIID > 0 {
		award, _, err := p.client.GL().AwardEmoji.CreateIssueAwardEmoji(p.state.ProjectID, p.state.IssueIID, &gl.CreateAwardEmojiOptions{Name: "thumbsup"}, gl.WithContext(ctx))
		if err == nil {
			p.state.IssueAwardID = award.ID
		}
		if p.state.IssueAwardID == 0 {
			awards, _, listErr := p.client.GL().AwardEmoji.ListIssueAwardEmoji(p.state.ProjectID, p.state.IssueIID, &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{PerPage: 100}}, gl.WithContext(ctx))
			if listErr != nil {
				return listErr
			}
			for _, existing := range awards {
				if existing.Name == "thumbsup" {
					p.state.IssueAwardID = existing.ID
					break
				}
			}
		}
	}
	if p.state.MergeRequestIID > 0 {
		award, _, err := p.client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(p.state.ProjectID, p.state.MergeRequestIID, &gl.CreateAwardEmojiOptions{Name: "rocket"}, gl.WithContext(ctx))
		if err == nil {
			p.state.MergeRequestAwardID = award.ID
		}
		if p.state.MergeRequestAwardID == 0 {
			awards, _, listErr := p.client.GL().AwardEmoji.ListMergeRequestAwardEmoji(p.state.ProjectID, p.state.MergeRequestIID, &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{PerPage: 100}}, gl.WithContext(ctx))
			if listErr != nil {
				return listErr
			}
			for _, existing := range awards {
				if existing.Name == "rocket" {
					p.state.MergeRequestAwardID = existing.ID
					break
				}
			}
		}
	}
	return nil
}

// ensureDiscussions performs the ensure discussions operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureDiscussions(ctx context.Context) error {
	if p.state.MergeRequestIID == 0 {
		return nil
	}
	discussion, _, err := p.client.GL().Discussions.CreateMergeRequestDiscussion(p.state.ProjectID, p.state.MergeRequestIID, &gl.CreateMergeRequestDiscussionOptions{
		Body: new("Evaluation fixture discussion."),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.MergeRequestThreadID = discussion.ID
	return nil
}

// ensureCommitDiscussion performs the ensure commit discussion operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureCommitDiscussion(ctx context.Context) error {
	branch, _, err := p.client.GL().Branches.GetBranch(p.state.ProjectID, p.defaultRef(), gl.WithContext(ctx))
	if err != nil {
		return err
	}
	if branch.Commit == nil || branch.Commit.ID == "" {
		return errors.New("default branch has no commit ID")
	}
	discussion, _, err := p.client.GL().Discussions.CreateCommitDiscussion(p.state.ProjectID, branch.Commit.ID, &gl.CreateCommitDiscussionOptions{
		Body: new("Evaluation commit discussion note."),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	p.state.CommitSHA = branch.Commit.ID
	p.state.CommitDiscussionID = discussion.ID
	if len(discussion.Notes) > 0 {
		p.state.CommitDiscussionNoteID = discussion.Notes[0].ID
	}
	return nil
}

// createIssue performs the create issue operation on *liveFixturePreparer.
func (p *liveFixturePreparer) createIssue(ctx context.Context, title, description string, labels []string) (*gl.Issue, error) {
	labelOptions := gl.LabelOptions(labels)
	issue, _, err := p.client.GL().Issues.CreateIssue(p.state.ProjectID, &gl.CreateIssueOptions{
		Title:       new(title),
		Description: new(description),
		Labels:      &labelOptions,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create issue %q: %w", title, err)
	}
	return issue, nil
}

// ensureFixtureMergeRequest performs the ensure fixture merge request operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureFixtureMergeRequest(ctx context.Context, sourceBranch, title string, mergeFixture bool) (*gl.BasicMergeRequest, error) {
	defaultRef := p.defaultRef()
	if err := p.ensureBranch(ctx, sourceBranch, defaultRef); err != nil {
		return nil, err
	}
	filePath := strings.TrimPrefix(sourceBranch, "feature/") + ".txt"
	if err := p.ensureFile(ctx, filePath, sourceBranch, fmt.Sprintf("%s\n", title), "Seed MR fixture file"); err != nil {
		return nil, err
	}
	open := "opened"
	mrs, _, err := p.client.GL().MergeRequests.ListProjectMergeRequests(p.state.ProjectID, &gl.ListProjectMergeRequestsOptions{
		State:        &open,
		SourceBranch: &sourceBranch,
		TargetBranch: new(defaultRef),
		ListOptions:  gl.ListOptions{PerPage: 1},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if len(mrs) > 0 {
		return mrs[0], nil
	}
	description := "Evaluation fixture merge request."
	if mergeFixture {
		description = "Evaluation fixture merge request safe for merge tests."
	}
	mr, _, err := p.client.GL().MergeRequests.CreateMergeRequest(p.state.ProjectID, &gl.CreateMergeRequestOptions{
		Title:              new(title),
		Description:        new(description),
		SourceBranch:       new(sourceBranch),
		TargetBranch:       new(defaultRef),
		RemoveSourceBranch: new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return &gl.BasicMergeRequest{ID: mr.ID, IID: mr.IID, ProjectID: mr.ProjectID, Title: mr.Title, State: mr.State, SourceBranch: mr.SourceBranch, TargetBranch: mr.TargetBranch, WebURL: mr.WebURL}, nil
}

// waitForMergeRequestMergeable performs the wait for merge request mergeable operation on *liveFixturePreparer.
func (p *liveFixturePreparer) waitForMergeRequestMergeable(ctx context.Context, iid int64) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		mr, _, err := p.client.GL().MergeRequests.GetMergeRequest(p.state.ProjectID, iid, nil, gl.WithContext(ctx))
		if err != nil {
			return err
		}
		if mr.DetailedMergeStatus == "mergeable" {
			return nil
		}
		if mr.DetailedMergeStatus != "checking" && mr.DetailedMergeStatus != "unchecked" && mr.DetailedMergeStatus != "preparing" {
			return fmt.Errorf("merge request !%d is not mergeable: %s", iid, mr.DetailedMergeStatus)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("merge request !%d did not become mergeable before timeout: %s", iid, mr.DetailedMergeStatus)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// closeOpenMergeRequestsForBranch performs the close open merge requests for branch operation on *liveFixturePreparer.
func (p *liveFixturePreparer) closeOpenMergeRequestsForBranch(ctx context.Context, sourceBranch string) error {
	open := "opened"
	mrs, _, err := p.client.GL().MergeRequests.ListProjectMergeRequests(p.state.ProjectID, &gl.ListProjectMergeRequestsOptions{
		State:        &open,
		SourceBranch: &sourceBranch,
		TargetBranch: new(p.defaultRef()),
		ListOptions:  gl.ListOptions{PerPage: 100},
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	for _, mr := range mrs {
		_, _, updateErr := p.client.GL().MergeRequests.UpdateMergeRequest(p.state.ProjectID, mr.IID, &gl.UpdateMergeRequestOptions{StateEvent: new("close")}, gl.WithContext(ctx))
		if updateErr != nil && !toolutil.IsHTTPStatus(updateErr, http.StatusNotFound) {
			return updateErr
		}
	}
	return nil
}

// ensureBranch performs the ensure branch operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureBranch(ctx context.Context, branch, ref string) error {
	_, _, err := p.client.GL().Branches.GetBranch(p.state.ProjectID, branch, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("get branch %s: %w", branch, err)
	}
	_, _, err = p.client.GL().Branches.CreateBranch(p.state.ProjectID, &gl.CreateBranchOptions{
		Branch: new(branch),
		Ref:    new(ref),
	}, gl.WithContext(ctx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	return nil
}

// waitForBranch performs the wait for branch operation on *liveFixturePreparer.
func (p *liveFixturePreparer) waitForBranch(ctx context.Context, branch string) error {
	deadline := time.Now().Add(45 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		_, _, err := p.client.GL().Branches.GetBranch(p.state.ProjectID, branch, gl.WithContext(ctx))
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("wait for branch %s: %w", branch, lastErr)
}

// ensureFile performs the ensure file operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureFile(ctx context.Context, path, branch, content, message string) error {
	_, _, err := p.client.GL().RepositoryFiles.GetFile(p.state.ProjectID, path, &gl.GetFileOptions{Ref: new(branch)}, gl.WithContext(ctx))
	if err == nil {
		_, _, updateErr := p.client.GL().RepositoryFiles.UpdateFile(p.state.ProjectID, path, &gl.UpdateFileOptions{
			Branch:        new(branch),
			Content:       new(content),
			CommitMessage: new(message),
		}, gl.WithContext(ctx))
		if updateErr == nil || isEmptyCommitError(updateErr) {
			return nil
		}
		if isMissingFileUpdateError(updateErr) {
			return p.createFile(ctx, path, branch, content, message)
		}
		return fmt.Errorf("update file %s on %s: %w", path, branch, updateErr)
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("get file %s on %s: %w", path, branch, err)
	}
	return p.createFile(ctx, path, branch, content, message)
}

// createFile creates a repository file and tolerates races where it already exists.
func (p *liveFixturePreparer) createFile(ctx context.Context, path, branch, content, message string) error {
	_, _, err := p.client.GL().RepositoryFiles.CreateFile(p.state.ProjectID, path, &gl.CreateFileOptions{
		Branch:        new(branch),
		Content:       new(content),
		CommitMessage: new(message),
	}, gl.WithContext(ctx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return fmt.Errorf("create file %s on %s: %w", path, branch, err)
	}
	return nil
}

// ensureTag performs the ensure tag operation on *liveFixturePreparer.
func (p *liveFixturePreparer) ensureTag(ctx context.Context, tag, ref string) error {
	_, _, err := p.client.GL().Tags.GetTag(p.state.ProjectID, tag, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return err
	}
	_, _, err = p.client.GL().Tags.CreateTag(p.state.ProjectID, &gl.CreateTagOptions{
		TagName: new(tag),
		Ref:     new(ref),
	}, gl.WithContext(ctx))
	return err
}

// waitForPipelineJobs performs the wait for pipeline jobs operation on *liveFixturePreparer.
func (p *liveFixturePreparer) waitForPipelineJobs(ctx context.Context, pipelineID int64) error {
	deadline := time.Now().Add(8 * time.Minute)
	var lastStatuses []string
	for time.Now().Before(deadline) {
		jobs, _, err := p.client.GL().Jobs.ListPipelineJobs(p.state.ProjectID, pipelineID, &gl.ListJobsOptions{ListOptions: gl.ListOptions{PerPage: 100}}, gl.WithContext(ctx))
		if err != nil {
			return err
		}
		lastStatuses = lastStatuses[:0]
		for _, job := range jobs {
			lastStatuses = append(lastStatuses, fmt.Sprintf("%s:%s", job.Name, job.Status))
			if job.Runner.ID > 0 && p.state.RunnerID == 0 {
				p.state.RunnerID = job.Runner.ID
			}
			if job.Status == "failed" && p.state.FailedJobID == 0 {
				p.state.FailedJobID = job.ID
			}
			if job.Status == "manual" && p.state.ManualJobID == 0 {
				p.state.ManualJobID = job.ID
			}
		}
		if p.state.FailedJobID > 0 && p.state.ManualJobID > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return fmt.Errorf("pipeline jobs did not reach failed/manual states; last statuses: %s", strings.Join(lastStatuses, ", "))
}

// applyLiveFixtureState is an internal helper for the main package.
func applyLiveFixtureState(tasks []evalTask, state *liveFixtureState) []evalTask {
	out := make([]evalTask, len(tasks))
	for i, task := range tasks {
		out[i] = task
		out[i].Prompt = replaceFixturePrompt(task.ID, task.Prompt, state)
	}
	return out
}

// filterTasksByLiveFixtureState removes tasks whose live Docker resources were not seeded.
func filterTasksByLiveFixtureState(tasks []evalTask, state *liveFixtureState) []evalTask {
	if state == nil {
		return tasks
	}
	filtered := make([]evalTask, 0, len(tasks))
	for _, task := range tasks {
		if taskLiveFixtureStateAvailable(task, state) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

// taskLiveFixtureStateAvailable reports whether a task's live fixture dependencies exist.
func taskLiveFixtureStateAvailable(task evalTask, state *liveFixtureState) bool {
	switch task.ID {
	case "MT-020", "MT-021", "MT-039", "MF-001":
		return state.PipelineID > 0
	case "MT-022", "MT-024", "MT-065", "MS-002":
		return state.FailedJobID > 0
	case "MT-064":
		return state.ManualJobID > 0
	case "MT-046", "MT-047":
		return state.RunnerID > 0
	case "MS-008":
		return state.RunnerID > 0 && state.FailedJobID > 0
	default:
		return true
	}
}

// addLiveAttemptResourceSuffix is an internal helper for the main package.
func addLiveAttemptResourceSuffix(task evalTask, modelLabel string, runIndex int, runSuffix string) evalTask {
	if !taskNeedsAttemptResourceSuffix(task.ID) {
		return task
	}
	suffix := liveAttemptResourceSuffix(modelLabel, runIndex, runSuffix)
	if suffix == "" {
		return task
	}
	if task.ID == taskFileCreateID || task.ID == "MS-017" {
		task.Prompt = suffixEvaluationFileCreatePath(task.Prompt, suffix)
		return task
	}
	task.Prompt = suffixEvaluationBacktickValues(task.Prompt, suffix)
	return task
}

// suffixEvaluationFileCreatePath is an internal helper for the main package.
func suffixEvaluationFileCreatePath(prompt, suffix string) string {
	return suffixEvaluationBacktickValuesMatching(prompt, suffix, func(value string) bool {
		return strings.HasPrefix(value, "tmp/eval")
	})
}

// suffixEvaluationBacktickValuesMatching is an internal helper for the main package.
func suffixEvaluationBacktickValuesMatching(prompt, suffix string, shouldSuffix func(string) bool) string {
	var out strings.Builder
	for {
		before, remaining, ok := strings.Cut(prompt, "`")
		if !ok {
			out.WriteString(prompt)
			return out.String()
		}
		out.WriteString(before)
		out.WriteByte('`')
		value, after, ok := strings.Cut(remaining, "`")
		if !ok {
			out.WriteString(remaining)
			return out.String()
		}
		if shouldSuffix(value) {
			value = suffixEvaluationValue(value, suffix)
		}
		out.WriteString(value)
		out.WriteByte('`')
		prompt = after
	}
}

// taskNeedsAttemptResourceSuffix is an internal helper for the main package.
func taskNeedsAttemptResourceSuffix(taskID string) bool {
	switch taskID {
	case "MT-007", "MT-015", "MT-026", taskFileCreateID, "MT-034", "MT-036", "MT-056", "MT-058", "MT-067", "MT-068",
		"MS-004", "MS-014", "MS-015", "MS-016", "MS-017", "MS-018", "MS-019", "MS-020", "MS-021", "MS-022", "MS-023", "MS-024", "MS-025", "MS-026", "MS-027", "MS-028", "MS-029", "MS-030", "MS-031", "MS-032", "MS-033", "MS-035", "MS-036":
		return true
	default:
		return false
	}
}

// liveAttemptResourceSuffix is an internal helper for the main package.
func liveAttemptResourceSuffix(modelLabel string, runIndex int, runSuffix string) string {
	modelPart := modelLabel
	if idx := strings.LastIndex(modelPart, ":"); idx >= 0 {
		modelPart = modelPart[idx+1:]
	}
	var slug strings.Builder
	for _, r := range strings.ToLower(modelPart) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			slug.WriteRune(r)
		}
	}
	if slug.Len() == 0 || runSuffix == "" {
		return ""
	}
	text := slug.String()
	if len(text) > 12 {
		text = text[:12]
	}
	return fmt.Sprintf("%s-r%d-%s", text, runIndex, runSuffix)
}

// suffixEvaluationBacktickValues is an internal helper for the main package.
func suffixEvaluationBacktickValues(prompt, suffix string) string {
	var out strings.Builder
	for {
		before, remaining, ok := strings.Cut(prompt, "`")
		if !ok {
			out.WriteString(prompt)
			return out.String()
		}
		out.WriteString(before)
		out.WriteByte('`')
		value, after, ok := strings.Cut(remaining, "`")
		if !ok {
			out.WriteString(remaining)
			return out.String()
		}
		out.WriteString(suffixEvaluationValue(value, suffix))
		out.WriteByte('`')
		prompt = after
	}
}

// suffixEvaluationValue is an internal helper for the main package.
func suffixEvaluationValue(value, suffix string) string {
	if strings.Contains(value, suffix) || !shouldSuffixEvaluationValue(value) {
		return value
	}
	separator := "-"
	if strings.HasPrefix(value, "EVAL_") || strings.HasPrefix(value, "GROUP_EVAL_") || strings.HasPrefix(value, "INSTANCE_EVAL_") {
		return value + "_" + strings.ReplaceAll(suffix, "-", "_")
	}
	if strings.HasPrefix(value, "Evaluation ") {
		separator = " "
	}
	return value + separator + suffix
}

// shouldSuffixEvaluationValue is an internal helper for the main package.
func shouldSuffixEvaluationValue(value string) bool {
	switch {
	case strings.HasPrefix(value, "Evaluation "):
		return true
	case strings.HasPrefix(value, "eval-"):
		return true
	case strings.HasPrefix(value, "feature/eval"):
		return true
	case strings.HasPrefix(value, "tmp/eval"):
		return true
	case strings.HasPrefix(value, "v0.0.0-eval"):
		return true
	case strings.HasPrefix(value, "v0.0.0-crud"):
		return true
	case strings.HasPrefix(value, "EVAL_"):
		return true
	case strings.HasPrefix(value, "GROUP_EVAL_"):
		return true
	case strings.HasPrefix(value, "INSTANCE_EVAL_"):
		return true
	default:
		return false
	}
}

// replaceFixturePrompt is an internal helper for the main package.
func replaceFixturePrompt(taskID, prompt string, state *liveFixtureState) string {
	replacements := map[string]string{
		"https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git": state.RemoteURL,
		"project ID `123`": fmt.Sprintf("project ID `%d`", state.ProjectID),
		"project path `my-org/tools/gitlab-mcp-server`": fmt.Sprintf("project path `%s`", state.ProjectPath),
	}
	for old, newValue := range replacements {
		if newValue != "" {
			prompt = strings.ReplaceAll(prompt, old, newValue)
		}
	}
	prompt = replaceIssuePlaceholders(taskID, prompt, state)
	prompt = replaceMergeRequestPlaceholders(taskID, prompt, state)
	prompt = replacePipelinePlaceholders(taskID, prompt, state)
	prompt = replaceResourcePlaceholders(taskID, prompt, state)
	prompt = replaceLifecyclePlaceholders(prompt, state)
	return prompt
}

// replaceLifecyclePlaceholders is an internal helper for the main package.
func replaceLifecyclePlaceholders(prompt string, state *liveFixtureState) string {
	suffix := fixtureUniqueSuffix(state)
	if suffix == "" {
		return prompt
	}
	replacements := map[string]string{
		"`eval-crud-issue`":                    fmt.Sprintf("`eval-crud-issue-%s`", suffix),
		"`eval-crud-issue-updated`":            fmt.Sprintf("`eval-crud-issue-updated-%s`", suffix),
		"`eval-note-issue`":                    fmt.Sprintf("`eval-note-issue-%s`", suffix),
		"`eval-link-source`":                   fmt.Sprintf("`eval-link-source-%s`", suffix),
		"`eval-link-target`":                   fmt.Sprintf("`eval-link-target-%s`", suffix),
		"`eval-protect-branch`":                fmt.Sprintf("`eval-protect-branch-%s`", suffix),
		"`eval-feature-list`":                  fmt.Sprintf("`eval-feature-list-%s`", suffix),
		"`eval-feature-flag-crud`":             fmt.Sprintf("`eval-feature-flag-crud-%s`", suffix),
		"`eval-deploy-token`":                  fmt.Sprintf("`eval-deploy-token-%s`", suffix),
		"`eval-deploy-key`":                    fmt.Sprintf("`eval-deploy-key-%s`", suffix),
		"`eval-deploy-key-updated`":            fmt.Sprintf("`eval-deploy-key-updated-%s`", suffix),
		"`eval-time-issue`":                    fmt.Sprintf("`eval-time-issue-%s`", suffix),
		"`eval-group-label`":                   fmt.Sprintf("`eval-group-label-%s`", suffix),
		"`eval-group-label-v2`":                fmt.Sprintf("`eval-group-label-v2-%s`", suffix),
		"`tmp/eval-crud.txt`":                  fmt.Sprintf("`tmp/eval-crud-%s.txt`", suffix),
		"`v0.0.0-crud`":                        fmt.Sprintf("`v0.0.0-crud-%s`", suffix),
		"`eval-crud-link`":                     fmt.Sprintf("`eval-crud-link-%s`", suffix),
		"`eval-crud-trigger`":                  fmt.Sprintf("`eval-crud-trigger-%s`", suffix),
		"`eval-crud-schedule`":                 fmt.Sprintf("`eval-crud-schedule-%s`", suffix),
		"`SCHEDULE_CRUD_TOKEN`":                fmt.Sprintf("`SCHEDULE_CRUD_TOKEN_%s`", suffix),
		"`https://example.com/eval-crud-hook`": fmt.Sprintf("`https://example.com/eval-crud-hook-%s`", suffix),
		"`eval-crud-badge`":                    fmt.Sprintf("`eval-crud-badge-%s`", suffix),
		"`eval-crud-wiki`":                     fmt.Sprintf("`eval-crud-wiki-%s`", suffix),
		"`eval-crud-snippet`":                  fmt.Sprintf("`eval-crud-snippet-%s`", suffix),
		"`EVAL_CRUD_TOKEN`":                    fmt.Sprintf("`EVAL_CRUD_TOKEN_%s`", suffix),
		"`GROUP_EVAL_CRUD_TOKEN`":              fmt.Sprintf("`GROUP_EVAL_CRUD_TOKEN_%s`", suffix),
		"`eval-mr-note`":                       fmt.Sprintf("`eval-mr-note-%s`", suffix),
		"`eval-mr-note-updated`":               fmt.Sprintf("`eval-mr-note-updated-%s`", suffix),
		"`Evaluation CRUD release`":            fmt.Sprintf("`Evaluation CRUD release %s`", suffix),
		"`Evaluation CRUD schedule`":           fmt.Sprintf("`Evaluation CRUD schedule %s`", suffix),
		"`Evaluation CRUD snippet`":            fmt.Sprintf("`Evaluation CRUD snippet %s`", suffix),
		"`Evaluation CRUD wiki`":               fmt.Sprintf("`Evaluation CRUD wiki %s`", suffix),
		"`Evaluation CRUD wiki v2`":            fmt.Sprintf("`Evaluation CRUD wiki v2 %s`", suffix),
		"`Evaluation CRUD badge link`":         fmt.Sprintf("`Evaluation CRUD badge link %s`", suffix),
		"`Evaluation Group Milestone`":         fmt.Sprintf("`Evaluation Group Milestone %s`", suffix),
		"`Evaluation Group Milestone v2`":      fmt.Sprintf("`Evaluation Group Milestone v2 %s`", suffix),
	}
	if state.DeployKeyCreateKey != "" {
		replacements["`ssh-rsa AAAAevalcrud`"] = fmt.Sprintf("`%s`", state.DeployKeyCreateKey)
	}
	for old, newValue := range replacements {
		prompt = strings.ReplaceAll(prompt, old, newValue)
	}
	return prompt
}

// replaceIssuePlaceholders is an internal helper for the main package.
func replaceIssuePlaceholders(taskID, prompt string, state *liveFixtureState) string {
	issueIID := state.IssueIID
	if taskID == "MT-013" && state.IssueDeleteIID > 0 {
		issueIID = state.IssueDeleteIID
	}
	if issueIID > 0 {
		prompt = strings.ReplaceAll(prompt, "issue `42`", fmt.Sprintf("issue `%d`", issueIID))
	}
	if taskID == "MT-110" && state.IssueAwardID > 0 {
		prompt = strings.ReplaceAll(prompt, "award emoji ID `12`", fmt.Sprintf("award emoji ID `%d`", state.IssueAwardID))
	}
	return prompt
}

// replaceMergeRequestPlaceholders is an internal helper for the main package.
func replaceMergeRequestPlaceholders(taskID, prompt string, state *liveFixtureState) string {
	mrIID := state.MergeRequestIID
	if taskID == "MT-017" && state.MergeRequestMergeIID > 0 {
		mrIID = state.MergeRequestMergeIID
	}
	if taskID == "MS-033" && state.MergeRequestAwardIID > 0 {
		mrIID = state.MergeRequestAwardIID
	}
	if mrIID > 0 {
		prompt = strings.ReplaceAll(prompt, "merge request `7`", fmt.Sprintf("merge request `%d`", mrIID))
		prompt = strings.ReplaceAll(prompt, "merge request IID `7`", fmt.Sprintf("merge request IID `%d`", mrIID))
		prompt = strings.ReplaceAll(prompt, "merge_request_iid `7`", fmt.Sprintf("merge_request_iid `%d`", mrIID))
		prompt = strings.ReplaceAll(prompt, "MR `7`", fmt.Sprintf("MR `%d`", mrIID))
		if taskID == "MS-033" {
			prompt = strings.ReplaceAll(prompt, "MR `1`", fmt.Sprintf("MR `%d`", mrIID))
		}
	}
	if taskID == "MT-061" && state.MergeRequestThreadID != "" {
		prompt = strings.ReplaceAll(prompt, "discussion `abc123`", fmt.Sprintf("discussion `%s`", state.MergeRequestThreadID))
		prompt = strings.ReplaceAll(prompt, "discussion_id `abc123`", fmt.Sprintf("discussion_id `%s`", state.MergeRequestThreadID))
	}
	if taskID == "MT-109" && state.MergeRequestAwardID > 0 {
		prompt = strings.ReplaceAll(prompt, "award emoji ID `12`", fmt.Sprintf("award emoji ID `%d`", state.MergeRequestAwardID))
	}
	return prompt
}

// replacePipelinePlaceholders is an internal helper for the main package.
func replacePipelinePlaceholders(taskID, prompt string, state *liveFixtureState) string {
	if state.PipelineID > 0 {
		pipelineID := state.PipelineID
		if taskID == "MT-088" && state.PipelineIID > 0 {
			pipelineID = state.PipelineIID
		}
		prompt = strings.ReplaceAll(prompt, "pipeline `12345`", fmt.Sprintf("pipeline `%d`", pipelineID))
		prompt = strings.ReplaceAll(prompt, "pipeline IID `12345`", fmt.Sprintf("pipeline IID `%d`", pipelineID))
	}
	jobID := state.FailedJobID
	if taskID == "MT-064" && state.ManualJobID > 0 {
		jobID = state.ManualJobID
	}
	if jobID > 0 {
		prompt = strings.ReplaceAll(prompt, "job `999`", fmt.Sprintf("job `%d`", jobID))
	}
	return prompt
}

// replaceResourcePlaceholders is an internal helper for the main package.
func replaceResourcePlaceholders(taskID, prompt string, state *liveFixtureState) string {
	switch taskID {
	case "MT-007":
		prompt = replaceID(prompt, "group ID", 123, state.GroupID)
		if suffix := fixtureUniqueSuffix(state); suffix != "" {
			prompt = strings.ReplaceAll(prompt, "`eval-temp`", fmt.Sprintf("`eval-temp-%s`", suffix))
		}
	case "MT-035":
		prompt = replaceID(prompt, "milestone IID", 7, state.MilestoneDeleteIID)
	case "MT-042":
		prompt = replaceID(prompt, "project access token ID", 77, state.ProjectTokenID)
	case "MT-044", "MS-007":
		prompt = replaceID(prompt, "package ID", 55, state.PackageID)
	case "MT-046", "MT-047", "MS-008":
		prompt = replaceID(prompt, "runner ID", 99, state.RunnerID)
	case "MT-049":
		prompt = replaceID(prompt, "environment ID", 7, state.EnvironmentID)
	case "MT-050", "MT-051":
		prompt = replaceID(prompt, "personal snippet ID", 33, state.SnippetID)
	case "MT-054", "MS-009":
		return prompt
	case "MT-057":
		prompt = replaceID(prompt, "webhook ID", 5, state.HookDeleteID)
	case "MT-059":
		prompt = replaceID(prompt, "badge ID", 8, state.BadgeDeleteID)
	case "MT-102":
		prompt = replaceID(prompt, "pipeline trigger token ID", 77, state.PipelineTriggerID)
	case taskPipelineScheduleID:
		scheduleID := state.PipelineScheduleID
		if state.PipelineSchedulePlayID > 0 && strings.Contains(prompt, "play") {
			scheduleID = state.PipelineSchedulePlayID
		}
		prompt = replaceID(prompt, "pipeline schedule ID", 12, scheduleID)
	case "MT-104", "MT-105", "MS-034":
		prompt = replaceID(prompt, "user ID", 55, state.UserID)
	case "MT-111":
		prompt = replaceID(prompt, "deploy key ID", 88, state.DeployKeyID)
	case "MT-112":
		prompt = replaceID(prompt, "project deploy token ID", 66, state.DeployTokenID)
	case "MT-113":
		prompt = replaceID(prompt, "commit discussion note", 999, state.CommitDiscussionNoteID)
		if state.CommitDiscussionID != "" {
			prompt = strings.ReplaceAll(prompt, "discussion `abc123`", fmt.Sprintf("discussion `%s`", state.CommitDiscussionID))
		}
		if state.CommitSHA != "" {
			prompt = strings.ReplaceAll(prompt, "commit `abc1234`", fmt.Sprintf("commit `%s`", state.CommitSHA))
		}
	case "MT-037", "MT-100", "MS-004":
		if state.CleanupReleaseTag != "" {
			prompt = strings.ReplaceAll(prompt, liveFixtureFailureTag, state.CleanupReleaseTag)
		}
	case "MT-095", "MS-012":
		if state.ReleaseSummaryTag != "" {
			prompt = strings.ReplaceAll(prompt, "`v0.0.0-eval-ms`", fmt.Sprintf("`%s`", state.ReleaseSummaryTag))
		}
	case "MS-006":
		prompt = replaceID(prompt, "deployment ID", 77, 0)
	case "MS-013":
		if state.FeatureFlagName != "" {
			prompt = strings.ReplaceAll(prompt, "`eval_flag`", fmt.Sprintf("`%s`", state.FeatureFlagName))
		}
	}
	if suffix := fixtureUniqueSuffix(state); suffix != "" {
		switch taskID {
		case taskFileCreateID:
			prompt = strings.ReplaceAll(prompt, "`tmp/eval.txt`", fmt.Sprintf("`tmp/eval-%s.txt`", suffix))
		case "MT-034":
			prompt = strings.ReplaceAll(prompt, "`Evaluation Sprint`", fmt.Sprintf("`Evaluation Sprint %s`", suffix))
		case "MT-036":
			prompt = strings.ReplaceAll(prompt, "`v0.0.0-eval`", fmt.Sprintf("`v0.0.0-eval-%s`", suffix))
		}
	}
	if taskID == taskPipelineScheduleID && state.PipelineTriggerRunID > 0 && strings.Contains(prompt, "run trigger") {
		prompt = replaceID(prompt, "pipeline trigger token ID", 77, state.PipelineTriggerRunID)
	}
	return prompt
}

// fixtureUniqueSuffix is an internal helper for the main package.
func fixtureUniqueSuffix(state *liveFixtureState) string {
	if state.PipelineID > 0 {
		return strconv.FormatInt(state.PipelineID, 10)
	}
	if state.ProjectID > 0 {
		return strconv.FormatInt(state.ProjectID, 10)
	}
	return ""
}

// replaceID is an internal helper for the main package.
func replaceID(prompt, label string, oldID, newID int64) string {
	if newID <= 0 {
		return prompt
	}
	return strings.ReplaceAll(prompt, fmt.Sprintf("%s `%d`", label, oldID), fmt.Sprintf("%s `%d`", label, newID))
}

// fixtureRemoteURL is an internal helper for the main package.
func fixtureRemoteURL(baseURL, projectPath string) string {
	return strings.TrimRight(baseURL, "/") + "/" + projectPath + ".git"
}

// fixtureReadme is an internal helper for the main package.
func fixtureReadme() string {
	return "# GitLab MCP Server Evaluation Fixture\n\nThis repository is seeded by cmd/eval_meta_tools for live MCP evaluation.\n\nfunc RegisterMCPMeta() {}\n\nTODO: keep evaluation coverage representative.\n"
}

// fixtureCI is an internal helper for the main package.
func fixtureCI() string {
	return `stages:
  - test

variables:
  GIT_STRATEGY: none

failing_fixture:
  stage: test
  script:
    - mkdir -p coverage
    - printf '<coverage />\n' > coverage/report.xml
    - echo 'intentional evaluation failure'
    - exit 1
  artifacts:
    when: always
    paths:
      - coverage/report.xml

manual_deploy:
  stage: test
  when: manual
  script:
    - echo "deploying ${DEPLOY_ENV:-staging}"
`
}

// pathBase is an internal helper for the main package.
func pathBase(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

// isEmptyCommitError is an internal helper for the main package.
func isEmptyCommitError(err error) bool {
	return toolutil.ContainsAny(err, "commit was empty", "You are trying to update the file with the same content")
}

// isMissingFileUpdateError reports GitLab update errors caused by a missing file.
func isMissingFileUpdateError(err error) bool {
	return toolutil.ContainsAny(err, "A file with this name doesn't exist", "file does not exist")
}

// newAuthorizedSSHKey performs the new authorized s s h key operation using the GitLab API and returns [string].
func newAuthorizedSSHKey() (string, error) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	sshKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshKey))), nil
}
