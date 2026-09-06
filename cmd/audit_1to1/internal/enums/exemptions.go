package enums

import "maps"

// Shared reasons. A client-go value type is declared once for the whole SDK,
// while GitLab documents what each endpoint accepts, so the same type-wide
// constant is excused on many fields for the same documented reason.
const (
	// membershipNoAdmin: 60 (Admin) is a constant on AccessLevelValue because
	// a protected ref can be restricted to administrators, not because a
	// membership can carry it.
	membershipNoAdmin = "60 (Admin) is not a level a membership can carry: doc/api/project_members.md, group_members.md, invitations.md and access_requests.md list 0 (No access) through 50 (Owner) for adding, editing, inviting and approving; only the group member edit endpoint documents 60, and that field offers it"
	// membershipNoAccessGrant: 0 (No access) is a level a membership reads
	// back with, not one the add and edit endpoints grant; the handlers treat
	// 0 as the field left unset and refuse the request before it leaves.
	membershipNoAccessGrant = "0 (No access) is read back on a membership but never granted: doc/api/members.md lists it under valid access levels while the add and edit handlers (members.go, group_members.go) treat 0 as access_level left unset and refuse the call"
	// memberRoleBaseLevels: a custom role is based on a role a member can
	// hold, which leaves out both ends of AccessLevelValue and the minimal
	// access level in between.
	memberRoleBaseLevels = "doc/api/member_roles.md#add-a-member-role-to-an-instance lists 10 (Guest) through 50 (Owner) as the valid base access levels"
	// broadcastTargetLevels: the roles a banner can target are the membership
	// roles proper, without the two ends of AccessLevelValue.
	broadcastTargetLevels = "doc/api/broadcast_messages.md lists target_access_levels as 10 (Guest), 15 (Planner), 20 (Reporter), 25 (Security Manager), 30 (Developer), 40 (Maintainer) and 50 (Owner); 5 (Minimal access) and 60 (Admin) are AccessLevelValue constants outside that set"
	// minAccessLevelFilter: the list filters document the membership roles a
	// caller can hold, which excludes both ends of the type.
	minAccessLevelFilter = "doc/api/groups.md and projects.md document min_access_level and shared_min_access_level as 5 (Minimal access) through 50 (Owner): 0 (No access) filters nothing and 60 (Admin) is not a membership level"
	// shareAccessLevel covers the two share endpoints, which grant a role.
	shareAccessLevel = "doc/api/groups.md#share-groups-with-groups and projects.md#share-a-project-with-a-group list 5 (Minimal access) through 50 (Owner); 0 grants nothing and 60 (Admin) is not a membership level"
	// protectedBranchLevels is the four-value set every protection level
	// parameter documents, against the membership-wide AccessLevelValue.
	protectedBranchLevels      = "doc/api/protected_branches.md#valid-access-levels lists 0 (No access, push and merge only), 30, 40 and 60 (GitLab Self-Managed only); AccessLevelValue is the membership-wide set, and the other levels are not protection levels"
	groupProtectedBranchLevels = "doc/api/group_protected_branches.md#valid-access-levels lists 0, 30, 40 and 60; the other AccessLevelValue constants are membership roles, not protection levels"
	// featureAccessLevelPublic: AccessControlValue carries public for the one
	// feature whose visibility can exceed the project's.
	featureAccessLevelPublic   = "doc/api/projects.md documents each feature access level as disabled, private or enabled; public is documented for pages_access_level alone, which offers it"
	groupWikiAccessLevelPublic = "doc/api/groups.md documents wiki_access_level as disabled, private or enabled; public is the AccessControlValue constant only project Pages uses" //nolint:gosec // a documentation citation, not a credential
	// deploymentCreated: a deployment is read in that state, never put in it.
	deploymentCreated = "doc/api/deployments.md documents the status a deployment is created with or updated to as running, success, failed or canceled; created is a state a deployment is read in (and the list filter offers it)"
	// The three SDK gaps below are recorded in docs/development/upstream-bugs.md.
	eventActionApproved  = "approved is a documented contribution event action (doc/api/events.md links the contributions calendar page that lists it); client-go's EventTypeValue has no constant for it, see docs/development/upstream-bugs.md"
	eventTargetEpic      = "epic is a documented target_type since GitLab 17.3 (doc/api/events.md); client-go's EventTargetTypeValue has no constant for it, see docs/development/upstream-bugs.md"
	todoActionUndeclared = "doc/api/todos.md#get-a-list-of-to-do-items lists it as a filter value; client-go's TodoAction has no constant for it, see docs/development/upstream-bugs.md"
	// projectCreationOwner: the constant exists upstream, the value is documented nowhere.
	projectCreationOwner = "doc/api/groups.md lists administrator, noone, maintainer and developer for project_creation_level; owner is declared by client-go but documented for neither create nor update"
	reviewerStrategyDAP  = "doc/api/projects.md documents reviewer_assignment_strategy as disabled or code_owners on input; dap_powered can only be read back from projects configured before GitLab 19.4"
)

// acceptedEnumGaps adjudicates the enum values the rule would otherwise
// report. Two key forms:
//   - "<pkg>.<MCPType>.<tag>"          — the whole field: every missing and
//     extra value on it is excused, for a value set the endpoint documents as
//     a subset of the SDK's type-wide constants.
//   - "<pkg>.<MCPType>.<tag>=<value>"  — one value, missing or extra.
//
// Each entry carries a reason that cites the GitLab API doc, and an entry
// that excuses nothing (the field is gone, or the gap closed) is a finding,
// so the table cannot outlive what it describes.
var acceptedEnumGaps = buildAcceptedEnumGaps()

// buildAcceptedEnumGaps assembles the table: the hand-written entries plus
// the one family large enough to spell out programmatically, the project
// feature access levels, which all carry the same documented three-value set.
func buildAcceptedEnumGaps() map[string]string {
	table := map[string]string{
		// --- Membership access levels (AccessLevelValue) ---
		"accessrequests.ApproveGroupInput.access_level=60":   membershipNoAdmin,
		"accessrequests.ApproveProjectInput.access_level=60": membershipNoAdmin,
		"groupmembers.AddInput.access_level=60":              membershipNoAdmin,
		"invites.GroupInvitesInput.access_level=60":          membershipNoAdmin,
		"invites.ProjectInvitesInput.access_level=60":        membershipNoAdmin,
		"members.AddInput.access_level=60":                   membershipNoAdmin,
		"members.EditInput.access_level=60":                  membershipNoAdmin,
		"members.AddInput.access_level=0":                    membershipNoAccessGrant,
		"members.EditInput.access_level=0":                   membershipNoAccessGrant,
		"groupmembers.AddInput.access_level=0":               membershipNoAccessGrant,
		"groupmembers.EditInput.access_level=0":              membershipNoAccessGrant,
		"groupldap.AddInput.group_access=60":                 "doc/api/group_ldap_links.md#add-an-ldap-group-link lists 0 (No access) through 50 (Owner); " + membershipNoAdmin,
		"groupsaml.AddInput.access_level=60":                 "doc/api/saml.md#add-a-saml-group-link lists 0 (No access) through 50 (Owner); " + membershipNoAdmin,

		"memberroles.CreateInstanceInput.base_access_level=0":  memberRoleBaseLevels,
		"memberroles.CreateInstanceInput.base_access_level=5":  memberRoleBaseLevels,
		"memberroles.CreateInstanceInput.base_access_level=60": memberRoleBaseLevels,

		"broadcastmessages.CreateInput.target_access_levels=0":  broadcastTargetLevels,
		"broadcastmessages.CreateInput.target_access_levels=5":  broadcastTargetLevels,
		"broadcastmessages.CreateInput.target_access_levels=60": broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=0":  broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=5":  broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=60": broadcastTargetLevels,

		"groups.InvitedListInput.min_access_level=0":                minAccessLevelFilter,
		"groups.InvitedListInput.min_access_level=60":               minAccessLevelFilter,
		"groups.ListInput.min_access_level=0":                       minAccessLevelFilter,
		"groups.ListInput.min_access_level=60":                      minAccessLevelFilter,
		"groups.ListProjectsInput.min_access_level=0":               minAccessLevelFilter,
		"groups.ListProjectsInput.min_access_level=60":              minAccessLevelFilter,
		"groups.ListSharedProjectsInput.min_access_level=0":         minAccessLevelFilter,
		"groups.ListSharedProjectsInput.min_access_level=60":        minAccessLevelFilter,
		"groups.SharedWithListInput.min_access_level=0":             minAccessLevelFilter,
		"groups.SharedWithListInput.min_access_level=60":            minAccessLevelFilter,
		"groups.SubgroupsListInput.min_access_level=0":              minAccessLevelFilter,
		"projects.ListInvitedGroupsInput.min_access_level=0":        minAccessLevelFilter,
		"projects.ListProjectGroupsInput.shared_min_access_level=0": minAccessLevelFilter,

		"groups.ShareGroupInput.group_access=0":      shareAccessLevel,
		"groups.ShareGroupInput.group_access=60":     shareAccessLevel,
		"projects.ShareProjectInput.group_access=0":  shareAccessLevel,
		"projects.ShareProjectInput.group_access=60": shareAccessLevel,
		// The group share handler admits 10, 20, 30 and 40 and refuses the
		// rest before the request leaves (Share in group_members.go, pinned
		// by its tests), so offering a level it refuses would only move the
		// error later. doc/api/groups.md#share-groups-with-groups lists 5
		// through 50; whether the handler should follow it is a decision
		// about the handler, recorded here rather than made here.
		"groupmembers.ShareInput.group_access": "the handler admits 10, 20, 30 and 40 only and refuses every other level before the request leaves (group_members.go Share, pinned by its tests); doc/api/groups.md#share-groups-with-groups lists 5 through 50",

		// --- Protection levels (AccessLevelValue) ---
		"branches.ProtectInput.push_access_level":                    protectedBranchLevels,
		"branches.ProtectInput.merge_access_level":                   protectedBranchLevels,
		"branches.ProtectInput.unprotect_access_level":               protectedBranchLevels,
		"groupprotectedbranches.ProtectInput.push_access_level":      groupProtectedBranchLevels,
		"groupprotectedbranches.ProtectInput.merge_access_level":     groupProtectedBranchLevels,
		"groupprotectedbranches.ProtectInput.unprotect_access_level": groupProtectedBranchLevels,
		"tags.ProtectTagInput.create_access_level":                   "doc/api/protected_tags.md lists 0 (No access), 30 (Developer) and 40 (Maintainer) as the recognized create access levels; the other AccessLevelValue constants are membership roles",

		// --- Feature visibility (AccessControlValue) ---
		"groups.CreateInput.wiki_access_level=public": groupWikiAccessLevelPublic,
		"groups.UpdateInput.wiki_access_level=public": groupWikiAccessLevelPublic,
		// client-go types this role field with the visibility enum, so the
		// two value sets share nothing.
		"projects.UpdateInput.ci_restrict_pipeline_cancellation_role": "doc/api/projects.md documents developer, maintainer and no_one; client-go types the field as AccessControlValue, whose constants are the feature-visibility set, see docs/development/upstream-bugs.md",

		// --- Documented subsets of other value types ---
		"commits.SetStatusInput.state":                                     "doc/api/commits.md#set-the-pipeline-status-of-a-commit lists pending, running, success, failed, canceled and skipped; BuildStateValue is the full job status set, which includes states a status cannot be set to",
		"deployments.CreateInput.status=created":                           deploymentCreated,
		"deployments.UpdateInput.status=created":                           deploymentCreated,
		"groups.CreateInput.project_creation_level=owner":                  projectCreationOwner,
		"groups.UpdateInput.project_creation_level=owner":                  projectCreationOwner,
		"groups.UpdateInput.shared_runners_setting=disabled_with_override": "doc/api/groups.md#options-for-shared_runners_setting marks disabled_with_override deprecated in favor of disabled_and_overridable, which is offered",
		"projects.CreateInput.reviewer_assignment_strategy=dap_powered":    reviewerStrategyDAP,
		"projects.UpdateInput.reviewer_assignment_strategy=dap_powered":    reviewerStrategyDAP,

		// --- Documented values client-go has no constant for (extras) ---
		"events.ListContributionEventsInput.action=approved":  eventActionApproved,
		"events.ListProjectEventsInput.action=approved":       eventActionApproved,
		"users.ListContributionEventsInput.action=approved":   eventActionApproved,
		"events.ListContributionEventsInput.target_type=epic": eventTargetEpic,
		"events.ListProjectEventsInput.target_type=epic":      eventTargetEpic,
		"users.ListContributionEventsInput.target_type=epic":  eventTargetEpic,
		"todos.ListInput.action=member_access_requested":      todoActionUndeclared,
		"todos.ListInput.action=merge_train_removed":          todoActionUndeclared,
		"todos.ListInput.action=unmergeable":                  todoActionUndeclared,
	}
	maps.Copy(table, projectFeatureAccessLevelExemptions())
	return table
}

// projectFeatureAccessLevels lists the project feature visibility parameters
// that doc/api/projects.md documents as disabled, private or enabled, on both
// the create and the update input. pages_access_level is deliberately absent:
// it is the one feature documented with public, and it offers it.
var projectFeatureAccessLevels = []string{
	"analytics_access_level",
	"builds_access_level",
	"container_registry_access_level",
	"environments_access_level",
	"feature_flags_access_level",
	"forking_access_level",
	"infrastructure_access_level",
	"issues_access_level",
	"merge_requests_access_level",
	"model_experiments_access_level",
	"model_registry_access_level",
	"monitor_access_level",
	"operations_access_level",
	"package_registry_access_level",
	"releases_access_level",
	"repository_access_level",
	"requirements_access_level",
	"security_and_compliance_access_level",
	"snippets_access_level",
	"wiki_access_level",
}

// projectFeatureAccessLevelExemptions excuses the public constant on every
// feature access level of the project create and update inputs.
func projectFeatureAccessLevelExemptions() map[string]string {
	out := make(map[string]string, 2*len(projectFeatureAccessLevels))
	for _, field := range projectFeatureAccessLevels {
		out["projects.CreateInput."+field+"=public"] = featureAccessLevelPublic
		out["projects.UpdateInput."+field+"=public"] = featureAccessLevelPublic
	}
	return out
}
