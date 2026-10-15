package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// releaseTestTable is the table every -check-release case is measured
// against unless it says otherwise, shaped like the one the build carries.
var releaseTestTable = &finegrained.Table{Version: "19.4.1-ee", Bucket: "19.4"}

// TestRunCheckRelease_EachVerdict_TakesItsStatusAndStream holds the mode
// run-docker-e2e.sh asks before go test to what the issue settled: the
// table's release, the prerelease right after it and an older release go
// ahead with exit 0 and one line on stdout; a newer release stops with a
// finding (exit 1) naming both releases and the steps; a version that does
// not parse stops as a run the gate could not judge (exit 2); and a mode
// handed no version at all is a usage error that names the flag.
func TestRunCheckRelease_EachVerdict_TakesItsStatusAndStream(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		want       int
		wantStdout []string
		wantStderr []string
	}{
		{name: "the table's release", version: "19.4.1-ee", want: exitOK, wantStdout: []string{"release: GitLab 19.4.1-ee", "(phase B)"}},
		{name: "the prerelease right after", version: "19.5.0-pre", want: exitOK, wantStdout: []string{"release: GitLab 19.5.0-pre", "the listing alone"}},
		{name: "an older release", version: "19.3.2", want: exitOK, wantStdout: []string{"release: GitLab 19.3.2 is older", "phase A"}},
		{
			name: "a newer release", version: "19.5.0-ee", want: exitFindings,
			wantStderr: []string{"release: GitLab 19.5.0-ee is newer", "19.4.1-ee", "make gen-api-live", "make gen-action-grants", "start the run again"},
		},
		{name: "a version that does not parse", version: "unknown", want: exitUsage, wantStderr: []string{`release: GitLab reported the version "unknown"`}},
		{name: "no version", version: "", want: exitUsage, wantStderr: []string{"-check-release needs -gitlab-version"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runFixture(t, options{
				dir: t.TempDir(), checkRelease: true, gitlabVersion: tc.version, releaseTable: releaseTestTable,
			})
			if code != tc.want {
				t.Errorf("run(-check-release %q) = %d, want %d; stderr %q", tc.version, code, tc.want, stderr)
			}
			assertStream(t, "stdout", stdout, tc.wantStdout)
			assertStream(t, "stderr", stderr, tc.wantStderr)
			if tc.want == exitOK && strings.Count(stdout, "\n") != 1 {
				t.Errorf("a release the run goes ahead on prints one line, and printed:\n%s", stdout)
			}
		})
	}
}

// TestRunCheckRelease_CompiledTable_IsTheDefault checks that the mode measures
// against the table the build carries when no test hands it another, which is
// the table the binary under test judges a grant by.
func TestRunCheckRelease_CompiledTable_IsTheDefault(t *testing.T) {
	compiled := actiongrants.Table()
	code, stdout, stderr := runFixture(t, options{dir: t.TempDir(), checkRelease: true, gitlabVersion: compiled.Version})
	if code != exitOK || !strings.Contains(stdout, "("+compiled.Version+")") {
		t.Errorf("run(-check-release %s) = %d, stdout %q, stderr %q; want exit 0 naming the compiled table's release",
			compiled.Version, code, stdout, stderr)
	}
}

// TestRun_CheckRelease_IsAModeOfItsOwn verifies that -check-release passes the
// "nothing to do" guard with no -calls, since the version is all it reads.
func TestRun_CheckRelease_IsAModeOfItsOwn(t *testing.T) {
	code, _, stderr := runFixture(t, options{
		dir: t.TempDir(), checkRelease: true, gitlabVersion: "19.5.0-ee", releaseTable: releaseTestTable,
	})
	if code != exitFindings || strings.Contains(stderr, "nothing to do") {
		t.Errorf("run(-check-release) = %d, stderr %q; want %d from the gate itself", code, stderr, exitFindings)
	}
}

// assertStream fails when one of the wanted fragments is missing from a
// stream, or when a stream that should be empty is not.
func assertStream(t *testing.T, name, got string, want []string) {
	t.Helper()
	if len(want) == 0 && got != "" {
		t.Errorf("%s = %q, want nothing", name, got)
	}
	for _, fragment := range want {
		if !strings.Contains(got, fragment) {
			t.Errorf("%s does not mention %q:\n%s", name, fragment, got)
		}
	}
}
