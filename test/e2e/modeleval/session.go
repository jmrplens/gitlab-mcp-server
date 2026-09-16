//go:build e2e

// session.go turns the run's configuration into the shape of a server and
// opens one.
//
// It is four lines of assignment on purpose. A ServerConfig names only what
// the released binary itself reads, so there is nowhere here to hand the
// server something a deployment could not have; the evaluator this replaces
// built an mcp.Server in its own process from three fields, and so measured a
// program with no scopes, no exclusions, no schema mode and no middleware
// chain, on a surface it could not serve at all.

package modeleval

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// serverShape is the server one surface of one run is measured against.
//
// The credential is deliberately not set: an empty Token is the run's own, and
// a model measured against a narrowed credential would be measured against a
// catalog the row does not name.
func serverShape(cfg runConfig, surface harness.Surface) harness.ServerConfig {
	return harness.ServerConfig{
		Surface:         surface,
		Mode:            cfg.Mode,
		Tier:            cfg.Tier,
		MetaParamSchema: cfg.MetaParamSchema,
	}
}

// openSession starts, or joins, the server for one surface of this run.
//
// Sessions are pooled by shape, so every attempt on one surface of one run
// talks to the same child. That is what a client does and what makes the tool
// list a model reads the same from one attempt to the next; the shape key
// carries the schema mode, so a run that changed it gets a child of its own
// rather than the previous one's schemas.
func openSession(env *harness.Env, cfg runConfig, surface harness.Surface) *harness.Session {
	env.T.Helper()
	return env.Session(serverShape(cfg, surface))
}

// The scripted sessions, and why there are so few of them.
//
// A session whose client answers elicitation is private by construction: the
// harness gives every scripted configuration a key nothing else can name, so
// asking for one per attempt would start a server process per attempt. Five
// interactive cases on three surfaces for four models is sixty children, each
// bounded by a three-minute start on the individual surface, and all sixty
// would serve identical catalogs.
//
// So there is one per surface and model, opened before any attempt runs, and
// the answers it gives come from a variable the current attempt sets. That
// variable is the only join between an elicitation and the attempt it belongs
// to: a responder is handed an elicitation request and nothing else, and the
// request names no attempt. Two attempts that shared one responder without
// overlapping is therefore a rule the runner has to keep, and it keeps it with
// a harness lock named after the model, declared on the Env each interactive
// case opens. Without it one attempt is answered with another's facts, the
// recipe's own check against GitLab fails, and the row charges the model for
// the harness.

// scriptedSession is one server whose client answers elicitation, and the
// attempt whose answers it is currently giving.
type scriptedSession struct {
	session *harness.Session
	tools   []provider.Tool

	mu sync.Mutex
	// respond is the current attempt's answers, nil between attempts.
	respond func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
}

// Session returns the harness session this holds.
func (s *scriptedSession) Session() *harness.Session { return s.session }

// Tools returns the tool list read when the session was opened.
func (s *scriptedSession) Tools() []provider.Tool { return s.tools }

// lend installs one attempt's answers and returns the function that takes them
// back.
//
// Taking them back matters as much as installing them: between attempts the
// responder answers nothing, so an elicitation that arrives late, from a call
// the previous attempt made and the server is still finishing, is declined
// rather than answered with the next attempt's world.
func (s *scriptedSession) lend(
	respond func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error),
) func() {
	s.mu.Lock()
	s.respond = respond
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.respond = nil
		s.mu.Unlock()
	}
}

// answer is the Responder the session's client is built with.
//
// It declines when no attempt has lent it answers, which is what a server
// asking outside an attempt should be told: an empty accept would have the
// flow create an object out of nothing and leave it behind.
func (s *scriptedSession) answer(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	s.mu.Lock()
	respond := s.respond
	s.mu.Unlock()

	if respond == nil {
		return &mcp.ElicitResult{Action: "decline"}, nil
	}
	return respond(ctx, req)
}

// scriptedKey names one scripted session: the surface it serves and the model
// whose attempts it serves them to.
type scriptedKey struct {
	surface harness.Surface
	model   string
}

// scriptedSessions is the set of them, opened up front and read by name.
type scriptedSessions struct {
	mu   sync.Mutex
	open map[scriptedKey]*scriptedSession
}

// newScriptedSessions returns an empty set.
func newScriptedSessions() *scriptedSessions {
	return &scriptedSessions{open: map[scriptedKey]*scriptedSession{}}
}

// Open starts the session for one surface and model, once.
//
// It is called from the test that owns env, before any attempt runs, because
// the session outlives every one of them: an Env's cleanups run when its own
// test ends, so a scripted session opened inside a case's subtest would be
// closed when that case finished and the next case would start another.
func (s *scriptedSessions) Open(
	env *harness.Env,
	cfg runConfig,
	surface harness.Surface,
	model string,
) (*scriptedSession, error) {
	env.T.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	key := scriptedKey{surface: surface, model: model}
	if known, seen := s.open[key]; seen {
		return known, nil
	}

	scripted := &scriptedSession{}
	shape := serverShape(cfg, surface)
	shape.Elicitation = harness.ElicitationScripted
	shape.Responder = scripted.answer
	scripted.session = env.Session(shape)

	tools, err := modelTools(scripted.session)
	if err != nil {
		return nil, fmt.Errorf("the scripted %s session for %s: %w", surface, model, err)
	}
	scripted.tools = tools

	s.open[key] = scripted
	return scripted, nil
}

// Lookup returns the session for one surface and model, and whether one was
// opened.
func (s *scriptedSessions) Lookup(surface harness.Surface, model string) (*scriptedSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	scripted, known := s.open[scriptedKey{surface: surface, model: model}]
	return scripted, known
}

// modelTools reads what a session serves, in the shape every adapter sends.
//
// The list is what the server published rather than a rendering of the catalog,
// so what the model is shown is what a client would be shown: the schemas are
// the ones tools/list serialized, pruned by the session's tier and narrowed by
// its mode and its credential.
func modelTools(session *harness.Session) ([]provider.Tool, error) {
	served := session.ToolDefinitions()
	tools := make([]provider.Tool, 0, len(served))
	for _, one := range served {
		var schema json.RawMessage
		if one.InputSchema != nil {
			encoded, err := json.Marshal(one.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("encoding the input schema of %s: %w", one.Name, err)
			}
			schema = encoded
		}
		tool, err := provider.NewTool(one.Name, one.Description, schema)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// toolCache holds the tool list of each ordinary session, by label.
//
// Listing is not free: the individual surface publishes roughly a thousand
// tools and several megabytes of schema, and every attempt on that surface
// would list them again. The list cannot change under a session, since the
// catalog is built when the child starts, so one reading per session is the
// whole of what a run needs.
type toolCache struct {
	mu   sync.Mutex
	read map[string][]provider.Tool
}

// newToolCache returns an empty cache.
func newToolCache() *toolCache { return &toolCache{read: map[string][]provider.Tool{}} }

// Tools returns what one session serves, reading it once.
func (c *toolCache) Tools(session *harness.Session) ([]provider.Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	label := session.Label()
	if known, seen := c.read[label]; seen {
		return known, nil
	}
	tools, err := modelTools(session)
	if err != nil {
		return nil, err
	}
	c.read[label] = tools
	return tools, nil
}
