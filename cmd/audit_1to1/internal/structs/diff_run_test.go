package structs

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// TestDiffRun_Answer_RecordsOnlyADeclaredKey verifies the one place a key is
// marked used: a lookup of a declared key answers true and is recorded against
// its own table, and a lookup of anything else answers false and records
// nothing, so a key that was merely asked about is still stale.
func TestDiffRun_Answer_RecordsOnlyADeclaredKey(t *testing.T) {
	first := &declarationTable{name: "first", entries: map[string]string{"pkg.Type": "reason"}}
	second := &declarationTable{name: "second", entries: map[string]string{"pkg.Type": "reason"}}
	run := newDiffRun()

	if run.answer(first, "pkg.Other") {
		t.Error("answer(undeclared) = true, want false")
	}
	if len(run.answered) != 0 {
		t.Errorf("an undeclared lookup recorded %v, want nothing", run.answered)
	}
	if !run.answer(first, "pkg.Type") {
		t.Error("answer(declared) = false, want true")
	}
	// Asked twice, a key is recorded once: the record is a set.
	run.answer(first, "pkg.Type")

	want := map[*declarationTable]map[string]struct{}{first: {"pkg.Type": {}}}
	if !reflect.DeepEqual(run.answered, want) {
		t.Errorf("answered = %v, want the key under its own table alone", run.answered)
	}
	if got := run.staleDeclarations([]*declarationTable{first, second}); !reflect.DeepEqual(got, []staleDeclaration{{Table: "second", Key: "pkg.Type", Reason: "reason"}}) {
		t.Errorf("staleDeclarations = %+v, want the same key in the table that was never asked", got)
	}
}

// TestDiffRun_StaleDeclarations_ListsUnansweredKeysTableByTable verifies the
// whole record a stale entry carries and the order the report lists them in:
// the tables in the order given, which is not their names' order, and each
// table's keys sorted, whatever order they were declared or answered in.
func TestDiffRun_StaleDeclarations_ListsUnansweredKeysTableByTable(t *testing.T) {
	zeta := &declarationTable{name: "zeta", entries: map[string]string{
		"b.Type.field": "zeta b",
		"a.Type.field": "zeta a",
		"c.Type.field": "zeta c",
	}}
	alpha := &declarationTable{name: "alpha", entries: map[string]string{"x.Type": "alpha x"}}
	empty := &declarationTable{name: "empty", entries: map[string]string{}}
	run := newDiffRun()
	run.answer(zeta, "c.Type.field")

	got := run.staleDeclarations([]*declarationTable{zeta, empty, alpha})

	want := []staleDeclaration{
		{Table: "zeta", Key: "a.Type.field", Reason: "zeta a"},
		{Table: "zeta", Key: "b.Type.field", Reason: "zeta b"},
		{Table: "alpha", Key: "x.Type", Reason: "alpha x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("staleDeclarations = %+v, want %+v", got, want)
	}
	if none := newDiffRun().staleDeclarations(nil); none != nil {
		t.Errorf("staleDeclarations(nil) = %+v, want nil", none)
	}
}

// TestDiffRun_EveryTable_ReportsAKeyNothingAnswers is the rule the six tables
// were never held to, asked of each of them through the path the report
// takes: one planted key per table that no candidate reaches, beside a live
// key per table that a candidate of the fixture diff does reach.
//
// The live keys are what make the planted ones mean something. Each is
// answered by the lookup the diff makes for exactly the shape of candidate its
// table exists for, so a rule reporting every key, or one recording a key when
// it is merely asked about, fails here as surely as one reporting none.
func TestDiffRun_EveryTable_ReportsAKeyNothingAnswers(t *testing.T) {
	live := map[*declarationTable]string{
		acceptedOutputRenames: "fixturepkg.FixtureOutput.renamed",
		curatedRefSubsets:     "fixturepkg.CuratedOutput",
		docOmittedFields:      "fixturepkg.FixtureOutput.omitted",
		docAddedFields:        "fixturepkg.FixtureOutput.added",
		acceptedExtraOutputs:  "fixturepkg.FixtureOutput.composed",
		acceptedMissingInputs: "fixturepkg.FixtureInput.skipped",
	}
	planted := map[*declarationTable]string{}
	for _, table := range declarationTables {
		declare(t, table, live[table], "live fixture")
		planted[table] = "fixturepkg.NoSuchOutput.planted_" + table.name
		declare(t, table, planted[table], "stale fixture")
	}

	run := newDiffRun()
	output := makeStruct(
		structField{"ID", "id", tInt},
		structField{"Renamed", "renamed", tString},
		structField{"Added", "added", tString},
		structField{"Composed", "composed", tString},
	)
	result := makeStruct(structField{"ID", "id", tInt}, structField{"Omitted", "omitted", tString})
	run.diffOutputGroup("fixturepkg", outputGroup{
		mcpName: "FixtureOutput", mcpType: output,
		pairs: []structPair{{mcpName: "FixtureOutput", mcpType: output, sdkName: "v2.Fixture", sdkType: result}},
	})
	curated := makeStruct(structField{"ID", "id", tInt})
	run.diffOutputGroup("fixturepkg", outputGroup{
		mcpName: "CuratedOutput", mcpType: curated,
		pairs: []structPair{{mcpName: "CuratedOutput", mcpType: curated, sdkName: "v2.Fixture", sdkType: result}},
	})
	input := makeStruct(structField{"ID", "id", tInt})
	options := makeStructWithTags(
		taggedField{name: "ID", tag: `url:"id"`, goType: tInt},
		taggedField{name: "Skipped", tag: `url:"skipped"`, goType: tString},
	)
	run.diffPair("fixturepkg", "input", structPair{
		mcpName: "FixtureInput", mcpType: input,
		sdkName: "v2.FixtureOptions", sdkType: options, sdkURLTags: true,
	})

	stale := map[string]bool{}
	for _, entry := range run.staleDeclarations(declarationTables) {
		stale[entry.Table+" "+entry.Key] = true
	}
	for _, table := range declarationTables {
		t.Run(table.name, func(t *testing.T) {
			if !stale[table.name+" "+planted[table]] {
				t.Errorf("%s: the planted key %s answered nothing and was not reported", table.name, planted[table])
			}
			if stale[table.name+" "+live[table]] {
				t.Errorf("%s: the live key %s answered a candidate and was reported stale", table.name, live[table])
			}
		})
	}
}

// TestBuildReport_Repository_NoDeclarationIsStale is the rule applied to the
// tree: every key of the six tables answers a candidate of a run over the
// whole of internal/tools. It is the test that fails the day a type a
// declaration names stops producing the finding the declaration excuses, and
// the failure names the key, so the fix is to delete it or to restore what it
// described, never to leave it answering the next finding that shares its name.
func TestBuildReport_Repository_NoDeclarationIsStale(t *testing.T) {
	for _, gapsOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("gaps-only=%v", gapsOnly), func(t *testing.T) {
			rep := cachedBuildReport(t, gapsOnly)
			for _, entry := range rep.StaleDeclarations {
				t.Errorf("%s key %q answers no finding of the tree (%s)", entry.Table, entry.Key, entry.Reason)
			}
			if rep.Summary.StaleDeclarations != 0 {
				t.Errorf("summary counts %d stale declarations, want 0", rep.Summary.StaleDeclarations)
			}
		})
	}
}

// TestRun_APlantedStaleDeclaration_IsReportedAndCounted verifies the wiring
// between the run and the document the command writes, on the one input where
// it is visible: a report of the real tree carries no stale declaration, so a
// count never assigned and a list never attached read exactly like a clean
// tree. One planted key makes both observable, through -gaps-only, which drops
// clean packages and must not drop this.
func TestRun_APlantedStaleDeclaration_IsReportedAndCounted(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	declare(t, docAddedFields, "fixturepkg.NoSuchOutput.planted", "planted by this test")

	content, err := Run(root, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var rep report
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}

	want := []staleDeclaration{{Table: "docAddedFields", Key: "fixturepkg.NoSuchOutput.planted", Reason: "planted by this test"}}
	if !reflect.DeepEqual(rep.StaleDeclarations, want) {
		t.Errorf("stale_declarations = %+v, want %+v", rep.StaleDeclarations, want)
	}
	if rep.Summary.StaleDeclarations != 1 {
		t.Errorf("summary stale_declarations = %d, want 1", rep.Summary.StaleDeclarations)
	}
	if len(rep.Packages) == 0 {
		t.Error("the report lost its packages when it gained a stale declaration")
	}
}
