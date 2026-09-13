//go:build e2e

// branches_test.go covers a branch through the server after its creation,
// which mergerequests_test.go already drives: reading and listing it,
// protecting it and changing the protection in place, the protection's
// removal, which GitLab applies through a background job, the delete of
// every merged branch, and the branch's own delete.

package common

import (
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The access levels the protection is created with: Maintainer to push,
// Developer to merge.
const (
	branchPushAccessLevel  = 40
	branchMergeAccessLevel = 30
)

// A removed protection stays listed while GitLab's background job applies
// the removal, so the listing is polled at this cadence.
const (
	unprotectPollInterval = 2 * time.Second
	unprotectPollTimeout  = 60 * time.Second
)

// branchNames returns the names of a branch listing.
func branchNames(listed []branches.Output) []string {
	names := make([]string, 0, len(listed))
	for _, branch := range listed {
		names = append(names, branch.Name)
	}
	return names
}

// protectedBranchNames returns the names of a protected branch listing.
func protectedBranchNames(listed []branches.ProtectedOutput) []string {
	names := make([]string, 0, len(listed))
	for _, branch := range listed {
		names = append(names, branch.Name)
	}
	return names
}

// branchProtectionLifecycle protects a branch with two access levels, reads
// the protection and raises allow_force_push on it, finds it among the
// protected branches, removes the protection and waits for the listing to
// agree.
func branchProtectionLifecycle(e *harness.Env, s *harness.Session, params, branch map[string]any, name string) {
	e.T.Helper()

	protected := harness.Do[branches.ProtectedOutput](s, actionBranchProtect, withParams(branch, map[string]any{
		"push_access_level": branchPushAccessLevel, "merge_access_level": branchMergeAccessLevel,
	}))
	if protected.Name != name || len(protected.PushAccessLevels) == 0 || len(protected.MergeAccessLevels) == 0 {
		e.T.Fatalf("branch protect answered %+v, want %s with push and merge access levels", protected, name)
	}
	if protected.PushAccessLevels[0].AccessLevel != branchPushAccessLevel || protected.MergeAccessLevels[0].AccessLevel != branchMergeAccessLevel {
		e.T.Errorf("the protection of %s carries push %d and merge %d, want %d and %d", name,
			protected.PushAccessLevels[0].AccessLevel, protected.MergeAccessLevels[0].AccessLevel, branchPushAccessLevel, branchMergeAccessLevel)
	}

	protectedGot := harness.Do[branches.ProtectedOutput](s, actionBranchGetProtected, branch)
	if protectedGot.Name != name || protectedGot.ID != protected.ID || protectedGot.AllowForcePush {
		e.T.Errorf("protected branch get answered %+v, want protection %d of %s without force push", protectedGot, protected.ID, name)
	}
	updated := harness.Do[branches.ProtectedOutput](s, actionBranchUpdateProtected, withParams(branch, map[string]any{"allow_force_push": true}))
	if updated.Name != name || !updated.AllowForcePush {
		e.T.Errorf("protected branch update answered %+v, want %s with force push allowed", updated, name)
	}
	protectedList := harness.Do[branches.ProtectedListOutput](s, actionBranchListProtected, params)
	if !slices.Contains(protectedBranchNames(protectedList.Branches), name) {
		e.T.Errorf("the protected branch listing does not hold %s: %v", name, protectedBranchNames(protectedList.Branches))
	}

	harness.DoVoid(s, actionBranchUnprotect, branch)
	unprotected := harness.Eventually(s, actionBranchListProtected, params, unprotectPollInterval, unprotectPollTimeout,
		func(out branches.ProtectedListOutput) bool {
			return !slices.Contains(protectedBranchNames(out.Branches), name)
		})
	e.T.Logf("%d protected branch(es) remain after the unprotect of %s", len(unprotected.Branches), name)
}

// TestBranch_Lifecycle_ProtectUpdateUnprotectDelete creates a branch per
// surface on a shared project, reads and lists it, gives it a commit of
// its own, protects it with two access levels, reads the protection and
// raises allow_force_push on it, finds it among the protected branches,
// removes the protection and waits for the listing to agree, deletes the
// merged branches, which leaves it alone, then deletes it by name and
// asserts the read afterwards is refused as not found.
//
// Replaces: TestIndividual_Branches, TestMeta_Branches
func TestBranch_Lifecycle_ProtectUpdateUnprotectDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("branches"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		name := "feature/" + e.Name("br")
		branch := withParams(params, map[string]any{"branch_name": name})

		created := harness.Do[branches.Output](s, actionBranchCreate, withParams(branch, map[string]any{"ref": project.DefaultBranch}))
		if created.Name != name || created.Commit == nil || created.Commit.ID == "" {
			e.T.Fatalf("branch create answered %+v, want %s with a commit", created, name)
		}

		got := harness.Do[branches.Output](s, actionBranchGet, branch)
		if got.Name != name || got.Commit == nil || got.Commit.ID != created.Commit.ID || got.Protected {
			e.T.Errorf("branch get answered %+v, want the unprotected %s at %s", got, name, fixture.ShortSHA(created.Commit.ID))
		}
		listed := harness.Do[branches.ListOutput](s, actionBranchList, params)
		if !slices.Contains(branchNames(listed.Branches), name) || !slices.Contains(branchNames(listed.Branches), project.DefaultBranch) {
			e.T.Errorf("the branch listing does not hold %s beside %s: %v", name, project.DefaultBranch, branchNames(listed.Branches))
		}

		// A branch still at the default branch's commit counts as merged, and
		// the sweep of merged branches below would take it; one commit of its
		// own keeps it out of that sweep.
		diverged := fixture.CommitFile(e, project, name, "diverge-"+string(surface)+".txt", "on the branch", "feat: diverge "+name)

		branchProtectionLifecycle(e, s, params, branch, name)

		// The branch is not merged, so the sweep of merged branches, which
		// GitLab runs in the background, leaves it to be deleted by name.
		harness.DoVoid(s, actionBranchDeleteMerged, params)
		stillThere := harness.Do[branches.Output](s, actionBranchGet, branch)
		if stillThere.Name != name || stillThere.Commit == nil || stillThere.Commit.ID != diverged.SHA {
			e.T.Errorf("the unmerged %s reads as %+v after the delete of merged branches, want it still at %s", name, stillThere, diverged.ShortID)
		}
		harness.DoVoid(s, actionBranchDelete, branch)
		refused := harness.Refused(s, actionBranchGet, branch, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
