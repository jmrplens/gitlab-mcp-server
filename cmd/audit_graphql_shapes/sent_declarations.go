package main

import (
	"slices"
	"sort"
)

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
	// categoryExperiment is a field GitLab marks Status: Experiment. GitLab
	// may change or remove an experiment without notice, and it refuses a
	// whole document that names a field it no longer has, so selecting one
	// stakes every action sharing the selection on the experiment rather than
	// only the one field. The pin cannot say it for the reason
	// categoryDeprecated's evidence is prose, and the evidence is the same
	// kind: the field's entry in GitLab's GraphQL API reference.
	categoryExperiment = "experiment-upstream"
	// categoryNewerThanFloor is a field GitLab added after the oldest release
	// the documents sharing the selection are held to. GitLab refuses a whole
	// document that names a field it does not have yet, just as it refuses one
	// naming a field it removed, so selecting it would stop every action
	// sharing the selection on every instance between that floor and the
	// field's release. The pin cannot say it, since it records one release and
	// no history; the evidence is the first versioned GitLab GraphQL reference
	// that lists the field, beside the last that does not.
	categoryNewerThanFloor = "newer-than-release-floor"
	// categorySeparateAction is a collection a caller would page through, or
	// change through mutations of its own, which is a catalog action of its
	// own rather than a list carried inside every object it hangs from.
	categorySeparateAction = "separate-action-not-a-field"
	// categoryPublishedElsewhere is a value this server does publish, under
	// another spelling, as part of another field or through an action the
	// response names, which the automatic same-name match did not find.
	categoryPublishedElsewhere = "published-elsewhere"
	// categoryAffordance is a value the web UI reads to decide which control
	// to draw for the viewer, which says nothing about the object a caller
	// asked for.
	categoryAffordance = "ui-affordance"
	// categoryUnusedIdentifier is a record's own global id that no action of
	// this server takes, so publishing it would hand a caller a handle nothing
	// here accepts.
	categoryUnusedIdentifier = "identifier-no-action-takes"
	// categoryRecursiveShape is a field whose type contains itself, so no
	// document can select the whole of it: any selection stops at a depth
	// chosen arbitrarily and silently drops whatever lies below.
	categoryRecursiveShape = "recursive-shape"
	// categoryWebRendering is a value GitLab computes for its own web
	// interface to draw with: the HTML rendering of a Markdown field the
	// response publishes as Markdown, an icon, the commands a comment box
	// offers, the token an edit form sends back.
	categoryWebRendering = "web-ui-rendering"
	// categoryViewer is a value about the token's own user rather than about
	// the object: what that user may do with it, or an address minted for
	// that user alone.
	categoryViewer = "about-the-viewer-not-the-object"
	// categoryNeverSentHere is a field the schema offers on a type GitLab
	// shares across several kinds of object and never fills for the kind
	// this document reads. The evidence is GitLab's resolver or model, since
	// the pin shares the type and cannot say which kinds fill it.
	categoryNeverSentHere = "never-sent-for-this-object"
	// categoryOutsideSurface is an object of a GitLab feature this server
	// serves no tool for, so surfacing it inside another domain's response
	// would start that domain in the wrong place.
	categoryOutsideSurface = "outside-this-servers-surface"
	// categoryRestated is a field whose value the caller already holds: a
	// mutation payload handing back the argument that named its target, or an
	// object reached again by walking from a child back to the parent the
	// same response carries. Selecting it would publish the same value twice.
	categoryRestated = "restates-a-known-value"
)

// One more category is wanted and is not written down until the first finding
// needs one, because a category nothing uses is a vocabulary rather than a
// decision: tier-gated-above-this-domain for a field GitLab serves only above
// the tier the domain is gated at. It needs prose evidence for the same reason
// categoryDeprecated does: the tier is not in the pin.

// Where the packages these findings are filed against live, spelled once. A
// finding names the package the decoding struct is declared in, so the note
// mutations a shared toolutil wrapper sends are answered under the domain that
// decodes the note and not under the wrapper; the payload around the note is
// the wrapper's own struct, and is answered under toolutilDir, as a shape two
// domains decode through one struct of toolutil's is, once.
const (
	toolsDir    = "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	toolutilDir = "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The two epic packages whose note and discussion declarations below name
// them row after row, spelled once so a reader compares names.
const (
	epicDiscussionsPkg = toolsDir + "/epicdiscussions"
	epicNotesPkg       = toolsDir + "/epicnotes"
)

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

// htmlRenderingReason is why the HTML renderings of a Markdown field are
// left out wherever the field itself is published.
const htmlRenderingReason = "The field is GitLab's HTML rendering of Markdown this response publishes in its source form " +
	"(a note's body, a work item's title and description). GitLab's REST note and issue entities carry the Markdown " +
	"alone, every note and issue tool of this server publishes that, and a caller that wants the page renders the " +
	"Markdown it already has."

// viewerPermissionsReason is why the permissions object GitLab attaches to a
// note, a thread or a work item is left out.
const viewerPermissionsReason = "The object says what the token's own user may do with this note, thread or work item " +
	"(edit it, resolve it, award an emoji), which is a property of the caller and not of the object, and GitLab's REST " +
	"entities for the same objects carry no equivalent. This server answers that question by attempting the action " +
	"and reporting GitLab's refusal, which is authoritative where a permissions snapshot can be stale."

// duoReason is why a Duo Agent Platform object under a note or a work item
// is left out.
const duoReason = "The object is a session or a link of the GitLab Duo Agent Platform, a feature this server serves no " +
	"tool for. Surfacing an agent session inside an epic note or an epic's child would start a Duo domain inside a " +
	"notes tool, with no action to act on what it names."

// epicNoteDeclarations answers the fields of GitLab's Note that a package
// decoding an epic's notes leaves out. The two packages that do so, the flat
// notes view and the threads view, select the same note and leave out the
// same fields for the same reasons, so the answers are written once.
func epicNoteDeclarations(pkg string) []sentDeclaration {
	note := func(field, category, reason string) sentDeclaration {
		return sentDeclaration{Package: pkg, SchemaType: "Note", Field: field, Category: category, Reason: reason}
	}
	return []sentDeclaration{
		note("authorIsContributor", categoryNeverSentHere, "Types::Notes::NoteType resolves it as Note#contributor?, "+
			"which is project&.team&.contributor?(author_id) (app/models/note.rb). An epic is a group-level work item, "+
			"so its notes carry no project and the value is null on every note of an epic; nothing in EE overrides it."),
		note("awardEmoji", categorySeparateAction, "A note's award emoji are a collection of their own, added and "+
			"removed through the awardEmojiAdd and awardEmojiRemove mutations. This server serves them as actions of the "+
			"awardemoji domain for issue, merge request and snippet notes; an epic note's would be actions there too, "+
			"not a list repeated inside every note an epic note tool answers with."),
		note("bodyFirstLineHtml", categoryWebRendering, htmlRenderingReason),
		note("bodyHtml", categoryWebRendering, htmlRenderingReason),
		note("duoCreatedSession", categoryOutsideSurface, duoReason),
		note("duoTriggeredSession", categoryOutsideSurface, duoReason),
		note("duoWorkflowLinks", categoryOutsideSurface, duoReason),
		note("maxAccessLevelOfAuthor", categoryNeverSentHere, "Types::Notes::NoteType resolves it as "+
			"Note#human_max_access, which is project&.team&.human_max_access(author_id) (app/models/note.rb); the EE "+
			"override (ee/app/models/ee/note.rb) answers only for a group wiki note. An epic's notes carry no project and "+
			"are not wiki notes, so the value is null on every note of an epic."),
		note("position", categoryNeverSentHere, "A note's position is the diff line a diff note sits on: "+
			"Types::Notes::NoteType answers it only when the note's position is a Gitlab::Diff::Position, which "+
			"only a diff note on a merge request or a commit carries, so it is null on every note of an epic."),
		note("project", categoryNeverSentHere, "An epic is a group-level work item, so its notes belong to the "+
			"group's namespace and carry no project id, and Types::Notes::NoteType loads project by that id, which "+
			"makes it null on every note of an epic."),
		note("suggestions", categoryNeverSentHere, "Suggestions::CreateService returns at once unless "+
			"Note#supports_suggestion?, which is false for every note but a diff note on a merge request, so no "+
			"suggestion is ever created on a note of an epic and the connection is empty on every one."),
		note("systemNoteIconName", categoryWebRendering, "The name of the icon GitLab's web interface draws "+
			"beside a system note. The note's system flag and its body, GitLab's own sentence for the event, are "+
			"published, as the REST note entity publishes them."),
		note("systemNoteMetadata", categoryWebRendering, "The metadata GitLab's web interface uses to draw a "+
			"system note: the kind of event for its icon and the description version its diff link opens. The REST "+
			"note entity carries none of it, and the note's system flag and body, GitLab's own sentence for the "+
			"event, are published."),
		note("userPermissions", categoryViewer, viewerPermissionsReason),
	}
}

// epicIssueDeclarations answers the fields of the objects the epic children
// query reads that it leaves out: the child work item, its type and its
// labels.
func epicIssueDeclarations() []sentDeclaration {
	pkg := toolsDir + "/epicissues"
	workItem := func(field, category, reason string) sentDeclaration {
		return sentDeclaration{Package: pkg, SchemaType: "WorkItem", Field: field, Category: category, Reason: reason}
	}
	return []sentDeclaration{
		workItem("availableQuickActions", categoryWebRendering, "The quick action commands GitLab's comment box "+
			"offers for the child, which is autocomplete for its own editor. A caller writes a quick action into a "+
			"note body, where GitLab reports what it did, and needs no list of them inside every child row."),
		workItem("commentTemplatesPaths", categoryWebRendering, "The paths of the comment templates GitLab's web "+
			"interface offers in the child's comment box, which are a setting of the editor rather than of the child."),
		workItem("createNoteEmail", categoryViewer, "The address the token's own user can email to comment on "+
			"the child, which carries that user's incoming email token: it is minted per user, identifies nothing "+
			"about the child, and is not a value to repeat in a list other readers of the output may see."),
		workItem("descriptionHtml", categoryWebRendering, htmlRenderingReason),
		workItem("duoWorkflowLinks", categoryOutsideSurface, duoReason),
		workItem("features", categoryNotThisResponse, "The object is the child's widgets in a second shape, one "+
			"field per widget. The row is a child in an epic's list and carries the child's own fields and labels; "+
			"the child's full widget surface is the work item a caller reads with the issue.work_item_get action, "+
			"which publishes it."),
		workItem("lockVersion", categoryWebRendering, "The optimistic-lock counter GitLab's edit form sends back "+
			"so that two edits cannot overwrite each other. Of every mutation the pinned schema declares, only "+
			"WorkItemConvertTaskInput takes it, and no action of this server sends that mutation, so no caller "+
			"could spend it."),
		workItem("name", categoryPublishedElsewhere, "The Todoable interface's name, which Types::TodoableInterface "+
			"resolves as the object's name or, since a work item has none, its title. The title is published on the "+
			"row as title."),
		workItem("namespace", categoryNotThisResponse, referenceStub("group or project a child lives in",
			"gitlab_group and gitlab_project")+" The row names it through the child's full reference, which is "+
			"the path child_project_path takes."),
		workItem("project", categoryNotThisResponse, referenceStub("project a child issue lives in",
			"gitlab_project")+" The row names it through the child's full reference, which is the path "+
			"child_project_path takes."),
		workItem("showPlanUpgradePromotion", categoryViewer, "Whether GitLab's web interface should show the "+
			"token's own user a promotion for a higher plan beside the child, which is marketing addressed to the "+
			"viewer and says nothing about the child."),
		workItem("titleHtml", categoryWebRendering, htmlRenderingReason),
		workItem("userPermissions", categoryViewer, viewerPermissionsReason),
		workItem("webPath", categoryPublishedElsewhere, "The path part of the child's web URL, without the scheme "+
			"and host, which the row already publishes whole as web_url; a second key carrying a substring of the "+
			"first would give a caller nothing to act on that the first does not."),
		{
			Package:    pkg,
			SchemaType: "WorkItemType",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: "The row names the child's type by its name, published as type, because an epic's hierarchy " +
				"lists tasks and child epics beside issues. The rest of the object is the type's definition (its " +
				"widgets, its conversions, where it may be created), which is the same for every child of that type " +
				"and belongs to the work item types a namespace defines rather than to one child.",
		},
		{
			Package:    pkg,
			SchemaType: "Label",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: "The row carries each label as the epic output does: its name under labels, and its id, name, " +
				"color, description, HTML description and text color under label_details. What is left is the " +
				"label's own lifecycle " +
				"(created, updated, archived, locked on merge), which is the labels domain's surface, published by " +
				"its label tools, and says nothing about the child.",
		},
	}
}

// declaredSent holds every field the schema offers that a document leaves out
// on purpose, each with the reason. It is what [auditRun.declarations] carries
// on a real run. The two GraphQL-only security domains and the shapes they
// share keep their answers in [securitySentDeclarations], which is most of the
// table. The branch rules, CI catalog, custom emoji and security attribute and
// category domains keep theirs in [domainSentDeclarations].
var declaredSent = slices.Concat(epicSentDeclarations(), securitySentDeclarations(), domainSentDeclarations()) //nolint:gochecknoglobals // the adjudication table this repository answers with

// epicSentDeclarations answers the epic domains' findings, and those of the
// shared note wrapper every epic note and discussion mutation goes through.
func epicSentDeclarations() []sentDeclaration {
	return slices.Concat([]sentDeclaration{
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
			Package:    epicDiscussionsPkg,
			SchemaType: "UserCore",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason:     userCoreReason,
		},
		{
			Package:    epicNotesPkg,
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
			Package:    epicDiscussionsPkg,
			SchemaType: "WorkItem",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason:     notesAnchorReason("queryListDiscussions", "gitlab_list_epic_discussions"),
		},
		{
			Package:    epicNotesPkg,
			SchemaType: "WorkItem",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason:     notesAnchorReason("queryListWorkItemNotes", "gitlab_epic_note_list"),
		},
		{
			Package:    epicNotesPkg,
			SchemaType: "Note",
			Field:      "discussion",
			Category:   categoryNotThisResponse,
			Reason: "The thread a note sits in is the epic discussions domain's object: its list and its read answer " +
				"each thread with its id and every note in it, and opening one answers with the id GitLab gave it. The " +
				"note tools mirror GitLab's flat note, which carries no thread in the REST note entity either.",
		},
		{
			Package:    epicDiscussionsPkg,
			SchemaType: "Discussion",
			Field:      "createdAt",
			Category:   categoryPublishedElsewhere,
			Reason: "Discussion delegates created_at to its first note (app/models/discussion.rb), and the thread's " +
				"notes are published in order, each with its created_at, so the thread's creation time is the first note's, " +
				"published as notes[0].created_at. Selecting it again costs six in the list query's complexity at a page of " +
				"a hundred, against a limit that query sits 30 under.",
		},
		{
			Package:    epicDiscussionsPkg,
			SchemaType: "Discussion",
			Field:      "noteable",
			Category:   categoryNotThisResponse,
			Reason: "The object a thread is on is the epic the caller named by full_path and epic_iid to reach the " +
				"thread at all. Its fields are the epic surface, which the epic tools publish, and repeating the epic " +
				"inside every thread of its own list would answer a threads call with the epic again.",
		},
		{
			Package:    epicDiscussionsPkg,
			SchemaType: "Discussion",
			Field:      "replyId",
			Category:   categoryPublishedElsewhere,
			Reason: "Discussion#reply_id is its first note's discussion_id, and Discussion#id is the same value unless " +
				"Discussion.override_discussion_id rewrites it, which only OutOfContextDiscussion does, for a commit's " +
				"thread shown on a merge request. A thread on an epic is never one, so reply_id is the thread's id, " +
				"published as id, which is what the reply action takes. Selecting it again costs six in the list query's " +
				"complexity at a page of a hundred, against a limit that query sits 30 under.",
		},
		{
			Package:    epicDiscussionsPkg,
			SchemaType: "Discussion",
			Field:      "truncatedDiffLines",
			Category:   categoryNeverSentHere,
			Reason: "The diff lines a thread was started on: Types::Notes::DiscussionType returns nothing unless the " +
				"thread is a diff discussion, which only a thread on a diff line of a merge request or a commit is. A " +
				"thread on an epic never is one, so the field is null on every thread these documents read.",
		},
		{
			Package:    epicDiscussionsPkg,
			SchemaType: "Discussion",
			Field:      "userPermissions",
			Category:   categoryViewer,
			Reason:     viewerPermissionsReason,
		},
		{
			Package:    toolutilDir,
			SchemaType: "DestroyNotePayload",
			Field:      declaredSegment,
			Category:   categoryNeverSentHere,
			Reason: "The payload type shares its note and quick actions status with the create and update payloads " +
				"through Mutations::Notes::Base, and Mutations::Notes::Destroy answers with neither: its resolver " +
				"returns the errors and nothing else, so both are null on every destroyNote response. The document " +
				"selects the errors, which ExecGraphQLDestroyNote reads.",
		},
	}, epicNoteDeclarations(epicNotesPkg), epicNoteDeclarations(epicDiscussionsPkg), epicIssueDeclarations())
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
