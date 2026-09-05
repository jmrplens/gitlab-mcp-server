package actions

import (
	"encoding/json"
	"errors"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestRun_MarshalFailure_IsReported reaches the encoding branch through the
// seam, since a report of strings and ints never fails to encode on its own.
func TestRun_MarshalFailure_IsReported(t *testing.T) {
	original := marshalIndent
	t.Cleanup(func() { marshalIndent = original })
	marshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("boom") }

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	if _, err = Run(root, true); err == nil || !strings.Contains(err.Error(), "marshal report: boom") {
		t.Fatalf("Run = %v, want the marshal failure", err)
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

// endpointInterface declares a client-go service interface named name whose
// methods all carry the variadic ...RequestOptionFunc endpoint marker, which
// is all the coverage adjudication needs to resolve them as API methods.
func endpointInterface(name string, methods ...string) *types.Named {
	pkg := types.NewPackage(shared.ClientGoPkgPath+"/v2", "gitlab")
	optionFunc := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "RequestOptionFunc", nil),
		types.NewSignatureType(nil, nil, nil, nil, nil, false), nil,
	)
	tail := types.NewParam(token.NoPos, pkg, "", types.NewSlice(optionFunc))
	funcs := make([]*types.Func, 0, len(methods))
	for _, method := range methods {
		sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(tail), nil, true)
		funcs = append(funcs, types.NewFunc(token.NoPos, pkg, method, sig))
	}
	iface := types.NewInterfaceType(funcs, nil)
	iface.Complete()
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), iface, nil)
}

// TestCoverageForService_Usage_AdjudicatesAcceptedMethods verifies the
// per-service coverage: a called method and an adjudicated missing method
// both count as covered, the rest are the sorted missing list, and the
// referencing packages are listed sorted.
func TestCoverageForService_Usage_AdjudicatesAcceptedMethods(t *testing.T) {
	service := endpointInterface("SearchServiceInterface", "Commits", "Milestones", "Blobs", "Users")
	use := &shared.ServiceUsage{
		Named:    service,
		Called:   map[string]struct{}{"Commits": {}},
		Packages: map[string]struct{}{"search": {}, "groups": {}},
	}
	got := coverageForService(use)
	want := serviceCoverage{
		Service:        "SearchServiceInterface",
		Packages:       []string{"groups", "search"},
		APIMethods:     4,
		CoveredMethods: 2,
		MissingMethods: []string{"Blobs", "Users"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("coverageForService = %+v, want %+v", got, want)
	}
}

// TestIsAcceptedMissingMethod_Keys_MatchServiceAndMethod verifies the
// adjudication lookup is keyed on the bare service name plus the method.
func TestIsAcceptedMissingMethod_Keys_MatchServiceAndMethod(t *testing.T) {
	cases := []struct {
		name    string
		service string
		method  string
		want    bool
	}{
		{name: "adjudicated_generic_search", service: "Search", method: "Milestones", want: true},
		{name: "adjudicated_graphql_epic", service: "Epics", method: "GetEpic", want: true},
		{name: "unlisted_method", service: "Search", method: "Blobs", want: false},
		{name: "service_suffix_is_not_part_of_the_key", service: "SearchServiceInterface", method: "Milestones", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAcceptedMissingMethod(tc.service, tc.method); got != tc.want {
				t.Errorf("isAcceptedMissingMethod(%q, %q) = %v, want %v", tc.service, tc.method, got, tc.want)
			}
		})
	}
}

// TestRun_Roots_EmitsJSONOrLoadError verifies the command-facing entry
// point: a root the loader cannot enter fails the run, and the repository
// root yields the report as indented JSON naming the client-go path.
func TestRun_Roots_EmitsJSONOrLoadError(t *testing.T) {
	if _, err := Run(filepath.Join(t.TempDir(), "absent"), true); err == nil || !strings.Contains(err.Error(), "load packages") {
		t.Fatalf("Run on a missing root = %v, want a load error", err)
	}

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, err := Run(root, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("report lacks the trailing newline")
	}
	var rep report
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.SchemaVersion != shared.SchemaVersion || rep.ClientGoPath != shared.ClientGoPkgPath {
		t.Errorf("report header = %d/%q, want %d/%q", rep.SchemaVersion, rep.ClientGoPath, shared.SchemaVersion, shared.ClientGoPkgPath)
	}
	for _, svc := range rep.Services {
		if len(svc.MissingMethods) == 0 {
			t.Errorf("gaps-only report kept %s, which has no missing method", svc.Service)
		}
	}
}
