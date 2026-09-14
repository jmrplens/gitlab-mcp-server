//go:build e2e

// mrreview_changes_test.go covers the review group's reads of what a merge
// request changes: the file diffs, the diff versions GitLab recorded and
// one version read by its ID, on every surface against one shared request.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMRReviewChanges_OneFileAdded_ChangesAndDiffVersionsShowIt reads the
// request's changes on every surface and finds the one file its branch
// adds, waits for the diff versions GitLab records and reads the first
// back by its ID.
//
// Replaces: TestMeta_MRReviewChanges
func TestMRReviewChanges_OneFileAdded_ChangesAndDiffVersionsShowIt(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) mergeRequestFixture {
		return newMergeRequestFixture(e, "mrchanges")
	}, func(e *harness.Env, surface harness.Surface, f mergeRequestFixture) {
		s := e.On(surface)
		params := f.params()

		// GitLab reports no changes while it is still preparing the diff,
		// which the fixture's readiness wait covers on a quiet instance and
		// a loaded one may outlast, so the read waits for the file.
		changes := harness.Eventually(s, actionMRReviewChangesGet, params, mergeRequestReadInterval, mergeRequestReadWait,
			func(out mrchanges.Output) bool { return fileChanged(out, f.commit.FilePath) })
		if changes.MRIID != f.mr.IID {
			e.T.Errorf("changes_get answered for request !%d, want !%d", changes.MRIID, f.mr.IID)
		}

		versions := harness.Eventually(s, actionMRReviewDiffVersionsList, params, mergeRequestReadInterval, mergeRequestReadWait,
			func(out mrchanges.DiffVersionsListOutput) bool { return len(out.DiffVersions) > 0 })
		first := versions.DiffVersions[0]
		if first.ID == 0 || first.HeadCommitSHA != f.commit.SHA {
			e.T.Errorf("the first diff version is %+v, want one with an ID whose head is the fixture commit %s", first, f.commit.ShortID)
		}

		version := harness.Do[mrchanges.DiffVersionOutput](s, actionMRReviewDiffVersionGet, withParams(params, map[string]any{"version_id": first.ID}))
		if version.ID != first.ID || version.HeadCommitSHA != first.HeadCommitSHA {
			e.T.Errorf("diff_version_get answered %+v, want version %d with head %s", version, first.ID, first.HeadCommitSHA)
		}
	})
}

// fileChanged reports whether a changes listing holds the file at its new
// path.
func fileChanged(out mrchanges.Output, path string) bool {
	for _, change := range out.Changes {
		if change.NewPath == path {
			return true
		}
	}
	return false
}
