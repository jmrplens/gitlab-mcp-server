// scan_test.go covers the govulncheck run over one binary and the reading of
// what it prints, against a vulnerability database written by the test so
// nothing here reaches the network.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureAdvisory is one entry of the database the tests write.
type fixtureAdvisory struct {
	id, module, summary string
}

// The two advisories every fixture database holds: one against the standard
// library at every version, which any Go binary carries, and one against a
// module no fixture links, which no scan may report.
var (
	everyStdlib = fixtureAdvisory{id: "GO-2099-0001", module: "stdlib", summary: "A fixture advisory against every standard library"}
	neverLinked = fixtureAdvisory{id: "GO-2099-0002", module: "example.com/never", summary: "A fixture advisory against a module nothing links"}
)

// writeVulnDB writes a vulnerability database in the layout vuln.go.dev
// serves (index/db.json, index/modules.json and one ID/<id>.json per
// advisory) and returns it as the file URL govulncheck's -db takes.
func writeVulnDB(t *testing.T, advisories ...fixtureAdvisory) string {
	t.Helper()

	dir := t.TempDir()
	const modified = "2026-09-30T00:00:00Z"
	type vuln struct {
		ID       string `json:"id"`
		Modified string `json:"modified"`
	}
	type module struct {
		Path  string `json:"path"`
		Vulns []vuln `json:"vulns"`
	}
	var modules []module
	files := map[string]any{"index/db.json": map[string]string{"modified": modified}}
	for _, a := range advisories {
		modules = append(modules, module{Path: a.module, Vulns: []vuln{{ID: a.id, Modified: modified}}})
		files["ID/"+a.id+".json"] = map[string]any{
			"schema_version": "1.3.1",
			"id":             a.id,
			"modified":       modified,
			"published":      modified,
			"summary":        a.summary,
			"details":        a.summary + ".",
			"affected": []any{map[string]any{
				"package": map[string]string{"name": a.module, "ecosystem": "Go"},
				"ranges":  []any{map[string]any{"type": "SEMVER", "events": []any{map[string]string{"introduced": "0"}}}},
				"ecosystem_specific": map[string]any{
					"imports": []any{map[string]string{"path": "fmt"}},
				},
			}},
		}
	}
	files["index/modules.json"] = modules
	for name, content := range files {
		data, encodeErr := json.Marshal(content)
		if encodeErr != nil {
			t.Fatalf("encoding %s: %v", name, encodeErr)
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o750); mkdirErr != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), mkdirErr)
		}
		if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
			t.Fatalf("writing %s: %v", name, writeErr)
		}
	}
	slashed := filepath.ToSlash(dir)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return "file://" + slashed
}

// buildFixture builds the fixture module for the host and returns the
// binary's path.
func buildFixture(t *testing.T) string {
	t.Helper()

	built, err := buildAll(context.Background(), writeFixtureModule(t), []build{{
		id: "fixture", main: ".", targets: []target{hostTarget},
	}}, t.TempDir())
	if err != nil {
		t.Fatalf("building the fixture: %v", err)
	}
	return built[0].path
}

// TestScanBinary_ReportsTheModuleAdvisoryAndNothingElse runs govulncheck over
// a real binary at module grain.
//
// The standard library is a module every Go binary links, so the advisory
// against it at every version must be reported, with the module and version
// the binary records; the advisory against a module the binary does not link
// must not be. The first half is the check this command exists for and the
// second is what makes the first mean something.
func TestScanBinary_ReportsTheModuleAdvisoryAndNothingElse(t *testing.T) {
	t.Parallel()

	db := writeVulnDB(t, everyStdlib, neverLinked)
	result, err := scanBinary(context.Background(), db, buildFixture(t))
	if err != nil {
		t.Fatalf("scanBinary: %v", err)
	}
	if len(result.findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the standard library advisory", result.findings)
	}
	f := result.findings[0]
	if f.osv != everyStdlib.id || f.module != "stdlib" || f.version == "" || f.fixed != "" {
		t.Errorf("finding = %+v, want %s against stdlib at the binary's version, with no fix", f, everyStdlib.id)
	}
	if result.summaries[everyStdlib.id] != everyStdlib.summary {
		t.Errorf("summary = %q, want %q", result.summaries[everyStdlib.id], everyStdlib.summary)
	}
	if result.db != db || result.dbModified == "" || result.goVersion == "" || result.modules == 0 {
		t.Errorf("result = %+v, want the database, its date, the Go version and the modules scanned", result)
	}
}

// TestScanBinary_AFileThatIsNotABinary_IsAnError covers the scan itself
// failing, whose message has to name the file and carry govulncheck's reason.
func TestScanBinary_AFileThatIsNotABinary_IsAnError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent")
	_, err := scanBinary(context.Background(), writeVulnDB(t, everyStdlib), path)
	if err == nil || !strings.Contains(err.Error(), "govulncheck on "+path) {
		t.Fatalf("scanBinary = %v, want an error naming %s", err, path)
	}
}

// TestParseScan_ReadsEachKindOfMessage covers the stream's four kinds of
// message, a finding without a trace among them, and ignores a kind this
// command does not read.
func TestParseScan_ReadsEachKindOfMessage(t *testing.T) {
	t.Parallel()

	stream := `{"config":{"db":"https://vuln.go.dev","db_last_modified":"2026-09-30T00:00:00Z"}}
{"progress":{"message":"Scanning your binary for known vulnerabilities..."}}
{"SBOM":{"go_version":"go1.27.1","modules":[{"path":"stdlib","version":"v1.27.1"},{"path":"golang.org/x/crypto","version":"v0.57.0"}]}}
{"osv":{"id":"GO-2026-5932","summary":"openpgp is unmaintained"}}
{"finding":{"osv":"GO-2026-5932","trace":[{"module":"golang.org/x/crypto","version":"v0.57.0"}]}}
{"finding":{"osv":"GO-2099-0003","fixed_version":"v1.2.3"}}
`
	result, err := parseScan(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("parseScan: %v", err)
	}
	if result.db != "https://vuln.go.dev" || result.dbModified != "2026-09-30T00:00:00Z" || result.goVersion != "go1.27.1" || result.modules != 2 {
		t.Errorf("result = %+v, want the configuration and the module list read", result)
	}
	if result.summaries["GO-2026-5932"] != "openpgp is unmaintained" {
		t.Errorf("summaries = %v, want the advisory's summary", result.summaries)
	}
	want := []finding{
		{osv: "GO-2026-5932", module: "golang.org/x/crypto", version: "v0.57.0"},
		{osv: "GO-2099-0003", fixed: "v1.2.3"},
	}
	if len(result.findings) != len(want) {
		t.Fatalf("findings = %+v, want %+v", result.findings, want)
	}
	for i, w := range want {
		t.Run(w.osv, func(t *testing.T) {
			t.Parallel()

			if result.findings[i] != w {
				t.Errorf("finding %d = %+v, want %+v", i, result.findings[i], w)
			}
		})
	}
}

// TestParseScan_RefusesWhatIsNotAScan covers the outputs that must not read
// as a clean binary: nothing at all, a stream that never said what it scanned
// or with what, and text that is not the stream.
//
// Each parse is given a deadline, because a decoder that has failed fails the
// same way on every later call: a read that stopped treating the error as the
// end would spin rather than fail, and the test has to say so rather than hang.
func TestParseScan_RefusesWhatIsNotAScan(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		stream string
	}{
		{name: "empty", stream: ""},
		{name: "no configuration", stream: `{"SBOM":{"go_version":"go1.27.1"}}`},
		{name: "no module list", stream: `{"config":{"db":"https://vuln.go.dev"}}`},
		{name: "not JSON", stream: `{"config":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type outcome struct {
				result scanResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := parseScan(strings.NewReader(tc.stream))
				done <- outcome{result, err}
			}()
			select {
			case got := <-done:
				if got.err == nil {
					t.Fatalf("parseScan accepted %q as a scan: %+v", tc.stream, got.result)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("parseScan(%q) did not return within 10s", tc.stream)
			}
		})
	}
}
