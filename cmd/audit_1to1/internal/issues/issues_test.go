// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// routesReport is a full routes report: a tracked family row, a
// client-protocol one, a skip row and a reached route, the summary counting
// the two declared.
const routesReport = `{
  "summary": {"declared": 2},
  "findings": [],
  "answered": [
    {"route": "DELETE /admin/queue", "class": "declared", "category": "tracked", "issue": 1614, "family": "* /admin/**"},
    {"route": "GET /-/cable", "class": "declared", "category": "client-protocol"},
    {"route": "POST /internal/allowed", "class": "skip", "category": "other-credential"}
  ],
  "reached": ["GET /projects"]
}`

// sdkReport is a full sdk report: a tracked identifier and two GitLab takes
// as an ID alone, the summary counting the one declared. The undeclared rows
// outnumber the declared one, so a count of the rows that are not declared
// does not agree with the summary by chance.
const sdkReport = `{
  "summary": {"identifiers_declared": 1},
  "identifiers": {"arguments": [
    {"input": "users.GetStatusInput.UserID", "status": "declared", "category": "tracked", "issue": 1602},
    {"input": "runners.GetInput.RunnerID", "status": "id_only"},
    {"input": "containerregistry.GetRepositoryInput.RepositoryID", "status": "id_only"}
  ]}
}`

// TestRows_TheRowsThatNameAnIssue_AreReadFromBothReports reads the two
// reports and holds Rows to the rows that rest on an issue, each with the
// report it came from: the tracked route and the tracked identifier, and
// nothing a row with no issue answers.
func TestRows_TheRowsThatNameAnIssue_AreReadFromBothReports(t *testing.T) {
	rows, err := Rows([]byte(routesReport), []byte(sdkReport))
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	want := []Row{
		{Scope: "routes", Name: "DELETE /admin/queue", Issue: 1614},
		{Scope: "sdk", Name: "users.GetStatusInput.UserID", Issue: 1602},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("Rows = %+v, want %+v", rows, want)
	}
	if got := rows[0].String(); got != "routes DELETE /admin/queue" {
		t.Errorf("Row.String() = %q, want the scope and the name", got)
	}
}

// TestRows_ARowOfAnotherCategoryNamingAnIssue_IsAskedAbout holds Rows to
// reading the issue a row names whatever its category, so a category added
// later that names an issue is asked about too, on both reports.
func TestRows_ARowOfAnotherCategoryNamingAnIssue_IsAskedAbout(t *testing.T) {
	routes := strings.Replace(routesReport, `"category": "client-protocol"`, `"category": "client-protocol", "issue": 1700`, 1)
	sdk := strings.Replace(sdkReport, `"status": "id_only"`, `"status": "id_only", "issue": 1701`, 1)
	rows, err := Rows([]byte(routes), []byte(sdk))
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 4 || rows[1].Issue != 1700 || rows[3].Issue != 1701 {
		t.Errorf("Rows = %+v, want the two rows of another category among them", rows)
	}
}

// TestRows_AReportItCannotJudge_IsAnError holds Rows to refusing what would
// leave a tracked row unasked: a report that is not JSON, a report run with
// -gaps-only, which lists none of the declared rows its summary counts, and a
// tracked row that names no issue.
func TestRows_AReportItCannotJudge_IsAnError(t *testing.T) {
	cases := []struct {
		name          string
		routes, sdk   string
		wantErrSubstr string
	}{
		{name: "routes_not_json", routes: "{", sdk: sdkReport, wantErrSubstr: "decode the routes report"},
		{name: "sdk_not_json", routes: routesReport, sdk: "{", wantErrSubstr: "decode the sdk report"},
		{
			name:          "routes_gaps_only",
			routes:        `{"summary": {"declared": 2}, "findings": []}`,
			sdk:           sdkReport,
			wantErrSubstr: "the routes report lists 0 of the 2 routes its summary declares: read it without -gaps-only",
		},
		{
			name:          "sdk_gaps_only",
			routes:        routesReport,
			sdk:           `{"summary": {"identifiers_declared": 1}, "identifiers": {"arguments": []}}`,
			wantErrSubstr: "the sdk report lists 0 of the 1 identifiers its summary declares: read it without -gaps-only",
		},
		{
			name:          "routes_tracked_without_issue",
			routes:        strings.Replace(routesReport, `"issue": 1614, `, "", 1),
			sdk:           sdkReport,
			wantErrSubstr: "routes DELETE /admin/queue is tracked and names no issue, so it cannot be judged",
		},
		{
			name:          "sdk_tracked_without_issue",
			routes:        routesReport,
			sdk:           strings.Replace(sdkReport, `, "issue": 1602`, "", 1),
			wantErrSubstr: "sdk users.GetStatusInput.UserID is tracked and names no issue",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := Rows([]byte(tc.routes), []byte(tc.sdk))
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Fatalf("Rows error = %v, want it to contain %q", err, tc.wantErrSubstr)
			}
			if rows != nil {
				t.Errorf("Rows returned %v beside its error", rows)
			}
		})
	}
}

// standIn is a stand-in GitHub: it answers each issue number with the
// status and body its table holds, and records the requests it was sent.
type standIn struct {
	answers map[int]answer
	mu      sync.Mutex
	asked   []int
	headers []http.Header
}

// requests returns the issue numbers asked and the headers each request
// carried, read under the lock the handler writes them under.
func (s *standIn) requests() ([]int, []http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.asked...), append([]http.Header(nil), s.headers...)
}

// answer is what the stand-in answers one issue with.
type answer struct {
	status int
	body   string
	// short claims a longer body than it sends, so reading it fails.
	short bool
}

// serve starts the stand-in and returns a client of it.
func (s *standIn) serve(t *testing.T, token string) Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/repos/owner/name/issues/"))
		s.mu.Lock()
		s.asked = append(s.asked, number)
		s.headers = append(s.headers, r.Header.Clone())
		s.mu.Unlock()
		reply, ok := s.answers[number]
		if err != nil || !ok {
			http.Error(w, "unexpected request "+r.URL.Path, http.StatusTeapot)
			return
		}
		if reply.short {
			w.Header().Set("Content-Length", strconv.Itoa(len(reply.body)+10))
		}
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
	t.Cleanup(server.Close)
	return Client{API: server.URL, Repository: "owner/name", Token: token, HTTP: server.Client()}
}

// rowsOf returns n rows naming issue, as the routes report would.
func rowsOf(issue, n int) []Row {
	rows := make([]Row, 0, n)
	for i := range n {
		rows = append(rows, Row{Scope: "routes", Name: fmt.Sprintf("GET /r%d", i), Issue: issue})
	}
	return rows
}

// TestCheck_EachAnswer_IsTheStateItMeans asks a stand-in GitHub about an issue
// answered each way GitHub can answer, and holds Check to what each means:
// open passes; closed, a pull request, a 404 and a 410 fail as what they are;
// and a refused token, a server error, an answer that is not an issue, a
// state GitHub does not use and a body cut short fail as unable to judge,
// never as a pass. Only a failing issue lists the rows that rest on it.
func TestCheck_EachAnswer_IsTheStateItMeans(t *testing.T) {
	github := &standIn{answers: map[int]answer{
		1:  {status: http.StatusOK, body: `{"number": 1, "state": "open"}`},
		2:  {status: http.StatusOK, body: `{"number": 2, "state": "closed", "state_reason": "not_planned"}`},
		3:  {status: http.StatusOK, body: `{"number": 3, "state": "closed", "pull_request": {"url": "u"}}`},
		4:  {status: http.StatusNotFound, body: `{"message": "Not Found"}`},
		5:  {status: http.StatusGone, body: `{"message": "This issue was deleted"}`},
		6:  {status: http.StatusForbidden, body: `{"message": "API rate limit exceeded"}`},
		7:  {status: http.StatusInternalServerError, body: "boom"},
		8:  {status: http.StatusOK, body: "not json"},
		9:  {status: http.StatusOK, body: `{"number": 9, "state": "locked"}`},
		10: {status: http.StatusOK, body: `{"number": 10, "state": "open"}`, short: true},
	}}
	var rows []Row
	for number := 1; number <= 10; number++ {
		rows = append(rows, rowsOf(number, 2)...)
	}
	report := Check(context.Background(), github.serve(t, ""), rows)

	want := map[int]struct {
		state, detail string
	}{
		1:  {state: StateOpen},
		2:  {state: StateClosed},
		3:  {state: StatePullRequest, detail: "the number is a pull request's"},
		4:  {state: StateMissing, detail: "GitHub answered 404 Not Found"},
		5:  {state: StateMissing, detail: "GitHub answered 410 Gone"},
		6:  {state: StateUnjudged, detail: `GitHub answered 403 Forbidden: {"message": "API rate limit exceeded"}`},
		7:  {state: StateUnjudged, detail: "GitHub answered 500 Internal Server Error: boom"},
		8:  {state: StateUnjudged, detail: "decode GitHub's answer: invalid character"},
		9:  {state: StateUnjudged, detail: `GitHub answered state "locked"`},
		10: {state: StateUnjudged, detail: "read GitHub's answer: unexpected EOF"},
	}
	if len(report.Issues) != len(want) {
		t.Fatalf("Check reported %d issues, want %d: %+v", len(report.Issues), len(want), report.Issues)
	}
	for i, issue := range report.Issues {
		t.Run(strconv.Itoa(issue.Number), func(t *testing.T) {
			expected := want[issue.Number]
			if issue.Number != i+1 {
				t.Errorf("issue %d is reported at %d, want number order", issue.Number, i)
			}
			if issue.State != expected.state || !strings.HasPrefix(issue.Detail, expected.detail) || (expected.detail == "") != (issue.Detail == "") {
				t.Errorf("issue %d = %q (%q), want %q (%q)", issue.Number, issue.State, issue.Detail, expected.state, expected.detail)
			}
			if issue.Rows != 2 {
				t.Errorf("issue %d counts %d rows, want 2", issue.Number, issue.Rows)
			}
			wantNamed := []string(nil)
			if expected.state != StateOpen {
				wantNamed = []string{"routes GET /r0", "routes GET /r1"}
			}
			if !reflect.DeepEqual(issue.NamedBy, wantNamed) {
				t.Errorf("issue %d is named by %q, want %q", issue.Number, issue.NamedBy, wantNamed)
			}
		})
	}
	wantSummary := Summary{Rows: 20, Issues: 10, Open: 1, Closed: 1, PullRequests: 1, Missing: 2, Unjudged: 5}
	if report.Summary != wantSummary {
		t.Errorf("Summary = %+v, want %+v", report.Summary, wantSummary)
	}
	if report.Clean() {
		t.Error("Clean() = true for a report with closed and unjudged issues")
	}
	if report.SchemaVersion != SchemaVersion || report.Repository != "owner/name" {
		t.Errorf("the report names version %d and %q, want %d and owner/name", report.SchemaVersion, report.Repository, SchemaVersion)
	}
}

// TestCheck_EveryIssueOpen_IsCleanAndAsksEachOnce holds Check to asking about
// an issue once however many rows name it, to passing when every issue is
// open, and to sending the headers GitHub's API documents, the token only
// when there is one.
func TestCheck_EveryIssueOpen_IsCleanAndAsksEachOnce(t *testing.T) {
	for name, token := range map[string]string{"with_token": "secret", "without_token": ""} {
		t.Run(name, func(t *testing.T) {
			github := &standIn{answers: map[int]answer{
				1604: {status: http.StatusOK, body: `{"state": "open"}`},
				1209: {status: http.StatusOK, body: `{"state": "open"}`},
			}}
			rows := append(rowsOf(1604, 3), rowsOf(1209, 1)...)
			report := Check(context.Background(), github.serve(t, token), rows)
			if !report.Clean() {
				t.Errorf("Clean() = false for %+v", report)
			}
			asked, headers := github.requests()
			if !reflect.DeepEqual(asked, []int{1209, 1604}) {
				t.Errorf("asked %v, want each issue once, in number order", asked)
			}
			assertGitHubHeaders(t, headers, token)
			for _, issue := range report.Issues {
				if issue.NamedBy != nil || issue.Detail != "" {
					t.Errorf("open issue %d carries rows %v or detail %q", issue.Number, issue.NamedBy, issue.Detail)
				}
			}
		})
	}
}

// assertGitHubHeaders holds every request to the headers GitHub's API
// documents, and to the token as a bearer credential when there is one and
// no Authorization header when there is not.
func assertGitHubHeaders(t *testing.T, headers []http.Header, token string) {
	t.Helper()
	wantAuthorization := ""
	if token != "" {
		wantAuthorization = "Bearer " + token
	}
	for _, header := range headers {
		if got := header.Get("Authorization"); got != wantAuthorization {
			t.Errorf("Authorization = %q, want %q", got, wantAuthorization)
		}
		if header.Get("Accept") != "application/vnd.github+json" || header.Get("X-GitHub-Api-Version") != "2022-11-28" || header.Get("User-Agent") == "" {
			t.Errorf("headers = %v, want GitHub's media type, API version and a user agent", header)
		}
	}
}

// TestCheck_NoRows_AsksNothingAndIsClean holds an empty row list to a clean
// report with no issue in it, written as an empty list.
func TestCheck_NoRows_AsksNothingAndIsClean(t *testing.T) {
	github := &standIn{}
	report := Check(context.Background(), github.serve(t, ""), nil)
	if asked, _ := github.requests(); !report.Clean() || len(asked) != 0 {
		t.Errorf("Check of no rows = %+v after asking %v, want a clean report and no request", report, asked)
	}
	if !strings.Contains(string(report.Marshal()), `"issues": []`) {
		t.Errorf("the report writes no issue as something else than []:\n%s", report.Marshal())
	}
}

// TestCheck_NoAnswer_IsUnableToJudge holds the two failures that happen before
// GitHub answers to unable to judge: an address nothing answers on, and an
// API root that is not a URL.
func TestCheck_NoAnswer_IsUnableToJudge(t *testing.T) {
	cases := map[string]Client{
		"nothing_answers": {API: netfixture.DeadEndpoint(t), Repository: "owner/name", HTTP: &http.Client{}},
		"not_a_url":       {API: "http://bad host", Repository: "owner/name", HTTP: &http.Client{}},
	}
	for name, client := range cases {
		t.Run(name, func(t *testing.T) {
			report := Check(context.Background(), client, rowsOf(1602, 1))
			if report.Summary.Unjudged != 1 || report.Issues[0].State != StateUnjudged || report.Issues[0].Detail == "" || report.Clean() {
				t.Errorf("Check = %+v, want the issue unjudged with the reason", report)
			}
		})
	}
}

// TestReport_Failure_NamesEachIssueThatIsNotOpenWithItsRows holds the line a
// failing check ends on to naming each issue that is not open, what GitHub
// made of it, and the first three rows that rest on it with how many more,
// and to leaving the open ones out.
func TestReport_Failure_NamesEachIssueThatIsNotOpenWithItsRows(t *testing.T) {
	report := Report{Issues: []Issue{
		{Number: 1604, State: StateOpen, Rows: 9},
		{Number: 1609, State: StateClosed, Rows: 4, NamedBy: []string{"routes A", "routes B", "routes C", "routes D"}},
		{Number: 1610, State: StateUnjudged, Detail: "GitHub answered 403 Forbidden", Rows: 3, NamedBy: []string{"routes E", "routes F", "routes G"}},
		{Number: 1611, State: StateMissing, Detail: "GitHub answered 404 Not Found", Rows: 1, NamedBy: []string{"sdk H"}},
	}}
	got := report.Failure()
	cases := []struct {
		name string
		part string
		// absent says the line must not carry part.
		absent bool
	}{
		{name: "the_opening", part: "audit_1to1: tracked rows rest on issues that are not open or could not be asked about: "},
		{name: "a_closed_issue_with_its_first_three_rows", part: "#1609 is closed, named by 4 rows: routes A; routes B; routes C; and 1 more"},
		{name: "an_unjudged_issue_with_its_reason", part: " | #1610 is unjudged (GitHub answered 403 Forbidden), named by 3 rows: routes E; routes F; routes G | "},
		{name: "a_missing_issue_and_the_advice", part: "#1611 is missing (GitHub answered 404 Not Found), named by 1 rows: sdk H; a closed issue's rows"},
		{name: "no_open_issue", part: "#1604", absent: true},
		{name: "no_more_after_every_row", part: "routes G; and", absent: true},
		{name: "no_row_past_the_third", part: "routes D", absent: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(got, tc.part) == tc.absent {
				t.Errorf("Failure() = %q, want it to contain %q: %t", got, tc.part, !tc.absent)
			}
		})
	}
}

// TestReport_Marshal_IsTheReportAsJSON holds the report's encoding to its
// fields and a trailing newline.
func TestReport_Marshal_IsTheReportAsJSON(t *testing.T) {
	report := Report{SchemaVersion: SchemaVersion, Repository: "owner/name", Issues: []Issue{{Number: 7, State: StateOpen, Rows: 1}}}
	content := report.Marshal()
	var decoded Report
	if err := json.Unmarshal(content, &decoded); err != nil || !reflect.DeepEqual(decoded, report) || !strings.HasSuffix(string(content), "}\n") {
		t.Errorf("Marshal = %s (%v), want the report back and a trailing newline", content, err)
	}
}
