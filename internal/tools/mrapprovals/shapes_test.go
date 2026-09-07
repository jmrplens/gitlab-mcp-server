// shapes_test.go contains unit tests for the MR approval sub-object converters
// that mirror client-go types (basic users, approver users, approver groups,
// project approval rules, protected branches, and groups). Tests cover full
// data, nil values, and nil-slice-element branches to exercise the nil-safe
// converter paths to completion.
package mrapprovals

import (
	"encoding/json"
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
	// created_at is intentionally not projected: the documented reference subset
	// (doc/api/merge_request_approvals.md) omits it. Supplying it on the SDK
	// input must not surface a CreatedAt field on the output (the field no
	// longer exists on BasicUserOutput).
	created := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	out := basicUserOutput(&gl.BasicUser{
		ID: 7, Username: "alice", Name: "Alice", State: "active",
		AvatarURL: "https://a", WebURL: "https://w", CreatedAt: &created,
	})
	if out == nil || out.ID != 7 || out.Username != "alice" || out.Name != "Alice" ||
		out.State != "active" || out.AvatarURL != "https://a" || out.WebURL != "https://w" {
		t.Fatalf("basicUserOutput full = %+v", out)
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

// The two approver-group converter tests stood here. Their subject is gone
// along with the field that reached it: GitLab answers approver_groups only at
// the deprecated POST this package does not call.

// TestGroupOutput_NilAndFull verifies groupOutput maps the documented reference
// subset fields and returns nil for a nil input. created_at is intentionally not
// projected per doc/api/merge_request_approvals.md, so supplying it on the SDK
// input must not surface a CreatedAt field on the output.
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
		!out.RequestAccessEnabled || !out.LFSEnabled {
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

// TestConfigToOutput_TakesTheFourFieldsTheGETAnswersWith verifies the converter
// against the endpoint rather than against the SDK type.
//
// Its predecessor asserted the opposite, field by field: that configToOutput
// surfaced every member of gl.MergeRequestApprovals. That is what held the
// defect in place. The SDK type models the response of the POST at this path,
// deprecated in GitLab 16.0, and a test demanding SDK fidelity from a converter
// reading a GET's answer demands that twenty fields be published which GitLab
// never sends. Filling every SDK field here and asserting only four come out is
// the shape that catches a re-widening.
func TestConfigToOutput_TakesTheFourFieldsTheGETAnswersWith(t *testing.T) {
	created := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	c := gl.MergeRequestApprovals{
		ID: 1, IID: 10, ProjectID: 42, Title: "MR", Description: "desc", State: "opened",
		CreatedAt: &created, UpdatedAt: &updated, MergeStatus: "can_be_merged",
		Approved: true, ApprovalsRequired: 2, ApprovalsLeft: 1, ApprovalsBeforeMerge: 2,
		RequirePasswordToApprove: true, HasApprovalRules: true, UserHasApproved: true,
		UserCanApprove: true, MergeRequestApproversAvailable: true,
		MultipleApprovalRulesAvailable: true,
		ApprovedBy:                     []*gl.MergeRequestApproverUser{{User: &gl.BasicUser{Name: "Alice"}}},
		SuggestedApprovers:             []*gl.BasicUser{{Name: "Bob"}},
		Approvers:                      []*gl.MergeRequestApproverUser{{User: &gl.BasicUser{Name: "Eve"}}},
		ApproverGroups:                 []*gl.MergeRequestApproverGroup{{Group: gl.MergeRequestApproverNestedGroup{Name: "Sec"}}},
		ApprovalRulesLeft: []*gl.MergeRequestApprovalRule{
			nil,
			{ID: 5, Name: "Left Rule", ApprovalsRequired: 1},
		},
	}

	out := configToOutput(&c)

	if !out.Approved || !out.UserHasApproved || !out.UserCanApprove {
		t.Errorf("configToOutput scalars = %+v, want the three booleans the GET answers with", out)
	}
	if len(out.ApprovedBy) != 1 || out.ApprovedBy[0].User == nil || out.ApprovedBy[0].User.Name != "Alice" {
		t.Errorf("ApprovedBy = %+v, want the one approver", out.ApprovedBy)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal the output: %v", err)
	}
	var keys map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(encoded, &keys); unmarshalErr != nil {
		t.Fatalf("read the output back: %v", unmarshalErr)
	}
	if len(keys) != 4 {
		t.Errorf("published %d keys (%v), want exactly approved, user_has_approved, user_can_approve and approved_by", len(keys), keys)
	}
}
