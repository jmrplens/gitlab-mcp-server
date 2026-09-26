package paths

import (
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// sentDeclaration answers a finding of the sent dimension: a field GitLab's
// document lists among an operation's response properties that the endpoint
// does not send, so that the field is a defect of the document rather than a
// gap in the surface.
//
// It is the same shape [shapeDeclaration] has, for the other direction of the
// same join, and it meets the same bar: GitLab's own source or documentation
// page saying what the endpoint sends. A declaration names the component the
// finding was read on rather than an operation, because a finding carries the
// operations of its type or package as a whole and the component per field.
// The fingerprint lookup of a key is described as answering with a user and
// answers with a key, and the user's fields are exactly the ones read on
// UserWithAdmin.
type sentDeclaration struct {
	// Package is repository relative, spelled the way [publishedType.Package]
	// spells it.
	Package string
	// Entity is the component the findings were read on.
	Entity string
	// Type narrows the declaration to one output type of the package. Empty
	// answers every type, which is what a declaration about the package's
	// route set wants. It is set where a package publishes one type per GitLab
	// entity and only one of them is being answered: the type grain holds each
	// of them against the union of endpoints their shared client-go struct
	// reaches, so without this a splat over the entity would silence the type
	// that really does model it.
	Type string
	// Field is the json name, or "*" for every field read on the component.
	Field string
	// Category says what kind of absence this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// Sent declaration categories.
const (
	// categoryDocumentedNotSent is a response the document describes for an
	// operation and the endpoint does not send: the operation's description
	// names one entity and its handler presents another.
	categoryDocumentedNotSent = "documented-response-is-not-the-one-sent"
	// categoryOptionNeverPassed is a field the entity exposes under an
	// option of the presenter, which no endpoint of the package passes: the
	// record marks it sent-when, and on these routes the when never holds.
	categoryOptionNeverPassed = "entity-option-no-endpoint-passes"
	// categoryOptionTurnedOff is its mirror image, for a presenter option
	// whose default is to send: the endpoint does name the option, and names
	// it false. The two are kept apart because reading the condition alone
	// gives the wrong answer for each. `options.fetch(:x, false)` is silent
	// unless a route asks, so "no route passes it" settles it; a default of
	// true is sent unless a route refuses, so the same reasoning would
	// publish a key the endpoint suppresses on every response.
	categoryOptionTurnedOff = "entity-option-the-endpoint-turns-off"
	// categoryOptionNeverRequested is the third reading of a presenter option,
	// where the route does declare it as a request parameter and passes it
	// through, and this package's own requests never send it. The evidence is
	// therefore the request inventory rather than the route: the key can be on
	// that endpoint's response, and never on one this package asked for. Kept
	// apart from [categoryOptionNeverPassed] because it is the one of the
	// three that a change here, and not only a GitLab release, can retire: a
	// package that starts sending the parameter starts receiving the key. That
	// change leaves the finding exactly where it was, so [optionEvidence] holds
	// each declaration to the route's params and the package's recorded
	// requests rather than trusting its match.
	categoryOptionNeverRequested = "entity-option-this-package-never-requests"
	// categoryEntityPublishedElsewhere is an entity the package does surface,
	// on another type or under a shape of this server's own, so the field
	// names the comparison looks for are not the names the caller reads.
	categoryEntityPublishedElsewhere = "entity-published-by-another-type-or-shape"
	// categorySDKRouteNeverCalled is an entity that reached a type through
	// client-go rather than through anything this server does: a service
	// method answering with the struct the type pairs with sends to an
	// endpoint no handler here calls, so [readSDKRoutes] puts that endpoint,
	// and whatever entity it answers with, into the union in front of a type
	// that has never held one of its responses.
	categorySDKRouteNeverCalled = "sdk-route-no-handler-calls"
	// categorySDKRouteFillsAnotherType is its sibling for the case where this
	// repository does call the endpoint, into a different output type. Several
	// types here decode one client-go struct from different route sets, and
	// [readSDKRoutes] unions every endpoint that struct's methods reach in
	// front of all of them, so the entity behind an endpoint a given type is
	// never filled from is still held against it. The two are kept apart
	// because the evidence differs: the other is answered by showing that
	// nothing calls the method, and this one by showing which routes fill the
	// type.
	categorySDKRouteFillsAnotherType = "sdk-route-fills-another-type"
	// categorySubclassCannotSatisfy is a condition on the class of the object
	// being presented that the class this entity is ever given cannot satisfy.
	// It is not an option nobody passes and not a license nobody holds: no
	// request, no parameter and no license can make it true, because the two
	// classes are siblings rather than one being the other's ancestor. Kept
	// apart from the option categories because the evidence is the model
	// hierarchy rather than a route's parameters, and because no route
	// declaring something can ever retire it.
	categorySubclassCannotSatisfy = "entity-condition-the-presented-class-cannot-satisfy"
	// categoryAbilityNoRoleGrants is a condition asking the policy for an
	// ability that no role grants on the kind of source these routes present,
	// so no caller, request, parameter or license makes it true on them. Kept
	// apart from [categorySubclassCannotSatisfy] because the evidence is the
	// authorization model (config/authz/roles and the policy that enables a
	// role's permissions per scope) rather than the model hierarchy, and
	// because a GitLab release that grants the ability in the other scope
	// retires it.
	categoryAbilityNoRoleGrants = "entity-condition-an-ability-no-role-grants-on-the-source"
	// categoryConstantEmpty is a field the entity renders through a block that
	// returns the same empty value whatever the object: the key can arrive, and
	// never with anything in it, so publishing it would offer a key no answer
	// fills. It is GitLab's way of deprecating a key without removing it. Kept
	// apart from the condition categories because the condition is beside the
	// point: it can hold and the value is still empty, and only a GitLab
	// release that removes the block retires the declaration.
	categoryConstantEmpty = "entity-renders-a-constant-empty-value"
	// categoryCompactRow is a key the endpoint does send, which a compact row
	// leaves out on purpose. It is the one category here that answers a
	// finding true of GitLab: the row keeps what tells the objects of a list
	// apart and lets a caller pick one, and repeating the rest of the entity
	// on every row is a token cost the list does not pay, since one action
	// returns the whole object. The reason names that action, so a caller
	// told the key is missing knows where it is. A key the row's caller needs
	// is published instead, and a declaration of this category is a decision
	// about the surface that a later need can overturn rather than a fact
	// about GitLab's record.
	categoryCompactRow = "compact-row-leaves-it-to-the-detail-action"
)

// The packages and entities the compact-row declarations name. The two
// milestone packages share one reason per entity, since a project milestone and
// a group milestone list the same two entities through routes of the same
// shape; the commits package lists the merge request entity again.
const (
	milestonesPkg      = toolsDir + "/milestones"
	groupMilestonesPkg = toolsDir + "/groupmilestones"
	commitsPkg         = toolsDir + "/commits"
	issueBasicEntity   = "API::Entities::IssueBasic"
	mrBasicEntity      = "API::Entities::MergeRequestBasic"
)

// The milestone rows' answers: what the row keeps, and why the rest is the
// detail action's.
const (
	reasonMilestoneIssueRow = "the milestone issue lists (GET /projects/:id/milestones/:milestone_id/issues and " +
		"GET /groups/:id/milestones/:milestone_id/issues) present Entities::IssueBasic through milestone_issuables_for in " +
		"lib/api/milestone_responses.rb. " +
		"The row keeps the identifiers and the project, title, state, labels, author, assignees, confidentiality, weight, " +
		"due date, web URL and the created, updated and closed instants, which is what tells the issues of a milestone " +
		"apart; the milestone is the one being listed, and the description, the deprecated single assignee, time " +
		"tracking, task counts and the counters are issue.get's, which returns the whole issue for the one a caller picks."
	reasonMilestoneMergeRequestRow = "the milestone merge request lists (GET /projects/:id/milestones/:milestone_id/merge_requests " +
		"and GET /groups/:id/milestones/:milestone_id/merge_requests) present Entities::MergeRequestBasic through " +
		"milestone_issuables_for in lib/api/milestone_responses.rb. The row keeps the identifiers and the project, title, state, draft flag, detailed " +
		"merge status, both branches, labels, author, assignees, reviewers, web URL and the created, updated, merged and " +
		"closed instants; the milestone is the one being listed, and the SHAs, the merge and squash options, the " +
		"description, references, time tracking and the counters are merge_request.get's, which returns the whole merge " +
		"request for the one a caller picks."
	reasonMilestoneRenderHTMLNeverPassed = "lib/api/entities/merge_request_basic.rb exposes title_html and description_html only " +
		"when the presenter is given render_html, and the milestone merge request lists declare no parameter but the " +
		"milestone, its parent and the page, so Grape passes the option on neither and the keys are never on their responses."
	reasonCommitMergeRequestRow = "GET /projects/:id/repository/commits/:sha/merge_requests presents Entities::MergeRequestBasic " +
		"(lib/api/commits.rb). The row keeps the identifiers and the project, title, state, draft flag, both branches, the " +
		"merge commit SHA, labels, author, web URL and the created, updated, merged and closed instants, which is what says " +
		"which merge request carried the commit and where it landed; the description, the source SHA, the merge and squash " +
		"options, the assignees and reviewers, references, time tracking and the counters are merge_request.get's, which " +
		"returns the whole merge request."
	reasonCommitRenderHTMLNeverPassed = "lib/api/entities/merge_request_basic.rb exposes title_html and description_html only " +
		"when the presenter is given render_html, and GET /projects/:id/repository/commits/:sha/merge_requests declares " +
		"no such parameter (only sha, state and the page), so the keys are never on its response."
	reasonGroupProjectRow = "GET /groups/:id/projects and GET /groups/:id/projects/shared present Entities::Project, or " +
		"Entities::BasicProjectDetails when the caller passes simple, through present_projects in lib/api/groups.rb. The row " +
		"keeps the names and paths, the web and clone URLs, visibility, default branch, topics, the star and fork counts, " +
		"the archived flag and the created and last-activity instants, which is what tells the projects of a group apart and " +
		"lets a caller open or clone one; the namespace, the avatar, license and readme links, the deprecated tag_list, and " +
		"the settings, permissions, statistics and links of the full entity are project.get's, which returns the whole project."
	reasonTransferLocationsPresented = "lib/api/groups.rb describes GET /groups/:id/transfer_locations with `success Entities::Group` " +
		"and presents `present_groups params, groups, serializer: Entities::PublicGroupDetails`: BasicGroupDetails (id, web_url, " +
		"name) plus avatar_url, full_name and full_path, the six keys this type publishes. The record holds the route under the " +
		"annotated entity, so every other key of a group is reported against a response that carries none of them."
	reasonUpcomingJobRow = "GET /projects/:id/resource_groups/:key/upcoming_jobs presents Entities::Ci::JobBasic " +
		"(lib/api/ci/resource_groups.rb). The row keeps the ID, name, status, stage, ref, the tag and allow-failure flags, " +
		"the pipeline, web URL and creation time, which is what tells the jobs waiting on the group apart and where each " +
		"comes from. The user, commit and project are job.get's, which returns the whole job, and the fields of a run " +
		"(started_at, finished_at, erased_at, duration, queued_duration, coverage and failure_reason) describe a job that " +
		"has run, which a job waiting on the resource group has not."
)

// The member-family package paths, spelled once because several declarations
// share each and a repeated literal is what a reader has to compare by eye.
const (
	accessRequestsPkg = toolsDir + "/accessrequests"
	groupMembersPkg   = toolsDir + "/groupmembers"
	groupsPkg         = toolsDir + "/groups"
	issuesPkg         = toolsDir + "/issues"
	projectsPkg       = toolsDir + "/projects"
)

// userBasicEntity is the user object every other user entity inherits, and
// what GET /projects/:id/users presents directly.
const userBasicEntity = "API::Entities::UserBasic"

// reasonSystemHookSibling answers organization_id on the project and group
// hook entities.
const reasonSystemHookSibling = "lib/api/entities/hook.rb:17 exposes organization_id only when the presented hook is_a?(SystemHook). " +
	"ProjectHook (app/models/hooks/project_hook.rb:3), GroupHook (ee/app/models/hooks/group_hook.rb:3) and SystemHook " +
	"(app/models/hooks/system_hook.rb:3) are all siblings under WebHook, so a hook these two entities render is never a " +
	"SystemHook and no response of theirs can carry the key. Nothing a caller sends can change that."

// The two packages whose Output is one and the same type,
// toolutil.MergeRequestOutput, reported once under each package that aliases
// it and so answered once under each.
const (
	mergeRequestsPkg           = toolsDir + "/mergerequests"
	deploymentMergeRequestsPkg = toolsDir + "/deploymentmergerequests"
)

// The three group-scoped user packages, beside internal/tools/users. All four
// publish a user, all four pair with client-go's User, and only the last is
// filled from an instance-wide route.
const (
	enterpriseUsersPkg = toolsDir + "/enterpriseusers"
	groupSAMLPkg       = toolsDir + "/groupsaml"
	usersPkg           = toolsDir + "/users"
)

// The four user entities the eleven endpoints behind client-go's User present.
// UserPublic is what every group-scoped list answers with; the other three
// belong to routes only internal/tools/users calls.
const (
	userPublicEntity     = "API::Entities::UserPublic"
	userProfileEntity    = "API::Entities::UserProfile"
	userWithAdminEntity  = "API::Entities::UserWithAdmin"
	serviceAccountEntity = "API::Entities::ServiceAccount"
)

// reasonGroupScopedUserRoutes answers the six keys held against the three
// group-scoped user types that only an instance-wide route can send.
//
// It is one reason for eighteen findings because it is one artifact. All four
// user output types pair with client-go's User, and readSDKRoutes unions the
// eleven endpoints its service methods reach in front of every one of them.
// Three of those endpoints fill these types: GET /groups/:id/enterprise_users
// (and its single-user sibling), /provisioned_users and /saml_users, each of
// which GitLab presents `with: ::API::Entities::UserPublic` and not merely
// annotates that way. The keys here are on entities the other eight endpoints
// present, and internal/tools/users is where they are published.
const reasonGroupScopedUserRoutes = "the three group-scoped user endpoints present ::API::Entities::UserPublic, which carries none of these keys: " +
	"bio_html comes from lib/api/entities/users/bio_html.rb, which only UserProfile includes and only GET /users/:id presents; " +
	"enterprise_group_id, enterprise_group_associated_at and provisioned_by_group_id come from " +
	"ee/lib/ee/api/entities/user_with_admin.rb and provisioned_by_project_id, since 19.4, from lib/api/entities/user_with_admin.rb, " +
	"the entity only instance routes present (POST /users and PUT /users/:id, and GET /users and GET /user to an administrator); and " +
	"unconfirmed_email is not a user key at all but one of the six on lib/api/entities/service_account.rb, which POST /service_accounts " +
	"answers with. All six reach this type only because it decodes the same gl.User those endpoints do, and internal/tools/users is " +
	"the package those routes fill."

// memberEntity is the entity the billable members route annotates and the one
// internal/tools/groupmembers really publishes on its member output.
const memberEntity = "API::Entities::Member"

// accessRequesterEntity is what the access-request routes present: UserBasic
// merged in, and requested_at.
const accessRequesterEntity = "API::Entities::AccessRequester"

// The two access-request types, split so that each publishes the entity its
// own routes present. Both decode client-go's AccessRequest, which is what
// every one of the six routes answers with in client-go, so the type grain
// holds each against the union and reports the other entity's keys.
const (
	reasonAccessRequesterNotAMember = "accessrequests.Output is filled by the two access-request lists and the two requests " +
		"to join (lib/api/access_requests.rb), which present Entities::AccessRequester: UserBasic merged in, and requested_at, " +
		"which this type publishes. Member is what the two approve routes present, and accessrequests.MemberOutput, which " +
		"they fill, publishes it. A pending request is a person asking, not a membership, so none of Member's keys has ever " +
		"been on a response this type is read from."
	reasonApprovedMemberNotARequester = "accessrequests.MemberOutput is filled by the two approve routes (lib/api/access_requests.rb), " +
		"which present Entities::Member and not Entities::AccessRequester. requested_at belongs to the pending request the " +
		"approval ended, and accessrequests.Output, which the lists and the requests to join fill, publishes it."
)

// The two-factor key Member exposes since 19.4, on the two member types whose
// routes can never send it. lib/api/entities/member.rb gates it on
// `Ability.allowed?(opts[:current_user], :read_two_factor_member, opts[:source]
// || member.source)`, and config/authz/roles/owner.yml lists that ability in
// its group section alone (line 166 at 19.4.1-ee), which the admin role
// inherits; ProjectPolicy enables each role's project permissions through
// app/policies/concerns/authz/role_permissions.rb and has no rule of its own
// for it. internal/tools/groupmembers publishes the key, since its routes ask
// the ability of a group.
const (
	reasonMemberTwoFactorOnProject = "every route internal/tools/members is filled from is a project member route in " +
		"lib/api/members.rb, which presents Entities::Member with the project as source, or with no source on the PUT, " +
		"where member.source is the same project. read_two_factor_member is granted by no role in a project scope, so " +
		"the ability is refused to every caller and the key is never on these responses."
	reasonApprovedMemberTwoFactor = "the two approve routes present `result[:member], with: Entities::Member` " +
		"(lib/api/access_requests.rb) and pass no current_user, so the ability is asked of no user and refused, and the " +
		"key is never sent. On the project route it would be refused to any caller as well, since no role grants " +
		"read_two_factor_member in a project scope."
)

// The three presenter options behind [categoryOptionNeverPassed] in the member
// and user families, each naming the routes checked against the record, first
// at v19.3.1-ee and again at 19.4.1-ee. only_path is declared by none of the
// 2152 routes in the record at 19.4.1-ee, which is why every type carrying a
// user answers avatar_path this way; the contrast that makes the check worth
// running is render_html, which looks the same in the entity and is a declared
// parameter on three of them.
//
// A presenter option is not a request parameter a caller can smuggle in: Grape
// passes it only where the endpoint declares it, so a route that does not
// declare one can never send the fields it gates. That is what separates these
// from a field gated on the caller's permissions or the member's own state,
// which the same endpoint does send to somebody.
const (
	reasonOnlyPathNeverPassed = "lib/api/entities/user_basic.rb exposes avatar_path only when the presenter is given only_path, which is not a " +
		"request parameter but an option the endpoint has to pass. None of the routes this type serves declares it, so the key has never been " +
		"on one of their responses. Publishing it would advertise a field the endpoint cannot return."
	reasonCustomAttributesNeverPassed = "lib/api/entities/user_basic.rb exposes custom_attributes under the with_custom_attributes option, which " +
		"lib/api/helpers/custom_attributes.rb only supplies on the endpoints that declare it, and there only to a caller who asks and may " +
		"read custom attributes. None of the routes this type serves declares it, so the key has never been on one of their responses."
	reasonShowSeatInfoNeverPassed = "ee/lib/ee/api/entities/member.rb exposes is_using_seat under the show_seat_info option. The group and project " +
		"member lists declare that parameter and the access-request routes do not, so a member can carry the key and an access request cannot. " +
		"The difference is per route set, not per entity: both render through the same Member."
)

// reasonBillableMemberEntity answers the ten membership keys the record reads
// against the billable members list.
//
// It is one reason for ten fields because it is one mistake: the route's desc
// annotates a different entity than its handler presents, so every key the
// annotated entity adds beyond the presented one is reported at once. The
// record itself holds both entities and settles it. API::Entities::Member
// carries access_level, created_by, expires_at, the two identities,
// is_using_seat, override, membership_state, member_role and, since 19.4,
// two_factor_enabled;
// API::Entities::BillableMember carries none of them and carries
// last_activity_on, membership_type, removable, is_last_owner and last_login_at
// instead, which client-go's BillableGroupMember models and this type
// publishes. The other direction of the join reports those same five as
// unpublished for the same reason, which is the second half of the proof.
//
// The four keys both entities do share, from the UserBasic each inherits, are
// published rather than declared: locked, public_email, avatar_path and
// custom_attributes are on a billable member exactly as they are on a member.
const reasonBillableMemberEntity = "ee/lib/ee/api/members.rb describes GET /groups/:id/billable_members with Entities::Member and presents " +
	"::API::Entities::BillableMember, which inherits UserBasic and adds the seat keys rather than the membership ones. " +
	"A billable member is a user who costs a seat and not a membership record: it has no access level, no expiry, no " +
	"creator and no role, so this key has never been on that response. The record holds both entities and only the " +
	"route's annotation is wrong."

// The two entities client-go drags in front of the merge request output type,
// each named by the one service method that puts it there.
//
// Both endpoints exist and both send what the record says; nothing here calls
// either. mrapprovals serves the approval endpoints through GetConfiguration,
// which is the GET and answers with MergeRequestApprovals, and mrchanges serves
// the diff through ListMergeRequestDiffs rather than the deprecated /changes.
// So the entity in front of this type is one no request this server makes has
// ever produced, and publishing an approval count or a diff on a merge request
// list entry would invent a key GitLab does not send there.
const (
	reasonApprovalStateSDKRoute = "client-go declares MergeRequestApprovalsService.ChangeApprovalConfiguration as answering with *MergeRequest, and it " +
		"sends POST /projects/:id/merge_requests/:merge_request_iid/approvals, whose desc annotates Entities::ApprovalState. readSDKRoutes therefore " +
		"puts the approval state into the union of the eighteen endpoints it reads for the merge request structs, and the seventeen others answer with " +
		"a merge request that carries none of these keys. No handler in this repository calls that method: the only recorded request to that path is " +
		"the GET, from internal/tools/mrapprovals through GetConfiguration, which answers with Entities::MergeRequestApprovals and is published there."
	reasonChangesSDKRoute = "client-go declares MergeRequestsService.GetMergeRequestChanges as answering with *MergeRequest, and it sends " +
		"GET /projects/:id/merge_requests/:merge_request_iid/changes, whose desc annotates Entities::MergeRequestChanges. That entity's changes array " +
		"and overflow flag join the union the same way the approval state does. The method is deprecated in client-go and no handler here calls it: " +
		"internal/tools/mrchanges serves the diff through ListMergeRequestDiffs on /diffs, and the request inventory records no request to /changes at all."
)

// reasonRenderHTMLNeverPassed answers the two rendered-markup keys on the types
// whose routes cannot ask for them.
//
// render_html is unlike the presenter options above in one way that does not
// change the answer: it is a real request parameter, so a caller can ask for it
// where an endpoint declares it. Of the 2152 routes in the record at 19.4.1-ee
// exactly three do, and the only merge request one is the single-merge-request
// GET, which is why toolutil.MergeRequestOutput publishes both keys and this
// type does not.
const reasonRenderHTMLNeverPassed = "lib/api/entities/merge_request_basic.rb exposes title_html and description_html only when the presenter is given " +
	"render_html. Neither GET /projects/:id/issues/:issue_iid/related_merge_requests nor GET /projects/:id/issues/:issue_iid/closed_by declares that " +
	"parameter, so Grape passes the option on neither and the keys have never been on either response. The single-merge-request GET does declare it, " +
	"which is where the same two keys are published rather than declared."

// reasonIncludeSubscribedTurnedOff answers `subscribed` on the related issue
// the issue links list renders.
//
// It is the one presenter option in this table whose default sends. The three
// above it read `options.fetch(:only_path, false)` and friends, where a route
// that says nothing sends nothing; this one reads
// `options.fetch(:include_subscribed, true)`, where a route that says nothing
// sends the key. Applying the earlier rule here, that no route declares the
// option so it is never sent, would have been right about the routes and
// wrong about the field. The route settles it in the other direction: it does
// name the option, and passes false, with the entity's own comment saying why
// (computing the flag renders Markdown, which cannot be done per row of a
// list).
const reasonIncludeSubscribedTurnedOff = "lib/api/entities/issue.rb exposes subscribed under `options.fetch(:include_subscribed, true)`, which sends the " +
	"key unless the endpoint refuses it. GET /projects/:id/issues/:issue_iid/links is the only route filling this type and its `present` call in " +
	"lib/api/issue_links.rb passes `include_subscribed: false`, so the key has never been on one of its responses. The entity says why above the " +
	"exposure: the value triggers Markdown processing, which GitLab will not do for every row of a list."

// The package items and the entity they are read against, spelled once for the
// declarations below that share them.
const (
	packagesPkg   = toolsDir + "/packages"
	packageEntity = "API::Entities::Package"
)

// reasonPackageGroupOptionNeverPassed answers the owning project held against
// the package items a project's listing and a request for one package fill.
const reasonPackageGroupOptionNeverPassed = "lib/api/entities/package.rb exposes project_id under `opts[:group]` and project_path " +
	"under the same option and the caller's read access. lib/api/project_packages.rb presents both GET /projects/:id/packages " +
	"and GET /projects/:id/packages/:package_id with `namespace:` and no `group:`, so neither route has ever sent either key; " +
	"lib/api/group_packages.rb passes `group: true`, and packages.GroupListItem, which that route fills, publishes both."

// reasonPackagePipelinesConstantEmpty answers the pipelines key held against
// every package item and the package grain alike.
const reasonPackagePipelinesConstantEmpty = "lib/api/entities/package.rb exposes pipelines with a block returning EMPTY_PIPELINES, " +
	"a frozen empty array, whatever the package, so when its condition holds the key arrives as [] and otherwise not at all. " +
	"doc/api/packages.md records it as deprecated in GitLab 16.1 on both the listings and the read of one package. The " +
	"pipeline that last built the package is sent under pipeline, which every package item publishes."

// reasonPackageVersionsOnTheDetailItem answers the other versions held against
// the item the project's listing fills.
const reasonPackageVersionsOnTheDetailItem = "lib/api/entities/package.rb exposes versions `unless: ->(_, opts) { opts[:collection] }`, " +
	"and grape-entity's represent sets collection on every object of an array it presents, so GET /projects/:id/packages " +
	"(lib/api/project_packages.rb:67, `present paginate(packages)`) never sends them. packages.ListItem is held against GET " +
	"/projects/:id/packages/:package_id as well only because client-go's GetProjectPackage answers with the same Package " +
	"struct; that route presents one package (line 89) and fills packages.DetailItem, which publishes the versions."

// The entities the fields below were read on that no constant above names.
const (
	projectWithAccessEntity   = "API::Entities::Projects::WithAccessAndCatalogSetting"
	basicProjectDetailsEntity = "API::Entities::BasicProjectDetails"
	groupDetailEntity         = "API::Entities::GroupDetail"
	projectEntity             = "API::Entities::Project"
)

// The packages whose project presenter-option rows below answer the same
// three keys each, spelled once because each is named on every one of them.
const (
	attestationsPkg     = toolsDir + "/attestations"
	eventsPkg           = toolsDir + "/events"
	jobTokenScopePkg    = toolsDir + "/jobtokenscope"
	projectDiscoveryPkg = toolsDir + "/projectdiscovery"
	securityFindingsPkg = toolsDir + "/securityfindings"
	vulnerabilitiesPkg  = toolsDir + "/vulnerabilities"
)

// The presenter options behind the 31 package-grain findings the live record
// used to report as sent on every response, because version 2 of it recorded
// every hash and symbol condition as its kind alone (issue 973). Each was read
// against GitLab's source at v19.3.1-ee, the version the record pinned then,
// every line they cite reads the same at 19.4.1-ee, which it pins now, and each
// holds on none of the requests the package makes: either the route presents
// the entity without the option, or it declares the option as a parameter the
// package never sends.
//
// The license pair and custom_attributes on a project answer six packages at
// once and are one reason, because it is one fact about one route and the
// request inventory's rows for it: each of those packages reads a project to
// probe that it exists or to resolve its web URL, and none reads it to present
// it.
const (
	reasonProjectOptionsNeverRequested = "lib/api/entities/basic_project_details.rb exposes license and license_url under the license " +
		"option (lines 23 and 31) and custom_attributes under with_custom_attributes (line 43). GET /projects/:id (lib/api/projects.rb) " +
		"declares both as parameters defaulting to false and passes them to the presenter, and this package calls it with no query at all, " +
		"which is what the request inventory's row for it records: it reads the project to probe that it exists or to resolve its web URL, " +
		"never to present it. Neither option is ever set, so the keys have never been on a response this package reads. " +
		"internal/tools/projects sends license and with_custom_attributes on the same route and publishes all three keys."
	reasonGroupProjectsLicenseNeverPassed = "lib/api/entities/basic_project_details.rb exposes license and license_url under the license " +
		"option. GET /groups/:id/projects and GET /groups/:id/projects/shared both present through present_projects in " +
		"lib/api/groups.rb, which passes the entity, the current user and whatever with_custom_attributes adds, and never license, which " +
		"neither route declares. The keys have never been on either response; GET /projects/:id is the one route that sends them."
	reasonJobTokenScopeProjectOptionsNeverPassed = "lib/api/entities/basic_project_details.rb exposes license and license_url under the " +
		"license option and custom_attributes under with_custom_attributes. lib/api/project_job_token_scope.rb presents the inbound " +
		"allowlist with Entities::BasicProjectDetails and no option at all, and none of the three routes declares either parameter; the " +
		"groups allowlist presents BasicGroupDetails and the POST a ProjectScopeLink, whatever their annotations say. None of the three " +
		"keys has ever been on one of their responses."
	reasonGroupStatisticsNeverPassed = "lib/api/entities/group.rb:42 exposes statistics under the statistics option, which only the group " +
		"list's present_groups in lib/api/groups.rb sets, and only for a caller who can read all resources. GET /groups/:id presents through " +
		"present_group_details and POST /groups/:id/share presents the group with the current user alone; neither passes the option and " +
		"neither declares the parameter, so the key has never been on either response."
	reasonGroupCustomAttributesNeverRequested = "lib/api/entities/group.rb:40 exposes custom_attributes under with_custom_attributes. " +
		"GET /groups/:id declares that parameter and present_group_details in lib/api/groups.rb passes it only when the caller sends it, " +
		"and this package calls the route with no query at all, as the probe that tells a group with no LDAP links from a group that is " +
		"not there. The key has never been on a response it reads; internal/tools/groups sends the parameter and publishes the key."
	reasonSubmoduleCommitStatsNeverPassed = "lib/api/entities/commit_detail.rb:8 exposes stats under the include_stats option, which " +
		"lib/api/commits.rb passes from its stats parameter on the two commit routes that declare it. PUT " +
		"/projects/:id/repository/submodules/:submodule (lib/api/submodules.rb:64) presents the commit with the current user alone and " +
		"declares no such parameter, so the key has never been on its response."
	reasonEpicReferenceNeverPassed = "ee/lib/api/entities/epic.rb:129 exposes reference `if: { with_reference: true }`, and no epic " +
		"route passes with_reference: the routes in ee/lib/api/epics.rb present through epic_options (ee/lib/api/helpers/epics_helpers.rb), " +
		"which never sets it, and neither do the options the list merges into it; the child-epic routes in ee/lib/api/epic_links.rb " +
		"present the entity with no options at all. The only caller that sets it is ee/lib/api/entities/epic_issue_link.rb, for the " +
		"epic nested in an epic-issue link. GET /groups/:id/epics has never sent the key, and GitLab deprecated it in favor of " +
		"references, which this package publishes."
)

// declaredUnsurfaced holds every field GitLab's document lists that the
// endpoint does not send, each with the source that says so.
//
// A variable rather than a constant table so the type-grain stub can empty
// it: the entries are about the real tree, and against a synthetic one every
// last one of them is unused.
var declaredUnsurfaced = []sentDeclaration{ //nolint:gochecknoglobals // the adjudication table, emptied by the test stub
	{
		Package:  toolsDir + "/keys",
		Entity:   "API::Entities::UserWithAdmin",
		Field:    declaredSegment,
		Category: categoryDocumentedNotSent,
		Reason: "lib/api/keys.rb describes the fingerprint lookup, GET /keys, as answering with Entities::UserWithAdmin " +
			"and presents Entities::SSHKeyWithUser or Entities::DeployKeyWithUser: the response is a key, and the user " +
			"fields the document lists at its top level are the key's user one level down, which the type publishes " +
			"under `user`.",
	},
	{
		Package:  toolsDir + "/invites",
		Entity:   "API::Entities::Invitation",
		Field:    declaredSegment,
		Category: categoryDocumentedNotSent,
		Reason: "lib/api/invitations.rb describes the add-a-member POST as answering with Entities::Invitation and " +
			"returns what Members::InviteService answers, the `status` and `message` pair invitations.md prints; the " +
			"pending-invitation object is what the GET at the same path lists. The shape declaration for the other " +
			"direction of this join records the same thing.",
	},
	{
		Package:  toolsDir + "/projects",
		Entity:   "API::Entities::Group",
		Field:    declaredSegment,
		Category: categoryDocumentedNotSent,
		Reason: "lib/api/projects.rb describes GET :id/share_locations and GET :id/invited_groups as answering with " +
			"Entities::Group, and both call present_groups, the helper defined in the same file, which presents " +
			"Entities::PublicGroupDetails. Their sibling GET :id/groups calls the same helper and is annotated " +
			"PublicGroupDetails, correctly. PublicGroupDetails is BasicGroupDetails plus avatar_url, full_name and " +
			"full_path, which is six keys, and all three endpoints answer with exactly those six on GitLab.com: id, " +
			"name, avatar_url, web_url, full_name, full_path. ProjectGroupOutput publishes all six, so the type is " +
			"already 1:1 and the 49 fields read against it are the whole Group entity arriving through the wrong " +
			"annotation. Recorded in docs/development/upstream-bugs.md; the fix is gitlab-org/gitlab!254699.",
	},
	// The ten membership keys the billable members list is read against.
	// Named one by one rather than with a splat: internal/tools/groupmembers
	// also publishes API::Entities::Member on its own Output, where a finding
	// is real, and a splat over the entity would swallow that too. The tenth,
	// which Member exposes since 19.4 and groupmembers.Output publishes, is
	// narrowed to the billable member type for the same reason.
	{Package: groupMembersPkg, Entity: memberEntity, Field: "access_level", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "created_by", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "expires_at", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "group_saml_identity", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "group_scim_identity", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "is_using_seat", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "member_role", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "membership_state", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "override", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Type: "BillableMemberOutput", Entity: memberEntity, Field: "two_factor_enabled", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},

	// The two user keys UserBasic gates behind a presenter option, across the
	// three member-family packages whose routes never pass one. Each entry is
	// keyed by the entity the finding was read on, so the groupmembers pair
	// answers that package's member output and its billable member output
	// together: neither route set declares either option.
	{Package: accessRequestsPkg, Entity: accessRequesterEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: accessRequestsPkg, Entity: accessRequesterEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},
	{Package: groupsPkg, Entity: memberEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: groupsPkg, Entity: memberEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},

	// is_using_seat is declared for the access requests alone. The group and
	// project member lists do declare show_seat_info, so the same key on those
	// types is published from the SDK rather than answered here.
	{Package: accessRequestsPkg, Entity: memberEntity, Field: "is_using_seat", Category: categoryOptionNeverPassed, Reason: reasonShowSeatInfoNeverPassed},

	// The two entities client-go's own return types put in front of the merge
	// request output, answered with a splat because every key the entity has is
	// there for the same reason and neither entity is one these two packages
	// ever receive. internal/tools/mrapprovals publishes the approval state it
	// really does serve, under a package this pair of declarations cannot
	// reach.
	{Package: mergeRequestsPkg, Entity: "API::Entities::ApprovalState", Field: declaredSegment, Category: categorySDKRouteNeverCalled, Reason: reasonApprovalStateSDKRoute},
	{Package: mergeRequestsPkg, Entity: "API::Entities::MergeRequestChanges", Field: declaredSegment, Category: categorySDKRouteNeverCalled, Reason: reasonChangesSDKRoute},
	{Package: deploymentMergeRequestsPkg, Entity: "API::Entities::ApprovalState", Field: declaredSegment, Category: categorySDKRouteNeverCalled, Reason: reasonApprovalStateSDKRoute},
	{Package: deploymentMergeRequestsPkg, Entity: "API::Entities::MergeRequestChanges", Field: declaredSegment, Category: categorySDKRouteNeverCalled, Reason: reasonChangesSDKRoute},

	// The two rendered-markup keys on the issue package's merge request row,
	// named one by one rather than with a splat: every other key of the same
	// entity on that type is published, and a splat would swallow the next one
	// GitLab adds.
	{Package: issuesPkg, Entity: "API::Entities::MergeRequestBasic", Field: "title_html", Category: categoryOptionNeverPassed, Reason: reasonRenderHTMLNeverPassed},
	{Package: issuesPkg, Entity: "API::Entities::MergeRequestBasic", Field: "description_html", Category: categoryOptionNeverPassed, Reason: reasonRenderHTMLNeverPassed},

	// avatar_path on the four types that publish a user. The user entities
	// inherit it from UserBasic, so it is the same option and the same answer
	// as in the member family: no route declares only_path, so no response of
	// theirs has ever carried the key.
	{Package: usersPkg, Entity: userPublicEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: enterpriseUsersPkg, Entity: userPublicEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: groupsPkg, Entity: userPublicEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: groupSAMLPkg, Entity: userPublicEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},

	// The six keys the three group-scoped user types are held to and only an
	// instance-wide route can send. Named one by one rather than with a splat
	// over each entity: every other key of UserPublic on these types is
	// published, and UserProfile and UserWithAdmin both inherit the whole of
	// it, so a splat would swallow the next key GitLab adds there.
	{Package: enterpriseUsersPkg, Entity: userProfileEntity, Field: "bio_html", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: enterpriseUsersPkg, Entity: userWithAdminEntity, Field: "enterprise_group_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: enterpriseUsersPkg, Entity: userWithAdminEntity, Field: "enterprise_group_associated_at", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: enterpriseUsersPkg, Entity: userWithAdminEntity, Field: "provisioned_by_group_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: enterpriseUsersPkg, Entity: serviceAccountEntity, Field: "unconfirmed_email", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupsPkg, Entity: userProfileEntity, Field: "bio_html", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupsPkg, Entity: userWithAdminEntity, Field: "enterprise_group_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupsPkg, Entity: userWithAdminEntity, Field: "enterprise_group_associated_at", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupsPkg, Entity: userWithAdminEntity, Field: "provisioned_by_group_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupsPkg, Entity: serviceAccountEntity, Field: "unconfirmed_email", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupSAMLPkg, Entity: userProfileEntity, Field: "bio_html", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupSAMLPkg, Entity: userWithAdminEntity, Field: "enterprise_group_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupSAMLPkg, Entity: userWithAdminEntity, Field: "enterprise_group_associated_at", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupSAMLPkg, Entity: userWithAdminEntity, Field: "provisioned_by_group_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupSAMLPkg, Entity: serviceAccountEntity, Field: "unconfirmed_email", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: enterpriseUsersPkg, Entity: userWithAdminEntity, Field: "provisioned_by_project_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupsPkg, Entity: userWithAdminEntity, Field: "provisioned_by_project_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},
	{Package: groupSAMLPkg, Entity: userWithAdminEntity, Field: "provisioned_by_project_id", Category: categorySDKRouteFillsAnotherType, Reason: reasonGroupScopedUserRoutes},

	// The same two user keys on the project's user list. GET /projects/:id/users
	// declares search, skip_users and pagination and neither option, so it is
	// the member family's answer for the same entity.
	{Package: projectsPkg, Entity: userBasicEntity, Field: "avatar_path", Category: categoryOptionNeverPassed, Reason: reasonOnlyPathNeverPassed},
	{Package: projectsPkg, Entity: userBasicEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},

	// The two packages that publish one output type per GitLab entity, each
	// answered on the narrower type alone. Both pair with one client-go struct,
	// so readSDKRoutes unions every endpoint that struct's methods reach in
	// front of both types, and the wider entity's keys are then held against
	// the narrower type as well. Named with the type set, so the type that
	// really does model the wider entity keeps being judged against it.
	{
		Package: projectsPkg, Type: "BasicOutput", Entity: projectEntity, Field: declaredSegment,
		Category: categoryEntityPublishedElsewhere,
		Reason: "projects.BasicOutput models API::Entities::BasicProjectDetails, which is what the project search scope " +
			"(lib/api/search.rb SCOPE_ENTITY) and the job token allowlist answer with, and what any route narrows to under " +
			"simple=true. projects.Output embeds it and publishes the whole of API::Entities::Project. Both pair with " +
			"client-go's Project, so every endpoint that struct reaches is unioned in front of both.",
	},
	{
		Package: groupsPkg, Type: "Output", Entity: "API::Entities::GroupDetail", Field: declaredSegment,
		Category: categoryEntityPublishedElsewhere,
		Reason: "groups.Output models API::Entities::Group, which is what every route answering with a page of groups " +
			"renders. groups.DetailOutput embeds it and publishes what GroupDetail adds, on the seven routes that answer " +
			"with one group. Both pair with client-go's Group, so every endpoint that struct reaches is unioned in front " +
			"of both.",
	},

	{
		Package: projectsPkg, Type: "BasicOutput", Entity: "API::Entities::Projects::WithAccessAndCatalogSetting", Field: declaredSegment,
		Category: categoryEntityPublishedElsewhere,
		Reason: "GET /projects/:id answers with Project plus permissions and cicd_catalog_enabled, and projects.Output " +
			"publishes both. BasicProjectDetails carries neither, which is what BasicOutput models.",
	},

	// EpicIssue on the issue types. The union carries it because client-go's
	// Issue methods reach the epic's issue list, which is a route of another
	// package.
	{
		Package: issuesPkg, Entity: "API::Entities::EpicIssue", Field: declaredSegment,
		Category: categorySDKRouteFillsAnotherType,
		Reason: "GET /groups/:id/epics/:epic_iid/issues is what renders EpicIssue, and internal/tools/epicissues is the " +
			"package that calls it and publishes it. None of the fifteen issue routes sends the epic-link keys.",
	},

	// The third case of a desc annotation naming an entity the handler does
	// not present, after the two already recorded upstream.
	{
		Package: issuesPkg, Entity: "API::Entities::MRNote", Field: "note",
		Category: categoryDocumentedNotSent,
		Reason: "lib/api/merge_requests.rb:980 declares success Entities::MRNote and line 999 presents Entities::IssueBasic " +
			"beside Entities::ExternalIssue, so GET /projects/:id/merge_requests/:iid/closes_issues sends issues and never a " +
			"note. Recorded in docs/development/upstream-bugs.md for the documentation merge request.",
	},

	// organization_id on the two hook types, which is the one condition here
	// that no response can ever satisfy.
	{Package: projectsPkg, Entity: "API::Entities::ProjectHook", Field: "organization_id", Category: categorySubclassCannotSatisfy, Reason: reasonSystemHookSibling},
	{Package: groupsPkg, Entity: "API::Entities::GroupHook", Field: "organization_id", Category: categorySubclassCannotSatisfy, Reason: reasonSystemHookSibling},

	// The owning project on the package items the two project-scoped routes
	// fill, the listing's and the read of one package's. Named with the type,
	// since the group listing's item publishes both keys and is judged on its
	// own.
	{Package: packagesPkg, Type: "ListItem", Entity: packageEntity, Field: "project_id", Category: categoryOptionNeverPassed, Reason: reasonPackageGroupOptionNeverPassed},
	{Package: packagesPkg, Type: "ListItem", Entity: packageEntity, Field: "project_path", Category: categoryOptionNeverPassed, Reason: reasonPackageGroupOptionNeverPassed},
	{Package: packagesPkg, Type: "DetailItem", Entity: packageEntity, Field: "project_id", Category: categoryOptionNeverPassed, Reason: reasonPackageGroupOptionNeverPassed},
	{Package: packagesPkg, Type: "DetailItem", Entity: packageEntity, Field: "project_path", Category: categoryOptionNeverPassed, Reason: reasonPackageGroupOptionNeverPassed},

	// The package's other versions, on the item only the listing fills.
	{Package: packagesPkg, Type: "ListItem", Entity: packageEntity, Field: "versions", Category: categorySDKRouteFillsAnotherType, Reason: reasonPackageVersionsOnTheDetailItem},

	// pipelines, on every package item and at the package grain alike, since
	// no route renders it with anything in it.
	{Package: packagesPkg, Entity: packageEntity, Field: "pipelines", Category: categoryConstantEmpty, Reason: reasonPackagePipelinesConstantEmpty},

	// subscribed on the related issue, named alone rather than with a splat:
	// every other key of that entity is published on the same type, and a
	// splat would swallow the next one GitLab adds.
	{
		Package:  toolsDir + "/issuelinks",
		Entity:   "API::Entities::RelatedIssue",
		Field:    "subscribed",
		Category: categoryOptionTurnedOff,
		Reason:   reasonIncludeSubscribedTurnedOff,
	},

	// The 31 package-grain findings issue 973 moved from sent-always to
	// sent-when, answered under the conditions the record now carries. Each
	// is named field by field: every other key of these entities is either
	// published or a finding of its own, and a splat would swallow the next
	// key GitLab adds.
	//
	// custom_attributes on a user, from routes that declare no option: the
	// current user's own GET /user, and the participants of an issue and of a
	// merge request.
	{Package: toolsDir + "/awardemoji", Entity: userPublicEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},
	{Package: toolsDir + "/health", Entity: userPublicEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},
	{Package: issuesPkg, Entity: userBasicEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},
	{Package: mergeRequestsPkg, Entity: userBasicEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonCustomAttributesNeverPassed},

	// The license pair and custom_attributes on the project the six packages
	// that read one without presenting it get back from GET /projects/:id.
	// users publishes a user's custom_attributes, so the package grain finds
	// that name published there and reports only the license pair.
	{Package: attestationsPkg, Entity: projectWithAccessEntity, Field: "custom_attributes", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: attestationsPkg, Entity: projectWithAccessEntity, Field: "license", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: attestationsPkg, Entity: projectWithAccessEntity, Field: "license_url", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: eventsPkg, Entity: projectWithAccessEntity, Field: "custom_attributes", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: eventsPkg, Entity: projectWithAccessEntity, Field: "license", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: eventsPkg, Entity: projectWithAccessEntity, Field: "license_url", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: projectDiscoveryPkg, Entity: projectWithAccessEntity, Field: "custom_attributes", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: projectDiscoveryPkg, Entity: projectWithAccessEntity, Field: "license", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: projectDiscoveryPkg, Entity: projectWithAccessEntity, Field: "license_url", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: securityFindingsPkg, Entity: projectWithAccessEntity, Field: "custom_attributes", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: securityFindingsPkg, Entity: projectWithAccessEntity, Field: "license", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: securityFindingsPkg, Entity: projectWithAccessEntity, Field: "license_url", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: usersPkg, Entity: projectWithAccessEntity, Field: "license", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: usersPkg, Entity: projectWithAccessEntity, Field: "license_url", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: vulnerabilitiesPkg, Entity: projectWithAccessEntity, Field: "custom_attributes", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: vulnerabilitiesPkg, Entity: projectWithAccessEntity, Field: "license", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},
	{Package: vulnerabilitiesPkg, Entity: projectWithAccessEntity, Field: "license_url", Category: categoryOptionNeverRequested, Reason: reasonProjectOptionsNeverRequested},

	// The license pair on a group's projects, which the group routes never
	// ask the presenter for.
	{Package: groupsPkg, Entity: projectEntity, Field: "license", Category: categoryOptionNeverPassed, Reason: reasonGroupProjectsLicenseNeverPassed},
	{Package: groupsPkg, Entity: projectEntity, Field: "license_url", Category: categoryOptionNeverPassed, Reason: reasonGroupProjectsLicenseNeverPassed},

	// The same three project keys on the job token allowlists.
	{Package: jobTokenScopePkg, Entity: basicProjectDetailsEntity, Field: "custom_attributes", Category: categoryOptionNeverPassed, Reason: reasonJobTokenScopeProjectOptionsNeverPassed},
	{Package: jobTokenScopePkg, Entity: basicProjectDetailsEntity, Field: "license", Category: categoryOptionNeverPassed, Reason: reasonJobTokenScopeProjectOptionsNeverPassed},
	{Package: jobTokenScopePkg, Entity: basicProjectDetailsEntity, Field: "license_url", Category: categoryOptionNeverPassed, Reason: reasonJobTokenScopeProjectOptionsNeverPassed},

	// A group's statistics, which no route presenting one group sets, and its
	// custom attributes, which the one route that can is never asked for.
	{Package: toolsDir + "/groupldap", Entity: groupDetailEntity, Field: "statistics", Category: categoryOptionNeverPassed, Reason: reasonGroupStatisticsNeverPassed},
	{Package: toolsDir + "/groupldap", Entity: groupDetailEntity, Field: "custom_attributes", Category: categoryOptionNeverRequested, Reason: reasonGroupCustomAttributesNeverRequested},
	{Package: groupMembersPkg, Entity: groupDetailEntity, Field: "statistics", Category: categoryOptionNeverPassed, Reason: reasonGroupStatisticsNeverPassed},

	// The commit a submodule update answers with, and an epic's deprecated
	// short reference.
	{Package: toolsDir + "/repositorysubmodules", Entity: "API::Entities::CommitDetail", Field: "stats", Category: categoryOptionNeverPassed, Reason: reasonSubmoduleCommitStatsNeverPassed},
	{Package: toolsDir + "/epics", Entity: "API::Entities::Epic", Field: "reference", Category: categoryOptionNeverPassed, Reason: reasonEpicReferenceNeverPassed},

	// The two access-request types, each answered for the entity of the
	// other's routes alone. Both pair with client-go's AccessRequest, so
	// readSDKRoutes puts all six routes in front of each. Both entities merge
	// UserBasic, and describedRoutes credits a key the two share to the entity
	// of the first route it absorbs; the routes are sorted by operation, so
	// that is always a GET or POST presenting AccessRequester and never a PUT
	// approve presenting Member. The splat over Member on Output therefore
	// reaches Member's own keys and nothing Output could be sent. The reverse
	// is not true: a splat over AccessRequester on MemberOutput would also
	// answer every UserBasic key, which the approve routes do send, so that
	// entry names requested_at, the one key AccessRequester adds.
	{Package: accessRequestsPkg, Type: "Output", Entity: memberEntity, Field: declaredSegment, Category: categorySDKRouteFillsAnotherType, Reason: reasonAccessRequesterNotAMember},
	{Package: accessRequestsPkg, Type: "MemberOutput", Entity: accessRequesterEntity, Field: "requested_at", Category: categorySDKRouteFillsAnotherType, Reason: reasonApprovedMemberNotARequester},

	// two_factor_enabled on the approved member and on a project member,
	// neither of which any caller can be sent. The access-request entry names
	// the package rather than MemberOutput, because the package grain holds the
	// same key against the same approve routes.
	{Package: accessRequestsPkg, Entity: memberEntity, Field: "two_factor_enabled", Category: categoryOptionNeverPassed, Reason: reasonApprovedMemberTwoFactor},
	{Package: toolsDir + "/members", Entity: memberEntity, Field: "two_factor_enabled", Category: categoryAbilityNoRoleGrants, Reason: reasonMemberTwoFactorOnProject},

	{
		Package:  toolsDir + "/geo",
		Entity:   "API::Entities::GeoSiteStatus",
		Field:    declaredSegment,
		Category: categoryEntityPublishedElsewhere,
		Reason: "ee/lib/api/entities/geo_site_status.rb builds most of this entity by looping over " +
			"GeoNodeStatus::RESOURCE_STATUS_FIELDS, which is thirteen metrics for each of the 44 replicator " +
			"classes flattened into key names, so 573 of its 605 distinct keys are one matrix. `geo.StatusOutput` " +
			"publishes that matrix as `replicables`, keyed by replicable, where `lfs_objects_synced_count` is " +
			"`replicables.lfs_objects.synced_count`: the values are read off the captured response and reach the " +
			"caller, under names the record cannot match. The entity is compared against `geo.Output` as well, " +
			"which models a site rather than a status, because client-go declares RepairGeoSite as returning " +
			"*GeoSite while POST /geo_sites/:id/repair is annotated `success Entities::GeoSiteStatus` and presents " +
			"a status; that puts the status entity into the union of the five endpoints readSDKRoutes reads for " +
			"gl.GeoSite, and the four that do answer with a site send none of these keys, so publishing them on " +
			"the site type would invent them. Both halves are recorded in docs/development/upstream-bugs.md.",
	},

	// The milestone rows. The two rendered-markup keys come first, since they
	// are not sent at all and the rows' splats would otherwise answer them
	// with a reason about a key that is.
	{Package: milestonesPkg, Entity: mrBasicEntity, Field: "title_html", Category: categoryOptionNeverPassed, Reason: reasonMilestoneRenderHTMLNeverPassed},
	{Package: milestonesPkg, Entity: mrBasicEntity, Field: "description_html", Category: categoryOptionNeverPassed, Reason: reasonMilestoneRenderHTMLNeverPassed},
	{Package: groupMilestonesPkg, Entity: mrBasicEntity, Field: "title_html", Category: categoryOptionNeverPassed, Reason: reasonMilestoneRenderHTMLNeverPassed},
	{Package: groupMilestonesPkg, Entity: mrBasicEntity, Field: "description_html", Category: categoryOptionNeverPassed, Reason: reasonMilestoneRenderHTMLNeverPassed},
	{Package: milestonesPkg, Entity: issueBasicEntity, Field: declaredSegment, Category: categoryCompactRow, Reason: reasonMilestoneIssueRow},
	{Package: milestonesPkg, Entity: mrBasicEntity, Field: declaredSegment, Category: categoryCompactRow, Reason: reasonMilestoneMergeRequestRow},
	{Package: groupMilestonesPkg, Entity: issueBasicEntity, Field: declaredSegment, Category: categoryCompactRow, Reason: reasonMilestoneIssueRow},
	{Package: groupMilestonesPkg, Entity: mrBasicEntity, Field: declaredSegment, Category: categoryCompactRow, Reason: reasonMilestoneMergeRequestRow},

	// The jobs waiting on a resource group.
	{Package: toolsDir + "/resourcegroups", Entity: "API::Entities::Ci::JobBasic", Field: declaredSegment, Category: categoryCompactRow, Reason: reasonUpcomingJobRow},

	// The merge requests a commit belongs to, the rendered markup first for
	// the reason the milestone rows give.
	{Package: commitsPkg, Entity: mrBasicEntity, Field: "title_html", Category: categoryOptionNeverPassed, Reason: reasonCommitRenderHTMLNeverPassed},
	{Package: commitsPkg, Entity: mrBasicEntity, Field: "description_html", Category: categoryOptionNeverPassed, Reason: reasonCommitRenderHTMLNeverPassed},
	{Package: commitsPkg, Entity: mrBasicEntity, Field: declaredSegment, Category: categoryCompactRow, Reason: reasonCommitMergeRequestRow},

	// A group's projects, and the groups it can be transferred to, whose route
	// annotates a whole group and presents six keys of one.
	{Package: groupsPkg, Entity: "API::Entities::Project", Field: declaredSegment, Category: categoryCompactRow, Reason: reasonGroupProjectRow},
	{Package: groupsPkg, Type: "TransferLocationOutput", Entity: "API::Entities::Group", Field: declaredSegment, Category: categoryDocumentedNotSent, Reason: reasonTransferLocationsPresented},
}

// covers reports whether this declaration accounts for one finding.
func (d sentDeclaration) covers(finding UnsurfacedField) bool {
	return d.Package == finding.Package &&
		d.Entity == finding.Entity &&
		(d.Type == "" || d.Type == finding.Type) &&
		(d.Field == declaredSegment || d.Field == finding.Field)
}

// key names one declaration in a report, which is how a stale one is reported.
// The type is part of the name so that two declarations differing only in it
// are two names, and a stale one says which type it stopped describing.
func (d sentDeclaration) key() string {
	if d.Type == "" {
		return d.Package + "." + d.Entity + "." + d.Field
	}
	return d.Package + "." + d.Type + "." + d.Entity + "." + d.Field
}

// classifySentFindings attaches the declaration that accounts for each
// finding at either grain, and names the declarations that accounted for
// none. The two grains are classified together because one declaration can
// only be stale once, for the reason [classifyShapeFindings] records.
func classifySentFindings(declarations []sentDeclaration, byPackage, byType []UnsurfacedField) (classifiedPackage, classifiedType []UnsurfacedField, unused []string) {
	used := map[string]bool{}
	classifiedPackage = classifySent(declarations, byPackage, used)
	classifiedType = classifySent(declarations, byType, used)

	// Left nil rather than empty when nothing is stale, for the reason
	// [classifyShapeFindings] records.
	for _, declaration := range declarations {
		if !used[declaration.key()] {
			unused = append(unused, declaration.key())
		}
	}
	sort.Strings(unused)
	return classifiedPackage, classifiedType, unused
}

// classifySent annotates one list of findings, recording in used the
// declarations that accounted for something. An empty list comes back nil
// rather than empty, so that "no finding" stays "no finding".
func classifySent(declarations []sentDeclaration, found []UnsurfacedField, used map[string]bool) []UnsurfacedField {
	var classified []UnsurfacedField
	for _, finding := range found {
		if declaration, ok := coveringDeclaration(declarations, finding); ok {
			finding.Category, finding.Reason = declaration.Category, declaration.Reason
			used[declaration.key()] = true
		}
		classified = append(classified, finding)
	}
	return classified
}

// coveringDeclaration is the declaration that answers a finding: the first in
// the table that covers it, which is the one rule both the classification and
// the evidence check below read, so the two can never disagree about which
// entry a finding was answered by.
func coveringDeclaration(declarations []sentDeclaration, finding UnsurfacedField) (sentDeclaration, bool) {
	for _, declaration := range declarations {
		if declaration.covers(finding) {
			return declaration, true
		}
	}
	return sentDeclaration{}, false
}

// requestKey names what one package was recorded sending to one operation, in
// the spelling a package-grain finding carries its operations in.
type requestKey struct {
	pkg       string
	operation string
}

// optionEvidence is what the two presenter-option categories that turn on a
// request parameter rest on, read from the same record and inventory the
// findings they answer came from: the parameters each route declares, and
// the names each package was recorded sending it.
//
// Staleness alone cannot hold these categories. A declaration is stale when
// it matches no finding, and both of these keep matching theirs after their
// evidence is gone: a package that starts sending the option and still does
// not publish the key leaves the finding exactly where it was, now answered by
// a reason that is false. So each is judged against its own evidence as well
// as against the findings.
type optionEvidence struct {
	conditions *conditionIndex
	index      *operationIndex
	sent       map[requestKey]map[string]bool
}

// newOptionEvidence indexes the inventory's query and body names by package
// and operation.
func newOptionEvidence(conditions *conditionIndex, index *operationIndex, requests []requestinventory.Row) optionEvidence {
	sent := map[requestKey]map[string]bool{}
	for _, request := range requests {
		key := requestKey{pkg: request.Package, operation: request.Method + " " + request.Path}
		names := sent[key]
		if names == nil {
			names = map[string]bool{}
			sent[key] = names
		}
		for _, name := range request.Query {
			names[name] = true
		}
		for _, name := range request.Body {
			names[name] = true
		}
	}
	return optionEvidence{conditions: conditions, index: index, sent: sent}
}

// contradictions names every option declaration whose evidence the record or
// the inventory refutes, each with what refutes it.
//
// Only a package-grain finding is judged, because only its operations are the
// inventory's own rows: a type-grain finding carries the collapsed spellings
// of the routes a client-go struct reaches, and no package was recorded
// sending those. That leaves one way for a declaration of
// [categoryOptionNeverRequested] to escape its evidence, answering type-grain
// findings alone, and that is refused too, since the request inventory is the
// whole of what the category claims.
//
// [categoryOptionNeverPassed] is held to the half of its evidence the record
// can read: none of the routes a finding names may declare the option as a
// parameter, since a route that does can be asked for the key. A field gated
// by a block or a hash condition names its option in a form this does not
// parse, and is passed over rather than guessed at.
func (e optionEvidence) contradictions(declarations []sentDeclaration, byPackage, byType []UnsurfacedField) []string {
	var found []string
	judged := map[string]bool{}
	for _, finding := range byPackage {
		declaration, ok := coveringDeclaration(declarations, finding)
		if !ok {
			continue
		}
		var problem string
		switch declaration.Category {
		case categoryOptionNeverRequested:
			judged[declaration.key()] = true
			problem = e.neverRequested(finding)
		case categoryOptionNeverPassed:
			problem = e.neverPassed(finding)
		}
		if problem != "" {
			found = append(found, declaration.key()+" is declared as "+declaration.Category+", and "+problem)
		}
	}
	for _, finding := range byType {
		declaration, ok := coveringDeclaration(declarations, finding)
		if !ok || declaration.Category != categoryOptionNeverRequested || judged[declaration.key()] {
			continue
		}
		judged[declaration.key()] = true
		found = append(found, declaration.key()+" is declared as "+declaration.Category+", and it answers type-grain findings alone: "+
			"their operations are not rows of the request inventory, which is the evidence the category rests on, so nothing can hold it to it")
	}
	sort.Strings(found)
	return slices.Compact(found)
}

// neverRequested judges one finding answered as an option this package never
// requests: the field is gated by a symbol condition, a route the finding
// names declares that option as a parameter, and the package was never
// recorded sending it to any of them. It returns what refutes the claim, or
// nothing when the evidence holds.
func (e optionEvidence) neverRequested(finding UnsurfacedField) string {
	option := e.conditions.presenterOption(finding.Entity, finding.Field)
	if option == "" {
		return "the record gates " + finding.Entity + "." + finding.Field + " by no symbol condition naming an option a request could send"
	}
	declared := false
	for _, operation := range finding.Operations {
		if e.sent[requestKey{pkg: finding.Package, operation: operation}][option] {
			return "the package sends " + option + " on " + operation + ", so the key is on a response it reads: publish the field and drop the declaration"
		}
		if e.declares(operation, option) {
			declared = true
		}
	}
	if !declared {
		return "no route the finding names declares " + option + " as a parameter, so no request can ask for the key: the category is " + categoryOptionNeverPassed
	}
	return ""
}

// neverPassed judges one finding answered as an option no endpoint passes, on
// the half of the claim the record can read: no route the finding names may
// declare the option as a parameter.
func (e optionEvidence) neverPassed(finding UnsurfacedField) string {
	option := e.conditions.presenterOption(finding.Entity, finding.Field)
	if option == "" {
		return ""
	}
	for _, operation := range finding.Operations {
		if e.declares(operation, option) {
			return operation + " declares " + option + " as a parameter, so a request can ask for the key: if this package never sends it the category is " +
				categoryOptionNeverRequested + ", and if it does the field belongs on the surface"
		}
	}
	return ""
}

// declares reports whether the route an operation reaches declares the named
// parameter. An operation the record holds no route for declares nothing.
func (e optionEvidence) declares(operation, param string) bool {
	method, path, _ := strings.Cut(operation, " ")
	route, _, _ := e.index.lookup(method, path)
	_, declared := route.Params[param]
	return declared
}
