package apishapes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apishapes"
)

// TestNormalizePath_GitLabSpelling_BecomesTheInventorySpelling verifies the one
// conversion that lets the two artifacts meet. The record keeps GitLab's own
// spelling and the request inventory keeps ours, so every comparison converts
// at the point of use and neither artifact is written in the other's dialect.
func TestNormalizePath_GitLabSpelling_BecomesTheInventorySpelling(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "prefix_and_placeholders", path: "/api/v4/projects/{id}/issues/{issue_iid}", want: "/projects/:id/issues/:issue_iid"},
		{name: "no_placeholder", path: "/api/v4/version", want: "/version"},
		{name: "graphql_is_not_under_v4", path: "/api/graphql", want: "/graphql"},
		{name: "trailing_placeholder", path: "/api/v4/groups/{id}", want: "/groups/:id"},
		{name: "adjacent_placeholders", path: "/api/v4/a/{x}/{y}", want: "/a/:x/:y"},
		{name: "already_bare", path: "/projects/{id}", want: "/projects/:id"},
		{name: "root", path: "/api/v4", want: "/"},
		{name: "unclosed_brace_is_left_alone", path: "/api/v4/x/{id", want: "/x/{id"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := apishapes.NormalizePath(testCase.path); got != testCase.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestKey_MethodAndPath_IsUpperCased verifies the operation key, which is what
// a comparison looks an operation up by.
func TestKey_MethodAndPath_IsUpperCased(t *testing.T) {
	if got := apishapes.Key("get", "/api/v4/version"); got != "GET /api/v4/version" {
		t.Errorf("Key() = %q, want the method upper-cased", got)
	}
}

// TestWriteThenRead_RoundTripsTheRecord verifies the pair a regeneration
// produces, through the same reader the gate uses, which is the only
// combination that proves a committed record is usable.
func TestWriteThenRead_RoundTripsTheRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "record")
	written := apishapes.Document{
		Note:   "a note",
		Source: apishapes.Source{URL: "https://example.test/spec", Ref: "master", RetrievedAt: "2026-09-07", SHA256: "abc", Operations: 1},
		Operations: map[string]apishapes.Operation{
			"GET /api/v4/version": {Response: []string{"revision", "version"}, Params: []string{"id"}, Body: []string{"name"}},
		},
	}

	if err := apishapes.Write(dir, written); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	read, err := apishapes.Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if read.SchemaVersion != apishapes.SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", read.SchemaVersion, apishapes.SchemaVersion)
	}
	op := read.Operations["GET /api/v4/version"]
	if strings.Join(op.Response, ",") != "revision,version" {
		t.Errorf("response = %v, want the pair it was written with", op.Response)
	}
	if got := read.Keys(); len(got) != 1 || got[0] != "GET /api/v4/version" {
		t.Errorf("Keys() = %v, want the one operation", got)
	}
	if !strings.Contains(read.Source.String(), "1 operations") {
		t.Errorf("Source.String() = %q, does not report the count", read.Source.String())
	}
}

// TestRead_UnusableRecords_AreRefusedWithTheReason verifies that a record this
// build cannot act on is refused rather than half-read. Every field in it is a
// list of names a comparison acts on, so a reader that guesses would report
// findings about its own confusion.
func TestRead_UnusableRecords_AreRefusedWithTheReason(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "not json", content: "{", want: "parse"},
		{name: "another schema version", content: `{"schema_version":99}`, want: "schema version 99"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, apishapes.FileName), []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("prepare the fixture: %v", err)
			}

			_, err := apishapes.Read(dir)

			if err == nil {
				t.Fatalf("Read() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Read() error = %v, want it to name %q", err, testCase.want)
			}
		})
	}

	t.Run("no file at all", func(t *testing.T) {
		if _, err := apishapes.Read(t.TempDir()); err == nil {
			t.Error("Read() error = nil for a directory holding no record")
		}
	})
}

// TestWrite_UnwritableDirectory_ReportsTheDirectory verifies the failure a
// regeneration meets when it cannot write, which must name what it could not
// create rather than fail silently and leave the old record in place.
func TestWrite_UnwritableDirectory_ReportsTheDirectory(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("a file where a directory should be"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	err := apishapes.Write(filepath.Join(blocked, "under"), apishapes.Document{})

	if err == nil {
		t.Fatal("Write() error = nil, want one naming the directory")
	}
	if !strings.Contains(err.Error(), "create ") {
		t.Errorf("Write() error = %v, want it to name the directory it could not create", err)
	}
}

// TestWrite_RecordPathIsADirectory_ReportsTheFile verifies the other half of a
// failed write, where the directory is fine and the record itself cannot be
// written. Both must fail loudly: a regeneration that reports success while
// leaving the previous record in place is how a gate starts speaking for a
// GitLab nobody fetched.
func TestWrite_RecordPathIsADirectory_ReportsTheFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, apishapes.FileName), 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	err := apishapes.Write(dir, apishapes.Document{})

	if err == nil {
		t.Fatal("Write() error = nil, want one naming the record")
	}
	if !strings.Contains(err.Error(), apishapes.FileName) {
		t.Errorf("Write() error = %v, want it to name %s", err, apishapes.FileName)
	}
}
