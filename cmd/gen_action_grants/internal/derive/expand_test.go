package derive

import (
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
)

// fixtureDeriver builds the deriver Derive builds, over the fixture program.
func fixtureDeriver(t *testing.T, sdk SDK) *deriver {
	t.Helper()
	prog := loadGrants(t)
	return &deriver{
		prog:       prog,
		sdk:        sdk,
		directives: collectDirectives(prog.Position, prog.Packages()),
		bodies:     map[*types.Func]*node{},
	}
}

// fixtureFunc finds a function the fixture declares.
func fixtureFunc(t *testing.T, name string) *types.Func {
	t.Helper()
	for _, pkg := range loadGrants(t).Packages() {
		if fn, ok := pkg.Types.Scope().Lookup(name).(*types.Func); ok {
			return fn
		}
	}
	t.Fatalf("the fixture declares no function %s", name)
	return nil
}

// TestExpansionCall_AFunctionWithNoBody_SendsNothing verifies a callee the
// program holds no body for (a function of a package outside the load)
// expands to the one path that sends nothing rather than failing the walk.
func TestExpansionCall_AFunctionWithNoBody_SendsNothing(t *testing.T) {
	walk := &expansion{d: fixtureDeriver(t, fixtureSDK()), action: &Action{}, index: map[string]int{}, stack: map[*types.Func]bool{}}
	outside := types.NewFunc(token.NoPos, nil, "Outside", types.NewSignatureType(nil, nil, nil, nil, nil, false))
	if got := walk.call(outside, nil, nil, context{}); !got.isUnit() {
		t.Errorf("call(Outside) = %v, want the empty path", got)
	}
}

// TestExpansionSDK_DocumentsPastTheBound_IsReported verifies a client-go
// method whose routes and documents multiply past the path bound stops the
// walk with the overflow reported, rather than handing on a truncated set as
// if it were whole.
func TestExpansionSDK_DocumentsPastTheBound_IsReported(t *testing.T) {
	routes := make([]Request, maxPaths+1)
	for i := range routes {
		routes[i] = Request{Kind: KindREST, Method: "GET", Path: "/many/" + strings.Repeat("x", i+1)}
	}
	sdk := fixtureSDK()
	sdk["Projects.ListProjects"] = stubMethod{routes: routes, documents: []Request{{Kind: KindGraphQL, Document: "query q { a }"}}}
	unsent := []actionrequests.Action{{ID: "fixture.unsent", Name: "unsent", Owner: "grants"}}
	act := Derive(loadGrants(t), unsent, sdk, nil).Actions[0]
	want := "fixture.unsent expands into more than 256 paths; declare how its requests combine"
	if len(act.Findings) != 1 || act.Findings[0] != want {
		t.Errorf("findings = %q, want %q", act.Findings, want)
	}
}
