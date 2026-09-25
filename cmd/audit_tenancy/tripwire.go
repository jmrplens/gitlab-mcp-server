package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// The parts of G10, as an exemption records which one it answered.
const (
	partConstructors = "constructors"
	partLiterals     = "literals"
	partNames        = "names"
)

// checkTripwire is G10: nothing limit-shaped exists outside a declared site.
//
//   - (a) A call of a limit constructor sits inside a declared site, and none
//     of its arguments is a literal or a package-level value no row declares.
//   - (b) A refusal literal whose code is a policy code, or is not constant,
//     or whose status is 429 or 503, sits inside a declared site.
//   - (c) In every package that holds a value or enforcing site of a row that
//     is not a request bound, a package-level const or var whose name reads
//     as a limit is a declared site.
//
// A declaration that is none of these, and is still shaped like one, is
// answered by the exemption table with a category and a reason.
func (g *gate) checkTripwire() []Finding {
	var found []Finding
	found = append(found, g.constructorFindings()...)
	found = append(found, g.literalFindings()...)
	found = append(found, g.nameFindings()...)
	return found
}

// covered reports whether a declaration is a declared site, or an exemption,
// recording which part of G10 the exemption answered.
func (g *gate) covered(key, part string) bool {
	if g.declared[key] {
		return true
	}
	if _, exempt := g.exempt[key]; exempt {
		g.used[key] = part
		return true
	}
	return false
}

// constructorFindings is G10(a).
func (g *gate) constructorFindings() []Finding {
	config := map[string]bool{}
	for _, d := range g.reg.decisions {
		for _, field := range d.Config {
			config[field] = true
		}
	}
	var found []Finding
	g.p.forEachCall(func(key string, info *types.Info, call *ast.CallExpr) {
		name, isLimit := g.limitConstructor(info, call)
		if !isLimit {
			return
		}
		if !g.covered(key, partConstructors) {
			found = append(found, Finding{
				Rule: "G10", Subject: key, Position: g.p.position(call.Pos()),
				Message: fmt.Sprintf("builds a limit with %s outside every declared site", name),
			})
		}
		args := call.Args
		if name == "make" {
			args = args[1:]
		}
		for _, arg := range args {
			for _, why := range g.undeclaredInputs(info, arg, config) {
				found = append(found, Finding{
					Rule: "G10", Subject: key, Position: g.p.position(arg.Pos()),
					Message: fmt.Sprintf("passes %s to %s", why, name),
				})
			}
		}
	})
	return found
}

// limitConstructor reports whether a call builds a limit: one of the listed
// constructors, or a channel of empty structs made with a capacity other than
// the literal 0 or 1, which is how a semaphore is written.
func (g *gate) limitConstructor(info *types.Info, call *ast.CallExpr) (string, bool) {
	if id, isIdent := ast.Unparen(call.Fun).(*ast.Ident); isIdent {
		if _, isBuiltin := info.Uses[id].(*types.Builtin); isBuiltin && id.Name == "make" {
			return "make", isSemaphore(info, call)
		}
	}
	fn := calleeOf(info, call)
	if fn == nil {
		return "", false
	}
	name := calleeName(fn)
	return name, slices.Contains(g.rules.constructors, name)
}

// isSemaphore reports whether a make call builds a chan struct{} whose
// capacity is anything but the literal 0 or 1.
func isSemaphore(info *types.Info, call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	ch, isChan := types.Unalias(info.TypeOf(call.Args[0])).Underlying().(*types.Chan)
	if !isChan {
		return false
	}
	if st, isStruct := ch.Elem().Underlying().(*types.Struct); !isStruct || st.NumFields() != 0 {
		return false
	}
	lit, isLit := ast.Unparen(call.Args[1]).(*ast.BasicLit)
	return !isLit || (lit.Value != "0" && lit.Value != "1")
}

// undeclaredInputs lists what, inside one argument of a limit constructor, is
// a number no row declares: a basic literal, or a package-level const or var
// of this program that is no declared site. A selector ending in a field some
// row names in Config is declared, and a value local to the function is
// accepted: it is whatever the caller handed down, and the caller is judged
// where it is declared.
func (g *gate) undeclaredInputs(info *types.Info, arg ast.Expr, config map[string]bool) []string {
	var out []string
	ast.Inspect(arg, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if sel, ok := info.Selections[e]; ok && sel.Kind() == types.FieldVal && config[e.Sel.Name] {
				return false
			}
		case *ast.BasicLit:
			out = append(out, "the literal "+e.Value)
		case *ast.Ident:
			// objectKey names a const or var only at package level in this
			// program, so a local, a field, a universe constant and another
			// module's value all have none.
			obj := info.Uses[e]
			if key := objectKey(obj); isValue(obj) && key != "" && !g.covered(key, partConstructors) {
				out = append(out, key+", which no row declares")
			}
		}
		return true
	})
	return out
}

// isValue reports whether obj is a const or a var, rather than a function, a
// type or a package.
func isValue(obj types.Object) bool {
	switch obj.(type) {
	case *types.Const, *types.Var:
		return true
	default:
		return false
	}
}

// literalFindings is G10(b).
func (g *gate) literalFindings() []Finding {
	var found []Finding
	g.p.forEachTopLevel(func(key string, info *types.Info, node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			lit, isLit := n.(*ast.CompositeLit)
			if !isLit {
				return true
			}
			rl, ok := g.refusalLiteral(info, lit)
			if !ok || !g.refusalShaped(info, rl) {
				return true
			}
			if !g.covered(key, partLiterals) {
				found = append(found, Finding{
					Rule: "G10", Subject: key, Position: g.p.position(lit.Pos()),
					Message: "builds a refusal that reads as a limit's (a policy code, a code that is not constant, or a 429 or 503) outside every declared site",
				})
			}
			return true
		})
	})
	return found
}

// refusalShaped reports whether a refusal literal reads as a limit's: its
// code is a policy code or is not a constant, or its status is 429 or 503. A
// literal that sets no code at all carries the zero code, which is none of
// those.
func (g *gate) refusalShaped(info *types.Info, rl refusalLit) bool {
	if expr, set := rl.fields[rl.typ.code]; set {
		code, folds := foldInt(info, expr)
		if !folds || slices.Contains(g.rules.policyCodes, code) {
			return true
		}
	}
	status, folds := rl.intField(info, rl.typ.status)
	return folds && slices.Contains(g.rules.limitStatuses, status)
}

// nameFindings is G10(c).
func (g *gate) nameFindings() []Finding {
	var found []Finding
	for _, dir := range g.namedPackages() {
		pkg := g.p.byDir[dir]
		if pkg == nil {
			continue
		}
		for _, name := range packageValueNames(pkg.Syntax) {
			if name.Name == "_" || !g.readsAsLimit(name.Name) {
				continue
			}
			if key := siteKey(dir, name.Name); !g.covered(key, partNames) {
				found = append(found, Finding{
					Rule: "G10", Subject: key, Position: g.p.position(name.Pos()),
					Message: "is named like a limit, and no row declares it and no exemption answers it",
				})
			}
		}
	}
	return found
}

// packageValueNames are the names of every package-level const and var the
// files declare.
func packageValueNames(files []*ast.File) []*ast.Ident {
	var names []*ast.Ident
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			for _, spec := range gen.Specs {
				vs, _ := spec.(*ast.ValueSpec)
				names = append(names, vs.Names...)
			}
		}
	}
	return names
}

// namedPackages are the packages part (c) reads, derived from the register on
// each run: every package holding an Alias, Arg, Pin or Enforce site of a row
// that is not a request bound. A row that lands in a new package brings that
// package under the rule.
func (g *gate) namedPackages() []string {
	var dirs []string
	for _, d := range g.reg.decisions {
		if d.Disposition == tenancy.RequestBound {
			continue
		}
		for _, s := range d.Sites {
			switch s.Role {
			case tenancy.Alias, tenancy.Arg, tenancy.Pin, tenancy.Enforce:
				if !slices.Contains(dirs, s.Pkg) {
					dirs = append(dirs, s.Pkg)
				}
			}
		}
	}
	slices.Sort(dirs)
	return dirs
}

// readsAsLimit reports whether one camel-case word of a name is in the
// lexicon.
func (g *gate) readsAsLimit(name string) bool {
	for _, word := range camelWords(name) {
		if slices.Contains(g.rules.lexicon, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

// camelWords splits an identifier at every lower-to-upper boundary, at every
// digit-to-upper one, and before the last capital of an acronym a lower-case
// run follows, so that baseHTTPMaxHeaderBytes reads base, HTTP, Max, Header,
// Bytes.
func camelWords(name string) []string {
	r := []rune(name)
	var words []string
	start := 0
	for i := 1; i < len(r); i++ {
		lowerToUpper := unicode.IsLower(r[i-1]) && unicode.IsUpper(r[i])
		acronymEnd := unicode.IsUpper(r[i-1]) && unicode.IsUpper(r[i]) && i+1 < len(r) && unicode.IsLower(r[i+1])
		digitToUpper := unicode.IsDigit(r[i-1]) && unicode.IsUpper(r[i])
		if lowerToUpper || acronymEnd || digitToUpper {
			words = append(words, string(r[start:i]))
			start = i
		}
	}
	return append(words, string(r[start:]))
}
