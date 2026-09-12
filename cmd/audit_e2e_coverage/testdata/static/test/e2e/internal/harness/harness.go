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
