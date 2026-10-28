// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkoptions

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// The two encoders client-go writes an option struct with, spelled by the
// struct tag each reads: encoding/json for the body of a PATCH, POST or PUT
// built through NewRequest, and go-querystring for the query of every other
// verb and for the multipart fields of every UploadRequest.
const (
	EncoderJSON  = "json"
	EncoderQuery = "url"
)

const (
	// omitEmptyOption and omitZeroOption are the two tag options that decide
	// whether a field is written when it holds nothing. encoding/json reads
	// both, because client-go uses both: DefaultBranchProtectionDefaultsOptions
	// spells its two slices omitzero, which encoding/json has honored since Go
	// 1.24, and reading only omitempty reported all four of them as values a
	// caller cannot decline to send. go-querystring v1.2.0 reads omitempty
	// alone.
	omitEmptyOption = "omitempty"
	omitZeroOption  = "omitzero"
	// hiddenName is the one tag name both encoders read as leaving the field
	// out.
	hiddenName = "-"
	// optionDepth bounds the walk through nested structs. Three levels is the
	// deepest client-go goes (a draft note's position carries a line range
	// carrying two ends) and the bound is what keeps a type that names itself
	// from walking forever.
	optionDepth = 6
	// maxResolve bounds how many defined types are followed to the struct
	// they are defined on, so a cycle of definitions a source could spell
	// cannot loop.
	maxResolve = 8
)

// Encoders names the encoders a request built by the given constructors
// under verb writes its options with, sorted and each once.
//
// NewRequest follows its own switch: encoding/json for PATCH, POST and PUT,
// go-querystring for every other verb, where a nil pointer without omitempty
// is written as an empty value rather than left out. UploadRequest writes
// every field as a multipart form field through go-querystring whatever the
// verb, so an upload is judged by the url tag even on a POST. A constructor
// neither names contributes nothing.
func Encoders(constructors []string, verb string) []string {
	var out []string
	for _, constructor := range constructors {
		if constructor == sdkroutes.ConstructorUpload {
			out = append(out, EncoderQuery)
		}
		if constructor == sdkroutes.ConstructorNewRequest {
			out = append(out, newRequestEncoder(verb))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// newRequestEncoder is the encoder NewRequest uses under verb.
func newRequestEncoder(verb string) string {
	switch strings.ToUpper(verb) {
	case http.MethodPatch, http.MethodPost, http.MethodPut:
		return EncoderJSON
	}
	return EncoderQuery
}

// Tag is how one encoder writes one field.
type Tag struct {
	// Name is the key the field is written under.
	Name string
	// Always is true when the encoder writes the key whatever the field
	// holds: null or the zero value under encoding/json, an empty value
	// under go-querystring.
	Always bool
	// Hidden is true when the encoder leaves the field out.
	Hidden bool
}

// Field is one exported field of a client-go struct, as each encoder writes
// it.
type Field struct {
	// GoName is the field's own name, so a finding reads the way the source
	// does.
	GoName string
	// JSON and Query are how encoding/json and go-querystring write it.
	JSON  Tag
	Query Tag
	// Nested names the client-go struct this field carries, through a
	// pointer, a slice or a map, and is empty for anything else. A generic
	// instantiation such as Nullable[T] names none, which is right: it is a
	// map and behaves like a scalar here.
	Nested string
	// Many is true when the field is a list, which Grape spells with an
	// empty subscript between the parent and the key of a nested struct.
	Many bool
}

// Tag returns how encoder writes the field, encoding/json's for any encoder
// but go-querystring's.
func (f Field) Tag(encoder string) Tag {
	if encoder == EncoderQuery {
		return f.Query
	}
	return f.JSON
}

// Type is one client-go struct: what it carries directly, and the structs
// whose fields are promoted into it.
type Type struct {
	Fields []Field
	// Embedded names the client-go structs embedded without a key, whose
	// fields both encoders promote into this one.
	Embedded []string
}

// SDK is what one parse of client-go's source says about the structs its
// service methods take their parameters in.
type SDK struct {
	// Types is every struct the package declares, by name, a type defined
	// on one or aliasing one included (CreateProjectForUserOptions is
	// declared as CreateProjectOptions).
	Types map[string]Type
	// Methods is every service method, keyed "Service.Method" the way
	// [sdkroutes.Method.Key] spells it, which is also how the action request
	// record names the methods a handler calls.
	Methods map[string]Method
}

// Method is one client-go service method as an option struct check joins
// it: the option structs it takes, the endpoints it reaches and the
// constructors it builds a request with.
type Method struct {
	// Options names the option structs the method takes, each once.
	Options []string
	// Routes is every endpoint the method reaches.
	Routes []sdkroutes.Route
	// Constructors names the request constructors the method can build its
	// request with, as [sdkroutes.Method.Constructors] reads them.
	Constructors []string
	// UploadFields names the form fields an upload of the method sends its
	// file under, which no field of its options carries.
	UploadFields []string
}

// Read parses the client-go source in dir and returns every struct it
// declares together with the service methods, each with the option structs
// it takes, the endpoints it reaches and the constructors it builds them
// with.
//
// The structs are read here, since what their fields write is this package's
// question alone; which endpoints a method given one reaches is
// [sdkroutes]', the reading the per-method view is cut from, so a method that
// hands its options to a helper is credited with the route the helper sends
// them to rather than with none.
//
// A directory that cannot be read, or a file that does not parse, contributes
// nothing rather than failing the caller.
func Read(dir string) SDK {
	files := parseSDKFiles(dir)
	if len(files) == 0 {
		return SDK{}
	}

	found := SDK{Types: map[string]Type{}, Methods: map[string]Method{}}
	defined := map[string]string{}
	for _, file := range files {
		collectTypes(file, found.Types, defined)
	}
	resolveDefined(found.Types, defined)
	keepStructFields(found.Types)
	for _, method := range sdkroutes.Read(dir).Methods() {
		found.Methods[method.Key()] = Method{
			Options: method.Options, Routes: method.Routes,
			Constructors: method.Constructors, UploadFields: method.UploadFields,
		}
	}
	return found
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

// fieldElement names the type a field expression holds, through pointers,
// slices and the values of a map, and whether a slice was passed through on
// the way. A type qualified by another package, such as bytes.Buffer, and a
// generic instantiation name none.
func fieldElement(expr ast.Expr) (name string, many bool) {
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ArrayType:
			many = true
			expr = typed.Elt
		case *ast.MapType:
			expr = typed.Value
		case *ast.Ident:
			return typed.Name, many
		default:
			return "", many
		}
	}
}

// isBareSlice reports whether a field's type is a slice or an array itself,
// not a pointer to one. go-querystring skips an empty one whatever its tag
// says, so its key is never written unconditionally.
func isBareSlice(expr ast.Expr) bool {
	_, isArray := expr.(*ast.ArrayType)
	return isArray
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

// collectTypes records every struct declaration of file into types, and in
// defined every type declared on another type of the package by name
// (`type A B` or `type A = B`), which [resolveDefined] reads once every file
// is in.
func collectTypes(file *ast.File, types map[string]Type, defined map[string]string) {
	for _, declaration := range file.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral {
			continue
		}
		for _, spec := range general.Specs {
			typed, isType := spec.(*ast.TypeSpec)
			if !isType {
				continue
			}
			switch underlying := typed.Type.(type) {
			case *ast.StructType:
				types[typed.Name.Name] = structFields(underlying)
			case *ast.Ident:
				defined[typed.Name.Name] = underlying.Name
			}
		}
	}
}

// resolveDefined gives each type defined on a struct of the package that
// struct's fields, following a definition on a definition up to
// [maxResolve] steps. A definition on anything else (a string, a slice, a
// type of another package) names no struct and is left out.
func resolveDefined(types map[string]Type, defined map[string]string) {
	for name, base := range defined {
		for range maxResolve {
			next, isDefined := defined[base]
			if !isDefined {
				break
			}
			base = next
		}
		if structType, isStruct := types[base]; isStruct {
			types[name] = structType
		}
	}
}

// keepStructFields drops from every field's Nested and every struct's
// Embedded the names that are no struct of the package, so a field of a
// defined scalar type (ISOTime, AccessControlValue) reads as the scalar it
// is.
func keepStructFields(types map[string]Type) {
	for name, structType := range types {
		var embedded []string
		for _, inner := range structType.Embedded {
			if _, isStruct := types[inner]; isStruct {
				embedded = append(embedded, inner)
			}
		}
		fields := slices.Clone(structType.Fields)
		for i := range fields {
			if _, isStruct := types[fields[i].Nested]; !isStruct {
				fields[i].Nested = ""
			}
		}
		types[name] = Type{Fields: fields, Embedded: embedded}
	}
}

// structFields reads one struct's fields as both encoders will write them.
func structFields(structType *ast.StructType) Type {
	var out Type
	if structType.Fields == nil {
		return out
	}
	for _, field := range structType.Fields.List {
		nested, many := fieldElement(field.Type)
		if len(field.Names) == 0 {
			// An embedded struct with no key of its own: its fields are
			// promoted, so they are the enclosing type's own as far as the
			// request is concerned. [keepStructFields] drops a name that is
			// no struct of the package.
			if nested != "" {
				out.Embedded = append(out.Embedded, nested)
			}
			continue
		}
		bareSlice := isBareSlice(field.Type)
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			query := tagFor(field.Tag, EncoderQuery, name.Name)
			query.Always = query.Always && !bareSlice
			out.Fields = append(out.Fields, Field{
				GoName: name.Name,
				JSON:   tagFor(field.Tag, EncoderJSON, name.Name),
				Query:  query,
				Nested: nested,
				Many:   many,
			})
		}
	}
	return out
}

// tagFor reads the key one field is written under by encoder and whether it
// is written unconditionally, or that the tag hides it.
//
// A field with no tag for the encoder is written under its Go name and
// always: that is what both encoders do, and naming it that way produces a
// param GitLab's record will not know, which costs a comparison that could
// not have been made rather than a finding that is wrong. A tag of "-" alone
// hides the field, while "-," names the key "-", which is both encoders' own
// distinction: the comma, not what follows it, is what makes the dash a
// name.
func tagFor(tag *ast.BasicLit, encoder, goName string) Tag {
	untagged := Tag{Name: goName, Always: true}
	if tag == nil {
		return untagged
	}
	value, ok := stringLiteral(tag)
	if !ok {
		return untagged
	}
	spec, tagged := reflect.StructTag(value).Lookup(encoder)
	if !tagged {
		return untagged
	}
	name, rest, hasOptions := strings.Cut(spec, ",")
	if name == hiddenName && !hasOptions {
		return Tag{Hidden: true}
	}
	if name == "" {
		name = goName
	}
	options := strings.Split(rest, ",")
	omitted := slices.Contains(options, omitEmptyOption) || (encoder == EncoderJSON && slices.Contains(options, omitZeroOption))
	return Tag{Name: name, Always: !omitted}
}

// Param is one field of a struct an encoder writes, named the way GitLab's
// record declares the param it becomes.
type Param struct {
	// Owner is the struct the field is declared on, which is not the one the
	// walk started at once a nested struct is reached.
	Owner string
	Field Field
	// Tag is how the walk's encoder writes the field.
	Tag Tag
	// Param is the name the record uses: the key at the top level, and the
	// enclosing keys in subscripts under it, as Grape declares a nested
	// param.
	Param string
}

// Walk calls visit for every field encoder writes when it writes the struct
// name, its own and those of the structs it nests, each named the way GitLab
// declares it. A name types does not hold visits nothing.
//
// encoding/json writes a list of structs as a list of objects, which Grape
// declares with an empty subscript (`actions[][action]`), and go-querystring
// writes each element of a list whole as one value, so a list of structs is
// walked into under the first and not under the second.
func Walk(types map[string]Type, name, encoder string, visit func(Param)) {
	walk(types, name, encoder, "", nil, 0, visit)
}

// walk is [Walk] below the top level: prefix names the keys around the
// struct, ancestors the structs above it and depth how far down it is.
//
// The walk carries the names already visited rather than a set of seen types,
// because one struct can legitimately appear twice under different parents;
// what it must not do is descend into itself forever, which the depth bound
// and the ancestor check together prevent.
func walk(types map[string]Type, name, encoder, prefix string, ancestors []string, depth int, visit func(Param)) {
	if depth > optionDepth || slices.Contains(ancestors, name) {
		return
	}
	structType, known := types[name]
	if !known {
		return
	}
	for _, embedded := range structType.Embedded {
		walk(types, embedded, encoder, prefix, append(ancestors, name), depth+1, visit)
	}
	for _, field := range structType.Fields {
		tag := field.Tag(encoder)
		if tag.Hidden {
			continue
		}
		param := tag.Name
		if prefix != "" {
			param = prefix + "[" + tag.Name + "]"
		}
		visit(Param{Owner: name, Field: field, Tag: tag, Param: param})
		if field.Nested == "" || (field.Many && encoder == EncoderQuery) {
			continue
		}
		child := param
		if field.Many {
			child += "[]"
		}
		walk(types, field.Nested, encoder, child, append(ancestors, name), depth+1, visit)
	}
}

// Canonical spells a param the way GitLab's record declares it, from the way
// an encoder or the request inventory spells it: a list's empty subscript is
// dropped wherever it stands, since the record names client-go's `iids[]`
// as `iids` and a list of objects' `custom_headers[][key]` as
// `custom_headers[key]`.
func Canonical(param string) string {
	return strings.ReplaceAll(param, "[]", "")
}
