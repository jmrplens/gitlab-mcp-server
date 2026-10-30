// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"errors"
	"fmt"
	"go/types"
	"reflect"
	"regexp"
	"strings"
)

// The two encoders client-go writes an option struct with: encoding/json for
// the body of a PATCH, POST or PUT built through NewRequest, and
// go-querystring, which reads the url tag, for the query of every other verb
// and for every field of an UploadRequest, which it sends as multipart form
// fields.
const (
	EncoderJSON  = "json"
	EncoderQuery = "url"
)

// SDK is what a predicate is evaluated against.
type SDK struct {
	// Types is the pinned client-go root package, as the tool packages
	// compile against it.
	Types *types.Package
	// Routes is every REST route a client-go method sends, spelled
	// "VERB /path" with each identifier segment as ":", the way
	// cmd/internal/sdkroutes spells them.
	Routes map[string]bool
	// Unresolved names every request a client-go method sends whose path
	// the reading did not fold to a route, the way cmd/internal/sdkroutes
	// reports one. Any of them could send any route, so while one is listed
	// no route can be told unsent.
	Unresolved []string
}

// Predicate is one thing that is true of the pinned client-go while a
// workaround is needed.
type Predicate interface {
	// Holds reports whether the predicate is true of sdk. The error is for a
	// predicate that names a type, a field or a method sdk does not declare,
	// which is a declaration to correct rather than a verdict.
	Holds(sdk SDK) (bool, error)
	// String spells the predicate the way a report prints it.
	String() string
	// Subject names what the predicate is about: the struct a field or key
	// predicate names, the method ("Service.Method") a method predicate
	// names, the route a route predicate names. The register row an entry
	// waits on is held to mentioning what its predicates are about.
	Subject() string
}

// FieldPredicate is a predicate about one field of a client-go struct. It
// answers the JSON key the field is written under, which is how the keys a
// body edit writes are tied to the predicates that say why it writes them.
type FieldPredicate interface {
	Predicate
	// Target names the struct and the field.
	Target() (typeName, field string)
}

// Verdict is what one predicate came to on one run.
type Verdict struct {
	Predicate string `json:"predicate"`
	Holds     bool   `json:"holds"`
	// Error is set when the predicate names something the SDK lacks.
	Error string `json:"error,omitempty"`
}

// Evaluate evaluates p against sdk. A predicate that names something the SDK
// lacks reports that it does not hold, beside the error.
func Evaluate(p Predicate, sdk SDK) Verdict {
	holds, err := p.Holds(sdk)
	verdict := Verdict{Predicate: p.String(), Holds: holds}
	if err != nil {
		verdict.Error = err.Error()
	}
	return verdict
}

// BodyKey returns the JSON key the field a predicate names is written under,
// and false when the predicate names no field, a field the SDK lacks, or a
// field encoding/json leaves out.
func BodyKey(p Predicate, sdk SDK) (string, bool) {
	target, isField := p.(FieldPredicate)
	if !isField {
		return "", false
	}
	typeName, name := target.Target()
	field, err := sdk.field(typeName, name)
	if err != nil {
		return "", false
	}
	encoded, _ := field.encoding(EncoderJSON)
	if encoded.skipped {
		return "", false
	}
	return encoded.name, true
}

// WritesAlways holds while the field's tag for Encoder carries neither
// omitempty nor omitzero, so the encoder writes it on every call whatever
// the caller set. Encoder is the one the request uses: the verb's with
// NewRequest, EncoderQuery with UploadRequest.
type WritesAlways struct {
	Type, Field, Encoder string
}

// String spells the predicate.
func (p WritesAlways) String() string {
	return "WritesAlways{" + p.Type + "." + p.Field + ", " + p.Encoder + "}"
}

// Target names the struct and the field.
func (p WritesAlways) Target() (typeName, field string) { return p.Type, p.Field }

// Subject names the struct.
func (p WritesAlways) Subject() string { return p.Type }

// Holds evaluates the predicate.
func (p WritesAlways) Holds(sdk SDK) (bool, error) {
	field, err := sdk.field(p.Type, p.Field)
	if err != nil {
		return false, err
	}
	encoded, err := field.encoding(p.Encoder)
	if err != nil {
		return false, err
	}
	return !encoded.skipped && !encoded.omits, nil
}

// LacksKey holds while no field of the struct, top level or embedded, is
// written or read under Key by either encoder. A dotted Key names a key of a
// nested object, each segment below a field whose type is or holds a struct:
// "variables.raw" is the raw of each element of a schedule's variables.
type LacksKey struct {
	Type, Key string
}

// String spells the predicate.
func (p LacksKey) String() string { return "LacksKey{" + p.Type + ", " + p.Key + "}" }

// Subject names the struct.
func (p LacksKey) Subject() string { return p.Type }

// Holds evaluates the predicate. A dotted key below a key no field is
// written under is refused rather than held: a struct that lacked the
// parent would be written LacksKey on the parent, so the parent is far
// likelier misspelled, and a misspelled key would hold forever.
func (p LacksKey) Holds(sdk SDK) (bool, error) {
	st, err := sdk.structType(p.Type)
	if err != nil {
		return false, err
	}
	segments := strings.Split(p.Key, ".")
	last := len(segments) - 1
	for i, segment := range segments {
		field, found := findField(st, func(f structField) bool { return f.answersKey(segment) })
		if !found && i == last {
			return true, nil
		}
		if !found {
			return false, fmt.Errorf("%s: no field is written under %s to look %s up in", p.Type, strings.Join(segments[:i+1], "."), strings.Join(segments[i+1:], "."))
		}
		if i == last {
			break
		}
		inner, isStruct := structOf(element(field.v.Type()))
		if !isStruct {
			return false, fmt.Errorf("%s: %s holds no struct to look %s up in", p.Type, strings.Join(segments[:i+1], "."), strings.Join(segments[i+1:], "."))
		}
		st = inner
	}
	return false, nil
}

// FieldKind holds while the field's type is spelled Kind, with client-go's
// own package left unqualified ("*int64", "*ISOTime", "[]string"). A field
// whose type changes is one whose workaround has to be looked at again.
type FieldKind struct {
	Type, Field, Kind string
}

// String spells the predicate.
func (p FieldKind) String() string {
	return "FieldKind{" + p.Type + "." + p.Field + ", " + p.Kind + "}"
}

// Target names the struct and the field.
func (p FieldKind) Target() (typeName, field string) { return p.Type, p.Field }

// Subject names the struct.
func (p FieldKind) Subject() string { return p.Type }

// Holds evaluates the predicate.
func (p FieldKind) Holds(sdk SDK) (bool, error) {
	field, err := sdk.field(p.Type, p.Field)
	if err != nil {
		return false, err
	}
	return sdk.spell(field.v.Type()) == p.Kind, nil
}

// routeSpelling is the shape a route predicate is written in: an uppercase
// verb, one space and a path from the API root.
var routeSpelling = regexp.MustCompile(`^(GET|HEAD|POST|PUT|PATCH|DELETE) /\S*$`)

// namedIdentifier matches a path segment that names its identifier (":id"),
// which [SDK.Routes] never holds: it writes every identifier as a bare ":".
var namedIdentifier = regexp.MustCompile(`/:[^/]`)

// NoMethodFor holds while no client-go method sends Route, spelled the way
// [SDK.Routes] spells it.
type NoMethodFor struct {
	Route string
}

// String spells the predicate.
func (p NoMethodFor) String() string { return "NoMethodFor{" + p.Route + "}" }

// Subject names the route.
func (p NoMethodFor) Subject() string { return p.Route }

// Holds evaluates the predicate. It holds only because client-go sends
// nothing there, never for a reason of its own: a route not spelled as one,
// or naming an identifier the SDK's routes write bare, can never be sent,
// and an SDK none of whose routes was read sends nothing at all, so the
// predicate would hold forever. Each is refused, and so is a route no route
// read sends while a request whose path was not read could send it.
func (p NoMethodFor) Holds(sdk SDK) (bool, error) {
	if !routeSpelling.MatchString(p.Route) || namedIdentifier.MatchString(p.Route) {
		return false, fmt.Errorf("%q is not spelled \"VERB /path\" with each identifier as \":\"", p.Route)
	}
	if len(sdk.Routes) == 0 {
		return false, errors.New("no route of the pinned client-go was read, so none can be told unsent")
	}
	if sdk.Routes[p.Route] {
		return false, nil
	}
	if len(sdk.Unresolved) > 0 {
		return false, fmt.Errorf("the pinned client-go sends requests whose path was not read, any of which could be %s, so it cannot be told unsent: %s", p.Route, strings.Join(sdk.Unresolved, "; "))
	}
	return true, nil
}

// TakesNoOptions holds while the method ("Service.Method") takes no
// client-go struct, so nothing but a request option can carry a parameter
// the route declares.
type TakesNoOptions struct {
	Method string
}

// String spells the predicate.
func (p TakesNoOptions) String() string { return "TakesNoOptions{" + p.Method + "}" }

// Subject names the method.
func (p TakesNoOptions) Subject() string { return p.Method }

// Holds evaluates the predicate.
func (p TakesNoOptions) Holds(sdk SDK) (bool, error) {
	signature, err := sdk.method(p.Method)
	if err != nil {
		return false, err
	}
	for param := range signature.Params().Variables() {
		if sdk.isClientGoStruct(param.Type()) {
			return false, nil
		}
	}
	return true, nil
}

// DecodesInto holds while the method ("Service.Method") answers with Type,
// spelled as [FieldKind] spells a type: what its first result is.
type DecodesInto struct {
	Method, Type string
}

// String spells the predicate.
func (p DecodesInto) String() string { return "DecodesInto{" + p.Method + ", " + p.Type + "}" }

// Subject names the method.
func (p DecodesInto) Subject() string { return p.Method }

// Holds evaluates the predicate.
func (p DecodesInto) Holds(sdk SDK) (bool, error) {
	signature, err := sdk.method(p.Method)
	if err != nil {
		return false, err
	}
	if signature.Results().Len() == 0 {
		return false, errors.New(p.Method + " answers nothing")
	}
	return sdk.spell(signature.Results().At(0).Type()) == p.Type, nil
}

// TagName holds while Encoder writes the field under a name other than Want,
// or not at all: a field tagged only for one encoder is written by the other
// under its Go name.
type TagName struct {
	Type, Field, Encoder, Want string
}

// String spells the predicate.
func (p TagName) String() string {
	return "TagName{" + p.Type + "." + p.Field + ", " + p.Encoder + " not " + p.Want + "}"
}

// Target names the struct and the field.
func (p TagName) Target() (typeName, field string) { return p.Type, p.Field }

// Subject names the struct.
func (p TagName) Subject() string { return p.Type }

// Holds evaluates the predicate.
func (p TagName) Holds(sdk SDK) (bool, error) {
	field, err := sdk.field(p.Type, p.Field)
	if err != nil {
		return false, err
	}
	encoded, err := field.encoding(p.Encoder)
	if err != nil {
		return false, err
	}
	return encoded.skipped || encoded.name != p.Want, nil
}

// NotNullable holds while the field cannot send JSON null: a pointer, slice,
// map or interface tagged omitempty or omitzero, which encoding/json leaves
// out when nil, or a value that is never nil, and not client-go's Nullable,
// which is how a struct asks for null. It is the reason for every null a
// body edit writes.
type NotNullable struct {
	Type, Field string
}

// String spells the predicate.
func (p NotNullable) String() string { return "NotNullable{" + p.Type + "." + p.Field + "}" }

// Target names the struct and the field.
func (p NotNullable) Target() (typeName, field string) { return p.Type, p.Field }

// Subject names the struct.
func (p NotNullable) Subject() string { return p.Type }

// Holds evaluates the predicate.
func (p NotNullable) Holds(sdk SDK) (bool, error) {
	field, err := sdk.field(p.Type, p.Field)
	if err != nil {
		return false, err
	}
	encoded, _ := field.encoding(EncoderJSON)
	if encoded.skipped {
		return true, nil
	}
	if sdk.isNullable(field.v.Type()) {
		return false, nil
	}
	return !nilable(field.v.Type()) || encoded.omits, nil
}

// NoEmptyList holds while the field cannot send an empty JSON list: a slice
// tagged omitempty, which encoding/json leaves out when empty. A pointer to
// a slice can send one, through a pointer to an empty slice. It is the
// reason for every [] a body edit writes.
type NoEmptyList struct {
	Type, Field string
}

// String spells the predicate.
func (p NoEmptyList) String() string { return "NoEmptyList{" + p.Type + "." + p.Field + "}" }

// Target names the struct and the field.
func (p NoEmptyList) Target() (typeName, field string) { return p.Type, p.Field }

// Subject names the struct.
func (p NoEmptyList) Subject() string { return p.Type }

// Holds evaluates the predicate.
func (p NoEmptyList) Holds(sdk SDK) (bool, error) {
	field, err := sdk.field(p.Type, p.Field)
	if err != nil {
		return false, err
	}
	t := field.v.Type()
	pointer, isPointer := t.Underlying().(*types.Pointer)
	if !isSliceType(t) && (!isPointer || !isSliceType(pointer.Elem())) {
		return false, fmt.Errorf("%s.%s is not a list", p.Type, p.Field)
	}
	encoded, _ := field.encoding(EncoderJSON)
	return encoded.skipped || (isSliceType(t) && encoded.omits), nil
}

// structField is one field of a client-go struct with its tag.
type structField struct {
	v   *types.Var
	tag reflect.StructTag
}

// encoded is how one encoder writes a field.
type encoded struct {
	// name is the key it is written under: the tag's name, or the field's
	// Go name when the tag gives none, which is what encoding/json and
	// go-querystring both fall back to.
	name string
	// omits is true when the tag carries omitempty or omitzero.
	omits bool
	// skipped is true when the tag is "-", which neither encoder writes.
	skipped bool
}

// encoding reads how encoder writes the field.
func (f structField) encoding(encoder string) (encoded, error) {
	if encoder != EncoderJSON && encoder != EncoderQuery {
		return encoded{}, fmt.Errorf("encoder %q is neither %s nor %s", encoder, EncoderJSON, EncoderQuery)
	}
	raw := f.tag.Get(encoder)
	if raw == "-" {
		return encoded{name: "-", skipped: true}, nil
	}
	name, options, _ := strings.Cut(raw, ",")
	if name == "" {
		name = f.v.Name()
	}
	out := encoded{name: name}
	for option := range strings.SplitSeq(options, ",") {
		out.omits = out.omits || option == "omitempty" || option == "omitzero"
	}
	return out, nil
}

// answersKey reports whether either encoder writes the field under key.
func (f structField) answersKey(key string) bool {
	for _, encoder := range []string{EncoderJSON, EncoderQuery} {
		if written, _ := f.encoding(encoder); !written.skipped && written.name == key {
			return true
		}
	}
	return false
}

// findField returns the first field of st match accepts: st's own fields
// first, then, depth first, the fields of each struct st embeds, which is the
// order encoding/json promotes them in. An embedded field is never matched
// itself, only through its fields.
func findField(st *types.Struct, match func(structField) bool) (structField, bool) {
	for i := range st.NumFields() {
		field := structField{v: st.Field(i), tag: reflect.StructTag(st.Tag(i))}
		if !field.v.Embedded() && match(field) {
			return field, true
		}
	}
	for field := range st.Fields() {
		if !field.Embedded() {
			continue
		}
		if inner, isStruct := structOf(field.Type()); isStruct {
			if found, ok := findField(inner, match); ok {
				return found, true
			}
		}
	}
	return structField{}, false
}

// structType returns the struct client-go declares under name, an alias of
// one refused, since a predicate names the struct the way client-go does.
func (s SDK) structType(name string) (*types.Struct, error) {
	obj, _ := s.Types.Scope().Lookup(name).(*types.TypeName)
	if obj == nil {
		return nil, fmt.Errorf("client-go declares no type %s", name)
	}
	st, isStruct := obj.Type().Underlying().(*types.Struct)
	if _, isNamed := obj.Type().(*types.Named); !isStruct || !isNamed {
		return nil, fmt.Errorf("client-go's %s is not a struct", name)
	}
	return st, nil
}

// field returns the field of the client-go struct typeName called name, top
// level or promoted from an embedded struct.
func (s SDK) field(typeName, name string) (structField, error) {
	st, err := s.structType(typeName)
	if err != nil {
		return structField{}, err
	}
	field, found := findField(st, func(f structField) bool { return f.v.Name() == name })
	if !found {
		return structField{}, fmt.Errorf("client-go's %s has no field %s", typeName, name)
	}
	return field, nil
}

// DeclaresMethod returns an error unless client-go declares the service
// method key names ("Service.Method"), which is what a method an entry
// replaces is held to.
func (s SDK) DeclaresMethod(key string) error {
	_, err := s.method(key)
	return err
}

// method returns the signature of a client-go service method, keyed
// "Service.Method" as a handler's call names it.
func (s SDK) method(key string) (*types.Signature, error) {
	service, name, found := strings.Cut(key, ".")
	if !found {
		return nil, fmt.Errorf("%q is not spelled \"Service.Method\"", key)
	}
	obj := s.Types.Scope().Lookup(service + "ServiceInterface")
	if obj == nil {
		return nil, fmt.Errorf("client-go declares no service %s", service)
	}
	iface, isInterface := obj.Type().Underlying().(*types.Interface)
	if !isInterface {
		return nil, fmt.Errorf("client-go's %sServiceInterface is not an interface", service)
	}
	for method := range iface.Methods() {
		if method.Name() == name {
			signature, _ := method.Type().(*types.Signature)
			return signature, nil
		}
	}
	return nil, fmt.Errorf("client-go's %s has no method %s", service, name)
}

// spell spells a type with client-go's own package unqualified. Every type
// a predicate reaches is reached through client-go's own scope, so the
// package is told by identity.
func (s SDK) spell(t types.Type) string {
	return types.TypeString(t, func(pkg *types.Package) string {
		if pkg == s.Types {
			return ""
		}
		return pkg.Name()
	})
}

// isClientGoStruct reports whether t is, or points to, a struct client-go
// declares.
func (s SDK) isClientGoStruct(t types.Type) bool { return s.clientGoStruct(t) != nil }

// nullableName is the generic type client-go sends an explicit null through.
const nullableName = "Nullable"

// isNullable reports whether t is client-go's Nullable.
func (s SDK) isNullable(t types.Type) bool {
	named, isNamed := t.(*types.Named)
	if !isNamed {
		return false
	}
	origin := named.Origin().Obj()
	return origin.Name() == nullableName && origin.Pkg() == s.Types
}

// nilable reports whether a value of t can be nil, so that encoding/json
// writes it as null when nothing leaves it out. A function or a channel can
// be nil too, but encoding/json refuses to encode either at all.
func nilable(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface:
		return true
	}
	return false
}

// isSliceType reports whether t itself is a slice, as opposed to a pointer to
// one.
func isSliceType(t types.Type) bool {
	_, isSlice := t.Underlying().(*types.Slice)
	return isSlice
}

// element returns what a field of type t holds one of: the type a pointer
// points to, the element of a slice, an array or a map, or t itself.
func element(t types.Type) types.Type {
	switch typed := t.Underlying().(type) {
	case *types.Pointer:
		return element(typed.Elem())
	case *types.Slice:
		return element(typed.Elem())
	case *types.Array:
		return element(typed.Elem())
	case *types.Map:
		return element(typed.Elem())
	}
	return t
}

// structOf returns the struct t is or points to.
func structOf(t types.Type) (*types.Struct, bool) {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	st, isStruct := t.Underlying().(*types.Struct)
	return st, isStruct
}
