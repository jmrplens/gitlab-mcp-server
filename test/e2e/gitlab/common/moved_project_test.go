// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// moved_project_test.go covers a project named by a path it no longer has.
// GitLab keeps a redirect from every path a project had and follows a GET
// through it, and find_project! refuses every other method that names the
// project by an old path with 405, a HEAD included, whose answer has no body
// to say why. The server describes that refusal as what it is, so a model
// retries with the ID (issue 1133).

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// movedProjectRetry is the part of the moved project description that names
// the way out, which every refusal below has to carry.
const movedProjectRetry = "retry with the numeric project ID or the current path, both of which project.get returns for the old path"

// TestProject_MovedOldPath_ReadsFollowAndWritesNameTheRetry renames a
// project's path on every surface and then names it by the path it had: the
// read follows the redirect and answers the current path and ID; a file
// create, the file metadata read (a HEAD), a merge and an auto-merge cancel
// are refused, each with the description that names the retry, and the merge
// of a draft is not explained by the draft, which GitLab never looked at; the
// file create named by the ID then commits.
func TestProject_MovedOldPath_ReadsFollowAndWritesNameTheRetry(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) mergeRequestFixture {
		return newMergeRequestFixture(e, "moved")
	}, func(e *harness.Env, surface harness.Surface, f mergeRequestFixture) {
		s := e.On(surface)
		byID := map[string]any{"project_id": f.project.IDParam()}

		// Each surface moves the project once more, so the path it names as
		// old is the one the project had when that surface started.
		before := harness.Do[projects.Output](s, actionProjectGet, byID)
		renamed := harness.Do[projects.Output](s, actionProjectUpdate, withParams(byID, map[string]any{"path": e.Name("moved")}))
		if renamed.ID != f.project.ID || renamed.PathWithNamespace == before.PathWithNamespace {
			e.T.Fatalf("project update answered %d at %q, want project %d away from %q", renamed.ID, renamed.PathWithNamespace, f.project.ID, before.PathWithNamespace)
		}
		byOldPath := map[string]any{"project_id": before.PathWithNamespace}

		read := harness.Do[projects.Output](s, actionProjectGet, byOldPath)
		if read.ID != renamed.ID || read.PathWithNamespace != renamed.PathWithNamespace {
			e.T.Errorf("project get by the old path %q answered %d at %q, want %d at %q", before.PathWithNamespace, read.ID, read.PathWithNamespace, renamed.ID, renamed.PathWithNamespace)
		}

		path := string(surface) + "/moved.txt"
		file := map[string]any{"file_path": path, "branch": f.project.DefaultBranch, "content": "written by the ID\n", "commit_message": "chore: add " + path}
		harness.ExpectToolError(s, actionRepositoryFileCreate, withParams(byOldPath, file), movedProjectRetry)
		harness.ExpectToolError(s, actionRepositoryFileMetadata, withParams(byOldPath, map[string]any{"file_path": "README.md", "ref": f.project.DefaultBranch}), movedProjectRetry)

		// A draft is a blocker the merge's pre-fetch reads through the
		// redirect, so a merge that explained the 405 by the merge request's
		// state would name it. The title is the same on every surface, so
		// the update leaves a draft that an earlier surface made as it was.
		mergeRequest := map[string]any{"merge_request_iid": f.mr.IID}
		drafted := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate, withParams(byID, withParams(mergeRequest, map[string]any{"title": "Draft: " + f.mr.Title})))
		if !drafted.Draft {
			e.T.Fatalf("merge request !%d titled %q is not a draft, so its blockers would not explain a merge", f.mr.IID, drafted.Title)
		}
		if said := harness.ExpectToolError(s, actionMergeRequestMerge, withParams(byOldPath, mergeRequest), movedProjectRetry); strings.Contains(said, "cannot be merged") || strings.Contains(said, "is a draft") {
			e.T.Errorf("the merge by the old path was explained by a blocker GitLab never reached: %s", said)
		}
		harness.ExpectToolError(s, actionMergeRequestCancelAutoMerge, withParams(byOldPath, mergeRequest), movedProjectRetry)
		if open := harness.Do[mergerequests.Output](s, actionMergeRequestGet, withParams(byID, mergeRequest)); open.State != "opened" {
			e.T.Errorf("merge request !%d reads %q after the refused merge, want it still open", f.mr.IID, open.State)
		}

		created := harness.Do[files.FileInfoOutput](s, actionRepositoryFileCreate, withParams(byID, file))
		if created.FilePath != path || created.Branch != f.project.DefaultBranch {
			e.T.Errorf("file create by the ID answered %+v, want %s on %s", created, path, f.project.DefaultBranch)
		}
	})
}
