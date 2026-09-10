package paths

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// publishedType is one output type and the field names it puts in front of a
// model.
type publishedType struct {
	// Package is repository relative, the same spelling the request inventory
	// records, so the two join without translation.
	Package string
	// Name is the Go type name.
	Name string
	// Fields are its json tags, sorted, with embedded types flattened.
	Fields []string
	// Nested holds, per json tag whose Go type is another output type of the
	// same package, that type's own name and fields. It is one level deep,
	// which is as far as the comparison goes (see [operation.Nested]), and it
	// is empty on an inner type: a type reached through a field of a field is
	// compared against nothing, so collecting it would only grow the walk.
	Nested map[string]nestedType
	// Inner is true for a type some struct of the package names as a field
	// type, or one not named as an output type at all.
	//
	// It says where the type appears in this repository's own shapes and
	// nothing about GitLab. It used to be read as "nobody's response", and
	// the type grain skipped every one of them on that reading, which was
	// backwards: the convention here wraps a response in a one-key envelope,
	// so the type that models what GitLab sends is named as the envelope's
	// field and is precisely what this marks. What decides whether GitLab
	// answers with the object is the converter pairing, so the type grain
	// judges an inner type that has one and passes over an inner type that
	// does not — see [TypedShapeCheck.ComparedInner].
	//
	// In the sent direction at package grain its fields count as the
	// package's either way, since the row of a list is inner and is what the
	// list endpoint sends.
	Inner bool
	// Payload is true for a type some struct of the package wraps and carries
	// nothing else beside: `{badge: BadgeItem}`, or a list plus its
	// pagination. That struct is this server's packaging and this type is
	// what GitLab answered with, so the type grain judges it against the
	// endpoint even though it is Inner. See [envelopePayloads].
	Payload bool
}

// nestedType is one output type reached through a field of another.
type nestedType struct {
	Name   string
	Fields []string
}

// toolsDir is where the domain packages live.
const toolsDir = "internal/tools"

// sharedDir is the package the domain packages share response shapes
// through: a user, a milestone, a pipeline, spelled once and named from the
// domain packages as toolutil.X or aliased into them.
const sharedDir = "internal/toolutil"

// sharedPrefix is how a shared shape is named in a domain package's source,
// and the key it is resolved under here.
const sharedPrefix = "toolutil."

// hintsType is the one shared shape every output embeds whose fields are not
// GitLab's data: the next-step hints the server adds to a result. It is left
// out of the walk on purpose, since counting its fields as published would
// report them as fields GitLab does not send, which is exactly what they are.
const hintsType = sharedPrefix + "HintableOutput"

// outputSuffix is how this repository names a type it returns to a model. The
// convention is enforced nowhere, so a type that does not follow it is simply
// not compared, which can only lose a finding.
const outputSuffix = "Output"

// inputSuffix is how this repository names a type it takes from a model. Its
// json names are what a caller sends, so they are no evidence that the package
// publishes anything, and a struct so named is left out of the walk entirely.
const inputSuffix = "Input"

// recordDir is where the GitLab API record lives for a given repository root.
func recordDir(root string) string {
	return filepath.Join(root, apilive.DefaultDir)
}

// publishedTypes reads every exported struct under internal/tools that is not
// an input and returns the json tags each one publishes, the `*Output` types
// nothing names as a field type first among them (see [publishedType.Inner]).
//
// It parses rather than type-checks on purpose. What the comparison needs is
// the set of names a client sees, which is exactly the set of json tags in the
// source; loading the whole program with types would cost twenty seconds and
// answer the same question. The one package a domain package takes a shape
// from is internal/toolutil, so its structs are read once and resolved
// wherever a domain package names one, embeds one or aliases one, the hints
// type aside (see [hintsType]); a type from any other package resolves to
// nothing, and a field of that type publishes its own name and nothing under
// it.
//
// A package that does not parse contributes nothing rather than failing the
// scope, for the reason [publishedTypesIn] records.
func publishedTypes(root string) []publishedType {
	base := filepath.Join(root, toolsDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	shared, sharedNested := sharedShapes(filepath.Join(root, sharedDir))

	var out []publishedType
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		out = append(out, publishedTypesIn(filepath.Join(base, entry.Name()), toolsDir+"/"+entry.Name(), shared, sharedNested)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// publishedTypesIn reads one package directory.
//
// It parses file by file rather than through go/parser's ParseDir, which is
// deprecated for associating files with packages without reading build tags.
// Nothing here needs that association: every file in one of these directories
// declares types for the same domain, and a file the parser refuses is simply
// passed over, since an audit that reads a tree it does not own the state of
// must not fail over somebody's half-written edit.
func publishedTypesIn(dir, pkg string, shared map[string]declaredStruct, sharedNested map[string]bool) []publishedType {
	parsed := parsePackage(dir)
	found, nested, scalars, aliases := parsed.structs, parsed.nested, parsed.scalars, parsed.aliases

	// Only a top-level output type can be compared with an operation's
	// response. GitLab's document lists the properties of the object an
	// endpoint returns, and, since schema version 2, the properties of the
	// objects one level inside it; comparing a nested type against the
	// top-level list reports every one of its fields as unpublished, which is
	// what the first run of this check did: 1418 findings, almost all of them
	// the fields of a user, a group or a rule sitting inside a response that
	// does carry them. Those types are now compared under the property they sit
	// under instead, by [typedShapeCheck], which is why they are kept here
	// rather than only counted.
	//
	// A type any struct of the package names as a field type is nested by
	// construction, which is the whole rule and needs no type checking. Such
	// a type is still returned, marked inner, because the sent direction asks
	// what the package publishes anywhere in a response: the row of a list is
	// nested under the list and is exactly what the list endpoint sends, and
	// leaving it out reported a package as failing to surface the fields of
	// its own rows. So is every other exported struct that is not an input,
	// for the same reason.
	byName := make(map[string]declaredStruct, len(found)+len(shared)+len(aliases))
	maps.Copy(byName, shared)
	for _, candidate := range found {
		byName[candidate.Name] = candidate
	}
	// An alias of a shared shape is that shape under the package's own name:
	// resolvable where a field names it, and published by the package, since
	// the name is the package's even though the fields are not. It is nested
	// when the shape is named under either name, or named by another shared
	// shape, since a package that keeps the alias for its converters while
	// the field naming the shape is typed toolutil.X, or sits inside
	// toolutil.MergeRequestOutput, has not made the alias a response of its
	// own; unless an exported function of the package returns it, which is
	// what a handler does with the note it adds to a discussion.
	for local, target := range aliases {
		resolved, ok := shared[target]
		if !ok {
			continue
		}
		resolved.Name = local
		byName[local] = resolved
		found = append(found, resolved)
		if (nested[target] || sharedNested[strings.TrimPrefix(target, sharedPrefix)]) && !parsed.returned[local] {
			nested[local] = true
		}
	}

	out := make([]publishedType, 0, len(found))
	for _, candidate := range found {
		if !ast.IsExported(candidate.Name) || strings.HasSuffix(candidate.Name, inputSuffix) {
			continue
		}
		whole := flatten(candidate, byName, scalars, map[string]bool{})
		if len(whole.Fields) == 0 {
			continue
		}
		published := publishedType{
			Package: pkg,
			Name:    candidate.Name,
			Fields:  whole.Fields,
			Inner:   !strings.HasSuffix(candidate.Name, outputSuffix) || nested[candidate.Name],
			Payload: parsed.enveloped[candidate.Name],
		}
		if !published.Inner {
			published.Nested = nestedTypes(whole, byName, scalars)
		}
		out = append(out, published)
	}
	return out
}

// parsedPackage is what one package's source says about its types.
type parsedPackage struct {
	// structs are the structs it declares, in file then source order.
	structs []declaredStruct
	// nested holds every locally declared or shared type any struct names as
	// a tagged field's type.
	nested map[string]bool
	// enveloped holds every type some struct wraps and carries nothing else
	// beside, which is this repository's shape for a whole response. See
	// [envelopePayloads].
	enveloped map[string]bool
	// alternatives holds the payloads of every struct that wraps more than
	// one type, left for [resolveAlternatives] to accept or refuse once every
	// embed in the package is known.
	alternatives [][]string
	// scalars holds every type it declares as something other than a struct.
	scalars map[string]bool
	// aliases maps every type it declares as, or from, a shared shape to that
	// shape's key.
	aliases map[string]string
	// returned holds every type an exported function of the package returns,
	// which is what a handler does with its response.
	returned map[string]bool
}

// parsePackage reads every non-test Go file of one directory.
//
// A file the parser refuses is passed over, since an audit that reads a tree
// it does not own the state of must not fail over somebody's half-written
// edit; a directory that cannot be read reads as empty for the same reason.
func parsePackage(dir string) parsedPackage {
	parsed := parsedPackage{
		nested: map[string]bool{}, enveloped: map[string]bool{},
		scalars: map[string]bool{}, aliases: map[string]string{}, returned: map[string]bool{},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return parsed
	}
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fileSet, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			continue
		}
		parsed.structs = append(parsed.structs, structsIn(file, &parsed)...)
	}
	resolveAlternatives(&parsed)
	return parsed
}

// resolveAlternatives marks as enveloped the payloads of every struct that
// wraps more than one type, when those types are shapes of one entity: each
// embeds, or is embedded by, another of them. That is how a list GitLab
// answers with one of two entities, chosen by the caller, keeps each in a
// field of its own (projects.ListOutput holds the full project and, under
// simple=true, BasicProjectDetails), and both are then responses of the
// endpoint. Two unrelated objects stay unwrapped, because a response carrying
// a group and a project carries two references and neither is the response.
func resolveAlternatives(parsed *parsedPackage) {
	embeds := map[string]map[string]bool{}
	for _, declared := range parsed.structs {
		for _, embedded := range declared.Embeds {
			if embeds[declared.Name] == nil {
				embeds[declared.Name] = map[string]bool{}
			}
			embeds[declared.Name][embedded] = true
		}
	}
	related := func(a, b string) bool { return embeds[a][b] || embeds[b][a] }
	for _, payloads := range parsed.alternatives {
		if !oneFamily(payloads, related) {
			continue
		}
		for _, payload := range payloads {
			parsed.enveloped[payload] = true
		}
	}
}

// oneFamily reports whether the types are connected under related, so that
// every one of them reaches every other through a chain of embeds rather than
// the set being two groups side by side.
func oneFamily(types []string, related func(a, b string) bool) bool {
	distinct := map[string]bool{}
	for _, name := range types {
		distinct[name] = true
	}
	if len(distinct) == 0 {
		return false
	}
	reached := map[string]bool{types[0]: true}
	queue := []string{types[0]}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for name := range distinct {
			if !reached[name] && related(current, name) {
				reached[name] = true
				queue = append(queue, name)
			}
		}
	}
	return len(reached) == len(distinct)
}

// sharedShapes reads the shapes internal/toolutil declares, each flattened
// within that package and keyed the way a domain package names it
// (toolutil.X), so that a field of that type, an embed of it, or an alias
// of it resolves to its fields, and returns beside them the shapes another
// shared shape names as a field type. A field of one shared shape typed as
// another is rekeyed the same way, so that it resolves from a domain package
// too. The hints type is left out (see [hintsType]).
func sharedShapes(dir string) (shapes map[string]declaredStruct, nested map[string]bool) {
	parsed := parsePackage(dir)
	byName := make(map[string]declaredStruct, len(parsed.structs))
	for _, candidate := range parsed.structs {
		byName[candidate.Name] = candidate
	}
	// The hints type resolves to nothing here too, or a shared shape that
	// embeds it, as most do, would carry the next steps into every package
	// naming it.
	delete(byName, strings.TrimPrefix(hintsType, sharedPrefix))
	shapes = make(map[string]declaredStruct, len(parsed.structs))
	for _, candidate := range parsed.structs {
		key := sharedPrefix + candidate.Name
		if !ast.IsExported(candidate.Name) || key == hintsType {
			continue
		}
		whole := flatten(candidate, byName, parsed.scalars, map[string]bool{})
		whole.Name = key
		for tag, typeName := range whole.FieldTypes {
			if _, local := byName[typeName]; local {
				whole.FieldTypes[tag] = sharedPrefix + typeName
			}
		}
		shapes[key] = whole
	}
	return shapes, parsed.nested
}

// declaredStruct is one struct as parsed, before the top-level output types
// are told from the nested ones and from the structs that are not outputs.
type declaredStruct struct {
	Name   string
	Fields []string
	// FieldTypes maps a json tag to the locally declared type its field
	// carries, for the fields whose type is one.
	FieldTypes map[string]string
	// Embeds names the locally declared types embedded under no json name,
	// whose fields encoding/json promotes into this struct's.
	Embeds []string
}

// flatten returns a struct with the fields its embeds promote into it, the
// way encoding/json marshals them, through as many levels as the embeds go:
// a details type embedding the row type publishes the row's fields as its
// own. A field the struct declares itself wins over a promoted one of the
// same name, as encoding/json's depth rule has it; two embeds promoting one
// name at the same depth, which encoding/json drops, are not told apart, as
// no type here has them. An embed of a type the package declares as
// something other than a struct is a field named after the type, which is
// what encoding/json writes for it. A struct embedding itself through a
// pointer is cut where it repeats, and an embed of a type not declared in the
// package is left out, which is the one thing the parse gives up (see
// [publishedTypes]).
func flatten(candidate declaredStruct, byName map[string]declaredStruct, scalars, walking map[string]bool) declaredStruct {
	if len(candidate.Embeds) == 0 || walking[candidate.Name] {
		return candidate
	}
	walking[candidate.Name] = true
	defer delete(walking, candidate.Name)

	whole := declaredStruct{Name: candidate.Name, Fields: slices.Clone(candidate.Fields), FieldTypes: maps.Clone(candidate.FieldTypes)}
	seen := make(map[string]bool, len(whole.Fields))
	for _, field := range whole.Fields {
		seen[field] = true
	}
	for _, name := range candidate.Embeds {
		embedded, ok := byName[name]
		if !ok {
			if scalars[name] && !seen[name] {
				seen[name] = true
				whole.Fields = append(whole.Fields, name)
			}
			continue
		}
		embedded = flatten(embedded, byName, scalars, walking)
		for _, field := range embedded.Fields {
			if seen[field] {
				continue
			}
			seen[field] = true
			whole.Fields = append(whole.Fields, field)
			if typeName, has := embedded.FieldTypes[field]; has {
				if whole.FieldTypes == nil {
					whole.FieldTypes = map[string]string{}
				}
				whole.FieldTypes[field] = typeName
			}
		}
	}
	sort.Strings(whole.Fields)
	return whole
}

// nestedTypes resolves the locally declared types one output type names as
// field types into their own fields, so a nested object can be held against the
// properties GitLab's record gives the property it sits under.
//
// A field whose type is declared in another package resolves to nothing, the
// same as one carrying a scalar: [namedType] returns "" for a qualified type,
// since a type from elsewhere is not one of ours to compare.
func nestedTypes(candidate declaredStruct, byName map[string]declaredStruct, scalars map[string]bool) map[string]nestedType {
	var out map[string]nestedType
	for tag, typeName := range candidate.FieldTypes {
		target, ok := byName[typeName]
		if !ok {
			continue
		}
		target = flatten(target, byName, scalars, map[string]bool{})
		if len(target.Fields) == 0 {
			continue
		}
		if out == nil {
			out = map[string]nestedType{}
		}
		out[tag] = nestedType{Name: target.Name, Fields: target.Fields}
	}
	return out
}

// structsIn reads the structs one parsed file declares, recording in the
// package every locally declared type any of them names as a field type,
// every type the file declares as something other than a struct, which an
// embed of is a field rather than a promotion, every type the file declares
// as, or from, a shared shape (`type X = toolutil.Y`, or `type X toolutil.Y`,
// which encoding/json treats alike), and every type an exported function
// returns.
//
// Every struct counts for that, not only the output types: an output type
// reached through a plain struct, such as the user under the row of a list of
// uploads, is nested all the same, and the first version of this walk, which
// looked at output types alone, held such a type to the endpoints that answer
// with a whole user. An embed marks nothing nested, for the opposite reason:
// its fields are promoted into the embedding struct, and the embedded type is
// often a response of its own, the row a details type is built on.
func structsIn(file *ast.File, parsed *parsedPackage) []declaredStruct {
	var found []declaredStruct
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			// A type declared inside a function is nobody's response; what an
			// exported function returns is one.
			noteReturned(typed, parsed.returned)
			return false
		case *ast.TypeSpec:
			structType, isStruct := typed.Type.(*ast.StructType)
			if !isStruct {
				if target := namedType(typed.Type); strings.HasPrefix(target, sharedPrefix) {
					parsed.aliases[typed.Name.Name] = target
				} else {
					parsed.scalars[typed.Name.Name] = true
				}
				return false
			}
			fields, fieldTypes, embeds := jsonTags(structType)
			for _, name := range fieldTypes {
				parsed.nested[name] = true
			}
			switch payloads := envelopePayloads(fields, fieldTypes); len(payloads) {
			case 0:
			case 1:
				parsed.enveloped[payloads[0]] = true
			default:
				parsed.alternatives = append(parsed.alternatives, payloads)
			}
			if len(fields) > 0 || len(embeds) > 0 {
				found = append(found, declaredStruct{Name: typed.Name.Name, Fields: fields, FieldTypes: fieldTypes, Embeds: embeds})
			}
			return false
		default:
			return true
		}
	})
	return found
}

// noteReturned records the types an exported function returns. A method is
// left out, since its receiver says the function belongs to a type rather
// than to the package, and so is an unexported function, which is a
// converter rather than a handler.
func noteReturned(fn *ast.FuncDecl, returned map[string]bool) {
	if fn.Recv != nil || !fn.Name.IsExported() || fn.Type.Results == nil {
		return
	}
	for _, result := range fn.Type.Results.List {
		if name := namedType(result.Type); name != "" {
			returned[name] = true
		}
	}
}

// namedType unwraps an expression to the type name at its core: a local
// name as written, a shared shape as toolutil.X, and "" for anything
// qualified by any other package, since a type from elsewhere is not one of
// ours to classify as nested or to resolve.
func namedType(expr ast.Expr) string {
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ArrayType:
			expr = typed.Elt
		case *ast.MapType:
			expr = typed.Value
		case *ast.Ident:
			return typed.Name
		case *ast.SelectorExpr:
			if pkg, isIdent := typed.X.(*ast.Ident); isIdent && pkg.Name+"." == sharedPrefix {
				return sharedPrefix + typed.Sel.Name
			}
			return ""
		default:
			return ""
		}
	}
}

// framingShapes are the shared shapes a response carries because of how this
// server answers rather than because of what GitLab sent. A struct wrapping
// one object beside one of these is still a wrapper.
//
// Only pagination is here. The hint shapes are embedded rather than given a
// json name, so they are never among a struct's tagged fields and need no
// entry.
var framingShapes = map[string]bool{
	sharedPrefix + "PaginationOutput":               true,
	sharedPrefix + "GraphQLPaginationOutput":        true,
	sharedPrefix + "GraphQLForwardPaginationOutput": true,
}

// envelopePayloads names the types a struct is a thin wrapper around, or nil
// when the struct carries content of its own beside them.
//
// The convention here is that a handler answers with a one-key envelope:
// `{badge: BadgeItem}` for a get, `{badges: []BadgeItem, pagination: …}` for a
// list. The envelope is this server's packaging and the payload is what GitLab
// sent, so the payload is the type the shape audit has to judge against the
// endpoint even though some struct names it as a field.
//
// The distinction matters in the other direction too, and getting it wrong is
// worse there: `jobs.ProjectObject` is named by a struct carrying thirty other
// fields, so it is a project REFERENCE inside a job rather than a project
// response, and judging it against the endpoints that answer with a whole
// project reported all eighty-five fields of one as missing from it.
//
// More than one payload comes back as candidates rather than as an answer:
// whether a struct wrapping two types is packaging depends on how the two are
// related, which [resolveAlternatives] decides once the whole package is read.
func envelopePayloads(fields []string, fieldTypes map[string]string) []string {
	var payloads []string
	for _, name := range fields {
		typeName, named := fieldTypes[name]
		if named && framingShapes[typeName] {
			continue
		}
		if !named {
			return nil
		}
		payloads = append(payloads, typeName)
	}
	return payloads
}

// jsonTags returns the json names a struct publishes, sorted, the locally
// declared type each of those names carries where it carries one, and the
// locally declared types it embeds under no name. The names follow the rules
// encoding/json applies to a tag: a field tagged "-" publishes nothing, an
// unexported field publishes nothing whatever its tag says, a tag that names
// no key (`json:",omitempty"`) publishes the Go field name, and an embed
// tagged with a name is published under that name while one tagged with none,
// or not tagged at all, has its fields promoted (see [flatten]). An untagged
// named field is left out rather than guessed at: this repository tags every
// field it means a client to see, so an untagged one is an oversight, and an
// oversight should not become a finding about GitLab.
func jsonTags(structType *ast.StructType) (names []string, fieldTypes map[string]string, embeds []string) {
	publish := func(key string, fieldType ast.Expr) {
		names = append(names, key)
		if typeName := namedType(fieldType); typeName != "" {
			if fieldTypes == nil {
				fieldTypes = map[string]string{}
			}
			fieldTypes[key] = typeName
		}
	}
	embed := func(fieldType ast.Expr) {
		if typeName := namedType(fieldType); typeName != "" {
			embeds = append(embeds, typeName)
		}
	}
	for _, field := range structType.Fields.List {
		name, tagged := jsonName(field)
		switch {
		case name == "-":
		case len(field.Names) == 0 && name != "":
			publish(name, field.Type)
		case len(field.Names) == 0:
			embed(field.Type)
		case tagged:
			for _, ident := range field.Names {
				if !ident.IsExported() {
					continue
				}
				key := name
				if key == "" {
					key = ident.Name
				}
				publish(key, field.Type)
			}
		}
	}
	sort.Strings(names)
	return names, fieldTypes, embeds
}

// jsonName reads the key a field's json tag names, "" for a tag naming none,
// and reports whether the field carries a tag that could be read at all.
func jsonName(field *ast.Field) (name string, tagged bool) {
	if field.Tag == nil {
		return "", false
	}
	raw, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return "", false
	}
	name, _, _ = strings.Cut(reflect.StructTag(raw).Get("json"), ",")
	return name, true
}
