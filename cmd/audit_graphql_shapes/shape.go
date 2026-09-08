package main

import (
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// scalarClass says how GitLab serializes a scalar, which is the only thing a
// decoder has to agree with.
type scalarClass int

const (
	// classString is a JSON string: String, ID, every global ID, and the
	// scalars GitLab renders as text, including BigInt, which is a string on
	// the wire because it may exceed what a 32-bit integer holds.
	classString scalarClass = iota
	// classInt is a JSON number with no fraction.
	classInt
	// classFloat is a JSON number that may carry a fraction, so an integer
	// kind would refuse some of the values GitLab sends.
	classFloat
	// classBool is a JSON boolean.
	classBool
	// classAny is arbitrary JSON, which only an interface, a map, a raw
	// message or a type that unmarshals itself can hold.
	classAny
)

// scalarClasses is how GitLab sends each scalar the pinned schema declares,
// from the scalar types section of its GraphQL reference. A scalar not in
// the table and not named as an ID is reported rather than guessed, so a
// scalar a future pin adds is classified on purpose.
//
// Duration is the one number among the named scalars: GitLab defines it as a
// floating point number of seconds. Upload never appears in a response and is
// listed so an input-only scalar is not mistaken for an unknown one.
var scalarClasses = map[string]scalarClass{ //nolint:gochecknoglobals // the serialization table this audit judges by
	"String":                       classString,
	"ID":                           classString,
	"GlobalID":                     classString,
	"BigInt":                       classString,
	"Color":                        classString,
	"Date":                         classString,
	"ISO8601Date":                  classString,
	"ISO8601DateTime":              classString,
	"Time":                         classString,
	"JsonString":                   classString,
	"UntrustedRegexp":              classString,
	"Upload":                       classString,
	"AiCatalogPinnedVersion":       classString,
	"GoogleCloudImage":             classString,
	"GoogleCloudMachineType":       classString,
	"GoogleCloudProject":           classString,
	"GoogleCloudRegion":            classString,
	"GoogleCloudZone":              classString,
	"Int":                          classInt,
	"Float":                        classFloat,
	"Duration":                     classFloat,
	"Boolean":                      classBool,
	"JSON":                         classAny,
	"CiInputsValue":                classAny,
	"PayloadAlertFieldPathSegment": classAny,
}

// idSuffix marks the global ID scalars, one per model, all strings.
const idSuffix = "ID"

// scalarClassOf classifies a scalar by name.
func scalarClassOf(name string) (scalarClass, bool) {
	if class, ok := scalarClasses[name]; ok {
		return class, true
	}
	if strings.HasSuffix(name, idSuffix) {
		return classString, true
	}
	return 0, false
}

// classKinds are the Go basic kinds each class decodes into.
//
// An integer scalar may land in a float, since every integer is a JSON number
// a float holds; a float may not land in an integer, since a fraction would
// fail to decode. Whether the value fits the width is the decoder's business
// at run time, not a shape. classAny is never checked against a basic kind,
// so its entry is empty.
var classKinds = [...]types.BasicInfo{ //nolint:gochecknoglobals // a table indexed by class
	classString: types.IsString,
	classInt:    types.IsInteger | types.IsFloat,
	classFloat:  types.IsFloat,
	classBool:   types.IsBoolean,
	classAny:    0,
}

// classDescriptions say what each class is on the wire, for a report line.
var classDescriptions = [...]string{ //nolint:gochecknoglobals // a table indexed by class
	classString: "sent as a JSON string",
	classInt:    "sent as a JSON integer",
	classFloat:  "sent as a JSON number that may carry a fraction",
	classBool:   "sent as a JSON boolean",
	classAny:    "sent as arbitrary JSON",
}

// holds reports whether a Go basic kind can decode what the class sends.
func (c scalarClass) holds(basic *types.Basic) bool {
	return basic.Info()&classKinds[c] != 0
}

// String says what the class is on the wire, for a report line.
func (c scalarClass) String() string {
	return classDescriptions[c]
}

// typenameType is the type of __typename, which every object carries and no
// schema declares as a field.
var typenameType = &ast.Type{NamedType: "String", NonNull: true} //nolint:gochecknoglobals // the one meta field's type

// finding is one disagreement between a document and its decoder, or one
// selection nothing reads.
type finding struct {
	pairing *pairing
	// Path is the response key the finding is about, from data down.
	Path string
	// Message says what disagrees.
	Message string
	// Fails says whether the finding fails the gate. A selection no Go field
	// reads is reported and does not: it is transfer, not truth.
	Fails bool
}

// goField is one field encoding/json would fill, with the name it answers to.
type goField struct {
	name string
	typ  types.Type
	// asString is the ",string" option, under which a number or boolean is
	// read out of a JSON string.
	asString bool
}

// judge walks one pairing.
type judge struct {
	schema   *ast.Schema
	document *ast.QueryDocument
	pairing  *pairing
	findings []finding
	// operation names the operation being walked, since a document may
	// define several and a sent finding says which one reached the object.
	operation string
	// sent are the fields the schema offers at an object this decoder reads
	// that the document never selects.
	sent []sentField
	// selected records what this pairing did select at each object, keyed the
	// way a finding is, so a sibling document of the same package cancels a
	// finding this one raised.
	selected map[string]bool
	// asked records the schema types this pairing has already been asked
	// about, which both bounds the recursion the real documents contain
	// (a security attribute's category holds security attributes) and keeps
	// one type from being reported once per position it is reached at.
	asked map[string]bool
	// coverage counts what became of every object position the walk entered,
	// which is what says how much of the walk this dimension covers.
	coverage sentCoverage
}

// judgePairing compares the document's selection set with the type it is
// decoded into and returns every disagreement, together with the fields the
// schema offers at the objects it reads that the document never selects.
//
// The response is client-go's whole body, so the struct is expected to carry a
// data field for the selection set; the top-level errors field, and anything
// else beside data, is not part of any selection and is left alone. Every
// operation the document defines is walked, since a document with several
// selects whichever GitLab is asked to run.
func judgePairing(schema *ast.Schema, document *ast.QueryDocument, p *pairing) pairingJudgement {
	j := &judge{schema: schema, document: document, pairing: p, asked: map[string]bool{}, selected: map[string]bool{}}
	body, ok := p.Response.Underlying().(*types.Struct)
	if !ok {
		j.fail("data", "the decode target is "+typeString(p.Response)+", not a struct with a data field, so nothing GitLab answers is kept")
		return pairingJudgement{findings: j.findings}
	}
	data := lookupField(jsonFields(body), "data")
	if data == nil {
		j.fail("data", "the decode target has no data field, so everything GitLab answers is dropped")
		return pairingJudgement{findings: j.findings}
	}
	for _, operation := range document.Operations {
		j.operation = operationName(operation)
		root := &ast.Type{NamedType: j.rootType(operation).Name, NonNull: true}
		j.judgeType(data.typ, root, operation.SelectionSet, "data", data.asString)
	}
	return pairingJudgement{findings: j.findings, sent: j.sent, selected: j.selected, coverage: j.coverage}
}

// pairingJudgement is everything one pairing answered: the disagreements and
// notes, the fields the schema offered that it did not select, the fields it
// did select, and what became of every position walked.
//
// The selections travel with the findings because they are the other half of
// one answer: a field this pairing did not select is a gap only if no sibling
// pairing of the same package selected it either, and that is decided once the
// run has walked them all.
type pairingJudgement struct {
	findings []finding
	sent     []sentField
	selected map[string]bool
	coverage sentCoverage
}

// operationName names the operation a finding was reached through: its kind,
// and the name when one was written.
func operationName(operation *ast.OperationDefinition) string {
	if operation.Name == "" {
		return string(operation.Operation)
	}
	return string(operation.Operation) + " " + operation.Name
}

// rootType is the object an operation selects from.
func (j *judge) rootType(operation *ast.OperationDefinition) *ast.Definition {
	switch operation.Operation {
	case ast.Mutation:
		return j.schema.Mutation
	case ast.Subscription:
		return j.schema.Subscription
	default:
		return j.schema.Query
	}
}

// judgeType compares one Go type with the GraphQL type it decodes, walking
// into lists and objects.
func (j *judge) judgeType(goType types.Type, gqlType *ast.Type, selections ast.SelectionSet, path string, asString bool) {
	goType, bound := j.concrete(goType)
	if !bound {
		j.note(path, "typed by a parameter no caller binds, so it is left unjudged")
		return
	}
	if unmarshalsItself(goType) {
		j.countSelfDecoding(gqlType)
		return
	}
	if gqlType.Elem != nil {
		element, ok := elementType(goType)
		if !ok {
			j.fail(path, fmt.Sprintf("%s is a list and is decoded into %s, which is not a slice", gqlType, typeString(goType)))
			return
		}
		j.judgeType(element, gqlType.Elem, selections, path+"[]", asString)
		return
	}
	definition := j.schema.Types[gqlType.NamedType]
	switch definition.Kind {
	case ast.Object, ast.Interface, ast.Union:
		j.judgeObject(goType, gqlType, selections, path)
	case ast.Enum:
		j.expectClass(goType, gqlType, path, classString, asString)
	default:
		class, ok := scalarClassOf(definition.Name)
		if !ok {
			j.fail(path, definition.Name+" is a scalar this audit has no serialization for; add it to scalarClasses with how GitLab sends it")
			return
		}
		j.expectClass(goType, gqlType, path, class, asString)
	}
}

// concrete reads through pointers, aliases and bound type parameters to the
// type a value will actually have, reporting false for a parameter nothing
// binds.
func (j *judge) concrete(goType types.Type) (types.Type, bool) {
	for {
		switch t := goType.(type) {
		case *types.Pointer:
			goType = t.Elem()
		case *types.Alias:
			goType = types.Unalias(t)
		case *types.TypeParam:
			bound, ok := j.pairing.TypeArgs[t]
			if !ok {
				return nil, false
			}
			goType = bound
		default:
			return goType, true
		}
	}
}

// judgeObject compares a Go type with an object's selection set: a struct
// field by field, or a map keyed by string whose every value is one element
// type.
func (j *judge) judgeObject(goType types.Type, gqlType *ast.Type, selections ast.SelectionSet, path string) {
	switch under := goType.Underlying().(type) {
	case *types.Struct:
		j.judgeStruct(under, gqlType, goType, selections, path)
	case *types.Map:
		if key, ok := under.Key().Underlying().(*types.Basic); ok && key.Info()&types.IsString != 0 {
			j.countMap(gqlType)
			for _, field := range j.merged(selections) {
				j.judgeType(under.Elem(), fieldType(field), field.SelectionSet, path+"."+responseKey(field), false)
			}
			return
		}
		j.fail(path, fmt.Sprintf("%s is an object and is decoded into %s, whose keys are not strings", gqlType, typeString(goType)))
	default:
		j.fail(path, fmt.Sprintf("%s is an object and is decoded into %s", gqlType, typeString(goType)))
	}
}

// judgeStruct matches every selected field to the Go field encoding/json
// would fill, reports the Go fields nothing selects, and asks the schema what
// it offers here that nothing selected.
//
// The three questions belong in one walk because they are three legs of one
// comparison, and this is the only place that holds all of what each needs:
// the schema type, the selection set the validator resolved, and the struct
// the answer lands in.
func (j *judge) judgeStruct(body *types.Struct, gqlType *ast.Type, goType types.Type, selections ast.SelectionSet, path string) {
	fields := jsonFields(body)
	read := make(map[string]bool, len(fields))
	selected := make(map[string]bool, len(selections))
	// Kept apart from selected because the two answer different questions.
	// selected suppresses a finding at THIS position, where asking about a
	// field the document just named would be absurd. decoded is the evidence
	// that cancels a SIBLING document's finding, and a value this document
	// asks GitLab for and then throws away is no evidence that the package
	// surfaces it: counting it would let one document's dead selection
	// silence the whole package's gap, which is the same union-for-an-
	// intersection error one level down.
	decoded := make(map[string]bool, len(selections))
	readsLeaf := false
	for _, field := range j.merged(selections) {
		key := responseKey(field)
		selected[field.Name] = true
		match := lookupField(fields, key)
		if match == nil {
			j.note(path+"."+key, "selected and never decoded")
			j.countUndecoded(field)
			continue
		}
		decoded[field.Name] = true
		read[match.name] = true
		// __typename is a meta field no schema declares, so decoding it says
		// nothing about whether this object is read or merely traversed.
		if field.Name != "__typename" && j.isLeafType(fieldType(field)) {
			readsLeaf = true
		}
		j.judgeType(match.typ, fieldType(field), field.SelectionSet, path+"."+key, match.asString)
	}
	for _, field := range fields {
		if !read[field.name] {
			j.fail(path+"."+field.name, "decoded from a field the document never selects, so it is always empty")
		}
	}
	j.askSchema(gqlType, fields, selections, selected, decoded, readsLeaf, path, goType)
}

// merged flattens a selection set into one field per response key, with
// inline fragments and fragment spreads expanded and a field selected twice
// merged the way GraphQL merges it, so a struct is judged against everything
// the key can carry.
func (j *judge) merged(selections ast.SelectionSet) []*ast.Field {
	var (
		order  []*ast.Field
		byName = map[string]*ast.Field{}
	)
	for _, field := range j.expand(selections) {
		key := responseKey(field)
		if existing, ok := byName[key]; ok {
			existing.SelectionSet = append(existing.SelectionSet, field.SelectionSet...)
			continue
		}
		copied := *field
		copied.SelectionSet = append(ast.SelectionSet(nil), field.SelectionSet...)
		byName[key] = &copied
		order = append(order, &copied)
	}
	return order
}

// expand lists the fields a selection set carries, through inline fragments
// and fragment spreads.
func (j *judge) expand(selections ast.SelectionSet) []*ast.Field {
	var fields []*ast.Field
	for _, selection := range selections {
		switch s := selection.(type) {
		case *ast.Field:
			fields = append(fields, s)
		case *ast.InlineFragment:
			fields = append(fields, j.expand(s.SelectionSet)...)
		case *ast.FragmentSpread:
			fields = append(fields, j.expand(s.Definition.SelectionSet)...)
		}
	}
	return fields
}

// expectClass checks that a Go type can hold a scalar of the class.
func (j *judge) expectClass(goType types.Type, gqlType *ast.Type, path string, class scalarClass, asString bool) {
	if class == classAny {
		return
	}
	if class == classString && unmarshalsText(goType) {
		return
	}
	basic, ok := goType.Underlying().(*types.Basic)
	if ok && asString && class == classString && basic.Info()&(types.IsNumeric|types.IsBoolean) != 0 {
		// The ",string" option reads a number or a boolean out of a JSON
		// string, which is how a BigInt lands in an int64 on purpose.
		return
	}
	if ok && class.holds(basic) {
		return
	}
	j.fail(path, fmt.Sprintf("%s is %s and is decoded into %s", gqlType, class, typeString(goType)))
}

// fail records a disagreement.
func (j *judge) fail(path, message string) {
	j.findings = append(j.findings, finding{pairing: j.pairing, Path: path, Message: message, Fails: true})
}

// note records something worth reading that does not fail the gate.
func (j *judge) note(path, message string) {
	j.findings = append(j.findings, finding{pairing: j.pairing, Path: path, Message: message})
}

// responseKey is the key a field answers under. The parser fills Alias for
// every field, with the field's own name when no alias was written, so the
// alias is the key whether or not one was written.
func responseKey(field *ast.Field) string {
	return field.Alias
}

// fieldType is the type a selected field carries, which the validator wrote
// on the field for every field but __typename.
func fieldType(field *ast.Field) *ast.Type {
	if field.Name == "__typename" {
		return typenameType
	}
	return field.Definition.Type
}

// elementType returns what a slice or array holds.
func elementType(goType types.Type) (types.Type, bool) {
	switch t := goType.Underlying().(type) {
	case *types.Slice:
		return t.Elem(), true
	case *types.Array:
		return t.Elem(), true
	default:
		return nil, false
	}
}

// unmarshalsItself reports whether a type decides its own decoding: the empty
// interface, which holds anything, or a type with an UnmarshalJSON method,
// which is trusted to know what it reads.
func unmarshalsItself(goType types.Type) bool {
	if iface, ok := goType.Underlying().(*types.Interface); ok {
		return iface.Empty()
	}
	return hasMethod(goType, "UnmarshalJSON")
}

// unmarshalsText reports whether a type reads itself out of a JSON string,
// which is what encoding/json does with an UnmarshalText method.
func unmarshalsText(goType types.Type) bool {
	return hasMethod(goType, "UnmarshalText")
}

// hasMethod reports whether the type or its pointer has the named method.
func hasMethod(goType types.Type, name string) bool {
	obj, _, _ := types.LookupFieldOrMethod(goType, true, nil, name)
	_, ok := obj.(*types.Func)
	return ok
}

// jsonFields lists the fields encoding/json fills on a struct, under the names
// it matches them by: the json tag's name, or the field's own; a "-" tag and
// an unexported field are skipped; an embedded struct without a tag has its
// fields promoted after the struct's own, so a shallower field wins a name.
func jsonFields(body *types.Struct) []goField {
	var (
		fields   []goField
		promoted []*types.Struct
	)
	for i := range body.NumFields() {
		field := body.Field(i)
		name, options, _ := strings.Cut(reflect.StructTag(body.Tag(i)).Get("json"), ",")
		if name == "-" {
			continue
		}
		if field.Embedded() && name == "" {
			if inner, ok := pointee(field.Type()).Underlying().(*types.Struct); ok {
				promoted = append(promoted, inner)
				continue
			}
		}
		if !field.Exported() {
			continue
		}
		if name == "" {
			name = field.Name()
		}
		fields = appendField(fields, goField{name: name, typ: field.Type(), asString: hasOption(options, "string")})
	}
	for _, inner := range promoted {
		for _, field := range jsonFields(inner) {
			fields = appendField(fields, field)
		}
	}
	return fields
}

// appendField adds a field unless one already answers to its name.
func appendField(fields []goField, field goField) []goField {
	for _, existing := range fields {
		if existing.name == field.name {
			return fields
		}
	}
	return append(fields, field)
}

// hasOption reports whether a json tag's option list carries the option.
func hasOption(options, option string) bool {
	return slices.Contains(strings.Split(options, ","), option)
}

// lookupField finds the field encoding/json would fill for a key: an exact
// match on its name, or failing that a case-insensitive one.
func lookupField(fields []goField, key string) *goField {
	for i := range fields {
		if fields[i].name == key {
			return &fields[i]
		}
	}
	for i := range fields {
		if strings.EqualFold(fields[i].name, key) {
			return &fields[i]
		}
	}
	return nil
}

// pointee reads through pointers.
func pointee(goType types.Type) types.Type {
	for {
		pointer, ok := goType.(*types.Pointer)
		if !ok {
			return goType
		}
		goType = pointer.Elem()
	}
}

// typeString renders a type the way a reader would write it, with package
// names rather than import paths.
func typeString(goType types.Type) string {
	return types.TypeString(goType, func(pkg *types.Package) string { return pkg.Name() })
}
