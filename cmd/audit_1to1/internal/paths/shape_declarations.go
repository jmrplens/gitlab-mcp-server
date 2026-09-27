package paths

import "sort"

// shapeDeclaration records why an output field GitLab's own OpenAPI document
// does not list for the operations its type models is published anyway.
//
// It is the same shape [endpointDeclaration] and [silentOwnerDeclaration] have,
// and it exists for the same reason. The oracle behind the type-grain join is
// generated from GitLab's own code and is nonetheless incomplete: a Grape
// endpoint that renders a plain hash rather than an entity has no response
// schema worth the name, and the document then describes something other than
// what the endpoint sends. A finding produced that way is real about the record
// and false about this server, and the only honest thing to do with it is to
// write down which it is, with the evidence, where the next reader can disagree.
//
// Every entry cites what settles it: GitLab's own documentation page printing
// the response, the handler in GitLab's source that presents it, or the code
// in this repository that fills the field.
type shapeDeclaration struct {
	// Package owns the type, repository relative, spelled the way
	// [publishedType.Package] spells it.
	Package string
	// Type is the Go type publishing the field.
	Type string
	// Field is the json tag. A "*" covers every field of the type, which is
	// right when the record describes a different response entirely rather than
	// an incomplete version of the same one.
	Field string
	// Category says what kind of absence this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// Shape declaration categories.
const (
	// categoryRecordSilent is a response GitLab's generated document does not
	// model, because the endpoint renders a bare hash instead of a Grape entity
	// and the generator has no entity to read. The document then carries some
	// other response for the operation, or none of the right shape.
	categoryRecordSilent = "record-does-not-model-the-response"
	// categoryServerShape is a name this server publishes for values GitLab
	// sends under names of its own, so no entity carries the name and the field
	// is not a phantom: the values under it are what the endpoint sent.
	categoryServerShape = "this-server-shapes-what-gitlab-sends-flat"
	// categoryAnnotationNotPresented is a route whose desc annotates one
	// entity while its handler presents another. The record takes the entity
	// of a route from the annotation, because that is what a route says about
	// itself before a request is served, so the join holds the type against
	// the annotated entity and reports every key the presented one adds. The
	// presented entity is in the record too; only the route's pointer to it is
	// wrong, and the evidence is the handler's own `present` call.
	categoryAnnotationNotPresented = "route-annotation-names-another-entity-than-the-handler-presents"
	// categoryServerDerived is a field this server computes from what GitLab
	// sends rather than a key any route of the type sends, so no entity can
	// carry it. The evidence is the code here that fills it.
	categoryServerDerived = "this-server-derives-the-value-from-what-gitlab-sends"
	// categorySharedTypeFilledElsewhere is a field of a type this package
	// shares with another package whose routes fill it, while no route of this
	// package ever sends the key. The type is shared because both packages
	// decode one client-go struct, and the field is tagged omitempty, so it is
	// absent from every response this package serves rather than published
	// empty. The evidence is the alias that shares the type, the route that
	// never sends the key, and the input that no longer offers a way to ask
	// for it; a package whose input still offered one would be a finding.
	categorySharedTypeFilledElsewhere = "shared-type-field-only-another-package-fills"
)

// reasonBillableMemberPresented answers the five seat keys held against the
// billable member type. It is the other direction of the join
// [reasonBillableMemberEntity] answers, and one mistake: the route's
// annotation.
const reasonBillableMemberPresented = "ee/lib/ee/api/members.rb describes GET /groups/:id/billable_members with " +
	"`success ::API::Entities::Member` and presents `with: ::API::Entities::BillableMember`, which is UserBasic plus " +
	"last_activity_on, membership_type, removable, created_at, is_last_owner (from last_owner?) and last_login_at (from " +
	"current_sign_in_at), in ee/lib/api/entities/billable_member.rb. The record holds that entity with every one of those " +
	"keys, and the join holds this type against Member instead, because Member is what the route says it answers with. " +
	"group_members.md prints the keys in the example body of \"List all billable group members\", and client-go's " +
	"BillableGroupMember models them."

// reasonUserAdminAddresses answers the two sign-in addresses on the user type.
const reasonUserAdminAddresses = "lib/api/users.rb describes GET /users/:id with `success Entities::UserProfile` and presents " +
	"`can_read_admin_user_data? ? Entities::UserDetailsWithAdmin : Entities::UserProfile`. UserDetailsWithAdmin " +
	"(lib/api/entities/user_details_with_admin.rb) adds current_sign_in_ip and last_sign_in_ip to UserWithAdmin, so the " +
	"keys reach an administrator's token and no other, which is why they are omitted when empty. users.md prints both " +
	"under \"Retrieve a single user\", \"As an administrator\", and client-go's User models both. No route is annotated " +
	"with UserDetailsWithAdmin, so no route the join reads names an entity carrying them."

// reasonLabelDetailsRekeyed answers label_details on the two types whose list
// routes take with_labels_details and pass it to the presenter.
const reasonLabelDetailsRekeyed = "GitLab sends no key of this name. With with_labels_details the list routes pass the option " +
	"to the presenter (lib/api/issues.rb for the three issue lists, serializer_options_for in lib/api/merge_requests.rb for " +
	"the three merge request lists), and IssueBasic and MergeRequestBasic then render `labels` as label objects rather " +
	"than titles. client-go's Issue.UnmarshalJSON and MergeRequest.UnmarshalJSON move those objects into label_details and " +
	"put the titles back in labels, and this type publishes what client-go decoded under client-go's name. Every list " +
	"input of the type offers with_labels_details, and without it the key is absent."

// reasonEventTargetURL answers target_url on the two event types.
const reasonEventTargetURL = "no events route sends it: lib/api/entities/event.rb exposes target_type and target_iid and no " +
	"URL for the target. enrichContributionEventURLs and enrichProjectEventURLs in internal/tools/events build it through " +
	"toolutil.BuildTargetURL from those two keys and the web_url GET /projects/:id answers with for the event's project, " +
	"so it is a link this server adds to what the event names, and it is empty for a target type the builder has no " +
	"path segment for."

// reasonGroupDatadogProperties answers properties on the group Datadog item.
const reasonGroupDatadogProperties = "lib/api/integrations/integratable_operations.rb mounts both routes this type is filled " +
	"from. PUT /groups/:id/integrations/datadog is described with `success Entities::IntegrationBasic` and presents " +
	"`with: Entities::Integration`, which is IntegrationBasic plus properties (lib/api/entities/integration.rb). The GET " +
	"is mounted once for every integration as /groups/:id/integrations/:slug and annotated Entities::Integration, and " +
	"client-go's GetGroupDatadogIntegration spells the slug as the literal datadog, which the join does not match to the " +
	"placeholder, so the one route of the pair it reads is the PUT under its annotation. Both responses carry properties, " +
	"which holds the Datadog settings group_integrations.md lists under \"Set up Datadog\"."

// reasonPipelineNamePresented answers name on the pipeline list row.
const reasonPipelineNamePresented = "lib/api/ci/pipelines.rb describes GET /projects/:id/pipelines with " +
	"`model: Entities::Ci::PipelineBasic` and presents `with: Entities::Ci::PipelineBasicWithMetadata`, which is " +
	"PipelineBasic plus name (lib/api/entities/ci/pipeline_basic_with_metadata.rb). pipelines.md prints name in the " +
	"example body of \"List project pipelines\", and client-go's PipelineInfo models it. The other two routes the type " +
	"is filled from send no name: GET /projects/:id/merge_requests/:merge_request_iid/pipelines presents PipelineBasic " +
	"and the POST at the same path presents Pipeline (lib/api/merge_requests.rb), so the field is tagged omitempty and " +
	"is absent from every row those routes filled rather than published empty."

// reasonDeploymentLabelDetails answers label_details on the deployment merge
// request type.
const reasonDeploymentLabelDetails = "deploymentmergerequests.Output is toolutil.MergeRequestOutput, the type the merge " +
	"request lists fill too. lib/api/deployments.rb presents GET /projects/:id/deployments/:deployment_id/merge_requests " +
	"`with: Entities::MergeRequestBasic, current_user: current_user` and never passes with_labels_details, so " +
	"lib/api/entities/merge_request_basic.rb renders labels as titles, client-go's MergeRequest.UnmarshalJSON has no " +
	"label objects to move into label_details, and the omitempty field is absent from every row. The input no longer " +
	"offers with_labels_details, which the route declares and never reads."

// declaredShapeFields holds every published field the type-grain join reports
// that GitLab does send, each with the reason the record does not say so.
//
// The bar for adding an entry is the bar the entries below met: GitLab's own
// documentation page printing the response body that carries the field, or
// GitLab's own handler presenting the entity that exposes it, or, for a field
// this server fills itself, the code that fills it. A finding that only looks
// wrong is not one of these.
var declaredShapeFields = []shapeDeclaration{
	{
		Package:  toolsDir + "/invites",
		Type:     "InviteResultOutput",
		Field:    declaredSegment,
		Category: categoryRecordSilent,
		Reason: "invitations.md prints the whole response of the add-a-member POST: `{\"status\": \"success\"}` " +
			"when every invitation was sent, a `status`/`message` pair naming each address that failed, and a " +
			"`queued_users` map on an instance with member promotion management enabled. GitLab's generated " +
			"document carries the pending-invitation member object under that POST instead, which is what the " +
			"GET at the same path answers with, so every field of the real response reads as unpublished.",
	},
	{
		Package:  toolsDir + "/geo",
		Type:     "StatusOutput",
		Field:    "replicables",
		Category: categoryServerShape,
		Reason: "API::Entities::GeoSiteStatus renders the same thirteen metrics for every replicator class and " +
			"flattens them into key names, so `lfs_objects_synced_count` and 570 siblings are one matrix. They are " +
			"published here as a map keyed by replicable, `replicables.lfs_objects.synced_count`, which absorbs the " +
			"replicables GitLab enables each release instead of needing 600 named fields regenerated. The values are " +
			"GitLab's own, read off the captured response.",
	},
	{
		Package:  toolsDir + "/geo",
		Type:     "StatusOutput",
		Field:    "additional_fields",
		Category: categoryServerShape,
		Reason: "the residue of the same decomposition: a key of the status answer that is neither a matrix cell " +
			"nor a field this type publishes under GitLab's own name is kept here rather than dropped, so a field " +
			"GitLab adds to the entity reaches the caller before this code knows its name. Empty against every " +
			"key the record carries today.",
	},

	// The five seat keys of a billable member, named one by one rather than
	// with a splat: every other key of the type is judged against Member as
	// well, and a splat would swallow a real finding among them.
	{Package: groupMembersPkg, Type: "BillableMemberOutput", Field: "is_last_owner", Category: categoryAnnotationNotPresented, Reason: reasonBillableMemberPresented},
	{Package: groupMembersPkg, Type: "BillableMemberOutput", Field: "last_activity_on", Category: categoryAnnotationNotPresented, Reason: reasonBillableMemberPresented},
	{Package: groupMembersPkg, Type: "BillableMemberOutput", Field: "last_login_at", Category: categoryAnnotationNotPresented, Reason: reasonBillableMemberPresented},
	{Package: groupMembersPkg, Type: "BillableMemberOutput", Field: "membership_type", Category: categoryAnnotationNotPresented, Reason: reasonBillableMemberPresented},
	{Package: groupMembersPkg, Type: "BillableMemberOutput", Field: "removable", Category: categoryAnnotationNotPresented, Reason: reasonBillableMemberPresented},

	// The two sign-in addresses GitLab sends an administrator.
	{Package: usersPkg, Type: "Output", Field: "current_sign_in_ip", Category: categoryAnnotationNotPresented, Reason: reasonUserAdminAddresses},
	{Package: usersPkg, Type: "Output", Field: "last_sign_in_ip", Category: categoryAnnotationNotPresented, Reason: reasonUserAdminAddresses},

	// The pipeline name the project list sends and the group Datadog
	// configuration both of its routes send.
	{Package: toolsDir + "/pipelines", Type: "Output", Field: "name", Category: categoryAnnotationNotPresented, Reason: reasonPipelineNamePresented},
	{Package: toolsDir + "/integrations", Type: "GroupDatadogItem", Field: "properties", Category: categoryAnnotationNotPresented, Reason: reasonGroupDatadogProperties},

	// label_details on the two types whose lists pass with_labels_details to
	// the presenter, and on the deployment merge request list, which shares
	// the merge request type and never sends the key.
	{Package: issuesPkg, Type: "Output", Field: "label_details", Category: categoryServerShape, Reason: reasonLabelDetailsRekeyed},
	{Package: mergeRequestsPkg, Type: "Output", Field: "label_details", Category: categoryServerShape, Reason: reasonLabelDetailsRekeyed},
	{Package: toolsDir + "/deploymentmergerequests", Type: "Output", Field: "label_details", Category: categorySharedTypeFilledElsewhere, Reason: reasonDeploymentLabelDetails},

	// The link this server builds to what an event names.
	{Package: toolsDir + "/events", Type: "ContributionEventOutput", Field: "target_url", Category: categoryServerDerived, Reason: reasonEventTargetURL},
	{Package: toolsDir + "/events", Type: "ProjectEventOutput", Field: "target_url", Category: categoryServerDerived, Reason: reasonEventTargetURL},
	{Package: usersPkg, Type: "ContributionEventOutput", Field: "target_url", Category: categoryServerDerived, Reason: reasonUserEventTargetURL},

	// Values this server echoes or derives rather than reads off a key.
	{Package: toolsDir + "/groupanalytics", Type: "IssuesCountOutput", Field: "group_path", Category: categoryServerDerived, Reason: reasonAnalyticsGroupPath},
	{Package: toolsDir + "/groupanalytics", Type: "MRCountOutput", Field: "group_path", Category: categoryServerDerived, Reason: reasonAnalyticsGroupPath},
	{Package: toolsDir + "/groupanalytics", Type: "MembersCountOutput", Field: "group_path", Category: categoryServerDerived, Reason: reasonAnalyticsGroupPath},
	{
		Package: toolsDir + "/projectdiscovery", Type: "ResolveOutput", Field: "extracted_path", Category: categoryServerDerived,
		Reason: "the path this server reads off the git remote the caller passed, which is what it asks GET /projects/:id for; " +
			"no route sends it, and it is published so a caller can see which part of the remote was taken as the project.",
	},
	{
		Package: mergeRequestsPkg, Type: "ApproveOutput", Field: "approvals_required", Category: categoryAnnotationNotPresented,
		Reason: "lib/api/merge_request_approvals.rb describes POST /projects/:id/merge_requests/:merge_request_iid/approve with " +
			"Entities::MergeRequestApprovals, which is what a Community Edition instance presents, and " +
			"ee/lib/ee/api/merge_request_approvals.rb overrides present_approval to present the merge request's approval state " +
			"with Entities::ApprovalState, which exposes approvals_required with no condition " +
			"(ee/lib/api/entities/approval_state.rb). The record reads the annotation, so the key an Enterprise instance sends " +
			"reads as one no response carries; on a Community Edition instance it is absent and the field reads zero.",
	},
	{
		Package: mergeRequestsPkg, Type: "ApproveOutput", Field: "approved_by_count", Category: categoryServerDerived,
		Reason: "the length of the approved_by list lib/api/entities/merge_request_approvals.rb sends, which this server counts; " +
			"no route sends a count.",
	},

	// Objects GitLab nests that this server publishes one level up.
	{Package: toolsDir + "/wikis", Type: "AttachmentOutput", Field: "url", Category: categoryServerShape, Reason: reasonWikiAttachmentLink},
	{Package: toolsDir + "/wikis", Type: "AttachmentOutput", Field: "markdown", Category: categoryServerShape, Reason: reasonWikiAttachmentLink},
	{Package: toolsDir + "/projectimportexport", Type: "ExportStatusOutput", Field: "api_url", Category: categoryServerShape, Reason: reasonExportStatusLinks},
	{Package: toolsDir + "/projectimportexport", Type: "ExportStatusOutput", Field: "web_url", Category: categoryServerShape, Reason: reasonExportStatusLinks},
	{Package: mergeRequestsPkg, Type: "CreateTodoOutput", Field: "project_name", Category: categoryServerShape, Reason: reasonTodoFlattened},
	{Package: mergeRequestsPkg, Type: "CreateTodoOutput", Field: "target_title", Category: categoryServerShape, Reason: reasonTodoFlattened},
	{Package: issuesPkg, Type: "TodoOutput", Field: "target_title", Category: categoryServerShape, Reason: reasonTodoFlattened},
	{Package: mergeRequestsPkg, Type: "ReviewerOutput", Field: "id", Category: categoryServerShape, Reason: reasonReviewerFlattened},
	{Package: mergeRequestsPkg, Type: "ReviewerOutput", Field: "username", Category: categoryServerShape, Reason: reasonReviewerFlattened},
	{Package: mergeRequestsPkg, Type: "ReviewerOutput", Field: "name", Category: categoryServerShape, Reason: reasonReviewerFlattened},
	{Package: mergeRequestsPkg, Type: "ReviewerOutput", Field: "avatar_url", Category: categoryServerShape, Reason: reasonReviewerFlattened},
	{Package: mergeRequestsPkg, Type: "ReviewerOutput", Field: "web_url", Category: categoryServerShape, Reason: reasonReviewerFlattened},
	{Package: mergeRequestsPkg, Type: "ReviewerOutput", Field: "review_state", Category: categoryServerShape, Reason: reasonReviewerFlattened},

	// The project starrers, whose route annotates a user and presents the star.
	{Package: projectsPkg, Type: "StarrerOutput", Field: "starred_since", Category: categoryAnnotationNotPresented, Reason: reasonStarrerPresented},
	{Package: projectsPkg, Type: "StarrerOutput", Field: "user", Category: categoryAnnotationNotPresented, Reason: reasonStarrerPresented},

	// The keys a tag's signature merges in from the signature's own entity.
	{Package: toolsDir + "/tags", Type: "SignatureOutput", Field: "verification_status", Category: categoryRecordSilent, Reason: reasonTagSignatureMerged},
	{Package: toolsDir + "/tags", Type: "SignatureOutput", Field: "x509_certificate", Category: categoryRecordSilent, Reason: reasonTagSignatureMerged},

	// The two project-only keys of the label type the group labels share.
	{Package: toolsDir + "/grouplabels", Type: "Output", Field: "priority", Category: categorySharedTypeFilledElsewhere, Reason: reasonGroupLabelProjectKeys},
	{Package: toolsDir + "/grouplabels", Type: "Output", Field: "is_project_label", Category: categorySharedTypeFilledElsewhere, Reason: reasonGroupLabelProjectKeys},
}

// reasonUserEventTargetURL answers target_url on the user contribution events.
const reasonUserEventTargetURL = "no events route sends it: lib/api/entities/event.rb exposes target_type and target_iid and " +
	"no URL for the target. internal/tools/users builds it the way internal/tools/events does, from those two keys and the " +
	"web_url GET /projects/:id answers with for the event's project, so it is a link this server adds to what the event names."

// reasonAnalyticsGroupPath answers group_path on the three group counts.
const reasonAnalyticsGroupPath = "the group_path the caller passed, echoed so the count says which group it counts: the " +
	"analytics routes (ee/lib/api/analytics/group_activity_analytics.rb) answer with the count alone."

// reasonWikiAttachmentLink answers the two keys of an uploaded wiki attachment's link.
const reasonWikiAttachmentLink = "lib/api/entities/wiki_attachment.rb sends url and markdown inside link " +
	"(`expose :link do expose :file_path, as: :url; expose :markdown end`), and wikis.AttachmentOutput publishes them one " +
	"level up beside file_name, file_path and branch. The values are GitLab's own."

// reasonExportStatusLinks answers the two download links of a finished export.
const reasonExportStatusLinks = "lib/api/entities/project_export_status.rb sends api_url and web_url inside _links once the " +
	"export has finished, and projectimportexport.ExportStatusOutput publishes them one level up. The values are GitLab's own."

// reasonTodoFlattened answers the target and project names a to-do publishes.
const reasonTodoFlattened = "lib/api/entities/todo.rb sends the target and the project as objects, and the to-do published " +
	"here carries the target's title and the project's name one level up, beside the target's URL. The values are GitLab's own."

// reasonReviewerFlattened answers the reviewer's keys.
const reasonReviewerFlattened = "GET /projects/:id/merge_requests/:merge_request_iid/reviewers presents " +
	"Entities::MergeRequestReviewer, the reviewer under user beside state and created_at. mergerequests.ReviewerOutput " +
	"publishes the user's keys one level up and the reviewer's state as review_state, since the user has a state of its own. " +
	"The values are GitLab's own."

// reasonStarrerPresented answers the two keys of a project's starrer.
const reasonStarrerPresented = "lib/api/projects.rb describes GET /projects/:id/starrers with `model: Entities::UserBasic` and " +
	"presents `with: Entities::UserStarsProject` (line 865), which is starred_since and the user under user " +
	"(lib/api/entities/user_stars_project.rb). The record holds the route under the annotated user, so the two keys the " +
	"presented entity has read as keys no response carries."

// reasonTagSignatureMerged answers the keys a tag's signature merges in.
const reasonTagSignatureMerged = "lib/api/entities/tag_signature.rb exposes signature_type and then the signature through a " +
	"`merge: true` block that presents X509Signature (verification_status and x509_certificate) into the same object. The " +
	"record reads a merged block as no key of its own, so the keys the block contributes are missing from it; " +
	"doc/api/tags.md lists both among the response attributes of the X.509 signature endpoint."

// reasonGroupLabelProjectKeys answers the two project-only keys on the group
// label type.
const reasonGroupLabelProjectKeys = "grouplabels.Output is labeldata.Output, the type the project labels fill too. " +
	"lib/api/entities/project_label.rb adds priority and is_project_label to Entities::Label, and Entities::GroupLabel " +
	"(lib/api/entities/group_label.rb) is Entities::Label with nothing added, so no group label route sends either key. " +
	"labeldata.GroupOutput leaves both unset and both are omitted when unset, so they are absent from every group label."

// declaredShapeField finds the declaration covering one finding.
func declaredShapeField(finding UnpublishedField) (shapeDeclaration, bool) {
	for _, declaration := range declaredShapeFields {
		if declaration.covers(finding) {
			return declaration, true
		}
	}
	return shapeDeclaration{}, false
}

// covers reports whether this declaration accounts for one finding.
func (d shapeDeclaration) covers(finding UnpublishedField) bool {
	return d.Package == finding.Package &&
		d.Type == finding.Type &&
		(d.Field == declaredSegment || d.Field == finding.Field)
}

// key names one declaration in a report, which is how a stale one is reported.
func (d shapeDeclaration) key() string {
	return d.Package + "." + d.Type + "." + d.Field
}

// classifyShapeFindings attaches the declaration that accounts for each finding
// at either level, and names the declarations that accounted for none.
//
// The two levels are classified together because one declaration can only be
// stale once: a table walked twice would report a declaration that matched a
// nested finding as unused by the top-level pass.
//
// A declaration that stops matching is a finding of its own, on the same terms
// as every other declaration table here: the field was removed, renamed, or the
// record grew the response it was missing, and in each case the excuse now
// outlives the thing it excused.
func classifyShapeFindings(topLevel, nested []UnpublishedField) (classifiedTop, classifiedNested []UnpublishedField, unused []string) {
	used := map[string]bool{}
	classifiedTop = classifyAgainstDeclarations(topLevel, used)
	classifiedNested = classifyAgainstDeclarations(nested, used)

	// Left nil rather than empty when nothing is stale: a check that reports no
	// stale declaration and one that has none to report are the same statement,
	// and the nil is what a comparison against a zero value reads as.
	for _, declaration := range declaredShapeFields {
		if !used[declaration.key()] {
			unused = append(unused, declaration.key())
		}
	}
	sort.Strings(unused)
	return classifiedTop, classifiedNested, unused
}

// classifyAgainstDeclarations annotates one list of findings, recording in used
// the declarations that accounted for something.
//
// An empty list comes back nil rather than empty, so annotating findings that
// are not there does not turn "no finding" into "a list of none".
func classifyAgainstDeclarations(found []UnpublishedField, used map[string]bool) []UnpublishedField {
	var classified []UnpublishedField
	for _, finding := range found {
		if declaration, ok := declaredShapeField(finding); ok {
			finding.Category = declaration.Category
			finding.Reason = declaration.Reason
			used[declaration.key()] = true
		}
		classified = append(classified, finding)
	}
	return classified
}
