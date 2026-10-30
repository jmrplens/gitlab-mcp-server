// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"go/types"
	"reflect"
	"testing"
)

// localSource is a tool package beside the stand-in client-go, with the
// structs a raw request carries: one that embeds a client-go struct and
// carries keys beside it in every way a real one does, and structs that are
// no superset. A tilde stands for a backtick.
const localSource = `package local

import gl "gitlab"

type extra struct {
	Count int64 ~json:"count"~
}

type deeper struct {
	Deep bool ~json:"deep"~
}

type innerAPI struct {
	*gl.Inner
	Raw   bool   ~json:"raw"~
	Extra string ~json:"extra"~
}

type mirror struct {
	Raw  string ~json:"raw"~
	Wide int64  ~json:"wide"~
}

type kindAPI struct {
	Value int ~json:"value"~
}

type widgetAPI struct {
	extra
	gl.Options
	*deeper
	gl.Embedded
	Always  *string           ~json:"always"~
	Scalar  int64             ~json:"scalar"~
	Date    *gl.ISOTime       ~json:"date,omitempty"~
	Nested  []*innerAPI       ~json:"nested"~
	ByName  map[string]mirror ~json:"by_name"~
	Fixed   []struct {
		Raw  bool     ~json:"raw"~
		More int      ~json:"more"~
		Copy gl.Inner ~json:"copy"~
	} ~json:"fixed"~
	Kind    kindAPI ~json:"kind"~
	Fresh   string  ~json:"fresh"~
	Ignored string  ~json:"-"~
	hidden  string
}

type notStruct int

type innerBase struct {
	*gl.Inner
	Archived bool ~json:"archived"~
}

type innerResponse struct {
	innerBase
	Raw   bool  ~json:"raw"~
	Count int64 ~json:"count"~
}

type looped struct {
	*looped
	gl.Inner
	Loop bool ~json:"loop"~
}

type loopedBase struct {
	*loopedBase
	Loop bool ~json:"loop"~
}

type loopedMirror struct {
	loopedBase
	Count int64 ~json:"count"~
}

var (
	widgets   []*widgetAPI
	one       *mirror
	number    notStruct
	options   *gl.Options
	responses []innerResponse
	loops     *looped
	mirrors   loopedMirror
)
`

// localPackage type-checks localSource beside the stand-in client-go.
func localPackage(t *testing.T, sdk SDK) *types.Package {
	t.Helper()
	return checkSource(t, "local", localSource, map[string]*types.Package{"gitlab": sdk.Types})
}

// TestSupersetOf_AStructEmbeddingClientGo_NeedsAPredicatePerKeyItCarriesBeside
// verifies the rule a raw request's superset is held to: every key the
// struct carries beside the client-go struct it embeds, through a struct it
// embeds of its own as well, needs a LacksKey when client-go does not model
// it and a FieldKind when it models it with another type at the top level.
// A key below a field holding a struct of the package's own in place of a
// client-go struct is judged against that struct, through a pointer, a
// slice, a map or a struct with no name, and spelled with its parent; a key
// such a struct mirrors with client-go's own type needs nothing and is not
// among the keys (holding client-go's struct in a field of its own is no
// embedding), a key one that embeds client-go's struct carries the same
// way shadows it for nothing, and so does a top-level key; a nested key
// carried with another type is spelled as what no predicate answers. An
// unexported field and one tagged "-" are never carried and need nothing.
func TestSupersetOf_AStructEmbeddingClientGo_NeedsAPredicatePerKeyItCarriesBeside(t *testing.T) {
	sdk := standInSDK(t)
	local := localPackage(t, sdk)
	got, ok := SupersetOf(sdk, local.Scope().Lookup("widgets").Type())
	if !ok {
		t.Fatal("SupersetOf(widgets) found no superset")
	}
	want := Superset{
		Type: "local.widgetAPI",
		Root: "Options",
		Keys: []string{
			"always", "by_name.raw", "by_name.wide", "count", "date", "deep",
			"fixed.copy", "fixed.more", "fresh", "kind", "nested.extra", "nested.raw", "scalar",
		},
		Needs: []string{
			"FieldKind{Options.Always, string}",
			"FieldKind{Options.Kind, NotStruct}",
			"LacksKey{Options, by_name.wide}",
			"LacksKey{Options, count}",
			"LacksKey{Options, deep}",
			"LacksKey{Options, fixed.copy}",
			"LacksKey{Options, fixed.more}",
			"LacksKey{Options, fresh}",
			"LacksKey{Options, nested.extra}",
			"retypes by_name.raw as string, which Options models as bool",
			"shadows date, which Options models as *ISOTime",
			"shadows nested.raw, which Options models as bool",
			"shadows scalar, which Options models as int64",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SupersetOf(widgets) =\n%+v\nwant\n%+v", got, want)
	}
}

// TestSupersetOf_ClientGoEmbeddedThroughAStructOfItsOwn_IsTheRoot verifies
// that a client-go struct embedded below a struct of the package's own the
// decoded struct embeds is its root, since the keys of both reach the JSON
// at the decoded struct's depth: the key the struct of its own carries
// beside it and the one the decoded struct adds each need a LacksKey, and a
// key the decoded struct carries the way the root models it shadows it. A
// struct that embeds itself through a pointer is read once, beside a
// client-go struct or not.
func TestSupersetOf_ClientGoEmbeddedThroughAStructOfItsOwn_IsTheRoot(t *testing.T) {
	sdk := standInSDK(t)
	local := localPackage(t, sdk)
	cases := []struct {
		name string
		want Superset
	}{
		{name: "responses", want: Superset{
			Type: "local.innerResponse", Root: "Inner", Keys: []string{"archived", "count", "raw"},
			Needs: []string{"LacksKey{Inner, archived}", "LacksKey{Inner, count}", "shadows raw, which Inner models as bool"},
		}},
		{name: "loops", want: Superset{
			Type: "local.looped", Root: "Inner", Keys: []string{"loop"},
			Needs: []string{"LacksKey{Inner, loop}"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := SupersetOf(sdk, local.Scope().Lookup(tc.name).Type())
			if !ok || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SupersetOf(%s) = %+v, %v, want %+v", tc.name, got, ok, tc.want)
			}
		})
	}
}

// TestSupersetOf_WhatIsNoSuperset_ReportsNone verifies the values a raw
// request can carry that the rule does not judge: client-go's own struct, a
// struct of the package's own that embeds none of client-go's (whose pairing
// only its entry's predicates say), one that embeds a struct of its own
// embedding itself and nothing of client-go's, a named type that is no
// struct and an interface.
func TestSupersetOf_WhatIsNoSuperset_ReportsNone(t *testing.T) {
	sdk := standInSDK(t)
	local := localPackage(t, sdk)
	cases := map[string]types.Type{
		"client-go's own struct":  local.Scope().Lookup("options").Type(),
		"a mirror":                local.Scope().Lookup("one").Type(),
		"a mirror embedding loop": local.Scope().Lookup("mirrors").Type(),
		"a named non-struct":      local.Scope().Lookup("number").Type(),
		"an interface":            types.Universe.Lookup("any").Type(),
	}
	for name, carried := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := SupersetOf(sdk, carried); ok {
				t.Errorf("SupersetOf(%s) = %+v, want none", carried, got)
			}
		})
	}
}
