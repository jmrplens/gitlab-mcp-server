// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// TestRunMain_Arguments_PrintTheMatrixOrRefuse verifies every outcome of a
// run: a flag it does not define and an argument are usage errors, a listing
// that failed, a module with no target and two targets that would share an
// artifact are each refused with the reason, and a module with targets gets
// the matrix on stdout, in the order found, and one line per target with the
// count on stderr. The listing is asked in the directory -dir names.
func TestRunMain_Arguments_PrintTheMatrixOrRefuse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"a/a_test.go":    "package a\n\nimport \"testing\"\n\nfunc FuzzOne(f *testing.F) {}\n\nfunc FuzzTwo(f *testing.F) {}\n",
		"b/b_test.go":    "package b\n\nimport \"testing\"\n\nfunc FuzzThree(f *testing.F) {}\n",
		"a-b/ab_test.go": "package ab\n\nimport \"testing\"\n\nfunc FuzzOne(f *testing.F) {}\n",
		"a/b/b_test.go":  "package b\n\nimport \"testing\"\n\nfunc FuzzOne(f *testing.F) {}\n",
		"none/n_test.go": "package none\n\nimport \"testing\"\n\nfunc TestN(t *testing.T) {}\n",
	})
	const module = "example.com/m"
	found := row(module+"/a", module, filepath.Join(root, "a"), "a_test.go", "") +
		row(module+"/b", module, filepath.Join(root, "b"), "", "b_test.go")
	cases := []struct {
		name       string
		args       []string
		listing    string
		err        error
		wantCode   int
		wantStdout string
		wantStderr string
		wantDir    string
	}{
		{name: "an unknown flag", args: []string{"-packages", "x"}, wantCode: 2, wantStderr: "flag provided but not defined: -packages\n"},
		{name: "an argument", args: []string{"extra"}, wantCode: 2, wantStderr: "fuzz_targets: takes no arguments, and was given [\"extra\"]\n"},
		{
			name: "a listing that failed", err: errors.New("planted"), wantCode: 1, wantDir: ".",
			wantStderr: "fuzz_targets: go list: planted\n",
		},
		{
			name: "no target", args: []string{"-dir", "elsewhere"}, wantDir: "elsewhere", wantCode: 1,
			listing:    row(module+"/none", module, filepath.Join(root, "none"), "n_test.go", ""),
			wantStderr: "fuzz_targets: no fuzz target found in elsewhere, so the workflow would fuzz nothing and pass\n",
		},
		{
			name: "two targets on one artifact", wantDir: ".", wantCode: 1,
			listing:    row(module+"/a-b", module, filepath.Join(root, "a-b"), "ab_test.go", "") + row(module+"/a/b", module, filepath.Join(root, "a", "b"), "b_test.go", ""),
			wantStderr: "fuzz_targets: FuzzOne in ./a-b and FuzzOne in ./a/b would both upload as fuzz-a-b-FuzzOne\n",
		},
		{
			name: "targets", wantDir: ".", listing: found,
			wantStdout: `matrix={"include":[` +
				`{"package":"./a","target":"FuzzOne","artifact":"fuzz-a-FuzzOne"},` +
				`{"package":"./a","target":"FuzzTwo","artifact":"fuzz-a-FuzzTwo"},` +
				`{"package":"./b","target":"FuzzThree","artifact":"fuzz-b-FuzzThree"}]}` + "\n",
			wantStderr: "fuzz_targets: ./a FuzzOne\nfuzz_targets: ./a FuzzTwo\nfuzz_targets: ./b FuzzThree\n" +
				"fuzz_targets: 3 fuzz target(s) in 2 package(s)\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []listCall
			var stdout, stderr bytes.Buffer
			if got := runMain(tc.args, &stdout, &stderr, fakeLister(tc.listing, tc.err, &calls)); got != tc.wantCode {
				t.Fatalf("runMain(%q) = %d, want %d; stderr:\n%s", tc.args, got, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if !strings.HasPrefix(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to start %q", stderr.String(), tc.wantStderr)
			}
			if tc.wantDir == "" && len(calls) != 0 || tc.wantDir != "" && (len(calls) != 1 || calls[0].dir != tc.wantDir) {
				t.Errorf("lister calls = %+v, want one in %q", calls, tc.wantDir)
			}
		})
	}
}

// TestMain_UnknownFlag_ExitsWithTheUsageStatus verifies that main hands the
// process arguments after its own name to runMain and passes the status on to
// the exit seam, without listing anything for a usage error.
func TestMain_UnknownFlag_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = previousArgs, previousExit })
	got := -1
	os.Args = []string{"fuzz_targets", "-no-such-flag"}
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// workflowStep is one step of a workflow job, as the contract below reads it.
type workflowStep struct {
	Name string            `yaml:"name"`
	ID   string            `yaml:"id"`
	If   string            `yaml:"if"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]any    `yaml:"with"`
}

// fuzzWorkflow is .github/workflows/fuzz.yml as the contract below reads it,
// with every named step keyed by its job and its name.
type fuzzWorkflow struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions any               `yaml:"permissions"`
		Needs       any               `yaml:"needs"`
		Outputs     map[string]string `yaml:"outputs"`
		Strategy    struct {
			FailFast *bool `yaml:"fail-fast"`
			Matrix   any   `yaml:"matrix"`
		} `yaml:"strategy"`
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
	steps map[string]workflowStep
}

// loadFuzzWorkflow reads and parses the workflow this command plans.
func loadFuzzWorkflow(t *testing.T) fuzzWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "fuzz.yml"))
	if err != nil {
		t.Fatalf("read the workflow: %v", err)
	}
	var workflow fuzzWorkflow
	if err = yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse the workflow: %v", err)
	}
	workflow.steps = map[string]workflowStep{}
	for name, job := range workflow.Jobs {
		for _, step := range job.Steps {
			workflow.steps[name+"/"+step.Name] = step
		}
	}
	return workflow
}

// TestFuzzWorkflow_Triggers_AreTheScheduleADispatchAndItsOwnChanges holds the
// workflow to when it runs: on a weekly schedule, when dispatched, and on a
// pull request that changes the workflow or this command and nothing else,
// since GitHub offers a workflow for dispatch only once it is on the default
// branch and the change would otherwise merge unrun. The token reads and
// nothing more, and no job raises it for itself.
func TestFuzzWorkflow_Triggers_AreTheScheduleADispatchAndItsOwnChanges(t *testing.T) {
	t.Parallel()
	workflow := loadFuzzWorkflow(t)
	triggers := slices.Sorted(maps.Keys(workflow.On))
	if !reflect.DeepEqual(triggers, []string{"pull_request", "schedule", "workflow_dispatch"}) {
		t.Errorf("on = %v, want pull_request, schedule and workflow_dispatch", triggers)
	}
	pullRequest, _ := workflow.On["pull_request"].(map[string]any)
	if len(pullRequest) != 1 || !reflect.DeepEqual(pullRequest["paths"], []any{".github/workflows/fuzz.yml", "cmd/fuzz_targets/**"}) {
		t.Errorf("on.pull_request = %v, want paths naming the workflow and this command alone", workflow.On["pull_request"])
	}
	if !reflect.DeepEqual(workflow.Permissions, map[string]string{"contents": "read"}) {
		t.Errorf("permissions = %v, want contents: read alone", workflow.Permissions)
	}
	if len(workflow.Jobs) != 2 {
		t.Errorf("jobs = %d, want plan and fuzz", len(workflow.Jobs))
	}
	for name, job := range workflow.Jobs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if job.Permissions != nil {
				t.Errorf("job %s sets permissions of its own: %v", name, job.Permissions)
			}
		})
	}
}

// TestFuzzWorkflow_Jobs_RunThisCommandsPlan holds the workflow to the matrix
// this command prints: the plan step writes it into $GITHUB_OUTPUT, the fuzz
// job fans out over it without stopping at the first failure, and every
// matrix field the jobs read is one a cell carries.
func TestFuzzWorkflow_Jobs_RunThisCommandsPlan(t *testing.T) {
	t.Parallel()
	workflow := loadFuzzWorkflow(t)
	plan := workflow.steps["plan/List the fuzz targets"]
	if plan.ID != "plan" || !strings.HasPrefix(plan.Run, "go run ./cmd/fuzz_targets ") || !strings.HasSuffix(strings.TrimSpace(plan.Run), `>> "$GITHUB_OUTPUT"`) {
		t.Errorf("the plan step has id %q and runs %q", plan.ID, plan.Run)
	}
	if got := workflow.Jobs["plan"].Outputs["matrix"]; got != "${{ steps.plan.outputs.matrix }}" {
		t.Errorf("the plan job's matrix output is %q", got)
	}
	fuzz := workflow.Jobs["fuzz"]
	if fuzz.Needs != "plan" || fuzz.Strategy.Matrix != "${{ fromJSON(needs.plan.outputs.matrix) }}" || fuzz.Strategy.FailFast == nil || *fuzz.Strategy.FailFast {
		t.Errorf("the fuzz job needs %v and fans out over %v with fail-fast %v", fuzz.Needs, fuzz.Strategy.Matrix, fuzz.Strategy.FailFast)
	}
	encoded, err := json.Marshal(cell{})
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"package", "target", "artifact"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			if _, carried := fields[field]; !carried {
				t.Errorf("a cell carries %v, not matrix.%s", fields, field)
			}
		})
	}
}

// TestFuzzWorkflow_FuzzStep_FuzzesTheCellsTargetAndKeepsWhatFailed holds the
// fuzzing to the cell: the one target named, anchored so it matches no other,
// in the one package named, with no other test run beside it, for ten minutes
// on the schedule and a dispatch and one on a pull request; and on a failure
// the directory the go command writes a failing input into is uploaded under
// the cell's artifact name.
func TestFuzzWorkflow_FuzzStep_FuzzesTheCellsTargetAndKeepsWhatFailed(t *testing.T) {
	t.Parallel()
	workflow := loadFuzzWorkflow(t)
	fuzz := workflow.steps["fuzz/Fuzz"]
	wantEnv := map[string]string{
		"PACKAGE":  "${{ matrix.package }}",
		"TARGET":   "${{ matrix.target }}",
		"FUZZTIME": "${{ github.event_name == 'pull_request' && '1m' || '10m' }}",
	}
	if !reflect.DeepEqual(fuzz.Env, wantEnv) {
		t.Errorf("the fuzz step's env = %v, want %v", fuzz.Env, wantEnv)
	}
	if want := `go test -run '^$' -fuzz "^${TARGET}\$" -fuzztime "$FUZZTIME" "$PACKAGE"`; strings.TrimSpace(fuzz.Run) != want {
		t.Errorf("the fuzz step runs %q, want %q", fuzz.Run, want)
	}
	upload := workflow.steps["fuzz/Upload the failing input"]
	if upload.If != "failure()" || upload.With["name"] != "${{ matrix.artifact }}" ||
		upload.With["path"] != "${{ matrix.package }}/testdata/fuzz/${{ matrix.target }}" {
		t.Errorf("the upload step runs if %q and takes %v", upload.If, upload.With)
	}
}
