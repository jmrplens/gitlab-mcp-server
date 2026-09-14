//go:build e2e

// repository_test.go covers the repository-level reads and writes: the
// tree, a blob by SHA in both shapes, a comparison between two refs and the
// merge base they share, the contributors and the archive of a repository,
// the changelog GitLab generates from commit trailers and the commit it
// makes of it, and the submodule listing of a repository whose .gitmodules
// names a submodule the tree holds no gitlink for, which is the one shape a
// REST-provisioned repository can take.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/repositorysubmodules"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The file every repository scenario commits on the default branch, and the
// content the blob reads are checked against.
const (
	repositoryProbeFile    = "probe.go"
	repositoryProbeContent = "package main\n\n// e2e blob probe content\nfunc main() {}\n"
)

// blobSHAOf returns the blob SHA the tree lists for a file at the root, which
// is what the blob actions take rather than a path.
func blobSHAOf(e *harness.Env, tree repository.TreeOutput, name string) string {
	e.T.Helper()
	for _, node := range tree.Tree {
		if node.Name == name && node.Type == "blob" {
			return node.ID
		}
	}
	e.T.Fatalf("the tree lists no blob named %q: %+v", name, tree.Tree)
	return ""
}

// TestRepository_Explore_TreeBlobsCompareMergeBaseContributorsArchive lists
// the tree of a shared project on every surface, reads the probe file's
// blob by SHA in both shapes, creates a branch with a commit of its own and
// compares it with the default branch, resolves the merge base of the two
// to the commit the branch was created from, and reads the contributors
// and the archive of the repository.
//
// Replaces: TestIndividual_Repository, TestMeta_Repository, TestIndividual_RepositoryBlobs, TestIndividual_RepositoryMergeBase, TestMeta_RepositoryExplore
func TestRepository_Explore_TreeBlobsCompareMergeBaseContributorsArchive(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("repo"))
		fixture.CommitFile(e, project, project.DefaultBranch, repositoryProbeFile, repositoryProbeContent, "chore: add the blob probe")
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		tree := harness.Do[repository.TreeOutput](s, actionRepositoryTree, withParams(params, map[string]any{"ref": project.DefaultBranch}))
		sha := blobSHAOf(e, tree, repositoryProbeFile)

		blob := harness.Do[repository.BlobOutput](s, actionRepositoryBlob, withParams(params, map[string]any{"sha": sha}))
		if blob.SHA != sha || !strings.Contains(blob.Content, "blob probe") || blob.Size != len(repositoryProbeContent) {
			e.T.Errorf("blob answered %+v, want blob %s of %d bytes carrying the probe", blob, sha, len(repositoryProbeContent))
		}
		raw := harness.Do[repository.RawBlobContentOutput](s, actionRepositoryRawBlob, withParams(params, map[string]any{"sha": sha}))
		if raw.SHA != sha || raw.Content != repositoryProbeContent {
			e.T.Errorf("raw blob answered %+v, want blob %s with the probe's exact content", raw, sha)
		}

		// A branch of this surface's own, diverged by one commit, so the
		// comparison has a commit and a diff to show and the merge base is
		// the commit the branch started from.
		branch := fixture.NewBranch(e, project, e.Name("feature"))
		diverged := fixture.CommitFile(e, project, branch.Name, "extra-"+string(surface)+".go", "package main\n\nfunc extra() {}\n", "feat: diverge "+branch.Name)

		compared := harness.Do[repository.CompareOutput](s, actionRepositoryCompare, withParams(params, map[string]any{
			"from": project.DefaultBranch, "to": branch.Name,
		}))
		if len(compared.Commits) != 1 || compared.Commits[0].ID != diverged.SHA || len(compared.Diffs) != 1 {
			e.T.Errorf("compare %s..%s answered %d commits and %d diffs, want the one commit %s and its one diff",
				project.DefaultBranch, branch.Name, len(compared.Commits), len(compared.Diffs), diverged.ShortID)
		}

		base := harness.Do[commits.Output](s, actionRepositoryMergeBase, withParams(params, map[string]any{
			"refs": []string{project.DefaultBranch, branch.Name},
		}))
		if base.ID != branch.SHA {
			e.T.Errorf("merge base of %s and %s is %s, want the branch point %s", project.DefaultBranch, branch.Name, base.ID, branch.SHA)
		}

		contributors := harness.Do[repository.ContributorsOutput](s, actionRepositoryContributors, params)
		if len(contributors.Contributors) == 0 || contributors.Contributors[0].Commits == 0 {
			e.T.Errorf("contributors answered %+v, want at least one contributor with a commit count", contributors.Contributors)
		}

		archive := harness.Do[repository.ArchiveOutput](s, actionRepositoryArchive, params)
		if archive.URL == "" || archive.Format == "" {
			e.T.Errorf("archive answered %+v, want a download URL and a format", archive)
		}
	})
}

// changelogVersion is the version every surface generates notes for. Each
// surface works on a project of its own, so one version is enough.
const changelogVersion = "1.0.0"

// TestRepository_Changelog_GeneratesNotesAndCommitsThem seeds two commits on
// every surface, a range anchor and a commit carrying a Changelog trailer,
// generates the notes for the range between them and then commits them to
// CHANGELOG.md. The range is named explicitly because a fresh project has
// no previous version tag for GitLab to infer it from.
//
// The project is built per surface rather than shared: GitLab rate limits
// changelog requests per project, and the three surfaces' six calls against
// one project exhaust the allowance, which the shared shape hit as a 429 on
// the last of them.
//
// Replaces: TestIndividual_RepositoryChangelog
func TestRepository_Changelog_GeneratesNotesAndCommitsThem(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("changelog"))
		version := changelogVersion
		title := "feat: add the " + string(surface) + " changelog feature"

		anchor := fixture.CommitFile(e, project, project.DefaultBranch, "anchor-"+string(surface)+".txt", "range anchor", "chore: changelog range anchor")
		trailer := fixture.CommitFile(e, project, project.DefaultBranch, "feature-"+string(surface)+".txt", "feature content", title+"\n\nChangelog: added")

		params := map[string]any{"project_id": project.IDParam(), "version": version, "from": anchor.SHA, "to": trailer.SHA}
		generated := harness.Do[repository.ChangelogDataOutput](s, actionRepositoryChangelogGenerate, params)
		if !strings.Contains(generated.Notes, version) || !strings.Contains(generated.Notes, title) {
			e.T.Errorf("the generated notes do not carry version %q and the trailer commit's title %q: %q", version, title, generated.Notes)
		}

		added := harness.Do[repository.AddChangelogOutput](s, actionRepositoryChangelogAdd, params)
		if !added.Success || added.Version != version {
			e.T.Errorf("changelog add answered %+v, want success for version %s", added, version)
		}
	})
}

// gitmodulesContent declares one submodule, which the listing parses. The
// REST surface creates blobs and never a gitlink (tree entry mode 160000),
// so the submodule's pinned commit cannot exist here, and the two actions
// that need one are asserted on the refusal they give without it.
const gitmodulesContent = `[submodule "libs/dep"]
	path = libs/dep
	url = https://gitlab.example.com/e2e/dep.git
`

// submodulePath is the path gitmodulesContent declares.
const submodulePath = "libs/dep"

// zeroSHA is a commit no repository has, for the submodule update a fresh
// project refuses.
const zeroSHA = "0000000000000000000000000000000000000000"

// TestRepository_Submodules_ListsAndRefusesWithoutAGitlink commits a
// .gitmodules on a shared project, lists the submodule it declares on every
// surface, and asserts the two refusals a repository without a gitlink
// gives: a file read that finds no tree entry of type commit, and an update
// of a submodule that does not exist.
//
// Replaces: TestIndividual_RepositorySubmodules, TestMeta_SubmoduleUpdate
func TestRepository_Submodules_ListsAndRefusesWithoutAGitlink(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("submod"))
		fixture.CommitFile(e, project, project.DefaultBranch, ".gitmodules", gitmodulesContent, "chore: declare a submodule")
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		listed := harness.Do[repositorysubmodules.ListOutput](s, actionRepositoryListSubmodules, params)
		if listed.Count != 1 || len(listed.Submodules) != 1 || listed.Submodules[0].Path != submodulePath {
			e.T.Errorf("submodule list answered %+v, want the one submodule at %s", listed, submodulePath)
		} else if listed.Submodules[0].CommitSHA != "" {
			e.T.Errorf("the submodule lists commit %q, and no gitlink exists to pin one", listed.Submodules[0].CommitSHA)
		}

		refused := harness.Refused(s, actionRepositoryReadSubmoduleFile, withParams(params, map[string]any{
			"submodule_path": submodulePath, "file_path": "README.md",
		}), harness.FailureNotFound)
		// GitLab either answers the tree listing of the submodule's parent
		// directory with a 404, or lists it without an entry of type commit;
		// both name the tree and the path the read was for.
		assertMentions(e, "the submodule file read without a gitlink", refused, "tree", submodulePath)

		updateRefused := harness.ExpectToolError(s, actionRepositoryUpdateSubmodule, withParams(params, map[string]any{
			"submodule": "group/nonexistent-submodule", "branch": project.DefaultBranch,
			"commit_sha": zeroSHA, "commit_message": "chore: update a submodule that does not exist",
		}), "submodule")
		if !strings.Contains(updateRefused, "400") && !strings.Contains(updateRefused, "404") {
			e.T.Errorf("the update of a missing submodule was refused with neither 400 nor 404: %s", firstLine(updateRefused))
		}
	})
}
