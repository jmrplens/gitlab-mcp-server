// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// The five rules, by the names a finding gives them.
const (
	// ruleArgs is I1: a render passes exactly the placeholders of its English
	// template, each through an inline S, N, M or L call with a literal name.
	ruleArgs = "I1"
	// ruleKey is I2: a render names its key with a key constant of the
	// catalog, and nothing outside the catalog makes a key any other way.
	ruleKey = "I2"
	// ruleEntries is I3: every English entry is rendered from somewhere, and
	// every key constant has an English template.
	ruleEntries = "I3"
	// ruleVerbatim is I4: Verbatim is a test's alone.
	ruleVerbatim = "I4"
	// ruleLiteral is I5: no Text or Message is written as a literal outside
	// the catalog, and Concat is never called with nothing to join.
	ruleLiteral = "I5"
)

// keyArgs names, per catalog function that renders a key, which of its
// arguments is the key: T takes the request's context before it, and New
// takes it first. Every argument after the key fills a placeholder.
var keyArgs = map[string]int{"T": 1, "New": 0}

// placeholderPattern is the grammar a template names a value in, {name}.
var placeholderPattern = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

// scanner accumulates what one load of the tree says about the catalog.
type scanner struct {
	root string
	cat  *catalog
	// wholeTree is whether the run was over the default patterns, the only
	// one that may say a key nothing references is dead.
	wholeTree bool
	// findings are the findings no declaration can answer, and conversions the
	// conversions to the Key type, which the declaration table is held to.
	findings    []Finding
	conversions []Finding
	// unjudged are the files a load left out that import the catalog: a
	// build constraint this load did not satisfy hid every render in them.
	unjudged []string
	// packages are the packages this run looked at, named the way the
	// repository names them, which the declaration table is held against.
	packages map[string]struct{}
	// referenced holds every key a key constant was used for in a file that
	// is not one of the catalog's own key and catalog files.
	referenced map[string]bool
	// renders counts the calls of T and New judged.
	renders int
}

// newScanner returns a scanner rooted at an absolute repository path, judging
// against cat.
func newScanner(root string, cat *catalog, wholeTree bool) *scanner {
	return &scanner{
		root:       root,
		cat:        cat,
		wholeTree:  wholeTree,
		packages:   map[string]struct{}{},
		referenced: map[string]bool{},
	}
}

// observe judges every package of one load, then the catalog against what the
// packages referenced.
func (s *scanner) observe(loaded []*packages.Package) {
	for _, pkg := range loaded {
		s.packages[goprogram.TrimModulePath(pkg.PkgPath)] = struct{}{}
		s.noteUnjudged(pkg)
		for _, file := range pkg.Syntax {
			s.observeFile(pkg, file)
		}
	}
	s.judgeEntries()
}

// noteUnjudged records the files this load left out that import the catalog.
// A test file is passed over, because the gate reads no test file under any
// constraint.
func (s *scanner) noteUnjudged(pkg *packages.Package) {
	hidden := slices.DeleteFunc(goprogram.IgnoredGoFiles(pkg), isTestFile)
	for _, path := range goprogram.FilesImporting(hidden, s.cat.path, nil) {
		s.unjudged = append(s.unjudged, goprogram.RelativePath(path, s.root))
	}
}

// isTestFile reports whether a file is a test file by its name, which is how
// the go tool decides it.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

// site is where the scan is: the package and file it is reading, and the
// declaration a finding there sits in.
type site struct {
	pkg *packages.Package
	// catalogFile is whether the file is one of the catalog's key and
	// catalog files, whose uses of a key are not references to it.
	catalogFile bool
	// inCatalog is whether the package is the catalog itself, whose own
	// renders and texts are the machinery every other one goes through.
	inCatalog bool
	enclosing string
	// called holds the identifiers that name the function of a call, so a
	// catalog function named anywhere else is known to be used as a value.
	called map[*ast.Ident]bool
}

// observeFile judges every declaration of one file, each finding named after
// the declaration it sits in: a function as its file spells it, and a
// package-level initializer by the first name its spec declares.
func (s *scanner) observeFile(pkg *packages.Package, file *ast.File) {
	base := filepath.Base(pkg.Fset.Position(file.Pos()).Filename)
	at := site{pkg: pkg, inCatalog: pkg.PkgPath == s.cat.path}
	at.catalogFile = at.inCatalog && (base == "keys.go" || strings.HasPrefix(base, "catalog_"))
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}
		at.enclosing = sourcegate.FuncDeclName(fn)
		s.observeNode(at, fn)
	}
	for _, value := range valueSpecs(file) {
		at.enclosing = value.Names[0].Name
		s.observeNode(at, value)
	}
}

// observeNode judges everything below one declaration, closures included.
func (s *scanner) observeNode(at site, node ast.Node) {
	at.called = map[*ast.Ident]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.CallExpr:
			at.called[namedIdent(typed.Fun)] = true
			s.observeCall(at, typed)
		case *ast.CompositeLit:
			s.observeLiteral(at, typed)
		case *ast.Ident:
			s.observeIdent(at, typed)
		}
		return true
	})
}

// observeCall judges one call: a conversion to the Key type, a join of
// nothing, and a render.
//
// A render whose one argument is a call with several results, which Go
// accepts for a variadic parameter, takes its key from those results and has
// fewer arguments than the key's index; that is an I2 finding of its own,
// since the key cannot be read from the call, and its arguments are not
// judged, since none of them is written in it.
func (s *scanner) observeCall(at site, call *ast.CallExpr) {
	if at.inCatalog {
		return
	}
	if fun := at.pkg.TypesInfo.Types[call.Fun]; fun.IsType() && s.cat.isNamed(fun.Type, "Key") {
		s.conversions = append(s.conversions, s.finding(at, call, ruleKey,
			"converts "+types.ExprString(call.Args[0])+" to "+s.cat.name+".Key, which only the catalog package does"))
		return
	}
	callee, isFunc := at.pkg.TypesInfo.Uses[namedIdent(call.Fun)].(*types.Func)
	if !isFunc || !s.cat.owns(callee) {
		return
	}
	if callee.Name() == "Concat" && len(call.Args) == 0 {
		s.findings = append(s.findings, s.finding(at, call, ruleLiteral,
			"calls "+s.cat.name+".Concat with nothing to join, which renders the empty text in every language"))
		return
	}
	index, renders := keyArgs[callee.Name()]
	if !renders {
		return
	}
	s.renders++
	if index >= len(call.Args) {
		s.findings = append(s.findings, s.finding(at, call.Args[0], ruleKey,
			"takes its key from the results of "+types.ExprString(call.Args[0])+", so which key it renders cannot be read from the call"))
		return
	}
	key := call.Args[index]
	s.judgeKey(at, key)
	s.judgeArgs(at, call, key, call.Args[index+1:])
}

// judgeKey holds the key of a render to a key constant of the catalog. A
// constant of the Key type declared anywhere else and an untyped literal both
// compile, the second by conversion, and both put a key the catalog's own
// file does not name into the tree.
func (s *scanner) judgeKey(at site, key ast.Expr) {
	declared, isConst := at.pkg.TypesInfo.Uses[namedIdent(key)].(*types.Const)
	if isConst && s.cat.isKey(declared) {
		return
	}
	s.findings = append(s.findings, s.finding(at, key, ruleKey,
		"names its key with "+types.ExprString(key)+", which is not a key constant declared in "+goprogram.TrimModulePath(s.cat.path)))
}

// judgeArgs holds the arguments of a render whose key is a constant the
// English catalog has a template for to that template's placeholders: each
// argument an inline S, N, M or L call with a literal name, each placeholder
// filled once, and nothing filled the template does not carry.
//
// A render the call cannot be read to bind, an argument built elsewhere or a
// list spread into it, is a finding for that argument, and its placeholders
// are not then reported unfilled, since the argument may fill any of them.
func (s *scanner) judgeArgs(at site, call *ast.CallExpr, key ast.Expr, args []ast.Expr) {
	value := at.pkg.TypesInfo.Types[key].Value
	if value == nil {
		return
	}
	keyName := constant.StringVal(value)
	template, known := s.cat.templates[keyName]
	if !known {
		return
	}
	if call.Ellipsis.IsValid() {
		spread := args[len(args)-1]
		s.findings = append(s.findings, s.finding(at, spread, ruleArgs,
			"spreads "+types.ExprString(spread)+" into the call, so the placeholders its arguments fill cannot be read from it"))
		return
	}
	carried := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		carried[match[1]] = true
	}
	filled := map[string]bool{}
	bound := true
	// The checks are an if chain rather than a switch, because the mutation
	// tool reads the condition of a case as a line no test covers.
	for _, arg := range args {
		name, problem := s.argName(at, arg)
		if problem != "" {
			s.findings = append(s.findings, s.finding(at, arg, ruleArgs, problem))
			bound = false
			continue
		}
		if filled[name] {
			s.findings = append(s.findings, s.finding(at, arg, ruleArgs, "fills {"+name+"} twice"))
		} else if !carried[name] {
			s.findings = append(s.findings, s.finding(at, arg, ruleArgs,
				"fills {"+name+"}, which the English template of "+keyName+" does not carry"))
		}
		filled[name] = true
	}
	if !bound {
		return
	}
	for _, name := range slices.Sorted(maps.Keys(carried)) {
		if !filled[name] {
			s.findings = append(s.findings, s.finding(at, call, ruleArgs, "leaves {"+name+"} of "+keyName+" unfilled"))
		}
	}
}

// argName reads the placeholder one argument of a render names, or says why
// the call cannot be read to bind it.
func (s *scanner) argName(at site, arg ast.Expr) (name, problem string) {
	call, isCall := ast.Unparen(arg).(*ast.CallExpr)
	var callee *types.Func
	if isCall {
		callee, _ = at.pkg.TypesInfo.Uses[namedIdent(call.Fun)].(*types.Func)
	}
	if callee == nil || !s.cat.owns(callee) {
		return "", "passes " + types.ExprString(arg) + ", which is not an inline " + s.cat.name +
			".S, N, M or L call, so the placeholder it fills cannot be read from the call"
	}
	literal, isLiteral := ast.Unparen(call.Args[0]).(*ast.BasicLit)
	if !isLiteral {
		return "", "names a placeholder with " + types.ExprString(call.Args[0]) + ", which is not a string literal"
	}
	return constant.StringVal(at.pkg.TypesInfo.Types[literal].Value), ""
}

// literalTypes are the catalog types a literal outside the catalog makes
// without a key. Every field of both is unexported, so the one literal either
// can be written as there is the empty one: a Text a person is shown as no
// text at all, and a Message that renders as the empty text in every
// language.
var literalTypes = []string{"Text", "Message"}

// observeLiteral holds a Text or a Message written as a literal outside the
// catalog.
func (s *scanner) observeLiteral(at site, literal *ast.CompositeLit) {
	if at.inCatalog {
		return
	}
	for _, name := range literalTypes {
		if s.cat.isNamed(at.pkg.TypesInfo.Types[literal].Type, name) {
			s.findings = append(s.findings, s.finding(at, literal, ruleLiteral,
				"writes an "+s.cat.name+"."+name+" literal, which only the catalog package makes"))
		}
	}
}

// observeIdent judges one identifier: Verbatim used outside a test file, T or
// New named without being called, and a key constant referenced.
func (s *scanner) observeIdent(at site, ident *ast.Ident) {
	switch used := at.pkg.TypesInfo.Uses[ident].(type) {
	case *types.Func:
		s.observeFuncUse(at, ident, used)
	case *types.Const:
		if s.cat.isKey(used) && !at.catalogFile {
			s.referenced[constant.StringVal(used.Val())] = true
		}
	}
}

// observeFuncUse judges one use of a function: Verbatim anywhere, since only
// a test may make a text without a key and this load reads no test, and T or
// New named as a value outside the catalog, since a call made through the
// value names its key where this audit cannot read it.
func (s *scanner) observeFuncUse(at site, ident *ast.Ident, used *types.Func) {
	if !s.cat.owns(used) {
		return
	}
	if used.Name() == "Verbatim" {
		s.findings = append(s.findings, s.finding(at, ident, ruleVerbatim,
			"uses "+s.cat.name+".Verbatim outside a test file, which serves one text in every language"))
		return
	}
	if _, renders := keyArgs[used.Name()]; renders && !at.called[ident] && !at.inCatalog {
		s.findings = append(s.findings, s.finding(at, ident, ruleKey,
			"uses "+s.cat.name+"."+used.Name()+" as a value, so the key of a call made through it cannot be read"))
	}
}

// judgeEntries holds the English catalog to the references the scan saw: an
// entry no file outside the catalog's own references is dead, which only a
// run over the whole tree can say, and a key constant with no entry renders as
// its own key wherever it is used.
func (s *scanner) judgeEntries() {
	inCatalog := site{pkg: s.cat.pkg, enclosing: englishCatalogVar}
	if s.wholeTree {
		for _, key := range slices.Sorted(maps.Keys(s.cat.templates)) {
			if !s.referenced[key] {
				s.findings = append(s.findings, s.findingAt(inCatalog, s.cat.entries[key], ruleEntries,
					key+" has an English template and nothing outside the catalog files references its key"))
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(s.cat.constants)) {
		if _, has := s.cat.templates[key]; has {
			continue
		}
		declared := s.cat.constants[key]
		at := site{pkg: s.cat.pkg, enclosing: declared.Name()}
		s.findings = append(s.findings, s.findingAt(at, declared.Pos(), ruleEntries,
			"names "+key+", which the English catalog has no template for"))
	}
}

// finding names one finding at the start of a node.
func (s *scanner) finding(at site, node ast.Node, rule, reason string) Finding {
	return s.findingAt(at, node.Pos(), rule, reason)
}

// findingAt names one finding at a position, in the package and declaration
// the scan is reading, with the file named below the root.
func (s *scanner) findingAt(at site, pos token.Pos, rule, reason string) Finding {
	where := at.pkg.Fset.Position(pos)
	return Finding{
		Rule:    rule,
		Package: goprogram.TrimModulePath(at.pkg.PkgPath),
		File:    goprogram.RelativePath(where.Filename, s.root),
		Line:    where.Line,
		Func:    at.enclosing,
		Reason:  reason,
	}
}

// namedIdent returns the identifier an expression names, by itself or
// through a package or receiver selector and any parentheses, which is how a
// call names its function and a render names its key. It is nil for anything
// else: a function literal or a call's result called, a key written as a
// literal or built by a conversion.
func namedIdent(expr ast.Expr) *ast.Ident {
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return typed
	case *ast.SelectorExpr:
		return typed.Sel
	default:
		return nil
	}
}
