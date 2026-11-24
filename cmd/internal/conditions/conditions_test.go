// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// gobcoAnswer is how the stand-in gobco answers one run: the figure it prints
// ("3/4" unless another is given, "none" for no figure at all), the words it
// opens it with when they are not its mode's, the conditions it names, its
// exit status, and whether go run could not reach the module proxy for it.
type gobcoAnswer struct {
	Figure      string   `json:"figure"`
	Kind        string   `json:"kind"`
	Conditions  []string `json:"conditions"`
	Status      int      `json:"status"`
	Unreachable bool     `json:"unreachable"`
}

// loadError names the listing whose error the record holds as $LOADERROR: the
// go command's own words, which are asked of it again rather than recorded,
// since a toolchain bump may reword them.
type loadError struct {
	Package string            `json:"package"`
	Env     map[string]string `json:"env"`
}

// recordedCall is one program a recorded run started, and what gobco found
// where it was run.
type recordedCall struct {
	Tool        string            `json:"tool"`
	Args        []string          `json:"args"`
	Dir         string            `json:"dir"`
	GOMODCACHE  string            `json:"gomodcache"`
	GOOS        string            `json:"goos"`
	Files       map[string]string `json:"files"`
	ModuleFiles []string          `json:"module_files"`
}

// recordedRun is one run of scripts/coverage-conditions.sh: what it was given,
// and what it did.
type recordedRun struct {
	Name            string             `json:"name"`
	Args            []string           `json:"args"`
	Env             map[string]string  `json:"env"`
	Unset           []string           `json:"unset"`
	Files           map[string]*string `json:"files"`
	Planted         map[string]string  `json:"planted"`
	Gobco           gobcoAnswer        `json:"gobco"`
	InstallFailures int                `json:"install_failures"`
	LoadError       *loadError         `json:"load_error"`
	Status          int                `json:"status"`
	Stdout          string             `json:"stdout"`
	Stderr          string             `json:"stderr"`
	Calls           []recordedCall     `json:"calls"`
}

// record is testdata/conditions_runs.json: the fixture module and every run
// recorded over it.
type record struct {
	Files map[string]string `json:"files"`
	Runs  []recordedRun     `json:"runs"`
}

// readRecord reads the recorded runs.
func readRecord(t *testing.T) record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "conditions_runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec record
	if err = json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// writeFile writes one file of a fixture, creating its directory.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// bench is a fixture module with the stand-in tools over it: where the module
// is, what is planted on PATH, and what was run.
type bench struct {
	scratch string
	root    string
	tmp     string
	environ []string
	planted map[string]string
	gobco   gobcoAnswer
	// installFailures is how many attempts at go install fail first.
	installFailures int
	// installs is how many attempts were made.
	installs int
	calls    []recordedCall
	pauses   []time.Duration
	// onGobco is called while the stand-in gobco runs, which is when a test
	// interrupts the run.
	onGobco func()
	// plant, when it is set, answers a program in place of what would, and
	// says whether it did.
	plant func(call recipe.Call) (answered bool, status int, err error)
}

// newBench lays out the module in files, and an environment holding nothing
// this machine sets that the recipe or the go command reads. The temporary
// directory is this test's, so what a run stages there and whether it removed
// it can both be seen. It sets the process's own, so no test using it runs in
// parallel.
func newBench(t *testing.T, files map[string]string) *bench {
	t.Helper()
	scratch, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &bench{scratch: scratch, root: filepath.Join(scratch, "module"), tmp: filepath.Join(scratch, "tmp"), planted: map[string]string{}}
	for rel, content := range files {
		writeFile(t, b.root, rel, content)
	}
	if err = os.MkdirAll(b.tmp, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, b.tmp)
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "GO") && !strings.HasPrefix(name, "CGO_") {
			b.environ = append(b.environ, entry)
		}
	}
	b.environ = append(b.environ, "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOENV=off", "GOWORK=off",
		"GOFLAGS=-buildvcs=false", "GOTOOLCHAIN=local", "GOROOT="+filepath.Dir(filepath.Dir(golist.Executable())),
		"GOCACHE="+goCache(t), "GOMODCACHE="+filepath.Join(scratch, "modcache"), "GOBIN="+filepath.Join(scratch, "gobin"))
	return b
}

// goCache is the build cache of this machine, since the benches name no HOME
// setting of their own for the go command to derive one from on every
// platform.
func goCache(t *testing.T) string {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = t.TempDir()
	}
	return filepath.Join(dir, "go-build")
}

// lookPath finds gobco where it is planted, the directory ahead on PATH
// first.
func (b *bench) lookPath(name string) (string, error) {
	for _, dir := range []string{"bin", "gobin"} {
		if _, ok := b.planted[dir]; ok && name == "gobco" {
			return filepath.ToSlash(filepath.Join(b.scratch, dir, name)), nil
		}
	}
	return "", fmt.Errorf("%s is not planted", name)
}

// run answers for the go command and gobco: what lists and describes the
// module and the toolchain is asked of the real go command, since what a run
// stages rests on its answers, and what would fetch or measure is recorded
// and answered as the bench says.
func (b *bench) run(ctx context.Context, call recipe.Call) (int, error) {
	if b.plant != nil {
		if answered, status, err := b.plant(call); answered {
			return status, err
		}
	}
	if call.Program != golist.Executable() {
		return b.answerGobco(call, "gobco", call.Args)
	}
	switch call.Args[0] {
	case "run":
		return b.answerGobco(call, "go", call.Args[2:])
	case "install":
		return b.answerInstall(call), nil
	default:
		return recipe.Exec(ctx, call)
	}
}

// answerInstall is go install of gobco.
func (b *bench) answerInstall(call recipe.Call) int {
	b.calls = append(b.calls, recordedCall{Tool: "go", Args: call.Args})
	b.installs++
	if b.installs <= b.installFailures {
		fmt.Fprintln(call.Stderr, "planted: the proxy is unreachable")
		return 1
	}
	gobin, _ := recipe.Getenv(call.Env, "GOBIN")
	b.planted[filepath.Base(gobin)] = "1.3.4"
	return 0
}

// answerGobco is gobco, through go run or from PATH.
func (b *bench) answerGobco(call recipe.Call, tool string, args []string) (int, error) {
	if tool == "gobco" && slices.Equal(args, []string{"-version"}) {
		b.calls = append(b.calls, recordedCall{Tool: tool, Args: args})
		fmt.Fprintln(call.Stdout, b.planted[filepath.Base(filepath.Dir(call.Program))])
		return 0, nil
	}
	entry := recordedCall{Tool: tool, Args: call.Args, Dir: filepath.ToSlash(call.Dir), Files: map[string]string{}}
	entry.GOMODCACHE, _ = recipe.Getenv(call.Env, "GOMODCACHE")
	entry.GOOS, _ = recipe.Getenv(call.Env, "GOOS")
	if err := entry.look(call.Dir, b.tmp); err != nil {
		return 0, err
	}
	b.calls = append(b.calls, entry)
	if b.onGobco != nil {
		b.onGobco()
	}
	if tool == "go" && b.gobco.Unreachable {
		fmt.Fprintln(call.Stderr, "planted: the proxy is unreachable")
		return 1, nil
	}
	b.gobco.print(call, slices.Contains(args, "-branch"))
	if tool == "go" {
		return min(b.gobco.Status, 1), nil
	}
	return b.gobco.Status, nil
}

// print writes the report the answer describes.
func (g gobcoAnswer) print(call recipe.Call, branch bool) {
	kind, figure := g.Kind, g.Figure
	if kind == "" {
		kind = "Condition coverage"
		if branch {
			kind = "Branch coverage"
		}
	}
	if figure == "" {
		figure = "3/4"
	}
	fmt.Fprintln(call.Stdout)
	if figure != "none" {
		fmt.Fprintf(call.Stdout, "%s: %s\n", kind, figure)
	}
	for _, line := range g.Conditions {
		fmt.Fprintln(call.Stdout, line)
	}
	fmt.Fprintln(call.Stderr, "gobco's own standard error")
}

// look records the .go files gobco would parse in dir, and, for a dir below
// the temporary directory, every file of the staged module it sits in.
func (c *recordedCall) look(dir, tmp string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return readErr
		}
		c.Files[entry.Name()] = string(data)
	}
	if !strings.HasPrefix(filepath.Clean(dir), tmp) {
		return nil
	}
	root := filepath.Clean(dir)
	for {
		if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
			break
		}
		above := filepath.Dir(root)
		if above == root {
			return fmt.Errorf("%s is below no go.mod", dir)
		}
		root = above
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel(root, path)
		c.ModuleFiles = append(c.ModuleFiles, filepath.ToSlash(rel))
		sort.Strings(c.ModuleFiles)
		return relErr
	})
}

// tools are the stand-ins as the recipe takes them.
func (b *bench) tools() Tools {
	return Tools{
		Go: golist.Executable(), Run: b.run, LookPath: b.lookPath,
		Pause: func(_ context.Context, wait time.Duration) { b.pauses = append(b.pauses, wait) },
	}
}

// measure runs the recipe over a package of the bench's module.
func (b *bench) measure(ctx context.Context, args ...string) (status int, stdout, stderr string) {
	var out, errs bytes.Buffer
	req := Request{Dir: b.root, Package: args[0], Environ: b.environ, Stdout: &out, Stderr: &errs}
	if len(args) > 1 {
		req.Tags = args[1]
	}
	status = Measure(ctx, b.tools(), req)
	return status, out.String(), errs.String()
}

// left is what a run left in the temporary directory, which should be
// nothing.
func (b *bench) left(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(b.tmp)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// said is how a message reads with this bench's own paths written as the
// record writes them, and the one command line that replaced another put
// back. The record's one other departure from what the script wrote is the
// spelling of "honors" in its message about build tags.
func (b *bench) said(text string) string {
	for _, path := range []struct{ from, to string }{{b.root, "$ROOT"}, {b.scratch, "$SCRATCH"}} {
		text = strings.ReplaceAll(text, path.from, path.to)
		text = strings.ReplaceAll(text, filepath.ToSlash(path.from), path.to)
	}
	return strings.ReplaceAll(text, installCommand, "scripts/coverage-conditions.sh --install")
}

// expand writes the record's placeholder for the scratch directory as this
// bench's.
func (b *bench) expand(value string) string {
	return strings.ReplaceAll(value, "$SCRATCH", b.scratch)
}

// prepare brings the bench to what a recorded run was given.
func (b *bench) prepare(t *testing.T, run recordedRun) {
	t.Helper()
	for rel, content := range run.Files {
		if content != nil {
			writeFile(t, b.root, rel, *content)
		}
	}
	maps.Copy(b.planted, run.Planted)
	for _, name := range run.Unset {
		b.environ = slices.DeleteFunc(b.environ, func(entry string) bool { return strings.HasPrefix(entry, name+"=") })
	}
	for name, value := range run.Env {
		b.environ = append(b.environ, name+"="+b.expand(value))
	}
	b.gobco, b.installFailures = run.Gobco, run.InstallFailures
}

// loadErrorText asks the go command for the error a recorded listing gave.
func (b *bench) loadErrorText(t *testing.T, of loadError) string {
	t.Helper()
	environ := slices.Clone(b.environ)
	for name, value := range of.Env {
		environ = append(environ, name+"="+value)
	}
	var out bytes.Buffer
	status, err := recipe.Exec(t.Context(), recipe.Call{
		Program: golist.Executable(), Args: []string{"list", "-e", "-f", "{{with .Error}}{{.}}{{end}}", of.Package},
		Dir: b.root, Env: environ, Stdout: &out,
	})
	if err != nil || status != 0 {
		t.Fatalf("go list: %d, %v", status, err)
	}
	return b.said(strings.TrimSpace(out.String()))
}

// normalCall writes a call as the record writes one: its directories with the
// placeholders, the staged module's temporary directory among them.
func (b *bench) normalCall(call recordedCall) recordedCall {
	call.GOMODCACHE = filepath.ToSlash(b.said(call.GOMODCACHE))
	if rel, below := strings.CutPrefix(filepath.ToSlash(call.Dir), filepath.ToSlash(b.tmp)+"/"); below {
		_, rest, _ := strings.Cut(rel, "/")
		call.Dir = "$STAGED/" + rest
	} else {
		call.Dir = filepath.ToSlash(b.said(call.Dir))
	}
	return call
}

// TestRecordedRuns_TheRecipe_DoesWhatTheScriptDid replays every recorded run
// of scripts/coverage-conditions.sh, over the same module and with the same
// stand-ins, and holds the recipe to it: the status, both streams, every
// fetch and measurement with its arguments and directory, the module cache
// gobco is handed, the files gobco finds where it is run and those of the
// module around it, and nothing left in the temporary directory.
func TestRecordedRuns_TheRecipe_DoesWhatTheScriptDid(t *testing.T) {
	rec := readRecord(t)
	if len(rec.Runs) == 0 {
		t.Fatal("the record holds no run")
	}
	for _, run := range rec.Runs {
		t.Run(run.Name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			b.prepare(t, run)
			var status int
			var stdout, stderr string
			if run.Args[0] == "--install" {
				var out, errs bytes.Buffer
				status = Install(t.Context(), b.tools(), b.environ, &out, &errs)
				stdout, stderr = out.String(), errs.String()
			} else {
				status, stdout, stderr = b.measure(t.Context(), run.Args...)
			}
			wantStderr := run.Stderr
			if run.LoadError != nil {
				wantStderr = strings.ReplaceAll(wantStderr, "$LOADERROR", b.loadErrorText(t, *run.LoadError))
			}
			if status != run.Status {
				t.Errorf("status = %d, want %d", status, run.Status)
			}
			if got := b.said(stdout); got != run.Stdout {
				t.Errorf("stdout =\n%s\nwant\n%s", got, run.Stdout)
			}
			if got := b.said(stderr); got != wantStderr {
				t.Errorf("stderr =\n%s\nwant\n%s", got, wantStderr)
			}
			b.compareCalls(t, run.Calls)
			if left := b.left(t); len(left) != 0 {
				t.Errorf("left in the temporary directory: %q", left)
			}
		})
	}
}

// askedNothing leaves out the calls that only ask a gobco for its version,
// which fetch and measure nothing and which the script sometimes repeated.
func askedNothing(calls []recordedCall) []recordedCall {
	return slices.DeleteFunc(slices.Clone(calls), func(call recordedCall) bool { return slices.Equal(call.Args, []string{"-version"}) })
}

// compareCalls holds what the bench saw fetched and measured to what the
// record saw.
func (b *bench) compareCalls(t *testing.T, want []recordedCall) {
	t.Helper()
	want = askedNothing(want)
	ran := askedNothing(b.calls)
	if len(ran) != len(want) {
		t.Fatalf("ran %d program(s), want %d: %+v", len(ran), len(want), ran)
	}
	for i, call := range ran {
		got := b.normalCall(call)
		expected := want[i]
		if got.Tool != expected.Tool || !slices.Equal(got.Args, expected.Args) {
			t.Errorf("call %d = %s %q, want %s %q", i, got.Tool, got.Args, expected.Tool, expected.Args)
		}
		if expected.Files == nil {
			continue
		}
		if got.Dir != expected.Dir || got.GOMODCACHE != expected.GOMODCACHE || got.GOOS != expected.GOOS {
			t.Errorf("call %d ran in %q with GOMODCACHE=%q GOOS=%q, want %q, %q and %q", i, got.Dir, got.GOMODCACHE, got.GOOS,
				expected.Dir, expected.GOMODCACHE, expected.GOOS)
		}
		if !reflect.DeepEqual(got.Files, expected.Files) {
			t.Errorf("call %d found the files %+v, want %+v", i, got.Files, expected.Files)
		}
		if !slices.Equal(got.ModuleFiles, expected.ModuleFiles) {
			t.Errorf("call %d found the module\n%q\nwant\n%q", i, got.ModuleFiles, expected.ModuleFiles)
		}
	}
}

// interruptible is a context a test ends as a signal would, and what ends it.
func interruptible(t *testing.T) (ctx context.Context, interrupt func(os.Signal)) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	t.Cleanup(func() { cancel(nil) })
	return ctx, func(sig os.Signal) { cancel(recipe.InterruptedError{Signal: sig}) }
}

// TestMeasure_AnInterruptWhileGobcoRuns_EndsTheRunAndRemovesTheCopy verifies
// that a signal that arrives while gobco runs lets it finish, and then ends
// the run with the status a shell gives the signal, whatever gobco's report
// says, with the copy gone and nothing said about a failure or by the gate: the
// report of a run that was ended is not judged. It is held with gobco run
// through go run and from PATH, and for a gobco the signal reached too, which
// failed.
func TestMeasure_AnInterruptWhileGobcoRuns_EndsTheRunAndRemovesTheCopy(t *testing.T) {
	rec := readRecord(t)
	cases := []struct {
		name    string
		signal  os.Signal
		planted bool
		status  int
		want    int
	}{
		{name: "SIGTERM", signal: syscall.SIGTERM, want: 143},
		{name: "SIGINT", signal: os.Interrupt, want: 130},
		{name: "SIGINT to gobco too", signal: os.Interrupt, status: 2, want: 130},
		{name: "SIGTERM, gobco on PATH", signal: syscall.SIGTERM, planted: true, want: 143},
		{name: "SIGINT, gobco on PATH", signal: os.Interrupt, planted: true, want: 130},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			if tc.planted {
				b.planted["gobin"] = "1.3.4"
			}
			b.gobco.Status = tc.status
			ctx, interrupt := interruptible(t)
			staged := 0
			b.onGobco = func() {
				staged = len(b.left(t))
				interrupt(tc.signal)
			}
			b.environ = append(b.environ, "GOBCO_GATE=all")
			status, stdout, stderr := b.measure(ctx, "./cmd/tool")
			if status != tc.want {
				t.Errorf("status = %d, want %d", status, tc.want)
			}
			if staged != 1 {
				t.Errorf("gobco ran beside %d staged copies, want one", staged)
			}
			if strings.Contains(stdout, "GOBCO_GATE") || stderr != "gobco's own standard error\n" {
				t.Errorf("stdout = %q, stderr = %q; want no verdict and no word of a failure", stdout, stderr)
			}
			if left := b.left(t); len(left) != 0 {
				t.Errorf("left in the temporary directory: %q", left)
			}
		})
	}
}

// TestMeasure_AnInterruptBeforeGobco_StartsNothingMore verifies that a run
// whose signal arrived before any of its programs, or between two of them,
// starts no other, says nothing about what it could not do, and leaves
// nothing.
func TestMeasure_AnInterruptBeforeGobco_StartsNothingMore(t *testing.T) {
	rec := readRecord(t)
	for _, after := range []string{"", "list", "env", "tool", "-version"} {
		t.Run("after "+after, func(t *testing.T) {
			b := newBench(t, rec.Files)
			b.planted["bin"] = "1.3.3"
			ctx, interrupt := interruptible(t)
			var started []string
			b.plant = func(call recipe.Call) (bool, int, error) {
				started = append(started, call.Args[0])
				if call.Args[0] == after {
					defer interrupt(os.Interrupt)
					if status, err := recipe.Exec(context.WithoutCancel(ctx), call); call.Program == golist.Executable() {
						return true, status, err
					}
					return true, 0, nil
				}
				return false, 0, nil
			}
			if after == "" {
				interrupt(os.Interrupt)
			}
			status, _, stderr := b.measure(ctx, "./cmd/tool")
			if status != 130 || stderr != "" {
				t.Errorf("status = %d, stderr = %q; want 130 and nothing said", status, stderr)
			}
			if strings.Join(started[max(len(started)-1, 0):], "") != after {
				t.Errorf("started %q, want nothing after %q", started, after)
			}
			if len(b.calls) != 0 || len(b.left(t)) != 0 {
				t.Errorf("ran %+v and left %q, want neither", b.calls, b.left(t))
			}
		})
	}
}

// answering is a plant for the go command's verb, with what it writes and how
// it ends.
func answering(verb, out string, status int, err error) func(recipe.Call) (bool, int, error) {
	return func(call recipe.Call) (bool, int, error) {
		if call.Args[0] != verb {
			return false, 0, nil
		}
		_, _ = call.Stdout.Write([]byte(out))
		return true, status, err
	}
}

// TestRunGobco_ATargetThatIsNotThere_IsTheRunsToReport verifies that a
// directory whose links cannot be resolved, as one that is gone, is handed to
// gobco as it was named, and that its refusal is what the run says.
func TestRunGobco_ATargetThatIsNotThere_IsTheRunsToReport(t *testing.T) {
	b := newBench(t, readRecord(t).Files)
	gone := filepath.Join(b.root, "gone")
	var out, errs bytes.Buffer
	m := &measurement{tools: b.tools(), Package: "./gone", Environ: b.environ, Stdout: &out, Stderr: &errs}
	var ranIn string
	b.plant = func(call recipe.Call) (bool, int, error) {
		ranIn = call.Dir
		return true, 0, fs.ErrNotExist
	}
	if report, passed := m.runGobco(t.Context(), gone); passed || report != "" {
		t.Errorf("runGobco() = %q, %t; want nothing measured", report, passed)
	}
	if want := "gobco: gobco could not be started on ./gone: file does not exist; refusing to measure\n"; ranIn != gone || errs.String() != want {
		t.Errorf("ran in %q and said %q, want %q and %q", ranIn, errs.String(), gone, want)
	}
}

// TestMeasure_AProgramThatDoesNotAnswer_IsRefused verifies every way the
// programs the run asks before gobco can fail to answer, and gobco itself
// failing to start: each is a run that measured nothing, with the reason, and
// nothing is left behind.
func TestMeasure_AProgramThatDoesNotAnswer_IsRefused(t *testing.T) {
	rec := readRecord(t)
	planted := errors.New("planted")
	cases := []struct {
		name  string
		pkg   string
		plant func(recipe.Call) (bool, int, error)
		want  string
	}{
		{name: "go list cannot start", plant: answering("list", "", 0, planted), want: "gobco: go list could not list ./cmd/tool under build tags (none): planted; refusing to measure\n"},
		{name: "go list fails", plant: answering("list", "", 1, nil), want: "gobco: go list could not list ./cmd/tool under build tags (none): "},
		{name: "go list answers something else", plant: answering("list", "x\n", 0, nil), want: "gobco: go list could not list ./cmd/tool under build tags (none): unexpected go list row: \"x\"; refusing to measure\n"},
		{name: "a pattern", pkg: "./internal/...", want: "gobco: go list could not list ./internal/... under build tags (none): it names "},
		{name: "go env cannot start", plant: answering("env", "", 0, planted), want: "gobco: go env failed: planted; refusing to measure\n"},
		{name: "go env answers too little", plant: answering("env", "linux\namd64", 0, nil), want: "gobco: go env answered \"linux\\namd64\" for three variables; refusing to measure\n"},
		{name: "go tool dist list cannot start", plant: answering("tool", "", 0, planted), want: "gobco: go tool dist list failed: planted; refusing to measure\n"},
		{name: "gobco cannot start", plant: answering("run", "", 0, planted), want: "gobco: gobco could not be started on ./cmd/tool: planted; refusing to measure\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			b.plant = tc.plant
			pkg := tc.pkg
			if pkg == "" {
				pkg = "./cmd/tool"
			}
			status, _, stderr := b.measure(t.Context(), pkg)
			if status != NotMeasured || !strings.HasPrefix(stderr, tc.want) {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, stderr, tc.want)
			}
			if len(b.calls) != 0 || len(b.left(t)) != 0 {
				t.Errorf("ran %+v and left %q, want neither", b.calls, b.left(t))
			}
		})
	}
}

// TestMeasure_OneWriterForBothStreams_TakesEverything verifies that the two
// streams may be one writer, as a caller that keeps a run's whole output
// gives them: gobco's report and its standard error, written from two
// goroutines at once, both reach it. The race detector is what holds the lock.
func TestMeasure_OneWriterForBothStreams_TakesEverything(t *testing.T) {
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	b.plant = func(call recipe.Call) (bool, int, error) {
		if call.Args[0] != "run" {
			return false, 0, nil
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 100 {
				_, _ = call.Stderr.Write([]byte("e"))
			}
		}()
		for range 100 {
			_, _ = call.Stdout.Write([]byte("o"))
		}
		<-done
		_, _ = call.Stdout.Write([]byte("\nCondition coverage: 1/2\n"))
		return true, 0, nil
	}
	var both bytes.Buffer
	if status := Measure(t.Context(), b.tools(), Request{Dir: b.root, Package: "./internal/plain", Environ: b.environ, Stdout: &both, Stderr: &both}); status != Measured {
		t.Errorf("status = %d, want measured; said %q", status, both.String())
	}
	// Counted apart from the report, which holds both letters.
	streams, ended := strings.CutSuffix(both.String(), "\nCondition coverage: 1/2\n")
	if !ended || strings.Count(streams, "e") != 100 || strings.Count(streams, "o") != 100 {
		t.Errorf("the writer took %d bytes: %q", both.Len(), both.String())
	}
}

// TestDefaultTools_AreTheToolchainAndThePath verifies the tools production
// measures with: the toolchain's own go command, and a pause that ends when
// its time is up or when the run is ending.
func TestDefaultTools_AreTheToolchainAndThePath(t *testing.T) {
	t.Parallel()
	tools := DefaultTools()
	if tools.Go != golist.Executable() || tools.Run == nil || tools.LookPath == nil {
		t.Errorf("DefaultTools() = %+v", tools)
	}
	started := time.Now()
	tools.Pause(t.Context(), 10*time.Millisecond)
	if took := time.Since(started); took < 10*time.Millisecond {
		t.Errorf("a pause of 10ms took %v", took)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tools.Pause(ctx, time.Hour)
}
