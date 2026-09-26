package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestWriteSentReport_EveryFieldReadsBackAsWritten verifies the property that
// makes the render error in writeSentReport unreachable, since that arm is
// kept rather than tested: a report holds strings, integers, slices and structs
// of them and nothing encoding/json can refuse, so it always renders, and what
// it renders is the report.
//
// Every field is filled with a value no other field shares, so a field that
// stopped being written, or came back under another's key, fails the
// comparison rather than reading back as the zero it started as.
func TestWriteSentReport_EveryFieldReadsBackAsWritten(t *testing.T) {
	written := sentReport{
		Check: sentOracles{
			Schema:   "12 types from schema.graphql, not the pinned schema",
			Grain:    sentGrain,
			Pairings: 3,
			Coverage: sentCoverage{Reached: 11, Asked: 5, Traversed: 4, RepeatedType: 2, Undecoded: 7, Map: 6, SelfDecoding: 8},
			Uncovered: sentUncovered{
				Source:     "docs/development/request-inventory.json",
				Reason:     uncoveredReason,
				Packages:   []sentUncoveredPackage{{Package: "internal/tools/epics", Operations: []string{"query Epics", "mutation Close"}}},
				Operations: 2,
			},
			TierOracle:              oracleNone,
			TierOracleReason:        tierOracleReason,
			DeprecationOracle:       "none yet",
			DeprecationOracleReason: deprecationOracleReason,
			Gating:                  gatingReason,
		},
		Summary: sentSummary{
			Findings: 13, Undeclared: 9, Leaf: 10, Object: 1, Collection: 14,
			Always: 15, Nullable: 16, Packages: 17, SchemaTypes: 18, Occurrences: 19,
		},
		Sent: []sentField{{
			Grain:             sentGrain,
			Package:           "fixture/domain",
			Document:          "getProject",
			Position:          "wrap/wrap.go:12",
			HandedOverAt:      "domain/domain.go:27",
			Operation:         "query Project",
			Path:              "data.project.pipeline",
			SchemaType:        "Pipeline",
			Type:              "domain.pipeline",
			Field:             "startedAt",
			FieldType:         "Time",
			Class:             sentLeaf,
			Sent:              sentNullable,
			SameNameInPackage: "started_at",
			Occurrences:       20,
			Category:          categoryNotThisResponse,
			Reason:            "the pipelines domain publishes a pipeline",
		}},
		UnusedDeclarations: []string{"fixture/domain.Label.color"},
	}
	path := filepath.Join(t.TempDir(), "sent.json")

	if err := writeSentReport(path, written); err != nil {
		t.Fatalf("writeSentReport() = %v, want nil for a report holding nothing encoding/json refuses", err)
	}
	content, err := os.ReadFile(path) //#nosec G304 -- a path this test chose
	if err != nil {
		t.Fatalf("read the report back: %v", err)
	}
	var read sentReport
	if decoded := json.Unmarshal(content, &read); decoded != nil {
		t.Fatalf("decode the report: %v", decoded)
	}
	if !reflect.DeepEqual(read, written) {
		t.Errorf("the report read back differs from the one written:\nread    %+v\nwritten %+v", read, written)
	}
}
