//go:build e2e

// groupprotectedbranches_test.go covers a group's protected branch rules:
// protect a wildcard, find it in the listing, read it, allow force pushes
// on it, unprotect it and check the read is then refused.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupprotectedbranches"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// maintainerAccessLevel is the access every side of the fixture rule is
// given: Maintainer, which GitLab accepts for push, merge and unprotect.
const maintainerAccessLevel = int64(40)

// protectedBranchNames lists the names of a rule listing.
func protectedBranchNames(branches []groupprotectedbranches.Output) []string {
	names := make([]string, 0, len(branches))
	for _, branch := range branches {
		names = append(names, branch.Name)
	}
	return names
}

// containsBranchName reports whether a rule listing holds one by name.
func containsBranchName(branches []groupprotectedbranches.Output, name string) bool {
	for _, branch := range branches {
		if branch.Name == name {
			return true
		}
	}
	return false
}

// TestGroupProtectedBranches_Lifecycle_ProtectListGetUpdateUnprotect walks
// one wildcard rule per surface through its whole life in a shared group.
// The wildcards carry the surface's name, so the three rules never collide.
//
// Replaces: TestMeta_GroupProtectedBranchesEE, TestEE_MetaGroupEnterpriseOperations
func TestGroupProtectedBranches_Lifecycle_ProtectListGetUpdateUnprotect(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("gpb"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam()}
		wildcard := e.Name("release") + "/*"
		rule := withParams(params, map[string]any{"branch": wildcard})

		protected := harness.Do[groupprotectedbranches.Output](s, actionGroupProtectedBranchProtect, withParams(params, map[string]any{
			"name": wildcard, "push_access_level": maintainerAccessLevel, "merge_access_level": maintainerAccessLevel,
			"unprotect_access_level": maintainerAccessLevel, "allow_force_push": false,
		}))
		if protected.Name != wildcard || protected.AllowForcePush {
			e.T.Fatalf("protected_branch_protect answered %+v, want the rule %q with force pushes off", protected, wildcard)
		}

		listed := harness.Do[groupprotectedbranches.ListOutput](s, actionGroupProtectedBranchList, withParams(params, map[string]any{"search": wildcard}))
		if !containsBranchName(listed.Branches, wildcard) {
			e.T.Errorf("the group's protected branches do not hold %q: %v", wildcard, protectedBranchNames(listed.Branches))
		}
		got := harness.Do[groupprotectedbranches.Output](s, actionGroupProtectedBranchGet, rule)
		if got.Name != wildcard {
			e.T.Errorf("protected_branch_get answered %q, want %q", got.Name, wildcard)
		}

		updated := harness.Do[groupprotectedbranches.Output](s, actionGroupProtectedBranchUpdate, withParams(rule, map[string]any{"allow_force_push": true}))
		if updated.Name != wildcard || !updated.AllowForcePush {
			e.T.Errorf("protected_branch_update answered %+v, want %q with force pushes on", updated, wildcard)
		}

		harness.DoVoid(s, actionGroupProtectedBranchUnprotect, rule)
		refused := harness.Refused(s, actionGroupProtectedBranchGet, rule, harness.FailureNotFound)
		e.T.Logf("the read of the unprotected rule was refused: %s", firstLine(refused))
		after := harness.Do[groupprotectedbranches.ListOutput](s, actionGroupProtectedBranchList, params)
		if containsBranchName(after.Branches, wildcard) {
			e.T.Errorf("the group's protected branches still hold %q after its unprotect", wildcard)
		}
	})
}
