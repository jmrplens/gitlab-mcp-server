// Tests for the two human-maintained adjudication tables. They check the shape
// an entry must have, not the judgement it records: a category from the agreed
// vocabulary, a reason long enough to be evidence rather than a shrug, and keys
// that name something real. A table nobody can check is how an audit turns into
// a list of assertions.

package sdk

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// minReasonLength is the shortest a reason can be and still say why. It is a
// smell test, not a measurement: "n/a" and "see above" fall under it, every
// real entry clears it comfortably.
const minReasonLength = 40

// TestDeclaredServices_Entries_CarryAKnownCategoryAndEvidence verifies every
// service declaration is well formed: a category from the agreed set, a
// substantial reason, and a key that is a bare service name rather than a
// Client struct field or an interface name.
func TestDeclaredServices_Entries_CarryAKnownCategoryAndEvidence(t *testing.T) {
	known := map[string]bool{
		coveredRaw: true, coveredGeneric: true, coveredGraphQL: true,
		supersededUpstream: true, unwrappedTracked: true,
	}
	for service, declared := range declaredServices {
		t.Run(service, func(t *testing.T) {
			if !known[declared.Category] {
				t.Errorf("category %q is not one of the declared categories", declared.Category)
			}
			if len(declared.Reason) < minReasonLength {
				t.Errorf("reason %q is too short to be evidence", declared.Reason)
			}
			if strings.HasSuffix(service, "ServiceInterface") {
				t.Error("key is an interface name; the table is keyed on bare service names")
			}
			if strings.TrimSpace(service) != service || service == "" {
				t.Errorf("key %q is not a bare service name", service)
			}
		})
	}
}

// TestGraphQLDecisions_Entries_CarryAKnownVerdictAndEvidence verifies every
// GraphQL decision is well formed and keyed "<package>.<function>".
func TestGraphQLDecisions_Entries_CarryAKnownVerdictAndEvidence(t *testing.T) {
	for key, adjudged := range graphqlDecisions {
		t.Run(key, func(t *testing.T) {
			if adjudged.Decision != decisionKeep && adjudged.Decision != decisionMigrate {
				t.Errorf("decision %q is neither %s nor %s", adjudged.Decision, decisionKeep, decisionMigrate)
			}
			if len(adjudged.Reason) < minReasonLength {
				t.Errorf("reason %q is too short to be evidence", adjudged.Reason)
			}
			pkg, function, ok := strings.Cut(key, ".")
			if !ok || pkg == "" || function == "" {
				t.Errorf("key %q is not <package>.<function>", key)
			}
			if strings.ToLower(pkg) != pkg {
				t.Errorf("key %q does not start with a package name", key)
			}
		})
	}
}

// TestGraphQLDecisions_Migrations_NameWhereTheyAreTracked verifies a MIGRATE
// verdict points somewhere, so the audit cannot be left green by writing
// "should move" and never moving it.
func TestGraphQLDecisions_Migrations_NameWhereTheyAreTracked(t *testing.T) {
	for key, adjudged := range graphqlDecisions {
		if adjudged.Decision != decisionMigrate {
			continue
		}
		t.Run(key, func(t *testing.T) {
			if !strings.Contains(adjudged.Reason, "://") {
				t.Errorf("MIGRATE reason %q names no tracking link", adjudged.Reason)
			}
		})
	}
}

// TestGraphQLServiceAliases_Targets_NameARealSDKService verifies every alias
// resolves against the SDK this project compiles with. An alias whose target
// does not exist silently disables the rule for that package, which is exactly
// the class of blind spot this scope was added to remove.
func TestGraphQLServiceAliases_Targets_NameARealSDKService(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		t.Fatalf("client-go types: %v", err)
	}
	services, err := collectSDKServices(clientGo)
	if err != nil {
		t.Fatalf("collectSDKServices: %v", err)
	}
	index := serviceIndex(services)
	for pkg, target := range graphqlServiceAliases {
		t.Run(pkg+"_to_"+target, func(t *testing.T) {
			if _, ok := index[target]; !ok {
				t.Errorf("alias target %q is not a client-go service", target)
			}
			if _, sameName := index[strings.ToUpper(pkg[:1])+pkg[1:]]; sameName {
				t.Errorf("package %q already matches a service by name; the alias is redundant", pkg)
			}
		})
	}
}
