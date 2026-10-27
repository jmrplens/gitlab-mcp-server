// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// otherSource is a package beside the stand-in client-go, so a type spelled
// from it carries its package's name and a Nullable from it is not
// client-go's.
const otherSource = `package other

type Time struct{}

type Nullable[T any] map[bool]T
`

// standInSource is a stand-in client-go root package, one field or method
// for each case a predicate tells apart. A tilde stands for a backtick.
const standInSource = `package gitlab

import "other"

type ISOTime struct{}

type Nullable[T any] map[bool]T

type NotStruct int

type RequestOptionFunc func() error

type Inner struct {
	Raw bool ~json:"raw"~
}

type Embedded struct {
	Page int64 ~json:"page"~
}

type PtrEmbedded struct {
	Token string ~json:"token"~
}

type InnerAlias = Inner

type Options struct {
	Always      string                  ~json:"always" url:"always"~
	Omitted     *string                 ~json:"omitted,omitempty" url:"omitted,omitempty"~
	Zero        int64                   ~json:"zero,omitzero"~
	Skipped     *string                 ~json:"-" url:"-"~
	Untagged    *string
	Date        *ISOTime                ~json:"date,omitempty"~
	When        *other.Time             ~json:"when"~
	Null        Nullable[string]        ~json:"null,omitempty"~
	ForeignNull other.Nullable[string]  ~json:"foreign_null"~
	List        []string                ~json:"list,omitempty"~
	Hidden      []string                ~json:"-"~
	PtrList     *[]string               ~json:"ptr_list,omitempty"~
	Bare        []string                ~json:"bare"~
	Nested      []*Inner                ~json:"nested"~
	ByName      map[string]Inner        ~json:"by_name"~
	Fixed       [2]Inner                ~json:"fixed"~
	Scalar      int64                   ~json:"scalar"~
	Format      *string                 ~url:"format,omitempty"~
	Fn          func()                  ~json:"fn"~
	Err         error                   ~json:"err,omitempty"~
	Both        *int64                  ~json:"both,omitempty,string"~
	Kind        NotStruct               ~json:"kind,omitempty"~
	NotStruct
	Embedded
	*PtrEmbedded
}

type WidgetsServiceInterface interface {
	List(opt *Options, options ...RequestOptionFunc) ([]*Inner, error)
	Archive(id int64, options ...RequestOptionFunc) error
	Mixed(at *other.Time, kind NotStruct, id int64, options ...RequestOptionFunc) (*Inner, error)
	Ping()
}

type BrokenServiceInterface struct{}

func standInFunc() {}
`

// importerFunc adapts a function to go/types' importer.
type importerFunc func(path string) (*types.Package, error)

// Import imports path.
func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// checkSource type-checks one file as the package at path.
func checkSource(t *testing.T, path, source string, imports map[string]*types.Package) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path+".go", strings.ReplaceAll(source, "~", "`"), 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	config := types.Config{Importer: importerFunc(func(imported string) (*types.Package, error) {
		if pkg, ok := imports[imported]; ok {
			return pkg, nil
		}
		return nil, errors.New("no package " + imported)
	})}
	pkg, err := config.Check(path, fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("check %s: %v", path, err)
	}
	return pkg
}

// standInSDK returns the stand-in client-go with the routes given.
func standInSDK(t *testing.T, routes ...string) SDK {
	t.Helper()
	other := checkSource(t, "other", otherSource, nil)
	sdk := SDK{Types: checkSource(t, "gitlab", standInSource, map[string]*types.Package{"other": other}), Routes: map[string]bool{}}
	for _, route := range routes {
		sdk.Routes[route] = true
	}
	return sdk
}

// predicateCase is one predicate with what it has to come to on the stand-in.
type predicateCase struct {
	name      string
	predicate Predicate
	want      bool
	// wantErr is a fragment of the error, empty for none.
	wantErr string
}

// runPredicateCases evaluates each case against sdk.
func runPredicateCases(t *testing.T, sdk SDK, cases []predicateCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.predicate.Holds(sdk)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("%s: Holds error = %v, want one naming %q", tc.predicate, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: Holds error = %v, want none", tc.predicate, err)
			}
			if got != tc.want {
				t.Errorf("%s: Holds = %v, want %v", tc.predicate, got, tc.want)
			}
		})
	}
}

// TestWritesAlways_StandInFields_HoldOnlyWhileTheEncoderWritesThemEveryCall
// verifies the predicate behind row 6: a field holds while its tag for the
// encoder carries neither omitempty nor omitzero and is not "-", a field
// with no tag is written under its Go name every call, and a type, a field
// or an encoder that does not exist is an error rather than a verdict.
func TestWritesAlways_StandInFields_HoldOnlyWhileTheEncoderWritesThemEveryCall(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "no omitempty in json", predicate: WritesAlways{"Options", "Always", EncoderJSON}, want: true},
		{name: "no omitempty in url", predicate: WritesAlways{"Options", "Always", EncoderQuery}, want: true},
		{name: "omitempty", predicate: WritesAlways{"Options", "Omitted", EncoderJSON}},
		{name: "omitzero", predicate: WritesAlways{"Options", "Zero", EncoderJSON}},
		{name: "omitempty before another option", predicate: WritesAlways{"Options", "Both", EncoderJSON}},
		{name: "skipped", predicate: WritesAlways{"Options", "Skipped", EncoderJSON}},
		{name: "untagged", predicate: WritesAlways{"Options", "Untagged", EncoderJSON}, want: true},
		{name: "promoted from an embedded struct", predicate: WritesAlways{"Options", "Page", EncoderJSON}, want: true},
		{name: "promoted through an embedded pointer", predicate: WritesAlways{"Options", "Token", EncoderJSON}, want: true},
		{name: "an embedded field is not a field", predicate: WritesAlways{"Options", "Embedded", EncoderJSON}, wantErr: "has no field Embedded"},
		{name: "unknown encoder", predicate: WritesAlways{"Options", "Always", "xml"}, wantErr: `encoder "xml"`},
		{name: "unknown field", predicate: WritesAlways{"Options", "Gone", EncoderJSON}, wantErr: "has no field Gone"},
		{name: "unknown type", predicate: WritesAlways{"Gone", "Always", EncoderJSON}, wantErr: "declares no type Gone"},
		{name: "not a struct", predicate: WritesAlways{"NotStruct", "Always", EncoderJSON}, wantErr: "NotStruct is not a struct"},
		{name: "an interface", predicate: WritesAlways{"WidgetsServiceInterface", "List", EncoderJSON}, wantErr: "is not a struct"},
		{name: "an alias", predicate: WritesAlways{"InnerAlias", "Raw", EncoderJSON}, wantErr: "InnerAlias is not a struct"},
		{name: "not a type", predicate: WritesAlways{"standInFunc", "Raw", EncoderJSON}, wantErr: "declares no type standInFunc"},
	})
}

// TestLacksKey_StandInKeys_HoldOnlyWhileNoFieldModelsTheKey verifies the
// predicate behind most raw decodes: a key either encoder writes, top level,
// promoted or, dotted, below a field holding a struct through a pointer,
// slice, map or array, is modeled; a key nothing writes, a skipped field's
// Go name included, is not; and a dotted key below a field that holds no
// struct, or below a key no field is written under, is an error. The last
// is what a misspelled parent would otherwise hold under forever: a struct
// that lacked the parent would be written LacksKey on the parent itself.
func TestLacksKey_StandInKeys_HoldOnlyWhileNoFieldModelsTheKey(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "json key", predicate: LacksKey{"Options", "always"}},
		{name: "url key alone", predicate: LacksKey{"Options", "format"}},
		{name: "go name of an untagged field", predicate: LacksKey{"Options", "Untagged"}},
		{name: "promoted key", predicate: LacksKey{"Options", "page"}},
		{name: "missing key", predicate: LacksKey{"Options", "entity_type"}, want: true},
		{name: "skipped field", predicate: LacksKey{"Options", "Skipped"}, want: true},
		{name: "nested through a slice of pointers", predicate: LacksKey{"Options", "nested.raw"}},
		{name: "nested through a map", predicate: LacksKey{"Options", "by_name.raw"}},
		{name: "nested through an array", predicate: LacksKey{"Options", "fixed.raw"}},
		{name: "nested key missing", predicate: LacksKey{"Options", "nested.overridden"}, want: true},
		{name: "parent missing", predicate: LacksKey{"Options", "gone.raw"}, wantErr: "no field is written under gone to look raw up in"},
		{name: "deeper parent missing", predicate: LacksKey{"Options", "nested.gone.raw"}, wantErr: "no field is written under nested.gone to look raw up in"},
		{name: "below a scalar", predicate: LacksKey{"Options", "scalar.raw"}, wantErr: "scalar holds no struct to look raw up in"},
		{name: "unknown type", predicate: LacksKey{"Gone", "always"}, wantErr: "declares no type Gone"},
	})
}

// TestFieldKind_StandInFields_HoldWhileTheTypeIsSpelledAsDeclared verifies
// the type spelling: client-go's own types unqualified, any other package's
// by its name.
func TestFieldKind_StandInFields_HoldWhileTheTypeIsSpelledAsDeclared(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "client-go type", predicate: FieldKind{"Options", "Date", "*ISOTime"}, want: true},
		{name: "another package's type", predicate: FieldKind{"Options", "When", "*other.Time"}, want: true},
		{name: "a different type", predicate: FieldKind{"Options", "When", "*time.Time"}},
		{name: "unknown field", predicate: FieldKind{"Options", "Gone", "int64"}, wantErr: "has no field Gone"},
	})
}

// TestNoMethodFor_Routes_HoldWhileNoMethodSendsTheRoute verifies a route
// predicate against the routes the SDK sends, and that it cannot hold for a
// reason other than client-go sending nothing there: a route that is not
// spelled as one, or that names its identifiers (":id") where the SDK's
// routes write a bare ":", could never be sent, and an SDK none of whose
// routes was read sends nothing at all, so each is refused rather than held.
func TestNoMethodFor_Routes_HoldWhileNoMethodSendsTheRoute(t *testing.T) {
	runPredicateCases(t, standInSDK(t, "GET /widgets/:/events"), []predicateCase{
		{name: "sent", predicate: NoMethodFor{"GET /widgets/:/events"}},
		{name: "not sent", predicate: NoMethodFor{"POST /widgets/:/events"}, want: true},
		{name: "no verb", predicate: NoMethodFor{"/widgets"}, wantErr: "is not spelled"},
		{name: "lower case verb", predicate: NoMethodFor{"get /widgets"}, wantErr: "is not spelled"},
		{name: "a named identifier", predicate: NoMethodFor{"GET /widgets/:id/events"}, wantErr: `each identifier as ":"`},
		{name: "a named last identifier", predicate: NoMethodFor{"GET /widgets/:/events/:event_id"}, wantErr: `each identifier as ":"`},
	})
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "no route read", predicate: NoMethodFor{"POST /widgets/:/events"}, wantErr: "no route of the pinned client-go was read"},
	})
}

// TestTakesNoOptions_Methods_HoldWhileNoParameterIsAClientGoStruct verifies
// the method lookup and that only a struct client-go declares, through a
// pointer or not, counts as options: another package's struct, a named type
// that is not a struct and a scalar do not.
func TestTakesNoOptions_Methods_HoldWhileNoParameterIsAClientGoStruct(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "no struct", predicate: TakesNoOptions{"Widgets.Archive"}, want: true},
		{name: "structs of others", predicate: TakesNoOptions{"Widgets.Mixed"}, want: true},
		{name: "takes options", predicate: TakesNoOptions{"Widgets.List"}},
		{name: "not spelled as a method", predicate: TakesNoOptions{"Widgets"}, wantErr: "is not spelled"},
		{name: "unknown service", predicate: TakesNoOptions{"Gadgets.List"}, wantErr: "declares no service Gadgets"},
		{name: "service that is no interface", predicate: TakesNoOptions{"Broken.List"}, wantErr: "is not an interface"},
		{name: "unknown method", predicate: TakesNoOptions{"Widgets.Gone"}, wantErr: "has no method Gone"},
	})
}

// TestDecodesInto_Methods_HoldWhileTheFirstResultIsSpelledAsDeclared
// verifies the answer's spelling, slice and pointer included, and that a
// method answering nothing is an error.
func TestDecodesInto_Methods_HoldWhileTheFirstResultIsSpelledAsDeclared(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "slice", predicate: DecodesInto{"Widgets.List", "[]*Inner"}, want: true},
		{name: "pointer", predicate: DecodesInto{"Widgets.Mixed", "*Inner"}, want: true},
		{name: "other answer", predicate: DecodesInto{"Widgets.List", "*Inner"}},
		{name: "answers nothing", predicate: DecodesInto{"Widgets.Ping", "*Inner"}, wantErr: "answers nothing"},
		{name: "unknown method", predicate: DecodesInto{"Widgets.Gone", "*Inner"}, wantErr: "has no method Gone"},
	})
}

// TestTagName_StandInFields_HoldWhileTheEncoderWritesAnotherName verifies
// the predicate behind the misspelled and mistagged parameters: it holds
// while the encoder writes the field under another name or not at all.
func TestTagName_StandInFields_HoldWhileTheEncoderWritesAnotherName(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "another name", predicate: TagName{"Options", "Format", EncoderQuery, "response_format"}, want: true},
		{name: "the wanted name", predicate: TagName{"Options", "Format", EncoderQuery, "format"}},
		{name: "go name for the encoder without a tag", predicate: TagName{"Options", "Format", EncoderJSON, "Format"}},
		{name: "skipped", predicate: TagName{"Options", "Skipped", EncoderJSON, "skipped"}, want: true},
		{name: "unknown encoder", predicate: TagName{"Options", "Format", "xml", "format"}, wantErr: `encoder "xml"`},
		{name: "unknown field", predicate: TagName{"Options", "Gone", EncoderQuery, "gone"}, wantErr: "has no field Gone"},
	})
}

// TestNotNullable_StandInFields_HoldWhileTheFieldCannotSendNull verifies
// the predicate behind every null a body edit writes: a nilable field tagged
// omitempty, a value that is never nil and a function, which encoding/json
// refuses, cannot send null; a nilable field without omitempty and
// client-go's Nullable can, and another package's Nullable is only a map.
func TestNotNullable_StandInFields_HoldWhileTheFieldCannotSendNull(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "pointer with omitempty", predicate: NotNullable{"Options", "Omitted"}, want: true},
		{name: "never nil", predicate: NotNullable{"Options", "Always"}, want: true},
		{name: "a client-go type that is not Nullable", predicate: NotNullable{"Options", "Kind"}, want: true},
		{name: "interface with omitempty", predicate: NotNullable{"Options", "Err"}, want: true},
		{name: "skipped", predicate: NotNullable{"Options", "Skipped"}, want: true},
		{name: "pointer without omitempty", predicate: NotNullable{"Options", "Untagged"}},
		{name: "slice without omitempty", predicate: NotNullable{"Options", "Bare"}},
		{name: "function, which encoding/json never encodes", predicate: NotNullable{"Options", "Fn"}, want: true},
		{name: "client-go Nullable", predicate: NotNullable{"Options", "Null"}},
		{name: "another package's Nullable", predicate: NotNullable{"Options", "ForeignNull"}},
		{name: "unknown field", predicate: NotNullable{"Options", "Gone"}, wantErr: "has no field Gone"},
	})
}

// TestNoEmptyList_StandInFields_HoldWhileTheFieldCannotSendAnEmptyList
// verifies the predicate behind every [] a body edit writes: a slice tagged
// omitempty cannot send one, a slice without it and a pointer to a slice
// can, and a field that is not a list is an error.
func TestNoEmptyList_StandInFields_HoldWhileTheFieldCannotSendAnEmptyList(t *testing.T) {
	runPredicateCases(t, standInSDK(t), []predicateCase{
		{name: "slice with omitempty", predicate: NoEmptyList{"Options", "List"}, want: true},
		{name: "skipped slice", predicate: NoEmptyList{"Options", "Hidden"}, want: true},
		{name: "slice without omitempty", predicate: NoEmptyList{"Options", "Bare"}},
		{name: "pointer to a slice", predicate: NoEmptyList{"Options", "PtrList"}},
		{name: "pointer to no slice", predicate: NoEmptyList{"Options", "Omitted"}, wantErr: "is not a list"},
		{name: "scalar", predicate: NoEmptyList{"Options", "Scalar"}, wantErr: "is not a list"},
		{name: "unknown field", predicate: NoEmptyList{"Options", "Gone"}, wantErr: "has no field Gone"},
	})
}

// TestPredicateString_EachKind_SpellsWhatItNames verifies the spelling a
// report prints, one per kind, so two predicates on one entry read apart.
func TestPredicateString_EachKind_SpellsWhatItNames(t *testing.T) {
	cases := []struct {
		predicate Predicate
		want      string
	}{
		{WritesAlways{"T", "F", EncoderJSON}, "WritesAlways{T.F, json}"},
		{LacksKey{"T", "k"}, "LacksKey{T, k}"},
		{FieldKind{"T", "F", "*int64"}, "FieldKind{T.F, *int64}"},
		{NoMethodFor{"GET /x"}, "NoMethodFor{GET /x}"},
		{TakesNoOptions{"S.M"}, "TakesNoOptions{S.M}"},
		{DecodesInto{"S.M", "*T"}, "DecodesInto{S.M, *T}"},
		{TagName{"T", "F", EncoderQuery, "k"}, "TagName{T.F, url not k}"},
		{NotNullable{"T", "F"}, "NotNullable{T.F}"},
		{NoEmptyList{"T", "F"}, "NoEmptyList{T.F}"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.predicate.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPredicateSubject_EachKind_NamesWhatItIsAbout verifies what a
// predicate is about, which is what the register row an entry waits on has
// to mention: the struct a field or key predicate names, the method a
// method predicate names and the route a route predicate names.
func TestPredicateSubject_EachKind_NamesWhatItIsAbout(t *testing.T) {
	cases := []struct {
		predicate Predicate
		want      string
	}{
		{WritesAlways{"T", "F", EncoderJSON}, "T"},
		{LacksKey{"T", "k"}, "T"},
		{FieldKind{"T", "F", "*int64"}, "T"},
		{NoMethodFor{"GET /x"}, "GET /x"},
		{TakesNoOptions{"S.M"}, "S.M"},
		{DecodesInto{"S.M", "*T"}, "S.M"},
		{TagName{"T", "F", EncoderQuery, "k"}, "T"},
		{NotNullable{"T", "F"}, "T"},
		{NoEmptyList{"T", "F"}, "T"},
	}
	for _, tc := range cases {
		t.Run(tc.predicate.String(), func(t *testing.T) {
			if got := tc.predicate.Subject(); got != tc.want {
				t.Errorf("Subject() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBodyKey_Predicates_AnswerTheJSONKeyOfTheFieldTheyName verifies how a
// body edit's key is tied to a predicate: a field predicate answers the key
// encoding/json writes its field under, and a predicate that names no field,
// a field the SDK lacks or a field encoding/json skips answers none.
func TestBodyKey_Predicates_AnswerTheJSONKeyOfTheFieldTheyName(t *testing.T) {
	sdk := standInSDK(t)
	cases := []struct {
		name      string
		predicate Predicate
		want      string
		wantOK    bool
	}{
		{name: "tagged", predicate: NotNullable{"Options", "Omitted"}, want: "omitted", wantOK: true},
		{name: "every field predicate", predicate: WritesAlways{"Options", "Always", EncoderQuery}, want: "always", wantOK: true},
		{name: "untagged", predicate: FieldKind{"Options", "Untagged", "*string"}, want: "Untagged", wantOK: true},
		{name: "skipped", predicate: NoEmptyList{"Options", "Hidden"}},
		{name: "unknown field", predicate: TagName{"Options", "Gone", EncoderJSON, "gone"}},
		{name: "no field", predicate: LacksKey{"Options", "always"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := BodyKey(tc.predicate, sdk)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("BodyKey(%s) = %q, %v, want %q, %v", tc.predicate, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestEvaluate_Predicates_CarryTheVerdictAndTheError verifies a verdict: a
// predicate that holds, one that does not, and one that names something the
// SDK lacks, which never holds.
func TestEvaluate_Predicates_CarryTheVerdictAndTheError(t *testing.T) {
	sdk := standInSDK(t)
	cases := []struct {
		name      string
		predicate Predicate
		want      Verdict
	}{
		{name: "holds", predicate: LacksKey{"Options", "gone"}, want: Verdict{Predicate: "LacksKey{Options, gone}", Holds: true}},
		{name: "does not hold", predicate: LacksKey{"Options", "always"}, want: Verdict{Predicate: "LacksKey{Options, always}"}},
		{name: "names what the SDK lacks", predicate: LacksKey{"Gone", "always"}, want: Verdict{Predicate: "LacksKey{Gone, always}", Error: "client-go declares no type Gone"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(tc.predicate, sdk); got != tc.want {
				t.Errorf("Evaluate = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestDeclaresMethod_Keys_AreHeldToTheServiceInterfaces verifies the check a
// replaced method is held to.
func TestDeclaresMethod_Keys_AreHeldToTheServiceInterfaces(t *testing.T) {
	sdk := standInSDK(t)
	if err := sdk.DeclaresMethod("Widgets.List"); err != nil {
		t.Errorf("DeclaresMethod(Widgets.List) = %v, want nil", err)
	}
	if err := sdk.DeclaresMethod("Widgets.Gone"); err == nil {
		t.Error("DeclaresMethod(Widgets.Gone) = nil, want an error")
	}
}
