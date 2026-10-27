// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// declarationsSDKSource is a stand-in client-go for the declarations: a
// widget with a size as a number and as a pointer, a name and an owner, a
// gadget with a size of its own, and the details of a widget. A tilde stands
// for a backtick.
const declarationsSDKSource = `package gitlab

type User struct {
	ID int64 ~json:"id"~
}

type Widget struct {
	Size  int64  ~json:"size"~
	Name  string ~json:"name"~
	Ptr   *int64 ~json:"ptr"~
	Owner *User  ~json:"owner"~
}

type Gadget struct {
	Size int64 ~json:"size"~
}

type Details struct {
	ID int64 ~json:"id"~
}
`

// declarationsSDK type-checks the stand-in client-go.
func declarationsSDK(t *testing.T) waits.SDK {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "gitlab.go", strings.ReplaceAll(declarationsSDKSource, "~", "`"), 0)
	if err != nil {
		t.Fatalf("parse the stand-in: %v", err)
	}
	pkg, err := (&types.Config{}).Check("gitlab", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("check the stand-in: %v", err)
	}
	return waits.SDK{Types: pkg}
}

// declarationsJudging is the judging of a captured type the stand-in pairs
// with the widget, the gadget and the details, which reads an owner's avatar
// and a fresh key no struct models.
func declarationsJudging(t *testing.T) captureJudging {
	t.Helper()
	return captureJudging{
		sdk:   declarationsSDK(t),
		pairs: map[string][]string{"local.extra": {"Details", "Gadget", "Widget"}},
		lacks: map[string][]waits.Lack{"local.extra": {
			{Key: "fresh", Owner: "Widget", Name: "fresh"},
			{Key: "owner.avatar", Owner: "User", Name: "avatar"},
		}},
	}
}

// declarationsKeys are the keys the captured type shares: a size the widget
// and the gadget both model as a number, a name the widget models, and the
// owner's id, read below an object both read.
func declarationsKeys() []capturedKey {
	return []capturedKey{
		{
			Type: "local.extra", Key: "name", Captured: "string", Pairs: []string{"Widget"}, Status: captureRedundant,
			Models: []waits.Shared{{Key: "name", Owner: "Widget", Field: "Name", Kind: "string", Captured: "string"}},
		},
		{
			Type: "local.extra", Key: "owner.id", Captured: "int64", Pairs: []string{"Widget"}, Status: captureRedundant,
			Models: []waits.Shared{{Key: "owner.id", Owner: "User", Field: "ID", Kind: "int64", Captured: "int64"}},
		},
		{
			Type: "local.extra", Key: "size", Captured: "*int64", Pairs: []string{"Gadget", "Widget"}, Status: captureRedundant,
			Models: []waits.Shared{
				{Key: "size", Owner: "Gadget", Field: "Size", Kind: "int64", Captured: "*int64"},
				{Key: "size", Owner: "Widget", Field: "Size", Kind: "int64", Captured: "*int64"},
			},
		},
	}
}

// Predicates the cases share.
var (
	widgetSize   = waits.FieldKind{Type: "Widget", Field: "Size", Kind: "int64"}
	gadgetSize   = waits.FieldKind{Type: "Gadget", Field: "Size", Kind: "int64"}
	widgetName   = waits.FieldKind{Type: "Widget", Field: "Name", Kind: "string"}
	detailsLacks = waits.LacksKey{Type: "Details", Key: "name"}
)

// TestJudgeCaptures_EachDeclaration_IsHeldToWhatItsCategoryRequires verifies
// how a declaration answers the keys it is written on, and what is wrong
// with one: a category the table does not define, a key no capture shares,
// a key two declarations answer, and each category's requirement, met and
// missed. A key two client-go structs model needs a FieldKind for each, a
// null or a presence needs a capture read as a pointer and a field that is
// not one, a sibling pairing needs a LacksKey on a struct the type is paired
// with that does not model the key, written on the key or an object above
// it, and an object needs to be written above the keys it answers and a
// LacksKey naming a key the captured object reads there and client-go does
// not model.
func TestJudgeCaptures_EachDeclaration_IsHeldToWhatItsCategoryRequires(t *testing.T) {
	needs := func(category, key string) string {
		return "capture declaration local.extra." + key + ": " + category + " needs " + captureCategories[category].lacks
	}
	cases := []struct {
		name      string
		declared  []captureDeclaration
		wantKeys  []string
		wantStale []string
	}{
		{
			name:      "a category the table does not define",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "size", Category: "kept-for-ever"}},
			wantKeys:  []string{"name redundant_capture", "owner.id redundant_capture", "size redundant_capture"},
			wantStale: []string{`capture declaration local.extra.size: category "kept-for-ever" is not one the table defines`},
		},
		{
			name:      "a key no capture shares",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "fresh", Category: keptForValue}, {Type: "local.other", Key: "size", Category: keptForValue}},
			wantKeys:  []string{"name redundant_capture", "owner.id redundant_capture", "size redundant_capture"},
			wantStale: []string{"capture declaration local.extra.fresh answers no key a capture shares with the client-go struct beside it", "capture declaration local.other.size answers no key a capture shares with the client-go struct beside it"},
		},
		{
			name: "a null kept for every struct that models it",
			declared: []captureDeclaration{
				{Type: "local.extra", Key: "size", Category: keptForNull, Predicates: []waits.Predicate{widgetSize, gadgetSize}},
				{Type: "local.extra", Key: "size", Category: keptForPresence, Predicates: []waits.Predicate{widgetSize, gadgetSize}},
			},
			wantKeys:  []string{"name redundant_capture", "owner.id redundant_capture", "size kept kept-for-null size"},
			wantStale: []string{"capture declaration local.extra.size answers size, which the declaration on size answers too"},
		},
		{
			name: "a null kept for one of the two structs that model it, the other named with a pointer",
			declared: []captureDeclaration{{Type: "local.extra", Key: "size", Category: keptForNull, Predicates: []waits.Predicate{
				widgetSize, waits.FieldKind{Type: "Gadget", Field: "Size", Kind: "*int64"},
			}}},
			wantKeys:  []string{"name redundant_capture", "owner.id redundant_capture", "size stale_predicate kept-for-null size"},
			wantStale: []string{needs(keptForNull, "size")},
		},
		{
			name:      "a null of a key the capture does not read as a pointer",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "name", Category: keptForNull, Predicates: []waits.Predicate{widgetName}}},
			wantKeys:  []string{"name kept kept-for-null name", "owner.id redundant_capture", "size redundant_capture"},
			wantStale: []string{needs(keptForNull, "name")},
		},
		{
			name:      "a presence of a key the capture does not read as a pointer",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "name", Category: keptForPresence, Predicates: []waits.Predicate{widgetName}}},
			wantKeys:  []string{"name kept kept-for-presence name", "owner.id redundant_capture", "size redundant_capture"},
			wantStale: []string{needs(keptForPresence, "name")},
		},
		{
			name: "a presence kept on a field held to a pointer, and to another field",
			declared: []captureDeclaration{{Type: "local.extra", Key: "size", Category: keptForPresence, Predicates: []waits.Predicate{
				waits.FieldKind{Type: "Widget", Field: "Ptr", Kind: "*int64"}, waits.FieldKind{Type: "Widget", Field: "Size", Kind: "*int64"}, gadgetSize,
			}}},
			wantKeys:  []string{"name redundant_capture", "owner.id redundant_capture", "size stale_predicate kept-for-presence size"},
			wantStale: []string{needs(keptForPresence, "size")},
		},
		{
			name:     "a value kept, and one the SDK no longer types so",
			declared: []captureDeclaration{{Type: "local.extra", Key: "name", Category: keptForValue, Predicates: []waits.Predicate{widgetName}}, {Type: "local.extra", Key: "owner.id", Category: keptForValue, Predicates: []waits.Predicate{waits.FieldKind{Type: "User", Field: "ID", Kind: "string"}}}},
			wantKeys: []string{"name kept kept-for-value name", "owner.id obsolete_capture kept-for-value owner.id", "size redundant_capture"},
		},
		{
			name:      "a value kept without a FieldKind",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "name", Category: keptForValue, Predicates: []waits.Predicate{detailsLacks}}},
			wantKeys:  []string{"name kept kept-for-value name", "owner.id redundant_capture", "size redundant_capture"},
			wantStale: []string{needs(keptForValue, "name")},
		},
		{
			name: "a sibling pairing on the key, and on a key below the object the sibling lacks",
			declared: []captureDeclaration{
				{Type: "local.extra", Key: "name", Category: keptForSiblingPairing, Predicates: []waits.Predicate{widgetName, detailsLacks}},
				{Type: "local.extra", Key: "owner.id", Category: keptForSiblingPairing, Predicates: []waits.Predicate{waits.LacksKey{Type: "Details", Key: "owner"}}},
			},
			wantKeys: []string{"name kept kept-for-sibling-pairing name", "owner.id kept kept-for-sibling-pairing owner.id", "size redundant_capture"},
		},
		{
			name: "a sibling pairing on a struct that models the key, one the type is not paired with, and a key not above it",
			declared: []captureDeclaration{
				{Type: "local.extra", Key: "name", Category: keptForSiblingPairing, Predicates: []waits.Predicate{waits.LacksKey{Type: "Widget", Key: "name"}, waits.LacksKey{Type: "Project", Key: "name"}}},
				{Type: "local.extra", Key: "size", Category: keptForSiblingPairing, Predicates: []waits.Predicate{waits.LacksKey{Type: "Details", Key: "sizes"}}},
			},
			wantKeys:  []string{"name obsolete_capture kept-for-sibling-pairing name", "owner.id redundant_capture", "size kept kept-for-sibling-pairing size"},
			wantStale: []string{needs(keptForSiblingPairing, "name"), needs(keptForSiblingPairing, "size")},
		},
		{
			name: "an object read whole for a key client-go does not model",
			declared: []captureDeclaration{{Type: "local.extra", Key: "owner", Category: keptWithItsObject, Predicates: []waits.Predicate{
				widgetName, waits.LacksKey{Type: "Widget", Key: "avatar"}, waits.LacksKey{Type: "User", Key: "email"}, waits.LacksKey{Type: "User", Key: "avatar"},
			}}},
			wantKeys: []string{"name redundant_capture", "owner.id kept kept-with-its-object owner", "size redundant_capture"},
		},
		{
			name: "an object written on the keys it answers",
			declared: []captureDeclaration{
				{Type: "local.extra", Key: "owner.id", Category: keptWithItsObject, Predicates: []waits.Predicate{waits.LacksKey{Type: "User", Key: "avatar"}}},
				{Type: "local.extra", Key: "size", Category: keptWithItsObject, Predicates: []waits.Predicate{waits.LacksKey{Type: "Widget", Key: "fresh"}}},
			},
			wantKeys:  []string{"name redundant_capture", "owner.id kept kept-with-its-object owner.id", "size kept kept-with-its-object size"},
			wantStale: []string{needs(keptWithItsObject, "owner.id"), needs(keptWithItsObject, "size")},
		},
		{
			name:      "an object whose lack the captured object does not read",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "owner", Category: keptWithItsObject, Predicates: []waits.Predicate{waits.LacksKey{Type: "User", Key: "email"}}}},
			wantKeys:  []string{"name redundant_capture", "owner.id kept kept-with-its-object owner", "size redundant_capture"},
			wantStale: []string{needs(keptWithItsObject, "owner")},
		},
		{
			name: "an object whose lack is read outside it, or on another struct",
			declared: []captureDeclaration{{Type: "local.extra", Key: "owner", Category: keptWithItsObject, Predicates: []waits.Predicate{
				waits.LacksKey{Type: "Widget", Key: "fresh"}, waits.LacksKey{Type: "Widget", Key: "avatar"},
			}}},
			wantKeys:  []string{"name redundant_capture", "owner.id kept kept-with-its-object owner", "size redundant_capture"},
			wantStale: []string{needs(keptWithItsObject, "owner")},
		},
		{
			name:      "a category that answers no object, written on one",
			declared:  []captureDeclaration{{Type: "local.extra", Key: "owner", Category: keptForValue, Predicates: []waits.Predicate{widgetName}}},
			wantKeys:  []string{"name redundant_capture", "owner.id redundant_capture", "size redundant_capture"},
			wantStale: []string{"capture declaration local.extra.owner answers no key a capture shares with the client-go struct beside it"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys := declarationsKeys()
			stale := judgeCaptures(declarationsJudging(t), keys, tc.declared)
			var got []string
			for _, key := range keys {
				got = append(got, strings.TrimSpace(key.Key+" "+key.Status+" "+key.Category+" "+key.Declared))
			}
			if !slices.Equal(got, tc.wantKeys) {
				t.Errorf("keys = %q, want %q", got, tc.wantKeys)
			}
			if !slices.Equal(stale, tc.wantStale) {
				t.Errorf("stale = %q, want %q", stale, tc.wantStale)
			}
		})
	}
}

// TestCaptureDeclarations_EveryEntry_NamesAKnownCategoryAndAReason verifies
// the real table is well formed on its own, whatever the tree: each entry
// has a category the table defines, a reason, and one declaration per key.
func TestCaptureDeclarations_EveryEntry_NamesAKnownCategoryAndAReason(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range captureDeclarations {
		if _, defined := captureCategories[declaration.Category]; !defined || declaration.Reason == "" || len(declaration.Predicates) == 0 {
			t.Errorf("%s: category %q, reason %q and %d predicates, want a defined category, a reason and a predicate", declaration.id(), declaration.Category, declaration.Reason, len(declaration.Predicates))
		}
		if seen[declaration.id()] {
			t.Errorf("%s is declared twice", declaration.id())
		}
		seen[declaration.id()] = true
	}
}
