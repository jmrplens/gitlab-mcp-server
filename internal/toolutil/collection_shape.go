// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"reflect"
	"strings"
)

// CollectionShape is what one output type publishes, in the two terms a reader
// of a list asks about: the single list the type is the envelope of, and the
// block that says where the page it carries sits in the whole.
//
// It lives here rather than in the audit that first needed it because two
// readers ask the same question and must get the same answer. R-PAGE
// (cmd/audit_1to1) asks it of every read-only action to decide which ones owe a
// caller a pagination block, and [ResolveContentKind] asks it to decide which
// reads are lists and so carry [ContentList]. Kept in two copies, an output
// type the audit counted as a list could have been annotated as a detail, and
// neither would have said so.
type CollectionShape struct {
	// Pagination names the block found, empty for a type carrying none.
	Pagination string
	// Collection is the json name of the single list the type publishes, empty
	// unless the type is a collection envelope.
	Collection string
}

// IsCollection reports whether the type is the envelope of a list.
func (s CollectionShape) IsCollection() bool { return s.Collection != "" }

// collectionPaginationShapes are the blocks an output publishes to say where
// the page it carries sits in the whole.
//
// They are named as types rather than matched by name, so that renaming one
// breaks this build instead of quietly emptying the set and turning every
// paginated list into a list without pagination.
var collectionPaginationShapes = map[reflect.Type]bool{
	reflect.TypeFor[PaginationOutput]():               true,
	reflect.TypeFor[GraphQLPaginationOutput]():        true,
	reflect.TypeFor[GraphQLForwardPaginationOutput](): true,
	reflect.TypeFor[GraphQLPageInfo]():                true,
}

// collectionHintsShape is the block every output embeds that carries the
// server's own next-step hints. It is framing rather than content, so it is
// taken out before a collection envelope is counted.
var collectionHintsShape = reflect.TypeFor[HintableOutput]()

// collectionEmbedDepth bounds the walk through embedded structs. Go forbids a
// struct from embedding itself by value, but a pointer embed can cycle, and a
// walk that cannot end is worse than one that stops short: six levels is
// deeper than any output type here and is reached by none of them.
const collectionEmbedDepth = 6

// CollectionShapeOf reads an output type the way encoding/json will write it:
// an untagged embed's fields are promoted into the type around it, and a field
// the tag hides publishes nothing.
//
// A type is the envelope of a collection when it publishes exactly one content
// field and that field is a list of objects. Framing is not content: the hints
// block every output embeds and a pagination block are this server's own and
// are taken out before the count.
//
// The strictness is the whole of it. A response that is one object carrying a
// list under it is a single-object read whose caller was never promised the
// list: a project carries shared_with_groups, a job carries artifacts, a merge
// request carries assignees. Admitting those turned R-PAGE's 24 findings into
// 79, of which 55 were about a list nobody asked to page.
//
// A pagination block found anywhere in that walk counts, embedded or named,
// because either way the key reaches the model. A nil type, which a route
// registered without one carries, and a type that is no struct read as neither.
func CollectionShapeOf(outputType reflect.Type) CollectionShape {
	root := dereferenceType(outputType)
	if root == nil || root.Kind() != reflect.Struct {
		return CollectionShape{}
	}

	var walk collectionWalk
	walk.visit(root, 0)

	// One list and nothing else beside it is the envelope of a collection.
	// Anything else is an object that happens to carry a list, whose caller
	// asked for the object.
	if walk.lists != 1 || walk.content {
		walk.shape.Collection = ""
	}
	return walk.shape
}

// collectionWalk is what one pass over an output type has found so far: the
// shape being assembled, and the tally that decides whether the type is an
// envelope.
//
// The tally cannot live on the shape, because it is not part of the answer: a
// reader needs the list's name and the pagination block, and what else sits
// beside them is the working the rule showed itself.
//
// The lists are counted because an envelope holds exactly one. The content
// beside them is only ever asked whether there is any, so it is a flag: kept
// as a count, it read the same whether it went up or down, and a decrement in
// its place survived every test.
type collectionWalk struct {
	shape   CollectionShape
	lists   int
	content bool
}

// visit reads one struct's fields, following the untagged embeds that promote
// their fields into it.
func (w *collectionWalk) visit(structType reflect.Type, depth int) {
	if depth > collectionEmbedDepth {
		return
	}
	for field := range structType.Fields() {
		w.field(field, depth)
	}
}

// field classifies one field as framing, an embed to follow, the list, or
// content beside it.
//
// A field the reflect package lists always has a type, and dereferencing a
// pointer chain ends at a type, so there is no nil to guard against here.
func (w *collectionWalk) field(field reflect.StructField, depth int) {
	fieldType := dereferenceType(field.Type)
	if collectionPaginationShapes[fieldType] {
		if w.shape.Pagination == "" {
			w.shape.Pagination = fieldType.Name()
		}
		return
	}
	if field.Anonymous && fieldType.Kind() == reflect.Struct {
		if fieldType != collectionHintsShape {
			w.visit(fieldType, depth+1)
		}
		return
	}
	name, published := publishedFieldName(field)
	if !published {
		return
	}
	if listOfObjects(fieldType) {
		w.lists++
		if w.shape.Collection == "" {
			w.shape.Collection = name
		}
		return
	}
	w.content = true
}

// publishedFieldName is the json key a field reaches a model under, and
// whether it reaches one at all.
func publishedFieldName(field reflect.StructField) (string, bool) {
	if field.PkgPath != "" {
		return "", false
	}
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "-" {
		return "", false
	}
	if name == "" {
		return field.Name, true
	}
	return name, true
}

// listOfObjects reports whether the type is a list whose elements are objects,
// which is the shape a page of GitLab's answer takes. A list of strings is a
// value rather than a collection a caller pages through: a project's topics are
// not a page of topics.
func listOfObjects(fieldType reflect.Type) bool {
	if fieldType.Kind() != reflect.Slice {
		return false
	}
	// A slice always names its element type, so what dereferenceType returns
	// here is never the nil it keeps for a route registered without a type.
	return dereferenceType(fieldType.Elem()).Kind() == reflect.Struct
}

// dereferenceType follows pointers to the type underneath, and returns nil for
// a nil type so that a route registered without one is read as no collection
// rather than panicking.
func dereferenceType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
