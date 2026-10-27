// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// collection_shape_test.go pins CollectionShapeOf, the one reading of an
// output type that R-PAGE and ResolveContentKind both rest on: which types are
// the envelope of a list, and which publish a pagination block.
package toolutil

import (
	"reflect"
	"testing"
)

// The fixture output types below stand for the shapes this repository really
// publishes, spelled small enough that a reader of a case can see the whole of
// what is being judged.

// shapeItem is one object of a list, since a list of objects is what a page of
// GitLab's answer holds.
type shapeItem struct {
	ID int64 `json:"id"`
}

// shapeListOutput is a collection envelope with no pagination: exactly the
// shape internal/tools/impersonationtokens publishes.
type shapeListOutput struct {
	HintableOutput
	Tokens []shapeItem `json:"tokens"`
}

// shapePaginatedListOutput is the same envelope saying where its page sits.
type shapePaginatedListOutput struct {
	HintableOutput
	Tokens     []shapeItem      `json:"tokens"`
	Pagination PaginationOutput `json:"pagination"`
}

// shapeCursorListOutput pages with a cursor rather than a page number, which
// is what a GraphQL connection publishes.
type shapeCursorListOutput struct {
	Nodes    []shapeItem             `json:"nodes"`
	PageInfo GraphQLPaginationOutput `json:"page_info"`
}

// shapeObjectOutput is one object that happens to carry a list under it. Its
// caller asked for the object, so it is not a collection read.
type shapeObjectOutput struct {
	HintableOutput
	Name   string      `json:"name"`
	Labels []shapeItem `json:"labels"`
}

// shapePagedObjectOutput is a page of a list with something of its own beside
// the list, the shape projects.ListOutput takes with its two alternative
// lists: no envelope, but the pagination block is still found.
type shapePagedObjectOutput struct {
	Projects       []shapeItem      `json:"projects"`
	SimpleProjects []shapeItem      `json:"simple_projects"`
	Pagination     PaginationOutput `json:"pagination"`
}

// shapeScalarListOutput publishes a list of strings, which is a value rather
// than a collection anybody pages: a project's topics are not a page of topics.
type shapeScalarListOutput struct {
	Topics []string `json:"topics"`
}

// shapeTwoListsOutput publishes two lists, so no single one of them is the
// response.
type shapeTwoListsOutput struct {
	Added   []shapeItem `json:"added"`
	Removed []shapeItem `json:"removed"`
}

// shapeEmbeddedListOutput reaches its list through an untagged embed, which
// encoding/json promotes into the type around it.
type shapeEmbeddedListOutput struct {
	shapeEmbeddedBody
}

type shapeEmbeddedBody struct {
	Keys []shapeItem `json:"keys"`
}

// shapeEmbeddedPagination reaches its pagination block through an embed too.
type shapeEmbeddedPagination struct {
	Pagination *GraphQLForwardPaginationOutput `json:"pagination"`
}

type shapeEmbeddedPaginationOutput struct {
	shapeEmbeddedPagination
	States []shapeItem `json:"states"`
}

// shapeTwoPaginationBlocksOutput carries two blocks, and the first one the
// walk meets is the one named.
type shapeTwoPaginationBlocksOutput struct {
	Nodes    []shapeItem     `json:"nodes"`
	PageInfo GraphQLPageInfo `json:"page_info"`
	Legacy   PaginationOutput
}

// shapeHiddenFieldOutput carries a field the json tag hides, which reaches no
// model and so is not content beside the list.
type shapeHiddenFieldOutput struct {
	Keys     []shapeItem `json:"keys"`
	internal string      //nolint:unused // present to prove an unexported field publishes nothing
	Skipped  string      `json:"-"`
}

// shapeUntaggedOutput publishes its list under the Go field name, since no
// tag names it otherwise.
type shapeUntaggedOutput struct {
	Entries []*shapeItem
}

// ShapeEmbeddedScalar is an exported named string a type embeds. An embed is
// only promoted when it is a struct: encoding/json writes this one under its
// own type name, so it is content beside a list rather than something to walk
// into.
type ShapeEmbeddedScalar string

// shapeEmbeddedScalarOutput is a list with one of those beside it, which is
// therefore not a collection envelope.
type shapeEmbeddedScalarOutput struct {
	ShapeEmbeddedScalar
	Keys []shapeItem `json:"keys"`
}

// shapeUnexportedScalar is the same thing under an unexported name, which
// encoding/json writes nowhere at all, so the list is alone after all.
type shapeUnexportedScalar string //nolint:unused // present as an embed, whose value nothing reads

type shapeUnexportedScalarOutput struct {
	shapeUnexportedScalar             //nolint:unused // the embed is the subject: it publishes nothing
	Keys                  []shapeItem `json:"keys"`
}

// The six-deep chain below stands at the embed bound: encoding/json promotes
// the list at the bottom of it into the type at the top, and the walk reads to
// exactly that level. The seven-deep chain is one past it.

type (
	shapeDepth6    struct{ shapeDepth5 }
	shapeDepth5    struct{ shapeDepth4 }
	shapeDepth4    struct{ shapeDepth3 }
	shapeDepth3    struct{ shapeDepth2 }
	shapeDepth2    struct{ shapeDepth1 }
	shapeDepth1    struct{ shapeDepthBody }
	shapeDepthBody struct {
		Keys []shapeItem `json:"keys"`
	}
)

type shapeDepth7 struct{ shapeDepth6 }

// TestCollectionShapeOf_CollectionEnvelope_IsOneListAndNothingElse verifies
// the one structural decision both readers of the shape rest on: a type is the
// envelope of a collection when it is a list and its framing, and an object
// carrying a list is a single-object read whose caller was never promised the
// list. The pagination block is found wherever the walk meets it, envelope or
// not, because that is what the content kind reads beside the envelope.
func TestCollectionShapeOf_CollectionEnvelope_IsOneListAndNothingElse(t *testing.T) {
	cases := []struct {
		name           string
		outputType     reflect.Type
		wantCollection string
		wantPagination string
	}{
		{
			name:           "a list and its hints is a collection",
			outputType:     reflect.TypeFor[shapeListOutput](),
			wantCollection: "tokens",
		},
		{
			name:           "a pagination block is framing rather than content",
			outputType:     reflect.TypeFor[shapePaginatedListOutput](),
			wantCollection: "tokens",
			wantPagination: "PaginationOutput",
		},
		{
			name:           "a cursor block counts as pagination too",
			outputType:     reflect.TypeFor[shapeCursorListOutput](),
			wantCollection: "nodes",
			wantPagination: "GraphQLPaginationOutput",
		},
		{
			name:       "an object carrying a list is not a collection",
			outputType: reflect.TypeFor[shapeObjectOutput](),
		},
		{
			name:           "a page with two alternative lists is no envelope but keeps its pagination",
			outputType:     reflect.TypeFor[shapePagedObjectOutput](),
			wantPagination: "PaginationOutput",
		},
		{
			name:       "a list of strings is a value, not a collection",
			outputType: reflect.TypeFor[shapeScalarListOutput](),
		},
		{
			name:       "two lists are no single response",
			outputType: reflect.TypeFor[shapeTwoListsOutput](),
		},
		{
			name:           "an embedded list is promoted into the type around it",
			outputType:     reflect.TypeFor[shapeEmbeddedListOutput](),
			wantCollection: "keys",
		},
		{
			name:           "an embedded pointer to a pagination block is found",
			outputType:     reflect.TypeFor[shapeEmbeddedPaginationOutput](),
			wantCollection: "states",
			wantPagination: "GraphQLForwardPaginationOutput",
		},
		{
			name:           "the first pagination block met is the one named",
			outputType:     reflect.TypeFor[shapeTwoPaginationBlocksOutput](),
			wantCollection: "nodes",
			wantPagination: "GraphQLPageInfo",
		},
		{
			name:           "a hidden field is not content beside the list",
			outputType:     reflect.TypeFor[shapeHiddenFieldOutput](),
			wantCollection: "keys",
		},
		{
			name:           "an untagged list of pointers is published under its field name",
			outputType:     reflect.TypeFor[shapeUntaggedOutput](),
			wantCollection: "Entries",
		},
		{
			name:           "a pointer to the envelope reads the same",
			outputType:     reflect.TypeFor[*shapeListOutput](),
			wantCollection: "tokens",
		},
		{
			name:       "an embedded named scalar is content beside the list",
			outputType: reflect.TypeFor[shapeEmbeddedScalarOutput](),
		},
		{
			name:           "an embedded unexported scalar reaches no model and is not content",
			outputType:     reflect.TypeFor[shapeUnexportedScalarOutput](),
			wantCollection: "keys",
		},
		{
			name:           "an embed chain as deep as the bound is still read",
			outputType:     reflect.TypeFor[shapeDepth6](),
			wantCollection: "keys",
		},
		{
			name:       "an embed chain past the bound is not read",
			outputType: reflect.TypeFor[shapeDepth7](),
		},
		{
			name:       "a route registered with no output type is skipped",
			outputType: nil,
		},
		{
			name:       "a type that is no struct is skipped",
			outputType: reflect.TypeFor[[]shapeItem](),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			shape := CollectionShapeOf(testCase.outputType)
			if shape.Collection != testCase.wantCollection {
				t.Errorf("collection = %q, want %q", shape.Collection, testCase.wantCollection)
			}
			if shape.Pagination != testCase.wantPagination {
				t.Errorf("pagination = %q, want %q", shape.Pagination, testCase.wantPagination)
			}
			if shape.IsCollection() != (testCase.wantCollection != "") {
				t.Errorf("IsCollection() = %t, want %t", shape.IsCollection(), testCase.wantCollection != "")
			}
		})
	}
}
