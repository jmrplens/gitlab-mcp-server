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
	"example body of \"List project pipelines\", and client-go's PipelineInfo models it. The merge request pipeline list " +
	"presents PipelineBasic and sends no name, so the key arrives from the project list, which is the route List fills " +
	"this type from."

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
	// the presenter. The deployment merge request list shares the merge
	// request type and is deliberately not declared: lib/api/deployments.rb
	// declares the parameter and presents MergeRequestBasic without passing
	// it, so on that package the key never arrives and the finding is real.
	{Package: issuesPkg, Type: "Output", Field: "label_details", Category: categoryServerShape, Reason: reasonLabelDetailsRekeyed},
	{Package: mergeRequestsPkg, Type: "Output", Field: "label_details", Category: categoryServerShape, Reason: reasonLabelDetailsRekeyed},

	// The link this server builds to what an event names.
	{Package: toolsDir + "/events", Type: "ContributionEventOutput", Field: "target_url", Category: categoryServerDerived, Reason: reasonEventTargetURL},
	{Package: toolsDir + "/events", Type: "ProjectEventOutput", Field: "target_url", Category: categoryServerDerived, Reason: reasonEventTargetURL},
}

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
