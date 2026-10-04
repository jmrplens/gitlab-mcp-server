package actionrequests

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// siteNamed builds a site with the identity Match reads and nothing else.
func siteNamed(pkg, name, tool string, pos token.Pos) Site {
	return Site{Package: pkg, Name: name, Tool: tool, Pos: pos}
}

// sitePositions returns the positions of the sites Match picked, so a case
// can say which of a set of candidates it wants.
func sitePositions(sites []Site) []token.Pos {
	positions := make([]token.Pos, 0, len(sites))
	for _, site := range sites {
		positions = append(positions, site.Pos)
	}
	return positions
}

// TestMatch_TheJoinOnTheCanonicalID verifies each rule of the join. The tool
// name decides first, and a site whose tool name was read and is another
// action's is never taken; where no tool name could be read the owner decides;
// and where the owner is none of the packages every site with the name is
// taken, because a wrong guess must not be the quiet one.
func TestMatch_TheJoinOnTheCanonicalID(t *testing.T) {
	sites := map[string][]Site{
		"badge_get": {
			siteNamed("badges", "badge_get", "gitlab_get_project_badge", 1),
			siteNamed("badges", "badge_get", "gitlab_get_group_badge", 2),
		},
		"license_get": {
			siteNamed("adminspecs", "license_get", "", 3),
			siteNamed("licensetemplates", "license_get", "", 4),
		},
		"mixed": {
			siteNamed("one", "mixed", "gitlab_other_mixed", 5),
			siteNamed("one", "mixed", "", 6),
		},
	}

	cases := []struct {
		name string
		act  Action
		want []token.Pos
	}{
		{
			name: "the tool name picks one of two sites of one package",
			act:  Action{Name: "badge_get", Owner: "badges", Tool: "gitlab_get_group_badge"},
			want: []token.Pos{2},
		},
		{
			name: "a tool name is compared trimmed",
			act:  Action{Name: "badge_get", Owner: "badges", Tool: "  gitlab_get_project_badge "},
			want: []token.Pos{1},
		},
		{
			name: "no site carries the action's tool name and every one names another",
			act:  Action{Name: "badge_get", Owner: "badges", Tool: "gitlab_badge_elsewhere"},
		},
		{
			name: "the owner decides among sites with no tool name",
			act:  Action{Name: "license_get", Owner: "licensetemplates"},
			want: []token.Pos{4},
		},
		{
			name: "an owner none of them is takes every one",
			act:  Action{Name: "license_get", Owner: "license"},
			want: []token.Pos{3, 4},
		},
		{
			name: "a site another action's tool name claims is left out",
			act:  Action{Name: "mixed", Owner: "elsewhere", Tool: "gitlab_mixed"},
			want: []token.Pos{6},
		},
		{
			name: "a name no site declares",
			act:  Action{Name: "never_declared", Owner: "badges"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := sitePositions(Match(sites, testCase.act)); !slices.Equal(got, testCase.want) {
				t.Errorf("Match() picked sites at %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestMatch_TheTwinsOfTheRequestsFixture_JoinToOneSiteEach verifies the join
// on source written the way the badge twins are: two lists of one package
// declaring one name, the tool name reaching the constructor through a helper
// that amends the options, a field assignment of the whole tool spec, and a
// field assignment of its name.
func TestMatch_TheTwinsOfTheRequestsFixture_JoinToOneSiteEach(t *testing.T) {
	prog := loadFixture(t, requestSources())
	sites := prog.Sites()

	cases := []struct {
		act     Action
		handler string
	}{
		{act: Action{Name: "twin_get", Owner: "requests", Tool: "gitlab_project_twin_get"}, handler: "Direct"},
		{act: Action{Name: "twin_get", Owner: "requests", Tool: "gitlab_group_twin_get"}, handler: "Pair"},
		{act: Action{Name: "twin_field", Owner: "requests", Tool: "gitlab_project_twin_field"}, handler: "Raw"},
		{act: Action{Name: "twin_field", Owner: "requests", Tool: "gitlab_group_twin_field"}, handler: "Table"},
	}
	for _, testCase := range cases {
		t.Run(testCase.act.Tool, func(t *testing.T) {
			matched := Match(sites, testCase.act)
			if len(matched) != 1 {
				t.Fatalf("Match() joined %d site(s), want one", len(matched))
			}
			if len(matched[0].Handlers) != 1 || matched[0].Handlers[0].Func.Name() != testCase.handler {
				t.Errorf("the site joined routes to %+v, want %s", matched[0].Handlers, testCase.handler)
			}
		})
	}
}

// sitesDeadline bounds resolving every construction site of the tree, which
// takes seconds. A resolver step that stopped counting toward its bound fans
// out without end through the tree's helpers, and is reported as a stall
// rather than left to run into the binary's own timeout, which would hide
// which step it was behind the time it took.
const sitesDeadline = 90 * time.Second

// sitesWithin resolves prog's construction sites on a goroutine of their own
// and fails the test when they are not resolved within [sitesDeadline].
func sitesWithin(t *testing.T, prog *Program) map[string][]Site {
	t.Helper()
	done := make(chan map[string][]Site, 1)
	go func() { done <- prog.Sites() }()
	select {
	case sites := <-done:
		return sites
	case <-time.After(sitesDeadline):
		t.Fatalf("resolving the tree's construction sites did not finish within %s", sitesDeadline)
		return nil
	}
}

// clientGoSourceDir returns the directory of the client-go root package the
// module at root builds against, which is what cmd/internal/sdkroutes reads.
func clientGoSourceDir(t *testing.T, root string) string {
	t.Helper()
	cfg := &packages.Config{Context: t.Context(), Mode: packages.NeedName | packages.NeedFiles, Dir: root}
	loaded, err := packages.Load(cfg, ClientGoPath)
	if err != nil || len(loaded) != 1 || len(loaded[0].GoFiles) == 0 {
		t.Fatalf("locate %s from %s: %d package(s), %v", ClientGoPath, root, len(loaded), err)
	}
	return filepath.Dir(loaded[0].GoFiles[0])
}

// TestMatch_EveryCatalogAction_JoinsOneSiteWhoseMethodsSDKRoutesReads holds
// the two facts the request derivation stands on to every action this
// repository publishes, not to the read-only half cmd/audit_readonly_graphql
// walks. Each action meets exactly one construction site, and that site's
// route resolves to a handler. Every client-go method a body reachable from
// those handlers names is a method cmd/internal/sdkroutes reads, and none of
// them sends a request whose path the reading could not fold.
//
// The readonly audit asks the first of these of read-only actions alone, so
// without this test a mutating action handed a twin, or one whose route
// resolves to nothing, passes every gate. Nothing else asks the second at
// all: a client-go upgrade that renames a service interface empties the join
// between the two readers without failing either of them.
func TestMatch_EveryCatalogAction_JoinsOneSiteWhoseMethodsSDKRoutesReads(t *testing.T) {
	root := repoRoot(t)
	prog, err := Load(root, []string{"./internal/..."}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sdk := sdkroutes.Read(clientGoSourceDir(t, root))
	sites := sitesWithin(t, prog)
	byID := realCatalog(t)

	reached := make(map[*types.Func]bool)
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		matched := Match(sites, byID[id])
		if len(matched) != 1 {
			t.Errorf("%s joins %d construction site(s), want exactly one", id, len(matched))
			continue
		}
		roots := prog.Roots(matched)
		if len(roots) == 0 {
			t.Errorf("%s routes to no handler", id)
			continue
		}
		maps.Copy(reached, prog.Reachable(roots))
	}

	named := make(map[string]bool)
	for fn := range reached {
		if body, ok := prog.Function(fn); ok {
			for _, key := range body.SDKMethods {
				named[key] = true
			}
		}
	}
	if len(named) == 0 {
		t.Fatal("no reachable body names a client-go method, so the join between the two readers was never asked")
	}
	for _, key := range slices.Sorted(maps.Keys(named)) {
		method, ok := sdk.Method(key)
		if !ok {
			t.Errorf("a handler names %s, which is not a method sdkroutes reads", key)
			continue
		}
		if len(method.Unresolved) > 0 {
			t.Errorf("%s sends a request whose path sdkroutes could not fold: %v", key, method.Unresolved)
		}
	}
}

// rootsReach returns the names of the functions the walk from one action's
// roots reaches that the program holds a body for, and the service methods
// they name, sorted.
func rootsReach(t *testing.T, prog *Program, act Action) (names, methods []string) {
	t.Helper()
	matched := Match(prog.Sites(), act)
	if len(matched) != 1 {
		t.Fatalf("%s joined %d site(s), want one", act.Name, len(matched))
	}
	for fn := range prog.Reachable(prog.Roots(matched)) {
		body, ok := prog.Function(fn)
		if !ok {
			continue
		}
		names = append(names, fn.Name())
		methods = append(methods, body.SDKMethods...)
	}
	sort.Strings(names)
	sort.Strings(methods)
	return names, methods
}

// TestRoots_ALiteralCallingABoundParameter_ReachesWhatItsCallerBound verifies
// the shape of the award emoji deletes: a route helper wraps the delete it was
// handed in a literal of its own, and the literal calls it through a
// parameter. The literal is one node bound to a different delete at each
// call, so it is indexed per call: indexed once, every action routed through
// it would reach the first one's delete.
func TestRoots_ALiteralCallingABoundParameter_ReachesWhatItsCallerBound(t *testing.T) {
	prog := loadFixture(t, requestSources())

	cases := []struct {
		action      string
		wantReached string
		wantMethods []string
	}{
		{action: "delete_project", wantReached: "DeleteProject", wantMethods: []string{"Projects.DeleteProject"}},
		{action: "delete_issue", wantReached: "DeleteIssue", wantMethods: []string{"Issues.DeleteIssue"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.action, func(t *testing.T) {
			names, methods := rootsReach(t, prog, Action{Name: testCase.action, Owner: "requests"})
			if !slices.Contains(names, literalName) || !slices.Contains(names, testCase.wantReached) {
				t.Errorf("%s reaches %v, want the literal and %s", testCase.action, names, testCase.wantReached)
			}
			if !slices.Equal(methods, testCase.wantMethods) {
				t.Errorf("%s reaches service methods %v, want only %v", testCase.action, methods, testCase.wantMethods)
			}
		})
	}
}

// TestRoots_ALiteralHandler_IsARootOfItsOwn verifies a handler written as a
// literal is walked from a stand-in holding its own body, so what the literal
// names directly is reached as well as what its callees do.
func TestRoots_ALiteralHandler_IsARootOfItsOwn(t *testing.T) {
	prog := loadFixture(t, mainSources())
	matched := Match(prog.Sites(), Action{Name: "closure", Owner: "shapes"})

	roots := prog.Roots(matched)

	if len(roots) != 1 || roots[0].Name() != literalName {
		t.Fatalf("Roots() = %v, want the literal's stand-in", roots)
	}
	reached := prog.Reachable(roots)
	for _, want := range []string{"closureAudit", "closureBody", "closureCleanup"} {
		t.Run(want, func(t *testing.T) {
			if !reached[lookupFunc(t, prog, "shapes", want)] {
				t.Errorf("the literal's walk does not reach %s", want)
			}
		})
	}
}

// TestRoots_ALiteralReachedTwiceInOneWalk_IsIndexedOnce verifies the guard
// that ends a literal bound back to itself: within one call a literal seen
// again is the stand-in already made, not a second indexing.
func TestRoots_ALiteralReachedTwiceInOneWalk_IsIndexedOnce(t *testing.T) {
	prog := loadFixture(t, mainSources())
	matched := Match(prog.Sites(), Action{Name: "closure", Owner: "shapes"})
	twice := Site{Handlers: []Handler{matched[0].Handlers[0], matched[0].Handlers[0]}}

	roots := prog.Roots([]Site{twice})

	if len(roots) != 2 || roots[0] != roots[1] {
		t.Errorf("Roots() = %v, want one stand-in twice", roots)
	}
}

// TestBoundHandlers_ANameBoundToNothing_IsPassedOver verifies the literal's
// parameters are resolved through the frame only where the frame binds them:
// a variable the literal names that its frame holds no binding for, and a
// bound one named a second time, add nothing.
func TestBoundHandlers_ANameBoundToNothing_IsPassedOver(t *testing.T) {
	info := synthInfo()
	handlerType := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	bound, at := synthBound(info, "fn", handlerType, &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}})
	again := ast.NewIdent("fn")
	info.Uses[again] = info.Uses[bound]
	unbound := ast.NewIdent("other")
	info.Uses[unbound] = types.NewVar(token.NoPos, nil, "other", handlerType)
	lit := &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.ExprStmt{X: bound}, &ast.ExprStmt{X: again}, &ast.ExprStmt{X: unbound},
	}}}

	found := synthResolver().boundHandlers(Handler{Lit: lit, pkg: at.pkg, at: at})

	if len(found) != 1 || found[0].handler.Lit == nil || found[0].variable != info.Uses[bound] {
		t.Errorf("boundHandlers() = %+v, want the one literal fn is bound to, through fn", found)
	}
}
