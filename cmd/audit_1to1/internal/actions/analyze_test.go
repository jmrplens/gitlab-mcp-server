package actions

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestShortPackage_ExtractsDomain verifies the internal/tools domain extraction
// and its fallbacks for paths outside the tools tree.
func TestShortPackage_ExtractsDomain(t *testing.T) {
	cases := map[string]string{
		"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branches": "branches",
		"github.com/x/internal/tools/group/sub":                            "group/sub",
		"flat":                                                             "flat",
		"a/b/c":                                                            "c",
	}
	for input, want := range cases {
		if got := shortPackage(input); got != want {
			t.Errorf("shortPackage(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestSortedSet_SortsAndDedups verifies set→sorted-slice conversion.
func TestSortedSet_SortsAndDedups(t *testing.T) {
	got := sortedSet(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedSet = %v, want %v", got, want)
	}
	if sortedSet(map[string]struct{}{}) == nil {
		t.Error("sortedSet of empty map should be non-nil empty slice")
	}
}

// TestSummarize_AggregatesServiceCounts verifies summary aggregation including
// the services-with-gaps tally.
func TestSummarize_AggregatesServiceCounts(t *testing.T) {
	s := summarize([]serviceCoverage{
		{Service: "A", APIMethods: 4, CoveredMethods: 4},
		{Service: "B", APIMethods: 6, CoveredMethods: 2, MissingMethods: []string{"X", "Y", "Z", "W"}},
	})
	if s.Services != 2 || s.ServicesWithGaps != 1 {
		t.Errorf("service counts = %d/%d, want 2/1", s.Services, s.ServicesWithGaps)
	}
	if s.APIMethods != 10 || s.CoveredMethods != 6 || s.MissingMethods != 4 {
		t.Errorf("method counts = %d/%d/%d, want 10/6/4", s.APIMethods, s.CoveredMethods, s.MissingMethods)
	}
}

// TestBuildReport_ResolvesSDKCoverage runs the auditor against the repository and
// asserts fix-agnostic invariants: every used service resolves API methods,
// coverage never exceeds the method count, and the output is sorted. It is the
// methodology regression guard for the SDK-method resolver.
func TestBuildReport_ResolvesSDKCoverage(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if rep.Summary.APIMethods == 0 || rep.Summary.CoveredMethods == 0 {
		t.Fatalf("summary looks empty: %+v", rep.Summary)
	}
	var prev string
	for _, svc := range rep.Services {
		if svc.Service < prev {
			t.Errorf("services not sorted: %q before %q", prev, svc.Service)
		}
		prev = svc.Service
		if svc.CoveredMethods > svc.APIMethods {
			t.Errorf("service %s covered %d > api %d", svc.Service, svc.CoveredMethods, svc.APIMethods)
		}
		if svc.APIMethods == 0 {
			t.Errorf("service %s resolved zero API methods", svc.Service)
		}
		if len(svc.Packages) == 0 {
			t.Errorf("service %s has no referencing packages", svc.Service)
		}
	}
}

// TestBuildReport_Deterministic verifies repeated runs are byte-identical, a
// prerequisite for committing the report as a backlog.
func TestBuildReport_Deterministic(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	first, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("first buildReport: %v", err)
	}
	second, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("second buildReport: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("buildReport is not deterministic across runs")
	}
}
