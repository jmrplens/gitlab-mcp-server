// main_test.go drives the command end to end: the reports and the database
// read from disk, the verdict reached and the exit code the gate turns on, and
// holds the platforms it expects to the release workflow.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// currentDatabase writes what trivy version prints for a database fetched an
// hour before the test runs, whose next update is still a day away.
func currentDatabase(t *testing.T) string {
	t.Helper()

	db := fixtureDatabase()
	vulnDB := db["VulnerabilityDB"].(map[string]any)
	vulnDB["UpdatedAt"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	vulnDB["NextUpdate"] = time.Now().Add(23 * time.Hour).UTC().Format(time.RFC3339Nano)
	return writeJSON(t, db)
}

// TestRunMain_TheReleasedImagePasses runs the command on what Trivy 0.76.1
// reported for the published 3.3.0 image: two fixable advisories against its
// Alpine packages, one rated MEDIUM and one by no source, which the rule
// reports and does not gate on, and a HIGH one against the server binary,
// which make check-binary-vulns judges.
func TestRunMain_TheReleasedImagePasses(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := runMain([]string{
		"-db", currentDatabase(t),
		filepath.Join("testdata", "release-3.3.0-amd64.json"),
		filepath.Join("testdata", "release-3.3.0-arm64.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	var missing []string
	for _, want := range []string{
		"audit_image_vulns: linux/arm64: alpine 3.24.2, 18 OS packages, in ghcr.io/jmrplens/gitlab-mcp-server:3.3.0, " +
			"image sha256:a0ebc7ea54e81683c6117fb100ad72a26fefd8b99847a6ba643b86917d1f07a5, scanned by Trivy 0.76.1\n",
		"reported CVE-2026-85091 zlib 1.3.2-r0, fixed in 1.3.2-r1, MEDIUM from other vendors (ubuntu), in linux/amd64, linux/arm64\n",
		"reported CVE-2026-107891 musl-utils 1.2.6-r2, fixed in 1.2.6-r3, UNKNOWN from no source, in linux/amd64, linux/arm64\n",
		"binary CVE-2026-111943 stdlib v1.27.2, fixed in 1.26.10, 1.27.3, HIGH from other vendors (alma, amazon, oracle-oval, redhat, rocky), in linux/amd64, linux/arm64: make check-binary-vulns judges the binary\n",
		"audit_image_vulns: no fixable HIGH or CRITICAL advisory affects an OS package of the 2 images\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			missing = append(missing, want)
		}
	}
	if len(missing) != 0 {
		t.Errorf("the report lacks %q:\n%s", missing, stdout.String())
	}
}

// TestRunMain_AnUndeclaredFindingFailsTheGate is the gate doing its job end
// to end: a fixable HIGH advisory against an OS package of both images fails
// the run, and the report names it once for both.
func TestRunMain_AnUndeclaredFindingFailsTheGate(t *testing.T) {
	t.Parallel()

	high := []fixtureVuln{{ID: "CVE-2026-1001", Pkg: "zlib", Installed: "1.3.2-r0", Fixed: "1.3.2-r1", Severity: "HIGH", Source: "nvd", Title: "a heap overflow"}}
	var stdout, stderr bytes.Buffer
	code := runMain([]string{
		"-db", currentDatabase(t),
		writeJSON(t, fixtureReport("amd64", high, nil)),
		writeJSON(t, fixtureReport("arm64", high, nil)),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if want := "FINDING CVE-2026-1001 zlib 1.3.2-r0, fixed in 1.3.2-r1, HIGH from nvd, in linux/amd64, linux/arm64: a heap overflow\n"; !strings.Contains(stdout.String(), want) {
		t.Errorf("the report lacks %q:\n%s", want, stdout.String())
	}
}

// TestRun_ARunThatCannotBeMade_FailsOnStderr covers each input failing: the
// database, a report, and the set of reports against the platforms. A gate
// that could not judge the images must not read as one that passed, so each
// says why on stderr and prints no verdict.
func TestRun_ARunThatCannotBeMade_FailsOnStderr(t *testing.T) {
	t.Parallel()

	amd64 := writeJSON(t, fixtureReport("amd64", nil, nil))
	arm64 := writeJSON(t, fixtureReport("arm64", nil, nil))
	db := currentDatabase(t)
	platforms := []string{"linux/amd64", "linux/arm64"}
	for _, tc := range []struct {
		name string
		want string
		cfg  auditConfig
	}{
		{name: "database", cfg: auditConfig{db: filepath.Join(t.TempDir(), "absent.json"), reports: []string{amd64, arm64}, platforms: platforms}, want: "absent.json"},
		{name: "report", cfg: auditConfig{db: db, reports: []string{amd64, filepath.Join(t.TempDir(), "gone.json")}, platforms: platforms}, want: "gone.json"},
		{name: "platforms", cfg: auditConfig{db: db, reports: []string{amd64}, platforms: platforms}, want: "no report scans linux/arm64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.cfg.now = time.Now()
			var stdout, stderr bytes.Buffer
			if code := run(tc.cfg, &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.HasPrefix(stderr.String(), toolName+": ") || !strings.Contains(stderr.String(), tc.want) || stdout.Len() != 0 {
				t.Errorf("stderr %q lacks %q, or stdout carried a verdict: %q", stderr.String(), tc.want, stdout.String())
			}
		})
	}
}

// TestRunMain_ArgumentsItCannotRead covers the command line: -h is not an
// error, and an unknown flag, no report, no database and a platform list that
// does not name each platform once as os/arch are usage errors rather than
// runs.
func TestRunMain_ArgumentsItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		stderr string
		args   []string
		want   int
	}{
		{name: "help", args: []string{"-h"}, want: 0},
		{name: "unknown flag", args: []string{"-nope"}, want: 2},
		{name: "no report", args: []string{"-db", "db.json"}, want: 2, stderr: "name the Trivy report of each platform"},
		{name: "no database", args: []string{"report.json"}, want: 2, stderr: "-db names the output of trivy version --format json"},
		{name: "no platform", args: []string{"-db", "db.json", "-platforms", "", "report.json"}, want: 2, stderr: `-platforms "" does not name each platform once as os/arch`},
		{name: "no architecture", args: []string{"-db", "db.json", "-platforms", "linux", "r.json"}, want: 2, stderr: `-platforms "linux"`},
		{name: "no os", args: []string{"-db", "db.json", "-platforms", "/amd64", "r.json"}, want: 2, stderr: `-platforms "/amd64"`},
		{name: "empty entry", args: []string{"-db", "db.json", "-platforms", "linux/amd64,,linux/arm64", "r.json"}, want: 2, stderr: `-platforms "linux/amd64,,linux/arm64"`},
		{name: "twice", args: []string{"-db", "db.json", "-platforms", "linux/amd64,linux/amd64", "r.json"}, want: 2, stderr: `-platforms "linux/amd64,linux/amd64"`},
		{name: "spaced", args: []string{"-db", "db.json", "-platforms", "linux/amd64, linux/arm64", "r.json"}, want: 2, stderr: `-platforms "linux/amd64, linux/arm64"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := runMain(tc.args, &stdout, &stderr); code != tc.want {
				t.Errorf("runMain(%q) = %d, want %d", tc.args, code, tc.want)
			}
			if !strings.Contains(stderr.String(), tc.stderr) {
				t.Errorf("stderr %q lacks %q", stderr.String(), tc.stderr)
			}
		})
	}
}

// TestRunMain_NamesThePlatformsItHoldsTheReportsTo covers -platforms read: a
// run told to expect one platform passes on that platform's report alone.
func TestRunMain_NamesThePlatformsItHoldsTheReportsTo(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := runMain([]string{"-platforms", "linux/arm64", "-db", currentDatabase(t), writeJSON(t, fixtureReport("arm64", nil, nil))}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main itself through the exit
// seam, with a database that does not exist so it stops before judging.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousArgs, previousExit := os.Args, exitProcess
	t.Cleanup(func() { os.Args, exitProcess = previousArgs, previousExit })

	os.Args = []string{toolName, "-db", filepath.Join(t.TempDir(), "absent.json"), filepath.Join("testdata", "release-3.3.0-amd64.json")}
	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 1 {
		t.Fatalf("main exited %d, want 1 for a database that does not exist", code)
	}
}

// repositoryFile reads a file of the repository this command lives in.
func repositoryFile(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// TestDefaultPlatforms_AreTheOnesTheReleasePushes holds the platforms the
// command expects a report for, by default, to the multi-platform build the
// release workflow pushes, so a platform added to the push is a platform the
// gate refuses to pass unscanned rather than one it never hears of.
func TestDefaultPlatforms_AreTheOnesTheReleasePushes(t *testing.T) {
	t.Parallel()

	var pushed []string
	for _, m := range regexp.MustCompile(`(?m)^\s*platforms:\s*(\S+)\s*$`).FindAllStringSubmatch(repositoryFile(t, ".github/workflows/release.yml"), -1) {
		if strings.Contains(m[1], ",") {
			pushed = append(pushed, m[1])
		}
	}
	if !slices.Equal(pushed, []string{defaultPlatforms}) {
		t.Fatalf("release.yml pushes %q, and the command expects a report for %q by default", pushed, defaultPlatforms)
	}
}
