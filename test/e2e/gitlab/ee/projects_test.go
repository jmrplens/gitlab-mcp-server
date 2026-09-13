//go:build e2e

// projects_test.go covers the licensed actions of the project group that
// stand on nothing but a project: target branch rules, pull mirroring, the
// project's approval configuration and rules, and its security settings.

package ee

import (
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestTargetBranchRules_Lifecycle_CreatesListsAndDeletes adds a rule
// mapping release branches to the default branch, finds it in the listing
// and deletes it, once per surface on a shared project.
//
// The create takes the numeric project ID and the list takes the full
// path: the two GraphQL operations behind them disagree about how a
// project is named, and the test states both spellings on purpose.
//
// Replaces: TestMeta_TargetBranchRules
func TestTargetBranchRules_Lifecycle_CreatesListsAndDeletes(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("tbr"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		pattern := "release/" + string(surface) + "/*"

		created := harness.Do[projects.TargetBranchRuleOutput](s, actionTargetBranchRuleCreate, map[string]any{
			"project_id": project.IDParam(), "name": pattern, "target_branch": project.DefaultBranch,
		})
		if created.ID == 0 || created.TargetBranch != project.DefaultBranch {
			e.T.Fatalf("create answered %+v, want a rule targeting %q with an ID", created, project.DefaultBranch)
		}

		listed := harness.Do[projects.ListTargetBranchRulesOutput](s, actionTargetBranchRuleList, map[string]any{"project_id": project.Path})
		found := false
		for _, rule := range listed.TargetBranchRules {
			if rule.ID == created.ID {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the listing does not hold the created rule %d: %+v", created.ID, listed.TargetBranchRules)
		}

		harness.DoVoid(s, actionTargetBranchRuleDelete, map[string]any{"rule_id": created.ID})
		remaining := harness.Do[projects.ListTargetBranchRulesOutput](s, actionTargetBranchRuleList, map[string]any{"project_id": project.Path})
		for _, rule := range remaining.TargetBranchRules {
			if rule.ID == created.ID {
				e.T.Errorf("rule %d is still listed after its delete", created.ID)
			}
		}
	})
}

// TestPullMirror_Lifecycle_ConfiguresStartsAndDisables reads the mirror of
// a project that has none, configures one from a public sibling project,
// reads it back, starts an update and disables it, once per surface. The
// source is reached at the address GitLab has for itself inside the Docker
// network, since the published one is not routable from the container.
//
// Replaces: TestMeta_ProjectMirroring
func TestPullMirror_Lifecycle_ConfiguresStartsAndDisables(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mirror-src"), fixture.WithVisibility(gl.PublicVisibility))
	}, func(e *harness.Env, surface harness.Surface, upstream fixture.Project) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("mirror"))
		source := fixture.RepositoryURL(e, upstream)

		refused := harness.ExpectToolError(s, actionPullMirrorGet, map[string]any{"project_id": project.IDParam()}, "not mirrored")
		assertMentions(e, "the read of a project that is not mirrored", refused, "pull_mirror_configure", "pull_mirror_get")

		configured := harness.Do[projects.PullMirrorOutput](s, actionPullMirrorConfigure, map[string]any{
			"project_id": project.IDParam(), "enabled": true, "url": source, "mirror_trigger_builds": false,
			"only_mirror_protected_branches": false, "mirror_overwrites_diverged_branches": true,
		})
		if !configured.Enabled || !strings.Contains(configured.URL, upstream.Path) {
			e.T.Errorf("configure answered %+v, want an enabled mirror of %s", configured, upstream.Path)
		}

		got := harness.Do[projects.PullMirrorOutput](s, actionPullMirrorGet, map[string]any{"project_id": project.IDParam()})
		if !got.Enabled || !strings.Contains(got.URL, upstream.Path) {
			e.T.Errorf("get answered %+v, want the enabled mirror of %s", got, upstream.Path)
		}

		harness.DoVoid(s, actionStartMirroring, map[string]any{"project_id": project.IDParam()})

		disabled := harness.Do[projects.PullMirrorOutput](s, actionPullMirrorConfigure, map[string]any{"project_id": project.IDParam(), "enabled": false})
		if disabled.Enabled {
			e.T.Errorf("configure answered %+v after disabling, want enabled=false", disabled)
		}
	})
}

// TestProjectApprovals_Lifecycle_ConfiguresAndManagesRules reads and
// changes the project's approval configuration, then walks one approval
// rule through create, list, get, update and delete, once per surface on a
// shared project. These are the calls the old suite kept behind an
// enterprise guard in a Community file, where they never ran; the
// configuration change is also the one the two merge request approval
// tests of that file made before approving, which their port makes here.
//
// Replaces: TestMeta_ProjectApprovals
func TestProjectApprovals_Lifecycle_ConfiguresAndManagesRules(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("approvals"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		id := project.IDParam()

		config := harness.Do[projects.ApprovalConfigOutput](s, actionProjectApprovalConfigGet, map[string]any{"project_id": id})
		e.T.Logf("approval configuration before the change: reset_on_push=%t author_approval=%t", config.ResetApprovalsOnPush, config.MergeRequestsAuthorApproval)

		changed := harness.Do[projects.ApprovalConfigOutput](s, actionProjectApprovalConfigChange, map[string]any{
			"project_id": id, "reset_approvals_on_push": true, "disable_overriding_approvers_per_merge_request": false,
			"merge_requests_author_approval": true, "merge_requests_disable_committers_approval": false,
		})
		if !changed.ResetApprovalsOnPush || changed.DisableOverridingApproversPerMergeRequest || !changed.MergeRequestsAuthorApproval {
			e.T.Errorf("approval_config_change answered %+v, want reset_on_push=true, overriding allowed and author approval allowed", changed)
		}

		name := e.Name("rule")
		created := harness.Do[projects.ApprovalRuleOutput](s, actionProjectApprovalRuleCreate, map[string]any{
			"project_id": id, "name": name, "approvals_required": 1,
		})
		if created.ID == 0 {
			e.T.Fatalf("approval_rule_create answered %+v, want a rule with an ID", created)
		}

		listed := harness.Do[projects.ListApprovalRulesOutput](s, actionProjectApprovalRuleList, map[string]any{"project_id": id})
		if !containsID(projectApprovalRuleIDs(listed.Rules), created.ID) {
			e.T.Errorf("the listing does not hold the created rule %d: %v", created.ID, projectApprovalRuleIDs(listed.Rules))
		}

		got := harness.Do[projects.ApprovalRuleOutput](s, actionProjectApprovalRuleGet, map[string]any{"project_id": id, "rule_id": created.ID})
		if got.ID != created.ID || got.Name != name {
			e.T.Errorf("approval_rule_get answered %+v, want rule %d named %q", got, created.ID, name)
		}

		updated := harness.Do[projects.ApprovalRuleOutput](s, actionProjectApprovalRuleUpdate, map[string]any{
			"project_id": id, "rule_id": created.ID, "name": name + "-updated", "approvals_required": 2,
		})
		if updated.ID != created.ID || updated.Name != name+"-updated" || updated.ApprovalsRequired != 2 {
			e.T.Errorf("approval_rule_update answered %+v, want rule %d renamed to %q requiring 2", updated, created.ID, name+"-updated")
		}

		harness.DoVoid(s, actionProjectApprovalRuleDelete, map[string]any{"project_id": id, "rule_id": created.ID})
		remaining := harness.Do[projects.ListApprovalRulesOutput](s, actionProjectApprovalRuleList, map[string]any{"project_id": id})
		if containsID(projectApprovalRuleIDs(remaining.Rules), created.ID) {
			e.T.Errorf("rule %d is still listed after its delete", created.ID)
		}
	})
}

// projectApprovalRuleIDs lists the IDs of a rule listing.
func projectApprovalRuleIDs(rules []projects.ApprovalRuleOutput) []int64 {
	ids := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

// TestProjectSecuritySettings_Read_ThenSecretPushProtectionOn reads a
// project's security settings and turns secret push protection on, once
// per surface on a project of the surface's own so that each starts from
// the default.
//
// Replaces: TestMeta_ProjectSecuritySettings
func TestProjectSecuritySettings_Read_ThenSecretPushProtectionOn(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("secset"))

		settings := harness.Do[securitysettings.ProjectOutput](s, actionProjectSecuritySettingsGet, map[string]any{"project_id": project.IDParam()})
		if settings.ProjectID != project.ID {
			e.T.Errorf("security_settings_get answered project %d, want %d", settings.ProjectID, project.ID)
		}

		updated := harness.Do[securitysettings.ProjectOutput](s, actionProjectSecuritySettingsUpdate, map[string]any{
			"project_id": project.IDParam(), "secret_push_protection_enabled": true,
		})
		if updated.ProjectID != project.ID || !updated.SecretPushProtectionEnabled {
			e.T.Errorf("security_settings_update answered %+v, want secret push protection on for project %d", updated, project.ID)
		}
	})
}
