//go:build e2e

// groupprotectedenvs_test.go covers a group's protected environments, which
// are named by deployment tier rather than by environment: a name outside
// the five tiers is refused with the list of them, and the production tier
// is protected, listed, read, given a wider deploy access level and
// unprotected.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupprotectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The tier the fixture rule protects and the access it widens to.
const (
	productionTier       = "production"
	developerAccessLevel = int64(30)
)

// protectedEnvNames lists the names of a protected environment listing.
func protectedEnvNames(environments []groupprotectedenvs.Output) []string {
	names := make([]string, 0, len(environments))
	for _, environment := range environments {
		names = append(names, environment.Name)
	}
	return names
}

// containsEnvName reports whether a listing holds a protected environment
// by name.
func containsEnvName(environments []groupprotectedenvs.Output, name string) bool {
	for _, environment := range environments {
		if environment.Name == name {
			return true
		}
	}
	return false
}

// hasDeployAccessLevel reports whether a rule grants the given access level
// on any of its deploy access levels.
func hasDeployAccessLevel(rule groupprotectedenvs.Output, level int64) bool {
	for _, access := range rule.DeployAccessLevels {
		if int64(access.AccessLevel) == level {
			return true
		}
	}
	return false
}

// The five deployment tiers a group protected environment may be named
// after, which every refusal of another name spells out.
var deploymentTiers = []string{productionTier, "staging", "testing", "development", "other"}

// TestGroupProtectedEnvironments_Lifecycle_RefusesANameOutsideTheTiers
// asks every surface to protect a name that is no deployment tier, which is
// refused with the tiers spelled out, then protects the production tier in
// a group of the surface's own, lists it, reads it, widens its deploy
// access to Developer and unprotects it.
//
// The refusal of the name comes from two places, and the test holds each
// surface to its own: the individual tool declares the tiers as an enum, so
// the SDK refuses the argument before the handler runs, while the two
// dispatchers hand the name to GitLab and wrap its refusal with the hint
// that lists the tiers.
//
// Replaces: TestMeta_GroupProtectedEnvironmentsEE, TestEE_MetaGroupEnterpriseOperations
func TestGroupProtectedEnvironments_Lifecycle_RefusesANameOutsideTheTiers(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("gpe"))
		params := map[string]any{"group_id": group.IDParam()}
		maintainers := []map[string]any{{"access_level": maintainerAccessLevel}}

		outsideTheTiers := withParams(params, map[string]any{"name": e.Name("tier"), "deploy_access_levels": maintainers})
		var refused string
		if surface == harness.SurfaceIndividual {
			refused = harness.Refused(s, actionGroupProtectedEnvProtect, outsideTheTiers, harness.FailureInvalidParams)
		} else {
			refused = harness.ExpectToolError(s, actionGroupProtectedEnvProtect, outsideTheTiers, "valid group protected environment tiers")
		}
		assertMentions(e, "the refusal of a name outside the tiers", refused, deploymentTiers...)

		protected := harness.Do[groupprotectedenvs.Output](s, actionGroupProtectedEnvProtect, withParams(params, map[string]any{
			"name": productionTier, "deploy_access_levels": maintainers,
		}))
		if protected.Name != productionTier || len(protected.DeployAccessLevels) == 0 || protected.DeployAccessLevels[0].ID == 0 {
			e.T.Fatalf("protected_env_protect answered %+v, want the %s tier with a deploy access level carrying an ID", protected, productionTier)
		}
		tier := withParams(params, map[string]any{"environment": productionTier})

		listed := harness.Do[groupprotectedenvs.ListOutput](s, actionGroupProtectedEnvList, params)
		if !containsEnvName(listed.Environments, productionTier) {
			e.T.Errorf("the group's protected environments do not hold %s: %v", productionTier, protectedEnvNames(listed.Environments))
		}
		got := harness.Do[groupprotectedenvs.Output](s, actionGroupProtectedEnvGet, tier)
		if got.Name != productionTier {
			e.T.Errorf("protected_env_get answered %q, want %s", got.Name, productionTier)
		}

		updated := harness.Do[groupprotectedenvs.Output](s, actionGroupProtectedEnvUpdate, withParams(tier, map[string]any{
			"deploy_access_levels": []map[string]any{{"id": protected.DeployAccessLevels[0].ID, "access_level": developerAccessLevel}},
		}))
		if updated.Name != productionTier || !hasDeployAccessLevel(updated, developerAccessLevel) {
			e.T.Errorf("protected_env_update answered %+v, want %s with a deploy access level of %d", updated, productionTier, developerAccessLevel)
		}

		harness.DoVoid(s, actionGroupProtectedEnvUnprotect, tier)
		after := harness.Do[groupprotectedenvs.ListOutput](s, actionGroupProtectedEnvList, params)
		if containsEnvName(after.Environments, productionTier) {
			e.T.Errorf("the group's protected environments still hold %s after its unprotect", productionTier)
		}
	})
}

// TestGroupProtectedEnvironments_EntriesGitLabRefuses_AreRefusedBeforeGitLab
// asks every surface to protect the production tier of a group of its own
// with a deploy access level naming nothing, the call find's example used to
// build (issue 1271), with one naming both a role and a user, and with an
// approval rule naming nothing. GitLab refuses each of the three, and the test
// checks that each is refused here, before it is sent, and that the tier is
// not protected afterwards.
//
// The individual tool's item schemas refuse the arguments before the handler
// runs, and the meta and the dynamic dispatchers, which read no item schema,
// hand the call to the handler, which refuses it with words that name the
// entry and the rule. None of them is in the hint a GitLab refusal is wrapped
// with, so a call that reached GitLab would not pass for one refused here.
func TestGroupProtectedEnvironments_EntriesGitLabRefuses_AreRefusedBeforeGitLab(t *testing.T) {
	e := harness.New(t)
	user := e.Runtime().UserID

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("gperule"))
		target := map[string]any{"group_id": group.IDParam(), "name": productionTier}
		maintainers := []map[string]any{{"access_level": maintainerAccessLevel}}

		calls := []struct {
			params  map[string]any
			refusal string
		}{
			{params: withParams(target, map[string]any{"deploy_access_levels": []map[string]any{{}}}), refusal: "deploy_access_levels[0] names none of access_level, user_id or group_id"},
			{params: withParams(target, map[string]any{"deploy_access_levels": []map[string]any{{"access_level": maintainerAccessLevel, "user_id": user}}}), refusal: "deploy_access_levels[0] names access_level and user_id"},
			{params: withParams(target, map[string]any{"deploy_access_levels": maintainers, "approval_rules": []map[string]any{{"required_approvals": 1}}}), refusal: "approval_rules[0] names none of access_level, user_id or group_id"},
		}
		for _, call := range calls {
			if surface == harness.SurfaceIndividual {
				harness.Refused(s, actionGroupProtectedEnvProtect, call.params, harness.FailureInvalidParams)
				continue
			}
			harness.ExpectToolError(s, actionGroupProtectedEnvProtect, call.params, call.refusal)
		}

		listed := harness.Do[groupprotectedenvs.ListOutput](s, actionGroupProtectedEnvList, map[string]any{"group_id": group.IDParam()})
		if containsEnvName(listed.Environments, productionTier) {
			e.T.Errorf("the refused calls protected %s: %v", productionTier, protectedEnvNames(listed.Environments))
		}
	})
}
