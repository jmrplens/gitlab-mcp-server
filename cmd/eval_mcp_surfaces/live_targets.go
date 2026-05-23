package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	customemoji "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

type liveAttemptResourceHandler func(context.Context, *gitlabclient.Client, *mcp.ClientSession, evalTask, string) (evalTask, error)

var liveAttemptResourceHandlers = map[string]liveAttemptResourceHandler{
	"MT-008": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveSubgroupDeleteTarget(ctx, client, task)
	},
	"MT-013": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveIssueDeleteTarget(ctx, client, task)
	},
	"MT-017": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveMergeRequestMergeTarget(ctx, client, task)
	},
	"MT-027": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveProjectVariableUpdateTarget(ctx, client, task.Prompt)
	},
	"MT-028": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveProjectVariableDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-015": func(ctx context.Context, _ *gitlabclient.Client, session *mcp.ClientSession, task evalTask, toolSurface string) (evalTask, error) {
		if session == nil {
			return task, errors.New("prepare MT-015 fixture requires an MCP session")
		}
		return task, ensureLiveMergeRequestSource(ctx, session, task.Prompt, toolSurface)
	},
	"MT-081": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveInteractiveMergeRequestTarget(ctx, client, task.Prompt)
	},
	"MT-083": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveInteractiveReleaseTarget(ctx, client, task.Prompt)
	},
	"MT-031": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveRepositoryFileDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-035": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveMilestoneDeleteTarget(ctx, client, task)
	},
	"MT-037": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveReleaseDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-044": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePackageDeleteTarget(ctx, client, task)
	},
	"MT-049": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveEnvironmentStopTarget(ctx, client, task)
	},
	"MT-054": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveBroadcastMessageDeleteTarget(ctx, client, task)
	},
	"MT-063": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveDraftNotePublishAllTarget(ctx, client, task)
	},
	"MT-066": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveJobTokenScopeRemoveProjectTarget(ctx, client, task)
	},
	"MT-107": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveCustomEmojiDeleteTarget(ctx, client, task)
	},
	"MT-114": func(ctx context.Context, _ *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveTerraformStateUnlockTarget(ctx, task)
	},
	"MT-116": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveMirrorForcePushTarget(ctx, client, task)
	},
	"MT-192": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePushRuleTarget(ctx, client, task, "MT-192", false)
	},
	"MT-193": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePushRuleTarget(ctx, client, task, "MT-193", true)
	},
	"MT-196": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePushRuleTarget(ctx, client, task, "MT-196", true)
	},
	"MT-197": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveGroupServiceAccountPATRevokeTarget(ctx, client, task)
	},
	"MT-198": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveGroupServiceAccountDeleteTarget(ctx, client, task)
	},
	"MS-004": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveReleaseDeleteTarget(ctx, client, task.Prompt)
	},
	"MS-007": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePackageDeleteTarget(ctx, client, task)
	},
	"MS-013": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveFeatureFlagDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-047": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveRunnerRemoveTarget(ctx, client, task)
	},
	"MT-051": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveSnippetDeleteTarget(ctx, client, task)
	},
	"MT-057": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveHookDeleteTarget(ctx, client, task)
	},
	"MT-023": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveFailedJobTarget(ctx, client, task)
	},
	"MT-024": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveFailedJobTarget(ctx, client, task)
	},
	"MT-065": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveFailedJobTarget(ctx, client, task)
	},
	"MT-059": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveBadgeDeleteTarget(ctx, client, task)
	},
	"MT-099": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveBranchDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-100": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveTagDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-101": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePipelineDeleteTarget(ctx, client, task)
	},
	"MT-102": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePipelineTriggerDeleteTarget(ctx, client, task)
	},
	"MT-103": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLivePipelineScheduleDeleteTarget(ctx, client, task)
	},
	"MT-106": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveFeatureFlagDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-108": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveWikiDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-109": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveMRAwardDeleteTarget(ctx, client, task)
	},
	"MT-110": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveIssueAwardDeleteTarget(ctx, client, task)
	},
	"MT-111": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveDeployKeyDeleteTarget(ctx, client, task)
	},
	"MT-112": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveDeployTokenDeleteTarget(ctx, client, task)
	},
	"MT-113": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveCommitDiscussionNoteDeleteTarget(ctx, client, task)
	},
	"MS-034": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveProjectMemberAbsent(ctx, client, task.Prompt)
	},
	"MT-068": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, cleanupLiveInstanceVariables(ctx, client, "INSTANCE_EVAL_TOKEN")
	},
	"MT-069": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return task, ensureLiveInstanceVariableDeleteTarget(ctx, client, task.Prompt)
	},
	"MT-064": func(ctx context.Context, client *gitlabclient.Client, _ *mcp.ClientSession, task evalTask, _ string) (evalTask, error) {
		return ensureLiveManualJob(ctx, client, task)
	},
}

func ensureLiveAttemptResources(ctx context.Context, client *gitlabclient.Client, session *mcp.ClientSession, task evalTask, toolSurface string) (evalTask, error) {
	if handler, ok := liveAttemptResourceHandlers[task.ID]; ok {
		return handler(ctx, client, session, task, toolSurface)
	}
	return task, nil
}

// ensureLiveProjectActive ensures live project active exists for live evaluation.
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

// ensureLiveSubgroupDeleteTarget handles ensure live subgroup delete target and returns [evalTask].
func ensureLiveSubgroupDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	parent, _, err := client.GL().Groups.GetGroup(liveFixtureGroupPath, nil, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-008 fixture parent group: %w", err)
	}
	path := "eval-temp-" + liveUniqueSuffix()
	visibility := gl.PrivateVisibility
	group, _, err := client.GL().Groups.CreateGroup(&gl.CreateGroupOptions{
		Name:       new(path),
		Path:       new(path),
		ParentID:   new(parent.ID),
		Visibility: &visibility,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-008 fixture subgroup: %w", err)
	}
	groupID := group.FullPath
	if groupID == "" {
		groupID = fmt.Sprintf("%s/%s", liveFixtureGroupPath, path)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "subgroup ", groupID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveIssueDeleteTarget handles ensure live issue delete target and returns [evalTask].
func ensureLiveIssueDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	_, issue, err := createLiveEvaluationIssue(ctx, client, task, "MT-013", "Evaluation issue safe to delete "+liveUniqueSuffix(), "Temporary issue for destructive evaluator coverage.")
	if err != nil {
		return task, err
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerIssue, issue.IID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

func createLiveEvaluationIssue(ctx context.Context, client *gitlabclient.Client, task evalTask, taskID, title, description string) (string, *gl.Issue, error) {
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return "", nil, fmt.Errorf("prepare %s fixture: project path not found in prompt %q", taskID, task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	issue, _, err := client.GL().Issues.CreateIssue(projectID, &gl.CreateIssueOptions{
		Title:       &title,
		Description: &description,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return "", nil, fmt.Errorf("prepare %s fixture issue: %w", taskID, err)
	}
	return projectID, issue, nil
}

// ensureLiveMergeRequestMergeTarget creates a mergeable MR in a disposable project and rewrites MT-017 to use it.
func ensureLiveMergeRequestMergeTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	project, err := createLiveTemporaryProject(setupCtx, client, "merge-mr")
	if err != nil {
		return task, fmt.Errorf("prepare MT-017 fixture project: %w", err)
	}
	projectID := project.PathWithNamespace
	targetBranch := project.DefaultBranch
	if targetBranch == "" {
		targetBranch = liveFixtureDefaultRef
	}
	sourceBranch := "eval-merge-" + liveUniqueSuffix()
	if branchErr := ensureLiveBranchExists(setupCtx, client, projectID, sourceBranch, targetBranch); branchErr != nil {
		return task, fmt.Errorf("prepare MT-017 fixture branch: %w", branchErr)
	}
	ciContent := "stages:\n  - test\n\nvariables:\n  GIT_STRATEGY: none\n\neval-pass:\n  stage: test\n  script:\n    - echo evaluation merge fixture\n"
	_, _, err = client.GL().RepositoryFiles.CreateFile(projectID, ".gitlab-ci.yml", &gl.CreateFileOptions{
		Branch:        &sourceBranch,
		Content:       &ciContent,
		CommitMessage: new("Seed passing CI for merge evaluation"),
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return task, fmt.Errorf("prepare MT-017 fixture CI: %w", err)
	}
	filePath := fmt.Sprintf("tmp/eval-merge-%s.txt", liveUniqueSuffix())
	_, _, err = client.GL().RepositoryFiles.CreateFile(projectID, filePath, &gl.CreateFileOptions{
		Branch:        &sourceBranch,
		Content:       new("evaluation merge request fixture\n"),
		CommitMessage: new("Seed merge request evaluation fixture"),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-017 fixture file: %w", err)
	}
	removeSource := false
	mergeRequest, _, err := client.GL().MergeRequests.CreateMergeRequest(projectID, &gl.CreateMergeRequestOptions{
		SourceBranch:       &sourceBranch,
		TargetBranch:       &targetBranch,
		Title:              new("Evaluation merge target"),
		RemoveSourceBranch: &removeSource,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-017 fixture merge request: %w", err)
	}
	pipeline, _, err := client.GL().Pipelines.CreatePipeline(projectID, &gl.CreatePipelineOptions{Ref: &sourceBranch}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-017 fixture pipeline: %w", err)
	}
	if waitErr := waitForLivePipelineStatus(setupCtx, client, projectID, pipeline.ID, "success"); waitErr != nil {
		return task, waitErr
	}
	if waitErr := waitForLiveMergeRequestReady(setupCtx, client, projectID, mergeRequest.IID); waitErr != nil {
		return task, waitErr
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerProject, projectID)
	if err != nil {
		return task, err
	}
	prompt, err = replacePromptBacktickValueAfter(prompt, "merge request ", mergeRequest.IID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveDraftNotePublishAllTarget creates an MR draft note and rewrites MT-063 to publish it.
func ensureLiveDraftNotePublishAllTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	projectID, mergeRequestIID, err := createLiveDraftNoteMergeRequest(setupCtx, client)
	if err != nil {
		return task, fmt.Errorf("prepare MT-063 fixture merge request: %w", err)
	}
	note := "Draft note ready for evaluator publish_all coverage."
	_, _, err = client.GL().DraftNotes.CreateDraftNote(projectID, mergeRequestIID, &gl.CreateDraftNoteOptions{
		Note: &note,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-063 fixture draft note: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerProject, projectID)
	if err != nil {
		return task, err
	}
	prompt, err = replacePromptMergeRequestIID(prompt, mergeRequestIID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// createLiveDraftNoteMergeRequest creates a small disposable MR suitable for draft-note lifecycle tasks.
func createLiveDraftNoteMergeRequest(ctx context.Context, client *gitlabclient.Client) (projectID string, mergeRequestIID int64, err error) {
	project, err := createLiveTemporaryProject(ctx, client, "draft-note")
	if err != nil {
		return "", 0, err
	}
	projectID = project.PathWithNamespace
	targetBranch := project.DefaultBranch
	if targetBranch == "" {
		targetBranch = liveFixtureDefaultRef
	}
	sourceBranch := "eval-draft-note-" + liveUniqueSuffix()
	if branchErr := ensureLiveBranchExists(ctx, client, projectID, sourceBranch, targetBranch); branchErr != nil {
		return "", 0, branchErr
	}
	filePath := fmt.Sprintf("tmp/eval-draft-note-%s.txt", liveUniqueSuffix())
	_, _, err = client.GL().RepositoryFiles.CreateFile(projectID, filePath, &gl.CreateFileOptions{
		Branch:        &sourceBranch,
		Content:       new("evaluation draft note fixture\n"),
		CommitMessage: new("Seed draft note evaluation fixture"),
	}, gl.WithContext(ctx))
	if err != nil {
		return "", 0, err
	}
	mergeRequest, _, err := client.GL().MergeRequests.CreateMergeRequest(projectID, &gl.CreateMergeRequestOptions{
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
		Title:        new("Evaluation draft note target"),
	}, gl.WithContext(ctx))
	if err != nil {
		return "", 0, err
	}
	return projectID, mergeRequest.IID, nil
}

// replacePromptMergeRequestIID handles replace prompt merge request IID and returns [string].
func replacePromptMergeRequestIID(prompt string, mergeRequestIID int64) (string, error) {
	prompt, err := replacePromptBacktickValueAfter(prompt, promptMarkerMergeRequest, mergeRequestIID)
	if err == nil {
		return prompt, nil
	}
	return replacePromptBacktickValueAfter(prompt, "MR ", mergeRequestIID)
}

// ensureLiveCustomEmojiDeleteTarget creates a disposable custom emoji and rewrites MT-107 to delete it.
func ensureLiveCustomEmojiDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	groupPath, err := createLiveTemporaryGroup(setupCtx, client, "emoji")
	if err != nil {
		return task, fmt.Errorf("prepare MT-107 fixture group: %w", err)
	}
	emojiURL := strings.TrimRight(os.Getenv("E2E_FIXTURE_URL"), "/") + "/emoji.png"
	if emojiURL == "/emoji.png" {
		emojiURL = "http://e2e-fixture:8080/emoji.png"
	}
	created, err := customemoji.Create(setupCtx, client, customemoji.CreateInput{
		GroupPath: groupPath,
		Name:      "eval_delete_" + liveUniqueSuffix(),
		URL:       emojiURL,
	})
	if err != nil {
		return task, fmt.Errorf("prepare MT-107 fixture emoji: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "custom emoji GID ", created.Emoji.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveTerraformStateUnlockTarget locks a state through the Terraform HTTP backend and rewrites MT-114.
func ensureLiveTerraformStateUnlockTarget(ctx context.Context, task evalTask) (evalTask, error) {
	projectID, ok := terraformStateUnlockProjectID(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-114 fixture: project path not found in prompt %q", task.Prompt)
	}
	stateName := "eval-unlock-" + liveUniqueSuffix()
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := createLiveTerraformStateLock(setupCtx, projectID, stateName); err != nil {
		return task, err
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "state ", stateName)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// terraformStateUnlockProjectID handles terraform state unlock project ID and returns [string].
func terraformStateUnlockProjectID(prompt string) (string, bool) {
	if value, ok := backtickValueAfter(prompt, " in project "); ok {
		return value, true
	}
	return exampleProjectIDValue(prompt)
}

// ensureLiveMirrorForcePushTarget creates a push mirror and rewrites MT-116 to use it.
func ensureLiveMirrorForcePushTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	source, err := createLiveTemporaryProject(setupCtx, client, "mirror-source")
	if err != nil {
		return task, fmt.Errorf("prepare MT-116 fixture source project: %w", err)
	}
	target, err := createLiveTemporaryProject(setupCtx, client, "mirror-target")
	if err != nil {
		return task, fmt.Errorf("prepare MT-116 fixture target project: %w", err)
	}
	mirrorURL, err := liveRemoteMirrorTargetURL(target)
	if err != nil {
		return task, err
	}
	enabled := true
	authMethod := "password"
	mirror, _, err := client.GL().ProjectMirrors.AddProjectMirror(source.PathWithNamespace, &gl.AddProjectMirrorOptions{
		URL:        &mirrorURL,
		Enabled:    &enabled,
		AuthMethod: &authMethod,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-116 fixture mirror add: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerProject, source.PathWithNamespace)
	if err != nil {
		return task, err
	}
	prompt, err = replacePromptBacktickValueAfter(prompt, "mirror ID ", mirror.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

func ensureLivePushRuleTarget(ctx context.Context, client *gitlabclient.Client, task evalTask, taskID string, seedRule bool) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	project, err := createLiveTemporaryProject(setupCtx, client, "push-rule")
	if err != nil {
		return task, fmt.Errorf("prepare %s fixture project: %w", taskID, err)
	}
	if seedRule {
		commitMessageRegex := ".*"
		_, _, err = client.GL().Projects.AddProjectPushRule(project.PathWithNamespace, &gl.AddProjectPushRuleOptions{CommitMessageRegex: &commitMessageRegex}, gl.WithContext(setupCtx))
		if err != nil {
			return task, fmt.Errorf("prepare %s fixture push rule: %w", taskID, err)
		}
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerProject, project.PathWithNamespace)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

func ensureLiveGroupServiceAccountPATRevokeTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	accountID, tokenID, err := createLiveGroupServiceAccountPAT(ctx, client, "MT-197")
	if err != nil {
		return task, err
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "service account user ID ", accountID)
	if err != nil {
		return task, err
	}
	prompt, err = replacePromptBacktickValueAfter(prompt, "PAT ID ", tokenID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

func ensureLiveGroupServiceAccountDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	accountID, _, err := createLiveGroupServiceAccount(ctx, client, "MT-198")
	if err != nil {
		return task, err
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "service account user ID ", accountID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
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
		return 0, "", nil
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

// ensureLiveProjectVariableUpdateTarget ensures live project variable update target exists for live evaluation.
func ensureLiveProjectVariableUpdateTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	return ensureLiveProjectVariableTarget(ctx, client, prompt, "MT-027", "masked-value-123")
}

// ensureLiveProjectVariableDeleteTarget ensures live project variable delete target exists for live evaluation.
func ensureLiveProjectVariableDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	return ensureLiveProjectVariableTarget(ctx, client, prompt, "MT-028", "masked-value-456")
}

// ensureLiveInstanceVariableDeleteTarget ensures live instance variable delete target exists for live evaluation.
func ensureLiveInstanceVariableDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	key, ok := backtickValueAfter(prompt, "instance CI variable ")
	if !ok {
		key, ok = backtickValueAfter(prompt, "instance variable ")
	}
	if !ok {
		return fmt.Errorf("prepare MT-069 fixture: instance variable key not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, err := client.GL().InstanceVariables.RemoveVariable(key, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-069 fixture variable cleanup %s: %w", key, err)
	}
	value := "masked-value-456"
	_, _, err = client.GL().InstanceVariables.CreateVariable(&gl.CreateInstanceVariableOptions{
		Key:   &key,
		Value: &value,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return fmt.Errorf("prepare MT-069 fixture variable %s: %w", key, err)
	}
	return nil
}

// ensureLiveProjectVariableTarget ensures live project variable target exists for live evaluation.
func ensureLiveProjectVariableTarget(ctx context.Context, client *gitlabclient.Client, prompt, taskID, value string) error {
	if client == nil {
		return nil
	}
	projectID, ok := exampleProjectIDValue(prompt)
	if !ok {
		return fmt.Errorf("prepare %s fixture: project path not found in prompt %q", taskID, prompt)
	}
	key, ok := backtickValueAfter(prompt, "CI variable ")
	if !ok {
		return fmt.Errorf("prepare %s fixture: variable key not found in prompt %q", taskID, prompt)
	}
	environmentScope := projectVariableEnvironmentScope(prompt)
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for _, scope := range []string{"*", environmentScope} {
		_, err := client.GL().ProjectVariables.RemoveVariable(projectID, key, &gl.RemoveProjectVariableOptions{
			Filter: &gl.VariableFilter{EnvironmentScope: scope},
		}, gl.WithContext(setupCtx))
		if err != nil && !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return fmt.Errorf("prepare %s fixture variable cleanup %s/%s: %w", taskID, key, scope, err)
		}
	}
	_, _, err := client.GL().ProjectVariables.CreateVariable(projectID, &gl.CreateProjectVariableOptions{
		Key:              &key,
		Value:            &value,
		EnvironmentScope: &environmentScope,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return fmt.Errorf("prepare %s fixture variable %s: %w", taskID, key, err)
	}
	return nil
}

// projectVariableEnvironmentScope extracts the environment scope expected by a variable task prompt.
func projectVariableEnvironmentScope(prompt string) string {
	if environmentScope, ok := optionalEnvironmentScopeFromPrompt(prompt); ok {
		return environmentScope
	}
	return "*"
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

// ensureLiveRepositoryFileDeleteTarget ensures live repository file delete target exists for live evaluation.
func ensureLiveRepositoryFileDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := exampleProjectIDValue(prompt)
	if !ok {
		return fmt.Errorf("prepare MT-031 fixture: project path not found in prompt %q", prompt)
	}
	filePath, ok := backtickValueAfter(prompt, "file ")
	if !ok {
		return fmt.Errorf("prepare MT-031 fixture: file path not found in prompt %q", prompt)
	}
	branch, ok := backtickValueAfter(prompt, promptMarkerBranch)
	if !ok {
		return fmt.Errorf("prepare MT-031 fixture: branch not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := ensureLiveBranchExists(setupCtx, client, projectID, branch, liveFixtureDefaultRef); err != nil {
		return fmt.Errorf("prepare MT-031 fixture branch %s: %w", branch, err)
	}
	_, _, err := client.GL().RepositoryFiles.GetFile(projectID, filePath, &gl.GetFileOptions{Ref: &branch}, gl.WithContext(setupCtx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-031 fixture file %s: %w", filePath, err)
	}
	_, _, err = client.GL().RepositoryFiles.CreateFile(projectID, filePath, &gl.CreateFileOptions{
		Branch:        &branch,
		Content:       new("temporary evaluation file\n"),
		CommitMessage: new("Seed file delete evaluation fixture"),
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return fmt.Errorf("prepare MT-031 fixture file %s: %w", filePath, err)
	}
	return nil
}

// ensureLiveMilestoneDeleteTarget handles ensure live milestone delete target and returns [evalTask].
func ensureLiveMilestoneDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-035 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	milestone, _, err := client.GL().Milestones.CreateMilestone(projectID, &gl.CreateMilestoneOptions{
		Title:       new("Evaluation Sprint Delete " + liveUniqueSuffix()),
		Description: new("Temporary milestone for destructive evaluator coverage."),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-035 fixture milestone: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "milestone IID ", milestone.IID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveReleaseDeleteTarget ensures live release delete target exists for live evaluation.
func ensureLiveReleaseDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := exampleProjectIDValue(prompt)
	if !ok {
		return fmt.Errorf("prepare MT-037 fixture: project path not found in prompt %q", prompt)
	}
	tagName, ok := backtickValueAfter(prompt, "release ")
	if !ok {
		return fmt.Errorf("prepare MT-037 fixture: release tag not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := ensureLiveTagExists(setupCtx, client, projectID, tagName, liveFixtureDefaultRef); err != nil {
		return fmt.Errorf("prepare MT-037 fixture tag %s: %w", tagName, err)
	}
	_, _, err := client.GL().Releases.GetRelease(projectID, tagName, gl.WithContext(setupCtx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-037 fixture release %s: %w", tagName, err)
	}
	_, _, err = client.GL().Releases.CreateRelease(projectID, &gl.CreateReleaseOptions{
		Name:        new("Evaluation release safe to delete"),
		TagName:     &tagName,
		Description: new("Temporary release for destructive evaluator coverage."),
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusConflict) && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return fmt.Errorf("prepare MT-037 fixture release %s: %w", tagName, err)
	}
	return nil
}

// ensureLiveInteractiveMergeRequestTarget prepares the source branch used by the elicitation mock.
func ensureLiveInteractiveMergeRequestTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := exampleProjectIDValue(prompt)
	if !ok {
		return fmt.Errorf("prepare MT-081 fixture: project path not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := ensureLiveBranchExists(setupCtx, client, projectID, liveFixtureFeatureRef, liveFixtureDefaultRef); err != nil {
		return fmt.Errorf("prepare MT-081 fixture branch %s: %w", liveFixtureFeatureRef, err)
	}
	content := fmt.Sprintf("interactive merge request fixture %s\n", liveUniqueSuffix())
	message := "Seed interactive merge request fixture"
	_, _, err := client.GL().RepositoryFiles.CreateFile(projectID, liveFixtureInteractiveMRFile, &gl.CreateFileOptions{
		Branch:        new(liveFixtureFeatureRef),
		Content:       &content,
		CommitMessage: &message,
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return fmt.Errorf("prepare MT-081 fixture file: %w", err)
	}
	if closeErr := closeLiveOpenMergeRequestsForBranch(setupCtx, client, projectID, liveFixtureFeatureRef, liveFixtureDefaultRef); closeErr != nil {
		return fmt.Errorf("prepare MT-081 fixture open merge requests: %w", closeErr)
	}
	return nil
}

// ensureLiveInteractiveReleaseTarget prepares the release tag used by the elicitation mock.
func ensureLiveInteractiveReleaseTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := exampleProjectIDValue(prompt)
	if !ok {
		return fmt.Errorf("prepare MT-083 fixture: project path not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	tagName := fmt.Sprintf("%s-%s", liveFixtureElicitationTag, liveUniqueSuffix())
	setEvalElicitationReleaseTag(tagName)
	if err := ensureLiveTagExists(setupCtx, client, projectID, tagName, liveFixtureDefaultRef); err != nil {
		return fmt.Errorf("prepare MT-083 fixture tag %s: %w", tagName, err)
	}
	return nil
}

// closeLiveOpenMergeRequestsForBranch closes open merge requests that would block MR creation.
func closeLiveOpenMergeRequestsForBranch(ctx context.Context, client *gitlabclient.Client, projectID, sourceBranch, targetBranch string) error {
	state := "opened"
	mrs, _, err := client.GL().MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		State:        &state,
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
		ListOptions:  gl.ListOptions{PerPage: 100},
	}, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	for _, mr := range mrs {
		_, _, updateErr := client.GL().MergeRequests.UpdateMergeRequest(projectID, mr.IID, &gl.UpdateMergeRequestOptions{StateEvent: new("close")}, gl.WithContext(ctx))
		if updateErr != nil && !toolutil.IsHTTPStatus(updateErr, http.StatusNotFound) {
			return updateErr
		}
	}
	return nil
}

// ensureLivePackageDeleteTarget handles ensure live package delete target and returns [evalTask].
func ensureLivePackageDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-044 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, _, err := client.GL().GenericPackages.PublishPackageFile(
		projectID,
		liveFixturePackageName,
		fmt.Sprintf("%s-delete-%s", liveFixturePackageVer, liveUniqueSuffix()),
		liveFixturePackageFile,
		bytes.NewBufferString("evaluation package\n"),
		nil,
		gl.WithContext(setupCtx),
	)
	if err != nil {
		return task, fmt.Errorf("prepare MT-044 fixture package publish: %w", err)
	}
	packages, _, err := client.GL().Packages.ListProjectPackages(projectID, &gl.ListProjectPackagesOptions{
		PackageType: new("generic"),
		PackageName: new(liveFixturePackageName),
		OrderBy:     new("created_at"),
		Sort:        new("desc"),
		ListOptions: gl.ListOptions{PerPage: 1},
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-044 fixture package list: %w", err)
	}
	if len(packages) == 0 {
		return task, errors.New("prepare MT-044 fixture package was not listed after publish")
	}
	prompt, err := replaceAllPromptBacktickValuesAfter(task.Prompt, "package ID ", packages[0].ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveEnvironmentStopTarget handles ensure live environment stop target and returns [evalTask].
func ensureLiveEnvironmentStopTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-049 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	name := "eval-stop-" + liveUniqueSuffix()
	env, _, err := client.GL().Environments.CreateEnvironment(projectID, &gl.CreateEnvironmentOptions{
		Name:        new(name),
		Description: new("Temporary environment for destructive evaluator coverage"),
		Tier:        new("testing"),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-049 fixture environment: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "environment ID ", env.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveBroadcastMessageDeleteTarget handles ensure live broadcast message delete target and returns [evalTask].
func ensureLiveBroadcastMessageDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	startsAt := time.Now().UTC().Add(24 * time.Hour)
	endsAt := startsAt.Add(time.Hour)
	message := "Evaluation broadcast safe to delete " + liveUniqueSuffix()
	broadcastType := "banner"
	dismissable := true
	msg, _, err := client.GL().BroadcastMessage.CreateBroadcastMessage(&gl.CreateBroadcastMessageOptions{
		Message:       &message,
		StartsAt:      &startsAt,
		EndsAt:        &endsAt,
		BroadcastType: &broadcastType,
		Dismissable:   &dismissable,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-054 fixture broadcast message: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "broadcast message ID ", msg.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveJobTokenScopeRemoveProjectTarget handles ensure live job token scope remove project target and returns [evalTask].
func ensureLiveJobTokenScopeRemoveProjectTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerAllowlistProject)
	if !ok {
		return task, fmt.Errorf("prepare MT-066 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	source, _, err := client.GL().Projects.GetProject(projectID, nil, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-066 fixture source project: %w", err)
	}
	target, err := createLiveTemporaryProject(setupCtx, client, "token-scope")
	if err != nil {
		return task, fmt.Errorf("prepare MT-066 fixture target project: %w", err)
	}
	_, err = client.GL().JobTokenScope.PatchProjectJobTokenAccessSettings(source.ID, &gl.PatchProjectJobTokenAccessSettingsOptions{
		Enabled: true,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-066 fixture token scope settings: %w", err)
	}
	_, _, err = client.GL().JobTokenScope.AddProjectToJobScopeAllowList(source.ID, &gl.JobTokenInboundAllowOptions{
		TargetProjectID: new(target.ID),
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return task, fmt.Errorf("prepare MT-066 fixture allowlist project: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "project ID ", target.ID)
	if err != nil {
		return task, err
	}
	prompt, err = replacePromptBacktickValueAfter(prompt, promptMarkerAllowlistProject, source.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// createLiveTemporaryProject creates live temporary project and returns [*gl.Project].
func createLiveTemporaryProject(ctx context.Context, client *gitlabclient.Client, prefix string) (*gl.Project, error) {
	toolsGroup, _, err := client.GL().Groups.GetGroup(liveFixtureToolsPath, nil, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get tools group %s: %w", liveFixtureToolsPath, err)
	}
	path := fmt.Sprintf("eval-%s-%s", prefix, liveUniqueSuffix())
	visibility := gl.PrivateVisibility
	project, _, err := client.GL().Projects.CreateProject(&gl.CreateProjectOptions{
		Name:                 new(path),
		Path:                 new(path),
		NamespaceID:          new(toolsGroup.ID),
		InitializeWithReadme: new(true),
		Visibility:           &visibility,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create project %s/%s: %w", liveFixtureToolsPath, path, err)
	}
	return project, nil
}

// createLiveTemporaryGroup creates live temporary group and returns [string].
func createLiveTemporaryGroup(ctx context.Context, client *gitlabclient.Client, prefix string) (string, error) {
	parent, _, err := client.GL().Groups.GetGroup(liveFixtureGroupPath, nil, gl.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("get parent group %s: %w", liveFixtureGroupPath, err)
	}
	path := fmt.Sprintf("eval-%s-%s", prefix, liveUniqueSuffix())
	visibility := gl.PrivateVisibility
	group, _, err := client.GL().Groups.CreateGroup(&gl.CreateGroupOptions{
		Name:       new(path),
		Path:       new(path),
		ParentID:   new(parent.ID),
		Visibility: &visibility,
	}, gl.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("create group %s/%s: %w", liveFixtureGroupPath, path, err)
	}
	if group.FullPath != "" {
		return group.FullPath, nil
	}
	return fmt.Sprintf("%s/%s", liveFixtureGroupPath, path), nil
}

// waitForLivePipelineStatus waits for for live pipeline status to become available.
func waitForLivePipelineStatus(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64, wantedStatus string) error {
	deadline := time.Now().Add(4 * time.Minute)
	lastStatus := "unknown"
	for time.Now().Before(deadline) {
		pipeline, _, err := client.GL().Pipelines.GetPipeline(projectID, pipelineID, gl.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("prepare fixture pipeline %d: %w", pipelineID, err)
		}
		lastStatus = pipeline.Status
		if pipeline.Status == wantedStatus {
			return nil
		}
		if isLivePipelineTerminal(pipeline.Status) {
			return fmt.Errorf("prepare fixture pipeline %d ended with status %s", pipelineID, pipeline.Status)
		}
		if waitErr := waitForContext(ctx, 5*time.Second); waitErr != nil {
			return waitErr
		}
	}
	return fmt.Errorf("prepare fixture pipeline %d did not reach %s before timeout; last status %s", pipelineID, wantedStatus, lastStatus)
}

// isLivePipelineTerminal reports whether a live pipeline status has stopped changing.
func isLivePipelineTerminal(status string) bool {
	switch status {
	case "success", "failed", "canceled", "skipped", "manual":
		return true
	default:
		return false
	}
}

// waitForLiveMergeRequestReady waits for for live merge request ready to become available.
func waitForLiveMergeRequestReady(ctx context.Context, client *gitlabclient.Client, projectID string, mergeRequestIID int64) error {
	deadline := time.Now().Add(90 * time.Second)
	lastStatus := "unknown"
	for time.Now().Before(deadline) {
		mergeRequest, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, mergeRequestIID, nil, gl.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("prepare fixture MR !%d: %w", mergeRequestIID, err)
		}
		lastStatus = mergeRequest.DetailedMergeStatus
		if mergeRequest.DetailedMergeStatus == "mergeable" {
			return nil
		}
		if waitErr := waitForContext(ctx, 2*time.Second); waitErr != nil {
			return waitErr
		}
	}
	return fmt.Errorf("prepare fixture MR !%d did not become mergeable before timeout; last status %s", mergeRequestIID, lastStatus)
}

// createLiveTerraformStateLock creates live terraform state lock for the main package.
func createLiveTerraformStateLock(ctx context.Context, projectID, stateName string) error {
	baseURL, err := liveDockerGitLabBaseURL()
	if err != nil {
		return err
	}
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return errors.New("prepare MT-114 fixture requires GITLAB_TOKEN")
	}
	lockBody, err := json.Marshal(map[string]string{
		"ID":        "eval-lock-" + liveUniqueSuffix(),
		"Operation": "OperationTypeApply",
		"Info":      "eval_mcp_surfaces terraform unlock fixture",
		"Who":       "eval_mcp_surfaces",
		"Version":   "1.6.0",
		"Created":   time.Now().UTC().Format(time.RFC3339Nano),
		"Path":      stateName,
	})
	if err != nil {
		return fmt.Errorf("prepare MT-114 fixture lock body: %w", err)
	}
	endpoint := terraformStateLockEndpoint(baseURL, projectID, stateName)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(lockBody)) // #nosec G107,G704 -- endpoint is constrained by liveDockerGitLabBaseURL and uses a fixed API path.
	if err != nil {
		return fmt.Errorf("prepare MT-114 fixture request: %w", err)
	}
	request.Header.Set("PRIVATE-TOKEN", token)
	request.Header.Set("Content-Type", "application/json")
	httpClient, err := liveGitLabHTTPClient()
	if err != nil {
		return err
	}
	response, err := httpClient.Do(request) // #nosec G704 -- request URL is constrained to the configured Docker GitLab base URL.
	if err != nil {
		return fmt.Errorf("prepare MT-114 fixture lock: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
	return fmt.Errorf("prepare MT-114 fixture lock for project %q state %q: GitLab returned %s: %s", projectID, stateName, response.Status, strings.TrimSpace(string(body)))
}

// liveGitLabHTTPClient coordinates live GitLab HTTP client and returns [*http.Client].
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

// gitlabSkipTLSVerify handles gitlab skip TLS verify and returns [bool].
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

// terraformStateLockEndpoint returns the terraform state lock endpoint used by evaluator requests.
func terraformStateLockEndpoint(baseURL *url.URL, projectID, stateName string) string {
	root := strings.TrimRight(baseURL.String(), "/")
	return root + "/api/v4/projects/" + url.PathEscape(projectID) + "/terraform/state/" + url.PathEscape(stateName) + "/lock"
}

// liveDockerGitLabBaseURL coordinates live docker GitLab base URL and returns [*url.URL].
func liveDockerGitLabBaseURL() (*url.URL, error) {
	rawURL := strings.TrimRight(os.Getenv("GITLAB_URL"), "/")
	if rawURL == "" {
		return nil, errors.New("prepare MT-114 fixture requires GITLAB_URL")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("prepare MT-114 fixture GitLab URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("prepare MT-114 fixture GitLab URL has unsupported scheme %q", parsedURL.Scheme)
	}
	if parsedURL.Host == "" || parsedURL.User != nil {
		return nil, errors.New("prepare MT-114 fixture GitLab URL must include a host and no credentials")
	}
	return parsedURL, nil
}

// liveRemoteMirrorTargetURL handles live remote mirror target URL and returns [string].
func liveRemoteMirrorTargetURL(project *gl.Project) (string, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return "", errors.New("prepare MT-116 fixture requires GITLAB_TOKEN")
	}
	baseURL := strings.TrimRight(os.Getenv("E2E_GITLAB_INTERNAL_URL"), "/")
	if baseURL == "" {
		baseURL = "http://gitlab-e2e"
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("prepare MT-116 fixture internal URL: %w", err)
	}
	projectPath := strings.TrimPrefix(project.PathWithNamespace, "/")
	if projectPath == "" {
		return "", errors.New("prepare MT-116 fixture target project path is empty")
	}
	parsedURL.User = url.UserPassword("oauth2", token)
	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + "/" + projectPath + ".git"
	return parsedURL.String(), nil
}

// ensureLiveProjectMemberAbsent ensures live project member absent exists for live evaluation.
func ensureLiveProjectMemberAbsent(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := exampleProjectIDValue(prompt)
	if !ok {
		return fmt.Errorf("prepare MS-034 fixture: project path not found in prompt %q", prompt)
	}
	userIDValue, ok := backtickValueAfter(prompt, "user ID ")
	if !ok {
		return fmt.Errorf("prepare MS-034 fixture: user ID not found in prompt %q", prompt)
	}
	userID, err := strconv.ParseInt(userIDValue, 10, 64)
	if err != nil {
		return fmt.Errorf("prepare MS-034 fixture user ID %q: %w", userIDValue, err)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, err = client.GL().ProjectMembers.DeleteProjectMember(projectID, userID, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("prepare MS-034 fixture member cleanup: %w", err)
	}
	return nil
}

// ensureLiveRunnerRemoveTarget handles ensure live runner remove target and returns [evalTask].
func ensureLiveRunnerRemoveTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	project, _, err := client.GL().Projects.GetProject(liveFixtureProjectPath, nil, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-047 fixture project lookup: %w", err)
	}
	runner, _, err := client.GL().Users.CreateUserRunner(&gl.CreateUserRunnerOptions{
		RunnerType:  new("project_type"),
		ProjectID:   new(project.ID),
		Description: new("eval-remove-runner-" + liveUniqueSuffix()),
		Paused:      new(false),
		Locked:      new(false),
		RunUntagged: new(true),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-047 fixture runner: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "runner ID ", runner.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveSnippetDeleteTarget handles ensure live snippet delete target and returns [evalTask].
func ensureLiveSnippetDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	visibility := gl.PrivateVisibility
	snippet, _, err := client.GL().Snippets.CreateSnippet(&gl.CreateSnippetOptions{
		Title:      new("Evaluation snippet safe to delete " + liveUniqueSuffix()),
		FileName:   new("eval.txt"),
		Content:    new("evaluation snippet content\n"),
		Visibility: &visibility,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-051 fixture snippet: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "personal snippet ID ", snippet.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveHookDeleteTarget handles ensure live hook delete target and returns [evalTask].
func ensureLiveHookDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-057 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	hook, _, err := client.GL().Projects.AddProjectHook(projectID, &gl.AddProjectHookOptions{
		Name:                  new(fmt.Sprintf(liveDeleteFixtureNameFormat, liveUniqueSuffix())),
		URL:                   new("https://example.com/gitlab-hook-delete"),
		PushEvents:            new(true),
		EnableSSLVerification: new(false),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-057 fixture hook: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "webhook ID ", hook.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveBadgeDeleteTarget handles ensure live badge delete target and returns [evalTask].
func ensureLiveBadgeDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-059 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	badge, _, err := client.GL().ProjectBadges.AddProjectBadge(projectID, &gl.AddProjectBadgeOptions{
		LinkURL:  new("https://example.com/coverage"),
		ImageURL: new("https://example.com/badge.svg"),
		Name:     new(fmt.Sprintf(liveDeleteFixtureNameFormat, liveUniqueSuffix())),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-059 fixture badge: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "badge ID ", badge.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveBranchExists ensures live branch exists exists for live evaluation.
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

// ensureLiveTagExists ensures live tag exists exists for live evaluation.
func ensureLiveTagExists(ctx context.Context, client *gitlabclient.Client, projectID, tagName, ref string) error {
	_, _, err := client.GL().Tags.GetTag(projectID, tagName, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return err
	}
	_, _, err = client.GL().Tags.CreateTag(projectID, &gl.CreateTagOptions{
		TagName: &tagName,
		Ref:     &ref,
	}, gl.WithContext(ctx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return err
	}
	return nil
}

// ensureLiveBranchDeleteTarget ensures live branch delete target exists for live evaluation.
func ensureLiveBranchDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := backtickValueAfter(prompt, promptMarkerProject)
	if !ok {
		return fmt.Errorf("prepare MT-099 fixture: project path not found in prompt %q", prompt)
	}
	branchName, ok := backtickValueAfter(prompt, promptMarkerBranch)
	if !ok {
		return fmt.Errorf("prepare MT-099 fixture: branch name not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, _, err := client.GL().Branches.GetBranch(projectID, branchName, gl.WithContext(setupCtx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-099 fixture branch %s: %w", branchName, err)
	}
	ref := liveFixtureDefaultRef
	_, _, err = client.GL().Branches.CreateBranch(projectID, &gl.CreateBranchOptions{
		Branch: &branchName,
		Ref:    &ref,
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return fmt.Errorf("prepare MT-099 fixture branch %s: %w", branchName, err)
	}
	return nil
}

// ensureLiveTagDeleteTarget ensures live tag delete target exists for live evaluation.
func ensureLiveTagDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := backtickValueAfter(prompt, promptMarkerProject)
	if !ok {
		return fmt.Errorf("prepare MT-100 fixture: project path not found in prompt %q", prompt)
	}
	tagName, ok := backtickValueAfter(prompt, "tag ")
	if !ok {
		return fmt.Errorf("prepare MT-100 fixture: tag name not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, _, err := client.GL().Tags.GetTag(projectID, tagName, gl.WithContext(setupCtx))
	if err == nil {
		return nil
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-100 fixture tag %s: %w", tagName, err)
	}
	ref := liveFixtureDefaultRef
	_, _, err = client.GL().Tags.CreateTag(projectID, &gl.CreateTagOptions{
		TagName: &tagName,
		Ref:     &ref,
	}, gl.WithContext(setupCtx))
	if err != nil && !toolutil.IsHTTPStatus(err, http.StatusBadRequest) && !toolutil.IsHTTPStatus(err, http.StatusConflict) {
		return fmt.Errorf("prepare MT-100 fixture tag %s: %w", tagName, err)
	}
	return nil
}

// ensureLiveFailedJobTarget creates an attempt-local failed job and rewrites the task prompt to use it.
func ensureLiveFailedJobTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerProject)
	if !ok {
		return task, fmt.Errorf("prepare %s fixture: project path not found in prompt %q", task.ID, task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	ref := liveFixtureDefaultRef
	pipeline, _, err := client.GL().Pipelines.CreatePipeline(projectID, &gl.CreatePipelineOptions{Ref: &ref}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare %s fixture pipeline: %w", task.ID, err)
	}
	jobID, err := waitForFailedJob(setupCtx, client, projectID, pipeline.ID)
	if err != nil {
		return task, err
	}
	prompt, err := replacePromptJobID(task.Prompt, jobID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLivePipelineDeleteTarget handles ensure live pipeline delete target and returns [evalTask].
func ensureLivePipelineDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	_, pipelineID, err := createLiveEvaluationPipeline(ctx, client, task, "MT-101")
	if err != nil {
		return task, err
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "pipeline ", pipelineID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLivePipelineTriggerDeleteTarget handles ensure live pipeline trigger delete target and returns [evalTask].
func ensureLivePipelineTriggerDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerProject)
	if !ok {
		return task, fmt.Errorf("prepare MT-102 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	trigger, _, err := client.GL().PipelineTriggers.AddPipelineTrigger(projectID, &gl.AddPipelineTriggerOptions{
		Description: new(fmt.Sprintf(liveDeleteFixtureNameFormat, liveUniqueSuffix())),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-102 fixture trigger: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "pipeline trigger token ID ", trigger.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLivePipelineScheduleDeleteTarget handles ensure live pipeline schedule delete target and returns [evalTask].
func ensureLivePipelineScheduleDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerProject)
	if !ok {
		return task, fmt.Errorf("prepare MT-103 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ref := liveFixtureDefaultRef
	schedule, _, err := client.GL().PipelineSchedules.CreatePipelineSchedule(projectID, &gl.CreatePipelineScheduleOptions{
		Description:  new(fmt.Sprintf(liveDeleteFixtureNameFormat, liveUniqueSuffix())),
		Ref:          &ref,
		Cron:         new("0 3 * * *"),
		CronTimezone: new("UTC"),
		Active:       new(false),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-103 fixture schedule: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "pipeline schedule ID ", schedule.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveFeatureFlagDeleteTarget ensures live feature flag delete target exists for live evaluation.
func ensureLiveFeatureFlagDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := backtickValueAfter(prompt, promptMarkerProject)
	if !ok {
		return fmt.Errorf("prepare MT-106 fixture: project path not found in prompt %q", prompt)
	}
	flagName, ok := backtickValueAfter(prompt, "feature flag ")
	if !ok {
		return fmt.Errorf("prepare MT-106 fixture: feature flag name not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, deleteErr := client.GL().ProjectFeatureFlags.DeleteProjectFeatureFlag(projectID, flagName, gl.WithContext(setupCtx))
	if deleteErr != nil && !toolutil.IsHTTPStatus(deleteErr, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-106 fixture feature flag cleanup project %s flag %s: %w", projectID, flagName, deleteErr)
	}
	active := false
	_, _, err := client.GL().ProjectFeatureFlags.CreateProjectFeatureFlag(projectID, &gl.CreateProjectFeatureFlagOptions{
		Name:   &flagName,
		Active: &active,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return fmt.Errorf("prepare MT-106 fixture feature flag %s: %w", flagName, err)
	}
	return nil
}

// ensureLiveWikiDeleteTarget ensures live wiki delete target exists for live evaluation.
func ensureLiveWikiDeleteTarget(ctx context.Context, client *gitlabclient.Client, prompt string) error {
	if client == nil {
		return nil
	}
	projectID, ok := backtickValueAfter(prompt, promptMarkerProject)
	if !ok {
		return fmt.Errorf("prepare MT-108 fixture: project path not found in prompt %q", prompt)
	}
	slug, ok := backtickValueAfter(prompt, "wiki page ")
	if !ok {
		return fmt.Errorf("prepare MT-108 fixture: wiki page slug not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, deleteErr := client.GL().Wikis.DeleteWikiPage(projectID, slug, gl.WithContext(setupCtx))
	if deleteErr != nil && !toolutil.IsHTTPStatus(deleteErr, http.StatusNotFound) {
		return fmt.Errorf("prepare MT-108 fixture wiki page cleanup project %s slug %s: %w", projectID, slug, deleteErr)
	}
	content := "# Delete fixture\n\nTemporary wiki page for destructive evaluator coverage."
	_, _, err := client.GL().Wikis.CreateWikiPage(projectID, &gl.CreateWikiPageOptions{
		Title:   &slug,
		Content: &content,
	}, gl.WithContext(setupCtx))
	if err != nil {
		return fmt.Errorf("prepare MT-108 fixture wiki page %s: %w", slug, err)
	}
	return nil
}

// ensureLiveMRAwardDeleteTarget handles ensure live MR award delete target and returns [evalTask].
func ensureLiveMRAwardDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerProject)
	if !ok {
		return task, fmt.Errorf("prepare MT-109 fixture: project path not found in prompt %q", task.Prompt)
	}
	mergeRequestIID, err := promptInt64After(task.Prompt, promptMarkerMergeRequest)
	if err != nil {
		return task, fmt.Errorf("prepare MT-109 fixture: %w", err)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	awardID, err := createLiveMRAwardEmoji(setupCtx, client, projectID, mergeRequestIID)
	if err != nil {
		return task, fmt.Errorf("prepare MT-109 fixture award emoji: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerAwardEmojiID, awardID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveIssueAwardDeleteTarget handles ensure live issue award delete target and returns [evalTask].
func ensureLiveIssueAwardDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, issue, err := createLiveEvaluationIssue(ctx, client, task, "MT-110", "Evaluation issue award delete target "+liveUniqueSuffix(), "Temporary issue for destructive award emoji evaluator coverage.")
	if err != nil {
		return task, err
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, promptMarkerIssue, issue.IID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	issueIID := issue.IID
	awardID, err := createLiveIssueAwardEmoji(setupCtx, client, projectID, issueIID)
	if err != nil {
		return task, fmt.Errorf("prepare MT-110 fixture award emoji: %w", err)
	}
	prompt, err = replacePromptBacktickValueAfter(task.Prompt, promptMarkerAwardEmojiID, awardID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// promptInt64After handles prompt int 64 after and returns [int64].
func promptInt64After(prompt, marker string) (int64, error) {
	value, ok := backtickValueAfter(prompt, marker)
	if !ok {
		return 0, fmt.Errorf("value after %q not found in prompt %q", marker, prompt)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("value after %q is not an integer: %w", marker, err)
	}
	return parsed, nil
}

// createLiveMRAwardEmoji creates live MR award emoji and returns [int64].
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

// createLiveIssueAwardEmoji creates live issue award emoji and returns [int64].
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

// liveAwardEmojiNames returns award emoji names for live evaluation runs.
func liveAwardEmojiNames() []string {
	return []string{"thumbsup", "thumbsdown", "rocket", "eyes", "heart", "tada"}
}

// ensureLiveDeployKeyDeleteTarget handles ensure live deploy key delete target and returns [evalTask].
func ensureLiveDeployKeyDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerProject)
	if !ok {
		return task, fmt.Errorf("prepare MT-111 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	key, err := newAuthorizedSSHKey()
	if err != nil {
		return task, fmt.Errorf("prepare MT-111 fixture public key: %w", err)
	}
	deployKey, _, err := client.GL().DeployKeys.AddDeployKey(projectID, &gl.AddDeployKeyOptions{
		Title:   new("eval-delete-key-" + liveUniqueSuffix()),
		Key:     &key,
		CanPush: new(false),
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-111 fixture deploy key: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "deploy key ID ", deployKey.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveDeployTokenDeleteTarget handles ensure live deploy token delete target and returns [evalTask].
func ensureLiveDeployTokenDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-112 fixture: project path not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	expiresAt := time.Now().UTC().AddDate(0, 1, 0)
	deployToken, _, err := client.GL().DeployTokens.CreateProjectDeployToken(projectID, &gl.CreateProjectDeployTokenOptions{
		Name:      new("eval-delete-deploy-token-" + liveUniqueSuffix()),
		ExpiresAt: &expiresAt,
		Scopes:    &[]string{"read_repository"},
	}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-112 fixture deploy token: %w", err)
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "deploy token ID ", deployToken.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// ensureLiveCommitDiscussionNoteDeleteTarget handles ensure live commit discussion note delete target and returns [evalTask].
func ensureLiveCommitDiscussionNoteDeleteTarget(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, ok := exampleProjectIDValue(task.Prompt)
	if !ok {
		return task, fmt.Errorf("prepare MT-113 fixture: project path not found in prompt %q", task.Prompt)
	}
	commitSHA, ok := backtickValueAfter(task.Prompt, "on commit ")
	if !ok {
		return task, fmt.Errorf("prepare MT-113 fixture: commit SHA not found in prompt %q", task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	body := "delete fixture " + liveUniqueSuffix()
	discussion, _, err := client.GL().Discussions.CreateCommitDiscussion(projectID, commitSHA, &gl.CreateCommitDiscussionOptions{Body: &body}, gl.WithContext(setupCtx))
	if err != nil {
		return task, fmt.Errorf("prepare MT-113 fixture commit discussion: %w", err)
	}
	if discussion.ID == "" || len(discussion.Notes) == 0 {
		return task, errors.New("prepare MT-113 fixture commit discussion returned no note")
	}
	prompt, err := replacePromptBacktickValueAfter(task.Prompt, "discussion note ", discussion.Notes[0].ID)
	if err != nil {
		return task, err
	}
	prompt, err = replacePromptBacktickValueAfter(prompt, "from discussion ", discussion.ID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

// cleanupLiveInstanceVariables removes cleanup live instance variables resources when present.
func cleanupLiveInstanceVariables(ctx context.Context, client *gitlabclient.Client, prefix string) error {
	if client == nil {
		return nil
	}
	vars, _, err := client.GL().InstanceVariables.ListVariables(&gl.ListInstanceVariablesOptions{
		ListOptions: gl.ListOptions{PerPage: 100},
	}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("clean up instance variables: %w", err)
	}
	for _, variable := range vars {
		if !strings.HasPrefix(variable.Key, prefix) {
			continue
		}
		if _, removeErr := client.GL().InstanceVariables.RemoveVariable(variable.Key, gl.WithContext(ctx)); removeErr != nil && !toolutil.IsHTTPStatus(removeErr, http.StatusNotFound) {
			return fmt.Errorf("clean up instance variable %s: %w", variable.Key, removeErr)
		}
	}
	return nil
}

// waitForFailedJob waits for a failed job in the pipeline and returns its ID.
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

// ensureLiveManualJob handles ensure live manual job and returns [evalTask].
func ensureLiveManualJob(ctx context.Context, client *gitlabclient.Client, task evalTask) (evalTask, error) {
	if client == nil {
		return task, nil
	}
	projectID, pipelineID, err := createLiveEvaluationPipeline(ctx, client, task, "MT-064")
	if err != nil {
		return task, err
	}
	setupCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	manualJobID, err := waitForManualJob(setupCtx, client, projectID, pipelineID)
	if err != nil {
		return task, err
	}
	prompt, err := replacePromptJobID(task.Prompt, manualJobID)
	if err != nil {
		return task, err
	}
	task.Prompt = prompt
	return task, nil
}

func createLiveEvaluationPipeline(ctx context.Context, client *gitlabclient.Client, task evalTask, taskID string) (projectID string, pipelineID int64, err error) {
	projectID, ok := backtickValueAfter(task.Prompt, promptMarkerProject)
	if !ok {
		return "", 0, fmt.Errorf("prepare %s fixture: project path not found in prompt %q", taskID, task.Prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	ref := liveFixtureDefaultRef
	pipeline, _, err := client.GL().Pipelines.CreatePipeline(projectID, &gl.CreatePipelineOptions{Ref: &ref}, gl.WithContext(setupCtx))
	if err != nil {
		return "", 0, fmt.Errorf("prepare %s fixture pipeline: %w", taskID, err)
	}
	return projectID, pipeline.ID, nil
}

// waitForManualJob handles wait for manual job and returns [int64].
func waitForManualJob(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64) (int64, error) {
	return waitForPipelineJobStatus(ctx, client, projectID, pipelineID, "manual", "prepare MT-064 fixture jobs", "prepare MT-064 fixture manual job")
}

// replacePromptJobID handles replace prompt job ID and returns [string].
func replacePromptJobID(prompt string, jobID int64) (string, error) {
	return replacePromptBacktickValueAfter(prompt, "job ", jobID)
}

// replacePromptBacktickValueAfter handles replace prompt backtick value after and returns [string].
func replacePromptBacktickValueAfter(prompt, marker string, value any) (string, error) {
	oldValue, ok := backtickValueAfter(prompt, marker)
	if !ok {
		return prompt, fmt.Errorf("backtick value after %q not found in prompt %q", marker, prompt)
	}
	oldText := marker + "`" + oldValue + "`"
	newText := fmt.Sprintf("%s`%v`", marker, value)
	return strings.Replace(prompt, oldText, newText, 1), nil
}

// replaceAllPromptBacktickValuesAfter handles replace all prompt backtick values after and returns [string].
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

// ensureLiveMergeRequestSource ensures live merge request source exists for live evaluation.
func ensureLiveMergeRequestSource(ctx context.Context, session *mcp.ClientSession, prompt, toolSurface string) error {
	projectID, ok := backtickValueAfter(prompt, promptMarkerProject)
	if !ok {
		return fmt.Errorf("prepare MT-015 fixture: project path not found in prompt %q", prompt)
	}
	sourceBranch, ok := backtickValueAfter(prompt, promptMarkerFrom)
	if !ok {
		return fmt.Errorf("prepare MT-015 fixture: source branch not found in prompt %q", prompt)
	}
	targetBranch, ok := backtickValueAfter(prompt, " into ")
	if !ok {
		return fmt.Errorf("prepare MT-015 fixture: target branch not found in prompt %q", prompt)
	}
	setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := callFixtureSetupTool(setupCtx, session, toolSurface, "branch.create", map[string]any{
		"project_id":  projectID,
		"branch_name": sourceBranch,
		"ref":         targetBranch,
	}, "already exists"); err != nil {
		return err
	}
	filePath := "tmp/eval-mr-" + safeFixturePathPart(sourceBranch) + ".txt"
	return callFixtureSetupTool(setupCtx, session, toolSurface, "repository.file_create", map[string]any{
		"project_id":     projectID,
		"file_path":      filePath,
		"branch":         sourceBranch,
		"content":        "evaluation merge request fixture\n",
		"commit_message": "Seed evaluation merge request fixture",
	}, "already exists")
}

// callFixtureSetupTool resolves call fixture setup tool for evaluator execution.
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

// fixtureSetupToolEnvelope returns the tool call shape for fixture setup helpers.
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

// callFixtureSetupToolByName handles call fixture setup tool by name and returns [*mcp.CallToolResult].
func callFixtureSetupToolByName(ctx context.Context, session *mcp.ClientSession, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	return session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
}

// splitFixtureSetupAction handles split fixture setup action and returns [string].
func splitFixtureSetupAction(action string) (toolName, splitAction string, ok bool) {
	domain, route, ok := strings.Cut(action, ".")
	if !ok || domain == "" || route == "" {
		return "", "", false
	}
	return "gitlab_" + domain, strings.ReplaceAll(route, ".", "_"), true
}

// backtickValueAfter handles backtick value after and returns [string].
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

// safeFixturePathPart returns the safe fixture path part used by evaluator requests.
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

// externalMCPEnv handles external MCP env and returns [[]string].
