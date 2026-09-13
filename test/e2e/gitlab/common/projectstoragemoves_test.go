//go:build e2e

// projectstoragemoves_test.go covers a project's repository storage moves,
// which are a Free feature of a Gitaly deployment on every edition. A
// single-storage instance can still schedule a move: GitLab picks the
// destination by weight and lands the repository where it already is, which
// is the one move a throwaway project can be given. A move to a storage
// the instance lacks is refused, and a move nobody scheduled is a not-found.

package common

import (
	"testing"

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

// TestProjectStorageMoves_SingleStorage_SchedulesListsAndRefuses schedules
// a move of a project of the surface's own, finds it in both listings,
// reads a move that does not exist both ways, and asks for a move to a
// storage the instance lacks, for the project and for every project.
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
