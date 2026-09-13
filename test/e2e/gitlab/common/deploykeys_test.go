//go:build e2e

// deploykeys_test.go covers a project's deploy keys through the access
// group: adding one from a fresh public key, reading and listing it,
// renaming it and giving it push access, and deleting it.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// deployKeyIDs returns the identifiers of a deploy key listing.
func deployKeyIDs(listed []deploykeys.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, key := range listed {
		ids = append(ids, key.ID)
	}
	return ids
}

// TestDeployKey_Lifecycle_AddGetListUpdateDelete adds a deploy key per
// surface to a shared project from a key pair generated for it, reads and
// lists it, renames it and lets it push, deletes it and asserts the read
// afterwards is refused as not found.
//
// Replaces: TestIndividual_DeployKeys, TestMeta_DeployKeys
func TestDeployKey_Lifecycle_AddGetListUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("deploykeys"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("deploy-key")
		publicKey, err := fixture.SSHPublicKey()
		if err != nil {
			e.T.Fatal(err)
		}

		added := harness.Do[deploykeys.Output](s, actionAccessDeployKeyAdd, withParams(params, map[string]any{"title": title, "key": publicKey}))
		if added.ID == 0 || added.Title != title || added.CanPush {
			e.T.Fatalf("deploy key add answered %+v, want %q without push access and with an ID", added, title)
		}
		byKey := withParams(params, map[string]any{"deploy_key_id": added.ID})

		got := harness.Do[deploykeys.Output](s, actionAccessDeployKeyGet, byKey)
		if got.ID != added.ID || got.Title != title {
			e.T.Errorf("deploy key get answered %+v, want key %d %q", got, added.ID, title)
		}
		listed := harness.Do[deploykeys.ListOutput](s, actionAccessDeployKeyListProject, params)
		if !containsID(deployKeyIDs(listed.DeployKeys), added.ID) {
			e.T.Errorf("the deploy key listing does not hold %d: %v", added.ID, deployKeyIDs(listed.DeployKeys))
		}
		updated := harness.Do[deploykeys.Output](s, actionAccessDeployKeyUpdate, withParams(byKey, map[string]any{"title": title + "-updated", "can_push": true}))
		if updated.ID != added.ID || updated.Title != title+"-updated" || !updated.CanPush {
			e.T.Errorf("deploy key update answered %+v, want key %d renamed and allowed to push", updated, added.ID)
		}

		harness.DoVoid(s, actionAccessDeployKeyDelete, byKey)
		refused := harness.Refused(s, actionAccessDeployKeyGet, byKey, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
