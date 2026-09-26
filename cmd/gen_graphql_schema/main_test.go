package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
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
	// The whole line, because it names two places and two counts, one of each
	// per side, and a sentence reporting either pair the wrong way round sends
	// the reader to the half that was right. tinySchema answers with one type.
	wantRefusal := fmt.Sprintf(
		"gen_graphql_schema: %s answered with 1 types and the pin in %s carries %d: refusing to replace a whole schema with a truncated answer\n",
		truncated.URL, dir, canonicalSource.Types,
	)
	if errOut != wantRefusal {
		t.Errorf("stderr = %q, want exactly %q", errOut, wantRefusal)
	}

	_, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("the pin is no longer readable after a refused generation: %v", err)
	}
	if source.Types != canonicalSource.Types {
		t.Errorf("the pin was replaced anyway: %d types, want the %d it had", source.Types, canonicalSource.Types)
	}
}

// wholeAnswer is an introspection payload that clears the floor the truncation
// guard applies, so a generation can be driven down the path a real pin of
// gitlab.com takes. Every other generation test here answers with [tinySchema]
// and therefore enters that guard, which left the branch for a whole answer
// evaluated one way only: the refusal could have become unconditional with
// nothing to say so. The types are trivial because the guard counts them and
// reads nothing else.
var wholeAnswer = func() string {
	var payload strings.Builder
	payload.WriteString(`{"data":{"__schema":{"queryType":{"name":"Query"},"types":[`)
	payload.WriteString(introspectedObject("Query"))
	for i := range graphqlintrospect.MinimumTypes {
		payload.WriteString(",")
		payload.WriteString(introspectedObject(fmt.Sprintf("Padding%d", i)))
	}
	payload.WriteString(`]}}}`)
	return payload.String()
}()

// introspectedObject is one object type carrying one scalar field, which is the
// smallest thing the SDL renderer emits a type block for.
func introspectedObject(name string) string {
	return fmt.Sprintf(`{"kind":"OBJECT","name":%q,"fields":[{"name":"ok","args":[],"type":{"kind":"SCALAR","name":"Boolean"}}]}`, name)
}

// TestRun_Generation_AWholeAnswer_WritesEveryFieldOfThePin verifies the whole
// generation this command exists for: an answer that clears the floor is
// converted, loaded and written without the truncation guard firing.
//
// It compares the record field by field rather than asserting one field of it,
// because the values a converter assigns have no branch for a gate to flip. The
// revision is the one that was loose: [graphqlschema.Source.String] does not
// print it and no check consults it, so an assignment that spoiled or dropped it
// was invisible to every test here. The bearer credential is asserted for the
// same reason (the token reaching the instance is one field of a struct
// literal) and both requests are counted, since the version is a second ask and a
// credential withheld from it silently records "unknown".
func TestRun_Generation_AWholeAnswer_WritesEveryFieldOfThePin(t *testing.T) {
	const credential = "glpat-probe-credential"

	var mu sync.Mutex
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		mu.Unlock()
		if strings.Contains(string(body), "metadata") {
			_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.5.1","revision":"def5678"}}}`))
			return
		}
		_, _ = w.Write([]byte(wholeAnswer))
	}))
	t.Cleanup(server.Close)
	dir := filepath.Join(t.TempDir(), "pinned")

	status, out, errOut := runCommand(t, genRun{
		endpoint: server.URL, dir: dir, token: credential, client: server.Client(), now: fixedClock,
	})

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	types, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("the artifacts it wrote do not read back: %v", err)
	}
	want := graphqlschema.Source{
		Instance:       server.URL,
		GitLabVersion:  "19.5.1",
		GitLabRevision: "def5678",
		RetrievedAt:    "2026-09-06",
		Types:          types,
	}
	// %#v rather than %+v: Source is a Stringer and its line omits the
	// revision, so the one field this comparison exists for would be missing
	// from the message reporting that it differs.
	if source != want {
		t.Errorf("the record it wrote = %#v, want %#v", source, want)
	}

	mu.Lock()
	asked := slices.Clone(authorizations)
	mu.Unlock()
	if wantAsked := []string{"Bearer " + credential, "Bearer " + credential}; !slices.Equal(asked, wantAsked) {
		t.Errorf("the instance was asked with %q, want the credential on the introspection and the version alike", asked)
	}

	// The report is compared whole. Its two numbers are a size and a count, in
	// that order, and it names the endpoint asked and both files written: each
	// pair is the same Go type, so reporting one in the other's place reads as
	// a plausible line about a different run.
	sdl, err := os.ReadFile(filepath.Join(dir, graphqlschema.SDLFileName))
	if err != nil {
		t.Fatalf("read the schema back: %v", err)
	}
	wantOut := fmt.Sprintf("gen_graphql_schema: introspecting %s\n"+
		"gen_graphql_schema: wrote gitlab-schema.graphql (%d KiB) and source.json; %s, %d loaded\n",
		server.URL, len(sdl)/1024, want, types)
	if out != wantOut {
		t.Errorf("stdout = %q, want exactly %q", out, wantOut)
	}

	// The endpoint is not gitlab.com, so that warning is expected; a whole
	// answer reported as narrow would mean the guard fired on it.
	if !strings.Contains(errOut, "not "+defaultEndpoint) {
		t.Errorf("stderr does not warn that the pin was not taken from gitlab.com:\n%s", errOut)
	}
	if strings.Contains(errOut, "narrower edition") {
		t.Errorf("an answer of %d types was reported as truncated:\n%s", graphqlintrospect.MinimumTypes+1, errOut)
	}
}

// TestRun_Generation_ATruncatedAnswer_ReplacesAPinAlreadyTruncated verifies the
// other side of the guard its sibling above pins. Refusing is about not losing a
// whole pin, so a directory holding a probe of a narrower instance has nothing
// to lose and the new probe is written: the `-schema` workflow is exactly that,
// run twice. Without this the inner condition was evaluated one way only, and a
// refusal that had quietly become unconditional would take that workflow with
// it.
func TestRun_Generation_ATruncatedAnswer_ReplacesAPinAlreadyTruncated(t *testing.T) {
	probed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(probed.Close)

	dir := filepath.Join(t.TempDir(), "pinned")
	earlier := withSource(func(s *graphqlschema.Source) {
		s.Instance = "https://gitlab.gnome.org/api/graphql"
		s.Types = graphqlintrospect.MinimumTypes - 1
	})
	if err := writeArtifacts(dir, minimalSDL, earlier); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	status, _, errOut := runCommand(t, genRun{
		endpoint: probed.URL, dir: dir, client: probed.Client(), now: fixedClock,
	})

	if status != 0 {
		t.Fatalf("exit status %d, want 0: a probe replacing a probe loses no guarantee.\n%s", status, errOut)
	}
	if strings.Contains(errOut, "refusing to replace") {
		t.Errorf("stderr refuses although the pin it would replace was itself truncated:\n%s", errOut)
	}
	types, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("the artifacts it wrote do not read back: %v", err)
	}
	if source.Instance != probed.URL || source.Types != types {
		t.Errorf("the pin was not replaced: %+v, want one naming %s and the %d types the schema beside it loads with",
			source, probed.URL, types)
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
//
// Every refusal spells its whole sentence rather than the phrase that names it.
// Most hold one value against another (the instance against gitlab.com, the
// record against the floor, the record against the schema beside it) and a
// message that reports the pair the wrong way round, or one value twice, tells
// a reader to fix the half that was already right. The dated ones read the
// pin's noun and its consequence out of one struct literal, two strings of one
// type. Nothing else here can see any of that: two arguments of one Sprintf
// have no branch to flip.
func TestRun_CheckMode_RefusesAPinOfSomethingElse(t *testing.T) {
	daysOld := provenance.Days(fixedClock().Sub(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))

	cases := []struct {
		name   string
		source graphqlschema.Source
		want   string
	}{
		{
			name:   "another instance",
			source: withSource(func(s *graphqlschema.Source) { s.Instance = "https://gitlab.gnome.org/api/graphql" }),
			want: "the pin was taken from https://gitlab.gnome.org/api/graphql, not https://gitlab.com/api/graphql: " +
				"the gate would then promise what that instance accepts, which is not what this server targets",
		},
		{
			name:   "a truncated or narrower answer",
			source: withSource(func(s *graphqlschema.Source) { s.Types = graphqlintrospect.MinimumTypes - 1 }),
			want: fmt.Sprintf(
				"the pin carries %d types and gitlab.com answers with more than %d: the introspection was truncated or the instance was a narrower edition",
				graphqlintrospect.MinimumTypes-1, graphqlintrospect.MinimumTypes,
			),
		},
		{
			name:   "an introspection with no token",
			source: withSource(func(s *graphqlschema.Source) { s.GitLabVersion = graphqlintrospect.UnknownVersion }),
			want:   noVersionRefusal,
		},
		{
			name:   "an introspection whose record was emptied by hand",
			source: withSource(func(s *graphqlschema.Source) { s.GitLabVersion = "" }),
			want:   noVersionRefusal,
		},
		{
			name:   "a pin past the window",
			source: withSource(func(s *graphqlschema.Source) { s.RetrievedAt = "2020-01-01" }),
			want: fmt.Sprintf(
				"the pin is %d days old and the window is %d: GitLab narrows fields in place, so a pin this old can no longer report a document that broke since",
				daysOld, provenance.Days(provenance.MaxAge),
			),
		},
		{
			name:   "a date nothing can read",
			source: withSource(func(s *graphqlschema.Source) { s.RetrievedAt = "one tuesday" }),
			want:   `the pin says it was taken on "one tuesday", which is not a date: nothing can then say how old the gate is`,
		},
		{
			name:   "a date that has not happened",
			source: withSource(func(s *graphqlschema.Source) { s.RetrievedAt = "2026-09-07" }),
			want:   "the pin says it was taken on 2026-09-07, which has not happened yet: no regeneration writes a day in the future",
		},
		{
			name:   "a record beside a schema it did not come from",
			source: withSource(func(s *graphqlschema.Source) { s.Types++ }),
			want: fmt.Sprintf(
				"%s records %d types and %s loads with %d: the two files were not written by one regeneration",
				graphqlschema.SourceFileName, canonicalSource.Types+1, graphqlschema.SDLFileName, canonicalSource.Types,
			),
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

// noVersionRefusal is the sentence both ways of recording no version produce.
const noVersionRefusal = "the pin records no GitLab version, which is what an introspection without GITLAB_TOKEN produces: " +
	"nothing can then say which release the gate speaks for"

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

// TestRun_Generation_RecordsTheUTCDayWhateverTheZone verifies that a pin
// records the UTC day it was taken on, not the calendar day of the machine
// that took it.
//
// The age check reads that day as midnight UTC, so a pin dated by a clock
// ahead of UTC would name a day that has not happened yet and fail its own
// --check the moment it was written. This clock is past midnight in UTC+14 and
// mid-morning in UTC, which is where the two calendars disagree.
func TestRun_Generation_RecordsTheUTCDayWhateverTheZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(server.Close)
	aheadOfUTC := func() time.Time {
		return time.Date(2026, 9, 7, 0, 30, 0, 0, time.FixedZone("UTC+14", 14*60*60))
	}
	dir := filepath.Join(t.TempDir(), "pinned")

	status, _, errOut := runCommand(t, genRun{endpoint: server.URL, dir: dir, client: server.Client(), now: aheadOfUTC})

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	_, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("the artifacts it wrote do not read back: %v", err)
	}
	if source.RetrievedAt != "2026-09-06" {
		t.Errorf("the record is dated %s, want 2026-09-06, the UTC day of %s", source.RetrievedAt, aheadOfUTC())
	}
	if strings.Contains(errOut, "has not happened yet") {
		t.Errorf("a pin written a moment ago was judged to come from the future:\n%s", errOut)
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
// main says out loud when it withholds the token, naming the instance GITLAB_URL
// gives it and the one -url asks, and says nothing when the two agree.
//
// -url takes an arbitrary endpoint, so this is the one decision in main that is
// not plumbing: a token resolved against the flag instead of the environment is
// a credential handed to an instance nobody named. A note that is not printed is
// the same defect one step quieter, since the version then reads "unknown" with
// nothing saying why. Check mode sends no request, so whether the token really
// stayed home is asserted by the generation test below.
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
			// No -url, so the default endpoint is asked while the token belongs
			// somewhere else. The note names both origins, which is what makes
			// this the one place the -url default is observable at all: a
			// generation with no flag pins whatever that default names, and
			// `make gen-graphql-schema` passes no flag.
			wantNote: "GITLAB_TOKEN belongs to https://gitlab.example.com and this run asks https://gitlab.com",
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

// TestMainEntry_Generation_SendsTheTokenOnlyToTheInstanceItBelongsTo verifies
// the request rather than the note about it: main hands run the credential
// CredentialFor resolved, never GITLAB_TOKEN as the environment holds it.
//
// Check mode sends nothing, so the note is all its sibling above can observe.
// This one generates against a local instance and records the Authorization
// header of every request that arrived. A main that read the environment
// directly would print the same note and send the token to whatever -url names
// anyway, which is the leak the resolution exists to prevent; one that sent
// nothing would lose the version for the instance the token does belong to.
func TestMainEntry_Generation_SendsTheTokenOnlyToTheInstanceItBelongsTo(t *testing.T) {
	const token = "glpat-not-a-real-token"

	cases := []struct {
		name     string
		instance func(serverURL string) string
		want     string
	}{
		{name: "GITLAB_URL names the instance asked", instance: func(serverURL string) string { return serverURL }, want: "Bearer " + token},
		{name: "GITLAB_URL names another instance", instance: func(string) string { return "https://gitlab.example.com" }, want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var mu sync.Mutex
			var authorizations []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				authorizations = append(authorizations, r.Header.Get("Authorization"))
				mu.Unlock()
				if strings.Contains(string(body), "metadata") {
					_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.4.0","revision":"abc1234"}}}`))
					return
				}
				_, _ = w.Write([]byte(tinySchema))
			}))
			t.Cleanup(server.Close)
			t.Setenv("GITLAB_URL", testCase.instance(server.URL))
			t.Setenv("GITLAB_TOKEN", token)

			status, _, errOut := runMainCapturing(t, "-url", server.URL, "-dir", filepath.Join(t.TempDir(), "pinned"))

			if status != 0 {
				t.Fatalf("exit status %d, want 0: a probe of a narrower instance still writes. stderr:\n%s", status, errOut)
			}
			mu.Lock()
			asked := slices.Clone(authorizations)
			mu.Unlock()
			if wantAsked := []string{testCase.want, testCase.want}; !slices.Equal(asked, wantAsked) {
				t.Errorf("the instance was asked with %q, want %q on the introspection and the version alike", asked, wantAsked)
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
