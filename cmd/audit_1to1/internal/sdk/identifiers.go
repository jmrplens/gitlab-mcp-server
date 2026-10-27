// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"go/ast"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// What an integer a handler hands a client-go parameter of type any came
// to. Four are findings: narrowed, undescribed, unplaced and untraced.
const (
	// identifierIDOnly: every placeholder the parameter fills takes an ID
	// alone, as the record describes it.
	identifierIDOnly = "id_only"
	// identifierNarrowed: a placeholder it fills takes a name or a path,
	// and the input field the integer is read from offers neither.
	identifierNarrowed = "narrowed"
	// identifierUndescribed: the record says nothing of a placeholder it
	// fills, so nothing tells an ID from a name there.
	identifierUndescribed = "undescribed"
	// identifierUnplaced: the reading of client-go places the parameter in
	// no route the record holds, so nothing can be said of it.
	identifierUnplaced = "unplaced"
	// identifierUntraced: it would be narrowed or undescribed, and the
	// trace stopped at an expression it does not follow, so what the
	// integer is read from is not known.
	identifierUntraced = "untraced"
	// identifierNotFromInput: it would be narrowed or undescribed, and the
	// trace followed the integer to nothing but a client-go struct, a
	// constant, a literal, a count or a position, so no input narrows
	// what a model sends there.
	identifierNotFromInput = "not_from_input"
	// identifierDeclared: a declaration answers the input field.
	identifierDeclared = "declared"
)

// What the record says a placeholder takes.
const (
	takesName        = "name-or-path"
	takesID          = "id"
	takesUndescribed = "undescribed"
)

// sdkIntegerTypes are the parameter types, as a client-go signature spells
// them, that carry an ID and nothing else.
var sdkIntegerTypes = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
}

// identifiersReport is the identifiers section of the report.
type identifiersReport struct {
	// Arguments is every integer a handler hands a client-go parameter of
	// type any, once per input field it is read from.
	Arguments []identifierArgument `json:"arguments"`
	// Unfollowed is every client-go method with a parameter of type any
	// taken as a value where the walk does not follow it to the calls made
	// through it, so what they hand it is not known. Each fails the gate.
	Unfollowed []unfollowedValue `json:"unfollowed"`
	// SDKNarrowed is every integer parameter of a client-go method that
	// fills a placeholder the record says takes a name or a path: client-go
	// narrows it, so the fix is upstream and nothing here gates on it.
	SDKNarrowed []sdkNarrowing `json:"sdk_narrowed"`
}

// identifierArgument is one integer a handler hands a parameter of type any.
type identifierArgument struct {
	// Site is the function the call is in, Method the client-go method it
	// calls ("Users.GetUserStatus"), and Param the parameter it fills.
	Site   string `json:"site"`
	Method string `json:"method"`
	Param  string `json:"param"`
	// Argument and Type are the expression handed over and its type.
	Argument string `json:"argument"`
	Type     string `json:"type"`
	// Input is the input field the integer is read from
	// ("users.GetStatusInput.UserID"), empty when it is read from none.
	Input string `json:"input,omitempty"`
	// StoppedAt, beside no input, names the expressions the trace stopped
	// at, each with the function it is in.
	StoppedAt string `json:"stopped_at,omitempty"`
	// Placeholders are the placeholders the parameter fills in the routes
	// the method sends, as the record describes them.
	Placeholders []placeholder `json:"placeholders,omitempty"`
	Status       string        `json:"status"`
	// Category, Issue and Reason are the declaration's, when one answers
	// the input field.
	Category string `json:"category,omitempty"`
	Issue    int    `json:"issue,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// placeholder is one identifier segment of a route the record holds.
type placeholder struct {
	// Route is the record's route ("GET /users/:id/events") and Name the
	// placeholder's ("id").
	Route string `json:"route"`
	Name  string `json:"name"`
	// Type and Desc are what the record declares of the parameter, and
	// Takes what they say it takes. DescribedBy names the route they are
	// read from when the route itself declares nothing of it.
	Type        string `json:"type,omitempty"`
	Desc        string `json:"desc,omitempty"`
	DescribedBy string `json:"described_by,omitempty"`
	Takes       string `json:"takes"`
}

// sdkNarrowing is one integer parameter of a client-go method that fills a
// placeholder the record says takes a name or a path.
type sdkNarrowing struct {
	Method       string        `json:"method"`
	Param        string        `json:"param"`
	Type         string        `json:"type"`
	Placeholders []placeholder `json:"placeholders"`
	// Called is set when a handler references the method.
	Called bool `json:"called,omitempty"`
}

// identifierCall is one integer a handler hands a client-go parameter of
// type any, with the input field it is read from, or, beside none, where
// the trace stopped.
type identifierCall struct {
	site, method, param, argument, argType, input, stopped string
	index                                                  int
}

// analyzeIdentifiers walks root's tool packages for every integer handed to
// a client-go parameter of type any, called by name or through a method
// value, and holds each to what the live API record says the placeholder it
// fills takes, and to declared; it lists every method value the walk does
// not follow, and beside them every integer parameter of client-go that
// narrows a placeholder the record says takes a name or a path. A record
// that cannot be read leaves every argument unplaced and is itself stale.
func analyzeIdentifiers(root string, pkgs []*packages.Package, usage map[string]*shared.ServiceUsage, declared []identifierDeclaration) (report identifiersReport, stale []string) {
	record, err := apilive.Read(filepath.Join(root, filepath.FromSlash(apilive.DefaultDir)))
	if err != nil {
		stale = append(stale, "identifiers: "+err.Error())
	}
	reading := sdkroutes.Read(structs.ClientGoDir(pkgs))
	index := newPlaceholderIndex(record)
	funcs := tracedFuncs(pkgs)
	trace := newInputTrace(pkgs, funcs)
	values := followMethodValues(funcs, trace)
	arguments := make([]identifierArgument, 0)
	for _, call := range collectIdentifierCalls(funcs, trace, values) {
		held := index.placeholders(reading.Placements(call.method), call.index)
		arguments = append(arguments, identifierArgument{
			Site: call.site, Method: call.method, Param: call.param, Argument: call.argument, Type: call.argType,
			Input: call.input, StoppedAt: call.stopped, Placeholders: held, Status: identifierStatus(call.input, call.stopped, held),
		})
	}
	stale = append(stale, applyIdentifierDeclarations(arguments, declared)...)
	return identifiersReport{Arguments: arguments, Unfollowed: values.unfollowed, SDKNarrowed: sdkNarrowings(reading, index, calledMethods(usage))}, stale
}

// identifierStatus is what an argument comes to with the input field it is
// read from, where its trace stopped beside none, and the placeholders it
// fills, before any declaration: the widest thing a placeholder takes
// decides, a name or a path before a placeholder the record does not
// describe. A parameter placed in no route is unplaced whatever it is read
// from, since that is a gap of the reading and not of the input; one read
// from no input field is untraced when the trace stopped and not from input
// when it did not.
func identifierStatus(input, stopped string, held []placeholder) string {
	if len(held) == 0 {
		return identifierUnplaced
	}
	status := identifierIDOnly
	for _, found := range held {
		if found.Takes == takesName {
			status = identifierNarrowed
			break
		}
		if found.Takes == takesUndescribed {
			status = identifierUndescribed
		}
	}
	if status == identifierIDOnly || input != "" {
		return status
	}
	if stopped != "" {
		return identifierUntraced
	}
	return identifierNotFromInput
}

// isIdentifierFinding reports whether an argument's status fails the gate.
func isIdentifierFinding(status string) bool {
	return status == identifierNarrowed || status == identifierUndescribed || status == identifierUnplaced || status == identifierUntraced
}

// collectIdentifierCalls finds, in every walked function, each integer a
// call hands a parameter of type any of a client-go service method it
// calls by name or through a variable holding it, once per input field it
// is read from, and once more where the trace stopped, sorted.
func collectIdentifierCalls(funcs []tracedFunc, trace *inputTrace, values methodValues) []identifierCall {
	found := map[identifierCall]bool{}
	for _, fn := range funcs {
		ast.Inspect(fn.body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			for _, held := range values.heldBy(fn.info, call) {
				for _, arg := range identifierArgs(fn.info, call, held.method) {
					arg.site = fn.site
					traced := trace.origins(fn, arg.expr, held.via)
					for _, input := range traced.inputs {
						arg.input = input
						found[arg.identifierCall] = true
					}
					arg.input = ""
					if len(traced.stopped) > 0 {
						arg.stopped = strings.Join(slices.Compact(slices.Sorted(slices.Values(traced.stopped))), "; ")
						found[arg.identifierCall] = true
					} else if len(traced.inputs) == 0 {
						found[arg.identifierCall] = true
					}
				}
			}
			return true
		})
	}
	return slices.SortedFunc(maps.Keys(found), func(a, b identifierCall) int {
		return cmp.Or(strings.Compare(a.site, b.site), strings.Compare(a.method, b.method), cmp.Compare(a.index, b.index),
			strings.Compare(a.input, b.input), strings.Compare(a.argument, b.argument), strings.Compare(a.stopped, b.stopped))
	})
}

// identifierArg is one integer a call hands a parameter of type any, with
// the expression it is.
type identifierArg struct {
	identifierCall
	expr ast.Expr
}

// identifierArgs lists the integers one call hands a parameter of type any
// of method, the client-go service method it reaches. The variadic tail is
// a slice, so it is never one.
func identifierArgs(info *types.Info, call *ast.CallExpr, method boundMethod) []identifierArg {
	params := method.signature.Params()
	var out []identifierArg
	for i := range min(len(call.Args), params.Len()) {
		param := params.At(i)
		argType := info.TypeOf(call.Args[i])
		if !isEmptyInterface(param.Type()) || !isInteger(argType) {
			continue
		}
		out = append(out, identifierArg{
			method: method.key, param: param.Name(), index: i,
			argument: types.ExprString(call.Args[i]), argType: types.TypeString(argType, (*types.Package).Name),
			expr: call.Args[i],
		})
	}
	return out
}

// isEmptyInterface reports whether t is any.
func isEmptyInterface(t types.Type) bool {
	iface, isInterface := t.Underlying().(*types.Interface)
	return isInterface && iface.Empty()
}

// isInteger reports whether t is an integer type, a named one included.
func isInteger(t types.Type) bool {
	basic, isBasic := t.Underlying().(*types.Basic)
	return isBasic && basic.Info()&types.IsInteger != 0
}

// placeholderIndex finds the record's description of the placeholder a
// placement fills.
type placeholderIndex struct {
	routes *apilive.RouteIndex
	// all is every route of the record, which a placeholder its own route
	// does not describe is looked for in.
	all []apilive.Route
}

// newPlaceholderIndex indexes the record's routes.
func newPlaceholderIndex(record apilive.Document) placeholderIndex {
	return placeholderIndex{routes: apilive.NewRouteIndex(record.Routes), all: record.Routes}
}

// placeholders describes the placeholders the parameter at index fills
// among a method's placements, each where the record holds its route.
func (p placeholderIndex) placeholders(placements []sdkroutes.Placement, index int) []placeholder {
	var out []placeholder
	for _, placed := range placements {
		if placed.Index != index {
			continue
		}
		if found, ok := p.lookup(placed); ok {
			out = append(out, found)
		}
	}
	return out
}

// lookup describes the placeholder one placement fills, and reports false
// for a route the record does not hold.
//
// A route can declare nothing of a placeholder its namespace declares on
// the others: the record holds GET /groups/:id/custom_attributes with the
// group's ID or path and GET /groups/:id/custom_attributes/:key with an id
// it says nothing of. Such a placeholder is described by the route that
// declares it and shares the most leading segments with this one, the
// placeholder's included, and among as many by the one that takes the most.
func (p placeholderIndex) lookup(placed sdkroutes.Placement) (placeholder, bool) {
	route, ok := p.routes.Match(placed.Route.Method, placed.Route.Path)
	if !ok {
		return placeholder{}, false
	}
	segments := spelledSegments(route.Path, placed.Route.Path)
	held := placeholder{Route: apilive.RouteName(route), Name: segments[placed.Segment][1:]}
	declared := route.Params[held.Name]
	if declared.Desc == "" && declared.Type == "" {
		declared, held.DescribedBy = p.sibling(segments, placed.Segment)
	}
	held.Type, held.Desc, held.Takes = declared.Type, declared.Desc, takes(declared)
	return held, true
}

// spelledSegments is the segments of the spelling of a record's path, its
// optional groups taken or left out, that a requested path meets, as the
// record writes them. The request holds a placeholder at each segment a
// parameter fills, so the record's segment there is one too.
func spelledSegments(recordPath, requested string) []string {
	want := apilive.PathSegments(requested)
	var segments []string
	for _, spelling := range apilive.ExpandOptional(strings.TrimPrefix(recordPath, apilive.EndpointPrefix)) {
		if segmentsMeet(apilive.PathSegments(spelling), want) {
			segments = strings.Split(strings.Trim(spelling, "/"), "/")
		}
	}
	return segments
}

// takesRank orders what a placeholder takes from the least to the most.
var takesRank = map[string]int{takesUndescribed: 0, takesID: 1, takesName: 2}

// sibling finds the declaration of the placeholder at a segment among the
// routes that declare it and share the segments up to it, the one sharing
// the most and among as many the one that takes the most, with the route
// it is read from; nothing when no route does.
func (p placeholderIndex) sibling(segments []string, segment int) (best apilive.Param, by string) {
	name := segments[segment][1:]
	deepest := 0
	for i := range p.all {
		route := &p.all[i]
		declared := route.Params[name]
		if declared.Desc == "" && declared.Type == "" {
			continue
		}
		for _, spelling := range apilive.ExpandOptional(strings.TrimPrefix(route.Path, apilive.EndpointPrefix)) {
			common := commonPrefix(segments, strings.Split(strings.Trim(spelling, "/"), "/"))
			if common <= segment {
				continue
			}
			if common > deepest || (common == deepest && takesRank[takes(declared)] > takesRank[takes(best)]) {
				best, by, deepest = declared, apilive.RouteName(route), common
			}
		}
	}
	return best, by
}

// commonPrefix counts the leading segments two paths share.
func commonPrefix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// segmentsMeet reports whether a record spelling's segments meet a
// request's: the same number, each the same or a record placeholder.
func segmentsMeet(record, requested []string) bool {
	if len(record) != len(requested) {
		return false
	}
	for i := range record {
		if record[i] != requested[i] && record[i] != ":" {
			return false
		}
	}
	return true
}

// takes reads what a placeholder takes off what the record declares of it.
// The description decides: a word ending in name or path, or slug, says a
// name or a path ("ID or username of the user", "ID or URL-encoded path of
// the project", "Group's ID or slug"), and any other description an ID
// alone, whatever the type ("ID of the repository." is [String, Integer]).
// Only with no description does the type say: one that admits a String
// takes a name, an Integer an ID, and anything else nothing the record
// tells.
func takes(param apilive.Param) string {
	if desc := strings.TrimSpace(param.Desc); desc != "" {
		if describesAName(desc) {
			return takesName
		}
		return takesID
	}
	if strings.Contains(param.Type, "String") {
		return takesName
	}
	if strings.Contains(param.Type, "Integer") {
		return takesID
	}
	return takesUndescribed
}

// describesAName reports whether a description names a name or a path.
func describesAName(desc string) bool {
	words := strings.FieldsFunc(strings.ToLower(desc), func(r rune) bool { return !unicode.IsLetter(r) })
	return slices.ContainsFunc(words, func(word string) bool {
		return strings.HasSuffix(word, "name") || strings.HasSuffix(word, "path") || word == "slug"
	})
}

// sdkNarrowings lists every integer parameter of a client-go method that
// fills a placeholder the record says takes a name or a path, the
// placeholders of each parameter together, and whether a handler calls it.
func sdkNarrowings(reading *sdkroutes.SDK, index placeholderIndex, called map[string]bool) []sdkNarrowing {
	out := make([]sdkNarrowing, 0)
	for _, method := range reading.Methods() {
		key := method.Key()
		for _, placed := range reading.Placements(key) {
			if !sdkIntegerTypes[placed.Type] {
				continue
			}
			found, ok := index.lookup(placed)
			if !ok || found.Takes != takesName {
				continue
			}
			// Placements come sorted by parameter, so a parameter's
			// placeholders are consecutive.
			if last := len(out) - 1; last >= 0 && out[last].Method == key && out[last].Param == placed.Param {
				out[last].Placeholders = append(out[last].Placeholders, found)
				continue
			}
			out = append(out, sdkNarrowing{Method: key, Param: placed.Param, Type: placed.Type, Placeholders: []placeholder{found}, Called: called[key]})
		}
	}
	return out
}

// summarizeIdentifiers counts the section's arguments by status, the method
// values it does not follow, and the client-go parameters it lists as
// narrowing.
func summarizeIdentifiers(summary *reportSummary, report identifiersReport) {
	summary.IdentifierArguments = len(report.Arguments)
	summary.IdentifiersUnfollowed = len(report.Unfollowed)
	summary.SDKNarrowed = len(report.SDKNarrowed)
	counters := map[string]*int{
		identifierIDOnly: &summary.IdentifiersIDOnly, identifierNarrowed: &summary.IdentifiersNarrowed,
		identifierUndescribed: &summary.IdentifiersUndescribed, identifierUnplaced: &summary.IdentifiersUnplaced,
		identifierUntraced: &summary.IdentifiersUntraced, identifierNotFromInput: &summary.IdentifiersNotFromInput,
		identifierDeclared: &summary.IdentifiersDeclared,
	}
	for _, argument := range report.Arguments {
		*counters[argument.Status]++
	}
}

// keepIdentifierFindings keeps only the arguments that are a finding and the
// method values the walk does not follow.
func keepIdentifierFindings(report identifiersReport) identifiersReport {
	arguments := make([]identifierArgument, 0)
	for _, argument := range report.Arguments {
		if isIdentifierFinding(argument.Status) {
			arguments = append(arguments, argument)
		}
	}
	return identifiersReport{Arguments: arguments, Unfollowed: report.Unfollowed, SDKNarrowed: make([]sdkNarrowing, 0)}
}
