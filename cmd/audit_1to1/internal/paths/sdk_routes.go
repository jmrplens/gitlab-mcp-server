package paths

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// sdkRoute is one endpoint a client-go service method reaches.
type sdkRoute struct {
	// Method is the HTTP verb, uppercase.
	Method string
	// Path is the endpoint with every identifier segment collapsed to the
	// placeholder, which is the spelling [pathShape] produces and the only one
	// in which our routes and GitLab's document meet.
	Path string
	// Many is true for a method answering with a slice of the type. It changes
	// no comparison, because GitLab's document describes a collection by the
	// element it returns rather than by the array around it, and it is
	// reported so a reader of a finding can see that the names searched are an
	// element's.
	Many bool
}

// operation spells the route the way a finding names it.
func (r sdkRoute) operation() string {
	if r.Many {
		return r.Method + " " + r.Path + " (collection)"
	}
	return r.Method + " " + r.Path
}

const (
	// routeFunc is the client-go helper every endpoint template is declared
	// through, so a package-level variable can register the route and still be
	// used as a format string.
	routeFunc = "route"
	// pathOption and methodOption are the two request options that name where
	// a method sends and how. A method that names no verb sends GET.
	pathOption   = "withPath"
	methodOption = "withMethod"
	// legacyRequest is the pre-option form a handful of methods still use.
	legacyRequest = "NewRequest"
	// httpPackage qualifies the net/http verb constants both forms name.
	httpPackage = "http"
	// verbConstantPrefix opens each of them, so http.MethodPost is POST.
	verbConstantPrefix = "Method"
	// defaultVerb is what a client-go method sends when it names none.
	defaultVerb = "GET"
	// responseTypeName is client-go's pagination wrapper, which every void
	// method answers with and no output type models.
	responseTypeName = "Response"
)

// routeVerb is the set of format verbs a route template stands an identifier
// in, spelled as client-go's own normalizer spells it.
var routeVerb = regexp.MustCompile(`%[sdv]`)

// readSDKRoutes parses the client-go source in dir and returns, for every
// struct a service method answers with, the endpoints those methods reach.
//
// It parses rather than type-checks because the question is textual: which
// route template does this method name, and which verb does it send. Loading
// client-go with types would cost seconds and answer the same thing. Every
// route template is a package-level variable declared through route(), which
// client-go's own test suite enforces, so the templates can be collected in one
// pass and resolved in the next without following any identifier out of the
// package.
//
// A directory that cannot be read, or a file that does not parse, contributes
// nothing rather than failing the scope: this reads a module cache it does not
// own the state of, and the join it feeds reports rather than gates.
func readSDKRoutes(dir string) map[string][]sdkRoute {
	files := parseSDKFiles(dir)
	if len(files) == 0 {
		return nil
	}

	templates := map[string]string{}
	for _, file := range files {
		collectRouteTemplates(file, templates)
	}

	found := map[string]map[sdkRoute]bool{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Recv == nil {
				continue
			}
			collectMethodRoutes(function, templates, found)
		}
	}
	return sortedRoutes(found)
}

// parseSDKFiles parses every non-test Go file directly in dir.
func parseSDKFiles(dir string) []*ast.File {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	fileSet := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fileSet, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			continue
		}
		files = append(files, file)
	}
	return files
}

// collectRouteTemplates records every `name = route("template")` declaration.
//
// The declaration's keyword is not consulted: only a var or a const holds a
// value at all, so asking whether a spec is one is the same question and one
// fewer place to keep in step with client-go.
func collectRouteTemplates(file *ast.File, into map[string]string) {
	for _, declaration := range file.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if template, ok := routeTemplate(value.Values[i]); ok {
					into[name.Name] = template
				}
			}
		}
	}
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

// collectMethodRoutes records the endpoints one service method reaches, under
// the name of the struct it answers with.
func collectMethodRoutes(function *ast.FuncDecl, templates map[string]string, into map[string]map[sdkRoute]bool) {
	if function.Type.Results == nil || len(function.Type.Results.List) == 0 {
		return
	}
	name, many := resultElement(function.Type.Results.List[0].Type)
	if name == "" || name == responseTypeName {
		return
	}

	verb, paths := requestShape(function.Body, templates)
	for _, path := range paths {
		routes := into[name]
		if routes == nil {
			routes = map[sdkRoute]bool{}
			into[name] = routes
		}
		routes[sdkRoute{Method: verb, Path: path, Many: many}] = true
	}
}

// requestShape reads the verb and the paths one method body names.
//
// Both are collected over the whole body rather than one call: a method that
// branches names its route twice, and a verb named anywhere in it is the verb
// the one request carries.
func requestShape(body *ast.BlockStmt, templates map[string]string) (verb string, paths []string) {
	verb = defaultVerb
	seen := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if found, ok := optionCall(call, templates); ok {
			if !seen[found] {
				seen[found] = true
				paths = append(paths, found)
			}
			return true
		}
		if named, ok := namedVerb(call); ok {
			verb = named
			return true
		}
		if legacyVerb, legacyPath, ok := legacyCall(call); ok {
			verb = legacyVerb
			if !seen[legacyPath] {
				seen[legacyPath] = true
				paths = append(paths, legacyPath)
			}
		}
		return true
	})
	sort.Strings(paths)
	return verb, paths
}

// optionCall returns the path a withPath option names.
func optionCall(call *ast.CallExpr, templates map[string]string) (string, bool) {
	name, isName := call.Fun.(*ast.Ident)
	if !isName || name.Name != pathOption || len(call.Args) == 0 {
		return "", false
	}
	route, isRoute := call.Args[0].(*ast.Ident)
	if !isRoute {
		return "", false
	}
	template, known := templates[route.Name]
	if !known {
		return "", false
	}
	return routeShape(template), true
}

// namedVerb returns the verb a withMethod option names.
func namedVerb(call *ast.CallExpr) (string, bool) {
	name, isName := call.Fun.(*ast.Ident)
	if !isName || name.Name != methodOption || len(call.Args) != 1 {
		return "", false
	}
	return httpVerb(call.Args[0])
}

// legacyCall returns the verb and path of a client.NewRequest call, the form
// the option helpers replaced and a handful of methods still use.
//
// Only a literal path is read. The rest build theirs by formatting into a local
// variable, and a path this cannot resolve leaves its type with one route fewer
// rather than with a wrong one.
func legacyCall(call *ast.CallExpr) (verb, path string, ok bool) {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != legacyRequest || len(call.Args) < 2 {
		return "", "", false
	}
	verb, ok = httpVerb(call.Args[0])
	if !ok {
		return "", "", false
	}
	literal, isLiteral := stringLiteral(call.Args[1])
	if !isLiteral || literal == "" {
		return "", "", false
	}
	return verb, routeShape(literal), true
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

// resultElement names the client-go struct a method answers with, and whether
// it answered with many of them. A result qualified by another package, such as
// bytes.Buffer, names no client-go struct and is left out.
func resultElement(expr ast.Expr) (name string, many bool) {
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ArrayType:
			many = true
			expr = typed.Elt
		case *ast.Ident:
			return typed.Name, many
		default:
			return "", many
		}
	}
}

// routeShape reduces a route template to the spelling [pathShape] produces.
//
// It is client-go's own normalizeTemplate: a segment that is nothing but a
// format verb stands for an identifier, and a verb embedded in a longer segment
// is dropped so the literal part still names the route. Reproducing that rule
// rather than inventing one is what keeps a template such as "archive%s"
// meeting the same path the SDK's own route registry meets.
func routeShape(template string) string {
	segments := strings.Split(strings.Trim(template, "/"), "/")
	for i, segment := range segments {
		switch {
		case segment == "":
		case routeVerb.FindString(segment) == segment:
			segments[i] = placeholder
		default:
			segments[i] = routeVerb.ReplaceAllString(segment, "")
		}
	}
	return "/" + strings.Join(segments, "/")
}

// stringLiteral unquotes an untyped string literal.
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

// sortedRoutes flattens the collected set into a stable slice per type.
func sortedRoutes(found map[string]map[sdkRoute]bool) map[string][]sdkRoute {
	out := make(map[string][]sdkRoute, len(found))
	for name, routes := range found {
		flat := make([]sdkRoute, 0, len(routes))
		for route := range routes {
			flat = append(flat, route)
		}
		sort.Slice(flat, func(i, j int) bool { return flat[i].operation() < flat[j].operation() })
		out[name] = flat
	}
	return out
}
