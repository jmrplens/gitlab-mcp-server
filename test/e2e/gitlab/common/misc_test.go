//go:build e2e

// misc_test.go ports the read-heavy grab bag the old suite spread across its
// misc files: the project feature flag listing, the feature flag user list
// lifecycle, branch rules, the CI/CD catalog listing, the project deployment
// listing and the authenticated user's SSH and GPG key listings.
//
// Each is placed here by the family of the action it drives. The cheap reads
// run on all three surfaces; the feature flag user list, which is a stateful
// project-scoped lifecycle, runs on the meta surface the old suite used.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/ffuserlists"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestFeatureFlags_ProjectList lists a project's feature flags on every
// surface.
//
// Replaces: TestMeta_FeatureFlags
func TestFeatureFlags_ProjectList(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("ffflags"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		out := harness.Do[featureflags.ListOutput](s, actionFeatureFlagList, map[string]any{"project_id": project.IDParam()})
		e.T.Logf("project %s has %d feature flag(s)", project.Path, len(out.FeatureFlags))
	})
}

// TestFeatureFlags_UserLists_Lifecycle creates a feature flag user list, finds
// it in the listing, reads and renames it, then deletes it and checks it is
// gone.
//
// Replaces: TestMeta_FeatureFlagUserLists
func TestFeatureFlags_UserLists_Lifecycle(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)
	project := fixture.NewProject(e, fixture.WithNamePrefix("ffuserlist"))
	base := map[string]any{"project_id": project.IDParam()}

	created := harness.Do[ffuserlists.Output](s, actionFFUserListCreate, withParams(base, map[string]any{
		"name": "e2e test user list", "user_xids": "user:1",
	}))
	if created.IID == 0 {
		e.T.Fatalf("ff_user_list_create answered %+v, want a user list with an IID", created)
	}
	listIID := created.IID
	item := withParams(base, map[string]any{"user_list_iid": listIID})

	list := harness.Do[ffuserlists.ListOutput](s, actionFFUserListList, base)
	if !ffUserListPresent(list, listIID) {
		e.T.Errorf("ff_user_list_list does not hold the created list %d: %+v", listIID, list.UserLists)
	}

	got := harness.Do[ffuserlists.Output](s, actionFFUserListGet, item)
	if got.IID != listIID || got.Name != "e2e test user list" {
		e.T.Errorf("ff_user_list_get answered %+v, want list %d with the created name", got, listIID)
	}

	updated := harness.Do[ffuserlists.Output](s, actionFFUserListUpdate, withParams(item, map[string]any{"name": "e2e updated user list"}))
	if updated.Name != "e2e updated user list" {
		e.T.Errorf("ff_user_list_update answered name %q, want the updated name", updated.Name)
	}

	harness.DoVoid(s, actionFFUserListDelete, item)
	after := harness.Do[ffuserlists.ListOutput](s, actionFFUserListList, base)
	if ffUserListPresent(after, listIID) {
		e.T.Errorf("ff_user_list_list still holds list %d after its delete", listIID)
	}
}

// ffUserListPresent reports whether a listing holds a user list by IID.
func ffUserListPresent(out ffuserlists.ListOutput, iid int64) bool {
	for _, list := range out.UserLists {
		if list.IID == iid {
			return true
		}
	}
	return false
}

// TestBranchRules_List protects a wildcard branch and finds the rule GitLab
// derives for it in the branch rule listing, on every surface.
//
// Replaces: TestMeta_BranchRules, TestIndividual_BranchRules
func TestBranchRules_List(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("branchrules"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		// A branch rule list is served by a raw GraphQL document, and a
		// document GitLab refuses comes back as an empty rule set rather than
		// an error; protecting a wildcard branch gives the project a rule the
		// list has to name, so an empty answer cannot pass for a correct one.
		ruleBranch := "e2e-rule-" + string(surface) + "-*"
		protected := harness.Do[branches.ProtectedOutput](s, actionBranchProtect, map[string]any{
			"project_id": project.IDParam(), "branch_name": ruleBranch, "push_access_level": 40, "merge_access_level": 30,
		})
		if protected.Name != ruleBranch {
			e.T.Fatalf("branch protect answered %q, want %q", protected.Name, ruleBranch)
		}

		rules := harness.Do[branchrules.ListOutput](s, actionBranchRuleList, map[string]any{"project_path": project.Path})
		if !slices.ContainsFunc(rules.Rules, func(r branchrules.BranchRuleItem) bool { return r.Name == ruleBranch }) {
			e.T.Errorf("the branch rules do not name the protected %q: %+v", ruleBranch, rules.Rules)
		}
	})
}

// TestCICatalog_List enumerates the instance-wide CI/CD catalog resources on
// every surface.
//
// Replaces: TestMeta_CICatalog, TestIndividual_CICatalog
func TestCICatalog_List(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		out := harness.Do[cicatalog.ListOutput](s, actionCICatalogList, nil)
		e.T.Logf("%d CI/CD catalog resource(s)", len(out.Resources))
	})
}

// TestDeployments_List lists a project's deployments on every surface.
//
// Replaces: TestMeta_Deployments
func TestDeployments_List(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("deployments"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		out := harness.Do[deployments.ListOutput](s, actionEnvironmentDeploymentList, map[string]any{"project_id": project.IDParam()})
		e.T.Logf("project %s has %d deployment(s)", project.Path, len(out.Deployments))
	})
}

// TestUserKeys_List lists the authenticated user's SSH and GPG keys on every
// surface.
//
// Replaces: TestMeta_UserKeys
func TestUserKeys_List(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		ssh := harness.Do[users.SSHKeyListOutput](s, actionUserSSHKeys, nil)
		e.T.Logf("the authenticated user has %d SSH key(s)", len(ssh.Keys))
		gpg := harness.Do[usergpgkeys.ListOutput](s, actionUserGPGKeys, nil)
		e.T.Logf("the authenticated user has %d GPG key(s)", len(gpg.Keys))
	})
}
