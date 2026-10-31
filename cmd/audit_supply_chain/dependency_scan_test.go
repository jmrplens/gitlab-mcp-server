// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Tests for the eighth property: a dependency advisory blocks the required
// check, through a scan the Makefile pins by digest, a job the verdict of ci
// counts, and allow lists that set an advisory aside only on the terms
// SECURITY.md states.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scanJobs are the jobs every fixture ci.yml ends with so the dependency scan
// rule finds nothing there: one that runs the scan, and the verdict job that
// needs it.
const scanJobs = "  dependency-vulns:\n    steps:\n      - run: make check-dependency-vulns\n" +
	"  ci:\n    needs: [dependency-vulns]\n    steps:\n      - run: \"true\"\n"

// pinnedScanner is an OSV-Scanner image named by a tag and a digest.
const pinnedScanner = "ghcr.io/google/osv-scanner:v2.7.0@sha256:4f3b7e2c64b9d38a5febc5a7e4a3dfef1d1362f726dea7a2ef4762b46469fcd0"

// scanMakefile is the smallest Makefile the dependency scan rule passes: the
// scanner pinned by digest and a recipe that runs it.
const scanMakefile = "OSV_SCANNER_IMAGE=" + pinnedScanner + "\n\ncheck-dependency-vulns:\n" +
	"\tdocker run --rm -v \"$(CURDIR):/src:ro\" \"$(OSV_SCANNER_IMAGE)\" scan source --recursive /src\n"

// reviewDay is the day the allow list tests measure the review bound from.
var reviewDay = time.Date(2026, time.October, 31, 15, 4, 5, 0, time.UTC)

// TestMakefileScanProblems_Recipe_RunsThePinnedScanner verifies that the
// Makefile's scan is held to an image named by its digest, assigned once, and
// run by a recipe that reads the allow list beside each lockfile.
//
// A tag alone would let whoever moves it decide what the required check
// accepts, an image the recipe does not run would make the pin decoration, and
// a --config would put one file in place of every allow list this audit
// judges.
func TestMakefileScanProblems_Recipe_RunsThePinnedScanner(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		makefile string
		want     []string
	}{
		{name: "pinned and run", makefile: scanMakefile},
		{
			name: "assigned with := behind a comment that names another",
			makefile: "# OSV_SCANNER_IMAGE=ghcr.io/google/osv-scanner:latest\nOSV_SCANNER_IMAGE := " + pinnedScanner +
				"\ncheck-dependency-vulns:\n\tdocker run $(OSV_SCANNER_IMAGE)\n",
		},
		{
			name:     "never assigned",
			makefile: "check-dependency-vulns:\n\tdocker run $(OSV_SCANNER_IMAGE)\n",
			want:     []string{"Makefile: OSV_SCANNER_IMAGE is not assigned, so make check-dependency-vulns names no scanner image this audit can hold to a digest"},
		},
		{
			name:     "assigned twice",
			makefile: "OSV_SCANNER_IMAGE=" + pinnedScanner + "\nOSV_SCANNER_IMAGE?=" + pinnedScanner + "\ncheck-dependency-vulns:\n\tdocker run $(OSV_SCANNER_IMAGE)\n",
			want:     []string{"Makefile: OSV_SCANNER_IMAGE is assigned 2 times, so which image make check-dependency-vulns runs depends on the order make reads them in"},
		},
		{
			name:     "named by a tag",
			makefile: "OSV_SCANNER_IMAGE=ghcr.io/google/osv-scanner:v2.7.0\ncheck-dependency-vulns:\n\tdocker run $(OSV_SCANNER_IMAGE)\n",
			want:     []string{"Makefile: OSV_SCANNER_IMAGE=ghcr.io/google/osv-scanner:v2.7.0 is not pinned to a sha256 image digest, so the scanner that decides is whatever the tag names when the job runs"},
		},
		{
			name:     "no rule",
			makefile: "OSV_SCANNER_IMAGE=" + pinnedScanner + "\n",
			want:     []string{"Makefile: no rule makes check-dependency-vulns, so nothing holds the lockfiles to the dependency advisories"},
		},
		{
			name:     "a recipe that runs another scanner and one allow list",
			makefile: "OSV_SCANNER_IMAGE=" + pinnedScanner + "\ncheck-dependency-vulns:\n\tosv-scanner scan source --config osv-scanner.toml -r .\n",
			want: []string{
				"Makefile: the recipe of check-dependency-vulns does not run $(OSV_SCANNER_IMAGE), so the image pinned above is not the scanner that decides",
				"Makefile: the recipe of check-dependency-vulns passes --config, which puts one file in place of the osv-scanner.toml beside each lockfile, the allow lists this audit judges",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := makefileScanProblems(testCase.makefile)
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("makefileScanProblems() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestWorkflowScanProblems_RequiredCheck_CountsTheScan verifies that ci.yml is
// held to running the scan in a job the verdict of ci counts: reached through
// needs, never through a verdict that excuses its failure or its skip, and
// under no condition that lets the job pass without the scan.
//
// The scan blocks a change only if the one check branch protection requires
// cannot be green while the scan is red, so each way a workflow edit could
// break that is a finding.
func TestWorkflowScanProblems_RequiredCheck_CountsTheScan(t *testing.T) {
	t.Parallel()

	const scanStep = "    steps:\n      - run: make check-dependency-vulns\n"
	const where = ciWorkflow + ": job deps runs make check-dependency-vulns"
	cases := []struct {
		name string
		jobs string
		want []string
	}{
		{name: "needed directly", jobs: scanJobs},
		{
			name: "needed through a fan-in, beside an excused job and a step that is not a mapping",
			jobs: "  deps:\n" + scanStep + "  docs:\n    steps:\n      - run: echo docs\n" +
				"  checks:\n    needs: [deps, docs]\n    steps:\n      - just a string\n      - env:\n          INFORMATIONAL: docs\n        run: verdict\n" +
				"  ci:\n    needs: checks\n    steps:\n      - run: verdict\n",
		},
		{
			name: "excused on the first path read and counted on another",
			jobs: "  deps:\n" + scanStep + "  checks:\n    needs: [deps]\n    env:\n      MAY_SKIP: deps\n" +
				"  other:\n    needs: [deps]\n  ci:\n    needs: [checks, other]\n",
		},
		{
			name: "no job runs it",
			jobs: "  lint:\n    steps:\n      - run: make lint\n  other: not a job\n  ci:\n    needs: [lint]\n",
			want: []string{ciWorkflow + ": no job runs make check-dependency-vulns, so no change is held to the dependency advisories before it merges"},
		},
		{
			name: "run from another makefile or directory",
			jobs: "  deps:\n    steps:\n      - run: make -C site check-dependency-vulns\n" +
				"      - run: make check-dependency-vulns\n        working-directory: site\n" +
				"      - run: |\n          # make check-dependency-vulns\n          echo nothing\n" +
				"  ci:\n    needs: [deps]\n",
			want: []string{ciWorkflow + ": no job runs make check-dependency-vulns, so no change is held to the dependency advisories before it merges"},
		},
		{
			name: "under conditions",
			jobs: "  deps:\n    if: github.event_name == 'pull_request'\n    steps:\n      - run: make check-dependency-vulns\n        continue-on-error: true\n" +
				"  ci:\n    needs: [deps]\n",
			want: []string{where + " under if: and continue-on-error:, so the required check can pass on a run where the scan did not"},
		},
		{
			name: "no verdict job",
			jobs: "  deps:\n" + scanStep,
			want: []string{where + ", and there is no job ci, the required check, to count it"},
		},
		{
			name: "not needed",
			jobs: "  deps:\n" + scanStep + "  lint:\n    steps:\n      - run: make lint\n  ci:\n    needs: [lint]\n",
			want: []string{where + ", and job ci, the required check, does not need it, directly or through the jobs it needs"},
		},
		{
			name: "needed only as informational",
			jobs: "  deps:\n" + scanStep + "  checks:\n    needs: [deps]\n    steps:\n      - env:\n          INFORMATIONAL: deps\n        run: verdict\n" +
				"  ci:\n    needs: [checks]\n",
			want: []string{where + ", and job ci, the required check, reaches it only through a verdict that excuses its failure or its skip (INFORMATIONAL or MAY_SKIP)"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			text := "name: CI\njobs:\n" + testCase.jobs
			doc, order, err := parseWorkflow(text)
			if err != nil {
				t.Fatalf("parseWorkflow: %v", err)
			}
			got := workflowScanProblems([]workflow{{path: ciWorkflow, text: text, doc: doc, jobOrder: order}})
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("workflowScanProblems() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestWorkflowScanProblems_NoReadableCI_IsAFinding verifies that a tree whose
// ci.yml is missing, or does not parse to a mapping, is reported rather than
// passed: nothing then shows the required check depends on the scan.
func TestWorkflowScanProblems_NoReadableCI_IsAFinding(t *testing.T) {
	t.Parallel()

	want := ciWorkflow + ": the workflow is missing or is not a mapping, so nothing shows the required check depends on make check-dependency-vulns"
	cases := []struct {
		name      string
		workflows []workflow
	}{
		{name: "missing", workflows: []workflow{{path: workflowDir + "/release.yml", doc: map[string]any{}}}},
		{name: "not a mapping", workflows: []workflow{{path: ciWorkflow, text: "- a list\n"}}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := workflowScanProblems(testCase.workflows); len(got) != 1 || got[0] != want {
				t.Errorf("workflowScanProblems() = %#v, want [%q]", got, want)
			}
		})
	}
}

// TestOSVConfigProblems_Entry_IsHeldToThePolicy verifies that every entry of
// an allow list names a well-formed advisory that is not a malicious
// package's, once, with a reason that opens with a not_affected justification
// and says why, and a review date at most six months ahead.
//
// Those are the terms SECURITY.md sets for declaring an advisory not
// exploitable, and OSV-Scanner itself checks none of them: it honors any id,
// any reason or none, and any ignoreUntil or none, which would set the
// advisory aside forever.
func TestOSVConfigProblems_Entry_IsHeldToThePolicy(t *testing.T) {
	t.Parallel()

	const reason = `reason = "vulnerable_code_not_present: nothing imports the package"`
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "a complete entry, indented, among comments",
			text: "# header\n\n[[IgnoredVulns]]\n  id = \"GO-2026-5932\"\n  ignoreUntil = 2027-05-02\n  " + reason + "\n",
		},
		{
			name: "a malicious package",
			text: "[[IgnoredVulns]]\nid = \"MAL-2025-20690\"\nignoreUntil = 2027-01-31\n" + reason + "\n",
			want: []string{"x.toml:2: MAL-2025-20690 is a malicious-package advisory, which is never set aside: the package is removed and the lockfile regenerated"},
		},
		{
			name: "an id in no advisory shape",
			text: "[[IgnoredVulns]]\nid = \"go-2026-5932\"\nignoreUntil = 2027-01-31\n" + reason + "\n",
			want: []string{`x.toml:2: "go-2026-5932" is not an advisory id the way OSV spells one`},
		},
		{
			name: "the same advisory twice",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-01-31\n" + reason + "\n" +
				"[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-01-31\n" + reason + "\n",
			want: []string{"x.toml:6: GO-2026-5932 is set aside again, first on line 2"},
		},
		{
			name: "a reason with no justification",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-01-31\nreason = \"not used\"\n",
			want: []string{"x.toml:4: the reason does not open with one of the OpenVEX justifications for not_affected (component_not_present, " +
				"inline_mitigations_already_exist, vulnerable_code_cannot_be_controlled_by_adversary, vulnerable_code_not_in_execute_path, " +
				"vulnerable_code_not_present) and a colon, so it does not say why the advisory is not exploitable here"},
		},
		{
			name: "a justification and nothing after it",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-01-31\nreason = \"inline_mitigations_already_exist:  \"\n",
			want: []string{"x.toml:4: the reason names inline_mitigations_already_exist and says nothing after it, so a reviewer has no claim to check"},
		},
		{
			name: "a date beyond six months",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-05-03\n" + reason + "\n",
			want: []string{"x.toml:3: ignoreUntil = 2027-05-03 is more than 183 days ahead, and an entry is reviewed again at least every six months (the latest day today is 2027-05-02)"},
		},
		{
			name: "a day the calendar does not have",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-02-30\n" + reason + "\n",
			want: []string{"x.toml:3: ignoreUntil = 2027-02-30 is not a day of the calendar"},
		},
		{
			name: "an entry with nothing in it",
			text: "[[IgnoredVulns]]\n",
			want: []string{"x.toml:1: the [[IgnoredVulns]] table gives no id and no ignoreUntil and no reason, and every entry names the advisory, the reason it is not exploitable and the day it is reviewed again"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := osvConfigProblems("x.toml", testCase.text, reviewDay)
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("osvConfigProblems() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestOSVConfigProblems_UnreadLine_IsAFinding verifies that the allow list is
// read in one shape only, and that anything outside it is reported rather
// than guessed at.
//
// What OSV-Scanner sets aside is what it makes of the file, and TOML spells
// the same thing many ways; a package override, a key the audit does not
// read, an inline comment or an escape is each a way for the file to set
// aside something no reviewer reads, so each is refused.
func TestOSVConfigProblems_UnreadLine_IsAFinding(t *testing.T) {
	t.Parallel()

	const entry = "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = 2027-01-31\nreason = \"vulnerable_code_not_present: nothing imports it\"\n"
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "a package override",
			text: entry + "[[PackageOverrides]]\n",
			want: `x.toml:5: "[[PackageOverrides]]" is not a line this audit reads: an osv-scanner.toml here holds comments and [[IgnoredVulns]] tables of id, ignoreUntil and reason, one key a line`,
		},
		{
			name: "a key before any table",
			text: "GoVersionOverride = \"1.27.3\"\n" + entry,
			want: "x.toml:1: GoVersionOverride sits outside an [[IgnoredVulns]] table, which is the only kind of entry this audit reads",
		},
		{
			name: "a key the audit does not read",
			text: entry + "effectiveUntil = 2027-01-31\n",
			want: "x.toml:5: effectiveUntil is not a key this audit reads: an entry gives id, ignoreUntil and reason and nothing else, so that what it sets aside is what a reviewer reads",
		},
		{
			name: "a key given twice",
			text: entry + "reason = \"vulnerable_code_not_present: again\"\n",
			want: "x.toml:5: the [[IgnoredVulns]] table gives reason twice",
		},
		{
			name: "a quoted date",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\"\nignoreUntil = \"2027-01-31\"\nreason = \"vulnerable_code_not_present: nothing imports it\"\n",
			want: `x.toml:3: ignoreUntil = "2027-01-31" is not a bare date, YYYY-MM-DD` + "\n" +
				"x.toml:1: the [[IgnoredVulns]] table gives no ignoreUntil, and every entry names the advisory, the reason it is not exploitable and the day it is reviewed again",
		},
		{
			name: "an escape and an inline comment",
			text: "[[IgnoredVulns]]\nid = \"GO-2026-5932\" # set aside\nignoreUntil = 2027-01-31\nreason = \"vulnerable_code_not_present: \\u0000\"\n",
			want: `x.toml:2: id = "GO-2026-5932" # set aside is not a double-quoted string on one line with no escape in it` + "\n" +
				`x.toml:4: reason = "vulnerable_code_not_present: \u0000" is not a double-quoted string on one line with no escape in it` + "\n" +
				"x.toml:1: the [[IgnoredVulns]] table gives no id and no reason, and every entry names the advisory, the reason it is not exploitable and the day it is reviewed again",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := strings.Join(osvConfigProblems("x.toml", testCase.text, reviewDay), "\n")
			if got != testCase.want {
				t.Errorf("osvConfigProblems() =\n%s\nwant\n%s", got, testCase.want)
			}
		})
	}
}

// TestOSVConfigsInTree_Walk_ReadsWhatTheScanReads verifies that the allow
// lists judged are the ones the scan could honor: the root's, the site's and a
// testdata directory's, which the scan walks into, and not those under what
// .gitignore keeps out of the tree, the worktrees Claude Code checks out,
// another checkout or a tool's state.
func TestOSVConfigsInTree_Walk_ReadsWhatTheScanReads(t *testing.T) {
	t.Parallel()

	root := osvConfigTree(t)
	want := []string{".github/requirements/osv-scanner.toml", "cmd/tool/testdata/osv-scanner.toml", "osv-scanner.toml", "site/osv-scanner.toml"}
	if got := osvConfigsInTree(root); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("osvConfigsInTree() = %#v, want %#v", got, want)
	}
}

// osvConfigTree builds a tree with an osv-scanner.toml in every place the
// allow list walk reads and in every place it skips, beside a file of another
// name and a symbolic link to one.
func osvConfigTree(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, relative := range []string{
		"osv-scanner.toml",
		"site/osv-scanner.toml",
		".github/requirements/osv-scanner.toml",
		"cmd/tool/testdata/osv-scanner.toml",
		"site/node_modules/a/osv-scanner.toml",
		"dist/osv-scanner.toml",
		"plan/osv-scanner.toml",
		".claude/worktrees/other/osv-scanner.toml",
		".cache/osv-scanner.toml",
		"scratch/osv-scanner.toml",
		"site/osv-scanner.toml.bak",
	} {
		writeFile(t, root, relative, "")
	}
	writeFile(t, root, "scratch/.git", "gitdir: /elsewhere\n")
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "osv-scanner.toml"), filepath.Join(root, "docs", "osv-scanner.toml")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	return root
}

// TestCheckDependencyScan_AllowList_IsJudgedWithTheRest verifies that the rule
// reads the Makefile, ci.yml and every allow list of the tree together, so a
// finding in an allow list reaches the audit's verdict.
func TestCheckDependencyScan_AllowList_IsJudgedWithTheRest(t *testing.T) {
	t.Parallel()

	root := brokenRepository(t)
	writeFile(t, root, "site/osv-scanner.toml", "[[IgnoredVulns]]\nid = \"MAL-2025-20690\"\nignoreUntil = 2026-11-30\n"+
		"reason = \"component_not_present: never installed\"\n")
	got, err := checkDependencyScan(root)
	if err != nil {
		t.Fatalf("checkDependencyScan: %v", err)
	}
	want := "site/osv-scanner.toml:2: MAL-2025-20690 is a malicious-package advisory, which is never set aside: the package is removed and the lockfile regenerated"
	if len(got) != 1 || got[0] != want {
		t.Errorf("checkDependencyScan() = %#v, want [%q]", got, want)
	}
}

// TestCheckDependencyScan_UnreadableInput_IsAnError verifies that an input the
// rule cannot read stops the audit rather than passing it: the workflows, the
// Makefile and an allow list the walk found.
func TestCheckDependencyScan_UnreadableInput_IsAnError(t *testing.T) {
	t.Parallel()

	noWorkflows := t.TempDir()
	noMakefile := brokenRepository(t)
	if err := os.Remove(filepath.Join(noMakefile, "Makefile")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	cases := []struct {
		name string
		root string
	}{
		{name: "no workflows", root: noWorkflows},
		{name: "no Makefile", root: noMakefile},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := checkDependencyScan(testCase.root); err == nil {
				t.Error("checkDependencyScan() = nil error, want the read failure")
			}
		})
	}
}

// TestCheckDependencyScan_UnreadableAllowList_IsAnError verifies that an allow
// list the walk found and could not read stops the audit: what it would have
// set aside is unknown, so passing it would be a guess. It swaps the package's
// read function, so it does not run in parallel.
func TestCheckDependencyScan_UnreadableAllowList_IsAnError(t *testing.T) {
	previous := readConfig
	t.Cleanup(func() { readConfig = previous })
	readConfig = func(string) (string, error) { return "", errors.New("refused") }

	root := brokenRepository(t)
	writeFile(t, root, "osv-scanner.toml", "")
	if _, err := checkDependencyScan(root); err == nil || err.Error() != "refused" {
		t.Errorf("checkDependencyScan() error = %v, want the read failure", err)
	}
}
