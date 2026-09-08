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
)

// memberOptionsReason is what lib/api/helpers/members_helpers.rb says about
// the two member fields that hang off presenter options.
const memberOptionsReason = "lib/api/entities/member.rb exposes avatar_path under options[:only_path] and " +
	"custom_attributes under :with_custom_attributes, and present_members in " +
	"lib/api/helpers/members_helpers.rb, which every member endpoint presents through, passes current_user, " +
	"source and show_seat_info alone, so neither option holds on any member route."

// declaredUnsurfaced holds every field GitLab's document lists that the
// endpoint does not send, each with the source that says so.
//
// A variable rather than a constant table so the type-grain stub can empty
// it: the entries are about the real tree, and against a synthetic one every
// last one of them is unused.
var declaredUnsurfaced = []sentDeclaration{ //nolint:gochecknoglobals // the adjudication table, emptied by the test stub
	{
		Package:  toolsDir + "/keys",
		Entity:   "APIEntitiesUserWithAdmin",
		Field:    declaredSegment,
		Category: categoryDocumentedNotSent,
		Reason: "lib/api/keys.rb describes the fingerprint lookup, GET /keys, as answering with Entities::UserWithAdmin " +
			"and presents Entities::SSHKeyWithUser or Entities::DeployKeyWithUser: the response is a key, and the user " +
			"fields the document lists at its top level are the key's user one level down, which the type publishes " +
			"under `user`.",
	},
	{
		Package:  toolsDir + "/invites",
		Entity:   "APIEntitiesInvitation",
		Field:    declaredSegment,
		Category: categoryDocumentedNotSent,
		Reason: "lib/api/invitations.rb describes the add-a-member POST as answering with Entities::Invitation and " +
			"returns what Members::InviteService answers, the `status` and `message` pair invitations.md prints; the " +
			"pending-invitation object is what the GET at the same path lists. The shape declaration for the other " +
			"direction of this join records the same thing.",
	},
	{
		Package:  toolsDir + "/groupmembers",
		Entity:   "APIEntitiesMember",
		Field:    "avatar_path",
		Category: categoryOptionNeverPassed,
		Reason:   memberOptionsReason,
	},
	{
		Package:  toolsDir + "/groupmembers",
		Entity:   "APIEntitiesMember",
		Field:    "custom_attributes",
		Category: categoryOptionNeverPassed,
		Reason:   memberOptionsReason,
	},
	{
		Package:  toolsDir + "/members",
		Entity:   "APIEntitiesMember",
		Field:    "avatar_path",
		Category: categoryOptionNeverPassed,
		Reason:   memberOptionsReason,
	},
	{
		Package:  toolsDir + "/members",
		Entity:   "APIEntitiesMember",
		Field:    "custom_attributes",
		Category: categoryOptionNeverPassed,
		Reason:   memberOptionsReason,
	},
	{
		Package:  toolsDir + "/groups",
		Entity:   "APIEntitiesMember",
		Field:    "avatar_path",
		Category: categoryOptionNeverPassed,
		Reason: memberOptionsReason + " The group's own custom_attributes are published by this package, so only " +
			"the member's avatar_path is reported at package grain.",
	},
}

// covers reports whether this declaration accounts for one finding.
func (d sentDeclaration) covers(finding UnsurfacedField) bool {
	return d.Package == finding.Package &&
		d.Entity == finding.Entity &&
		(d.Field == declaredSegment || d.Field == finding.Field)
}

// key names one declaration in a report, which is how a stale one is reported.
func (d sentDeclaration) key() string {
	return d.Package + "." + d.Entity + "." + d.Field
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
