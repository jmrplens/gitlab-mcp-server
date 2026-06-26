package mrapprovals

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface the SDK sub-object fields on
// the canonical JSON keys and are replicated here rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the merge-request approval sub-objects: approver users
// (approved_by, approvers), approver groups (approver_groups), basic users
// (suggested_approvers, eligible_approvers, users, approved_by on rules),
// rule groups (groups), and the project-level source rule (source_rule). The
// compact-mirror depth follows the project norm established by
// internal/tools/groups Output (curated identifying fields rather than the full
// gl.Group / gl.ProjectApprovalRule surface, which embeds large nested
// statistics, deprecated project lists, and LDAP/SAML links).

// BasicUserOutput is the documented reference subset of the compact user object
// embedded in approval payloads (suggested_approvers, eligible_approvers, users,
// and the approved_by list on approval rules).
//
// Documented reference subset per doc/api/merge_request_approvals.md: the rule
// users[]/eligible_approvers[]/approved_by[] examples surface only id, name,
// username, state, avatar_url, and web_url. gl.BasicUser additionally carries
// created_at, which the documented reference object omits; it is therefore not
// projected here.
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// basicUserOutput converts a single gl.BasicUser to its output shape, returning
// nil when the SDK value is nil.
func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// basicUserOutputs converts a slice of gl.BasicUser, skipping nil elements and
// returning nil for an empty or all-nil slice.
func basicUserOutputs(users []*gl.BasicUser) []*BasicUserOutput {
	if len(users) == 0 {
		return nil
	}
	out := make([]*BasicUserOutput, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, basicUserOutput(u))
	}
	return out
}

// MergeRequestApproverUserOutput mirrors gl.MergeRequestApproverUser, pairing an
// approver's user object with the timestamp at which they approved. Its nested
// user object is the documented reference subset BasicUserOutput per
// doc/api/merge_request_approvals.md (the approved_by[].user example).
type MergeRequestApproverUserOutput struct {
	User       *BasicUserOutput `json:"user,omitempty"`
	ApprovedAt string           `json:"approved_at,omitempty"`
}

// approverUserOutput converts a single gl.MergeRequestApproverUser to its
// output shape, returning nil when the SDK value is nil.
func approverUserOutput(u *gl.MergeRequestApproverUser) *MergeRequestApproverUserOutput {
	if u == nil {
		return nil
	}
	return &MergeRequestApproverUserOutput{
		User:       basicUserOutput(u.User),
		ApprovedAt: toolutil.FormatTimePtr(u.ApprovedAt),
	}
}

// approverUserOutputs converts a slice of gl.MergeRequestApproverUser, skipping
// nil elements and returning nil for an empty or all-nil slice.
func approverUserOutputs(users []*gl.MergeRequestApproverUser) []*MergeRequestApproverUserOutput {
	if len(users) == 0 {
		return nil
	}
	out := make([]*MergeRequestApproverUserOutput, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, approverUserOutput(u))
	}
	return out
}

// MergeRequestApproverNestedGroupOutput mirrors
// gl.MergeRequestApproverNestedGroup, the group object embedded in the
// approver_groups list of a merge-request approval configuration.
type MergeRequestApproverNestedGroupOutput struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Path                 string `json:"path"`
	Description          string `json:"description,omitempty"`
	Visibility           string `json:"visibility,omitempty"`
	AvatarURL            string `json:"avatar_url,omitempty"`
	WebURL               string `json:"web_url,omitempty"`
	FullName             string `json:"full_name,omitempty"`
	FullPath             string `json:"full_path,omitempty"`
	LFSEnabled           bool   `json:"lfs_enabled"`
	RequestAccessEnabled bool   `json:"request_access_enabled"`
}

// MergeRequestApproverGroupOutput mirrors gl.MergeRequestApproverGroup, the
// wrapper object carrying a single nested approver group.
type MergeRequestApproverGroupOutput struct {
	Group MergeRequestApproverNestedGroupOutput `json:"group"`
}

// approverGroupOutput converts a single gl.MergeRequestApproverGroup to its
// output shape, returning nil when the SDK value is nil.
func approverGroupOutput(g *gl.MergeRequestApproverGroup) *MergeRequestApproverGroupOutput {
	if g == nil {
		return nil
	}
	ng := g.Group
	return &MergeRequestApproverGroupOutput{
		Group: MergeRequestApproverNestedGroupOutput{
			ID: ng.ID, Name: ng.Name, Path: ng.Path, Description: ng.Description,
			Visibility: ng.Visibility, AvatarURL: ng.AvatarURL, WebURL: ng.WebURL,
			FullName: ng.FullName, FullPath: ng.FullPath, LFSEnabled: ng.LFSEnabled,
			RequestAccessEnabled: ng.RequestAccessEnabled,
		},
	}
}

// approverGroupOutputs converts a slice of gl.MergeRequestApproverGroup,
// skipping nil elements and returning nil for an empty or all-nil slice.
func approverGroupOutputs(groups []*gl.MergeRequestApproverGroup) []*MergeRequestApproverGroupOutput {
	if len(groups) == 0 {
		return nil
	}
	out := make([]*MergeRequestApproverGroupOutput, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		out = append(out, approverGroupOutput(g))
	}
	return out
}

// GroupOutput is the documented reference subset of the group object embedded
// in an approval rule's groups list.
//
// Documented reference subset per doc/api/merge_request_approvals.md: the rule
// groups[] example surfaces id, name, path, description, visibility,
// lfs_enabled, avatar_url, web_url, request_access_enabled, full_name,
// full_path, and parent_id (the ldap_cn/ldap_access LDAP extras are EE-only and
// the large nested statistics/deprecated project lists carried by gl.Group are
// not part of the documented reference object). gl.Group additionally carries
// created_at, which the documented reference object omits; it is therefore not
// projected here.
type GroupOutput struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Path                 string `json:"path"`
	FullPath             string `json:"full_path,omitempty"`
	FullName             string `json:"full_name,omitempty"`
	Description          string `json:"description,omitempty"`
	Visibility           string `json:"visibility,omitempty"`
	WebURL               string `json:"web_url,omitempty"`
	AvatarURL            string `json:"avatar_url,omitempty"`
	ParentID             int64  `json:"parent_id,omitempty"`
	RequestAccessEnabled bool   `json:"request_access_enabled"`
	LFSEnabled           bool   `json:"lfs_enabled"`
}

// groupOutput converts a single gl.Group to its compact output shape, returning
// nil when the SDK value is nil.
func groupOutput(g *gl.Group) *GroupOutput {
	if g == nil {
		return nil
	}
	return &GroupOutput{
		ID: g.ID, Name: g.Name, Path: g.Path, FullPath: g.FullPath,
		FullName: g.FullName, Description: g.Description,
		Visibility: string(g.Visibility), WebURL: g.WebURL, AvatarURL: g.AvatarURL,
		ParentID: g.ParentID, RequestAccessEnabled: g.RequestAccessEnabled,
		LFSEnabled: g.LFSEnabled,
	}
}

// groupOutputs converts a slice of gl.Group, skipping nil elements and
// returning nil for an empty or all-nil slice.
func groupOutputs(groups []*gl.Group) []*GroupOutput {
	if len(groups) == 0 {
		return nil
	}
	out := make([]*GroupOutput, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		out = append(out, groupOutput(g))
	}
	return out
}

// ProtectedBranchOutput mirrors the identifying fields of gl.ProtectedBranch as
// embedded in a project approval rule's protected_branches list.
type ProtectedBranchOutput struct {
	ID                        int64  `json:"id"`
	Name                      string `json:"name"`
	AllowForcePush            bool   `json:"allow_force_push"`
	CodeOwnerApprovalRequired bool   `json:"code_owner_approval_required"`
}

// protectedBranchOutputs converts a slice of gl.ProtectedBranch, skipping nil
// elements and returning nil for an empty or all-nil slice.
func protectedBranchOutputs(branches []*gl.ProtectedBranch) []*ProtectedBranchOutput {
	if len(branches) == 0 {
		return nil
	}
	out := make([]*ProtectedBranchOutput, 0, len(branches))
	for _, b := range branches {
		if b == nil {
			continue
		}
		out = append(out, &ProtectedBranchOutput{
			ID: b.ID, Name: b.Name, AllowForcePush: b.AllowForcePush,
			CodeOwnerApprovalRequired: b.CodeOwnerApprovalRequired,
		})
	}
	return out
}

// ProjectApprovalRuleOutput mirrors gl.ProjectApprovalRule, the project-level
// approval rule referenced as the source_rule of a merge-request approval rule.
//
// The documented examples in doc/api/merge_request_approvals.md always render
// source_rule as null, so the doc provides no reference sub-field shape to trim
// against. This SDK-modeled superset is retained additively (version drift →
// keep) so instances that populate source_rule do not lose data. Its nested
// user lists reuse the documented reference subset BasicUserOutput and its
// groups reuse the documented reference subset GroupOutput.
type ProjectApprovalRuleOutput struct {
	ID                            int64                    `json:"id"`
	Name                          string                   `json:"name"`
	RuleType                      string                   `json:"rule_type,omitempty"`
	ReportType                    string                   `json:"report_type,omitempty"`
	EligibleApprovers             []*BasicUserOutput       `json:"eligible_approvers,omitempty"`
	ApprovalsRequired             int64                    `json:"approvals_required"`
	Users                         []*BasicUserOutput       `json:"users,omitempty"`
	Groups                        []*GroupOutput           `json:"groups,omitempty"`
	ContainsHiddenGroups          bool                     `json:"contains_hidden_groups"`
	ProtectedBranches             []*ProtectedBranchOutput `json:"protected_branches,omitempty"`
	AppliesToAllProtectedBranches bool                     `json:"applies_to_all_protected_branches"`
}

// projectApprovalRuleOutput converts a single gl.ProjectApprovalRule to its
// output shape, returning nil when the SDK value is nil.
func projectApprovalRuleOutput(r *gl.ProjectApprovalRule) *ProjectApprovalRuleOutput {
	if r == nil {
		return nil
	}
	return &ProjectApprovalRuleOutput{
		ID: r.ID, Name: r.Name, RuleType: r.RuleType, ReportType: r.ReportType,
		EligibleApprovers:             basicUserOutputs(r.EligibleApprovers),
		ApprovalsRequired:             r.ApprovalsRequired,
		Users:                         basicUserOutputs(r.Users),
		Groups:                        groupOutputs(r.Groups),
		ContainsHiddenGroups:          r.ContainsHiddenGroups,
		ProtectedBranches:             protectedBranchOutputs(r.ProtectedBranches),
		AppliesToAllProtectedBranches: r.AppliesToAllProtectedBranches,
	}
}

// mergeRequestApprovalRuleAPI is the raw-fetch superset of
// gl.MergeRequestApprovalRule. It mirrors every SDK field on its canonical JSON
// key and additionally decodes the documented "overridden" boolean, which the
// SDK's gl.MergeRequestApprovalRule does not model (the SDK struct has no
// Overridden field, yet the approval_state/list/create/update rule responses
// document it).
//
// The single client.GL().Do(&rule) unmarshal is naturally version-tolerant: on
// older instances that omit "overridden" the field decodes to its zero value
// (false) with no error, and the omitempty tag on RuleOutput.Overridden keeps
// it out of the rendered JSON. The nested objects reuse the SDK sub-object
// types so the documented-reference-subset converters (basicUserOutputs,
// groupOutputs, projectApprovalRuleOutput) apply uniformly.
type mergeRequestApprovalRuleAPI struct {
	ID                   int64                   `json:"id"`
	Name                 string                  `json:"name"`
	RuleType             string                  `json:"rule_type"`
	ReportType           string                  `json:"report_type"`
	Section              string                  `json:"section"`
	ApprovalsRequired    int64                   `json:"approvals_required"`
	Approved             bool                    `json:"approved"`
	ContainsHiddenGroups bool                    `json:"contains_hidden_groups"`
	Overridden           bool                    `json:"overridden,omitempty"`
	ApprovedBy           []*gl.BasicUser         `json:"approved_by"`
	EligibleApprovers    []*gl.BasicUser         `json:"eligible_approvers"`
	Users                []*gl.BasicUser         `json:"users"`
	Groups               []*gl.Group             `json:"groups"`
	SourceRule           *gl.ProjectApprovalRule `json:"source_rule"`
}

// mergeRequestApprovalStateAPI is the raw-fetch superset of
// gl.MergeRequestApprovalState. It mirrors the SDK's approval_rules_overwritten
// flag and decodes each rule into the [mergeRequestApprovalRuleAPI] superset so
// the documented per-rule "overridden" boolean is preserved.
type mergeRequestApprovalStateAPI struct {
	ApprovalRulesOverwritten bool                           `json:"approval_rules_overwritten"`
	Rules                    []*mergeRequestApprovalRuleAPI `json:"rules"`
}
