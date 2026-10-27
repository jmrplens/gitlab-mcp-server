// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/types"
	"regexp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// i18nPath is the import path of the message catalog, which the texts a
// person reads directly are to be rendered from.
//
// It is written out whole rather than joined onto modulePath, for the reason
// promptsPath is: the mutation tool cannot see a package-level declaration
// and reads the plus of a joined one as never covered.
// TestI18nPath_UnderTheModulePath_MovesWithIt holds it to modulePath.
const i18nPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"

// catalogKeyArgs names, per catalog function that renders a key, which of its
// arguments is the key: T takes the request's context before it, and New
// takes it first. Every argument after the key fills a placeholder.
var catalogKeyArgs = map[string]int{"T": 1, "New": 0}

// placeholderPattern is the grammar a catalog template names a value in,
// {name}, which is the only use a template makes of a brace.
var placeholderPattern = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

// englishTemplate reads the English template of a key, the text every
// language's rendering is held to and the one this audit judges.
//
// It is a variable so the package's tests hand the audit templates of their
// own: the fixture keys are in no catalog the server ships, and a test of how
// a render is read must not change when the catalog does.
var englishTemplate = catalogEnglishTemplate

// catalogEnglishTemplate asks the catalog compiled into this binary for the
// English template of a key read from source, which arrives as the string the
// type checker folded the key constant to.
func catalogEnglishTemplate(key string) (string, bool) {
	return i18n.EnglishTemplate(i18n.Key(key))
}

// The reasons a placeholder of a render is unresolved for, by what the call
// does not show.
const (
	// unboundByArgument is an argument that is not an inline S, N, M or L call
	// with a constant name, at the top of the render or inside a message
	// nested in it, which could fill any placeholder.
	unboundByArgument = "filled by an argument that is not an inline i18n.S, N, M or L call with a constant name, so which value lands here cannot be read from the call"
	// unboundByMessage is a message M or L nests that the call does not build
	// inline out of arguments it names: one held in a variable or returned by
	// a function, a list of them spread into the call, or one whose key and
	// arguments come from the results of a single call.
	unboundByMessage = "filled by a catalog message the call does not build inline, with i18n.New or i18n.Concat, out of arguments it names, so which values it carries cannot be read from the call"
)

// catalogFill is one value a placeholder of a render receives: the expression
// that carries it and, when the call does not show that value, why.
type catalogFill struct {
	expr    ast.Expr
	unbound string
}

// catalogSink splits a render of the message catalog into its holes.
//
// The template is the English one of the key, which must be a constant for
// the audit to know where a value lands; a key that is not one, including one
// that arrives in the results of a single call that fills the parameters
// before it too, is reported as it stands. Each {name} placeholder is a hole,
// filled by the argument the call names it by and judged in the construct the
// placeholder's line opens, exactly as a verb of an fmt template is: an S
// value is the expression judged, N renders an integer in decimal and is no
// hole, and M and L render messages whose own S values land in the
// placeholder whatever their templates say, so each of those is judged there
// in turn (see [sinkFile.messageFills]). A placeholder no argument names is
// filled by the first argument the call does not name at all, if there is
// one, which is unresolved rather than judged. The card rule reads the
// template's lines with each placeholder taken as the %s verb an fmt template
// would hold, so a row written through the catalog is read as one written
// with Sprintf, and a //gitlab:allow-card directive on the function holding
// the call answers it.
//
// The catalog package's own calls are not renders: T builds its message with
// New from the key it was handed, which is no constant, and reading that as a
// render would report the machinery every render goes through.
func (f sinkFile) catalogSink(call *ast.CallExpr, callee *types.Func) (sink, bool) {
	at, renders := catalogKeyArgs[callee.Name()]
	if !renders || f.pkg.PkgPath == i18nPath {
		return sink{}, false
	}
	s := sink{pkg: f.pkg, call: call, callee: callee.Name()}
	// A call whose one argument is another call with several results, which
	// Go accepts for a variadic parameter, has fewer arguments than the key's
	// index; that one argument then carries the key, and is no constant.
	key := call.Args[0]
	if at < len(call.Args) {
		key = call.Args[at]
	}
	tv := f.pkg.TypesInfo.Types[key]
	if tv.Value == nil {
		s.holes = []sinkHole{{expr: key, ctx: ctxCatalogKey, verb: "key"}}
		return s, true
	}
	template, known := englishTemplate(constantText(tv))
	if !known {
		return sink{}, false
	}
	fills, unbound := f.catalogFills(callee.Pkg(), call.Args[at+1:])
	places := placeholderPattern.FindAllStringSubmatchIndex(template, -1)
	holes := make([]hole, len(places))
	for i, place := range places {
		holes[i] = hole{offset: place[0]}
	}
	// A render writes to no destination, as Sprintf does not, so its
	// template is read from the start of an empty document.
	fenced := fenceHoles(template, newFenceCursor(), holes)
	for i, place := range places {
		name := template[place[2]:place[3]]
		ctx := contextAt(template, place[0])
		if fenced[i] {
			ctx = ctxFence
		}
		values, bound := fills[name]
		for _, value := range values {
			s.holes = append(s.holes, sinkHole{expr: value.expr, ctx: ctx, verb: "{" + name + "}", unbound: value.unbound})
		}
		if !bound && unbound != nil {
			s.holes = append(s.holes, sinkHole{expr: unbound, ctx: ctx, verb: "{" + name + "}", unbound: unboundByArgument})
		}
	}
	s.holes = append(s.holes, f.cardHoles(key, cardRowsRead(template, asFormat))...)
	return s, len(s.holes) > 0
}

// catalogFills binds the arguments of a render to the placeholders they name.
//
// A placeholder maps to the values the argument that names it fills it with,
// which is none for an N argument or for messages that carry no S value.
// When two arguments name one placeholder the first is kept, which is the one
// the render's replacer uses. An argument the call does not name a
// placeholder by, one built elsewhere and handed over or a list spread into
// the call, could fill any placeholder nothing else names, so the first such
// argument is returned beside the bindings.
func (f sinkFile) catalogFills(catalog *types.Package, args []ast.Expr) (fills map[string][]catalogFill, unbound ast.Expr) {
	fills = map[string][]catalogFill{}
	for _, arg := range args {
		name, values, ok := f.catalogArg(catalog, arg)
		if !ok {
			if unbound == nil {
				unbound = arg
			}
			continue
		}
		if _, seen := fills[name]; !seen {
			fills[name] = values
		}
	}
	return fills, unbound
}

// catalogArg reads one argument of a render: the placeholder it names and the
// values it fills that placeholder with. It answers false for an argument
// that is not an inline call of the catalog's own constructors with a
// constant name, which the call cannot be read to bind.
//
// Every function of the catalog that makes an argument is one of S, N, M and
// L, since nothing else returns one. S carries one text the caller produced,
// N an integer no construct can be changed by, and M and L messages, each of
// which brings the values of its own S arguments into the placeholder.
func (f sinkFile) catalogArg(catalog *types.Package, arg ast.Expr) (name string, values []catalogFill, ok bool) {
	call, isCall := ast.Unparen(arg).(*ast.CallExpr)
	if !isCall {
		return "", nil, false
	}
	callee := calleeOf(f.pkg, call)
	if callee == nil || callee.Pkg() != catalog {
		return "", nil, false
	}
	name = constantText(f.pkg.TypesInfo.Types[call.Args[0]])
	if name == "" {
		return "", nil, false
	}
	if callee.Name() == "N" {
		return name, nil, true
	}
	if callee.Name() == "S" {
		return name, []catalogFill{{expr: call.Args[1]}}, true
	}
	for _, message := range call.Args[1:] {
		values = append(values, f.messageFills(catalog, message)...)
	}
	return name, values, true
}

// messageFills reads the values one message nested in a render brings into
// the placeholder that holds it.
//
// A message renders its template with its own arguments in place, and lands
// whole where the outer placeholder sits, so every value an S of it carries
// lands there too, whatever construct the inner template wraps it in. That is
// the question asked of each, the way a value of a nested Sprintf is judged
// where the Sprintf lands; the inner render is also a sink of its own, read
// from its own template, so the value answers both. A message built inline
// with New brings the values of its arguments, through any depth of M and L,
// and one built inline with Concat those of its parts. Anything else, a
// message held in a variable or returned by a function, a list spread into
// the call, or a New whose key and arguments arrive in the results of a
// single call, is one value whose contents the call does not show, and an
// argument of a nested New the call cannot bind is one as well.
//
// The catalog makes a message with New or Concat alone, so a call of the
// catalog that is not Concat is New.
func (f sinkFile) messageFills(catalog *types.Package, message ast.Expr) []catalogFill {
	unreadable := []catalogFill{{expr: message, unbound: unboundByMessage}}
	call, isCall := ast.Unparen(message).(*ast.CallExpr)
	if !isCall {
		return unreadable
	}
	callee := calleeOf(f.pkg, call)
	if callee == nil || callee.Pkg() != catalog {
		return unreadable
	}
	var fills []catalogFill
	if callee.Name() == "Concat" {
		for _, part := range call.Args {
			fills = append(fills, f.messageFills(catalog, part)...)
		}
		return fills
	}
	if _, results := f.pkg.TypesInfo.Types[call.Args[0]].Type.(*types.Tuple); results {
		return unreadable
	}
	for _, arg := range call.Args[1:] {
		_, values, ok := f.catalogArg(catalog, arg)
		if !ok {
			values = []catalogFill{{expr: arg, unbound: unboundByArgument}}
		}
		fills = append(fills, values...)
	}
	return fills
}

// asFormat renders one line of a catalog template as an fmt template would
// write it, each placeholder as the %s verb that would stand in its place,
// which is the form the card rule's patterns read.
func asFormat(line string) string {
	return placeholderPattern.ReplaceAllLiteralString(line, "%s")
}
