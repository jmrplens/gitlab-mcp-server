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
	var found []publishedType
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
		found = append(found, outputTypesIn(file, pkg, nested)...)
	}

	// Only a top-level output type can be compared with an operation's
	// response. GitLab's document lists the properties of the object an
	// endpoint returns, not of the objects nested inside it, so comparing a
	// nested type against that list reports every one of its fields as
	// unpublished: the first run of this check produced 1418 findings, and
	// almost all of them were the fields of a user, a group or a rule sitting
	// inside a response that does carry them.
	//
	// A type another output type names as a field type is nested by
	// construction, which is the whole rule and needs no type checking.
	out := make([]publishedType, 0, len(found))
	for _, candidate := range found {
		if !nested[candidate.Name] {
			out = append(out, candidate)
		}
	}
	return out
}

// outputTypesIn reads the output types one parsed file declares, recording in
// nested every locally declared type any of them uses as a field type.
func outputTypesIn(file *ast.File, pkg string, nested map[string]bool) []publishedType {
	var found []publishedType
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
			for _, name := range referencedTypes(structType) {
				nested[name] = true
			}
			if fields := jsonTags(structType); len(fields) > 0 {
				found = append(found, publishedType{Package: pkg, Name: typeSpec.Name.Name, Fields: fields})
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

// jsonTags returns the json names a struct publishes, sorted. A field tagged
// "-" publishes nothing, and an untagged one is left out rather than guessed
// at: this repository tags every field it means a client to see, so an untagged
// one is an embed or an oversight, and neither should become a finding about
// GitLab.
func jsonTags(structType *ast.StructType) []string {
	var names []string
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		name, _, _ := strings.Cut(reflect.StructTag(raw).Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
