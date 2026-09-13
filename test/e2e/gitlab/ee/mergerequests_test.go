//go:build e2e

// mergerequests_test.go covers the licensed half of the merge request
// group: the approval settings of a project and of a group, a request's
// approval state and rules, the reset of its approvals, and the blocking
// dependencies between two requests.
//
// The approval reset is the one scenario that needs a second credential.
// GitLab allows that call to a bot user alone, one holding a project or a
// group token, and answers a person with 401; so the request is approved
// and reset from a private session started on a group token, and the run's
// own session then reads that no approval is left.

package ee

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrapprovalsettings"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The propagation waits: a bot minted a moment ago is not yet a member
// every endpoint recognizes, and the old suite retried on that for the
// same reason.
const (
	membershipInterval = 2 * time.Second
	membershipWait     = 60 * time.Second
)

// approvalFixture is a project with an open, mergeable merge request.
type approvalFixture struct {
	project fixture.Project
	mr      fixture.MergeRequest
}

// newApprovalFixture creates the project, a branch with one commit, and the
// merge request from it. The prefix keeps the projects of the tests here
// apart in a listing.
func newApprovalFixture(e *harness.Env, prefix string, opts ...fixture.ProjectOption) approvalFixture {
	project := fixture.NewProject(e, append([]fixture.ProjectOption{fixture.WithNamePrefix(prefix)}, opts...)...)
	branch := fixture.NewBranch(e, project, e.Name(prefix))
	fixture.CommitFile(e, project, branch.Name, prefix+".txt", prefix+" fixture\n", "add the "+prefix+" fixture")
	mr := fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, prefix+" fixture")
	return approvalFixture{project: project, mr: mr}
}

// TestMRApprovalSettings_ProjectAndGroup_ReadAndUpdate reads the approval
// settings of a project and of a group on every surface, then turns author
// approval and approval retention on for both and reads the change off the
// answer.
//
// Replaces: TestMeta_MRApprovalSettings, TestMeta_GroupMRApprovalSettings
func TestMRApprovalSettings_ProjectAndGroup_ReadAndUpdate(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("apprset"))
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("apprset"))

		before := harness.Do[mrapprovalsettings.Output](s, actionApprovalSettingsProjectGet, map[string]any{"project_id": project.IDParam()})
		e.T.Logf("project approval settings before: author=%t committer=%t", before.AllowAuthorApproval.Value, before.AllowCommitterApproval.Value)
		updated := harness.Do[mrapprovalsettings.Output](s, actionApprovalSettingsProjectUpdate, map[string]any{
			"project_id": project.IDParam(), "allow_author_approval": true, "retain_approvals_on_push": true,
		})
		if !updated.AllowAuthorApproval.Value || !updated.RetainApprovalsOnPush.Value {
			e.T.Errorf("the project update answered author=%t retain=%t, want both true", updated.AllowAuthorApproval.Value, updated.RetainApprovalsOnPush.Value)
		}

		groupBefore := harness.Do[mrapprovalsettings.Output](s, actionApprovalSettingsGroupGet, map[string]any{"group_id": group.IDParam()})
		e.T.Logf("group approval settings before: author=%t retain=%t", groupBefore.AllowAuthorApproval.Value, groupBefore.RetainApprovalsOnPush.Value)
		groupUpdated := harness.Do[mrapprovalsettings.Output](s, actionApprovalSettingsGroupUpdate, map[string]any{
			"group_id": group.IDParam(), "allow_author_approval": true, "retain_approvals_on_push": true,
		})
		if !groupUpdated.AllowAuthorApproval.Value || !groupUpdated.RetainApprovalsOnPush.Value {
			e.T.Errorf("the group update answered author=%t retain=%t, want both true", groupUpdated.AllowAuthorApproval.Value, groupUpdated.RetainApprovalsOnPush.Value)
		}
	})
}

// mrApprovalRuleIDs lists the IDs of a request's rules.
func mrApprovalRuleIDs(rules []mrapprovals.RuleOutput) []int64 {
	ids := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

// TestMRApprovalRules_Lifecycle_ReadsStateAndManagesARule reads a fresh
// request's approval state, rules and configuration, then creates, updates
// and deletes a rule of its own, once per surface on a shared request. This
// is the block the old suite kept behind an enterprise guard in its
// Community merge request file, where it never ran.
//
// Replaces: TestMeta_MRDeep
func TestMRApprovalRules_Lifecycle_ReadsStateAndManagesARule(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) approvalFixture {
		return newApprovalFixture(e, "apprrules")
	}, func(e *harness.Env, surface harness.Surface, f approvalFixture) {
		s := e.On(surface)
		params := map[string]any{"project_id": f.project.IDParam(), "merge_request_iid": f.mr.IID}

		state := harness.Do[mrapprovals.StateOutput](s, actionMRApprovalState, params)
		e.T.Logf("approval state: overwritten=%t rules=%d", state.ApprovalRulesOverwritten, len(state.Rules))
		rules := harness.Do[mrapprovals.RulesOutput](s, actionMRApprovalRules, params)
		e.T.Logf("%d approval rule(s) before the create", len(rules.Rules))
		// What the fixture guarantees is that nobody approved. Whether the
		// request counts as approved is GitLab's own reading of a request
		// that requires no approvals, which it settles as it computes the
		// request's mergeability, and a licensed run saw both answers.
		config := harness.Do[mrapprovals.ConfigOutput](s, actionMRApprovalConfig, params)
		if len(config.ApprovedBy) != 0 || config.UserHasApproved {
			e.T.Errorf("a fresh request reports %d approver(s) and user_has_approved=%t, want none", len(config.ApprovedBy), config.UserHasApproved)
		}

		name := e.Name("rule")
		created := harness.Do[mrapprovals.RuleOutput](s, actionMRApprovalRuleCreate, withParams(params, map[string]any{"name": name, "approvals_required": 1}))
		if created.ID == 0 {
			e.T.Fatalf("approval_rule_create answered %+v, want a rule with an ID", created)
		}

		updated := harness.Do[mrapprovals.RuleOutput](s, actionMRApprovalRuleUpdate, withParams(params, map[string]any{
			"approval_rule_id": created.ID, "name": name + "-updated", "approvals_required": 2,
		}))
		if updated.ID != created.ID || updated.Name != name+"-updated" || updated.ApprovalsRequired != 2 {
			e.T.Errorf("approval_rule_update answered %+v, want rule %d renamed to %q requiring 2", updated, created.ID, name+"-updated")
		}

		harness.DoVoid(s, actionMRApprovalRuleDelete, withParams(params, map[string]any{"approval_rule_id": created.ID}))
		remaining := harness.Do[mrapprovals.RulesOutput](s, actionMRApprovalRules, params)
		if containsID(mrApprovalRuleIDs(remaining.Rules), created.ID) {
			e.T.Errorf("rule %d is still listed after its delete", created.ID)
		}
	})
}

// TestMRApprovalReset_GroupBot_ClearsTheApproval approves a request from a
// session started on a group bot's token, resets the approvals from that
// same session, and reads from the run's own session that none is left.
// The bot is the request's only approver, through a rule that names it.
//
// The bot's session is served the Free catalog on a licensed instance, since
// the license endpoint answers administrators only, and the two actions it
// makes are Free: what the license gates is the rule that makes the bot an
// approver, which the run's own session creates.
//
// Replaces: TestMeta_MRDeep
func TestMRApprovalReset_GroupBot_ClearsTheApproval(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("apprreset"))
		f := newApprovalFixture(e, "apprreset", fixture.InGroup(group))
		bot := fixture.NewGroupToken(e, group, gl.OwnerPermissions)
		params := map[string]any{"project_id": f.project.IDParam(), "merge_request_iid": f.mr.IID}

		// The rule is retried while the bot's membership propagates, the
		// window the old suite met as 400s and 404s on this same call.
		rule := harness.Eventually(s, actionMRApprovalRuleCreate, withParams(params, map[string]any{
			"name": e.Name("bot-rule"), "approvals_required": 1, "user_ids": []int64{bot.UserID},
		}), membershipInterval, membershipWait, func(out mrapprovals.RuleOutput) bool { return out.ID != 0 })
		e.T.Logf("rule %d names the bot user %d as the approver", rule.ID, bot.UserID)

		botSession := e.Session(harness.ServerConfig{Surface: surface, Token: bot.Value, Private: true})
		approved := harness.Eventually(botSession, actionMRApprove, params, membershipInterval, membershipWait,
			func(out mergerequests.ApproveOutput) bool { return out.ApprovedBy > 0 })
		e.T.Logf("the bot approved: approved=%t by %d", approved.Approved, approved.ApprovedBy)

		harness.DoVoid(botSession, actionMRApprovalReset, params)

		config := harness.Do[mrapprovals.ConfigOutput](s, actionMRApprovalConfig, params)
		if len(config.ApprovedBy) != 0 {
			e.T.Errorf("the request still lists %d approver(s) after the reset: %+v", len(config.ApprovedBy), config.ApprovedBy)
		}
	})
}

// TestMRDependencies_Lifecycle_BlocksThenUnblocksARequest opens two
// requests in one project, makes the first block the second, finds the
// block in the second's listing and removes it, once per surface. The
// create names the blocking request by its instance-wide ID and the delete
// names the block record, which is the shape GitLab gives both.
//
// Replaces: TestMeta_MRBlockingDependencies, TestIndividual_MRDependenciesList
func TestMRDependencies_Lifecycle_BlocksThenUnblocksARequest(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		blocking := newApprovalFixture(e, "mrdeps")
		dependentBranch := fixture.NewBranch(e, blocking.project, e.Name("dependent"))
		fixture.CommitFile(e, blocking.project, dependentBranch.Name, "dependent.txt", "dependent fixture\n", "add the dependent fixture")
		dependent := fixture.NewMergeRequest(e, blocking.project, dependentBranch.Name, blocking.project.DefaultBranch, "dependent fixture")
		params := map[string]any{"project_id": blocking.project.IDParam(), "merge_request_iid": dependent.IID}

		fresh := harness.Do[mergerequests.DependenciesOutput](s, actionMRDependenciesList, params)
		if len(fresh.Dependencies) != 0 {
			e.T.Errorf("a fresh request lists %d dependencies: %+v", len(fresh.Dependencies), fresh.Dependencies)
		}

		created := harness.Do[mergerequests.DependencyOutput](s, actionMRDependencyCreate, withParams(params, map[string]any{
			"blocking_merge_request_id": blocking.mr.ID,
		}))
		if created.ID == 0 || created.BlockingMergeRequest == nil || created.BlockingMergeRequest.IID != blocking.mr.IID {
			e.T.Fatalf("dependency_create answered %+v, want a block record naming request !%d", created, blocking.mr.IID)
		}

		blocked := harness.Do[mergerequests.DependenciesOutput](s, actionMRDependenciesList, params)
		if len(blocked.Dependencies) != 1 {
			e.T.Errorf("the blocked request lists %d dependencies, want the one: %+v", len(blocked.Dependencies), blocked.Dependencies)
		}

		harness.DoVoid(s, actionMRDependencyDelete, withParams(params, map[string]any{"blocking_merge_request_id": created.ID}))
		unblocked := harness.Do[mergerequests.DependenciesOutput](s, actionMRDependenciesList, params)
		if len(unblocked.Dependencies) != 0 {
			e.T.Errorf("the request lists %d dependencies after the delete: %+v", len(unblocked.Dependencies), unblocked.Dependencies)
		}
	})
}
