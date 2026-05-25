package evaluator

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// FixtureContext describes the runtime preparing a case fixture.
type FixtureContext struct {
	Client         *gitlabclient.Client
	MCPSession     *mcp.ClientSession
	RuntimeEdition EvalCaseEdition
	ToolSurface    string
	RunSuffix      string
	ModelName      string
	RunIndex       int
	CaseID         EvalCaseID
	FixtureName    string
	IdempotencyKey string
	Logf           func(string, ...any)
}

// CaseFixtureContext is kept as a temporary alias for Phase 2 migration code.
type CaseFixtureContext = FixtureContext
