//go:build e2e

// packages_test.go covers the package registry through the server: the
// generic package lifecycle with the read of one package and its other
// versions, the group-level listings, the two composite
// publishes, the package protection rules, the container registry's own
// protection and tag protection rules, and the image-backed registry
// actions on the repository the provisioning script seeded with two tags,
// one seed per surface because deleting a tag consumes it.

package common

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/protectedpackages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The version and file every generic package here is published with, and
// the content the download is checked against. The lifecycle publishes a
// second version of its package as well, since only the read of one package
// names its other versions.
const (
	packageVersion      = "1.0.0"
	packageOtherVersion = "2.0.0"
	packageFileName     = "data.txt"
	packageFileContent  = "hello package"
)

// The tags the provisioning script pushes into each registry seed project.
const (
	seedTagKept    = "seed-a"
	seedTagDeleted = "seed-b"
)

// A package published into a project of a group takes a moment to appear
// in the group's listing, so that read is polled at this cadence.
const (
	groupPackagePollInterval = 2 * time.Second
	groupPackagePollTimeout  = 90 * time.Second
)

// packageNames returns the names of a package listing.
func packageNames(listed []packages.ListItem) []string {
	names := make([]string, 0, len(listed))
	for _, pkg := range listed {
		names = append(names, pkg.Name)
	}
	return names
}

// registryTagNames returns the names of a registry tag listing.
func registryTagNames(listed []containerregistry.TagOutput) []string {
	names := make([]string, 0, len(listed))
	for _, tag := range listed {
		names = append(names, tag.Name)
	}
	return names
}

// packageIDParam spells a package or package file identifier the way the
// parameters declared as strings take it on every surface.
func packageIDParam(id int64) string {
	return strconv.FormatInt(id, 10)
}

// publishPackage publishes one file as a generic package through the server
// and checks the answer names the package and the file.
func publishPackage(e *harness.Env, s *harness.Session, project fixture.Project, name string) packages.PublishOutput {
	e.T.Helper()
	return publishPackageVersion(e, s, project, name, packageVersion)
}

// publishPackageVersion is publishPackage for a version of the caller's
// choosing, which GitLab stores as a package of its own.
func publishPackageVersion(e *harness.Env, s *harness.Session, project fixture.Project, name, version string) packages.PublishOutput {
	e.T.Helper()

	published := harness.Do[packages.PublishOutput](s, actionPackagePublish, map[string]any{
		"project_id": project.IDParam(), "package_name": name, "package_version": version,
		"file_name": packageFileName, "content_base64": base64.StdEncoding.EncodeToString([]byte(packageFileContent)),
	})
	if published.PackageID == 0 || published.PackageFileID == 0 || published.FileName != packageFileName {
		e.T.Fatalf("package publish answered %+v, want %s %s with a package and a file ID", published, name, version)
	}
	return published
}

// assertPackageGet reads one package through the server and checks it is the
// package published as name at packageVersion, naming the other version among
// its other versions, which no listing sends.
func assertPackageGet(e *harness.Env, s *harness.Session, byPackage map[string]any, name string, published, other packages.PublishOutput) {
	e.T.Helper()

	got := harness.Do[packages.GetOutput](s, actionPackageGet, byPackage)
	if got.Package.ID != published.PackageID || got.Package.Name != name || got.Package.Version != packageVersion {
		e.T.Errorf("package get answered %d %q %q, want package %d, %s %s",
			got.Package.ID, got.Package.Name, got.Package.Version, published.PackageID, name, packageVersion)
	}
	if !slices.ContainsFunc(got.Package.Versions, func(v toolutil.PackageVersionOutput) bool {
		return v.ID == other.PackageID && v.Version == packageOtherVersion
	}) {
		e.T.Errorf("package get of %d lists the other versions %+v, want %s (%d) among them",
			published.PackageID, got.Package.Versions, packageOtherVersion, other.PackageID)
	}
	e.T.Logf("package %d was published by user %d", got.Package.ID, got.Package.CreatorID)
}

// TestPackage_GenericLifecycle_PublishListDownloadDelete publishes a
// generic package per surface into a shared project, and a second version
// of it, finds it in the listing, reads it with the second version named
// among its other versions, lists its one file, downloads the file into
// the test's own directory and compares its content, deletes the file and
// then the package, asserts the read of it is refused as not found, deletes
// the second version, and asserts the listing no longer holds the name.
//
// Replaces: TestPackages
func TestPackage_GenericLifecycle_PublishListDownloadDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("packages"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		name := "e2e-pkg-" + string(surface)
		params := map[string]any{"project_id": project.IDParam()}

		published := publishPackage(e, s, project, name)
		other := publishPackageVersion(e, s, project, name, packageOtherVersion)
		byPackage := withParams(params, map[string]any{"package_id": packageIDParam(published.PackageID)})

		listed := harness.Do[packages.ListOutput](s, actionPackageList, params)
		if !slices.Contains(packageNames(listed.Packages), name) {
			e.T.Errorf("the package listing does not hold %s: %v", name, packageNames(listed.Packages))
		}
		assertPackageGet(e, s, byPackage, name, published, other)
		files := harness.Do[packages.FileListOutput](s, actionPackageFileList, byPackage)
		if len(files.Files) != 1 || files.Files[0].FileName != packageFileName || files.Files[0].PackageFileID != published.PackageFileID {
			e.T.Errorf("the file listing of package %d answered %+v, want the one file %s (%d)", published.PackageID, files.Files, packageFileName, published.PackageFileID)
		}

		// The download lands in the test's temporary directory, which is
		// under the OS temporary directory every child may write into.
		outputPath := filepath.Join(e.T.TempDir(), "downloaded.txt")
		downloaded := harness.Do[packages.DownloadOutput](s, actionPackageDownload, withParams(params, map[string]any{
			"package_name": name, "package_version": packageVersion, "file_name": packageFileName, "output_path": outputPath,
		}))
		content, err := os.ReadFile(outputPath)
		if err != nil || downloaded.Size != int64(len(packageFileContent)) || string(content) != packageFileContent {
			e.T.Errorf("package download answered %+v and wrote %q (%v), want %d bytes reading %q", downloaded, content, err, len(packageFileContent), packageFileContent)
		}

		harness.DoVoid(s, actionPackageFileDelete, withParams(byPackage, map[string]any{"package_file_id": packageIDParam(published.PackageFileID)}))
		harness.DoVoid(s, actionPackageDelete, byPackage)
		refused := harness.Refused(s, actionPackageGet, byPackage, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
		harness.DoVoid(s, actionPackageDelete, withParams(params, map[string]any{"package_id": packageIDParam(other.PackageID)}))
		remaining := harness.Do[packages.ListOutput](s, actionPackageList, params)
		if slices.Contains(packageNames(remaining.Packages), name) {
			e.T.Errorf("the package listing still holds %s after its delete: %v", name, packageNames(remaining.Packages))
		}
	})
}

// groupPackageFixture is a group and a project inside it, since the group
// listings need a project with a group namespace.
type groupPackageFixture struct {
	group   fixture.Group
	project fixture.Project
}

// groupPackageNames returns the names of a group package listing.
func groupPackageNames(listed []packages.GroupListItem) []string {
	names := make([]string, 0, len(listed))
	for _, pkg := range listed {
		names = append(names, pkg.Name)
	}
	return names
}

// TestPackage_GroupListings_ListsPackagesAndRegistryRepositories publishes
// a package per surface into a project of a shared group, waits for the
// group listing to show it, and lists the group's container registry
// repositories, which are none on the Docker instance and an error naming
// the registry on one that does not serve it.
//
// Replaces: TestIndividual_PackageGroupExtras
func TestPackage_GroupListings_ListsPackagesAndRegistryRepositories(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) groupPackageFixture {
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("pkggrp"))
		return groupPackageFixture{group: group, project: fixture.NewProject(e, fixture.InGroup(group), fixture.WithNamePrefix("pkggrp"))}
	}, func(e *harness.Env, surface harness.Surface, f groupPackageFixture) {
		s := e.On(surface)
		name := "e2e-grp-pkg-" + string(surface)
		scope := map[string]any{"group_id": f.group.IDParam()}

		publishPackage(e, s, f.project, name)
		inGroup := harness.Eventually(s, actionPackageGroupList, scope, groupPackagePollInterval, groupPackagePollTimeout,
			func(out packages.GroupListOutput) bool { return slices.Contains(groupPackageNames(out.Packages), name) })
		e.T.Logf("group %d lists %d package(s), among them %s", f.group.ID, len(inGroup.Packages), name)

		repositories, err := harness.Try[containerregistry.RepositoryListOutput](s, actionPackageRegistryListGroup, scope)
		switch {
		case err != nil:
			assertMentions(e, "the group registry listing on an instance without a registry", err.Error(), "registry")
			e.T.Logf("the instance does not serve the container registry: %s", firstLine(err.Error()))
		case len(repositories.Repositories) != 0:
			e.T.Errorf("the registry of a fresh group lists %d repositories, want none", len(repositories.Repositories))
		}
	})
}

// TestPackage_Composite_PublishAndLinkAndPublishDirectory creates a release
// per surface on a shared project, publishes one file while linking it to
// the release in one call, and publishes every file of a directory of the
// test's own in another, asserting the per-file results and the totals.
//
// Replaces: TestIndividual_PackagePublishComposite
func TestPackage_Composite_PublishAndLinkAndPublishDirectory(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("pkgcomp"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		tag := e.Name("v0.1.0")

		release := harness.Do[releases.Output](s, actionReleaseCreate, withParams(params, map[string]any{
			"tag_name": tag, "ref": project.DefaultBranch, "name": "e2e package composite release",
		}))
		if release.TagName != tag {
			e.T.Fatalf("release create answered %+v, want a release on %s", release, tag)
		}

		linked := harness.Do[packages.PublishAndLinkOutput](s, actionPackagePublishAndLink, withParams(params, map[string]any{
			"package_name": "e2e-linked-" + string(surface), "package_version": packageVersion, "file_name": "artifact.txt",
			"content_base64": base64.StdEncoding.EncodeToString([]byte("linked artifact payload")), "tag_name": tag,
		}))
		if linked.Package.PackageID == 0 || linked.ReleaseLink.ID == 0 || linked.ReleaseLink.Name != "artifact.txt" || linked.ReleaseLink.URL != linked.Package.URL {
			e.T.Errorf("publish and link answered %+v, want a published package and a release link named artifact.txt pointing at it", linked)
		}

		dir := e.T.TempDir()
		for _, name := range []string{"first.txt", "second.txt"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("directory payload "+name), 0o600); err != nil {
				e.T.Fatalf("writing %s: %v", name, err)
			}
		}
		published := harness.Do[packages.PublishDirOutput](s, actionPackagePublishDirectory, withParams(params, map[string]any{
			"package_name": "e2e-dir-" + string(surface), "package_version": packageVersion, "directory_path": dir,
		}))
		if published.TotalFiles != 2 || len(published.Published) != 2 || len(published.Errors) != 0 || published.TotalBytes == 0 {
			e.T.Errorf("publish directory answered %+v, want both files published with no error", published)
		}
	})
}

// TestPackage_ProtectionRules_Lifecycle creates a package protection rule
// per surface on a shared project, finds it in the listing, changes its
// pattern, deletes it and asserts the listing no longer holds it.
//
// Replaces: TestMeta_PackagesProtectionRules
func TestPackage_ProtectionRules_Lifecycle(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("pkgrules"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		pattern := "e2e-" + string(surface) + "-*"

		created := harness.Do[protectedpackages.Output](s, actionPackageProtectionRuleCreate, withParams(params, map[string]any{
			"package_name_pattern": pattern, "package_type": "generic", "minimum_access_level_for_push": "maintainer",
		}))
		if created.ID == 0 || created.PackageNamePattern != pattern || created.PackageType != "generic" {
			e.T.Fatalf("package protection rule create answered %+v, want a generic rule on %q with an ID", created, pattern)
		}
		byRule := withParams(params, map[string]any{"rule_id": created.ID})

		listed := harness.Do[protectedpackages.ListOutput](s, actionPackageProtectionRuleList, params)
		if !slices.ContainsFunc(listed.Rules, func(rule protectedpackages.Output) bool { return rule.ID == created.ID }) {
			e.T.Errorf("the package protection rule listing does not hold %d: %d listed", created.ID, len(listed.Rules))
		}
		// The package type goes with every update: GitLab validates the rule
		// as a whole and answers 422 "Package type can't be blank" for a
		// change that names only the pattern.
		updatedPattern := "e2e-" + string(surface) + "-updated-*"
		updated := harness.Do[protectedpackages.Output](s, actionPackageProtectionRuleUpdate, withParams(byRule, map[string]any{
			"package_name_pattern": updatedPattern, "package_type": "generic",
		}))
		if updated.ID != created.ID || updated.PackageNamePattern != updatedPattern || updated.PackageType != "generic" {
			e.T.Errorf("package protection rule update answered %+v, want rule %d on %q", updated, created.ID, updatedPattern)
		}

		harness.DoVoid(s, actionPackageProtectionRuleDelete, byRule)
		remaining := harness.Do[protectedpackages.ListOutput](s, actionPackageProtectionRuleList, params)
		if slices.ContainsFunc(remaining.Rules, func(rule protectedpackages.Output) bool { return rule.ID == created.ID }) {
			e.T.Errorf("the package protection rule listing still holds %d after its delete", created.ID)
		}
	})
}

// registryUnavailable reports whether a container registry answer says the
// instance does not serve one: the 422 GitLab gives with the registry
// disabled, or the 404 of a release without the tag rules API.
func registryUnavailable(err error) bool {
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "container registry api not supported") || strings.Contains(lowered, "404")
}

// TestPackage_RegistryRules_RepositoryAndTagRulesLifecycle lists the
// container repositories of a fresh shared project, which are none, and
// walks both rule families on every surface: a repository path rule is
// created, listed, updated and deleted, and a tag rule the same way where
// the instance serves the registry, with the answer asserted otherwise.
//
// Replaces: TestMeta_PackagesRegistry, TestIndividual_PackageRegistryRules
func TestPackage_RegistryRules_RepositoryAndTagRulesLifecycle(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("regrules"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		repositories := harness.Do[containerregistry.RepositoryListOutput](s, actionPackageRegistryListProject, params)
		if len(repositories.Repositories) != 0 {
			e.T.Errorf("the registry of a fresh project lists %d repositories, want none", len(repositories.Repositories))
		}

		// A rule's pattern has to start with the project's own path, which
		// the registry spells in lower case.
		pattern := strings.ToLower(project.Path) + "/e2e-" + string(surface) + "*"
		updatedPattern := strings.ToLower(project.Path) + "/e2e-" + string(surface) + "-updated*"
		rule := harness.Do[containerregistry.ProtectionRuleOutput](s, actionPackageRegistryRuleCreate, withParams(params, map[string]any{
			"repository_path_pattern": pattern, "minimum_access_level_for_push": "maintainer", "minimum_access_level_for_delete": "maintainer",
		}))
		if rule.ID == 0 || rule.RepositoryPathPattern != pattern {
			e.T.Fatalf("registry rule create answered %+v, want a rule on %q with an ID", rule, pattern)
		}
		byRule := withParams(params, map[string]any{"rule_id": rule.ID})
		rules := harness.Do[containerregistry.ProtectionRuleListOutput](s, actionPackageRegistryRuleList, params)
		if !slices.ContainsFunc(rules.Rules, func(r containerregistry.ProtectionRuleOutput) bool { return r.ID == rule.ID }) {
			e.T.Errorf("the registry rule listing does not hold %d: %d listed", rule.ID, len(rules.Rules))
		}
		ruleUpdated := harness.Do[containerregistry.ProtectionRuleOutput](s, actionPackageRegistryRuleUpdate, withParams(byRule, map[string]any{"repository_path_pattern": updatedPattern}))
		if ruleUpdated.ID != rule.ID || ruleUpdated.RepositoryPathPattern != updatedPattern {
			e.T.Errorf("registry rule update answered %+v, want rule %d on %q", ruleUpdated, rule.ID, updatedPattern)
		}
		harness.DoVoid(s, actionPackageRegistryRuleDelete, byRule)
		rulesAfter := harness.Do[containerregistry.ProtectionRuleListOutput](s, actionPackageRegistryRuleList, params)
		if slices.ContainsFunc(rulesAfter.Rules, func(r containerregistry.ProtectionRuleOutput) bool { return r.ID == rule.ID }) {
			e.T.Errorf("the registry rule listing still holds %d after its delete", rule.ID)
		}

		// The tag rules API answers only where the registry is served; the
		// Docker instance serves it, and an instance that does not is
		// asserted on the refusal it gives.
		tagRules, err := harness.Try[containerregistry.TagProtectionRuleListOutput](s, actionPackageRegistryTagRuleList, params)
		if err != nil {
			if !registryUnavailable(err) {
				e.T.Fatalf("the registry tag rule listing was refused for a reason other than a missing registry: %v", err)
			}
			e.T.Logf("the instance does not serve the container registry tag rules: %s", firstLine(err.Error()))
			return
		}
		e.T.Logf("the project starts with %d registry tag rule(s)", len(tagRules.Rules))

		tagRule := harness.Do[containerregistry.TagProtectionRuleOutput](s, actionPackageRegistryTagRuleCreate, withParams(params, map[string]any{
			"tag_name_pattern": "v.+", "minimum_access_level_for_push": "maintainer", "minimum_access_level_for_delete": "maintainer",
		}))
		if tagRule.ID == 0 || tagRule.TagNamePattern != "v.+" {
			e.T.Fatalf("registry tag rule create answered %+v, want a rule on v.+ with an ID", tagRule)
		}
		byTagRule := withParams(params, map[string]any{"rule_id": tagRule.ID})
		tagRuleUpdated := harness.Do[containerregistry.TagProtectionRuleOutput](s, actionPackageRegistryTagRuleUpdate, withParams(byTagRule, map[string]any{"tag_name_pattern": "release-.+"}))
		if tagRuleUpdated.ID != tagRule.ID || tagRuleUpdated.TagNamePattern != "release-.+" {
			e.T.Errorf("registry tag rule update answered %+v, want rule %d on release-.+", tagRuleUpdated, tagRule.ID)
		}
		harness.DoVoid(s, actionPackageRegistryTagRuleDelete, byTagRule)
		tagRulesAfter := harness.Do[containerregistry.TagProtectionRuleListOutput](s, actionPackageRegistryTagRuleList, params)
		if slices.ContainsFunc(tagRulesAfter.Rules, func(r containerregistry.TagProtectionRuleOutput) bool { return r.ID == tagRule.ID }) {
			e.T.Errorf("the registry tag rule listing still holds %d after its delete", tagRule.ID)
		}
	})
}

// TestPackage_RegistryImages_ReadsAndDeletesTheSeededTags reads the
// registry seed project provisioned for each surface: its one repository,
// with its tag count, the two seeded tags and one of them by name; then
// deletes the other by name, asks for the rest to be deleted in bulk by a
// pattern, and deletes the repository. The last two are scheduled by
// GitLab rather than done in the request, and are asserted on what they
// answer.
//
// Replaces: TestMeta_PackageRegistryImages, TestIndividual_PackageRegistryRules
func TestPackage_RegistryImages_ReadsAndDeletesTheSeededTags(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		seed := fixture.RegistryProject(e, surface)
		params := map[string]any{"project_id": seed}

		listed := harness.Do[containerregistry.RepositoryListOutput](s, actionPackageRegistryListProject, withParams(params, map[string]any{"tags_count": true}))
		if len(listed.Repositories) != 1 || listed.Repositories[0].ID == 0 {
			e.T.Fatalf("the registry of %s lists %+v, want the one seeded repository", seed, listed.Repositories)
		}
		repository := listed.Repositories[0]
		byRepository := withParams(params, map[string]any{"repository_id": repository.ID})

		got := harness.Do[containerregistry.RepositoryOutput](s, actionPackageRegistryGet, map[string]any{"repository_id": repository.ID, "tags_count": true})
		if got.ID != repository.ID || got.TagsCount != 2 {
			e.T.Errorf("registry get answered %+v, want repository %d with the two seeded tags", got, repository.ID)
		}
		tags := harness.Do[containerregistry.TagListOutput](s, actionPackageRegistryTagList, byRepository)
		if !slices.Contains(registryTagNames(tags.Tags), seedTagKept) || !slices.Contains(registryTagNames(tags.Tags), seedTagDeleted) {
			e.T.Errorf("the tags of repository %d are %v, want %s and %s", repository.ID, registryTagNames(tags.Tags), seedTagKept, seedTagDeleted)
		}
		tag := harness.Do[containerregistry.TagOutput](s, actionPackageRegistryTagGet, withParams(byRepository, map[string]any{"tag_name": seedTagKept}))
		if tag.Name != seedTagKept || tag.Digest == "" {
			e.T.Errorf("registry tag get answered %+v, want %s with a digest", tag, seedTagKept)
		}

		deleted := harness.Do[toolutil.DeleteOutput](s, actionPackageRegistryTagDelete, withParams(byRepository, map[string]any{"tag_name": seedTagDeleted}))
		if deleted.Status != "success" {
			e.T.Errorf("registry tag delete answered %+v, want success", deleted)
		}
		remaining := harness.Eventually(s, actionPackageRegistryTagList, byRepository, groupPackagePollInterval, groupPackagePollTimeout,
			func(out containerregistry.TagListOutput) bool {
				return !slices.Contains(registryTagNames(out.Tags), seedTagDeleted)
			})
		if !slices.Contains(registryTagNames(remaining.Tags), seedTagKept) {
			e.T.Errorf("the tags after the delete of %s are %v, want %s kept", seedTagDeleted, registryTagNames(remaining.Tags), seedTagKept)
		}

		bulk := harness.Do[toolutil.DeleteOutput](s, actionPackageRegistryTagDeleteBulk, withParams(byRepository, map[string]any{"name_regex_delete": "seed-.*"}))
		if bulk.Status != "success" {
			e.T.Errorf("registry tag delete bulk answered %+v, want the deletion scheduled", bulk)
		}
		repositoryDeleted := harness.Do[toolutil.DeleteOutput](s, actionPackageRegistryDelete, byRepository)
		if repositoryDeleted.Status != "success" {
			e.T.Errorf("registry delete answered %+v, want the deletion scheduled", repositoryDeleted)
		}
	})
}
