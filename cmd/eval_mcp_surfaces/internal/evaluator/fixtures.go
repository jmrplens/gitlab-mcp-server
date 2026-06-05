package evaluator

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
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
	"golang.org/x/crypto/ssh"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// liveFixtureGroupPath identifies the live fixture group path constant used by this package.
	liveFixtureGroupPath = "my-org"
	// liveFixtureToolsPath identifies the live fixture tools path constant used by this package.
	liveFixtureToolsPath = "my-org/tools"
	// liveFixtureProjectPath identifies the live fixture project path constant used by this package.
	liveFixtureProjectPath = "my-org/tools/gitlab-mcp-server"
	// liveFixtureDefaultRef identifies the live fixture default ref constant used by this package.
	liveFixtureDefaultRef = "main"
	// liveFixtureFeatureRef identifies the live fixture feature ref constant used by this package.
	liveFixtureFeatureRef = "feature/eval"
	// liveFixtureObsoleteRef identifies the live fixture obsolete ref constant used by this package.
	liveFixtureObsoleteRef = "obsolete/eval"
	// liveFixtureCleanupTag identifies the live fixture cleanup tag constant used by this package.
	liveFixtureCleanupTag = "v0.0.0-eval-ms"
	// liveFixtureReleaseSummaryTag identifies the live fixture release summary tag constant used by this package.
	liveFixtureReleaseSummaryTag = "v0.0.0-eval-summary-ms"
	// liveFixtureElicitationTag identifies the live fixture elicitation tag constant used by this package.
	liveFixtureElicitationTag = "v0.0.0-eval-elicit"
	// liveFixtureInteractiveMRFile identifies the live fixture interactive MR file constant used by this package.
	liveFixtureInteractiveMRFile = "interactive/eval-mr.txt"
	// liveFixtureFeatureFlag identifies the live fixture feature flag constant used by this package.
	liveFixtureFeatureFlag = "eval_flag"
	// liveFixturePackageName identifies the live fixture package name constant used by this package.
	liveFixturePackageName = "eval-package"
	// liveFixturePackageVer identifies the live fixture package ver constant used by this package.
	liveFixturePackageVer = "0.0.1"
	// liveFixturePackageFile identifies the live fixture package file constant used by this package.
	liveFixturePackageFile = "artifact.txt"
	// liveFixturePackageReleaseName identifies the package name for the package-to-release workflow fixture.
	liveFixturePackageReleaseName = "eval-release-package"
	// liveFixturePackageReleaseVersion identifies the package version for the package-to-release workflow fixture.
	liveFixturePackageReleaseVersion = "0.1.0"
	// liveFixturePackageReleaseTag identifies the release tag for the package-to-release workflow fixture.
	liveFixturePackageReleaseTag = "v0.0.0-eval-packages"
	// liveFixtureProjectServiceAccountName identifies the Enterprise project service-account fixture name.
	liveFixtureProjectServiceAccountName = "eval-project-service-account"
	// liveFixtureProjectServiceAccountUsername identifies the Enterprise project service-account username prefix.
	liveFixtureProjectServiceAccountUsername = "eval-project-svc"
	// liveFixtureProjectServiceAccountPATName identifies the Enterprise project service-account PAT fixture.
	liveFixtureProjectServiceAccountPATName = "eval-project-service-token"
	// liveFixtureWikiSlug identifies the live fixture wiki slug constant used by this package.
	liveFixtureWikiSlug = "obsolete-eval"
	// liveFixtureReviewBranch identifies the live fixture review branch constant used by this package.
	liveFixtureReviewBranch = "feature/eval-review-fixture"
	// liveFixtureMergeBranch identifies the live fixture merge branch constant used by this package.
	liveFixtureMergeBranch = "feature/eval-merge-fixture"
	// liveFixtureAwardBranchPrefix identifies the live fixture award branch prefix constant used by this package.
	liveFixtureAwardBranchPrefix = "feature/eval-award-fixture-"
	// liveDeleteFixtureFormat identifies the live delete fixture format constant used by this package.
	liveDeleteFixtureFormat = "delete-fixture-%d"
	// taskPackageReleaseID identifies the package publish plus release workflow task.
	taskPackageReleaseID  = "MS-038"
	resourceLabelRunnerID = "runner ID"
	resourceLabelUserID   = "user ID"
)

var packageReleaseFixtureFiles = []struct {
	name    string
	content string
}{
	{name: "gitlab-mcp-server-linux-amd64.txt", content: "linux amd64 evaluation package\n"},
	{name: "gitlab-mcp-server-darwin-arm64.txt", content: "darwin arm64 evaluation package\n"},
	{name: "checksums.txt", content: "sha256  gitlab-mcp-server-linux-amd64.txt\nsha256  gitlab-mcp-server-darwin-arm64.txt\n"},
}

// liveFixtureState captures live fixture state data for live evaluation fixtures.
type liveFixtureState struct {
	GeneratedAt                  string   `json:"generated_at"`
	GitLabURL                    string   `json:"gitlab_url"`
	GroupPath                    string   `json:"group_path"`
	GroupID                      int64    `json:"group_id"`
	ToolsGroupPath               string   `json:"tools_group_path"`
	ToolsGroupID                 int64    `json:"tools_group_id"`
	ProjectPath                  string   `json:"project_path"`
	ProjectID                    int64    `json:"project_id"`
	DefaultBranch                string   `json:"default_branch"`
	RemoteURL                    string   `json:"remote_url"`
	IssueIID                     int64    `json:"issue_iid"`
	IssueDeleteIID               int64    `json:"issue_delete_iid"`
	MergeRequestIID              int64    `json:"merge_request_iid"`
	MergeRequestMergeIID         int64    `json:"merge_request_merge_iid"`
	MergeRequestAwardIID         int64    `json:"merge_request_award_iid,omitempty"`
	MergeRequestThreadID         string   `json:"merge_request_thread_id,omitempty"`
	PipelineID                   int64    `json:"pipeline_id"`
	PipelineIID                  int64    `json:"pipeline_iid"`
	FailedJobID                  int64    `json:"failed_job_id"`
	ManualJobID                  int64    `json:"manual_job_id"`
	RunnerID                     int64    `json:"runner_id"`
	MilestoneDeleteIID           int64    `json:"milestone_delete_iid"`
	HookDeleteID                 int64    `json:"hook_delete_id"`
	BadgeDeleteID                int64    `json:"badge_delete_id"`
	SnippetID                    int64    `json:"snippet_id"`
	EnvironmentID                int64    `json:"environment_id"`
	ProjectTokenID               int64    `json:"project_token_id"`
	PackageID                    int64    `json:"package_id"`
	PackageReleaseName           string   `json:"package_release_name,omitempty"`
	PackageReleaseVersion        string   `json:"package_release_version,omitempty"`
	PackageReleaseTag            string   `json:"package_release_tag,omitempty"`
	PackageReleaseDir            string   `json:"package_release_dir,omitempty"`
	PackageReleaseFiles          []string `json:"package_release_files,omitempty"`
	PackageReleasePaths          []string `json:"package_release_paths,omitempty"`
	ProjectServiceAccountID      int64    `json:"project_service_account_id,omitempty"`
	ProjectServiceAccountTokenID int64    `json:"project_service_account_token_id,omitempty"`
	DeployKeyID                  int64    `json:"deploy_key_id"`
	DeployKeyCreateKey           string   `json:"deploy_key_create_key,omitempty"`
	DeployTokenID                int64    `json:"deploy_token_id"`
	PipelineTriggerID            int64    `json:"pipeline_trigger_id"`
	PipelineTriggerRunID         int64    `json:"pipeline_trigger_run_id"`
	PipelineScheduleID           int64    `json:"pipeline_schedule_id"`
	PipelineSchedulePlayID       int64    `json:"pipeline_schedule_play_id"`
	UserID                       int64    `json:"user_id"`
	IssueAwardID                 int64    `json:"issue_award_id"`
	MergeRequestAwardID          int64    `json:"merge_request_award_id"`
	CommitSHA                    string   `json:"commit_sha,omitempty"`
	CommitDiscussionID           string   `json:"commit_discussion_id,omitempty"`
	CommitDiscussionNoteID       int64    `json:"commit_discussion_note_id,omitempty"`
	FeatureFlagName              string   `json:"feature_flag_name"`
	WikiSlug                     string   `json:"wiki_slug"`
	CleanupReleaseTag            string   `json:"cleanup_release_tag"`
	ReleaseSummaryTag            string   `json:"release_summary_tag,omitempty"`
	ElicitationReleaseTag        string   `json:"elicitation_release_tag,omitempty"`
	Notes                        []string `json:"notes,omitempty"`
}

// liveFixturePreparer captures live fixture preparer data for live evaluation fixtures.
type liveFixturePreparer struct {
	client *gitlabclient.Client
	state  *liveFixtureState
}

// prepareLiveFixtures handles prepare live fixtures and returns [*liveFixtureState].
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
		ReleaseSummaryTag:     liveFixtureReleaseSummaryTag,
		ElicitationReleaseTag: liveFixtureElicitationTag,
	}
	if fixtureErr := ensurePackageReleaseFixtureFiles(state, opts.Fixtures); fixtureErr != nil {
		return nil, fixtureErr
	}
	preparer := &liveFixturePreparer{client: client, state: state}
	if prepareErr := preparer.prepare(ctx); prepareErr != nil {
		return nil, prepareErr
	}
	return state, nil
}

// validateFixtureOptions validates fixture options for the evaluator package.
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

// writeLiveFixtures writes live fixtures to disk.
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

// readLiveFixtures handles read live fixtures and returns [*liveFixtureState].
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
	// Older fixture snapshots sometimes persisted CleanupReleaseTag into
	// ReleaseSummaryTag; treat that value as unset so summary checks migrate to
	// the dedicated release-summary tag.
	if state.ReleaseSummaryTag == "" || state.ReleaseSummaryTag == liveFixtureCleanupTag {
		state.ReleaseSummaryTag = liveFixtureReleaseSummaryTag
	}
	if state.ElicitationReleaseTag == "" {
		state.ElicitationReleaseTag = liveFixtureElicitationTag
	}
	if fixtureErr := ensurePackageReleaseFixtureFiles(&state, path); fixtureErr != nil {
		return nil, fixtureErr
	}
	return &state, nil
}

// ensurePackageReleaseFixtureFiles creates local files used by the package-to-release workflow.
func ensurePackageReleaseFixtureFiles(state *liveFixtureState, fixturesPath string) error {
	if state == nil {
		return errors.New("package release fixture state is nil")
	}
	dir, err := packageReleaseFixtureDir(fixturesPath)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create package release fixture directory: %w", err)
	}
	names := make([]string, 0, len(packageReleaseFixtureFiles))
	paths := make([]string, 0, len(packageReleaseFixtureFiles))
	for _, file := range packageReleaseFixtureFiles {
		path := filepath.Join(dir, file.name)
		if writeErr := os.WriteFile(path, []byte(file.content), 0o600); writeErr != nil {
			return fmt.Errorf("write package release fixture file %s: %w", path, writeErr)
		}
		names = append(names, file.name)
		paths = append(paths, path)
	}
	state.PackageReleaseName = liveFixturePackageReleaseName
	state.PackageReleaseVersion = liveFixturePackageReleaseVersion
	state.PackageReleaseTag = liveFixturePackageReleaseTag
	state.PackageReleaseDir = dir
	state.PackageReleaseFiles = names
	state.PackageReleasePaths = paths
	return nil
}

// packageReleaseFixtureDir resolves the absolute local directory for package workflow files.
func packageReleaseFixtureDir(fixturesPath string) (string, error) {
	if fixturesPath == "" {
		fixturesPath = defaultFixtures
	}
	dir := filepath.Join(filepath.Dir(fixturesPath), "package-release-files")
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve package release fixture directory: %w", err)
	}
	return abs, nil
}

// prepare creates or updates the live fixture resources tracked by liveFixturePreparer.
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
	p.bestEffort(ctx, "project service account", p.ensureProjectServiceAccount)
	// Project alias is a hard dependency for MS-ENT-DYN-5; do not silence
	// failures here (a missing alias would silently produce a false
	// failure for that case with no diagnostic).
	if aliasErr := p.ensureProjectAlias(ctx); aliasErr != nil {
		p.notef("project alias fixture unavailable: %v", aliasErr)
	}
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

// bestEffort runs cleanup work for liveFixturePreparer without aborting fixture preparation.
func (p *liveFixturePreparer) bestEffort(ctx context.Context, name string, fn func(context.Context) error) {
	if err := fn(ctx); err != nil {
		p.notef("%s fixture unavailable: %v", name, err)
	}
}

// notef records a fixture preparation note for liveFixturePreparer.
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

// ensureGroup ensures group exists for liveFixturePreparer.
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

// ensureProject ensures project exists for liveFixturePreparer.
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

// ensureProjectServiceAccount seeds a Premium/Ultimate project service account
// and PAT used by Enterprise live evaluator tasks.
func (p *liveFixturePreparer) ensureProjectServiceAccount(ctx context.Context) error {
	account, foundAccount, err := p.findProjectServiceAccount(ctx)
	if err != nil {
		return err
	}
	if !foundAccount {
		username := fmt.Sprintf("%s-%d", liveFixtureProjectServiceAccountUsername, p.state.ProjectID)
		email := username + "@example.com"
		account, _, err = p.client.GL().Projects.CreateProjectServiceAccount(p.state.ProjectID, &gl.CreateProjectServiceAccountOptions{
			Name:     new(liveFixtureProjectServiceAccountName),
			Username: &username,
			Email:    &email,
		}, gl.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("create project service account: %w", err)
		}
	}
	p.state.ProjectServiceAccountID = account.ID
	token, foundToken, err := p.findProjectServiceAccountPAT(ctx, account.ID)
	if err != nil {
		return err
	}
	if !foundToken {
		scopes := []string{"api"}
		description := "Evaluation fixture token for project service-account scenarios"
		expiresAt := gl.ISOTime(time.Now().AddDate(0, 1, 0))
		token, _, err = p.client.GL().Projects.CreateProjectServiceAccountPersonalAccessToken(p.state.ProjectID, account.ID, &gl.CreateProjectServiceAccountPersonalAccessTokenOptions{
			Name:        new(liveFixtureProjectServiceAccountPATName),
			Scopes:      &scopes,
			Description: &description,
			ExpiresAt:   &expiresAt,
		}, gl.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("create project service account PAT: %w", err)
		}
	}
	p.state.ProjectServiceAccountTokenID = token.ID
	return nil
}

func (p *liveFixturePreparer) findProjectServiceAccount(ctx context.Context) (*gl.ProjectServiceAccount, bool, error) {
	accounts, _, err := p.client.GL().Projects.ListProjectServiceAccounts(p.state.ProjectID, &gl.ListProjectServiceAccountsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, false, fmt.Errorf("list project service accounts: %w", err)
	}
	usernamePrefix := fmt.Sprintf("%s-%d", liveFixtureProjectServiceAccountUsername, p.state.ProjectID)
	for _, account := range accounts {
		if account.Name == liveFixtureProjectServiceAccountName || strings.HasPrefix(account.Username, usernamePrefix) {
			return account, true, nil
		}
	}
	return nil, false, nil
}

func (p *liveFixturePreparer) findProjectServiceAccountPAT(ctx context.Context, serviceAccountID int64) (*gl.PersonalAccessToken, bool, error) {
	tokens, _, err := p.client.GL().Projects.ListProjectServiceAccountPersonalAccessTokens(p.state.ProjectID, serviceAccountID, &gl.ListProjectServiceAccountPersonalAccessTokensOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, false, fmt.Errorf("list project service account PATs: %w", err)
	}
	for _, token := range tokens {
		if token.Name == liveFixtureProjectServiceAccountPATName && token.Active && !token.Revoked {
			return token, true, nil
		}
	}
	return nil, false, nil
}

// ensureProjectAlias seeds a project alias (`e2e-enterprise-alias`) pointing
// at the fixture project. Used by enterprise read cases that resolve a
// project alias by name (e.g. MS-ENT-DYN-5). Admin-only API; best-effort.
func (p *liveFixturePreparer) ensureProjectAlias(ctx context.Context) error {
	const aliasName = "e2e-enterprise-alias"
	aliases, _, err := p.client.GL().ProjectAliases.ListProjectAliases(gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("list project aliases: %w", err)
	}
	for _, a := range aliases {
		if a.Name == aliasName {
			return nil
		}
	}
	_, _, err = p.client.GL().ProjectAliases.CreateProjectAlias(&gl.CreateProjectAliasOptions{
		Name:      new(aliasName),
		ProjectID: p.state.ProjectID,
	}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create project alias %q: %w", aliasName, err)
	}
	return nil
}

// ensureRepository ensures repository exists for liveFixturePreparer.
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

// ensureLabels ensures labels exists for liveFixturePreparer.
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

// ensureBranches ensures branches exists for liveFixturePreparer.
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

// ensureCoreIssues ensures core issues exists for liveFixturePreparer.
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

// ensureMergeRequests ensures merge requests exists for liveFixturePreparer.
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

// ensurePipeline ensures pipeline exists for liveFixturePreparer.
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

// ensureMilestone ensures milestone exists for liveFixturePreparer.
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

// ensureCleanupRelease ensures cleanup release exists for liveFixturePreparer.
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
	if p.state.ReleaseSummaryTag == "" {
		p.state.ReleaseSummaryTag = liveFixtureReleaseSummaryTag
	}
	if tagErr := p.ensureTag(ctx, p.state.ReleaseSummaryTag, p.defaultRef()); tagErr != nil {
		return tagErr
	}
	_, _, err = p.client.GL().Releases.GetRelease(p.state.ProjectID, p.state.ReleaseSummaryTag, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return err
	}
	_, _, err = p.client.GL().Releases.CreateRelease(p.state.ProjectID, &gl.CreateReleaseOptions{
		Name:        new("Evaluation release summary fixture"),
		TagName:     &p.state.ReleaseSummaryTag,
		Description: new("Fixture release for release-summary workflows."),
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	return nil
}

// ensureHooks ensures hooks exists for liveFixturePreparer.
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

// cleanupProjectHooks removes cleanup project hooks fixture resources for liveFixturePreparer when present.
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

// deleteEvaluationProjectHooks removes delete evaluation project hooks fixture resources for liveFixturePreparer when present.
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

// isEvaluationProjectHook reports whether a hook belongs to evaluator fixture cleanup.
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

// ensureBadge ensures badge exists for liveFixturePreparer.
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

// ensureSnippet ensures snippet exists for liveFixturePreparer.
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

// ensureEnvironment ensures environment exists for liveFixturePreparer.
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

// ensureProjectAccessToken ensures project access token exists for liveFixturePreparer.
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

// ensureCIVariables ensures CI variables exists for liveFixturePreparer.
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

// ignoreNotFound handles ignore not found for liveFixturePreparer.
func (p *liveFixturePreparer) ignoreNotFound(_ *gl.Response, err error) {
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		p.notef("cleanup warning: %v", err)
	}
}

// ensurePackage ensures package exists for liveFixturePreparer.
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

// ensureDeployKey ensures deploy key exists for liveFixturePreparer.
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

// ensureDeployToken ensures deploy token exists for liveFixturePreparer.
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

// ensurePipelineTriggers ensures pipeline triggers exists for liveFixturePreparer.
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

// ensurePipelineSchedules ensures pipeline schedules exists for liveFixturePreparer.
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

// cleanupPipelineSchedules removes cleanup pipeline schedules fixture resources for liveFixturePreparer when present.
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

// ensureDisposableRunner ensures disposable runner exists for liveFixturePreparer.
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

// ensureDisposableUser ensures disposable user exists for liveFixturePreparer.
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

// ensureFeatureFlag ensures feature flag exists for liveFixturePreparer.
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

// ensureWiki ensures wiki exists for liveFixturePreparer.
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

// ensureAwardEmoji ensures award emoji exists for liveFixturePreparer.
func (p *liveFixturePreparer) ensureAwardEmoji(ctx context.Context) error {
	if p.state.IssueIID > 0 {
		awardID, err := p.ensureIssueAwardEmoji(ctx, "thumbsup")
		if err != nil {
			return err
		}
		p.state.IssueAwardID = awardID
	}
	if p.state.MergeRequestIID > 0 {
		awardID, err := p.ensureMergeRequestAwardEmoji(ctx, "rocket")
		if err != nil {
			return err
		}
		p.state.MergeRequestAwardID = awardID
	}
	return nil
}

func (p *liveFixturePreparer) ensureIssueAwardEmoji(ctx context.Context, name string) (int64, error) {
	award, _, err := p.client.GL().AwardEmoji.CreateIssueAwardEmoji(p.state.ProjectID, p.state.IssueIID, &gl.CreateAwardEmojiOptions{Name: name}, gl.WithContext(ctx))
	if err == nil {
		return award.ID, nil
	}
	awards, _, listErr := p.client.GL().AwardEmoji.ListIssueAwardEmoji(p.state.ProjectID, p.state.IssueIID, &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{PerPage: 100}}, gl.WithContext(ctx))
	if listErr != nil {
		return 0, listErr
	}
	return findAwardEmojiID(awards, name), nil
}

func (p *liveFixturePreparer) ensureMergeRequestAwardEmoji(ctx context.Context, name string) (int64, error) {
	award, _, err := p.client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(p.state.ProjectID, p.state.MergeRequestIID, &gl.CreateAwardEmojiOptions{Name: name}, gl.WithContext(ctx))
	if err == nil {
		return award.ID, nil
	}
	awards, _, listErr := p.client.GL().AwardEmoji.ListMergeRequestAwardEmoji(p.state.ProjectID, p.state.MergeRequestIID, &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{PerPage: 100}}, gl.WithContext(ctx))
	if listErr != nil {
		return 0, listErr
	}
	return findAwardEmojiID(awards, name), nil
}

func findAwardEmojiID(awards []*gl.AwardEmoji, name string) int64 {
	for _, existing := range awards {
		if existing.Name == name {
			return existing.ID
		}
	}
	return 0
}

// ensureDiscussions ensures discussions exists for liveFixturePreparer.
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

// ensureCommitDiscussion ensures commit discussion exists for liveFixturePreparer.
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

// createIssue handles create issue for liveFixturePreparer.
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

// ensureFixtureMergeRequest ensures fixture merge request exists for liveFixturePreparer.
func (p *liveFixturePreparer) ensureFixtureMergeRequest(ctx context.Context, sourceBranch, title string, mergeFixture bool) (*gl.BasicMergeRequest, error) {
	defaultRef := p.defaultRef()
	if err := p.ensureBranch(ctx, sourceBranch, defaultRef); err != nil {
		return nil, err
	}
	filePath := strings.TrimPrefix(sourceBranch, "feature/") + ".txt"
	if err := p.ensureFile(ctx, filePath, sourceBranch, title+"\n", "Seed MR fixture file"); err != nil {
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

// waitForMergeRequestMergeable handles wait for merge request mergeable for liveFixturePreparer.
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

// closeOpenMergeRequestsForBranch handles close open merge requests for branch for liveFixturePreparer.
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

// ensureBranch ensures branch exists for liveFixturePreparer.
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

// waitForBranch handles wait for branch for liveFixturePreparer.
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

// ensureFile ensures file exists for liveFixturePreparer.
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
	if err != nil && !isFileAlreadyExistsError(err) {
		return fmt.Errorf("create file %s on %s: %w", path, branch, err)
	}
	return nil
}

func isFileAlreadyExistsError(err error) bool {
	return (toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusConflict)) &&
		toolutil.ContainsAny(err, "already exists", "file already exists")
}

// ensureTag ensures tag exists for liveFixturePreparer.
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

// waitForPipelineJobs handles wait for pipeline jobs for liveFixturePreparer.
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

// applyLiveFixtureState applies live fixture state transformations.
func applyLiveFixtureState(tasks []evalTask, state *liveFixtureState) []evalTask {
	if state == nil {
		return tasks
	}
	fixtureOutput := fixtureOutputFromLiveState(state)
	out := make([]evalTask, len(tasks))
	for i, task := range tasks {
		out[i] = task
		if task.Case == nil || task.Case.PromptTemplate.Text == "" {
			continue
		}
		renderedPrompt, err := RenderCasePrompt(*task.Case, fixtureOutput)
		if err != nil {
			continue
		}
		out[i].Prompt = renderedPrompt
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
		if task.ID == "MS-002" {
			return state.PipelineID > 0 && state.FailedJobID > 0
		}
		return state.FailedJobID > 0
	case "MT-064":
		return state.ManualJobID > 0
	case "MT-046", "MT-047":
		return state.RunnerID > 0
	case "MT-182", "MT-183", "MT-184", "MT-185", "MT-195", "MS-054":
		return state.ProjectServiceAccountID > 0
	case "MT-186", "MT-187":
		return state.ProjectServiceAccountID > 0 && state.ProjectServiceAccountTokenID > 0
	case "MS-008":
		return state.RunnerID > 0 && state.FailedJobID > 0
	default:
		return true
	}
}

// liveAttemptResourceSuffix returns attempt resource suffix for live evaluation runs.
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

// suffixEvaluationValue appends evaluation value to isolate live evaluation resources.
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

// shouldSuffixEvaluationValue reports whether should suffix evaluation value.
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

// fixtureRemoteURL returns remote URL fixture content.
func fixtureRemoteURL(baseURL, projectPath string) string {
	return strings.TrimRight(baseURL, "/") + "/" + projectPath + ".git"
}

// fixtureReadme returns readme fixture content.
func fixtureReadme() string {
	return "# GitLab MCP Server Evaluation Fixture\n\nThis repository is seeded by cmd/eval_mcp_surfaces for live MCP evaluation.\n\nfunc RegisterMCPMeta() {}\n\nTODO: keep evaluation coverage representative.\n"
}

// fixtureCI returns CI fixture content.
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

// pathBase returns the final path element without importing filepath for URL-style paths.
func pathBase(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

// isEmptyCommitError reports whether GitLab rejected a no-op file commit.
func isEmptyCommitError(err error) bool {
	return toolutil.ContainsAny(err, "commit was empty", "You are trying to update the file with the same content")
}

// isMissingFileUpdateError reports GitLab update errors caused by a missing file.
func isMissingFileUpdateError(err error) bool {
	return toolutil.ContainsAny(err, "A file with this name doesn't exist", "file does not exist")
}

// newAuthorizedSSHKey handles new authorized SSH key and returns [string].
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
