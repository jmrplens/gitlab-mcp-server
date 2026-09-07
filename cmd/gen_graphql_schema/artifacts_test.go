package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// sampleSource is the provenance a write test round-trips.
var sampleSource = graphqlschema.Source{
	Instance:       "https://gitlab.example.com/api/graphql",
	GitLabVersion:  "19.4.0",
	GitLabRevision: "abc1234",
	RetrievedAt:    "2026-09-06",
	Types:          3,
}

// minimalSDL is a schema small enough to write in a test and real enough to
// load.
const minimalSDL = "type Query {\n  ok: Boolean\n}\n\nschema {\n  query: Query\n}\n"

// TestWriteArtifacts_ThenRead_RoundTripsBothFiles verifies the pair a
// regeneration produces, through the same reader --check uses, which is the
// only combination that proves a committed artifact is usable.
func TestWriteArtifacts_ThenRead_RoundTripsBothFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pinned")

	if err := writeArtifacts(dir, minimalSDL, sampleSource); err != nil {
		t.Fatalf("writeArtifacts() error = %v, want nil", err)
	}

	types, source, err := readArtifacts(dir)
	if err != nil {
		t.Fatalf("readArtifacts() error = %v, want nil", err)
	}
	if types == 0 {
		t.Error("readArtifacts() reported no types")
	}
	if source != sampleSource {
		t.Errorf("readArtifacts() source = %+v, want %+v", source, sampleSource)
	}

	record, err := os.ReadFile(filepath.Join(dir, graphqlschema.SourceFileName))
	if err != nil {
		t.Fatalf("read the record back: %v", err)
	}
	if !strings.HasSuffix(string(record), "}\n") {
		t.Errorf("the record does not end with a newline:\n%s", record)
	}
}

// TestWriteArtifacts_UnusableDirectory_ReportsWhichPath verifies that a run
// which cannot write says what it was trying to write, since the operator
// chose the directory with a flag.
func TestWriteArtifacts_UnusableDirectory_ReportsWhichPath(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("this is a file, not a directory"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	cases := []struct {
		name string
		dir  string
		want string
	}{
		{name: "a directory that cannot be created", dir: filepath.Join(blocked, "under"), want: "create "},
		{name: "a schema path that is a directory", dir: mkdirAll(t, root, "asdir", graphqlschema.SDLFileName), want: "write "},
		{name: "a record path that is a directory", dir: mkdirAll(t, root, "recdir", graphqlschema.SourceFileName), want: "write "},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := writeArtifacts(testCase.dir, minimalSDL, sampleSource)

			if err == nil {
				t.Fatalf("writeArtifacts() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("writeArtifacts() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// mkdirAll creates dir under root with an inner directory occupying the name a
// file has to be written to, and returns dir.
func mkdirAll(t *testing.T, root, name, occupied string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, occupied), 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	return dir
}

// TestReadArtifacts_MissingOrCorrupt_NamesTheFile verifies each way the
// committed pair can be unusable. This is what the CI gate reports, so it has
// to say which of the two files to look at.
func TestReadArtifacts_MissingOrCorrupt_NamesTheFile(t *testing.T) {
	cases := []struct {
		name  string
		build func(*testing.T) string
		want  string
	}{
		{
			name:  "nothing there at all",
			build: func(t *testing.T) string { t.Helper(); return t.TempDir() },
			want:  graphqlschema.SDLFileName,
		},
		{
			name: "a schema that is not SDL",
			build: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				write(t, dir, graphqlschema.SDLFileName, []byte("this is prose, not a schema"))
				return dir
			},
			want: "parse the schema",
		},
		{
			name: "a schema with no record beside it",
			build: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				write(t, dir, graphqlschema.SDLFileName, []byte(minimalSDL))
				return dir
			},
			want: graphqlschema.SourceFileName,
		},
		{
			name: "a record that is not JSON",
			build: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				write(t, dir, graphqlschema.SDLFileName, []byte(minimalSDL))
				write(t, dir, graphqlschema.SourceFileName, []byte("{nope"))
				return dir
			},
			want: "parse " + graphqlschema.SourceFileName,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			types, source, err := readArtifacts(testCase.build(t))

			if err == nil {
				t.Fatalf("readArtifacts() error = nil, want one naming %q", testCase.want)
			}
			if types != 0 || source != (graphqlschema.Source{}) {
				t.Errorf("readArtifacts() returned %d types and %+v, want the zero values on failure", types, source)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("readArtifacts() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// write puts one artifact in place for a fixture.
func write(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
}

// TestWriteArtifacts_WritesTheSDLVerbatim verifies that what lands on disk is
// the SDL itself, byte for byte. It is the whole point of committing the pin as
// text: a reviewer reads a re-pin as a diff of what GitLab changed, and anyone
// verifying a repair greps the file rather than decompressing it.
func TestWriteArtifacts_WritesTheSDLVerbatim(t *testing.T) {
	dir := t.TempDir()

	if err := writeArtifacts(dir, minimalSDL, sampleSource); err != nil {
		t.Fatalf("writeArtifacts() error = %v, want nil", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, graphqlschema.SDLFileName))
	if err != nil {
		t.Fatalf("read the schema back: %v", err)
	}
	if string(written) != minimalSDL {
		t.Errorf("writeArtifacts() wrote %q, want the SDL verbatim %q", written, minimalSDL)
	}
}
