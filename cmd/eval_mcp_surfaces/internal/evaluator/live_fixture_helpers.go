package evaluator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func ensureLiveProjectActive(ctx context.Context, client *gitlabclient.Client) error {
	if client == nil {
		return nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	project, _, err := client.GL().Projects.GetProject(liveFixtureProjectPath, nil, gl.WithContext(setupCtx))
	if err != nil {
		return fmt.Errorf("get project %s: %w", liveFixtureProjectPath, err)
	}
	if !project.Archived {
		return nil
	}
	if _, _, unarchiveErr := client.GL().Projects.UnarchiveProject(project.ID, gl.WithContext(setupCtx)); unarchiveErr != nil {
		return fmt.Errorf("unarchive project %s: %w", liveFixtureProjectPath, unarchiveErr)
	}
	return nil
}

func createLiveGroupServiceAccountPAT(ctx context.Context, client *gitlabclient.Client, taskID string) (accountID, tokenID int64, err error) {
	accountID, suffix, err := createLiveGroupServiceAccount(ctx, client, taskID)
	if err != nil {
		return 0, 0, err
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	scopes := []string{"api"}
	tokenName := "eval-group-service-token-" + suffix
	pat, _, err := client.GL().Groups.CreateServiceAccountPersonalAccessToken(liveFixtureGroupPath, accountID, &gl.CreateServiceAccountPersonalAccessTokenOptions{
		Name:   &tokenName,
		Scopes: &scopes,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return 0, 0, fmt.Errorf("prepare %s fixture group service account PAT: %w", taskID, err)
	}
	return accountID, pat.ID, nil
}

func createLiveGroupServiceAccount(ctx context.Context, client *gitlabclient.Client, taskID string) (accountID int64, suffix string, err error) {
	if client == nil {
		return 0, "", errors.New("group service account fixture requires GitLab client")
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	suffix = liveUniqueSuffix()
	accountName := "eval-group-service-account-" + suffix
	username := "eval-group-svc-" + suffix
	account, _, err := client.GL().Groups.CreateServiceAccount(liveFixtureGroupPath, &gl.CreateServiceAccountOptions{
		Name:     &accountName,
		Username: &username,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return 0, "", fmt.Errorf("prepare %s fixture group service account: %w", taskID, err)
	}
	return account.ID, suffix, nil
}

func createLiveTemporaryProject(ctx context.Context, client *gitlabclient.Client, prefix string) (*gl.Project, error) {
	toolsGroup, _, err := client.GL().Groups.GetGroup(liveFixtureToolsPath, nil, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get tools group %s: %w", liveFixtureToolsPath, err)
	}
	visibility := gl.PrivateVisibility
	approvalsBeforeMerge := int64(0)
	mergePipelinesEnabled := false
	onlyAllowMergeIfAllDiscussionsAreResolved := false
	onlyAllowMergeIfAllStatusChecksPassed := false
	allowMergeOnSkippedPipeline := true
	var lastErr error
	for range 5 {
		path := fmt.Sprintf("eval-%s-%s", prefix, liveUniqueSuffix())
		project, _, createErr := client.GL().Projects.CreateProject(&gl.CreateProjectOptions{
			Name:                  new(path),
			Path:                  new(path),
			NamespaceID:           new(toolsGroup.ID),
			InitializeWithReadme:  new(true),
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
		lastErr = fmt.Errorf("create project %s/%s: %w", liveFixtureToolsPath, path, createErr)
		if !temporaryProjectNameTaken(createErr) {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func temporaryProjectNameTaken(err error) bool {
	return (toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusConflict)) &&
		toolutil.ContainsAny(err, "has already been taken")
}

func waitForLiveMergeRequestReady(ctx context.Context, client *gitlabclient.Client, projectID string, mergeRequestIID int64) error {
	deadline := time.Now().Add(4 * time.Minute)
	lastStatus := "unknown"
	for time.Now().Before(deadline) {
		mergeRequest, err := getLiveMergeRequestWithStatusRecheck(ctx, client, projectID, mergeRequestIID)
		if err != nil {
			return fmt.Errorf("prepare fixture MR !%d: %w", mergeRequestIID, err)
		}
		lastStatus = mergeRequest.DetailedMergeStatus
		if mergeRequest.DetailedMergeStatus == "mergeable" {
			return nil
		}
		if mergeRequest.DetailedMergeStatus == "approvals_syncing" && liveMergeRequestApprovalsSatisfied(ctx, client, projectID, mergeRequestIID) {
			return nil
		}
		if !liveMergeStatusStillPreparing(mergeRequest.DetailedMergeStatus) {
			return fmt.Errorf("prepare fixture MR !%d is not mergeable: %s", mergeRequestIID, mergeRequest.DetailedMergeStatus)
		}
		if waitErr := waitForContext(ctx, 2*time.Second); waitErr != nil {
			return waitErr
		}
	}
	return fmt.Errorf("prepare fixture MR !%d did not become mergeable before timeout; last status %s", mergeRequestIID, lastStatus)
}

func liveMergeRequestApprovalsSatisfied(ctx context.Context, client *gitlabclient.Client, projectID string, mergeRequestIID int64) bool {
	configuration, _, err := client.GL().MergeRequestApprovals.GetConfiguration(projectID, mergeRequestIID, gl.WithContext(ctx))
	if err == nil && configuration != nil {
		return configuration.ApprovalsLeft == 0 && configuration.ApprovalsRequired == 0
	}
	state, _, err := client.GL().MergeRequestApprovals.GetApprovalState(projectID, mergeRequestIID, gl.WithContext(ctx))
	if err != nil || state == nil {
		return false
	}
	for _, rule := range state.Rules {
		if rule != nil && rule.ApprovalsRequired > 0 && !rule.Approved {
			return false
		}
	}
	return true
}

func getLiveMergeRequestWithStatusRecheck(ctx context.Context, client *gitlabclient.Client, projectID string, mergeRequestIID int64) (*gl.BasicMergeRequest, error) {
	recheck := true
	iids := []int64{mergeRequestIID}
	mergeRequests, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		IIDs:                   &iids,
		WithMergeStatusRecheck: &recheck,
		ListOptions:            gl.ListOptions{PerPage: 1},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if len(mergeRequests) == 0 {
		return nil, fmt.Errorf("merge request !%d not found", mergeRequestIID)
	}
	return mergeRequests[0], nil
}

func liveMergeStatusStillPreparing(status string) bool {
	switch status {
	case "", "checking", "unchecked", "preparing", "ci_still_running", "approvals_syncing":
		return true
	default:
		return false
	}
}

func ensureLiveBranchExists(ctx context.Context, client *gitlabclient.Client, projectID, branch, ref string) error {
	_, _, err := client.GL().Branches.GetBranch(projectID, branch, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return err
	}
	_, _, err = client.GL().Branches.CreateBranch(projectID, &gl.CreateBranchOptions{
		Branch: &branch,
		Ref:    &ref,
	}, gl.WithContext(ctx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return err
	}
	return nil
}

func createLiveMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, projectID string, mergeRequestIID int64) (int64, error) {
	for _, name := range liveAwardEmojiNames() {
		emoji, _, createErr := client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(projectID, mergeRequestIID, &gl.CreateAwardEmojiOptions{Name: name}, gl.WithContext(ctx))
		if createErr == nil {
			return emoji.ID, nil
		}
		if !toolutil.IsHTTPStatus(createErr, http.StatusBadRequest) && !toolutil.IsHTTPStatus(createErr, http.StatusConflict) {
			return 0, createErr
		}
	}
	return 0, errors.New("no merge request award emoji available after create attempts")
}

func createLiveIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, projectID string, issueIID int64) (int64, error) {
	for _, name := range liveAwardEmojiNames() {
		emoji, _, createErr := client.GL().AwardEmoji.CreateIssueAwardEmoji(projectID, issueIID, &gl.CreateAwardEmojiOptions{Name: name}, gl.WithContext(ctx))
		if createErr == nil {
			return emoji.ID, nil
		}
		if !toolutil.IsHTTPStatus(createErr, http.StatusBadRequest) && !toolutil.IsHTTPStatus(createErr, http.StatusConflict) {
			return 0, createErr
		}
	}
	return 0, errors.New("no issue award emoji available after create attempts")
}

func liveAwardEmojiNames() []string {
	return []string{"thumbsup", "thumbsdown", "rocket", "eyes", "heart", "tada"}
}

func waitForFailedJob(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64) (int64, error) {
	return waitForPipelineJobStatus(ctx, client, projectID, pipelineID, "failed", "prepare failed-job fixture jobs", "prepare failed-job fixture failed job")
}

func waitForPipelineJobStatus(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64, targetStatus, listContext, notFoundContext string) (int64, error) {
	deadline := time.Now().Add(4 * time.Minute)
	var lastStatuses []string
	for time.Now().Before(deadline) {
		jobs, _, err := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, &gl.ListJobsOptions{ListOptions: gl.ListOptions{PerPage: 100}}, gl.WithContext(ctx))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", listContext, err)
		}
		lastStatuses = lastStatuses[:0]
		for _, job := range jobs {
			lastStatuses = append(lastStatuses, fmt.Sprintf("%s:%s", job.Name, job.Status))
			if job.Status == targetStatus {
				return job.ID, nil
			}
		}
		if waitErr := waitForContext(ctx, 5*time.Second); waitErr != nil {
			return 0, waitErr
		}
	}
	return 0, fmt.Errorf("%s not found for pipeline %d; last statuses: %s", notFoundContext, pipelineID, strings.Join(lastStatuses, ", "))
}

func terraformStateLockEndpoint(baseURL *url.URL, projectID, stateName string) string {
	root := strings.TrimRight(baseURL.String(), "/")
	return root + "/api/v4/projects/" + url.PathEscape(projectID) + "/terraform/state/" + url.PathEscape(stateName) + "/lock"
}

func terraformStateUnlockProjectID(prompt string) (string, bool) {
	if value, ok := backtickValueAfter(prompt, " in project "); ok {
		return value, true
	}
	return exampleProjectIDValue(prompt)
}

func liveGitLabHTTPClient() (*http.Client, error) {
	skipTLSVerify, err := gitlabSkipTLSVerify()
	if err != nil {
		return nil, err
	}
	if !skipTLSVerify {
		return http.DefaultClient, nil
	}
	return &http.Client{Transport: gitlabclient.HTTPTransport(true)}, nil
}

func gitlabSkipTLSVerify() (bool, error) {
	value := strings.TrimSpace(os.Getenv("GITLAB_SKIP_TLS_VERIFY"))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid GITLAB_SKIP_TLS_VERIFY %q: %w", value, err)
	}
	return parsed, nil
}

func liveDockerGitLabBaseURL() (*url.URL, error) {
	rawURL := strings.TrimRight(os.Getenv("GITLAB_URL"), "/")
	if rawURL == "" {
		return nil, errors.New("prepare terraform fixture requires GITLAB_URL")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("prepare terraform fixture GitLab URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("prepare terraform fixture GitLab URL has unsupported scheme %q", parsedURL.Scheme)
	}
	if parsedURL.Host == "" || parsedURL.User != nil {
		return nil, errors.New("prepare terraform fixture GitLab URL must include a host and no credentials")
	}
	return parsedURL, nil
}

func liveRemoteMirrorTargetURL(project *gl.Project) (string, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return "", errors.New("prepare mirror fixture requires GITLAB_TOKEN")
	}
	baseURL := strings.TrimRight(os.Getenv("E2E_GITLAB_INTERNAL_URL"), "/")
	if baseURL == "" {
		baseURL = "http://gitlab-e2e"
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("prepare mirror fixture internal URL: %w", err)
	}
	projectPath := strings.TrimPrefix(project.PathWithNamespace, "/")
	if projectPath == "" {
		return "", errors.New("prepare mirror fixture target project path is empty")
	}
	parsedURL.User = url.UserPassword("oauth2", token)
	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + "/" + projectPath + ".git"
	return parsedURL.String(), nil
}

func backtickValueAfter(text, marker string) (string, bool) {
	_, remaining, found := strings.Cut(text, marker)
	if !found {
		return "", false
	}
	_, remaining, found = strings.Cut(remaining, "`")
	if !found {
		return "", false
	}
	value, _, found := strings.Cut(remaining, "`")
	if !found {
		return "", false
	}
	return value, true
}

func optionalEnvironmentScopeFromPrompt(prompt string) (string, bool) {
	for _, marker := range []string{"environment_scope ", "environment scope "} {
		if environmentScope, ok := backtickValueAfter(prompt, marker); ok {
			if environmentScope = strings.TrimSpace(environmentScope); environmentScope != "" {
				return environmentScope, true
			}
		}
	}
	if strings.Contains(strings.ToLower(prompt), "production scope") {
		return "production", true
	}
	return "", false
}

func replaceAllPromptBacktickValuesAfter(prompt, marker string, value any) (string, error) {
	if _, ok := backtickValueAfter(prompt, marker); !ok {
		return prompt, fmt.Errorf("backtick value after %q not found in prompt %q", marker, prompt)
	}
	var out strings.Builder
	for {
		before, remaining, ok := strings.Cut(prompt, marker+"`")
		if !ok {
			out.WriteString(prompt)
			return out.String(), nil
		}
		out.WriteString(before)
		fmt.Fprintf(&out, "%s`%v`", marker, value)
		_, after, ok := strings.Cut(remaining, "`")
		if !ok {
			return "", fmt.Errorf("unterminated backtick value after %q in prompt %q", marker, prompt)
		}
		prompt = after
	}
}

func callFixtureSetupTool(ctx context.Context, session *mcp.ClientSession, toolSurface, action string, params map[string]any, ignoredErrors ...string) error {
	toolName, arguments := fixtureSetupToolEnvelope(toolSurface, "gitlab", action, params)
	result, err := callFixtureSetupToolByName(ctx, session, toolName, arguments)
	if err != nil && !isDynamicEvalSurface(toolSurface) && strings.Contains(strings.ToLower(err.Error()), "unknown tool \"gitlab\"") {
		if fallbackToolName, splitAction, ok := splitFixtureSetupAction(action); ok {
			_, arguments = fixtureSetupToolEnvelope(toolSurface, fallbackToolName, splitAction, params)
			result, err = callFixtureSetupToolByName(ctx, session, fallbackToolName, arguments)
		}
	}
	if err != nil {
		return fmt.Errorf("prepare fixture %s: %w", action, err)
	}
	if result == nil || !result.IsError {
		return nil
	}
	text := callToolResultText(result)
	lowerText := strings.ToLower(text)
	for _, ignored := range ignoredErrors {
		if strings.Contains(lowerText, strings.ToLower(ignored)) {
			return nil
		}
	}
	return fmt.Errorf("prepare fixture %s: %s", action, text)
}

func fixtureSetupToolEnvelope(toolSurface, toolName, action string, params map[string]any) (targetTool string, arguments map[string]any) {
	arguments = map[string]any{
		"action": action,
		"params": params,
	}
	if isDynamicEvalSurface(toolSurface) {
		return dynamicExecuteActionTool, arguments
	}
	return toolName, arguments
}

func callFixtureSetupToolByName(ctx context.Context, session *mcp.ClientSession, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	return session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
}

func splitFixtureSetupAction(action string) (toolName, splitAction string, ok bool) {
	domain, route, ok := strings.Cut(action, ".")
	if !ok || domain == "" || route == "" {
		return "", "", false
	}
	return "gitlab_" + domain, strings.ReplaceAll(route, ".", "_"), true
}

func safeFixturePathPart(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('-')
	}
	return strings.Trim(out.String(), "-")
}
