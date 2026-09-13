//go:build e2e

// projectmirrors_test.go covers a project's push mirrors, the Free half of
// mirroring: one added, read, edited, listed, forced to sync and deleted,
// with a second project of the same instance as its target. The target is
// reached at the address GitLab has for itself inside the Docker network,
// which is why the scenario needs the Docker stack.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectmirrors"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// mirrorIDs lists the ids of a mirror listing.
func mirrorIDs(listed []projectmirrors.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, mirror := range listed {
		ids = append(ids, mirror.ID)
	}
	return ids
}

// TestProjectMirrors_Lifecycle_AddGetEditForcePushDelete adds a
// password-authenticated push mirror to a project of each surface's own,
// reads it back, shows the public key read refused for a mirror that has
// no key, widens it to every branch, lists it, forces a sync and deletes
// it.
//
// Replaces: TestMeta_ProjectRemoteMirrors, TestMeta_ProjectMirrorForcePush
func TestProjectMirrors_Lifecycle_AddGetEditForcePushDelete(t *testing.T) {
	e := harness.New(t)
	if !e.DockerMode() {
		e.Skipf("a push mirror targets the instance at the address it has for itself inside the Docker network, which only the Docker stack configures")
	}

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("mirrorsrc"))
		target := fixture.NewProject(e, fixture.WithNamePrefix("mirrordst"))
		params := map[string]any{"project_id": project.IDParam()}
		targetURL := fixture.MirrorTargetURL(e, target)

		empty := harness.Do[projectmirrors.ListOutput](s, actionProjectMirrorList, params)
		if len(empty.Mirrors) != 0 {
			e.T.Errorf("a fresh project lists the mirrors %v, want none", mirrorIDs(empty.Mirrors))
		}

		added := harness.Do[projectmirrors.Output](s, actionProjectMirrorAdd, withParams(params, map[string]any{
			"url": targetURL, "enabled": true, "auth_method": "password", "only_protected_branches": true,
		}))
		if added.ID == 0 || !added.Enabled || !added.OnlyProtectedBranches {
			e.T.Fatalf("mirror_add answered %+v, want an enabled mirror of protected branches with an ID", added)
		}
		mirror := withParams(params, map[string]any{"mirror_id": added.ID})

		got := harness.Do[projectmirrors.Output](s, actionProjectMirrorGet, mirror)
		if got.ID != added.ID || !got.Enabled {
			e.T.Errorf("mirror_get answered %+v, want the enabled mirror %d", got, added.ID)
		}
		// A password-authenticated mirror has no SSH key to show.
		refused := harness.Refused(s, actionProjectMirrorGetPublicKey, mirror, harness.FailureNotFound)
		assertMentions(e, "the public key read of a password mirror", refused, "ssh")

		edited := harness.Do[projectmirrors.Output](s, actionProjectMirrorEdit, withParams(mirror, map[string]any{"only_protected_branches": false}))
		if edited.ID != added.ID || edited.OnlyProtectedBranches {
			e.T.Errorf("mirror_edit answered %+v, want mirror %d widened to every branch", edited, added.ID)
		}
		listed := harness.Do[projectmirrors.ListOutput](s, actionProjectMirrorList, params)
		if ids := mirrorIDs(listed.Mirrors); len(ids) != 1 || ids[0] != added.ID {
			e.T.Errorf("the project lists the mirrors %v, want exactly %d", ids, added.ID)
		}

		assertForcePushAccepted(e, s, mirror)

		harness.DoVoid(s, actionProjectMirrorDelete, mirror)
		remaining := harness.Do[projectmirrors.ListOutput](s, actionProjectMirrorList, params)
		if len(remaining.Mirrors) != 0 {
			e.T.Errorf("the project still lists the mirrors %v after the delete", mirrorIDs(remaining.Mirrors))
		}
	})
}

// assertForcePushAccepted forces a sync of the mirror and accepts the one
// other answer GitLab gives: it throttles mirror syncs to one per interval,
// and the sync the add scheduled can still hold the slot, in which case the
// refusal names the throttle and nothing about the mirror.
func assertForcePushAccepted(e *harness.Env, s *harness.Session, mirror map[string]any) {
	e.T.Helper()
	forced, err := harness.Try[toolutil.DeleteOutput](s, actionProjectMirrorForcePush, mirror)
	switch {
	case err != nil && strings.Contains(err.Error(), "429"):
		e.T.Logf("the forced sync is throttled behind the one the add scheduled: %s", firstLine(err.Error()))
	case err != nil:
		e.T.Errorf("mirror_force_push failed for another reason than the throttle: %v", err)
	case !strings.Contains(forced.Message, "triggered"):
		e.T.Errorf("mirror_force_push answered %+v, want a message saying the sync was triggered", forced)
	}
}
