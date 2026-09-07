//go:build e2e && !enterprise

// misc_meta_ce_test.go tests miscellaneous MCP tools against a live GitLab instance.
// Covers feature flags, feature flag user lists, branch rules (GraphQL), CI/CD catalog (GraphQL),
// deployments, and user SSH/GPG key listing for both individual and meta-tool modes.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/ffuserlists"
)

// TestMeta_FeatureFlags exercises feature flag listing via the
// gitlab_feature_flags meta-tool. Feature flags may require a
// Premium/Ultimate license; errors are fatal.
//
// The test creates a project fixture and lists the project's feature
// flags through feature_flag_list, logging the count. The assertion
// treats errors as fatal because the meta-tool requires the feature
// flag API to be available in the running GitLab edition.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_FeatureFlags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/FeatureFlag/List", func(t *testing.T) {
		out, err := callToolOn[featureflags.ListOutput](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
			"action": "feature_flag_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "feature flag list")
		t.Logf("Feature flag list: %d flags", len(out.FeatureFlags))
	})
}

// TestMeta_BranchRules exercises branch rule listing via the gitlab_branch
// meta-tool (GraphQL backend).
//
// The test creates a project fixture and lists the project's branch
// rules through rule_list. The list may be empty when no branch rules
// have been defined; the assertion checks the call succeeds and logs
// the count for visibility.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_BranchRules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	// Protect a wildcard branch so the project owns a rule the list has to
	// name. The list is served by a raw GraphQL document, and a document
	// GitLab refuses comes back as an empty rule set rather than an error, so
	// counting rules cannot tell the two apart and naming one can.
	const ruleBranch = "e2e-rule-*"
	protected, protectErr := callToolOn[branches.ProtectedOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
		"action": "protect",
		"params": map[string]any{
			"project_id":         proj.pidStr(),
			"branch_name":        ruleBranch,
			"push_access_level":  40,
			"merge_access_level": 30,
		},
	})
	requireNoError(t, protectErr, "protect branch for branch rule fixture")
	requireTruef(t, protected.Name == ruleBranch, "protected branch = %q, want %q", protected.Name, ruleBranch)

	t.Run("Meta/BranchRule/List", func(t *testing.T) {
		out, err := callToolOn[branchrules.ListOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "rule_list",
			"params": map[string]any{
				"project_path": proj.Path,
			},
		})
		requireNoError(t, err, "branch rule list")
		requireTruef(t, slices.ContainsFunc(out.Rules, func(r branchrules.BranchRuleItem) bool {
			return r.Name == ruleBranch
		}), "branch rules do not name the protected %q: %+v", ruleBranch, out.Rules)
		t.Logf("Project %s has %d branch rule(s)", proj.Path, len(out.Rules))
	})
}

// TestMeta_CICatalog exercises CI/CD catalog listing via the
// gitlab_ci_catalog meta-tool.
//
// The test calls list without a project fixture to enumerate instance-wide
// CI/CD catalog resources. The list may be empty when no catalog resources
// have been published; the assertion checks the call succeeds and logs the
// count for visibility.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_CICatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/CICatalog/List", func(t *testing.T) {
		out, err := callToolOn[cicatalog.ListOutput](ctx, sess.meta, "gitlab_ci_catalog", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "ci catalog list")
		t.Logf("Found %d CI/CD catalog resource(s)", len(out.Resources))
	})
}

// TestMeta_Deployments exercises deployment listing via the
// gitlab_environment meta-tool.
//
// The test creates a project fixture and lists its deployments through
// deployment_list. The list may be empty when no CI pipeline has been
// triggered; the assertion checks the call succeeds and logs the count
// for visibility.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Deployments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Deployment/List", func(t *testing.T) {
		out, err := callToolOn[deployments.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "deployment list")
		t.Logf("Deployments: %d (may be empty without CI pipeline)", len(out.Deployments))
	})
}

// TestMeta_UserKeys exercises SSH and GPG key listing via the
// gitlab_user meta-tool.
//
// The test calls ssh_keys and gpg_keys on the authenticated user
// without a project fixture, asserting both calls succeed. The list
// outputs are discarded because key listings can be long; the test's
// purpose is to verify the meta-tool route works against the live
// GitLab instance.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_UserKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/User/SSHKeys", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "ssh_keys",
			"params": map[string]any{},
		})
		requireNoError(t, err, "user ssh_keys")
		t.Log("SSH keys OK")
	})

	t.Run("Meta/User/GPGKeys", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "gpg_keys",
			"params": map[string]any{},
		})
		requireNoError(t, err, "user gpg_keys")
		t.Log("GPG keys OK")
	})
}

// TestIndividual_BranchRules exercises the gitlab_list_branch_rules
// individual tool (GraphQL backend).
//
// The test creates a project fixture and lists the project's branch
// rules. The list may be empty when no branch rules have been defined;
// the assertion checks the call succeeds and logs the count.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_BranchRules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProject(ctx, t, sess.individual)

	// Same reasoning as TestMeta_BranchRules: give the project a rule of its
	// own so an empty answer cannot pass for a correct one.
	const ruleBranch = "e2e-rule-*"
	protected, protectErr := callToolOn[branches.ProtectedOutput](ctx, sess.individual, "gitlab_branch_protect", branches.ProtectInput{
		ProjectID:        proj.pidOf(),
		BranchName:       ruleBranch,
		PushAccessLevel:  40,
		MergeAccessLevel: 30,
	})
	requireNoError(t, protectErr, "protect branch for branch rule fixture")
	requireTruef(t, protected.Name == ruleBranch, "protected branch = %q, want %q", protected.Name, ruleBranch)

	t.Run("ListBranchRules", func(t *testing.T) {
		out, err := callToolOn[branchrules.ListOutput](ctx, sess.individual, "gitlab_list_branch_rules", branchrules.ListInput{
			ProjectPath: proj.Path,
		})
		requireNoError(t, err, "list branch rules")
		requireTruef(t, slices.ContainsFunc(out.Rules, func(r branchrules.BranchRuleItem) bool {
			return r.Name == ruleBranch
		}), "branch rules do not name the protected %q: %+v", ruleBranch, out.Rules)
		t.Logf("Project %s has %d branch rule(s)", proj.Path, len(out.Rules))
	})
}

// TestIndividual_CICatalog exercises CI/CD catalog individual tools (GraphQL).
func TestIndividual_CICatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("ListCatalogResources", func(t *testing.T) {
		out, err := callToolOn[cicatalog.ListOutput](ctx, sess.individual, "gitlab_list_catalog_resources", cicatalog.ListInput{})
		requireNoError(t, err, "list catalog resources")
		t.Logf("Found %d CI/CD catalog resource(s)", len(out.Resources))
	})
}

// TestMeta_FeatureFlagUserLists exercises feature flag user list CRUD via the
// gitlab_feature_flags meta-tool. User lists are project-scoped, not flag-scoped;
// they don't require a feature flag to exist first.
func TestMeta_FeatureFlagUserLists(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	ctx := context.Background()

	proj := createProjectMeta(ctx, t, sess.meta)

	var listIID int64
	t.Run("Meta/FFUserList/Create", func(t *testing.T) {
		out, err := callToolOn[ffuserlists.Output](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
			"action": "ff_user_list_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e test user list",
				"user_xids":  "user:1",
			},
		})
		requireNoError(t, err, "ff_user_list_create")
		listIID = out.IID
		t.Logf("Created user list: IID=%d name=%q", out.IID, out.Name)
	})

	t.Run("Meta/FFUserList/List", func(t *testing.T) {
		// Find our IID in the list with retries because newly created
		// feature-flag user lists are not always visible to the list
		// endpoint immediately after creation.
		var found bool
		var lastList ffuserlists.ListOutput
		_, listErr := retryWithBackoff(ctx, t, "ff_user_list_list find created", 5, func(int) (struct{}, bool, string, error) {
			out, err := callToolOn[ffuserlists.ListOutput](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
				"action": "ff_user_list_list",
				"params": map[string]any{
					"project_id": proj.pidStr(),
				},
			})
			if err != nil {
				return struct{}{}, true, "transient list error", err
			}
			lastList = out
			for _, ul := range out.UserLists {
				if ul.IID == listIID {
					found = true
					return struct{}{}, false, "", nil
				}
			}
			return struct{}{}, true, "newly created IID not yet visible in list", nil
		})
		requireNoError(t, listErr, "ff_user_list_list")
		requireTruef(t, found, "newly created user list IID=%d not visible in list after retries", listIID)
		t.Logf("User lists for project %s: %d (IID=%d present)", proj.Path, len(lastList.UserLists), listIID)
	})

	t.Run("Meta/FFUserList/Get", func(t *testing.T) {
		requireTruef(t, listIID > 0, "listIID not set (Create must run first)")
		out, err := callToolOn[ffuserlists.Output](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
			"action": "ff_user_list_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"user_list_iid": listIID,
			},
		})
		requireNoError(t, err, "ff_user_list_get")
		requireTruef(t, out.IID == listIID, "ff_user_list_get answered for IID %d, want %d", out.IID, listIID)
		requireTruef(t, out.Name == "e2e test user list", "user list name = %q, want the name the create sent", out.Name)
		t.Logf("Got user list: IID=%d name=%q", out.IID, out.Name)
	})

	t.Run("Meta/FFUserList/Update", func(t *testing.T) {
		requireTruef(t, listIID > 0, "listIID not set (Create must run first)")
		out, err := callToolOn[ffuserlists.Output](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
			"action": "ff_user_list_update",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"user_list_iid": listIID,
				"name":          "e2e updated user list",
			},
		})
		requireNoError(t, err, "ff_user_list_update")
		requireTruef(t, out.Name == "e2e updated user list", "name mismatch: got %q want %q", out.Name, "e2e updated user list")
		t.Logf("Updated user list: IID=%d", out.IID)
	})

	t.Run("Meta/FFUserList/Delete", func(t *testing.T) {
		requireTruef(t, listIID > 0, "listIID not set (Create must run first)")
		_, err := callToolOn[ffuserlists.Output](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
			"action": "ff_user_list_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"user_list_iid": listIID,
			},
		})
		requireNoError(t, err, "ff_user_list_delete")

		// Verify deletion: list should no longer contain our IID.
		// Deletion may not be reflected immediately, so retry briefly.
		var absent bool
		_, delErr := retryWithBackoff(ctx, t, "ff_user_list_list verify deleted", 5, func(int) (struct{}, bool, string, error) {
			listOut, lerr := callToolOn[ffuserlists.ListOutput](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
				"action": "ff_user_list_list",
				"params": map[string]any{
					"project_id": proj.pidStr(),
				},
			})
			if lerr != nil {
				return struct{}{}, true, "transient list error", lerr
			}
			for _, ul := range listOut.UserLists {
				if ul.IID == listIID {
					return struct{}{}, true, "deleted IID still present in list", nil
				}
			}
			absent = true
			return struct{}{}, false, "", nil
		})
		requireNoError(t, delErr, "ff_user_list_list post-delete")
		requireTruef(t, absent, "deleted user list IID=%d still present in list after retries", listIID)
		t.Logf("Deleted user list: IID=%d (verified absent from list)", listIID)
	})
}
