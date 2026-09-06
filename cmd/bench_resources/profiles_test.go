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
	"runtime"
	"strings"
	"sync"
	"testing"
)

// newPprofTestClient serves the profiling handlers from this test process
// and returns a client for them.
func newPprofTestClient(t *testing.T) *pprofClient {
	t.Helper()
	return newRecordedPprofClient(t).client
}

// recordedPprofClient serves the same handlers and remembers what was asked
// of them, so a test can check the request the driver made rather than
// inferring it from the answer.
//
// The handlers are the runtime's own, not a stand-in for them: gc=1 there
// really does run a collection before the profile is written, which is what
// makes the assertion on this process's collection count evidence rather
// than a restatement of the driver's code.
type recordedPprofClient struct {
	client *pprofClient

	mu       sync.Mutex
	requests []string
}

// newRecordedPprofClient serves the profiling handlers from this test process
// behind a recorder.
func newRecordedPprofClient(t *testing.T) *recordedPprofClient {
	t.Helper()
	rec := &recordedPprofClient{}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.requests = append(rec.requests, r.URL.RequestURI())
		rec.mu.Unlock()
		pprof.Index(w, r)
	})
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	rec.client = newPprofClient(srv.URL)
	return rec
}

// asked lists the request URIs the listener answered, oldest first.
func (r *recordedPprofClient) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.requests...)
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

// TestPprofClient_HeapReads_ForceACollection verifies both heap reads ask for
// one, and that the collection actually happened.
//
// Without gc=1 the handler serves whatever survived the last cycle, which is
// not the live heap: it is the live heap plus however much garbage the
// collector had not reached, and on a process serving thousands of calls a
// second that is most of what the profile shows. The profile is the evidence
// the hot-spot analysis reads and the settled figure is what the series
// publishes as tenancy, so both have to mean what their readers think.
//
// The proof is this process's own collection count, taken across the read.
// runtime.GC blocks until its cycle is complete, so a read that forced one
// leaves NumGC higher; a read that did not, cannot. The recorded request is
// asserted beside it because a growing count alone could be some other
// goroutine's collection.
func TestPprofClient_HeapReads_ForceACollection(t *testing.T) {
	cases := []struct {
		name       string
		read       func(*testing.T, *pprofClient)
		wantReads  int
		wantSuffix string
	}{
		{
			name: "the profile written beside the record",
			read: func(t *testing.T, client *pprofClient) {
				t.Helper()
				if got := client.heapProfile(t.Context()); got.err != nil || !isGzip(got.data) {
					t.Errorf("heapProfile = %d bytes, %v; want a compressed profile", len(got.data), got.err)
				}
			},
			wantReads: 1, wantSuffix: "/debug/pprof/heap?gc=1",
		},
		{
			name: "the settled reading",
			read: func(t *testing.T, client *pprofClient) {
				t.Helper()
				heap, err := client.settledHeap(t.Context())
				if err != nil {
					t.Errorf("settledHeap: %v", err)
					return
				}
				if heap == 0 {
					t.Error("settledHeap = 0 bytes, want this process's live heap")
				}
			},
			// Two: the first collection still counts what the moment before
			// it left behind, so the figure is taken from the second.
			wantReads: 2, wantSuffix: "/debug/pprof/heap?gc=1&debug=1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecordedPprofClient(t)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)

			tc.read(t, rec.client)

			runtime.ReadMemStats(&after)
			asked := rec.asked()
			if len(asked) != tc.wantReads {
				t.Fatalf("the listener answered %v, want %d read(s)", asked, tc.wantReads)
			}
			for _, uri := range asked {
				if uri != tc.wantSuffix {
					t.Errorf("the driver asked for %q, want %q", uri, tc.wantSuffix)
				}
			}
			//#nosec G115 -- both are collection counts of this test process, orders below the conversion's range
			if collections := int(after.NumGC) - int(before.NumGC); collections < tc.wantReads {
				t.Errorf("%d collections ran across %d read(s), want one per read: the handler was not asked to collect",
					collections, tc.wantReads)
			}
		})
	}
}

// TestParseHeapAlloc_ReadsTheMemStatsLine verifies the live heap is taken
// from the MemStats dump that ends a debug=1 heap profile, and that a body
// carrying no such line is refused rather than read as zero, which would be
// published as a credential costing nothing.
func TestParseHeapAlloc_ReadsTheMemStatsLine(t *testing.T) {
	const dump = "heap profile: 1: 16 [2: 32] @ heap/8192\n\n# runtime.MemStats\n# Alloc = 4194304\n# HeapAlloc = 4194304\n# HeapSys = 8388608\n"
	cases := []struct {
		name    string
		body    string
		want    uint64
		wantErr string
	}{
		{name: "a dump", body: dump, want: 4194304},
		{name: "the line alone", body: "# HeapAlloc = 12\n", want: 12},
		{name: "no such line", body: "heap profile: 0: 0 [0: 0] @ heap/8192\n", wantErr: "carried no HeapAlloc line"},
		{name: "empty", body: "", wantErr: "carried no HeapAlloc line"},
		{name: "not a number", body: "# HeapAlloc = plenty\n", wantErr: "parse HeapAlloc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHeapAlloc([]byte(tc.body))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("parseHeapAlloc = %d, %v; want an error saying %q", got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("parseHeapAlloc = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

// TestSettledHeap_ListenerGone_ReportsTheFirstRead verifies a listener that
// does not answer fails the settled reading at the first collection rather
// than at the second, so the note on the step names what was unreachable.
func TestSettledHeap_ListenerGone_ReportsTheFirstRead(t *testing.T) {
	_, err := newPprofClient("http://127.0.0.1:1").settledHeap(t.Context())
	if err == nil || !strings.Contains(err.Error(), "GET /debug/pprof/heap") {
		t.Errorf("settledHeap = %v, want the unreachable listener named", err)
	}
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
