package paths

import (
	"cmp"
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
	// Wraps names the payloads of a type that is itself such a wrapper, sorted,
	// and is empty for every other type. It is what lets the type grain tell an
	// envelope whose payload it judges from an output type it cannot judge at
	// all: both carry no pairing of their own, and only the second is a skip.
	Wraps []string
	// Borrowed is true for a type another package under internal/tools
	// declares that a struct of this package publishes a field of, returned
	// under the package that names it: a merge request's commit list is
	// `commits: []commits.Output`. Name is then the type as the package
	// qualifies it and Fields are that type's, flattened where it is declared.
	// The package publishes those fields to every caller of the action, so the
	// sent direction counts them as the package's, which is the one thing the
	// entry is for: it is always Inner, so the type grain passes it over and
	// the package grain's unpublished direction never holds it to the
	// package's endpoints. Its own package judges it at both grains.
	//
	// A type a handler returns whole, without a field of the package naming
	// it, is not borrowed. The guided creation flows of elicitationtools are
	// the case: they return the issue, merge request, project and release
	// outputs of those four packages as they are, publish nothing else, and
	// are judged by nothing for that reason (see [sentCheck]); borrowing their
	// results would hold a package that owns none of the four creation routes
	// to every answer the four owners already give about them.
	Borrowed bool
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
// One exception to that: a type a domain package declares as, or from, a type
// of another package under internal/tools (`type Output = labeldata.Output`)
// is that type under the package's own name, the way an alias of a shared
// shape is. Four packages publish their whole response that way, the project
// and group labels and iterations, and without it the package grain read
// each as publishing no field of the entity its every route answers with.
// Every package under internal/tools is read as such a library too, keyed the
// way a domain package qualifies it (labeldata.Output), and only an alias or
// a defined type resolves through it as a type of the package: a field typed
// as another tools package's type still publishes its own name and nothing
// under it, since resolving it there would hold one domain's shape to another
// domain's endpoints. What such a field names is returned beside the package's
// own types instead, marked borrowed (see [publishedType.Borrowed]), which is
// what lets the sent direction see that a merge request's commit list
// publishes a commit's fields. A package imported
// under another name than its directory's is not recognized, and neither is a
// field of a shape a borrowed type takes from internal/toolutil, since a shape
// library is flattened within itself.
//
// A package that does not parse contributes nothing rather than failing the
// scope, for the reason [publishedTypesIn] records.
func publishedTypes(root string) []publishedType {
	base := filepath.Join(root, toolsDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	shared, sharedNested := sharedShapes(filepath.Join(root, sharedDir), sharedPrefix)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		shapes, nestedShapes := sharedShapes(filepath.Join(base, entry.Name()), entry.Name()+".")
		maps.Copy(shared, shapes)
		maps.Copy(sharedNested, nestedShapes)
	}

	var out []publishedType
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		out = append(out, publishedTypesIn(filepath.Join(base, entry.Name()), toolsDir+"/"+entry.Name(), shared, sharedNested)...)
	}
	slices.SortFunc(out, func(a, b publishedType) int {
		return cmp.Or(strings.Compare(a.Package, b.Package), strings.Compare(a.Name, b.Name))
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
	byName := map[string]declaredStruct{}
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
	//
	// A type declared from another tools package is resolved the same way when
	// that package declares it, and stays the scalar the parse recorded it as
	// when it does not, which is every type from outside this repository.
	resolve := func(local, target string) {
		resolved, ok := shared[target]
		if !ok {
			return
		}
		resolved.Name = local
		byName[local] = resolved
		found = append(found, resolved)
		if (nested[target] || sharedNested[target]) && !parsed.returned[local] {
			nested[local] = true
		}
	}
	for _, local := range slices.Sorted(maps.Keys(aliases)) {
		resolve(local, aliases[local])
	}
	for _, local := range slices.Sorted(maps.Keys(parsed.qualified)) {
		resolve(local, parsed.qualified[local])
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
			Wraps:   structsOnly(parsed.wraps[candidate.Name], byName),
		}
		if !published.Inner {
			published.Nested = nestedTypes(whole, byName, scalars)
		}
		out = append(out, published)
	}
	for _, name := range slices.Sorted(maps.Keys(parsed.borrowed)) {
		shape, known := shared[name]
		if !known || len(shape.Fields) == 0 {
			// A type from outside this repository, or one publishing nothing.
			continue
		}
		out = append(out, publishedType{Package: pkg, Name: name, Fields: shape.Fields, Inner: true, Borrowed: true})
	}
	return out
}

// structsOnly keeps the payloads that name a struct the package can resolve,
// or nil when none does.
//
// The envelope rule reads a field's type by name alone, so a struct whose one
// field is a string reads as wrapping "string", and one whose one field is a
// type from another package reads as wrapping nothing the walk knows. Neither
// is packaging around a response, and a type-grain envelope that claims it is
// would be credited with a payload no pairing can ever name.
func structsOnly(payloads []string, byName map[string]declaredStruct) []string {
	var out []string
	for _, payload := range payloads {
		if _, known := byName[payload]; known {
			out = append(out, payload)
		}
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
	// wraps holds, per wrapping struct, the payloads it wraps, sorted: the
	// other half of enveloped.
	wraps map[string][]string
	// alternatives holds the payloads of every struct that wraps more than
	// one type, keyed by that struct, left for [resolveAlternatives] to accept
	// or refuse once every embed in the package is known.
	alternatives map[string][]string
	// scalars holds every type it declares as something other than a struct.
	scalars map[string]bool
	// aliases maps every type it declares as, or from, a shared shape to that
	// shape's key.
	aliases map[string]string
	// qualified maps every type it declares as, or from, a type another
	// package declares, the shared one aside, to that type's qualified name
	// (labeldata.Output). Each is among the scalars too, since nothing here
	// knows whether the other package is one this repository can read, and
	// [publishedTypesIn] resolves the ones it can.
	qualified map[string]string
	// returned holds every type an exported function of the package returns,
	// which is what a handler does with its response.
	returned map[string]bool
	// borrowed holds every type another package declares that a struct of this
	// package publishes a field of or embeds, qualified as written
	// (commits.Output). A shared shape is left out, since [namedType] already
	// resolves it where it is named; a type [publishedTypesIn] cannot resolve,
	// such as every client-go type, is kept here and dropped there.
	borrowed map[string]bool
}

// parsePackage reads every non-test Go file of one directory.
//
// A file the parser refuses is passed over, since an audit that reads a tree
// it does not own the state of must not fail over somebody's half-written
// edit; a directory that cannot be read reads as empty for the same reason.
func parsePackage(dir string) parsedPackage {
	parsed := parsedPackage{
		nested: map[string]bool{}, enveloped: map[string]bool{}, wraps: map[string][]string{},
		alternatives: map[string][]string{},
		scalars:      map[string]bool{}, aliases: map[string]string{}, qualified: map[string]string{},
		returned: map[string]bool{}, borrowed: map[string]bool{},
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
// wraps more than one type, when those types are distinct shapes of one
// entity: each embeds, or is embedded by, another of them, directly or
// through types of the package that are not payloads themselves. That is how
// a list GitLab answers with one of two entities, chosen by the caller, keeps
// each in a field of its own (projects.ListOutput holds the full project and,
// under simple=true, BasicProjectDetails), and both are then responses of the
// endpoint. Two unrelated objects stay unwrapped, because a response carrying
// a group and a project carries two references and neither is the response;
// so do two fields of one type, a before and an after, which are two
// references to the same kind of object rather than two shapes of it.
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
	related := func(a, b string) bool { return embedsTransitively(embeds, a, b) || embedsTransitively(embeds, b, a) }
	for wrapper, payloads := range parsed.alternatives {
		if !oneFamily(payloads, related) {
			continue
		}
		for _, payload := range payloads {
			parsed.enveloped[payload] = true
		}
		parsed.wraps[wrapper] = slices.Sorted(slices.Values(payloads))
	}
}

// embedsTransitively reports whether from reaches to by following embeds
// through any type of the package, so that a chain whose middle link is not a
// payload still ties its two ends into one family.
func embedsTransitively(embeds map[string]map[string]bool, from, to string) bool {
	seen := map[string]bool{from: true}
	stack := []string{from}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for next := range embeds[current] {
			if next == to {
				return true
			}
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return false
}

// oneFamily reports whether the types are distinct and connected under
// related, so that every one of them reaches every other through a chain of
// embeds rather than the set being two groups side by side. A type named
// twice is not a second shape, so a set that repeats one is no family.
func oneFamily(types []string, related func(a, b string) bool) bool {
	distinct := map[string]bool{}
	for _, name := range types {
		distinct[name] = true
	}
	if len(distinct) == 0 || len(distinct) != len(types) {
		return false
	}
	reached := map[string]bool{types[0]: true}
	queue := []string{types[0]}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for name := range distinct {
			// Two tests rather than one conjunction: joined, the reached
			// check reads as a guard a mutation may drop, and without it
			// every related name is queued again on every pass and the walk
			// never ends.
			if reached[name] {
				continue
			}
			if related(current, name) {
				reached[name] = true
				queue = append(queue, name)
			}
		}
	}
	return len(reached) == len(distinct)
}

// sharedShapes reads the shapes one package declares, each flattened within
// that package and keyed the way another package names it (prefix is
// "toolutil." for internal/toolutil and "labeldata." for the tools package of
// that name), so that a field of that type, an embed of it, or an alias of it
// resolves to its fields, and returns beside them, keyed the same way, the
// shapes another shape of the package names as a field type. A field of one
// shape typed as another of the package is rekeyed the same way, so that it
// resolves from a domain package too. The hints type is left out (see
// [hintsType]).
func sharedShapes(dir, prefix string) (shapes map[string]declaredStruct, nested map[string]bool) {
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
		key := prefix + candidate.Name
		if !ast.IsExported(candidate.Name) || key == hintsType {
			continue
		}
		whole := flatten(candidate, byName, parsed.scalars, map[string]bool{})
		whole.Name = key
		for tag, typeName := range whole.FieldTypes {
			if _, local := byName[typeName]; local {
				whole.FieldTypes[tag] = prefix + typeName
			}
		}
		shapes[key] = whole
	}
	// Only the package's own shapes are named, since what one package nests
	// of another's says nothing about how a third uses it.
	nested = map[string]bool{}
	for name := range parsed.nested {
		if _, local := byName[name]; local {
			nested[prefix+name] = true
		}
	}
	return shapes, nested
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
					if qualified := qualifiedName(typed.Type); qualified != "" {
						parsed.qualified[typed.Name.Name] = qualified
					}
				}
				return false
			}
			fields, fieldTypes, embeds := jsonTags(structType)
			noteNamedTypes(structType, fieldTypes, parsed)
			payloads := envelopePayloads(fields, fieldTypes)
			if len(fields) == 0 {
				payloads = embeddedPayload(embeds)
			}
			switch len(payloads) {
			case 0:
			case 1:
				parsed.enveloped[payloads[0]] = true
				parsed.wraps[typed.Name.Name] = payloads
			default:
				parsed.alternatives[typed.Name.Name] = payloads
			}
			// A struct with neither fields nor embeds is kept too: it
			// publishes nothing, so it is never compared, and a struct naming
			// it resolves it to nothing either way.
			found = append(found, declaredStruct{Name: typed.Name.Name, Fields: fields, FieldTypes: fieldTypes, Embeds: embeds})
			return false
		default:
			return true
		}
	})
	return found
}

// noteNamedTypes records, for one struct, the types it names that the package
// has to know about: every locally declared or shared type a field carries,
// which is thereby nested, and every type of another package it publishes a
// field of or embeds, which the package borrows.
func noteNamedTypes(structType *ast.StructType, fieldTypes map[string]string, parsed *parsedPackage) {
	for _, name := range fieldTypes {
		parsed.nested[name] = true
	}
	for _, name := range borrowedTypes(structType) {
		parsed.borrowed[name] = true
	}
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

// borrowedTypes names the types of other packages a struct publishes a field
// of or embeds, under the rules [jsonTags] reads a struct by: a field tagged
// "-", an unexported field and an untagged named field publish nothing, and an
// embed publishes its fields whether or not it is tagged with a name.
func borrowedTypes(structType *ast.StructType) []string {
	var names []string
	for _, field := range structType.Fields.List {
		name, tagged := jsonName(field)
		if name == "-" || (len(field.Names) > 0 && (!tagged || !anyExported(field.Names))) {
			continue
		}
		if borrowed := borrowedName(field.Type); borrowed != "" {
			names = append(names, borrowed)
		}
	}
	return names
}

// anyExported reports whether one of a field's names is exported, which is
// what makes encoding/json write it.
func anyExported(names []*ast.Ident) bool {
	for _, ident := range names {
		if ident.IsExported() {
			return true
		}
	}
	return false
}

// borrowedName unwraps a field's type the way [namedType] does and names the
// type at its core when another package declares it, qualified as written, or
// returns "" for a local name, a shared shape (which [namedType] resolves) and
// anything that is not a type name at all.
func borrowedName(expr ast.Expr) string {
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ArrayType:
			expr = typed.Elt
		case *ast.MapType:
			expr = typed.Value
		default:
			name := qualifiedName(expr)
			if strings.HasPrefix(name, sharedPrefix) {
				return ""
			}
			return name
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

// qualifiedName is the qualified name a type is declared as, or from, when it
// is written as another package's type and nothing else (`labeldata.Output`),
// and "" otherwise. It is kept apart from [namedType], which is read wherever a
// field names a type and would otherwise start resolving the fields of every
// type from another package that a struct names.
func qualifiedName(expr ast.Expr) string {
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return ""
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return ""
	}
	return pkg.Name + "." + selector.Sel.Name
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

// embeddedPayload names the one type a struct publishing no field of its own
// embeds, the next-step hints aside, or nil when it embeds none or several.
//
// It is the envelope rule for the other way this repository writes one:
// `GetOutput{HintableOutput; PlanLimitItem}` publishes exactly the plan
// limits' fields, promoted into it, and adds nothing of its own, so the plan
// limits are what GitLab answered with and the struct around them is
// packaging, as `{badge: BadgeItem}` is. Without it the embedded type was
// read as a reference for being named by another struct and never judged,
// while the struct around it had no pairing to be judged by.
//
// Two embeds are left alone: a struct built from two shapes is a response of
// its own, and neither half is what GitLab sent.
func embeddedPayload(embeds []string) []string {
	var payload []string
	for _, name := range embeds {
		if name == hintsType {
			continue
		}
		payload = append(payload, name)
	}
	if len(payload) != 1 {
		return nil
	}
	return payload
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
