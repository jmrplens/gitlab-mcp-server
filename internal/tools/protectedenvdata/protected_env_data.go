// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package protectedenvdata

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

// ---------- Input types ----------.

// DeployAccessLevelInput represents an access level for deployment. It
// mirrors gl.EnvironmentAccessOptions and gl.GroupEnvironmentAccessOptions,
// which carry the same fields.
type DeployAccessLevelInput struct {
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Role allowed to deploy: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name exactly one of access_level, user_id or group_id."`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"ID of a user allowed to deploy. Name exactly one of access_level, user_id or group_id."`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"ID of a group whose members may deploy. Name exactly one of access_level, user_id or group_id."`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type for a group_id entry (0=direct members only, 1=inherited members too)"`
}

// EnvironmentAccess answers the entry as the checks of GitLab's rules read
// it.
func (d DeployAccessLevelInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{AccessLevel: d.AccessLevel, UserID: d.UserID, GroupID: d.GroupID, GroupInheritanceType: d.GroupInheritanceType}
}

// ApprovalRuleInput represents an approval rule for an environment. It
// mirrors gl.EnvironmentApprovalRuleOptions and
// gl.GroupEnvironmentApprovalRuleOptions, which carry the same fields, field
// for field but for access_level_description, which neither the protect nor
// the update route declares, so GitLab drops it (upstream register row 139).
type ApprovalRuleInput struct {
	UserID                *int64 `json:"user_id,omitempty"                  jsonschema:"ID of a user who may approve. Name at least one of access_level, user_id or group_id."`
	GroupID               *int64 `json:"group_id,omitempty"                 jsonschema:"ID of a group whose members may approve. Name at least one of access_level, user_id or group_id."`
	AccessLevel           *int   `json:"access_level,omitempty"             jsonschema:"Role whose members may approve: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name at least one of access_level, user_id or group_id."`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"       jsonschema:"Approvals this rule requires, 1 to 5 (GitLab's default is 1)"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty"   jsonschema:"Group inheritance type for a group_id rule (0=direct members only, 1=inherited members too)"`
}

// EnvironmentAccess answers the rule as the checks of GitLab's rules read it.
func (r ApprovalRuleInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{AccessLevel: r.AccessLevel, UserID: r.UserID, GroupID: r.GroupID, GroupInheritanceType: r.GroupInheritanceType, RequiredApprovals: r.RequiredApprovalCount}
}

// UpdateDeployAccessLevelInput represents an updated access level for
// deployment. It mirrors gl.UpdateEnvironmentAccessOptions and
// gl.UpdateGroupEnvironmentAccessOptions, which carry the same fields.
type UpdateDeployAccessLevelInput struct {
	ID                   *int64 `json:"id,omitempty"                     jsonschema:"ID of the existing access level to change or remove. An entry without one adds an access level."`
	AccessLevel          *int   `json:"access_level,omitempty"           jsonschema:"Role allowed to deploy: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name at most one of access_level, user_id or group_id, and one of them without an id."`
	UserID               *int64 `json:"user_id,omitempty"                jsonschema:"ID of a user allowed to deploy. Name at most one of access_level, user_id or group_id, and one of them without an id."`
	GroupID              *int64 `json:"group_id,omitempty"               jsonschema:"ID of a group whose members may deploy. Name at most one of access_level, user_id or group_id, and one of them without an id."`
	GroupInheritanceType *int64 `json:"group_inheritance_type,omitempty" jsonschema:"Group inheritance type for a group_id entry (0=direct members only, 1=inherited members too)"`
	Destroy              *bool  `json:"_destroy,omitempty"               jsonschema:"Set true, with the entry's id, to remove this access level"`
}

// EnvironmentAccess answers the entry as the checks of GitLab's rules read
// it.
func (d UpdateDeployAccessLevelInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{ID: d.ID, AccessLevel: d.AccessLevel, UserID: d.UserID, GroupID: d.GroupID, GroupInheritanceType: d.GroupInheritanceType, Destroy: d.Destroy}
}

// UpdateApprovalRuleInput represents an updated approval rule for an
// environment. It mirrors gl.UpdateEnvironmentApprovalRuleOptions and
// gl.UpdateGroupEnvironmentApprovalRuleOptions, which carry the same fields,
// field for field but for access_level_description, which the update route
// does not declare, so GitLab drops it (upstream register row 139).
type UpdateApprovalRuleInput struct {
	ID                    *int64 `json:"id,omitempty"                       jsonschema:"ID of the existing approval rule to change or remove. A rule without one adds a rule."`
	UserID                *int64 `json:"user_id,omitempty"                  jsonschema:"ID of a user who may approve. Name at least one of access_level, user_id or group_id unless _destroy is true, on an existing rule as well."`
	GroupID               *int64 `json:"group_id,omitempty"                 jsonschema:"ID of a group whose members may approve. Name at least one of access_level, user_id or group_id unless _destroy is true, on an existing rule as well."`
	AccessLevel           *int   `json:"access_level,omitempty"             jsonschema:"Role whose members may approve: 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin). Name at least one of access_level, user_id or group_id unless _destroy is true, on an existing rule as well."`
	RequiredApprovalCount *int64 `json:"required_approvals,omitempty"       jsonschema:"Approvals this rule requires, 1 to 5 unless _destroy is true"`
	GroupInheritanceType  *int64 `json:"group_inheritance_type,omitempty"   jsonschema:"Group inheritance type for a group_id rule (0=direct members only, 1=inherited members too)"`
	Destroy               *bool  `json:"_destroy,omitempty"                 jsonschema:"Set true, with the rule's id, to remove this rule"`
}

// EnvironmentAccess answers the rule as the checks of GitLab's rules read it.
func (r UpdateApprovalRuleInput) EnvironmentAccess() toolutil.EnvironmentAccess {
	return toolutil.EnvironmentAccess{ID: r.ID, AccessLevel: r.AccessLevel, UserID: r.UserID, GroupID: r.GroupID, GroupInheritanceType: r.GroupInheritanceType, RequiredApprovals: r.RequiredApprovalCount, Destroy: r.Destroy}
}

// ---------- Output rows ----------.

// AccessLevel represents a deploy access level on a protected environment, as
// gl.EnvironmentAccessDescription and gl.GroupEnvironmentAccessDescription
// both decode it.
//
// It and [ApprovalRule] are named without the Output suffix because neither
// is a response on its own: each is a row of the protected environment a
// project or a group route answers with, and is published there under the
// name its package gives it, AccessLevelOutput and ApprovalRuleOutput. The
// request-path audit reads a type named for an output as a response.
type AccessLevel struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	GroupInheritanceType   int64  `json:"group_inheritance_type,omitempty"`
}

// ApprovalRule represents an approval rule on a protected environment, as
// gl.EnvironmentApprovalRule and gl.GroupEnvironmentApprovalRule both decode
// it.
type ApprovalRule struct {
	ID                     int64  `json:"id"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	RequiredApprovalCount  int64  `json:"required_approvals"`
	GroupInheritanceType   int64  `json:"group_inheritance_type,omitempty"`
}
