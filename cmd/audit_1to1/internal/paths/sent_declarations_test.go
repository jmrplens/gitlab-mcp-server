package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
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
	contradicted := []string{"internal/tools/keys.APIEntitiesUser.license is declared as " + categoryOptionNeverRequested + ", and it is not true"}
	if stale := (SentCheck{Ran: false, UnusedDeclarations: unused, ContradictedDeclarations: contradicted}).staleDeclarations(); stale != nil {
		t.Errorf("staleDeclarations() = %v before the check ran, want nothing", stale)
	}
	stale := (SentCheck{Ran: true, UnusedDeclarations: unused, ContradictedDeclarations: contradicted}).staleDeclarations()
	if len(stale) != 2 || stale[0][:len(unused[0])] != unused[0] || stale[1] != contradicted[0] {
		t.Errorf("staleDeclarations() = %v, want the unused key with its explanation, then the contradicted one as written", stale)
	}
}

// The spellings the option evidence fixture shares.
const (
	evidenceEntity    = "API::Entities::Proj"
	evidenceReader    = "internal/tools/reader"
	evidenceAsker     = "internal/tools/asker"
	evidenceProject   = "GET /projects/:project_id"
	evidenceLanguages = "GET /projects/:project_id/languages"
)

// optionEvidenceFixture is a record and an inventory in which every way the
// two option categories can be refuted is planted beside a case where the
// evidence holds.
//
// GET /projects/:id declares license and with_custom_attributes, the languages
// route declares nothing, reader calls the project route with no query at all,
// and asker calls it with statistics in its query and license in its body, so
// a name in either half of a request counts as sent. license_url shares
// license's option, which is how two findings come to fail one declaration in
// the same words.
func optionEvidenceFixture() optionEvidence {
	symbol := func(option string, inverse bool) []apilive.Condition {
		return []apilive.Condition{{Kind: "SymbolCondition", Symbol: option, Inverse: inverse}}
	}
	record := apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Entities: map[string]apilive.Entity{
			evidenceEntity: {Fields: []apilive.Field{
				{Name: "license", Conditions: symbol("license", false)},
				{Name: "license_url", Conditions: symbol("license", false)},
				{Name: "custom_attributes", Conditions: symbol("with_custom_attributes", false)},
				{Name: "stats", Conditions: symbol("include_stats", false)},
				{Name: "archived_only", Conditions: symbol("archived", true)},
				{Name: "reference", Conditions: []apilive.Condition{{Kind: "HashCondition", Hash: "{:with_reference=>true}"}}},
				{Name: "name"},
			}},
		},
		Routes: []apilive.Route{
			{Method: "GET", Path: apilive.EndpointPrefix + "/projects/:id", Params: map[string]apilive.Param{"id": {Required: true}, "license": {}, "with_custom_attributes": {}}},
			{Method: "GET", Path: apilive.EndpointPrefix + "/projects/:id/languages"},
		},
	}
	requests := []requestinventory.Row{
		{Package: evidenceReader, Kind: "rest", Method: "GET", Path: "/projects/:project_id"},
		{Package: evidenceAsker, Kind: "rest", Method: "GET", Path: "/projects/:project_id", Query: []string{"statistics"}, Body: []string{"license"}},
		{Package: evidenceAsker, Kind: "rest", Method: "GET", Path: "/projects/:project_id/languages"},
	}
	return newOptionEvidence(newConditionIndex(record), newOperationIndex(record), requests)
}

// TestOptionEvidence_AnOptionDeclaration_IsHeldToTheRouteAndTheInventory
// verifies that the two presenter-option categories are judged against their
// own evidence and not only against whether a finding still matches them.
//
// Each crossing is planted once: a package that sends the option it is
// declared never to request, a never-requested option no route declares, a
// field gated by no symbol condition at all or by an inverse one, and an
// option declared never passed that a route does declare. Beside them sit a
// never-requested option whose evidence holds with the declaring route listed
// before one that declares nothing, a never-passed option no route declares,
// a hash condition passed over, and a finding of another category left alone.
// Two findings failing one declaration in the same words are one entry, and a
// never-requested declaration that answers only type-grain findings is refused
// once, while one that is judged at package grain is not refused for also
// answering a type.
func TestOptionEvidence_AnOptionDeclaration_IsHeldToTheRouteAndTheInventory(t *testing.T) {
	declare := func(pkg, field, category string) sentDeclaration {
		return sentDeclaration{Package: pkg, Entity: evidenceEntity, Field: field, Category: category, Reason: "planted"}
	}
	declarations := []sentDeclaration{
		declare(evidenceReader, "license", categoryOptionNeverRequested),
		declare(evidenceReader, "custom_attributes", categoryOptionNeverRequested),
		declare(evidenceAsker, "license", categoryOptionNeverRequested),
		declare(evidenceReader, "stats", categoryOptionNeverRequested),
		declare(evidenceReader, "reference", categoryOptionNeverRequested),
		declare(evidenceReader, "archived_only", categoryOptionNeverRequested),
		declare(evidenceAsker, "custom_attributes", categoryOptionNeverPassed),
		declare(evidenceAsker, "stats", categoryOptionNeverPassed),
		declare(evidenceAsker, "reference", categoryOptionNeverPassed),
		declare("internal/tools/twice", declaredSegment, categoryOptionNeverPassed),
		declare("internal/tools/typeonly", "license", categoryOptionNeverRequested),
		declare(evidenceAsker, declaredSegment, categoryDocumentedNotSent),
	}
	finding := func(pkg, field string, operations ...string) UnsurfacedField {
		return UnsurfacedField{Grain: grainPackage, Package: pkg, Field: field, Entity: evidenceEntity, Operations: operations}
	}
	byPackage := []UnsurfacedField{
		finding(evidenceReader, "license", evidenceProject, evidenceLanguages),
		finding(evidenceReader, "custom_attributes", evidenceProject),
		finding(evidenceAsker, "license", evidenceProject),
		finding(evidenceReader, "stats", evidenceProject, evidenceLanguages),
		finding(evidenceReader, "reference", evidenceProject),
		finding(evidenceReader, "archived_only", evidenceProject),
		finding(evidenceAsker, "custom_attributes", evidenceProject),
		finding(evidenceAsker, "stats", evidenceProject, evidenceLanguages),
		finding(evidenceAsker, "reference", evidenceProject),
		finding("internal/tools/twice", "license", evidenceProject),
		finding("internal/tools/twice", "license_url", evidenceProject),
		finding(evidenceAsker, "name", evidenceProject),
		finding(evidenceReader, "name", evidenceProject),
	}
	typed := func(pkg, typeName, field string) UnsurfacedField {
		return UnsurfacedField{Grain: grainType, Package: pkg, Type: typeName, Field: field, Entity: evidenceEntity, Operations: []string{"GET /projects/:"}}
	}
	byType := []UnsurfacedField{
		typed("internal/tools/typeonly", "Output", "license"),
		typed("internal/tools/typeonly", "DetailOutput", "license"),
		typed(evidenceReader, "Output", "custom_attributes"),
		typed(evidenceAsker, "Output", "custom_attributes"),
		typed("internal/tools/nobody", "Output", "license"),
	}

	got := optionEvidenceFixture().contradictions(declarations, byPackage, byType)

	passedAs := " is declared as " + categoryOptionNeverPassed + ", and "
	requestedAs := " is declared as " + categoryOptionNeverRequested + ", and "
	noSymbol := " by no symbol condition naming an option a request could send"
	want := []string{
		evidenceAsker + "." + evidenceEntity + ".license" + requestedAs +
			"the package sends license on GET /projects/:project_id, so the key is on a response it reads: publish the field and drop the declaration",
		evidenceReader + "." + evidenceEntity + ".stats" + requestedAs +
			"no route the finding names declares include_stats as a parameter, so no request can ask for the key: the category is " + categoryOptionNeverPassed,
		evidenceReader + "." + evidenceEntity + ".reference" + requestedAs + "the record gates " + evidenceEntity + ".reference" + noSymbol,
		evidenceReader + "." + evidenceEntity + ".archived_only" + requestedAs + "the record gates " + evidenceEntity + ".archived_only" + noSymbol,
		evidenceAsker + "." + evidenceEntity + ".custom_attributes" + passedAs +
			"GET /projects/:project_id declares with_custom_attributes as a parameter, so a request can ask for the key: if this package never sends it " +
			"the category is " + categoryOptionNeverRequested + ", and if it does the field belongs on the surface",
		"internal/tools/twice." + evidenceEntity + ".*" + passedAs +
			"GET /projects/:project_id declares license as a parameter, so a request can ask for the key: if this package never sends it " +
			"the category is " + categoryOptionNeverRequested + ", and if it does the field belongs on the surface",
		"internal/tools/typeonly." + evidenceEntity + ".license" + requestedAs +
			"it answers type-grain findings alone: their operations are not rows of the request inventory, which is the evidence the category rests on, " +
			"so nothing can hold it to it",
	}
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("contradictions() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if none := optionEvidenceFixture().contradictions(declarations, nil, nil); none != nil {
		t.Errorf("contradictions() over no finding = %q, want nil", none)
	}
}

// TestOptionEvidence_TheRealTable_HoldsUntilThePackageAsksForTheOption
// verifies the crossing issue 973's review was about against the committed
// record and inventory rather than a fixture: attestations reads a project to
// probe it, never asks GET /projects/:id for its license, and so is declared
// never to receive the license pair. Planting one recorded request that asks
// for it has to turn both declarations from answers into refusals, since the
// finding they answer would stay exactly where it is.
func TestOptionEvidence_TheRealTable_HoldsUntilThePackageAsksForTheOption(t *testing.T) {
	root := repoRoot(t)
	record, err := apilive.Read(filepath.Join(root, apilive.DefaultDir))
	if err != nil {
		t.Fatalf("read the live record: %v", err)
	}
	inventory, err := requestinventory.Read(root)
	if err != nil {
		t.Fatalf("read the request inventory: %v", err)
	}
	attestations := toolsDir + "/attestations"
	findings := []UnsurfacedField{
		{Grain: grainPackage, Package: attestations, Field: "license", Entity: projectWithAccessEntity, Operations: []string{evidenceProject}},
		{Grain: grainPackage, Package: attestations, Field: "license_url", Entity: projectWithAccessEntity, Operations: []string{evidenceProject}},
	}
	evidence := func(requests []requestinventory.Row) optionEvidence {
		return newOptionEvidence(newConditionIndex(record), newOperationIndex(record), requests)
	}

	if got := evidence(inventory.Requests).contradictions(declaredUnsurfaced, findings, nil); got != nil {
		t.Fatalf("contradictions() = %q on the committed inventory, want the declarations to hold", got)
	}

	planted := append(slices.Clone(inventory.Requests), requestinventory.Row{
		Package: attestations, Kind: "rest", Method: "GET", Path: "/projects/:project_id", Query: []string{"license"},
	})
	got := evidence(planted).contradictions(declaredUnsurfaced, findings, nil)
	refusal := " is declared as " + categoryOptionNeverRequested + ", and the package sends license on " + evidenceProject +
		", so the key is on a response it reads: publish the field and drop the declaration"
	want := []string{
		attestations + "." + projectWithAccessEntity + ".license" + refusal,
		attestations + "." + projectWithAccessEntity + ".license_url" + refusal,
	}
	if !slices.Equal(got, want) {
		t.Errorf("contradictions() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestDeclaredUnsurfaced_NamesWhatTheTreeHolds verifies the real table against
// the real tree, since an entry is a claim about it: each names a package
// under internal/tools, a type that package declares when it narrows to one,
// an entity the committed live record holds, a known category and a reason.
// What the reason claims about GitLab's source is not checked here; that is
// what the reason is written down for.
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
			} else if declaration.Type != "" && !packageTypeNames(t, filepath.Join(root, declaration.Package))[declaration.Type] {
				t.Errorf("package %s declares no type %s", declaration.Package, declaration.Type)
			}
			if _, held := doc.Entities[declaration.Entity]; !held {
				t.Errorf("entity %s is not in the live record", declaration.Entity)
			}
			known := declaration.Category == categoryDocumentedNotSent ||
				declaration.Category == categoryOptionNeverPassed ||
				declaration.Category == categoryOptionTurnedOff ||
				declaration.Category == categoryOptionNeverRequested ||
				declaration.Category == categoryEntityPublishedElsewhere ||
				declaration.Category == categorySDKRouteNeverCalled ||
				declaration.Category == categorySDKRouteFillsAnotherType ||
				declaration.Category == categorySubclassCannotSatisfy ||
				declaration.Category == categoryAbilityNoRoleGrants ||
				declaration.Category == categoryConstantEmpty
			if !known || declaration.Reason == "" || declaration.Field == "" {
				t.Errorf("declaration %+v is missing its category, reason or field", declaration)
			}
		})
	}
}

// TestDeclaredUnsurfaced_AccessRequestTypes_AnswerOnlyTheOtherEntitysOwnKeys
// crosses the two Type-scoped access-request declarations against the
// committed record, through the same join the audit makes: the six routes
// client-go reads AccessRequest from, absorbed in the order readSDKRoutes
// sorts them.
//
// Both entities merge UserBasic, so the case that matters is a key both carry
// (locked). It has to be credited to AccessRequester, since that is what keeps
// the splat over Member on Output from answering it; and on either type it has
// to stay a finding, since the lists send it to Output and the approve routes
// send it to MemberOutput. Beside it, the one key only AccessRequester carries
// is answered on MemberOutput and a key only Member carries on Output.
func TestDeclaredUnsurfaced_AccessRequestTypes_AnswerOnlyTheOtherEntitysOwnKeys(t *testing.T) {
	doc, err := apilive.Read(filepath.Join(repoRoot(t), apilive.DefaultDir))
	if err != nil {
		t.Fatalf("read the live record: %v", err)
	}
	found := map[string]map[sdkRoute]bool{"AccessRequest": {}}
	for _, scope := range []string{"/groups/:", "/projects/:"} {
		found["AccessRequest"][sdkRoute{Method: "GET", Path: scope + "/access_requests", Many: true}] = true
		found["AccessRequest"][sdkRoute{Method: "POST", Path: scope + "/access_requests"}] = true
		found["AccessRequest"][sdkRoute{Method: "PUT", Path: scope + "/access_requests/:/approve"}] = true
	}
	described := describedRoutes([]string{"AccessRequest"}, sortedRoutes(found), newOperationIndex(doc))
	if len(described.Operations) != 6 {
		t.Fatalf("describedRoutes() read %d operations, want the six access-request routes: %v", len(described.Operations), described.Operations)
	}
	for key, want := range map[string]string{"locked": accessRequesterEntity, "requested_at": accessRequesterEntity, "access_level": memberEntity} {
		t.Run("credited "+key, func(t *testing.T) {
			if got := described.EntityOf[key]; got != want {
				t.Errorf("EntityOf[%q] = %q, want %q", key, got, want)
			}
		})
	}

	finding := func(typeName, field string) UnsurfacedField {
		return UnsurfacedField{Grain: grainType, Package: accessRequestsPkg, Type: typeName, Field: field, Entity: described.EntityOf[field], Sent: sentAlways}
	}
	cases := []struct {
		name         string
		finding      UnsurfacedField
		wantCategory string
	}{
		{name: "a shared key MemberOutput drops stays a finding", finding: finding("MemberOutput", "locked")},
		{name: "a shared key Output drops stays a finding", finding: finding("Output", "locked")},
		{name: "the requester's own key is answered on MemberOutput", finding: finding("MemberOutput", "requested_at"), wantCategory: categorySDKRouteFillsAnotherType},
		{name: "a member's own key is answered on Output", finding: finding("Output", "access_level"), wantCategory: categorySDKRouteFillsAnotherType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, classified, _ := classifySentFindings(declaredUnsurfaced, nil, []UnsurfacedField{tc.finding})
			if len(classified) != 1 || classified[0].Category != tc.wantCategory {
				t.Errorf("classifySentFindings(%+v) = %+v, want category %q", tc.finding, classified, tc.wantCategory)
			}
		})
	}
}
