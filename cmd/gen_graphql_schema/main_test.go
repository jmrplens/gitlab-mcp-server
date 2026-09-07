package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

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
	if _, _, err := readArtifacts(dir); err != nil {
		t.Errorf("the artifacts it wrote do not read back: %v", err)
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

// TestRun_CheckMode_JudgesTheCommittedFilesWithoutNetwork verifies the CI half.
// It must need no instance at all, because a gate that reaches gitlab.com is a
// gate that fails when gitlab.com does.
func TestRun_CheckMode_JudgesTheCommittedFilesWithoutNetwork(t *testing.T) {
	sound := filepath.Join(t.TempDir(), "sound")
	if err := writeArtifacts(sound, minimalSDL, canonicalSource); err != nil {
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

// canonicalSource is a provenance record that satisfies every requirement
// [pinProblems] enforces, so a test about something else is not tripped by the
// identity checks.
var canonicalSource = graphqlschema.Source{
	Instance:       defaultEndpoint,
	GitLabVersion:  "19.4.0",
	GitLabRevision: "abc1234",
	RetrievedAt:    "2026-09-06",
	Types:          minimumTypes + 1,
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
			source: withSource(func(s *graphqlschema.Source) { s.Types = minimumTypes - 1 }),
			want:   "truncated or the instance was a narrower edition",
		},
		{
			name:   "an introspection with no token",
			source: withSource(func(s *graphqlschema.Source) { s.GitLabVersion = unknownVersion }),
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
			want:   "",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "pinned")
			if err := writeArtifacts(dir, minimalSDL, testCase.source); err != nil {
				t.Fatalf("prepare the fixture: %v", err)
			}

			status, _, errOut := runCommand(t, genRun{dir: dir, check: true, now: fixedClock})

			if testCase.want == "" {
				// An unreadable date is left to the record's own decoding, which
				// has already accepted it, rather than becoming a second
				// complaint about the same field.
				if status != 0 {
					t.Fatalf("exit status %d, want 0 for a date the age check cannot read:\n%s", status, errOut)
				}
				return
			}
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
