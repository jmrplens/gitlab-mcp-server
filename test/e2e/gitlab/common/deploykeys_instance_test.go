//go:build e2e

// deploykeys_instance_test.go covers the deploy key actions that reach past
// one project: enabling a project's key on a second project, the
// instance-wide listing and the per-user listing an administrator reads,
// and the instance-level key an administrator adds. The single-project
// lifecycle of a deploy key, add through delete, is the deploy keys family's
// own; the add and the delete here are the fixture around the enable.

package common

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// deployKeyPair is what the enable scenario stands on: the project a key is
// added to and the second project it is then enabled on.
type deployKeyPair struct {
	first  fixture.Project
	second fixture.Project
}

// TestDeployKeys_EnableAcrossProjects_AndAdminListings adds a key to one
// project per surface, finds it in the instance's listing, enables it on a
// second project, finds it among the run user's project deploy keys, and
// deletes it from both projects, which is what takes it off the instance.
// The two listings answer administrators only.
//
// Replaces: TestMeta_DeployKeysExtended
func TestDeployKeys_EnableAcrossProjects_AndAdminListings(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	rt := e.Runtime()

	harness.SurfacesWith(e, func(e *harness.Env) deployKeyPair {
		return deployKeyPair{
			first:  fixture.NewProject(e, fixture.WithNamePrefix("dkey-a")),
			second: fixture.NewProject(e, fixture.WithNamePrefix("dkey-b")),
		}
	}, func(e *harness.Env, surface harness.Surface, pair deployKeyPair) {
		s := e.On(surface)
		title := e.Name("dkey")

		added := harness.Do[deploykeys.Output](s, actionAccessDeployKeyAdd, map[string]any{"project_id": pair.first.IDParam(), "title": title, "key": mintSSHPublicKey(e)})
		if added.ID == 0 || added.Title != title {
			e.T.Fatalf("deploy_key_add answered %+v, want a key titled %q with an ID", added, title)
		}
		e.Defer("deploy key "+title, func(ctx context.Context) error {
			_, err := e.Client().GL().DeployKeys.DeleteDeployKey(pair.first.ID, added.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		everywhere := harness.Do[deploykeys.InstanceListOutput](s, actionAccessDeployKeyListAll, map[string]any{"per_page": 100})
		if !containsID(instanceDeployKeyIDs(everywhere.DeployKeys), added.ID) {
			e.T.Errorf("the instance's deploy keys do not hold %d: %d listed", added.ID, len(everywhere.DeployKeys))
		}

		enabled := harness.Do[deploykeys.Output](s, actionAccessDeployKeyEnable, map[string]any{"project_id": pair.second.IDParam(), "deploy_key_id": added.ID})
		if enabled.ID != added.ID {
			e.T.Errorf("deploy_key_enable on project %d answered key %d, want %d", pair.second.ID, enabled.ID, added.ID)
		}

		// The key is now enabled on two projects the run user owns, which is
		// what the per-user listing reports.
		mine := harness.Do[deploykeys.ListOutput](s, actionAccessDeployKeyListUserProject, map[string]any{"user_id": rt.Username, "per_page": 100})
		if !containsID(listedDeployKeyIDs(mine.DeployKeys), added.ID) {
			e.T.Errorf("the project deploy keys of %s do not hold %d: %d listed", rt.Username, added.ID, len(mine.DeployKeys))
		}

		// A delete removes the key from one project and leaves it on the
		// instance while another project still has it enabled, so the key
		// only leaves the instance's listing once both projects let go.
		harness.DoVoid(s, actionAccessDeployKeyDelete, map[string]any{"project_id": pair.second.IDParam(), "deploy_key_id": added.ID})
		harness.DoVoid(s, actionAccessDeployKeyDelete, map[string]any{"project_id": pair.first.IDParam(), "deploy_key_id": added.ID})
		after := harness.Do[deploykeys.InstanceListOutput](s, actionAccessDeployKeyListAll, map[string]any{"per_page": 100})
		if containsID(instanceDeployKeyIDs(after.DeployKeys), added.ID) {
			e.T.Errorf("deploy key %d is still listed on the instance after both projects deleted it", added.ID)
		}
	})
}

// TestDeployKeys_Instance_AddsOne adds an instance-level deploy key on
// every surface and checks the answer carries the key under the title it
// was given. GitLab offers no way to delete an instance-level key through
// the API, so the key stays, which is only acceptable on the disposable
// Docker instance.
//
// Replaces: TestIndividual_InstanceDeployKeyAdd
func TestDeployKeys_Instance_AddsOne(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	if !e.DockerMode() {
		e.Skipf("an instance-level deploy key cannot be deleted through the API, so one is only added to the ephemeral Docker instance")
	}

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		title := e.Name("instkey")

		added := harness.Do[deploykeys.InstanceOutput](s, actionAccessDeployKeyAddInstance, map[string]any{"title": title, "key": mintSSHPublicKey(e)})
		if added.ID == 0 || added.Title != title || added.Key == "" {
			e.T.Errorf("deploy_key_add_instance answered %+v, want a key titled %q with an ID", added, title)
		}
	})
}

// instanceDeployKeyIDs collects the IDs of the instance's listed deploy keys.
func instanceDeployKeyIDs(listed []deploykeys.InstanceOutput) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, key := range listed {
		ids = append(ids, key.ID)
	}
	return ids
}

// listedDeployKeyIDs collects the IDs of a project listing's deploy keys.
func listedDeployKeyIDs(listed []deploykeys.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, key := range listed {
		ids = append(ids, key.ID)
	}
	return ids
}
