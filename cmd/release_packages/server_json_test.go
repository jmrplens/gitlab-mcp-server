// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// server_json_test.go holds the server-json-packages subcommand to what
// scripts/validate-server-json-packages.sh said and asked.
//
// testdata/server_json_packages.json is the record. Each scenario laid out a
// server.json, and whatever else it reads (a Dockerfile beside it, the npm
// launcher's package.json), and ran the script at fe8773534, the revision
// that removed it, with jq 1.6 and curl 7.88.1. A stand-in for curl first on
// PATH rewrote every https:// URL to one stand-in HTTP server, which answered
// each request from the scenario's routes, keyed by the method and the
// original URL, and recorded the method, URL, Authorization and Accept of
// every request. The bundles and NuGet packages the routes serve are the
// files in testdata/server_json_files, so a hash the script printed is the
// hash the command prints. The scenarios are scripts/
// validate_server_json_packages_sh_test.py's cases and, for every branch of
// the script that suite never reached, at least one more.
//
// Here the same routes are served by an httptest server, and the command's
// client is handed a transport that sends every request there the same way.
// Each scenario is held to the exit status, to every line on stdout, to
// every line on stderr but curl's own error lines (where the command
// describes a failed request in its own words), and to the requests, in
// order. serverJSONDifferences lists the lines the command says differently
// on purpose, and why. The credentials the stand-in hands out are written in
// the record as placeholders, which [readServerJSONRecord] puts back before
// anything reads it.
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// serverJSONRecord is testdata/server_json_packages.json.
type serverJSONRecord struct {
	RecordedFrom string           `json:"recorded_from"`
	Runs         []serverJSONCase `json:"runs"`
}

// serverJSONCase is one recorded scenario.
type serverJSONCase struct {
	Name       string            `json:"name"`
	ServerJSON json.RawMessage   `json:"server_json"`
	Files      map[string]string `json:"files"`
	Env        map[string]string `json:"env"`
	Routes     map[string]route  `json:"routes"`
	Exit       int               `json:"exit"`
	Stdout     string            `json:"stdout"`
	Stderr     string            `json:"stderr"`
	Requests   []sentRequest     `json:"requests"`
}

// route is how the stand-in answers one method and URL.
type route struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	File    string            `json:"file"`
}

// sentRequest is one request the stand-in was sent.
type sentRequest struct {
	Method        string `json:"method"`
	URL           string `json:"url"`
	Authorization string `json:"authorization"`
	Accept        string `json:"accept"`
}

// credentialPlaceholder is how the record writes a credential the stand-in
// hands out: <credential NAME> stands for NAME, in the route that hands it out,
// the environment that sets it and the Authorization of every request that
// carries it. A scenario therefore runs with, and is held to, exactly the
// values the script was handed and sent, while no line of the file reads to
// a secret scanner as a bearer credential.
var credentialPlaceholder = regexp.MustCompile(`<credential ([a-z-]+)>`)

// writtenOutBearer is a bearer credential the record writes out rather than
// as a placeholder.
var writtenOutBearer = regexp.MustCompile(`Bearer [^<]`)

// readServerJSONRecord reads testdata/server_json_packages.json with every
// credential placeholder put back.
func readServerJSONRecord(t *testing.T) serverJSONRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "server_json_packages.json"))
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if written := writtenOutBearer.Find(data); written != nil {
		t.Fatalf("the record writes a credential out (%q): write it as <credential NAME>", written)
	}
	expanded := credentialPlaceholder.ReplaceAll(data, []byte("${1}"))
	if left := bytes.Index(expanded, []byte("<credential")); left >= 0 {
		t.Fatalf("the record holds a credential placeholder the pattern does not read, at byte %d: write it as <credential NAME>", left)
	}
	var record serverJSONRecord
	if decodeErr := json.Unmarshal(expanded, &record); decodeErr != nil {
		t.Fatalf("decoding the record server_json_packages.json: %v", decodeErr)
	}
	return record
}

// emDash is U+2014, which the repository's text may not carry, built from
// its code point to quote the script's line that did.
const emDash = string(rune(0x2014))

// serverJSONDifferences are the stderr lines of a recorded scenario the
// command does not repeat, each mapped to what it writes in its place, or to
// nothing.
var serverJSONDifferences = map[string]map[string]string{
	// The script held every npm entry to the fields of every entry sharing
	// its identifier, so one entry's fileSha256 failed the three beside it,
	// and counted each of those lines as a failed package. Each entry is
	// judged by its own fields here.
	"npm entries the registry would refuse": {
		"  FAIL: carries field(s) the registry rejects for npm packages: fileSha256#2": "",
		"  FAIL: carries field(s) the registry rejects for npm packages: fileSha256#3": "",
		"6 of 4 declared package(s) failed validation":                                 "4 of 4 declared package(s) failed validation",
	},
	// Python's JSON error is Python's, and the command's is encoding/json's,
	// so only the words before it are held (a trailing * matches the rest).
	"mcpb refusals of every kind": {
		"manifest.json is not JSON: Expecting value: line 1 column 12 (char 11)": "manifest.json is not JSON: *",
	},
	// The repository's text carries no em dash, so the script's is spelled
	// by its code point here and replaced by a colon in the command's.
	"npm published before mcpName, on a mutable release, with a token": {
		"  WARNING: release v9.8.7 is mutable " + emDash + " its assets and checksums.txt can be replaced together. Enable immutable releases in the repository settings; the workflow already publishes drafts, which the feature requires.": "  WARNING: release v9.8.7 is mutable: its assets and checksums.txt can be replaced together. Enable immutable releases in the repository settings; the workflow already publishes drafts, which the feature requires.",
	},
}

// standIn serves a scenario's routes and records what it was sent.
type standIn struct {
	routes map[string]route
	mu     sync.Mutex
	sent   []sentRequest
}

// ServeHTTP answers a request rewritten by [rewritingTransport] from the
// route of its original URL, and a 404 when there is none.
func (s *standIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	original := "https://" + strings.TrimPrefix(r.RequestURI, "/")
	s.mu.Lock()
	s.sent = append(s.sent, sentRequest{Method: r.Method, URL: original, Authorization: r.Header.Get("Authorization"), Accept: r.Header.Get("Accept")})
	s.mu.Unlock()
	answer, ok := s.routes[r.Method+" "+original]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	body := []byte(answer.Body)
	if answer.File != "" {
		data, err := os.ReadFile(filepath.Join("testdata", "server_json_files", answer.File))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body = data
	}
	for key, value := range answer.Headers {
		w.Header()[key] = []string{value}
	}
	status := answer.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// rewritingTransport sends every request to the stand-in, with the host it
// named as the first segment of the path, the way the stand-in for curl
// rewrote what the script handed curl.
type rewritingTransport struct {
	target *url.URL
}

// RoundTrip rewrites the request's URL and sends it.
func (rt rewritingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rewritten := req.Clone(req.Context())
	target, err := url.Parse(rt.target.String() + "/" + req.URL.Host + req.URL.RequestURI())
	if err != nil {
		return nil, err
	}
	rewritten.URL, rewritten.Host = target, target.Host
	return http.DefaultTransport.RoundTrip(rewritten)
}

// serveScenario starts the stand-in for one scenario and returns it with a
// fetcher that sends everything there and never waits to retry.
func serveScenario(t *testing.T, routes map[string]route, diagnostics *bytes.Buffer) (*standIn, *fetcher) {
	t.Helper()
	stand := &standIn{routes: routes}
	server := httptest.NewServer(stand)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return stand, &fetcher{client: &http.Client{Transport: rewritingTransport{target: target}}, diagnostics: diagnostics}
}

// layOutScenario writes a scenario's server.json and files into a new
// directory.
func layOutScenario(t *testing.T, scenario serverJSONCase) string {
	t.Helper()
	root := t.TempDir()
	if string(scenario.ServerJSON) != "null" {
		writeFile(t, root, "server.json", scenario.ServerJSON)
	}
	for rel, text := range scenario.Files {
		writeFile(t, root, rel, []byte(text))
	}
	return root
}

// recordedStderr is the stderr lines a scenario is compared on: curl's own
// lines dropped, and the differences applied. A repeated line is told apart
// by #n from its second occurrence on.
func recordedStderr(stderr string, differences map[string]string) []string {
	var lines []string
	seen := map[string]int{}
	for line := range strings.Lines(stderr) {
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, "curl: ") {
			continue
		}
		seen[line]++
		key := line
		if seen[line] > 1 {
			key = line + "#" + strconv.Itoa(seen[line])
		}
		if replacement, ok := differences[key]; ok {
			if replacement == "" {
				continue
			}
			line = replacement
		}
		lines = append(lines, line)
	}
	return lines
}

// matchLines compares lines, a wanted line ending in * matching any line
// that opens with what comes before it.
func matchLines(got, want []string) bool {
	return slices.EqualFunc(got, want, func(g, w string) bool {
		if open, ok := strings.CutSuffix(w, "*"); ok {
			return strings.HasPrefix(g, open)
		}
		return g == w
	})
}

// commandStderr is the stderr lines of the command a scenario is compared
// on: its descriptions of failed requests, which stand where curl's lines
// stood, dropped.
func commandStderr(stderr string) []string {
	var lines []string
	for line := range strings.Lines(stderr) {
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, "  GET ") || strings.HasPrefix(line, "  HEAD ") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// TestServerJSONCheck_ServerJSONRecord_SaysAndAsksWhatTheScriptDid verifies
// every recorded scenario: the exit status, stdout, stderr and the requests
// sent, in order.
func TestServerJSONCheck_ServerJSONRecord_SaysAndAsksWhatTheScriptDid(t *testing.T) {
	t.Parallel()

	record := readServerJSONRecord(t)
	if len(record.Runs) == 0 {
		t.Fatal("the record holds no scenario")
	}
	for _, scenario := range record.Runs {
		t.Run(scenario.Name, func(t *testing.T) {
			t.Parallel()

			root := layOutScenario(t, scenario)
			var stdout, stderr bytes.Buffer
			stand, fetch := serveScenario(t, scenario.Routes, &stderr)
			check := newServerJSONCheck(root, "server.json", &stdout, &stderr, func(key string) string { return scenario.Env[key] })
			check.fetch = fetch
			code := check.run()

			if code != scenario.Exit {
				t.Errorf("exit %d, want %d\nstderr:\n%s", code, scenario.Exit, stderr.String())
			}
			if stdout.String() != scenario.Stdout {
				t.Errorf("stdout\n%s\nwant\n%s", stdout.String(), scenario.Stdout)
			}
			if got, want := commandStderr(stderr.String()), recordedStderr(scenario.Stderr, serverJSONDifferences[scenario.Name]); !matchLines(got, want) {
				t.Errorf("stderr\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			if !slices.Equal(stand.sent, scenario.Requests) {
				t.Errorf("requests\n%+v\nwant\n%+v", stand.sent, scenario.Requests)
			}
		})
	}
}

// checkResult is what one run of the package check said.
type checkResult struct {
	code           int
	stdout, stderr string
	sent           []sentRequest
}

// runCheck runs the package check over a server.json and the files beside
// it, against routes served by the stand-in.
func runCheck(t *testing.T, serverJSON string, routes map[string]route, files, env map[string]string) checkResult {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "server.json", []byte(serverJSON))
	for rel, text := range files {
		writeFile(t, root, rel, []byte(text))
	}
	var stdout, stderr bytes.Buffer
	stand, fetch := serveScenario(t, routes, &stderr)
	check := newServerJSONCheck(root, "server.json", &stdout, &stderr, func(key string) string { return env[key] })
	check.fetch = fetch
	code := check.run()
	return checkResult{code: code, stdout: stdout.String(), stderr: stderr.String(), sent: stand.sent}
}

// TestServerJSONCheck_AServerJSONThatIsNotJSON_IsRefused covers a manifest
// the check cannot read.
func TestServerJSONCheck_AServerJSONThatIsNotJSON_IsRefused(t *testing.T) {
	t.Parallel()

	got := runCheck(t, `{"packages": [`, nil, nil, nil)
	if want := "ERROR: server.json could not be read as JSON: "; got.code != 1 || !strings.HasPrefix(got.stderr, want) || got.stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want 1, nothing and %q", got.code, got.stdout, got.stderr, want)
	}
}

// TestServerJSONDifferences_EachNamesALineOfItsScenario holds the table of
// differences to the record: a difference whose line its scenario does not
// hold excuses nothing.
func TestServerJSONDifferences_EachNamesALineOfItsScenario(t *testing.T) {
	t.Parallel()

	record := readServerJSONRecord(t)
	stderr := map[string]string{}
	for _, scenario := range record.Runs {
		stderr[scenario.Name] = scenario.Stderr
	}
	for name, lines := range serverJSONDifferences {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			text, ok := stderr[name]
			if !ok {
				t.Fatalf("no recorded scenario is named %q", name)
			}
			for line := range lines {
				bare, _, _ := strings.Cut(line, "#")
				if !strings.Contains(text, bare+"\n") {
					t.Errorf("the scenario's stderr holds no line %q", bare)
				}
			}
		})
	}
}
