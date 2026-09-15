package evaluator

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/config"

// promptDeclaration records why one answer literal a case's own text carries is
// not coaching: the case cannot ask for what it asks for without that word.
//
// It is the shape every declaration table in this repository has, and it exists
// for the reason they do. The prompt audit judges a literal by its spelling and
// nothing else, so it cannot tell a case that hands its answer over from one
// whose request is the literal, and the only honest thing to do with the second
// kind is to write down which it is, with the reason, where the next reader can
// disagree.
type promptDeclaration struct {
	// Surface is the tool surface the finding is rendered on, one of the two
	// make check-eval-prompts audits. It is part of the key because the
	// builder spells a resource URI per surface, so one sentence of case text
	// is a different literal on each.
	Surface string
	// Case is the case ID the finding is reported under.
	Case string
	// Kind is the answer-key kind the literal belongs to.
	Kind promptAuditKind
	// Value is the literal exactly as the finding reports it.
	Value string
	// Category says what kind of acceptance this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it
	// still holds.
	Reason string
}

// promptCategoryLiteralIsTheRequest is a literal that addresses what the case
// asks the model to read, render or complete, such as a resource URI carrying
// an action ID. Withholding it would not hide the answer; it would change the
// request into one with a step the answer key does not score.
//
// The category set is one entry long on purpose. A literal that addresses the
// thing the case asks for is the only kind of answer a request cannot withhold.
// GitLab's own spelling of a setting is not one, since MT-169 and MT-194 both
// ask for secret push protection in prose; and an instruction to the harness
// that names the answer is the coaching itself rather than an excuse for it.
const promptCategoryLiteralIsTheRequest = "the-literal-is-the-request"

// promptDeclarationCategories is the set a category must belong to, so a typo
// cannot invent a second kind of acceptance.
var promptDeclarationCategories = map[string]bool{
	promptCategoryLiteralIsTheRequest: true,
}

// promptDeclarationSurfaces is the set a declaration may name: the surfaces
// make check-eval-prompts audits. A declaration for any other surface could
// never be used and so would never be reported stale.
var promptDeclarationSurfaces = map[string]bool{
	config.ToolSurfaceDynamic: true,
	config.ToolSurfaceMeta:    true,
}

// promptDeclarationCaseSchemaResource is the case that asks a model to read the
// schema resource of the action it then calls, which is why its URI carries an
// action ID on both surfaces.
const promptDeclarationCaseSchemaResource = "MS-040"

// declaredPromptFindings holds every case-site finding the gate accepts, each
// with the surface it is rendered on, a category and a reason.
//
// The bar for adding an entry is the bar the entries below met: the case's
// request is unwritable without the literal, and the rewrite that would remove
// it was tried and found to change what the case measures.
var declaredPromptFindings = []promptDeclaration{
	{
		Surface:  config.ToolSurfaceDynamic,
		Case:     promptDeclarationCaseSchemaResource,
		Kind:     promptLeakAction,
		Value:    "project.get",
		Category: promptCategoryLiteralIsTheRequest,
		Reason: "the case asks the model to read the schema resource of the action it then calls, and " +
			"gitlab://tools/<id> is that resource's address. A request that withheld the ID would be a " +
			"request to find the resource first, which is a read step 2 accepts for any uri and step 3 " +
			"then refuses: a different and flakier case, not a more honest one.",
	},
	{
		Surface:  config.ToolSurfaceMeta,
		Case:     promptDeclarationCaseSchemaResource,
		Kind:     promptLeakTool,
		Value:    "gitlab_project",
		Category: promptCategoryLiteralIsTheRequest,
		Reason: "the same URI, spelled gitlab://tools/gitlab_project.get on this surface, so the tool " +
			"half of the address is the literal here.",
	},
	{
		Surface:  config.ToolSurfaceMeta,
		Case:     promptDeclarationCaseSchemaResource,
		Kind:     promptLeakAction,
		Value:    "get",
		Category: promptCategoryLiteralIsTheRequest,
		Reason:   "the action half of the same address, read as dotted code by the audit.",
	},
}

// key names one declaration in a report, which is how a stale one is reported.
func (d promptDeclaration) key() string {
	return d.Surface + " " + d.Case + " " + string(d.Kind) + " " + d.Value
}

// covers reports whether this declaration accounts for one finding at the case
// site of one audited case.
func (d promptDeclaration) covers(surface, caseID string, finding promptAuditFinding) bool {
	return d.Surface == surface && d.Case == caseID && d.Kind == finding.Kind && d.Value == finding.Value
}

// promptDeclarationProblem is one declaration the gate refuses to act on,
// because it names a surface that is never audited or a category that does not
// exist, and so could never be used and never be reported stale either.
func promptDeclarationProblem(declaration promptDeclaration) string {
	if !promptDeclarationSurfaces[declaration.Surface] {
		return declaration.key() + ": names a surface the audit never renders"
	}
	if !promptDeclarationCategories[declaration.Category] {
		return declaration.key() + ": category " + declaration.Category + " is not one of the declared kinds"
	}
	return ""
}
