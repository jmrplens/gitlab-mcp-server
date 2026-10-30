// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"go/types"
	"iter"
	"maps"
	"reflect"
	"slices"
)

// Superset is a struct of a tool package's own that a raw request decodes
// its answer into, or sends as its options, embedding the client-go struct
// the method it stands in for would use, held to the keys it reads or
// writes beside that struct. Every such key is a reason the request cannot
// go back to client-go's method, so the entry naming the request's site has
// to carry a predicate for each, and a key carried for no reason is a
// finding.
type Superset struct {
	// Type is the struct, spelled with its package's name.
	Type string `json:"type"`
	// Root is the client-go struct it embeds, which every predicate it
	// needs names.
	Root string `json:"root"`
	// Keys are the keys it carries beside Root, dotted below a field that
	// holds a struct of the package's own in place of one of client-go's.
	// A predicate on Root that names a key outside them names a reason the
	// request does not have.
	Keys []string `json:"keys"`
	// Needs are the predicates, spelled as a report prints them, that say
	// why each key is carried: a LacksKey for a key Root does not model and
	// a FieldKind for a top-level key Root models with another type. A key
	// carried the way Root already carries it, or a nested key carried with
	// another type, is spelled as what no predicate answers.
	Needs []string `json:"needs"`
}

// SupersetOf returns the superset a raw request's value of type t is, the
// type a pointer, a slice, an array or a map holds one of looked through,
// and false when t holds no struct that embeds one of client-go's:
// client-go's own struct, a mirror of one that embeds nothing, whose
// pairing only its entry's predicates name, or no struct at all.
func SupersetOf(sdk SDK, t types.Type) (Superset, bool) {
	named, isNamed := element(t).(*types.Named)
	if !isNamed || named.Obj().Pkg() == sdk.Types {
		return Superset{}, false
	}
	local, _ := named.Underlying().(*types.Struct)
	root := sdk.embeddedClientGo(local)
	if root == nil {
		return Superset{}, false
	}
	needs := map[string]string{}
	sdk.collectNeeds(local, root, root.Obj().Name(), "", needs)
	return Superset{
		Type:  sdk.spell(named),
		Root:  root.Obj().Name(),
		Keys:  slices.Sorted(maps.Keys(needs)),
		Needs: slices.Sorted(maps.Values(needs)),
	}, true
}

// embeddedClientGo returns the first client-go struct st embeds, through a
// pointer or not, and nil when st embeds none or is no struct. One st
// embeds through a struct of the package's own it embeds counts too, as its
// keys are st's own in the JSON: the shallowest is first.
func (s SDK) embeddedClientGo(st *types.Struct) *types.Named {
	for named := range s.embeddedClientGoStructs(st) {
		return named
	}
	return nil
}

// embeddedClientGoStructs yields every client-go struct st embeds, through a
// pointer or not, directly or through the structs of the package's own it
// embeds, the shallowest first. Each struct of the package's own is read
// once, so one that embeds itself through a pointer ends the walk.
func (s SDK) embeddedClientGoStructs(st *types.Struct) iter.Seq[*types.Named] {
	return func(yield func(*types.Named) bool) {
		seen := map[*types.Struct]bool{}
		for level := []*types.Struct{st}; len(level) > 0; {
			var found []*types.Named
			found, level = s.embeddedLevel(level, seen)
			for _, named := range found {
				if !yield(named) {
					return
				}
			}
		}
	}
}

// embeddedLevel returns the client-go structs the structs of level embed, in
// field order, and the structs of the package's own they embed, the level
// below. A struct seen records is not read again, and is recorded once read.
func (s SDK) embeddedLevel(level []*types.Struct, seen map[*types.Struct]bool) (found []*types.Named, next []*types.Struct) {
	for _, current := range level {
		if current == nil || seen[current] {
			continue
		}
		seen[current] = true
		for field := range current.Fields() {
			if !field.Embedded() {
				continue
			}
			if named := s.clientGoStruct(field.Type()); named != nil {
				found = append(found, named)
			} else if inner := s.localStruct(field.Type()); inner != nil {
				next = append(next, inner)
			}
		}
	}
	return found, next
}

// collectNeeds records in needs, by key, what each key local carries beside
// sdkNamed needs, prefix spelling the keys above it. A struct of the
// package's own that local embeds carries its keys at local's depth, and
// one of client-go's is the half client-go already carries.
func (s SDK) collectNeeds(local *types.Struct, sdkNamed *types.Named, root, prefix string, needs map[string]string) {
	s.collectEmbeddedNeeds(local, sdkNamed, root, prefix, needs, map[*types.Struct]bool{})
}

// collectEmbeddedNeeds is collectNeeds for local and the structs of the
// package's own it embeds, walked records each of them already read at this
// depth, so one that embeds itself through a pointer is read once.
func (s SDK) collectEmbeddedNeeds(local *types.Struct, sdkNamed *types.Named, root, prefix string, needs map[string]string, walked map[*types.Struct]bool) {
	if walked[local] {
		return
	}
	walked[local] = true
	sdkStruct, _ := sdkNamed.Underlying().(*types.Struct)
	embeds := s.embeds(local, sdkNamed)
	for i := range local.NumFields() {
		field := structField{v: local.Field(i), tag: reflect.StructTag(local.Tag(i))}
		if inner := s.localStruct(field.v.Type()); field.v.Embedded() && inner != nil {
			s.collectEmbeddedNeeds(inner, sdkNamed, root, prefix, needs, walked)
		}
		written, _ := field.encoding(EncoderJSON)
		if field.v.Embedded() || !field.v.Exported() || written.skipped {
			continue
		}
		s.addNeed(field, written.name, sdkStruct, embeds, root, prefix, needs)
	}
}

// addNeed records what one key local carries needs. A key carried with
// client-go's own type below a struct that does not embed client-go's,
// which has to repeat it, needs nothing.
func (s SDK) addNeed(field structField, name string, sdkStruct *types.Struct, embeds bool, root, prefix string, needs map[string]string) {
	key := prefix + name
	modeled, found := findField(sdkStruct, func(f structField) bool {
		theirs, _ := f.encoding(EncoderJSON)
		return !theirs.skipped && theirs.name == name
	})
	if !found {
		needs[key] = LacksKey{Type: root, Key: key}.String()
		return
	}
	ownInner := s.localStruct(element(field.v.Type()))
	sdkInner := s.clientGoStruct(element(modeled.v.Type()))
	if ownInner != nil && sdkInner != nil {
		s.collectNeeds(ownInner, sdkInner, root, key+".", needs)
		return
	}
	own, theirs := s.spell(field.v.Type()), s.spell(modeled.v.Type())
	if own == theirs && embeds {
		needs[key] = "shadows " + key + ", which " + root + " models as " + theirs
	}
	if own != theirs && prefix == "" {
		needs[key] = FieldKind{Type: root, Field: modeled.v.Name(), Kind: theirs}.String()
	}
	if own != theirs && prefix != "" {
		needs[key] = "retypes " + key + " as " + own + ", which " + root + " models as " + theirs
	}
}

// embeds reports whether st embeds named, through a pointer or not, directly
// or through a struct of the package's own it embeds: a key st carries the
// way named models it shadows named's either way.
func (s SDK) embeds(st *types.Struct, named *types.Named) bool {
	for embedded := range s.embeddedClientGoStructs(st) {
		if embedded == named {
			return true
		}
	}
	return false
}

// clientGoStruct returns the struct client-go declares that t is or points
// to, and nil for anything else.
func (s SDK) clientGoStruct(t types.Type) *types.Named {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	named, isNamed := t.(*types.Named)
	if !isNamed || named.Obj().Pkg() != s.Types {
		return nil
	}
	if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
		return nil
	}
	return named
}

// localStruct returns the struct t is or points to when no client-go type
// declares it: a named struct of another package, or one with no name. It
// returns nil for anything else.
func (s SDK) localStruct(t types.Type) *types.Struct {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	if named, isNamed := t.(*types.Named); isNamed && named.Obj().Pkg() == s.Types {
		return nil
	}
	st, _ := t.Underlying().(*types.Struct)
	return st
}
