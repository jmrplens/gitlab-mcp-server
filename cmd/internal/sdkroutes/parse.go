package sdkroutes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	// routeFunc is the client-go helper every endpoint template is declared
	// through, so a package-level variable registers the route and is still
	// usable as a format string.
	routeFunc = "route"
	// pathOption and methodOption are the request options that name where a
	// method sends and how. A method that names no verb sends GET.
	pathOption   = "withPath"
	methodOption = "withMethod"
	defaultVerb  = "GET"
	// requestMachinery is the package function every option-form method hands
	// its options to. It builds the request out of whatever withPath set, so it
	// is never followed as a delegation: its own request names a field rather
	// than a path, and the path is read where withPath names it.
	requestMachinery = "do"
	// httpPackage qualifies the net/http verb constants both request forms name,
	// and verbConstantPrefix opens each of them, so http.MethodPost is POST.
	httpPackage        = "http"
	verbConstantPrefix = "Method"
	// verbField is the request field a method reassigns when it builds a
	// request under one verb and sends it under another.
	verbField = "Method"
	// graphQLField and sendMethod spell the GraphQL transport: a body that
	// calls X.GraphQL.Do posts a document rather than a REST request.
	graphQLField = "GraphQL"
	sendMethod   = "Do"
	// serviceSuffix closes the name of every concrete client-go service.
	serviceSuffix = "Service"
	// optionsSuffix closes the name of every struct a service method takes its
	// parameters in, which is a naming convention rather than an interface.
	optionsSuffix = "Options"
	// responseType is client-go's pagination wrapper, which a method answering
	// with nothing else answers with.
	responseType = "Response"
	// unnamedParam stands for a parameter declared without a name, so the
	// positions of the named ones after it still line up with the arguments.
	unnamedParam = "_"
)

// legacyRequests are the client methods the pre-option form builds a request
// with: the verb is the first argument and the path the second.
var legacyRequests = map[string]bool{"NewRequest": true, "UploadRequest": true}

// reading is one parse of the package: the facts every function body is read
// against, and the functions themselves.
type reading struct {
	// templates maps a route variable to the template route() registered.
	templates map[string]string
	// consts maps a package-level string constant to its value.
	consts map[string]string
	// fields maps a struct type to its fields' type names, which is how a call
	// through a helper held in a field reaches the helper's method.
	fields map[string]map[string]string
	// vars maps a package-level variable to where its initializer is written
	// and what it names, which is how a GraphQL document reaches a body that
	// only names the template parsed out of it.
	vars map[string]variable
	// funcs maps a function's key to it.
	funcs map[string]*function
	// entries is every exported service method, in the order the files declare
	// them. Each is resolved on its own and [Read] sorts the keys, so the order
	// decides nothing.
	entries []*function
}

// function is one function or method body, with everything the evaluation
// reads from it.
type function struct {
	// key is "Type.name" for a method and the bare name for a package function.
	key      string
	recvType string
	recvVar  string
	name     string
	params   []string
	decl     *ast.FuncDecl
	// verb is the verb withMethod names, "" when none is named.
	verb string
	// override is the verb the body reassigns on a request it built, "" when
	// it reassigns none.
	override  string
	templates []templateUse
	legacy    []legacyUse
	calls     []delegation
	graphQL   bool
	idents    map[string]bool
	// file, start and end place the body, which is how an inline GraphQL
	// document is attributed to the function that writes it.
	file  string
	start int
	end   int
	// answers, many and options describe an entry's signature.
	answers string
	many    bool
	options []string
}

// variable is one package-level variable's initializer: where it is written,
// which places a document written inline inside it, and the identifiers it
// names, which places a named one and the variables it was built from.
type variable struct {
	file  string
	start int
	end   int
	refs  map[string]bool
}

// templateUse is one withPath call: the template and the arguments formatted
// into it.
type templateUse struct {
	template string
	args     []ast.Expr
}

// legacyUse is one request built with NewRequest or UploadRequest.
type legacyUse struct {
	verb ast.Expr
	path ast.Expr
}

// delegation is one call into another function of the package that may send
// a request: the callee's key and the arguments handed to it.
type delegation struct {
	callee string
	args   []ast.Expr
}

// parseDir parses every non-test Go file directly in dir.
func parseDir(dir string) *reading {
	parsed := &reading{
		templates: map[string]string{},
		consts:    map[string]string{},
		fields:    map[string]map[string]string{},
		vars:      map[string]variable{},
		funcs:     map[string]*function{},
	}
	files := parseFiles(dir)
	for _, file := range files {
		parsed.collectDeclarations(file)
	}
	for _, file := range files {
		for _, declaration := range file.syntax.Decls {
			if decl, isFunc := declaration.(*ast.FuncDecl); isFunc && decl.Body != nil {
				parsed.addFunction(file, decl)
			}
		}
	}
	return parsed
}

// parsedFile is one parsed source file with the file set that placed it.
type parsedFile struct {
	syntax  *ast.File
	fileSet *token.FileSet
	path    string
}

// parseFiles parses every non-test Go file directly in dir, in name order.
func parseFiles(dir string) []parsedFile {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	fileSet := token.NewFileSet()
	var files []parsedFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		syntax, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			continue
		}
		files = append(files, parsedFile{syntax: syntax, fileSet: fileSet, path: path})
	}
	return files
}

// collectDeclarations records one file's route templates, string constants,
// struct fields and variable initializers.
func (r *reading) collectDeclarations(file parsedFile) {
	for _, declaration := range file.syntax.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral {
			continue
		}
		for _, spec := range general.Specs {
			switch typed := spec.(type) {
			case *ast.ValueSpec:
				r.collectValues(file, general.Tok, typed)
			case *ast.TypeSpec:
				r.collectFields(typed)
			}
		}
	}
}

// collectValues records the templates, constants and variable initializers
// one value declaration carries.
//
// A constant's initializer is not recorded as a variable: a query constant
// assembled from a fragment constant already folds to the whole document under
// the query's own name, and following the fragment's name out of it would
// attribute the fragment a second time as a document of its own.
func (r *reading) collectValues(file parsedFile, keyword token.Token, spec *ast.ValueSpec) {
	for i, name := range spec.Names {
		if i >= len(spec.Values) {
			continue
		}
		value := spec.Values[i]
		if template, ok := routeTemplate(value); ok {
			r.templates[name.Name] = template
		}
		if literal, ok := stringLiteral(value); ok {
			r.consts[name.Name] = literal
		}
		if keyword != token.VAR {
			continue
		}
		refs := map[string]bool{}
		ast.Inspect(value, func(node ast.Node) bool {
			if ident, isIdent := node.(*ast.Ident); isIdent {
				refs[ident.Name] = true
			}
			return true
		})
		r.vars[name.Name] = variable{
			file:  file.path,
			start: file.fileSet.Position(value.Pos()).Line,
			end:   file.fileSet.Position(value.End()).Line,
			refs:  refs,
		}
	}
}

// collectFields records the type names of one struct's named fields.
func (r *reading) collectFields(spec *ast.TypeSpec) {
	structType, isStruct := spec.Type.(*ast.StructType)
	if !isStruct {
		return
	}
	fields := map[string]string{}
	for _, field := range structType.Fields.List {
		typeName, _ := elementName(field.Type)
		for _, name := range field.Names {
			fields[name.Name] = typeName
		}
	}
	r.fields[spec.Name.Name] = fields
}

// addFunction reads one function body.
func (r *reading) addFunction(file parsedFile, decl *ast.FuncDecl) {
	fn := &function{
		name:   decl.Name.Name,
		decl:   decl,
		idents: map[string]bool{},
		file:   file.path,
		start:  file.fileSet.Position(decl.Pos()).Line,
		end:    file.fileSet.Position(decl.End()).Line,
		params: paramNames(decl.Type.Params),
	}
	fn.key = fn.name
	if decl.Recv != nil {
		fn.recvType, fn.recvVar = receiver(decl.Recv)
		fn.key = fn.recvType + "." + fn.name
	}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		r.readNode(fn, node)
		return true
	})
	r.funcs[fn.key] = fn
	if isEntry(fn) {
		fn.answers, fn.many = answers(decl.Type.Results)
		fn.options = optionParameters(decl.Type.Params)
		r.entries = append(r.entries, fn)
	}
}

// readNode records what one node of a body contributes.
func (r *reading) readNode(fn *function, node ast.Node) {
	switch typed := node.(type) {
	case *ast.Ident:
		fn.idents[typed.Name] = true
	case *ast.SelectorExpr:
		if typed.Sel.Name == sendMethod && isSelectorNamed(typed.X, graphQLField) {
			fn.graphQL = true
		}
	case *ast.AssignStmt:
		if verb, ok := reassignedVerb(typed); ok {
			fn.override = verb
		}
	case *ast.CallExpr:
		r.readCall(fn, typed)
	}
}

// readCall records what one call contributes: a verb, a path, a legacy
// request, or a delegation.
func (r *reading) readCall(fn *function, call *ast.CallExpr) {
	switch callee := unindex(call.Fun).(type) {
	case *ast.Ident:
		r.readFunctionCall(fn, callee.Name, call)
	case *ast.SelectorExpr:
		r.readMethodCall(fn, callee, call)
	}
}

// readFunctionCall records a call of a package-level function.
func (r *reading) readFunctionCall(fn *function, name string, call *ast.CallExpr) {
	switch name {
	case methodOption:
		if len(call.Args) == 1 {
			if verb, ok := httpVerb(call.Args[0]); ok {
				fn.verb = verb
			}
		}
	case pathOption:
		if len(call.Args) == 0 {
			return
		}
		if template, ok := r.templateNamed(call.Args[0]); ok {
			fn.templates = append(fn.templates, templateUse{template: template, args: call.Args[1:]})
		}
	case requestMachinery:
	default:
		fn.calls = append(fn.calls, delegation{callee: name, args: call.Args})
	}
}

// readMethodCall records a call of a method: a legacy request, a call of the
// receiver's own method, or a call of a method of a helper the receiver holds
// in a field.
func (r *reading) readMethodCall(fn *function, selector *ast.SelectorExpr, call *ast.CallExpr) {
	if legacyRequests[selector.Sel.Name] && len(call.Args) >= 2 {
		fn.legacy = append(fn.legacy, legacyUse{verb: call.Args[0], path: call.Args[1]})
		return
	}
	if fn.recvVar == "" {
		return
	}
	switch target := selector.X.(type) {
	case *ast.Ident:
		if target.Name == fn.recvVar {
			fn.calls = append(fn.calls, delegation{callee: fn.recvType + "." + selector.Sel.Name, args: call.Args})
		}
	case *ast.SelectorExpr:
		holder, isIdent := target.X.(*ast.Ident)
		if !isIdent || holder.Name != fn.recvVar {
			return
		}
		if fieldType := r.fields[fn.recvType][target.Sel.Name]; strings.HasSuffix(fieldType, serviceSuffix) {
			fn.calls = append(fn.calls, delegation{callee: fieldType + "." + selector.Sel.Name, args: call.Args})
		}
	}
}

// templateNamed returns the template a withPath argument names: a route
// variable, or a literal written in place of one.
func (r *reading) templateNamed(expr ast.Expr) (string, bool) {
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		template, known := r.templates[ident.Name]
		return template, known
	}
	return stringLiteral(expr)
}

// isEntry reports whether a function is an exported method of an exported
// service, which is what a handler can call.
func isEntry(fn *function) bool {
	return fn.recvType != serviceSuffix &&
		strings.HasSuffix(fn.recvType, serviceSuffix) &&
		ast.IsExported(fn.recvType) &&
		ast.IsExported(fn.name)
}

// receiver names a method's receiver type and variable.
func receiver(fields *ast.FieldList) (typeName, varName string) {
	field := fields.List[0]
	if len(field.Names) > 0 {
		varName = field.Names[0].Name
	}
	typeName, _ = elementName(field.Type)
	return typeName, varName
}

// paramNames lists a signature's parameter names in order, an unnamed one
// spelled as [unnamedParam].
func paramNames(params *ast.FieldList) []string {
	var names []string
	for _, field := range params.List {
		if len(field.Names) == 0 {
			names = append(names, unnamedParam)
			continue
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

// answers names the struct a method's first result carries.
func answers(results *ast.FieldList) (name string, many bool) {
	if results == nil || len(results.List) == 0 {
		return "", false
	}
	name, many = elementName(results.List[0].Type)
	if name == responseType {
		return "", false
	}
	return name, many
}

// optionParameters names the option structs one method takes, each once,
// passing over the variadic transport tail.
func optionParameters(params *ast.FieldList) []string {
	var names []string
	for _, param := range params.List {
		if _, variadic := param.Type.(*ast.Ellipsis); variadic {
			continue
		}
		name, _ := elementName(param.Type)
		if strings.HasSuffix(name, optionsSuffix) && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// elementName names the type an expression spells, through pointers, slices
// and type arguments, and whether a slice was passed through. A type qualified
// by another package names nothing of this one.
func elementName(expr ast.Expr) (name string, many bool) {
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ArrayType:
			many = true
			expr = typed.Elt
		case *ast.IndexExpr:
			expr = typed.X
		case *ast.IndexListExpr:
			expr = typed.X
		case *ast.Ident:
			return typed.Name, many
		default:
			return "", many
		}
	}
}

// unindex strips the type arguments off a generic function's name.
func unindex(expr ast.Expr) ast.Expr {
	for {
		switch typed := expr.(type) {
		case *ast.IndexExpr:
			expr = typed.X
		case *ast.IndexListExpr:
			expr = typed.X
		default:
			return expr
		}
	}
}

// isSelectorNamed reports whether an expression selects a field by name.
func isSelectorNamed(expr ast.Expr, name string) bool {
	selector, isSelector := expr.(*ast.SelectorExpr)
	return isSelector && selector.Sel.Name == name
}

// reassignedVerb reads `req.Method = http.MethodX`.
func reassignedVerb(assign *ast.AssignStmt) (string, bool) {
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || !isSelectorNamed(assign.Lhs[0], verbField) {
		return "", false
	}
	return httpVerb(assign.Rhs[0])
}

// routeTemplate returns the template a route() call registers.
func routeTemplate(expr ast.Expr) (string, bool) {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall || len(call.Args) != 1 {
		return "", false
	}
	if name, isName := call.Fun.(*ast.Ident); !isName || name.Name != routeFunc {
		return "", false
	}
	return stringLiteral(call.Args[0])
}

// httpVerb reads a net/http method constant.
func httpVerb(expr ast.Expr) (string, bool) {
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	pkg, isPkg := selector.X.(*ast.Ident)
	if !isPkg || pkg.Name != httpPackage || !strings.HasPrefix(selector.Sel.Name, verbConstantPrefix) {
		return "", false
	}
	return strings.ToUpper(strings.TrimPrefix(selector.Sel.Name, verbConstantPrefix)), true
}

// stringLiteral unquotes a string literal.
func stringLiteral(expr ast.Expr) (string, bool) {
	literal, isLiteral := expr.(*ast.BasicLit)
	if !isLiteral || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
