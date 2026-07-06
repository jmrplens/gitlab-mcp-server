//go:build e2e && enterprise

// projectextras_ee_test.go covers the remaining Premium project- and
// issue-domain actions the e2e gap audit reported as unexercised: target
// branch rules (project.target_branch_rule_create/list/delete) via the
// gitlab_project meta-tool and issue weight events
// (issue.event_issue_weight_list) via the gitlab_issue meta-tool. The two
// small domains share a file per the gap-audit file layout.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourceevents"
)

// pjExtrasGraphQLFieldMissing reports whether err indicates the target
// branch rule GraphQL mutations/queries are absent from the instance schema,
// which happens when the pinned EE image predates the feature.
func pjExtrasGraphQLFieldMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "doesn't exist on type") || strings.Contains(msg, "Field 'targetBranchRule")
}

// TestMeta_TargetBranchRules exercises target branch rule create, list, and
// delete via the gitlab_project meta-tool against a live GitLab
// Premium/Ultimate instance.
//
// The test creates a project, adds a rule mapping the release/* wildcard to
// the default branch (create requires the numeric project ID), lists the
// rules by full path (the GraphQL query only accepts the full path), and
// deletes the rule by its ID. When the instance schema predates the target
// branch rule GraphQL API the test skips with a version-gated reason.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_TargetBranchRules(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var ruleID int64
	var ruleDeleted bool
	t.Cleanup(func() {
		if ruleID == 0 || ruleDeleted {
			return
		}
		cctx, ccancel := cleanupContext(defaultCleanupTimeout)
		defer ccancel()
		_ = callToolVoidOn(cctx, sess.meta, "gitlab_project", map[string]any{
			"action": "target_branch_rule_delete",
			"params": map[string]any{"rule_id": ruleID},
		})
	})

	t.Run("Meta/TargetBranchRule/Create", func(t *testing.T) {
		out, err := callToolOn[projects.TargetBranchRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "target_branch_rule_create",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"name":          "release/*",
				"target_branch": defaultBranch,
			},
		})
		if err != nil && pjExtrasGraphQLFieldMissing(err) {
			t.Skipf("target branch rule GraphQL API unavailable on this GitLab version: %v", err)
		}
		requirePremiumFeature(t, err, "target branch rule create")
		requireTruef(t, out.ID > 0, "target branch rule ID should be positive, got %d", out.ID)
		requireTruef(t, out.TargetBranch == defaultBranch,
			"target branch = %q, want %q", out.TargetBranch, defaultBranch)
		ruleID = out.ID
		t.Logf("Created target branch rule %d (%s -> %s)", out.ID, out.Name, out.TargetBranch)
	})

	t.Run("Meta/TargetBranchRule/List", func(t *testing.T) {
		requireTruef(t, ruleID > 0, "ruleID not set by the create subtest")
		out, err := callToolOn[projects.ListTargetBranchRulesOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "target_branch_rule_list",
			"params": map[string]any{
				// The list query resolves project(fullPath:) and rejects
				// numeric IDs, unlike the create mutation above.
				"project_id": proj.Path,
			},
		})
		requireNoError(t, err, "target branch rule list")
		found := false
		for _, rule := range out.TargetBranchRules {
			if rule.ID == ruleID {
				found = true
				break
			}
		}
		requireTruef(t, found, "created target branch rule %d not present in list of %d rule(s)", ruleID, len(out.TargetBranchRules))
		t.Logf("Listed %d target branch rule(s)", len(out.TargetBranchRules))
	})

	t.Run("Meta/TargetBranchRule/Delete", func(t *testing.T) {
		requireTruef(t, ruleID > 0, "ruleID not set by the create subtest")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "target_branch_rule_delete",
			"params": map[string]any{
				"rule_id": ruleID,
			},
		})
		requireNoError(t, err, "target branch rule delete")
		ruleDeleted = true
		t.Logf("Deleted target branch rule %d", ruleID)
	})
}

// TestMeta_IssueWeightEvents exercises event_issue_weight_list via the
// gitlab_issue meta-tool against a live GitLab Premium/Ultimate instance.
//
// The test creates a project and an issue, changes the issue weight twice so
// resource weight events are guaranteed to exist, then polls the weight
// event list until events surface and asserts the latest recorded weight
// matches the final update.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_IssueWeightEvents(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	issueOut, createErr := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"title":      uniqueName("weight-events"),
		},
	})
	requireNoError(t, createErr, "create issue for weight events")
	requireTruef(t, issueOut.IID > 0, "issue IID should be positive")

	// Two weight changes guarantee at least two weight events regardless of
	// whether GitLab records an event for the initial weight assignment.
	for _, weight := range []int64{3, 5} {
		_, updErr := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueOut.IID,
				"weight":     weight,
			},
		})
		requireNoError(t, updErr, "update issue weight")
	}

	out, err := retryWithBackoff(ctx, t, "issue weight events list", 6, func(int) (resourceevents.ListWeightEventsOutput, bool, string, error) {
		out, err := callToolOn[resourceevents.ListWeightEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_weight_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueOut.IID,
			},
		})
		if err != nil {
			return out, isRetryableError(err), "transient weight event API error", err
		}
		if len(out.Events) == 0 {
			return out, true, "weight events not recorded yet", geExtrasError("issue weight events empty")
		}
		return out, false, "", nil
	})
	requirePremiumFeature(t, err, "issue weight events")

	lastWeightSeen := false
	for _, event := range out.Events {
		requireTruef(t, event.ID > 0, "weight event ID should be positive")
		if event.Weight == 5 {
			lastWeightSeen = true
		}
	}
	requireTruef(t, lastWeightSeen, "expected a weight event recording weight=5, got %d event(s)", len(out.Events))
	t.Logf("Issue !%d weight events: %d", issueOut.IID, len(out.Events))
}
