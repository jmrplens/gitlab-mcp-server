// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package enums

import (
	"maps"
	"slices"
)

// Shared reasons. A client-go value type is declared once for the whole SDK,
// while GitLab documents what each endpoint accepts, so the same type-wide
// constant is excused on many fields for the same documented reason.
const (
	// The membership reasons below cite the routes and models at v19.5.0-ee,
	// which register row 110 of docs/development/upstream-bugs.md records,
	// rather than the API pages, which listed levels the routes refuse until
	// the documentation change that row names. The schemas offer what
	// toolutil.AccessLevelSet holds (issue 1280).
	//
	// membershipNoAdmin: 60 (Admin) is a constant on AccessLevelValue because
	// a protected ref can be restricted to administrators, not because a
	// membership can carry it.
	membershipNoAdmin = "60 (Admin) is not a level a membership can carry: no member, invitation, access request, share or group link route accepts it at v19.5.0-ee, the group member update included (docs/development/upstream-bugs.md row 110)"
	// membershipNoAccessGrant: 0 (No access) is a level a membership reads
	// back with, not one any write grants.
	membershipNoAccessGrant = "0 (No access) is read back on a membership but never granted: no write route accepts it at v19.5.0-ee, and an access request approval answers it with 201 while the request stays pending, so the approvals refuse it before the request (docs/development/upstream-bugs.md row 110)"
	// projectNoMinimalAccess: Minimal access is a group level.
	projectNoMinimalAccess = "5 (Minimal access) is refused on a project in every edition at v19.5.0-ee: a project member, invitation, share and approval are held to Gitlab::Access.all_values, Guest (10) to Owner (50) (docs/development/upstream-bugs.md row 110)"
	// groupLinkNoAccess: a SAML or LDAP group link grants a role.
	groupLinkNoAccess = "the SAML and LDAP group link routes declare values_with_minimal_access at v19.5.0-ee, Minimal access (5) to Owner (50), so 0 (No access) is refused though doc/api/saml.md and group_ldap_links.md list it (docs/development/upstream-bugs.md row 110)"
	// memberRoleBaseLevels: a custom role is based on a role a member can
	// hold, below Owner.
	memberRoleBaseLevels = "the create route declares Gitlab::Access.all_values and MemberRole validates the level against options_for_custom_roles at v19.5.0-ee, so Guest (10) to Maintainer (40) is what both accept: 0, 5 (Minimal access), 50 (Owner) and 60 (Admin) are each refused by one of the two, though doc/api/member_roles.md lists 50 (docs/development/upstream-bugs.md row 110)"
	// broadcastTargetLevels: the roles a banner can target.
	broadcastTargetLevels = "the routes declare target_access_levels with values: System::BroadcastMessage::ALLOWED_TARGET_ACCESS_LEVELS at v19.5.0-ee, Guest, Planner, Reporter, Developer, Maintainer and Owner; 0, 5 (Minimal access), 25 (Security Manager) and 60 (Admin) are AccessLevelValue constants outside it, though doc/api/broadcast_messages.md lists 25 (docs/development/upstream-bugs.md row 110)"
	// minAccessLevelFilter: the list filters take the membership roles a
	// caller can hold.
	minAccessLevelFilter = "every min_access_level and shared_min_access_level route declares values: Gitlab::Access.all_values at v19.5.0-ee, Guest (10) to Owner (50) in every edition: 0 filters nothing, 5 (Minimal access) is refused and 60 (Admin) is not a membership level (docs/development/upstream-bugs.md row 110)"
	// resourceTokenLevels: a project or group access token is created with
	// a membership role, which leaves out both ends of AccessLevelValue and
	// the minimal access level.
	resourceTokenLevels = "lib/api/resource_access_tokens.rb validates access_level against Gitlab::Access.options_with_owner, and doc/api/project_access_tokens.md#create-a-project-access-token and group_access_tokens.md#create-a-group-access-token list the same roles, 10 (Guest) through 50 (Owner): 0 (No access), 5 (Minimal access) and 60 (Admin) are no role a token is created with"
	// notificationLevelNames: the type's constants are integers, and what
	// goes out is the name each one stands for.
	notificationLevelNames = "NotificationLevelValue is an int whose String and MarshalJSON write the level's name (client-go types.go), so its constants read 0 through 5 while doc/api/notification_settings.md documents the names disabled, participating, watch, global, mention and custom, which the input offers and notifications.buildUpdateOpts maps onto the constants; global is the level a group or project falls back to and is not one the global settings take"
	// shareAccessLevel covers the two share endpoints, which grant a role.
	shareAccessLevel = "a group share declares values_with_minimal_access and a project share Gitlab::Access.all_values at v19.5.0-ee: 0 grants nothing, 60 (Admin) is not a membership level, and a project share refuses 5 (Minimal access) as well (docs/development/upstream-bugs.md row 110)"
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
	// The SDK gaps below are recorded in docs/development/upstream-bugs.md.
	eventActionApproved = "approved is a documented contribution event action (doc/api/events.md links the contributions calendar page that lists it); client-go's EventTypeValue has no constant for it, see docs/development/upstream-bugs.md"
	// eventFilterAccepted: the events routes filter on the model, not on
	// doc/api/events.md, which lists epic (refused) and omits these three
	// (filtered on). Grape validates target_type only: Event.actions is a
	// Hash, which Grape 2.4 reads as an options hash with no value set.
	eventFilterAccepted  = "the events routes filter action on Event.actions (EventsFinder#by_action) and validate target_type against Event.target_types (lib/api/helpers/events_helpers.rb, app/models/event.rb), which hold it although doc/api/events.md does not list it; client-go's EventTypeValue and EventTargetTypeValue have no constant for it, see docs/development/upstream-bugs.md"
	todoActionUndeclared = "doc/api/todos.md#get-a-list-of-to-do-items lists it as a filter value; client-go's TodoAction has no constant for it, see docs/development/upstream-bugs.md"
	// todoActionAccepted: the route validates action against the model's set,
	// which is wider than the page's list; transfer_failed is the item a
	// background transfer leaves when it fails, which the transfer hints send a
	// model to find.
	todoActionAccepted = "lib/api/todos.rb validates action against Todo.action_names (app/models/todo.rb and ee/app/models/ee/todo.rb), which holds it although doc/api/todos.md does not list it; client-go's TodoAction has no constant for it, see docs/development/upstream-bugs.md"
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
// Each entry carries a reason that cites the GitLab API doc, or the route
// and model where the doc lists a value they refuse, and an entry
// that excuses nothing (the field is gone, or the gap closed) is a finding,
// so the table cannot outlive what it describes.
var acceptedEnumGaps = buildAcceptedEnumGaps()

// buildAcceptedEnumGaps assembles the table: the hand-written entries plus
// the one family large enough to spell out programmatically, the project
// feature access levels, which all carry the same documented three-value set.
func buildAcceptedEnumGaps() map[string]string {
	table := map[string]string{
		// --- Membership access levels (AccessLevelValue) ---
		"accessrequests.ApproveGroupInput.access_level=0":    membershipNoAccessGrant,
		"accessrequests.ApproveGroupInput.access_level=60":   membershipNoAdmin,
		"accessrequests.ApproveProjectInput.access_level=0":  membershipNoAccessGrant,
		"accessrequests.ApproveProjectInput.access_level=5":  projectNoMinimalAccess,
		"accessrequests.ApproveProjectInput.access_level=60": membershipNoAdmin,
		"groupmembers.AddInput.access_level=60":              membershipNoAdmin,
		"groupmembers.EditInput.access_level=60":             membershipNoAdmin,
		"invites.GroupInvitesInput.access_level=0":           membershipNoAccessGrant,
		"invites.GroupInvitesInput.access_level=60":          membershipNoAdmin,
		"invites.ProjectInvitesInput.access_level=0":         membershipNoAccessGrant,
		"invites.ProjectInvitesInput.access_level=5":         projectNoMinimalAccess,
		"invites.ProjectInvitesInput.access_level=60":        membershipNoAdmin,
		"members.AddInput.access_level=5":                    projectNoMinimalAccess,
		"members.AddInput.access_level=60":                   membershipNoAdmin,
		"members.EditInput.access_level=5":                   projectNoMinimalAccess,
		"members.EditInput.access_level=60":                  membershipNoAdmin,
		"members.AddInput.access_level=0":                    membershipNoAccessGrant,
		"members.EditInput.access_level=0":                   membershipNoAccessGrant,
		"groupmembers.AddInput.access_level=0":               membershipNoAccessGrant,
		"groupmembers.EditInput.access_level=0":              membershipNoAccessGrant,
		"groupldap.AddInput.group_access=0":                  groupLinkNoAccess,
		"groupldap.AddInput.group_access=60":                 membershipNoAdmin,
		"groupsaml.AddInput.access_level=0":                  groupLinkNoAccess,
		"groupsaml.AddInput.access_level=60":                 membershipNoAdmin,

		"memberroles.CreateInstanceInput.base_access_level=0":  memberRoleBaseLevels,
		"memberroles.CreateInstanceInput.base_access_level=5":  memberRoleBaseLevels,
		"memberroles.CreateInstanceInput.base_access_level=50": memberRoleBaseLevels,
		"memberroles.CreateInstanceInput.base_access_level=60": memberRoleBaseLevels,

		"broadcastmessages.CreateInput.target_access_levels=0":  broadcastTargetLevels,
		"broadcastmessages.CreateInput.target_access_levels=5":  broadcastTargetLevels,
		"broadcastmessages.CreateInput.target_access_levels=25": broadcastTargetLevels,
		"broadcastmessages.CreateInput.target_access_levels=60": broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=0":  broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=5":  broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=25": broadcastTargetLevels,
		"broadcastmessages.UpdateInput.target_access_levels=60": broadcastTargetLevels,

		"groups.InvitedListInput.min_access_level=0":                 minAccessLevelFilter,
		"groups.InvitedListInput.min_access_level=5":                 minAccessLevelFilter,
		"groups.InvitedListInput.min_access_level=60":                minAccessLevelFilter,
		"groups.ListInput.min_access_level=0":                        minAccessLevelFilter,
		"groups.ListInput.min_access_level=5":                        minAccessLevelFilter,
		"groups.ListInput.min_access_level=60":                       minAccessLevelFilter,
		"groups.ListProjectsInput.min_access_level=0":                minAccessLevelFilter,
		"groups.ListProjectsInput.min_access_level=5":                minAccessLevelFilter,
		"groups.ListProjectsInput.min_access_level=60":               minAccessLevelFilter,
		"groups.ListSharedProjectsInput.min_access_level=0":          minAccessLevelFilter,
		"groups.ListSharedProjectsInput.min_access_level=5":          minAccessLevelFilter,
		"groups.ListSharedProjectsInput.min_access_level=60":         minAccessLevelFilter,
		"groups.SharedWithListInput.min_access_level=0":              minAccessLevelFilter,
		"groups.SharedWithListInput.min_access_level=5":              minAccessLevelFilter,
		"groups.SharedWithListInput.min_access_level=60":             minAccessLevelFilter,
		"groups.SubgroupsListInput.min_access_level=0":               minAccessLevelFilter,
		"groups.SubgroupsListInput.min_access_level=5":               minAccessLevelFilter,
		"groups.SubgroupsListInput.min_access_level=60":              minAccessLevelFilter,
		"projects.ListInvitedGroupsInput.min_access_level=0":         minAccessLevelFilter,
		"projects.ListInvitedGroupsInput.min_access_level=5":         minAccessLevelFilter,
		"projects.ListInvitedGroupsInput.min_access_level=60":        minAccessLevelFilter,
		"projects.ListProjectGroupsInput.shared_min_access_level=0":  minAccessLevelFilter,
		"projects.ListProjectGroupsInput.shared_min_access_level=5":  minAccessLevelFilter,
		"projects.ListProjectGroupsInput.shared_min_access_level=60": minAccessLevelFilter,
		// The project lists build their options in a helper the handlers
		// hand their input's fields, or their input whole, which is where the
		// pairing meets them.
		"projects.ListInput.min_access_level=0":                         minAccessLevelFilter,
		"projects.ListInput.min_access_level=5":                         minAccessLevelFilter,
		"projects.ListInput.min_access_level=60":                        minAccessLevelFilter,
		"projects.ListForksInput.min_access_level=0":                    minAccessLevelFilter,
		"projects.ListForksInput.min_access_level=5":                    minAccessLevelFilter,
		"projects.ListForksInput.min_access_level=60":                   minAccessLevelFilter,
		"projects.ListUserProjectsInput.min_access_level=0":             minAccessLevelFilter,
		"projects.ListUserProjectsInput.min_access_level=5":             minAccessLevelFilter,
		"projects.ListUserProjectsInput.min_access_level=60":            minAccessLevelFilter,
		"projects.ListUserContributedProjectsInput.min_access_level=0":  minAccessLevelFilter,
		"projects.ListUserContributedProjectsInput.min_access_level=5":  minAccessLevelFilter,
		"projects.ListUserContributedProjectsInput.min_access_level=60": minAccessLevelFilter,
		"projects.ListUserStarredProjectsInput.min_access_level=0":      minAccessLevelFilter,
		"projects.ListUserStarredProjectsInput.min_access_level=5":      minAccessLevelFilter,
		"projects.ListUserStarredProjectsInput.min_access_level=60":     minAccessLevelFilter,

		// The group member role is built by the instance one's builder, and
		// the two access token creates in a helper handed an intermediate.
		"memberroles.CreateGroupInput.base_access_level=0":  memberRoleBaseLevels,
		"memberroles.CreateGroupInput.base_access_level=5":  memberRoleBaseLevels,
		"memberroles.CreateGroupInput.base_access_level=50": memberRoleBaseLevels,
		"memberroles.CreateGroupInput.base_access_level=60": memberRoleBaseLevels,
		"accesstokens.GroupCreateInput.access_level=0":      resourceTokenLevels,
		"accesstokens.GroupCreateInput.access_level=5":      resourceTokenLevels,
		"accesstokens.GroupCreateInput.access_level=60":     resourceTokenLevels,
		"accesstokens.ProjectCreateInput.access_level=0":    resourceTokenLevels,
		"accesstokens.ProjectCreateInput.access_level=5":    resourceTokenLevels,
		"accesstokens.ProjectCreateInput.access_level=60":   resourceTokenLevels,

		"groups.ShareGroupInput.group_access=0":      shareAccessLevel,
		"groups.ShareGroupInput.group_access=60":     shareAccessLevel,
		"groupmembers.ShareInput.group_access=0":     shareAccessLevel,
		"groupmembers.ShareInput.group_access=60":    shareAccessLevel,
		"projects.ShareProjectInput.group_access=0":  shareAccessLevel,
		"projects.ShareProjectInput.group_access=5":  shareAccessLevel,
		"projects.ShareProjectInput.group_access=60": shareAccessLevel,

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
		"projects.UpdateInput.reviewer_assignment_strategy=dap_powered":    reviewerStrategyDAP,

		// --- Documented values client-go has no constant for (extras) ---
		"events.ListContributionEventsInput.action=approved":   eventActionApproved,
		"events.ListProjectEventsInput.action=approved":        eventActionApproved,
		"users.ListContributionEventsInput.action=approved":    eventActionApproved,
		"todos.ListInput.action=member_access_requested":       todoActionUndeclared,
		"todos.ListInput.action=merge_train_removed":           todoActionUndeclared,
		"todos.ListInput.action=unmergeable":                   todoActionUndeclared,
		"todos.ListInput.action=added_approver":                todoActionAccepted,
		"todos.ListInput.action=duo_core_access_granted":       todoActionAccepted,
		"todos.ListInput.action=duo_enterprise_access_granted": todoActionAccepted,
		"todos.ListInput.action=duo_pro_access_granted":        todoActionAccepted,
		"todos.ListInput.action=duo_workflow_input_required":   todoActionAccepted,
		"todos.ListInput.action=okr_checkin_requested":         todoActionAccepted,
		"todos.ListInput.action=review_requested":              todoActionAccepted,
		"todos.ListInput.action=review_submitted":              todoActionAccepted,
		"todos.ListInput.action=ssh_key_expired":               todoActionAccepted,
		"todos.ListInput.action=ssh_key_expiring_soon":         todoActionAccepted,
		"todos.ListInput.action=transfer_failed":               todoActionAccepted,

		// --- Event filter values the model holds and the page omits (extras) ---
		"events.ListContributionEventsInput.action=transferred": eventFilterAccepted,
		"events.ListProjectEventsInput.action=transferred":      eventFilterAccepted,
		"users.ListContributionEventsInput.action=transferred":  eventFilterAccepted,
		"events.ListContributionEventsInput.target_type=wiki":   eventFilterAccepted,
		"events.ListProjectEventsInput.target_type=wiki":        eventFilterAccepted,
		"users.ListContributionEventsInput.target_type=wiki":    eventFilterAccepted,
		"events.ListContributionEventsInput.target_type=design": eventFilterAccepted,
		"events.ListProjectEventsInput.target_type=design":      eventFilterAccepted,
		"users.ListContributionEventsInput.target_type=design":  eventFilterAccepted,
	}
	maps.Copy(table, projectFeatureAccessLevelExemptions())
	maps.Copy(table, notificationLevelExemptions())
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
	"package_registry_access_level",
	"releases_access_level",
	"repository_access_level",
	"requirements_access_level",
	"security_and_compliance_access_level",
	"snippets_access_level",
	"wiki_access_level",
}

// notificationLevelConstants are the values of client-go's
// NotificationLevelValue constants as the rule reads them, disabled (0)
// through custom (5).
var notificationLevelConstants = []string{"0", "1", "2", "3", "4", "5"}

// notificationLevelOffers lists, per notification settings input, the level
// names its schema offers: the global settings take every level but global,
// which is the one a group or project falls back to.
var notificationLevelOffers = map[string][]string{
	"UpdateGlobalInput":  {"custom", "disabled", "mention", "participating", "watch"},
	"UpdateGroupInput":   {"custom", "disabled", "global", "mention", "participating", "watch"},
	"UpdateProjectInput": {"custom", "disabled", "global", "mention", "participating", "watch"},
}

// notificationLevelExemptions excuses the notification level value by value:
// each integer the SDK's constants read as, and each name an input offers in
// their place. It is spelled per value rather than per field so that a
// constant client-go adds, or a name an input drops, is a finding instead of
// being excused with the rest of the field.
func notificationLevelExemptions() map[string]string {
	out := map[string]string{}
	for input, offered := range notificationLevelOffers {
		for _, value := range slices.Concat(notificationLevelConstants, offered) {
			out["notifications."+input+".level="+value] = notificationLevelNames
		}
	}
	return out
}

// projectFeatureAccessLevelExemptions excuses the public constant on every
// feature access level of the project create and update inputs.
func projectFeatureAccessLevelExemptions() map[string]string {
	// No size hint: a hint is invisible to every caller, so the arithmetic
	// one carried was code no test could hold to anything.
	out := map[string]string{}
	for _, field := range projectFeatureAccessLevels {
		out["projects.CreateInput."+field+"=public"] = featureAccessLevelPublic
		out["projects.UpdateInput."+field+"=public"] = featureAccessLevelPublic
	}
	return out
}
