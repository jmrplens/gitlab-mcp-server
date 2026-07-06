//go:build e2e && !enterprise

// miscextras_ce_test.go covers scattered single-action coverage gaps across
// several domains: latest release retrieval, merge request raw diffs, CI/CD
// catalog resource get, deployment merge requests, and the GraphQL work item
// type listing.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/workitems"
)

// TestIndividual_ReleaseGetLatest exercises gitlab_release_latest via the
// individual tool surface.
//
// The test creates a tag and a release on a fresh project, then fetches the
// latest release and asserts the tag name round-trips. The fetch is retried
// because a just-created release can lag behind the latest-release endpoint.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_ReleaseGetLatest(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	const tagName = "v1.0.0-latest-e2e"

	_, tagErr := callToolOn[tags.Output](ctx, sess.individual, "gitlab_tag_create", tags.CreateInput{
		ProjectID: proj.pidOf(),
		TagName:   tagName,
		Ref:       defaultBranch,
		Message:   "Latest release E2E tag",
	})
	requireNoError(t, tagErr, "create tag for latest release")

	_, createErr := callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_create", releases.CreateInput{
		ProjectID:   proj.pidOf(),
		TagName:     tagName,
		Name:        "E2E Latest Release",
		Description: "Automated E2E latest-release fixture.",
	})
	requireNoError(t, createErr, "create release for latest release")

	out, err := retryOnTransient(ctx, t, "get latest release", 5, func() (releases.Output, error) {
		return callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_latest", releases.GetLatestInput{
			ProjectID: proj.pidOf(),
		})
	})
	requireNoError(t, err, "get latest release")
	requireTruef(t, out.TagName == tagName, "expected latest release tag %q, got %q", tagName, out.TagName)
	t.Logf("Latest release is %s (%s)", out.Name, out.TagName)
}

// TestIndividual_MRRawDiffs exercises gitlab_mr_raw_diffs via the individual
// tool surface.
//
// The test creates a merge request from a diverged feature branch, waits for
// the MR to become ready, and polls the raw diffs endpoint until the unified
// diff is non-empty (diff computation is asynchronous). It asserts the diff
// mentions the file committed to the feature branch.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_MRRawDiffs(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj, branch := setupMRProject(ctx, t, sess.individual)
	mr := createMR(ctx, t, sess.individual, proj, branch, defaultBranch, "MR for raw diffs test")
	waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)

	var out mrchanges.RawDiffsOutput
	pollErr := Poll(ctx, 2*time.Second, 60*time.Second, func() (bool, string, error) {
		raw, err := callToolOn[mrchanges.RawDiffsOutput](ctx, sess.individual, "gitlab_mr_raw_diffs", mrchanges.RawDiffsInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		if err != nil {
			//nolint:nilerr // transient lookup failures are retried until the poll deadline
			return false, "get raw diffs: " + err.Error(), nil
		}
		out = raw
		if raw.RawDiff == "" {
			return false, "raw diff not yet computed", nil
		}
		return true, "raw diff available", nil
	})
	requireNoError(t, pollErr, "wait for raw diffs")
	requireTruef(t, out.MRIID == mr.IID, "expected MR IID %d, got %d", mr.IID, out.MRIID)
	requireTruef(t, strings.Contains(out.RawDiff, "mr-test.txt"), "expected raw diff to mention the feature-branch file, got %q", out.RawDiff)
	t.Logf("Got raw diff for MR !%d (%d bytes)", out.MRIID, len(out.RawDiff))
}

// TestIndividual_CICatalogGet exercises gitlab_get_catalog_resource via the
// individual tool surface.
//
// The test marks its own project fixture as a CI/CD catalog resource through
// the catalogResourcesCreate GraphQL mutation (the fixture already satisfies
// the description and README prerequisites), then fetches the resource by
// full path. Marking is version- and instance-policy-dependent, so mutation
// failures skip the test with the reported reason rather than failing it.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_CICatalogGet(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	const mutation = `mutation($projectPath: ID!) {
		catalogResourcesCreate(input: {projectPath: $projectPath}) {
			errors
		}
	}`
	var resp struct {
		Data struct {
			CatalogResourcesCreate struct {
				Errors []string `json:"errors"`
			} `json:"catalogResourcesCreate"`
		} `json:"data"`
	}
	if _, err := sess.glClient.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutation,
		Variables: map[string]any{"projectPath": proj.Path},
	}, &resp, gl.WithContext(ctx)); err != nil {
		t.Skipf("catalogResourcesCreate mutation unavailable on this GitLab instance: %v", err)
	}
	if len(resp.Data.CatalogResourcesCreate.Errors) > 0 {
		t.Skipf("could not mark project as a catalog resource: %v", resp.Data.CatalogResourcesCreate.Errors)
	}

	out, err := callToolOn[cicatalog.GetOutput](ctx, sess.individual, "gitlab_get_catalog_resource", cicatalog.GetInput{
		FullPath: proj.Path,
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "not found") {
		// An unreleased (draft) catalog resource may not be queryable on all
		// GitLab versions; the marking mutation above already succeeded.
		t.Skipf("catalog resource not queryable before its first release on this GitLab version: %v", err)
	}
	requireNoError(t, err, "get catalog resource")
	requireTruef(t, out.Resource.FullPath != "", "expected catalog resource full path, got empty")
	t.Logf("Got catalog resource %s (id=%s)", out.Resource.FullPath, out.Resource.ID)
}

// TestIndividual_DeploymentMergeRequests exercises
// gitlab_list_deployment_merge_requests via the individual tool surface.
//
// The test creates an environment and an API-driven deployment for the head
// commit of the default branch, then lists the merge requests shipped by the
// deployment. A deployment created directly through the API has no merged MR
// range behind it, so an empty list is the expected well-formed response;
// the assertion targets the successful listing shape.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_DeploymentMergeRequests(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	commit := commitFile(ctx, t, sess.individual, proj, defaultBranch, "deploy-mr.txt", "deployment content", "deployment commit")

	envName := uniqueName("dmr-env")
	_, envErr := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_create", environments.CreateInput{
		ProjectID: proj.pidOf(),
		Name:      envName,
	})
	requireNoError(t, envErr, "create environment")

	// GitLab 19 rejects deployment creation when the tag flag is omitted, so
	// state explicitly that the ref is a branch.
	dep, depErr := callToolOn[deployments.Output](ctx, sess.individual, "gitlab_deployment_create", deployments.CreateInput{
		ProjectID:   proj.pidOf(),
		Environment: envName,
		Ref:         defaultBranch,
		SHA:         commit.SHA,
		Tag:         new(false),
		Status:      "running",
	})
	requireNoError(t, depErr, "create deployment")
	requireTruef(t, dep.ID > 0, "expected positive deployment ID")

	out, err := callToolOn[deploymentmergerequests.ListOutput](ctx, sess.individual, "gitlab_list_deployment_merge_requests", deploymentmergerequests.ListInput{
		ProjectID:    proj.pidOf(),
		DeploymentID: int64(dep.ID),
	})
	requireNoError(t, err, "list deployment merge requests")
	t.Logf("Deployment %d has %d associated merge request(s)", dep.ID, len(out.MergeRequests))
}

// TestIndividual_WorkItemTypes exercises gitlab_list_work_item_types via the
// individual tool surface (GraphQL, read-only).
//
// The test lists the work item types available in a fresh project namespace
// and asserts the system-defined Issue type is present with a populated GID,
// which validates the GraphQL round-trip end to end on CE.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_WorkItemTypes(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	out, err := callToolOn[workitems.WorkItemTypeListOutput](ctx, sess.individual, "gitlab_list_work_item_types", workitems.ListWorkItemTypesInput{
		FullPath: proj.Path,
	})
	requireNoError(t, err, "list work item types")
	requireTruef(t, len(out.Types) > 0, "expected at least one work item type")

	foundIssue := false
	for _, wiType := range out.Types {
		requireTruef(t, wiType.ID != "", "expected work item type GID, got empty for %q", wiType.Name)
		if strings.EqualFold(wiType.Name, "issue") {
			foundIssue = true
		}
	}
	requireTruef(t, foundIssue, "expected the system-defined Issue work item type among %d types", len(out.Types))
	t.Logf("Listed %d work item types", len(out.Types))
}
