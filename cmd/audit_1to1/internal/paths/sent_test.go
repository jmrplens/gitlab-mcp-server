package paths

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apiexposes"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apishapes"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// conditionsIn writes a conditions record beside the OpenAPI record a test
// already wrote under root.
func conditionsIn(t *testing.T, root string, entities map[string]apiexposes.Entity) {
	t.Helper()
	if err := apiexposes.Write(filepath.Join(root, apiexposes.DefaultDir), apiexposes.Document{
		Source:   apiexposes.Source{Ref: "master", Commit: "0123456789abcdef0123456789abcdef01234567", RetrievedAt: "2026-09-08", SHA256: "abc"},
		Entities: entities,
	}); err != nil {
		t.Fatalf("prepare the conditions record: %v", err)
	}
}

// projectRecord is an OpenAPI record where one endpoint names a component and
// another names none.
func projectRecord(t *testing.T) string {
	t.Helper()
	return recordIn(t, map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}":       {Entity: "APIEntitiesProject", Response: []string{"archived", "id", "mirror", "name", "star_count", "unlisted"}},
		"GET /api/v4/projects/{id}/plain": {Response: []string{"extra"}},
	})
}

// projectConditions is the conditions record for it: a parent exposing id,
// a child exposing the rest, one field behind a licensed feature.
func projectConditions() map[string]apiexposes.Entity {
	return map[string]apiexposes.Entity{
		"APIEntitiesBasic": {File: "lib/api/entities/basic.rb", Line: 3, Fields: []apiexposes.Field{{Name: "id", Line: 4}}},
		"APIEntitiesProject": {File: "lib/api/entities/project.rb", Line: 3, Parent: "APIEntitiesBasic", Fields: []apiexposes.Field{
			{Name: "name", Line: 4},
			{Name: "archived", Line: 5},
			{Name: "star_count", Line: 6},
			{Name: "mirror", Line: 9, File: "ee/lib/ee/api/entities/project.rb", Edition: "ee", If: "->(project, _) { project.feature_available?(:repository_mirrors) }", Features: []string{"repository_mirrors"}, Tier: apiexposes.TierPremium},
		}},
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
	conditionsIn(t, root, projectConditions())
	published := []publishedType{
		{Package: "internal/tools/projects", Name: "Output", Fields: []string{"archived", "id"}},
		{Package: "internal/tools/projects", Name: "RowOutput", Fields: []string{"star_count"}, Inner: true},
	}

	check := shapeCheck(root, projectRows(), published)

	if !check.Sent.Ran || check.Sent.Record != apiexposes.FileName {
		t.Fatalf("Sent = %+v, want it run against the conditions record", check.Sent)
	}
	want := []UnsurfacedField{
		{Grain: grainPackage, Package: "internal/tools/projects", Field: "extra", Operations: []string{"GET /projects/:project_id/plain"}, Sent: sentUnknown},
		{
			Grain: grainPackage, Package: "internal/tools/projects", Field: "mirror", Operations: []string{"GET /projects/:project_id"}, Entity: "APIEntitiesProject", Sent: sentWhen,
			If: "->(project, _) { project.feature_available?(:repository_mirrors) }", Tier: apiexposes.TierPremium, Edition: "ee",
		},
		{Grain: grainPackage, Package: "internal/tools/projects", Field: "name", Operations: []string{"GET /projects/:project_id"}, Entity: "APIEntitiesProject", Sent: sentAlways},
		{Grain: grainPackage, Package: "internal/tools/projects", Field: "unlisted", Operations: []string{"GET /projects/:project_id"}, Entity: "APIEntitiesProject", Sent: sentUnknown},
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
// component the first operation carrying it named, and the condition
// record's answer for the field on that component, with a splat in the
// entity leaving the field it stands for unknown. The component is per field
// rather than per type because one type's operations resolve to different
// components: the field only the second operation carries is read from the
// second's component.
func TestTypedShapeCheck_FieldsGitLabSendsThatTheTypeDoesNotPublish_AreListed(t *testing.T) {
	twoTypes := structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs: []structs.OutputPairing{
			{Package: "mrapprovals", MCPType: "SummaryOutput", SDKType: "MergeRequestApprovals"},
			{Package: "mrapprovals", MCPType: "ConfigOutput", SDKType: "MergeRequestApprovals"},
		},
	}
	stubTypeGrainInputs(t, twoTypes, nil, approvalRoutes)
	root := t.TempDir()
	conditionsIn(t, root, map[string]apiexposes.Entity{
		"APIEntitiesApprovals": {File: "lib/api/entities/approvals.rb", Line: 3, Fields: []apiexposes.Field{
			{Name: "approved", Line: 4},
			{Name: "approvers", Line: 5, If: ":with_approvers"},
			{Name: "*Helper.attributes", Line: 6, Splat: true},
		}},
		"APIEntitiesApproved": {File: "lib/api/entities/approved.rb", Line: 3, Fields: []apiexposes.Field{
			{Name: "approved", Line: 4},
			{Name: "approved_at", Line: 5},
		}},
	})
	operations := map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {
			Entity: "APIEntitiesApprovals", Response: []string{"approved", "approvers", "attribute_a"},
		},
		"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve": {
			Entity: "APIEntitiesApproved", Response: []string{"approved", "approved_at", "approvers"},
		},
	}

	check := typedShapeCheck(root, indexOf(operations), []publishedType{
		{Package: "internal/tools/mrapprovals", Name: "SummaryOutput", Fields: []string{"approved", "attribute_a"}},
		{Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"approved"}},
	})

	searched := []string{"GET /projects/:/merge_requests/:/approvals", "POST /projects/:/merge_requests/:/approve"}
	want := []UnsurfacedField{
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "ConfigOutput", Field: "approved_at", Operations: searched, Entity: "APIEntitiesApproved", Sent: sentAlways},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "ConfigOutput", Field: "approvers", Operations: searched, Entity: "APIEntitiesApprovals", Sent: sentWhen, If: ":with_approvers"},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "ConfigOutput", Field: "attribute_a", Operations: searched, Entity: "APIEntitiesApprovals", Sent: sentUnknown},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "SummaryOutput", Field: "approved_at", Operations: searched, Entity: "APIEntitiesApproved", Sent: sentAlways},
		{Grain: grainType, Package: "internal/tools/mrapprovals", Type: "SummaryOutput", Field: "approvers", Operations: searched, Entity: "APIEntitiesApprovals", Sent: sentWhen, If: ":with_approvers"},
	}
	if !reflect.DeepEqual(check.Unsurfaced, want) {
		t.Errorf("Unsurfaced = %+v, want %+v", check.Unsurfaced, want)
	}
}

// TestSentCheck_TheComponentOfAField_IsTheFirstOperationNamingOne verifies
// the package grain's choice of component when the same field comes back
// from an operation the document names no component for and from one it
// does: the field is read on the named one whichever order the requests were
// recorded in, so that a request answered without a component does not hold
// the field at unknown.
func TestSentCheck_TheComponentOfAField_IsTheFirstOperationNamingOne(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{
		"GET /api/v4/things/{id}/plain": {Response: []string{"extra"}},
		"GET /api/v4/things/{id}":       {Entity: "APIEntitiesThing", Response: []string{"extra", "id"}},
	})
	conditionsIn(t, root, map[string]apiexposes.Entity{
		"APIEntitiesThing": {File: "lib/api/entities/thing.rb", Line: 3, Fields: []apiexposes.Field{{Name: "id", Line: 4}, {Name: "extra", Line: 5}}},
	})
	rows := []requestinventory.Row{
		{Package: "internal/tools/things", Kind: "rest", Method: "GET", Path: "/things/:thing_id/plain"},
		{Package: "internal/tools/things", Kind: "rest", Method: "GET", Path: "/things/:thing_id"},
	}

	check := shapeCheck(root, rows, []publishedType{{Package: "internal/tools/things", Name: "Output", Fields: []string{"id"}}})

	want := []UnsurfacedField{
		{Grain: grainPackage, Package: "internal/tools/things", Field: "extra", Operations: []string{"GET /things/:thing_id", "GET /things/:thing_id/plain"}, Entity: "APIEntitiesThing", Sent: sentAlways},
	}
	if !reflect.DeepEqual(check.Sent.Unsurfaced, want) {
		t.Errorf("Unsurfaced = %+v, want %+v", check.Sent.Unsurfaced, want)
	}
}

// TestSentCheck_WithoutAConditionsRecord_ListsFieldsAsUnknown verifies that
// the document alone still produces the list, with nothing said about when a
// field is sent, and that the check says the record was not read.
func TestSentCheck_WithoutAConditionsRecord_ListsFieldsAsUnknown(t *testing.T) {
	root := projectRecord(t)
	published := []publishedType{{Package: "internal/tools/projects", Name: "Output", Fields: []string{"archived", "extra", "id", "mirror", "name", "unlisted"}}}

	check := shapeCheck(root, projectRows(), published)

	if check.Sent.Ran || check.Sent.Record != "" {
		t.Errorf("Sent = %+v, want it to say the conditions record was not read", check.Sent)
	}
	want := []UnsurfacedField{{Grain: grainPackage, Package: "internal/tools/projects", Field: "star_count", Operations: []string{"GET /projects/:project_id"}, Entity: "APIEntitiesProject", Sent: sentUnknown}}
	if !reflect.DeepEqual(check.Sent.Unsurfaced, want) {
		t.Errorf("Unsurfaced = %+v, want %+v", check.Sent.Unsurfaced, want)
	}
}

// TestSentCheck_AComponentTheRecordDoesNotHold_IsUnknown verifies the other
// silence: a component the document names that the conditions record never
// read, which is what the components rendered outside the entity directories
// are, contributes findings the record cannot speak for.
func TestSentCheck_AComponentTheRecordDoesNotHold_IsUnknown(t *testing.T) {
	root := projectRecord(t)
	conditionsIn(t, root, map[string]apiexposes.Entity{"APIEntitiesOther": {File: "x.rb", Line: 1, Fields: []apiexposes.Field{}}})
	published := []publishedType{{Package: "internal/tools/projects", Name: "Output", Fields: []string{"archived", "extra", "id", "mirror", "name", "star_count"}}}

	check := shapeCheck(root, projectRows(), published)

	want := []UnsurfacedField{{Grain: grainPackage, Package: "internal/tools/projects", Field: "unlisted", Operations: []string{"GET /projects/:project_id"}, Entity: "APIEntitiesProject", Sent: sentUnknown}}
	if !check.Sent.Ran || !reflect.DeepEqual(check.Sent.Unsurfaced, want) {
		t.Errorf("Sent = %+v, want one unknown finding from a record that holds another component", check.Sent)
	}
}

// TestSentCheck_APackagePublishingNothingReadable_IsNotJudged verifies that a
// package the walk read no output type for is left out rather than reported
// as missing every field of every endpoint it calls.
func TestSentCheck_APackagePublishingNothingReadable_IsNotJudged(t *testing.T) {
	root := projectRecord(t)
	conditionsIn(t, root, projectConditions())

	check := shapeCheck(root, projectRows(), nil)

	if len(check.Sent.Unsurfaced) != 0 {
		t.Errorf("Unsurfaced = %+v, want nothing for a package with no published type", check.Sent.Unsurfaced)
	}
}
