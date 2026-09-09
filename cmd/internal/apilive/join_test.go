package apilive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// gated builds an entity whose one field carries the conditions given, which is
// the shape every gate case here is about.
func gated(field string, conditions ...Condition) Document {
	return Document{
		Entities: map[string]Entity{
			"API::Entities::Thing": {Fields: []Field{{Name: field, Conditions: conditions}}},
		},
		Features: map[string]string{
			"repository_mirrors":              TierPremium,
			"security_orchestration_policies": TierUltimate,
		},
	}
}

// TestNormalizePath_TheMountPrefixGoes_AndThePlaceholdersStay verifies the one
// difference between the path Grape holds and the path a caller writes.
//
// The placeholders are the point: this record spells them `:id`, which is
// already how the request inventory records what the SDK sent, so nothing else
// has to be translated. The generated document this replaced spelled the same
// placeholder `{id}` and needed a converter, and a converter is a place two
// dialects can drift apart.
func TestNormalizePath_TheMountPrefixGoes_AndThePlaceholdersStay(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "a mounted route loses its prefix and keeps its placeholders",
			path: "/api/:version/projects/:id/issues/:issue_iid",
			want: "/projects/:id/issues/:issue_iid",
		},
		{
			name: "the prefix alone is the root",
			path: "/api/:version",
			want: "/",
		},
		{
			name: "a path that never carried the prefix is returned as it is",
			path: "/projects/:id",
			want: "/projects/:id",
		},
		{
			name: "the wildcard route keeps its star",
			path: "/api/:version/*path",
			want: "/*path",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizePath(testCase.path); got != testCase.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestFields_TheEntityAsItRenders verifies that an entity's fields come back
// keyed by the name GitLab sends, with the last declaration of a name winning
// as Grape's own render does, and that an entity this record does not hold is
// reported as absent rather than as one exposing nothing.
//
// Telling those two apart is the whole value of the second return: an entity
// the boot never loaded says nothing about GitLab, and an entity that exposes
// no fields says GitLab sends none.
func TestFields_TheEntityAsItRenders(t *testing.T) {
	t.Parallel()
	doc := Document{Entities: map[string]Entity{
		"API::Entities::Thing": {Fields: []Field{
			{Name: "id"},
			{Name: "name", Attribute: "title"},
			{Name: "name", Attribute: "display_name"},
		}},
		"API::Entities::Empty": {},
	}}

	t.Run("the last declaration of a name wins", func(t *testing.T) {
		t.Parallel()
		fields, ok := doc.Fields("API::Entities::Thing")
		if !ok {
			t.Fatal("the entity is in the record and was reported absent")
		}
		if len(fields) != 2 || fields["name"].Attribute != "display_name" {
			t.Errorf("fields = %+v, want id and name, name reading display_name", fields)
		}
	})
	t.Run("an entity exposing nothing is still held", func(t *testing.T) {
		t.Parallel()
		fields, ok := doc.Fields("API::Entities::Empty")
		if !ok || len(fields) != 0 {
			t.Errorf("got %+v, %v; want an entity that is held and exposes nothing", fields, ok)
		}
	})
	t.Run("an entity the record does not hold is absent", func(t *testing.T) {
		t.Parallel()
		if _, ok := doc.Fields("API::Entities::Missing"); ok {
			t.Error("an entity the record does not hold was reported as held")
		}
	})
}

// TestFieldNames_SortedAndWithoutRepeats verifies the list an audit holds a
// response against: every key the entity sends, once each, in a stable order.
//
// Sorted rather than in declaration order because the comparison it feeds is a
// set difference whose output a person reads, and a repeat would report one
// field twice.
func TestFieldNames_SortedAndWithoutRepeats(t *testing.T) {
	t.Parallel()
	doc := Document{Entities: map[string]Entity{
		"API::Entities::Thing": {Fields: []Field{
			{Name: "title"}, {Name: "id"}, {Name: "title"}, {Name: "created_at"},
		}},
	}}

	t.Run("the keys come back sorted and deduplicated", func(t *testing.T) {
		t.Parallel()
		want := []string{"created_at", "id", "title"}
		if got := doc.FieldNames("API::Entities::Thing"); !reflect.DeepEqual(got, want) {
			t.Errorf("FieldNames = %v, want %v", got, want)
		}
	})
	t.Run("an entity the record does not hold names nothing", func(t *testing.T) {
		t.Parallel()
		if got := doc.FieldNames("API::Entities::Missing"); got != nil {
			t.Errorf("FieldNames = %v, want nothing for an entity the record does not hold", got)
		}
	})
}

// TestNestedNames_TheEdgeAGeneratedDocumentFlattens verifies the tree this
// record carries: a field rendering with an entity of its own says exactly
// which object sits under that key.
//
// It is the join a nested comparison needs. A generated OpenAPI document
// flattens the same thing into an anonymous object, so a nested question there
// has to guess from a property name; here `expose :author, using: UserBasic`
// names the entity outright.
func TestNestedNames_TheEdgeAGeneratedDocumentFlattens(t *testing.T) {
	t.Parallel()
	doc := Document{Entities: map[string]Entity{
		"API::Entities::Note": {Fields: []Field{
			{Name: "body"},
			{Name: "author", Using: "API::Entities::UserBasic"},
			{Name: "resolver", Using: "API::Entities::Unloaded"},
		}},
		"API::Entities::UserBasic": {Fields: []Field{{Name: "id"}, {Name: "username"}}},
	}}

	t.Run("a field rendering an entity names that entity's keys", func(t *testing.T) {
		t.Parallel()
		want := map[string][]string{"author": {"id", "username"}}
		if got := doc.NestedNames("API::Entities::Note"); !reflect.DeepEqual(got, want) {
			t.Errorf("NestedNames = %v, want %v", got, want)
		}
	})
	t.Run("a field rendering an entity the record lacks is left out", func(t *testing.T) {
		t.Parallel()
		// Left out rather than recorded empty: an entity the boot did not load
		// says nothing about what sits under that key, and an empty list there
		// would read as GitLab sending an object with no fields.
		if _, named := doc.NestedNames("API::Entities::Note")["resolver"]; named {
			t.Error("a field whose entity the record does not hold was given a key")
		}
	})
	t.Run("an entity the record does not hold nests nothing", func(t *testing.T) {
		t.Parallel()
		if got := doc.NestedNames("API::Entities::Missing"); got != nil {
			t.Errorf("NestedNames = %v, want nothing", got)
		}
	})
	t.Run("an entity whose fields render nothing nests nothing", func(t *testing.T) {
		t.Parallel()
		if got := doc.NestedNames("API::Entities::UserBasic"); got != nil {
			t.Errorf("NestedNames = %v, want nothing for an entity of scalars", got)
		}
	})
}

// TestLicensedFeatures_TheThreeSpellingsAConditionUses verifies what is read
// out of a condition's text, which is what a tier is then resolved from.
//
// A symbol found here is not yet a license: feature_available?(:issues) asks a
// project setting. Only the licensed feature table tells them apart, which is
// why this reports symbols and [Document.Tier] reports the plan.
func TestLicensedFeatures_TheThreeSpellingsAConditionUses(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name      string
		condition string
		want      []string
	}{
		{
			name:      "a model's own feature_available?",
			condition: "->(project, _) { project.feature_available?(:repository_mirrors) }",
			want:      []string{"repository_mirrors"},
		},
		{
			name:      "the licensed_ prefixed spelling",
			condition: "->(p, _) { p.licensed_feature_available?( :merge_pipelines ) }",
			want:      []string{"merge_pipelines"},
		},
		{
			name:      "License.feature_available? at the top level",
			condition: "->(_, _) { ::License.feature_available?(:security_orchestration_policies) }",
			want:      []string{"security_orchestration_policies"},
		},
		{
			name:      "several symbols keep their order and repeat once",
			condition: "feature_available?(:a) && licensed_feature_available?(:b) && feature_available?(:a)",
			want:      []string{"a", "b"},
		},
		{
			name:      "a condition asking no license names none",
			condition: "->(member, _) { member.source_type == 'Namespace' }",
			want:      nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := LicensedFeatures(testCase.condition); !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("LicensedFeatures = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestGateOf_WhatAConditionAmountsTo verifies the four things a finding reports
// about a gated field, each read from the conditions the instance recorded.
func TestGateOf_WhatAConditionAmountsTo(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		conditions []Condition
		want       Gate
	}{
		{
			name: "an unconditional field is gated by nothing",
		},
		{
			name:       "a block condition is the if, quoted as written",
			conditions: []Condition{{Kind: "BlockCondition", Text: "->(p, _) { p.public? }"}},
			want:       Gate{If: "->(p, _) { p.public? }"},
		},
		{
			name:       "an inverse condition is the unless",
			conditions: []Condition{{Kind: "BlockCondition", Inverse: true, Text: "->(p, _) { p.archived? }"}},
			want:       Gate{Unless: "->(p, _) { p.archived? }"},
		},
		{
			name: "two conditions join, because Grape requires both",
			conditions: []Condition{
				{Kind: "BlockCondition", Text: "->(p, _) { p.public? }"},
				{Kind: "BlockCondition", Text: "->(_, o) { o[:with_stats] }"},
			},
			want: Gate{If: "->(p, _) { p.public? } && ->(_, o) { o[:with_stats] }"},
		},
		{
			name:       "a hash condition speaks through its own data",
			conditions: []Condition{{Kind: "HashCondition", Hash: ":with_approvers"}},
			want:       Gate{If: ":with_approvers"},
		},
		{
			name: "a condition carrying no text at all contributes none",
			// Joining it would produce a leading or doubled separator, which
			// reads as a condition somebody forgot to write down.
			conditions: []Condition{
				{Kind: "HashCondition"},
				{Kind: "BlockCondition", Text: "->(p, _) { p.public? }"},
			},
			want: Gate{If: "->(p, _) { p.public? }"},
		},
		{
			name: "a licensed feature resolves to the plan that unlocks it",
			conditions: []Condition{{
				Kind: "BlockCondition",
				Text: "->(p, _) { p.feature_available?(:repository_mirrors) }",
			}},
			want: Gate{
				If:   "->(p, _) { p.feature_available?(:repository_mirrors) }",
				Tier: TierPremium,
			},
		},
		{
			name: "two licensed features take the dearer plan",
			conditions: []Condition{{
				Kind: "BlockCondition",
				Text: "feature_available?(:repository_mirrors) || " +
					"::License.feature_available?(:security_orchestration_policies)",
			}},
			want: Gate{
				If: "feature_available?(:repository_mirrors) || " +
					"::License.feature_available?(:security_orchestration_policies)",
				Tier: TierUltimate,
			},
		},
		{
			name: "a condition written under ee/ is Enterprise",
			conditions: []Condition{{
				Kind: "BlockCondition",
				File: "ee/lib/ee/api/entities/project.rb",
				Line: 9,
				Text: "->(p, _) { p.licensed_feature_available?(:repository_mirrors) }",
			}},
			want: Gate{
				If:      "->(p, _) { p.licensed_feature_available?(:repository_mirrors) }",
				Tier:    TierPremium,
				Edition: "ee",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			doc := gated("field", testCase.conditions...)
			fields, ok := doc.Fields("API::Entities::Thing")
			if !ok {
				t.Fatal("the fixture entity went missing")
			}
			if got := doc.GateOf(fields["field"]); got != testCase.want {
				t.Errorf("GateOf = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestGate_Gated_IsWhetherAnythingGatesIt verifies the predicate that decides
// whether a finding reads "always" or "when", which is the line a reviewer
// triages on.
func TestGate_Gated_IsWhetherAnythingGatesIt(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		gate Gate
		want bool
	}{
		{name: "nothing at all", gate: Gate{}, want: false},
		{name: "an if", gate: Gate{If: "->(p, _) { p.public? }"}, want: true},
		{name: "an unless", gate: Gate{Unless: "->(p, _) { p.archived? }"}, want: true},
		{
			name: "a tier with no condition, which cannot happen and is not gated",
			gate: Gate{Tier: TierPremium},
			want: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.gate.Gated(); got != testCase.want {
				t.Errorf("Gated() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestNames_ListsTheEntitiesInAStableOrder verifies the listing a report walks,
// which must not depend on map iteration order or a re-run would reorder a
// report nobody changed.
func TestNames_ListsTheEntitiesInAStableOrder(t *testing.T) {
	t.Parallel()
	doc := Document{Entities: map[string]Entity{
		"API::Entities::Project": {}, "API::Entities::Ci::Variable": {}, "API::Entities::Basic": {},
	}}

	want := []string{"API::Entities::Basic", "API::Entities::Ci::Variable", "API::Entities::Project"}
	if got := doc.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}

// TestSourceString_TheProvenanceLineAGateReports verifies that the one line a
// check prints carries what a reader needs to judge the record: how much of
// GitLab it holds, which GitLab, and when it was taken.
func TestSourceString_TheProvenanceLineAGateReports(t *testing.T) {
	t.Parallel()
	line := Source{
		Image: "gitlab/gitlab-ee:19.3.1-ee.0", Version: "19.3.1-ee", RetrievedAt: "2026-09-09",
		Entities: 582, Fields: 7317, Routes: 2110, Features: 264,
	}.String()

	for _, want := range []string{"582 entities", "7317 fields", "2110 routes", "264 licensed features", "19.3.1-ee", "2026-09-09"} {
		if !strings.Contains(line, want) {
			t.Errorf("provenance line %q is missing %q", line, want)
		}
	}
}

// TestRead_ARecordThatIsNotJSON_SaysSo verifies the one failure a reader
// cannot recover from and must not absorb: bytes on disk that are not this
// record. Absorbed, it would read as a GitLab with no entities and every audit
// downstream would report the whole surface as a gap.
func TestRead_ARecordThatIsNotJSON_SaysSo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("staging a malformed record: %v", err)
	}

	_, err := Read(dir)

	if err == nil {
		t.Fatal("a record that is not JSON was read")
	}
	if !strings.Contains(err.Error(), "decoding") {
		t.Errorf("error = %q, want it to say what it could not do", err)
	}
}

// TestWrite_WhenTheRecordCannotBeStored_SaysWhichStepFailed verifies that the
// two ways storing can fail are told apart, since one is a directory a caller
// cannot create and the other a file it cannot open, and the fix differs.
func TestWrite_WhenTheRecordCannotBeStored_SaysWhichStepFailed(t *testing.T) {
	t.Parallel()
	doc := Document{SchemaVersion: SchemaVersion}

	t.Run("the directory cannot be created", func(t *testing.T) {
		t.Parallel()
		blocked := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("staging the blocking file: %v", err)
		}

		err := Write(filepath.Join(blocked, "under"), doc)

		if err == nil {
			t.Fatal("a record was written under a path that is a file")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Errorf("error = %q, want it to name the directory step", err)
		}
	})

	t.Run("the file cannot be written", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// A directory standing where the record goes: the parent is creatable
		// and the file is not, which is the second step failing on its own.
		if err := os.Mkdir(Path(dir), 0o750); err != nil {
			t.Fatalf("staging a directory in the record's place: %v", err)
		}

		err := Write(dir, doc)

		if err == nil {
			t.Fatal("a record was written over a directory")
		}
		if !strings.Contains(err.Error(), "writing") {
			t.Errorf("error = %q, want it to name the write step", err)
		}
	})
}
