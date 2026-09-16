package modelcorpus

import "slices"

// Action is a canonical catalog action ID as data, such as "issue.list".
//
// It is deliberately not harness.ActionID. The harness is built behind the e2e
// tag, and the end-to-end coverage gate credits every harness.ActionID
// constant it finds to the e2e ratchet and holds it to a runtime placement; a
// model case is neither a scenario nor placed, so borrowing that type would
// make every case look like an end-to-end scenario to a gate that has no way
// to tell otherwise. The runner converts at the call site, and types_test.go
// checks every Action here against the catalog at Ultimate.
type Action string

// Surface names a tool surface as data, for the same reason [Action] is not
// the harness's own type: this package is untagged and cannot import the
// harness. The runner converts to harness.Surface at the call site.
type Surface string

// The three surfaces a case can run on.
const (
	// SurfaceDynamic is gitlab_find_action plus gitlab_execute_action, the
	// server's default surface.
	SurfaceDynamic Surface = "dynamic"
	// SurfaceMeta is one dispatcher tool per catalog group, the operation
	// traveling in an action argument.
	SurfaceMeta Surface = "meta"
	// SurfaceIndividual is one registered tool per action.
	SurfaceIndividual Surface = "individual"
)

// Surfaces returns the three surfaces in publication order. It is what the
// prompt audit renders every stimulus on and what a report iterates.
func Surfaces() []Surface {
	return []Surface{SurfaceDynamic, SurfaceMeta, SurfaceIndividual}
}

// Tier is the licensing tier an instance must have for a case to run, as data
// rather than as internal/edition.Tier, which this package may not import.
type Tier string

// The three tiers, spelled as internal/edition spells them.
const (
	TierFree     Tier = "free"
	TierPremium  Tier = "premium"
	TierUltimate Tier = "ultimate"
)

// Case is one stimulus and its key, held apart by the two accessors.
//
// Prompt is a text/template rendered over the recipe's facts and over nothing
// else: a value a case needs is a fact the recipe produces, never a constant
// written into the text, because a text carrying the value one run of one
// fixture happened to have is a case that silently stops being about anything
// the next time the fixture changes.
type Case struct {
	// ID is the case identifier, allocated once and never reused.
	ID string
	// Prompt is the stimulus, a template over {{ .Facts.<key> }}.
	Prompt string
	// Recipe names the world this case runs in.
	Recipe Recipe
	// Needs is what the instance must offer for the case to run at all.
	Needs Needs
	// Surfaces restricts the case to some of the three, with the reason.
	Surfaces Restrict
	// key is the answer. Unexported on purpose: it is reachable only
	// through [Keys], and the boundary test names every package allowed to
	// call that.
	key Key
}

// Needs is what a case asks of the instance, never of the catalog: what the
// server serves is decided by the surface under test, not by a need.
type Needs struct {
	// Tier is the lowest licensing tier that can run this case. Empty means
	// Free; [Needs.MinimumTier] is the reader.
	Tier Tier
	// Runner requires a CI runner able to pick a job up.
	Runner bool
	// Admin requires a token whose user administers the instance.
	Admin bool
	// FixtureService requires the fixture HTTP service the Docker stack
	// runs, which answers what GitLab cannot be made to produce
	// deterministically.
	FixtureService bool
}

// MinimumTier returns the tier this case needs, reading the empty tier as
// Free so that the great majority of cases, which need no license, can leave
// the field out.
func (n Needs) MinimumTier() Tier {
	if n.Tier == "" {
		return TierFree
	}
	return n.Tier
}

// Restrict is a case that cannot run on every surface, with the reason a
// report publishes beside it.
//
// It exists because a case that runs on one surface is a declaration, and a
// runner that special-cases an identifier is a declaration hidden in code.
// types_test.go refuses Only without a Reason, which is the rule the
// harness's own OnSurfaces already states, and the runner passes both to it.
type Restrict struct {
	Only   []Surface
	Reason string
}

// Runs reports whether a case restricted this way runs on one surface. The
// empty restriction runs on all three.
func (r Restrict) Runs(surface Surface) bool {
	return len(r.Only) == 0 || slices.Contains(r.Only, surface)
}

// Key is the answer: the steps in the order they must dispatch, each with the
// arguments whose values are compared.
type Key struct {
	Steps []Step
}

// Step is one tools/call the key expects, named canonically.
//
// Exactly one of Action and Standalone is set. A catalog action is projected
// to each surface's own spelling by the harness; a standalone tool
// (gitlab_discover_project, the gitlab_interactive_* flows) is registered
// outside the catalog under one name on every surface, so it is named here as
// that name.
type Step struct {
	// Action is the canonical catalog action this step must dispatch.
	Action Action
	// Standalone is the registered name of a tool outside the catalog.
	Standalone string
	// Args are the arguments whose values are compared, with where each
	// one's right value comes from. An argument the case does not pin is
	// simply absent: comparing a value the model was never given is the
	// defect this shape exists to end.
	Args []Arg
	// Optional marks a step a completed attempt need not have reached.
	Optional bool
	// Produces names the fields of this step's result that a later step's
	// argument may bind to with [Truth.Produced].
	Produces []string
}

// Arg is one argument of a step, and where its right value comes from.
type Arg struct {
	Name     string
	Truth    Truth
	Required bool
}

// Truth is where an argument's right value comes from. It is a closed sum of
// four, so a scorer can be exhaustive and the corpus gate can reject a case
// that binds a name no recipe produces.
//
// Exactly one is set. A value expressed in the prompt as a verb rather than as
// a value ("pause the runner", "resolve the discussion") is deliberately not
// one of them: it is checked by the recipe's own verification against GitLab
// after the run, because a Literal the prompt never spells is a value the
// model was never given, and the corpus gate refuses one.
type Truth struct {
	// Fact names a recipe fact. Every spelling the recipe accepts for it
	// matches, which is what lets an argument that takes a project's
	// numeric ID be satisfied by its path.
	Fact string
	// Literal is a value the prompt states verbatim, compared exactly.
	Literal string
	// Produced binds to a field of an earlier step's result, read from the
	// record the run wrote.
	Produced Ref
	// Authored marks a value the model writes, such as a title or a note
	// body derived from the request. It is reported as authored and never
	// compared.
	Authored bool
}

// Ref names a field of an earlier step's result.
type Ref struct {
	// Step is the 1-based position of the earlier step in the key.
	Step int
	// Field is the path into that step's structured result, segments
	// separated by dots.
	Field string
}

// IsZero reports whether this reference names nothing, which is what every
// truth that is not a [Truth.Produced] carries.
func (r Ref) IsZero() bool {
	return r.Step == 0 && r.Field == ""
}

// The constructors the case files are written with. They exist so that a case
// reads as one line of data rather than as a nest of composite literals, and
// so that the one common argument of all, the project a case works in, is
// spelled once here instead of seventy times there.

// step is one catalog action and the arguments whose values are compared.
func step(action Action, args ...Arg) Step {
	return Step{Action: action, Args: args}
}

// standalone is one tool registered outside the catalog, named as it is
// registered on every surface.
func standalone(tool string, args ...Arg) Step {
	return Step{Standalone: tool, Args: args}
}

// req is an argument a completed attempt must have sent.
func req(name string, truth Truth) Arg {
	return Arg{Name: name, Truth: truth, Required: true}
}

// opt is an argument compared when it was sent and not missed when it was not.
func opt(name string, truth Truth) Arg {
	return Arg{Name: name, Truth: truth}
}

// project is the argument almost every step takes: the project the case works
// in, satisfied by either spelling the recipe promises for it.
func project() Arg {
	return req("project_id", fact(FactProjectPath))
}

// group is the argument the group-scoped actions take: the group a case works
// in, satisfied by either spelling the recipe promises for it.
func group() Arg {
	return req("group_id", fact(FactGroupPath))
}

// fullPath is what the GraphQL-backed group actions call the same thing. It is
// spelled apart from [group] rather than aliased to it, because the two names
// are different properties of different input schemas and the corpus gate
// checks each against the schema of the action it sits on.
func fullPath() Arg {
	return req("full_path", fact(FactGroupPath))
}

// fact binds an argument to a value the recipe produced.
func fact(key string) Truth {
	return Truth{Fact: key}
}

// produced binds an argument to a field of an earlier step's result, by the
// step's 1-based position in the key.
func produced(step int, field string) Truth {
	return Truth{Produced: Ref{Step: step, Field: field}}
}

// literal binds an argument to a value the prompt states verbatim.
func literal(value string) Truth {
	return Truth{Literal: value}
}

// authored marks a value the model composes, which is reported as authored
// and never compared.
func authored() Truth {
	return Truth{Authored: true}
}
