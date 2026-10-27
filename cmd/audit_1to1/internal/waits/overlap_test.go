// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"go/types"
	"reflect"
	"testing"
)

// overlapSDKSource is a stand-in client-go for the comparison of a captured
// struct with the client-go struct decoded from the same response: a widget
// whose keys are modeled every way a real one models them. A tilde stands for
// a backtick.
const overlapSDKSource = `package gitlab

import "other"

type User struct {
	ID   int64  ~json:"id"~
	Name string ~json:"name"~
}

type Pipeline struct {
	Duration int64 ~json:"duration"~
}

type Change struct {
	Value int64 ~json:"value"~
}

type History struct {
	Limit []Change ~json:"limit"~
}

type Base struct {
	Token string ~json:"token"~
}

type Widget struct {
	Base
	ID       int64       ~json:"id"~
	Name     string      ~json:"name"~
	Users    []*User     ~json:"users"~
	Pipeline *Pipeline   ~json:"pipeline"~
	Owner    *User       ~json:"owner"~
	When     *other.Time ~json:"when"~
	History  History     ~json:"history"~
	Err      error       ~json:"err"~
	Label    string      ~json:"label"~
	Marker   string      ~json:"Embedded"~
	Hidden   string      ~json:"-"~
	Self     *Widget     ~json:"self"~
}
`

// overlapSharedSource is a second package of the repository's own, whose
// struct a captured struct embeds.
const overlapSharedSource = `package shared

type Embedded struct {
	ID     *int64 ~json:"id"~
	Name   string ~json:"name"~
	Marker string ~json:"Embedded"~
}
`

// overlapLocalSource is a tool package with the structs a capture is decoded
// into: one that reads a key every way a real one does, one that holds
// itself, and a variable of a struct literal.
const overlapLocalSource = `package local

import (
	gl "gitlab"
	"other"
	"example.com/repo/internal/shared"
)

type userExtra struct {
	ID          int64  ~json:"id"~
	PublicEmail string ~json:"public_email"~
}

type measures struct {
	Duration *int64 ~json:"duration"~
}

type change struct {
	Value *int64 ~json:"value"~
}

type labelExtra struct {
	Text string ~json:"text"~
}

type widgetExtra struct {
	shared.Embedded
	gl.Base
	Name     *string             ~json:"name"~
	Token    string              ~json:"token"~
	Users    []userExtra         ~json:"users"~
	Pipeline *measures           ~json:"pipeline"~
	Owner    *gl.User            ~json:"owner"~
	When     *other.Time         ~json:"when"~
	History  map[string][]change ~json:"history"~
	Err      error               ~json:"err"~
	Label    *labelExtra         ~json:"label"~
	Fresh    string              ~json:"fresh"~
	Hidden   string              ~json:"hidden"~
	Ignored  string              ~json:"-"~
	unread   string
	id       int64 ~json:"id"~
}

type node struct {
	Self *node  ~json:"self"~
	ID   *int64 ~json:"id"~
}

var literal struct {
	Owner *struct {
		Avatar string ~json:"avatar"~
		ID     *int64 ~json:"id"~
	} ~json:"owner"~
}
`

// overlapModule is the import path every package of the stand-in
// repository starts with.
const overlapModule = "example.com/repo"

// overlapPackages type-checks the stand-in client-go and the two packages of
// the repository beside it.
func overlapPackages(t *testing.T) (SDK, *types.Package) {
	t.Helper()
	other := checkSource(t, "other", otherSource, nil)
	sdk := SDK{Types: checkSource(t, "gitlab", overlapSDKSource, map[string]*types.Package{"other": other})}
	shared := checkSource(t, overlapModule+"/internal/shared", overlapSharedSource, nil)
	local := checkSource(t, overlapModule+"/internal/tools/local", overlapLocalSource, map[string]*types.Package{
		"gitlab": sdk.Types, "other": other, overlapModule + "/internal/shared": shared,
	})
	return sdk, local
}

// namedIn returns the named type pkg declares under name.
func namedIn(t *testing.T, pkg *types.Package, name string) *types.Named {
	t.Helper()
	named, isNamed := pkg.Scope().Lookup(name).Type().(*types.Named)
	if !isNamed {
		t.Fatalf("%s.%s is not a named type", pkg.Name(), name)
	}
	return named
}

// structIn returns the struct of what pkg declares under name, a named type
// or a variable.
func structIn(t *testing.T, pkg *types.Package, name string) *types.Struct {
	t.Helper()
	st, isStruct := pkg.Scope().Lookup(name).Type().Underlying().(*types.Struct)
	if !isStruct {
		t.Fatalf("%s.%s is not a struct", pkg.Name(), name)
	}
	return st
}

// TestCompare_ACapturedWidget_SharesEveryKeyBothReadAndListsTheRest verifies
// the comparison every captured key is judged by. A key both read as an
// object, through a pointer or a list held the same way, is compared below
// it; a key read as client-go's own object, as a time of another package, as
// an interface of the universe, as a map where client-go reads a struct or as
// an object where it reads a string is a value both read; a struct of the
// repository's own the captured struct
// embeds carries its keys at its depth, where a shallower key of the same
// name shadows it; client-go's struct it embeds is the SDK's half and is not
// compared; and a key client-go does not model at that depth is a lack, an
// unexported field and one tagged "-" never being read. Neither an unexported
// field nor the name of an embedded struct is a key, so neither shadows a key
// of the struct it embeds.
func TestCompare_ACapturedWidget_SharesEveryKeyBothReadAndListsTheRest(t *testing.T) {
	sdk, local := overlapPackages(t)
	widget := namedIn(t, sdk.Types, "Widget")
	got := Compare(sdk, overlapModule, structIn(t, local, "widgetExtra"), widget)
	want := Comparison{
		Shared: []Shared{
			{Key: "Embedded", Owner: "Widget", Field: "Marker", Kind: "string", Captured: "string"},
			{Key: "err", Owner: "Widget", Field: "Err", Kind: "error", Captured: "error"},
			{Key: "history", Owner: "Widget", Field: "History", Kind: "History", Captured: "map[string][]local.change"},
			{Key: "id", Owner: "Widget", Field: "ID", Kind: "int64", Captured: "*int64"},
			{Key: "label", Owner: "Widget", Field: "Label", Kind: "string", Captured: "*local.labelExtra"},
			{Key: "name", Owner: "Widget", Field: "Name", Kind: "string", Captured: "*string"},
			{Key: "owner", Owner: "Widget", Field: "Owner", Kind: "*User", Captured: "*gitlab.User"},
			{Key: "pipeline.duration", Owner: "Pipeline", Field: "Duration", Kind: "int64", Captured: "*int64"},
			{Key: "token", Owner: "Widget", Field: "Token", Kind: "string", Captured: "string"},
			{Key: "users.id", Owner: "User", Field: "ID", Kind: "int64", Captured: "int64"},
			{Key: "when", Owner: "Widget", Field: "When", Kind: "*other.Time", Captured: "*other.Time"},
		},
		Lacks: []Lack{
			{Key: "fresh", Owner: "Widget", Name: "fresh"},
			{Key: "hidden", Owner: "Widget", Name: "hidden"},
			{Key: "users.public_email", Owner: "User", Name: "public_email"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Compare(widgetExtra, Widget) =\n%+v\nwant\n%+v", got, want)
	}
}

// TestCompare_AStructThatHoldsItself_IsComparedOnce verifies that a captured
// struct holding itself where client-go's holds itself too is compared at the
// top and not again below, which would never end.
func TestCompare_AStructThatHoldsItself_IsComparedOnce(t *testing.T) {
	sdk, local := overlapPackages(t)
	got := Compare(sdk, overlapModule, structIn(t, local, "node"), namedIn(t, sdk.Types, "Widget"))
	want := Comparison{Shared: []Shared{{Key: "id", Owner: "Widget", Field: "ID", Kind: "int64", Captured: "*int64"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Compare(node, Widget) = %+v, want %+v", got, want)
	}
}

// TestCompare_AStructLiteral_IsComparedAsANamedStructIs verifies that a
// capture decoded into a struct literal, which has no name and no package,
// is compared the same way, a literal below it read as an object both read.
func TestCompare_AStructLiteral_IsComparedAsANamedStructIs(t *testing.T) {
	sdk, local := overlapPackages(t)
	got := Compare(sdk, overlapModule, structIn(t, local, "literal"), namedIn(t, sdk.Types, "Widget"))
	want := Comparison{
		Shared: []Shared{{Key: "owner.id", Owner: "User", Field: "ID", Kind: "int64", Captured: "*int64"}},
		Lacks:  []Lack{{Key: "owner.avatar", Owner: "User", Name: "avatar"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Compare(literal, Widget) = %+v, want %+v", got, want)
	}
}

// TestContainers_EachHolder_IsSpelledAsItHoldsTheStruct verifies the shape two
// keys have to share to be compared as one object: a pointer is read through,
// a slice and an array are a list, and a map is a map.
func TestContainers_EachHolder_IsSpelledAsItHoldsTheStruct(t *testing.T) {
	sdk, _ := overlapPackages(t)
	user := namedIn(t, sdk.Types, "User")
	cases := []struct {
		name string
		t    types.Type
		want string
	}{
		{name: "a struct", t: user, want: ""},
		{name: "a pointer", t: types.NewPointer(user), want: ""},
		{name: "a slice of pointers", t: types.NewSlice(types.NewPointer(user)), want: "[]"},
		{name: "an array", t: types.NewArray(user, 2), want: "[]"},
		{name: "a map of slices", t: types.NewMap(types.Typ[types.String], types.NewSlice(user)), want: "map[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := containers(tc.t); got != tc.want {
				t.Errorf("containers(%s) = %q, want %q", tc.t, got, tc.want)
			}
		})
	}
}
