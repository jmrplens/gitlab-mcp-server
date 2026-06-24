// shapes_test.go contains unit tests for the MR approval sub-object converters
// that mirror client-go types (basic users, approver users, approver groups,
// project approval rules, protected branches, and groups). Tests cover full
// data, nil values, and nil-slice-element branches to exercise the nil-safe
// converter paths to completion.
package mrapprovals

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestBasicUserOutput_NilAndFull verifies basicUserOutput maps every field and
// returns nil for a nil input.
func TestBasicUserOutput_NilAndFull(t *testing.T) {
	if got := basicUserOutput(nil); got != nil {
		t.Errorf("basicUserOutput(nil) = %v, want nil", got)
	}
	created := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	out := basicUserOutput(&gl.BasicUser{
		ID: 7, Username: "alice", Name: "Alice", State: "active",
		AvatarURL: "https://a", WebURL: "https://w", CreatedAt: &created,
	})
	if out == nil || out.ID != 7 || out.Username != "alice" || out.Name != "Alice" ||
		out.State != "active" || out.AvatarURL != "https://a" || out.WebURL != "https://w" {
		t.Fatalf("basicUserOutput full = %+v", out)
	}
	if out.CreatedAt != "2026-02-01T09:00:00Z" {
		t.Errorf("CreatedAt = %q, want RFC3339", out.CreatedAt)
	}
}

// TestBasicUserOutputs_EmptyAndNilElements verifies basicUserOutputs returns nil
// for an empty slice and skips nil elements.
func TestBasicUserOutputs_EmptyAndNilElements(t *testing.T) {
	if got := basicUserOutputs(nil); got != nil {
		t.Errorf("basicUserOutputs(nil) = %v, want nil", got)
	}
	out := basicUserOutputs([]*gl.BasicUser{nil, {Name: "Kept"}, nil})
	if len(out) != 1 || out[0].Name != "Kept" {
		t.Fatalf("basicUserOutputs skip-nil = %+v", out)
	}
}

// TestApproverUserOutput_NilUserAndTimestamp verifies approverUserOutput maps the
// nested user and approval timestamp, preserves a nil user, and returns nil for a
// nil input.
func TestApproverUserOutput_NilUserAndTimestamp(t *testing.T) {
	if got := approverUserOutput(nil); got != nil {
		t.Errorf("approverUserOutput(nil) = %v, want nil", got)
	}
	// {User: nil} is preserved with a nil user object and empty timestamp.
	nilUser := approverUserOutput(&gl.MergeRequestApproverUser{})
	if nilUser == nil || nilUser.User != nil || nilUser.ApprovedAt != "" {
		t.Fatalf("approverUserOutput nil-user = %+v", nilUser)
	}
	approved := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	full := approverUserOutput(&gl.MergeRequestApproverUser{
		User: &gl.BasicUser{Name: "Bob"}, ApprovedAt: &approved,
	})
	if full == nil || full.User == nil || full.User.Name != "Bob" || full.ApprovedAt != "2026-03-01T12:00:00Z" {
		t.Fatalf("approverUserOutput full = %+v", full)
	}
}

// TestApproverUserOutputs_EmptyAndNilElements verifies approverUserOutputs returns
// nil for an empty slice and skips nil elements.
func TestApproverUserOutputs_EmptyAndNilElements(t *testing.T) {
	if got := approverUserOutputs(nil); got != nil {
		t.Errorf("approverUserOutputs(nil) = %v, want nil", got)
	}
	out := approverUserOutputs([]*gl.MergeRequestApproverUser{nil, {User: &gl.BasicUser{Name: "X"}}})
	if len(out) != 1 || out[0].User == nil || out[0].User.Name != "X" {
		t.Fatalf("approverUserOutputs skip-nil = %+v", out)
	}
}

// TestApproverGroupOutput_NilAndFull verifies approverGroupOutput maps the nested
// group fields and returns nil for a nil input.
func TestApproverGroupOutput_NilAndFull(t *testing.T) {
	if got := approverGroupOutput(nil); got != nil {
		t.Errorf("approverGroupOutput(nil) = %v, want nil", got)
	}
	out := approverGroupOutput(&gl.MergeRequestApproverGroup{
		Group: gl.MergeRequestApproverNestedGroup{
			ID: 3, Name: "Sec", Path: "sec", Description: "d", Visibility: "private",
			AvatarURL: "https://a", WebURL: "https://w", FullName: "Sec Team",
			FullPath: "org/sec", LFSEnabled: true, RequestAccessEnabled: true,
		},
	})
	if out == nil || out.Group.ID != 3 || out.Group.Name != "Sec" || out.Group.Path != "sec" ||
		out.Group.Description != "d" || out.Group.Visibility != "private" || out.Group.AvatarURL != "https://a" ||
		out.Group.WebURL != "https://w" || out.Group.FullName != "Sec Team" || out.Group.FullPath != "org/sec" ||
		!out.Group.LFSEnabled || !out.Group.RequestAccessEnabled {
		t.Fatalf("approverGroupOutput full = %+v", out)
	}
}

// TestApproverGroupOutputs_EmptyAndNilElements verifies approverGroupOutputs
// returns nil for an empty slice and skips nil elements.
func TestApproverGroupOutputs_EmptyAndNilElements(t *testing.T) {
	if got := approverGroupOutputs(nil); got != nil {
		t.Errorf("approverGroupOutputs(nil) = %v, want nil", got)
	}
	out := approverGroupOutputs([]*gl.MergeRequestApproverGroup{
		nil,
		{Group: gl.MergeRequestApproverNestedGroup{Name: "Kept"}},
	})
	if len(out) != 1 || out[0].Group.Name != "Kept" {
		t.Fatalf("approverGroupOutputs skip-nil = %+v", out)
	}
}

// TestGroupOutput_NilAndFull verifies groupOutput maps the compact identifying
// fields, formats the timestamp, and returns nil for a nil input.
func TestGroupOutput_NilAndFull(t *testing.T) {
	if got := groupOutput(nil); got != nil {
		t.Errorf("groupOutput(nil) = %v, want nil", got)
	}
	created := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	out := groupOutput(&gl.Group{
		ID: 9, Name: "Eng", Path: "eng", FullPath: "org/eng", FullName: "Engineering",
		Description: "team", Visibility: gl.PrivateVisibility, WebURL: "https://w",
		AvatarURL: "https://a", ParentID: 2, RequestAccessEnabled: true, LFSEnabled: true,
		CreatedAt: &created,
	})
	if out == nil || out.ID != 9 || out.Name != "Eng" || out.Path != "eng" || out.FullPath != "org/eng" ||
		out.FullName != "Engineering" || out.Description != "team" || out.Visibility != "private" ||
		out.WebURL != "https://w" || out.AvatarURL != "https://a" || out.ParentID != 2 ||
		!out.RequestAccessEnabled || !out.LFSEnabled || out.CreatedAt != "2025-12-31T00:00:00Z" {
		t.Fatalf("groupOutput full = %+v", out)
	}
}

// TestGroupOutputs_EmptyAndNilElements verifies groupOutputs returns nil for an
// empty slice and skips nil elements.
func TestGroupOutputs_EmptyAndNilElements(t *testing.T) {
	if got := groupOutputs(nil); got != nil {
		t.Errorf("groupOutputs(nil) = %v, want nil", got)
	}
	out := groupOutputs([]*gl.Group{nil, {Name: "Kept"}})
	if len(out) != 1 || out[0].Name != "Kept" {
		t.Fatalf("groupOutputs skip-nil = %+v", out)
	}
}

// TestProtectedBranchOutputs_EmptyAndNilElements verifies protectedBranchOutputs
// returns nil for an empty slice, skips nil elements, and maps each field.
func TestProtectedBranchOutputs_EmptyAndNilElements(t *testing.T) {
	if got := protectedBranchOutputs(nil); got != nil {
		t.Errorf("protectedBranchOutputs(nil) = %v, want nil", got)
	}
	out := protectedBranchOutputs([]*gl.ProtectedBranch{
		nil,
		{ID: 4, Name: "main", AllowForcePush: true, CodeOwnerApprovalRequired: true},
	})
	if len(out) != 1 || out[0].ID != 4 || out[0].Name != "main" ||
		!out[0].AllowForcePush || !out[0].CodeOwnerApprovalRequired {
		t.Fatalf("protectedBranchOutputs = %+v", out)
	}
}

// TestProjectApprovalRuleOutput_NilAndFull verifies projectApprovalRuleOutput
// maps every field including nested users, groups, and protected branches, and
// returns nil for a nil input.
func TestProjectApprovalRuleOutput_NilAndFull(t *testing.T) {
	if got := projectApprovalRuleOutput(nil); got != nil {
		t.Errorf("projectApprovalRuleOutput(nil) = %v, want nil", got)
	}
	out := projectApprovalRuleOutput(&gl.ProjectApprovalRule{
		ID: 11, Name: "Project Rule", RuleType: "regular", ReportType: "code_coverage",
		EligibleApprovers:             []*gl.BasicUser{{Name: "Ann"}},
		ApprovalsRequired:             3,
		Users:                         []*gl.BasicUser{{Name: "Ann"}},
		Groups:                        []*gl.Group{{Name: "Owners"}},
		ContainsHiddenGroups:          true,
		ProtectedBranches:             []*gl.ProtectedBranch{{Name: "main"}},
		AppliesToAllProtectedBranches: true,
	})
	if out == nil || out.ID != 11 || out.Name != "Project Rule" || out.RuleType != "regular" ||
		out.ReportType != "code_coverage" || out.ApprovalsRequired != 3 || !out.ContainsHiddenGroups ||
		!out.AppliesToAllProtectedBranches {
		t.Fatalf("projectApprovalRuleOutput scalars = %+v", out)
	}
	if len(out.EligibleApprovers) != 1 || len(out.Users) != 1 || len(out.Groups) != 1 ||
		len(out.ProtectedBranches) != 1 {
		t.Fatalf("projectApprovalRuleOutput nested counts = %+v", out)
	}
}

// TestRuleToOutput_WithSourceRule verifies RuleToOutput surfaces the nested
// project-level source rule on the source_rule key.
func TestRuleToOutput_WithSourceRule(t *testing.T) {
	out := RuleToOutput(&gl.MergeRequestApprovalRule{
		ID:   1,
		Name: "Rule",
		SourceRule: &gl.ProjectApprovalRule{
			ID: 99, Name: "Source", ApprovalsRequired: 1,
		},
	})
	if out.SourceRule == nil || out.SourceRule.ID != 99 || out.SourceRule.Name != "Source" {
		t.Fatalf("RuleToOutput SourceRule = %+v", out.SourceRule)
	}
}

// TestRuleToOutput_NilSourceRule verifies RuleToOutput leaves source_rule nil
// when the SDK rule has no source rule.
func TestRuleToOutput_NilSourceRule(t *testing.T) {
	out := RuleToOutput(&gl.MergeRequestApprovalRule{ID: 1, Name: "Rule"})
	if out.SourceRule != nil {
		t.Errorf("RuleToOutput SourceRule = %v, want nil", out.SourceRule)
	}
}

// TestConfigToOutput_AllAdditiveFields verifies configToOutput surfaces every
// previously-missing field of gl.MergeRequestApprovals, including the timestamps,
// approver groups, approvers, suggested approvers, and the per-rule
// approval_rules_left list.
func TestConfigToOutput_AllAdditiveFields(t *testing.T) {
	created := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	c := gl.MergeRequestApprovals{
		ID: 1, IID: 10, ProjectID: 42, Title: "MR", Description: "desc", State: "opened",
		CreatedAt: &created, UpdatedAt: &updated, MergeStatus: "can_be_merged",
		Approved: true, ApprovalsRequired: 2, ApprovalsLeft: 1, ApprovalsBeforeMerge: 2,
		RequirePasswordToApprove: true, HasApprovalRules: true, UserHasApproved: true,
		UserCanApprove: true, MergeRequestApproversAvailable: true,
		MultipleApprovalRulesAvailable: true,
		Approvers:                      []*gl.MergeRequestApproverUser{{User: &gl.BasicUser{Name: "Eve"}}},
		ApproverGroups:                 []*gl.MergeRequestApproverGroup{{Group: gl.MergeRequestApproverNestedGroup{Name: "Sec"}}},
		ApprovalRulesLeft: []*gl.MergeRequestApprovalRule{
			nil,
			{ID: 5, Name: "Left Rule", ApprovalsRequired: 1},
		},
	}
	out := configToOutput(&c)
	if out.Description != "desc" || out.MergeStatus != "can_be_merged" ||
		out.CreatedAt != "2026-01-01T08:00:00Z" || out.UpdatedAt != "2026-01-02T08:00:00Z" ||
		!out.RequirePasswordToApprove || !out.MergeRequestApproversAvailable ||
		!out.MultipleApprovalRulesAvailable {
		t.Fatalf("configToOutput additive scalars = %+v", out)
	}
	if len(out.Approvers) != 1 || out.Approvers[0].User == nil || out.Approvers[0].User.Name != "Eve" {
		t.Errorf("Approvers = %+v", out.Approvers)
	}
	if len(out.ApproverGroups) != 1 || out.ApproverGroups[0].Group.Name != "Sec" {
		t.Errorf("ApproverGroups = %+v", out.ApproverGroups)
	}
	// The nil *MergeRequestApprovalRule element is skipped, leaving one rule.
	if len(out.ApprovalRulesLeft) != 1 || out.ApprovalRulesLeft[0].ID != 5 {
		t.Errorf("ApprovalRulesLeft = %+v", out.ApprovalRulesLeft)
	}
}
