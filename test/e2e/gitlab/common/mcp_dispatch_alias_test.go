//go:build e2e

// mcp_dispatch_alias_test.go covers two catalog actions that share one handler
// and one individual tool name, which is the shape the call recorder is
// strictest about.
//
// The recorder holds every call to the route its span says ran and fails the
// test when they differ: a call that named issue.list and ran issue.get proves
// nothing about issue.list. repository.file_history and repository.commit_list
// are the case that tests the rule, because they are one handler
// (internal/tools/commits/action_specs.go:36 gives file_history the commit
// listing's route and the gitlab_commit_list tool name) reached under two
// canonical IDs.
//
// What that does NOT mean, and what an earlier version of this file asserted
// wrongly: on the dynamic and meta surfaces there is no rewrite at all. Those
// resolvers key on canonical IDs, which are unique, so a call to
// repository.file_history dispatches repository.file_history
// (internal/tools/catalog_identity.go:73). Only the individual surface has to
// choose, because there one tool name can belong to one action, and the
// choice is the first in registration order rather than the last. That
// collapse is worth its own scenario and is not this one: whether the losing
// action is reachable there at all has to be settled before a test can be
// written for it.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// actionRepositoryFileHistory is the second ID on the commit listing's handler.
const actionRepositoryFileHistory harness.ActionID = "repository.file_history"

// TestDispatchAlias_FileHistory_RunsUnderItsOwnIDOnTheDynamicSurface calls both
// IDs and holds each to running as itself.
//
// The assertion is that sharing a handler does not make the calls
// interchangeable in the record: each answers with a commit listing, and each
// is recorded against the ID the caller named. A server that resolved one to
// the other would still answer correctly and would be caught here, which is
// the whole reason the recorder compares the call with the span.
//
// The World's shared project is used rather than a new one: this reads history,
// and a freshly created project has exactly the one commit its initialization
// made, which is enough and costs nothing.
func TestDispatchAlias_FileHistory_RunsUnderItsOwnIDOnTheDynamicSurface(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceDynamic)

	projectID, bound := world.BindPromptArgument("project_id")
	if !bound {
		t.Skip("the World binds no project_id, so there is no repository to read history from")
	}
	params := map[string]any{"project_id": projectID}

	// No ExpectDispatch on either call: the declaration is for a rewrite the
	// server performs, and on this surface it performs none. Passing one here
	// would assert the opposite of what the catalog does.
	history := harness.Do[commits.ListOutput](s, actionRepositoryFileHistory, params)
	if len(history.Commits) == 0 {
		t.Error("file_history answered an empty history for a project that has commits")
	}

	listed := harness.Do[commits.ListOutput](s, actionRepositoryCommitList, params)
	if len(listed.Commits) == 0 {
		t.Error("commit_list answered an empty listing for a project that has commits")
	}

	// One handler, so the same project gives the same head commit under both
	// IDs. This is what "they share a route" means from a caller's side, and
	// it is the part a future split of the two would have to keep or announce.
	if len(history.Commits) > 0 && len(listed.Commits) > 0 && history.Commits[0].ID != listed.Commits[0].ID {
		t.Errorf("file_history heads at %s and commit_list at %s, and they are one handler",
			history.Commits[0].ID, listed.Commits[0].ID)
	}
}

// TestDispatchAlias_FileHistory_CollapsesIntoCommitListOnTheIndividualSurface
// covers the one surface where the shared tool name forces a rewrite.
//
// There a tool name belongs to one action, so gitlab_commit_list is registered
// under whichever of the two comes first in registration order, and a call that
// named the other one runs as it. That collapse is what
// internal/tools/catalog_identity.go:73 records, along with the defect it
// caused: the resolver used to keep the last of the two and put every call to
// gitlab_commit_list under repository.file_history.
//
// This is the scenario harness.ExpectDispatch exists for. Without the
// declaration the recorder fails the call for running a route the caller did
// not name, which is the right default and the reason a deliberate rewrite has
// to be declared rather than tolerated.
func TestDispatchAlias_FileHistory_CollapsesIntoCommitListOnTheIndividualSurface(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceIndividual)

	projectID, bound := world.BindPromptArgument("project_id")
	if !bound {
		t.Skip("the World binds no project_id, so there is no repository to read history from")
	}
	if !s.Serves(actionRepositoryFileHistory) {
		// Worth saying rather than passing over: it would mean the losing
		// action of a shared tool name is unreachable on this surface, which
		// is a fact about the projection nothing else in the suite states.
		t.Skipf("%s is not served on the individual surface, so the shared tool name leaves it unreachable there",
			actionRepositoryFileHistory)
	}

	history := harness.Do[commits.ListOutput](s, actionRepositoryFileHistory,
		map[string]any{"project_id": projectID},
		harness.ExpectDispatch(actionRepositoryCommitList))
	if len(history.Commits) == 0 {
		t.Error("the collapsed call answered an empty history for a project that has commits")
	}
}
