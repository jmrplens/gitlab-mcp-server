// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The stand-in's narrowed method, what the fixtures here declare for it, and the
// action whose input holds its field.
const (
	ownerMethod = "Widgets.GetWidgetOwner"
	ownerParam  = "wid"
	ownerAction = "widget.owner"
	ownerInput  = "widgets.OwnerInput"
	ownerRow    = 7
)

// registerWithOwnerRow is a register whose Summary holds the row the fixtures
// declare, a client-go row beside another project, and rows that are not
// client-go's.
const registerWithOwnerRow = `# Upstream bugs

| # | Project | Issue |
| - | ------- | ----- |
| 6 | go-sdk | another |
| 7 | client-go | the owner's parameter |
| 8 | client-go and gitlab-org/gitlab | both |
| 9 | client-gone | not this project |
`

// ownerDeclared is the table of the stand-in's one called narrowing.
func ownerDeclared() narrowingTable {
	return narrowingTable{
		declared: []narrowingDeclaration{{Method: ownerMethod, Param: ownerParam, Action: ownerAction, UpstreamRow: ownerRow}},
		actions:  map[string]string{ownerAction: ownerInput},
	}
}

// ownerNarrowing builds the section over the stand-in with the tool put through
// the replacements, the register written, and table, and reads the narrowed
// parameter back with what came stale.
func ownerNarrowing(t *testing.T, table narrowingTable, oldnew ...string) (sdkNarrowing, []string) {
	t.Helper()
	root, inputs := identifiersFixture(t, strings.NewReplacer(oldnew...), identifiersRecord())
	writeFiles(t, root, map[string]string{upstreamRegister: registerWithOwnerRow})
	report, stale := analyzeIdentifiers(root, inputs.pkgs, inputs.usage, nil, table)
	for _, narrowing := range report.SDKNarrowed {
		if narrowing.Method == ownerMethod {
			return narrowing, stale
		}
	}
	t.Fatalf("sdk_narrowed holds no %s: %+v", ownerMethod, report.SDKNarrowed)
	return sdkNarrowing{}, nil
}

// The stand-in's handler of the narrowed method, as the tool source spells it,
// and the shapes the tests below replace it with.
const (
	ownerHandler = "type OwnerInput struct{ WidgetID int64 }\n\nfunc Owner(c *gl.Client, input OwnerInput) error {\n\t_, _, err := c.Widgets.GetWidgetOwner(input.WidgetID)\n\treturn err\n}"
	// ownerWidened reads a name into the integer the method takes, as the real
	// handlers do: through a helper, and a second handler reads the same field
	// again and one more of its own, so the list has a duplicate to fold and two
	// fields to sort.
	ownerWidened = `type OwnerInput struct{ WidgetID StringOrInt }

type AnotherInput struct{ Name string }

func Owner(c *gl.Client, input OwnerInput) error {
	_, _, err := c.Widgets.GetWidgetOwner(resolved(c, input.WidgetID))
	return err
}

func OwnerAgain(c *gl.Client, input OwnerInput) error {
	_, _, err := c.Widgets.GetWidgetOwner(resolved(c, input.WidgetID))
	return err
}

func Another(c *gl.Client, input AnotherInput) error {
	_, _, err := c.Widgets.GetWidgetOwner(resolved(c, StringOrInt(input.Name)))
	return err
}

func resolved(c *gl.Client, value StringOrInt) int64 {
	if id := value.Int64(); id > 0 {
		return id
	}
	owner, _, err := c.Widgets.GetWidgetDetails(string(value))
	if err != nil {
		return 0
	}
	return owner.ID
}`
	// ownerThroughValue calls the method through a variable, which the walk does
	// not follow for a method with no parameter of type any.
	ownerThroughValue = "type OwnerInput struct{ WidgetID int64 }\n\nfunc Owner(c *gl.Client, input OwnerInput) error {\n\tget := c.Widgets.GetWidgetOwner\n\t_, _, err := get(input.WidgetID)\n\treturn err\n}"
	// ownerStopped hands the method what a closure's parameter holds, twice from
	// one literal and once from another, so there is a duplicate stop and two
	// stops to sort.
	ownerStopped = "type OwnerInput struct{ WidgetID int64 }\n\nfunc Owner(c *gl.Client, input OwnerInput) error {\n\tfirst := func(zeta int64) { c.Widgets.GetWidgetOwner(zeta); c.Widgets.GetWidgetOwner(zeta) }\n\tsecond := func(alpha int64) { c.Widgets.GetWidgetOwner(alpha) }\n\tfirst(input.WidgetID)\n\tsecond(input.WidgetID)\n\treturn nil\n}"
	// ownerFromAnswer hands the method an integer read off an answer.
	ownerFromAnswer = "type OwnerInput struct{ WidgetID int64 }\n\nfunc Owner(c *gl.Client, input OwnerInput) error {\n\towner, _, err := c.Widgets.GetWidgetDetails(int(input.WidgetID))\n\tif err != nil {\n\t\treturn err\n\t}\n\t_, _, err = c.Widgets.GetWidgetOwner(owner.ID)\n\treturn err\n}"
)

// TestAnalyzeIdentifiers_ACalledNarrowingNoDeclarationAnswers_FailsTheGate is the
// planted defect: client-go narrows the parameter, a handler calls the method, and
// nothing says what a model can send there. The section only listed it before.
func TestAnalyzeIdentifiers_ACalledNarrowingNoDeclarationAnswers_FailsTheGate(t *testing.T) {
	narrowing, stale := ownerNarrowing(t, narrowingTable{})
	if narrowing.Status != narrowingUndeclared || len(stale) != 0 {
		t.Fatalf("status = %q, stale = %q, want undeclared and nothing stale", narrowing.Status, stale)
	}
	var summary reportSummary
	summarizeIdentifiers(&summary, identifiersReport{SDKNarrowed: []sdkNarrowing{narrowing}})
	if summary.clean() {
		t.Errorf("summary = %+v, want an undeclared narrowing to fail the gate", summary)
	}
}

// TestAnalyzeIdentifiers_AHandlerThatReadsANameIntoTheInteger_IsResolved verifies
// the correct case through the whole section: a declaration, a field that takes a
// name and a trace that reaches it across a helper. The inputs are the fields read,
// once each and sorted, and the declaration's action and row reach the report.
func TestAnalyzeIdentifiers_AHandlerThatReadsANameIntoTheInteger_IsResolved(t *testing.T) {
	narrowing, stale := ownerNarrowing(t, ownerDeclared(), ownerHandler, ownerWidened)
	if len(stale) != 0 {
		t.Errorf("stale = %q, want nothing", stale)
	}
	want := sdkNarrowing{
		Method: ownerMethod, Param: ownerParam, Type: "int64", Called: true, Status: narrowingResolved,
		Action: ownerAction, UpstreamRow: ownerRow, Inputs: []string{"widgets.AnotherInput.Name", "widgets.OwnerInput.WidgetID"},
	}
	narrowing.Placeholders = nil
	if !reflect.DeepEqual(narrowing, want) {
		t.Errorf("narrowing = %+v, want %+v", narrowing, want)
	}
}

// TestAnalyzeIdentifiers_ADeclarationThatDoesNotResolve_FailsTheGate verifies a
// declaration is not taken at its word: each of these has one, and each hands the
// method something that is not known to be a name.
func TestAnalyzeIdentifiers_ADeclarationThatDoesNotResolve_FailsTheGate(t *testing.T) {
	t.Run("an input that offers an integer alone", func(t *testing.T) {
		narrowing, stale := ownerNarrowing(t, ownerDeclared())
		if narrowing.Status != narrowingUnresolved || len(stale) != 0 {
			t.Errorf("status = %q, stale = %q, want unresolved and nothing stale", narrowing.Status, stale)
		}
	})
	t.Run("a call through a variable the walk does not follow", func(t *testing.T) {
		narrowing, _ := ownerNarrowing(t, ownerDeclared(), ownerHandler, ownerThroughValue)
		if narrowing.Status != narrowingUnresolved || narrowing.Inputs != nil {
			t.Errorf("status = %q from %q, want unresolved from nothing read", narrowing.Status, narrowing.Inputs)
		}
	})
	t.Run("a trace that stopped", func(t *testing.T) {
		narrowing, _ := ownerNarrowing(t, ownerDeclared(), ownerHandler, ownerStopped)
		want := "widgets.Owner: alpha; widgets.Owner: zeta"
		if narrowing.Status != narrowingUnresolved || narrowing.StoppedAt != want {
			t.Errorf("status = %q, stopped at %q, want unresolved at %q", narrowing.Status, narrowing.StoppedAt, want)
		}
	})
}

// TestAnalyzeIdentifiers_AnIntegerFromAnAnswer_NarrowsNothingAModelSends verifies
// the one called narrowing that needs no declaration: no input reaches the
// parameter. A declaration of it answers nothing, since the action's input hands
// the method nothing.
func TestAnalyzeIdentifiers_AnIntegerFromAnAnswer_NarrowsNothingAModelSends(t *testing.T) {
	narrowing, stale := ownerNarrowing(t, narrowingTable{}, ownerHandler, ownerFromAnswer)
	if narrowing.Status != narrowingNotFromInput || len(stale) != 0 {
		t.Errorf("status = %q, stale = %q, want not_from_input and nothing stale", narrowing.Status, stale)
	}
	narrowing, stale = ownerNarrowing(t, ownerDeclared(), ownerHandler, ownerFromAnswer)
	want := []string{"narrowing declaration " + narrowingKey(ownerMethod, ownerParam) + ": the input of " + ownerAction + " hands the method nothing"}
	if narrowing.Status != narrowingNotFromInput || !slices.Equal(stale, want) {
		t.Errorf("status = %q, stale = %q, want not_from_input and %q", narrowing.Status, stale, want)
	}
}

// TestApplyNarrowingDeclarations_EachDeclaration_IsHeldToWhatItNames verifies each
// way a declaration stops answering anything, with the message that says which,
// and that the parameter it answers still gets its status. A bump that widens a
// method is the first of them, which is what tells the maintainer the lookup can go.
func TestApplyNarrowingDeclarations_EachDeclaration_IsHeldToWhatItNames(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{upstreamRegister: registerWithOwnerRow})
	narrowed := func() []sdkNarrowing {
		return []sdkNarrowing{
			{Method: "A.Called", Param: "id", Called: true},
			{Method: "A.Uncalled", Param: "id"},
		}
	}
	read := map[string]*origins{narrowingKey("A.Called", "id"): {inputs: []string{"a.In.Field"}}}
	holdsName := func(string) bool { return true }
	actions := map[string]string{"a.good": "a.In", "a.other": "a.Other", "a.prefix": "a.I"}
	good := narrowingDeclaration{Method: "A.Called", Param: "id", Action: "a.good", UpstreamRow: ownerRow}

	cases := []struct {
		name     string
		declared []narrowingDeclaration
		want     []string
	}{
		{name: "it holds", declared: []narrowingDeclaration{good}},
		{name: "a row beside another project is client-go's", declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.good", UpstreamRow: 8}}},
		{
			name:     "written twice",
			declared: []narrowingDeclaration{good, good},
			want:     []string{"narrowing declaration A.Called id is declared twice"},
		},
		{
			name:     "client-go no longer narrows it",
			declared: []narrowingDeclaration{good, {Method: "A.Widened", Param: "id", Action: "a.good", UpstreamRow: ownerRow}},
			want:     []string{"narrowing declaration A.Widened id: client-go no longer narrows it, so the lookup in front of it and this row can go"},
		},
		{
			name:     "another parameter of a narrowed method",
			declared: []narrowingDeclaration{good, {Method: "A.Called", Param: "other", Action: "a.good", UpstreamRow: ownerRow}},
			want:     []string{"narrowing declaration A.Called other: client-go no longer narrows it, so the lookup in front of it and this row can go"},
		},
		{
			name:     "no handler calls the method",
			declared: []narrowingDeclaration{{Method: "A.Uncalled", Param: "id", Action: "a.good", UpstreamRow: ownerRow}},
			want:     []string{"narrowing declaration A.Uncalled id: no handler calls the method"},
		},
		{
			name:     "an action the catalog does not hold",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.gone", UpstreamRow: ownerRow}},
			want:     []string{`narrowing declaration A.Called id: the catalog holds no action "a.gone"`},
		},
		{
			name:     "no action",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", UpstreamRow: ownerRow}},
			want:     []string{`narrowing declaration A.Called id: the catalog holds no action ""`},
		},
		{
			name:     "an action whose input hands the method nothing",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.other", UpstreamRow: ownerRow}},
			want:     []string{"narrowing declaration A.Called id: the input of a.other hands the method nothing"},
		},
		{
			name:     "an action whose input is only a prefix of the struct",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.prefix", UpstreamRow: ownerRow}},
			want:     []string{"narrowing declaration A.Called id: the input of a.prefix hands the method nothing"},
		},
		{
			name:     "a row of another project",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.good", UpstreamRow: 6}},
			want:     []string{"narrowing declaration A.Called id: row 6 of " + upstreamRegister + " is not one of client-go's"},
		},
		{
			name:     "a project whose name only starts like client-go",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.good", UpstreamRow: 9}},
			want:     []string{"narrowing declaration A.Called id: row 9 of " + upstreamRegister + " is not one of client-go's"},
		},
		{
			name:     "no row",
			declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.good"}},
			want:     []string{"narrowing declaration A.Called id: row 0 of " + upstreamRegister + " is not one of client-go's"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyNarrowingDeclarations(root, narrowed(), read, narrowingTable{declared: tc.declared, actions: actions}, holdsName)
			if !slices.Equal(got, tc.want) {
				t.Errorf("stale =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// TestApplyNarrowingDeclarations_ARegisterThatCannotBeRead_FailsEveryRowAndSaysSo
// verifies an unreadable register fails the gate rather than excusing what it
// would have checked, and is not read at all when nothing is declared, which is
// what a stand-in tree with no declarations asks of it.
func TestApplyNarrowingDeclarations_ARegisterThatCannotBeRead_FailsEveryRowAndSaysSo(t *testing.T) {
	narrowed := []sdkNarrowing{{Method: "A.Called", Param: "id", Called: true}}
	read := map[string]*origins{narrowingKey("A.Called", "id"): {inputs: []string{"a.In.Field"}}}
	holdsName := func(string) bool { return true }

	table := narrowingTable{
		declared: []narrowingDeclaration{{Method: "A.Called", Param: "id", Action: "a.good", UpstreamRow: ownerRow}},
		actions:  map[string]string{"a.good": "a.In"},
	}
	stale := applyNarrowingDeclarations(t.TempDir(), narrowed, read, table, holdsName)
	if len(stale) != 2 || !strings.HasPrefix(stale[0], "identifiers: reading the upstream register") || !strings.Contains(stale[1], "row 7 of") {
		t.Errorf("stale = %q, want the read's error and the row it could not find", stale)
	}
	if stale = applyNarrowingDeclarations(t.TempDir(), narrowed, read, narrowingTable{}, holdsName); len(stale) != 0 {
		t.Errorf("stale = %q with nothing declared, want the register left unread", stale)
	}
}

// TestNarrowingStatus_EachShape_ComesToOneStatus verifies the status from each
// thing it reads, the declaration and the calls the walk read, taken one at a time.
func TestNarrowingStatus_EachShape_ComesToOneStatus(t *testing.T) {
	holdsName := func(input string) bool { return input == "a.In.Name" }
	name := &origins{inputs: []string{"a.In.Name"}}
	integer := &origins{inputs: []string{"a.In.Name", "a.In.ID"}}
	stopped := &origins{inputs: []string{"a.In.Name"}, stopped: []string{"a.F: x"}}
	fromAnswer := &origins{}
	cases := []struct {
		name     string
		called   bool
		read     *origins
		declared bool
		want     string
	}{
		{name: "uncalled", read: nil, want: narrowingUncalled},
		{name: "uncalled, declared, with a call read", read: name, declared: true, want: narrowingUncalled},
		{name: "no call read", called: true, want: narrowingUndeclared},
		{name: "no call read, declared", called: true, declared: true, want: narrowingUnresolved},
		{name: "read from no input", called: true, read: fromAnswer, want: narrowingNotFromInput},
		{name: "read from no input, declared", called: true, read: fromAnswer, declared: true, want: narrowingNotFromInput},
		{name: "a name and no declaration", called: true, read: name, want: narrowingUndeclared},
		{name: "a name, declared", called: true, read: name, declared: true, want: narrowingResolved},
		{name: "an integer beside a name, declared", called: true, read: integer, declared: true, want: narrowingUnresolved},
		{name: "a stop beside a name", called: true, read: stopped, want: narrowingUndeclared},
		{name: "a stop beside a name, declared", called: true, read: stopped, declared: true, want: narrowingUnresolved},
		{name: "only a stop, declared", called: true, read: &origins{stopped: []string{"a.F: x"}}, declared: true, want: narrowingUnresolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := narrowingStatus(sdkNarrowing{Called: tc.called}, tc.read, tc.declared, holdsName)
			if got != tc.want {
				t.Errorf("narrowingStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestInputHoldsName_EachField_SaysWhetherAModelCanSendAName verifies the reading
// of a traced input against the loaded types: a string of any name holds a name,
// through an embedded struct as well, and anything it cannot find or that is not a
// string does not.
func TestInputHoldsName_EachField_SaysWhetherAModelCanSendAName(t *testing.T) {
	const embedding = "type OwnerInput struct{ WidgetID int64 }\n\ntype Plain struct {\n\tName string\n\tOwner StringOrInt\n\tCount int64\n\tRef *string\n}\n\ntype Wrapped struct{ Plain }\n\nfunc (Plain) Named() string { return \"\" }\n"
	_, inputs := identifiersFixture(t, strings.NewReplacer("type OwnerInput struct{ WidgetID int64 }\n", embedding), identifiersRecord())
	holdsName := inputHoldsName(inputs.pkgs)
	cases := []struct {
		input string
		want  bool
	}{
		{"widgets.Plain.Name", true},
		{"widgets.Plain.Owner", true},
		{"widgets.Wrapped.Owner", true},
		{"widgets.Plain.Count", false},
		{"widgets.Plain.Ref", false},
		{"widgets.Plain.Named", false},
		{"widgets.Plain.Missing", false},
		{"widgets.Missing.Name", false},
		{"nowhere.Plain.Name", false},
		{"widgets", false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := holdsName(tc.input); got != tc.want {
				t.Errorf("holdsName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestInputLabel_EachRouteInput_IsSpelledAsTheTraceSpellsIt verifies a named input
// comes out as the trace spells a field's struct, and an input with no name as
// nothing, which no field starts with.
func TestInputLabel_EachRouteInput_IsSpelledAsTheTraceSpellsIt(t *testing.T) {
	if got, want := inputLabel(reflect.TypeFor[narrowingDeclaration]()), "sdk.narrowingDeclaration"; got != want {
		t.Errorf("inputLabel(named) = %q, want %q", got, want)
	}
	if got := inputLabel(nil); got != "" {
		t.Errorf("inputLabel(nil) = %q, want nothing", got)
	}
	if got := inputLabel(reflect.TypeFor[*narrowingDeclaration]()); got != "" {
		t.Errorf("inputLabel(pointer) = %q, want nothing", got)
	}
}

// TestCatalogActionInputs_TheDeclaredActions_ReadTheStructsTheirHandlersTake
// verifies the catalog's half of the join against the two kinds of action declared:
// the input struct comes out under the canonical action ID.
func TestCatalogActionInputs_TheDeclaredActions_ReadTheStructsTheirHandlersTake(t *testing.T) {
	inputs := catalogActionInputs()
	for action, want := range map[string]string{
		"admin.custom_attr_list":        "customattributes.ListInput",
		"admin.custom_attr_delete":      "customattributes.DeleteInput",
		"storage_move.schedule_group":   "groupstoragemoves.ScheduleInput",
		"storage_move.retrieve_project": "projectstoragemoves.ListForProjectInput",
	} {
		t.Run(action, func(t *testing.T) {
			if inputs[action] != want {
				t.Errorf("inputs[%q] = %q, want %q", action, inputs[action], want)
			}
		})
	}
}

// TestNarrowingDeclarations_EveryEntry_NamesAnActionAndARow verifies the committed
// table on its own: every entry names both, and no parameter twice. That each holds
// is the repository gate's to say.
func TestNarrowingDeclarations_EveryEntry_NamesAnActionAndARow(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range narrowingDeclarations {
		key := narrowingKey(declaration.Method, declaration.Param)
		t.Run(key, func(t *testing.T) {
			if declaration.Action == "" || declaration.UpstreamRow == 0 {
				t.Errorf("declaration = %+v, want an action and a row", declaration)
			}
			if seen[key] {
				t.Errorf("%s is declared twice", key)
			}
			seen[key] = true
		})
	}
}

// TestIsNarrowingFinding_EachStatus_SaysWhetherItFailsTheGate verifies the two
// statuses that fail and the three that do not.
func TestIsNarrowingFinding_EachStatus_SaysWhetherItFailsTheGate(t *testing.T) {
	for status, want := range map[string]bool{
		narrowingUndeclared: true, narrowingUnresolved: true,
		narrowingResolved: false, narrowingNotFromInput: false, narrowingUncalled: false,
	} {
		t.Run(status, func(t *testing.T) {
			if got := isNarrowingFinding(status); got != want {
				t.Errorf("isNarrowingFinding(%q) = %v, want %v", status, got, want)
			}
		})
	}
}
