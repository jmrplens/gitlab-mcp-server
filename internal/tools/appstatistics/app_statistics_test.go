// app_statistics_test.go contains unit tests for the application statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package appstatistics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
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
// The first is why it exists at all: GitLab answers this endpoint with every
// count encoded as a JSON string, since it renders each through Rails'
// number_with_delimiter, and the SDK types every field of
// ApplicationStatistics as int64, so the SDK call fails outright and
// rawStatistics is the only reason the tool answers anything
// (docs/development/upstream-bugs.md, entry 7). A fixture sending JSON
// numbers, which the SDK would have decoded too, could not stop the
// workaround being collapsed back to int64 with the suite still green.
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

// TestGet_DelimitedCounts_ReadInEveryFormGitLabSends holds the handler to the
// answers an instance with a thousand or more of anything actually sends.
//
// number_with_delimiter groups the digits the way the caller's
// preferred_language groups them, and among the languages GitLab offers that
// is one of three forms: a comma (English, Japanese, Korean, and every language
// GitLab spells with a region, such as pt_BR, which rails-i18n spells pt-BR and
// so falls back to English), a period (German, Spanish, Italian) or an ASCII
// space (French, Russian, Ukrainian, Bulgarian, Esperanto). The handler used to
// decode each count as a json.Number, which refuses all three, so the whole
// call failed at the first count past 999. The last case is the answer as
// JSON numbers, which the handler reads as well.
//
// Every count carries a value of its own in every form, and the values hold a
// 0 and a 9 between separators, so a digit test that stopped admitting either
// end of its range fails here.
func TestGet_DelimitedCounts_ReadInEveryFormGitLabSends(t *testing.T) {
	want := GetOutput{
		Forks: 1001, Issues: 12345, MergeRequests: 2009, Notes: 1234567,
		Snippets: 3000, SSHKeys: 4096, Milestones: 5555, Users: 10000,
		Groups: 1100, Projects: 999999, ActiveUsers: 9870,
	}
	cases := []struct {
		name string
		body string
	}{
		{name: "comma", body: `{
			"forks": "1,001", "issues": "12,345", "merge_requests": "2,009",
			"notes": "1,234,567", "snippets": "3,000", "ssh_keys": "4,096",
			"milestones": "5,555", "users": "10,000", "groups": "1,100",
			"projects": "999,999", "active_users": "9,870"
		}`},
		{name: "period", body: `{
			"forks": "1.001", "issues": "12.345", "merge_requests": "2.009",
			"notes": "1.234.567", "snippets": "3.000", "ssh_keys": "4.096",
			"milestones": "5.555", "users": "10.000", "groups": "1.100",
			"projects": "999.999", "active_users": "9.870"
		}`},
		{name: "space", body: `{
			"forks": "1 001", "issues": "12 345", "merge_requests": "2 009",
			"notes": "1 234 567", "snippets": "3 000", "ssh_keys": "4 096",
			"milestones": "5 555", "users": "10 000", "groups": "1 100",
			"projects": "999 999", "active_users": "9 870"
		}`},
		{name: "JSON numbers", body: `{
			"forks": 1001, "issues": 12345, "merge_requests": 2009,
			"notes": 1234567, "snippets": 3000, "ssh_keys": 4096,
			"milestones": 5555, "users": 10000, "groups": 1100,
			"projects": 999999, "active_users": 9870
		}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))
			out, err := Get(t.Context(), client, GetInput{})
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if !reflect.DeepEqual(out, want) {
				t.Errorf("Get() = %+v, want %+v", out, want)
			}
		})
	}
}

// TestGet_UnreadableCount_RefusedRatherThanReadAsZero pins the other half of
// the defect: the handler used to discard the error of json.Number.Int64, so a
// count that decoded and did not fit an int64 was published as 0, which reads
// as a fact about the instance. A count the handler cannot read now fails the
// call, and the error quotes what GitLab sent so a reader can see why.
func TestGet_UnreadableCount_RefusedRatherThanReadAsZero(t *testing.T) {
	cases := []struct {
		name  string
		count string
		quote string
	}{
		{name: "past int64 as a string", count: `"9,223,372,036,854,775,808"`, quote: `invalid count "9,223,372,036,854,775,808"`},
		{name: "past int64 as a number", count: `9223372036854775808`, quote: "9223372036854775808"},
		{name: "not a number", count: `"many"`, quote: `invalid count "many"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"issues": `+tc.count+`, "users": "1,234"}`)
			}))
			out, err := Get(t.Context(), client, GetInput{})
			if err == nil {
				t.Fatalf("Get() = %+v, want an error for issues %s", out, tc.count)
			}
			if !strings.Contains(err.Error(), tc.quote) {
				t.Errorf("Get() error = %q, want it to quote %q", err, tc.quote)
			}
		})
	}
}

// TestDelimitedCount_UnmarshalJSON states, one form at a time, what a count of
// the statistics answer is read as, and which forms are refused.
//
// The accepted half is what GitLab sends (the three separators, a count below a
// thousand with none, a negative fork count, which is fork network members less
// fork networks, both approximated, and a JSON number), plus a no-break space,
// the form a later rails-i18n could give a language that uses a space today.
// Two rows are there for parity with client-go's own decoding, which this one
// mirrors so that moving the handler onto the SDK changes nothing: a minus sign
// anywhere but first is punctuation and so a separator, and a null reads as
// zero. GitLab sends neither, since Rails writes the sign only in front and the
// count helper raises rather than rendering a missing count. The refused half
// is everything that would otherwise have been published as a number GitLab
// did not send.
func TestDelimitedCount_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name    string
		count   string
		want    int64
		wantErr bool
	}{
		{name: "JSON number", count: `1234`, want: 1234},
		{name: "zero", count: `"0"`, want: 0},
		{name: "below a thousand, no separator", count: `"999"`, want: 999},
		{name: "comma", count: `"1,234"`, want: 1234},
		{name: "period", count: `"1.234"`, want: 1234},
		{name: "space", count: `"1 234"`, want: 1234},
		{name: "no-break space", count: "\"1 234\"", want: 1234},
		{name: "millions", count: `"1,234,567"`, want: 1234567},
		{name: "negative", count: `"-1,234"`, want: -1234},
		{name: "hyphen past the first place is punctuation like any other", count: `"1-234"`, want: 1234},
		{name: "null", count: `null`, want: 0},
		{name: "letters", count: `"many"`, wantErr: true},
		{name: "letter after digits", count: `"12a"`, wantErr: true},
		{name: "empty string", count: `""`, wantErr: true},
		{name: "a sign alone", count: `"-"`, wantErr: true},
		{name: "past int64 as a string", count: `"9,223,372,036,854,775,808"`, wantErr: true},
		{name: "past int64 as a number", count: `9223372036854775808`, wantErr: true},
		{name: "fraction", count: `1.5`, wantErr: true},
		{name: "boolean", count: `true`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw rawStatistics
			err := json.Unmarshal([]byte(`{"issues": `+tc.count+`}`), &raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("json.Unmarshal(%s) read %d, want an error", tc.count, raw.Issues)
				}
				return
			}
			if err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", tc.count, err)
			}
			if int64(raw.Issues) != tc.want {
				t.Errorf("json.Unmarshal(%s) = %d, want %d", tc.count, raw.Issues, tc.want)
			}
		})
	}
}

// TestDelimitedCount_UnmarshalJSON_MalformedString_Refused covers the one input
// encoding/json never hands the method, because it validates the document
// first: bytes that open a string and do not close it. The method is exported
// to the decoder and nothing else stops a direct caller, so it refuses them
// rather than reading them as an empty count.
func TestDelimitedCount_UnmarshalJSON_MalformedString_Refused(t *testing.T) {
	c := delimitedCount(7)
	if err := c.UnmarshalJSON([]byte(`"1,234`)); err == nil {
		t.Fatalf("UnmarshalJSON of an unterminated string = %d, want an error", c)
	}
	if c != 7 {
		t.Errorf("UnmarshalJSON of an unterminated string left %d, want the count untouched at 7", c)
	}
}

// TestFetchStatistics_NewRequestError covers the branch the constant route of
// Get never reaches: a path holding an invalid percent-escape ("%zz") makes
// NewRequest fail before anything is sent, and the helper returns that error
// with the operation named instead of sending a request it could not build.
// The mock refuses any request that does arrive.
func TestFetchStatistics_NewRequestError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := fetchStatistics(t.Context(), client, "application/%zz")
	if err == nil {
		t.Fatal("fetchStatistics() error = nil, want the request construction error")
	}
	if !strings.HasPrefix(err.Error(), "get_application_statistics: ") {
		t.Errorf("fetchStatistics() error = %q, want it to name get_application_statistics", err)
	}
}

// TestGet_ErrorStatus_HintsAdministratorAccessOnlyOnAPermissionRefusal states
// the branch the handler chooses, which nothing asserted: the two error tests
// here check only that some error came back, so the refusal the hint is keyed
// to could move and the wording could vanish without failing anything.
//
// A plain 403 is what authenticated_as_admin! answers a token that is not an
// administrator's, and the suggestion is what tells a model to stop retrying
// and ask for a different credential. The hint names a role, so it is keyed on
// toolutil.IsPermissionRefusal rather than on the status: a plain 401 is one
// too, while a 403 naming an RFC 6750 error code refuses the token's scope,
// and any other status means something else. Carrying the advice there would
// send the reader after a fix that cannot help.
func TestGet_ErrorStatus_HintsAdministratorAccessOnlyOnAPermissionRefusal(t *testing.T) {
	const adminHint = "application statistics require administrator access"
	cases := []struct {
		name     string
		status   int
		body     string
		wantHint bool
	}{
		{name: "forbidden names the credential", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`, wantHint: true},
		{name: "a plain unauthorized is a permission refusal too", status: http.StatusUnauthorized, body: `{"message":"401 Unauthorized"}`, wantHint: true},
		{name: "a scope refusal says nothing about the role", status: http.StatusForbidden, body: `{"error":"insufficient_scope"}`, wantHint: false},
		{name: "bad request says nothing about credentials", status: http.StatusBadRequest, body: `{"message":"nope"}`, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, tc.body)
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

// statisticsCard renders the eleven rows of counts in the order the card writes
// them, so a test states the counts it cares about and the rest read zero. It
// takes the output the card is rendered from, with each row spelled out here,
// so a card that put one count under another's label still fails.
func statisticsCard(counts GetOutput) string {
	return "## Application Statistics\n\n" +
		fmt.Sprintf("- **Active Users**: %d\n", counts.ActiveUsers) +
		fmt.Sprintf("- **Users**: %d\n", counts.Users) +
		fmt.Sprintf("- **Projects**: %d\n", counts.Projects) +
		fmt.Sprintf("- **Groups**: %d\n", counts.Groups) +
		fmt.Sprintf("- **Issues**: %d\n", counts.Issues) +
		fmt.Sprintf("- **Merge Requests**: %d\n", counts.MergeRequests) +
		fmt.Sprintf("- **Notes**: %d\n", counts.Notes) +
		fmt.Sprintf("- **Forks**: %d\n", counts.Forks) +
		fmt.Sprintf("- **Snippets**: %d\n", counts.Snippets) +
		fmt.Sprintf("- **SSH Keys**: %d\n", counts.SSHKeys) +
		fmt.Sprintf("- **Milestones**: %d\n", counts.Milestones) +
		"\n" + approximationNote + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use individual resource tools to explore specific statistics\n"
}

// TestFormatGetMarkdown verifies the whole card: one row per metric, zero
// included, and the note saying which figures GitLab approximates.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{ActiveUsers: 80, Projects: 45, Issues: 200}
	got := FormatGetMarkdown(out)
	want := statisticsCard(out)
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
	want := statisticsCard(out)
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}
