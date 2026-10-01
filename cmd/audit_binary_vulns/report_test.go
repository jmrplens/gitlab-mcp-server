// report_test.go covers how the findings of every binary are merged, held to
// the declaration table and printed.
package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// scanOf is one binary's scan carrying the given findings.
func scanOf(build string, tgt target, findings ...finding) scanned {
	return scanned{
		binary: binary{build: build, target: tgt, path: "/tmp/" + build},
		result: scanResult{
			db: "https://vuln.go.dev", dbModified: "2026-09-30T00:00:00Z", goVersion: "go1.27.1", modules: 49,
			findings:  findings,
			summaries: map[string]string{"GO-2026-5932": "openpgp is unmaintained"},
		},
	}
}

var (
	linuxAMD64   = target{goos: "linux", goarch: "amd64"}
	windowsARM64 = target{goos: "windows", goarch: "arm64"}
	cryptoAdvice = finding{osv: "GO-2026-5932", module: "golang.org/x/crypto", version: "v0.57.0"}
	stdlibAdvice = finding{osv: "GO-2099-0001", module: "stdlib", version: "v1.27.1", fixed: "v1.27.2"}
)

// TestJudge_MergesOneAdvisoryAcrossTheBinaries covers the merge: one advisory
// against one module is one finding naming every target that carries it, in
// the order they were scanned, and a target two builds share is named once.
func TestJudge_MergesOneAdvisoryAcrossTheBinaries(t *testing.T) {
	t.Parallel()

	rep := judge([]scanned{
		scanOf("server", linuxAMD64, cryptoAdvice),
		scanOf("server", windowsARM64, cryptoAdvice, stdlibAdvice),
		scanOf("other", linuxAMD64, cryptoAdvice),
	}, nil)

	if len(rep.findings) != 2 {
		t.Fatalf("findings = %+v, want the two advisories once each", rep.findings)
	}
	crypto := rep.findings[0]
	if crypto.osv != cryptoAdvice.osv || crypto.summary != "openpgp is unmaintained" {
		t.Errorf("first finding = %+v, want GO-2026-5932 with its summary, sorted first", crypto)
	}
	if want := []string{"linux/amd64", "windows/arm64"}; !slices.Equal(crypto.targets, want) {
		t.Errorf("targets = %v, want %v", crypto.targets, want)
	}
	if rep.findings[1].fixed != "v1.27.2" || rep.findings[1].declaration != nil {
		t.Errorf("second finding = %+v, want the stdlib advisory, fixed, and undeclared", rep.findings[1])
	}
	if rep.ok() || rep.undeclared() != 2 {
		t.Errorf("ok = %v with %d undeclared, want a failure with two", rep.ok(), rep.undeclared())
	}
}

// TestJudge_HoldsTheFindingsToTheTable covers the three outcomes an entry can
// have: accepting a finding, matching nothing, and not being readable.
func TestJudge_HoldsTheFindingsToTheTable(t *testing.T) {
	t.Parallel()

	accepted := declaration{category: categoryNotLinked, reason: "openpgp is not linked"}
	rep := judge([]scanned{scanOf("server", linuxAMD64, cryptoAdvice)}, map[string]declaration{
		declarationKey(cryptoAdvice.osv, cryptoAdvice.module): accepted,
		"GO-2099-0009 example.com/gone":                       {category: categoryFixNotYetAdoptable, reason: "fixed in v2"},
	})
	if len(rep.findings) != 1 || rep.findings[0].declaration == nil || *rep.findings[0].declaration != accepted {
		t.Fatalf("findings = %+v, want GO-2026-5932 accepted by its declaration", rep.findings)
	}
	if want := []string{"GO-2099-0009 example.com/gone"}; !slices.Equal(rep.stale, want) {
		t.Errorf("stale = %v, want %v", rep.stale, want)
	}
	if rep.ok() {
		t.Error("a run with a stale declaration passed")
	}

	unreadable := judge(nil, map[string]declaration{"not a key at all": {category: categoryNotLinked, reason: "x"}})
	if len(unreadable.invalid) != 1 || unreadable.ok() {
		t.Errorf("invalid = %v, ok = %v; want the unreadable entry reported and the run failed", unreadable.invalid, unreadable.ok())
	}

	// An entry whose key matches a finding but whose category nobody defined
	// is neither stale nor leaves the finding undeclared, and still fails the
	// run: a match is not a reviewed excuse.
	unreviewed := judge([]scanned{scanOf("server", linuxAMD64, cryptoAdvice)}, map[string]declaration{
		declarationKey(cryptoAdvice.osv, cryptoAdvice.module): {category: "invented", reason: "x"},
	})
	if unreviewed.undeclared() != 0 || len(unreviewed.stale) != 0 || len(unreviewed.invalid) != 1 || unreviewed.ok() {
		t.Errorf("undeclared %d, stale %v, invalid %v, ok %v; want only the unknown category reported, and the run failed",
			unreviewed.undeclared(), unreviewed.stale, unreviewed.invalid, unreviewed.ok())
	}
}

// TestReportWrite_SaysWhatWasScannedAndTheVerdict covers each shape of the
// report: a clean run, a run whose findings are all declared, and a failed
// one naming each kind of failure.
func TestReportWrite_SaysWhatWasScannedAndTheVerdict(t *testing.T) {
	t.Parallel()

	accepted := declaration{category: categoryNotLinked, reason: "openpgp is not linked"}
	for _, tc := range []struct {
		name string
		rep  report
		want []string
	}{
		{
			name: "clean",
			rep:  judge([]scanned{scanOf("server", linuxAMD64), scanOf("server", windowsARM64)}, nil),
			want: []string{
				"audit_binary_vulns: server linux/amd64 built with go1.27.1, 49 modules, checked against https://vuln.go.dev (modified 2026-09-30T00:00:00Z)\n",
				"audit_binary_vulns: no advisory affects a module the 2 binaries link\n",
			},
		},
		{
			name: "every finding declared",
			rep: judge([]scanned{scanOf("server", linuxAMD64, cryptoAdvice)},
				map[string]declaration{declarationKey(cryptoAdvice.osv, cryptoAdvice.module): accepted}),
			want: []string{
				"accepted GO-2026-5932 golang.org/x/crypto@v0.57.0 (no fixed version) in linux/amd64 [not-linked]: openpgp is not linked\n",
				"audit_binary_vulns: 1 findings in the 1 binaries, each accepted by a declaration\n",
			},
		},
		{
			name: "failed",
			rep: judge([]scanned{scanOf("server", linuxAMD64, cryptoAdvice, stdlibAdvice)}, map[string]declaration{
				"GO-2099-0009 example.com/gone": {category: categoryFixNotYetAdoptable, reason: "fixed in v2"},
				"broken":                        {category: categoryNotLinked, reason: "x"},
			}),
			want: []string{
				"FINDING GO-2026-5932 golang.org/x/crypto@v0.57.0 (no fixed version) in linux/amd64: openpgp is unmaintained\n",
				"FINDING GO-2099-0001 stdlib@v1.27.1 (fixed in v1.27.2) in linux/amd64: \n",
				`STALE declaration "GO-2099-0009 example.com/gone": no binary of this run carries the finding it accepts` + "\n",
				`INVALID declaration "broken": the key is not an advisory id and a module separated by one space` + "\n",
				"audit_binary_vulns: FAILED: 2 undeclared findings, 2 stale and 1 invalid declarations\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			tc.rep.write(&out)
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("the report lacks %q:\n%s", want, out.String())
				}
			}
		})
	}
}
