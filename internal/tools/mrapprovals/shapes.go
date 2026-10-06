package mrapprovals

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface the SDK sub-object fields on
// the canonical JSON keys and are replicated here rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the merge-request approval sub-objects: approver users
// (approved_by), basic users (eligible_approvers, users, approved_by on rules),
// rule groups (groups), the project-level source rule (source_rule), and the
// Enterprise Edition approval state with its approvers, approver groups and
// short rule references, which is read off the captured answer. The
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

// EnterpriseApprovalState is what an Enterprise Edition instance adds to the
// approvals of a merge request: twenty keys a Community Edition instance never
// sends.
//
// GitLab answers GET .../approvals, POST .../approve and POST .../unapprove
// through one helper, present_approval. Community Edition presents
// Entities::MergeRequestApprovals, which is approved, approved_by,
// user_has_approved and user_can_approve
// (lib/api/entities/merge_request_approvals.rb).
// ee/lib/ee/api/merge_request_approvals.rb overrides that helper in its
// prepended block, with no license check, so every Enterprise Edition build,
// licensed or not and GitLab.com included, presents the merge request's
// approval state with Entities::ApprovalState instead
// (ee/lib/api/entities/approval_state.rb): the same four, the merge request's
// own identity, the approval counts, the rules still to satisfy and the flags
// below. The routes keep the Community Edition annotation, so GitLab's
// generated OpenAPI document lists the four alone for all three.
//
// Every field is read off the captured answer (ADR-0021) and is absent when
// GitLab did not send it: a Community Edition answer publishes none of them,
// while an Enterprise one keeps its zeros, since approvals_left 0 says no
// approval is missing and has_approval_rules false says no rule exists. That is
// why the scalars are pointers, save the two timestamps, which are strings left
// empty when GitLab sent none, and the lists are omitted only when GitLab sent
// no list. client-go's MergeRequestApprovals cannot carry that difference: its
// fields read zero either way. It also has no invalid_approvers_rules, types
// approval_rules_left with the whole approval rule where GitLab sends the short
// reference, and declares approvals_before_merge, which no edition sends here
// and which is not published.
type EnterpriseApprovalState struct {
	ID                             *int64                     `json:"id,omitempty"                                jsonschema:"Merge request ID (Enterprise Edition only)"`
	IID                            *int64                     `json:"iid,omitempty"                               jsonschema:"Merge request IID (Enterprise Edition only)"`
	ProjectID                      *int64                     `json:"project_id,omitempty"                        jsonschema:"Project ID (Enterprise Edition only)"`
	Title                          *string                    `json:"title,omitempty"                             jsonschema:"Merge request title (Enterprise Edition only)"`
	Description                    *string                    `json:"description,omitempty"                       jsonschema:"Merge request description (Enterprise Edition only)"`
	State                          *string                    `json:"state,omitempty"                             jsonschema:"Merge request state (Enterprise Edition only)"`
	CreatedAt                      string                     `json:"created_at,omitempty"                        jsonschema:"When the merge request was created (Enterprise Edition only)"`
	UpdatedAt                      string                     `json:"updated_at,omitempty"                        jsonschema:"When the merge request was last updated (Enterprise Edition only)"`
	MergeStatus                    *string                    `json:"merge_status,omitempty"                      jsonschema:"Deprecated mergeability status (Enterprise Edition only)"`
	ApprovalsRequired              *int64                     `json:"approvals_required,omitempty"                jsonschema:"Approvals the merge request requires (Enterprise Edition only)"`
	ApprovalsLeft                  *int64                     `json:"approvals_left,omitempty"                    jsonschema:"Approvals still missing, 0 when none are (Enterprise Edition only)"`
	RequirePasswordToApprove       *bool                      `json:"require_password_to_approve,omitempty"       jsonschema:"Deprecated: whether approving asks for the password (Enterprise Edition only)"`
	SuggestedApprovers             []toolutil.UserBasicOutput `json:"suggested_approvers,omitzero"                jsonschema:"Users suggested as approvers (Enterprise Edition only)"`
	Approvers                      []ApproverOutput           `json:"approvers,omitzero"                          jsonschema:"Deprecated: the users of the first regular rule (Enterprise Edition only)"`
	ApproverGroups                 []ApproverGroupOutput      `json:"approver_groups,omitzero"                    jsonschema:"Deprecated: the groups of the first regular rule (Enterprise Edition only)"`
	ApprovalRulesLeft              []ApprovalRuleShortOutput  `json:"approval_rules_left,omitzero"                jsonschema:"Rules not yet satisfied (Enterprise Edition only)"`
	HasApprovalRules               *bool                      `json:"has_approval_rules,omitempty"                jsonschema:"Whether any user-defined approval rule applies (Enterprise Edition only)"`
	MergeRequestApproversAvailable *bool                      `json:"merge_request_approvers_available,omitempty" jsonschema:"Whether the license includes approval rules (Enterprise Edition only)"`
	MultipleApprovalRulesAvailable *bool                      `json:"multiple_approval_rules_available,omitempty" jsonschema:"Whether the license allows several approval rules (Enterprise Edition only)"`
	InvalidApproversRules          []ApprovalRuleShortOutput  `json:"invalid_approvers_rules,omitzero"            jsonschema:"Rules no eligible user can satisfy (Enterprise Edition only)"`
}

// ApproverOutput is one entry of the deprecated approvers list of an
// Enterprise Edition approval state: a user of the merge request's first
// regular rule, under user, which is how ee/lib/api/entities/approval_state.rb
// builds it. Unlike an entry of approved_by it carries no approval time.
type ApproverOutput struct {
	User *toolutil.UserBasicOutput `json:"user,omitempty"`
}

// ApproverGroupOutput is one entry of the deprecated approver_groups list of an
// Enterprise Edition approval state: a group of the merge request's first
// regular rule, under group. The group is the documented reference subset
// [GroupOutput] keeps of Entities::Group.
type ApproverGroupOutput struct {
	Group *GroupOutput `json:"group,omitempty"`
}

// ApprovalRuleShortOutput mirrors ee/lib/api/entities/approval_rule_short.rb,
// the reference an Enterprise Edition approval state gives each rule it names
// in approval_rules_left and invalid_approvers_rules: the rule's ID, name and
// type, and nothing else.
type ApprovalRuleShortOutput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	RuleType string `json:"rule_type"`
}

// enterpriseApprovalCapture is the decoding of a captured approvals answer
// into [EnterpriseApprovalState]. The two timestamps are read as times, so
// they are published in the RFC 3339 form every other timestamp here takes
// rather than in whatever precision GitLab wrote; each shadows the string
// field of the same name in the embedded state, which encoding/json leaves
// alone because the shallower field wins.
type enterpriseApprovalCapture struct {
	EnterpriseApprovalState
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// CapturedEnterpriseState reads the Enterprise Edition approval state off the
// captured answer to an approvals, approve or unapprove request. A Community
// Edition answer carries none of its keys and yields the zero value, which
// publishes nothing. A body that does not decode is an error, since the fault
// is then in the type rather than in the answer.
func CapturedEnterpriseState(capture *gitlabclient.ResponseCapture) (EnterpriseApprovalState, error) {
	var decoded enterpriseApprovalCapture
	if err := capture.Decode(&decoded); err != nil {
		return EnterpriseApprovalState{}, err
	}
	state := decoded.EnterpriseApprovalState
	state.CreatedAt = toolutil.RFC3339Ptr(decoded.CreatedAt)
	state.UpdatedAt = toolutil.RFC3339Ptr(decoded.UpdatedAt)
	return state, nil
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
