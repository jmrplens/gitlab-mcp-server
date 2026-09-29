package paths

import (
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// goShape is a field's Go type the way encoding/json reads and writes it: the
// pointers, slices and arrays around a core type, the core, and the two tag
// options that change what the core takes.
//
// The core is kept as the name the source writes rather than resolved when
// the struct is read, because what a name is depends on the package reading
// it: a struct of the package, a type it declares as something else, a shape
// internal/toolutil shares, or a type of another package. [typeSource.kinds]
// resolves it against the package the Orbit walk is in.
type goShape struct {
	// Layers are the wrappers around the core, outermost first, one
	// character each: layerPointer, layerSlice or layerArray.
	Layers string
	// Core is the type in the middle: the name the source writes ("int64",
	// "StatusSystem", "time.Time", "toolutil.Person"), coreMap, coreInterface
	// or coreStruct for a type written in place, and "" for anything else (a
	// function, a channel, an instantiated generic type).
	Core string
	// Quoted is the ",string" option, which makes a scalar travel inside a
	// JSON string.
	Quoted bool
	// OmitEmpty is the ",omitempty" option.
	OmitEmpty bool
	// Spelled is the type as the source writes it, for a finding to name.
	Spelled string
}

// The wrappers a goShape records around its core.
const (
	layerPointer = "*"
	layerSlice   = "["
	layerArray   = "#"
)

// The cores a goShape records for a type written in place. None of them can
// be the name of a type, so none is mistaken for one.
const (
	coreMap       = "map"
	coreInterface = "interface{}"
	coreStruct    = "struct{}"
)

// shapeOf reads a field's type expression and the options of its json tag
// into a goShape.
func shapeOf(expr ast.Expr, options string) goShape {
	shape := goShape{Spelled: types.ExprString(expr), Quoted: hasOption(options, "string"), OmitEmpty: hasOption(options, "omitempty")}
	var layers strings.Builder
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			layers.WriteString(layerPointer)
			expr = typed.X
			continue
		case *ast.ArrayType:
			layers.WriteString(arrayLayer(typed))
			expr = typed.Elt
			continue
		case *ast.Ident:
			shape.Core = typed.Name
		case *ast.SelectorExpr:
			shape.Core = qualifiedName(typed)
		case *ast.MapType:
			shape.Core = coreMap
		case *ast.InterfaceType:
			shape.Core = coreInterface
		case *ast.StructType:
			shape.Core = coreStruct
		}
		shape.Layers = layers.String()
		return shape
	}
}

// arrayLayer tells a slice, whose zero value is nil, from an array, whose
// zero value is not.
func arrayLayer(array *ast.ArrayType) string {
	if array.Len == nil {
		return layerSlice
	}
	return layerArray
}

// hasOption reports whether a json tag's options, the part after its name,
// list one.
func hasOption(options, option string) bool {
	return slices.Contains(strings.Split(options, ","), option)
}

// scalarCores are the cores encoding/json reads a scalar into and writes one
// from, each with the kinds it does both for: a string, a boolean, the
// predeclared numeric types, and json.Number, which is written as a number.
// The complex types are not among them: encoding/json refuses them.
var scalarCores = map[string][]string{
	"string": {orbitrecord.KindString}, "bool": {orbitrecord.KindBoolean},
	"json.Number": {orbitrecord.KindNumber},
	"int":         {orbitrecord.KindNumber}, "int8": {orbitrecord.KindNumber}, "int16": {orbitrecord.KindNumber},
	"int32": {orbitrecord.KindNumber}, "int64": {orbitrecord.KindNumber},
	"uint": {orbitrecord.KindNumber}, "uint8": {orbitrecord.KindNumber}, "uint16": {orbitrecord.KindNumber},
	"uint32": {orbitrecord.KindNumber}, "uint64": {orbitrecord.KindNumber}, "uintptr": {orbitrecord.KindNumber},
	"float32": {orbitrecord.KindNumber}, "float64": {orbitrecord.KindNumber},
	"byte": {orbitrecord.KindNumber}, "rune": {orbitrecord.KindNumber},
}

// readAlso are the kinds a scalar core is read from beside the ones it is
// written as: json.Number takes a string holding a number, and writes it back
// as the number. Only the decoder's reading asks for them.
var readAlso = map[string][]string{
	"json.Number": {orbitrecord.KindString},
}

// wholeCores are the cores that take any JSON value as it comes: an
// interface, which is what `any` is, a map, which the check asks nothing of
// below it, and a raw message, which is the bytes themselves.
var wholeCores = map[string]bool{
	"any": true, coreInterface: true, coreMap: true, "json.RawMessage": true,
}

// kindSet is what a Go type takes at one depth of elements below a field.
type kindSet struct {
	// every is true for a type that takes any kind, or a depth below a value
	// that was not a list, which the depth above it reports.
	every bool
	// unknown is true for a type this reader cannot map to a kind: one whose
	// own methods decide what it takes, one from a package it does not model,
	// or one it cannot read at all.
	unknown bool
	// kinds are the kinds taken otherwise.
	kinds []string
}

// refuses returns the recorded kinds the set does not take, sorted, or nil.
func (k kindSet) refuses(recorded map[string]bool) []string {
	if k.every {
		return nil
	}
	var refused []string
	for _, kind := range slices.Sorted(maps.Keys(recorded)) {
		if !slices.Contains(k.kinds, kind) {
			refused = append(refused, kind)
		}
	}
	return refused
}

// resolvedValue is a field's type at one depth of elements below it, once the
// pointers in front of the value and the names the package declares have
// been followed to what they stand for.
type resolvedValue struct {
	layers, core string
	// pointer is true when a pointer stands in front of the value.
	pointer bool
	// below is true when the depth sits under a value that is not a list,
	// which the depth above it reports.
	below bool
}

// resolve follows a field's type down to one depth of elements: the field's
// own value at 0, the elements of the list it holds at 1, the elements of
// those at 2. A name the package declares is followed to what it is declared
// as, unless its own methods decide what it takes under the reading asked
// (see [typeSource.opaque]), and names declared as each other end where they
// repeat.
func (s *typeSource) resolve(shape goShape, depth int, decoding bool) resolvedValue {
	layers, core, pointer := shape.Layers, shape.Core, false
	following := map[string]bool{}
	for {
		for strings.HasPrefix(layers, layerPointer) {
			layers, pointer = layers[1:], true
		}
		if layers == "" {
			next, named := s.named[core]
			if !named || s.opaque(core, decoding) || following[core] {
				break
			}
			following[core] = true
			layers, core = next.Layers, next.Core
			continue
		}
		if depth == 0 {
			break
		}
		layers, depth, pointer = layers[1:], depth-1, false
	}
	return resolvedValue{layers: layers, core: core, pointer: pointer, below: depth > 0}
}

// opaque reports whether a type's own methods decide the kinds it takes under
// one reading: a type that decodes itself under both, and one that writes
// itself under the published type's, which hands on whatever that method
// writes.
func (s *typeSource) opaque(core string, decoding bool) bool {
	return s.decoders[core] || (!decoding && s.encoders[core])
}

// kinds answers which JSON kinds a field of this shape takes at a depth of
// elements below it, under one of two readings of encoding/json, which is what
// tells the two halves of the kinds question apart.
//
// The decoder's reading (decoding true) is which kinds encoding/json reads
// into the field without an error, which is what fails a handler at run time:
// a null into anything, since it leaves a value it cannot hold as it was; a
// byte slice from a base64 string or from an array of numbers; and json.Number
// from a number or from a string holding one.
//
// The published type's reading (decoding false) is which kinds the field
// carries as they came: read into it and written back out as the same kind,
// which is what a caller is shown. json.Number is written as a number and a
// byte slice as a base64 string, so neither carries the other kind it reads. A
// null carries only where the value it leaves is written as a null or as the
// key left out: behind a pointer, as a nil slice or map, in an interface, and
// in a scalar tagged omitempty, whose zero is the key left out. A scalar
// without omitempty is written as a zero GitLab did not send, and a struct
// value is written whole whatever its tag says, as is an element of a list
// whatever its field's tag says, so a null reaching any of them does not
// carry.
//
// Both readings are of kinds and never of values, which the record does not
// keep: a string that json.Number or time.Time cannot read, one holding no
// number or one that is not RFC 3339, passes as a string, and a zero GitLab
// sent that an omitempty scalar leaves out passes as the number it is.
func (s *typeSource) kinds(shape goShape, depth int, decoding bool) kindSet {
	value := s.resolve(shape, depth, decoding)
	if value.below {
		return kindSet{every: true}
	}
	set, scalar := s.valueKinds(value.layers, value.core, decoding)
	if set.every || set.unknown {
		return set
	}
	nullable := decoding || value.pointer
	if scalar && depth == 0 {
		if shape.Quoted {
			set.kinds = []string{orbitrecord.KindString}
		}
		nullable = nullable || shape.OmitEmpty
	}
	if nullable && !slices.Contains(set.kinds, orbitrecord.KindNull) {
		set.kinds = append(set.kinds, orbitrecord.KindNull)
	}
	return set
}

// valueKinds is what one value takes once the pointers in front of it are
// set aside, before a null is: a list, or its core. It also says whether the
// value is a scalar, which the tag options apply to.
//
// It is written as plain conditions rather than as a switch of cases,
// because a case expression carries no statement counter and a mutation
// tester reports every change to one as never reached.
func (s *typeSource) valueKinds(layers, core string, decoding bool) (set kindSet, scalar bool) {
	if layers != "" {
		return listKinds(layers, core, decoding), false
	}
	return s.coreKinds(core, decoding)
}

// listKinds is what a slice or an array takes, its first layer being one of
// the two. A slice may be nil and an array may not; a byte slice is written as
// a base64 string, and read from one or from an array of numbers.
func listKinds(layers, core string, decoding bool) kindSet {
	if strings.HasPrefix(layers, layerArray) {
		return kindSet{kinds: []string{orbitrecord.KindArray}}
	}
	if layers == layerSlice && (core == "byte" || core == "uint8") {
		if decoding {
			return kindSet{kinds: []string{orbitrecord.KindArray, orbitrecord.KindString, orbitrecord.KindNull}}
		}
		return kindSet{kinds: []string{orbitrecord.KindString, orbitrecord.KindNull}}
	}
	return kindSet{kinds: []string{orbitrecord.KindArray, orbitrecord.KindNull}}
}

// coreKinds is what a core takes, and whether it is a scalar.
func (s *typeSource) coreKinds(core string, decoding bool) (set kindSet, scalar bool) {
	if s.opaque(core, decoding) {
		return kindSet{unknown: true}, false
	}
	if kinds, isScalar := scalarCores[core]; isScalar {
		set.kinds = slices.Clone(kinds)
		if decoding {
			set.kinds = append(set.kinds, readAlso[core]...)
		}
		return set, true
	}
	if core == "time.Time" {
		return kindSet{kinds: []string{orbitrecord.KindString}}, false
	}
	if wholeCores[core] {
		return kindSet{every: true}, false
	}
	if core == coreStruct || s.isStruct(core) {
		return kindSet{kinds: []string{orbitrecord.KindObject}}, false
	}
	return kindSet{unknown: true}, false
}

// kindJudgement is what the kinds question found in one output type, or in
// the client-go struct its handler decodes into.
type kindJudgement struct {
	// compared counts the field and depth pairs judged.
	compared int
	// found are the kinds refused, one finding per pair.
	found []OrbitField
	// unjudged are the fields whose type could not be read, once each.
	unjudged []OrbitField
}

// mismatched reports every field whose recorded kinds its Go type does not
// take under the judgement's reading, once for each depth of elements they
// were recorded at, with the path spelled with that many element markers, and
// names, once each, the fields whose Go type this reader cannot map to a kind.
// The body itself is judged by nothing here: an output type is this server's
// own wrapper and publishes no field of it, and a client-go struct that
// decodes the whole body itself is named by [decoderSource.judge] instead.
func (j orbitJudgement) mismatched() kindJudgement {
	var judged kindJudgement
	for _, path := range slices.Sorted(maps.Keys(j.published.fields)) {
		key, carried := j.recorded[path]
		if !carried {
			continue
		}
		entry := j.published.fields[path]
		for _, depth := range slices.Sorted(maps.Keys(key.kinds)) {
			at := path + strings.Repeat(orbitrecord.Element, depth)
			set := j.types.kinds(entry.shape, depth, j.decoding)
			if set.unknown {
				judged.unjudged = append(judged.unjudged, j.kindFinding(at, entry, slices.Sorted(maps.Keys(key.kinds[depth]))))
				break
			}
			judged.compared++
			if refused := set.refuses(key.kinds[depth]); refused != nil {
				judged.found = append(judged.found, j.kindFinding(at, entry, refused))
			}
		}
	}
	return judged
}

// kindFinding is one field held to the kinds recorded at it.
func (j orbitJudgement) kindFinding(path string, entry publishedField, kinds []string) OrbitField {
	return OrbitField{
		Package: j.pkg, Output: j.output, Path: path, Type: j.qualifier + entry.owner, Field: entry.field,
		GoType: entry.shape.Spelled, Kinds: kinds, Calls: j.calls,
	}
}

// trailingElements counts the element markers a recorded path ends in: 0 for
// a key's own value, 1 for the elements of the list it holds.
func trailingElements(path string) int {
	depth := 0
	for strings.HasSuffix(path, orbitrecord.Element) {
		path = strings.TrimSuffix(path, orbitrecord.Element)
		depth++
	}
	return depth
}

// clientGoQualifier is how this repository imports client-go, which is how a
// finding about one of its structs names it.
const clientGoQualifier = "gl."

// decoderSource is client-go's package as the decoder half of the kinds
// question reads it, and the structs a converter pairs with each output type,
// which are what the handlers decode GitLab's answer into before the output
// type is filled from them.
type decoderSource struct {
	types *typeSource
	// paired maps a package's short name and an output type to the client-go
	// structs a converter reads to fill it, sorted.
	paired map[[2]string][]string
}

// readDecoders is the seam over the decoder half's source, so a test of the
// whole run can hand the Orbit check a client-go package without handing it
// to every other check that reads the converter pairing.
var readDecoders = readDecoderSource

// readDecoderSource reads the client-go package the handlers compile against
// and the converter pairings that name its structs, through the same pairing
// the type grain and the field diff read, or returns nil when the pairing
// cannot be had, which is the one way the decoder half is skipped.
func readDecoderSource(root string) *decoderSource {
	pairings, err := collectPairings(root)
	if err != nil || pairings.ClientGoDir == "" {
		return nil
	}
	return decoderSourceFrom(pairings)
}

// decoderSourceFrom parses the client-go package a pairing names and indexes
// its output pairings by package and output type.
//
// The package is read by the parse every tools package is read by, so a
// struct's fields are its json tags and nothing more: an untagged exported
// field, which encoding/json reads under its Go name, is not judged, and none
// of the Orbit structs has one.
func decoderSourceFrom(pairings structs.Pairings) *decoderSource {
	parsed := parsePackage(pairings.ClientGoDir)
	byName := make(map[string]declaredStruct, len(parsed.structs))
	for _, declared := range parsed.structs {
		byName[declared.Name] = declared
	}
	paired := map[[2]string][]string{}
	for _, pairing := range pairings.Outputs {
		key := [2]string{pairing.Package, pairing.MCPType}
		if !slices.Contains(paired[key], pairing.SDKType) {
			paired[key] = append(paired[key], pairing.SDKType)
		}
	}
	for _, decodedBy := range paired {
		slices.Sort(decodedBy)
	}
	return &decoderSource{
		types:  &typeSource{structs: byName, scalars: parsed.scalars, named: parsed.named, decoders: parsed.decoders, encoders: parsed.encoders},
		paired: paired,
	}
}

// judge puts the decoder half of the kinds question to the client-go structs
// one output type is filled from: whether each struct reads every kind the
// answers carried at a key it declares without an error, which is what fails
// the handler before the converter runs. An output type no converter pairs is
// named, and so is a struct whose whole body its own method decodes, or which
// the parse did not find, since nothing about its fields says what it takes.
func (d *decoderSource) judge(check *OrbitCheck, output recordedOutput, pkg, name string) {
	named := shortPackage(pkg) + "." + name
	decodedBy := d.paired[[2]string{shortPackage(pkg), name}]
	if len(decodedBy) == 0 {
		check.Undecoded = append(check.Undecoded, named)
		return
	}
	calls := output.labels()
	for _, decoder := range decodedBy {
		check.Decoders = append(check.Decoders, named+" from "+clientGoQualifier+decoder)
		if !d.types.isStruct(decoder) || d.types.decoders[decoder] {
			check.DecoderKindsUnjudged = append(check.DecoderKindsUnjudged, OrbitField{
				Package: pkg, Output: name, Path: orbitrecord.Root, Type: clientGoQualifier + decoder,
				GoType: decoder, Kinds: bodyKinds(output.calls), Calls: calls,
			})
			continue
		}
		tree := d.types.walk(decoder)
		recorded, _ := recordedKeys(output.calls, tree.topLevel)
		judge := orbitJudgement{
			pkg: pkg, output: name, calls: calls, published: tree, recorded: recorded,
			types: d.types, decoding: true, qualifier: clientGoQualifier,
		}
		kinds := judge.mismatched()
		check.DecoderKindsCompared += kinds.compared
		check.DecoderMismatched = append(check.DecoderMismatched, kinds.found...)
		check.DecoderKindsUnjudged = append(check.DecoderKindsUnjudged, kinds.unjudged...)
	}
}

// bodyKinds are the kinds the calls answered with as a whole, sorted.
func bodyKinds(calls []orbitrecord.Call) []string {
	kinds := map[string]bool{}
	for _, call := range calls {
		for _, key := range call.Response.Keys {
			if key.Path == orbitrecord.Root {
				for _, kind := range key.Kinds {
					kinds[kind] = true
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(kinds))
}
