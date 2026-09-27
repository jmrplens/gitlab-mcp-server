package branchrules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// BranchRuleItem represents a branch rule summary.
//
// A field the Community document cannot select is a pointer or a slice that
// stays empty on a Community instance, so a caller told nothing about code
// owners reads the field as absent rather than as false. The tier tags name
// the licensed feature each field reports on; GitLab's schema declares none,
// so they come from the feature table the GitLab release licenses
// (ee/app/models/gitlab_subscriptions/features.rb).
type BranchRuleItem struct {
	ID                    string                `json:"id,omitempty"`
	Name                  string                `json:"name"`
	IsDefault             bool                  `json:"is_default"`
	IsProtected           bool                  `json:"is_protected"`
	MatchingBranchesCount int                   `json:"matching_branches_count"`
	CreatedAt             string                `json:"created_at,omitempty"`
	UpdatedAt             string                `json:"updated_at,omitempty"`
	BranchProtection      *BranchProtection     `json:"branch_protection,omitempty"`
	ApprovalRules         []ApprovalRule        `json:"approval_rules,omitempty" tier:"premium"`
	ExternalStatusChecks  []ExternalStatusCheck `json:"external_status_checks,omitempty" tier:"ultimate"`
}

// BranchProtection holds protection settings for a branch rule: who may push,
// merge and unprotect, and the flags that decide what a push may do. The four
// security-policy flags are absent rather than false on a release older than
// 18.8, which is not asked about them (see [List]).
type BranchProtection struct {
	AllowForcePush                        bool         `json:"allow_force_push"`
	CodeOwnerApprovalRequired             *bool        `json:"code_owner_approval_required,omitempty" tier:"premium"`
	ModificationBlockedByPolicy           *bool        `json:"modification_blocked_by_policy,omitempty" tier:"ultimate"`
	ProtectedFromPushBySecurityPolicy     *bool        `json:"protected_from_push_by_security_policy,omitempty" tier:"ultimate"`
	WarnModificationBlockedByPolicy       *bool        `json:"warn_modification_blocked_by_policy,omitempty" tier:"ultimate"`
	WarnProtectedFromPushBySecurityPolicy *bool        `json:"warn_protected_from_push_by_security_policy,omitempty" tier:"ultimate"`
	PushAccessLevels                      []PushAccess `json:"push_access_levels,omitempty"`
	MergeAccessLevels                     []Access     `json:"merge_access_levels,omitempty"`
	UnprotectAccessLevels                 []Access     `json:"unprotect_access_levels,omitempty" tier:"premium"`
}

// Access is one grant on a protected branch: a role, and on Premium and above
// a specific user or group, allowed to merge into, push to or unprotect the
// branches the rule matches. The description is GitLab's own reading of the
// grant: the role's name, or the user's or the group's.
type Access struct {
	AccessLevel            int          `json:"access_level"`
	AccessLevelDescription string       `json:"access_level_description"`
	User                   *UserRef     `json:"user,omitempty" tier:"premium"`
	Group                  *AccessGroup `json:"group,omitempty" tier:"premium"`
}

// PushAccess is a push grant, the one kind that may name a deploy key.
type PushAccess struct {
	Access
	DeployKey *AccessDeployKey `json:"deploy_key,omitempty"`
}

// UserRef identifies a user a grant, a deploy key or an approval rule names.
// The id is the user's numeric id as a string wherever the user appears: a
// grant's user and a deploy key's user carry it bare, and an eligible
// approver, which GitLab identifies by a global ID (gid://gitlab/User/21), is
// read back to the same number, so one user matches itself across the
// response.
type UserRef struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	PublicEmail string `json:"public_email,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	WebURL      string `json:"web_url"`
	WebPath     string `json:"web_path"`
}

// AccessGroup identifies a group a grant names. Its parent is not selected:
// the chain is recursive, so a document could only select it to a depth fixed
// in advance, and the group's web URL already spells its whole path.
type AccessGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	WebURL    string `json:"web_url"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// AccessDeployKey is the deploy key a push grant names, with the user it is
// assigned to.
type AccessDeployKey struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	ExpiresAt string  `json:"expires_at,omitempty"`
	User      UserRef `json:"user"`
}

// ApprovalRule represents an approval rule associated with a branch rule.
type ApprovalRule struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	ApprovalsRequired int       `json:"approvals_required"`
	Type              string    `json:"type,omitempty"`
	EligibleApprovers []UserRef `json:"eligible_approvers,omitempty"`
}

// ExternalStatusCheck represents an external status check on a branch rule.
// Whether an HMAC secret signs its requests is absent rather than false on a
// release older than 18.8, which is not asked (see [List]).
type ExternalStatusCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ExternalURL string `json:"external_url"`
	HMAC        *bool  `json:"hmac,omitempty"`
}

// GraphQL query.
//
// The selections a document repeats are spelled once below and concatenated,
// so the three grant lists and the two user positions cannot drift apart.
// Every Enterprise field is one GitLab adds in ee/, so on a Community instance
// the whole document would be refused rather than the field answered empty:
// that is why the Community document is a document of its own.
//
// The same refusal makes the newest field a document selects the oldest
// release it works on. Measured against GitLab's versioned GraphQL references
// (16.11, the oldest read, to 19.4), every field the Community document and
// the base Enterprise document select is served from 16.11, while five the
// Enterprise document adds are newer: an external status check's hmac (17.3),
// modificationBlockedByPolicy (18.0), protectedFromPushBySecurityPolicy (18.7)
// and the two warn-mode flags (18.8). A licensed instance is therefore sent
// the 18.8 document first and the base one when it refuses a field it does not
// have (see [List]), so the listing works from 16.11 on every edition and a
// release from 18.8 reports all five. One from 17.3 to 18.7 is answered
// without the one or two of them it already serves, the price of two
// Enterprise documents rather than one per release that added a field.
//
// The fields GitLab marks as experiments (the rule's and the protection's
// isGroupLevel, the rule's squashOption, a grant's memberRole) are left out,
// since GitLab may change or remove an experiment and would then refuse every
// call, and so is the approval rule's coverageMinimumThreshold, added in 19.2,
// which would move every release from 18.8 to 19.1 onto the base document for
// one number. cmd/audit_graphql_shapes declares each.
//
// GitLab also charges each document a complexity it refuses above 250 for an
// authenticated caller, and branchRules multiplies what one rule costs by the
// page size. Measured on GitLab.com on 2026-09-26, the Enterprise document
// scores 137 at the default page of 20 and 226 at the largest of 100, the base
// Enterprise one 126 and 208, and the Community one 49 and 80. Selecting each
// group's parent as well cost 256 at a page of 100, which GitLab refuses, and
// is why no grant selects it.

// userRefSelection is every field AccessLevelUser offers, which is also what a
// user reference is published as wherever this package names one.
const userRefSelection = `id
            username
            name
            publicEmail
            avatarUrl
            webUrl
            webPath`

// accessGroupSelection is every field AccessLevelGroup offers but its parent.
const accessGroupSelection = `id
              name
              webUrl
              avatarUrl`

// accessLevelSelectionCE is what every edition reports about a grant.
const accessLevelSelectionCE = `accessLevel
              accessLevelDescription`

// accessLevelSelection is a grant as the Enterprise document selects it,
// with the user or group it names.
const accessLevelSelection = accessLevelSelectionCE + `
              user {
                ` + userRefSelection + `
              }
              group {
                ` + accessGroupSelection + `
              }`

// deployKeySelection is the deploy key a push grant may name.
const deployKeySelection = `deployKey {
                id
                title
                expiresAt
                user {
                  ` + userRefSelection + `
                }
              }`

// grantListsSelection is the three grant lists of a protection as both
// Enterprise documents select them.
const grantListsSelection = `pushAccessLevels {
            nodes {
              ` + accessLevelSelection + `
              ` + deployKeySelection + `
            }
          }
          mergeAccessLevels {
            nodes {
              ` + accessLevelSelection + `
            }
          }
          unprotectAccessLevels {
            nodes {
              ` + accessLevelSelection + `
            }
          }`

// approvalRulesSelection is a rule's approval rules with the users each lets
// approve, as both Enterprise documents select them.
const approvalRulesSelection = `approvalRules {
          nodes {
            id
            name
            approvalsRequired
            type
            eligibleApprovers {
              nodes {
                ` + userRefSelection + `
              }
            }
          }
        }`

// queryListBranchRulesEE includes Enterprise-only fields (codeOwnerApprovalRequired,
// approvalRules, externalStatusChecks, the security-policy flags). Used when the
// resolved tier is Premium or Ultimate (GITLAB_MCP_TIER=premium/ultimate or a
// detected EE license), on GitLab 18.8 and later.
const queryListBranchRulesEE = `
query($projectPath: ID!, $first: Int!, $after: String) {
  project(fullPath: $projectPath) {
    branchRules(first: $first, after: $after) {
      nodes {
        id
        name
        isDefault
        isProtected
        matchingBranchesCount
        createdAt
        updatedAt
        branchProtection {
          allowForcePush
          codeOwnerApprovalRequired
          modificationBlockedByPolicy
          protectedFromPushBySecurityPolicy
          warnModificationBlockedByPolicy
          warnProtectedFromPushBySecurityPolicy
          ` + grantListsSelection + `
        }
        ` + approvalRulesSelection + `
        externalStatusChecks {
          nodes {
            id
            name
            externalUrl
            hmac
          }
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

// queryListBranchRulesEEBase is the Enterprise document without the five
// fields GitLab added after 16.11: the four security-policy flags and a status
// check's hmac. A licensed instance is sent it only once it has refused
// queryListBranchRulesEE for naming a field it does not have.
const queryListBranchRulesEEBase = `
query($projectPath: ID!, $first: Int!, $after: String) {
  project(fullPath: $projectPath) {
    branchRules(first: $first, after: $after) {
      nodes {
        id
        name
        isDefault
        isProtected
        matchingBranchesCount
        createdAt
        updatedAt
        branchProtection {
          allowForcePush
          codeOwnerApprovalRequired
          ` + grantListsSelection + `
        }
        ` + approvalRulesSelection + `
        externalStatusChecks {
          nodes {
            id
            name
            externalUrl
          }
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

// queryListBranchRulesCE uses only fields available on GitLab Community Edition.
const queryListBranchRulesCE = `
query($projectPath: ID!, $first: Int!, $after: String) {
  project(fullPath: $projectPath) {
    branchRules(first: $first, after: $after) {
      nodes {
        id
        name
        isDefault
        isProtected
        matchingBranchesCount
        createdAt
        updatedAt
        branchProtection {
          allowForcePush
          pushAccessLevels {
            nodes {
              ` + accessLevelSelectionCE + `
              ` + deployKeySelection + `
            }
          }
          mergeAccessLevels {
            nodes {
              ` + accessLevelSelectionCE + `
            }
          }
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

// GraphQL response structs.
//
// The two documents select two shapes, so there are two node types, one per
// document, rather than one node carrying fields the Community document can
// never fill: a decoder declaring a field its document does not select holds
// a value that is always empty, which make check-graphql-shapes refuses. The
// same holds one level down, which is why a grant has a Community and an
// Enterprise decoder too.

// gqlNodes is a connection read through its nodes alone: the lists a branch
// rule carries are short, and none of them is paged by this action.
type gqlNodes[T any] struct {
	Nodes []T `json:"nodes"`
}

// gqlUserRef decodes a user reference: an AccessLevelUser, or an approval
// rule's eligible approver, which is a UserCore selected for the same fields.
type gqlUserRef struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	PublicEmail string `json:"publicEmail"`
	AvatarURL   string `json:"avatarUrl"`
	WebURL      string `json:"webUrl"`
	WebPath     string `json:"webPath"`
}

// gqlAccessGroup decodes a group a grant names.
type gqlAccessGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	WebURL    string `json:"webUrl"`
	AvatarURL string `json:"avatarUrl"`
}

// gqlDeployKey decodes the deploy key a push grant names.
type gqlDeployKey struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	ExpiresAt string     `json:"expiresAt"`
	User      gqlUserRef `json:"user"`
}

// gqlAccessLevelCE is a grant as every edition reports it.
type gqlAccessLevelCE struct {
	AccessLevel            int    `json:"accessLevel"`
	AccessLevelDescription string `json:"accessLevelDescription"`
}

// gqlPushAccessLevelCE is a Community push grant, which may name a deploy key.
type gqlPushAccessLevelCE struct {
	gqlAccessLevelCE
	DeployKey *gqlDeployKey `json:"deployKey"`
}

// gqlAccessLevel is a grant as the Enterprise document selects it.
type gqlAccessLevel struct {
	gqlAccessLevelCE
	User  *gqlUserRef     `json:"user"`
	Group *gqlAccessGroup `json:"group"`
}

// gqlPushAccessLevel is an Enterprise push grant.
type gqlPushAccessLevel struct {
	gqlAccessLevel
	DeployKey *gqlDeployKey `json:"deployKey"`
}

// gqlBranchProtectionCE is the protection every edition reports.
type gqlBranchProtectionCE struct {
	AllowForcePush    bool                            `json:"allowForcePush"`
	PushAccessLevels  *gqlNodes[gqlPushAccessLevelCE] `json:"pushAccessLevels"`
	MergeAccessLevels *gqlNodes[gqlAccessLevelCE]     `json:"mergeAccessLevels"`
}

// gqlBranchProtectionEEBase is the protection both Enterprise documents
// select, which every release from 16.11 serves.
type gqlBranchProtectionEEBase struct {
	AllowForcePush            bool                          `json:"allowForcePush"`
	CodeOwnerApprovalRequired bool                          `json:"codeOwnerApprovalRequired"`
	PushAccessLevels          *gqlNodes[gqlPushAccessLevel] `json:"pushAccessLevels"`
	MergeAccessLevels         *gqlNodes[gqlAccessLevel]     `json:"mergeAccessLevels"`
	UnprotectAccessLevels     *gqlNodes[gqlAccessLevel]     `json:"unprotectAccessLevels"`
}

// gqlBranchProtection is the protection the 18.8 Enterprise document selects:
// the base one with the security-policy flags.
type gqlBranchProtection struct {
	gqlBranchProtectionEEBase
	ModificationBlockedByPolicy           bool `json:"modificationBlockedByPolicy"`
	ProtectedFromPushBySecurityPolicy     bool `json:"protectedFromPushBySecurityPolicy"`
	WarnModificationBlockedByPolicy       bool `json:"warnModificationBlockedByPolicy"`
	WarnProtectedFromPushBySecurityPolicy bool `json:"warnProtectedFromPushBySecurityPolicy"`
}

type gqlApprovalRule struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	ApprovalsRequired int                   `json:"approvalsRequired"`
	Type              *string               `json:"type"`
	EligibleApprovers *gqlNodes[gqlUserRef] `json:"eligibleApprovers"`
}

// gqlExternalStatusCheckBase is a status check as both Enterprise documents
// select it.
type gqlExternalStatusCheckBase struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ExternalURL string `json:"externalUrl"`
}

// gqlExternalStatusCheck is a status check as the 18.8 Enterprise document
// selects it, with whether an HMAC secret signs its requests.
type gqlExternalStatusCheck struct {
	gqlExternalStatusCheckBase
	HMAC bool `json:"hmac"`
}

// gqlBranchRuleFields are the fields both documents select.
type gqlBranchRuleFields struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	IsDefault             bool    `json:"isDefault"`
	IsProtected           bool    `json:"isProtected"`
	MatchingBranchesCount int     `json:"matchingBranchesCount"`
	CreatedAt             *string `json:"createdAt"`
	UpdatedAt             *string `json:"updatedAt"`
}

// gqlBranchRuleNodeCE is a branch rule as the Community document selects it.
type gqlBranchRuleNodeCE struct {
	gqlBranchRuleFields
	BranchProtection *gqlBranchProtectionCE `json:"branchProtection"`
}

// gqlBranchRuleNodeEEBase is a branch rule as the base Enterprise document
// selects it.
type gqlBranchRuleNodeEEBase struct {
	gqlBranchRuleFields
	BranchProtection     *gqlBranchProtectionEEBase            `json:"branchProtection"`
	ApprovalRules        *gqlNodes[gqlApprovalRule]            `json:"approvalRules"`
	ExternalStatusChecks *gqlNodes[gqlExternalStatusCheckBase] `json:"externalStatusChecks"`
}

// gqlBranchRuleNode is a branch rule as the 18.8 Enterprise document selects
// it.
type gqlBranchRuleNode struct {
	gqlBranchRuleFields
	BranchProtection     *gqlBranchProtection              `json:"branchProtection"`
	ApprovalRules        *gqlNodes[gqlApprovalRule]        `json:"approvalRules"`
	ExternalStatusChecks *gqlNodes[gqlExternalStatusCheck] `json:"externalStatusChecks"`
}

// branchRuleConverter is what the list decodes a node as: either edition's
// shape, each knowing how to become the one output item. It is named for the
// role it fills rather than for the shape it admits, which is the convention
// the rest of this repository follows for an interface with one method
// (promptAdder, errorReporter, modelProvider); the previous spelling named the
// shape, and a reader met a constraint whose name did not say what the generic
// code was going to do with it.
type branchRuleConverter interface {
	item() BranchRuleItem
}

// convertNodes converts the nodes of a connection, answering nil for one
// GitLab sent as null. An empty connection converts to an empty list, which
// every list field of the output omits just as it omits nil.
func convertNodes[T, R any](connection *gqlNodes[T], convert func(T) R) []R {
	if connection == nil {
		return nil
	}
	out := make([]R, 0, len(connection.Nodes))
	for _, node := range connection.Nodes {
		out = append(out, convert(node))
	}
	return out
}

// userRef converts a decoded user reference.
func (u gqlUserRef) userRef() UserRef {
	return UserRef(u)
}

// approver converts an eligible approver. It is a UserCore, whose id GitLab
// sends as a global ID (gid://gitlab/User/21) where an access level's user
// carries the bare numeric id, so it is read back to that number and the same
// user is published with the same id wherever a branch rule names it. An id
// that is not a user global ID is published as GitLab sent it rather than
// dropped.
func (u gqlUserRef) approver() UserRef {
	ref := u.userRef()
	if _, id, err := toolutil.ParseGID(u.ID); err == nil {
		ref.ID = strconv.FormatInt(id, 10)
	}
	return ref
}

// access converts a grant as every edition reports it.
func (a gqlAccessLevelCE) access() Access {
	return Access{
		AccessLevel:            a.AccessLevel,
		AccessLevelDescription: a.AccessLevelDescription,
	}
}

// deployKey converts the deploy key a push grant names, or nil for one that
// names none.
func deployKey(key *gqlDeployKey) *AccessDeployKey {
	if key == nil {
		return nil
	}
	return &AccessDeployKey{
		ID:        key.ID,
		Title:     key.Title,
		ExpiresAt: key.ExpiresAt,
		User:      key.User.userRef(),
	}
}

// pushAccess converts a Community push grant.
func (a gqlPushAccessLevelCE) pushAccess() PushAccess {
	return PushAccess{Access: a.access(), DeployKey: deployKey(a.DeployKey)}
}

// access converts an Enterprise grant with the user or group it names.
func (a gqlAccessLevel) access() Access {
	out := a.gqlAccessLevelCE.access()
	if a.User != nil {
		user := a.User.userRef()
		out.User = &user
	}
	if a.Group != nil {
		group := AccessGroup(*a.Group)
		out.Group = &group
	}
	return out
}

// pushAccess converts an Enterprise push grant.
func (a gqlPushAccessLevel) pushAccess() PushAccess {
	return PushAccess{Access: a.access(), DeployKey: deployKey(a.DeployKey)}
}

// protection converts the protection every edition reports.
func (p gqlBranchProtectionCE) protection() *BranchProtection {
	return &BranchProtection{
		AllowForcePush:    p.AllowForcePush,
		PushAccessLevels:  convertNodes(p.PushAccessLevels, gqlPushAccessLevelCE.pushAccess),
		MergeAccessLevels: convertNodes(p.MergeAccessLevels, gqlAccessLevelCE.access),
	}
}

// protection converts the protection both Enterprise documents select.
func (p gqlBranchProtectionEEBase) protection() *BranchProtection {
	return &BranchProtection{
		AllowForcePush:            p.AllowForcePush,
		CodeOwnerApprovalRequired: new(p.CodeOwnerApprovalRequired),
		PushAccessLevels:          convertNodes(p.PushAccessLevels, gqlPushAccessLevel.pushAccess),
		MergeAccessLevels:         convertNodes(p.MergeAccessLevels, gqlAccessLevel.access),
		UnprotectAccessLevels:     convertNodes(p.UnprotectAccessLevels, gqlAccessLevel.access),
	}
}

// protection converts the protection the 18.8 Enterprise document selects,
// with its security-policy flags.
func (p gqlBranchProtection) protection() *BranchProtection {
	out := p.gqlBranchProtectionEEBase.protection()
	out.ModificationBlockedByPolicy = new(p.ModificationBlockedByPolicy)
	out.ProtectedFromPushBySecurityPolicy = new(p.ProtectedFromPushBySecurityPolicy)
	out.WarnModificationBlockedByPolicy = new(p.WarnModificationBlockedByPolicy)
	out.WarnProtectedFromPushBySecurityPolicy = new(p.WarnProtectedFromPushBySecurityPolicy)
	return out
}

// approvalRule converts an approval rule with its eligible approvers.
func (ar gqlApprovalRule) approvalRule() ApprovalRule {
	rule := ApprovalRule{
		ID:                ar.ID,
		Name:              ar.Name,
		ApprovalsRequired: ar.ApprovalsRequired,
		EligibleApprovers: convertNodes(ar.EligibleApprovers, gqlUserRef.approver),
	}
	if ar.Type != nil {
		rule.Type = *ar.Type
	}
	return rule
}

// statusCheck converts an external status check as both Enterprise documents
// select it.
func (esc gqlExternalStatusCheckBase) statusCheck() ExternalStatusCheck {
	return ExternalStatusCheck{ID: esc.ID, Name: esc.Name, ExternalURL: esc.ExternalURL}
}

// statusCheck converts an external status check with its HMAC flag.
func (esc gqlExternalStatusCheck) statusCheck() ExternalStatusCheck {
	check := esc.gqlExternalStatusCheckBase.statusCheck()
	check.HMAC = new(esc.HMAC)
	return check
}

// item converts the fields both editions select into a [BranchRuleItem],
// extracting the timestamps.
func (n gqlBranchRuleFields) item() BranchRuleItem {
	item := BranchRuleItem{
		ID:                    n.ID,
		Name:                  n.Name,
		IsDefault:             n.IsDefault,
		IsProtected:           n.IsProtected,
		MatchingBranchesCount: n.MatchingBranchesCount,
	}
	if n.CreatedAt != nil {
		item.CreatedAt = *n.CreatedAt
	}
	if n.UpdatedAt != nil {
		item.UpdatedAt = *n.UpdatedAt
	}
	return item
}

// item converts a Community node into a [BranchRuleItem].
func (n gqlBranchRuleNodeCE) item() BranchRuleItem {
	item := n.gqlBranchRuleFields.item()
	if n.BranchProtection != nil {
		item.BranchProtection = n.BranchProtection.protection()
	}
	return item
}

// item converts a node of the base Enterprise document into a
// [BranchRuleItem], with its approval rules and external status checks.
func (n gqlBranchRuleNodeEEBase) item() BranchRuleItem {
	item := n.gqlBranchRuleFields.item()
	if n.BranchProtection != nil {
		item.BranchProtection = n.BranchProtection.protection()
	}
	item.ApprovalRules = convertNodes(n.ApprovalRules, gqlApprovalRule.approvalRule)
	item.ExternalStatusChecks = convertNodes(n.ExternalStatusChecks, gqlExternalStatusCheckBase.statusCheck)
	return item
}

// item converts a node of the 18.8 Enterprise document into a
// [BranchRuleItem], with its approval rules and external status checks.
func (n gqlBranchRuleNode) item() BranchRuleItem {
	item := n.gqlBranchRuleFields.item()
	if n.BranchProtection != nil {
		item.BranchProtection = n.BranchProtection.protection()
	}
	item.ApprovalRules = convertNodes(n.ApprovalRules, gqlApprovalRule.approvalRule)
	item.ExternalStatusChecks = convertNodes(n.ExternalStatusChecks, gqlExternalStatusCheck.statusCheck)
	return item
}

// List.

// ListInput is the input for listing branch rules.
type ListInput struct {
	ProjectPath string `json:"project_path" jsonschema:"required,Project full path (e.g. my-group/my-project)"`
	toolutil.GraphQLPaginationInput
}

// ListOutput is the output for listing branch rules.
//
// Pagination is forward-only because Project.branchRules is: it accepts first
// and after alone, and reports neither a previous page nor a start cursor.
type ListOutput struct {
	toolutil.HintableOutput
	Rules      []BranchRuleItem                        `json:"rules"`
	Pagination toolutil.GraphQLForwardPaginationOutput `json:"pagination"`
}

// errFieldNotServed marks a document GitLab refused for naming a field the
// instance does not have, which is how a release older than a document's
// newest field answers it.
var errFieldNotServed = errors.New("this GitLab release does not have a field the document selects")

// undefinedFieldCode is the code GitLab's GraphQL validation gives a field the
// type it is selected on does not define (graphql-ruby's
// FieldsAreDefinedOnTypeError; GitLab.com answers it with HTTP 200).
const undefinedFieldCode = "undefinedField"

// List retrieves branch rules for a project via the GitLab GraphQL API.
// It selects the EE query (with approval rules, external status checks, and
// code owner approval) when the client is configured for Enterprise, otherwise
// it uses the CE-compatible query that omits EE-only fields.
//
// A licensed instance is sent the 18.8 Enterprise document first, and the base
// one when it refuses a field the first names: that is how a release from 16.11
// to 18.7 answers, and it answers before running anything, so a retry repeats
// no work GitLab did. Holding every licensed instance to the base document
// instead would leave the four security-policy flags and a status check's hmac
// unread on every release that serves them, and holding it to the 18.8 one
// alone would refuse the whole listing, grants included, below 18.8. A
// refusal of any other kind is reported as it is.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectPath == "" {
		return ListOutput{}, errors.New("list_branch_rules: project_path is required")
	}

	if !client.IsEnterprise() {
		return listWith[gqlBranchRuleNodeCE](ctx, client, queryListBranchRulesCE, input)
	}
	out, err := listWith[gqlBranchRuleNode](ctx, client, queryListBranchRulesEE, input)
	if errors.Is(err, errFieldNotServed) {
		return listWith[gqlBranchRuleNodeEEBase](ctx, client, queryListBranchRulesEEBase, input)
	}
	return out, err
}

// listWith runs one branch rules document against the variables the input
// resolves to, decoding each node as N, the shape that document selects.
//
// The document is a parameter rather than picked here so that a test can hand
// it one declaring too little and prove the pagination guard refuses it. The
// alternative, a package-level variable a test reassigns, would put a document
// under a parallel neighbor's feet, and the race detector would report it as a
// data race rather than as this guard.
//
// Building the variables here rather than in the caller is what keeps the check
// honest: the pair is checked against the document this call will actually
// send, not against the one a tier decision happened to pick first.
func listWith[N branchRuleConverter](ctx context.Context, client *gitlabclient.Client, query string, input ListInput) (ListOutput, error) {
	vars, err := input.Variables(query)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_branch_rules: %w", err)
	}
	vars["projectPath"] = input.ProjectPath

	return doGraphQLList[N](ctx, client, query, vars, input.ProjectPath)
}

// gqlBranchRulesConnection holds the paginated list of branch rule nodes.
//
// Pagination is the forward half alone, because both documents select that
// half alone: Project.branchRules accepts first and after and nothing else.
type gqlBranchRulesConnection[N branchRuleConverter] struct {
	Nodes    []N                                `json:"nodes"`
	PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
}

// gqlProjectBranchRules wraps the branch rules connection inside a project.
type gqlProjectBranchRules[N branchRuleConverter] struct {
	BranchRules gqlBranchRulesConnection[N] `json:"branchRules"`
}

// gqlResponse is the GraphQL response envelope for branch rules, with each
// node decoded as N.
type gqlResponse[N branchRuleConverter] struct {
	Data struct {
		Project *gqlProjectBranchRules[N] `json:"project"`
	} `json:"data"`
	Errors []gqlError `json:"errors"`
}

// gqlError is a top-level GraphQL error with the code GitLab classifies it by,
// which is what tells a field the instance does not have from any other
// refusal without reading the message.
type gqlError struct {
	Message    string `json:"message"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

// namesUndefinedField reports whether GitLab refused the document for naming a
// field it does not define.
func (e gqlError) namesUndefinedField() bool {
	return e.Extensions.Code == undefinedFieldCode
}

// refusal reads the top-level errors GitLab answered a document with, marking
// the error with [errFieldNotServed] when one of them is a field the instance
// does not have, and answers nil when there are none.
func refusal(responseErrors []gqlError) error {
	messages := make([]toolutil.GraphQLError, 0, len(responseErrors))
	for _, responseError := range responseErrors {
		messages = append(messages, toolutil.GraphQLError{Message: responseError.Message})
	}
	err := toolutil.GraphQLTopLevelError("list_branch_rules", messages)
	if slices.ContainsFunc(responseErrors, gqlError.namesUndefinedField) {
		return fmt.Errorf("%w (%w)", err, errFieldNotServed)
	}
	return err
}

// doGraphQLList executes a branch rules GraphQL query and returns the output.
func doGraphQLList[N branchRuleConverter](ctx context.Context, client *gitlabclient.Client, query string, vars map[string]any, projectPath string) (ListOutput, error) {
	var resp gqlResponse[N]

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list_branch_rules", err, "verify the project fullPath is correct and your token has read_api scope")
	}

	// GitLab answers a rejected document with HTTP 200 and a top-level errors
	// array, which client-go does not turn into an error, so a query the
	// instance refused would otherwise be reported as a missing project.
	if resp.Data.Project == nil {
		if graphQLErr := refusal(resp.Errors); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
		return ListOutput{}, fmt.Errorf("list_branch_rules: project %q not found", projectPath)
	}

	items := make([]BranchRuleItem, 0, len(resp.Data.Project.BranchRules.Nodes))
	for _, n := range resp.Data.Project.BranchRules.Nodes {
		items = append(items, n.item())
	}

	return ListOutput{
		Rules:      items,
		Pagination: toolutil.ForwardPageInfoToOutput(resp.Data.Project.BranchRules.PageInfo),
	}, nil
}
