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

// treeLister is a lister that answers the tree's listing with listing, or
// fails with err.
func treeLister(listing string, err error) lister {
	return func(context.Context, string, []string, ...string) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		return []byte(listing), nil
	}
}

// TestRunPlan_Dispatches_PrintTheMatrixOrRefuse verifies the plan end to end:
// the matrix line it prints for $GITHUB_OUTPUT, what it says on stderr, and
// its status, for every input it accepts or refuses, for the three ways the
// discovery of an empty package list can end, and for a -tree plan, which
// refuses a package list beside it and the two ways its listing can come back
// with nothing to plan.
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
				`{"system":"windows-latest","name":"./cmd/server","packages":"./cmd/server","artifact":"gobco-windows-latest-cmd-server"},` +
				`{"system":"ubuntu-latest","name":"./cmd/server","packages":"./cmd/server","artifact":"gobco-ubuntu-latest-cmd-server"},` +
				`{"system":"windows-latest","name":"./internal/toolutil","packages":"./internal/toolutil","artifact":"gobco-windows-latest-internal-toolutil"},` +
				`{"system":"ubuntu-latest","name":"./internal/toolutil","packages":"./internal/toolutil","artifact":"gobco-ubuntu-latest-internal-toolutil"}]}` + "\n",
			wantStderr: "measure_conditions: 4 run(s) planned, ./cmd/server ./internal/toolutil on windows-latest ubuntu-latest, with GOBCO_GATE=beyond:linux/amd64\n",
		},
		{
			name:       "no package named: the constrained ones are found",
			args:       []string{"-systems", "macos-latest", "-gate", "all"},
			list:       discovered,
			wantStdout: `matrix={"include":[{"system":"macos-latest","name":"./cmd/server","packages":"./cmd/server","artifact":"gobco-macos-latest-cmd-server"}]}` + "\n",
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
		{
			name: "the tree, a shard per package while there are fewer than the shards",
			args: []string{"-tree", "-systems", "ubuntu-latest windows-latest", "-gate", "all"},
			list: treeLister(row(module+"/cmd/a", module, "a.go", "", "", "")+row(module+"/internal/b", module, "b.go", "", "", ""), nil),
			wantStdout: `matrix={"include":[` +
				`{"system":"windows-latest","name":"tree 01 of 02","packages":"./cmd/a","artifact":"gobco-windows-latest-tree-01"},` +
				`{"system":"ubuntu-latest","name":"tree 01 of 02","packages":"./cmd/a","artifact":"gobco-ubuntu-latest-tree-01"},` +
				`{"system":"windows-latest","name":"tree 02 of 02","packages":"./internal/b","artifact":"gobco-windows-latest-tree-02"},` +
				`{"system":"ubuntu-latest","name":"tree 02 of 02","packages":"./internal/b","artifact":"gobco-ubuntu-latest-tree-02"}]}` + "\n",
			wantStderr: "measure_conditions: 4 run(s) planned, the 2 packages below cmd/ and internal/ in 2 shard(s) on windows-latest ubuntu-latest, with GOBCO_GATE=all\n",
		},
		{
			name: "the tree and packages besides", args: []string{"-tree", "-packages", "./cmd/server", "-systems", "ubuntu-latest", "-gate", "all"}, list: broken, wantCode: 2,
			wantStderr: "measure_conditions: -tree measures every package below cmd/ and internal/, and -packages names ./cmd/server besides; leave -packages empty\n",
		},
		{
			name: "the tree, and the listing failed", args: []string{"-tree", "-systems", "ubuntu-latest", "-gate", "all"}, list: treeLister("", errors.New("planted failure")), wantCode: 1,
			wantStderr: "measure_conditions: -tree: the packages below cmd/ and internal/ could not be listed: go list: planted failure\n",
		},
		{
			name: "the tree, and nothing below cmd/ and internal/", args: []string{"-tree", "-systems", "ubuntu-latest", "-gate", "all"}, list: treeLister("", nil), wantCode: 1,
			wantStderr: "measure_conditions: -tree: there is no package below cmd/ and internal/, so there is nothing to measure\n",
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

// TestBuildShards_Packages_AreDealtRoundTheShards verifies how a -tree plan
// splits the packages: dealt one at a time round the shards in the order
// given, so neighbors land in different jobs, each shard on every system
// with the same name and members, and never more shards than packages.
func TestBuildShards_Packages_AreDealtRoundTheShards(t *testing.T) {
	t.Parallel()
	got := buildShards([]string{"./a", "./b", "./c", "./d", "./e"}, []string{"macos-latest", "ubuntu-latest"}, 2)
	want := matrix{Include: []cell{
		{System: "macos-latest", Name: "tree 01 of 02", Packages: "./a ./c ./e", Artifact: "gobco-macos-latest-tree-01"},
		{System: "ubuntu-latest", Name: "tree 01 of 02", Packages: "./a ./c ./e", Artifact: "gobco-ubuntu-latest-tree-01"},
		{System: "macos-latest", Name: "tree 02 of 02", Packages: "./b ./d", Artifact: "gobco-macos-latest-tree-02"},
		{System: "ubuntu-latest", Name: "tree 02 of 02", Packages: "./b ./d", Artifact: "gobco-ubuntu-latest-tree-02"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildShards() =\n%+v\nwant\n%+v", got, want)
	}
	if fewer := buildShards([]string{"./a"}, []string{"macos-latest"}, 20); len(fewer.Include) != 1 || fewer.Include[0].Name != "tree 01 of 01" {
		t.Errorf("buildShards(one package, twenty shards) = %+v, want one shard of one", fewer)
	}
}

// TestRunPlan_TreeDirFlag_IsWhereTheTreeIsListed verifies that -dir reaches
// the tree's listing too, and that the listing is asked for the packages
// below cmd/ and internal/ under the platform at hand, with no environment of
// its own.
func TestRunPlan_TreeDirFlag_IsWhereTheTreeIsListed(t *testing.T) {
	t.Parallel()
	var calls []listCall
	list := func(_ context.Context, dir string, env []string, args ...string) ([]byte, error) {
		calls = append(calls, listCall{dir: dir, env: env, args: args})
		return nil, errors.New("stop")
	}
	var stdout, stderr bytes.Buffer
	if got := runPlan([]string{"-tree", "-dir", "elsewhere", "-systems", "macos-latest", "-gate", "all"}, &stdout, &stderr, list); got != 1 {
		t.Fatalf("runPlan() = %d, want 1", got)
	}
	want := []listCall{{dir: "elsewhere", args: []string{"list", "-f", listFormat, "./cmd/...", "./internal/..."}}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("the listing calls were %+v, want %+v", calls, want)
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
