package paths

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
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
	// and it is empty on a nested type: a type reached through a field of a
	// field is compared against nothing, so collecting it would only grow the
	// walk.
	Nested map[string]nestedType
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

// recordDir is where the GitLab API record lives for a given repository root.
func recordDir(root string) string {
	return filepath.Join(root, apishapes.DefaultDir)
}

// publishedTypes reads every `*Output` struct under internal/tools and returns
// the json tags each one publishes.
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
	var found []declaredOutputType
	nested := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fileSet, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			continue
		}
		found = append(found, outputTypesIn(file, nested)...)
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
	// A type another output type names as a field type is nested by
	// construction, which is the whole rule and needs no type checking.
	byName := make(map[string]declaredOutputType, len(found))
	for _, candidate := range found {
		byName[candidate.Name] = candidate
	}

	out := make([]publishedType, 0, len(found))
	for _, candidate := range found {
		if nested[candidate.Name] {
			continue
		}
		out = append(out, publishedType{
			Package: pkg,
			Name:    candidate.Name,
			Fields:  candidate.Fields,
			Nested:  nestedTypes(candidate, byName),
		})
	}
	return out
}

// declaredOutputType is one `*Output` struct as parsed, before the top-level
// ones are told from the nested ones.
type declaredOutputType struct {
	Name   string
	Fields []string
	// FieldTypes maps a json tag to the locally declared type its field
	// carries, for the fields whose type is one.
	FieldTypes map[string]string
}

// nestedTypes resolves the locally declared types one output type names as
// field types into their own fields, so a nested object can be held against the
// properties GitLab's record gives the property it sits under.
//
// A field whose type is declared in another package resolves to nothing, the
// same as one carrying a scalar: [namedType] returns "" for a qualified type,
// since a type from elsewhere is not one of ours to compare.
func nestedTypes(candidate declaredOutputType, byName map[string]declaredOutputType) map[string]nestedType {
	var out map[string]nestedType
	for tag, typeName := range candidate.FieldTypes {
		target, ok := byName[typeName]
		if !ok || len(target.Fields) == 0 {
			continue
		}
		if out == nil {
			out = map[string]nestedType{}
		}
		out[tag] = nestedType{Name: target.Name, Fields: target.Fields}
	}
	return out
}

// outputTypesIn reads the output types one parsed file declares, recording in
// nested every locally declared type any of them uses as a field type.
func outputTypesIn(file *ast.File, nested map[string]bool) []declaredOutputType {
	var found []declaredOutputType
	for _, declaration := range file.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, isType := spec.(*ast.TypeSpec)
			if !isType || !strings.HasSuffix(typeSpec.Name.Name, outputSuffix) {
				continue
			}
			structType, isStruct := typeSpec.Type.(*ast.StructType)
			if !isStruct {
				continue
			}
			// Every locally declared type used as a field type is nested,
			// whether or not that field carries a json tag: an untagged embed
			// of one output type in another is still not a response of its own.
			for _, name := range referencedTypes(structType) {
				nested[name] = true
			}
			fields, fieldTypes := jsonTags(structType)
			if len(fields) > 0 {
				found = append(found, declaredOutputType{Name: typeSpec.Name.Name, Fields: fields, FieldTypes: fieldTypes})
			}
		}
	}
	return found
}

// referencedTypes names every locally declared type this struct uses as a field
// type, through any number of pointers, slices and maps.
func referencedTypes(structType *ast.StructType) []string {
	var names []string
	for _, field := range structType.Fields.List {
		if name := namedType(field.Type); name != "" {
			names = append(names, name)
		}
	}
	return names
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

// jsonTags returns the json names a struct publishes, sorted, and the locally
// declared type each of those names carries where it carries one. The names
// follow the rules encoding/json applies to a tag: a field tagged "-"
// publishes nothing, an unexported field publishes nothing whatever its tag
// says, a tag that names no key (`json:",omitempty"`) publishes the Go field
// name, and an embed tagged with a name is published under that name. An
// untagged field is left out rather than guessed at: this repository tags
// every field it means a client to see, so an untagged one is an embed or an
// oversight, and neither should become a finding about GitLab.
func jsonTags(structType *ast.StructType) (names []string, fieldTypes map[string]string) {
	publish := func(key string, fieldType ast.Expr) {
		names = append(names, key)
		if typeName := namedType(fieldType); typeName != "" {
			if fieldTypes == nil {
				fieldTypes = map[string]string{}
			}
			fieldTypes[key] = typeName
		}
	}
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		name, _, _ := strings.Cut(reflect.StructTag(raw).Get("json"), ",")
		if name == "-" {
			continue
		}
		if len(field.Names) == 0 {
			if name != "" {
				publish(name, field.Type)
			}
			continue
		}
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
	sort.Strings(names)
	return names, fieldTypes
}
