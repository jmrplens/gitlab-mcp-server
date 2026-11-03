// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package derive

import (
	"go/ast"
	"go/types"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// This file is which field of an action's input fills each placeholder of a
// route a client-go call sends. R-REQUIRED (cmd/audit_1to1 -scope=required)
// holds every field an input schema
// publishes to whether GitLab requires the parameter it is sent as, and it
// used to find that parameter by its name alone: GitLab's :key_id is the
// input's deploy_key_id, so the field was never judged, and the :id of
// /groups/:id/share was found under group_id, which is also the name of the
// body param beside it. The call says which field fills which segment:
// cmd/internal/sdkroutes reads where each argument of a method lands in its
// routes, and the argument is read back here to the input field it is built
// out of.

// routeFills is what one call fills the placeholders of each route it sends
// from, by the route (its key for a client-go call, its path for a raw
// request) and then by segment: the field's name, or "" for a segment filled
// from no one field. A nil one is a call whose arguments cannot be read.
type routeFills map[string]map[int]string

// of returns what the call fills one route's placeholders from: nothing for a
// call that cannot be read, and an empty set for a route no argument of the
// method lands in.
func (f routeFills) of(route Request) map[int]string {
	if f == nil {
		return nil
	}
	if segments, ok := f[route.Key()]; ok {
		return segments
	}
	return map[int]string{}
}

// fills reads what one client-go call fills each placeholder of its method's
// routes from. A method named without being called there, which is how a
// handler hands a helper the variants it chooses between, has no arguments to
// read. Two arguments landing in one segment fill it from one field only
// when both are read to the same fields.
func (d *deriver) fills(l *leaf, at *frame) routeFills {
	if l.call == nil {
		return nil
	}
	read := map[string]map[int][]string{}
	for _, placed := range d.sdk.Placements(l.sdk) {
		var fields []string
		if placed.Index < len(l.call.Args) {
			fields = d.inputFields(l.call.Args[placed.Index], at, 0)
		}
		segments := read[placed.Route]
		if segments == nil {
			segments = map[int][]string{}
			read[placed.Route] = segments
		}
		if earlier, seen := segments[placed.Segment]; seen && !slices.Equal(earlier, fields) {
			fields = nil
		}
		segments[placed.Segment] = fields
	}
	out := make(routeFills, len(read))
	for route, segments := range read {
		out[route] = settle(segments)
	}
	return out
}

// settle reads the one field each segment of a route is filled from, given
// the fields each is built out of. A segment built out of several is a lookup
// handed what scopes it as well as what it looks up
// (resolveIID(ctx, client, input.ProjectID, input.MilestoneIID)), so a field
// another segment of the route is filled from alone is the scope, and the
// segment is filled from the field left, when exactly one is. Any other
// segment of several fields, and one of none, is filled from no one field.
func settle(built map[int][]string) map[int]string {
	alone := map[string]bool{}
	for _, fields := range built {
		if len(fields) == 1 {
			alone[fields[0]] = true
		}
	}
	settled := make(map[int]string, len(built))
	for segment, fields := range built {
		settled[segment] = only(slices.DeleteFunc(slices.Clone(fields), func(field string) bool {
			return len(fields) > 1 && alone[field]
		}))
	}
	return settled
}

// What an identifier a raw request's path carries folds to while the path is
// folded to read its placeholders: the one input field it is built out of,
// between two bytes no path GitLab routes carries and the fold never writes.
const (
	markOpen  = "\x02"
	markClose = "\x03"
)

// markPattern finds a mark, spelled out as one literal so the initializer
// holds no operator the mutation tool could report as never run.
var markPattern = regexp.MustCompile("\x02[^\x03]*\x03")

// identifier is what an identifier a path carries folds to: the placeholder,
// or, while [deriver.rawFills] folds the path, a mark naming the one input
// field it is built out of when there is one.
func (d *deriver) identifier(expr ast.Expr, at *frame) string {
	if !d.marking {
		return placeholder
	}
	if field := only(d.inputFields(expr, at, 0)); field != "" {
		return markOpen + field + markClose
	}
	return placeholder
}

// rawFills reads what a raw request's path fills each placeholder of the
// route it names from, by the route's shape: the path folded once more with
// every identifier marked with the field it is built out of. A segment that
// is one mark is filled from that field, and one that holds an identifier
// otherwise, or a field two spellings of the same shape disagree on, from
// none.
func (d *deriver) rawFills(path ast.Expr, at *frame) routeFills {
	d.marking = true
	spellings := d.fold(path, at, 0)
	d.marking = false
	out := routeFills{}
	for _, marked := range spellings {
		shape, ok := normalizePath(markPattern.ReplaceAllLiteralString(marked, placeholder))
		if !ok {
			continue
		}
		segments := map[int]string{}
		for i, segment := range pathSegments(marked) {
			if markPattern.FindString(segment) == segment {
				segments[i] = strings.TrimSuffix(strings.TrimPrefix(segment, markOpen), markClose)
			}
		}
		earlier, seen := out[shape]
		if !seen {
			out[shape] = segments
			continue
		}
		for segment, field := range earlier {
			if segments[segment] != field {
				earlier[segment] = ""
			}
		}
	}
	return out
}

// only returns the one name of a list, and "" for none or several.
func only(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return ""
}

// fillState is what the calls recorded so far fill one request's
// placeholders from: by segment, the field every call agreed on, or "" where
// two disagreed, and whether a call that could not be read recorded it, which
// leaves every segment unknown.
type fillState struct {
	fields  map[int]string
	unknown bool
}

// merge folds what one more call fills the request's placeholders from into
// what the earlier ones did. A segment the earlier calls agreed on keeps its
// field only when this call fills it from the same one; a segment this call
// fills and an earlier one did not is left without, since that earlier call
// filled it from no field of the input.
func (s *fillState) merge(fills map[int]string) {
	if fills == nil {
		s.unknown = true
		return
	}
	if s.fields == nil {
		s.fields = maps.Clone(fills)
		return
	}
	for segment, field := range s.fields {
		if fills[segment] != field {
			s.fields[segment] = ""
		}
	}
}

// placeholders returns the segments every recorded call fills from one field
// of the input, nothing when any call could not be read.
func (s *fillState) placeholders() map[int]string {
	if s.unknown {
		return nil
	}
	out := map[int]string{}
	for segment, field := range s.fields {
		if field != "" {
			out[segment] = field
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// inputFields names, sorted and once each, the fields of the action's input
// an expression is built out of: a field selected on the input, read through
// the conversions, operators and calls it passes through, the locals it is
// assigned to, the parameters it is handed down as, and the composite literal
// of a struct a field is given in. A field selected on anything else is a
// value GitLab or a helper produced and names none, except the outer field of
// a chain the input heads (`input.Position.BaseSHA` is position). depth is
// how many reads enclose this one, which keeps a local assigned from itself
// from being read again without end.
func (d *deriver) inputFields(expr ast.Expr, at *frame, depth int) []string {
	depth++
	if depth > maxFoldDepth {
		return nil
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return d.identFields(typed, at, depth)
	case *ast.SelectorExpr:
		return d.selectorFields(typed, at, depth)
	case *ast.CallExpr:
		return d.callFields(typed, at, depth)
	case *ast.StarExpr:
		return d.inputFields(typed.X, at, depth)
	case *ast.UnaryExpr:
		return d.inputFields(typed.X, at, depth)
	case *ast.BinaryExpr:
		return sortedNames(slices.Concat(d.inputFields(typed.X, at, depth), d.inputFields(typed.Y, at, depth)))
	}
	return nil
}

// identFields reads a name: a parameter through what the caller handed it, a
// local through every value assigned to it, and a constant, a function or a
// parameter no call bound as nothing.
func (d *deriver) identFields(ident *ast.Ident, at *frame, depth int) []string {
	variable, ok := at.pkg.TypesInfo.Uses[ident].(*types.Var)
	if !ok {
		return nil
	}
	if bound, isBound := at.env[variable]; isBound {
		return d.inputFields(bound.expr, bound.at, depth)
	}
	var found []string
	for _, value := range valuesOf(at, variable) {
		found = append(found, d.inputFields(value, at, depth)...)
	}
	return sortedNames(found)
}

// selectorFields reads a selection: a field of the input names itself, a
// field of a literal the value it was given, a field of a field the outer
// one, and a method value what its receiver is built out of. A qualified
// name (a package's variable or constant) names nothing.
func (d *deriver) selectorFields(sel *ast.SelectorExpr, at *frame, depth int) []string {
	selection, ok := at.pkg.TypesInfo.Selections[sel]
	if !ok {
		return nil
	}
	if selection.Kind() != types.FieldVal {
		return d.inputFields(sel.X, at, depth)
	}
	if d.isInput(sel.X, at) {
		return inputField(selection)
	}
	if value, in, given := literalValue(sel.X, sel.Sel.Name, at); given {
		return d.inputFields(value, in, depth)
	}
	if inner, isChain := ast.Unparen(sel.X).(*ast.SelectorExpr); isChain {
		return d.selectorFields(inner, at, depth)
	}
	return nil
}

// callFields reads a call: what its arguments are built out of, and, for a
// method call, what its receiver is. A conversion is a call of one argument,
// and a call of a function nothing here reads (url.PathEscape,
// strconv.FormatInt, a lookup of ours that turns a path into an ID) is built
// out of what it is handed.
func (d *deriver) callFields(call *ast.CallExpr, at *frame, depth int) []string {
	var found []string
	if fun, isSelector := ast.Unparen(call.Fun).(*ast.SelectorExpr); isSelector {
		if selection, ok := at.pkg.TypesInfo.Selections[fun]; ok && selection.Kind() == types.MethodVal {
			found = append(found, d.inputFields(fun.X, at, depth)...)
		}
	}
	for _, arg := range call.Args {
		found = append(found, d.inputFields(arg, at, depth)...)
	}
	return sortedNames(found)
}

// isInput reports whether an expression is the input a handler is handed: a
// parameter of the function the walk entered that no call bound, read
// through the calls that hand it on, behind & or *. The walk enters a handler
// with nothing bound, so its parameters are what the route was called with.
func (d *deriver) isInput(expr ast.Expr, at *frame) bool {
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		// The operand of a field selection that is a name is a variable.
		variable, _ := at.pkg.TypesInfo.Uses[typed].(*types.Var)
		if bound, isBound := at.env[variable]; isBound {
			return d.isInput(bound.expr, bound.at)
		}
		return at.declares(variable)
	case *ast.UnaryExpr:
		return d.isInput(typed.X, at)
	case *ast.StarExpr:
		return d.isInput(typed.X, at)
	}
	return false
}

// declares reports whether a variable is one of the parameters the frame's
// function declares. A frame entered through a package-level variable's
// initializer that is no function literal declares none.
func (f *frame) declares(variable *types.Var) bool {
	if f.params == nil {
		return false
	}
	for _, field := range f.params.List {
		for _, name := range field.Names {
			if f.pkg.TypesInfo.Defs[name] == variable {
				return true
			}
		}
	}
	return false
}

// literalValue returns the value a composite literal gives a field, with the
// frame it is read in, when the operand of the selection is one: written in
// place, behind &, or handed down as an argument. A handler that calls
// another with an input it builds (`issues.Get(ctx, client, issues.GetInput{
// ProjectID: input.ProjectID})`) is read through to the field it copies.
func literalValue(expr ast.Expr, name string, at *frame) (ast.Expr, *frame, bool) {
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		variable, _ := at.pkg.TypesInfo.Uses[typed].(*types.Var)
		if bound, isBound := at.env[variable]; isBound {
			return literalValue(bound.expr, name, bound.at)
		}
	case *ast.UnaryExpr:
		return literalValue(typed.X, name, at)
	case *ast.CompositeLit:
		for _, element := range typed.Elts {
			pair, keyed := element.(*ast.KeyValueExpr)
			if !keyed {
				continue
			}
			// The keys of a struct literal are the names of its fields.
			if key, _ := pair.Key.(*ast.Ident); key.Name == name {
				return pair.Value, at, true
			}
		}
	}
	return nil, nil, false
}

// inputField names the field a selection selects the way the input's JSON
// spells it, through any embedded struct it is promoted from. A field of a
// struct declared outside this module (a client-go struct, the MCP request a
// handler is also handed) is none of the input's, and neither is one the
// JSON leaves out; a field with no tag is spelled as encoding/json spells
// it, by its Go name.
func inputField(selection *types.Selection) []string {
	typ := selection.Recv()
	var field *types.Var
	var tag string
	for _, index := range selection.Index() {
		holder := structOf(typ)
		field, tag = holder.Field(index), holder.Tag(index)
		typ = field.Type()
	}
	if !strings.HasPrefix(field.Pkg().Path(), goprogram.ModulePath+"/") {
		return nil
	}
	name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
	if name == "-" {
		return nil
	}
	if name == "" {
		name = field.Name()
	}
	return []string{name}
}

// structOf is the struct a field is selected through: the type underneath its
// name, or underneath the name of what a pointer points to.
func structOf(typ types.Type) *types.Struct {
	if pointer, isPointer := typ.Underlying().(*types.Pointer); isPointer {
		typ = pointer.Elem()
	}
	holder, _ := typ.Underlying().(*types.Struct)
	return holder
}

// valuesOf lists every value a frame's body assigns to a variable: through
// := and =, the right side of a compound assignment (+=), a var declaration
// with values, and the one call on the right of an assignment that takes
// several of its results.
func valuesOf(at *frame, variable *types.Var) []ast.Expr {
	var found []ast.Expr
	ast.Inspect(at.body, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range typed.Lhs {
				if ident, isIdent := lhs.(*ast.Ident); isIdent && at.pkg.TypesInfo.ObjectOf(ident) == variable {
					found = append(found, valueAt(typed.Rhs, i))
				}
			}
		case *ast.ValueSpec:
			for i, name := range typed.Names {
				if len(typed.Values) > 0 && at.pkg.TypesInfo.ObjectOf(name) == variable {
					found = append(found, valueAt(typed.Values, i))
				}
			}
		}
		return true
	})
	return found
}

// valueAt is what an assignment or a declaration puts in its i-th target:
// the value written in that position, or the one call that answers all of
// them.
func valueAt(values []ast.Expr, i int) ast.Expr {
	if i < len(values) {
		return values[i]
	}
	return values[0]
}

// sortedNames sorts names and drops repeats.
func sortedNames(names []string) []string {
	slices.Sort(names)
	return slices.Compact(names)
}
