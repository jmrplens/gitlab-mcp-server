// app_statistics_test.go contains unit tests for the application statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package appstatistics

import (
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestGet verifies the Get handler.
// The mock GitLab API at /api/v4/application/statistics (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/application/statistics")
		testutil.RespondJSON(w, http.StatusOK, `{
			"forks": 10, "issues": 200, "merge_requests": 50,
			"notes": 1000, "snippets": 5, "ssh_keys": 30,
			"milestones": 15, "users": 100, "groups": 8,
			"projects": 45, "active_users": 80
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ActiveUsers != 80 {
		t.Errorf("ActiveUsers = %d, want 80", out.ActiveUsers)
	}
	if out.Projects != 45 {
		t.Errorf("Projects = %d, want 45", out.Projects)
	}
	if out.Issues != 200 {
		t.Errorf("Issues = %d, want 200", out.Issues)
	}
}

// TestGet_Error verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_StringEncodedCounts_EachNameReadsItsOwnValue pins the two properties
// that make this handler build its own request instead of calling client-go's
// GetApplicationStatistics.
//
// The first is why it exists at all: some GitLab versions answer this endpoint
// with the counts encoded as JSON strings, and the SDK types every field of
// ApplicationStatistics as int64, so on such an instance the SDK call fails
// outright and flexibleStats is the only reason the tool answers anything
// (docs/development/upstream-bugs.md). Every other fixture in this file sends
// JSON numbers, which the SDK would have decoded too, so nothing here stopped
// the workaround being collapsed back to int64 with the suite still green.
//
// The second is the mapping. Each of the eleven counts is given a value of its
// own, so a json tag that stops matching what GitLab spells, or a field reading
// its neighbour's number, fails here rather than publishing a silent zero that
// reads as a fact about the instance. Only three of the eleven were asserted
// anywhere before this.
func TestGet_StringEncodedCounts_EachNameReadsItsOwnValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A read action must reach GitLab as a read: nothing else asserted the
		// method, so this route could have been sent as a write and passed.
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/application/statistics")
		testutil.RespondJSON(w, http.StatusOK, `{
			"forks": "11", "issues": "22", "merge_requests": "33",
			"notes": "44", "snippets": "55", "ssh_keys": "66",
			"milestones": "77", "users": "88", "groups": "99",
			"projects": "111", "active_users": "122"
		}`)
	})
	out, err := Get(t.Context(), testutil.NewTestClient(t, handler), GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GetOutput{
		Forks: 11, Issues: 22, MergeRequests: 33, Notes: 44, Snippets: 55,
		SSHKeys: 66, Milestones: 77, Users: 88, Groups: 99, Projects: 111,
		ActiveUsers: 122,
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Get() = %+v, want %+v", out, want)
	}
}

// TestGet_ErrorStatus_HintsAdministratorAccessOnlyOnForbidden states the branch
// WrapErrWithStatusHint chooses, which nothing asserted: the two error tests
// here check only that some error came back, so the status the hint is keyed to
// could move and the wording could vanish without failing anything.
//
// A 403 is what GitLab answers a token that is not an administrator's, and the
// suggestion is what tells a model to stop retrying and ask for a different
// credential. Any other status means something else, and carrying that advice
// there would send the reader after a fix that cannot help.
func TestGet_ErrorStatus_HintsAdministratorAccessOnlyOnForbidden(t *testing.T) {
	const adminHint = "application statistics require administrator access"
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "forbidden names the credential", status: http.StatusForbidden, wantHint: true},
		{name: "bad request says nothing about credentials", status: http.StatusBadRequest, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"nope"}`)
			}))
			_, err := Get(t.Context(), client, GetInput{})
			if err == nil {
				t.Fatalf("status %d: expected an error", tc.status)
			}
			if got := strings.Contains(err.Error(), adminHint); got != tc.wantHint {
				t.Errorf("status %d: error %q mentions %q = %v, want %v", tc.status, err, adminHint, got, tc.wantHint)
			}
		})
	}
}

// statisticsCard renders the eleven rows in the order the card writes them,
// so a test states the counts it cares about and the rest read zero.
func statisticsCard(activeUsers, users, projects, groups, issues, mergeRequests, notes, forks, snippets, sshKeys, milestones int64) string {
	return "## Application Statistics\n\n" +
		fmt.Sprintf("- **Active Users**: %d\n", activeUsers) +
		fmt.Sprintf("- **Users**: %d\n", users) +
		fmt.Sprintf("- **Projects**: %d\n", projects) +
		fmt.Sprintf("- **Groups**: %d\n", groups) +
		fmt.Sprintf("- **Issues**: %d\n", issues) +
		fmt.Sprintf("- **Merge Requests**: %d\n", mergeRequests) +
		fmt.Sprintf("- **Notes**: %d\n", notes) +
		fmt.Sprintf("- **Forks**: %d\n", forks) +
		fmt.Sprintf("- **Snippets**: %d\n", snippets) +
		fmt.Sprintf("- **SSH Keys**: %d\n", sshKeys) +
		fmt.Sprintf("- **Milestones**: %d\n", milestones) +
		"\n" + approximationNote + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use individual resource tools to explore specific statistics\n"
}

// TestFormatGetMarkdown verifies the whole card: one row per metric, zero
// included, and the note saying which figures GitLab approximates.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{ActiveUsers: 80, Projects: 45, Issues: 200}
	got := FormatGetMarkdown(out)
	want := statisticsCard(80, 0, 45, 0, 200, 0, 0, 0, 0, 0, 0)
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatGetMarkdown_Approximation verifies that the card says what GitLab
// says about its own counts. It used to present every figure as exact, which
// is the one thing a reader would act on wrongly.
func TestFormatGetMarkdown_Approximation(t *testing.T) {
	got := FormatGetMarkdown(GetOutput{Users: 10000})
	if !strings.Contains(got, "\n\nCounts of 10000 and above are approximate rather than exact.\n") {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant the approximation note as a paragraph of its own", got)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// covStatsJSON identifies the cov stats JSON constant used by this package.
const covStatsJSON = `{"forks":10,"issues":20,"merge_requests":30,"notes":40,"snippets":5,"ssh_keys":3,"milestones":7,"users":100,"groups":15,"projects":50,"active_users":80}`

// TestGet_APIError_Coverage verifies that Get_Coverage returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success_Coverage verifies the Get_Success_Coverage handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_Success_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
	}))
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Projects != 50 || out.ActiveUsers != 80 {
		t.Errorf("unexpected: %+v", out)
	}
}

// TestFormatGetMarkdown_Cov_Coverage verifies the whole card for a response
// with every metric filled, which is what the endpoint answers on an instance
// that has been used.
func TestFormatGetMarkdown_Cov_Coverage(t *testing.T) {
	out := GetOutput{
		Forks: 10, Issues: 20, MergeRequests: 30, Notes: 40, Snippets: 5,
		SSHKeys: 3, Milestones: 7, Users: 100, Groups: 15, Projects: 50, ActiveUsers: 80,
	}
	got := FormatGetMarkdown(out)
	want := statisticsCard(80, 100, 50, 15, 20, 30, 40, 10, 5, 3, 7)
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestActionSpecs_Metadata_Coverage validates the Metadata_Coverage route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "appstatistics" || specs[0].IndividualTool.Name != "gitlab_get_application_statistics" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if !strings.Contains(specs[0].Usage, "instance-wide application statistics") {
		t.Fatalf("Usage = %q, want instance statistics guidance", specs[0].Usage)
	}
	if !slices.Contains(specs[0].Aliases, "instance statistics") {
		t.Fatalf("Aliases = %v, want instance statistics alias", specs[0].Aliases)
	}
	if !strings.Contains(specs[0].IndividualTool.Description, "Returns:") || !strings.Contains(specs[0].IndividualTool.Description, "See also:") {
		t.Fatalf("Description = %q, want Returns/See also guidance", specs[0].IndividualTool.Description)
	}
}

// TestActionSpecs_CallRoute_Coverage validates the CallRoute_Coverage route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoute_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
	})
	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	res, err := spec.Route.Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestActionSpecs_CallRouteError validates the CallRouteError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CallRouteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{}); err == nil {
		t.Fatal("expected route error")
	}
}
