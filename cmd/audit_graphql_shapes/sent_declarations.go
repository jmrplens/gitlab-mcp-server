package main

import "sort"

// sentDeclaration answers a finding of the sent dimension: a field the pinned
// schema offers at an object this server decodes, that the document leaves out
// for a reason, so that a reader triaging the list is not asked the same
// question twice.
//
// It is the shape the REST tables in cmd/audit_1to1/internal/paths have, and
// it meets the same bar: evidence a reviewer can check, and enough of it to
// judge whether the reason still holds. It is keyed by the schema type rather
// than by the response path, because a path is brittle and multi-valued for
// one type: WorkItem is reached at nine positions across four packages, four
// of them in epicissues alone, and one answer covers a package's worth of
// them.
type sentDeclaration struct {
	// Package is the import path of the package whose struct decodes the
	// object, the same one a finding carries, which is not always the package
	// the send is in.
	Package string
	// SchemaType is the object the fields are offered on.
	SchemaType string
	// Field is the schema's field name, or "*" for every field offered on
	// that object at that package. The star is what makes this table
	// tractable: one entry answers all fifty-odd fields of a user object
	// under a note's author.
	Field string
	// Category says what kind of absence this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it
	// still holds.
	Reason string
}

// declaredSegment covers every field offered on one object.
const declaredSegment = "*"

// Sent declaration categories.
const (
	// categoryNotThisResponse is a reference stub: the object is named here
	// to identify something, and the fields belong to the domain that owns
	// it, which surfaces them through its own tools.
	categoryNotThisResponse = "not-part-of-this-response"
	// categoryLookup is a document that exists to resolve an identifier
	// rather than to answer a caller: nothing it does not select was ever
	// meant to reach anybody.
	categoryLookup = "lookup-not-a-response"
	// categoryDeprecated is a field GitLab has deprecated, which the pin
	// cannot say for the reason the report records, so the evidence is a
	// GitLab documentation or changelog citation.
	categoryDeprecated = "deprecated-upstream"
)

// Three more categories are wanted and are not written down until the first
// finding needs one, because a category nothing uses is a vocabulary rather
// than a decision: separate-action-not-a-field for a collection that would be
// its own catalog action, tier-gated-above-this-domain for a field GitLab
// serves only above the tier the domain is gated at, and published-elsewhere
// for a value this server does publish under a spelling the automatic match
// did not find. The first two need prose evidence for the same reason
// categoryDeprecated does: neither the tier nor the deprecation is in the pin.

// Where the packages these findings are filed against live, spelled once. A
// finding names the package the decoding struct is declared in, so the note
// mutations a shared toolutil wrapper sends are answered under the domain that
// decodes them and not under the wrapper.
const toolsDir = "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"

// userCoreReason is what a user object under an author is doing there, which
// is the largest single block of this dimension's findings.
const userCoreReason = "The object is the author of a note, a discussion or a work item, named so a reader knows " +
	"who wrote it. Its own fields are the users domain's surface, which publishes them through gitlab_user, and " +
	"a note tool answering with a user's saved replies, callouts, group memberships and workspaces would be " +
	"that domain twice over. What a note is asked for is the identity, which these documents select."

// notesAnchorReason is why the work item under a notes query is an anchor
// rather than the answer.
//
// It is the same judgement referenceStub makes, at the one shape where the
// walk's own third condition cannot make it: the anchor selects an id, which
// counts as reading a leaf, so the position is asked about and the epic's
// whole surface comes back as a gap in a notes tool. The evidence is the
// document, which selects the id and the widget list and nothing else, and the
// epic's own surface, which is where those fields already are.
func notesAnchorReason(document, tool string) string {
	return "The work item is an anchor, not the response: " + document + " selects its id and its widget list " +
		"only so the notes widget under it can be reached, and what " + tool + " answers with is the notes. Its " +
		"own fields are the epic surface, which epicissues and the epic tools publish and are held to this same " +
		"question on their own documents; adding them here would answer a notes call with an epic."
}

// referenceStub is why an object this server has a domain of its own for is
// not surfaced a second time under a vulnerability or a finding.
//
// It is the judgement the walk's third condition makes on its own for an
// object that is only traversed, written down for the positions where the stub
// selects an identity field and so counts as read. The evidence is the surface
// itself: each of these objects is a domain of this server, surfaced over
// REST, where R-PATH asks this same question against GitLab's own OpenAPI
// record with a tier-aware oracle this dimension does not have.
func referenceStub(object, tools string) string {
	return "The object is a reference: it is selected to say which " + object + " the answer is about, and its " +
		"own fields are that domain's surface, published by " + tools + ". Answering them here would be that " +
		"domain a second time, through a document nobody maintains against it, and the sent question is " +
		"already asked of it where the tier is known."
}

// declaredSent holds every field the schema offers that a document leaves out
// on purpose, each with the reason. It is what [auditRun.declarations] carries
// on a real run.
var declaredSent = []sentDeclaration{ //nolint:gochecknoglobals // the adjudication table this repository answers with
	{
		Package:    toolsDir + "/epicworkitems",
		SchemaType: "WorkItem",
		Field:      declaredSegment,
		Category:   categoryLookup,
		Reason: "queryResolveWorkItemGID exists to turn a namespace path and an iid into the global id the epic " +
			"mutations take, and it reads the one field that answers it. Nothing else it could select was ever " +
			"meant to reach a caller: the work item itself is surfaced by epicissues and epicnotes, which are " +
			"held to this same question on their own documents.",
	},
	{
		Package:    toolsDir + "/epicdiscussions",
		SchemaType: "UserCore",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     userCoreReason,
	},
	{
		Package:    toolsDir + "/epicnotes",
		SchemaType: "UserCore",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     userCoreReason,
	},
	{
		Package:    toolsDir + "/epicissues",
		SchemaType: "UserCore",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     userCoreReason,
	},
	{
		Package:    toolsDir + "/epicdiscussions",
		SchemaType: "WorkItem",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     notesAnchorReason("queryListDiscussions", "gitlab_list_epic_discussions"),
	},
	{
		Package:    toolsDir + "/epicnotes",
		SchemaType: "WorkItem",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     notesAnchorReason("queryListWorkItemNotes", "gitlab_epic_note_list"),
	},
	{
		Package:    toolsDir + "/vulnerabilities",
		SchemaType: "Project",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     referenceStub("project", "gitlab_project"),
	},
	{
		Package:    toolsDir + "/vulnerabilities",
		SchemaType: "MergeRequest",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     referenceStub("merge request a vulnerability was raised on", "gitlab_merge_request"),
	},
	{
		Package:    toolsDir + "/securityfindings",
		SchemaType: "Vulnerability",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason: referenceStub("vulnerability a pipeline finding was promoted to", "gitlab_vulnerability") +
			" The vulnerabilities package sends its own documents against this same object, and is held to this " +
			"question on them.",
	},
}

// covers reports whether this declaration accounts for one finding.
func (d sentDeclaration) covers(finding sentField) bool {
	return d.Package == finding.Package &&
		d.SchemaType == finding.SchemaType &&
		(d.Field == declaredSegment || d.Field == finding.Field)
}

// key names one declaration in a report, which is how a stale one is
// reported.
func (d sentDeclaration) key() string {
	return d.Package + "." + d.SchemaType + "." + d.Field
}

// classifySent attaches the declaration accounting for each finding and names
// the declarations that accounted for none.
//
// A declaration that stops matching is a finding of its own, on the same terms
// as every other declaration table in this repository: the document now
// selects the field, the send is gone, or the schema no longer offers it, and
// in each case the excuse outlives the thing it excused.
func classifySent(declarations []sentDeclaration, found []sentField) (classified []sentField, unused []string) {
	used := map[string]bool{}
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
	// Left nil rather than empty when nothing is stale, so that a check with
	// no stale declaration and one with none to report read the same.
	for _, declaration := range declarations {
		if !used[declaration.key()] {
			unused = append(unused, declaration.key())
		}
	}
	sort.Strings(unused)
	return classified, unused
}

// staleSentDeclarations renders the unused declarations as the lines the run
// reports beside the findings.
func staleSentDeclarations(unused []string) []string {
	stale := make([]string, 0, len(unused))
	for _, key := range unused {
		stale = append(stale, key+" is declared as a field the schema offers and the document leaves out on purpose, and no finding matched it: the document now selects it, the schema no longer offers it, or the send is gone")
	}
	return stale
}
