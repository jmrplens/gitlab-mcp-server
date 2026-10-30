// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// The release the fixture REST record names.
const (
	fixtureRelease  = "19.5.0-ee"
	fixtureRevision = "883b289c40e"
)

// recordDir writes a REST record naming version and revision, the two fields
// the release pin is held to, and returns its directory.
func recordDir(t *testing.T, version, revision string) string {
	t.Helper()
	dir := t.TempDir()
	if err := apilive.Write(dir, apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Source:        apilive.Source{Version: version, Revision: revision},
	}); err != nil {
		t.Fatalf("prepare the REST record: %v", err)
	}
	return dir
}

// dumpFile writes a schema dump and returns its path.
func dumpFile(t *testing.T, sdl string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gitlab_schema.graphql")
	if err := os.WriteFile(path, []byte(sdl), 0o600); err != nil {
		t.Fatalf("prepare the dump: %v", err)
	}
	return path
}

// digestOf is the record's digest of a dump.
func digestOf(sdl string) string {
	digest := sha256.Sum256([]byte(sdl))
	return hex.EncodeToString(digest[:])
}

// TestRunRelease_FromAFile_WritesThePinTheCheckAccepts verifies a whole
// release generation from a dump on disk and the check that follows it: the
// record names the release and revision the REST record does, the file it was
// read from and that it was a file, the digest of what was read and of the
// schema written, and the pair it writes is one the offline check accepts
// against the same REST record.
func TestRunRelease_FromAFile_WritesThePinTheCheckAccepts(t *testing.T) {
	records := recordDir(t, fixtureRelease, fixtureRevision)
	from := dumpFile(t, wholeSDL)
	dir := filepath.Join(t.TempDir(), "pins")

	status, out, errOut := runCommand(t, genRun{release: true, dir: dir, apiLiveDir: records, from: from, now: fixedClock})
	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	for _, line := range []struct{ name, want string }{
		{name: "names the schema it wrote", want: "wrote " + graphqlschema.ReleaseSDLFileName},
		{name: "names the file the dump came from", want: "from the file " + from + " (release as the REST record names it)"},
	} {
		t.Run(line.name, func(t *testing.T) {
			if !strings.Contains(out, line.want) {
				t.Errorf("stdout does not say %q:\n%s", line.want, out)
			}
		})
	}
	types, sdlDigest, source, err := readReleaseArtifacts(dir)
	if err != nil {
		t.Fatalf("readReleaseArtifacts() error = %v", err)
	}
	rendered, err := os.ReadFile(filepath.Join(dir, graphqlschema.ReleaseSDLFileName))
	if err != nil {
		t.Fatalf("read the rendered schema: %v", err)
	}
	want := graphqlschema.ReleaseSource{
		GitLabVersion:  fixtureRelease,
		GitLabRevision: fixtureRevision,
		Origin:         from,
		FromFile:       true,
		SHA256:         digestOf(wholeSDL),
		SDLSHA256:      digestOf(string(rendered)),
		RetrievedAt:    "2026-09-06",
		Types:          loadedTypes(wholeSDL),
	}
	if source != want || types != want.Types || sdlDigest != want.SDLSHA256 {
		t.Errorf("the release pin is %d types hashing to %s and %+v, want %d, %s and %+v",
			types, sdlDigest, source, want.Types, want.SDLSHA256, want)
	}

	status, out, errOut = runCommand(t, genRun{release: true, check: true, dir: dir, apiLiveDir: records})
	if status != 0 {
		t.Fatalf("the check refuses the pin it was just handed (exit %d):\n%s", status, errOut)
	}
	if !strings.Contains(out, "the release schema parses") || !strings.Contains(out, fixtureRelease) {
		t.Errorf("stdout does not report the release pin:\n%s", out)
	}
}

// TestRunRelease_FromTheReleaseTag_DownloadsTheDumpAndNamesTheJob verifies the
// download: the request names the release's tag and GitLab's dump job, and the
// origin recorded is the URL the redirect ended at, which is the one naming
// the job whose artifact was read.
func TestRunRelease_FromTheReleaseTag_DownloadsTheDumpAndNamesTheJob(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.RequestURI())
		mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/artifacts/") {
			http.Redirect(w, r, "/jobs/17/artifacts/raw/tmp/tests/graphql/gitlab_schema.graphql", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(wholeSDL))
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()

	status, out, errOut := runCommand(t, genRun{
		release: true, dir: dir, apiLiveDir: recordDir(t, fixtureRelease, fixtureRevision),
		client: server.Client(), dumpBase: server.URL + "/artifacts", now: fixedClock,
	})
	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	mu.Lock()
	defer mu.Unlock()
	const wantRequest = "/artifacts/v19.5.0-ee/raw/tmp/tests/graphql/gitlab_schema.graphql?job=graphql-schema-dump"
	if len(asked) != 2 || asked[0] != wantRequest {
		t.Errorf("the generator asked %v, want %s and then the job it redirected to", asked, wantRequest)
	}
	if !strings.Contains(out, "downloading the schema of GitLab "+fixtureRelease) {
		t.Errorf("stdout does not say what it downloads:\n%s", out)
	}
	_, _, source, err := readReleaseArtifacts(dir)
	if err != nil {
		t.Fatalf("readReleaseArtifacts() error = %v", err)
	}
	if want := server.URL + "/jobs/17/artifacts/raw/tmp/tests/graphql/gitlab_schema.graphql"; source.Origin != want || source.FromFile {
		t.Errorf("the record names %q (from a file: %v), want the job's URL %q", source.Origin, source.FromFile, want)
	}
}

// TestRunRelease_GenerationFailures_ExitNonZeroAndSayWhy verifies every way a
// release generation stops, each with the sentence that says why and nothing
// written.
func TestRunRelease_GenerationFailures_ExitNonZeroAndSayWhy(t *testing.T) {
	missing := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(missing.Close)
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 64)))
	}))
	t.Cleanup(large.Close)
	// A body shorter than the length it declares ends the read with an
	// unexpected EOF, which is a download that failed midway.
	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("type Query"))
	}))
	t.Cleanup(cut.Close)
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("a file where a directory should be"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	// A directory where the schema goes is a schema that is there and cannot
	// be read, which a failed record write would have nothing to put back from.
	unreadable := t.TempDir()
	if err := os.Mkdir(filepath.Join(unreadable, graphqlschema.ReleaseSDLFileName), 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	// Types and no query root: the loader takes it, and its rendering, a
	// schema block naming no root, does not load.
	rootless := strings.Replace(strings.Replace(wholeSDL, "type Query {\n  ok: Boolean\n}\n\n", "", 1), "schema {\n  query: Query\n}\n", "", 1)
	records := recordDir(t, fixtureRelease, fixtureRevision)

	for _, testCase := range []struct {
		name string
		cfg  genRun
		want string
	}{
		{name: "no REST record", cfg: genRun{apiLiveDir: t.TempDir(), from: dumpFile(t, wholeSDL)}, want: "reading the live API record"},
		{name: "a REST record naming no version", cfg: genRun{apiLiveDir: recordDir(t, "", ""), from: dumpFile(t, wholeSDL)}, want: "names no GitLab version"},
		{name: "a dump file that is not there", cfg: genRun{apiLiveDir: records, from: filepath.Join(t.TempDir(), "absent.graphql")}, want: "read the release schema"},
		// A directory opens and then refuses the read, which is the read
		// failing after the open succeeded.
		{name: "a dump path that is a directory", cfg: genRun{apiLiveDir: records, from: t.TempDir()}, want: "read the release schema"},
		{
			name: "a dump file larger than any dump", cfg: genRun{apiLiveDir: records, from: dumpFile(t, strings.Repeat("x", 64)), dumpLimit: 32},
			want: "holds more than 32 bytes, which is not a schema dump",
		},
		{name: "a dump that is not SDL", cfg: genRun{apiLiveDir: records, from: dumpFile(t, "type Query {")}, want: "parse "},
		{name: "a dump too small to be a release", cfg: genRun{apiLiveDir: records, from: dumpFile(t, minimalSDL)}, want: "it is not the dump of a whole release"},
		{name: "a dump whose rendering does not load", cfg: genRun{apiLiveDir: records, from: dumpFile(t, rootless)}, want: "the converted release schema does not parse"},
		{name: "a directory that cannot be written", cfg: genRun{apiLiveDir: records, from: dumpFile(t, wholeSDL), dir: blocked}, want: "blocked"},
		{
			name: "a schema there that cannot be read", cfg: genRun{apiLiveDir: records, from: dumpFile(t, wholeSDL), dir: unreadable},
			want: "which a failed write would put back",
		},
		{name: "a base that is not a URL", cfg: genRun{apiLiveDir: records, dumpBase: "://nowhere"}, want: "build the request"},
		{name: "a host nothing answers on", cfg: genRun{apiLiveDir: records, dumpBase: netfixture.DeadEndpoint(t), client: http.DefaultClient}, want: "download "},
		{name: "an artifact GitLab no longer keeps", cfg: genRun{apiLiveDir: records, dumpBase: missing.URL, client: missing.Client()}, want: "answered 404 Not Found: GitLab keeps a job's artifacts"},
		{name: "an answer larger than any dump", cfg: genRun{apiLiveDir: records, dumpBase: large.URL, client: large.Client(), dumpLimit: 32}, want: "answered with more than 32 bytes"},
		{name: "a download cut short", cfg: genRun{apiLiveDir: records, dumpBase: cut.URL, client: cut.Client()}, want: "read " + cut.URL},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := testCase.cfg
			cfg.release, cfg.now = true, fixedClock
			if cfg.dir == "" {
				cfg.dir = t.TempDir()
			}
			status, _, errOut := runCommand(t, cfg)
			if status != 1 {
				t.Fatalf("exit status %d, want 1. stderr:\n%s", status, errOut)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("stderr does not say %q:\n%s", testCase.want, errOut)
			}
			if _, err := os.Stat(filepath.Join(cfg.dir, graphqlschema.ReleaseSourceFileName)); err == nil {
				t.Errorf("a failed generation wrote %s", graphqlschema.ReleaseSourceFileName)
			}
		})
	}
}

// soundRelease is a release record the check accepts against the fixture REST
// record, for a check test about something else.
var soundRelease = graphqlschema.ReleaseSource{
	GitLabVersion:  fixtureRelease,
	GitLabRevision: fixtureRevision,
	Origin:         "https://gitlab.com/gitlab-org/gitlab/-/jobs/17/artifacts/raw/tmp/tests/graphql/gitlab_schema.graphql",
	SHA256:         digestOf(wholeSDL),
	SDLSHA256:      digestOf(wholeSDL),
	RetrievedAt:    "2026-10-30",
	Types:          loadedTypes(wholeSDL),
}

// editedSDL is wholeSDL with one field's type changed by hand: it loads with
// the same count of types, which is what the count alone could not tell apart.
var editedSDL = strings.Replace(wholeSDL, "type Padding0 {\n  ok: Boolean\n}", "type Padding0 {\n  ok: String\n}", 1)

// TestRunRelease_CheckMode_RefusesAPinOfAnotherRelease verifies every refusal
// of the offline check, each spelled whole: the two releases are named in the
// order the sentence gives them, since a message reporting the pair the wrong
// way round tells a reader to re-pin the half that was right.
func TestRunRelease_CheckMode_RefusesAPinOfAnotherRelease(t *testing.T) {
	records := recordDir(t, fixtureRelease, fixtureRevision)
	spoil := func(change func(*graphqlschema.ReleaseSource)) graphqlschema.ReleaseSource {
		source := soundRelease
		change(&source)
		return source
	}
	for _, testCase := range []struct {
		name   string
		sdl    string
		source graphqlschema.ReleaseSource
		want   string
	}{
		{
			name: "another release", sdl: wholeSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) { s.GitLabVersion = "19.4.1-ee" }),
			want: "the release schema is GitLab 19.4.1-ee (883b289c40e) and the REST record is taken from GitLab " +
				"19.5.0-ee (883b289c40e): the documents would be held to a release the rest of the repository is not pinned to",
		},
		{
			name: "another revision", sdl: wholeSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) { s.GitLabRevision = "0000000" }),
			want:   "the release schema is GitLab 19.5.0-ee (0000000) and the REST record is taken from GitLab 19.5.0-ee (883b289c40e)",
		},
		{
			name: "a truncated dump", sdl: minimalSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) {
				s.Types, s.SDLSHA256 = loadedTypes(minimalSDL), digestOf(minimalSDL)
			}),
			want: "the release schema carries " + strconv.Itoa(loadedTypes(minimalSDL)) + " types and a GitLab schema has more than 4000",
		},
		{
			name: "a schema edited by hand", sdl: editedSDL, source: soundRelease,
			want: "gitlab-release-schema.graphql hashes to " + digestOf(editedSDL) + " and release-source.json records \"" +
				digestOf(wholeSDL) + "\": the schema was changed after it was rendered, or the two files were not written by one regeneration",
		},
		{
			name: "a record written before the schema digest", sdl: wholeSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) { s.SDLSHA256 = "" }),
			want:   "gitlab-release-schema.graphql hashes to " + digestOf(wholeSDL) + " and release-source.json records \"\"",
		},
		{
			name: "no origin", sdl: wholeSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) { s.Origin = "" }),
			want:   "the release record names no origin or no digest",
		},
		{
			name: "no digest", sdl: wholeSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) { s.SHA256 = "" }),
			want:   "the release record names no origin or no digest",
		},
		{
			name: "a record of another regeneration", sdl: wholeSDL,
			source: spoil(func(s *graphqlschema.ReleaseSource) { s.Types++ }),
			want: "release-source.json records " + strconv.Itoa(loadedTypes(wholeSDL)+1) + " types and gitlab-release-schema.graphql loads with " +
				strconv.Itoa(loadedTypes(wholeSDL)) + ": the two files were not written by one regeneration",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := writeReleaseArtifacts(dir, testCase.sdl, testCase.source); err != nil {
				t.Fatalf("prepare the pin: %v", err)
			}
			status, _, errOut := runCommand(t, genRun{release: true, check: true, dir: dir, apiLiveDir: records})
			if status != 1 {
				t.Fatalf("exit status %d, want 1. stderr:\n%s", status, errOut)
			}
			for _, want := range []string{testCase.want, "re-pin the release schema with `make gen-graphql-release-schema`"} {
				if !strings.Contains(errOut, want) {
					t.Errorf("stderr does not say %q:\n%s", want, errOut)
				}
			}
		})
	}
}

// TestRunRelease_CheckMode_FilesThatCannotBeRead_NameTheFile verifies the
// check's failures to read, each naming the file to look at.
func TestRunRelease_CheckMode_FilesThatCannotBeRead_NameTheFile(t *testing.T) {
	records := recordDir(t, fixtureRelease, fixtureRevision)
	sound := t.TempDir()
	if err := writeReleaseArtifacts(sound, wholeSDL, soundRelease); err != nil {
		t.Fatalf("prepare the pin: %v", err)
	}
	for _, testCase := range []struct {
		name string
		// files are written into a directory of the case's own, or sound is
		// used when there are none.
		files      map[string]string
		apiLiveDir string
		want       string
	}{
		{name: "no schema", files: map[string]string{"unrelated.txt": ""}, apiLiveDir: records, want: graphqlschema.ReleaseSDLFileName},
		{name: "a schema that is not SDL", files: map[string]string{graphqlschema.ReleaseSDLFileName: "type Query {"}, apiLiveDir: records, want: graphqlschema.ReleaseSDLFileName},
		{name: "no record", files: map[string]string{graphqlschema.ReleaseSDLFileName: minimalSDL}, apiLiveDir: records, want: graphqlschema.ReleaseSourceFileName},
		{
			name:       "a record that is not JSON",
			files:      map[string]string{graphqlschema.ReleaseSDLFileName: minimalSDL, graphqlschema.ReleaseSourceFileName: "{not json"},
			apiLiveDir: records, want: graphqlschema.ReleaseSourceFileName,
		},
		{name: "no REST record", apiLiveDir: t.TempDir(), want: "reading the live API record"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := sound
			if testCase.files != nil {
				dir = t.TempDir()
				for name, content := range testCase.files {
					write(t, dir, name, []byte(content))
				}
			}
			status, _, errOut := runCommand(t, genRun{release: true, check: true, dir: dir, apiLiveDir: testCase.apiLiveDir})
			if status != 1 {
				t.Fatalf("exit status %d, want 1. stderr:\n%s", status, errOut)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("stderr does not name %q:\n%s", testCase.want, errOut)
			}
		})
	}
}

// TestWriteReleaseArtifacts_ARecordThatCannotBeWritten_LeavesThePairItFound
// verifies that a write which fails at the record, the second of the two
// files, takes back the schema it already wrote: the one the directory held
// is restored, and a schema the directory did not hold is removed, so the
// failure leaves no new schema beside an old record.
func TestWriteReleaseArtifacts_ARecordThatCannotBeWritten_LeavesThePairItFound(t *testing.T) {
	for _, testCase := range []struct {
		name string
		// previous is the schema the directory holds before the write, none
		// when empty.
		previous string
	}{
		{name: "a schema there before", previous: minimalSDL},
		{name: "no schema there before"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			schemaPath := filepath.Join(dir, graphqlschema.ReleaseSDLFileName)
			if testCase.previous != "" {
				write(t, dir, graphqlschema.ReleaseSDLFileName, []byte(testCase.previous))
			}
			// A directory where the record goes refuses the record's write
			// after the schema's succeeded.
			if err := os.Mkdir(filepath.Join(dir, graphqlschema.ReleaseSourceFileName), 0o750); err != nil {
				t.Fatalf("prepare the fixture: %v", err)
			}

			err := writeReleaseArtifacts(dir, wholeSDL, soundRelease)
			if err == nil || !strings.Contains(err.Error(), graphqlschema.ReleaseSourceFileName) {
				t.Fatalf("writeReleaseArtifacts() error = %v, want one naming %s", err, graphqlschema.ReleaseSourceFileName)
			}
			got, readErr := os.ReadFile(schemaPath)
			switch {
			case testCase.previous == "" && !errors.Is(readErr, fs.ErrNotExist):
				t.Errorf("the failed write left %s behind (read error %v), want it removed", graphqlschema.ReleaseSDLFileName, readErr)
			case testCase.previous != "" && string(got) != testCase.previous:
				t.Errorf("the failed write left %s as %d bytes (read error %v), want the %d it held before",
					graphqlschema.ReleaseSDLFileName, len(got), readErr, len(testCase.previous))
			}
		})
	}
}

// TestRunRelease_CheckMode_AcceptsTheCommittedRepositoryArtifacts is the gate
// itself over the real files: the committed release pin parses, agrees with
// its record, and is the schema of the release the committed REST record is
// taken from.
func TestRunRelease_CheckMode_AcceptsTheCommittedRepositoryArtifacts(t *testing.T) {
	status, out, errOut := runCommand(t, genRun{
		release: true, check: true,
		dir:        filepath.Join("..", "..", defaultDir),
		apiLiveDir: filepath.Join("..", "..", apilive.DefaultDir),
	})
	if status != 0 {
		t.Fatalf("the committed release pin does not pass its own gate (exit %d):\n%s", status, errOut)
	}
	if !strings.Contains(out, "the release schema parses") {
		t.Errorf("stdout does not report the release pin:\n%s", out)
	}
}

// TestMainEntry_Release_TheFlagsReachTheRun verifies the command line the
// Makefile runs for the release pin, -release with --check, -dir and
// -api-live-dir, and the one it refuses, -from without -release, which the
// gitlab.com pin's run would otherwise ignore in silence.
func TestMainEntry_Release_TheFlagsReachTheRun(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")
	records := recordDir(t, fixtureRelease, fixtureRevision)
	dir := t.TempDir()
	if err := writeReleaseArtifacts(dir, wholeSDL, soundRelease); err != nil {
		t.Fatalf("prepare the pin: %v", err)
	}

	t.Run("the release check", func(t *testing.T) {
		status, out, errOut := runMainCapturing(t, "-release", "--check", "-dir", dir, "-api-live-dir", records)
		if status != 0 {
			t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
		}
		if !strings.Contains(out, "the release schema parses") {
			t.Errorf("stdout does not report the release pin:\n%s", out)
		}
	})
	t.Run("-from without -release", func(t *testing.T) {
		status, _, errOut := runMainCapturing(t, "-from", dumpFile(t, wholeSDL))
		if status != 2 {
			t.Fatalf("exit status %d, want 2", status)
		}
		if !strings.Contains(errOut, errReleaseFlags.Error()) {
			t.Errorf("stderr does not refuse the flag:\n%s", errOut)
		}
	})
}
