// Unit tests for the service universe: how a Client struct field becomes a
// service entry, how a status is assigned, and when a declaration goes stale.

package sdk

import (
	"go/types"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
)

// TestCollectSDKServices_NoClientStruct_Fails verifies the enumeration refuses
// a package that declares no Client struct instead of returning an empty
// universe that would audit as clean. The loader-backed cases cannot reach
// this branch, because such a package never resolves as the client-go root.
func TestCollectSDKServices_NoClientStruct_Fails(t *testing.T) {
	pkg := types.NewPackage(shared.ClientGoPkgPath+"/v2", "gitlab")
	services, err := collectSDKServices(pkg)
	if err == nil || !strings.Contains(err.Error(), "Client struct not found") {
		t.Fatalf("collectSDKServices = %v, want a not-found error", err)
	}
	if services != nil {
		t.Errorf("services = %+v, want none", services)
	}
}

// TestAdjudicateServices_Statuses_PreferCallsOverDeclarations verifies the
// order the three statuses are decided in: a call wins over a declaration
// (which is what makes such a declaration stale rather than authoritative), a
// declaration carries its category and reason through, and anything else is
// the finding.
func TestAdjudicateServices_Statuses_PreferCallsOverDeclarations(t *testing.T) {
	services := []sdkService{
		{Service: "Branches", Interface: "BranchesServiceInterface"},
		{Service: "Widgets", Interface: "WidgetsServiceInterface"},
		{Service: "Gadgets", Interface: "GadgetsServiceInterface"},
		{Service: "Sprockets", Interface: "SprocketsServiceInterface"},
	}
	usage := map[string]*shared.ServiceUsage{
		"BranchesServiceInterface": {Packages: map[string]struct{}{"tags": {}, "branches": {}}},
		"WidgetsServiceInterface":  {Packages: map[string]struct{}{"widgets": {}}},
	}
	declaredBy := map[string]declaration{
		"Widgets": {coveredRaw, "declared even though it is called"},
		"Gadgets": {unwrappedTracked, "tracked in an issue"},
	}

	got := adjudicateServices(services, usage, declaredBy)
	want := []sdkService{
		{Service: "Branches", Interface: "BranchesServiceInterface", Status: statusCovered, Packages: []string{"branches", "tags"}},
		{Service: "Widgets", Interface: "WidgetsServiceInterface", Status: statusCovered, Packages: []string{"widgets"}},
		{Service: "Gadgets", Interface: "GadgetsServiceInterface", Status: statusDeclared, Category: unwrappedTracked, Reason: "tracked in an issue"},
		{Service: "Sprockets", Interface: "SprocketsServiceInterface", Status: statusUndeclared},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("adjudicateServices = %+v, want %+v", got, want)
	}
}

// TestStaleServiceDeclarations_Cases_NameBothWaysADeclarationExpires verifies
// the two ways a declaration stops describing reality, and that a declaration
// still doing its job is not reported.
func TestStaleServiceDeclarations_Cases_NameBothWaysADeclarationExpires(t *testing.T) {
	services := []sdkService{
		{Service: "Branches", Status: statusCovered},
		{Service: "Gadgets", Status: statusDeclared},
		{Service: "Sprockets", Status: statusUndeclared},
	}
	declaredBy := map[string]declaration{
		"Branches": {coveredRaw, "now called directly"},
		"Gadgets":  {unwrappedTracked, "still not exposed"},
		"Removed":  {coveredRaw, "upstream dropped the service"},
	}
	want := []string{
		"service Branches (now called directly)",
		"service Removed (no longer declared by client-go)",
	}
	if got := staleServiceDeclarations(services, declaredBy); !reflect.DeepEqual(got, want) {
		t.Errorf("staleServiceDeclarations = %v, want %v", got, want)
	}
	if got := staleServiceDeclarations(services, nil); got != nil {
		t.Errorf("staleServiceDeclarations with no table = %v, want nil", got)
	}
}

// TestServiceIndex_Names_KeysByBareServiceName verifies the lookup the GraphQL
// rule resolves a package against is keyed on the bare service name, not the
// interface or the struct field.
func TestServiceIndex_Names_KeysByBareServiceName(t *testing.T) {
	index := serviceIndex([]sdkService{
		{Service: "IssueBoards", Field: "Boards", Interface: "IssueBoardsServiceInterface", APIMethods: 7},
	})
	service, ok := index["IssueBoards"]
	if !ok || service.APIMethods != 7 {
		t.Fatalf("index[IssueBoards] = %+v, %v", service, ok)
	}
	for _, absent := range []string{"Boards", "IssueBoardsServiceInterface"} {
		t.Run(absent, func(t *testing.T) {
			if _, found := index[absent]; found {
				t.Errorf("index is keyed on %q, want the bare service name only", absent)
			}
		})
	}
}
