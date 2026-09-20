package paths

import (
	"go/ast"
	"reflect"
	"slices"
	"sort"
	"strings"
)

const (
	// optionsSuffix ends the name of every client-go struct a service method
	// takes its parameters in. It is a naming convention rather than an
	// interface, so it is what identifies one.
	optionsSuffix = "Options"
	// requestOptionType is the variadic tail every service method carries. It
	// ends in Options too and is the transport's own, carrying nothing a
	// request body sends.
	requestOptionType = "RequestOptionFunc"
	// omitEmptyOption and omitZeroOption are the two json tag options that
	// decide whether a field is written when it holds nothing. Both are read,
	// because client-go uses both: DefaultBranchProtectionDefaultsOptions
	// spells its two slices omitzero, which encoding/json has honored since Go
	// 1.24, and reading only omitempty reported all four of them as values a
	// caller cannot decline to send.
	omitEmptyOption = "omitempty"
	omitZeroOption  = "omitzero"
	// jsonTagKey and hiddenName are encoding/json's own spellings.
	jsonTagKey = "json"
	hiddenName = "-"
	// optionDepth bounds the walk through nested option structs. Three levels
	// is the deepest client-go goes (a draft note's position carries a line
	// range carrying two ends) and the bound is what keeps a type that names
	// itself from walking forever.
	optionDepth = 6
)

// sdkOptionField is one field of a client-go option struct, as the request
// body will carry it.
type sdkOptionField struct {
	// GoName is the field's own name, so a finding reads the way the source
	// does.
	GoName string
	// Name is the json key, which is what GitLab's param is called.
	Name string
	// Always is true when the json tag carries neither omitempty nor omitzero,
	// so encoding/json writes the key whatever the field holds: null for a nil
	// pointer, the zero value for a scalar.
	Always bool
	// Nested names the option struct this field carries, empty for anything
	// else. A generic instantiation such as Nullable[T] names none, which is
	// right: it is a map and behaves like a scalar here.
	Nested string
	// Many is true when the field is a list of that struct, which Grape spells
	// with an empty subscript between the parent and the key.
	Many bool
}

// sdkOptionType is one client-go option struct: what it carries directly, and
// the option structs whose fields are promoted into it.
type sdkOptionType struct {
	Fields []sdkOptionField
	// Embedded names the option structs embedded without a json key, whose
	// fields encoding/json promotes into this one.
	Embedded []string
}

// sdkOptions is what one parse of client-go's source says about the structs its
// service methods take their parameters in.
type sdkOptions struct {
	// Types is every option struct the package declares, by name.
	Types map[string]sdkOptionType
	// Routes is, per option struct, the endpoints the methods taking it reach,
	// in the spelling [routeShape] produces.
	Routes map[string][]sdkRoute
}

// readSDKOptions parses the client-go source in dir and returns every option
// struct it declares together with the endpoints the methods taking one reach.
//
// It parses rather than type-checks for the reason [readSDKRoutes] does, and it
// parses the same files a second time rather than sharing that pass: the two
// answer different questions of the same source (which struct a method answers
// with, which struct a method is given) and keeping them apart is worth one
// walk of a directory that is already in the page cache.
//
// A directory that cannot be read, or a file that does not parse, contributes
// nothing rather than failing the scope.
func readSDKOptions(dir string) sdkOptions {
	files := parseSDKFiles(dir)
	if len(files) == 0 {
		return sdkOptions{}
	}

	templates := map[string]string{}
	found := sdkOptions{Types: map[string]sdkOptionType{}}
	for _, file := range files {
		collectRouteTemplates(file, templates)
		collectOptionTypes(file, found.Types)
	}

	routes := map[string]map[sdkRoute]bool{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Recv == nil {
				continue
			}
			collectOptionRoutes(function, templates, routes)
		}
	}
	found.Routes = sortedRoutes(routes)
	return found
}

// collectOptionTypes records every `type XxxOptions struct { … }` declaration.
func collectOptionTypes(file *ast.File, into map[string]sdkOptionType) {
	for _, declaration := range file.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral {
			continue
		}
		for _, spec := range general.Specs {
			typed, isType := spec.(*ast.TypeSpec)
			if !isType || !isOptionTypeName(typed.Name.Name) {
				continue
			}
			structType, isStruct := typed.Type.(*ast.StructType)
			if !isStruct {
				continue
			}
			into[typed.Name.Name] = optionFields(structType)
		}
	}
}

// isOptionTypeName reports whether a type name is one of the structs a service
// method takes its parameters in, which client-go names by convention.
func isOptionTypeName(name string) bool {
	return strings.HasSuffix(name, optionsSuffix) && name != requestOptionType
}

// optionFields reads one option struct's fields as encoding/json will write
// them.
func optionFields(structType *ast.StructType) sdkOptionType {
	var out sdkOptionType
	if structType.Fields == nil {
		return out
	}
	for _, field := range structType.Fields.List {
		nested, many := resultElement(field.Type)
		if len(field.Names) == 0 {
			// An embedded struct with no json key of its own: its fields are
			// promoted, so they are the enclosing type's own as far as the
			// body is concerned.
			if isOptionTypeName(nested) {
				out.Embedded = append(out.Embedded, nested)
			}
			continue
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			key, always, published := jsonField(field.Tag, name.Name)
			if !published {
				continue
			}
			out.Fields = append(out.Fields, sdkOptionField{
				GoName: name.Name,
				Name:   key,
				Always: always,
				Nested: optionTypeName(nested),
				Many:   many,
			})
		}
	}
	return out
}

// optionTypeName keeps a field's type only when it is another option struct.
func optionTypeName(name string) string {
	if isOptionTypeName(name) {
		return name
	}
	return ""
}

// jsonField reads the key one field is written under and whether it is written
// unconditionally, reporting false for a field the tag hides.
//
// A field with no json tag at all is written under its Go name and always: that
// is what encoding/json does, and naming it that way produces a param GitLab's
// record will not know, which costs a comparison that could not have been made
// rather than a finding that is wrong.
func jsonField(tag *ast.BasicLit, goName string) (key string, always, published bool) {
	if tag == nil {
		return goName, true, true
	}
	value, ok := stringLiteral(tag)
	if !ok {
		return goName, true, true
	}
	spec, tagged := reflect.StructTag(value).Lookup(jsonTagKey)
	if !tagged {
		return goName, true, true
	}
	name, rest, _ := strings.Cut(spec, ",")
	if name == hiddenName && rest == "" {
		return "", false, false
	}
	if name == "" {
		name = goName
	}
	options := strings.Split(rest, ",")
	omitted := slices.Contains(options, omitEmptyOption) || slices.Contains(options, omitZeroOption)
	return name, !omitted, true
}

// collectOptionRoutes records the endpoints one service method reaches, under
// the name of every option struct it is given.
func collectOptionRoutes(function *ast.FuncDecl, templates map[string]string, into map[string]map[sdkRoute]bool) {
	names := optionParameters(function)
	if len(names) == 0 {
		return
	}
	verb, paths := requestShape(function.Body, templates)
	for _, name := range names {
		for _, path := range paths {
			routes := into[name]
			if routes == nil {
				routes = map[sdkRoute]bool{}
				into[name] = routes
			}
			routes[sdkRoute{Method: verb, Path: path}] = true
		}
	}
}

// optionParameters names the option structs one method takes.
//
// The variadic transport tail is left out by name, since it ends in Options
// too and carries nothing a body sends; a variadic parameter is skipped
// structurally as well, which is what keeps that true if the tail is ever
// renamed.
func optionParameters(function *ast.FuncDecl) []string {
	if function.Type.Params == nil {
		return nil
	}
	var names []string
	for _, parameter := range function.Type.Params.List {
		if _, variadic := parameter.Type.(*ast.Ellipsis); variadic {
			continue
		}
		named, _ := resultElement(parameter.Type)
		if isOptionTypeName(named) && !slices.Contains(names, named) {
			names = append(names, named)
		}
	}
	return names
}

// optionParam is one field of an option struct, named the way GitLab's record
// declares the param it becomes.
type optionParam struct {
	// Owner is the option struct the field is declared on, which is not the
	// one the walk started at once a nested struct is reached.
	Owner string
	Field sdkOptionField
	// Param is the name the record uses: the json key at the top level, and
	// the enclosing keys in subscripts under it, as Grape declares a nested
	// param.
	Param string
}

// walkOptionParams calls visit for every field an option struct sends, its own
// and those of the option structs it nests, each named the way GitLab declares
// it.
//
// The walk carries the names already visited rather than a set of seen types,
// because one option struct can legitimately appear twice under different
// parents; what it must not do is descend into itself forever, which the depth
// bound and the ancestor check together prevent.
func walkOptionParams(types map[string]sdkOptionType, name, prefix string, ancestors []string, depth int, visit func(optionParam)) {
	if depth > optionDepth || slices.Contains(ancestors, name) {
		return
	}
	optionType, known := types[name]
	if !known {
		return
	}
	for _, embedded := range optionType.Embedded {
		walkOptionParams(types, embedded, prefix, append(ancestors, name), depth+1, visit)
	}
	for _, field := range optionType.Fields {
		param := field.Name
		if prefix != "" {
			param = prefix + "[" + field.Name + "]"
		}
		visit(optionParam{Owner: name, Field: field, Param: param})
		if field.Nested == "" {
			continue
		}
		child := param
		if field.Many {
			child += "[]"
		}
		walkOptionParams(types, field.Nested, child, append(ancestors, name), depth+1, visit)
	}
}

// optionTypesByRoute indexes the option structs whose methods reach each
// endpoint, keyed the way an inventory row's method and path key it.
func optionTypesByRoute(options sdkOptions) map[string][]string {
	byRoute := map[string]map[string]bool{}
	for name, routes := range options.Routes {
		for _, route := range routes {
			key := route.Method + " " + pathShape(route.Path)
			types := byRoute[key]
			if types == nil {
				types = map[string]bool{}
				byRoute[key] = types
			}
			types[name] = true
		}
	}
	out := make(map[string][]string, len(byRoute))
	for key, types := range byRoute {
		names := make([]string, 0, len(types))
		for name := range types {
			names = append(names, name)
		}
		sort.Strings(names)
		out[key] = names
	}
	return out
}
