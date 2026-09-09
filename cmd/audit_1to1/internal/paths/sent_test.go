package paths

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// mirrorCondition gates a field behind an Enterprise licensed feature, written
// where an Enterprise module prepends it.
var mirrorCondition = []apilive.Condition{{
	Kind: "BlockCondition",
	File: "ee/lib/ee/api/entities/project.rb",
	Line: 9,
	Text: "->(project, _) { project.feature_available?(:repository_mirrors) }",
}}

// projectRecord is a record where one endpoint is annotated with an entity and
// another with none, and one of the entity's fields is licensed.
//
// The conditions sit on the entity rather than in a record of their own, which
// is the whole of what the port changed here: the answer and the gate on it
// come from one reading of one GitLab, so a fixture cannot put them out of step
// and neither can a regeneration.
func projectRecord(t *testing.T) string {
	t.Helper()
	return recordIn(t, projectOperations())
}

// projectOperations is that fixture.
func projectOperations() map[string]response {
	return map[string]response{
		"GET /api/v4/projects/{id}": {
			Entity:     "API::Entities::Project",
			Response:   []string{"archived", "id", "mirror", "name", "star_count"},
			Conditions: map[string][]apilive.Condition{"mirror": mirrorCondition},
		},
		"GET /api/v4/projects/{id}/plain": {},
	}
}

// projectRows are two requests of one package, to both endpoints.
func projectRows() []requestinventory.Row {
	return []requestinventory.Row{
		{Package: "internal/tools/projects", Kind: "rest", Method: "GET", Path: "/projects/:project_id"},
		{Package: "internal/tools/projects", Kind: "rest", Method: "GET", Path: "/projects/:project_id/plain"},
	}
}

// TestSentCheck_FieldsGitLabSendsThatWeDoNotPublish_AreListedWithTheirCondition
// verifies the finding this comparison exists for, with each answer the
// conditions record can give: a field the entity exposes with no condition
// is sent always, one behind a licensed feature is sent when, with its tier
// and edition beside it, one the document lists and the entity does not
// expose is unknown, and one from an endpoint naming no component is unknown
// too. The published fields are left out, an inner type's among them, since
// the row of a list is what the list endpoint sends, and the list is ordered
// by field.
func TestSentCheck_FieldsGitLabSendsThatWeDoNotPublish_AreListedWithTheirCondition(t *testing.T) {
	root := projectRecord(t)
	published := []publishedType{
		{Package: "internal/tools/projects", Name: "Output", Fields: []string{"archived", "id"}},
		{Package: "internal/tools/projects", Name: "RowOutput", Fields: []string{"star_count"}, Inner: true},
	}

	check := shapeCheck(root, projectRows(), published)

	if !check.Sent.Ran || check.Sent.Record != apilive.FileName {
		t.Fatalf("Sent = %+v, want it run against the live record", check.Sent)
	}
	want := []UnsurfacedField{
		{
			Grain: grainPackage, Package: "internal/tools/projects", Field: "mirror", Operations: []string{"GET /projects/:project_id"}, Entity: "API::Entities::Project", Sent: sentWhen,
			If: "->(project, _) { project.feature_available?(:repository_mirrors) }", Tier: apilive.TierPremium, Edition: "ee",
		},
		{Grain: grainPackage, Package: "internal/tools/projects", Field: "name", Operations: []string{"GET /projects/:project_id"}, Entity: "API::Entities::Project", Sent: sentAlways},
	}
	if !reflect.DeepEqual(check.Sent.Unsurfaced, want) {
		t.Errorf("Unsurfaced = %+v, want %+v", check.Sent.Unsurfaced, want)
	}
	if always, when, declared := unsurfacedCounts(check.Sent.Unsurfaced); always != 1 || when != 1 || declared != 0 {
		t.Errorf("unsurfacedCounts() = %d always, %d when, %d declared; want 1, 1 and 0", always, when, declared)
	}
}

// TestTypedShapeCheck_FieldsGitLabSendsThatTheTypeDoesNotPublish_AreListed
// verifies the same finding at type grain, which is the list the review
// reads: the type, the operations its client-go struct models, per field the
// entity the first operation carrying it named, and what gates the field on
// that entity. The entity is per field rather than per type because one type's
// operations resolve to different entities: the field only the second
// operation carries is read from the second's.
//
// The splat this test used to carry is gone with the record that had one. A
// scanner reading `expose *Helper.attributes` could not say which fields it
// stood for and left them unknown; an instance has already run it, so the
// fields are named here like any other.
func TestTypedShapeCheck_FieldsGitLabSendsThatTheTypeDoesNotPublish_AreListed(t *testing.T) {
	// The SDK struct carries approved_at and not approvers, so the two findings
	// this produces land on opposite sides of the upstream split: one is a
	// field client-go could already give us and one is a field nobody models.
	twoTypes := structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs: []structs.OutputPairing{
			{Package: "mrapprovals", MCPType: "SummaryOutput", SDKType: "MergeRequestApprovals", SDKFields: []string{"approved", "approved_at"}},
			{Package: "mrapprovals", MCPType: "ConfigOutput", SDKType: "MergeRequestApprovals", SDKFields: []string{"approved", "approved_at"}},
		},
	}
	stubTypeGrainInputs(t, twoTypes, nil, approvalRoutes)
	operations := map[string]response{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {
			Entity:     "API::Entities::Approvals",
			Response:   []string{"approved", "approvers"},
			Conditions: map[string][]apilive.Condition{"approvers": {{Kind: "HashCondition", Hash: ":with_approvers"}}},
		},
		"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve": {
			Entity: "API::Entities::Approved", Response: []string{"approved", "approved_at"},
		},
	}

	check := typedCheckOf("", operations, []publishedType{
		{Package: "internal/tools/mrapprovals", Name: "SummaryOutput", Fields: []string{"approved"}},
		{Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"approved"}},
	})

	searched := []string{"GET /projects/:/merge_requests/:/approvals", "POST /projects/:/merge_requests/:/approve"}
	want := []UnsurfacedField{
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "ConfigOutput", Field: "approved_at", Operations: searched, Entity: "API::Entities::Approved", SDKType: "MergeRequestApprovals", SDKModels: true, Sent: sentAlways},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "ConfigOutput", Field: "approvers", Operations: searched, Entity: "API::Entities::Approvals", SDKType: "MergeRequestApprovals", Sent: sentWhen, If: ":with_approvers"},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "SummaryOutput", Field: "approved_at", Operations: searched, Entity: "API::Entities::Approved", SDKType: "MergeRequestApprovals", SDKModels: true, Sent: sentAlways},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "SummaryOutput", Field: "approvers", Operations: searched, Entity: "API::Entities::Approvals", SDKType: "MergeRequestApprovals", Sent: sentWhen, If: ":with_approvers"},
	}
	if !reflect.DeepEqual(check.Unsurfaced, want) {
		t.Errorf("Unsurfaced = %+v, want %+v", check.Unsurfaced, want)
	}
	t.Run("the upstream split names which half each finding is in", func(t *testing.T) {
		// approved_at is a field client-go already models and only this server
		// drops, which is a local fix. approvers is one nobody models, so
		// surfacing it means an upstream contribution or a captured response,
		// and the finding is evidence for that merge request.
		if notModelledBySDK(check.Unsurfaced) != 2 {
			t.Errorf("client-go gaps = %d, want the two approvers findings", notModelledBySDK(check.Unsurfaced))
		}
	})
}

// TestUnsurfacedAtTypeGrain_ATypeModellingSeveralStructs_AsksAllOfThem verifies
// that a key one of the paired structs carries counts as modeled.
//
// A type modeling several client-go structs publishes the union of them, so a
// key any of them carries is one the SDK can already give us and an upstream
// contribution would have nothing to add.
func TestUnsurfacedAtTypeGrain_ATypeModellingSeveralStructs_AsksAllOfThem(t *testing.T) {
	sdkFields := map[string]map[string]bool{
		"BasicMergeRequest": {"iid": true},
		"MergeRequest":      {"iid": true, "squash": true},
	}
	described := describedResponses{
		Known:      map[string]bool{"iid": true, "squash": true, "nowhere": true},
		EntityOf:   map[string]string{},
		Operations: []string{"GET /merge_requests"},
	}
	candidate := publishedType{Package: "internal/tools/mergerequests", Name: "Output"}

	found := unsurfacedAtTypeGrain(candidate, []string{"BasicMergeRequest", "MergeRequest"}, sdkFields, described, newConditionIndex(apilive.Document{}))

	sortUnsurfaced(found)
	modeled := map[string]bool{}
	for _, f := range found {
		modeled[f.Field] = f.SDKModels
	}
	for _, testCase := range []struct {
		field string
		want  bool
	}{
		{field: "iid", want: true},
		{field: "squash", want: true},
		{field: "nowhere", want: false},
	} {
		t.Run(testCase.field, func(t *testing.T) {
			if modeled[testCase.field] != testCase.want {
				t.Errorf("%s modeled = %v, want %v", testCase.field, modeled[testCase.field], testCase.want)
			}
		})
	}
	t.Run("the finding names every struct the type models", func(t *testing.T) {
		if found[0].SDKType != "BasicMergeRequest|MergeRequest" {
			t.Errorf("SDKType = %q, want both structs named", found[0].SDKType)
		}
	})
}

// TestSentCheck_TheEntityOfAField_IsTheFirstOperationCarryingIt verifies the
// package grain's choice of entity when two of a package's endpoints send the
// same key: the gate is read from the first, the finding names both endpoints,
// and the two are not merged into a claim neither entity makes.
//
// The choice matters because two entities can expose one key under different
// gates, and a finding that averaged them would be true of nothing. Naming the
// endpoints beside it is what lets a reader check the other.
func TestSentCheck_TheEntityOfAField_IsTheFirstOperationCarryingIt(t *testing.T) {
	root := recordIn(t, map[string]response{
		"GET /api/v4/things/{id}/plain": {Entity: "API::Entities::ThingPlain", Response: []string{"extra"}},
		"GET /api/v4/things/{id}":       {Entity: "API::Entities::Thing", Response: []string{"extra", "id"}},
	})
	rows := []requestinventory.Row{
		{Package: "internal/tools/things", Kind: "rest", Method: "GET", Path: "/things/:thing_id/plain"},
		{Package: "internal/tools/things", Kind: "rest", Method: "GET", Path: "/things/:thing_id"},
	}

	check := shapeCheck(root, rows, []publishedType{{Package: "internal/tools/things", Name: "Output", Fields: []string{"id"}}})

	want := []UnsurfacedField{
		{Grain: grainPackage, Package: "internal/tools/things", Field: "extra", Operations: []string{"GET /things/:thing_id", "GET /things/:thing_id/plain"}, Entity: "API::Entities::ThingPlain", Sent: sentAlways},
	}
	if !reflect.DeepEqual(check.Sent.Unsurfaced, want) {
		t.Errorf("Unsurfaced = %+v, want %+v", check.Sent.Unsurfaced, want)
	}
}

// TestSentCheck_EveryFinding_IsAnsweredByTheRecordThatProducedIt verifies the
// invariant one record buys, which is the whole point of the port: nothing is
// ever reported as a field GitLab might send under conditions nobody can read.
//
// It could be, and often was, while two records answered: the OpenAPI document
// said what an endpoint returned and a separate scan of the Ruby said what each
// field was gated by, so a response could name a component that scan had never
// read, a field could be listed for a component that did not expose it, and the
// whole check could run with no conditions record at all. Every one of those
// said unknown, and 1826 findings did. Now a field is in a response because an
// entity of this record exposes it, so the same entity always has its gate.
func TestSentCheck_EveryFinding_IsAnsweredByTheRecordThatProducedIt(t *testing.T) {
	root := projectRecord(t)
	published := []publishedType{{Package: "internal/tools/projects", Name: "Output", Fields: []string{"archived", "id"}}}

	check := shapeCheck(root, projectRows(), published)

	if !check.Sent.Ran || check.Sent.Record != apilive.FileName {
		t.Fatalf("Sent = %+v, want it run against the one record", check.Sent)
	}
	if len(check.Sent.Unsurfaced) == 0 {
		t.Fatal("no findings to judge, so the invariant is not being tested")
	}
	for _, finding := range check.Sent.Unsurfaced {
		if finding.Sent == sentUnknown {
			t.Errorf("%s is unknown, and a field this record listed is a field it can answer for", finding.Field)
		}
	}
}

// TestSentCheck_APackagePublishingNothingReadable_IsNotJudged verifies that a
// package the walk read no output type for is left out rather than reported
// as missing every field of every endpoint it calls.
func TestSentCheck_APackagePublishingNothingReadable_IsNotJudged(t *testing.T) {
	root := projectRecord(t)

	check := shapeCheck(root, projectRows(), nil)

	if len(check.Sent.Unsurfaced) != 0 {
		t.Errorf("Unsurfaced = %+v, want nothing for a package with no published type", check.Sent.Unsurfaced)
	}
}
