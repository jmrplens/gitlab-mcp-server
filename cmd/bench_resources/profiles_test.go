// profiles_test.go covers reading profiles off a pprof listener and writing
// them beside the record, against the real handlers served in-process rather
// than a canned answer, so the parsing cannot drift from what the runtime
// prints.
package main

import (
	"net/http"
	"net/http/httptest"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newPprofTestClient serves the profiling handlers from this test process
// and returns a client for them.
func newPprofTestClient(t *testing.T) *pprofClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return newPprofClient(srv.URL)
}

// isGzip reports whether data starts with the gzip magic number, which is
// what `go tool pprof` looks at first in a profile file.
func isGzip(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

// TestPprofClient_ReadsProfilesAndTheGoroutineTotal verifies the three reads
// the series makes: a CPU profile over a second, a heap profile, and the
// goroutine total, each in the shape the analysis tooling expects.
func TestPprofClient_ReadsProfilesAndTheGoroutineTotal(t *testing.T) {
	client := newPprofTestClient(t)

	t.Run("cpu", func(t *testing.T) {
		got := client.cpuProfile(t.Context(), 1)
		if got.err != nil {
			t.Fatalf("cpuProfile: %v", got.err)
		}
		if !isGzip(got.data) {
			t.Error("the CPU profile is not a compressed pprof file")
		}
	})
	t.Run("heap", func(t *testing.T) {
		got := client.heapProfile(t.Context())
		if got.err != nil {
			t.Fatalf("heapProfile: %v", got.err)
		}
		if !isGzip(got.data) {
			t.Error("the heap profile is not a compressed pprof file")
		}
	})
	t.Run("goroutines", func(t *testing.T) {
		count, err := client.goroutineCount(t.Context())
		if err != nil {
			t.Fatalf("goroutineCount: %v", err)
		}
		if count < 1 {
			t.Errorf("goroutineCount = %d, want at least this test's goroutine", count)
		}
	})
}

// TestPprofClient_Failures covers every way a read fails, each reported with
// the route so a note on a step says what was missing: nothing listening, a
// route the listener does not serve, a body cut short, and a base URL that
// cannot be made into a request.
func TestPprofClient_Failures(t *testing.T) {
	empty := httptest.NewServer(http.NewServeMux())
	t.Cleanup(empty.Close)
	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	t.Cleanup(truncated.Close)

	cases := []struct {
		name string
		base string
		want string
	}{
		{name: "nothing listening", base: "http://127.0.0.1:1", want: "GET /debug/pprof/goroutine"},
		{name: "route not served", base: empty.URL, want: "HTTP 404"},
		{name: "body cut short", base: truncated.URL, want: "read /debug/pprof/goroutine"},
		{name: "unusable base", base: "http://bad host", want: "build request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newPprofClient(tc.base).goroutineCount(t.Context())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("goroutineCount = %v, want an error saying %q", err, tc.want)
			}
		})
	}
}

// TestParseGoroutineTotal_ReadsTheHeaderLine verifies the total is taken
// from the first line of a debug=1 listing and nothing else, since the lines
// below it carry numbers of their own.
func TestParseGoroutineTotal_ReadsTheHeaderLine(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    int
		wantErr string
	}{
		{name: "a listing", body: "goroutine profile: total 42\n7 @ 0x1 0x2\n#\t0x1\tmain.main+0x1\n", want: 42},
		{name: "header only", body: "goroutine profile: total 3", want: 3},
		{name: "not a listing", body: "<html>not found</html>", wantErr: "unexpected goroutine listing header"},
		{name: "total not a number", body: "goroutine profile: total lots\n", wantErr: "parse goroutine total"},
		{name: "empty", body: "", wantErr: "unexpected goroutine listing header"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGoroutineTotal([]byte(tc.body))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("parseGoroutineTotal = %d, %v; want an error saying %q", got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("parseGoroutineTotal = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

// TestWriteProfiles_StoresUnderTheScenarioAndNotesWhatItCouldNot verifies
// both profiles land at <scenario>/<clients>.<kind>.pb.gz with slash paths
// in the record, and that a profile that was not captured or cannot be
// written leaves an empty path and a note rather than failing the step.
func TestWriteProfiles_StoresUnderTheScenarioAndNotesWhatItCouldNot(t *testing.T) {
	dir := t.TempDir()
	got, notes := writeProfiles(dir, "http-meta-series", 50, captured{data: []byte("cpu")}, captured{data: []byte("heap")})
	want := StepProfiles{CPU: "http-meta-series/50.cpu.pb.gz", Heap: "http-meta-series/50.heap.pb.gz"}
	if got != want {
		t.Errorf("profiles = %+v, want %+v", got, want)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none", notes)
	}
	for rel, content := range map[string]string{got.CPU: "cpu", got.Heap: "heap"} {
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
			if err != nil || string(data) != content {
				t.Errorf("%s holds %q, %v; want %q", rel, data, err, content)
			}
		})
	}
}

// TestWriteProfiles_NotesWhatItCouldNot verifies a profile that was not
// captured, a profiles directory that is a file, and a profile path that is
// a directory each leave an empty path and a note rather than failing the
// step: the memory and latency of a step are worth keeping without them.
func TestWriteProfiles_NotesWhatItCouldNot(t *testing.T) {
	t.Run("one not captured", func(t *testing.T) {
		got, notes := writeProfiles(t.TempDir(), "s", 1, captured{err: os.ErrDeadlineExceeded}, captured{data: []byte("heap")})
		if got.CPU != "" || got.Heap == "" {
			t.Errorf("profiles = %+v, want no CPU path and a heap path", got)
		}
		if len(notes) != 1 || !strings.Contains(notes[0], "cpu profile unavailable") {
			t.Errorf("notes = %v, want one about the CPU profile", notes)
		}
	})

	t.Run("directory cannot be made", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write the blocking file: %v", err)
		}
		got, notes := writeProfiles(blocker, "s", 1, captured{data: []byte("cpu")}, captured{data: []byte("heap")})
		if got != (StepProfiles{}) {
			t.Errorf("profiles = %+v, want none written under a file", got)
		}
		if len(notes) != 2 || !strings.Contains(notes[0], "not written") {
			t.Errorf("notes = %v, want one per profile", notes)
		}
	})

	t.Run("path is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "s", "1.cpu.pb.gz"), 0o750); err != nil {
			t.Fatalf("make the directory in the way: %v", err)
		}
		got, notes := writeProfiles(dir, "s", 1, captured{data: []byte("cpu")}, captured{data: []byte("heap")})
		if got.CPU != "" || got.Heap == "" {
			t.Errorf("profiles = %+v, want the CPU write refused and the heap written", got)
		}
		if len(notes) != 1 || !strings.Contains(notes[0], "cpu profile not written") {
			t.Errorf("notes = %v", notes)
		}
	})
}
