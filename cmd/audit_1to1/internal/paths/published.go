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

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apishapes"
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
	// which is as far as GitLab's record goes (see [apishapes.Operation.Nested]),
	// and it is empty on an inner type: a type reached through a field of a
	// field is compared against nothing, so collecting it would only grow the
	// walk.
	Nested map[string]nestedType
	// Inner is true for a type that is nobody's response on its own: one some
	// struct of the package names as a field type, or one not named as an
	// output type. It is compared with no operation and counted with none,
	// and in the sent direction its fields still count as the package's, since
	// the row of a list is inner and is what the list endpoint sends.
	Inner bool
}

// nestedType is one output type reached through a field of another.
type nestedType struct {
	Name   string
	Fields []string
}

// toolsDir is where the domain packages live.
const toolsDir = "internal/tools"

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
	return filepath.Join(root, apishapes.DefaultDir)
}

// publishedTypes reads every exported struct under internal/tools that is not
// an input and returns the json tags each one publishes, the `*Output` types
// nothing names as a field type first among them (see [publishedType.Inner]).
//
// It parses rather than type-checks on purpose. What the comparison needs is
// the set of names a client sees, which is exactly the set of json tags in the
// source; loading the whole program with types would cost twenty seconds and
// answer the same question. The one thing parsing gives up is a field promoted
// from an embedded type in another package, and every such embed in this
// repository is toolutil.HintableOutput, whose fields are hints rather than
// GitLab's data.
//
// A package that does not parse contributes nothing rather than failing the
// scope, for the reason [publishedTypesIn] records.
func publishedTypes(root string) []publishedType {
	base := filepath.Join(root, toolsDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	var out []publishedType
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		out = append(out, publishedTypesIn(filepath.Join(base, entry.Name()), toolsDir+"/"+entry.Name())...)
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
func publishedTypesIn(dir, pkg string) []publishedType {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	fileSet := token.NewFileSet()
	var found []declaredStruct
	nested := map[string]bool{}
	scalars := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fileSet, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			continue
		}
		found = append(found, structsIn(file, nested, scalars)...)
	}

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
	byName := make(map[string]declaredStruct, len(found))
	for _, candidate := range found {
		byName[candidate.Name] = candidate
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
		}
		if !published.Inner {
			published.Nested = nestedTypes(whole, byName, scalars)
		}
		out = append(out, published)
	}
	return out
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

// structsIn reads the structs one parsed file declares, recording in nested
// every locally declared type any of them names as a field type, and in
// scalars every type the file declares as something other than a struct,
// which an embed of is a field rather than a promotion.
//
// Every struct counts for that, not only the output types: an output type
// reached through a plain struct, such as the user under the row of a list of
// uploads, is nested all the same, and the first version of this walk, which
// looked at output types alone, held such a type to the endpoints that answer
// with a whole user. An embed marks nothing nested, for the opposite reason:
// its fields are promoted into the embedding struct, and the embedded type is
// often a response of its own, the row a details type is built on.
func structsIn(file *ast.File, nested, scalars map[string]bool) []declaredStruct {
	var found []declaredStruct
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			// A type declared inside a function is nobody's response.
			return false
		case *ast.TypeSpec:
			structType, isStruct := typed.Type.(*ast.StructType)
			if !isStruct {
				scalars[typed.Name.Name] = true
				return false
			}
			fields, fieldTypes, embeds := jsonTags(structType)
			for _, name := range fieldTypes {
				nested[name] = true
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

// namedType unwraps an expression to the local type name at its core, and
// returns "" for anything qualified by a package: a type from another package
// is not one of ours to classify as nested.
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
		default:
			return ""
		}
	}
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
