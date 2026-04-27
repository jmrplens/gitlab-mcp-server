//go:build e2e

// resources_read_test.go exercises ReadResource against every registered
// MCP resource URI template. It first creates the required GitLab fixtures
// (project, branch, commit, issue, merge request, label, milestone, tag,
// release, wiki page, board, deploy key, environment, deployment, feature
// flag, project snippet, personal snippet, group, group label, group
// milestone) and then reads each resource template to verify that the
// canonical resource handlers return non-empty JSON content with the
// expected URI and MIME type.
//
// CI-runner-only resources (latest pipeline, pipeline, pipeline jobs, job)
// are exercised only when the suite runs against the ephemeral Docker
// GitLab CE setup with a registered runner (isDockerMode() == true).
package suite

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// extractFirstID parses the first `"id": <int>` integer literal it finds in
// the given text. It is intentionally schema-agnostic so the helper works
// against arbitrary JSON shapes (single object, array, paginated wrapper).
// Returns 0 when no positive integer ID is found.
func extractFirstID(text string) int64 {
	const key = `"id":`
	_, after, ok := strings.Cut(text, key)
	if !ok {
		return 0
	}
	rest := strings.TrimLeft(after, " \t")
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	v, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

// TestResources_ReadAll creates a comprehensive fixture set, then reads
// every registered MCP resource URI template via ReadResource and asserts
// non-empty content with the documented MIME type. CI-runner resources are
// skipped unless the suite runs in Docker ephemeral mode.
func TestResources_ReadAll(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// -----------------------------------------------------------------
	// Fixture setup
	// -----------------------------------------------------------------
	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	pidPath := proj.pidStr()
	pidStr := proj.pidStr()
	projURI := func(suffix string) string { return "gitlab://project/" + pidPath + suffix }

	commit := commitFile(ctx, t, sess.individual, proj, defaultBranch, "hello.txt", "world\n", "add hello.txt")

	const featBranch = "feature/resources-read"
	createBranch(ctx, t, sess.individual, proj, featBranch)
	commitFile(ctx, t, sess.individual, proj, featBranch, "feat.txt", "feat\n", "feat: add feat.txt")

	issue := createIssue(ctx, t, sess.individual, proj, "E2E resource issue")
	mr := createMR(ctx, t, sess.individual, proj, featBranch, defaultBranch, "E2E resource MR")

	label, err := callToolOn[labels.Output](ctx, sess.individual, "gitlab_label_create", labels.CreateInput{
		ProjectID: proj.pidOf(),
		Name:      "e2e-res-label",
		Color:     "#ff00ff",
	})
	requireNoError(t, err, "create label")

	milestone, err := callToolOn[milestones.Output](ctx, sess.individual, "gitlab_milestone_create", milestones.CreateInput{
		ProjectID:   proj.pidOf(),
		Title:       "e2e-res-milestone",
		Description: "for resource read test",
	})
	requireNoError(t, err, "create milestone")

	const tagName = "v0.0.1-resources"
	_, err = callToolOn[tags.Output](ctx, sess.individual, "gitlab_tag_create", tags.CreateInput{
		ProjectID: proj.pidOf(),
		TagName:   tagName,
		Ref:       defaultBranch,
		Message:   "tag for resource read test",
	})
	requireNoError(t, err, "create tag")

	_, err = callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_create", releases.CreateInput{
		ProjectID:   proj.pidOf(),
		TagName:     tagName,
		Name:        "Resource Read Release",
		Description: "for resource read test",
	})
	requireNoError(t, err, "create release")

	wiki, err := callToolOn[wikis.Output](ctx, sess.individual, "gitlab_wiki_create", wikis.CreateInput{
		ProjectID: proj.pidOf(),
		Title:     "E2E Resource Wiki",
		Content:   "Wiki for resource read test.",
	})
	requireNoError(t, err, "create wiki page")

	board, err := callToolOn[boards.BoardOutput](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "board_create",
		"params": map[string]any{
			"project_id": pidStr,
			"name":       "e2e-res-board",
		},
	})
	requireNoError(t, err, "create board")

	dk, err := callToolOn[deploykeys.Output](ctx, sess.individual, "gitlab_deploy_key_add", deploykeys.AddInput{
		ProjectID: proj.pidOf(),
		Title:     "e2e-res-deploy-key",
		Key:       generateTestSSHKey(t),
	})
	requireNoError(t, err, "add deploy key")

	env, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_create", environments.CreateInput{
		ProjectID: proj.pidOf(),
		Name:      "e2e-res-env",
	})
	requireNoError(t, err, "create environment")

	const deployEnvName = "e2e-res-deploy-env"
	if err = callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": pidStr, "name": deployEnvName},
	}); err != nil {
		t.Fatalf("create deployment environment: %v", err)
	}

	type deployOut struct {
		ID int `json:"id"`
	}
	dep, err := callToolOn[deployOut](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "deployment_create",
		"params": map[string]any{
			"project_id":  pidStr,
			"environment": deployEnvName,
			"sha":         commit.SHA,
			"ref":         defaultBranch,
			"tag":         false,
			"status":      "running",
		},
	})
	requireNoError(t, err, "create deployment")

	const ffName = "e2e-res-flag"
	_, err = callToolOn[featureflags.Output](ctx, sess.individual, "gitlab_feature_flag_create", featureflags.CreateInput{
		ProjectID: proj.pidOf(),
		Name:      ffName,
		Version:   "new_version_flag",
	})
	requireNoError(t, err, "create feature flag")

	personalSnip, err := callToolOn[snippets.Output](ctx, sess.individual, "gitlab_snippet_create", snippets.CreateInput{
		Title:       "E2E Resource Personal Snippet",
		FileName:    "snippet.txt",
		ContentBody: "personal",
		Visibility:  "private",
	})
	requireNoError(t, err, "create personal snippet")
	t.Cleanup(func() {
		ccx, cclose := context.WithTimeout(context.Background(), 30*time.Second)
		defer cclose()
		_ = callToolVoidOn(ccx, sess.individual, "gitlab_snippet_delete", snippets.DeleteInput{SnippetID: personalSnip.ID})
	})

	projSnip, err := callToolOn[snippets.Output](ctx, sess.individual, "gitlab_project_snippet_create", snippets.ProjectCreateInput{
		ProjectID:   proj.pidOf(),
		Title:       "E2E Resource Project Snippet",
		FileName:    "psnip.txt",
		ContentBody: "in project",
		Visibility:  "private",
	})
	requireNoError(t, err, "create project snippet")

	groupPath := uniqueName("e2e-res-grp")
	grp, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
		Name:       groupPath,
		Path:       groupPath,
		Visibility: "public",
	})
	requireNoError(t, err, "create group")
	gidStr := strconv.FormatInt(grp.ID, 10)
	t.Cleanup(func() {
		ccx, cclose := context.WithTimeout(context.Background(), 30*time.Second)
		defer cclose()
		_ = callToolVoidOn(ccx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(gidStr),
		})
	})

	grpLabel, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "group_label_create",
		"params": map[string]any{
			"group_id": gidStr,
			"name":     "e2e-res-glabel",
			"color":    "#abcdef",
		},
	})
	requireNoError(t, err, "create group label")

	grpMs, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "group_milestone_create",
		"params": map[string]any{
			"group_id": gidStr,
			"title":    "e2e-res-gms",
		},
	})
	requireNoError(t, err, "create group milestone")

	// -----------------------------------------------------------------
	// ReadResource helpers
	// -----------------------------------------------------------------
	readJSON := func(t *testing.T, uri string) {
		t.Helper()
		result, rerr := sess.individual.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if rerr != nil {
			t.Fatalf("ReadResource %s: %v", uri, rerr)
		}
		if len(result.Contents) == 0 {
			t.Fatalf("ReadResource %s: empty Contents", uri)
		}
		c := result.Contents[0]
		if c.URI != uri {
			t.Errorf("ReadResource %s: returned URI %q", uri, c.URI)
		}
		if c.Text == "" {
			t.Errorf("ReadResource %s: empty Text", uri)
		}
		if c.MIMEType != "application/json" {
			t.Errorf("ReadResource %s: expected MIMEType application/json, got %q", uri, c.MIMEType)
		}
	}

	readMarkdown := func(t *testing.T, uri string) {
		t.Helper()
		result, rerr := sess.individual.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if rerr != nil {
			t.Fatalf("ReadResource %s: %v", uri, rerr)
		}
		if len(result.Contents) == 0 {
			t.Fatalf("ReadResource %s: empty Contents", uri)
		}
		c := result.Contents[0]
		if c.Text == "" {
			t.Errorf("ReadResource %s: empty Text", uri)
		}
		if !strings.HasPrefix(c.MIMEType, "text/markdown") {
			t.Errorf("ReadResource %s: expected text/markdown MIME, got %q", uri, c.MIMEType)
		}
	}

	// -----------------------------------------------------------------
	// Static resources (3)
	// -----------------------------------------------------------------
	t.Run("Static/UserCurrent", func(t *testing.T) { readJSON(t, "gitlab://user/current") })
	t.Run("Static/Groups", func(t *testing.T) { readJSON(t, "gitlab://groups") })
	// gitlab://workspace/roots is exercised by TestCapability_RootsAdvertised
	// (capabilities_test.go) which uses a dedicated server with the roots
	// manager wired up. It is intentionally not registered on the shared
	// individual server.

	// -----------------------------------------------------------------
	// Project-scoped resources
	// -----------------------------------------------------------------
	t.Run("Project/Self", func(t *testing.T) { readJSON(t, projURI("")) })
	t.Run("Project/Members", func(t *testing.T) { readJSON(t, projURI("/members")) })
	t.Run("Project/Labels", func(t *testing.T) { readJSON(t, projURI("/labels")) })
	t.Run("Project/Label", func(t *testing.T) {
		readJSON(t, projURI("/label/"+strconv.FormatInt(label.ID, 10)))
	})
	t.Run("Project/Milestones", func(t *testing.T) { readJSON(t, projURI("/milestones")) })
	t.Run("Project/Milestone", func(t *testing.T) {
		readJSON(t, projURI("/milestone/"+strconv.FormatInt(milestone.IID, 10)))
	})
	t.Run("Project/Branches", func(t *testing.T) { readJSON(t, projURI("/branches")) })
	t.Run("Project/Branch", func(t *testing.T) {
		readJSON(t, projURI("/branch/"+defaultBranch))
	})
	t.Run("Project/Issues", func(t *testing.T) { readJSON(t, projURI("/issues")) })
	t.Run("Project/Issue", func(t *testing.T) {
		readJSON(t, projURI("/issue/"+strconv.FormatInt(issue.IID, 10)))
	})
	t.Run("Project/Releases", func(t *testing.T) { readJSON(t, projURI("/releases")) })
	t.Run("Project/Release", func(t *testing.T) {
		readJSON(t, projURI("/release/"+tagName))
	})
	t.Run("Project/Tags", func(t *testing.T) { readJSON(t, projURI("/tags")) })
	t.Run("Project/Tag", func(t *testing.T) {
		readJSON(t, projURI("/tag/"+tagName))
	})
	t.Run("Project/Commit", func(t *testing.T) {
		readJSON(t, projURI("/commit/"+commit.SHA))
	})
	t.Run("Project/File", func(t *testing.T) {
		readJSON(t, projURI("/file/"+defaultBranch+"/hello.txt"))
	})
	t.Run("Project/Wiki", func(t *testing.T) {
		readJSON(t, projURI("/wiki/"+wiki.Slug))
	})
	t.Run("Project/Board", func(t *testing.T) {
		readJSON(t, projURI("/board/"+strconv.FormatInt(board.ID, 10)))
	})
	t.Run("Project/Deployment", func(t *testing.T) {
		readJSON(t, projURI("/deployment/"+strconv.Itoa(dep.ID)))
	})
	t.Run("Project/Environment", func(t *testing.T) {
		readJSON(t, projURI("/environment/"+strconv.FormatInt(env.ID, 10)))
	})
	t.Run("Project/FeatureFlag", func(t *testing.T) {
		readJSON(t, projURI("/feature_flag/"+ffName))
	})
	t.Run("Project/DeployKey", func(t *testing.T) {
		readJSON(t, projURI("/deploy_key/"+strconv.FormatInt(dk.ID, 10)))
	})
	t.Run("Project/Snippet", func(t *testing.T) {
		readJSON(t, projURI("/snippet/"+strconv.FormatInt(projSnip.ID, 10)))
	})

	// -----------------------------------------------------------------
	// Merge request resources
	// -----------------------------------------------------------------
	mrPath := projURI("/mr/" + strconv.FormatInt(mr.IID, 10))
	t.Run("MR/Self", func(t *testing.T) { readJSON(t, mrPath) })
	t.Run("MR/Notes", func(t *testing.T) { readJSON(t, mrPath+"/notes") })
	t.Run("MR/Discussions", func(t *testing.T) { readJSON(t, mrPath+"/discussions") })

	// -----------------------------------------------------------------
	// Group resources
	// -----------------------------------------------------------------
	groupURI := func(suffix string) string { return "gitlab://group/" + gidStr + suffix }
	t.Run("Group/Self", func(t *testing.T) { readJSON(t, groupURI("")) })
	t.Run("Group/Members", func(t *testing.T) { readJSON(t, groupURI("/members")) })
	t.Run("Group/Projects", func(t *testing.T) { readJSON(t, groupURI("/projects")) })
	t.Run("Group/Label", func(t *testing.T) {
		readJSON(t, groupURI("/label/"+strconv.FormatInt(grpLabel.ID, 10)))
	})
	t.Run("Group/Milestone", func(t *testing.T) {
		readJSON(t, groupURI("/milestone/"+strconv.FormatInt(grpMs.IID, 10)))
	})

	// -----------------------------------------------------------------
	// Personal snippet
	// -----------------------------------------------------------------
	t.Run("Snippet/Personal", func(t *testing.T) {
		readJSON(t, "gitlab://snippet/"+strconv.FormatInt(personalSnip.ID, 10))
	})

	// -----------------------------------------------------------------
	// Workflow guides (text/markdown)
	// -----------------------------------------------------------------
	for _, slug := range []string{
		"git-workflow",
		"merge-request-hygiene",
		"conventional-commits",
		"code-review",
		"pipeline-troubleshooting",
	} {
		uri := "gitlab://guides/" + slug
		t.Run("Guide/"+slug, func(t *testing.T) {
			readMarkdown(t, uri)
		})
	}

	// -----------------------------------------------------------------
	// CI/CD resources — require Docker ephemeral mode + runner
	// -----------------------------------------------------------------
	if !isDockerMode() {
		t.Log("Skipping pipeline/job resources: not in Docker mode (no CI runner)")
		return
	}

	// Push a minimal .gitlab-ci.yml on a fresh branch so we don't disturb
	// prior fixtures. We then poll /pipelines/latest until a pipeline ID
	// appears, and /pipeline/{id}/jobs until a job ID appears.
	const ciYAML = "stages:\n  - test\ntest:\n  stage: test\n  script:\n    - echo ok\n"
	commitFile(ctx, t, sess.individual, proj, defaultBranch, ".gitlab-ci.yml", ciYAML, "ci: add minimal pipeline")

	var pipelineID int64
	pipelineDeadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(pipelineDeadline) {
		result, rerr := sess.individual.ReadResource(ctx, &mcp.ReadResourceParams{URI: projURI("/pipelines/latest")})
		if rerr == nil && len(result.Contents) > 0 {
			if id := extractFirstID(result.Contents[0].Text); id > 0 {
				pipelineID = id
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if pipelineID == 0 {
		t.Fatal("no pipeline produced within 180s")
	}
	pipelineIDStr := strconv.FormatInt(pipelineID, 10)

	t.Run("CI/PipelinesLatest", func(t *testing.T) {
		readJSON(t, projURI("/pipelines/latest"))
	})
	t.Run("CI/Pipeline", func(t *testing.T) {
		readJSON(t, projURI("/pipeline/"+pipelineIDStr))
	})
	t.Run("CI/PipelineJobs", func(t *testing.T) {
		readJSON(t, projURI("/pipeline/"+pipelineIDStr+"/jobs"))
	})

	var jobID int64
	jobDeadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(jobDeadline) {
		result, rerr := sess.individual.ReadResource(ctx, &mcp.ReadResourceParams{URI: projURI("/pipeline/" + pipelineIDStr + "/jobs")})
		if rerr == nil && len(result.Contents) > 0 {
			if id := extractFirstID(result.Contents[0].Text); id > 0 {
				jobID = id
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if jobID == 0 {
		t.Fatal("no job produced within 180s")
	}
	t.Run("CI/Job", func(t *testing.T) {
		readJSON(t, projURI("/job/"+strconv.FormatInt(jobID, 10)))
	})
}
