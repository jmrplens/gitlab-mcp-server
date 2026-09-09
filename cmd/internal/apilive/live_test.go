package apilive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestOpenAPIName_ARubyName_BecomesTheOpenAPISpelling verifies the one
// translation that lets a reader holding an entity name from this record look
// it up in the OpenAPI one, and the other way round.
//
// It matters while both records exist: this one keys entities the way Ruby
// names them and that one the way GitLab's document names its schemas, and a
// join that guessed at the difference would silently match nothing for the
// nested namespaces.
func TestOpenAPIName_ARubyName_BecomesTheOpenAPISpelling(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		ruby string
		want string
	}{
		{name: "a top-level entity", ruby: "API::Entities::Project", want: "APIEntitiesProject"},
		{name: "a nested namespace", ruby: "API::Entities::Ci::Variable", want: "APIEntitiesCiVariable"},
		{name: "two nested namespaces", ruby: "API::Entities::Ci::JobRouter::JobInfo", want: "APIEntitiesCiJobRouterJobInfo"},
		{name: "an Enterprise entity keeps its own casing", ruby: "API::Entities::GeoSiteStatus", want: "APIEntitiesGeoSiteStatus"},
		{name: "a name with no separators is unchanged", ruby: "Project", want: "Project"},
		{name: "nothing", ruby: "", want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := OpenAPIName(testCase.ruby); got != testCase.want {
				t.Errorf("OpenAPIName(%q) = %q, want %q", testCase.ruby, got, testCase.want)
			}
		})
	}
}

// TestTier_SeveralFeatures_ResolveToTheDearestPlan verifies the resolution
// every tier tag downstream is written against.
//
// The rule is the one the scanned record applied before this one replaced it,
// deliberately: the highest rank a symbol appears under wins. Whether that or the cheapest
// plan is the better answer is a question for the table's owner, and changing
// it here without changing the 172 struct tags written against it would be a
// worse defect than either answer.
func TestTier_SeveralFeatures_ResolveToTheDearestPlan(t *testing.T) {
	t.Parallel()
	doc := Document{Features: map[string]string{
		"epics":            TierPremium,
		"security_reports": TierUltimate,
		"audit_events":     TierGlobal,
	}}

	for _, testCase := range []struct {
		name     string
		features []string
		want     string
	}{
		{name: "one premium feature", features: []string{"epics"}, want: TierPremium},
		{name: "one ultimate feature", features: []string{"security_reports"}, want: TierUltimate},
		{name: "one global feature", features: []string{"audit_events"}, want: TierGlobal},
		{name: "ultimate outranks premium", features: []string{"epics", "security_reports"}, want: TierUltimate},
		{name: "premium outranks global", features: []string{"audit_events", "epics"}, want: TierPremium},
		{
			// A project setting such as :issues is not a license, so a
			// condition naming one resolves to no tier rather than to free,
			// which would read as a checked answer.
			name:     "a symbol the table does not list resolves to nothing",
			features: []string{"issues"}, want: "",
		},
		{name: "no features at all", features: nil, want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := doc.Tier(testCase.features...); got != testCase.want {
				t.Errorf("Tier(%v) = %q, want %q", testCase.features, got, testCase.want)
			}
		})
	}
}

// TestRoutesByEntity_TheIndexAnAuditWalks verifies the join that turns this
// record from a list into something an audit can follow: from an entity, to
// the endpoints annotated as rendering it.
//
// A route with no entity is left out rather than filed under the empty string,
// which would collect every unannotated endpoint into one bucket that looks
// like an entity.
func TestRoutesByEntity_TheIndexAnAuditWalks(t *testing.T) {
	t.Parallel()
	doc := Document{Routes: []Route{
		{Method: "GET", Path: "/api/:version/projects", Entity: "API::Entities::Project"},
		{Method: "GET", Path: "/api/:version/projects/:id", Entity: "API::Entities::Project"},
		{Method: "GET", Path: "/api/:version/issues", Entity: "API::Entities::Issue"},
		{Method: "DELETE", Path: "/api/:version/projects/:id/star"},
	}}

	index := doc.RoutesByEntity()

	t.Run("an entity carries every route annotated with it", func(t *testing.T) {
		t.Parallel()
		var paths []string
		for _, route := range index["API::Entities::Project"] {
			paths = append(paths, route.Path)
		}
		want := []string{"/api/:version/projects", "/api/:version/projects/:id"}
		if !reflect.DeepEqual(paths, want) {
			t.Errorf("project routes = %v, want %v", paths, want)
		}
	})
	t.Run("a route naming no entity is filed under none", func(t *testing.T) {
		t.Parallel()
		if _, ok := index[""]; ok {
			t.Error("the unannotated route was filed under the empty entity name")
		}
	})
	t.Run("the index holds only the entities that were named", func(t *testing.T) {
		t.Parallel()
		if len(index) != 2 {
			t.Errorf("index holds %d entities, want 2: %v", len(index), index)
		}
	})
}

// TestFieldCount_SpansEveryEntity verifies the figure the floors are set
// against. Counting one entity, or counting entities rather than fields, would
// let a record that lost most of its content pass a floor written in fields.
func TestFieldCount_SpansEveryEntity(t *testing.T) {
	t.Parallel()
	doc := Document{Entities: map[string]Entity{
		"API::Entities::Project": {Fields: []Field{{Name: "id"}, {Name: "name"}}},
		"API::Entities::Issue":   {Fields: []Field{{Name: "iid"}}},
		"API::Entities::Broken":  {Error: "NoMethodError"},
	}}

	if got := doc.FieldCount(); got != 3 {
		t.Errorf("FieldCount() = %d, want 3", got)
	}
}

// TestReadWrite_ARoundTrip_KeepsWhatTheAuditReads verifies that the record
// survives the trip to disk with the parts an audit joins on intact, and that
// a record from another schema version is refused rather than decoded.
//
// The refusal is the half worth having. A shape that moved would still parse:
// the fields that kept their names would populate and the rest would be zero,
// which reads downstream as GitLab having stopped sending them.
// Not parallel: the subtests rewrite the one record this test wrote, so
// running them at once would have each reading another's file.
func TestReadWrite_ARoundTrip_KeepsWhatTheAuditReads(t *testing.T) {
	dir := t.TempDir()
	want := Document{
		SchemaVersion: SchemaVersion,
		Source:        Source{Image: "gitlab/gitlab-ee:latest", Version: "19.3.1-ee", RetrievedAt: "2026-09-09"},
		Entities: map[string]Entity{
			"API::Entities::Project": {Fields: []Field{
				{Name: "id"},
				{Name: "owner", Using: "API::Entities::UserBasic"},
				{Name: "approvals_before_merge", Conditions: []Condition{{
					Kind: "BlockCondition",
					File: "ee/lib/ee/api/entities/project.rb", Line: 19,
					Text: "expose :approvals_before_merge, if: ->(project, _) { project.feature_available?(:merge_request_approvers) }",
				}}},
			}},
		},
		Routes:   []Route{{Method: "GET", Path: "/api/:version/projects", Entity: "API::Entities::Project", Params: map[string]Param{"page": {Type: "Integer", Default: "1"}}}},
		Features: map[string]string{"merge_request_approvers": TierPremium},
	}

	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the record:\n got %+v\nwant %+v", got, want)
	}

	t.Run("a record from another schema version is refused", func(t *testing.T) {
		other := want
		other.SchemaVersion = SchemaVersion + 1
		if writeErr := Write(dir, other); writeErr != nil {
			t.Fatalf("Write: %v", writeErr)
		}
		_, readErr := Read(dir)
		if readErr == nil {
			t.Fatal("a record from another schema version was read")
		}
		if !strings.Contains(readErr.Error(), "schema version") {
			t.Errorf("error = %q, want it to name the schema version", readErr)
		}
	})

	t.Run("a missing record says so rather than reading as empty", func(t *testing.T) {
		if _, readErr := Read(filepath.Join(dir, "nowhere")); readErr == nil {
			t.Fatal("a missing record read as an empty one")
		}
	})
}

// TestWrite_TheRecordIsDiffable verifies that a re-pin reads as a diff rather
// than as one very long line, which is what makes a regeneration reviewable.
func TestWrite_TheRecordIsDiffable(t *testing.T) {
	dir := t.TempDir()
	doc := Document{
		SchemaVersion: SchemaVersion,
		Entities:      map[string]Entity{"API::Entities::Project": {Fields: []Field{{Name: "id"}}}},
	}

	if err := Write(dir, doc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	t.Run("it is indented across many lines", func(t *testing.T) {
		if lines := strings.Count(string(raw), "\n"); lines < 5 {
			t.Errorf("the record has %d newlines, want it indented", lines)
		}
	})
	t.Run("it ends with a newline", func(t *testing.T) {
		if !strings.HasSuffix(string(raw), "\n") {
			t.Error("the record does not end with a newline")
		}
	})
}
