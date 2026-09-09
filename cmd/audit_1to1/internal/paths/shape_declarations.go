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
// Every entry was read against GitLab's own documentation page for the
// endpoint, and cites what that page says the response is.
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
)

// declaredShapeFields holds every published field the type-grain join reports
// that GitLab does send, each with the reason the record does not say so.
//
// The bar for adding an entry is the bar the entries below met: GitLab's own
// documentation page printing the response body that carries the field. A
// finding that only looks wrong is not one of these.
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
