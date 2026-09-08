package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apishapes"
)

// fixedClock is the day a generated record is asserted against.
func fixedClock() time.Time { return time.Date(2026, 9, 7, 11, 30, 0, 0, time.UTC) }

// refusedDial is a transport nothing can answer through: every request fails
// the way a connection refused fails, without a socket that another process
// could answer on.
type refusedDial struct{}

func (refusedDial) RoundTrip(request *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("dial tcp %s: connect: connection refused", request.URL.Host)
}

// runCommand runs the command with both streams captured.
func runCommand(t *testing.T, cfg genRun) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(cfg, &out, &errOut)
	return status, out.String(), errOut.String()
}

// wholeDocument builds a document carrying enough operations to clear the floor
// a whole GitLab API has to reach, so a test about something else is not
// stopped by the truncation check. The operations are trivial because the floor
// counts them and reads nothing else.
func wholeDocument(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("openapi: 3.0.0\ninfo:\n  version: v4\npaths:\n")
	b.WriteString("  /api/v4/projects/{id}:\n    get:\n      parameters:\n      - name: id\n        in: path\n" +
		"      responses:\n        '200':\n          content:\n            application/json:\n              schema:\n                $ref: '#/components/schemas/Project'\n" +
		"    put:\n      requestBody:\n        content:\n          application/json:\n            schema:\n              $ref: '#/components/schemas/Project'\n" +
		"      responses:\n        '200':\n          content:\n            application/json:\n              schema:\n                $ref: '#/components/schemas/Project'\n")
	for i := range apishapes.MinimumOperations {
		fmt.Fprintf(&b, "  /api/v4/filler%d:\n    get:\n      responses:\n        '204':\n          description: none\n", i)
	}
	b.WriteString("components:\n  schemas:\n    Project:\n      type: object\n      properties:\n        id: {type: integer}\n        name: {type: string}\n")
	return b.String()
}

// serving returns a server answering every request with body.
func serving(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// truncating returns a server that promises more body than it sends and then
// hangs up, which is what a connection dropped mid-download looks like to the
// reader.
func truncating(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("openapi: 3.0.0\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if hijacker, ok := w.(http.Hijacker); ok {
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack the connection: %v", err)
				return
			}
			_ = connection.Close()
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestRun_AgainstTheDocument_WritesARecordThatReadsBack verifies a whole
// generation: fetch, extract, and write a record the gate can then read.
func TestRun_AgainstTheDocument_WritesARecordThatReadsBack(t *testing.T) {
	server := serving(t, wholeDocument(t))
	dir := filepath.Join(t.TempDir(), "record")

	status, out, errOut := runCommand(t, genRun{
		ref: "master", dir: dir, client: server.Client(), url: server.URL, now: fixedClock,
	})

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	for _, want := range []string{"fetching", "wrote " + apishapes.FileName, "with a response schema"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out)
			}
		})
	}

	doc, err := apishapes.Read(dir)
	if err != nil {
		t.Fatalf("the record it wrote does not read back: %v", err)
	}
	if doc.Source.RetrievedAt != "2026-09-07" || doc.Source.SHA256 == "" {
		t.Errorf("provenance = %+v, want the day and the digest of what it read", doc.Source)
	}
	if got := doc.Operations["GET /api/v4/projects/{id}"].Response; strings.Join(got, ",") != "id,name" {
		t.Errorf("the recorded response = %v, want the schema's properties", got)
	}
}

// TestRun_GenerationFailures_ExitNonZeroAndSayWhy verifies that nothing
// replaces the committed record silently. The truncation case is the one that
// matters: a short read that overwrote a whole record would leave a gate
// speaking for an API nobody fetched, and it would pass its own --check.
func TestRun_GenerationFailures_ExitNonZeroAndSayWhy(t *testing.T) {
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(refusing.Close)

	cases := []struct {
		name string
		cfg  genRun
		want string
	}{
		{
			name: "the ref does not exist",
			cfg:  genRun{dir: t.TempDir(), client: refusing.Client(), url: refusing.URL, now: fixedClock},
			want: "404",
		},
		{
			// A refused dial rather than a closed httptest server: closing one
			// frees its port, and under a parallel suite another test binary
			// can bind that port between the close and the request, which is
			// how this case answered once on a loaded box.
			name: "nothing answered",
			cfg:  genRun{dir: t.TempDir(), client: &http.Client{Transport: refusedDial{}}, url: "http://127.0.0.1:1", now: fixedClock},
			want: "fetch ",
		},
		{
			name: "what answered is not the document",
			cfg:  genRun{dir: t.TempDir(), client: http.DefaultClient, url: serving(t, "name: something else\n").URL, now: fixedClock},
			want: "not GitLab's API specification",
		},
		{
			name: "a truncated read",
			cfg: genRun{
				dir: t.TempDir(), client: http.DefaultClient, now: fixedClock,
				url: serving(t, "openapi: 3.0.0\npaths:\n  /api/v4/x:\n    get:\n      responses:\n        '204':\n          description: none\n").URL,
			},
			want: "refusing to replace the record with a truncated read",
		},
		{
			name: "the directory cannot be written",
			cfg: genRun{
				dir: filepath.Join(t.TempDir(), "blocked", "under"), client: http.DefaultClient, now: fixedClock,
				url: serving(t, wholeDocument(t)).URL,
			},
			want: "create ",
		},
		{
			name: "the request cannot be built",
			cfg:  genRun{dir: t.TempDir(), client: http.DefaultClient, url: "http://\x7f", now: fixedClock},
			want: "build the request",
		},
		{
			// A body that stops early is the failure a flaky network produces,
			// and the one that must not be mistaken for a short document: it is
			// refused as a read failure rather than extracted from.
			name: "the body stops short of what it promised",
			cfg: genRun{
				dir: t.TempDir(), client: http.DefaultClient, now: fixedClock,
				url: truncating(t).URL,
			},
			want: "read ",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if strings.Contains(testCase.cfg.dir, "blocked") {
				if err := os.WriteFile(filepath.Dir(testCase.cfg.dir), []byte("a file where a directory should be"), 0o600); err != nil {
					t.Fatalf("prepare the fixture: %v", err)
				}
			}

			status, _, errOut := runCommand(t, testCase.cfg)

			if status != 1 {
				t.Fatalf("exit status %d, want 1", status)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("stderr does not contain %q:\n%s", testCase.want, errOut)
			}
		})
	}
}

// canonicalRecord is a record satisfying every requirement recordProblems
// enforces, so a test about one of them is not tripped by the others.
func canonicalRecord() apishapes.Document {
	operations := make(map[string]apishapes.Operation, apishapes.MinimumOperations)
	for i := range apishapes.MinimumOperations {
		operations[fmt.Sprintf("GET /api/v4/filler%d", i)] = apishapes.Operation{}
	}
	return apishapes.Document{
		Source: apishapes.Source{
			URL: "https://gitlab.com/spec", Ref: "master", RetrievedAt: "2026-09-07",
			SHA256: "abc", Operations: len(operations),
		},
		Operations: operations,
	}
}

// TestRun_CheckMode_RefusesARecordItCannotRestOn verifies the CI half. It must
// need no network, because a gate that reaches gitlab.com is a gate that fails
// when gitlab.com does, and it must refuse every way the record can fail to be
// what it claims.
func TestRun_CheckMode_RefusesARecordItCannotRestOn(t *testing.T) {
	spoil := func(f func(*apishapes.Document)) apishapes.Document {
		doc := canonicalRecord()
		f(&doc)
		return doc
	}

	cases := []struct {
		name string
		doc  apishapes.Document
		want string
	}{
		{
			name: "a truncated extraction",
			doc:  spoil(func(d *apishapes.Document) { d.Operations = map[string]apishapes.Operation{"GET /api/v4/x": {}} }),
			want: "the download was truncated",
		},
		{
			name: "no provenance",
			doc:  spoil(func(d *apishapes.Document) { d.Source.Ref = "" }),
			want: "does not say which ref",
		},
		{
			name: "a date nothing can read",
			doc:  spoil(func(d *apishapes.Document) { d.Source.RetrievedAt = "one tuesday" }),
			want: "is not a date",
		},
		{
			name: "a date that has not happened",
			doc:  spoil(func(d *apishapes.Document) { d.Source.RetrievedAt = "2026-09-08" }),
			want: "has not happened yet",
		},
		{
			name: "a record past the window",
			doc:  spoil(func(d *apishapes.Document) { d.Source.RetrievedAt = "2020-01-01" }),
			want: "days old and the window is",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := apishapes.Write(dir, testCase.doc); err != nil {
				t.Fatalf("prepare the fixture: %v", err)
			}

			status, _, errOut := runCommand(t, genRun{dir: dir, check: true, now: fixedClock})

			if status != 1 {
				t.Fatalf("exit status %d, want 1. stderr:\n%s", status, errOut)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("stderr does not explain the refusal %q:\n%s", testCase.want, errOut)
			}
			if !strings.Contains(errOut, "make gen-api-shapes") {
				t.Errorf("stderr does not say how to fix it:\n%s", errOut)
			}
		})
	}
}

// TestRun_CheckMode_ARecordItCanRestOn_Passes verifies the other side of the
// gate, including the two shapes that are not a spoiled record: no record at
// all, and no clock handed in.
func TestRun_CheckMode_ARecordItCanRestOn_Passes(t *testing.T) {
	t.Run("a sound record passes", func(t *testing.T) {
		dir := t.TempDir()
		if err := apishapes.Write(dir, canonicalRecord()); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}

		status, out, errOut := runCommand(t, genRun{dir: dir, check: true, now: fixedClock})

		if status != 0 {
			t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
		}
		if !strings.Contains(out, "the record is usable") {
			t.Errorf("stdout does not report the record:\n%s", out)
		}
	})

	t.Run("no record at all", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{dir: t.TempDir(), check: true, now: fixedClock})

		if status != 1 {
			t.Fatalf("exit status %d, want 1", status)
		}
		if !strings.Contains(errOut, prefix) {
			t.Errorf("stderr does not name the command:\n%s", errOut)
		}
	})

	t.Run("no clock supplied", func(t *testing.T) {
		dir := t.TempDir()
		if err := apishapes.Write(dir, canonicalRecord()); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}

		// The age check needs a clock and main is the only caller with one to
		// give, so the default has to hold for every other caller. It cannot be
		// asserted as a pass, since this fixture ages past the window in 2027,
		// only that nothing panics and any refusal is about age.
		status, _, errOut := runCommand(t, genRun{dir: dir, check: true})

		if status != 0 && !strings.Contains(errOut, "days old") {
			t.Errorf("exit status %d for a reason other than age:\n%s", status, errOut)
		}
	})
}

// TestTarget_NoOverride_BuildsTheRawURLForTheRef verifies the URL the command
// fetches in production, which no other test exercises because they all point
// it at a local server.
func TestTarget_NoOverride_BuildsTheRawURLForTheRef(t *testing.T) {
	got := genRun{ref: "v19.3.1-ee"}.target()

	if !strings.Contains(got, "gitlab-org/gitlab/-/raw/v19.3.1-ee/") || !strings.HasSuffix(got, apishapes.SpecPath) {
		t.Errorf("target() = %q, want the raw URL for the ref", got)
	}
}

// TestRun_CheckModeAgainstTheCommittedRecord_Passes is the gate itself, run
// against the real file rather than a fixture: what CI asserts on every push is
// asserted here on every test run.
func TestRun_CheckModeAgainstTheCommittedRecord_Passes(t *testing.T) {
	status, out, errOut := runCommand(t, genRun{dir: filepath.Join("..", "..", defaultDir), check: true})

	if status != 0 {
		t.Fatalf("the committed record does not pass its own gate (exit %d):\n%s", status, errOut)
	}
	if !strings.Contains(out, "operations from") {
		t.Errorf("stdout does not report the record:\n%s", out)
	}
}
