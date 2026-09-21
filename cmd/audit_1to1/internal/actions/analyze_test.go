package actions

import (
	"encoding/json"
	"errors"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
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
// the services-with-gaps tally. Two services have findings and one has none,
// because a table with one of each counts the same either way round: the tally
// would read 1 whether it counts the services with findings or the services
// without, and a summary that reports every clean service as a gap is the one
// thing this number exists to rule out. Every count differs from every other
// for the same reason, so no two of the four accumulators can be swapped
// without the assertion noticing.
func TestSummarize_AggregatesServiceCounts(t *testing.T) {
	s := summarize([]serviceCoverage{
		{Service: "A", APIMethods: 4, CoveredMethods: 4},
		{Service: "B", APIMethods: 6, CoveredMethods: 2, MissingMethods: []string{"X", "Y", "Z", "W"}},
		{Service: "C", APIMethods: 3, CoveredMethods: 1, MissingMethods: []string{"P", "Q"}},
	})
	if s.Services != 3 || s.ServicesWithGaps != 2 {
		t.Errorf("service counts = %d/%d, want 3/2", s.Services, s.ServicesWithGaps)
	}
	if s.APIMethods != 13 || s.CoveredMethods != 7 || s.MissingMethods != 6 {
		t.Errorf("method counts = %d/%d/%d, want 13/7/6", s.APIMethods, s.CoveredMethods, s.MissingMethods)
	}
}

// withAdjudication installs table as the adjudication for the duration of the
// test. It is what lets a test say how much of the backlog is answered instead
// of inheriting whatever the tree answers today, so an assertion about the
// filter does not turn into an assertion about the size of the backlog.
func withAdjudication(t *testing.T, table map[string]string) {
	t.Helper()
	original := acceptedMissingMethods
	t.Cleanup(func() { acceptedMissingMethods = original })
	acceptedMissingMethods = table
}

// TestBuildReport_GapsOnly_KeepsExactlyTheServicesWithFindings holds the
// -gaps-only filter to both of its halves: asked for gaps it keeps every
// service that has a missing method and drops the rest, and not asked it keeps
// the clean ones too. Asserting only the first half leaves the filter free to
// drop everything, which a caller reads as an audit with nothing to report.
// The adjudication is emptied so both halves exist whatever the backlog looks
// like: with nothing answered, a service every one of whose API methods a
// handler calls is clean and one with an uncalled method is a finding.
func TestBuildReport_GapsOnly_KeepsExactlyTheServicesWithFindings(t *testing.T) {
	withAdjudication(t, map[string]string{})
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	full, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("full buildReport: %v", err)
	}
	gaps, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("gaps-only buildReport: %v", err)
	}

	var withFindings []serviceCoverage
	for _, svc := range full.Services {
		if len(svc.MissingMethods) > 0 {
			withFindings = append(withFindings, svc)
		}
	}
	switch {
	case len(withFindings) == 0:
		t.Fatal("no service has a missing method, so dropping the clean ones cannot be observed")
	case len(withFindings) == len(full.Services):
		t.Fatal("every service has a missing method, so keeping the clean ones cannot be observed")
	}
	if !reflect.DeepEqual(gaps.Services, withFindings) {
		t.Errorf("gaps-only kept %d services, want the %d of the %d in the full report that have findings",
			len(gaps.Services), len(withFindings), len(full.Services))
	}
	if full.Summary.ServicesWithGaps != len(withFindings) {
		t.Errorf("summary counted %d services with gaps, want %d", full.Summary.ServicesWithGaps, len(withFindings))
	}
	// The summary is computed after the filter, so each report describes what
	// it carries. Summarizing before it would leave the gaps-only file saying
	// it covers services it does not list, which is the shape a reader would
	// take for a coverage figure over the whole SDK.
	if full.Summary.Services != len(full.Services) {
		t.Errorf("full summary counted %d services over %d listed", full.Summary.Services, len(full.Services))
	}
	if gaps.Summary.Services != len(gaps.Services) {
		t.Errorf("gaps-only summary counted %d services over %d listed", gaps.Summary.Services, len(gaps.Services))
	}
}

// TestBuildReport_StaleAcceptances_NameTheDeclarationsWhoseMethodIsCalled
// holds the adjudication table's second claim. The first is that an entry
// answers an uncovered method; the second is that an entry whose method a
// handler does call is itself a finding, since the table then claims the
// endpoint is reached another way while the scanner sees the direct call. Only
// the first had a test, and the shape of that claim is invisible to both
// gates: no operator decides it and no condition is left unevaluated by it.
// Both directions are asserted here, keyed the way the adjudication table is:
// on the bare service name, which is not the spelling the report's own
// `service` field carries, since that one keeps the ServiceInterface suffix.
func TestBuildReport_StaleAcceptances_NameTheDeclarationsWhoseMethodIsCalled(t *testing.T) {
	withAdjudication(t, map[string]string{
		"Branches.ListBranches":                 "called directly by internal/tools/branches, so this entry is stale",
		"Branches.GetBranch":                    "called directly by internal/tools/branches, so this entry is stale",
		"Branches.NeverCalled":                  "answers a method no handler calls, so it is a live adjudication",
		"NoSuchService.Whatever":                "names a service the scanner never saw, so it is not stale either",
		"BranchesServiceInterface.ListBranches": "spells the interface name in full, which is not how this table is keyed",
	})
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	want := []string{"Branches.GetBranch", "Branches.ListBranches"}
	if !reflect.DeepEqual(rep.StaleAcceptances, want) {
		t.Errorf("stale acceptances = %v, want %v", rep.StaleAcceptances, want)
	}
}

// TestBuildReport_CommittedAdjudication_ReportsNoStaleEntry holds the same
// claim against the tree rather than against a table the test wrote, which is
// the half a synthetic case cannot state. The rule has to read the methods a
// handler *calls*; computing it over the methods the interface *declares*
// leaves every synthetic case green, because the two branch names a synthetic
// table lists are both, and turns every live adjudication on the real table
// into a finding. It also names the entry that was already stale when this was
// written: Integrations.ListActiveGroupIntegrations, adjudicated as reached
// through the generic slug dispatcher while internal/tools/integrations calls
// the wrapper, which the report has been saying and nothing has been reading.
func TestBuildReport_CommittedAdjudication_ReportsNoStaleEntry(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if len(rep.StaleAcceptances) != 0 {
		t.Errorf("acceptedMissingMethods adjudicates %d method(s) a handler calls directly, so each claims a coverage route that is not the one taken: %v",
			len(rep.StaleAcceptances), rep.StaleAcceptances)
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
	// Strictly increasing, not merely sorted: shared.CollectServiceUsage keys
	// its map on the same interface name this field carries, so two entries
	// can never tie and the comparator's `<` is never asked about equals. That
	// is why its boundary survives mutation, and asserting the uniqueness is
	// the property behind the branch rather than the branch itself.
	var prev string
	for _, svc := range rep.Services {
		if svc.Service <= prev {
			t.Errorf("services not in strictly increasing order: %q before %q", prev, svc.Service)
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

// TestRun_Layout_IsTwoSpacePerLevelWithNoLinePrefix pins the two arguments
// json.MarshalIndent is given, which are interchangeable to every assertion
// that only parses the result: swapping them for a two-space prefix and no
// indent still yields valid JSON that still ends in a closing brace, so the
// backlog would silently stop being a readable diff. Both halves are asserted
// where the swap moves them: the closing brace leaves column zero, and a
// nested key stops being indented under its parent.
func TestRun_Layout_IsTwoSpacePerLevelWithNoLinePrefix(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, err := Run(root, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(string(content), "\n}\n") {
		t.Error("the closing brace is not at column zero, so the encoder was given a line prefix")
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	summary := slices.Index(lines, `  "summary": {`)
	if summary < 0 {
		t.Fatalf("no first-level summary key indented by two spaces in:\n%s", strings.Join(lines[:min(len(lines), 6)], "\n"))
	}
	if next := lines[summary+1]; !strings.HasPrefix(next, `    "services": `) {
		t.Errorf("the key under summary reads %q, want it indented by four spaces", next)
	}
}
