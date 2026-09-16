package main

import (
	"strings"
	"testing"
)

// oneRow is a row that agrees with its siblings on everything, so a test below
// can differ from it in one field and say what that one field decides.
func oneRow() row {
	return row{Key: rowKey{
		Model:            fixtureModel,
		Surface:          "dynamic",
		Mode:             "default",
		Tier:             "ultimate",
		CorpusDigest:     "corpus-1",
		ContractDigest:   fixtureContract,
		ToolSchemaDigest: fixtureTools,
		Repeat:           1,
	}}
}

// TestCrossVendorKey_DiffersInEveryFieldButTheModel is the whole of the
// cross-vendor rule: the model is what such a table varies, and everything else
// puts a row in a table of its own.
func TestCrossVendorKey_DiffersInEveryFieldButTheModel(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*row)
		apart bool
	}{
		{"another model", func(r *row) { r.Key.Model = "openai:other" }, false},
		{"another surface", func(r *row) { r.Key.Surface = "meta" }, true},
		{"another mode", func(r *row) { r.Key.Mode = "read-only" }, true},
		{"another tier", func(r *row) { r.Key.Tier = "free" }, true},
		{"a tier pin", func(r *row) { r.Key.TierPin = "premium" }, true},
		{"another meta schema", func(r *row) { r.Key.MetaParamSchema = "compact" }, true},
		{"another slice size", func(r *row) { r.Key.SliceSize = 128 }, true},
		{"another corpus", func(r *row) { r.Key.CorpusDigest = "corpus-2" }, true},
		{"another contract", func(r *row) { r.Key.ContractDigest = "contract-2" }, true},
		{"another tool schema", func(r *row) { r.Key.ToolSchemaDigest = "tools-2" }, true},
		{"another repeat", func(r *row) { r.Key.Repeat = 3 }, true},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			other := oneRow()
			one.edit(&other)
			apart := crossVendorKey(oneRow()) != crossVendorKey(other)
			if apart != one.apart {
				t.Errorf("rows differing by %s are in %d table(s), want %d",
					one.name, tables(apart), tables(one.apart))
			}
		})
	}
}

// tables spells a boolean as the number of tables it produces, so the failure
// above reads as the decision it is about.
func tables(apart bool) int {
	if apart {
		return 2
	}
	return 1
}

// TestCrossSurfaceKey_VariesTheSurfaceAndWhatTheSurfaceDecides is the second
// rule, and the one a single key could not have expressed: a cross-surface
// table must vary the surface, and with it the three things a surface decides
// for itself, while holding everything else.
func TestCrossSurfaceKey_VariesTheSurfaceAndWhatTheSurfaceDecides(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*row)
		apart bool
	}{
		{"another surface", func(r *row) { r.Key.Surface = "meta" }, false},
		{"another meta schema", func(r *row) { r.Key.MetaParamSchema = "opaque" }, false},
		{"another slice size", func(r *row) { r.Key.SliceSize = 128 }, false},
		{"another tool schema", func(r *row) { r.Key.ToolSchemaDigest = "tools-2" }, false},
		{"another model", func(r *row) { r.Key.Model = "openai:other" }, true},
		{"another mode", func(r *row) { r.Key.Mode = "safe" }, true},
		{"another tier", func(r *row) { r.Key.Tier = "free" }, true},
		{"a tier pin", func(r *row) { r.Key.TierPin = "premium" }, true},
		{"another corpus", func(r *row) { r.Key.CorpusDigest = "corpus-2" }, true},
		{"another contract", func(r *row) { r.Key.ContractDigest = "contract-2" }, true},
		{"another repeat", func(r *row) { r.Key.Repeat = 3 }, true},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			other := oneRow()
			one.edit(&other)
			apart := crossSurfaceKey(oneRow()) != crossSurfaceKey(other)
			if apart != one.apart {
				t.Errorf("rows differing by %s are in %d comparison(s), want %d",
					one.name, tables(apart), tables(one.apart))
			}
		})
	}
}

// TestGroupBySurface_RowsDifferingInSurfaceAndNothingElse_AreOneComparison is
// the table the first draft of this rule would have forbidden: one model,
// measured on the three surfaces, is one comparison and not three.
func TestGroupBySurface_RowsDifferingInSurfaceAndNothingElse_AreOneComparison(t *testing.T) {
	dynamic := oneRow()
	meta := oneRow()
	meta.Key.Surface = "meta"
	meta.Key.MetaParamSchema = opaqueSchema
	meta.Key.ToolSchemaDigest = "tools-meta"
	individual := oneRow()
	individual.Key.Surface = "individual"
	individual.Key.SliceSize = 128
	individual.Key.ToolSchemaDigest = "tools-individual"

	groups := groupBySurface([]row{meta, individual, dynamic})
	if len(groups) != 1 {
		t.Fatalf("got %d comparisons, want the three surfaces of one model in one", len(groups))
	}
	if got := groups[0].surfaces(); got != 3 {
		t.Errorf("the comparison names %d surface(s), want 3", got)
	}
	if got := groups[0].Rows[0].Key.Surface; got != "dynamic" {
		t.Errorf("the comparison begins with %q, want it ordered by surface", got)
	}
	if strings.Contains(groups[0].Caption, "surface") {
		t.Errorf("the caption %q names the surface, which a cross-surface comparison varies", groups[0].Caption)
	}
}

// TestGroupByVendor_OrdersTheTablesAndTheirRows keeps a re-render of one record
// byte for byte what the last one was, which is what lets a gate compare the
// pages at all.
func TestGroupByVendor_OrdersTheTablesAndTheirRows(t *testing.T) {
	first := oneRow()
	first.Key.Model = "zzz:model"
	second := oneRow()
	second.Key.Model = "aaa:model"
	third := oneRow()
	third.Key.Surface = "meta"

	groups := groupByVendor([]row{first, third, second})
	if len(groups) != 2 {
		t.Fatalf("got %d tables, want the two surfaces apart", len(groups))
	}
	if groups[0].Rows[0].Key.Model != "aaa:model" {
		t.Errorf("the first table begins with %q, want it ordered by model", groups[0].Rows[0].Key.Model)
	}
	if !strings.Contains(groups[0].Caption, "surface `dynamic`") {
		t.Errorf("the caption %q does not name what the table holds fixed", groups[0].Caption)
	}
}

// TestVendorKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist keeps a caption
// from claiming a pin, a schema mode or a slice a row never had.
func TestVendorKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist(t *testing.T) {
	plain := crossVendorKey(oneRow()).String()
	for _, absent := range []string{"pinned", "meta schema", "slice"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(plain, absent) {
				t.Errorf("the caption %q names %q, which this row has none of", plain, absent)
			}
		})
	}

	pinned := oneRow()
	pinned.Key.TierPin = "premium"
	pinned.Key.MetaParamSchema = "compact"
	pinned.Key.SliceSize = 128
	full := crossVendorKey(pinned).String()
	for _, present := range []string{"pinned to `premium`", "meta schema `compact`", "slice of 128 tools"} {
		t.Run(present, func(t *testing.T) {
			if !strings.Contains(full, present) {
				t.Errorf("the caption %q does not name %q", full, present)
			}
		})
	}
}

// TestSurfaceKey_String_NamesTheModelAndThePin is what a cross-surface note
// prints, and the only place a reader is told what two rows of different
// surfaces agree on.
func TestSurfaceKey_String_NamesTheModelAndThePin(t *testing.T) {
	pinned := oneRow()
	pinned.Key.TierPin = "premium"
	got := crossSurfaceKey(pinned).String()
	for _, present := range []string{"model `" + fixtureModel + "`", "pinned to `premium`", "repeat 1"} {
		t.Run(present, func(t *testing.T) {
			if !strings.Contains(got, present) {
				t.Errorf("the note %q does not name %q", got, present)
			}
		})
	}
}

// TestRowKey_String_IsWhatACollisionIsRefusedBy holds the one property a key's
// spelling has to have: two rows that differ anywhere must spell differently,
// or a collision would hide one measurement behind another.
func TestRowKey_String_IsWhatACollisionIsRefusedBy(t *testing.T) {
	one := oneRow()
	other := oneRow()
	other.Key.SliceSize = 128
	if one.Key.String() == other.Key.String() {
		t.Fatalf("two different rows spell the same key: %q", one.Key.String())
	}
	if !strings.Contains(other.Key.String(), "slice=128") {
		t.Errorf("the key %q does not name the slice it was measured on", other.Key.String())
	}
}

// TestRowKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist keeps the name a
// collision is refused by from claiming a pin, a schema mode or a slice the row
// never had, and from dropping one it did.
func TestRowKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist(t *testing.T) {
	plain := oneRow().Key.String()
	for _, absent := range []string{"tier-pin=", "meta-schema=", "slice="} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(plain, absent) {
				t.Errorf("the key %q names %q, which this row has none of", plain, absent)
			}
		})
	}

	full := oneRow()
	full.Key.TierPin = "premium"
	full.Key.MetaParamSchema = "compact"
	full.Key.SliceSize = 128
	spelled := full.Key.String()
	for _, present := range []string{"tier-pin=premium", "meta-schema=compact", "slice=128", "repeat=1"} {
		t.Run(present, func(t *testing.T) {
			if !strings.Contains(spelled, present) {
				t.Errorf("the key %q does not name %q", spelled, present)
			}
		})
	}
}
