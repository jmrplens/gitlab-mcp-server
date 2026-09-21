//go:build e2e

package harness

import (
	"errors"
	"testing"

	"example.com/e2efake/internal/edition"
)

// ActionID is a canonical catalog action identifier.
type ActionID string

// Env is one test's environment.
type Env struct {
	// T is the test.
	T *testing.T
	// Label is an exported field nothing in the fixture reads.
	Label string
}

// Session is one server session.
type Session struct {
	env *Env
}

// Option configures one Env.
type Option func(*Env)

// Need is an environment requirement.
type Need struct {
	name string
}

// Failure is a class of unhappy answer.
type Failure string

// String spells the class; nothing in the fixture calls it.
func (f Failure) String() string { return string(f) }

// FailureNotFound is GitLab answering 404.
const FailureNotFound Failure = "not_found"

// Label is an alias, which has no members to list.
type Label = string

// New returns an Env for the test.
func New(t *testing.T, opts ...Option) *Env {
	e := &Env{T: t}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Session opens a session on the default surface.
func (e *Env) Session() *Session { return &Session{env: e} }

// Close ends the session; nothing in the fixture calls it.
func (s *Session) Close() {}

// Needs declares environment requirements.
func Needs(needs ...Need) Option {
	return func(*Env) { _ = needs }
}

// Tier requires an instance licensed at least at the given tier.
func Tier(want edition.Tier) Need {
	return Need{name: "tier " + string(rune('0'+int(want)))}
}

// Do runs an action and decodes its answer.
func Do[O any](s *Session, id ActionID, params map[string]any) O {
	var output O
	_, _, _ = s, id, params
	return output
}

// DoVoid runs an action whose answer the test does not read.
func DoVoid(s *Session, id ActionID, params map[string]any) {
	_, _, _ = s, id, params
}

// Try runs an action and hands back both halves.
func Try[O any](s *Session, id ActionID, params map[string]any) (O, error) {
	var output O
	_, _, _ = s, id, params
	return output, errors.New("fixture")
}

// Refused asserts that the server declined the call in the named class.
func Refused(s *Session, id ActionID, params map[string]any, want Failure) string {
	_, _, _, _ = s, id, params, want
	return ""
}

// ExpectToolError asserts that the call came back as a tool error.
func ExpectToolError(s *Session, id ActionID, params map[string]any, contains string) string {
	_, _, _, _ = s, id, params, contains
	return ""
}

// Eventually runs an action until the predicate accepts its answer.
func Eventually[O any](s *Session, id ActionID, params map[string]any, until func(O) bool) O {
	var output O
	_, _, _, _ = s, id, params, until
	return output
}

// Unused is exported and referenced by nothing, which is what the dead-export
// rule reports.
func Unused() {}

// Timeout is an exported variable nothing reads. The rule is about exported
// symbols and not about functions, so a variable is reported like any other.
var Timeout = 0

// reset is unexported, so the dead-export rule never looks at it however
// little the fixture uses it: an unexported symbol nothing reads is what
// staticcheck's own unused check is for.
func reset() {}

// close is an unexported method on an exported type, passed over for the
// same reason as [reset] while the exported Close beside it is reported.
func (s *Session) close() { reset() }

// ModelOnly is exported and called only from test/e2e/modeleval, which is the
// shape the gate has to accept: the model evaluation package is loaded as a
// consumer of the harness and never scanned for placement, so a symbol it is
// the first and only user of is live.
func ModelOnly() {}

// Version is read only by the command under test/e2e/internal/e2ectl, a main
// package that is not the test main go test generates: the scan keeps it as a
// consumer, so this constant is live.
const Version = "fixture"

// Anon is a variable of an anonymous struct type. Its field has no owner the
// dead-export scan can key it under, so a scenario reading Anon.Field is a use
// of Anon and of nothing else; [Field] beside it shares the name and stays
// unused.
var Anon struct{ Field int }

// Field is a package-level function sharing its name with Anon's field, and is
// referenced by nothing.
func Field() {}

// Stopper is a variable of an interface literal. Its method has no receiver
// type to be keyed under, so a scenario calling Stopper.Stop is a use of
// Stopper and of nothing else; [Stop] beside it shares the name and stays
// unused.
var Stopper interface{ Stop() }

// Stop is a package-level function sharing its name with Stopper's method, and
// is referenced by nothing.
func Stop() {}
