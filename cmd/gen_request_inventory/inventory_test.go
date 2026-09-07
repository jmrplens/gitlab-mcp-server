// inventory_test.go covers reading the recorder's shards and folding them into
// the committed artifact.
//
// The properties under test are the ones the artifact is worth nothing
// without: two runs over the same tree produce the same bytes, an endpoint
// reached by several calls is one row carrying the union of their parameter
// names, and a directory with no shards in it is an error rather than an empty
// inventory that would erase the committed one.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeShard writes one shard file holding the given lines.
func writeShard(t *testing.T, dir, name string, lines ...string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}

// TestReadShards_TwoShards_ReadsEveryLine verifies that the merge sees every
// process's recording, since one test binary writes one shard and a suite runs
// a hundred and seventy-six of them.
func TestReadShards_TwoShards_ReadsEveryLine(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "requests-1.jsonl",
		`{"package":"internal/tools/issues","test":"TestList","kind":"rest","method":"GET","path":"/projects/:id/issues"}`,
		"",
		`{"package":"internal/tools/issues","test":"TestGet","kind":"rest","method":"GET","path":"/projects/:id/issues/:iid"}`)
	writeShard(t, dir, "requests-2.jsonl",
		`{"package":"internal/tools/tags","test":"TestList","kind":"rest","method":"GET","path":"/projects/:id/repository/tags"}`)
	writeShard(t, dir, "notes.txt", "not a shard")
	if err := os.Mkdir(filepath.Join(dir, "nested.jsonl"), 0o750); err != nil {
		t.Fatalf("Mkdir error = %v", err)
	}

	records, err := readShards(dir)
	if err != nil {
		t.Fatalf("readShards error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("read %d record(s), want 3: %+v", len(records), records)
	}
}

// TestReadShards_NothingRecorded_IsAnError verifies the mistake that would
// otherwise be invisible: a merge run without a recorded suite behind it would
// write an empty inventory and report the whole artifact as a change.
func TestReadShards_NothingRecorded_IsAnError(t *testing.T) {
	tests := []struct {
		name string
		dir  func(t *testing.T) string
		want string
	}{
		{
			name: "a directory that is not there",
			dir:  func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "absent") },
			want: "read shard directory",
		},
		{
			name: "a directory holding no shard",
			dir:  func(t *testing.T) string { t.Helper(); return t.TempDir() },
			want: "no .jsonl shard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readShards(tt.dir(t))
			if err == nil {
				t.Fatal("readShards succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestReadShard_UnreadableShard_NamesTheFailure verifies that a shard the
// merge cannot read stops it, rather than producing an inventory missing
// whatever that shard held.
func TestReadShard_UnreadableShard_NamesTheFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "broken.jsonl")); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	_, err := readShards(dir)

	if err == nil || !strings.Contains(err.Error(), "open shard") {
		t.Fatalf("readShards error = %v, want an open failure", err)
	}
}

// TestReadShard_MalformedLine_NamesFileAndLine verifies that a corrupt shard
// is reported precisely enough to find, since the merge reads a hundred and
// seventy-six of them.
func TestReadShard_MalformedLine_NamesFileAndLine(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "requests-1.jsonl",
		`{"package":"internal/tools/issues","kind":"rest","method":"GET","path":"/projects"}`,
		`{"package":`)

	_, err := readShards(dir)

	if err == nil {
		t.Fatal("readShards succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error = %q, want it to name line 2", err)
	}
}

// TestReadShard_OverlongLine_IsReported verifies that a line too long to read
// stops the merge, because a line silently dropped for its length would look
// exactly like a request nobody makes.
func TestReadShard_OverlongLine_IsReported(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "requests-1.jsonl", `{"path":"/`+strings.Repeat("x", maxShardLine)+`"}`)

	_, err := readShards(dir)

	if err == nil || !strings.Contains(err.Error(), "read ") {
		t.Fatalf("readShards error = %v, want a read failure", err)
	}
}

// TestMerge_SameEndpoint_IsOneRowWithTheUnionOfParameters verifies what a row
// means: an endpoint a package calls, carrying every parameter name that
// package was seen to send it. Two calls differing only in an optional filter
// are the same endpoint, and the union is the answer to what this server can
// send it.
func TestMerge_SameEndpoint_IsOneRowWithTheUnionOfParameters(t *testing.T) {
	rows := merge([]shardRecord{
		{Package: "internal/tools/issues", Test: "TestList", Kind: "rest", Method: "GET", Path: "/projects/:id/issues", Query: []string{"state"}},
		{Package: "internal/tools/issues", Test: "TestListPaged", Kind: "rest", Method: "GET", Path: "/projects/:id/issues", Query: []string{"per_page", "state"}},
		{Package: "internal/tools/issues", Test: "TestCreate", Kind: "rest", Method: "POST", Path: "/projects/:id/issues"},
	})

	if len(rows) != 2 {
		t.Fatalf("merged into %d row(s), want 2: %+v", len(rows), rows)
	}
	if rows[0].Method != http.MethodGet || !slices.Equal(rows[0].Query, []string{"per_page", "state"}) {
		t.Errorf("first row = %+v, want the GET carrying [per_page state]", rows[0])
	}
	if rows[1].Query != nil {
		t.Errorf("the POST carries %v, want no query names", rows[1].Query)
	}
}

// TestMerge_GraphQL_KeepsOperationsApart verifies that two documents posted to
// the one GraphQL endpoint are two rows, since the path says nothing about
// either of them.
func TestMerge_GraphQL_KeepsOperationsApart(t *testing.T) {
	rows := merge([]shardRecord{
		{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "query group", Variables: []string{"fullPath"}},
		{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "mutation epicSetSubscription", Variables: []string{"id"}},
	})

	if len(rows) != 2 {
		t.Fatalf("merged into %d row(s), want 2: %+v", len(rows), rows)
	}
	if rows[0].Operation != "mutation epicSetSubscription" || rows[1].Operation != "query group" {
		t.Errorf("rows = %+v, want them ordered by operation", rows)
	}
}

// TestMerge_Rows_AreTotallyOrdered verifies the property the whole artifact
// rests on: the order does not depend on the order the shards were read in, so
// two runs over the same tree write the same bytes.
func TestMerge_Rows_AreTotallyOrdered(t *testing.T) {
	records := []shardRecord{
		{Package: "internal/tools/tags", Kind: "rest", Method: "GET", Path: "/projects/:id/repository/tags"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:id/issues"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "GET", Path: "/projects/:id/issues"},
		{Package: "internal/tools/issues", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "query project"},
	}

	forward := merge(records)
	slices.Reverse(records)
	backward := merge(records)

	if string(render(forward)) != string(render(backward)) {
		t.Fatalf("merge depends on the order it read the shards in:\n%+v\n%+v", forward, backward)
	}
	want := []string{"/graphql", "/projects/:id/issues", "/projects/:id/issues", "/projects/:id/repository/tags"}
	for i, got := range forward {
		if got.Path != want[i] {
			t.Errorf("row %d path = %q, want %q", i, got.Path, want[i])
		}
	}
	// sort.Slice needs a strict ordering, and a row that sorts before itself
	// is the one way this comparison could make the output depend on the
	// algorithm rather than on the rows.
	if less(forward[0], forward[0]) {
		t.Error("a row sorts before itself")
	}
}

// TestDifferences_StaleArtifact_NamesTheRequestsThatMoved verifies what a
// stale-artifact failure is worth: the point of committing the inventory is
// that a handler calling a different endpoint stops being invisible, and a
// gate answering "the file changed" would hand that back.
func TestDifferences_StaleArtifact_NamesTheRequestsThatMoved(t *testing.T) {
	committed := render([]row{
		{Package: "internal/tools/issues", Kind: "rest", Method: http.MethodGet, Path: "/projects/:id/issues", Query: []string{"state"}},
		{Package: "internal/tools/issues", Kind: "rest", Method: http.MethodGet, Path: "/projects/:id/old_place"},
	})
	now := []row{
		{Package: "internal/tools/issues", Kind: "rest", Method: http.MethodGet, Path: "/projects/:id/issues", Query: []string{"per_page", "state"}},
		{Package: "internal/tools/issues", Kind: "graphql", Method: http.MethodPost, Path: "/graphql", Operation: "query project", Variables: []string{"fullPath"}},
	}

	report := differences(committed, now)

	for _, want := range []string{
		"now issued and not in the committed inventory (2)",
		"/graphql query project $fullPath",
		"/projects/:id/issues ?per_page,state",
		"in the committed inventory and no longer issued (2)",
		"/projects/:id/old_place",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(report, want) {
				t.Errorf("report = %q, want it to contain %q", report, want)
			}
		})
	}
}

// TestDifferences_ManyOrUnreadable_StaysReadable verifies the two edges of the
// report: a change too large to print is capped rather than dumped into a CI
// log, and a committed file that is not this format leaves the failure to
// stand on its own.
func TestDifferences_ManyOrUnreadable_StaysReadable(t *testing.T) {
	many := make([]row, 0, maxReportedDifferences+3)
	for i := range cap(many) {
		many = append(many, row{Package: "internal/tools/issues", Kind: "rest", Method: http.MethodGet, Path: "/projects/:id/thing" + string(rune('a'+i))})
	}

	t.Run("a change larger than the cap", func(t *testing.T) {
		report := differences(render(nil), many)
		if !strings.Contains(report, "and 3 more") {
			t.Errorf("report = %q, want it to cap the list", report)
		}
	})
	t.Run("a committed file in another format", func(t *testing.T) {
		if report := differences([]byte("not json at all"), many); report != "" {
			t.Errorf("report = %q, want nothing", report)
		}
	})
}

// TestRender_Inventory_IsIndentedJSONEndingInANewline verifies the artifact is
// shaped like every other text file here, since a diff is the whole point of
// committing it.
func TestRender_Inventory_IsIndentedJSONEndingInANewline(t *testing.T) {
	content := render([]row{{Package: "internal/tools/issues", Kind: "rest", Method: "GET", Path: "/projects/:id/issues"}})

	if !strings.HasSuffix(string(content), "\n") {
		t.Error("the rendered inventory does not end in a newline")
	}
	var decoded inventory
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if decoded.Note == "" {
		t.Error("the rendered inventory carries no note saying where it comes from")
	}
	if len(decoded.Requests) != 1 || decoded.Requests[0].Path != "/projects/:id/issues" {
		t.Errorf("requests = %+v, want the one row it was given", decoded.Requests)
	}
}
