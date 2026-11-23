// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// sonarRequest is one request a recorded run sent SonarCloud.
type sonarRequest struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
	User  string            `json:"user"`
}

// sonarCall is one program a recorded run started.
type sonarCall struct {
	Tool string   `json:"tool"`
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
}

// sonarRun is one run of scripts/sonar-scan.sh: what it was given, how its
// programs and SonarCloud answered, and what it did.
type sonarRun struct {
	Name          string            `json:"name"`
	Args          []string          `json:"args"`
	Dotenv        *string           `json:"dotenv"`
	Env           map[string]string `json:"env"`
	Gate          json.RawMessage   `json:"gate"`
	Measures      json.RawMessage   `json:"measures"`
	CE            []json.RawMessage `json:"ce"`
	Total         string            `json:"total"`
	Properties    string            `json:"properties"`
	GoStatus      int               `json:"go_status"`
	ScannerStatus int               `json:"scanner_status"`
	Status        int               `json:"status"`
	Stdout        string            `json:"stdout"`
	Stderr        string            `json:"stderr"`
	Requests      []sonarRequest    `json:"requests"`
	Calls         []sonarCall       `json:"calls"`
}

// recordedRuns reads testdata/sonar_runs.json.
func recordedRuns(t *testing.T) []sonarRun {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sonar_runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		Runs []sonarRun `json:"runs"`
	}
	if err = json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.Runs) == 0 {
		t.Fatal("the record holds no run")
	}
	return rec.Runs
}

// usual is the recorded run of the whole pipeline, which the cases the record
// does not cover depart from.
func usual(t *testing.T) sonarRun {
	t.Helper()
	runs := recordedRuns(t)
	at := slices.IndexFunc(runs, func(run sonarRun) bool { return run.Name == "the whole pipeline" })
	if at < 0 {
		t.Fatal("the record holds no run of the whole pipeline")
	}
	return runs[at]
}

// bench is a throwaway repository with a session over it, whose programs and
// SonarCloud answer as a run says.
type bench struct {
	root     string
	run      sonarRun
	server   *httptest.Server
	session  *session
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	calls    []sonarCall
	environs [][]string
	requests []sonarRequest
	waited   []time.Duration
	clock    time.Time
	// refuse, when it is set, answers a program in place of the bench, and
	// says whether it did.
	refuse func(call recipe.Call) (answered bool, status int, err error)
	// answer, when it is set, answers a request in place of the bench.
	answer func(w http.ResponseWriter, r *http.Request)
}

// newBench makes the repository, which reads no git configuration of the
// person running the tests.
func newBench(t *testing.T, run sonarRun) *bench {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &bench{root: root, run: run, clock: time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)}
	b.server = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.server.Close)
	environ := []string{"GIT_CONFIG_GLOBAL=" + filepath.Join(root, "absent"), "GIT_CONFIG_NOSYSTEM=1", "SONAR_HOST_URL=" + b.server.URL}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") && !strings.HasPrefix(entry, "SONAR") && !strings.HasPrefix(entry, "COVERAGE_") {
			environ = append(environ, entry)
		}
	}
	for name, value := range run.Env {
		environ = append(environ, name+"="+value)
	}
	b.session = &session{
		dir: root, environ: environ, stdout: &b.stdout, stderr: &b.stderr, goTool: golist.Executable(), run: b.program,
		lookPath: b.lookPath, client: b.server.Client(), now: func() time.Time { return b.clock },
		// A wait of nothing still moves the clock, so a run that polls without
		// pausing ends.
		sleep: func(wait time.Duration) {
			b.waited, b.clock = append(b.waited, wait), b.clock.Add(max(wait, time.Second))
		},
	}
	b.write(t, "sonar-project.properties", run.Properties)
	if run.Dotenv != nil {
		b.write(t, ".env", *run.Dotenv)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	identity := []string{"GIT_AUTHOR_NAME=f", "GIT_AUTHOR_EMAIL=f@example.com", "GIT_COMMITTER_NAME=f", "GIT_COMMITTER_EMAIL=f@example.com"}
	for _, args := range [][]string{{"init", "-q", "-b", "trunk"}, {"commit", "-q", "--allow-empty", "-m", "x"}} {
		if _, err = recipe.Output(t.Context(), recipe.Exec, recipe.Call{Program: git, Args: args, Dir: root, Env: append(environ, identity...), Stderr: os.Stderr}); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// write writes a file of the repository.
func (b *bench) write(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(b.root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// lookPath finds git where it is and sonar-scanner in the stand-ins.
func (b *bench) lookPath(name string) (string, error) {
	if name == "sonar-scanner" {
		return filepath.Join(b.root, "bin", name), nil
	}
	return exec.LookPath(name)
}

// program answers for go and sonar-scanner, and asks the real git.
func (b *bench) program(ctx context.Context, call recipe.Call) (int, error) {
	tool := strings.TrimSuffix(filepath.Base(call.Program), ".exe")
	if b.refuse != nil {
		if answered, status, err := b.refuse(call); answered {
			return status, err
		}
	}
	if tool == "git" {
		return recipe.Exec(ctx, call)
	}
	b.calls = append(b.calls, sonarCall{Tool: tool, Args: call.Args, Dir: call.Dir})
	b.environs = append(b.environs, call.Env)
	switch {
	case tool == "sonar-scanner":
		path := filepath.Join(call.Dir, ".scannerwork", "report-task.txt")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return 0, err
		}
		if err := os.WriteFile(path, []byte("projectKey=x\r\nceTaskUrl="+b.server.URL+"/api/ce/task?id=AB=C\r\n"), 0o600); err != nil {
			return 0, err
		}
		fmt.Fprintln(call.Stdout, "scanned")
		return b.run.ScannerStatus, nil
	case call.Args[0] == "test":
		fmt.Fprintln(call.Stdout, "ok  \tpkg")
		return b.run.GoStatus, nil
	default:
		fmt.Fprintf(call.Stdout, "pkg/a.go:1:\tF\t100.0%%\ntotal:\t\t\t(statements)\t%s%%\n", b.run.Total)
		return 0, nil
	}
}

// serve answers for SonarCloud.
func (b *bench) serve(w http.ResponseWriter, r *http.Request) {
	user, _, _ := r.BasicAuth()
	query := map[string]string{}
	for name, values := range r.URL.Query() {
		query[name] = values[0]
	}
	b.requests = append(b.requests, sonarRequest{Path: r.URL.Path, Query: query, User: user + ":"})
	if b.answer != nil {
		b.answer(w, r)
		return
	}
	body := b.run.CE[0]
	switch r.URL.Path {
	case "/api/qualitygates/project_status":
		body = b.run.Gate
	case "/api/measures/component":
		body = b.run.Measures
	default:
		if len(b.run.CE) > 1 {
			b.run.CE = b.run.CE[1:]
		}
	}
	_, _ = w.Write(body)
}

// said is how text reads with this bench's own addresses written as the record
// writes them.
func (b *bench) said(text string) string {
	for _, path := range []struct{ from, to string }{{b.server.URL, "$HOST"}, {b.root, "$ROOT"}, {filepath.ToSlash(b.root), "$ROOT"}} {
		text = strings.ReplaceAll(text, path.from, path.to)
	}
	return text
}

// TestRecordedRuns_TheCommand_DoesWhatTheScriptDid replays every recorded run
// of scripts/sonar-scan.sh and holds the command to it: the status, both
// streams, every program with its arguments and directory, and every request
// with its query and the user it is made as.
func TestRecordedRuns_TheCommand_DoesWhatTheScriptDid(t *testing.T) {
	t.Parallel()
	for _, run := range recordedRuns(t) {
		t.Run(run.Name, func(t *testing.T) {
			t.Parallel()
			b := newBench(t, run)
			if status := b.session.main(t.Context(), run.Args); status != run.Status {
				t.Errorf("status = %d, want %d", status, run.Status)
			}
			if got := b.said(b.stdout.String()); got != run.Stdout {
				t.Errorf("stdout =\n%s\nwant\n%s", got, run.Stdout)
			}
			if got := b.said(b.stderr.String()); got != run.Stderr {
				t.Errorf("stderr =\n%s\nwant\n%s", got, run.Stderr)
			}
			for i := range b.calls {
				b.calls[i].Dir = filepath.ToSlash(b.said(b.calls[i].Dir))
				b.calls[i].Args = slices.Clone(b.calls[i].Args)
				for j, arg := range b.calls[i].Args {
					b.calls[i].Args[j] = b.said(arg)
				}
			}
			if len(run.Calls)+len(b.calls) > 0 && !reflect.DeepEqual(b.calls, run.Calls) {
				t.Errorf("ran\n%+v\nwant\n%+v", b.calls, run.Calls)
			}
			if len(run.Requests)+len(b.requests) > 0 && !reflect.DeepEqual(b.requests, run.Requests) {
				t.Errorf("sent\n%+v\nwant\n%+v", b.requests, run.Requests)
			}
		})
	}
}

// TestMain_ACommandLineItCannotRead_IsAUsageError verifies that a flag or an
// argument the command does not know runs nothing, where the script took it
// for a full scan.
func TestMain_ACommandLineItCannotRead_IsAUsageError(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"-other"}, {"now"}, {"-no-scan", "now"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			b := newBench(t, usual(t))
			if status := b.session.main(t.Context(), args); status != 2 || b.stderr.Len() == 0 || b.stdout.Len()+len(b.calls)+len(b.requests) != 0 {
				t.Errorf("status = %d, stderr %q, ran %+v", status, b.stderr.String(), b.calls)
			}
		})
	}
}

// TestMain_AnUnknownFlag_ExitsWithTheUsageStatus verifies that main hands the
// process arguments after its own name to the session production runs, and
// passes the status on to the exit seam.
func TestMain_AnUnknownFlag_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit, previousStderr := os.Args, osExit, os.Stderr
	t.Cleanup(func() { os.Args, osExit, os.Stderr = previousArgs, previousExit, previousStderr })
	silent, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.Close() }()
	os.Stderr = silent
	got := -1
	os.Args = []string{"sonar_scan", "-other"}
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// TestDefaultSession_IsThisProcess verifies the session production runs: this
// process's directory, environment and streams, real tools, and a client whose
// requests are bounded.
func TestDefaultSession_IsThisProcess(t *testing.T) {
	t.Parallel()
	s := defaultSession()
	if s.dir != "." || s.goTool != golist.Executable() || len(s.environ) == 0 || s.stdout != os.Stdout || s.stderr != os.Stderr {
		t.Errorf("defaultSession() = %+v", s)
	}
	if s.run == nil || s.lookPath == nil || s.now == nil || s.sleep == nil || s.client.Timeout != requestTimeout {
		t.Errorf("defaultSession() leaves a tool unset or a request unbounded: %+v", s)
	}
}

// TestPipeline_WhatDotenvSets_ReachesTheScannerAlone verifies that the tests
// run with the environment the command was given, as CI runs them, and that
// sonar-scanner is given what .env sets besides.
func TestPipeline_WhatDotenvSets_ReachesTheScannerAlone(t *testing.T) {
	t.Parallel()
	run := usual(t)
	dotenv := "SONARQUBE_TOKEN=tok\nGITLAB_TOKEN=secret\n"
	run.Dotenv = &dotenv
	b := newBench(t, run)
	if status := b.session.main(t.Context(), nil); status != 0 || len(b.calls) != 3 {
		t.Fatalf("status = %d after %+v; stderr %q", status, b.calls, b.stderr.String())
	}
	for i, want := range []string{"", "", "secret"} {
		t.Run(b.calls[i].Tool, func(t *testing.T) {
			t.Parallel()
			if got, _ := recipe.Getenv(b.environs[i], "GITLAB_TOKEN"); got != want {
				t.Errorf("%s ran with GITLAB_TOKEN=%q, want %q", b.calls[i].Tool, got, want)
			}
		})
	}
}

// TestPipeline_AStepThatCannotBeDone_SaysWhy verifies every step that can
// fail otherwise than by a program's own status: each is an error of one line,
// and nothing is done after it.
func TestPipeline_AStepThatCannotBeDone_SaysWhy(t *testing.T) {
	t.Parallel()
	planted := errors.New("planted")
	failing := func(words ...string) func(call recipe.Call) (bool, int, error) {
		return func(call recipe.Call) (bool, int, error) {
			return slices.Contains(append([]string{filepath.Base(call.Program)}, call.Args...), words[0]), 0, planted
		}
	}
	cases := []struct {
		name    string
		prepare func(b *bench)
		want    string
	}{
		{"the tests cannot start", func(b *bench) { b.refuse = failing("test") }, "ERROR: planted\n"},
		{"the total cannot be asked", func(b *bench) { b.refuse = failing("cover") }, "ERROR: planted\n"},
		{"there is no total", func(b *bench) { b.run.Total = "" }, "ERROR: go tool cover gave no total\n"},
		{"there is no scanner", func(b *bench) {
			b.session.lookPath = func(name string) (string, error) {
				if name == "git" {
					return exec.LookPath(name)
				}
				return "", planted
			}
		}, "ERROR: find sonar-scanner: planted\n"},
		{"the scanner cannot start", func(b *bench) { b.refuse = failing("sonar-scanner") }, "ERROR: planted\n"},
		{"the scanner left no report", func(b *bench) {
			b.refuse = func(call recipe.Call) (bool, int, error) {
				return filepath.Base(call.Program) == "sonar-scanner", 0, nil
			}
		}, "ERROR: open "},
		{"the task cannot be asked", func(b *bench) {
			b.answer = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }
		}, "ERROR: /api/ce/task answered 401 Unauthorized\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBench(t, usual(t))
			tc.prepare(b)
			if status := b.session.main(t.Context(), nil); status != 1 || !strings.HasPrefix(b.stderr.String(), tc.want) {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, b.stderr.String(), tc.want)
			}
			if strings.Contains(b.stdout.String(), "[4/4]") {
				t.Errorf("stdout = %q, want no gate fetched", b.stdout.String())
			}
		})
	}
}

// TestAwaitTask_ThePolling_IsBoundedAndSpaced verifies that the task is looked
// at again after the interval, at least once whatever the timeout, and no more
// once the time it is given is up.
func TestAwaitTask_ThePolling_IsBoundedAndSpaced(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		env  map[string]string
		want []time.Duration
		says string
	}{
		{"the defaults", nil, slices.Repeat([]time.Duration{5 * time.Second}, 48), "ERROR: timed out after 240s waiting for CE task\n"},
		{"its own", map[string]string{"SONAR_POLL_TIMEOUT": "4", "SONAR_POLL_INTERVAL": "3"}, slices.Repeat([]time.Duration{3 * time.Second}, 2), "ERROR: timed out after 4s waiting for CE task\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			run := usual(t)
			run.Env, run.CE = tc.env, []json.RawMessage{json.RawMessage(`{"task":{"status":"PENDING"}}`)}
			b := newBench(t, run)
			if status := b.session.main(t.Context(), nil); status != 1 || b.stderr.String() != tc.says {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, b.stderr.String(), tc.says)
			}
			if !slices.Equal(b.waited, tc.want) || len(b.requests) != len(tc.want)+1 {
				t.Errorf("waited %v over %d look(s), want %v", b.waited, len(b.requests), tc.want)
			}
		})
	}
}

// TestProperty_EachLine_IsReadToItsEnd verifies that a property is read whole
// from the line that opens with its name, a carriage return dropped, and that
// a file without one is an error.
func TestProperty_EachLine_IsReadToItsEnd(t *testing.T) {
	t.Parallel()
	file := []byte("# a=b\nab=no\na=x=y\r\na=later")
	for name, want := range map[string]string{"a": "x=y", "ab": "no"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := property(file, name); err != nil || got != want {
				t.Errorf("property(%q) = %q, %v; want %q", name, got, err, want)
			}
		})
	}
	if got, err := property(file, "b"); err == nil || got != "" {
		t.Errorf("property(b) = %q, %v; want an error", got, err)
	}
}

// TestMakefile_TheTargets_RunTheCommand verifies that both make targets run
// this command.
func TestMakefile_TheTargets_RunTheCommand(t *testing.T) {
	t.Parallel()
	makefile, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\nsonar:\n\t@go run ./cmd/sonar_scan\n", "\nsonar-status:\n\t@go run ./cmd/sonar_scan -no-scan\n"} {
		t.Run(strings.Fields(want)[0], func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(string(makefile), want) {
				t.Errorf("the Makefile does not hold %q", want)
			}
		})
	}
}
