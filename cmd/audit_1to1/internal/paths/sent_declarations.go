package paths

import "sort"

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

// reasonGroupScopedUserRoutes answers the five keys held against the three
// group-scoped user types that only an instance-wide route can send.
//
// It is one reason for fifteen findings because it is one artifact. All four
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
	"ee/lib/ee/api/entities/user_with_admin.rb, which only POST /users and PUT /users/:id present; and unconfirmed_email is not a user " +
	"key at all but one of the six on lib/api/entities/service_account.rb, which POST /service_accounts answers with. All five reach " +
	"this type only because it decodes the same gl.User those endpoints do, and all five are published on internal/tools/users, which " +
	"is the package those routes fill."

// memberEntity is the entity the billable members route annotates and the one
// internal/tools/groupmembers really publishes on its member output.
const memberEntity = "API::Entities::Member"

// accessRequesterEntity is what the access-request routes present, inheriting
// Member and merging UserBasic into it.
const accessRequesterEntity = "API::Entities::AccessRequester"

// The three presenter options behind [categoryOptionNeverPassed] in the member
// and user families, each naming the routes checked against the record at
// v19.3.1-ee. only_path is declared by none of the 2110 routes in that record,
// which is why every type carrying a user answers avatar_path this way; the
// contrast that makes the check worth running is render_html, which looks the
// same in the entity and is a declared parameter on three of them.
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
		"lib/api/helpers/custom_attributes_helpers.rb only supplies on the endpoints that declare it. None of the routes this type serves does, " +
		"so the key has never been on one of their responses."
	reasonShowSeatInfoNeverPassed = "ee/lib/ee/api/entities/member.rb exposes is_using_seat under the show_seat_info option. The group and project " +
		"member lists declare that parameter and the access-request routes do not, so a member can carry the key and an access request cannot. " +
		"The difference is per route set, not per entity: both render through the same Member."
)

// reasonBillableMemberEntity answers the nine membership keys the record reads
// against the billable members list.
//
// It is one reason for nine fields because it is one mistake: the route's desc
// annotates a different entity than its handler presents, so every key the
// annotated entity adds beyond the presented one is reported at once. The
// record itself holds both entities and settles it. API::Entities::Member
// carries access_level, created_by, expires_at, the two identities,
// is_using_seat, override, membership_state and member_role;
// API::Entities::BillableMember carries none of them and carries
// last_activity_on, membership_type, removable, is_last_owner and last_login_at
// instead, which client-go's BillableGroupMember models and this type
// publishes. The other direction of the join reports those same five as
// unpublished for the same reason, which is the second half of the proof.
//
// The four keys both entities do share, from the UserBasic each inherits, are
// published rather than declared: locked, public_email, avatar_path and
// custom_attributes are on a billable member exactly as they are on a member.
const reasonBillableMemberEntity = "ee/lib/api/groups.rb describes GET /groups/:id/billable_members with Entities::Member and presents " +
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
// where an endpoint declares it. Of the 2110 routes in the record exactly three
// do, and the only merge request one is the single-merge-request GET, which is
// why toolutil.MergeRequestOutput publishes both keys and this type does not.
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
	// The nine membership keys the billable members list is read against.
	// Named one by one rather than with a splat: internal/tools/groupmembers
	// also publishes API::Entities::Member on its own Output, where a finding
	// is real, and a splat over the entity would swallow that too.
	{Package: groupMembersPkg, Entity: memberEntity, Field: "access_level", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "created_by", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "expires_at", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "group_saml_identity", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "group_scim_identity", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "is_using_seat", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "member_role", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "membership_state", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},
	{Package: groupMembersPkg, Entity: memberEntity, Field: "override", Category: categoryDocumentedNotSent, Reason: reasonBillableMemberEntity},

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

	// The five keys the three group-scoped user types are held to and only an
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
		Package: projectsPkg, Type: "BasicOutput", Entity: "API::Entities::Project", Field: declaredSegment,
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

	// The two entities client-go's Group methods put in front of the group
	// types, neither of which a group route sends.
	{
		Package: groupsPkg, Entity: "API::Entities::BasicProjectDetails", Field: declaredSegment,
		Category: categoryDocumentedNotSent,
		Reason: "the only endpoint in the union naming this entity is GET /projects/:id/job_token_scope/groups_allowlist, " +
			"which presents BasicGroupDetails and whose desc annotation is the project allowlist's, copied; measured against " +
			"a fixture and recorded in docs/development/upstream-bugs.md under \"Three job token scope endpoints declare a " +
			"response entity they do not send\". internal/tools/projects publishes BasicProjectDetails, from the routes that " +
			"really send it.",
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
		Reason: "lib/api/merge_requests.rb:975 declares success Entities::MRNote and line 994 presents Entities::IssueBasic " +
			"beside Entities::ExternalIssue, so GET /projects/:id/merge_requests/:iid/closes_issues sends issues and never a " +
			"note. Recorded in docs/development/upstream-bugs.md for the documentation merge request.",
	},

	// organization_id on the two hook types, which is the one condition here
	// that no response can ever satisfy.
	{Package: projectsPkg, Entity: "API::Entities::ProjectHook", Field: "organization_id", Category: categorySubclassCannotSatisfy, Reason: reasonSystemHookSibling},
	{Package: groupsPkg, Entity: "API::Entities::GroupHook", Field: "organization_id", Category: categorySubclassCannotSatisfy, Reason: reasonSystemHookSibling},

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
		for _, declaration := range declarations {
			if declaration.covers(finding) {
				finding.Category, finding.Reason = declaration.Category, declaration.Reason
				used[declaration.key()] = true
				break
			}
		}
		classified = append(classified, finding)
	}
	return classified
}
