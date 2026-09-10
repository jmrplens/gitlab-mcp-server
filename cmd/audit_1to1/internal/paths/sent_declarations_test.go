package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// TestClassifySentFindings_ADeclaration_AnswersItsFindingsAtEitherGrain
// verifies the table's contract: a declaration naming a component covers
// every field read on it, or one field when it names one, at both grains
// with one use counted for both; a finding nothing covers is left as it is;
// a declaration that covers nothing is named stale, once; and an empty list
// of findings stays nil rather than becoming a list of none.
func TestClassifySentFindings_ADeclaration_AnswersItsFindingsAtEitherGrain(t *testing.T) {
	declarations := []sentDeclaration{
		{Package: "internal/tools/keys", Entity: "APIEntitiesUserWithAdmin", Field: declaredSegment, Category: categoryDocumentedNotSent, Reason: "the lookup presents a key"},
		{Package: "internal/tools/keys", Entity: "APIEntitiesSSHKeyWithUser", Field: "usage_type", Category: categoryDocumentedNotSent, Reason: "one field"},
		{Package: "internal/tools/keys", Entity: "APIEntitiesNobody", Field: declaredSegment, Category: categoryDocumentedNotSent, Reason: "stale"},
	}
	byPackage := []UnsurfacedField{
		{Grain: grainPackage, Package: "internal/tools/keys", Field: "bio", Entity: "APIEntitiesUserWithAdmin", Sent: sentAlways},
		{Grain: grainPackage, Package: "internal/tools/keys", Field: "expires_at", Entity: "APIEntitiesSSHKeyWithUser", Sent: sentAlways},
	}
	byType := []UnsurfacedField{
		{Grain: grainType, Package: "internal/tools/keys", Type: "Output", Field: "bio", Entity: "APIEntitiesUserWithAdmin", Sent: sentAlways},
		{Grain: grainType, Package: "internal/tools/keys", Type: "Output", Field: "usage_type", Entity: "APIEntitiesSSHKeyWithUser", Sent: sentAlways},
		{Grain: grainType, Package: "internal/tools/other", Type: "Output", Field: "bio", Entity: "APIEntitiesUserWithAdmin", Sent: sentAlways},
	}

	classifiedPackage, classifiedType, unused := classifySentFindings(declarations, byPackage, byType)

	wantPackage := []UnsurfacedField{
		{Grain: grainPackage, Package: "internal/tools/keys", Field: "bio", Entity: "APIEntitiesUserWithAdmin", Sent: sentAlways, Category: categoryDocumentedNotSent, Reason: "the lookup presents a key"},
		{Grain: grainPackage, Package: "internal/tools/keys", Field: "expires_at", Entity: "APIEntitiesSSHKeyWithUser", Sent: sentAlways},
	}
	wantType := []UnsurfacedField{
		{Grain: grainType, Package: "internal/tools/keys", Type: "Output", Field: "bio", Entity: "APIEntitiesUserWithAdmin", Sent: sentAlways, Category: categoryDocumentedNotSent, Reason: "the lookup presents a key"},
		{Grain: grainType, Package: "internal/tools/keys", Type: "Output", Field: "usage_type", Entity: "APIEntitiesSSHKeyWithUser", Sent: sentAlways, Category: categoryDocumentedNotSent, Reason: "one field"},
		{Grain: grainType, Package: "internal/tools/other", Type: "Output", Field: "bio", Entity: "APIEntitiesUserWithAdmin", Sent: sentAlways},
	}
	if !reflect.DeepEqual(classifiedPackage, wantPackage) {
		t.Errorf("package grain = %+v, want %+v", classifiedPackage, wantPackage)
	}
	if !reflect.DeepEqual(classifiedType, wantType) {
		t.Errorf("type grain = %+v, want %+v", classifiedType, wantType)
	}
	if !reflect.DeepEqual(unused, []string{"internal/tools/keys.APIEntitiesNobody.*"}) {
		t.Errorf("unused = %v, want the one declaration covering nothing", unused)
	}
	if always, when, declared := unsurfacedCounts(classifiedType); always != 3 || when != 0 || declared != 2 {
		t.Errorf("unsurfacedCounts() = %d always, %d when, %d declared; want 3, 0 and 2", always, when, declared)
	}

	nothingPackage, nothingType, none := classifySentFindings(declarations, nil, nil)
	if nothingPackage != nil || nothingType != nil || len(none) != 3 {
		t.Errorf("classifying nothing = %v, %v, %v; want nil, nil and every declaration unused", nothingPackage, nothingType, none)
	}
}

// TestSentCheck_StaleDeclarations_AreSilentUntilTheCheckRuns verifies that a
// check which did not run reports no stale declaration, since every one of
// them would be unused then, and that one which ran names each unused one
// with what a reader should look for.
func TestSentCheck_StaleDeclarations_AreSilentUntilTheCheckRuns(t *testing.T) {
	unused := []string{"internal/tools/keys.APIEntitiesUserWithAdmin.*"}
	if stale := (SentCheck{Ran: false, UnusedDeclarations: unused}).staleDeclarations(); stale != nil {
		t.Errorf("staleDeclarations() = %v before the check ran, want nothing", stale)
	}
	stale := (SentCheck{Ran: true, UnusedDeclarations: unused}).staleDeclarations()
	if len(stale) != 1 || stale[0][:len(unused[0])] != unused[0] {
		t.Errorf("staleDeclarations() = %v, want the one key with its explanation", stale)
	}
}

// TestDeclaredUnsurfaced_NamesWhatTheTreeHolds verifies the real table against
// the real tree, since an entry is a claim about it: each names a package
// under internal/tools, an entity the committed live record holds, a known
// category and a reason. What the reason claims about GitLab's source is not
// checked here; that is what the reason is written down for.
func TestDeclaredUnsurfaced_NamesWhatTheTreeHolds(t *testing.T) {
	root := repoRoot(t)
	doc, err := apilive.Read(filepath.Join(root, apilive.DefaultDir))
	if err != nil {
		t.Fatalf("read the live record: %v", err)
	}
	for _, declaration := range declaredUnsurfaced {
		t.Run(declaration.key(), func(t *testing.T) {
			if _, statErr := os.Stat(filepath.Join(root, declaration.Package)); statErr != nil {
				t.Errorf("package %s: %v", declaration.Package, statErr)
			}
			if _, held := doc.Entities[declaration.Entity]; !held {
				t.Errorf("entity %s is not in the live record", declaration.Entity)
			}
			known := declaration.Category == categoryDocumentedNotSent ||
				declaration.Category == categoryOptionNeverPassed ||
				declaration.Category == categoryOptionTurnedOff ||
				declaration.Category == categoryEntityPublishedElsewhere ||
				declaration.Category == categorySDKRouteNeverCalled ||
				declaration.Category == categorySDKRouteFillsAnotherType
			if !known || declaration.Reason == "" || declaration.Field == "" {
				t.Errorf("declaration %+v is missing its category, reason or field", declaration)
			}
		})
	}
}
