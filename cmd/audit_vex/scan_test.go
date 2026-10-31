// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// scan_test.go covers the source scan of the dependency set and the reading
// of what it prints, against a vulnerability database and a module the test
// writes, so nothing here reaches the network.
package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureAdvisory is one entry of the database the tests write: an advisory
// against module, naming one package and, when symbols is set, the functions
// of it that are vulnerable.
type fixtureAdvisory struct {
	id, module, pkg, summary string
	symbols                  []string
}

// The advisories the fixture database holds, one per level the scan can
// reach into the standard library, which the fixture module's one call to
// fmt.Println decides, and one against a module nothing loads.
var (
	moduleOnly = fixtureAdvisory{id: "GO-2099-0001", module: "stdlib", pkg: "net/smtp", symbols: []string{"SendMail"}, summary: "A package the fixture does not import"}
	packageOf  = fixtureAdvisory{id: "GO-2099-0002", module: "stdlib", pkg: "fmt", symbols: []string{"Sscanf"}, summary: "A function the fixture does not call"}
	calledFunc = fixtureAdvisory{id: "GO-2099-0003", module: "stdlib", pkg: "fmt", symbols: []string{"Println"}, summary: "A function the fixture calls"}
	neverRead  = fixtureAdvisory{id: "GO-2099-0004", module: "example.com/never", pkg: "example.com/never", summary: "A module nothing loads"}
)

// fixtureTag is the build tag under which the fixture module imports
// net/smtp, which moves moduleOnly from the module to the package level.
const fixtureTag = "fixturesmtp"

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
	byModule := map[string][]vuln{}
	var order []string
	files := map[string]any{"index/db.json": map[string]string{"modified": modified}}
	for _, a := range advisories {
		if byModule[a.module] == nil {
			order = append(order, a.module)
		}
		byModule[a.module] = append(byModule[a.module], vuln{ID: a.id, Modified: modified})
		imported := map[string]any{"path": a.pkg}
		if len(a.symbols) != 0 {
			imported["symbols"] = a.symbols
		}
		files["ID/"+a.id+".json"] = map[string]any{
			"schema_version": "1.3.1",
			"id":             a.id,
			"modified":       modified,
			"published":      modified,
			"summary":        a.summary,
			"details":        a.summary + ".",
			"affected": []any{map[string]any{
				"package":            map[string]string{"name": a.module, "ecosystem": "Go"},
				"ranges":             []any{map[string]any{"type": "SEMVER", "events": []any{map[string]string{"introduced": "0"}}}},
				"ecosystem_specific": map[string]any{"imports": []any{imported}},
			}},
		}
	}
	modules := make([]module, 0, len(order))
	for _, path := range order {
		modules = append(modules, module{Path: path, Vulns: byModule[path]})
	}
	files["index/modules.json"] = modules
	for name, content := range files {
		data, err := json.Marshal(content)
		if err != nil {
			t.Fatalf("encoding %s: %v", name, err)
		}
		writeFile(t, dir, filepath.FromSlash(name), string(data))
	}
	slashed := filepath.ToSlash(dir)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return "file://" + slashed
}

// writeFixtureModule writes a module with no dependencies whose main package
// calls fmt.Println, and imports net/smtp under fixtureTag, and returns its
// directory.
func writeFixtureModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module "+fixtureModule+"\n\ngo 1.27\n")
	writeFile(t, dir, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"fixture\") }\n")
	writeFile(t, dir, "smtp.go", "//go:build "+fixtureTag+"\n\npackage main\n\nimport _ \"net/smtp\"\n")
	return dir
}

// TestScanSource_ReportsEachLevelTheScanReaches runs govulncheck over the
// fixture module, with and without the build tag that brings net/smtp in.
//
// Each advisory against the standard library is reported at the level the
// fixture's code decides, and the advisory against a module nothing loads is
// not reported at all: that second half is what makes the first mean
// something, and the tag is what makes the build tags the Makefile passes
// part of the dependency set.
func TestScanSource_ReportsEachLevelTheScanReaches(t *testing.T) {
	t.Parallel()

	dir := writeFixtureModule(t)
	db := writeVulnDB(t, moduleOnly, packageOf, calledFunc, neverRead)
	for _, tc := range []struct {
		name string
		tags string
		smtp level
	}{
		{name: "without the tag", smtp: levelModule},
		{name: "with the tag", tags: fixtureTag, smtp: levelPackage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := scanSource(context.Background(), auditConfig{dir: dir, db: db, tags: tc.tags, patterns: []string{"./..."}})
			if err != nil {
				t.Fatalf("scanSource: %v", err)
			}
			want := map[findingKey]level{
				{osv: moduleOnly.id, module: "stdlib"}: tc.smtp,
				{osv: packageOf.id, module: "stdlib"}:  levelPackage,
				{osv: calledFunc.id, module: "stdlib"}: levelSymbol,
			}
			if len(result.levels) != len(want) {
				t.Errorf("levels = %v, want %v", result.levels, want)
			}
			for key, reached := range want {
				if result.levels[key] != reached {
					t.Errorf("%v reached %v, want %v", key, result.levels[key], reached)
				}
			}
			if result.db != db || result.dbModified == "" || result.summaries[calledFunc.id] != calledFunc.summary {
				t.Errorf("result = %+v, want the database, its date and the summaries", result)
			}
			if v := result.versions[findingKey{osv: calledFunc.id, module: "stdlib"}]; !strings.HasPrefix(v, "v1.") {
				t.Errorf("the standard library's version = %q, want the toolchain's", v)
			}
		})
	}
}

// TestScanSource_AScanThatCannotBeMade_IsAnError covers govulncheck failing,
// whose message has to name the packages and carry its reason.
func TestScanSource_AScanThatCannotBeMade_IsAnError(t *testing.T) {
	t.Parallel()

	db := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "absent"))
	_, err := scanSource(context.Background(), auditConfig{dir: writeFixtureModule(t), db: db, patterns: []string{"./..."}})
	if err == nil || !strings.Contains(err.Error(), "govulncheck over ./...") {
		t.Errorf("scanSource error = %v, want one naming the packages", err)
	}
}

// TestParseScan_KeepsTheDeepestLevelOfEachFinding covers the stream govulncheck
// writes for one advisory reached at several levels, which is one finding per
// level, and a finding with no trace, which names no module and is passed over.
func TestParseScan_KeepsTheDeepestLevelOfEachFinding(t *testing.T) {
	t.Parallel()

	stream := `{"config": {"db": "file:///db", "db_last_modified": "2026-09-30T00:00:00Z", "scan_mode": "source"}}
{"osv": {"id": "GO-2099-0003", "summary": "called"}}
{"finding": {"osv": "GO-2099-0003", "trace": [{"module": "stdlib", "version": "v1.27.3"}]}}
{"finding": {"osv": "GO-2099-0003", "trace": [{"module": "stdlib", "version": "v1.27.3", "package": "fmt", "function": "Println"}, {"module": "example.com/fixture"}]}}
{"finding": {"osv": "GO-2099-0003", "trace": [{"module": "stdlib", "version": "v1.27.3", "package": "fmt"}]}}
{"finding": {"osv": "GO-2099-0005", "trace": []}}
`
	result, err := parseScan(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("parseScan: %v", err)
	}
	key := findingKey{osv: "GO-2099-0003", module: "stdlib"}
	if len(result.levels) != 1 || result.levels[key] != levelSymbol || result.versions[key] != "v1.27.3" {
		t.Errorf("levels = %v, versions = %v, want %v at the symbol level once", result.levels, result.versions, key)
	}
	if result.db != "file:///db" || result.summaries["GO-2099-0003"] != "called" {
		t.Errorf("result = %+v, want the database and the summary", result)
	}
}

// TestParseScan_RefusesAStreamThatIsNotASourceScan covers output with no
// configuration, the configuration of a binary scan, and output that is not
// JSON, none of which may read as a clean dependency set.
func TestParseScan_RefusesAStreamThatIsNotASourceScan(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, stream, want string
	}{
		{name: "empty", stream: "", want: `a "" scan`},
		{name: "a binary scan", stream: `{"config": {"scan_mode": "binary"}}`, want: `a "binary" scan`},
		{name: "not JSON", stream: `{"config": `, want: "reading govulncheck's output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseScan(strings.NewReader(tc.stream)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parseScan error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}
