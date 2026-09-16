package modelcorpus

// The retired cases: identifiers the corpus this one replaces declared and
// this one does not, each with the class it failed and the reason.
//
// It exists because an identifier is the only handle anybody has on a case. A
// report from before the move names MT-201; a person reading it needs to be
// able to find out what became of that case, and "it is not in the corpus" is
// not an answer. Writing the reason down here is also what stops a later
// author reviving a retired identifier for something else: retired_test.go
// holds every one of these to being absent from the live corpus and to never
// being reused, which is the allocation rule the corpus already states.
//
// The survivor rule is one sentence: a case survives when every step of its
// key is a tools/call the server registers on the surface under test, its
// result comes from the server or from GitLab, and a recipe can build its
// world on a Docker instance. The first two clauses are what the two
// categories below are; the third is answered by a builder rather than by a
// list, which is why [RetiredNoRecipe] is declared here and used by the step
// that has the evidence.

// The three categories a retirement can have.
const (
	// RetiredBridgeTool is a case whose step called a tool the evaluator
	// invented to ask the MCP **client** for something: a resource, a
	// prompt, a completion, a capability listing. No client exposes those to
	// a model under those names, each client that exposes them at all does
	// it its own way, and whether this server answers resources/read
	// correctly is a deterministic question the end-to-end suite already
	// asks on every push.
	RetiredBridgeTool = "bridge-tool"
	// RetiredSimulatedResult is a case whose step had its result fabricated
	// by the harness rather than returned by the server, so the model
	// reacted to something nothing under test produced.
	RetiredSimulatedResult = "simulated-result"
	// RetiredNoRecipe is a case whose world no builder can raise on a Docker
	// instance. It is declared here and used where the evidence is: the
	// fixture step finds out by trying, and a retirement written before
	// anybody tried would be a decision with no grounds.
	//
	// **Nothing carries it, and that is the answer rather than an omission.**
	// The step that built the worlds tried every recipe this corpus names and
	// raised one for each, so no case was left without a world. A handful of
	// the licensed worlds do name an object a Docker instance cannot hold: an
	// attestation is published by a CI job that attests, a SCIM identity by an
	// identity provider, an enterprise user by a verified domain, a storage
	// move by a second Gitaly storage. None of those retires its case. The
	// world is raised around the object and the identifier it would have had
	// is reserved, so the case still dispatches the action it is about and
	// GitLab answers the not-found, which is the "dispatched, GitLab refused"
	// class the scoring keeps apart from a model failure, and which the
	// end-to-end suite's own scenarios already assert for the same actions.
	// The category stays declared because that judgement can change: a
	// fixture stack that grows an identity provider, or a corpus that grows a
	// case whose world needs one, would use it.
	RetiredNoRecipe = "no-recipe"
)

// retiredCategories is the set a category must belong to, so a typo cannot
// invent a fourth kind of retirement.
var retiredCategories = map[string]bool{
	RetiredBridgeTool:      true,
	RetiredSimulatedResult: true,
	RetiredNoRecipe:        true,
}

// Retirement is one identifier the corpus no longer holds.
type Retirement struct {
	// ID is the case identifier, which stays allocated for ever.
	ID string
	// Category is one of the three above.
	Category string
	// Reason says why, in the words a reader of an old report needs.
	Reason string
}

// retirements is every identifier the port left behind.
//
// Fourteen of them, and they are the whole of the two categories that could be
// decided by reading: five bridge-tool cases in the read partition, five more
// that were the capability partition entire, and four simulated-result cases
// that were the error-recovery partition entire. 272 identifiers were
// declared; 258 survive.
//
// The third category, [RetiredNoRecipe], holds none: the step that built the
// worlds raised one for every recipe this corpus names, and the comment on
// that constant says what it did with the objects a Docker instance cannot
// hold.
var retirements = []Retirement{
	{
		ID:       "MT-201",
		Category: RetiredBridgeTool,
		Reason: `asked for the server's capability listing through gitlab_list_capabilities, a tool the
evaluator built and no client registers. What a server declares at initialize is checked by the
transport end-to-end modules, which need no model.`,
	},
	{
		ID:       "MT-202",
		Category: RetiredBridgeTool,
		Reason: `listed MCP resources through gitlab_list_resources. resources/list is a client method,
not a tool, and the end-to-end suite asks it directly.`,
	},
	{
		ID:       "MT-203",
		Category: RetiredBridgeTool,
		Reason:   "read a gitlab:// resource through gitlab_read_resource, which is the same request to the client.",
	},
	{
		ID:       "MT-210",
		Category: RetiredBridgeTool,
		Reason:   "listed MCP prompts through gitlab_list_prompts; prompts/list is a client method.",
	},
	{
		ID:       "MT-211",
		Category: RetiredBridgeTool,
		Reason:   "fetched one prompt through gitlab_get_prompt; prompts/get is a client method.",
	},
	{
		ID:       "MS-039",
		Category: RetiredBridgeTool,
		Reason: `a capability-discovery workflow whose every step was one of the six bridge tools, so
nothing in it reached this server at all.`,
	},
	{
		ID:       "MS-040",
		Category: RetiredBridgeTool,
		Reason: `read gitlab://tools/project.get to learn an action's schema. That is a request to the
MCP client, and on the rebuilt meta surface it was also the only way a model could see parameter
names, which is why the meta baseline now measures what the default opaque schema mode really
costs instead of hiding it behind a bridged resource read.`,
	},
	{
		ID:       "MS-041",
		Category: RetiredBridgeTool,
		Reason:   "completed an argument value through gitlab_complete; completion/complete is a client method.",
	},
	{
		ID:       "MS-042",
		Category: RetiredBridgeTool,
		Reason:   "combined a prompt fetch with a resource read, both through bridge tools.",
	},
	{
		ID:       "MS-ENT-DYN-10",
		Category: RetiredBridgeTool,
		Reason: `the licensed half of the capability-discovery set: the same bridge tools against an
Enterprise surface, so the license changed nothing about why it could not be scored.`,
	},
	{
		ID:       "MF-001",
		Category: RetiredSimulatedResult,
		Reason: `the harness answered the first call with a transient error of its own making and scored
the retry. Nothing about the server was under test: a real refusal is a legitimate stimulus, and
this was not one.`,
	},
	{
		ID:       "MF-002",
		Category: RetiredSimulatedResult,
		Reason: `the harness answered with a fabricated not-found and scored whether the model carried
on. Asking for something that really is not there is a case somebody may write above the
high-water mark; it is an addition, not this case surviving.`,
	},
	{
		ID:       "MF-003",
		Category: RetiredSimulatedResult,
		Reason: `the harness fed the model a poisoned tool result to see whether it followed the
instructions inside it. The question is worth asking and the answer has to come from something the
server returned, not from a string the harness wrote.`,
	},
	{
		ID:       "MS-ENT-DYN-9",
		Category: RetiredSimulatedResult,
		Reason:   "the licensed error-recovery case, simulated the same way and for the same reason not scorable.",
	},
}

// Retired returns every retired identifier with its category and reason, in
// declaration order. It is what a reader of an old report looks a case up in,
// and it answers about the corpus rather than about any run, so it is
// exported beside [Stimuli] and needs none of [Keys]'s sanction.
func Retired() []Retirement {
	return append([]Retirement(nil), retirements...)
}

// retirementProblem is one retirement the gate refuses to act on, because it
// names a category that does not exist or gives no reason, and so could never
// be judged by the reader it is written for.
func retirementProblem(one Retirement) string {
	if one.ID == "" {
		return "a retirement names no case"
	}
	if !retiredCategories[one.Category] {
		return one.ID + ": category " + one.Category + " is not one of the declared kinds"
	}
	if one.Reason == "" {
		return one.ID + ": gives no reason"
	}
	return ""
}
