//go:build e2e

// merge_train.go builds the world a merge train case runs in: a project whose
// namespace carries the licensed feature, with the two switches on, and a
// merge request in it.
//
// The project lives in a group on purpose. GitLab keeps merge_trains_enabled
// only on a project whose namespace holds the licensed feature; on a personal
// namespace the edit answers 200 and the flag is dropped, which is how the
// end-to-end suite this borrows from spent its first runs asserting on a
// project with no train.
//
// What this does not build is an entry on the train. A merge request with no
// pipeline of its own cannot board one, and a project whose pipelines nothing
// runs cannot give it one, so GitLab refuses the add and answers a not-found
// for the entry. That refusal is the answer a case gets, and it is GitLab's
// rather than the surface's, which is the distinction the scoring keeps.

package fixture

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// MergeTrain is the world a merge train case runs in.
type MergeTrain struct {
	// Project is the group project with the train switches on.
	Project Project
	// MergeRequest is the request a case names, which has not boarded.
	MergeRequest MergeRequest
}

// NewMergeTrain creates the group, the project in it with merge trains on,
// and a merge request ready to be named.
//
// A project whose switches GitLab did not keep fails the build rather than
// being handed back: a case running against it would be asking about a
// project with no train, which is not what the case claims to be about.
func NewMergeTrain(e *harness.Env) MergeTrain {
	e.T.Helper()

	group := NewGroup(e, WithGroupNamePrefix("mergetrain"))
	project := NewProject(e, WithNamePrefix("mergetrain"), InGroup(group))
	if !EnableMergeTrains(e, project) {
		e.T.Fatalf("GitLab did not keep merge trains enabled on %s, which is a group project on a licensed instance", project.Path)
	}

	branch := NewBranch(e, project, e.Name("train"))
	CommitFile(e, project, branch.Name, "merge-train.txt", "merge train fixture\n", "add the merge train fixture")
	mergeRequest := NewMergeRequest(e, project, branch.Name, project.DefaultBranch, "merge train fixture")

	return MergeTrain{Project: project, MergeRequest: mergeRequest}
}
