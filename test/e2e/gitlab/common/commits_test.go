// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// commits_test.go covers a commit through the server: the reads of one and
// of the branch it is on, the comments and the statuses a commit carries,
// the trailers a listing parses out of its message, the signature an
// unsigned commit has none of, the two writes that make a new commit out of
// an existing one, and the merge requests GitLab associates a commit with
// once one is opened from its branch.

package common

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The association between a commit and the merge request opened from its
// branch is computed by a background job, so the read that shows it is
// polled at this cadence.
const (
	commitMRPollInterval = 2 * time.Second
	commitMRPollTimeout  = 90 * time.Second
)

// commitIDs returns the identifiers of a commit listing.
func commitIDs(listed []commits.Output) []string {
	ids := make([]string, 0, len(listed))
	for _, commit := range listed {
		ids = append(ids, commit.ID)
	}
	return ids
}

// TestCommit_Inspect_ListGetDiffRefsCommentsStatusesAndSignature makes one
// commit per surface on a shared project and reads it back every way the
// server offers: the listing of its branch, the commit itself, its diff and
// the file it wrote, the refs that reach it, a comment posted on it and the
// comments listing, a status set on it and the statuses listing, and the
// signature GitLab answers 404 for on a commit nobody signed.
//
// Replaces: TestIndividual_Commits, TestMeta_Commits, TestMeta_CommitExtended
func TestCommit_Inspect_ListGetDiffRefsCommentsStatusesAndSignature(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("commits"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		path := "notes-" + string(surface) + ".md"
		title := "docs: add the " + string(surface) + " notes"
		commit := fixture.CommitFile(e, project, project.DefaultBranch, path, "# notes\n", title)
		params := map[string]any{"project_id": project.IDParam()}
		bySHA := withParams(params, map[string]any{"sha": commit.SHA})

		listed := harness.Do[commits.ListOutput](s, actionRepositoryCommitList, withParams(params, map[string]any{"ref_name": project.DefaultBranch}))
		if !slices.Contains(commitIDs(listed.Commits), commit.SHA) {
			e.T.Errorf("the commit listing of %s does not hold %s: %v", project.DefaultBranch, commit.ShortID, commitIDs(listed.Commits))
		}

		got := harness.Do[commits.DetailOutput](s, actionRepositoryCommitGet, bySHA)
		if got.ID != commit.SHA || got.ShortID != commit.ShortID || got.Title != title {
			e.T.Errorf("commit get answered %+v, want %s titled %q", got, commit.ShortID, title)
		}

		diff := harness.Do[commits.DiffOutput](s, actionRepositoryCommitDiff, bySHA)
		if len(diff.Diffs) != 1 || diff.Diffs[0].NewPath != path {
			e.T.Errorf("commit diff answered %d diffs, want the one that added %s: %+v", len(diff.Diffs), path, diff.Diffs)
		}

		file := harness.Do[files.Output](s, actionRepositoryFileGet, withParams(params, map[string]any{"file_path": path, "ref": project.DefaultBranch}))
		if file.FileName != path || file.LastCommitID != commit.SHA {
			e.T.Errorf("file get of %s answered %+v, want the file last written by %s", path, file, commit.ShortID)
		}

		refs := harness.Do[commits.RefsOutput](s, actionRepositoryCommitRefs, bySHA)
		if !slices.ContainsFunc(refs.Refs, func(ref commits.RefOutput) bool { return ref.Name == project.DefaultBranch }) {
			e.T.Errorf("the refs of %s do not include %s: %+v", commit.ShortID, project.DefaultBranch, refs.Refs)
		}

		note := "e2e comment on " + string(surface)
		comment := harness.Do[commits.CommentOutput](s, actionRepositoryCommitCommentCreate, withParams(bySHA, map[string]any{"note": note}))
		if comment.Note != note {
			e.T.Errorf("commit comment create answered %+v, want the note %q", comment, note)
		}
		comments := harness.Do[commits.CommentsOutput](s, actionRepositoryCommitComments, bySHA)
		if !slices.ContainsFunc(comments.Comments, func(c commits.CommentOutput) bool { return c.Note == note }) {
			e.T.Errorf("the comments of %s do not hold %q: %+v", commit.ShortID, note, comments.Comments)
		}

		statusName := "e2e-" + string(surface)
		status := harness.Do[commits.StatusOutput](s, actionRepositoryCommitStatusSet, withParams(bySHA, map[string]any{"state": "success", "name": statusName}))
		if status.Status != "success" || status.Name != statusName || status.SHA != commit.SHA {
			e.T.Errorf("commit status set answered %+v, want a success status named %s on %s", status, statusName, commit.ShortID)
		}
		statuses := harness.Do[commits.StatusesOutput](s, actionRepositoryCommitStatuses, bySHA)
		if !slices.ContainsFunc(statuses.Statuses, func(st commits.StatusOutput) bool { return st.Name == statusName && st.Status == "success" }) {
			e.T.Errorf("the statuses of %s do not hold the success status %s: %+v", commit.ShortID, statusName, statuses.Statuses)
		}

		refused := harness.Refused(s, actionRepositoryCommitSignature, bySHA, harness.FailureNotFound)
		assertMentions(e, "the signature read of an unsigned commit", refused, "unsigned")
	})
}

// commitGetTool is the individual tool of repository.commit_get, named for
// the one raw call below: the embedded resource is a content block the
// projected verbs decode past.
const commitGetTool = "gitlab_commit_get"

// TestCommit_GetByBranch_EmbedsTheCommitUnderItsSHA asks for a commit by the
// name of its branch and holds the canonical resource embedded in the answer
// to the SHA the branch pointed at.
//
// The URI was expanded from the call's own sha parameter, so a commit asked
// for as "main" was embedded as gitlab://project/<id>/commit/main: a snapshot
// of one commit filed under a URI that reads whatever the branch points at by
// the time anyone reads it back.
func TestCommit_GetByBranch_EmbedsTheCommitUnderItsSHA(t *testing.T) {
	e := harness.New(t)

	harness.OnSurfaces(e, "the embedded block is written by the result tail every surface shares, and only the individual "+
		"tool's envelope can be read raw without re-spelling the dispatchers' call shapes",
		[]harness.Surface{harness.SurfaceIndividual}, func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)
			if !slices.Contains(s.Tools(), commitGetTool) {
				e.T.Fatalf("the %s session does not serve %s, so the raw read below would be refused as an unknown tool", surface, commitGetTool)
			}
			project := fixture.NewProject(e, fixture.WithNamePrefix("commitembed"))
			commit := fixture.CommitFile(e, project, project.DefaultBranch, "EMBED.md", "embed\n", "docs: add the embed fixture")

			result, err := s.Raw(&mcp.CallToolParams{Name: commitGetTool, Arguments: map[string]any{
				"project_id": project.IDParam(), "sha": project.DefaultBranch,
			}})
			if err != nil {
				e.T.Fatalf("%s: %v", commitGetTool, err)
			}
			if result.IsError {
				e.T.Fatalf("%s answered an error: %s", commitGetTool, firstLine(rawText(result)))
			}

			wantURI := fmt.Sprintf("gitlab://project/%d/commit/%s", project.ID, commit.SHA)
			embedded := embeddedResource(result)
			switch {
			case embedded == nil || embedded.Resource == nil:
				e.T.Errorf("%s carries no embedded resource block among %d content block(s)", commitGetTool, len(result.Content))
			case embedded.Resource.URI != wantURI:
				e.T.Errorf("the embedded resource is %q, want %q, the commit %s pointed at", embedded.Resource.URI, wantURI, project.DefaultBranch)
			}
		})
}

// TestCommit_Trailers_ListWithTrailersReadsEveryValueOfEachTrailer makes one
// commit per surface whose message ends in two Signed-off-by trailers and a
// Reviewed-by, and lists its branch with trailers set, the one route GitLab
// parses trailers on today. GitLab answers extended_trailers as each trailer
// mapped to the list of its values, which client-go's Commit cannot decode,
// so the listing used to fail as a whole (issue 1026); it now carries the
// commit with both Signed-off-by values in the order they were written.
func TestCommit_Trailers_ListWithTrailersReadsEveryValueOfEachTrailer(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("trailers"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		message := "docs: sign the " + string(surface) + " notes\n\n" +
			"Signed-off-by: Ada Lovelace <ada@example.com>\n" +
			"Signed-off-by: Grace Hopper <grace@example.com>\n" +
			"Reviewed-by: Alan Turing <alan@example.com>\n"
		created := harness.Do[commits.Output](s, actionRepositoryCommitCreate, withParams(params, map[string]any{
			"branch": project.DefaultBranch, "commit_message": message,
			"actions": []map[string]any{{
				"action": "create", "file_path": "signed-" + string(surface) + ".md",
				"content": "# signed\n",
			}},
		}))
		if created.ID == "" {
			e.T.Fatalf("commit create answered %+v, want a commit", created)
		}

		listed := harness.Do[commits.ListOutput](s, actionRepositoryCommitList,
			withParams(params, map[string]any{"ref_name": project.DefaultBranch, "trailers": true}))
		at := slices.IndexFunc(listed.Commits, func(c commits.Output) bool { return c.ID == created.ID })
		if at < 0 {
			e.T.Fatalf("the commit listing of %s does not hold %s: %v", project.DefaultBranch, created.ShortID, commitIDs(listed.Commits))
		}
		want := map[string][]string{
			"Signed-off-by": {"Ada Lovelace <ada@example.com>", "Grace Hopper <grace@example.com>"},
			"Reviewed-by":   {"Alan Turing <alan@example.com>"},
		}
		if got := listed.Commits[at].ExtendedTrailers; !maps.EqualFunc(got, want, slices.Equal[[]string]) {
			e.T.Errorf("extended_trailers of %s = %v, want %v", created.ShortID, got, want)
		}
	})
}

// TestCommit_Rewrite_CherryPicksRevertsAndFindsMergeRequests creates a
// branch per surface, cherry-picks a commit of the default branch onto it,
// reverts the cherry-pick there, and then opens a merge request from a
// second branch and waits until GitLab lists it against the branch's head
// commit.
//
// Replaces: TestMeta_CommitExtended, TestIndividual_CommitRevertMergeRequests
func TestCommit_Rewrite_CherryPicksRevertsAndFindsMergeRequests(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("rewrite"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		// The target branch is cut before the commit lands on the default
		// branch, so the cherry-pick has something to bring over.
		target := fixture.NewBranch(e, project, e.Name("target"))
		picked := fixture.CommitFile(e, project, project.DefaultBranch, "cherry-"+string(surface)+".txt", "only on the default branch", "feat: the commit to cherry-pick")

		cherry := harness.Do[commits.Output](s, actionRepositoryCommitCherryPick, withParams(params, map[string]any{"sha": picked.SHA, "branch": target.Name}))
		if cherry.ID == "" || cherry.ID == picked.SHA || cherry.Title != "feat: the commit to cherry-pick" {
			e.T.Fatalf("cherry-pick answered %+v, want a new commit on %s carrying the picked title", cherry, target.Name)
		}

		reverted := harness.Do[commits.Output](s, actionRepositoryCommitRevert, withParams(params, map[string]any{"sha": cherry.ID, "branch": target.Name}))
		if reverted.ID == "" || reverted.ID == cherry.ID {
			e.T.Errorf("revert answered %+v, want a new commit on %s undoing %s", reverted, target.Name, fixture.ShortSHA(cherry.ID))
		}

		source := fixture.NewBranch(e, project, e.Name("source"))
		head := fixture.CommitFile(e, project, source.Name, "mr-"+string(surface)+".txt", "content under review", "feat: the merge request's commit")
		mr := fixture.NewMergeRequest(e, project, source.Name, project.DefaultBranch, e.Name("mr"))

		associated := harness.Eventually(s, actionRepositoryCommitMergeRequests, withParams(params, map[string]any{"sha": head.SHA}),
			commitMRPollInterval, commitMRPollTimeout, func(out commits.MRsByCommitOutput) bool {
				return slices.ContainsFunc(out.MergeRequests, func(listed commits.BasicMROutput) bool { return listed.IID == mr.IID })
			})
		e.T.Logf("commit %s is associated with %d merge request(s), among them !%d", head.ShortID, len(associated.MergeRequests), mr.IID)
	})
}
