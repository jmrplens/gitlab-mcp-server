package apiexposes

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sampleDocument is a record small enough to reason about: a parent, a child,
// a merge, and a loop.
func sampleDocument() Document {
	return Document{
		Source: Source{Ref: "master", Commit: "0123456789abcdef0123456789abcdef01234567", RetrievedAt: "2026-09-08", Entities: 3, Features: 2},
		Entities: map[string]Entity{
			"APIEntitiesBasic": {File: "lib/api/entities/basic.rb", Line: 5, Fields: []Field{
				{Name: "id", Line: 6},
				{Name: "bits", Line: 7, Merge: true, Using: "APIEntitiesBits", If: "outer?"},
			}},
			"APIEntitiesBits": {File: "lib/api/entities/bits.rb", Line: 3, Fields: []Field{
				{Name: "bit", Line: 4, If: "inner?", Unless: "off?"},
			}},
			"APIEntitiesChild": {File: "lib/api/entities/child.rb", Line: 3, Parent: "APIEntitiesBasic", Fields: []Field{
				{Name: "extra", Line: 4},
			}},
			"APIEntitiesLoop": {File: "lib/api/entities/loop.rb", Line: 3, Parent: "APIEntitiesLoop", Fields: []Field{
				{Name: "self_ref", Line: 4, Merge: true, Using: "APIEntitiesLoop"},
			}},
		},
		Features: map[string]string{"epics": TierPremium},
	}
}

// TestWriteAndRead_RoundTrip_KeepsTheRecordAndStampsTheVersion verifies that
// what Write commits is what Read returns, with the schema version stamped by
// Write rather than trusted from the caller.
func TestWriteAndRead_RoundTrip_KeepsTheRecordAndStampsTheVersion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "record")
	doc := sampleDocument()

	if err := Write(dir, doc); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	doc.SchemaVersion = SchemaVersion
	if !reflect.DeepEqual(got, doc) {
		t.Errorf("Read() = %+v, want what was written", got)
	}
	if got.Names()[0] != "APIEntitiesBasic" || len(got.Names()) != 4 {
		t.Errorf("Names() = %v, want the four entities sorted", got.Names())
	}
}

// TestRead_RecordThatCannotBeUsed_IsRefused verifies each way a committed
// record fails to be one a reader can rest on: missing, not JSON, and a schema
// version this build does not read.
func TestRead_RecordThatCannotBeUsed_IsRefused(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "missing", want: "read "},
		{name: "not JSON", content: "{", want: "parse "},
		{name: "another schema version", content: `{"schema_version": 99}`, want: "is schema version 99 and this build reads 1: regenerate it with `make gen-api-exposes`"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			if testCase.content != "" {
				if err := os.WriteFile(filepath.Join(dir, FileName), []byte(testCase.content), 0o600); err != nil {
					t.Fatalf("prepare: %v", err)
				}
			}

			_, err := Read(dir)

			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Read() error = %v, want it to say %q", err, testCase.want)
			}
		})
	}
}

// TestWrite_DirectoryThatCannotBeCreated_IsReported verifies the write half's
// two failures: a directory that cannot be made because a file is in its way,
// and a file that cannot be written because a directory already bears its
// name.
func TestWrite_DirectoryThatCannotBeCreated_IsReported(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	occupied := t.TempDir()
	if err := os.Mkdir(filepath.Join(occupied, FileName), 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	cases := []struct {
		name string
		dir  string
		want string
	}{
		{name: "a file where the directory should be", dir: filepath.Join(blocker, "record"), want: "create "},
		{name: "a directory where the record should be written", dir: occupied, want: "write "},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := Write(testCase.dir, sampleDocument())

			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Write() error = %v, want it to say %q", err, testCase.want)
			}
		})
	}
}

// TestEffective_AssemblesFieldsTheWayGrapeDoes verifies the one reading a
// reviewer needs: the parent's fields first, a merge replaced by the merged
// entity's fields with the merging field's conditions joined onto theirs, an
// unknown name reported as such, and a chain that loops cut where it repeats.
func TestEffective_AssemblesFieldsTheWayGrapeDoes(t *testing.T) {
	doc := sampleDocument()
	cases := []struct {
		name  string
		which string
		want  []Field
		known bool
	}{
		{
			name:  "a child inherits its parent, merges included",
			which: "APIEntitiesChild",
			known: true,
			want: []Field{
				{Name: "id", Line: 6},
				{Name: "bit", Line: 4, If: "outer? && inner?", Unless: "off?"},
				{Name: "extra", Line: 4},
			},
		},
		{name: "an unknown entity", which: "APIEntitiesNope", known: false},
		{name: "a loop is cut where it repeats", which: "APIEntitiesLoop", known: true, want: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, known := doc.Effective(testCase.which)

			if known != testCase.known {
				t.Fatalf("Effective() known = %v, want %v", known, testCase.known)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("Effective() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestSourceString_NamesTheProvenanceInOneLine verifies the line a check
// prints, with the commit abbreviated the way git does and a short commit
// left alone.
func TestSourceString_NamesTheProvenanceInOneLine(t *testing.T) {
	long := sampleDocument().Source
	if got := long.String(); got != "3 entities and 2 licensed features from gitlab-org/gitlab at master (01234567), retrieved 2026-09-08" {
		t.Errorf("String() = %q", got)
	}
	short := Source{Ref: "v1", Commit: "abc", RetrievedAt: "2026-01-01"}
	if got := short.String(); !strings.Contains(got, "at v1 (abc)") {
		t.Errorf("String() = %q, want the short commit kept whole", got)
	}
}

// TestOpenAPIName_RendersTheConstantPathAsGrapeSwaggerDoes verifies the join
// between this record and the OpenAPI one.
func TestOpenAPIName_RendersTheConstantPathAsGrapeSwaggerDoes(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "API::Entities::Project", want: "APIEntitiesProject"},
		{in: "::API::Entities::Ci::Job", want: "APIEntitiesCiJob"},
		{in: "DetailedStatusEntity", want: "DetailedStatusEntity"},
	}
	for _, testCase := range cases {
		t.Run(testCase.in, func(t *testing.T) {
			if got := OpenAPIName(testCase.in); got != testCase.want {
				t.Errorf("OpenAPIName(%q) = %q, want %q", testCase.in, got, testCase.want)
			}
		})
	}
}
