package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// testRules are a required and a pending rule, short enough to read in an
// expected report.
var testRules = struct{ required, pending rule }{
	required: rule{name: "X-Content-Type-Options", recommended: "nosniff"},
	pending:  rule{name: "X-Frame-Options", recommended: "DENY", pending: "the rule"},
}

// TestPageLines_Verdicts_SayWhatHoldsWhatFailsAndWhatWaits verifies the line
// each verdict and each unjudged page is reported with: a header that holds is
// shown with its value, a required one missing or permissive fails with the
// value to set, a pending one in the same state says what it waits for instead
// of failing, a pending one that is served asks to be declared required, and a
// page that could not be fetched or did not answer 2xx fails without verdicts.
func TestPageLines_Verdicts_SayWhatHoldsWhatFailsAndWhatWaits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		page page
		want []line
	}{
		{
			name: "a page that could not be fetched",
			page: page{url: "https://site.example/", err: errors.New("dial tcp: refused")},
			want: []line{{level: levelFail, title: "https://site.example/", text: "https://site.example/ could not be fetched: dial tcp: refused"}},
		},
		{
			name: "a page that answered 404",
			page: page{url: "https://site.example/", status: 404},
			want: []line{{level: levelFail, title: "https://site.example/", text: "https://site.example/ answered 404, so its headers were not judged"}},
		},
		{
			name: "every state of a verdict",
			page: page{url: "https://site.example/", status: 200, verdicts: []verdict{
				{rule: testRules.required, served: []string{"nosniff"}},
				{rule: testRules.required},
				{rule: testRules.required, served: []string{"sniff", "x"}, reason: "is wrong"},
				{rule: testRules.pending},
				{rule: testRules.pending, served: []string{"ALLOWALL"}, reason: "frames"},
				{rule: testRules.pending, served: []string{"DENY"}},
			}},
			want: []line{
				{level: levelOK, title: "X-Content-Type-Options", text: "X-Content-Type-Options: nosniff"},
				{level: levelFail, title: "X-Content-Type-Options", text: "X-Content-Type-Options is not sent. Set: nosniff"},
				{level: levelFail, title: "X-Content-Type-Options", text: "X-Content-Type-Options is permissive: it is wrong. Served: sniff, x. Set: nosniff"},
				{level: levelPending, title: "X-Frame-Options", text: "X-Frame-Options is not sent; it waits for the rule. Set: DENY"},
				{level: levelPending, title: "X-Frame-Options", text: "X-Frame-Options is permissive: it frames, and it waits for the rule. Served: ALLOWALL. Set: DENY"},
				{level: levelNotice, title: "X-Frame-Options", text: "X-Frame-Options: DENY. It is served, and declared as waiting for the rule: declare it required in cmd/audit_site_headers/headers.go and in the tests that pin the declarations, so that losing it fails this check"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := pageLines(tc.page); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("pageLines =\n%#v\nwant\n%#v", got, tc.want)
			}
		})
	}
}

// reportPages are two pages, one judged and one not, that the report tests
// below write.
var reportPages = []page{
	{url: "https://site.example/", status: 200, verdicts: []verdict{
		{rule: testRules.required, served: []string{"nosniff"}},
		{rule: testRules.pending},
	}},
	{url: "https://site.example/es/", err: errors.New("timeout")},
}

// TestWriteReport_Pages_PrintEachLineAndTheCount verifies the report on
// stdout: each judged page's status line, its verdicts indented under it with
// the level padded, a page not judged on a line of its own at the margin, the
// count of what held, failed and waits, and, with
// annotations asked for, one workflow command per line that is not ok, with
// its data and its title escaped as the runner reads them.
func TestWriteReport_Pages_PrintEachLineAndTheCount(t *testing.T) {
	t.Parallel()
	const report = "https://site.example/ answered 200\n" +
		"  ok       X-Content-Type-Options: nosniff\n" +
		"  pending  X-Frame-Options is not sent; it waits for the rule. Set: DENY\n" +
		"FAIL     https://site.example/es/ could not be fetched: timeout\n" +
		"audit_site_headers: 2 page(s), 1 header(s) held, 1 failed, 1 waiting for the maintainer's rule\n"
	var plain bytes.Buffer
	writeReport(&plain, reportPages, false)
	if plain.String() != report {
		t.Errorf("report =\n%s\nwant\n%s", plain.String(), report)
	}
	var annotated bytes.Buffer
	writeReport(&annotated, reportPages, true)
	want := report +
		"::warning title=X-Frame-Options::https://site.example/: X-Frame-Options is not sent; it waits for the rule. Set: DENY\n" +
		"::error title=https%3A//site.example/es/::https://site.example/es/ could not be fetched: timeout\n"
	if annotated.String() != want {
		t.Errorf("annotated report =\n%s\nwant\n%s", annotated.String(), want)
	}
}

// TestFailed_Pages_FailOnARequiredHeaderOrAnUnjudgedPage verifies the exit
// decision: a page not judged fails the run, as does a required header that
// does not hold, while a pending header never does.
func TestFailed_Pages_FailOnARequiredHeaderOrAnUnjudgedPage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		pages []page
		want  bool
	}{
		{name: "no page", want: false},
		{name: "pending only", pages: []page{{status: 200, verdicts: []verdict{{rule: testRules.pending}}}}, want: false},
		{name: "a required header that holds", pages: []page{{status: 204, verdicts: []verdict{{rule: testRules.required, served: []string{"nosniff"}}}}}, want: false},
		{name: "a required header missing", pages: []page{{status: 200, verdicts: []verdict{{rule: testRules.pending}, {rule: testRules.required}}}}, want: true},
		{name: "a page not fetched", pages: []page{{err: errors.New("x")}}, want: true},
		{name: "a page that answered 199", pages: []page{{status: 199}}, want: true},
		{name: "a page that answered 300", pages: []page{{status: 300}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := failed(tc.pages); got != tc.want {
				t.Errorf("failed = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestWriteSummary_Pages_WriteOneTableRowPerLine verifies the Markdown the
// job summary is given: a heading, a row per line with a pipe in any cell
// escaped so the table keeps its columns, and the count beneath.
func TestWriteSummary_Pages_WriteOneTableRowPerLine(t *testing.T) {
	t.Parallel()
	pages := append([]page{{url: "https://site.example/pipe", status: 200, verdicts: []verdict{
		{rule: testRules.required, served: []string{"a|b"}, reason: "is a|b"},
	}}}, reportPages...)
	var summary bytes.Buffer
	writeSummary(&summary, pages)
	want := "## Documentation site security headers\n\n" +
		"| Page | Header | State | Detail |\n" +
		"| --- | --- | --- | --- |\n" +
		"| https://site.example/pipe | X-Content-Type-Options | FAIL | X-Content-Type-Options is permissive: it is a\\|b. Served: a\\|b. Set: nosniff |\n" +
		"| https://site.example/ | X-Content-Type-Options | ok | X-Content-Type-Options: nosniff |\n" +
		"| https://site.example/ | X-Frame-Options | pending | X-Frame-Options is not sent; it waits for the rule. Set: DENY |\n" +
		"| https://site.example/es/ |  | FAIL | https://site.example/es/ could not be fetched: timeout |\n\n" +
		"3 page(s), 1 header(s) held, 2 failed, 1 waiting for the maintainer's rule\n"
	if summary.String() != want {
		t.Errorf("summary =\n%s\nwant\n%s", summary.String(), want)
	}
}

// TestEscape_WorkflowCommands_EncodeWhatTheRunnerWouldRead verifies the two
// escapes of a workflow command: its data keeps a colon and a comma and
// encodes the percent sign and line breaks, and a property encodes the colon
// and the comma as well, since the runner reads them as separators there.
func TestEscape_WorkflowCommands_EncodeWhatTheRunnerWouldRead(t *testing.T) {
	t.Parallel()
	const raw = "100% a:b, c\r\nd"
	if got, want := escapeData(raw), "100%25 a:b, c%0D%0Ad"; got != want {
		t.Errorf("escapeData(%q) = %q, want %q", raw, got, want)
	}
	if got, want := escapeProperty(raw), "100%25 a%3Ab%2C c%0D%0Ad"; got != want {
		t.Errorf("escapeProperty(%q) = %q, want %q", raw, got, want)
	}
	if strings.Contains(escapeData("plain"), "%") {
		t.Error("escapeData changed text that needs no escaping")
	}
}
