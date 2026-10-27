// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/types"
	"slices"
	"testing"
)

// sharedPermissionSource is a tool package that declares an input element
// another package names through an alias, the shape of protectedenvdata.
const sharedPermissionSource = `package permissiondata

type SharedPermission struct {
	UserID int64 ~json:"user_id"~
}
`

// nestedSource builds each nested struct of the pairing SDK in one of the
// shapes the pairing reads its source from.
const nestedSource = `package widgets

import (
	gl "` + fixtureSDKPath + `"
	"example.com/x/internal/tools/permissiondata"
)

type PermissionInput struct {
	UserID int64 ~json:"user_id"~
}

type ProtectInput struct {
	Name    string            ~json:"name"~
	Allowed []PermissionInput ~json:"allowed"~
}

// Protect reads each element's field in the nested literal; the handler's
// own input is paired with the options it sends, never with the nested
// struct.
func Protect(c *gl.Client, in ProtectInput) error {
	allowed := make([]*gl.PermissionOptions, 0, len(in.Allowed))
	for _, p := range in.Allowed {
		allowed = append(allowed, &gl.PermissionOptions{UserID: &p.UserID})
	}
	return c.Widgets.Protect(1, &gl.ProtectOptions{Name: &in.Name, Allowed: &allowed})
}

type AssignedInput struct {
	UserID int64 ~json:"user_id"~
}

// assigned fills the bound literal field by field after building it.
func assigned(items []AssignedInput) []*gl.PermissionOptions {
	var out []*gl.PermissionOptions
	for _, item := range items {
		opt := &gl.PermissionOptions{}
		opt.UserID = &item.UserID
		opt = &gl.PermissionOptions{UserID: opt.UserID}
		out = append(out, opt)
	}
	return out
}

type StarInput struct {
	UserID int64 ~json:"user_id"~
}

// starred fills the literal through the pointer it is bound to.
func starred(item StarInput) *gl.PermissionOptions {
	ptr := &gl.PermissionOptions{}
	(*ptr).UserID = &item.UserID
	return ptr
}

type EqualInput struct {
	UserID int64 ~json:"user_id"~
}

// equal binds the literal with = rather than :=.
func equal(item EqualInput) *gl.PermissionOptions {
	var opt *gl.PermissionOptions
	opt = &gl.PermissionOptions{}
	opt.UserID = &item.UserID
	return opt
}

type FallbackInput struct {
	Value int64 ~json:"value"~
}

// fallback builds the literal from a local computed off its input, which is
// read through the local.
func fallback(in FallbackInput) *gl.Level {
	doubled := in.Value * 2
	return &gl.Level{Value: &doubled}
}

type UploadInput struct {
	Value int64 ~json:"value"~
}

type ExportInput struct {
	Upload *UploadInput ~json:"upload"~
	Flat   int64        ~json:"flat"~
}

type OtherInput struct {
	Value int64 ~json:"value"~
}

// export reads the nested input, or the flat value it deprecates, into a
// local, and the local into itself, before it builds the literal with a
// function of it; the element it assigns names no variable, so what it
// assigns there is no value of the literal.
func export(in ExportInput, other []OtherInput) *gl.Level {
	value := in.Flat
	if in.Upload != nil {
		value = in.Upload.Value
	}
	value = max(value, 0)
	other[0] = OtherInput{Value: other[0].Value}
	return &gl.Level{Value: toPointer(value)}
}

// fromInts builds the literal out of a list of integers, in a function with
// no input of its own, so there is nothing to pair it with.
func fromInts(values []int64) []*gl.Level {
	out := make([]*gl.Level, 0, len(values))
	for _, value := range values {
		out = append(out, &gl.Level{Value: &value})
	}
	return out
}

type MultiInput struct {
	First  int64 ~json:"first"~
	Second int64 ~json:"second"~
}

func split() (*gl.Level, error) { return &gl.Level{}, nil }

// multi binds two literals in one statement and assigns in the shapes that
// bind nothing.
func multi(in MultiInput) []*gl.Level {
	first, second := &gl.Level{}, &gl.Level{}
	first.Value = &in.First
	second.Value = &in.Second
	levels := []*gl.Level{first, second, nil}
	levels[2] = &gl.Level{}
	levels[2].Value = &in.First
	_, _ = split()
	count := 0
	_ = count
	return levels
}

type AliasInput = permissiondata.SharedPermission

// aliased reads an element spelled through an alias, by value and through a
// pointer.
func aliased(items []AliasInput, ptr *AliasInput) []*gl.PermissionOptions {
	out := []*gl.PermissionOptions{{UserID: &ptr.UserID}}
	for _, item := range items {
		out = append(out, &gl.PermissionOptions{UserID: &item.UserID})
	}
	return out
}

type MethodInput struct {
	UserID int64 ~json:"user_id"~
}

func (m MethodInput) id() int64 { return m.UserID }

func toPointer(value int64) *int64 { return &value }

// methodRead reads a method of the input, a field of a client-go struct and
// a nested client-go literal, none of which is a field of a tool struct read
// in place, so both literals fall back to the function's input.
func methodRead(m MethodInput, other *gl.PermissionOptions) *gl.PermissionOptions {
	return &gl.PermissionOptions{UserID: toPointer(m.id()), Level: &gl.Level{Value: other.UserID}}
}

type PositionalInput struct {
	Value int64 ~json:"value"~
}

// positional builds the literal without keys.
func positional(in PositionalInput) gl.Level { return gl.Level{&in.Value} }

type ElementInput struct {
	Value int64 ~json:"value"~
}

// element stores the literal in an element, which names no variable, so the
// assignment to the element's field is not read as the literal's, and the
// function has no input to fall back on.
func element(items []ElementInput) []*gl.Level {
	out := make([]*gl.Level, 1)
	out[0] = &gl.Level{}
	out[0].Value = &items[0].Value
	return out
}

type WrappedInput struct {
	Value int64 ~json:"value"~
}

// wrapped reads the field inside a literal of its own, which is entered.
func wrapped(in WrappedInput) *gl.Level { return &gl.Level{Value: []*int64{&in.Value}[0]} }

type AnonInput struct {
	Value int64 ~json:"value"~
}

// anon reads a field of a struct that has no name.
func anon(in AnonInput) *gl.Level {
	holder := struct{ Value int64 }{Value: in.Value}
	return &gl.Level{Value: &holder.Value}
}
`

// TestCollectNestedInputs_EveryShape_PairsTheNestedStructWithTheStructItsValuesComeFrom
// verifies where a nested literal's input is found: the struct whose field a
// value of the literal reads, there, in an assignment to the variable the
// literal is bound to (by := or =, through a pointer, through an alias, by
// value or behind a pointer) or in what the function assigns a local the
// value reads (every struct each assignment reads, a local that refers to
// itself read once), or, when the literal reads none, the input of the
// function it is in. A literal in a function with no input that reads no
// field pairs with nothing, and the handler's own input is never paired with
// a nested struct.
func TestCollectNestedInputs_EveryShape_PairsTheNestedStructWithTheStructItsValuesComeFrom(t *testing.T) {
	pkgs := checkedTree(t, pairingSDK,
		toolSource{path: "example.com/x/internal/tools/permissiondata", source: sharedPermissionSource},
		toolSource{path: "example.com/x/internal/tools/widgets", source: nestedSource},
	)

	pairs := map[[2]string]structPair{}
	collectNestedInputs(pkgs[1], pairs)

	var got []string
	for key, pair := range pairs {
		got = append(got, key[0]+" "+key[1]+" "+pair.via)
	}
	slices.Sort(got)
	want := []string{
		"AnonInput Level nested",
		"AssignedInput PermissionOptions nested",
		"EqualInput PermissionOptions nested",
		"ExportInput Level nested",
		"FallbackInput Level nested",
		"MethodInput Level nested",
		"MethodInput PermissionOptions nested",
		"MultiInput Level nested",
		"PermissionInput PermissionOptions nested",
		"PositionalInput Level nested",
		"SharedPermission PermissionOptions nested",
		"StarInput PermissionOptions nested",
		"UploadInput Level nested",
		"WrappedInput Level nested",
	}
	if !slices.Equal(got, want) {
		t.Errorf("collectNestedInputs = %v, want %v", got, want)
	}
	shared := pairs[[2]string{"SharedPermission", "PermissionOptions"}]
	if shared.mcpNamed == nil || shared.mcpNamed.Obj().Pkg().Name() != "permissiondata" {
		t.Errorf("the alias pair names %v, want the struct the alias stands for", shared.mcpNamed)
	}

	direct, _ := discoverPairs(pkgs[1])
	if _, paired := direct[[2]string{"ProtectInput", "PermissionOptions"}]; paired {
		t.Error("the handler's input was paired with the nested struct it never fills")
	}
	if _, paired := direct[[2]string{"ProtectInput", "ProtectOptions"}]; !paired {
		t.Error("the handler's input was not paired with the options it sends")
	}
}

// TestDisjointPhantomInput_ANestedPair_IsNeverAPhantom verifies a nested
// pair is kept whatever its overlap: it is paired with the struct its values
// are read from, so an input that carries the fields under other names is a
// rename to adjudicate, where the same disjoint pair found by a handler's
// literal beside an overlapping one is a phantom the diff drops.
func TestDisjointPhantomInput_ANestedPair_IsNeverAPhantom(t *testing.T) {
	mcp := makeStruct(structField{"Who", "who", tInt}, structField{"Name", "name", tString})
	disjoint := makeStructWithTags(taggedField{name: "UserID", tag: `url:"user_id"`, goType: tInt})
	overlapping := makeStructWithTags(taggedField{name: "Name", tag: `url:"name"`, goType: tString})
	others := func(via string) map[[2]string]structPair {
		return map[[2]string]structPair{
			{"Input", "Disjoint"}:    {mcpName: "Input", mcpType: mcp, sdkName: "v3.Disjoint", sdkType: disjoint, sdkURLTags: true, via: via},
			{"Input", "Overlapping"}: {mcpName: "Input", mcpType: mcp, sdkName: "v3.Overlapping", sdkType: overlapping, sdkURLTags: true},
		}
	}

	for _, tc := range []struct {
		via     string
		phantom bool
	}{{via: "", phantom: true}, {via: ViaCaller, phantom: true}, {via: ViaNested, phantom: false}} {
		t.Run("via "+tc.via, func(t *testing.T) {
			all := others(tc.via)
			if got := disjointPhantomInput(all[[2]string{"Input", "Disjoint"}], all); got != tc.phantom {
				t.Errorf("disjointPhantomInput = %v, want %v", got, tc.phantom)
			}
		})
	}
}

// TestUnaliasPointer_EachSpelling_IsReadAsTheStructItNames verifies the
// unwrapping the nested pairing reads a value's type through: an alias, a
// pointer to one, and a type spelled without either, which is returned as it
// is.
func TestUnaliasPointer_EachSpelling_IsReadAsTheStructItNames(t *testing.T) {
	target := namedStruct(types.NewPackage("example.com/x/internal/tools/permissiondata", "permissiondata"), "SharedPermission", makeStruct())
	alias := types.NewAlias(types.NewTypeName(0, types.NewPackage("example.com/x/internal/tools/widgets", "widgets"), "AliasInput", nil), target)

	if got := unaliasPointer(alias); got != types.Type(target) {
		t.Errorf("unaliasPointer(alias) = %v, want %v", got, target)
	}
	if got, isPtr := unaliasPointer(types.NewPointer(alias)).(*types.Pointer); !isPtr || got.Elem() != types.Type(target) {
		t.Errorf("unaliasPointer(*alias) = %v, want a pointer to %v", got, target)
	}
	if got := unaliasPointer(types.Typ[types.Int64]); got != types.Type(types.Typ[types.Int64]) {
		t.Errorf("unaliasPointer(int64) = %v, want it unchanged", got)
	}
}

// TestCollectPairs_Repository_PairsANestedStructWithTheInputItsElementsComeFrom
// is the witness of the nested pairing on the tree as it stands: these
// structs are built element by element out of a list an input carries, and
// were compared with the handler's input, then dropped as disjoint, or with
// nothing. Each is now compared with the input element it is built from, so
// a field client-go adds to it is a missing input on that element; the
// export's upload destination with the nested input that mirrors it, the
// deprecated flat fields read as one.
func TestCollectPairs_Repository_PairsANestedStructWithTheInputItsElementsComeFrom(t *testing.T) {
	assertRepositoryPairs(t, []string{
		"projectimportexport ScheduleExportUploadInput ScheduleExportUploadOptions",
		"branches BranchPermissionInput BranchPermissionOptions",
		"commits Action CommitActionOptions",
		"featureflags CreateStrategyInput CreateFeatureFlagStrategyOptions",
		"groupprotectedbranches BranchPermissionInput GroupBranchPermissionOptions",
		"groupprotectedenvs DeployAccessLevelInput GroupEnvironmentAccessOptions",
		"groups BranchProtectionDefaultsInput DefaultBranchProtectionDefaultsOptions",
		"groups HookCustomHeaderInput HookCustomHeader",
		"protectedenvs DeployAccessLevelInput EnvironmentAccessOptions",
		"snippets CreateFileInput CreateSnippetFileOptions",
		"tags TagPermission TagsPermissionOptions",
	})
}
