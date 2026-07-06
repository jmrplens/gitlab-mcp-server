//go:build e2e && !enterprise

// packageextras_ce_test.go tests package registry MCP tools that were not
// covered by the core package lifecycle tests: group-level package listing,
// group-level container registry listing, the publish-and-link and
// publish-directory composite tools, and container registry protection rule
// updates.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releases"
)

// pkgExtrasCreateGroupProject creates a group and a project inside it via
// individual MCP tools, registering best-effort cleanup for both. Group-level
// package and registry listings need a project that lives in a group
// namespace, which the shared project fixtures (personal namespace) cannot
// provide.
func pkgExtrasCreateGroupProject(ctx context.Context, t *testing.T) (groups.Output, projects.Output) {
	t.Helper()

	groupName := uniqueName("e2e-pkgx-grp")
	grp, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
		Name:       groupName,
		Path:       groupName,
		Visibility: "private",
	})
	requireNoError(t, err, "create group fixture")
	//nolint:contextcheck // Cleanup runs after the test context is canceled, so it owns a fresh context.
	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancelCleanup()
		_ = callToolVoidOn(cleanupCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: i64soi(grp.ID),
		})
	})

	projName := uniqueName("e2e-pkgx-proj")
	proj, err := callToolOn[projects.Output](ctx, sess.individual, "gitlab_project_create", projects.CreateInput{
		Name:        projName,
		NamespaceID: int(grp.ID),
		Visibility:  "private",
	})
	requireNoError(t, err, "create project in group")
	//nolint:contextcheck // Cleanup runs after the test context is canceled, so it owns a fresh context.
	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancelCleanup()
		_ = callToolVoidOn(cleanupCtx, sess.individual, "gitlab_project_delete", projects.DeleteInput{
			ProjectID:         i64soi(proj.ID),
			PermanentlyRemove: true,
			FullPath:          proj.PathWithNamespace,
		})
	})

	return grp, proj
}

// TestIndividual_PackageGroupExtras exercises group-scoped package tools using
// individual MCP tools: group package listing and group container registry
// listing.
//
// The test creates a group with a project inside it, publishes a generic
// package into the project, polls the group package list until the package
// surfaces, and lists the group's container registry repositories. The
// registry listing asserts a successful (typically empty) response; it skips
// gracefully when the instance has the container registry disabled.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_PackageGroupExtras(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	grp, proj := pkgExtrasCreateGroupProject(ctx, t)

	const (
		pkgName    = "e2e-pkgx-group"
		pkgVersion = "1.0.0"
	)
	_, err := callToolOn[packages.PublishOutput](ctx, sess.individual, "gitlab_package_publish", packages.PublishInput{
		ProjectID:      i64soi(proj.ID),
		PackageName:    pkgName,
		PackageVersion: pkgVersion,
		FileName:       "group-data.txt",
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("group package payload")),
	})
	requireNoError(t, err, "publish generic package into group project")

	t.Run("GroupList", func(t *testing.T) {
		// Package indexing is quick but not guaranteed synchronous; poll
		// briefly so the assertion does not race the publish.
		maxWait := e2eTimeout(30*time.Second, 90*time.Second)
		pollCtx, cancelPoll := context.WithTimeout(ctx, maxWait)
		defer cancelPoll()

		found := false
		pErr := Poll(pollCtx, time.Second, maxWait, func() (bool, string, error) {
			out, callErr := callToolOn[packages.GroupListOutput](pollCtx, sess.individual, "gitlab_list_group_packages", packages.GroupListInput{
				GroupID: i64soi(grp.ID),
			})
			if callErr != nil {
				return false, fmt.Sprintf("group package list call: %v", callErr), nil
			}
			for _, pkg := range out.Packages {
				if pkg.Name == pkgName {
					found = true
					return true, "published package listed in group", nil
				}
			}
			return false, fmt.Sprintf("%d group packages, %s not yet listed", len(out.Packages), pkgName), nil
		})
		requireNoError(t, pErr, "poll group package list")
		requireTruef(t, found, "expected package %s in group %d package list", pkgName, grp.ID)
		t.Logf("Package %s visible in group %d", pkgName, grp.ID)
	})

	t.Run("RegistryListGroup", func(t *testing.T) {
		out, rErr := callToolOn[containerregistry.RepositoryListOutput](ctx, sess.individual, "gitlab_registry_list_group", containerregistry.ListGroupInput{
			GroupID: i64soi(grp.ID),
		})
		// The Docker fixture enables the container registry, so the call
		// succeeds there with an empty repository list (nothing pushes
		// images). The skip is a fallback for instances with the registry
		// disabled.
		if containerRegistryUnavailable(rErr) {
			t.Skipf("container registry not available on this instance: %v", rErr)
		}
		requireNoError(t, rErr, "list group registry repositories")
		requireTruef(t, len(out.Repositories) == 0, "expected no registry repositories in fresh group, got %d", len(out.Repositories))
		t.Logf("Group %d registry repositories: %d", grp.ID, len(out.Repositories))
	})
}

// TestIndividual_PackagePublishComposite exercises the composite generic
// package publish tools using individual MCP tools: publish-and-link and
// publish-directory.
//
// The test creates a project fixture with a release, publishes an in-memory
// file to the Generic Package Registry while linking it to the release in one
// call, and then publishes every file from a temporary local directory,
// asserting per-file results and totals. Both tools run against the
// in-process MCP server, so local temp paths are directly readable.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_PackagePublishComposite(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	const releaseTag = "v0.1.0-e2e-pkgx"
	_, err := callToolOn[releases.Output](ctx, sess.individual, "gitlab_release_create", releases.CreateInput{
		ProjectID: proj.pidOf(),
		TagName:   releaseTag,
		Ref:       defaultBranch,
		Name:      "E2E package composite release",
	})
	requireNoError(t, err, "create release fixture")

	t.Run("PublishAndLink", func(t *testing.T) {
		out, pErr := callToolOn[packages.PublishAndLinkOutput](ctx, sess.individual, "gitlab_package_publish_and_link", packages.PublishAndLinkInput{
			ProjectID:      proj.pidOf(),
			PackageName:    "e2e-pkgx-linked",
			PackageVersion: "1.0.0",
			FileName:       "artifact.txt",
			ContentBase64:  base64.StdEncoding.EncodeToString([]byte("linked artifact payload")),
			TagName:        releaseTag,
		})
		requireNoError(t, pErr, "publish and link package")
		requireTruef(t, out.Package.PackageID > 0, "expected non-zero published package ID")
		requireTruef(t, out.ReleaseLink.ID > 0, "expected non-zero release link ID")
		requireTruef(t, out.ReleaseLink.Name == "artifact.txt", "expected link name artifact.txt, got %q", out.ReleaseLink.Name)
		t.Logf("Published package %d and linked it to release %s (link %d)", out.Package.PackageID, releaseTag, out.ReleaseLink.ID)
	})

	t.Run("PublishDirectory", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"first.txt", "second.txt"} {
			requireNoError(t, os.WriteFile(filepath.Join(dir, name), []byte("directory payload "+name), 0o600), "write temp file "+name)
		}

		out, pErr := callToolOn[packages.PublishDirOutput](ctx, sess.individual, "gitlab_package_publish_directory", packages.PublishDirInput{
			ProjectID:      proj.pidOf(),
			PackageName:    "e2e-pkgx-dir",
			PackageVersion: "1.0.0",
			DirectoryPath:  dir,
		})
		requireNoError(t, pErr, "publish directory")
		requireTruef(t, out.TotalFiles == 2, "expected 2 published files, got %d", out.TotalFiles)
		requireTruef(t, len(out.Errors) == 0, "expected no per-file errors, got %v", out.Errors)
		requireTruef(t, out.TotalBytes > 0, "expected non-zero total bytes")
		t.Logf("Published %d files (%d bytes) from directory", out.TotalFiles, out.TotalBytes)
	})
}

// TestIndividual_PackageRegistryRules exercises the container registry
// protection rule update tool using individual MCP tools.
//
// The test creates a repository-path protection rule (create/list/delete are
// covered by the meta-tool registry tests), updates its path pattern through
// the update tool, and asserts the rule ID and pattern round-trip. A skip
// subtest documents the registry repository and tag actions that need a
// pushed container image and therefore stay uncovered until the Wave-4 image
// push enabler lands.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_PackageRegistryRules(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	createOut, err := callToolOn[containerregistry.ProtectionRuleOutput](ctx, sess.individual, "gitlab_registry_protection_create", containerregistry.CreateProtectionRuleInput{
		ProjectID:                   proj.pidOf(),
		RepositoryPathPattern:       proj.Path + "/e2e-rule*",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "maintainer",
	})
	requireNoError(t, err, "create registry protection rule fixture")
	requireTruef(t, createOut.ID > 0, "expected non-zero protection rule ID")
	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		_ = callToolVoidOn(cleanupCtx, sess.individual, "gitlab_registry_protection_delete", containerregistry.DeleteProtectionRuleInput{
			ProjectID: proj.pidOf(),
			RuleID:    createOut.ID,
		})
	})

	t.Run("RuleUpdate", func(t *testing.T) {
		updatedPattern := proj.Path + "/e2e-rule-updated*"
		out, uErr := callToolOn[containerregistry.ProtectionRuleOutput](ctx, sess.individual, "gitlab_registry_protection_update", containerregistry.UpdateProtectionRuleInput{
			ProjectID:             proj.pidOf(),
			RuleID:                createOut.ID,
			RepositoryPathPattern: updatedPattern,
		})
		requireNoError(t, uErr, "update registry protection rule")
		requireTruef(t, out.ID == createOut.ID, "expected rule ID %d, got %d", createOut.ID, out.ID)
		requireTruef(t, out.RepositoryPathPattern == updatedPattern, "expected pattern %q, got %q", updatedPattern, out.RepositoryPathPattern)
		t.Logf("Updated registry protection rule %d", out.ID)
	})

	t.Run("ImageBackedActionsPending", func(t *testing.T) {
		// The registry repository get/delete and tag get/list/delete/bulk
		// delete actions require a container repository with pushed image
		// tags. The Docker fixture enables the registry on :5050, but no
		// setup step pushes images yet, so those actions cannot succeed here.
		// Skipping before any tool call keeps the e2e gap audit honest: the
		// actions stay reported as uncovered until the Wave-4 image push
		// enabler (docker login/push in the fixture setup scripts) lands.
		t.Skip("container registry repository and tag actions require a pushed image; pending Wave-4 enabler")
	})
}
