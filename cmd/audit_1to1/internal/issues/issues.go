// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// The repository the issues are asked of, and GitHub's REST API root.
const (
	DefaultRepository = "jmrplens/gitlab-mcp-server"
	DefaultAPI        = "https://api.github.com"
)

// SchemaVersion is the version of the report's shape.
const SchemaVersion = 1

// What GitHub's answer makes of an issue. Only StateOpen passes.
const (
	StateOpen = "open"
	// StateClosed is an issue GitHub answers closed, as done or as not
	// planned alike: either way the work a row promises is not pending.
	StateClosed = "closed"
	// StatePullRequest is a number GitHub answers with a pull request, which
	// tracks nothing once merged or closed.
	StatePullRequest = "pull-request"
	// StateMissing is a number the repository holds no issue for, a 404 or
	// a 410 for a deleted one.
	StateMissing = "missing"
	// StateUnjudged is an issue that could not be asked about: the network,
	// a refused or rate-limited token, an answer that is not an issue.
	StateUnjudged = "unjudged"
)

// categoryTracked is the category of a row that names its issue.
const categoryTracked = "tracked"

// maxAnswer is the most of an answer read: an issue is a few kilobytes,
// its body included.
const maxAnswer = 1 << 20

// shownRows is how many rows a failure names for one issue; the report
// holds them all.
const shownRows = 3

// Row is one row of a report that names an issue.
type Row struct {
	// Scope is the report it is read from, and Name what names it there: a
	// route of the routes scope, an input field of the sdk scope.
	Scope string `json:"scope"`
	Name  string `json:"name"`
	Issue int    `json:"issue"`
}

// String spells the row as a failure names it.
func (r Row) String() string {
	return r.Scope + " " + r.Name
}

// declaredRow is a row of either report as far as its issue goes.
type declaredRow struct {
	Category string `json:"category"`
	Issue    int    `json:"issue"`
}

// Rows reads the rows that name an issue out of a full routes report and a
// full sdk report. Each report counts its declared rows in its summary, and a
// report listing fewer, as -gaps-only does, is an error, since its tracked
// rows would go unasked; so is a tracked row that names no issue.
func Rows(routesReport, sdkReport []byte) ([]Row, error) {
	var routes struct {
		Summary struct {
			Declared int `json:"declared"`
		} `json:"summary"`
		Answered []struct {
			Route string `json:"route"`
			Class string `json:"class"`
			declaredRow
		} `json:"answered"`
	}
	if err := json.Unmarshal(routesReport, &routes); err != nil {
		return nil, fmt.Errorf("decode the routes report: %w", err)
	}
	var sdk struct {
		Summary struct {
			Declared int `json:"identifiers_declared"`
		} `json:"summary"`
		Identifiers struct {
			Arguments []struct {
				Input  string `json:"input"`
				Status string `json:"status"`
				declaredRow
			} `json:"arguments"`
		} `json:"identifiers"`
	}
	if err := json.Unmarshal(sdkReport, &sdk); err != nil {
		return nil, fmt.Errorf("decode the sdk report: %w", err)
	}
	var rows []Row
	declared := 0
	for _, route := range routes.Answered {
		if route.Class == "declared" {
			declared++
		}
		if route.names() {
			rows = append(rows, Row{Scope: "routes", Name: route.Route, Issue: route.Issue})
		}
	}
	if declared != routes.Summary.Declared {
		return nil, fmt.Errorf("the routes report lists %d of the %d routes its summary declares: read it without -gaps-only", declared, routes.Summary.Declared)
	}
	declared = 0
	for _, argument := range sdk.Identifiers.Arguments {
		if argument.Status == "declared" {
			declared++
		}
		if argument.names() {
			rows = append(rows, Row{Scope: "sdk", Name: argument.Input, Issue: argument.Issue})
		}
	}
	if declared != sdk.Summary.Declared {
		return nil, fmt.Errorf("the sdk report lists %d of the %d identifiers its summary declares: read it without -gaps-only", declared, sdk.Summary.Declared)
	}
	for _, row := range rows {
		if row.Issue <= 0 {
			return nil, fmt.Errorf("%s is tracked and names no issue, so it cannot be judged", row)
		}
	}
	return rows, nil
}

// names reports whether the row rests on an issue: it names one, or it is
// tracked, which names one or is an error.
func (r declaredRow) names() bool {
	return r.Issue > 0 || r.Category == categoryTracked
}

// Client asks GitHub about an issue.
type Client struct {
	// API is GitHub's REST API root, with no trailing slash, and Repository
	// the owner and name the issues are numbered in.
	API        string
	Repository string
	// Token authenticates the requests when it is not empty. Without one
	// GitHub answers sixty requests an hour per address, which a run of this
	// check can exhaust.
	Token string
	HTTP  *http.Client
}

// ask returns what GitHub's answer about issue number makes of it, and, for
// any state but open and closed, why.
func (c Client) ask(ctx context.Context, number int) (state, detail string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/issues/%d", c.API, c.Repository, number), http.NoBody)
	if err != nil {
		return StateUnjudged, err.Error()
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "gitlab-mcp-server-audit_1to1")
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, err := c.HTTP.Do(request) //#nosec G704 -- the GitHub API root this command is configured with, never a URL a report names
	if err != nil {
		return StateUnjudged, err.Error()
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	if err != nil {
		return StateUnjudged, "read GitHub's answer: " + err.Error()
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return StateMissing, "GitHub answered " + response.Status
	}
	if response.StatusCode != http.StatusOK {
		return StateUnjudged, "GitHub answered " + response.Status + ": " + strings.TrimSpace(string(body))
	}
	var issue struct {
		State       string          `json:"state"`
		PullRequest json.RawMessage `json:"pull_request"`
	}
	if err = json.Unmarshal(body, &issue); err != nil {
		return StateUnjudged, "decode GitHub's answer: " + err.Error()
	}
	if issue.PullRequest != nil {
		return StatePullRequest, "the number is a pull request's"
	}
	if issue.State != StateOpen && issue.State != StateClosed {
		return StateUnjudged, fmt.Sprintf("GitHub answered state %q", issue.State)
	}
	return issue.State, ""
}

// Report is what -check-issues reports.
type Report struct {
	SchemaVersion int     `json:"schema_version"`
	Repository    string  `json:"repository"`
	Summary       Summary `json:"summary"`
	// Issues are every issue a row names, in number order.
	Issues []Issue `json:"issues"`
}

// Summary counts the rows read and the issues they name, by state.
type Summary struct {
	Rows         int `json:"rows"`
	Issues       int `json:"issues"`
	Open         int `json:"open"`
	Closed       int `json:"closed"`
	PullRequests int `json:"pull_requests"`
	Missing      int `json:"missing"`
	Unjudged     int `json:"unjudged"`
}

// Issue is one issue a row names, with what GitHub made of it.
type Issue struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	// Rows counts the rows that name it, and NamedBy lists them for an issue
	// that fails, which is where a reader starts.
	Rows    int      `json:"rows"`
	NamedBy []string `json:"named_by,omitempty"`
}

// Check asks GitHub about every issue rows name, once each, in number order.
func Check(ctx context.Context, client Client, rows []Row) Report {
	byIssue := map[int][]string{}
	for _, row := range rows {
		byIssue[row.Issue] = append(byIssue[row.Issue], row.String())
	}
	numbers := make([]int, 0, len(byIssue))
	for number := range byIssue {
		numbers = append(numbers, number)
	}
	slices.Sort(numbers)
	report := Report{SchemaVersion: SchemaVersion, Repository: client.Repository, Issues: []Issue{}}
	report.Summary.Rows = len(rows)
	report.Summary.Issues = len(numbers)
	for _, number := range numbers {
		state, detail := client.ask(ctx, number)
		issue := Issue{Number: number, State: state, Detail: detail, Rows: len(byIssue[number])}
		if state != StateOpen {
			issue.NamedBy = byIssue[number]
		}
		*report.Summary.counter(state)++
		report.Issues = append(report.Issues, issue)
	}
	return report
}

// counter is the summary's counter of state; the closing case counts what
// ask could not judge.
func (s *Summary) counter(state string) *int {
	switch state {
	case StateOpen:
		return &s.Open
	case StateClosed:
		return &s.Closed
	case StatePullRequest:
		return &s.PullRequests
	case StateMissing:
		return &s.Missing
	default:
		return &s.Unjudged
	}
}

// Clean reports whether every issue a row names is open.
func (r Report) Clean() bool {
	return r.Summary.Open == r.Summary.Issues
}

// Marshal returns the report as indented JSON with a trailing newline.
func (r Report) Marshal() []byte {
	return append(cmdutil.Must(json.MarshalIndent(r, "", "  ")), '\n')
}

// Failure is the line a failing check ends on: each issue that is not open,
// what GitHub made of it, and the first rows that rest on it.
func (r Report) Failure() string {
	var failing []string
	for _, issue := range r.Issues {
		if issue.State == StateOpen {
			continue
		}
		text := fmt.Sprintf("#%d is %s", issue.Number, issue.State)
		if issue.Detail != "" {
			text += " (" + issue.Detail + ")"
		}
		shown := issue.NamedBy[:min(len(issue.NamedBy), shownRows)]
		text += fmt.Sprintf(", named by %d rows: %s", issue.Rows, strings.Join(shown, "; "))
		if more := issue.Rows - len(shown); more > 0 {
			text += fmt.Sprintf("; and %d more", more)
		}
		failing = append(failing, text)
	}
	return "audit_1to1: tracked rows rest on issues that are not open or could not be asked about: " + strings.Join(failing, " | ") +
		"; a closed issue's rows are answered again or retired, and a failure to ask is run again (see report)"
}
