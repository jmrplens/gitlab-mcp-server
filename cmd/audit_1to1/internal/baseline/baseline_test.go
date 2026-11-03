// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package baseline

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeFile writes content to a file under a fresh directory and returns its
// path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// TestMarshal_ReadsBackAsTheSameBaseline writes the baseline the fixtures
// build and reads it back: what -write-baseline commits is what
// -check-baseline compares with, and the file ends in a newline and lists
// the streams in name order.
func TestMarshal_ReadsBackAsTheSameBaseline(t *testing.T) {
	built, err := Build(encodeReports(t, fixtureReports()))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	content := built.Marshal()
	if !strings.HasSuffix(string(content), "}\n") {
		t.Errorf("the marshaled baseline does not end in a newline: %q", content[len(content)-8:])
	}
	if first, last := strings.Index(string(content), ActionsMissingMethods), strings.Index(string(content), StructsTypeMismatches); first < 0 || last < first {
		t.Errorf("the streams are not written in name order:\n%s", content)
	}
	read, err := Read(writeFile(t, string(content)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(read, built) {
		t.Errorf("Read gave back\n%v\nwant\n%v", read, built)
	}
}

// TestRead_AFileItCannotRead_IsAnError holds Read to refusing a file that is
// not there, is not JSON, carries a field the format does not have (a
// misspelled streams key would otherwise read as a baseline with nothing in
// it) and is of another schema version.
func TestRead_AFileItCannotRead_IsAnError(t *testing.T) {
	cases := []struct {
		name string
		// content is what the file holds, and nil leaves no file there.
		content *string
		wantErr string
		wantIs  error
	}{
		{
			name:    "missing",
			wantErr: "read the baseline",
			wantIs:  fs.ErrNotExist,
		},
		{
			name:    "not_json",
			content: new("{"),
			wantErr: "decode the baseline",
		},
		{
			name:    "unknown_field",
			content: new(`{"schema_version":1,"stream":{}}`),
			wantErr: `unknown field "stream"`,
		},
		{
			name:    "other_version",
			content: new(`{"schema_version":2,"streams":{}}`),
			wantErr: "is of schema version 2 and this command reads 1: make gen-audit-1to1-baseline rewrites it",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "absent.json")
			if tc.content != nil {
				path = writeFile(t, *tc.content)
			}
			got, err := Read(path)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Read error = %v, want it to contain %q", err, tc.wantErr)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("Read error %v does not wrap %v", err, tc.wantIs)
			}
			if got.Streams != nil || got.SchemaVersion != 0 {
				t.Errorf("Read returned %v beside its error", got)
			}
		})
	}
}

// TestCompare_TheTreeAndTheBaseline_DifferKeyByKey is the check itself: a
// tree equal to the baseline is clean; a finding the baseline does not hold
// is new and a key no finding answers is stale, each with its stream; and a
// stream one side lacks is an empty one there, so a stream added to the
// command shows its findings as new and a stream it drops shows its keys as
// stale.
func TestCompare_TheTreeAndTheBaseline_DifferKeyByKey(t *testing.T) {
	extra := []string{"alpha | Output | v3.Alpha | web_url"}
	committed := Baseline{SchemaVersion: SchemaVersion, Streams: map[string][]string{
		ParamsUnmodeled:     {"GET /a | x", "GET /b | y"},
		"retired.stream":    {"gone"},
		RequiredUnplaced:    {},
		StructsExtraOutputs: extra,
	}}
	cases := []struct {
		name      string
		current   map[string][]string
		wantNew   []Difference
		wantStale []Difference
		wantKeys  int
	}{
		{
			name:     "equal",
			current:  committed.Streams,
			wantKeys: 4,
		},
		{
			name: "one_finding_more",
			current: map[string][]string{
				ParamsUnmodeled:     {"GET /a | x", "GET /b | y", "GET /c | z"},
				"retired.stream":    {"gone"},
				RequiredUnplaced:    {},
				StructsExtraOutputs: extra,
			},
			wantNew:  []Difference{{Stream: ParamsUnmodeled, Key: "GET /c | z"}},
			wantKeys: 5,
		},
		{
			name: "one_finding_fixed",
			current: map[string][]string{
				ParamsUnmodeled:     {"GET /b | y"},
				"retired.stream":    {"gone"},
				RequiredUnplaced:    {},
				StructsExtraOutputs: extra,
			},
			wantStale: []Difference{{Stream: ParamsUnmodeled, Key: "GET /a | x"}},
			wantKeys:  3,
		},
		{
			name: "a_stream_each_side_lacks",
			current: map[string][]string{
				ParamsUnmodeled:     {"GET /a | x", "GET /b | y"},
				RequiredUnplaced:    {},
				StructsExtraOutputs: extra,
				EnumsUnsurfaced:     {"alpha.get | Output | state"},
			},
			wantNew:   []Difference{{Stream: EnumsUnsurfaced, Key: "alpha.get | Output | state"}},
			wantStale: []Difference{{Stream: "retired.stream", Key: "gone"}},
			wantKeys:  4,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comparison := Compare(committed, Baseline{SchemaVersion: SchemaVersion, Streams: tc.current})
			wantNew, wantStale := orEmpty(tc.wantNew), orEmpty(tc.wantStale)
			if !reflect.DeepEqual(comparison.New, wantNew) || !reflect.DeepEqual(comparison.Stale, wantStale) {
				t.Errorf("Compare new = %v, stale = %v; want %v and %v", comparison.New, comparison.Stale, wantNew, wantStale)
			}
			wantSummary := ComparisonSummary{Streams: len(tc.current), Keys: tc.wantKeys, New: len(wantNew), Stale: len(wantStale)}
			if comparison.Summary != wantSummary {
				t.Errorf("Summary = %+v, want %+v", comparison.Summary, wantSummary)
			}
			if comparison.Clean() != (len(wantNew)+len(wantStale) == 0) {
				t.Errorf("Clean() = %t with %d new and %d stale", comparison.Clean(), len(wantNew), len(wantStale))
			}
			if comparison.SchemaVersion != SchemaVersion || comparison.Baseline != File {
				t.Errorf("the comparison names version %d and baseline %q, want %d and %q", comparison.SchemaVersion, comparison.Baseline, SchemaVersion, File)
			}
		})
	}
}

// orEmpty is differences, or an empty list for none, which is how Compare
// writes a kind with no difference.
func orEmpty(differences []Difference) []Difference {
	if differences == nil {
		return []Difference{}
	}
	return differences
}

// TestCompare_EachKindOnly_IsNotClean holds Clean to failing on either kind
// of difference alone, so a check that looked only at new findings would let
// a baseline outlive what it holds.
func TestCompare_EachKindOnly_IsNotClean(t *testing.T) {
	for name, comparison := range map[string]Comparison{
		"new_only":   {New: []Difference{{Stream: "s", Key: "k"}}},
		"stale_only": {Stale: []Difference{{Stream: "s", Key: "k"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if comparison.Clean() {
				t.Errorf("Clean() = true for %+v", comparison)
			}
		})
	}
}

// TestComparison_Marshal_WritesBothListsEvenEmpty holds the comparison report
// to its shape: both lists present, as [] when empty, and a trailing
// newline.
func TestComparison_Marshal_WritesBothListsEvenEmpty(t *testing.T) {
	content := Compare(Baseline{}, Baseline{}).Marshal()
	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("decode %s: %v", content, err)
	}
	for _, key := range []string{"new", "stale"} {
		t.Run(key, func(t *testing.T) {
			if list, ok := decoded[key].([]any); !ok || len(list) != 0 {
				t.Errorf("%s = %#v, want an empty list", key, decoded[key])
			}
		})
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Errorf("the comparison does not end in a newline: %q", content)
	}
}

// differences returns n differences of stream, keyed k0, k1 and on.
func differences(stream string, n int) []Difference {
	out := make([]Difference, 0, n)
	for i := range n {
		out = append(out, Difference{Stream: stream, Key: "k" + string(rune('0'+i))})
	}
	return out
}

// TestComparison_Failure_NamesTheStreamAndKeyOfTheFirstDifferences holds the
// line a failing check ends on to naming each difference it shows as
// "stream: key", five at most of each kind with how many more the report
// holds, both kinds when both are there, and the command that records the
// tree.
func TestComparison_Failure_NamesTheStreamAndKeyOfTheFirstDifferences(t *testing.T) {
	cases := []struct {
		name       string
		comparison Comparison
		want       []string
		notWant    []string
	}{
		{
			name:       "one_new",
			comparison: Comparison{New: differences(ParamsUnmodeled, 1)},
			want: []string{
				"audit_1to1: 1 findings the baseline does not hold (params.unmodeled: k0); when the change means them, make gen-audit-1to1-baseline records the tree (see report)",
			},
			notWant: []string{"no finding answers", "more"},
		},
		{
			name:       "five_stale_shown_whole",
			comparison: Comparison{Stale: differences(EnumsUnsurfaced, 5)},
			want:       []string{"5 keys of the baseline no finding answers (enums.unsurfaced_output_fields: k0; enums.unsurfaced_output_fields: k1; enums.unsurfaced_output_fields: k2; enums.unsurfaced_output_fields: k3; enums.unsurfaced_output_fields: k4)"},
			notWant:    []string{"more", "does not hold"},
		},
		{
			name:       "seven_new_and_one_stale",
			comparison: Comparison{New: differences(StructsTypeMismatches, 7), Stale: differences(GrantsNotRecorded, 1)},
			want: []string{
				"7 findings the baseline does not hold (",
				"structs.type_mismatches: k4; and 2 more) and 1 keys of the baseline no finding answers (grants.derived_not_recorded: k0)",
			},
			notWant: []string{"k5"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.comparison.Failure()
			for _, part := range tc.want {
				if !strings.Contains(got, part) {
					t.Errorf("Failure() = %q, want it to contain %q", got, part)
				}
			}
			for _, part := range tc.notWant {
				if strings.Contains(got, part) {
					t.Errorf("Failure() = %q, want it not to contain %q", got, part)
				}
			}
		})
	}
}
