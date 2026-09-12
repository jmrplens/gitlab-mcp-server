package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// callsFixture names one committed shard directory.
func callsFixture(name string) string {
	return filepath.Join("testdata", "calls", name)
}

// writeFile writes one fixture file, failing the test if it cannot.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestReadRuntimes_OneDirectory_OneRuntime verifies the plain layout: a
// directory holding shards is one runtime, named by its run line, with every
// line type folded and the late dispatch joined onto its call.
func TestReadRuntimes_OneDirectory_OneRuntime(t *testing.T) {
	runtimes, err := readRuntimes(callsFixture("ce"))
	if err != nil {
		t.Fatalf("readRuntimes() error = %v", err)
	}
	if len(runtimes) != 1 {
		t.Fatalf("readRuntimes() = %d runtimes, want 1", len(runtimes))
	}
	rt := runtimes[0]
	if rt.key != "community/free" || rt.edition != "community" || rt.tier != edition.Free {
		t.Errorf("runtime = %s (%s, %s), want community/free", rt.key, rt.edition, rt.tier)
	}
	if len(rt.runs) != 1 || len(rt.sessions) != 3 || len(rt.calls) != 7 || len(rt.skips) != 1 || rt.dispatches != 1 {
		t.Errorf("folded %d runs, %d sessions, %d calls, %d skips, %d dispatches; want 1, 3, 7, 1, 1",
			len(rt.runs), len(rt.sessions), len(rt.calls), len(rt.skips), rt.dispatches)
	}
	if rt.lateJoins != 1 {
		t.Errorf("lateJoins = %d, want the one meta call joined from its dispatch line", rt.lateJoins)
	}
	for _, call := range rt.calls {
		if call.TraceID == "t2" && call.Dispatched != "issue.list" {
			t.Errorf("call t2 dispatched = %q after the join, want issue.list", call.Dispatched)
		}
	}
}

// TestReadRuntimes_ParentDirectory_OneRuntimePerChild verifies the layout
// dist/e2e-calls produces: a parent holding no shard, one child per target.
func TestReadRuntimes_ParentDirectory_OneRuntimePerChild(t *testing.T) {
	runtimes, err := readRuntimes(callsFixture("two-runtimes"))
	if err != nil {
		t.Fatalf("readRuntimes() error = %v", err)
	}
	var keys []string
	for _, rt := range runtimes {
		keys = append(keys, rt.key)
	}
	if want := []string{"community/free", "enterprise/ultimate"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("runtimes = %q, want %q", keys, want)
	}
}

// TestReadRuntimes_MixedRunLines_Refused verifies that one directory whose
// run lines name two runtimes is refused with both named, since folding them
// would credit each against the other's catalog.
func TestReadRuntimes_MixedRunLines_Refused(t *testing.T) {
	_, err := readRuntimes(callsFixture("mixed"))
	if err == nil {
		t.Fatal("readRuntimes() accepted a directory naming two runtimes")
	}
	for _, want := range []string{"community/free", "enterprise/ultimate", "one directory per runtime"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		})
	}
}

// TestReadRuntimes_EmptyDirectory_Refused verifies that a directory with no
// shard anywhere is refused with the reader's own message, which names the
// variable that turns recording on.
func TestReadRuntimes_EmptyDirectory_Refused(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "child"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := readRuntimes(dir)
	if err == nil || !strings.Contains(err.Error(), e2ecalls.DirEnv) {
		t.Errorf("readRuntimes() error = %v, want the no-shard refusal naming %s", err, e2ecalls.DirEnv)
	}
}

// TestReadRuntimes_MissingDirectory_Refused verifies that a path that does
// not exist, or that is a file, is an error rather than an empty result.
func TestReadRuntimes_MissingDirectory_Refused(t *testing.T) {
	if _, err := readRuntimes(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("readRuntimes() accepted a directory that does not exist")
	}
	file := filepath.Join(t.TempDir(), "calls-x.jsonl")
	writeFile(t, file, "")
	if _, err := readRuntimes(file); err == nil {
		t.Error("readRuntimes() accepted a file in place of a directory")
	}
}

// TestReadRuntimes_BrokenChild_Refused verifies that a child directory whose
// shard cannot be read stops the parent read with that error.
func TestReadRuntimes_BrokenChild_Refused(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "ce")
	if err := os.MkdirAll(child, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(child, "calls-x.jsonl"), "not json\n")
	if _, err := readRuntimes(dir); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("readRuntimes() error = %v, want the parse failure of the child's shard", err)
	}
}

// TestReadRuntimes_DeeperShards_ReadAsOne verifies that shards two levels
// down, with no shard at the first level, are read as one runtime.
func TestReadRuntimes_DeeperShards_ReadAsOne(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(deep, "calls-x.jsonl"), `{"schema":1,"type":"run","run":{"package":"p","requirement":"any","edition":"community","tier":"free","run_id":"r","status":"started"}}`+"\n")
	runtimes, err := readRuntimes(dir)
	if err != nil {
		t.Fatalf("readRuntimes() error = %v", err)
	}
	if len(runtimes) != 1 || runtimes[0].key != "community/free" {
		t.Errorf("readRuntimes() = %d runtimes, want one community/free", len(runtimes))
	}
}

// TestFoldRecords_RunLines_Ruled verifies the run-line rules one by one: no
// run line at all, run lines that never probed, a tier the server does not
// know, and a call whose dispatch line carries no action.
func TestFoldRecords_RunLines_Ruled(t *testing.T) {
	run := func(editionToken, tier, status string) e2ecalls.Record {
		return e2ecalls.Record{Schema: 1, Type: e2ecalls.TypeRun, Run: &e2ecalls.Run{Package: "p", Edition: editionToken, Tier: tier, Status: status}}
	}
	call := e2ecalls.Record{Schema: 1, Type: e2ecalls.TypeCall, Call: &e2ecalls.Call{Test: "T", Action: "a.b", TraceID: "t"}}
	cases := []struct {
		name    string
		records []e2ecalls.Record
		wantErr error
		wantMsg string
		check   func(*testing.T, *runtimeRecords)
	}{
		{name: "no run line", records: []e2ecalls.Record{call}, wantErr: errNoRunLine},
		{name: "refused before probing", records: []e2ecalls.Record{run("", "", e2ecalls.RunRefused)}, wantMsg: "refused before probing"},
		{name: "unknown tier", records: []e2ecalls.Record{run("community", "platinum", e2ecalls.RunStarted)}, wantMsg: "platinum"},
		{
			name: "a refused run beside a started one takes the started one's runtime",
			records: []e2ecalls.Record{
				run("", "", e2ecalls.RunRefused), run("enterprise", "premium", e2ecalls.RunStarted),
			},
			check: func(t *testing.T, rt *runtimeRecords) {
				t.Helper()
				if rt.key != "enterprise/premium" || rt.tier != edition.Premium {
					t.Errorf("runtime = %s, want enterprise/premium", rt.key)
				}
			},
		},
		{
			name: "a dispatch line without an action does not join",
			records: []e2ecalls.Record{
				run("community", "free", e2ecalls.RunStarted), call,
				{Schema: 1, Type: e2ecalls.TypeDispatch, Dispatch: &e2ecalls.Dispatch{TraceID: "t", Tool: "x"}},
			},
			check: func(t *testing.T, rt *runtimeRecords) {
				t.Helper()
				if rt.calls[0].Dispatched != "" || rt.lateJoins != 0 || rt.dispatches != 1 {
					t.Errorf("dispatched = %q, lateJoins = %d, dispatches = %d; want no join", rt.calls[0].Dispatched, rt.lateJoins, rt.dispatches)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := foldRecords(tc.records)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("foldRecords() error = %v, want %v", err, tc.wantErr)
				}
			case tc.wantMsg != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
					t.Errorf("foldRecords() error = %v, want one mentioning %q", err, tc.wantMsg)
				}
			default:
				if err != nil {
					t.Fatalf("foldRecords() error = %v", err)
				}
				tc.check(t, rt)
			}
		})
	}
}

// TestMatchesRuntime_Selectors_Matched verifies the two shorthands and the literal
// key: ce is any community runtime, ee is a licensed enterprise one, an
// unlicensed enterprise image is neither, and a key matches itself.
func TestMatchesRuntime_Selectors_Matched(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		selector string
		want     bool
	}{
		{name: "ce matches community", key: "community/free", selector: "ce", want: true},
		{name: "ce does not match enterprise", key: "enterprise/free", selector: "ce", want: false},
		{name: "ee matches ultimate", key: "enterprise/ultimate", selector: "ee", want: true},
		{name: "ee matches premium", key: "enterprise/premium", selector: "ee", want: true},
		{name: "ee does not match an unlicensed image", key: "enterprise/free", selector: "ee", want: false},
		{name: "ee does not match community", key: "community/free", selector: "ee", want: false},
		{name: "a key matches itself", key: "enterprise/free", selector: "enterprise/free", want: true},
		{name: "selectors are trimmed and case-insensitive", key: "community/free", selector: " CE ", want: true},
		{name: "a key without a slash matches nothing", key: "community", selector: "ce", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesRuntime(tc.key, tc.selector); got != tc.want {
				t.Errorf("matchesRuntime(%q, %q) = %t, want %t", tc.key, tc.selector, got, tc.want)
			}
		})
	}
}

// TestSelectRuntimes_Selectors_KeepMatches verifies that selectors filter the
// runtimes and that no selector keeps them all.
func TestSelectRuntimes_Selectors_KeepMatches(t *testing.T) {
	runtimes := []*runtimeRecords{{key: "community/free"}, {key: "enterprise/ultimate"}, {key: "enterprise/free"}}
	cases := []struct {
		name      string
		selectors []string
		want      []string
	}{
		{name: "none keeps all", selectors: nil, want: []string{"community/free", "enterprise/ultimate", "enterprise/free"}},
		{name: "ce", selectors: []string{"ce"}, want: []string{"community/free"}},
		{name: "ce and ee", selectors: splitSelectors("ce, ee,,"), want: []string{"community/free", "enterprise/ultimate"}},
		{name: "nothing matches", selectors: []string{"gitlab.com"}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, rt := range selectRuntimes(runtimes, tc.selectors) {
				got = append(got, rt.key)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("selectRuntimes() = %q, want %q", got, tc.want)
			}
		})
	}
}
