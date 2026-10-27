// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// projectstoragemoves_test.go covers a project's repository storage moves,
// which are a Free feature of a Gitaly deployment on every edition. A
// single-storage instance can still schedule a move: GitLab picks the
// destination by weight and lands the repository where it already is, which
// is the one move a throwaway project can be given. A move to a storage
// the instance lacks is refused, and a move nobody scheduled is a not-found.

package common

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two things nothing on an instance has: a move by this ID, and a
// Gitaly storage by this name.
const (
	missingMoveID  = int64(999999)
	missingStorage = "e2e-missing-storage"
)

// How often and for how long the scenario reads the move it scheduled
// while Sidekiq runs it, and how long a cleanup waits for a move the body
// failed before waiting for: half of the 60 seconds an unlicensed instance
// gives a test's whole cleanup, so the project's deletion keeps the rest.
const (
	storageMovePollInterval = 2 * time.Second
	storageMoveWait         = 120 * time.Second
	storageMoveCleanupWait  = 30 * time.Second
)

// storageMoveFinished is the state a move on a single-storage instance ends
// in: source and destination share a filesystem, so the worker copies and
// removes nothing and goes straight from started through replicated to
// finished.
const storageMoveFinished = "finished"

// storageMoveEndStates are the states RepositoryStorageMovable advances no
// further, as the API spells them: it answers the state's human name, so
// cleanup_failed reads "cleanup failed".
var storageMoveEndStates = []string{storageMoveFinished, "failed", "cleanup failed"}

// TestProjectStorageMoves_SingleStorage_SchedulesListsAndRefuses schedules
// a move of a project of the surface's own, finds it in both listings,
// reads a move that does not exist both ways, asks for a move to a storage
// the instance lacks, for the project and for every project, and waits for
// the scheduled move to finish.
//
// The wait is what keeps the project's cleanup from deleting the project
// while Sidekiq still moves it: the delete and the move's worker then lock
// the same project and namespace rows at once, Postgres ends the worker as
// a deadlock, and its retries look for a move that went with the project.
// On 19.5.0 that happened on a complete CE run and again on this scenario
// run alone (issue 1426). A body that fails before its wait leaves the same
// race behind, so a cleanup registered after the project's, and therefore
// run before it, waits in its place.
//
// Replaces: TestMeta_StorageMoves, TestMeta_ProjectStorageMoves_Graceful404
func TestProjectStorageMoves_SingleStorage_SchedulesListsAndRefuses(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("psm"))

		none := harness.Do[projectstoragemoves.ListOutput](s, actionStorageMoveRetrieveProject, map[string]any{"project_id": project.ID})
		if len(none.Moves) != 0 {
			e.T.Errorf("a fresh project lists %d storage moves: %+v", len(none.Moves), none.Moves)
		}

		scheduled := harness.Do[projectstoragemoves.Output](s, actionStorageMoveScheduleProject, map[string]any{"project_id": project.ID})
		if scheduled.ID == 0 {
			e.T.Fatalf("schedule_project answered %+v, want a move with an ID", scheduled)
		}
		waited := false
		e.Defer(fmt.Sprintf("the end of storage move %d", scheduled.ID), func(ctx context.Context) error {
			if waited {
				return nil
			}
			return awaitStorageMoveEnd(ctx, s, project.ID, scheduled.ID)
		})
		mine := harness.Do[projectstoragemoves.ListOutput](s, actionStorageMoveRetrieveProject, map[string]any{"project_id": project.ID})
		if !moveListed(mine.Moves, scheduled.ID) {
			e.T.Errorf("the project's moves do not hold the scheduled move %d: %+v", scheduled.ID, mine.Moves)
		}
		all := harness.Do[projectstoragemoves.ListOutput](s, actionStorageMoveRetrieveAllProject, map[string]any{"per_page": 100})
		if !moveListed(all.Moves, scheduled.ID) {
			e.T.Errorf("the instance's project moves do not hold the scheduled move %d among %d", scheduled.ID, len(all.Moves))
		}

		refused := harness.Refused(s, actionStorageMoveGetProject, map[string]any{"id": missingMoveID}, harness.FailureNotFound)
		e.T.Logf("the read of a missing move is refused: %s", firstLine(refused))
		refused = harness.Refused(s, actionStorageMoveGetProjectForProject, map[string]any{"project_id": project.ID, "id": missingMoveID}, harness.FailureNotFound)
		e.T.Logf("the read of a missing move of the project is refused: %s", firstLine(refused))

		refused = harness.ExpectToolError(s, actionStorageMoveScheduleProject, map[string]any{
			"project_id": project.ID, "destination_storage_name": missingStorage,
		}, "storage")
		assertMentions(e, "a move to a storage the instance lacks", refused, "destination_storage_name", "Gitaly")
		refused = harness.ExpectToolError(s, actionStorageMoveScheduleAllProject, map[string]any{
			"source_storage_name": missingStorage + "-source", "destination_storage_name": missingStorage,
		}, "storage")
		e.T.Logf("a move of every project between storages the instance lacks is refused: %s", firstLine(refused))

		// Each state is logged as it is first read, since the timeout
		// names none and the one a stuck move stays in tells a Sidekiq
		// that never took the job from a worker that never ended it.
		seen := ""
		ended := harness.Eventually[projectstoragemoves.Output](s, actionStorageMoveGetProjectForProject,
			map[string]any{"project_id": project.ID, "id": scheduled.ID}, storageMovePollInterval, storageMoveWait,
			func(move projectstoragemoves.Output) bool {
				if move.State != seen {
					e.T.Logf("the scheduled move %d reads %q", scheduled.ID, move.State)
					seen = move.State
				}
				return slices.Contains(storageMoveEndStates, move.State)
			})
		waited = true
		e.T.Logf("the scheduled move %d ended %q, from %s to %s", scheduled.ID, ended.State,
			ended.SourceStorageName, ended.DestinationStorageName)
		if ended.State != storageMoveFinished {
			e.T.Errorf("the scheduled move %d ended %q, want %q on a single-storage instance: %s",
				scheduled.ID, ended.State, storageMoveFinished, ended.ErrorMessage)
		}
	})
}

// awaitStorageMoveEnd holds a cleanup until the move reports a state it
// advances no further, and names the state it last read when the wait runs
// out. It asserts nothing about which end the move reached: the body that
// would have is the one that failed.
func awaitStorageMoveEnd(ctx context.Context, s *harness.Session, projectID, moveID int64) error {
	return harness.Poll(ctx, storageMovePollInterval, storageMoveCleanupWait, func() (bool, string, error) {
		move, err := harness.Undo[projectstoragemoves.Output](ctx, s, actionStorageMoveGetProjectForProject,
			map[string]any{"project_id": projectID, "id": moveID})
		if err != nil {
			return harness.WaitThrough(fmt.Sprintf("reading storage move %d", moveID), err)
		}
		return slices.Contains(storageMoveEndStates, move.State), fmt.Sprintf("storage move %d reads %q", moveID, move.State), nil
	})
}

// moveListed reports whether a move listing holds one by ID.
func moveListed(moves []projectstoragemoves.Output, id int64) bool {
	for _, move := range moves {
		if move.ID == id {
			return true
		}
	}
	return false
}
