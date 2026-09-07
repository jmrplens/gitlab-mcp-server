package paths

import (
	"slices"
	"testing"
)

// withEndpointDeclarations swaps the declaration table for one test, the way
// withDeclarations does for the silent owners, so a case exercises the shapes
// it declares and not the repository's own fifteen.
func withEndpointDeclarations(t *testing.T, table []endpointDeclaration) {
	t.Helper()
	original := declaredUndocumentedEndpoints
	declaredUndocumentedEndpoints = table
	t.Cleanup(func() { declaredUndocumentedEndpoints = original })
}

// TestClassifyUndocumented_Shapes_MatchWhatTheyWereWrittenFor verifies the
// matching a declaration rests on: a literal segment, a "*" standing for one
// segment whatever placeholder the fixture earned, a trailing "..." standing
// for the rest, and a method list that narrows a declaration to the method it
// was written about.
func TestClassifyUndocumented_Shapes_MatchWhatTheyWereWrittenFor(t *testing.T) {
	withEndpointDeclarations(t, []endpointDeclaration{
		{Shape: "/projects/*/services/...", Category: categoryUndocumentedAlias, Reason: "an alias"},
		{Shape: "/projects/*/repository/files/*/raw", Methods: []string{"HEAD"}, Category: categoryUndocumentedMethod, Reason: "a method"},
		{Shape: "/usage_data/track_events", Category: categoryDocumentedInProse, Reason: "prose"},
	})

	tests := []struct {
		name     string
		endpoint Endpoint
		want     string
	}{
		{"a family under a prefix", Endpoint{Method: "GET", Path: "/projects/:project_id/services/jira"}, categoryUndocumentedAlias},
		{"the same family deeper", Endpoint{Method: "PUT", Path: "/projects/:project_id/services/jira/settings"}, categoryUndocumentedAlias},
		{"the prefix itself", Endpoint{Method: "GET", Path: "/projects/:project_id/services"}, categoryUndocumentedAlias},
		{"a method the declaration names", Endpoint{Method: "HEAD", Path: "/projects/:project_id/repository/files/main.go/raw"}, categoryUndocumentedMethod},
		{"a method it does not", Endpoint{Method: "GET", Path: "/projects/:project_id/repository/files/main.go/raw"}, ""},
		{"an exact path", Endpoint{Method: "POST", Path: "/usage_data/track_events"}, categoryDocumentedInProse},
		{"a longer path than the declaration", Endpoint{Method: "POST", Path: "/usage_data/track_events/all"}, ""},
		{"a shorter one", Endpoint{Method: "POST", Path: "/usage_data"}, ""},
		{"a path nothing declares", Endpoint{Method: "GET", Path: "/projects/:project_id/statistics"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classified, _ := classifyUndocumented([]Endpoint{tt.endpoint})

			got := classified[0]
			if got.Category != tt.want {
				t.Errorf("category = %q, want %q", got.Category, tt.want)
			}
			if got.declared() != (tt.want != "") {
				t.Errorf("declared() = %v for category %q", got.declared(), got.Category)
			}
		})
	}
}

// TestClassifyUndocumented_ADeclarationMatchingNothing_IsNamed verifies the
// half that keeps the table honest: a declaration is an excuse for something,
// and one that excuses nothing has to be retired rather than left for a later
// reader to trust.
func TestClassifyUndocumented_ADeclarationMatchingNothing_IsNamed(t *testing.T) {
	withEndpointDeclarations(t, []endpointDeclaration{
		{Shape: "/orbit/...", Category: categoryUndocumentedAPI, Reason: "experimental"},
		{Shape: "/gone/...", Category: categoryUndocumentedAPI, Reason: "not any more"},
	})

	classified, unused := classifyUndocumented([]Endpoint{{Method: "GET", Path: "/orbit/status"}})

	if len(classified) != 1 || classified[0].Category != categoryUndocumentedAPI {
		t.Errorf("classified = %+v, want the orbit endpoint declared", classified)
	}
	if !slices.Equal(unused, []string{"/gone/..."}) {
		t.Errorf("unused = %v, want only the declaration nothing matched", unused)
	}
}

// TestEndpointCheck_StaleDeclarations_SayNothingWhenNothingRan verifies that a
// run which never compared anything makes no claim about the declarations.
// Every one of them is unused in that run, and reporting them all as stale
// would be the loudest possible wrong answer.
func TestEndpointCheck_StaleDeclarations_SayNothingWhenNothingRan(t *testing.T) {
	check := EndpointCheck{Ran: false, UnusedDeclarations: []string{"/orbit/..."}}

	if stale := check.staleDeclarations(); len(stale) != 0 {
		t.Errorf("staleDeclarations() = %v, want none from a comparison that did not run", stale)
	}
}

// TestDeclaredUndocumentedEndpoints_Table_IsWellFormed verifies the table
// itself: every entry carries a category and a reason, and no two entries
// declare the same shape, since the second one could then never be used and
// would be reported as stale forever.
func TestDeclaredUndocumentedEndpoints_Table_IsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range declaredUndocumentedEndpoints {
		t.Run(declaration.Shape, func(t *testing.T) {
			if declaration.Category == "" || declaration.Reason == "" {
				t.Errorf("declaration = %+v, want a category and a reason", declaration)
			}
			if seen[declaration.Shape] {
				t.Errorf("shape %q is declared twice", declaration.Shape)
			}
			seen[declaration.Shape] = true
		})
	}
}
