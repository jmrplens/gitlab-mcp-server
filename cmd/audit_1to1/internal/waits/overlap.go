// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"go/types"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Shared is one key a struct of this repository reads off a captured
// response (ADR-0021) that the client-go struct decoded from the same
// response models too.
type Shared struct {
	// Key is the key, dotted below each key both structs read as an object.
	Key string `json:"key"`
	// Owner is the client-go struct that models the key: the paired struct,
	// or one a field of it holds.
	Owner string `json:"owner"`
	// Field is the name of Owner's field that answers the key, which a
	// predicate on the key names.
	Field string `json:"field"`
	// Kind spells that field's type the way [FieldKind] spells one, and
	// Captured spells the captured field's type with its package's name.
	Kind     string `json:"kind"`
	Captured string `json:"captured"`
}

// Lack is one key a captured struct reads that the client-go struct at its
// depth does not model: the paired struct at the top, or one a field of it
// holds below a key both read as an object.
type Lack struct {
	// Key is the key, dotted as a [Shared] key is.
	Key string
	// Owner is the client-go struct at the key's depth, and Name the key as
	// Owner would be written under it, which a LacksKey on Owner names.
	Owner, Name string
}

// Comparison is what a captured struct reads beside the client-go struct it
// is paired with: the keys both model, and the keys Owner does not.
type Comparison struct {
	Shared []Shared
	Lacks  []Lack
}

// Compare compares captured, a struct a capture is decoded into, with
// paired, each list sorted by key. module is the import path every package
// of the repository's own starts with, which tells a struct of its own from
// one of the standard library or of a dependency; captured is a named
// struct's or a struct literal's. A key both read as an object (notes, lists,
// a pipeline, an access level) is no answer of its own: its keys are compared
// in turn, dotted below it. A key read as an object of client-go's own, or of
// any package not this repository's (a time), is a value read the way it is
// spelled. A client-go struct captured embeds is the half the SDK decodes, so
// only the keys beside it are compared, and a struct of the repository's own
// it embeds carries its keys at its depth, a shallower field of the same key
// shadowing it as it does in encoding/json.
func Compare(sdk SDK, module string, captured *types.Struct, paired *types.Named) Comparison {
	walk := sharedWalk{sdk: sdk, module: module, on: map[*types.Struct]bool{}}
	walk.collect(captured, paired, "", map[string]bool{})
	slices.SortFunc(walk.out.Shared, func(a, b Shared) int { return strings.Compare(a.Key, b.Key) })
	slices.SortFunc(walk.out.Lacks, func(a, b Lack) int { return strings.Compare(a.Key, b.Key) })
	return walk.out
}

// sharedWalk is one comparison of a captured struct with a client-go struct.
type sharedWalk struct {
	sdk SDK
	// module is the import path every package of the repository's own
	// starts with.
	module string
	// on holds the captured structs being compared further up, so a struct
	// that holds itself is compared once.
	on  map[*types.Struct]bool
	out Comparison
}

// collect appends what own reads beside sdkNamed, prefix spelling the keys
// above it. shadowed holds the keys a shallower field of the struct that
// embeds own decodes, which encoding/json reads there rather than here.
func (w *sharedWalk) collect(own *types.Struct, sdkNamed *types.Named, prefix string, shadowed map[string]bool) {
	if w.on[own] {
		return
	}
	w.on[own] = true
	defer delete(w.on, own)
	sdkStruct, _ := sdkNamed.Underlying().(*types.Struct)
	decoded := maps.Clone(shadowed)
	for i := range own.NumFields() {
		field := structField{v: own.Field(i), tag: reflect.StructTag(own.Tag(i))}
		if written, _ := field.encoding(EncoderJSON); !field.v.Embedded() && field.v.Exported() && !written.skipped {
			decoded[written.name] = true
		}
	}
	for i := range own.NumFields() {
		field := structField{v: own.Field(i), tag: reflect.StructTag(own.Tag(i))}
		if inner := w.ownStruct(field.v.Type()); field.v.Embedded() && inner != nil {
			w.collect(inner, sdkNamed, prefix, decoded)
		}
		written, _ := field.encoding(EncoderJSON)
		if field.v.Embedded() || !field.v.Exported() || written.skipped || shadowed[written.name] {
			continue
		}
		w.compareKey(field, written.name, sdkStruct, sdkNamed, prefix)
	}
}

// compareKey records what one key own reads comes to beside sdkNamed.
func (w *sharedWalk) compareKey(field structField, name string, sdkStruct *types.Struct, sdkNamed *types.Named, prefix string) {
	key := prefix + name
	modeled, found := findField(sdkStruct, func(f structField) bool {
		theirs, _ := f.encoding(EncoderJSON)
		return !theirs.skipped && theirs.name == name
	})
	if !found {
		w.out.Lacks = append(w.out.Lacks, Lack{Key: key, Owner: sdkNamed.Obj().Name(), Name: name})
		return
	}
	ownInner := w.ownStruct(element(field.v.Type()))
	sdkInner := w.sdk.clientGoStruct(element(modeled.v.Type()))
	if ownInner != nil && sdkInner != nil && containers(field.v.Type()) == containers(modeled.v.Type()) {
		w.collect(ownInner, sdkInner, key+".", map[string]bool{})
		return
	}
	w.out.Shared = append(w.out.Shared, Shared{
		Key:      key,
		Owner:    sdkNamed.Obj().Name(),
		Field:    modeled.v.Name(),
		Kind:     w.sdk.spell(modeled.v.Type()),
		Captured: types.TypeString(field.v.Type(), func(pkg *types.Package) string { return pkg.Name() }),
	})
}

// containers spells what holds the struct a value of type t is read into: a
// list for each slice or array and a map for each map, a pointer read
// through. Two keys hold the same object only when they hold it the same
// way, since a map's keys are what a struct's fields are on the other side.
func containers(t types.Type) string {
	switch typed := t.Underlying().(type) {
	case *types.Pointer:
		return containers(typed.Elem())
	case *types.Slice:
		return "[]" + containers(typed.Elem())
	case *types.Array:
		return "[]" + containers(typed.Elem())
	case *types.Map:
		return "map" + containers(typed.Elem())
	}
	return ""
}

// ownStruct returns the struct t is or points to when the repository
// declares it, or when it has no name, and nil for anything else.
func (w *sharedWalk) ownStruct(t types.Type) *types.Struct {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	if named, isNamed := t.(*types.Named); isNamed && !w.declares(named.Obj().Pkg()) {
		return nil
	}
	st, _ := t.Underlying().(*types.Struct)
	return st
}

// declares reports whether pkg is one of the repository's own packages. A
// type of the universe (error) has no package.
func (w *sharedWalk) declares(pkg *types.Package) bool {
	return pkg != nil && strings.HasPrefix(pkg.Path(), w.module+"/")
}
