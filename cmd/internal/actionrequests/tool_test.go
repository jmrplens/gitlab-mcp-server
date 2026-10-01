package actionrequests

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

// TestSites_TheToolNameEachConstructionDeclares verifies the tool name a site
// carries, for every way this tree hands it to the constructor, and that a
// construction whose options name no tool carries none rather than a guess.
func TestSites_TheToolNameEachConstructionDeclares(t *testing.T) {
	cases := []struct {
		name    string
		sources map[string]string
		pkg     string
		action  string
		want    []string
	}{
		{
			name: "options built in a literal and amended by a helper", sources: requestSources(), pkg: "requests",
			action: "twin_get", want: []string{"gitlab_project_twin_get", "gitlab_group_twin_get"},
		},
		{
			name: "a field assignment of the whole spec, and of its name over a literal", sources: requestSources(), pkg: "requests",
			action: "twin_field", want: []string{"gitlab_project_twin_field", "gitlab_group_twin_field"},
		},
		{
			name: "a spec literal's IndividualTool field", sources: requestSources(), pkg: "requests",
			action: "literal_tool", want: []string{"gitlab_literal_tool"},
		},
		{name: "options that name no tool", sources: requestSources(), pkg: "requests", action: "expression", want: []string{""}},
		{name: "a spec literal with no IndividualTool field", sources: mainSources(), pkg: "shapes", action: "literal", want: []string{""}},
		{name: "a spec held in a variable", sources: mainSources(), pkg: "shapes", action: "variable", want: []string{""}},
		{name: "a decorated spec", sources: mainSources(), pkg: "shapes", action: "chained", want: []string{""}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got []string
			for _, site := range loadFixture(t, testCase.sources).Sites()[testCase.action] {
				if site.Package == testCase.pkg {
					got = append(got, site.Tool)
				}
			}
			if len(got) != len(testCase.want) {
				t.Fatalf("%s carries tool names %q, want %q", testCase.action, got, testCase.want)
			}
			for i := range got {
				if got[i] != testCase.want[i] {
					t.Errorf("site %d of %s carries tool %q, want %q", i, testCase.action, got[i], testCase.want[i])
				}
			}
		})
	}
}

// synthOptionTypes builds the two toolutil types the tool name travels in.
func synthOptionTypes(pkg *types.Package) (options, toolSpec types.Type) {
	options = types.NewNamed(types.NewTypeName(token.NoPos, pkg, optionsTypeName, nil), types.NewStruct(nil, nil), nil)
	toolSpec = types.NewNamed(types.NewTypeName(token.NoPos, pkg, toolSpecTypeName, nil), types.NewStruct(nil, nil), nil)
	return options, toolSpec
}

// keyed builds a keyed composite literal element.
func keyed(field string, value ast.Expr) *ast.KeyValueExpr {
	return &ast.KeyValueExpr{Key: ast.NewIdent(field), Value: value}
}

// synthToolSpec is an IndividualToolSpec literal naming tool.
func synthToolSpec(info *types.Info, tool string) *ast.CompositeLit {
	return &ast.CompositeLit{Elts: []ast.Expr{keyed(nameField, synthString(info, tool))}}
}

// synthOptions is an ActionSpecOptions literal whose IndividualTool names tool.
func synthOptions(info *types.Info, tool string) *ast.CompositeLit {
	return &ast.CompositeLit{Elts: []ast.Expr{keyed(individualToolField, synthToolSpec(info, tool))}}
}

// synthSpecWithTool is an ActionSpec literal declaring an individual tool.
func synthSpecWithTool(info *types.Info, tool string) *ast.CompositeLit {
	return &ast.CompositeLit{Elts: []ast.Expr{
		keyed(nameField, synthString(info, "deep.action")),
		keyed(individualToolField, synthToolSpec(info, tool)),
	}}
}

// synthAssignedOptions builds a frame whose body assigns into a variable's
// IndividualTool field (or, with name set, that field's Name), and an
// identifier naming the variable in that frame.
func synthAssignedOptions(info *types.Info, optionsType types.Type, rhs ast.Expr, name bool) (*ast.Ident, frame) {
	variable := types.NewVar(token.NoPos, nil, "options", optionsType)
	use, root := ast.NewIdent("options"), ast.NewIdent("options")
	info.Uses[use], info.Uses[root] = variable, variable
	var lhs ast.Expr = &ast.SelectorExpr{X: root, Sel: ast.NewIdent(individualToolField)}
	if name {
		lhs = &ast.SelectorExpr{X: lhs, Sel: ast.NewIdent(nameField)}
	}
	decl := &ast.FuncDecl{Body: &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}},
	}}}
	return use, synthFrame(info, decl)
}

// toolProbe is one recursive step of the tool resolvers, resolved at a depth.
// steps is how many levels the resolution descends from the probe's entry to
// the string that names the tool, each step one call that adds one to the
// depth.
type toolProbe struct {
	name    string
	steps   int
	resolve func(depth int) string
}

// toolSteps builds one probe per place a tool resolver calls another with a
// deeper depth, as recursiveSteps does for the action name and the handlers.
func toolSteps() []toolProbe {
	toolutilPkg, specType, routeType := synthToolutilTypes()
	optionsType, toolSpecType := synthOptionTypes(toolutilPkg)
	str := types.Typ[types.String]
	domain := types.NewPackage("example.com/domain", "domain")
	info := synthInfo()
	at := synthFrame(info, nil)

	specIdent, specAt := synthBound(info, "spec", specType, synthSpecWithTool(info, "gitlab_deep"))
	constructor := synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType, optionsType}, []types.Type{specType})
	constructorCall := synthCall(info, constructor, synthString(info, "deep.action"), synthRoute(), synthOptions(info, "gitlab_deep"))
	specHelper := synthFunc(domain, "buildSpec", nil, []types.Type{specType})
	specHelperRes := synthIndexed(info, specHelper, &ast.BlockStmt{List: []ast.Stmt{
		&ast.ReturnStmt{Results: []ast.Expr{synthSpecWithTool(info, "gitlab_deep")}},
	}})
	decorator := synthFunc(toolutilPkg, "WithTags", nil, []types.Type{specType})
	decorated := synthMethodCall(info, decorator, synthSpecWithTool(info, "gitlab_deep"))
	optionsIdent, optionsAt := synthBound(info, "options", optionsType, synthOptions(info, "gitlab_deep"))
	assignedSpec, assignedSpecAt := synthAssignedOptions(info, optionsType, synthToolSpec(info, "gitlab_deep"), false)
	assignedName, assignedNameAt := synthAssignedOptions(info, optionsType, synthString(info, "gitlab_deep"), true)
	optionsHelper := synthFunc(domain, "buildOptions", nil, []types.Type{optionsType})
	optionsHelperRes := synthIndexed(info, optionsHelper, &ast.BlockStmt{List: []ast.Stmt{
		&ast.ReturnStmt{Results: []ast.Expr{synthOptions(info, "gitlab_deep")}},
	}})
	toolIdent, toolAt := synthBound(info, "tool", toolSpecType, synthToolSpec(info, "gitlab_deep"))
	toolHelper := synthFunc(domain, "buildTool", nil, []types.Type{toolSpecType})
	toolHelperRes := synthIndexed(info, toolHelper, &ast.BlockStmt{List: []ast.Stmt{
		&ast.ReturnStmt{Results: []ast.Expr{synthToolSpec(info, "gitlab_deep")}},
	}})

	return []toolProbe{
		{name: "a spec held in an identifier", steps: 3, resolve: func(d int) string { return synthResolver().resolveSpecTool(specIdent, specAt, d) }},
		{name: "the IndividualTool field of a spec literal", steps: 2, resolve: func(d int) string {
			return synthResolver().resolveSpecTool(synthSpecWithTool(info, "gitlab_deep"), at, d)
		}},
		{name: "the options a toolutil constructor is given", steps: 3, resolve: func(d int) string {
			return synthResolver().resolveSpecCallTool(constructorCall, at, d)
		}},
		{name: "the spec a helper returns", steps: 3, resolve: func(d int) string {
			return specHelperRes.resolveSpecCallTool(synthCall(info, specHelper), at, d)
		}},
		{name: "the receiver of a decorating method", steps: 3, resolve: func(d int) string { return synthResolver().resolveSpecCallTool(decorated, at, d) }},
		{name: "options held in an identifier", steps: 3, resolve: func(d int) string {
			return synthResolver().resolveOptionsTool(optionsIdent, optionsAt, d)
		}},
		{name: "a tool spec assigned into the options", steps: 2, resolve: func(d int) string {
			return synthResolver().resolveOptionsTool(assignedSpec, assignedSpecAt, d)
		}},
		{name: "a tool name assigned into the options", steps: 1, resolve: func(d int) string {
			return synthResolver().resolveOptionsTool(assignedName, assignedNameAt, d)
		}},
		{name: "the IndividualTool field of an options literal", steps: 2, resolve: func(d int) string {
			return synthResolver().resolveOptionsTool(synthOptions(info, "gitlab_deep"), at, d)
		}},
		{name: "the options a helper returns", steps: 3, resolve: func(d int) string {
			return optionsHelperRes.resolveOptionsTool(synthCall(info, optionsHelper), at, d)
		}},
		{name: "a tool spec held in an identifier", steps: 2, resolve: func(d int) string { return synthResolver().resolveToolSpec(toolIdent, toolAt, d) }},
		{name: "the Name field of a tool spec literal", steps: 1, resolve: func(d int) string {
			return synthResolver().resolveToolSpec(synthToolSpec(info, "gitlab_deep"), at, d)
		}},
		{name: "the tool spec a helper returns", steps: 2, resolve: func(d int) string {
			return toolHelperRes.resolveToolSpec(synthCall(info, toolHelper), at, d)
		}},
	}
}

// TestToolResolvers_EveryRecursiveStep_CountsOneAgainstTheBound verifies each
// step the tool resolvers take counts exactly one against the bound, as it
// does for the name and the handlers: a probe of n steps resolves the tool
// from n short of the bound, where the string it ends on sits at the bound,
// and nothing from one deeper. A step that counted nothing or counted down
// would still resolve one deeper, and one that counted two would not resolve
// at all.
func TestToolResolvers_EveryRecursiveStep_CountsOneAgainstTheBound(t *testing.T) {
	for _, probe := range toolSteps() {
		t.Run(probe.name, func(t *testing.T) {
			deepest := maxResolveDepth - probe.steps
			if got := probe.resolve(deepest); got != "gitlab_deep" {
				t.Errorf("resolving %d step(s) short of the bound = %q, want the fixture tool", probe.steps, got)
			}
			if got := probe.resolve(deepest + 1); got != "" {
				t.Errorf("resolving one level deeper = %q, want nothing: its last step is past the bound", got)
			}
		})
	}
}

// TestToolResolvers_PastTheBound_ResolveNothing verifies the three resolvers
// that guard their own entry answer nothing past the bound.
func TestToolResolvers_PastTheBound_ResolveNothing(t *testing.T) {
	info := synthInfo()
	at := synthFrame(info, nil)
	res := synthResolver()
	beyond := maxResolveDepth + 1

	if got := res.resolveSpecTool(synthSpecWithTool(info, "gitlab_deep"), at, beyond); got != "" {
		t.Errorf("resolveSpecTool() past the bound = %q", got)
	}
	if got := res.resolveOptionsTool(synthOptions(info, "gitlab_deep"), at, beyond); got != "" {
		t.Errorf("resolveOptionsTool() past the bound = %q", got)
	}
	if got := res.resolveToolSpec(synthToolSpec(info, "gitlab_deep"), at, beyond); got != "" {
		t.Errorf("resolveToolSpec() past the bound = %q", got)
	}
}

// TestToolResolvers_ExpressionsThatCarryNoTool_ResolveToNothing verifies each
// shape the tool resolvers refuse: a node kind that carries no value, a call
// whose callee cannot be named or is neither a constructor, a helper with a
// body nor a method, a name bound to nothing, and a literal with no field
// that names the tool.
func TestToolResolvers_ExpressionsThatCarryNoTool_ResolveToNothing(t *testing.T) {
	info := synthInfo()
	at := synthFrame(info, nil)
	res := synthResolver()
	literalCall := &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}}
	toolutilPkg, specType, _ := synthToolutilTypes()
	optionsType, toolSpecType := synthOptionTypes(toolutilPkg)
	opaque := synthFunc(types.NewPackage("example.com/elsewhere", "elsewhere"), "opaque", nil, []types.Type{specType})
	unbound := ast.NewIdent("unbound")
	info.Uses[unbound] = types.NewVar(token.NoPos, nil, "unbound", optionsType)
	untool := synthFunc(types.NewPackage("example.com/domain", "domain"), "untool", nil, []types.Type{toolSpecType})
	untoolRes := synthIndexed(info, untool, &ast.BlockStmt{List: []ast.Stmt{
		&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{}}},
	}})
	selector := &ast.SelectorExpr{X: ast.NewIdent("x"), Sel: ast.NewIdent("Options")}

	cases := []struct {
		name    string
		resolve func() string
	}{
		{name: "a spec that is a basic literal", resolve: func() string { return res.resolveSpecTool(&ast.BasicLit{Kind: token.INT, Value: "1"}, at, 0) }},
		{name: "a spec call whose callee cannot be named", resolve: func() string { return res.resolveSpecCallTool(literalCall, at, 0) }},
		{name: "a spec call into a function with no body here", resolve: func() string {
			return res.resolveSpecCallTool(synthCall(info, opaque), at, 0)
		}},
		{name: "options read off a selector", resolve: func() string { return res.resolveOptionsTool(selector, at, 0) }},
		{name: "options bound to nothing", resolve: func() string { return res.resolveOptionsTool(unbound, at, 0) }},
		{name: "a tool spec read off a selector", resolve: func() string { return res.resolveToolSpec(selector, at, 0) }},
		{name: "a tool spec bound to nothing", resolve: func() string { return res.resolveToolSpec(unbound, at, 0) }},
		{name: "a tool spec literal with no Name", resolve: func() string { return res.resolveToolSpec(&ast.CompositeLit{}, at, 0) }},
		{name: "a tool spec call whose callee cannot be named", resolve: func() string { return res.resolveToolSpec(literalCall, at, 0) }},
		{name: "a helper returning a tool spec with no Name", resolve: func() string {
			return untoolRes.resolveToolSpec(synthCall(info, untool), at, 0)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.resolve(); got != "" {
				t.Errorf("resolved %q, want nothing", got)
			}
		})
	}
}

// TestAssignedTool_OnlyAnAssignmentIntoTheVariablesToolCounts verifies the
// field assignments read: one into the variable's IndividualTool or its Name
// is the tool, and an assignment of several values from one call, an
// assignment into an indexed element, one into another variable's tool or its
// Name, one into another field of the variable or of its IndividualTool, one
// into a field below the Name, and one of the whole variable are not. The
// last of two writes that count wins.
func TestAssignedTool_OnlyAnAssignmentIntoTheVariablesToolCounts(t *testing.T) {
	info := synthInfo()
	toolutilPkg, _, _ := synthToolutilTypes()
	optionsType, _ := synthOptionTypes(toolutilPkg)
	variable := types.NewVar(token.NoPos, nil, "options", optionsType)
	other := types.NewVar(token.NoPos, nil, "other", optionsType)
	ident := func(name string, obj types.Object) *ast.Ident {
		id := ast.NewIdent(name)
		info.Uses[id] = obj
		return id
	}
	field := func(root *ast.Ident, path ...string) ast.Expr {
		var expr ast.Expr = root
		for _, name := range path {
			expr = &ast.SelectorExpr{X: expr, Sel: ast.NewIdent(name)}
		}
		return expr
	}
	assign := func(lhs []ast.Expr, rhs ...ast.Expr) ast.Stmt {
		return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs}
	}
	// Every write that must not count comes after the last one that does, so
	// reading any of them would change the answer.
	body := &ast.BlockStmt{List: []ast.Stmt{
		assign([]ast.Expr{field(ident("options", variable), individualToolField, nameField)}, synthString(info, "gitlab_first")),
		assign([]ast.Expr{field(ident("options", variable), individualToolField)}, synthToolSpec(info, "gitlab_last")),
		assign([]ast.Expr{ast.NewIdent("a"), ast.NewIdent("b")}, &ast.CallExpr{Fun: ast.NewIdent("pair")}),
		assign([]ast.Expr{&ast.IndexExpr{X: ast.NewIdent("m"), Index: ast.NewIdent("k")}}, synthString(info, "gitlab_index")),
		assign([]ast.Expr{field(ident("other", other), individualToolField)}, synthToolSpec(info, "gitlab_other")),
		assign([]ast.Expr{field(ident("other", other), individualToolField, nameField)}, synthString(info, "gitlab_other_name")),
		assign([]ast.Expr{field(ident("options", variable), "Tags")}, synthString(info, "gitlab_tags")),
		assign([]ast.Expr{field(ident("options", variable), individualToolField, "Description")}, synthString(info, "gitlab_description")),
		assign([]ast.Expr{field(ident("options", variable), individualToolField, nameField, "Inner")}, synthString(info, "gitlab_inner")),
		assign([]ast.Expr{field(ident("options", variable))}, synthOptions(info, "gitlab_whole")),
	}}
	at := synthFrame(info, &ast.FuncDecl{Body: body})

	if got := synthResolver().assignedTool(ident("options", variable), at, 0); got != "gitlab_last" {
		t.Errorf("assignedTool() = %q, want the last write into the variable's tool", got)
	}
	if got := synthResolver().assignedTool(ast.NewIdent("unknown"), at, 0); got != "" {
		t.Errorf("assignedTool() of a name the type checker gave no variable = %q, want nothing", got)
	}
}

// TestSelectorPath_TheChainAFieldIsReachedThrough verifies how a selection is
// split: a chain of selections on a name gives the name and the fields, and
// anything else gives no root.
func TestSelectorPath_TheChainAFieldIsReachedThrough(t *testing.T) {
	root := ast.NewIdent("options")
	chain := &ast.SelectorExpr{X: &ast.ParenExpr{X: &ast.SelectorExpr{X: root, Sel: ast.NewIdent("IndividualTool")}}, Sel: ast.NewIdent("Name")}

	path, got := selectorPath(chain)
	if got != root || len(path) != 2 || path[0] != "IndividualTool" || path[1] != "Name" {
		t.Errorf("selectorPath() = %v, %v; want [IndividualTool Name] on options", path, got)
	}
	if onCall, callRoot := selectorPath(&ast.SelectorExpr{X: &ast.CallExpr{Fun: ast.NewIdent("f")}, Sel: ast.NewIdent("Name")}); callRoot != nil || onCall != nil {
		t.Errorf("selectorPath() of a selection on a call = %v, %v; want no root", onCall, callRoot)
	}
}

// TestFieldValue_KeyedElementsOnly verifies a field is read only from a keyed
// element whose key is the field's name.
func TestFieldValue_KeyedElementsOnly(t *testing.T) {
	value := ast.NewIdent("value")
	lit := &ast.CompositeLit{Elts: []ast.Expr{
		&ast.BasicLit{Kind: token.STRING, Value: `"positional"`},
		&ast.KeyValueExpr{Key: &ast.BasicLit{Kind: token.INT, Value: "0"}, Value: ast.NewIdent("indexed")},
		keyed("Other", ast.NewIdent("other")),
		keyed(nameField, value),
	}}

	if got, ok := fieldValue(lit, nameField); !ok || got != value {
		t.Errorf("fieldValue(Name) = %v, %t; want the keyed value", got, ok)
	}
	if got, ok := fieldValue(lit, individualToolField); ok || got != nil {
		t.Errorf("fieldValue(IndividualTool) = %v, %t; want nothing", got, ok)
	}
}

// TestSpecConstructorOptions_OnlyTheOptionsOfTheConstructorShape verifies the
// options argument is read only from a toolutil constructor of the (name,
// route, options) shape, called with all three.
func TestSpecConstructorOptions_OnlyTheOptionsOfTheConstructorShape(t *testing.T) {
	toolutilPkg, specType, routeType := synthToolutilTypes()
	optionsType, _ := synthOptionTypes(toolutilPkg)
	str := types.Typ[types.String]
	info := synthInfo()
	name, route, options := synthString(info, "action"), synthRoute(), synthOptions(info, "gitlab_tool")

	cases := []struct {
		name   string
		callee *types.Func
		args   []ast.Expr
		want   bool
	}{
		{
			name:   "the constructor shape, called with its options",
			callee: synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType, optionsType}, []types.Type{specType}),
			args:   []ast.Expr{name, route, options}, want: true,
		},
		{
			name:   "a function that is no constructor",
			callee: synthFunc(types.NewPackage("example.com/domain", "domain"), "helper", []types.Type{str, routeType, optionsType}, []types.Type{specType}),
			args:   []ast.Expr{name, route, options},
		},
		{
			name:   "a constructor taking no options",
			callee: synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType}, []types.Type{specType}),
			args:   []ast.Expr{name, route},
		},
		{
			name:   "a constructor called without its options",
			callee: synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType, optionsType}, []types.Type{specType}),
			args:   []ast.Expr{name, route},
		},
		{
			name:   "a constructor whose third parameter is something else",
			callee: synthFunc(toolutilPkg, "NewActionSpec", []types.Type{str, routeType, str}, []types.Type{specType}),
			args:   []ast.Expr{name, route, name},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := specConstructorOptions(testCase.callee, &ast.CallExpr{Args: testCase.args})
			if ok != testCase.want || (ok && got != options) {
				t.Errorf("specConstructorOptions() = %v, %t; want the options argument: %t", got, ok, testCase.want)
			}
		})
	}
}
