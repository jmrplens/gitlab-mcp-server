// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	yaml "go.yaml.in/yaml/v3"
)

// errFetch is the transport error a scripted fetch fails with.
var errFetch = errors.New("refused")

// siteResponse is the header a fully hardened site sends, built from the
// declarations so the expected report below names each value once.
func siteResponse() http.Header {
	header := http.Header{}
	for _, each := range siteHeaders {
		header.Set(each.name, each.recommended)
	}
	return header
}

// runCase is one command line of the table below, the answers its fetches
// get, and what the run must do with them.
type runCase struct {
	name       string
	args       []string
	responses  []response
	wantCode   int
	wantAsked  []string
	wantWaits  int
	wantDelay  time.Duration
	wantStdout []string
	wantStderr string
}

// check runs the case and reports every way the run departed from it: the
// exit code, the start of stderr (empty when none is wanted), each fragment
// stdout must hold (nothing at all when none is listed), the pages fetched in
// order, and the waits with their delay.
func (tc runCase) check(t *testing.T) {
	t.Helper()
	var asked []string
	var waits []time.Duration
	var stdout, stderr bytes.Buffer
	code := runMain(tc.args, &stdout, &stderr, scriptedFetch(t, tc.responses, &asked), func(d time.Duration) { waits = append(waits, d) })
	if code != tc.wantCode {
		t.Errorf("runMain(%q) = %d, want %d; stderr:\n%s", tc.args, code, tc.wantCode, stderr.String())
	}
	if tc.wantStderr == "" && stderr.Len() != 0 || !strings.HasPrefix(stderr.String(), tc.wantStderr) {
		t.Errorf("stderr = %q, want it to start %q", stderr.String(), tc.wantStderr)
	}
	for _, want := range tc.wantStdout {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
		}
	}
	if tc.wantStdout == nil && stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
	if !reflect.DeepEqual(asked, tc.wantAsked) {
		t.Errorf("fetched %q, want %q", asked, tc.wantAsked)
	}
	if len(waits) != tc.wantWaits || tc.wantWaits > 0 && waits[0] != tc.wantDelay {
		t.Errorf("waited %v, want %d wait(s) of %v", waits, tc.wantWaits, tc.wantDelay)
	}
}

// TestRunMain_Arguments_JudgeEachPageOrRefuse verifies every outcome of a run:
// a flag it does not define, an argument, a URL that is not https, no attempt
// and a negative delay are usage errors; with no -url the published site is
// judged; a site serving every header passes; a site missing the two headers
// the Cloudflare rule adds, or either one of them, fails and says what to set,
// as an error annotation when annotations are asked for; either of them sent
// with a value its judge refuses fails and says why; any other header
// missing, a page that answered 404 and a page that could not be fetched each
// fail; every -url is judged in order; and the attempts and the delay reach
// the retry.
func TestRunMain_Arguments_JudgeEachPageOrRefuse(t *testing.T) {
	t.Parallel()
	withoutRule := siteResponse()
	withoutRule.Del("Content-Security-Policy")
	withoutRule.Del("X-Frame-Options")
	noPolicy := siteResponse()
	noPolicy.Del("Content-Security-Policy")
	noFrameOptions := siteResponse()
	noFrameOptions.Del("X-Frame-Options")
	framedAnywhere := siteResponse()
	framedAnywhere.Set("X-Frame-Options", "ALLOWALL")
	const evalPolicy = "default-src 'self'; script-src 'self' 'unsafe-eval'; object-src 'none'"
	evaluatesText := siteResponse()
	evaluatesText.Set("Content-Security-Policy", evalPolicy)
	noSniff := siteResponse()
	noSniff.Del("X-Content-Type-Options")
	cases := []runCase{
		{name: "an unknown flag", args: []string{"-no-such-flag"}, wantCode: 2, wantStderr: "flag provided but not defined: -no-such-flag\n"},
		{name: "an argument", args: []string{"extra"}, wantCode: 2, wantStderr: "audit_site_headers: takes no arguments, and was given [\"extra\"]; name each page with -url\n"},
		{name: "a plain http URL", args: []string{"-url", "http://site.example/"}, wantCode: 2, wantStderr: "audit_site_headers: -url \"http://site.example/\" is not an https URL with a host; the headers judged are the ones a browser reads over https\n"},
		{name: "a URL with no host", args: []string{"-url", "https:///path"}, wantCode: 2, wantStderr: "audit_site_headers: -url \"https:///path\" is not an https URL with a host; the headers judged are the ones a browser reads over https\n"},
		{name: "a URL that does not parse", args: []string{"-url", "https://%zz/"}, wantCode: 2, wantStderr: "audit_site_headers: -url \"https://%zz/\" is not an https URL with a host; the headers judged are the ones a browser reads over https\n"},
		{name: "no attempt", args: []string{"-attempts", "0"}, wantCode: 2, wantStderr: "audit_site_headers: -attempts is 0, and the page has to be fetched at least once\n"},
		{name: "a negative delay", args: []string{"-wait", "-1s"}, wantCode: 2, wantStderr: "audit_site_headers: -wait is -1s, and a delay cannot be negative\n"},
		{
			name: "the published site, fully hardened", responses: []response{{status: 200, header: siteResponse()}},
			wantAsked: []string{defaultSiteURL},
			wantStdout: []string{
				defaultSiteURL + " answered 200\n",
				"  ok       Content-Security-Policy: " + recommendedPolicy + "\n",
				"  ok       Strict-Transport-Security: max-age=31536000; includeSubDomains; preload\n",
				"  ok       X-Content-Type-Options: nosniff\n",
				"  ok       X-Frame-Options: DENY\n",
				"audit_site_headers: 1 page(s), 4 header(s) held, 0 failed, 0 waiting for the maintainer's rule\n",
			},
		},
		{
			name: "without the Cloudflare rule", args: []string{"-url", "https://site.example/"}, responses: []response{{status: 200, header: withoutRule}}, wantCode: 1,
			wantAsked: []string{"https://site.example/"},
			wantStdout: []string{
				"  FAIL     Content-Security-Policy is not sent. Set: " + recommendedPolicy + "\n",
				"  FAIL     X-Frame-Options is not sent. Set: DENY\n",
				"audit_site_headers: 1 page(s), 2 header(s) held, 2 failed, 0 waiting for the maintainer's rule\n",
			},
		},
		{
			name: "without Content-Security-Policy", args: []string{"-url", "https://site.example/"}, responses: []response{{status: 200, header: noPolicy}}, wantCode: 1,
			wantAsked: []string{"https://site.example/"},
			wantStdout: []string{
				"  FAIL     Content-Security-Policy is not sent. Set: " + recommendedPolicy + "\n",
				"  ok       X-Frame-Options: DENY\n",
				"audit_site_headers: 1 page(s), 3 header(s) held, 1 failed, 0 waiting for the maintainer's rule\n",
			},
		},
		{
			name: "without X-Frame-Options, annotated", args: []string{"-url", "https://site.example/", "-annotate"}, responses: []response{{status: 200, header: noFrameOptions}}, wantCode: 1,
			wantAsked: []string{"https://site.example/"},
			wantStdout: []string{
				"  ok       Content-Security-Policy: " + recommendedPolicy + "\n",
				"  FAIL     X-Frame-Options is not sent. Set: DENY\n",
				"audit_site_headers: 1 page(s), 3 header(s) held, 1 failed, 0 waiting for the maintainer's rule\n",
				"::error title=X-Frame-Options::https://site.example/: X-Frame-Options is not sent. Set: DENY\n",
			},
		},
		{
			name: "X-Frame-Options as ALLOWALL", args: []string{"-url", "https://site.example/"}, responses: []response{{status: 200, header: framedAnywhere}}, wantCode: 1,
			wantAsked: []string{"https://site.example/"},
			wantStdout: []string{
				"  FAIL     X-Frame-Options is permissive: it is \"ALLOWALL\", and a browser refuses a frame from another site only for DENY or SAMEORIGIN. Served: ALLOWALL. Set: DENY\n",
				"audit_site_headers: 1 page(s), 3 header(s) held, 1 failed, 0 waiting for the maintainer's rule\n",
			},
		},
		{
			name: "a policy allowing 'unsafe-eval'", args: []string{"-url", "https://site.example/"}, responses: []response{{status: 200, header: evaluatesText}}, wantCode: 1,
			wantAsked: []string{"https://site.example/"},
			wantStdout: []string{
				"  FAIL     Content-Security-Policy is permissive: it lets a script run text as code: script-src allows 'unsafe-eval'. Served: " + evalPolicy + ". Set: " + recommendedPolicy + "\n",
				"audit_site_headers: 1 page(s), 3 header(s) held, 1 failed, 0 waiting for the maintainer's rule\n",
			},
		},
		{
			name: "without X-Content-Type-Options", args: []string{"-url", "https://site.example/"}, responses: []response{{status: 200, header: noSniff}}, wantCode: 1,
			wantAsked:  []string{"https://site.example/"},
			wantStdout: []string{"  FAIL     X-Content-Type-Options is not sent. Set: nosniff\n", "3 header(s) held, 1 failed"},
		},
		{
			name: "two pages, one of them not found", args: []string{"-url", "https://site.example/", "-url", "https://site.example/missing/", "-annotate"},
			responses: []response{{status: 200, header: siteResponse()}, {status: 404}}, wantCode: 1,
			wantAsked:  []string{"https://site.example/", "https://site.example/missing/"},
			wantStdout: []string{"FAIL     https://site.example/missing/ answered 404, so its headers were not judged\n", "::error title=https%3A//site.example/missing/::"},
		},
		{
			name: "a page that cannot be fetched", args: []string{"-url", "https://site.example/", "-attempts", "2", "-wait", "3s"},
			responses: []response{{err: errors.New("refused")}, {err: errors.New("refused again")}}, wantCode: 1, wantWaits: 1, wantDelay: 3 * time.Second,
			wantAsked:  []string{"https://site.example/", "https://site.example/"},
			wantStdout: []string{"FAIL     https://site.example/ could not be fetched: after 2 attempt(s): refused again\n"},
		},
		{
			name: "five attempts ten seconds apart by default", responses: []response{{err: errFetch}, {err: errFetch}, {err: errFetch}, {err: errFetch}, {err: errFetch}},
			wantCode: 1, wantWaits: 4, wantDelay: 10 * time.Second,
			wantAsked:  []string{defaultSiteURL, defaultSiteURL, defaultSiteURL, defaultSiteURL, defaultSiteURL},
			wantStdout: []string{"could not be fetched: after 5 attempt(s): refused\n"},
		},
		{
			name: "one attempt and no delay", args: []string{"-attempts", "1", "-wait", "0s"}, responses: []response{{status: 200, header: siteResponse()}},
			wantAsked:  []string{defaultSiteURL},
			wantStdout: []string{"4 header(s) held, 0 failed"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t)
		})
	}
}

// TestRunMain_Summary_AppendsToTheFileItNames verifies -summary: the Markdown
// is appended to the file, which keeps what earlier steps wrote to it, and a
// file that cannot be opened fails the run after the report was printed.
func TestRunMain_Summary_AppendsToTheFileItNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(path, []byte("earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var asked []string
	var stdout, stderr bytes.Buffer
	ok := []response{{status: 200, header: siteResponse()}}
	if code := runMain([]string{"-summary", path}, &stdout, &stderr, scriptedFetch(t, ok, &asked), func(time.Duration) {}); code != 0 {
		t.Fatalf("runMain = %d; stderr:\n%s", code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "earlier\n## Documentation site security headers\n") || !strings.HasSuffix(string(data), "0 waiting for the maintainer's rule\n") {
		t.Errorf("summary file =\n%s", data)
	}

	stdout.Reset()
	missing := filepath.Join(dir, "no-such-dir", "summary.md")
	if code := runMain([]string{"-summary", missing}, &stdout, &stderr, scriptedFetch(t, ok, &asked), func(time.Duration) {}); code != 1 {
		t.Errorf("runMain with an unwritable summary = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr.String(), "audit_site_headers: write summary: ") || !strings.Contains(stdout.String(), "answered 200") {
		t.Errorf("stderr = %q, stdout = %q", stderr.String(), stdout.String())
	}
}

// TestMain_UnknownFlag_ExitsWithTheUsageStatus verifies that main hands the
// process arguments after its own name to runMain and passes the status on to
// the exit seam, without fetching anything for a usage error.
func TestMain_UnknownFlag_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = previousArgs, previousExit })
	got := -1
	os.Args = []string{"audit_site_headers", "-no-such-flag"}
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// pagesWorkflow is .github/workflows/pages.yml as the contract below reads it.
type pagesWorkflow struct {
	Jobs map[string]struct {
		Needs       any               `yaml:"needs"`
		Permissions map[string]string `yaml:"permissions"`
		Outputs     map[string]string `yaml:"outputs"`
		Steps       []struct {
			Name string            `yaml:"name"`
			Uses string            `yaml:"uses"`
			Run  string            `yaml:"run"`
			Env  map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestPagesWorkflow_AfterTheDeploy_RunsThisCheckOnThePublishedURL holds the
// Pages workflow to this command: a job after the deploy, holding no
// permission of its own beyond the workflow's read token, runs it on the URL
// the deploy published, with annotations and the job summary, and nothing in
// the deploy job itself changed to make room for it.
func TestPagesWorkflow_AfterTheDeploy_RunsThisCheckOnThePublishedURL(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "pages.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow pagesWorkflow
	if err = yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if got := workflow.Jobs["deploy"].Outputs["page_url"]; got != "${{ steps.deployment.outputs.page_url }}" {
		t.Errorf("the deploy job's page_url output is %q", got)
	}
	job, found := workflow.Jobs["headers"]
	if !found {
		t.Fatal("pages.yml has no headers job")
	}
	if job.Needs != "deploy" || job.Permissions != nil {
		t.Errorf("the headers job needs %v and sets permissions %v, want deploy and none of its own", job.Needs, job.Permissions)
	}
	var runs []string
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "./cmd/audit_site_headers") {
			runs = append(runs, step.Run)
			if step.Env["PAGE_URL"] != "${{ needs.deploy.outputs.page_url }}" {
				t.Errorf("the check reads PAGE_URL %q, want the deploy job's page_url", step.Env["PAGE_URL"])
			}
		}
	}
	want := `go run ./cmd/audit_site_headers -url "$PAGE_URL" -annotate -summary "$GITHUB_STEP_SUMMARY"`
	if len(runs) != 1 || strings.TrimSpace(runs[0]) != want {
		t.Errorf("the headers job runs %q, want one step running %q", runs, want)
	}
}

// TestRepositorySettings_ThePage_ShowsEveryValueThisCheckAsksFor holds the
// maintainer's checklist to the declarations: the section siteHeaders points
// to exists, and every value this check tells Cloudflare to set appears in
// it verbatim, so the page and the check cannot disagree on what to paste.
func TestRepositorySettings_ThePage_ShowsEveryValueThisCheckAsksFor(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "development", "repository-settings.md"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	_, section, found := strings.Cut(page, "\n## Documentation site response headers\n")
	if !found {
		t.Fatal("repository-settings.md has no Documentation site response headers section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	for _, each := range siteHeaders {
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(section, each.name) || !strings.Contains(section, each.recommended) {
				t.Errorf("the section does not show %s with %q", each.name, each.recommended)
			}
		})
	}
}
