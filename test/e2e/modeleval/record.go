//go:build e2e

// record.go writes the shard: what the run was, what each session served, and
// every attempt, turn, call and check.
//
// Two lifetimes, and the split is not arbitrary. An attempt's own lines are
// written when that attempt ends, because they are complete then and because a
// run that dies in the middle should leave behind everything it paid for. The
// run and session lines are written from an exit hook, because neither is
// complete until the last attempt has been made: a session's tool-schema
// digests accumulate one per provider as each model first sees the list, and
// the run line's instance facts are read off the first Env that exists, which
// is not the moment the run was configured.
//
// No verdict is written anywhere. The record is observation, so a scoring rule
// can be corrected later and every past run re-scored without spending a
// token, which is what the Markdown-then-parse loop of the evaluator this
// replaces made impossible.

package modeleval

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// settingCommit is where the revision under test is read from. It is the
// harness's own variable, set by every make target that starts a run, so a
// recorded baseline says which tree produced it.
const settingCommit = "E2E_COMMIT"

// recorder is this run's shard.
//
// A nil writer is what recording being off looks like, and the writer absorbs
// it: a run that was not given a directory makes every call it would have made
// and writes nothing, which is what a maintainer driving one case by hand
// wants.
type recorder struct {
	writer *modelrecord.Writer

	mu sync.Mutex
	// run is the run line, filled in two moments: the configuration up front
	// and the instance facts off the first Env.
	run modelrecord.Run
	// runtimeKnown says the instance half of the run line has been filled.
	runtimeKnown bool
	// sessions are the session lines, by label, each gaining a tool-schema
	// digest as a provider first sees that session's list.
	sessions map[string]*modelrecord.Session
	// order is the order sessions were first described, so a shard reads the
	// way the run happened.
	order []string
	// scopes are the run credential's own, read off the first Env. Every
	// session this package opens is on that credential, so they are the
	// session's scopes too, and they decide the catalog before anything is
	// registered.
	scopes []string
	// problems are what the exit hook could not write, reported as the hook's
	// error.
	problems []string
}

// newRecorder opens this process's shard and arranges for the run and session
// lines to be written after the last test.
//
// The hook is registered here rather than by the caller, so a recorder that
// exists is a recorder that will be flushed: the alternative is a run that
// wrote every attempt and no run line, and an attempt with no run line is
// unpublishable, since the commit, the instance and the tier are all on it.
func newRecorder() *recorder {
	rec := &recorder{
		writer:   modelrecord.OpenDir(harness.Setting(modelrecord.DirEnv)),
		sessions: map[string]*modelrecord.Session{},
	}
	harness.AtExit(rec.flush)
	return rec
}

// describeRun records the half of the run line that is known before anything
// runs: the configuration, the models and what a run may spend.
func (r *recorder) describeRun(cfg runConfig, specs []provider.Spec, prices map[string]*modelrecord.Price) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.run.StartedAt = time.Now().UTC()
	r.run.Commit = harness.Setting(settingCommit)
	r.run.Filter = testFilter()
	r.run.CorpusDigest = modelcorpus.Digest()
	r.run.ContractDigest = contractDigest()
	r.run.BudgetUSD = cfg.BudgetUSD
	r.run.Repeat = cfg.Repeat
	r.run.Providers = providerLines(specs, prices)
}

// noteRuntime fills the instance half of the run line, once.
//
// It takes an Env rather than reading the harness directly because that is
// where the probe's answers live, and it is called from the first attempt
// rather than at configuration time because creating an Env is what contacts
// GitLab: a run that measures nothing must not reach an instance, and a run
// that measures something reaches one anyway.
func (r *recorder) noteRuntime(env *harness.Env) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.runtimeKnown {
		return
	}
	facts := env.Runtime()
	r.run.Package = env.Package()
	r.run.RunID = env.RunID()
	r.run.Edition = editionName(facts.Enterprise)
	r.run.GitLabVersion = facts.Version
	r.run.Tier = facts.Tier.String()
	r.run.TierConfirmed = facts.TierConfirmed
	r.scopes = facts.Scopes
	r.runtimeKnown = true
}

// RunID returns the identifier this run's names are scoped to, empty until an
// Env has existed: a run whose first case was skipped has not contacted the
// instance and has no run identifier to give a line.
func (r *recorder) RunID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.run.RunID
}

// runLine returns the run line as it stands.
//
// It is what a reader joining an attempt to its run needs before the shard has
// been written, and the only reader is the live verdict the runner logs as
// each attempt ends: the tier a catalog is read at is on the run line and on
// nothing else. It hands back a copy rather than a pointer, so a reader cannot
// replace a field of the line the shard is going to hold.
func (r *recorder) runLine() modelrecord.Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.run
}

// editionName spells what the instance reported about itself.
//
// It is the version endpoint's own flag and not the tier: an enterprise image
// reports true whether or not a license is installed, which is a different
// fact from what the license resolves to and is kept apart from it here for
// the same reason the harness keeps them apart.
func editionName(enterprise bool) string {
	if enterprise {
		return "enterprise"
	}
	return "community"
}

// describeSession records one server configuration, or returns the line
// already recorded for it.
//
// A session is shared by every attempt on one surface of one run, so this is
// called once per attempt and writes once per session.
func (r *recorder) describeSession(session *harness.Session, cfg runConfig) *modelrecord.Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	label := session.Label()
	if known, seen := r.sessions[label]; seen {
		return known
	}
	line := &modelrecord.Session{
		Label:           label,
		Surface:         session.Surface().String(),
		Mode:            session.Mode().String(),
		Capabilities:    session.Capabilities().String(),
		MetaParamSchema: string(cfg.MetaParamSchema),
		TierPin:         string(cfg.Tier),
		// What the model was shown of what the session served, which is the
		// whole list everywhere but the individual surface. It is the budget
		// rather than a length because a slice is chosen per case and a row
		// aggregates many: what the row can say is how many tools an attempt
		// was shown at most, and ServedTools beside it says what it was shown
		// out of.
		SliceSize:         budgetFor(session.Surface(), cfg.Slice),
		TokenScopes:       slices.Clone(r.scopes),
		ServedTools:       len(session.Tools()),
		ToolSchemaDigests: map[string]string{},
	}
	r.sessions[label] = line
	r.order = append(r.order, label)
	return line
}

// noteToolDigest records the digest of the tool list as one provider received
// it.
//
// Per provider and not one digest, because a provider-specific rewrite of the
// schemas is exactly what this catches: two of the four adapters this replaces
// injected parameter names into the execute schema, so two columns of a
// published table were measuring a surface the other two never received and
// nothing on the row said so.
func (r *recorder) noteToolDigest(label, spec, digest string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if line, known := r.sessions[label]; known {
		line.ToolSchemaDigests[spec] = digest
	}
}

// writeAttempt writes one attempt and everything that names it.
//
// The order is the order a reader wants: the attempt, then its turns, then its
// calls, then the checks against GitLab that followed. The reporter is the
// test that made the attempt, so a shard that cannot be written fails the test
// that was writing rather than the run as a whole.
func (r *recorder) writeAttempt(
	reporter modelrecord.Reporter,
	attempt *modelrecord.Attempt,
	turns []*modelrecord.Turn,
	calls []*modelrecord.Call,
	checks []*modelrecord.Verify,
) {
	lines := make([]modelrecord.Line, 0, 1+len(turns)+len(calls)+len(checks))
	lines = append(lines, attempt)
	for _, turn := range turns {
		lines = append(lines, turn)
	}
	for _, call := range calls {
		lines = append(lines, call)
	}
	for _, check := range checks {
		lines = append(lines, check)
	}
	r.writer.Write(reporter, lines...)
}

// flush writes the run and session lines, after the last test.
//
// It is the exit hook, so there is no test to report a failure through: what
// the writer says it could not record is collected and returned, which fails
// the run the way every other exit hook failure does.
func (r *recorder) flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer == nil {
		return nil
	}
	reporter := &exitReporter{}
	lines := make([]modelrecord.Line, 0, 1+len(r.order))
	run := r.run
	lines = append(lines, &run)
	for _, label := range r.order {
		lines = append(lines, r.sessions[label])
	}
	r.writer.Write(reporter, lines...)

	r.problems = append(r.problems, reporter.problems...)
	if len(r.problems) == 0 {
		return nil
	}
	return errors.New("the model record's run and session lines: " + strings.Join(r.problems, "; "))
}

// exitReporter collects what a writer could not record when there is no test
// to report it to.
type exitReporter struct {
	problems []string
}

// Errorf records one problem, the way testing.T.Errorf reports one.
func (r *exitReporter) Errorf(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

// providerLines describes the models a run was configured with, as they went
// on the wire.
//
// The options are the spec's own rendering, so a model that refuses a
// temperature reads "provider default" rather than "0": they are different
// requests and a table printing the second for the first would claim a
// determinism the run did not ask for.
func providerLines(specs []provider.Spec, prices map[string]*modelrecord.Price) []modelrecord.Provider {
	lines := make([]modelrecord.Provider, 0, len(specs))
	for _, spec := range specs {
		lines = append(lines, modelrecord.Provider{
			Spec:    spec.String(),
			Name:    spec.Provider,
			Model:   spec.Model,
			Options: maps.Clone(spec.Wire()),
			Price:   prices[spec.String()],
		})
	}
	return lines
}

// testFilter returns the -run expression this binary was given.
//
// A partial run is not a claim about the rest, so the publisher refuses a
// record carrying one. Reading it from the flag rather than from a setting is
// what makes that refusal unavoidable: nothing has to remember to write it
// down.
func testFilter() string {
	if filter := flag.Lookup("test.run"); filter != nil {
		return filter.Value.String()
	}
	return ""
}
