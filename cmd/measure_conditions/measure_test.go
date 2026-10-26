package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// scriptCall is one call a fake script runner received.
type scriptCall struct {
	dir string
	pkg string
	env []string
}

// fakeScript answers for coverageScript: each package prints the output and
// exits with the status its entry gives, and a package without one fails to
// run with err. It records every call, and is safe to call from several
// goroutines at once, as the tree's workers do.
type fakeScript struct {
	mu      sync.Mutex
	outputs map[string]string
	status  map[string]int
	err     error
	calls   []scriptCall
}

// run is the fake's scriptRunner.
func (f *fakeScript) run(_ context.Context, dir, pkg string, env []string, output io.Writer) (int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, scriptCall{dir: dir, pkg: pkg, env: env})
	f.mu.Unlock()
	status, known := f.status[pkg]
	if !known {
		return 0, f.err
	}
	_, err := io.WriteString(output, f.outputs[pkg])
	return status, err
}

// platform is the platform a record written by this test binary names.
var platform = runtime.GOOS + "/" + runtime.GOARCH

// readFile returns the content of a file the test expects to exist.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRunMeasure_Usage_IsRefusedWithStatus2 verifies every way the command
// line can be wrong: an unknown flag, an argument, each of the three flags
// left empty, and a package name the plan would refuse too.
func TestRunMeasure_Usage_IsRefusedWithStatus2(t *testing.T) {
	t.Parallel()
	full := func(overrides ...string) []string {
		values := map[string]string{"-packages": "./cmd/server", "-system": "s", "-records": "r"}
		for i := 0; i+1 < len(overrides); i += 2 {
			values[overrides[i]] = overrides[i+1]
		}
		return []string{"-packages", values["-packages"], "-system", values["-system"], "-records", values["-records"]}
	}
	const flagsWanted = "measure_conditions: measure takes -packages, -system and -records, all three non-empty, and no arguments\n"
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "an unknown flag", args: []string{"-package", "./a"}, wantStderr: "flag provided but not defined: -package\n"},
		{name: "an argument", args: append(full(), "extra"), wantStderr: flagsWanted},
		{name: "no package", args: full("-packages", " "), wantStderr: flagsWanted},
		{name: "no system", args: full("-system", ""), wantStderr: flagsWanted},
		{name: "no records", args: full("-records", ""), wantStderr: flagsWanted},
		{
			name: "a package outside the module", args: full("-packages", "../x"),
			wantStderr: "measure_conditions: -packages: \"../x\" is not a package directory below the module root, such as ./cmd/server\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script := &fakeScript{}
			var stdout, stderr bytes.Buffer
			if got := runMeasure(tc.args, &stdout, &stderr, script.run); got != 2 {
				t.Fatalf("runMeasure() = %d, want 2", got)
			}
			if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), tc.wantStderr) || len(script.calls) != 0 {
				t.Errorf("stdout = %q, stderr = %q, %d script call(s); want nothing, %q and none", stdout.String(), stderr.String(), len(script.calls), tc.wantStderr)
			}
		})
	}
}

// TestRunMeasure_Packages_AreMeasuredOneAfterAnother verifies a job's
// measuring end to end: each package is run in the module root -dir names,
// in the order named, with nothing added to the environment the step gives
// it; its output streams to stdout and is kept beside its record, below the
// package's own directory of -records; stderr says what became of each; and
// a package the script could not measure fails the job only after every
// other package has been measured, named with the reason's whereabouts.
func TestRunMeasure_Packages_AreMeasuredOneAfterAnother(t *testing.T) {
	t.Parallel()
	script := &fakeScript{
		outputs: map[string]string{"./cmd/a": "Condition coverage: 1/2\n", "./internal/b": "gobco: refused\n", "./internal/c": "Condition coverage: 3/4\n"},
		status:  map[string]int{"./cmd/a": 0, "./internal/b": 1, "./internal/c": 3},
	}
	records := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := []string{"-packages", "./cmd/a ./internal/b,./internal/c", "-system", "ubuntu-latest", "-records", records, "-dir", "root"}
	if got := runMeasure(args, &stdout, &stderr, script.run); got != 1 {
		t.Fatalf("runMeasure() = %d, want 1; stderr:\n%s", got, stderr.String())
	}
	wantCalls := []scriptCall{{dir: "root", pkg: "./cmd/a"}, {dir: "root", pkg: "./internal/b"}, {dir: "root", pkg: "./internal/c"}}
	if !reflect.DeepEqual(script.calls, wantCalls) {
		t.Errorf("script calls = %+v, want %+v", script.calls, wantCalls)
	}
	if want := "Condition coverage: 1/2\ngobco: refused\nCondition coverage: 3/4\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	wantStderr := "measure_conditions: ./cmd/a on ubuntu-latest: measured (exit 0)\n" +
		"measure_conditions: ./internal/b on ubuntu-latest: not measured (exit 1)\n" +
		"measure_conditions: ./internal/c on ubuntu-latest: measured (exit 3)\n" +
		"measure_conditions: 1 of 3 package(s) could not be measured on ubuntu-latest: ./internal/b; the gobco: lines above say why\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr =\n%s\nwant\n%s", stderr.String(), wantStderr)
	}
	for _, each := range []struct {
		slug, pkg, output string
		status            int
	}{
		{slug: "cmd-a", pkg: "./cmd/a", output: "Condition coverage: 1/2\n", status: 0},
		{slug: "internal-b", pkg: "./internal/b", output: "gobco: refused\n", status: 1},
		{slug: "internal-c", pkg: "./internal/c", output: "Condition coverage: 3/4\n", status: 3},
	} {
		t.Run(each.pkg, func(t *testing.T) {
			t.Parallel()
			wantRecord := fmt.Sprintf("package=%s\nsystem=ubuntu-latest\nplatform=%s\nstatus=%d\n", each.pkg, platform, each.status)
			if got := readFile(t, filepath.Join(records, each.slug, recordFile)); got != wantRecord {
				t.Errorf("record = %q, want %q", got, wantRecord)
			}
			if got := readFile(t, filepath.Join(records, each.slug, outputFile)); got != each.output {
				t.Errorf("raw output = %q, want %q", got, each.output)
			}
		})
	}
}

// TestRunMeasure_EveryPackageMeasured_Passes verifies that a job whose every
// package was measured passes, whatever its gate held.
func TestRunMeasure_EveryPackageMeasured_Passes(t *testing.T) {
	t.Parallel()
	script := &fakeScript{outputs: map[string]string{"./a": "Condition coverage: 1/2\n"}, status: map[string]int{"./a": 3}}
	var stdout, stderr bytes.Buffer
	if got := runMeasure([]string{"-packages", "./a", "-system", "s", "-records", t.TempDir()}, &stdout, &stderr, script.run); got != 0 {
		t.Fatalf("runMeasure() = %d, want 0; stderr:\n%s", got, stderr.String())
	}
	if want := "measure_conditions: ./a on s: measured (exit 3)\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// TestRunMeasure_Failures_StopTheJobAndSayWhy verifies that a package the
// script could not be run over, and a record that could not be written in
// any of its three steps, stop the job at that package with the reason, so
// the package after it is never run, and that the package is left without a
// record readRecords could read.
func TestRunMeasure_Failures_StopTheJobAndSayWhy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		prepare func(t *testing.T, records string) string
		want    string
	}{
		{name: "the script could not run", want: "measure_conditions: scripts/coverage-conditions.sh could not be run over ./gone: planted\n"},
		{
			name: "the records directory is a file",
			prepare: func(t *testing.T, records string) string {
				t.Helper()
				file := filepath.Join(records, "file")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return file
			},
			want: "measure_conditions: mkdir ",
		},
		{
			name: "the raw output cannot be written",
			prepare: func(t *testing.T, records string) string {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(records, "a", outputFile), 0o750); err != nil {
					t.Fatal(err)
				}
				return records
			},
			want: "measure_conditions: open " + filepath.Join("a", outputFile),
		},
		{
			name: "the record cannot be written",
			prepare: func(t *testing.T, records string) string {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(records, "a", recordFile), 0o750); err != nil {
					t.Fatal(err)
				}
				return records
			},
			want: "measure_conditions: open " + filepath.Join("a", recordFile),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			records := t.TempDir()
			pkg := "./gone"
			if tc.prepare != nil {
				records = tc.prepare(t, records)
				pkg = "./a"
			}
			script := &fakeScript{outputs: map[string]string{"./a": "x\n"}, status: map[string]int{"./a": 0}, err: errors.New("planted")}
			var stdout, stderr bytes.Buffer
			if got := runMeasure([]string{"-packages", pkg + " ./later", "-system", "s", "-records", records}, &stdout, &stderr, script.run); got != 1 {
				t.Fatalf("runMeasure() = %d, want 1", got)
			}
			got := strings.ReplaceAll(stderr.String(), records+string(filepath.Separator), "")
			if !strings.HasPrefix(got, tc.want) || len(script.calls) != 1 {
				t.Errorf("stderr = %q after %d script call(s), want one call and a line starting %q", got, len(script.calls), tc.want)
			}
			if found, _ := readRecords(records); len(found) != 0 {
				t.Errorf("readRecords() = %+v, want no record of a package whose measuring failed", found)
			}
		})
	}
}

// TestMeasurePackage_Record_ReadsBackAsTheSummaryReadsIt holds the writer of
// a record to its reader: what measurePackage leaves is what readRecords
// finds, field for field, with the platform the measuring ran on.
func TestMeasurePackage_Record_ReadsBackAsTheSummaryReadsIt(t *testing.T) {
	t.Parallel()
	script := &fakeScript{outputs: map[string]string{"./internal/tools/x": "Condition coverage: 5/6\n"}, status: map[string]int{"./internal/tools/x": 3}}
	records := t.TempDir()
	run := measurer{run: script.run, dir: ".", system: "macos-latest", records: records, env: []string{"GOBCO_GATE=all"}}
	status, err := measurePackage(context.Background(), run, "./internal/tools/x", io.Discard)
	if err != nil || status != 3 {
		t.Fatalf("measurePackage() = %d, %v; want 3", status, err)
	}
	got, problems := readRecords(records)
	want := []record{{pkg: "./internal/tools/x", system: "macos-latest", platform: platform, status: 3, output: "Condition coverage: 5/6\n"}}
	if len(problems) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("readRecords() = %+v, %q; want %+v", got, problems, want)
	}
	if wantCalls := []scriptCall{{dir: ".", pkg: "./internal/tools/x", env: []string{"GOBCO_GATE=all"}}}; !reflect.DeepEqual(script.calls, wantCalls) {
		t.Errorf("script calls = %+v, want %+v", script.calls, wantCalls)
	}
}

// TestRunScript_PlantedScript_RunsThroughBashInTheModuleRoot runs the
// production runner over a planted scripts/coverage-conditions.sh: the
// script is run through bash in the directory given, with the package as its
// argument and the environment added, both of its streams reach the writer
// in the order printed, and its exit status comes back as the status, 0
// included. A directory that does not exist is an error, not a status.
func TestRunScript_PlantedScript_RunsThroughBashInTheModuleRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatal(err)
	}
	script := "echo \"out $1 $GOBCO_GATE\"\necho err >&2\nexit \"$EXIT\"\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(coverageScript)), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, exit := range []int{0, 3} {
		t.Run(strconv.Itoa(exit), func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			status, err := runScript(context.Background(), root, "./x", []string{"GOBCO_GATE=all", fmt.Sprintf("EXIT=%d", exit)}, &output)
			if err != nil || status != exit || output.String() != "out ./x all\nerr\n" {
				t.Errorf("runScript() = %d, %v, output %q; want %d, no error and both streams", status, err, output.String(), exit)
			}
		})
	}
	if _, err := runScript(context.Background(), filepath.Join(root, "absent"), "./x", nil, io.Discard); err == nil {
		t.Errorf("runScript(absent directory) error = nil, want one")
	}
}

// TestRunScript_NoBashOnPath_IsTheLookupsFailure verifies that bash is found
// on PATH before the script is run through it, and that a PATH holding none
// is reported as the lookup's failure rather than as a run that failed.
func TestRunScript_NoBashOnPath_IsTheLookupsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := runScript(context.Background(), t.TempDir(), "./x", nil, io.Discard); err == nil || !strings.HasPrefix(err.Error(), "find bash: ") {
		t.Errorf("runScript() with no bash on PATH = %v, want the lookup's failure", err)
	}
}
