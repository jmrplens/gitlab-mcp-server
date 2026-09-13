//go:build e2e

// groupstoragemoves_test.go covers the group half of the storage move
// actions, the half a license gates. A single-storage instance has nothing
// to move a group to, so the listings answer empty, a move nobody scheduled
// is a not-found, and a schedule naming a storage the instance does not
// have is refused. The project and snippet halves are Free and live in the
// common package.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingStorage is a Gitaly storage name no instance has.
const missingStorage = "e2e-missing-storage"

// TestGroupStorageMoves_SingleStorage_ListsRefusesAndCannotSchedule lists
// the group storage moves at both scopes on every surface, reads a move
// that does not exist both ways, and asks for a move to a storage the
// instance lacks, alone and for every group.
//
// Replaces: TestMeta_StorageMoves, TestMeta_GroupStorageMoves_Graceful404
func TestGroupStorageMoves_SingleStorage_ListsRefusesAndCannotSchedule(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("gsm"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)

		all := harness.Do[groupstoragemoves.ListOutput](s, actionStorageMoveRetrieveAllGroup, nil)
		e.T.Logf("%d group storage move(s) on the instance", len(all.Moves))
		mine := harness.Do[groupstoragemoves.ListOutput](s, actionStorageMoveRetrieveGroup, map[string]any{"group_id": group.ID})
		if len(mine.Moves) != 0 {
			e.T.Errorf("a fresh group lists %d storage moves: %+v", len(mine.Moves), mine.Moves)
		}

		refused := harness.Refused(s, actionStorageMoveGetGroup, map[string]any{"id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the read of a missing group move", refused, "storage move")
		refused = harness.Refused(s, actionStorageMoveGetGroupForGroup, map[string]any{"group_id": group.ID, "id": missingID}, harness.FailureNotFound)
		assertMentions(e, "the read of a missing move of the group", refused, "storage move")

		refused = harness.ExpectToolError(s, actionStorageMoveScheduleGroup, map[string]any{
			"group_id": group.ID, "destination_storage_name": missingStorage,
		}, "storage")
		e.T.Logf("a move to a storage the instance lacks is refused: %s", firstLine(refused))
		refused = harness.ExpectToolError(s, actionStorageMoveScheduleAllGroup, map[string]any{
			"source_storage_name": missingStorage + "-source", "destination_storage_name": missingStorage,
		}, "storage")
		e.T.Logf("a move of every group between storages the instance lacks is refused: %s", firstLine(refused))
	})
}
