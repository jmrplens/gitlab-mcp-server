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
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
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
