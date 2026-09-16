//go:build e2e

// run_test.go is the run: every model, every case the selection admits, on
// every surface the run was configured for, against the real binary and a real
// GitLab.
//
// What fails this test and what does not is the decision the rest of it hangs
// on. An attempt fails the run only when the run could not put the case to the
// model or could not hear the answer: a world that would not build, a stimulus
// that would not render, a session that would not start, a provider that would
// not answer after its retries. What the model did is never a failure, because
// it is the measurement, and a test that failed on a wrong argument would be a
// test whose exit code is a score.
//
// The one exception is the fake that replays the corpus key. It is not a model:
// anything short of a completed attempt whose checks passed is this
// repository's own defect, so a run of it is a gate on the pipe rather than a
// measurement of anybody. Its rows are refused by the publisher for the same
// reason.

package modeleval

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// TestModelEval runs the corpus against the configured models.
//
// It skips when no model is named, which is the ordinary state of this package:
// nothing here runs in CI, a run costs money at a provider, and
// `go test ./test/e2e/modeleval/` has to be a run of the offline halves that
// reaches no instance. Every refusal that can be made before a request is made
// before one, so a misspelled surface or an unpriced model costs a message
// rather than a bill.
func TestModelEval(t *testing.T) {
	cfg, err := loadRunConfig()
	if err != nil {
		t.Fatalf("reading the run's configuration: %v", err)
	}
	if !modelsNamed() {
		t.Skipf("%s names no model, so there is nobody to ask", settingModels)
	}
	specs, err := configuredModels()
	if err != nil {
		t.Fatalf("reading %s: %v", settingModels, err)
	}
	if consentErr := consentToSpend(specs, cfg.Spend); consentErr != nil {
		t.Fatal(consentErr)
	}
	prices, err := resolvePrices(specs, cfg.Unpriced)
	if err != nil {
		t.Fatal(err)
	}

	selected := selectedStimuli(cfg.Cases)
	if missing := unselectedCases(cfg.Cases, selected); len(missing) > 0 {
		t.Fatalf("%s names %s, which no stimulus answered to", settingCases, strings.Join(missing, ", "))
	}
	if len(selected) == 0 {
		t.Skip("the corpus admitted no case, so there is nothing to ask")
	}

	adapters, err := buildAdapters(specs)
	if err != nil {
		t.Fatal(err)
	}

	run := &runner{
		cfg:      cfg,
		prices:   prices,
		record:   newRecorder(),
		spend:    newBudget(cfg.BudgetUSD),
		tools:    newToolCache(),
		scripted: newScriptedSessions(),
		actions:  newIdentifiers(),
		slots:    make(chan struct{}, cfg.Parallel),
	}
	run.record.describeRun(cfg, specs, prices)
	t.Logf("%d case(s) x %d model(s) x %d surface(s), repeated %d time(s); "+
		"budget %s, individual slice %d tools",
		len(selected), len(specs), len(cfg.Surfaces), cfg.Repeat, budgetText(cfg.BudgetUSD), cfg.Slice)

	// The run's own Env, opened before any case can be skipped.
	//
	// Everything that names a run is read off an Env: the run identifier every
	// attempt id is built from, the edition and version of the instance, and
	// the tier a row is published under. A case skipped inside harness.New for
	// a need this instance does not meet writes its lines from here rather
	// than from an Env of its own, so a run whose first case is skipped would
	// otherwise write identifiers with an empty run in them, and a run whose
	// every case is skipped would leave a run line with no instance on it at
	// all, which is unpublishable. It costs a probe the run was going to make
	// anyway: there is a model and there is a case, so this run reaches the
	// instance.
	env := harness.New(t)
	run.record.noteRuntime(env)

	run.openScripted(t, env, adapters, selected)

	for _, adapter := range adapters {
		t.Run(modelName(adapter.Spec().String()), func(t *testing.T) {
			run.askModel(t, adapter, selected)
		})
	}
}

// runner is one run's shared state: what it was configured with, what it has
// spent, and the sessions and readings it opens once and reuses.
type runner struct {
	cfg      runConfig
	prices   map[string]*modelrecord.Price
	record   *recorder
	spend    *budget
	tools    *toolCache
	scripted *scriptedSessions
	actions  *identifiers
	// slots bounds how many attempts of one model are in flight.
	//
	// One channel and not one per model, because the models never overlap: a
	// model's subtest does not itself run in parallel, so the testing package
	// finishes its parallel children before the next model's subtest begins.
	slots chan struct{}
}

// openScripted starts the sessions whose clients answer elicitation, before
// any attempt runs.
//
// Up front, and from the test that owns the Env they hang off, for two
// reasons. A scripted session outlives the case that first needed one, so
// opening it inside a case's subtest would close it when that case ended. And
// opening a session can fail the test, which must happen on this goroutine:
// attempts may run in parallel, and a failure raised from one of those against
// the parent's Env would abort the wrong test.
//
// The Env is the run's own rather than one of this function's, because a run
// with no interactive case still needs one: the identifiers and the instance
// facts are read off it before any case can be skipped.
func (r *runner) openScripted(
	t *testing.T,
	env *harness.Env,
	adapters []provider.Provider,
	selected []modelcorpus.Stimulus,
) {
	t.Helper()

	var wanted []harness.Surface
	for _, one := range selected {
		if !Interactive(one.Recipe) {
			continue
		}
		for _, surface := range surfacesFor(one, r.cfg.Surfaces) {
			if !slices.Contains(wanted, surface) {
				wanted = append(wanted, surface)
			}
		}
	}
	if len(wanted) == 0 {
		return
	}

	for _, adapter := range adapters {
		for _, surface := range wanted {
			if _, err := r.scripted.Open(env, r.cfg, surface, adapter.Spec().String()); err != nil {
				t.Fatalf("opening a scripted session: %v", err)
			}
		}
	}
}

// askModel puts every selected case to one model.
func (r *runner) askModel(t *testing.T, adapter provider.Provider, selected []modelcorpus.Stimulus) {
	t.Helper()
	for _, one := range selected {
		for repeat := 1; repeat <= r.cfg.Repeat; repeat++ {
			t.Run(attemptName(one.ID, repeat, r.cfg.Repeat), func(t *testing.T) {
				if r.cfg.Parallel > 1 {
					t.Parallel()
				}
				r.slots <- struct{}{}
				defer func() { <-r.slots }()
				r.askCase(t, adapter, one, repeat)
			})
		}
	}
}

// attemptName names one case's subtest, adding the repeat only when there is
// more than one: a run that asked once should not read as though it asked a
// series.
func attemptName(id string, repeat, repeats int) string {
	if repeats <= 1 {
		return id
	}
	return fmt.Sprintf("%s/r%d", id, repeat)
}

// askCase opens the Env one case runs in and puts it on each surface.
//
// The lock is declared here and nowhere else. A case whose world answers
// elicitation shares one scripted session with every other attempt of the same
// model, and a responder is handed an elicitation request that names no
// attempt, so two such attempts overlapping would have one answered with the
// other's facts. It names the model rather than the model and the surface the
// session is keyed on, because the surface is not chosen yet: the harness gives
// each surface's subtest an Env built with no options, so this is the outermost
// point that can declare one, and a per-model lock is a superset of the pair.
func (r *runner) askCase(
	t *testing.T,
	adapter provider.Provider,
	one modelcorpus.Stimulus,
	repeat int,
) {
	t.Helper()

	needs, err := needsOf(one.Needs)
	if err != nil {
		t.Fatalf("case %s: %v", one.ID, err)
	}
	options := []harness.Option{harness.Needs(needs...)}
	if Interactive(one.Recipe) {
		options = append(options, harness.Locks(scriptedLock(adapter.Spec().String())))
	}

	// Registered before the Env and therefore run after it. A case whose needs
	// this instance does not meet is skipped inside harness.New, which returns
	// nothing this function can write a line from, and a record that could not
	// tell a licensed case absent on a community instance from a case nobody
	// wrote is the reason the evaluator this replaces published a coverage
	// figure that meant nothing.
	opened := false
	t.Cleanup(func() {
		if opened || !t.Skipped() {
			return
		}
		r.recordSkip(t, adapter.Spec().String(), one, repeat, needs)
	})

	env := harness.New(t, options...)
	opened = true
	r.record.noteRuntime(env)

	surfaces := surfacesFor(one, r.cfg.Surfaces)
	if len(surfaces) == 0 {
		env.Skipf("case %s runs on %v and this run measures %v", one.ID, one.Surfaces.Only, r.cfg.Surfaces)
		return
	}

	attempt := func(env *harness.Env, surface harness.Surface) {
		r.attempt(env, adapter, one, surface, repeat)
	}
	if slices.Equal(surfaces, harness.AllSurfaces()) {
		harness.EachSurface(env, attempt)
		return
	}
	harness.OnSurfaces(env, narrowingReason(one, r.cfg.Surfaces), surfaces, attempt)
}

// recordSkip writes down the attempts a case's declared needs kept from
// running, one per surface it would have run on.
//
// Per surface rather than one for the case, so a report reads a skip at the
// grain it reads every other attempt at, and the denominator of a column is
// the same shape whether the case ran or not.
func (r *runner) recordSkip(
	reporter modelrecord.Reporter,
	spec string,
	one modelcorpus.Stimulus,
	repeat int,
	needs []harness.Need,
) {
	named := make([]string, 0, len(needs))
	for _, need := range needs {
		named = append(named, need.String())
	}
	reason := "this instance does not meet the case's needs: " + strings.Join(named, ", ")
	if len(named) == 0 {
		reason = "the case was skipped before it opened an Env"
	}

	runID := r.record.RunID()
	for _, surface := range surfacesFor(one, r.cfg.Surfaces) {
		r.record.writeAttempt(reporter, &modelrecord.Attempt{
			ID:      attemptID(runID, one.ID, spec, surface, repeat),
			Case:    one.ID,
			Model:   spec,
			Surface: surface.String(),
			Repeat:  repeat,
			EndedBy: modelrecord.EndedSkipped,
			Reason:  reason,
		}, nil, nil, nil)
	}
}

// scriptedLock names the shared state one model's interactive attempts
// contend for: the responder of its scripted sessions.
func scriptedLock(spec string) harness.Lock {
	return harness.Lock("modeleval-scripted/" + spec)
}

// narrowingReason says why a case is not run on all three surfaces.
//
// The case's own reason comes first, since it is the one a report should
// publish; a case that runs everywhere and a run that measures less than
// everywhere is the run's own narrowing and says so.
func narrowingReason(one modelcorpus.Stimulus, configured []harness.Surface) string {
	if reason := restrictionReason(one); restricted(one) && reason != "" {
		return reason
	}
	return fmt.Sprintf("this run measures %v, from %s", configured, settingSurfaces)
}

// attempt is one case, put to one model, on one surface.
//
// The world is built here rather than on the parent, inside the surface's own
// subtest, because a case that changes what it was given must get its own copy
// per surface: built once on the parent, the first surface would consume the
// fixture the other two need. A case declaring the shared read-only world
// stands on the fixture library's digest-checked copy, so a mutating case
// wrongly declared to run there fails the run rather than corrupting a later
// attempt.
func (r *runner) attempt(
	env *harness.Env,
	adapter provider.Provider,
	one modelcorpus.Stimulus,
	surface harness.Surface,
	repeat int,
) {
	t := env.T
	spec := adapter.Spec().String()
	id := attemptID(env.RunID(), one.ID, spec, surface, repeat)
	line := &modelrecord.Attempt{
		ID: id, Case: one.ID, Model: spec, Surface: surface.String(), Repeat: repeat,
	}

	// Registered before anything that can end this subtest, and therefore run
	// after all of it.
	//
	// Every step from here to the conversation ends the attempt with t.Fatalf
	// when it cannot go on, and a Fatalf writes no line: the attempt would be
	// absent from the record with nothing saying it was ever made, and a
	// report cannot tell an attempt the run could not put from a case nobody
	// asked. That is the silent denominator this record exists to remove, so
	// what the failure did not write, this does.
	progress := &attemptProgress{stage: "opening the attempt"}
	t.Cleanup(func() { r.noteHarnessError(t, t.Failed(), progress, *line) })

	if r.spend.Exhausted() {
		line.EndedBy = modelrecord.EndedSkipped
		line.Reason = fmt.Sprintf("the run's budget of %s was spent before this attempt began",
			budgetText(r.cfg.BudgetUSD))
		r.record.writeAttempt(t, line, nil, nil, nil)
		progress.written = true
		env.Skipf("the run's budget of %s is spent", budgetText(r.cfg.BudgetUSD))
		return
	}

	progress.at("building the world the case runs in")
	world, known := BuildWorld(env, one.Recipe)
	if !known {
		progress.failf(t, "case %s names recipe %q, which this package cannot build", one.ID, one.Recipe)
	}
	progress.at("rendering the stimulus")
	sent, err := renderStimulus(one, world)
	if err != nil {
		progress.failf(t, "rendering the stimulus: %v", err)
	}
	line.Facts = sent.Facts
	line.Stimulus = sent.Text

	progress.at("opening the session this attempt talks to")
	session, tools := r.sessionFor(env, one, surface, spec)
	sessionLine := r.record.describeSession(session, r.cfg)
	line.Session = sessionLine.Label
	// The digest is of what the session serves, not of what this attempt was
	// shown, and on the individual surface those differ: the slice is chosen
	// per case, so a digest of one attempt's list would be one case's slice
	// standing for the row. What the digest is for is unaffected, since a
	// provider that rewrites a schema rewrites it in whichever list it
	// receives, and what each attempt saw is still exactly determined by the
	// row: this digest, the slice size beside it, and the corpus digest that
	// fixes the case IDs the slices are seeded from.
	r.record.noteToolDigest(sessionLine.Label, spec, adapter.ToolDigest(tools))

	progress.at("reading the catalog a call is resolved against")
	requested, err := r.actions.For(session.Tier(), surface)
	if err != nil {
		progress.failf(t, "reading the catalog a call is resolved against: %v", err)
	}

	progress.at("choosing the tools this attempt is shown")
	shown, err := r.show(one, surface, session.Tier(), tools)
	if err != nil {
		progress.failf(t, "choosing the tools this attempt is shown: %v", err)
	}
	if shown.Sliced {
		t.Logf("%s on %s: shown %s", one.ID, surface, shown.Summary(len(tools)))
	}

	if world.Respond != nil {
		progress.at("lending the scripted session this attempt's answers")
		defer r.lendResponder(t, world, surface, spec)()
	}

	progress.at("reading how long this attempt's conversation may go on")
	steps, hasSteps := modelcorpus.StepCount(one.ID)
	if !hasSteps {
		progress.failf(t, "the corpus has no case %s", one.ID)
	}
	talk := &conversation{
		adapter:   adapter,
		dispatch:  sendThrough(session),
		tools:     shown.Tools,
		contract:  r.contractFor(t, surface),
		stimulus:  sent,
		caseID:    one.ID,
		surface:   surface,
		attempt:   id,
		cap:       turnCap(steps, surface),
		requested: requested,
		charge:    func(usage modelrecord.Usage) { r.spend.charge(usage, r.prices[spec]) },
	}
	progress.at("putting the case to the model")
	outcome := talk.run(env.Ctx)

	progress.at("checking against GitLab what the model did")
	checks := r.verify(env, world, one, id)
	line.EndedBy = outcome.EndedBy
	line.Reason = outcome.Reason
	r.record.writeAttempt(t, line, outcome.Turns, outcome.Calls, checks)
	progress.written = true

	t.Logf("%s on %s: %s after %d turn(s) and %d call(s)%s",
		one.ID, surface, outcome.EndedBy, len(outcome.Turns), len(outcome.Calls), detailOf(outcome.Reason))
	r.logVerdict(t, *line, *sessionLine, outcome, checks)
	r.judgeAttempt(t, adapter.Spec(), outcome, checks)
}

// attemptProgress is how far one attempt got and what stopped it, which is all
// the line a failed attempt leaves behind can say.
//
// The testing package keeps a Fatalf's message to itself, so a cleanup can
// read that the subtest failed and nothing about why. This is what the attempt
// wrote down as it went, so the line says what the run was doing rather than
// only that something went wrong.
type attemptProgress struct {
	// stage is what the attempt was about to do, updated as it goes.
	stage string
	// detail is the message of a failure this file raised itself, empty for
	// one raised inside the harness, a recipe or the corpus.
	detail string
	// written says an attempt line has already reached the shard, so nothing
	// more is owed.
	written bool
}

// at records what the attempt is about to do.
func (p *attemptProgress) at(stage string) { p.stage = stage }

// failf records why the run could not make this attempt and ends the subtest.
//
// The detail is recorded before the Fatalf and not after, because there is no
// after: t.Fatalf does not return.
func (p *attemptProgress) failf(t *testing.T, format string, args ...any) {
	t.Helper()
	p.detail = fmt.Sprintf(format, args...)
	t.Fatalf("%s", p.detail)
}

// reason spells why an attempt ended for the line that says it did.
func (p *attemptProgress) reason() string {
	if p.detail != "" {
		return p.detail
	}
	return "the run failed while " + p.stage + ", and said why only to the test log"
}

// noteHarnessError writes the attempt line an attempt leaves behind when the
// run itself could not make it.
//
// It is the write side of [modelrecord.EndedHarnessError], which the judgement
// below already treats as the run's own failure: without this nothing ever
// produced that ending, so the constant was read and never written and every
// attempt the run could not put vanished from the record.
func (r *runner) noteHarnessError(
	reporter modelrecord.Reporter,
	failed bool,
	progress *attemptProgress,
	line modelrecord.Attempt,
) {
	if progress.written || !failed {
		return
	}
	line.EndedBy = modelrecord.EndedHarnessError
	line.Reason = progress.reason()
	r.record.writeAttempt(reporter, &line, nil, nil, nil)
}

// logVerdict scores the attempt that just ended and puts the verdict in the
// log.
//
// It is for the person watching a run, who otherwise reads an ending and a
// call count and cannot tell an attempt that did the right thing from one that
// completed doing the wrong one. Nothing is written to the record: the record
// stays observation and a report re-scores it from the corpus at HEAD, so a
// scoring rule corrected later re-reads every past run.
//
// The key is resolved by the scorer, which is a sanctioned reader of one; this
// package is not, and never learns what the answer was. The verdict is
// computed after the conversation has ended, so nothing a model was shown can
// be derived from it.
//
// A verdict that cannot be computed is logged and never failed. It is a
// convenience for a reader, the record already holds everything it was
// computed from, and failing a paid attempt over the log line it printed
// afterwards would throw away the measurement.
func (r *runner) logVerdict(
	t *testing.T,
	line modelrecord.Attempt,
	session modelrecord.Session,
	outcome conversationResult,
	checks []*modelrecord.Verify,
) {
	t.Helper()

	verdict, err := modelscore.ScoreCase(modelscore.Attempt{
		Run:      r.record.runLine(),
		Session:  session,
		Line:     line,
		Turns:    valuesOf(outcome.Turns),
		Calls:    valuesOf(outcome.Calls),
		Verifies: valuesOf(checks),
	})
	if err != nil {
		t.Logf("%s on %s: no live verdict: %v", line.Case, line.Surface, err)
		return
	}
	t.Logf("%s on %s: verdict %s", line.Case, line.Surface, verdictText(verdict))
}

// verdictText is one verdict on one line.
//
// The step count is what a reader wants first and the outcome is what they
// want to act on, so both are there whatever happened; the rest is only said
// when it has something to say, since "0 discovery call(s)" on every row of a
// meta run is a line nobody reads.
func verdictText(verdict modelscore.Verdict) string {
	complete := 0
	for _, step := range verdict.Steps {
		if step.Complete {
			complete++
		}
	}
	text := fmt.Sprintf("%s, %d/%d step(s)", verdict.Outcome, complete, len(verdict.Steps))
	if verdict.Discovery > 0 {
		text += fmt.Sprintf(", %d discovery call(s)", verdict.Discovery)
	}
	if verdict.InvalidParams > 0 {
		text += fmt.Sprintf(", %d refused for arguments", verdict.InvalidParams)
	}
	if !verdict.Verified {
		text += ", the check against GitLab failed"
	}
	return text + detailOf(verdict.Reason)
}

// valuesOf is the record's own lines as the scorer takes them: a run holds
// pointers, because a writer takes each line as one, and an attempt is scored
// from values.
func valuesOf[T any](lines []*T) []T {
	values := make([]T, 0, len(lines))
	for _, line := range lines {
		values = append(values, *line)
	}
	return values
}

// lendResponder gives the scripted session this attempt's answers and returns
// the function that takes them back.
func (r *runner) lendResponder(
	t *testing.T,
	world World,
	surface harness.Surface,
	spec string,
) func() {
	t.Helper()

	scripted, open := r.scripted.Lookup(surface, spec)
	if !open {
		t.Fatalf("no scripted session for %s on %s, and this case's world answers elicitation", spec, surface)
		return func() {}
	}
	return scripted.lend(world.Respond)
}

// sessionFor returns the server this attempt talks to and the tools it is
// shown.
//
// An interactive case talks to the scripted session for its model and surface,
// which was opened before any attempt ran; every other case talks to the
// ordinary session for the surface, which the harness pools by shape so that
// every attempt on one surface of one run shares a child.
func (r *runner) sessionFor(
	env *harness.Env,
	one modelcorpus.Stimulus,
	surface harness.Surface,
	spec string,
) (*harness.Session, []provider.Tool) {
	env.T.Helper()

	if Interactive(one.Recipe) {
		if scripted, open := r.scripted.Lookup(surface, spec); open {
			return scripted.Session(), scripted.Tools()
		}
		env.T.Fatalf("no scripted session for %s on %s", spec, surface)
		return nil, nil
	}

	session := openSession(env, r.cfg, surface)
	tools, err := r.tools.Tools(session)
	if err != nil {
		env.T.Fatalf("reading what the %s session serves: %v", surface, err)
		return nil, nil
	}
	return session, tools
}

// show returns the tools this attempt puts in front of the model.
//
// On dynamic and meta it is everything the session serves. On individual it is
// a slice, because the served list does not fit a request there, and the slice
// is built around the domains the case's key touches: the corpus hands those
// over through a door of its own, which is the one thing about an answer that
// reaches a model and is why an individual row is published as a comparison
// class of its own.
//
// A case the corpus does not have is a failure and not an empty domain list. An
// attempt shown a slice of pure distractors would end without completing, and a
// row cannot tell that from a model that chose badly.
func (r *runner) show(
	one modelcorpus.Stimulus,
	surface harness.Surface,
	tier edition.Tier,
	served []provider.Tool,
) (toolSlice, error) {
	if budgetFor(surface, r.cfg.Slice) < 1 {
		// Nothing is chosen here, so nothing about the answer is read: a
		// surface whose list fits a request is shown whole, which is the same
		// list sliceTools returns for a budget of none.
		return toolSlice{Tools: slices.Clone(served)}, nil
	}
	domains, known := modelcorpus.Domains(one.ID)
	if !known {
		return toolSlice{}, fmt.Errorf("the corpus has no case %s", one.ID)
	}
	domainOf, err := r.actions.Domains(tier, surface)
	if err != nil {
		return toolSlice{}, err
	}
	return shownTools(served, surface, one.ID, domains, r.cfg.Slice, domainOf), nil
}

// contractFor returns the surface's own introduction, failing the test when
// there is none: a model given no contract is a model told nothing about the
// surface it was handed.
func (r *runner) contractFor(t *testing.T, surface harness.Surface) string {
	t.Helper()

	contract, err := contractFor(surface)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return contract
}

// verify asks the recipe whether the change the case was for actually
// happened.
//
// It is the half of the answer no tool call can give: a model that created the
// right thing through the right action may still have created it wrong, and the
// recipe that built the world is what knows how to look. A recipe with no check
// writes no line, rather than a line saying it passed.
func (r *runner) verify(
	env *harness.Env,
	world World,
	one modelcorpus.Stimulus,
	attempt string,
) []*modelrecord.Verify {
	if world.Verify == nil {
		return nil
	}
	check := &modelrecord.Verify{Attempt: attempt, Name: string(one.Recipe), Passed: true}
	if err := world.Verify(env); err != nil {
		check.Passed = false
		check.Detail = err.Error()
	}
	return []*modelrecord.Verify{check}
}

// judgeAttempt fails the test for the endings that are this side's fault, and
// for nothing the model did.
//
// The three it fails on are the three that say the run could not put the case
// or could not hear the answer. Over budget, malformed, a turn with no tool
// call and every wrong argument are the measurement, and the record is where
// they are reported.
//
// The fake that replays the key is judged as the pipe rather than as a model,
// since every step it takes is the corpus's own answer: an attempt of it that
// did not complete, or whose check against GitLab failed, is a defect here.
// The reporter is the part of the test a judgement needs, so the judgement
// itself can be driven by a table: a test that checked this by letting it fail
// would have no way to pass.
func (r *runner) judgeAttempt(
	reporter modelrecord.Reporter,
	spec provider.Spec,
	outcome conversationResult,
	checks []*modelrecord.Verify,
) {
	switch outcome.EndedBy {
	case modelrecord.EndedHarnessError, modelrecord.EndedProviderError:
		reporter.Errorf("the run could not complete this attempt: %s: %s", outcome.EndedBy, outcome.Reason)
		return
	}
	if !replaysTheKey(spec) {
		return
	}
	if outcome.EndedBy != modelrecord.EndedCompleted {
		reporter.Errorf("%s replays the corpus key and ended %s: %s", spec, outcome.EndedBy, outcome.Reason)
	}
	for _, check := range checks {
		if !check.Passed {
			reporter.Errorf("%s replays the corpus key and the %s check failed: %s",
				spec, check.Name, check.Detail)
		}
	}
}

// replaysTheKey reports whether this spec is the fake variant that plays the
// corpus's own answer, which is the one configuration whose attempts are a gate
// on this repository rather than a measurement of a model.
func replaysTheKey(spec provider.Spec) bool {
	return spec.Provider == provider.Fake && spec.Model == provider.FakePerfect
}

// detailOf renders a reason for the one-line log, bounded so a model's whole
// closing paragraph does not become the line.
func detailOf(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ""
	}
	const room = 160
	if len(trimmed) > room {
		trimmed = trimmed[:room] + "..."
	}
	return ": " + strings.ReplaceAll(trimmed, "\n", " ")
}

// budgetText spells a ceiling for a message, saying "none" for the absence of
// one rather than "$0.00", which reads as a run that may spend nothing.
func budgetText(limit float64) string {
	if limit <= 0 {
		return "none"
	}
	return fmt.Sprintf("$%.2f", limit)
}

// consentToSpend refuses a run that would call a real provider without being
// told to.
//
// The fake needs none: it talks to nobody. Everything else does, and the
// refusal is what stops `make modeleval-ce` with a real model in it from
// spending a budget nobody asked it to spend.
func consentToSpend(specs []provider.Spec, consented bool) error {
	if consented {
		return nil
	}
	var paid []string
	for _, spec := range specs {
		if spec.Provider != provider.Fake {
			paid = append(paid, spec.String())
		}
	}
	if len(paid) == 0 {
		return nil
	}
	return fmt.Errorf("%s would call %s, which costs money: set %s=yes to allow it",
		settingModels, strings.Join(paid, ", "), settingSpend)
}

// resolvePrices reads what each configured model's tokens cost.
//
// A model the table has no price for refuses the run, unless the run said it
// knows: a run that cannot say what it will cost cannot be stopped at a budget
// either, and starting one anyway is how a sweep becomes a bill nobody
// predicted. The fake is priced at nothing by being absent from the table and
// costing nothing to call, so it is skipped rather than declared free.
func resolvePrices(specs []provider.Spec, unpriced bool) (map[string]*modelrecord.Price, error) {
	prices := map[string]*modelrecord.Price{}
	for _, spec := range specs {
		if spec.Provider == provider.Fake {
			continue
		}
		price, known := provider.PriceFor(spec)
		if !known {
			if unpriced {
				continue
			}
			return nil, fmt.Errorf("%s", provider.UnpricedRefusal(spec, settingUnpriced))
		}
		stored := price
		prices[spec.String()] = &stored
	}
	return prices, nil
}

// buildAdapters builds one adapter per configured model.
//
// A real adapter with no credential is refused here rather than at its first
// request, which is [provider.New]'s own rule: a run that cannot authenticate
// should say so before it builds a world.
func buildAdapters(specs []provider.Spec) ([]provider.Provider, error) {
	adapters := make([]provider.Provider, 0, len(specs))
	for _, spec := range specs {
		adapter, err := provider.New(provider.Config{Spec: spec, APIKey: credentialFor(spec)})
		if err != nil {
			return nil, err
		}
		adapters = append(adapters, adapter)
	}
	return adapters, nil
}

// identifiers builds one call identifier per tier and surface.
//
// The reading is over the whole catalog at the session's tier rather than over
// the set that session serves, because the calls that matter most on a
// protective row are exactly the ones the served catalog no longer has: a
// mutating action removed by read-only mode is absent from the identifier the
// server built, so a session-side reading would report the one call a
// read-only row is measured on as naming nothing at all.
type identifiers struct {
	mu    sync.Mutex
	built map[identifierKey]mcpotel.CallIdentifier
}

// identifierKey names one reading: the tier the catalog is built at and the
// surface whose spelling of a call it resolves.
type identifierKey struct {
	tier    edition.Tier
	surface harness.Surface
}

// newIdentifiers returns an empty set.
func newIdentifiers() *identifiers {
	return &identifiers{built: map[identifierKey]mcpotel.CallIdentifier{}}
}

// For returns the resolver for one tier and surface, building it once.
func (i *identifiers) For(
	tier edition.Tier,
	surface harness.Surface,
) (func(tool string, arguments json.RawMessage) string, error) {
	identify, err := i.identifier(tier, surface)
	if err != nil {
		return nil, err
	}
	return func(tool string, arguments json.RawMessage) string {
		identity, named := identify.Identify(tool, arguments)
		if !named {
			return ""
		}
		return identity.ActionID
	}, nil
}

// Domains returns the reader that names the catalog domain a served tool
// belongs to, and the empty string for one that belongs to none.
//
// It is the same identifier [identifiers.For] resolves a call with, asked for
// the other half of what it knows, so the domain a tool is placed in when the
// slice is built and the domain a call is credited to afterwards are one
// reading. The arguments are nil because they are not part of the question: the
// individual surface's resolver decodes none, which is stated at its definition,
// and this reader is for that surface.
func (i *identifiers) Domains(
	tier edition.Tier,
	surface harness.Surface,
) (func(tool string) string, error) {
	identify, err := i.identifier(tier, surface)
	if err != nil {
		return nil, err
	}
	return func(tool string) string {
		identity, named := identify.Identify(tool, nil)
		if !named {
			return ""
		}
		return identity.Domain
	}, nil
}

// identifier builds one tier and surface's reading of a call, once.
func (i *identifiers) identifier(
	tier edition.Tier,
	surface harness.Surface,
) (mcpotel.CallIdentifier, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	key := identifierKey{tier: tier, surface: surface}
	identify, built := i.built[key]
	if !built {
		catalog, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{
			Tier:       tier,
			IncludeMCP: true,
		})
		if err != nil {
			return nil, fmt.Errorf("building the %s catalog: %w", tier, err)
		}
		identify = gitlabtools.NewCallIdentifier(catalog, string(surface))
		i.built[key] = identify
	}
	return identify, nil
}

// TestConsentToSpend_RefusesARealProviderWithoutIt checks the gate that stops
// a target with a real model in it from spending a budget nobody asked it to.
func TestConsentToSpend_RefusesARealProviderWithoutIt(t *testing.T) {
	fake, err := provider.ParseSpec("fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	paid, err := provider.ParseSpec("anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}

	if fakeErr := consentToSpend([]provider.Spec{fake}, false); fakeErr != nil {
		t.Errorf("the fake needed consent: %v", fakeErr)
	}
	refusal := consentToSpend([]provider.Spec{fake, paid}, false)
	if refusal == nil {
		t.Errorf("a real provider was allowed without consent")
	} else if !strings.Contains(refusal.Error(), settingSpend) {
		t.Errorf("the refusal is %v, want it to name %s", refusal, settingSpend)
	}
	if allowed := consentToSpend([]provider.Spec{fake, paid}, true); allowed != nil {
		t.Errorf("consent was given and the run was still refused: %v", allowed)
	}
}

// TestResolvePrices_RefusesAModelItCannotPrice checks the other half of the
// refusal: a run that cannot say what it will cost cannot be stopped at a
// budget either.
func TestResolvePrices_RefusesAModelItCannotPrice(t *testing.T) {
	priced, err := provider.ParseSpec("anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	unknown, err := provider.ParseSpec("anthropic:claude-from-the-future")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	fake, err := provider.ParseSpec("fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}

	prices, err := resolvePrices([]provider.Spec{priced, fake}, false)
	if err != nil {
		t.Fatalf("resolvePrices error = %v, want nil", err)
	}
	if prices[priced.String()] == nil {
		t.Errorf("a priced model came back with no price")
	}
	if prices[fake.String()] != nil {
		t.Errorf("the fake came back priced at %+v", prices[fake.String()])
	}

	if _, refused := resolvePrices([]provider.Spec{unknown}, false); refused == nil {
		t.Errorf("a model the table has no price for was allowed to start a run")
	}
	if _, allowed := resolvePrices([]provider.Spec{unknown}, true); allowed != nil {
		t.Errorf("the unpriced escape hatch still refused: %v", allowed)
	}
}

// TestBuildAdapters_RefusesACredentiallessProviderBeforeAWorldIsBuilt checks
// that a run which cannot authenticate says so before it spends a fixture.
func TestBuildAdapters_RefusesACredentiallessProviderBeforeAWorldIsBuilt(t *testing.T) {
	fake, err := provider.ParseSpec("fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	adapters, err := buildAdapters([]provider.Spec{fake})
	if err != nil {
		t.Fatalf("buildAdapters(fake) error = %v, want nil", err)
	}
	if len(adapters) != 1 || adapters[0].Spec().Provider != provider.Fake {
		t.Errorf("buildAdapters returned %d adapter(s)", len(adapters))
	}

	if harness.Setting("ANTHROPIC_API_KEY") != "" {
		t.Skip("this machine has an Anthropic credential, so the refusal cannot be reached here")
	}
	paid, err := provider.ParseSpec("anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	if _, refused := buildAdapters([]provider.Spec{paid}); refused == nil {
		t.Errorf("an adapter with no credential was built")
	}
}

// TestJudgeAttempt_FailsForWhatTheRunCouldNotDoAndNotForWhatAModelDid is the
// decision this whole file hangs on.
func TestJudgeAttempt_FailsForWhatTheRunCouldNotDoAndNotForWhatAModelDid(t *testing.T) {
	model, err := provider.ParseSpec("anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	replay, err := provider.ParseSpec("fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	declines, err := provider.ParseSpec("fake:declines")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}

	tests := []struct {
		name     string
		spec     provider.Spec
		outcome  conversationResult
		checks   []*modelrecord.Verify
		wantFail bool
	}{
		{
			name:    "a model that ran out of turns is a measurement",
			spec:    model,
			outcome: conversationResult{EndedBy: modelrecord.EndedOverBudget, Reason: "cap"},
		},
		{
			name:    "a model that wrote a malformed call is a measurement",
			spec:    model,
			outcome: conversationResult{EndedBy: modelrecord.EndedMalformed},
		},
		{
			name:    "a model that answered in text is a measurement",
			spec:    model,
			outcome: conversationResult{EndedBy: modelrecord.EndedNoToolCall},
		},
		{
			name:     "a provider that would not answer is the run's",
			spec:     model,
			outcome:  conversationResult{EndedBy: modelrecord.EndedProviderError, Reason: "429 forever"},
			wantFail: true,
		},
		{
			name:     "a harness error is the run's",
			spec:     model,
			outcome:  conversationResult{EndedBy: modelrecord.EndedHarnessError, Reason: "no session"},
			wantFail: true,
		},
		{
			name:    "the replaying fake completing is the pipe working",
			spec:    replay,
			outcome: conversationResult{EndedBy: modelrecord.EndedCompleted},
			checks:  []*modelrecord.Verify{{Passed: true}},
		},
		{
			name:     "the replaying fake not completing is this repository's defect",
			spec:     replay,
			outcome:  conversationResult{EndedBy: modelrecord.EndedOverBudget, Reason: "cap"},
			wantFail: true,
		},
		{
			name:     "a check that failed under the replaying fake is too",
			spec:     replay,
			outcome:  conversationResult{EndedBy: modelrecord.EndedCompleted},
			checks:   []*modelrecord.Verify{{Name: "issue", Passed: false, Detail: "still there"}},
			wantFail: true,
		},
		{
			name:    "a fake that declines on purpose is judged as a model",
			spec:    declines,
			outcome: conversationResult{EndedBy: modelrecord.EndedNoToolCall},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := &runner{}
			spy := &failureSpy{}
			run.judgeAttempt(spy, tc.spec, tc.outcome, tc.checks)
			if spy.failed != tc.wantFail {
				t.Errorf("judgeAttempt failed = %t, want %t (%v)", spy.failed, tc.wantFail, spy.messages)
			}
		})
	}
}

// failureSpy stands in for the test a judgement is reported to.
type failureSpy struct {
	failed   bool
	messages []string
}

// Errorf records a failure instead of raising one.
func (s *failureSpy) Errorf(format string, args ...any) {
	s.failed = true
	s.messages = append(s.messages, fmt.Sprintf(format, args...))
}

// TestAttemptName_NamesTheRepeatOnlyWhenThereIsMoreThanOne checks that a run
// that asked once does not read as a series.
func TestAttemptName_NamesTheRepeatOnlyWhenThereIsMoreThanOne(t *testing.T) {
	if got := attemptName("MT-001", 1, 1); got != "MT-001" {
		t.Errorf("attemptName with one repeat = %q", got)
	}
	if got := attemptName("MT-001", 2, 3); got != "MT-001/r2" {
		t.Errorf("attemptName with three repeats = %q", got)
	}
}

// TestNarrowingReason_PrefersTheCasesOwn checks which of the two reasons a
// report is given for a hole on a surface.
func TestNarrowingReason_PrefersTheCasesOwn(t *testing.T) {
	restrictedCase := modelcorpus.Stimulus{
		ID: "MT-001",
		Surfaces: modelcorpus.Restrict{
			Only:   []modelcorpus.Surface{modelcorpus.SurfaceMeta},
			Reason: "the dynamic find tool cannot name this",
		},
	}
	if got := narrowingReason(restrictedCase, harness.AllSurfaces()); got != "the dynamic find tool cannot name this" {
		t.Errorf("narrowingReason = %q, want the case's own", got)
	}

	plain := modelcorpus.Stimulus{ID: "MT-002"}
	got := narrowingReason(plain, []harness.Surface{harness.SurfaceDynamic})
	if !strings.Contains(got, settingSurfaces) {
		t.Errorf("narrowingReason = %q, want it to name the setting that narrowed the run", got)
	}
}

// TestScriptedLock_IsPerModel checks the lock's grain: a superset of the
// (surface, model) pair the session is keyed on, because the surface is not
// chosen at the point the lock has to be declared.
func TestScriptedLock_IsPerModel(t *testing.T) {
	first := scriptedLock("fake:perfect")
	if first != scriptedLock("fake:perfect") {
		t.Errorf("two attempts of one model named two locks")
	}
	if first == scriptedLock("fake:declines") {
		t.Errorf("two models share one lock, which would serialize attempts that cannot collide")
	}
	if !strings.Contains(string(first), "fake:perfect") {
		t.Errorf("the lock %q does not name the model it serializes", first)
	}
}

// TestBudgetText_SaysNoneRatherThanZero checks the wording of a ceiling that
// is not there: "$0.00" reads as a run that may spend nothing.
func TestBudgetText_SaysNoneRatherThanZero(t *testing.T) {
	if got := budgetText(0); got != "none" {
		t.Errorf("budgetText(0) = %q", got)
	}
	if got := budgetText(12.5); got != "$12.50" {
		t.Errorf("budgetText(12.5) = %q", got)
	}
}

// TestDetailOf_BoundsWhatReachesTheLogLine checks that a model's closing
// paragraph does not become the line.
func TestDetailOf_BoundsWhatReachesTheLogLine(t *testing.T) {
	if got := detailOf("   "); got != "" {
		t.Errorf("detailOf of nothing = %q", got)
	}
	if got := detailOf("two\nlines"); got != ": two lines" {
		t.Errorf("detailOf of two lines = %q", got)
	}
	long := detailOf(strings.Repeat("x", 500))
	if len(long) > 200 || !strings.HasSuffix(long, "...") {
		t.Errorf("detailOf of a long reason is %d characters and ends %q", len(long), long[len(long)-3:])
	}
}

// TestIdentifiers_ResolveACallOverTheWholeCatalog checks the reading a call
// line's requested action is written from.
//
// Over the whole catalog at the row's tier rather than the served set: the
// calls that matter most on a protective row are exactly the ones the served
// catalog no longer has.
func TestIdentifiers_ResolveACallOverTheWholeCatalog(t *testing.T) {
	built := newIdentifiers()
	resolve, err := built.For(edition.Ultimate, harness.SurfaceDynamic)
	if err != nil {
		t.Fatalf("For error = %v, want nil", err)
	}
	if got := resolve("gitlab_execute_action", json.RawMessage(`{"action":"issue.list"}`)); got != "issue.list" {
		t.Errorf("the dynamic execute tool resolved to %q, want issue.list", got)
	}
	if got := resolve("gitlab_find_action", json.RawMessage(`{"query":"issues"}`)); got != "" {
		t.Errorf("a discovery call resolved to %q, want no action", got)
	}

	again, err := built.For(edition.Ultimate, harness.SurfaceDynamic)
	if err != nil || again == nil {
		t.Fatalf("the second reading of one tier and surface returned %v", err)
	}

	meta, err := built.For(edition.Free, harness.SurfaceMeta)
	if err != nil {
		t.Fatalf("For(free, meta) error = %v", err)
	}
	if got := meta("gitlab_issue", json.RawMessage(`{"action":"list"}`)); got != "issue.list" {
		t.Errorf("the meta dispatcher resolved to %q, want issue.list", got)
	}
}

// TestNoteHarnessError_WritesDownTheAttemptAFatalWouldHaveLost covers the one
// ending nothing used to write.
//
// An attempt this side could not put ends with t.Fatalf, which writes no line,
// so the attempt was absent from the record with nothing saying it had been
// made: a report could not tell it from a case nobody asked, which is exactly
// the silent denominator this record exists to remove. What is checked here is
// that the line is written when the attempt failed and had written none, that
// it is not written twice, and that it says why as far as this side can know.
func TestNoteHarnessError_WritesDownTheAttemptAFatalWouldHaveLost(t *testing.T) {
	tests := []struct {
		name      string
		progress  *attemptProgress
		failed    bool
		wantLines int
		wantIn    string
	}{
		{
			name: "a failure this file raised is written down as it was raised",
			progress: &attemptProgress{
				stage:  "building the world the case runs in",
				detail: `case MT-900 names recipe "nothing", which this package cannot build`,
			},
			failed:    true,
			wantLines: 1,
			wantIn:    "names recipe",
		},
		{
			name:      "a failure raised elsewhere is written down as how far the attempt got",
			progress:  &attemptProgress{stage: "opening the session this attempt talks to"},
			failed:    true,
			wantLines: 1,
			wantIn:    "opening the session",
		},
		{
			name:      "an attempt that wrote its own line is not written a second time",
			progress:  &attemptProgress{stage: "putting the case to the model", written: true},
			failed:    true,
			wantLines: 0,
		},
		{
			name:      "an attempt that did not fail leaves nothing behind",
			progress:  &attemptProgress{stage: "putting the case to the model"},
			failed:    false,
			wantLines: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Cleanup(modelrecord.Release)
			run := &runner{record: &recorder{
				writer:   modelrecord.OpenDir(dir),
				sessions: map[string]*modelrecord.Session{},
			}}

			run.noteHarnessError(&failureSpy{}, tc.failed, tc.progress, modelrecord.Attempt{
				ID: "run-1/MT-900/fake-perfect/dynamic/r1", Case: "MT-900",
				Model: "fake:perfect", Surface: "dynamic", Repeat: 1,
			})

			written := attemptLines(t, dir)
			if len(written) != tc.wantLines {
				t.Fatalf("the shard holds %d attempt line(s), want %d", len(written), tc.wantLines)
			}
			if tc.wantLines == 0 {
				return
			}
			line := written[0]
			if line.EndedBy != modelrecord.EndedHarnessError {
				t.Errorf("the line ended %q, want %q", line.EndedBy, modelrecord.EndedHarnessError)
			}
			if line.Case != "MT-900" || line.Model != "fake:perfect" || line.Surface != "dynamic" {
				t.Errorf("the line is %+v, want it to name the attempt that was not made", line)
			}
			if !strings.Contains(line.Reason, tc.wantIn) {
				t.Errorf("the reason is %q, want it to say %q", line.Reason, tc.wantIn)
			}
		})
	}
}

// attemptLines reads the attempt lines a directory holds, treating a directory
// with no shard in it as a run that wrote nothing.
//
// A shard is created by the first line written and not before, so "wrote
// nothing" leaves an empty directory, which [modelrecord.ReadShards] refuses as
// a directory that was never recorded into. Here that is an answer rather than
// a failure, so the empty directory is recognized before the read.
func attemptLines(t *testing.T, dir string) []modelrecord.Attempt {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) == 0 {
		return nil
	}
	shards, err := modelrecord.ReadShards(dir)
	if err != nil {
		t.Fatalf("reading the shards under %s: %v", dir, err)
	}
	var lines []modelrecord.Attempt
	for _, shard := range shards {
		for _, record := range shard.Records {
			if record.Attempt != nil {
				lines = append(lines, *record.Attempt)
			}
		}
	}
	return lines
}

// TestVerdictText_SaysTheOutcomeAndOnlyWhatHappened checks the line a run
// prints as each attempt is scored.
//
// The outcome and the steps are always there, since a reader watching a run
// wants both whatever happened; the rest is said only when there is something
// to say, because a line that ends "0 discovery call(s)" on every row is a
// line nobody reads.
func TestVerdictText_SaysTheOutcomeAndOnlyWhatHappened(t *testing.T) {
	clean := verdictText(modelscore.Verdict{
		Outcome:  modelscore.OutcomeCompleted,
		Verified: true,
		Steps:    []modelscore.StepVerdict{{Complete: true}, {Complete: true}},
	})
	if clean != "completed, 2/2 step(s)" {
		t.Errorf("verdictText of a clean attempt = %q", clean)
	}

	noisy := verdictText(modelscore.Verdict{
		Outcome:       modelscore.OutcomeFailed,
		Reason:        "the second step was never reached",
		Discovery:     2,
		InvalidParams: 1,
		Steps:         []modelscore.StepVerdict{{Complete: true}, {}},
	})
	const want = "failed, 1/2 step(s), 2 discovery call(s), 1 refused for arguments, " +
		"the check against GitLab failed: the second step was never reached"
	if noisy != want {
		t.Errorf("verdictText of an attempt that went wrong = %q, want %q", noisy, want)
	}
}

// TestAttemptProgress_SaysWhatTheRunWasDoingWhenItCouldNotGoOn covers the
// reason the line above carries.
func TestAttemptProgress_SaysWhatTheRunWasDoingWhenItCouldNotGoOn(t *testing.T) {
	staged := &attemptProgress{stage: "rendering the stimulus"}
	if reason := staged.reason(); !strings.Contains(reason, "rendering the stimulus") {
		t.Errorf("reason = %q, want it to name the stage the attempt reached", reason)
	}
	staged.at("putting the case to the model")
	if reason := staged.reason(); !strings.Contains(reason, "putting the case to the model") {
		t.Errorf("reason after at() = %q, want it to name the newer stage", reason)
	}

	detailed := &attemptProgress{stage: "rendering the stimulus", detail: "no fact named issue_iid"}
	if reason := detailed.reason(); reason != "no fact named issue_iid" {
		t.Errorf("reason = %q, want the message the failure itself carried", reason)
	}
}

// TestRecordSkip_SaysWhyACaseWasAbsent checks the line a case's unmet needs
// leave behind.
//
// Without it a record cannot tell a licensed case absent on a community
// instance from a case nobody wrote, which is the reading that made the
// evaluator this replaces publish a coverage figure that meant nothing.
func TestRecordSkip_SaysWhyACaseWasAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(modelrecord.Release)

	run := &runner{
		cfg:    runConfig{Surfaces: []harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta}},
		record: &recorder{writer: modelrecord.OpenDir(dir), sessions: map[string]*modelrecord.Session{}},
	}
	run.record.run.RunID = "run-1"

	one := modelcorpus.Stimulus{ID: "MT-900", Needs: modelcorpus.Needs{Tier: modelcorpus.TierUltimate}}
	needs, err := needsOf(one.Needs)
	if err != nil {
		t.Fatalf("needsOf error = %v", err)
	}
	run.recordSkip(&failureSpy{}, "fake:perfect", one, 1, needs)

	records := readOneShard(t, dir)
	if counted := countTypes(records); counted[modelrecord.TypeAttempt] != 2 {
		t.Fatalf("the shard holds %v, want one attempt line per configured surface", counted)
	}
	for _, record := range records {
		if record.Attempt == nil {
			continue
		}
		if record.Attempt.EndedBy != modelrecord.EndedSkipped {
			t.Errorf("the line ended %q, want %q", record.Attempt.EndedBy, modelrecord.EndedSkipped)
		}
		if !strings.Contains(record.Attempt.Reason, "ultimate") {
			t.Errorf("the reason is %q, want it to name the need that was not met", record.Attempt.Reason)
		}
	}

	// A case with nothing declared cannot be skipped for a need, and the line
	// says that rather than naming an empty list.
	plain := modelcorpus.Stimulus{ID: "MT-901"}
	run.recordSkip(&failureSpy{}, "fake:perfect", plain, 1, nil)
	for _, record := range readOneShard(t, dir) {
		if record.Attempt != nil && record.Attempt.Case == "MT-901" &&
			strings.Contains(record.Attempt.Reason, "needs:") {
			t.Errorf("a case with no declared need was skipped for one: %q", record.Attempt.Reason)
		}
	}
}
