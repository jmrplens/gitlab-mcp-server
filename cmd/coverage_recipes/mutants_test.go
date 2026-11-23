// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// mutantsCall is one program a recorded run started: what it was given, and
// the files below the directory it named.
type mutantsCall struct {
	Args         []string `json:"args"`
	Dir          string   `json:"dir"`
	GOFLAGS      string   `json:"goflags"`
	GOROOT       string   `json:"goroot"`
	Tree         []string `json:"tree"`
	AddressSpace int64    `json:"address_space"`
}

// mutantsRun is one run of scripts/coverage-mutants.sh: what it was given, the
// seconds its baseline read by the clock, and what it did.
type mutantsRun struct {
	Name           string             `json:"name"`
	Args           []string           `json:"args"`
	Env            map[string]string  `json:"env"`
	Files          map[string]*string `json:"files"`
	TestFailRun    string             `json:"test_fail_run"`
	GremlinsStatus int                `json:"gremlins_status"`
	LoadError      *struct {
		Package string `json:"package"`
	} `json:"load_error"`
	Base   string        `json:"base"`
	Status int           `json:"status"`
	Stdout string        `json:"stdout"`
	Stderr string        `json:"stderr"`
	Calls  []mutantsCall `json:"calls"`
}

// mutantsRecord is testdata/mutants_runs.json: the fixture module and every
// run recorded over it.
type mutantsRecord struct {
	Files map[string]string `json:"files"`
	Runs  []mutantsRun      `json:"runs"`
}

// readRecord reads a recorded file into out.
func readRecord(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
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

// fileTree is every file below dir, by its path relative to it.
func fileTree(dir string) []string {
	files := []string{}
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		files = append(files, filepath.ToSlash(rel))
		return relErr
	})
	sort.Strings(files)
	return files
}

// mutantsBench is a fixture module with the stand-in go test and gremlins
// over it: where it is, how they answer, and what was run.
type mutantsBench struct {
	scratch  string
	root     string
	tmp      string
	session  *session
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	run      mutantsRun
	calls    []mutantsCall
	profiles []string
	clock    time.Time
	// base is what each tick of the clock adds, so the two readings around
	// the timed baseline are that far apart.
	base time.Duration
	// onCall is called while a stand-in runs, with the call's first word,
	// which is when a test interrupts the run.
	onCall func(verb string)
}

// newMutantsBench lays out the module in files and a session over it, its
// environment holding nothing this machine sets that the recipe or the go
// command reads. It sets the process's temporary directory, so no test using
// it runs in parallel.
func newMutantsBench(t *testing.T, files map[string]string) *mutantsBench {
	t.Helper()
	scratch, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &mutantsBench{scratch: scratch, root: filepath.Join(scratch, "module"), tmp: filepath.Join(scratch, "tmp"), base: 250 * time.Millisecond}
	for rel, content := range files {
		writeFile(t, b.root, rel, content)
	}
	if err = os.MkdirAll(b.tmp, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, b.tmp)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = t.TempDir()
	}
	var environ []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "GO") && !strings.HasPrefix(name, "CGO_") && !strings.HasPrefix(name, "GREMLINS_") && !strings.HasPrefix(name, "MUTANT_") {
			environ = append(environ, entry)
		}
	}
	environ = append(environ, "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOENV=off", "GOWORK=off", "GOFLAGS=-buildvcs=false",
		"GOTOOLCHAIN=local", "GOCACHE="+filepath.Join(cache, "go-build"), "GOMODCACHE="+filepath.Join(scratch, "modcache"))
	b.session = &session{
		Go: golist.Executable(), Run: b.answer, now: b.now, goos: "linux",
		limits: func() (soft, hard uint64, err error) { return math.MaxUint64, math.MaxUint64, nil },
		dir:    b.root, environ: environ, stdout: &b.stdout, stderr: &b.stderr,
	}
	return b
}

// now is the bench's clock: each reading is base after the one before.
func (b *mutantsBench) now() time.Time {
	b.clock = b.clock.Add(b.base)
	return b.clock
}

// answer runs a call: go list and go env as the real go command, and the rest
// recorded and answered as the bench's run says.
func (b *mutantsBench) answer(ctx context.Context, call recipe.Call) (int, error) {
	verb := call.Args[0]
	if verb == "list" || verb == "env" {
		return recipe.Exec(ctx, call)
	}
	entry := mutantsCall{Args: slices.Clone(call.Args), Dir: call.Dir, AddressSpace: -1}
	if call.AddressSpace != 0 {
		entry.AddressSpace = int64(call.AddressSpace) //#nosec G115 -- a ceiling the tests set, well under the limit
	}
	entry.GOFLAGS, _ = recipe.Getenv(call.Env, "GOFLAGS")
	entry.GOROOT, _ = recipe.Getenv(call.Env, "GOROOT")
	status := 0
	switch verb {
	case "mod":
		entry.GOFLAGS, entry.GOROOT = "", ""
	case "test":
		status = b.answerTest(call, &entry)
	case "run":
		entry.Tree = fileTree(filepath.Join(call.Dir, call.Args[len(call.Args)-1]))
		fmt.Fprintln(call.Stdout, "gremlins ran")
		status = b.run.GremlinsStatus
	}
	b.calls = append(b.calls, entry)
	if b.onCall != nil {
		b.onCall(verb)
	}
	return status, nil
}

// answerTest is go test: its output, and the failure of the run the bench
// plants one in.
func (b *mutantsBench) answerTest(call recipe.Call, entry *mutantsCall) int {
	if at := slices.Index(entry.Args, "-coverprofile"); at >= 0 {
		b.profiles = append(b.profiles, entry.Args[at+1])
		entry.Args[at+1] = "$PROFILE"
	}
	pattern := call.Args[len(call.Args)-1]
	entry.Tree = fileTree(filepath.Join(call.Dir, strings.TrimSuffix(strings.TrimRight(pattern, "."), "/")))
	tests := 1
	for _, earlier := range b.calls {
		if earlier.Args[0] == "test" {
			tests++
		}
	}
	if b.run.TestFailRun == strconv.Itoa(tests) {
		fmt.Fprintln(call.Stdout, "--- FAIL: TestPlanted (0.00s)")
		return 1
	}
	fmt.Fprintf(call.Stdout, "ok  \t%s\t0.912s\tcoverage: 71.4%% of statements\n", pattern)
	return 0
}

// prepare brings the bench to what a recorded run was given.
func (b *mutantsBench) prepare(t *testing.T, run mutantsRun) {
	t.Helper()
	b.run = run
	for rel, content := range run.Files {
		if content != nil {
			writeFile(t, b.root, rel, *content)
		} else if err := os.Remove(filepath.Join(b.root, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range run.Env {
		b.session.environ = append(b.session.environ, name+"="+value)
	}
	if run.Base != "" {
		base, err := time.ParseDuration(run.Base + "s")
		if err != nil {
			t.Fatal(err)
		}
		b.base = base
	}
}

// said is how text reads with this bench's own paths, and the toolchain's
// root, written as the record writes them.
func (b *mutantsBench) said(text string) string {
	goroot := filepath.Dir(filepath.Dir(golist.Executable()))
	for _, path := range []struct{ from, to string }{{b.root, "$ROOT"}, {b.scratch, "$SCRATCH"}, {goroot, "$GOROOT"}} {
		text = strings.ReplaceAll(text, path.from, path.to)
		text = strings.ReplaceAll(text, filepath.ToSlash(path.from), path.to)
	}
	return strings.ReplaceAll(text, "the recipe's second, third and fourth arguments", "the script's second, third and fourth arguments")
}

// compareCalls holds what the bench saw run to what the record saw run.
func (b *mutantsBench) compareCalls(t *testing.T, want []mutantsCall) {
	t.Helper()
	if len(b.calls) != len(want) {
		t.Fatalf("ran %d program(s), want %d: %+v", len(b.calls), len(want), b.calls)
	}
	for i, call := range b.calls {
		got := fmt.Sprintf("%q in %s, GOFLAGS %q, GOROOT %s, address space %d, found %q", call.Args, filepath.ToSlash(b.said(call.Dir)),
			strings.TrimSpace(call.GOFLAGS), b.said(call.GOROOT), call.AddressSpace, call.Tree)
		expected := want[i]
		if expected.Args[0] != "run" {
			expected.AddressSpace = -1
		}
		if expected.Tree == nil {
			expected.Tree = []string{}
		}
		if call.Tree == nil {
			got = fmt.Sprintf("%q in %s, GOFLAGS %q, GOROOT %s, address space %d, found %q", call.Args, filepath.ToSlash(b.said(call.Dir)),
				strings.TrimSpace(call.GOFLAGS), b.said(call.GOROOT), call.AddressSpace, []string{})
		}
		wanted := fmt.Sprintf("%q in %s, GOFLAGS %q, GOROOT %s, address space %d, found %q", expected.Args, expected.Dir,
			strings.TrimSpace(expected.GOFLAGS), expected.GOROOT, expected.AddressSpace, expected.Tree)
		if got != wanted {
			t.Errorf("call %d =\n%s\nwant\n%s", i, got, wanted)
		}
	}
	for _, profile := range b.profiles {
		if _, err := os.Stat(profile); !os.IsNotExist(err) {
			t.Errorf("the baseline's profile %s was left behind (%v)", profile, err)
		}
	}
}

// loadErrorText asks the go command for the error a recorded listing gave.
func (b *mutantsBench) loadErrorText(t *testing.T, pkg string) string {
	t.Helper()
	out, err := recipe.Output(t.Context(), recipe.Exec, recipe.Call{
		Program: golist.Executable(), Args: []string{"list", "-e", "-f", "{{with .Error}}{{.}}{{end}}", pkg}, Dir: b.root, Env: b.session.environ,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.said(strings.TrimSpace(out))
}

// TestRecordedRuns_Mutants_DoesWhatTheScriptDid replays every recorded run of
// scripts/coverage-mutants.sh, over the same module and with the same
// stand-ins, and holds the recipe to it: the status, both streams, every
// program with its arguments, directory, flags and toolchain root, the files
// it found where it ran, the ceiling gremlins ran under, and nothing left in
// the module or the temporary directory.
func TestRecordedRuns_Mutants_DoesWhatTheScriptDid(t *testing.T) {
	var rec mutantsRecord
	readRecord(t, "mutants_runs.json", &rec)
	if len(rec.Runs) == 0 {
		t.Fatal("the record holds no run")
	}
	for _, run := range rec.Runs {
		t.Run(run.Name, func(t *testing.T) {
			b := newMutantsBench(t, rec.Files)
			b.prepare(t, run)
			before := fileTree(b.root)
			status := b.session.run(t.Context(), append([]string{"mutants"}, run.Args...))
			wantStderr := run.Stderr
			if run.LoadError != nil {
				wantStderr = strings.ReplaceAll(wantStderr, "$LOADERROR", b.loadErrorText(t, run.LoadError.Package))
			}
			if status != run.Status {
				t.Errorf("status = %d, want %d", status, run.Status)
			}
			if got := b.said(b.stdout.String()); got != run.Stdout {
				t.Errorf("stdout =\n%s\nwant\n%s", got, run.Stdout)
			}
			if got := b.said(b.stderr.String()); got != wantStderr {
				t.Errorf("stderr =\n%s\nwant\n%s", got, wantStderr)
			}
			b.compareCalls(t, run.Calls)
			if after := fileTree(b.root); !slices.Equal(after, before) {
				t.Errorf("the module holds\n%q\nafter the run, want\n%q", after, before)
			}
			if left := fileTree(b.tmp); len(left) != 0 {
				t.Errorf("left in the temporary directory: %q", left)
			}
		})
	}
}

// refuseCall wraps a bench's runner so the program whose arguments hold every
// word given answers as planted, writing out first.
func refuseCall(b *mutantsBench, out string, status int, err error, words ...string) {
	answer := b.session.Run
	b.session.Run = func(ctx context.Context, call recipe.Call) (int, error) {
		for _, word := range words {
			if !slices.Contains(call.Args, word) {
				return answer(ctx, call)
			}
		}
		_, _ = call.Stdout.Write([]byte(out))
		return status, err
	}
}

// TestMutants_AProgramThatDoesNotAnswer_IsRefused verifies every way a program
// of the run can fail to answer before gremlins, and gremlins failing to
// start: each is a run that measured nothing, with the reason, and gremlins is
// never started on it.
func TestMutants_AProgramThatDoesNotAnswer_IsRefused(t *testing.T) {
	files := mutantsFixture(t)
	planted := errors.New("planted")
	cases := []struct {
		name   string
		pkg    string
		out    string
		status int
		err    error
		words  []string
		want   string
	}{
		{name: "go env", words: []string{"env"}, err: planted, want: "gremlins: go env failed: planted; refusing to measure\n"},
		{name: "go list -m", words: []string{"list", "-m"}, err: planted, want: "gremlins: go list -m failed: planted; refusing to measure\n"},
		{name: "go list", words: []string{golist.FilesFormat}, err: planted, want: "gremlins: go list could not list ./internal/pkg: planted; refusing to measure\n"},
		{name: "go list answers something else", words: []string{golist.FilesFormat}, out: "x\n", want: "gremlins: go list could not list ./internal/pkg: unexpected go list row: \"x\"; refusing to measure\n"},
		{name: "a pattern", pkg: "./...", words: []string{"nothing"}, want: "gremlins: ./... names 3 packages, and one is measured at a time; refusing to measure\n"},
		{name: "go mod download cannot start", words: []string{"mod"}, err: planted, want: "gremlins: go mod download could not be started: planted; refusing to measure\n"},
		{name: "go mod download fails", words: []string{"mod"}, status: 1, want: "gremlins: go mod download exited 1; refusing to measure\n"},
		{name: "go test cannot start", words: []string{"test"}, err: planted, want: "gremlins: go test could not be started: planted\ngremlins: ./internal/pkg does not pass its own tests"},
		{name: "gremlins cannot start", words: []string{"unleash"}, err: planted, want: "gremlins: gremlins could not be started: planted\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newMutantsBench(t, files)
			refuseCall(b, tc.out, tc.status, tc.err, tc.words...)
			pkg := tc.pkg
			if pkg == "" {
				pkg = "./internal/pkg"
			}
			status := b.session.run(t.Context(), []string{"mutants", pkg})
			if status != 1 || !strings.HasPrefix(b.stderr.String(), tc.want) {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, b.stderr.String(), tc.want)
			}
			for _, call := range b.calls {
				if call.Args[0] == "run" {
					t.Error("gremlins was started")
				}
			}
		})
	}
}

// TestMutants_ACoverpkgGoCannotResolve_IsRefusedAsSuch verifies that a
// -coverpkg with an empty entry, which go list refuses outright, is not the
// same answer as a pattern that resolved and missed the package, and that the
// refusal says which it was, below go's own words.
func TestMutants_ACoverpkgGoCannotResolve_IsRefusedAsSuch(t *testing.T) {
	b := newMutantsBench(t, mutantsFixture(t))
	b.prepare(t, mutantsRun{Env: map[string]string{"GREMLINS_FLAGS": "-i --coverpkg ./internal/pkg,,./cmd/tool"}, Files: map[string]*string{"internal/pkg/pkg_test.go": nil}})
	if status := b.session.run(t.Context(), []string{"mutants", "./internal/pkg"}); status != 1 {
		t.Errorf("status = %d, want 1", status)
	}
	said := b.stderr.String()
	if !strings.Contains(said, "gremlins: go list could not resolve -coverpkg ./internal/pkg,,./cmd/tool (its own message is above); refusing to measure\n") || strings.HasPrefix(said, "gremlins:") {
		t.Errorf("stderr = %q, want go's own message and then the refusal", said)
	}
	if strings.Contains(said, "does not name") || len(b.calls) != 0 {
		t.Errorf("stderr = %q after running %+v", said, b.calls)
	}
}

// TestMutants_ABaselineThatTookNoTime_IsRefused verifies that a clock that
// read no time passing derives no deadline.
func TestMutants_ABaselineThatTookNoTime_IsRefused(t *testing.T) {
	b := newMutantsBench(t, mutantsFixture(t))
	b.base = 0
	if status := b.session.run(t.Context(), []string{"mutants", "./internal/pkg"}); status != 1 ||
		b.stderr.String() != "gremlins: the timed baseline reads '0.000', which is not a positive number of seconds; refusing to derive a deadline from it\n" {
		t.Errorf("status = %d, stderr = %q", status, b.stderr.String())
	}
}

// TestMutants_NoTemporaryDirectory_IsRefused verifies that a profile that
// cannot be made ends the run with the reason.
func TestMutants_NoTemporaryDirectory_IsRefused(t *testing.T) {
	b := newMutantsBench(t, mutantsFixture(t))
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, filepath.Join(b.scratch, "absent"))
	}
	if status := b.session.run(t.Context(), []string{"mutants", "./internal/pkg"}); status != 1 || !strings.Contains(b.stderr.String(), "absent") {
		t.Errorf("status = %d, stderr = %q; want why no profile could be made", status, b.stderr.String())
	}
}

// TestMemoryCeiling_EachLimitInherited_IsKeptOrLowered verifies the ceiling
// gremlins is started under for each limit the run inherited: a looser one is
// lowered to the ceiling, one at it or under it is kept, and none is set where
// the limits cannot be read or the platform is not Linux, which the run says.
func TestMemoryCeiling_EachLimitInherited_IsKeptOrLowered(t *testing.T) {
	t.Parallel()
	const gib = 1 << 30
	cases := []struct {
		name string
		goos string
		held uint64
		err  error
		want uint64
		says string
	}{
		{"none", "linux", math.MaxUint64, nil, 8 * gib, "is held to 8 GiB of address space (MUTANT_MEMORY_MAX)"},
		{"a looser one", "linux", 16 * gib, nil, 8 * gib, "is held to 8 GiB of address space (MUTANT_MEMORY_MAX)"},
		{"one byte looser", "linux", 8*gib + 1, nil, 8 * gib, "is held to 8 GiB"},
		{"the ceiling", "linux", 8 * gib, nil, 0, "is already held to 8192 MiB of address space, the limit this run inherited"},
		{"a tighter one", "linux", 4 * gib, nil, 0, "is already held to 4096 MiB of address space, the limit this run inherited, which is no more than MUTANT_MEMORY_MAX=8 GiB and so is kept"},
		{"unreadable", "linux", 0, errors.New("planted"), 0, "MUTANT_MEMORY_MAX=8 cannot be set here (planted), so a mutant that allocates without end runs until the host stops it"},
		{"not Linux", "darwin", math.MaxUint64, nil, 0, "relies on Linux to enforce, so on darwin a mutant that allocates without end runs until the host stops it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &mutation{session: &session{}, knobs: knobs{memoryMax: 8}}
			m.goos = tc.goos
			m.limits = func() (soft, hard uint64, err error) { return tc.held, math.MaxUint64, tc.err }
			ceiling, said := m.memoryCeiling()
			if ceiling != tc.want || !strings.Contains(said, tc.says) {
				t.Errorf("memoryCeiling() = %d, %q; want %d and %q", ceiling, said, tc.want, tc.says)
			}
		})
	}
}
