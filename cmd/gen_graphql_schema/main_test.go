package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// tinySchema is the smallest introspection payload that renders to loadable
// SDL: one object type that is also the query root.
const tinySchema = `{"data":{"__schema":{
  "queryType":{"name":"Query"},
  "types":[{"kind":"OBJECT","name":"Query","fields":[{"name":"ok","args":[],"type":{"kind":"SCALAR","name":"Boolean"}}]}]
}}}`

// fixedClock is the day a generated provenance record is asserted against.
func fixedClock() time.Time { return time.Date(2026, 9, 6, 11, 30, 0, 0, time.UTC) }

// runCommand runs the command with both streams captured.
func runCommand(t *testing.T, cfg genRun) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(cfg, &out, &errOut)
	return status, out.String(), errOut.String()
}

// TestRun_AgainstAnInstance_WritesBothArtifacts verifies a whole generation:
// introspect, convert, load what was converted, and write the pair with the
// provenance the instance reported.
func TestRun_AgainstAnInstance_WritesBothArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if strings.Contains(string(body), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.4.0","revision":"abc1234"}}}`))
			return
		}
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(server.Close)
	dir := filepath.Join(t.TempDir(), "pinned")

	status, out, errOut := runCommand(t, genRun{endpoint: server.URL, dir: dir, client: server.Client(), now: fixedClock})

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	for _, want := range []string{"introspecting", "wrote gitlab-schema.graphql", "GitLab 19.4.0", "retrieved 2026-09-06"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out)
			}
		})
	}
	types, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("the artifacts it wrote do not read back: %v", err)
	}
	// The record's count is the loaded one, so that --check can recompute it
	// from the schema on disk and refuse a record beside a schema it did not
	// come from. Recording the introspected count instead would pass a whole
	// schema and fail every synthetic one, and bind nothing either way.
	if types != source.Types {
		t.Errorf("the record says %d types and the schema loads with %d: the two files it wrote disagree", source.Types, types)
	}
}

// TestRun_GenerationFailures_ExitNonZeroAndSayWhy verifies that nothing is
// committed silently: each stage that can fail reports on stderr and stops.
func TestRun_GenerationFailures_ExitNonZeroAndSayWhy(t *testing.T) {
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"introspection is disabled"}]}`))
	}))
	t.Cleanup(refusing.Close)

	// A schema whose only type is not an operation root renders SDL that
	// parses as definitions and fails to load as a schema, which is the
	// conversion failure the round-trip check exists to catch.
	rootless := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"__schema":{"queryType":{"name":"Missing"},"types":[
		  {"kind":"OBJECT","name":"Thing","fields":[{"name":"id","args":[],"type":{"kind":"SCALAR","name":"ID"}}]}]}}}`))
	}))
	t.Cleanup(rootless.Close)

	answering := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(answering.Close)

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("a file where a directory should be"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	cases := []struct {
		name string
		cfg  genRun
		want string
	}{
		{
			name: "the instance refuses introspection",
			cfg:  genRun{endpoint: refusing.URL, dir: t.TempDir(), client: refusing.Client(), now: fixedClock},
			want: "introspection is disabled",
		},
		{
			name: "the conversion produces something that is not a schema",
			cfg:  genRun{endpoint: rootless.URL, dir: t.TempDir(), client: rootless.Client(), now: fixedClock},
			want: "the converted schema does not parse",
		},
		{
			name: "the directory cannot be written",
			cfg:  genRun{endpoint: answering.URL, dir: filepath.Join(blocked, "under"), client: answering.Client(), now: fixedClock},
			want: "create ",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, _, errOut := runCommand(t, testCase.cfg)

			if status != 1 {
				t.Fatalf("exit status %d, want 1", status)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("stderr does not contain %q:\n%s", testCase.want, errOut)
			}
		})
	}
}

// TestRun_Generation_ATruncatedAnswer_DoesNotReplaceAWholePin verifies that a
// regeneration cannot destroy the committed pin and report success.
//
// The floor alone cannot tell a truncation from a narrower edition, so probing
// such an instance into an empty directory stays allowed: the SDL it writes is
// what `-schema` reads. What must not happen is the same answer overwriting a
// pin that already cleared the floor, which is a working tree that has lost its
// guarantee for a command that exited 0. `--check` would refuse the result, but
// only after the good pin was already gone.
func TestRun_Generation_ATruncatedAnswer_DoesNotReplaceAWholePin(t *testing.T) {
	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(truncated.Close)

	dir := filepath.Join(t.TempDir(), "pinned")
	if err := writeArtifacts(dir, minimalSDL, canonicalSource); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	status, _, errOut := runCommand(t, genRun{
		endpoint: truncated.URL, dir: dir, client: truncated.Client(), now: fixedClock,
	})

	if status != 1 {
		t.Fatalf("exit status %d, want 1 for an answer that would replace a whole pin:\n%s", status, errOut)
	}
	if !strings.Contains(errOut, "refusing to replace a whole schema with a truncated answer") {
		t.Errorf("stderr does not say what it refused:\n%s", errOut)
	}

	_, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("the pin is no longer readable after a refused generation: %v", err)
	}
	if source.Types != canonicalSource.Types {
		t.Errorf("the pin was replaced anyway: %d types, want the %d it had", source.Types, canonicalSource.Types)
	}
}

// TestRun_CheckMode_JudgesTheCommittedFilesWithoutNetwork verifies the CI half.
// It must need no instance at all, because a gate that reaches gitlab.com is a
// gate that fails when gitlab.com does.
func TestRun_CheckMode_JudgesTheCommittedFilesWithoutNetwork(t *testing.T) {
	sound := filepath.Join(t.TempDir(), "sound")
	if err := writeArtifacts(sound, wholeSDL, canonicalSource); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	t.Run("a pair that parses", func(t *testing.T) {
		status, out, errOut := runCommand(t, genRun{dir: sound, check: true, now: fixedClock})

		if status != 0 {
			t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
		}
		if !strings.Contains(out, "the pinned schema parses") || !strings.Contains(out, "GitLab 19.4.0") {
			t.Errorf("stdout does not report the pin:\n%s", out)
		}
	})

	t.Run("a directory with nothing in it", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{dir: t.TempDir(), check: true, now: fixedClock})

		if status != 1 {
			t.Fatalf("exit status %d, want 1", status)
		}
		if !strings.Contains(errOut, "gen_graphql_schema:") {
			t.Errorf("stderr does not name the command:\n%s", errOut)
		}
	})

	t.Run("no clock supplied", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{dir: sound, check: true})

		// The age check needs a clock and main is the only caller with one to
		// give, so the default has to hold for every other caller. It cannot be
		// asserted as a pass, since this fixture ages past the window in 2027,
		// only that nothing panics and any refusal is about age.
		if status != 0 && !strings.Contains(errOut, "days old") {
			t.Errorf("exit status %d for a reason other than age:\n%s", status, errOut)
		}
	})
}

// wholeSDL is a schema large enough to clear the floor --check holds a pin to,
// so a check-mode test is refused only for the reason it is about. Its types
// are trivial because the floor counts them and reads nothing else.
var wholeSDL = func() string {
	var sdl strings.Builder
	sdl.WriteString("type Query {\n  ok: Boolean\n}\n\n")
	for i := range graphqlintrospect.MinimumTypes {
		fmt.Fprintf(&sdl, "type Padding%d {\n  ok: Boolean\n}\n\n", i)
	}
	sdl.WriteString("schema {\n  query: Query\n}\n")
	return sdl.String()
}()

// canonicalSource is a provenance record that satisfies every requirement
// [pinProblems] enforces, so a test about something else is not tripped by the
// identity checks. Its count is the one wholeSDL loads with, because --check
// holds the record to the schema beside it.
var canonicalSource = graphqlschema.Source{
	Instance:       defaultEndpoint,
	GitLabVersion:  "19.4.0",
	GitLabRevision: "abc1234",
	RetrievedAt:    "2026-09-06",
	Types:          loadedTypes(wholeSDL),
}

// loadedTypes is the count --check recomputes for a schema on disk.
func loadedTypes(sdl string) int {
	return len(cmdutil.Must(graphqlschema.Load([]byte(sdl))).Types)
}

// TestRun_CheckMode_RefusesAPinOfSomethingElse verifies the checks that ask
// what the pin is a pin of.
//
// A schema that parses says nothing about which instance answered, how complete
// the answer was, or how long ago: until these existed, a run against a
// self-managed instance, or one without a token, wrote a narrower or anonymous
// pin that every gate in the repository accepted in silence, and the guarantee
// the whole gate rests on could be swapped out by one flag.
func TestRun_CheckMode_RefusesAPinOfSomethingElse(t *testing.T) {
	cases := []struct {
		name   string
		source graphqlschema.Source
		want   string
	}{
		{
			name:   "another instance",
			source: withSource(func(s *graphqlschema.Source) { s.Instance = "https://gitlab.gnome.org/api/graphql" }),
			want:   "not https://gitlab.com/api/graphql",
		},
		{
			name:   "a truncated or narrower answer",
			source: withSource(func(s *graphqlschema.Source) { s.Types = graphqlintrospect.MinimumTypes - 1 }),
			want:   "truncated or the instance was a narrower edition",
		},
		{
			name:   "an introspection with no token",
			source: withSource(func(s *graphqlschema.Source) { s.GitLabVersion = graphqlintrospect.UnknownVersion }),
			want:   "records no GitLab version",
		},
		{
			name:   "an introspection whose record was emptied by hand",
			source: withSource(func(s *graphqlschema.Source) { s.GitLabVersion = "" }),
			want:   "records no GitLab version",
		},
		{
			name:   "a pin past the window",
			source: withSource(func(s *graphqlschema.Source) { s.RetrievedAt = "2020-01-01" }),
			want:   "days old and the window is",
		},
		{
			name:   "a date nothing can read",
			source: withSource(func(s *graphqlschema.Source) { s.RetrievedAt = "one tuesday" }),
			want:   "is not a date",
		},
		{
			name:   "a date that has not happened",
			source: withSource(func(s *graphqlschema.Source) { s.RetrievedAt = "2026-09-07" }),
			want:   "has not happened yet",
		},
		{
			name:   "a record beside a schema it did not come from",
			source: withSource(func(s *graphqlschema.Source) { s.Types++ }),
			want:   "were not written by one regeneration",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "pinned")
			if err := writeArtifacts(dir, wholeSDL, testCase.source); err != nil {
				t.Fatalf("prepare the fixture: %v", err)
			}

			status, _, errOut := runCommand(t, genRun{dir: dir, check: true, now: fixedClock})

			if status != 1 {
				t.Fatalf("exit status %d, want 1. stderr:\n%s", status, errOut)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("stderr does not explain the refusal %q:\n%s", testCase.want, errOut)
			}
			if !strings.Contains(errOut, "make gen-graphql-schema") {
				t.Errorf("stderr does not say how to fix it:\n%s", errOut)
			}
		})
	}
}

// withSource returns canonicalSource with one field spoiled, so each case names
// only the thing it is about.
func withSource(spoil func(*graphqlschema.Source)) graphqlschema.Source {
	source := canonicalSource
	spoil(&source)
	return source
}

// TestRun_Generation_WarnsWhenThePinIsNotOfGitLabCom verifies that the person
// who ran a non-canonical generation is told at once. The same facts fail in
// CI, and learning them an hour later from a red pipeline is the worse of the
// two ways to find out.
func TestRun_Generation_WarnsWhenThePinIsNotOfGitLabCom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if strings.Contains(string(body), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":null}}`))
			return
		}
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(server.Close)

	status, _, errOut := runCommand(t, genRun{
		endpoint: server.URL, dir: filepath.Join(t.TempDir(), "pinned"),
		client: server.Client(), now: fixedClock,
	})

	if status != 0 {
		t.Fatalf("exit status %d, want 0: a warning must not stop a deliberate probe.\n%s", status, errOut)
	}
	for _, want := range []string{"not https://gitlab.com/api/graphql", "records no GitLab version", "narrower edition"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not warn %q:\n%s", want, errOut)
			}
		})
	}
}

// runMainCapturing drives main with args as the command line and returns the
// status it handed the process along with what it wrote to each stream.
//
// main is the only place that reads the process itself: the flag names, os.Args,
// the environment and the two real streams. Each is restored before the helper
// returns, and the streams are pointed at files rather than a pipe, so no amount
// of output can fill a pipe buffer and deadlock the test.
func runMainCapturing(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	stdout, stderr := captureFile(t, "stdout"), captureFile(t, "stderr")

	originalArgs, originalExit, originalFlags := os.Args, osExit, flag.CommandLine
	originalOut, originalErr := os.Stdout, os.Stderr
	defer func() {
		os.Args, osExit, flag.CommandLine = originalArgs, originalExit, originalFlags
		os.Stdout, os.Stderr = originalOut, originalErr
	}()

	os.Args = append([]string{"gen_graphql_schema"}, args...)
	os.Stdout, os.Stderr = stdout, stderr
	// main registers its flags on the package-level set, so a second call would
	// redefine the flags the first one left there and panic. ContinueOnError
	// keeps a flag this test got wrong a readable failure instead of an
	// os.Exit(2) from inside the flag package, which no seam here covers.
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	status, exited := 0, false
	osExit = func(code int) { status, exited = code, true }

	main()

	if !exited {
		t.Fatal("main returned without reaching osExit: the process would exit 0 whatever run reported")
	}
	return status, readCapture(t, stdout), readCapture(t, stderr)
}

// captureFile is one of the streams main is given in place of the process's own.
func captureFile(t *testing.T, name string) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("create the %s capture: %v", name, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// readCapture reads back what main wrote to one of those streams.
func readCapture(t *testing.T, file *os.File) string {
	t.Helper()
	written, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("read back %s: %v", file.Name(), err)
	}
	return string(written)
}

// pinnedDir writes a sound pin whose record is dated today, because main
// supplies time.Now: a fixture with a fixed date would pass until the window
// closed on it and then fail for a reason no test here is about.
func pinnedDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pinned")
	source := withSource(func(s *graphqlschema.Source) {
		s.RetrievedAt = time.Now().UTC().Format(time.DateOnly)
	})
	if err := writeArtifacts(dir, wholeSDL, source); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	return dir
}

// TestMainEntry_CheckMode_ReportsThePinAndHandsBackTheStatus verifies the
// command line the Makefile actually runs: --check names the offline half, -dir
// names where the pin is, the report reaches the process's stdout, and the
// status run returned is the status the process exits with.
//
// The flag names are the assertion that matters as much as the status. They are
// spelled in the Makefile and in CI, so renaming one here breaks a gate in a
// place nothing else in this package can see.
func TestMainEntry_CheckMode_ReportsThePinAndHandsBackTheStatus(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")

	t.Run("a pin that passes", func(t *testing.T) {
		status, out, errOut := runMainCapturing(t, "--check", "-dir", pinnedDir(t))

		if status != 0 {
			t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
		}
		if !strings.Contains(out, "the pinned schema parses") || !strings.Contains(out, "GitLab 19.4.0") {
			t.Errorf("stdout does not report the pin:\n%s", out)
		}
		if errOut != "" {
			t.Errorf("a passing check wrote to stderr:\n%s", errOut)
		}
	})

	t.Run("a directory with no pin in it", func(t *testing.T) {
		status, out, errOut := runMainCapturing(t, "--check", "-dir", t.TempDir())

		if status != 1 {
			t.Fatalf("exit status %d, want 1: a refusal main swallowed is a gate that passes", status)
		}
		if !strings.Contains(errOut, prefix) {
			t.Errorf("stderr does not name the command:\n%s", errOut)
		}
		if out != "" {
			t.Errorf("a refused check wrote to stdout:\n%s", out)
		}
	})
}

// TestMainEntry_CredentialResolution_FollowsGITLABURLNotTheFlag verifies that
// the token is judged against the instance GITLAB_URL names rather than followed
// to whatever -url points at, and that the withholding is said out loud.
//
// -url takes an arbitrary endpoint, so this is the one decision in main that is
// not plumbing: a token resolved against the flag instead of the environment is
// a credential handed to an instance nobody named. A note that is not printed is
// the same defect one step quieter, since the version then reads "unknown" with
// nothing saying why.
func TestMainEntry_CredentialResolution_FollowsGITLABURLNotTheFlag(t *testing.T) {
	const instance = "https://gitlab.example.com"

	cases := []struct {
		name      string
		instance  string
		endpoint  []string
		wantNote  string
		wantQuiet bool
	}{
		{
			name:     "an endpoint the token does not belong to",
			instance: instance,
			// No -url, so the default gitlab.com endpoint is asked while the
			// token belongs somewhere else.
			wantNote: "GITLAB_TOKEN belongs to https://gitlab.example.com",
		},
		{
			name:     "a token nothing says the instance of",
			instance: "",
			wantNote: "GITLAB_TOKEN is set and GITLAB_URL is not",
		},
		{
			name:      "the endpoint the token belongs to",
			instance:  instance,
			endpoint:  []string{"-url", instance + "/api/graphql"},
			wantQuiet: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testCase.instance)
			t.Setenv("GITLAB_TOKEN", "glpat-not-a-real-token")
			args := append([]string{"--check", "-dir", pinnedDir(t)}, testCase.endpoint...)

			status, _, errOut := runMainCapturing(t, args...)

			// Check mode sends nothing anywhere, so the note is the whole
			// observable difference and must not change the outcome.
			if status != 0 {
				t.Fatalf("exit status %d, want 0: resolving a credential is not a reason to refuse. stderr:\n%s", status, errOut)
			}
			if testCase.wantQuiet {
				if strings.Contains(errOut, "note:") {
					t.Errorf("the token was withheld from the instance it belongs to:\n%s", errOut)
				}
				return
			}
			if !strings.Contains(errOut, prefix+" note:") {
				t.Errorf("stderr does not carry the withholding note:\n%s", errOut)
			}
			if !strings.Contains(errOut, testCase.wantNote) {
				t.Errorf("stderr does not say why the token was withheld (%q):\n%s", testCase.wantNote, errOut)
			}
		})
	}
}

// TestRun_CheckMode_AcceptsTheCommittedRepositoryArtifacts is the gate itself,
// run against the real files rather than a fixture: what CI asserts on every
// push is asserted here on every test run.
func TestRun_CheckMode_AcceptsTheCommittedRepositoryArtifacts(t *testing.T) {
	status, out, errOut := runCommand(t, genRun{dir: filepath.Join("..", "..", defaultDir), check: true})

	if status != 0 {
		t.Fatalf("the committed schema does not pass its own gate (exit %d):\n%s", status, errOut)
	}
	if !strings.Contains(out, "gitlab.com") {
		t.Errorf("the committed pin does not name gitlab.com:\n%s", out)
	}
}
