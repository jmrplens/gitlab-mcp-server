package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// answeringLister is a lister that names constrained packages by answering
// every target with the listing its own function writes, or fails.
func answeringLister(listing func(target string) string, err error) lister {
	return func(_ context.Context, _ string, env []string, _ ...string) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		return []byte(listing(strings.TrimPrefix(env[0], "GOOS=") + "/" + strings.TrimPrefix(env[1], "GOARCH="))), nil
	}
}

// TestRunPlan_Dispatches_PrintTheMatrixOrRefuse verifies the plan end to end:
// the matrix line it prints for $GITHUB_OUTPUT, what it says on stderr, and
// its status, for every input it accepts or refuses and for the three ways the
// discovery of an empty package list can end.
func TestRunPlan_Dispatches_PrintTheMatrixOrRefuse(t *testing.T) {
	t.Parallel()
	const module = "example.com/m"
	discovered := answeringLister(func(target string) string {
		files := "s.go"
		if strings.HasPrefix(target, "windows/") {
			files = "s.go s_windows.go"
		}
		return row(module+"/cmd/server", module, files, "", "", "") + row(module+"/plain", module, "p.go", "", "", "")
	}, nil)
	nothing := answeringLister(func(string) string { return row(module+"/plain", module, "p.go", "", "", "") }, nil)
	broken := answeringLister(nil, errors.New("planted failure"))
	cases := []struct {
		name       string
		args       []string
		list       lister
		wantCode   int
		wantStdout string
		wantStderr string
		// stderrPrefix accepts more after wantStderr, which is the usage
		// the flag package prints after its own message.
		stderrPrefix bool
	}{
		{
			name: "packages and systems named, in any order and spelling",
			args: []string{"-packages", "cmd/server, ./internal/toolutil ./cmd/server", "-systems", "ubuntu-latest,windows-latest ubuntu-latest", "-gate", "beyond:linux/amd64"},
			list: broken,
			wantStdout: `matrix={"include":[` +
				`{"system":"windows-latest","package":"./cmd/server","artifact":"gobco-windows-latest-cmd-server"},` +
				`{"system":"ubuntu-latest","package":"./cmd/server","artifact":"gobco-ubuntu-latest-cmd-server"},` +
				`{"system":"windows-latest","package":"./internal/toolutil","artifact":"gobco-windows-latest-internal-toolutil"},` +
				`{"system":"ubuntu-latest","package":"./internal/toolutil","artifact":"gobco-ubuntu-latest-internal-toolutil"}]}` + "\n",
			wantStderr: "measure_conditions: 4 run(s) planned, ./cmd/server ./internal/toolutil on windows-latest ubuntu-latest, with GOBCO_GATE=beyond:linux/amd64\n",
		},
		{
			name:       "no package named: the constrained ones are found",
			args:       []string{"-systems", "macos-latest", "-gate", "all"},
			list:       discovered,
			wantStdout: `matrix={"include":[{"system":"macos-latest","package":"./cmd/server","artifact":"gobco-macos-latest-cmd-server"}]}` + "\n",
			wantStderr: "measure_conditions: no package named; measuring the 1 whose built files differ between release targets: ./cmd/server\n" +
				"measure_conditions: 1 run(s) planned, ./cmd/server on macos-latest, with GOBCO_GATE=all\n",
		},
		{
			name: "no package named and none found", args: []string{"-systems", "macos-latest", "-gate", "all"}, list: nothing, wantCode: 1,
			wantStderr: "measure_conditions: no package named, and no package builds different files on two of the release targets (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64), so there is nothing to measure\n",
		},
		{
			name: "no package named and the search failed", args: []string{"-systems", "macos-latest", "-gate", "all"}, list: broken, wantCode: 1,
			wantStderr: "measure_conditions: no package named, and the packages carrying a GOOS- or GOARCH-constrained file could not be found: go list for linux/amd64: planted failure\n",
		},
		{
			name: "a flag it does not know", args: []string{"-gates", "all"}, list: broken, wantCode: 2,
			wantStderr: "flag provided but not defined: -gates\n", stderrPrefix: true,
		},
		{
			name: "an argument", args: []string{"-gate", "all", "-systems", "macos-latest", "./cmd/server"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: plan takes no arguments, and was given [\"./cmd/server\"]\n",
		},
		{
			name: "no gate", args: []string{"-systems", "macos-latest"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -gate \"\" is neither all nor beyond:GOOS/GOARCH, which is what scripts/coverage-conditions.sh reads\n",
		},
		{
			name: "a gate the script would refuse", args: []string{"-systems", "macos-latest", "-gate", "beyond:linux"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -gate \"beyond:linux\" is neither all nor beyond:GOOS/GOARCH, which is what scripts/coverage-conditions.sh reads\n",
		},
		{
			name: "a gate with more after it", args: []string{"-systems", "macos-latest", "-gate", "all,"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -gate \"all,\" is neither all nor beyond:GOOS/GOARCH, which is what scripts/coverage-conditions.sh reads\n",
		},
		{
			name: "a runner outside the three", args: []string{"-systems", "self-hosted", "-gate", "all"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -systems: \"self-hosted\" is not one of windows-latest, macos-latest, ubuntu-latest\n",
		},
		{
			name: "no runner", args: []string{"-systems", " , ", "-gate", "all"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -systems: name at least one of windows-latest, macos-latest, ubuntu-latest\n",
		},
		{
			name: "a pattern", args: []string{"-packages", "./internal/...", "-systems", "macos-latest", "-gate", "all"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -packages: \"./internal/...\" is not a package directory below the module root, such as ./cmd/server\n",
		},
		{
			name: "two names for one artifact", args: []string{"-packages", "./a-b/c ./a/b-c", "-systems", "macos-latest", "-gate", "all"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: ./a-b/c and ./a/b-c on macos-latest would both upload as gobco-macos-latest-a-b-c\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if got := runPlan(tc.args, &stdout, &stderr, tc.list); got != tc.wantCode {
				t.Fatalf("runPlan(%q) = %d, want %d; stderr:\n%s", tc.args, got, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			got := stderr.String()
			if tc.stderrPrefix {
				got = got[:min(len(got), len(tc.wantStderr))]
			}
			if got != tc.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

// TestRunPlan_DirFlag_IsWhereTheDiscoveryLooks verifies that -dir reaches the
// listing, so the workflow could point the plan at another checkout.
func TestRunPlan_DirFlag_IsWhereTheDiscoveryLooks(t *testing.T) {
	t.Parallel()
	var dirs []string
	list := func(_ context.Context, dir string, _ []string, _ ...string) ([]byte, error) {
		dirs = append(dirs, dir)
		return nil, errors.New("stop")
	}
	var stdout, stderr bytes.Buffer
	if got := runPlan([]string{"-dir", "elsewhere", "-systems", "macos-latest", "-gate", "all"}, &stdout, &stderr, list); got != 1 {
		t.Fatalf("runPlan() = %d, want 1", got)
	}
	if !reflect.DeepEqual(dirs, []string{"elsewhere"}) {
		t.Errorf("the listing ran in %q, want [elsewhere]", dirs)
	}
}

// TestParsePackages_Names_AreNormalizedOrRefused verifies the package reader:
// a name with or without its ./ is the same package, each is kept once in the
// order first named, and anything that is not a directory below the module
// root, in a character this module's directories do not use, is refused.
func TestParsePackages_Names_AreNormalizedOrRefused(t *testing.T) {
	t.Parallel()
	got, err := parsePackages("\tcmd/server,./internal/toolutil\n./cmd/server ./internal/tools/packages")
	want := []string{"./cmd/server", "./internal/toolutil", "./internal/tools/packages"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("parsePackages() = %q, %v; want %q", got, err, want)
	}
	if blank, blankErr := parsePackages(" "); blankErr != nil || blank != nil {
		t.Errorf("parsePackages(blank) = %q, %v; want nothing", blank, blankErr)
	}
	for _, refused := range []string{".", "./", "../x", "./cmd/../x", "/abs", "cmd//server", "cmd/server/", "cmd\\server", "cmd/$(id)", "cmd/server;x", "-x", ".hidden", "_x/.y"} {
		t.Run(refused, func(t *testing.T) {
			t.Parallel()
			if accepted, refusal := parsePackages(refused); refusal == nil {
				t.Errorf("parsePackages(%q) = %q, want a refusal", refused, accepted)
			}
		})
	}
}
