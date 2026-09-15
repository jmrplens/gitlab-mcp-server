package modelcorpus

// promptDeclaration records why one answer literal a case's own text carries is
// not coaching: the case cannot ask for what it asks for without that word.
//
// It is the shape every declaration table in this repository has, and it exists
// for the reason they do. The prompt audit judges a literal by its spelling and
// nothing else, so it cannot tell a case that hands its answer over from one
// whose request is the literal, and the only honest thing to do with the second
// kind is to write down which it is, with the reason, where the next reader can
// disagree.
//
// The key is the case and the literal. The evaluator this replaces keyed on the
// surface as well, because its prompt builders spelled a resource URI
// differently per surface and one sentence of case text was a different literal
// on each; nothing renders a case's text per surface any more, so a per-surface
// key would only ever be three copies of one entry.
type promptDeclaration struct {
	// Case is the case identifier the finding is reported under.
	Case string
	// Value is the literal exactly as the finding reports it.
	Value string
	// Category says what kind of acceptance this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it
	// still holds.
	Reason string
}

// promptCategoryLiteralIsTheRequest is a literal that addresses what the case
// asks the model to do, such as the English name of the thing being acted on.
// Withholding it would not hide the answer; it would change the request into a
// different one, or into one no person would write.
//
// The category set is one entry long on purpose. A literal that addresses the
// thing the case asks for is the only kind of answer a request cannot withhold.
// An instruction to the harness that names the answer is the coaching itself
// rather than an excuse for it, and a value the prompt could state in plainer
// English is a rewrite rather than a declaration: MS-ENV-DEP-2, MT-050 and
// MT-046 all read better without the argument name in them, so they were
// rewritten and are not in this table.
const promptCategoryLiteralIsTheRequest = "the-literal-is-the-request"

// promptDeclarationCategories is the set a category must belong to, so a typo
// cannot invent a second kind of acceptance.
var promptDeclarationCategories = map[string]bool{
	promptCategoryLiteralIsTheRequest: true,
}

// declaredPromptFindings holds every finding in a case's own text the gate
// accepts, each with a category and a reason.
//
// The bar for adding an entry is the bar the entries below met: the case's
// request is unwritable without the literal, and the rewrite that would remove
// it was tried and found to change what the case measures or to produce a
// sentence no person would send.
var declaredPromptFindings = []promptDeclaration{
	{
		Case:     "MS-001",
		Value:    "remote_url",
		Category: promptCategoryLiteralIsTheRequest,
		Reason: `the case asks the model to resolve a git remote URL to its project, and
"remote URL" is what that string is called in English. The argument being named the
same thing is the tool's spelling agreeing with the world's, not the case handing an
answer over: the address itself is in the prompt, so nothing is learned from the name.`,
	},
	{
		Case:     "MS-002",
		Value:    "remote_url",
		Category: promptCategoryLiteralIsTheRequest,
		Reason:   "the same request, inside the pipeline investigation: the prompt states the URL to resolve.",
	},
	{
		Case:     "MS-011",
		Value:    "remote_url",
		Category: promptCategoryLiteralIsTheRequest,
		Reason:   "the same request, before the guided issue flow.",
	},
	{
		Case:     "MT-030",
		Value:    "commit_message",
		Category: promptCategoryLiteralIsTheRequest,
		Reason: `the case asks for a file to be committed with a particular message, and the message
is the request. Every English phrasing of it is "commit message"; the alternatives
tried either drop the message from the request or say it in words no person would use.`,
	},
	{
		Case:     "MT-199",
		Value:    "per_page",
		Category: promptCategoryLiteralIsTheRequest,
		Reason: `the case asks for a page size, which is how a person states one: "5 results per
page". The key compares that 5, so the value is the request and the argument name
happens to be the same two words.`,
	},
	{
		Case:     "MT-207",
		Value:    "per_page",
		Category: promptCategoryLiteralIsTheRequest,
		Reason:   "the same request, for the hundred-release page.",
	},
}

// key names one declaration in a report, which is how a stale one is reported.
func (d promptDeclaration) key() string {
	return d.Case + " " + d.Value
}

// covers reports whether this declaration accounts for one finding.
func (d promptDeclaration) covers(caseID, value string) bool {
	return d.Case == caseID && d.Value == value
}

// promptDeclarationProblem is one declaration the gate refuses to act on,
// because it names a category that does not exist or gives no reason, and so
// could never be judged by the reader it is written for.
func promptDeclarationProblem(declaration promptDeclaration) string {
	if declaration.Case == "" || declaration.Value == "" {
		return declaration.key() + ": names no case or no literal"
	}
	if !promptDeclarationCategories[declaration.Category] {
		return declaration.key() + ": category " + declaration.Category + " is not one of the declared kinds"
	}
	if declaration.Reason == "" {
		return declaration.key() + ": gives no reason"
	}
	return ""
}
