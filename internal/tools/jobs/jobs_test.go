// jobs_test.go contains unit tests for GitLab CI/CD job operations
// (list, get, trace, cancel, retry). Tests use httptest to mock the
// GitLab Jobs API and verify both success and error paths.
package jobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestRawHelpers_UnescapablePath_FailBeforeTheRequest verifies both raw
// helpers refuse a path the client cannot unescape, and never reach the
// server with it. The public handlers escape every identifier before building
// a path, so the branch is reached by calling the helpers directly.
func TestRawHelpers_UnescapablePath_FailBeforeTheRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler called, want the request to fail while it is built")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	const badPath = "projects/%zz/jobs"

	t.Run("list", func(t *testing.T) {
		jobs, resp, err := rawListJobs(client, badPath, nil, gl.WithContext(t.Context()))
		if err == nil || jobs != nil || resp != nil {
			t.Fatalf("rawListJobs = %v/%v/%v, want only an error", jobs, resp, err)
		}
	})
	t.Run("get", func(t *testing.T) {
		job, resp, err := rawGetJob(t.Context(), client, badPath)
		if err == nil || job != nil || resp != nil {
			t.Fatalf("rawGetJob = %v/%v/%v, want only an error", job, resp, err)
		}
	})
}

// serveTrace answers the trace route with log and refuses anything else.
func serveTrace(t *testing.T, log string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != pathJobTrace {
			http.NotFound(w, r)
			return
		}
		w.Header().Set(testHeaderContentType, "text/plain")
		_, _ = w.Write([]byte(log))
	}))
}

// failingLog is a log over the trace limit whose last line is the failure,
// as the issue's reproduction was: thousands of install lines, then FATAL.
func failingLog(lines int) string {
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "installing dependency number %d of the build ......\n", i)
	}
	b.WriteString("FATAL unit test TestCheckout failed expected 200 got 500\n")
	return b.String()
}

// TestJobTrace_LongLog_KeepsItsEnd verifies that a log over the limit keeps
// its last 100 KiB, from the start of a line, and says how much of its
// beginning was left out. It used to keep the first 100 KiB, so the failure,
// which is almost always at the end of a log, was exactly what was dropped.
func TestJobTrace_LongLog_KeepsItsEnd(t *testing.T) {
	log := failingLog(3000)
	out, err := Trace(t.Context(), serveTrace(t, log), TraceInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Trace() error: %v", err)
	}
	if !out.Truncated || len(out.Trace) > hundredKiB || out.OmittedBytes != len(log)-len(out.Trace) {
		t.Errorf("Truncated/len/OmittedBytes = %v/%d/%d, want true, at most %d, and %d", out.Truncated, len(out.Trace), out.OmittedBytes, hundredKiB, len(log)-len(out.Trace))
	}
	if !strings.HasSuffix(out.Trace, "FATAL unit test TestCheckout failed expected 200 got 500\n") {
		t.Errorf("the kept log ends %q, want the FATAL line", out.Trace[max(0, len(out.Trace)-80):])
	}
	if !strings.HasPrefix(out.Trace, "installing dependency number ") {
		t.Errorf("the kept log starts %q, want the start of a line", out.Trace[:min(40, len(out.Trace))])
	}
}

// TestJobTrace_OneLongLine_IsCutOnACharacter verifies that a log with no
// line break in its last 100 KiB is cut at a character boundary, so what is
// kept is still valid UTF-8.
func TestJobTrace_OneLongLine_IsCutOnACharacter(t *testing.T) {
	log := strings.Repeat("é", hundredKiB) // two bytes each, so the byte cut lands mid-character half the time
	out, err := Trace(t.Context(), serveTrace(t, log+"x"), TraceInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Trace() error: %v", err)
	}
	if !utf8.ValidString(out.Trace) || !strings.HasSuffix(out.Trace, "éx") || len(out.Trace) > hundredKiB {
		t.Errorf("kept %d bytes, valid UTF-8 %v, ending %q; want at most %d valid bytes ending in éx", len(out.Trace), utf8.ValidString(out.Trace), out.Trace[max(0, len(out.Trace)-4):], hundredKiB)
	}
}

// TestJobTrace_TailLines_ReturnsTheLastLines verifies the tail_lines input:
// the last N lines, the whole log when it has fewer, and a refusal for a
// count outside 1 to 10000, where 0 means the input was not given.
func TestJobTrace_TailLines_ReturnsTheLastLines(t *testing.T) {
	const log = "one\ntwo\nthree\nfour\n"
	for _, tc := range []struct {
		name  string
		lines int
		want  string
	}{
		{"not given", 0, log},
		{"the last two", 2, "three\nfour\n"},
		{"the last one", 1, "four\n"},
		{"more than the log has", 9, log},
		{"the ceiling", 10000, log},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Trace(t.Context(), serveTrace(t, log), TraceInput{ProjectID: "42", JobID: 100, TailLines: tc.lines})
			if err != nil {
				t.Fatalf("Trace() error: %v", err)
			}
			if out.Trace != tc.want || out.Truncated != (tc.want != log) {
				t.Errorf("Trace = %q, truncated %v; want %q", out.Trace, out.Truncated, tc.want)
			}
		})
	}
	t.Run("a log with no final newline", func(t *testing.T) {
		out, err := Trace(t.Context(), serveTrace(t, "one\ntwo"), TraceInput{ProjectID: "42", JobID: 100, TailLines: 1})
		if err != nil || out.Trace != "two" {
			t.Errorf("Trace = %q, %v; want the last line", out.Trace, err)
		}
	})
	for _, bad := range []int{-1, 10001} {
		t.Run(fmt.Sprint("refuses ", bad), func(t *testing.T) {
			_, err := Trace(t.Context(), testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), TraceInput{ProjectID: "42", JobID: 100, TailLines: bad})
			if err == nil || !strings.Contains(err.Error(), "tail_lines") {
				t.Errorf("Trace(tail_lines %d) error = %v, want a refusal naming tail_lines", bad, err)
			}
		})
	}
}

// TestJobTrace_Markup_IsStrippedFromTheLog verifies that the structured log
// carries the text without the runner's ANSI sequences and section markers,
// which used to reach the structured channel raw and the text channel as
// "[36;1m" and "section_start:" once the escape byte was dropped.
func TestJobTrace_Markup_IsStrippedFromTheLog(t *testing.T) {
	log := "section_start:1791747389:prepare_executor\r\x1b[0K\x1b[0K\x1b[36;1mPreparing the executor\x1b[0;m\nsection_end:1791747390:prepare_executor\r\x1b[0K\n"
	out, err := Trace(t.Context(), serveTrace(t, log), TraceInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Trace() error: %v", err)
	}
	if out.Trace != "Preparing the executor\n\n" {
		t.Errorf("Trace = %q, want the text alone", out.Trace)
	}
}

// TestJobTrace_ResponseOverTheClientCeiling_NamesTheWayOut verifies that a
// log the client-wide response ceiling stops is answered with the route to
// read it from GitLab directly, rather than a bare "unexpected error".
func TestJobTrace_ResponseOverTheClientCeiling_NamesTheWayOut(t *testing.T) {
	client := serveTrace(t, strings.Repeat("x", 4096))
	client.SetMaxResponseBytes(1024)
	_, err := Trace(t.Context(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil || !strings.Contains(err.Error(), "GET /projects/:id/jobs/:job_id/trace") {
		t.Fatalf("Trace() error = %v, want the way out", err)
	}
}

// markupLog is a runner log of at least size bytes, as a runner writes one:
// every line colored, a collapsible section every hundred lines, and the
// failure on the last line.
func markupLog(size int) string {
	var b strings.Builder
	b.Grow(size + 128)
	for i := 1; b.Len() < size; i++ {
		if i%100 == 1 {
			fmt.Fprintf(&b, "section_start:1791747389:step_%d\r\x1b[0K\x1b[36;1mStep %d\x1b[0;m\n", i, i)
		}
		fmt.Fprintf(&b, "\x1b[0;32m[ok]\x1b[0m installing dependency number %d of the build\n", i)
	}
	b.WriteString("\x1b[31;1mFATAL unit test TestCheckout failed expected 200 got 500\x1b[0;m\n")
	return b.String()
}

// The headers GitLab Runner writes in front of each line of a log it
// timestamps (FF_TIMESTAMPS): a space after the stream when the line is a
// line of its own, a + when it continues the one before it.
const (
	traceStdoutHeader    = "2026-10-13T18:57:48.608003Z 00O "
	traceStdoutContinued = "2026-10-13T18:57:48.608670Z 00O+"
)

// sectionMarkerWriteEnd ends the write GitLab Runner makes of a section
// marker of its own: the marker's carriage return and the erase-line
// sequence after it, with no line break.
const sectionMarkerWriteEnd = "\r\x1b[0K"

// timestamped writes log as GitLab Runner writes it with FF_TIMESTAMPS: a
// header in front of every line, and a line broken after each section
// marker's write, the piece after it flagged as continuing the line before
// it. The runner writes a section marker of its own as one write ending in
// sectionMarkerWriteEnd, flushes that write whole as a line of the log, since
// it holds a carriage return after its last line break, and the header text
// it writes next continues that line.
func timestamped(log string) string {
	var b strings.Builder
	b.Grow(2 * len(log))
	for line := range strings.SplitAfterSeq(log, "\n") {
		if line == "" {
			continue
		}
		b.WriteString(traceStdoutHeader)
		b.WriteString(strings.ReplaceAll(line, sectionMarkerWriteEnd, sectionMarkerWriteEnd+"\n"+traceStdoutContinued))
	}
	return b.String()
}

// TestJobTrace_TimestampedLog_ReadsAsTheJobPrintedIt verifies that a log the
// runner wrote with timestamps reads as the same log written without them,
// over a log longer than the part Trace reads: the header in front of every
// line is gone and the pieces the runner flushed each section marker in are
// one line again, so the end kept is the same and tail_lines counts the lines
// the job printed. The header used to stay at the start of every line, and
// every section marker's line came back as two.
func TestJobTrace_TimestampedLog_ReadsAsTheJobPrintedIt(t *testing.T) {
	plain := markupLog(1200 << 10)
	log := timestamped(plain)
	for _, tc := range []struct {
		name      string
		tailLines int
	}{
		{"the end of the log", 0},
		{"the last five lines", 5},
		{"the last two hundred lines", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := TraceInput{ProjectID: "42", JobID: 100, TailLines: tc.tailLines}
			want, err := Trace(t.Context(), serveTrace(t, plain), input)
			if err != nil {
				t.Fatalf("Trace(untimestamped) error: %v", err)
			}
			got, err := Trace(t.Context(), serveTrace(t, log), input)
			if err != nil {
				t.Fatalf("Trace(timestamped) error: %v", err)
			}
			if got.Trace != want.Trace || !got.Truncated {
				t.Errorf("the timestamped log answered %d bytes starting %q, truncated %v; want the %d bytes the untimestamped one answers, starting %q", len(got.Trace), firstTraceLine(got.Trace), got.Truncated, len(want.Trace), firstTraceLine(want.Trace))
			}
			assertTailOfLog(t, log, got.Trace, got.OmittedBytes)
		})
	}
}

// firstTraceLine returns the first line of a log, for a failure message.
func firstTraceLine(log string) string {
	line, _, _ := strings.Cut(log, "\n")
	return line
}

// allocatedBy returns how many bytes f allocated on the heap.
func allocatedBy(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestJobTrace_LongLog_ReadsOnlyItsEnd verifies what Trace costs on top of
// the read client-go makes of the body anyway, for a 64 MiB log, the size of
// the client's response ceiling: a bounded number of the windows it reads
// from the end of the log, whatever the log's size, since only that end is
// copied and stripped of markup. Trace used to copy the whole log and strip
// all of it before keeping 100 KB: more than 1 GiB allocated for this log,
// with several copies of it held at once, for every call of the 192 the
// process holds open at most. A log the runner timestamped is held to the
// same bound, its headers and the lines it wrote in pieces included.
func TestJobTrace_LongLog_ReadsOnlyItsEnd(t *testing.T) {
	for _, tc := range []struct {
		name string
		log  func() string
	}{
		{"untimestamped", func() string { return markupLog(64 << 20) }},
		{"timestamped", func() string { return timestamped(markupLog(40 << 20)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := tc.log()
			client := serveTrace(t, log)
			client.SetMaxResponseBytes(int64(len(log)) + 1)
			read := allocatedBy(func() {
				if _, _, err := client.GL().Jobs.GetTraceFile("42", 100, gl.WithContext(t.Context())); err != nil {
					t.Fatalf("GetTraceFile() error: %v", err)
				}
			})
			var out TraceOutput
			traced := allocatedBy(func() {
				var err error
				if out, err = Trace(t.Context(), client, TraceInput{ProjectID: "42", JobID: 100}); err != nil {
					t.Fatalf("Trace() error: %v", err)
				}
			})
			t.Logf("Trace allocated %d bytes, client-go's read alone %d, for a log of %d bytes", traced, read, len(log))
			// A race build reallocates what sync.Pool hands back (jobs_race_test.go),
			// so the bound is held only where allocation is what the server pays.
			if !raceEnabled && traced > read+20*traceWindowBytes {
				t.Errorf("Trace allocated %d bytes and client-go's read alone %d, want at most twenty windows of %d between them", traced, read, traceWindowBytes)
			}
			if !strings.HasSuffix(out.Trace, "FATAL unit test TestCheckout failed expected 200 got 500\n") || !out.Truncated {
				t.Errorf("Trace ends %q, truncated %v; want the failure line, truncated", out.Trace[max(0, len(out.Trace)-80):], out.Truncated)
			}
		})
	}
}

const (
	// pathPipelineJobs identifies the path pipeline jobs constant used by this package.
	pathPipelineJobs = "/api/v4/projects/42/pipelines/10/jobs"
	// pathJobGet identifies the path job get constant used by this package.
	pathJobGet = "/api/v4/projects/42/jobs/100"
	// pathJobTrace identifies the path job trace constant used by this package.
	pathJobTrace = "/api/v4/projects/42/jobs/100/trace"
	// pathJobCancel identifies the path job cancel constant used by this package.
	pathJobCancel = "/api/v4/projects/42/jobs/100/cancel"
	// pathJobRetry identifies the path job retry constant used by this package.
	pathJobRetry = "/api/v4/projects/42/jobs/100/retry"

	// testHeaderContentType identifies the test header content type constant used by this package.
	testHeaderContentType = "Content-Type"
	// testReportContent identifies the test report content constant used by this package.
	testReportContent = "test report content"
	// testReportFileName identifies the test report file name constant used by this package.
	testReportFileName = "report.txt"
	// testRefArtifactContent identifies the test ref artifact content constant used by this package.
	testRefArtifactContent = "ref artifact content"
	// fmtIDWant100 identifies the fmt ID want 100 constant used by this package.
	fmtIDWant100 = "ID = %d, want 100"
)

// jobJSON identifies the job JSON constant used by this package.
const jobJSON = `{
	"id":100,
	"name":"build",
	"stage":"build",
	"status":"success",
	"ref":"main",
	"tag":false,
	"allow_failure":false,
	"duration":45.5,
	"queued_duration":2.1,
	"web_url":"https://gitlab.example.com/-/jobs/100",
	"pipeline":{"id":10},
	"created_at":"2026-03-01T10:00:00Z",
	"started_at":"2026-03-01T10:00:05Z",
	"finished_at":"2026-03-01T10:00:50Z",
	"user":{"username":"testuser"},
	"runner":{"id":1}
}`

// TestJobList_Success verifies JobList when success.
func TestJobList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		PipelineID: 10,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	if out.Jobs[0].Name != "build" {
		t.Errorf("Jobs[0].Name = %q, want %q", out.Jobs[0].Name, "build")
	}
	if out.Jobs[0].Status != "success" {
		t.Errorf("Jobs[0].Status = %q, want %q", out.Jobs[0].Status, "success")
	}
	if out.Jobs[0].Pipeline == nil || out.Jobs[0].Pipeline.ID != 10 {
		t.Errorf("Jobs[0].Pipeline = %+v, want id 10", out.Jobs[0].Pipeline)
	}
	if out.Jobs[0].User == nil || out.Jobs[0].User.Username != "testuser" {
		t.Errorf("Jobs[0].User = %+v, want username testuser", out.Jobs[0].User)
	}
	if out.Jobs[0].Runner == nil || out.Jobs[0].Runner.ID != 1 {
		t.Errorf("Jobs[0].Runner = %+v, want ID 1", out.Jobs[0].Runner)
	}
}

// TestJobList_WithScope verifies JobList when with scope.
func TestJobList_WithScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			q := r.URL.Query()
			scopes := q["scope[]"]
			if len(scopes) != 1 || scopes[0] != "failed" {
				t.Errorf("expected scope[]=failed, got %v", scopes)
			}
			testutil.RespondJSON(w, http.StatusOK, "[]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		PipelineID: 10,
		Scope:      []string{"failed"},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Jobs) != 0 {
		t.Errorf("len(Jobs) = %d, want 0", len(out.Jobs))
	}
}

// TestJobGet_Success verifies JobGet when success.
func TestJobGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobGet {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Name != "build" {
		t.Errorf("out.Name = %q, want %q", out.Name, "build")
	}
	if out.Duration == nil || *out.Duration != 45.5 {
		t.Errorf("out.Duration = %v, want 45.5", out.Duration)
	}
}

// jobKeys marshals v and returns its top-level object, for a test about which
// keys a job is published with and what each carries.
func jobKeys(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]any
	if err = json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return keys
}

// neverRanJob is a manual job that never started, which GitLab answers with
// a null duration, queued duration and coverage.
const neverRanJob = `{"id":100,"name":"deploy","stage":"deploy","status":"manual","duration":null,"queued_duration":null,"coverage":null}`

// measuredZeroJob is a job GitLab measured at zero: a real 0 for each, which
// must survive as a 0 rather than vanish with the nulls.
const measuredZeroJob = `{"id":100,"name":"deploy","stage":"deploy","status":"success","duration":0,"queued_duration":0,"coverage":0}`

// assertJobMeasures holds a job's published measures to what GitLab sent:
// absent for a null, present as the number for a measured value.
func assertJobMeasures(t *testing.T, label string, keys map[string]any, measured bool) {
	t.Helper()
	for _, key := range []string{"duration", "queued_duration", "coverage"} {
		got, present := keys[key]
		if !measured && present {
			t.Errorf("%s: %s = %v, want it left out for a job that never ran", label, key, got)
		}
		if measured && (!present || got != float64(0)) {
			t.Errorf("%s: %s = %v (present %v), want the measured 0", label, key, got, present)
		}
	}
}

// TestJobHandlers_Measures_AreWhatGitLabMeasured verifies that every handler
// presenting a job, through the raw decode or through client-go, publishes a
// null duration, queued duration or coverage as absent and a measured zero as
// zero. client-go decodes all three as float64, so a manual job that never
// started read as one that ran for 0.0s, and a real 0% coverage vanished with
// omitempty.
func TestJobHandlers_Measures_AreWhatGitLabMeasured(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		measured bool
	}{{"never ran", neverRanJob, false}, {"measured at zero", measuredZeroJob, true}} {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/jobs") {
					testutil.RespondJSON(w, http.StatusOK, "["+tc.body+"]")
					return
				}
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))
			ctx := context.Background()
			got, err := Get(ctx, client, GetInput{ProjectID: "42", JobID: 100})
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			assertJobMeasures(t, "get", jobKeys(t, got), tc.measured)
			list, err := List(ctx, client, ListInput{ProjectID: "42", PipelineID: 10})
			if err != nil || len(list.Jobs) != 1 {
				t.Fatalf("List() = %+v, %v", list, err)
			}
			assertJobMeasures(t, "list", jobKeys(t, list.Jobs[0]), tc.measured)
			cancelled, err := Cancel(ctx, client, CancelInput{ProjectID: "42", JobID: 100})
			if err != nil {
				t.Fatalf("Cancel() error: %v", err)
			}
			assertJobMeasures(t, "cancel", jobKeys(t, cancelled), tc.measured)
			for name, call := range map[string]func() (Output, error){
				"retry": func() (Output, error) { return Retry(ctx, client, ActionInput{ProjectID: "42", JobID: 100}) },
				"erase": func() (Output, error) { return Erase(ctx, client, ActionInput{ProjectID: "42", JobID: 100}) },
				"keep":  func() (Output, error) { return KeepArtifacts(ctx, client, ActionInput{ProjectID: "42", JobID: 100}) },
				"play":  func() (Output, error) { return Play(ctx, client, PlayInput{ProjectID: "42", JobID: 100}) },
			} {
				out, callErr := call()
				if callErr != nil {
					t.Fatalf("%s error: %v", name, callErr)
				}
				assertJobMeasures(t, name, jobKeys(t, out), tc.measured)
			}
		})
	}
}

// TestJobMarkdown_UnmeasuredDuration_IsADash verifies the list and the card
// do not print a duration GitLab did not measure: the list wrote "0.0s" in
// the Duration column for a job that never started.
func TestJobMarkdown_UnmeasuredDuration_IsADash(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "["+neverRanJob+"]")
	}))
	list, err := List(context.Background(), client, ListInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	md := FormatListMarkdown(list)
	if strings.Contains(md, "0.0s") || !strings.Contains(md, "| - |") {
		t.Errorf("the list states a duration for a job that never ran:\n%s", md)
	}
}

// TestListBridges_Measures_AreWhatGitLabMeasured verifies the same of a
// bridge job, whose durations client-go decodes as float64 too, on both
// routes: gl.Bridge and gl.TriggerJob decode them alike, so each route's
// answer is read off the capture.
func TestListBridges_Measures_AreWhatGitLabMeasured(t *testing.T) {
	for _, route := range bridgeRoutes {
		t.Run(route.name, func(t *testing.T) {
			standIn := atVersion(route.version, `[{"id":5,"name":"trigger","stage":"deploy","status":"manual","duration":null,"queued_duration":null,"coverage":null}]`)
			client := testutil.NewTestClient(t, standIn)
			out, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
			if err != nil || len(out.Bridges) != 1 {
				t.Fatalf("ListBridges() = %+v, %v", out, err)
			}
			standIn.assertRoutes(t, pathVersion, route.path)
			assertJobMeasures(t, "bridge", jobKeys(t, out.Bridges[0]), false)
			if md := FormatBridgeListMarkdown(out); strings.Contains(md, "0.0s") {
				t.Errorf("the bridge list states a duration for a bridge that never ran:\n%s", md)
			}
		})
	}
}

// TestCapturedJob_AnAnswerTheReaderCannotHold_IsReported verifies the
// converter the job writes share reports a captured answer it cannot read
// rather than publishing the job without its measures. It is not reachable
// through the transport, since the SDK decodes the same bytes into fields of
// the same kinds, so the converter is driven directly.
func TestCapturedJob_AnAnswerTheReaderCannotHold_IsReported(t *testing.T) {
	_, untouched := gitlabclient.WithResponseCapture(t.Context())
	if _, err := capturedJob(&gl.Job{ID: 1}, untouched); err == nil || !strings.Contains(err.Error(), "jobMeasures") {
		t.Errorf("capturedJob() with no captured response = %v, want an error naming the operation", err)
	}
}

// jobDistinctJSON is a job in which no two values agree, which is the fixture a
// converter swap cannot survive: the shared fixtures name the job and its stage
// "build" alike, so ToOutput reading the stage into the name passed every test.
// Every documented sub-object is present, each with values of its own.
const jobDistinctJSON = `{
	"id":7001,
	"name":"compile",
	"stage":"assemble",
	"status":"failed",
	"ref":"feature/x",
	"tag":true,
	"allow_failure":false,
	"duration":12.5,
	"queued_duration":3.25,
	"failure_reason":"script_failure",
	"web_url":"https://gitlab.example.com/g/p/-/jobs/7001",
	"created_at":"2026-03-01T10:00:00Z",
	"started_at":"2026-03-01T10:01:00Z",
	"finished_at":"2026-03-01T10:02:00Z",
	"artifacts_expire_at":"2026-04-01T00:00:00Z",
	"coverage":81.5,
	"tag_list":["docker","linux"],
	"erased_at":"2026-03-05T09:00:00Z",
	"archived":true,
	"commit":{
		"id":"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
		"short_id":"a1b2c3d4",
		"title":"Fix the build",
		"author_name":"Ada Lovelace",
		"author_email":"ada@example.com",
		"created_at":"2026-02-28T18:30:00Z",
		"message":"Fix the build\n\nLonger body."
	},
	"pipeline":{"id":901,"project_id":4242,"ref":"release/1.0","sha":"0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c","status":"running"},
	"project":{"ci_job_token_scope_enabled":true},
	"runner":{
		"id":32,
		"description":"shared-runner",
		"ip_address":"10.0.0.7",
		"active":true,
		"paused":false,
		"is_shared":false,
		"runner_type":"instance_type",
		"name":"gitlab-runner",
		"online":false,
		"status":"offline"
	},
	"runner_manager":{
		"id":11,
		"system_id":"s_89e5e9956577",
		"version":"16.11.1",
		"revision":"535ced5f",
		"platform":"linux",
		"architecture":"amd64",
		"created_at":"2024-05-01T10:12:02.507Z",
		"contacted_at":"2024-05-07T06:30:09.355Z",
		"ip_address":"127.0.0.1",
		"status":"online"
	},
	"user":{
		"id":5,
		"name":"Grace Hopper",
		"username":"grace",
		"state":"active",
		"avatar_url":"https://gitlab.example.com/uploads/user/avatar/5/grace.png",
		"web_url":"https://gitlab.example.com/grace",
		"created_at":"2020-01-02T03:04:05Z",
		"bio":"Compiler pioneer",
		"location":"Arlington",
		"public_email":"grace@example.com",
		"linkedin":"grace-hopper",
		"twitter":"gracehopper",
		"website_url":"https://grace.example.com",
		"organization":"US Navy"
	},
	"artifacts":[{"file_type":"archive","filename":"artifacts.zip","size":2048,"file_format":"zip"}],
	"artifacts_file":{"filename":"artifacts.zip","size":2048}
}`

// jobDistinctOutput is what [jobDistinctJSON] must arrive as, field for field.
func jobDistinctOutput() Output {
	return Output{
		ID: 7001, Name: "compile", Stage: "assemble", Status: "failed", Ref: "feature/x",
		Tag: true, AllowFailure: false,
		Duration: new(12.5), QueuedDuration: new(3.25),
		FailureReason:     "script_failure",
		WebURL:            "https://gitlab.example.com/g/p/-/jobs/7001",
		CreatedAt:         "2026-03-01T10:00:00Z",
		StartedAt:         "2026-03-01T10:01:00Z",
		FinishedAt:        "2026-03-01T10:02:00Z",
		ArtifactsExpireAt: "2026-04-01T00:00:00Z",
		Coverage:          new(81.5),
		TagList:           []string{"docker", "linux"},
		ErasedAt:          "2026-03-05T09:00:00Z",
		Archived:          true,
		Commit: &CommitObject{
			ID: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", ShortID: "a1b2c3d4", Title: "Fix the build",
			AuthorName: "Ada Lovelace", AuthorEmail: "ada@example.com",
			CreatedAt: "2026-02-28T18:30:00Z", Message: "Fix the build\n\nLonger body.",
		},
		Pipeline: &PipelineObject{ID: 901, ProjectID: 4242, Ref: "release/1.0", SHA: "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c", Status: "running"},
		Project:  &ProjectObject{CIJobTokenScopeEnabled: true},
		Runner: &RunnerObject{
			ID: 32, Description: "shared-runner", IPAddress: "10.0.0.7", Active: true,
			RunnerType: "instance_type", Name: "gitlab-runner", Status: "offline",
		},
		RunnerManager: &RunnerManagerObject{
			ID: 11, SystemID: "s_89e5e9956577", Version: "16.11.1", Revision: "535ced5f",
			Platform: "linux", Architecture: "amd64",
			CreatedAt: "2024-05-01T10:12:02.507Z", ContactedAt: "2024-05-07T06:30:09.355Z",
			IPAddress: "127.0.0.1", Status: "online",
		},
		User: &UserObject{
			ID: 5, Name: "Grace Hopper", Username: "grace", State: "active",
			AvatarURL: "https://gitlab.example.com/uploads/user/avatar/5/grace.png",
			WebURL:    "https://gitlab.example.com/grace",
			CreatedAt: "2020-01-02T03:04:05Z", Bio: "Compiler pioneer", Location: "Arlington",
			PublicEmail: "grace@example.com", Linkedin: "grace-hopper", Twitter: "gracehopper",
			WebsiteURL: "https://grace.example.com", Organization: "US Navy",
		},
		Artifacts:     []ArtifactObject{{FileType: "archive", Filename: "artifacts.zip", Size: 2048, FileFormat: "zip"}},
		ArtifactsFile: &ArtifactsFileObject{Filename: "artifacts.zip", Size: 2048},
	}
}

// renderForDiff prints an output as indented JSON for a mismatch report, since
// %+v prints the sub-objects as addresses.
func renderForDiff(t *testing.T, v any) string {
	t.Helper()
	rendered, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Errorf("render %T for the report: %v", v, err)
	}
	return string(rendered)
}

// assertJobOutput compares a job field for field and prints both sides as JSON
// on a mismatch.
func assertJobOutput(t *testing.T, got, want Output) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("job output differs\n got %s\nwant %s", renderForDiff(t, got), renderForDiff(t, want))
	}
}

// TestJobGet_MapsEveryDocumentedField holds the whole job, sub-objects
// included, to a fixture where no two values agree. A swap of two assignments
// in a converter has no branch either gate can flip, and the shared fixtures
// hid one by giving the name and the stage the same value.
func TestJobGet_MapsEveryDocumentedField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/7001" {
			testutil.RespondJSON(w, http.StatusOK, jobDistinctJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 7001})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertJobOutput(t, out, jobDistinctOutput())
}

// TestJobGet_Flags_EachSurfacedOnItsOwnKey drives the job's and the runner's
// booleans one at a time: a struct of flags has no fixture where no two values
// agree, so a fixture setting them all cannot tell a swap apart. Each case
// sends one flag and expects exactly that flag back.
func TestJobGet_Flags_EachSurfacedOnItsOwnKey(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Output
	}{
		{"tag", `{"id":1,"tag":true}`, Output{ID: 1, Tag: true}},
		{"allow_failure", `{"id":1,"allow_failure":true}`, Output{ID: 1, AllowFailure: true}},
		{"archived", `{"id":1,"archived":true}`, Output{ID: 1, Archived: true}},
		{"runner.active", `{"id":1,"runner":{"id":2,"active":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, Active: true}}},
		{"runner.paused", `{"id":1,"runner":{"id":2,"paused":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, Paused: true}}},
		{"runner.is_shared", `{"id":1,"runner":{"id":2,"is_shared":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, IsShared: true}}},
		{"runner.online", `{"id":1,"runner":{"id":2,"online":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, Online: true}}},
		{"project.ci_job_token_scope_enabled", `{"id":1,"project":{"ci_job_token_scope_enabled":true}}`, Output{ID: 1, Project: &ProjectObject{CIJobTokenScopeEnabled: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/1" {
					testutil.RespondJSON(w, http.StatusOK, tt.body)
					return
				}
				http.NotFound(w, r)
			}))
			out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 1})
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			assertJobOutput(t, out, tt.want)
		})
	}
}

// jobFullJSON is a single-job fixture that includes the documented fields
// gl.Job does not expose: top-level archived, the runner_manager object, and
// the full runner object (with ip_address, paused, runner_type, online,
// status). Used to assert the raw-fetch handlers surface these 1:1.
const jobFullJSON = `{
	"id":100,
	"name":"build",
	"stage":"build",
	"status":"failed",
	"ref":"main",
	"tag":false,
	"archived":true,
	"runner":{
		"id":32,
		"description":"shared-runner",
		"ip_address":null,
		"active":true,
		"paused":false,
		"is_shared":true,
		"runner_type":"instance_type",
		"name":null,
		"online":false,
		"status":"offline"
	},
	"runner_manager":{
		"id":1,
		"system_id":"s_89e5e9956577",
		"version":"16.11.1",
		"revision":"535ced5f",
		"platform":"linux",
		"architecture":"amd64",
		"created_at":"2024-05-01T10:12:02.507Z",
		"contacted_at":"2024-05-07T06:30:09.355Z",
		"ip_address":"127.0.0.1",
		"status":"offline"
	}
}`

// assertJobFullFields asserts that the raw-fetched documented fields surfaced.
func assertJobFullFields(t *testing.T, out Output) {
	t.Helper()
	if !out.Archived {
		t.Error("Archived = false, want true")
	}
	if out.Runner == nil {
		t.Fatal("Runner = nil, want populated")
	}
	if out.Runner.RunnerType != "instance_type" {
		t.Errorf("Runner.RunnerType = %q, want %q", out.Runner.RunnerType, "instance_type")
	}
	if out.Runner.Status != "offline" {
		t.Errorf("Runner.Status = %q, want %q", out.Runner.Status, "offline")
	}
	if out.Runner.Paused {
		t.Error("Runner.Paused = true, want false")
	}
	if out.Runner.Online {
		t.Error("Runner.Online = true, want false")
	}
	if out.RunnerManager == nil {
		t.Fatal("RunnerManager = nil, want populated")
	}
	if out.RunnerManager.SystemID != "s_89e5e9956577" {
		t.Errorf("RunnerManager.SystemID = %q, want %q", out.RunnerManager.SystemID, "s_89e5e9956577")
	}
	if out.RunnerManager.Version != "16.11.1" {
		t.Errorf("RunnerManager.Version = %q, want %q", out.RunnerManager.Version, "16.11.1")
	}
	if out.RunnerManager.Platform != "linux" {
		t.Errorf("RunnerManager.Platform = %q, want %q", out.RunnerManager.Platform, "linux")
	}
	if out.RunnerManager.IPAddress != "127.0.0.1" {
		t.Errorf("RunnerManager.IPAddress = %q, want %q", out.RunnerManager.IPAddress, "127.0.0.1")
	}
	if out.RunnerManager.Status != "offline" {
		t.Errorf("RunnerManager.Status = %q, want %q", out.RunnerManager.Status, "offline")
	}
}

// TestJobGet_FullFields verifies that Get surfaces the documented archived,
// runner_manager, and runner-extra fields fetched via the raw REST path.
func TestJobGet_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobGet {
			testutil.RespondJSON(w, http.StatusOK, jobFullJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertJobFullFields(t, out)
}

// TestJobList_FullFields verifies that List surfaces the documented archived,
// runner_manager, and runner-extra fields via the raw REST path.
func TestJobList_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobFullJSON))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	assertJobFullFields(t, out.Jobs[0])
}

// TestListProject_FullFields verifies that ListProject surfaces the documented
// archived, runner_manager, and runner-extra fields via the raw REST path.
func TestListProject_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobFullJSON))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListProject() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	assertJobFullFields(t, out.Jobs[0])
}

// TestJobGet_OlderInstanceOmitsFields verifies version tolerance: when an older
// GitLab instance returns a job response WITHOUT archived or runner_manager,
// the raw-fetch handler still succeeds and simply omits those fields (they
// stay zero-valued / nil) rather than failing.
func TestJobGet_OlderInstanceOmitsFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobGet {
			// jobJSON has no archived/runner_manager and a minimal runner.
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Archived {
		t.Error("Archived = true, want false (field absent on older instance)")
	}
	if out.RunnerManager != nil {
		t.Errorf("RunnerManager = %+v, want nil (field absent on older instance)", out.RunnerManager)
	}
	if out.Runner == nil || out.Runner.ID != 1 {
		t.Errorf("Runner = %+v, want minimal runner with ID 1", out.Runner)
	}
}

// TestJobTrace_Success verifies JobTrace when success.
func TestJobTrace_Success(t *testing.T) {
	traceContent := "Running with gitlab-runner 15.0.0\nJob succeeded"
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
			w.Header().Set(testHeaderContentType, "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(traceContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Trace(context.Background(), client, TraceInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Trace() unexpected error: %v", err)
	}
	if out.JobID != 100 {
		t.Errorf("out.JobID = %d, want 100", out.JobID)
	}
	if out.Trace != traceContent {
		t.Errorf("out.Trace = %q, want %q", out.Trace, traceContent)
	}
	if out.Truncated {
		t.Error("out.Truncated = true, want false")
	}
}

// hundredKiB is the limit the trace card's note promises a reader, written as
// its own number so no test here can follow maxTraceBytes wherever it goes.
const hundredKiB = 102400

// TestJobTrace_LimitIsTheHundredKiBTheNotePromises holds the trace limit to
// the figure the truncation note states, from both sides of the boundary: a
// log of exactly that size is returned whole and unmarked, and one byte more
// is cut to it, from the end, and marked. The truncation tests before this
// one measured the limit against the constant that set it, so a constant
// that shrank to a kilobyte passed them all.
func TestJobTrace_LimitIsTheHundredKiBTheNotePromises(t *testing.T) {
	tests := []struct {
		name          string
		size          int
		wantLen       int
		wantTruncated bool
	}{
		{"exactly the limit is kept whole", hundredKiB, hundredKiB, false},
		{"one byte over keeps the last limit of it", hundredKiB + 1, hundredKiB, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Repeat("x", tt.size-1) + "$"
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
					w.Header().Set(testHeaderContentType, "text/plain")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(body))
					return
				}
				http.NotFound(w, r)
			}))

			out, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
			if err != nil {
				t.Fatalf("Trace() unexpected error: %v", err)
			}
			if out.Truncated != tt.wantTruncated {
				t.Errorf("Truncated = %v, want %v for a %d-byte log", out.Truncated, tt.wantTruncated, tt.size)
			}
			if out.Trace != body[len(body)-tt.wantLen:] {
				t.Errorf("Trace is %d bytes and ends %q, want the last %d bytes of the log", len(out.Trace), out.Trace[max(0, len(out.Trace)-3):], tt.wantLen)
			}
		})
	}
}

// TestJobCancel_Success verifies JobCancel when success.
func TestJobCancel_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobCancel {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":100,"name":"build","stage":"build","status":"canceled",
				"ref":"main","tag":false,"duration":10.0,"queued_duration":1.0,
				"web_url":"https://gitlab.example.com/-/jobs/100",
				"pipeline":{"id":10},"created_at":"2026-03-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Cancel(context.Background(), client, CancelInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Cancel() unexpected error: %v", err)
	}
	if out.Status != "canceled" {
		t.Errorf("out.Status = %q, want %q", out.Status, "canceled")
	}
}

// TestJobRetry_Success verifies JobRetry when success.
func TestJobRetry_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobRetry {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":101,"name":"build","stage":"build","status":"pending",
				"ref":"main","tag":false,"duration":0,"queued_duration":0,
				"web_url":"https://gitlab.example.com/-/jobs/101",
				"pipeline":{"id":10},"created_at":"2026-03-01T10:01:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Retry(context.Background(), client, ActionInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Retry() unexpected error: %v", err)
	}
	if out.Status != "pending" {
		t.Errorf("out.Status = %q, want %q", out.Status, "pending")
	}
	if out.ID != 101 {
		t.Errorf("out.ID = %d, want 101", out.ID)
	}
}

// TestJobList_CancelledContext verifies JobList when cancelled context.
func TestJobList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TASK-024 tests.

const (
	// pathProjectJobs identifies the path project jobs constant used by this package.
	pathProjectJobs = "/api/v4/projects/42/jobs"
	// pathJobErase identifies the path job erase constant used by this package.
	pathJobErase = "/api/v4/projects/42/jobs/100/erase"
	// pathJobPlay identifies the path job play constant used by this package.
	pathJobPlay = "/api/v4/projects/42/jobs/100/play"
	// pathJobArtifacts identifies the path job artifacts constant used by this package.
	pathJobArtifacts = "/api/v4/projects/42/jobs/100/artifacts"

	// bridgeJSON identifies the bridge JSON constant used by this package.
	bridgeJSON = `{
		"id":200,"name":"trigger-downstream","stage":"deploy",
		"status":"success","ref":"main","tag":false,"allow_failure":false,
		"duration":10.0,"queued_duration":1.0,
		"web_url":"https://gitlab.example.com/-/jobs/200",
		"pipeline":{"id":10},
		"created_at":"2026-03-01T10:00:00Z",
		"user":{"username":"testuser"},
		"downstream_pipeline":{"id":50}
	}`
)

// TestListProject_Success verifies ListProject when success.
func TestListProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListProject() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	if out.Jobs[0].ID != 100 {
		t.Errorf("Jobs[0].ID = %d, want 100", out.Jobs[0].ID)
	}
}

// TestListBridges_Success verifies ListBridges when success.
func TestListBridges_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipelines/10/bridges" {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeJSON))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("ListBridges() unexpected error: %v", err)
	}
	if len(out.Bridges) != 1 {
		t.Fatalf("len(Bridges) = %d, want 1", len(out.Bridges))
	}
	if out.Bridges[0].ID != 200 {
		t.Errorf("Bridges[0].ID = %d, want 200", out.Bridges[0].ID)
	}
	if out.Bridges[0].DownstreamPipeline == nil || out.Bridges[0].DownstreamPipeline.ID != 50 {
		t.Errorf("Bridges[0].DownstreamPipeline = %+v, want ID 50", out.Bridges[0].DownstreamPipeline)
	}
}

// bridgeDistinctJSON is a bridge in which no two values agree, the pipeline it
// belongs to and the one it triggered included, and the project key the SDK
// does not model beside them.
const bridgeDistinctJSON = `{
	"id":8002,"name":"trigger-child","stage":"fan-out",
	"status":"success","ref":"topic","tag":false,"allow_failure":true,
	"duration":4.5,"queued_duration":1.25,"failure_reason":"","coverage":55.5,
	"web_url":"https://gitlab.example.com/g/p/-/jobs/8002",
	"created_at":"2026-03-02T10:00:00Z","started_at":"2026-03-02T10:01:00Z",
	"finished_at":"2026-03-02T10:02:00Z","erased_at":"2026-03-06T09:00:00Z",
	"commit":{
		"id":"c0ffee0123456789abcdef0123456789abcdef01","short_id":"c0ffee01","title":"Add the child pipeline",
		"author_name":"Linus","author_email":"linus@example.com",
		"created_at":"2026-03-02T09:00:00Z","message":"Add the child pipeline\n"
	},
	"pipeline":{
		"id":902,"project_id":4242,"status":"pending","ref":"main",
		"sha":"9876543210fedcba9876543210fedcba98765432",
		"web_url":"https://gitlab.example.com/g/p/-/pipelines/902",
		"updated_at":"2026-03-02T10:03:00Z","created_at":"2026-03-02T09:59:00Z"
	},
	"user":{"id":6,"name":"Linus","username":"linus","state":"blocked",
		"avatar_url":"https://gitlab.example.com/uploads/user/avatar/6/linus.png",
		"web_url":"https://gitlab.example.com/linus"},
	"downstream_pipeline":{
		"id":903,"project_id":4343,"status":"created","ref":"child",
		"sha":"1357913579135791357913579135791357913579",
		"web_url":"https://gitlab.example.com/g/child/-/pipelines/903",
		"updated_at":"2026-03-02T10:04:00Z","created_at":"2026-03-02T10:01:30Z"
	},
	"project":{"ci_job_token_scope_enabled":true}
}`

// TestListBridges_MapsEveryDocumentedField holds the whole bridge to a fixture
// where no two values agree, the way [TestJobGet_MapsEveryDocumentedField]
// holds the job: TriggerJobToOutput is a converter of the same shape, and its
// pipeline and downstream pipeline are the one pair most alike. Both routes
// answer with GitLab's one bridge entity, so both are held to the same output.
func TestListBridges_MapsEveryDocumentedField(t *testing.T) {
	for _, route := range bridgeRoutes {
		t.Run(route.name, func(t *testing.T) {
			standIn := atVersion(route.version, fmt.Sprintf("[%s]", bridgeDistinctJSON))
			client := testutil.NewTestClient(t, standIn)

			out, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
			if err != nil {
				t.Fatalf("ListBridges() unexpected error: %v", err)
			}
			standIn.assertRoutes(t, pathVersion, route.path)
			if len(out.Bridges) != 1 {
				t.Fatalf("len(Bridges) = %d, want 1", len(out.Bridges))
			}
			if got := out.Bridges[0]; !reflect.DeepEqual(got, wantDistinctBridge) {
				t.Errorf("bridge output differs\n got %s\nwant %s", renderForDiff(t, got), renderForDiff(t, wantDistinctBridge))
			}
		})
	}
}

// wantDistinctBridge is what [bridgeDistinctJSON] publishes as, every value
// in the place GitLab sent it.
var wantDistinctBridge = BridgeOutput{
	ID: 8002, Name: "trigger-child", Stage: "fan-out", Status: "success", Ref: "topic",
	Tag: false, AllowFailure: true,
	Duration: new(4.5), QueuedDuration: new(1.25), Coverage: new(55.5),
	WebURL:     "https://gitlab.example.com/g/p/-/jobs/8002",
	CreatedAt:  "2026-03-02T10:00:00Z",
	StartedAt:  "2026-03-02T10:01:00Z",
	FinishedAt: "2026-03-02T10:02:00Z",
	ErasedAt:   "2026-03-06T09:00:00Z",
	Commit: &CommitObject{
		ID: "c0ffee0123456789abcdef0123456789abcdef01", ShortID: "c0ffee01", Title: "Add the child pipeline",
		AuthorName: "Linus", AuthorEmail: "linus@example.com",
		CreatedAt: "2026-03-02T09:00:00Z", Message: "Add the child pipeline\n",
	},
	Pipeline: &PipelineInfoObject{
		ID: 902, ProjectID: 4242, Status: "pending", Ref: "main",
		SHA:       "9876543210fedcba9876543210fedcba98765432",
		WebURL:    "https://gitlab.example.com/g/p/-/pipelines/902",
		UpdatedAt: "2026-03-02T10:03:00Z", CreatedAt: "2026-03-02T09:59:00Z",
	},
	User: &UserObject{
		ID: 6, Name: "Linus", Username: "linus", State: "blocked",
		AvatarURL: "https://gitlab.example.com/uploads/user/avatar/6/linus.png",
		WebURL:    "https://gitlab.example.com/linus",
	},
	DownstreamPipeline: &PipelineInfoObject{
		ID: 903, ProjectID: 4343, Status: "created", Ref: "child",
		SHA:       "1357913579135791357913579135791357913579",
		WebURL:    "https://gitlab.example.com/g/child/-/pipelines/903",
		UpdatedAt: "2026-03-02T10:04:00Z", CreatedAt: "2026-03-02T10:01:30Z",
	},
	Project: &ProjectObject{CIJobTokenScopeEnabled: true},
}

// The routes a pipeline's bridge list can reach: trigger_jobs, which GitLab
// 19.2 added, and bridges, which the same release deprecated and every release
// serves, beside the version endpoint the handler chooses between them by.
const (
	pathTriggerJobs = "/api/v4/projects/42/pipelines/10/trigger_jobs"
	pathBridges     = "/api/v4/projects/42/pipelines/10/bridges"
	pathVersion     = "/api/v4/version"
	// versionMetadataRefusal is GitLab's refusal of the version endpoint to a
	// fine-grained token without Metadata: Read.
	versionMetadataRefusal = `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read]."}`
)

// bridgeRoutes are the two ways a bridge list runs: through bridges on an
// instance older than 19.2, and through trigger_jobs on 19.2 itself, the
// release that added it, so a change of the release either way fails.
var bridgeRoutes = []struct{ name, version, path string }{
	{"bridges before 19.2", "19.1.3-ee", pathBridges},
	{"trigger_jobs from 19.2", "19.2.0-ee", pathTriggerJobs},
}

// bridgeRequest is one request a [bridgeStandIn] saw: its method and path,
// and the query it carried.
type bridgeRequest struct {
	route string
	query url.Values
}

// bridgeStandIn answers every request a bridge list can send: the version
// endpoint with versionStatus and versionBody (not found when versionStatus
// is zero), either list route with listStatus and body (200 and the
// pagination of [filteredBridgePage] when listStatus is zero), and anything
// else not found, recording each request. It runs on the httptest server's
// goroutine, so it records under a lock and never stops the test.
type bridgeStandIn struct {
	versionStatus int
	versionBody   string
	listStatus    int
	body          string
	// triggerJobsMissing answers trigger_jobs 404 as a release before 19.2
	// does, whatever listStatus says, as one node of an instance in the middle
	// of a rolling upgrade can.
	triggerJobsMissing bool

	mu   sync.Mutex
	seen []bridgeRequest
}

// ServeHTTP records the request and answers it.
func (s *bridgeStandIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.seen = append(s.seen, bridgeRequest{route: r.Method + " " + r.URL.Path, query: r.URL.Query()})
	s.mu.Unlock()
	isList := r.Method == http.MethodGet && (r.URL.Path == pathTriggerJobs || r.URL.Path == pathBridges)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == pathVersion && s.versionStatus != 0:
		testutil.RespondJSON(w, s.versionStatus, s.versionBody)
	case r.Method == http.MethodGet && r.URL.Path == pathTriggerJobs && s.triggerJobsMissing:
		testutil.RespondJSON(w, http.StatusNotFound, `{"error":"404 Not Found"}`)
	case isList && s.listStatus != 0:
		testutil.RespondJSON(w, s.listStatus, s.body)
	case isList:
		testutil.RespondJSONWithPagination(w, http.StatusOK, s.body, testutil.PaginationHeaders{
			Page: "2", PerPage: "5", Total: "7", TotalPages: "2", PrevPage: "1",
		})
	default:
		http.NotFound(w, r)
	}
}

// requests returns the requests recorded so far.
func (s *bridgeStandIn) requests() []bridgeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

// assertRoutes fails the test unless the stand-in saw exactly a GET of each
// path, in order.
func (s *bridgeStandIn) assertRoutes(t *testing.T, paths ...string) {
	t.Helper()
	var got, want []string
	for _, request := range s.requests() {
		got = append(got, request.route)
	}
	for _, path := range paths {
		want = append(want, http.MethodGet+" "+path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("ListBridges() sent %q, want %q", got, want)
	}
}

// lastQuery is the query of the last request the stand-in saw, the list
// request in every test here.
func (s *bridgeStandIn) lastQuery(t *testing.T) url.Values {
	t.Helper()
	seen := s.requests()
	if len(seen) == 0 {
		t.Fatal("the stand-in saw no request")
	}
	return seen[len(seen)-1].query
}

// atVersion is a stand-in for an instance reporting version, answering
// either list route with body.
func atVersion(version, body string) *bridgeStandIn {
	return &bridgeStandIn{versionStatus: http.StatusOK, versionBody: `{"version":"` + version + `"}`, body: body}
}

// filteredBridgeInput sets every parameter a bridge list takes, and
// filteredBridgePage is the pagination the stand-in answers it with.
var (
	filteredBridgeInput = BridgeListInput{
		ProjectID: "42", PipelineID: 10,
		Scope:          []string{"success", "failed"},
		IncludeRetried: true,
		Page:           2, PerPage: 5,
	}
	filteredBridgePage = toolutil.PaginationOutput{Page: 2, PerPage: 5, TotalItems: 7, TotalPages: 2, PrevPage: 1}
)

// assertFilteredBridgeQuery checks that query carries every parameter of
// [filteredBridgeInput] under GitLab's spelling, and nothing else: GitLab
// pages either route by offset alone and orders neither.
func assertFilteredBridgeQuery(t *testing.T, query url.Values) {
	t.Helper()
	want := url.Values{
		"scope[]":         {"success", "failed"},
		"include_retried": {"true"},
		"page":            {"2"},
		"per_page":        {"5"},
	}
	if !reflect.DeepEqual(query, want) {
		t.Errorf("query = %v, want %v", query, want)
	}
}

// TestListBridges_NineteenTwoOrLater_ListsThroughTriggerJobs verifies the
// route an instance known to run GitLab 19.2 or later is sent: trigger_jobs,
// the route that release added, after the version read and in place of the
// bridges route the same release deprecated, with the scope, the page and
// include_retried the caller set. 19.6.0-pre is what GitLab.com reports, and
// a prerelease counts as the minor it names.
func TestListBridges_NineteenTwoOrLater_ListsThroughTriggerJobs(t *testing.T) {
	for _, version := range []string{"19.2.0-ee", "19.5.0-ee", "19.6.0-pre", "20.0.0"} {
		t.Run(version, func(t *testing.T) {
			standIn := atVersion(version, fmt.Sprintf("[%s]", bridgeJSON))
			client := testutil.NewTestClient(t, standIn)

			out, err := ListBridges(t.Context(), client, filteredBridgeInput)
			if err != nil {
				t.Fatalf("ListBridges() error = %v", err)
			}
			standIn.assertRoutes(t, pathVersion, pathTriggerJobs)
			assertFilteredBridgeQuery(t, standIn.lastQuery(t))
			if len(out.Bridges) != 1 || out.Bridges[0].ID != 200 || out.Bridges[0].DownstreamPipeline == nil || out.Bridges[0].DownstreamPipeline.ID != 50 {
				t.Errorf("ListBridges() answered %+v, want the bridge trigger_jobs answered with", out.Bridges)
			}
			if out.Pagination != filteredBridgePage {
				t.Errorf("Pagination = %+v, want %+v", out.Pagination, filteredBridgePage)
			}
		})
	}
}

// TestListBridges_OlderOrUnknownRelease_ListsThroughBridges verifies that an
// instance older than 19.2, or one whose release is not known, is sent the
// bridges route every release serves, with the same parameters: a version
// endpoint that did not answer, one that refused this token Metadata: Read and
// one that named something that is not a version all read as not known.
func TestListBridges_OlderOrUnknownRelease_ListsThroughBridges(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"19.1.3-ee", http.StatusOK, `{"version":"19.1.3-ee"}`},
		{"18.11.0", http.StatusOK, `{"version":"18.11.0"}`},
		{"no answer", http.StatusBadGateway, `{"message":"502 Bad Gateway"}`},
		{"Metadata: Read refused", http.StatusForbidden, versionMetadataRefusal},
		{"not a version", http.StatusOK, `{"version":"unknown"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			standIn := &bridgeStandIn{versionStatus: tt.status, versionBody: tt.body, body: fmt.Sprintf("[%s]", bridgeJSON)}
			client := testutil.NewTestClient(t, standIn)

			out, err := ListBridges(t.Context(), client, filteredBridgeInput)
			if err != nil {
				t.Fatalf("ListBridges() error = %v", err)
			}
			standIn.assertRoutes(t, pathVersion, pathBridges)
			assertFilteredBridgeQuery(t, standIn.lastQuery(t))
			if len(out.Bridges) != 1 || out.Bridges[0].ID != 200 {
				t.Errorf("ListBridges() answered %+v, want the bridge the bridges route answered with", out.Bridges)
			}
			if out.Pagination != filteredBridgePage {
				t.Errorf("Pagination = %+v, want %+v", out.Pagination, filteredBridgePage)
			}
		})
	}
}

// TestListBridges_NoFilter_SendsNoParameter verifies that a caller who set
// no filter sends none on either route, so GitLab applies its own defaults:
// every status, the latest attempt of each bridge and its first page.
func TestListBridges_NoFilter_SendsNoParameter(t *testing.T) {
	for _, route := range bridgeRoutes {
		t.Run(route.name, func(t *testing.T) {
			standIn := atVersion(route.version, "[]")
			client := testutil.NewTestClient(t, standIn)

			if _, err := ListBridges(t.Context(), client, BridgeListInput{ProjectID: "42", PipelineID: 10}); err != nil {
				t.Fatalf("ListBridges() error = %v", err)
			}
			standIn.assertRoutes(t, pathVersion, route.path)
			if query := standIn.lastQuery(t); len(query) != 0 {
				t.Errorf("query = %v, want none", query)
			}
		})
	}
}

// TestListBridges_TriggerJobsRefusal_CarriesTheHintOnNotFoundOnly verifies
// that the trigger_jobs route's refusals are answered as the bridges route's
// are: a 404 with the pipeline hint, once the bridges route has answered 404
// too, and a 403 without it and without asking the bridges route.
func TestListBridges_TriggerJobsRefusal_CarriesTheHintOnNotFoundOnly(t *testing.T) {
	const hint = "Bridges only exist"
	tests := []struct {
		name     string
		status   int
		routes   []string
		wantHint bool
	}{
		{"404 carries the hint", http.StatusNotFound, []string{pathVersion, pathTriggerJobs, pathBridges}, true},
		{"403 does not", http.StatusForbidden, []string{pathVersion, pathTriggerJobs}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			standIn := atVersion("19.2.0-ee", `{"message":"refused"}`)
			standIn.listStatus = tt.status
			client := testutil.NewTestClient(t, standIn)

			_, err := ListBridges(t.Context(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			standIn.assertRoutes(t, tt.routes...)
			if got := strings.Contains(err.Error(), hint); got != tt.wantHint {
				t.Errorf("error %q carries the hint %q: %t, want %t", err, hint, got, tt.wantHint)
			}
		})
	}
}

// TestListBridges_TriggerJobsNotFound_AsksTheBridgesRoute verifies that a
// list sent to trigger_jobs and answered 404 is asked once more through the
// bridges route every release serves, with the same parameters, and answers
// with what that route sent.
//
// The release is read once per client and kept, and an instance upgraded one
// node at a time behind a balancer can answer the version read from a node on
// 19.2 and the list from one still before it, which has no trigger_jobs
// route. A pipeline that is not there answers 404 on both routes, which the
// refusal test above holds.
func TestListBridges_TriggerJobsNotFound_AsksTheBridgesRoute(t *testing.T) {
	standIn := atVersion("19.5.0-ee", fmt.Sprintf("[%s]", bridgeJSON))
	standIn.triggerJobsMissing = true
	client := testutil.NewTestClient(t, standIn)

	out, err := ListBridges(t.Context(), client, filteredBridgeInput)
	if err != nil {
		t.Fatalf("ListBridges() error = %v", err)
	}
	standIn.assertRoutes(t, pathVersion, pathTriggerJobs, pathBridges)
	assertFilteredBridgeQuery(t, standIn.lastQuery(t))
	if len(out.Bridges) != 1 || out.Bridges[0].ID != 200 {
		t.Errorf("ListBridges() answered %+v, want the bridge the bridges route answered with", out.Bridges)
	}
	if out.Pagination != filteredBridgePage {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, filteredBridgePage)
	}
}

// TestGetArtifacts_Success verifies GetArtifacts when success.
func TestGetArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobArtifacts {
			w.Header().Set(testHeaderContentType, "application/zip")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PK\x03\x04fake-zip-content"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetArtifacts(context.Background(), client, ArtifactsInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("GetArtifacts() unexpected error: %v", err)
	}
	if out.Size == 0 {
		t.Error("Size = 0, want > 0")
	}
	if out.Content == "" {
		t.Error("Content is empty")
	}
}

// TestGetArtifacts_FileType_ReachesTheQueryOnlyWhenGiven verifies the one
// parameter the artifacts route gained in GitLab 19.4: a file_type the caller
// names reaches the query string with its value, and a call naming none sends
// no file_type at all, so it asks for the archive exactly as it did before the
// parameter existed and an instance older than 19.4 is sent nothing it does
// not declare. The content GitLab answers with is what the output carries,
// whichever artifact that is.
func TestGetArtifacts_FileType_ReachesTheQueryOnlyWhenGiven(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		body     string
	}{
		{name: "junit report", fileType: "junit", body: "\x1f\x8bjunit-gzip"},
		{name: "sarif report", fileType: "sarif", body: `{"version":"2.1.0"}`},
		{name: "no file_type", body: "PK\x03\x04archive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != pathJobArtifacts {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				query := r.URL.Query()
				if _, sent := query["file_type"]; sent != (tt.fileType != "") || query.Get("file_type") != tt.fileType {
					t.Errorf("query = %q, want file_type=%q sent only when given", r.URL.RawQuery, tt.fileType)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.body))
			}))

			out, err := GetArtifacts(context.Background(), client, ArtifactsInput{ProjectID: "42", JobID: 100, FileType: tt.fileType})
			if err != nil {
				t.Fatalf("GetArtifacts() unexpected error: %v", err)
			}
			decoded, decodeErr := base64.StdEncoding.DecodeString(out.Content)
			if decodeErr != nil || string(decoded) != tt.body {
				t.Errorf("Content decodes to %q (err %v), want the %d bytes GitLab answered", decoded, decodeErr, len(tt.body))
			}
			if out.JobID != 100 || out.Size != len(tt.body) || out.Truncated {
				t.Errorf("JobID/Size/Truncated = %d/%d/%v, want 100/%d/false", out.JobID, out.Size, out.Truncated, len(tt.body))
			}
		})
	}
}

// TestGetArtifacts_NotFound_HintNamesTheFileType verifies the corrective text
// a 404 carries now that a 404 has one more cause: GitLab answers it for a
// file_type the job produced no artifact of, as well as for a job whose
// artifacts are gone. Any other refusal carries no such hint.
func TestGetArtifacts_NotFound_HintNamesTheFileType(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "404 names file_type", status: http.StatusNotFound, wantHint: true},
		{name: "403 has no artifact hint", status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"refused"}`)
			}))
			_, err := GetArtifacts(context.Background(), client, ArtifactsInput{ProjectID: "42", JobID: 100, FileType: "junit"})
			if err == nil {
				t.Fatal("GetArtifacts() error = nil, want the refusal")
			}
			if got := strings.Contains(err.Error(), "verify job_id and file_type"); got != tt.wantHint {
				t.Errorf("error %q carries the file_type hint = %v, want %v", err, got, tt.wantHint)
			}
		})
	}
}

// oneMiB is the limit the artifact cards name, written as its own number so
// no test here can follow maxArtifactBytes wherever it goes.
const oneMiB = 1048576

// serveArtifact answers path with body and refuses anything else.
func serveArtifact(t *testing.T, path, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == path {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
}

// TestGetArtifacts_ArchiveOverTheLimit_ReturnsItsSizeAndNoContent holds
// the archive limit to the mebibyte the card names, from both sides: an
// archive that fits comes back whole, and one byte more comes back with its
// real size, truncated set and no content at all. It used to come back cut
// to its first MiB, base64-encoded: a zip keeps its central directory at the
// end, so the prefix could not be opened, and it cost a model some 350k
// tokens of nothing it could use.
func TestGetArtifacts_ArchiveOverTheLimit_ReturnsItsSizeAndNoContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{{"exactly the limit is returned whole", oneMiB}, {"one byte over is withheld", oneMiB + 1}} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Repeat("x", tc.size-1) + "$"
			out, err := GetArtifacts(t.Context(), serveArtifact(t, pathJobArtifacts, body), ArtifactsInput{ProjectID: "42", JobID: 100})
			if err != nil {
				t.Fatalf("GetArtifacts() unexpected error: %v", err)
			}
			over := tc.size > oneMiB
			if out.Truncated != over || out.Size != tc.size {
				t.Errorf("Truncated/Size = %v/%d, want %v/%d", out.Truncated, out.Size, over, tc.size)
			}
			want := base64.StdEncoding.EncodeToString([]byte(body))
			if over {
				want = ""
			}
			if out.Content != want {
				t.Errorf("Content holds %d characters, want %d", len(out.Content), len(want))
			}
		})
	}
}

// TestDownloadArtifacts_ArchiveOverTheLimit_ReturnsItsSizeAndNoContent is
// the same rule on the by-ref download, which returns through the same read.
func TestDownloadArtifacts_ArchiveOverTheLimit_ReturnsItsSizeAndNoContent(t *testing.T) {
	body := strings.Repeat("x", oneMiB+1)
	client := serveArtifact(t, "/api/v4/projects/42/jobs/artifacts/main/download", body)
	out, err := DownloadArtifacts(t.Context(), client, DownloadArtifactsInput{ProjectID: "42", RefName: "main", JobName: "build"})
	if err != nil {
		t.Fatalf("DownloadArtifacts() unexpected error: %v", err)
	}
	if !out.Truncated || out.Size != oneMiB+1 || out.Content != "" {
		t.Errorf("Truncated/Size/Content = %v/%d/%d characters, want true/%d/none", out.Truncated, out.Size, len(out.Content), oneMiB+1)
	}
}

// TestArtifacts_ResponseOverTheClientCeiling_NamesTheSingleFileRead verifies
// that an archive the client-wide response ceiling stops is answered with
// the way out, a read of one file of it, rather than a bare error, and that
// the way out takes what the caller has: the read by job ID after the
// download by job ID, and the read by ref after the download by ref, whose
// caller holds a ref and a job name and no job ID.
func TestArtifacts_ResponseOverTheClientCeiling_NamesTheSingleFileRead(t *testing.T) {
	client := serveArtifact(t, pathJobArtifacts, strings.Repeat("x", 4096))
	client.SetMaxResponseBytes(1024)
	_, err := GetArtifacts(t.Context(), client, ArtifactsInput{ProjectID: "42", JobID: 100})
	if err == nil || !strings.Contains(err.Error(), "job.download_single_artifact") || strings.Contains(err.Error(), "_by_ref") {
		t.Fatalf("GetArtifacts() error = %v, want it to name job.download_single_artifact", err)
	}
	client = serveArtifact(t, "/api/v4/projects/42/jobs/artifacts/main/download", strings.Repeat("x", 4096))
	client.SetMaxResponseBytes(1024)
	_, err = DownloadArtifacts(t.Context(), client, DownloadArtifactsInput{ProjectID: "42", RefName: "main", JobName: "build"})
	if err == nil || !strings.Contains(err.Error(), "job.download_single_artifact_by_ref") {
		t.Fatalf("DownloadArtifacts() error = %v, want it to name job.download_single_artifact_by_ref", err)
	}
}

// TestSingleArtifact_ResponseOverTheClientCeiling_NamesTheWayOut verifies
// that a file the client-wide response ceiling stops is answered with the
// route to download it from GitLab directly, by job and by ref.
func TestSingleArtifact_ResponseOverTheClientCeiling_NamesTheWayOut(t *testing.T) {
	client := serveArtifact(t, "/api/v4/projects/42/jobs/100/artifacts/out.bin", strings.Repeat("x", 4096))
	client.SetMaxResponseBytes(1024)
	_, err := DownloadSingleArtifact(t.Context(), client, SingleArtifactInput{ProjectID: "42", JobID: 100, ArtifactPath: "out.bin"})
	if err == nil || !strings.Contains(err.Error(), "download it from GitLab directly") {
		t.Fatalf("DownloadSingleArtifact() error = %v, want the way out", err)
	}
	client = serveArtifact(t, "/api/v4/projects/42/jobs/artifacts/main/raw/out.bin", strings.Repeat("x", 4096))
	client.SetMaxResponseBytes(1024)
	_, err = DownloadSingleArtifactByRef(t.Context(), client, SingleArtifactRefInput{ProjectID: "42", RefName: "main", ArtifactPath: "out.bin", JobName: "build"})
	if err == nil || !strings.Contains(err.Error(), "download it from GitLab directly") {
		t.Fatalf("DownloadSingleArtifactByRef() error = %v, want the way out", err)
	}
}

// TestDownloadSingleArtifact_Content_KeepsEveryByte verifies that a single
// artifact comes back as text when its bytes are valid UTF-8 and base64
// otherwise, the encoding field saying which, so a binary file survives the
// JSON result byte for byte. Its bytes used to be put into a JSON string as
// they were, and every invalid UTF-8 byte of a PNG came back as U+FFFD.
func TestDownloadSingleArtifact_Content_KeepsEveryByte(t *testing.T) {
	binary := string([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0xff, 0xfe, 0x00, 0x80})
	for _, tc := range []struct {
		name, body, wantEncoding, wantContent string
	}{
		{"text", "ok: 3 passed\nñ done\n", "text", "ok: 3 passed\nñ done\n"},
		{"binary", binary, "base64", base64.StdEncoding.EncodeToString([]byte(binary))},
		{"empty", "", "text", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := serveArtifact(t, "/api/v4/projects/42/jobs/100/artifacts/out.bin", tc.body)
			out, err := DownloadSingleArtifact(t.Context(), client, SingleArtifactInput{ProjectID: "42", JobID: 100, ArtifactPath: "out.bin"})
			if err != nil {
				t.Fatalf("DownloadSingleArtifact() error: %v", err)
			}
			if out.Encoding != tc.wantEncoding || out.Content != tc.wantContent || out.Size != len(tc.body) || out.Truncated {
				t.Errorf("got encoding %q, %d characters, size %d, truncated %v; want %q, %d, %d, false",
					out.Encoding, len(out.Content), out.Size, out.Truncated, tc.wantEncoding, len(tc.wantContent), len(tc.body))
			}
		})
	}
}

// TestDownloadSingleArtifact_OverTheLimit_KeepsAReadableTextPrefix verifies
// the single file over the mebibyte: text keeps its first mebibyte, cut back
// to the last whole character so the prefix is still valid UTF-8, with the
// real size and truncated set; a binary over the limit is withheld, since a
// prefix of it is no more readable than one of an archive.
func TestDownloadSingleArtifact_OverTheLimit_KeepsAReadableTextPrefix(t *testing.T) {
	// The two-byte "é" straddles the limit, so a byte cut leaves half of it.
	text := strings.Repeat("x", oneMiB-1) + "é" + "tail"
	binary := strings.Repeat("\xff", oneMiB+1)
	for _, tc := range []struct {
		name, body, wantEncoding, wantContent string
	}{
		{"text", text, "text", strings.Repeat("x", oneMiB-1)},
		{"binary", binary, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := serveArtifact(t, "/api/v4/projects/42/jobs/artifacts/main/raw/big.out", tc.body)
			out, err := DownloadSingleArtifactByRef(t.Context(), client, SingleArtifactRefInput{ProjectID: "42", RefName: "main", ArtifactPath: "big.out", JobName: "build"})
			if err != nil {
				t.Fatalf("DownloadSingleArtifactByRef() error: %v", err)
			}
			if out.Encoding != tc.wantEncoding || out.Content != tc.wantContent || out.Size != len(tc.body) || !out.Truncated {
				t.Errorf("got encoding %q, %d characters, size %d, truncated %v; want %q, %d, %d, true",
					out.Encoding, len(out.Content), out.Size, out.Truncated, tc.wantEncoding, len(tc.wantContent), len(tc.body))
			}
		})
	}
}

// TestDownloadSingleArtifact_ExactlyTheLimit_IsWhole verifies that a single
// file exactly a mebibyte long comes back whole and is not marked truncated:
// only a file over the limit loses anything, and a binary one loses all of it.
func TestDownloadSingleArtifact_ExactlyTheLimit_IsWhole(t *testing.T) {
	body := strings.Repeat("\xff", oneMiB)
	client := serveArtifact(t, "/api/v4/projects/42/jobs/100/artifacts/out.bin", body)
	out, err := DownloadSingleArtifact(t.Context(), client, SingleArtifactInput{ProjectID: "42", JobID: 100, ArtifactPath: "out.bin"})
	if err != nil {
		t.Fatalf("DownloadSingleArtifact() error: %v", err)
	}
	if out.Truncated || out.Encoding != "base64" || out.Content != base64.StdEncoding.EncodeToString([]byte(body)) || out.Size != oneMiB {
		t.Errorf("got encoding %q, %d characters, size %d, truncated %v; want the whole mebibyte base64-encoded",
			out.Encoding, len(out.Content), out.Size, out.Truncated)
	}
}

// TestDownloadArtifacts_Success verifies DownloadArtifacts reaches the ref's
// download endpoint with the job name the caller gave as its job parameter,
// which is what selects the job whose artifacts come back.
func TestDownloadArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/artifacts/main/download" {
			if got := r.URL.Query().Get("job"); got != "build" {
				t.Errorf("job query = %q, want build", got)
			}
			w.Header().Set(testHeaderContentType, "application/zip")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PK\x03\x04fake-zip"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DownloadArtifacts(context.Background(), client, DownloadArtifactsInput{
		ProjectID: "42", RefName: "main", JobName: "build",
	})
	if err != nil {
		t.Fatalf("DownloadArtifacts() unexpected error: %v", err)
	}
	if out.Size == 0 {
		t.Error("Size = 0, want > 0")
	}
}

// TestDownloadSingleArtifact_Success verifies DownloadSingleArtifact when success.
func TestDownloadSingleArtifact_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/100/artifacts/"+testReportFileName {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testReportContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{
		ProjectID: "42", JobID: 100, ArtifactPath: testReportFileName,
	})
	if err != nil {
		t.Fatalf("DownloadSingleArtifact() unexpected error: %v", err)
	}
	if out.Content != testReportContent {
		t.Errorf("Content = %q, want %q", out.Content, testReportContent)
	}
	if out.ArtifactPath != testReportFileName {
		t.Errorf("ArtifactPath = %q, want %q", out.ArtifactPath, testReportFileName)
	}
}

// TestDownloadSingleArtifactByRef_Success verifies DownloadSingleArtifactByRef
// reaches the ref's raw endpoint with the job name as its job parameter.
func TestDownloadSingleArtifactByRef_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/artifacts/main/raw/"+testReportFileName {
			if got := r.URL.Query().Get("job"); got != "build" {
				t.Errorf("job query = %q, want build", got)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testRefArtifactContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: testReportFileName, JobName: "build",
	})
	if err != nil {
		t.Fatalf("DownloadSingleArtifactByRef() unexpected error: %v", err)
	}
	if out.Content != testRefArtifactContent {
		t.Errorf("Content = %q, want %q", out.Content, testRefArtifactContent)
	}
}

// TestErase_Success verifies Erase when success.
func TestErase_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobErase {
			testutil.RespondJSON(w, http.StatusCreated, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Erase() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestKeepArtifacts_Success verifies KeepArtifacts when success.
func TestKeepArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/jobs/100/artifacts/keep" {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("KeepArtifacts() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestPlay_Success verifies Play when success.
func TestPlay_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Play() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestDeleteArtifacts_Success verifies DeleteArtifacts when success.
func TestDeleteArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathJobArtifacts {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("DeleteArtifacts() unexpected error: %v", err)
	}
}

// TestDeleteProjectArtifacts_Success verifies DeleteProjectArtifacts when success.
func TestDeleteProjectArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/artifacts" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("DeleteProjectArtifacts() unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestProjectIDRequired_Validation ensures every handler refuses an empty
// project_id before reaching GitLab. The forbidden mock is what makes the
// assertion about the handler: the tests this replaces answered 404 to
// whatever arrived, so an error came back with the check deleted too.
func TestProjectIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{PipelineID: 10}); return e }},
		{"ListProject", func() error { _, e := ListProject(ctx, client, ListProjectInput{}); return e }},
		{"ListBridges", func() error { _, e := ListBridges(ctx, client, BridgeListInput{PipelineID: 10}); return e }},
		{"Get", func() error { _, e := Get(ctx, client, GetInput{JobID: 100}); return e }},
		{"Trace", func() error { _, e := Trace(ctx, client, TraceInput{JobID: 100}); return e }},
		{"Cancel", func() error { _, e := Cancel(ctx, client, CancelInput{JobID: 100}); return e }},
		{"Retry", func() error { _, e := Retry(ctx, client, ActionInput{JobID: 100}); return e }},
		{"GetArtifacts", func() error { _, e := GetArtifacts(ctx, client, ArtifactsInput{JobID: 100}); return e }},
		{"DownloadArtifacts", func() error {
			_, e := DownloadArtifacts(ctx, client, DownloadArtifactsInput{RefName: "main", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifact", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{JobID: 100, ArtifactPath: "a.txt"})
			return e
		}},
		{"DownloadSingleArtifactByRef", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{RefName: "main", ArtifactPath: "a.txt", JobName: "build"})
			return e
		}},
		{"Erase", func() error { _, e := Erase(ctx, client, ActionInput{JobID: 100}); return e }},
		{"KeepArtifacts", func() error { _, e := KeepArtifacts(ctx, client, ActionInput{JobID: 100}); return e }},
		{"Play", func() error { _, e := Play(ctx, client, PlayInput{JobID: 100}); return e }},
		{"DeleteArtifacts", func() error { return DeleteArtifacts(ctx, client, DeleteArtifactsInput{JobID: 100}) }},
		{"DeleteProjectArtifacts", func() error { return DeleteProjectArtifacts(ctx, client, DeleteProjectArtifactsInput{}) }},
		{"Wait", func() error { _, e := Wait(ctx, nil, client, WaitInput{JobID: 100}); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "project_id")
		})
	}
}

// TestStringFieldsRequired_Validation ensures the by-ref and by-path downloads
// refuse a missing ref_name, artifact_path or job by name before reaching
// GitLab, on the same forbidden mock as the project_id table.
func TestStringFieldsRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		want string
		fn   func() error
	}{
		{"DownloadArtifacts_ref_name", "ref_name", func() error {
			_, e := DownloadArtifacts(ctx, client, DownloadArtifactsInput{ProjectID: pid, JobName: "build"})
			return e
		}},
		{"DownloadArtifacts_job", "job is required", func() error {
			_, e := DownloadArtifacts(ctx, client, DownloadArtifactsInput{ProjectID: pid, RefName: "main"})
			return e
		}},
		{"DownloadSingleArtifact_artifact_path", "artifact_path", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{ProjectID: pid, JobID: 100})
			return e
		}},
		{"DownloadSingleArtifactByRef_ref_name", "ref_name", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{ProjectID: pid, ArtifactPath: "a.txt", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifactByRef_artifact_path", "artifact_path", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{ProjectID: pid, RefName: "main", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifactByRef_job", "job is required", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{ProjectID: pid, RefName: "main", ArtifactPath: "a.txt"})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), tt.want)
		})
	}
}

// TestJobIDRequired_Validation ensures all handlers that require job_id
// reject zero and negative values.
func TestJobIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get_zero", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, JobID: 0}); return e }},
		{"Get_negative", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, JobID: -1}); return e }},
		{"Trace_zero", func() error { _, e := Trace(ctx, client, TraceInput{ProjectID: pid, JobID: 0}); return e }},
		{"Trace_negative", func() error { _, e := Trace(ctx, client, TraceInput{ProjectID: pid, JobID: -1}); return e }},
		{"Cancel_zero", func() error { _, e := Cancel(ctx, client, CancelInput{ProjectID: pid, JobID: 0}); return e }},
		{"Cancel_negative", func() error { _, e := Cancel(ctx, client, CancelInput{ProjectID: pid, JobID: -3}); return e }},
		{"Retry_zero", func() error { _, e := Retry(ctx, client, ActionInput{ProjectID: pid, JobID: 0}); return e }},
		{"Retry_negative", func() error { _, e := Retry(ctx, client, ActionInput{ProjectID: pid, JobID: -1}); return e }},
		{"GetArtifacts_zero", func() error { _, e := GetArtifacts(ctx, client, ArtifactsInput{ProjectID: pid, JobID: 0}); return e }},
		{"GetArtifacts_negative", func() error { _, e := GetArtifacts(ctx, client, ArtifactsInput{ProjectID: pid, JobID: -1}); return e }},
		{"DownloadSingleArtifact_zero", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{ProjectID: pid, JobID: 0, ArtifactPath: "a.txt"})
			return e
		}},
		{"DownloadSingleArtifact_negative", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{ProjectID: pid, JobID: -2, ArtifactPath: "a.txt"})
			return e
		}},
		{"Erase_zero", func() error { _, e := Erase(ctx, client, ActionInput{ProjectID: pid, JobID: 0}); return e }},
		{"Erase_negative", func() error { _, e := Erase(ctx, client, ActionInput{ProjectID: pid, JobID: -1}); return e }},
		{"KeepArtifacts_zero", func() error { _, e := KeepArtifacts(ctx, client, ActionInput{ProjectID: pid, JobID: 0}); return e }},
		{"KeepArtifacts_negative", func() error { _, e := KeepArtifacts(ctx, client, ActionInput{ProjectID: pid, JobID: -5}); return e }},
		{"Play_zero", func() error { _, e := Play(ctx, client, PlayInput{ProjectID: pid, JobID: 0}); return e }},
		{"Play_negative", func() error { _, e := Play(ctx, client, PlayInput{ProjectID: pid, JobID: -1}); return e }},
		{"DeleteArtifacts_zero", func() error { return DeleteArtifacts(ctx, client, DeleteArtifactsInput{ProjectID: pid, JobID: 0}) }},
		{"DeleteArtifacts_negative", func() error { return DeleteArtifacts(ctx, client, DeleteArtifactsInput{ProjectID: pid, JobID: -1}) }},
		// Wait was tested for a zero job_id against a mock that answered 404,
		// so its check could drop the zero and the mock still produced the error.
		{"Wait_zero", func() error { _, e := Wait(ctx, nil, client, WaitInput{ProjectID: pid, JobID: 0}); return e }},
		{"Wait_negative", func() error { _, e := Wait(ctx, nil, client, WaitInput{ProjectID: pid, JobID: -1}); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "job_id")
		})
	}
}

// TestPipelineIDRequired_ValidationJobs ensures handlers that require pipeline_id
// reject zero and negative values.
func TestPipelineIDRequired_ValidationJobs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List_zero", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"List_negative", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, PipelineID: -1}); return e }},
		{"ListBridges_zero", func() error {
			_, e := ListBridges(ctx, client, BridgeListInput{ProjectID: pid, PipelineID: 0})
			return e
		}},
		{"ListBridges_negative", func() error {
			_, e := ListBridges(ctx, client, BridgeListInput{ProjectID: pid, PipelineID: -1})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "pipeline_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List: API error, pagination params, include_retried
// ---------------------------------------------------------------------------.

// TestJobList_APIError verifies JobList when API error.
func TestJobList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestReadHandlers_NotFoundHint_GatedOnTheStatus verifies each read handler
// attaches its own corrective hint to a 404 and to nothing else: a 403 from
// the same endpoint carries the generic message. The status literal in each
// WrapErrWithStatusHint is what decides it, and no gate mutates a literal, so
// a handler that checked the wrong code kept every other test green.
func TestReadHandlers_NotFoundHint_GatedOnTheStatus(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		hint string
		call func(client *gitlabclient.Client) error
	}{
		{"List", actionPipelineList, func(c *gitlabclient.Client) error {
			_, e := List(ctx, c, ListInput{ProjectID: "42", PipelineID: 10})
			return e
		}},
		{"ListProject", "project.get", func(c *gitlabclient.Client) error {
			_, e := ListProject(ctx, c, ListProjectInput{ProjectID: "42"})
			return e
		}},
		{"ListBridges", "Bridges only exist", func(c *gitlabclient.Client) error {
			_, e := ListBridges(ctx, c, BridgeListInput{ProjectID: "42", PipelineID: 10})
			return e
		}},
		{"Get", "not the per-pipeline index", func(c *gitlabclient.Client) error {
			_, e := Get(ctx, c, GetInput{ProjectID: "42", JobID: 100})
			return e
		}},
		{"Trace", "erased/expired", func(c *gitlabclient.Client) error {
			_, e := Trace(ctx, c, TraceInput{ProjectID: "42", JobID: 100})
			return e
		}},
		{"GetArtifacts", "expire_in", func(c *gitlabclient.Client) error {
			_, e := GetArtifacts(ctx, c, ArtifactsInput{ProjectID: "42", JobID: 100})
			return e
		}},
		{"DownloadArtifacts", "non-expired artifacts", func(c *gitlabclient.Client) error {
			_, e := DownloadArtifacts(ctx, c, DownloadArtifactsInput{ProjectID: "42", RefName: "main", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifact", "artifacts:paths in .gitlab-ci.yml", func(c *gitlabclient.Client) error {
			_, e := DownloadSingleArtifact(ctx, c, SingleArtifactInput{ProjectID: "42", JobID: 100, ArtifactPath: "a.txt"})
			return e
		}},
		{"DownloadSingleArtifactByRef", "produced an artifact at artifact_path", func(c *gitlabclient.Client) error {
			_, e := DownloadSingleArtifactByRef(ctx, c, SingleArtifactRefInput{ProjectID: "42", RefName: "main", ArtifactPath: "a.txt", JobName: "build"})
			return e
		}},
		{"Wait", "deleted or expired during polling", func(c *gitlabclient.Client) error {
			_, e := Wait(ctx, nil, c, WaitInput{ProjectID: "42", JobID: 100, IntervalSeconds: 5, TimeoutSeconds: 30})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("404 carries the hint", func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
				}))
				assertContains(t, tt.call(client), tt.hint)
			})
			t.Run("403 does not", func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
				}))
				err := tt.call(client)
				if err == nil {
					t.Fatal(errExpectedAPI)
				}
				if strings.Contains(err.Error(), tt.hint) {
					t.Errorf("403 error %q carries the 404 hint %q", err, tt.hint)
				}
			})
		})
	}
}

// assertListQuery checks, inside a mock, that every list parameter a caller
// can set reached the query under GitLab's own spelling; it is called with the
// values the test sent, so a forwarding dropped from one handler is caught by
// that handler's own test rather than by the helper they share.
func assertListQuery(t *testing.T, r *http.Request, want map[string]string) {
	t.Helper()
	q := r.URL.Query()
	for name, value := range want {
		if got := q.Get(name); got != value {
			t.Errorf("query %s = %q, want %q", name, got, value)
		}
	}
}

// TestJobList_ForwardsEveryListParameter verifies List sends the retried flag,
// the offset page, the keyset cursor and the ordering as the caller set them,
// and reads the pagination block off the response it got.
func TestJobList_ForwardsEveryListParameter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			assertListQuery(t, r, map[string]string{
				"include_retried": "true", "page": "2", "per_page": "5",
				"order_by": "id", "sort": "asc", "pagination": "keyset", "page_token": "tok-list",
			})
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:      "42",
		PipelineID:     10,
		IncludeRetried: true,
		OrderBy:        "id",
		Sort:           "asc",
		Page:           2, PerPage: 5,
		Pagination: "keyset", PageToken: "tok-list",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	wantPage := toolutil.PaginationOutput{Page: 2, PerPage: 5, TotalItems: 10, TotalPages: 2, PrevPage: 1}
	if out.Pagination != wantPage {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, wantPage)
	}
}

// ---------------------------------------------------------------------------
// Get: API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobGet_APIError verifies JobGet when API error.
func TestJobGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobGet_CancelledContext verifies JobGet when cancelled context.
func TestJobGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Trace: API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobTrace_APIError verifies JobTrace when API error.
func TestJobTrace_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobTrace_CancelledContext verifies JobTrace when cancelled context.
func TestJobTrace_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Trace(ctx, client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestJobTrace_BodyReadError verifies Trace wraps non-EOF/non-UnexpectedEOF
// errors from the body reader. The server claims gzip content-encoding but
// writes raw text, so the http transport's auto-decompression fails with a
// gzip error that is neither io.EOF nor io.ErrUnexpectedEOF, exercising the
// fallback error path in Trace.
func TestJobTrace_BodyReadError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
			w.Header().Set(testHeaderContentType, "text/plain")
			// Claim gzip so Go's http transport auto-decompresses; the body
			// is not actually gzipped, so Read on the gzip reader errors.
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("definitely not gzipped data"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal("expected error from invalid gzip body, got nil")
	}
	if !strings.Contains(err.Error(), "jobTrace") {
		t.Errorf("error = %v, want jobTrace context", err)
	}
}

// ---------------------------------------------------------------------------
// Cancel: API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobCancel_APIError verifies a 403 on cancel carries the cancel hint, the
// one naming the role and the force override, and not a sibling's.
func TestJobCancel_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Cancel(context.Background(), client, CancelInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "canceling jobs requires Developer+ role")
}

// TestJobCancel_NotFoundAPIError verifies JobCancel when GitLab returns not found.
func TestJobCancel_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Cancel(context.Background(), client, CancelInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, actionJobList)
}

// TestJobCancel_Body_CarriesForceOnlyWhenAsked pins the body of the cancel
// POST with and without force, and that either call returns the cancelled job.
//
// The case without force is the one with a history. client-go's CancelJob
// delegates to CancelJobWithOptions with a nil options pointer, which
// NewRequestToURL marshals as the four bytes "null" (entry 51 of
// docs/development/upstream-bugs.md). Cancel now calls the options variant
// itself with a non-nil value, so a call without force sends the empty
// object, and a body of "null" fails here before the comparison.
func TestJobCancel_Body_CarriesForceOnlyWhenAsked(t *testing.T) {
	tests := []struct {
		name  string
		force bool
		want  map[string]any
	}{
		{name: "without force", force: false, want: map[string]any{}},
		{name: "with force", force: true, want: map[string]any{"force": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			client := cancelBodyRecordingClient(t, &body)

			out, err := Cancel(context.Background(), client, CancelInput{ProjectID: "42", JobID: 100, Force: tt.force})
			if err != nil {
				t.Fatalf("Cancel() unexpected error: %v", err)
			}
			var sent map[string]any
			if err = json.Unmarshal(body, &sent); err != nil {
				t.Fatalf("cancel body %q is not a JSON object: %v", body, err)
			}
			if sent == nil {
				t.Fatalf("cancel body = %q, want a JSON object rather than null", body)
			}
			if !reflect.DeepEqual(sent, tt.want) {
				t.Errorf("cancel body = %#v, want %#v", sent, tt.want)
			}
			if out.ID != 100 || out.Status != "canceled" {
				t.Errorf("Cancel() = job %d with status %q, want job 100 with status canceled", out.ID, out.Status)
			}
		})
	}
}

// cancelBodyRecordingClient returns a client whose mock answers the cancel
// POST with a cancelled job and keeps the request body it was sent in *body.
// Anything else is reported and answered 404. The body is written on the
// httptest server's goroutine and read after Cancel returns, which the
// response it waited for orders.
func cancelBodyRecordingClient(t *testing.T, body *[]byte) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != pathJobCancel {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		*body = read
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":100,"name":"build","stage":"build","status":"canceled",
			"ref":"main","tag":false,"duration":10.0,"queued_duration":1.0,
			"web_url":"https://gitlab.example.com/-/jobs/100",
			"pipeline":{"id":10},"created_at":"2026-03-01T10:00:00Z"
		}`)
	}))
}

// TestJobCancel_CancelledContext verifies JobCancel when cancelled context.
func TestJobCancel_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Cancel(ctx, client, CancelInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Retry: API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobRetry_APIError verifies a 403 on retry carries the retry hint.
func TestJobRetry_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "retrying jobs requires Developer+ role")
}

// TestJobRetry_NotFoundAPIError verifies JobRetry when GitLab returns not found.
func TestJobRetry_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, actionJobList)
}

// TestJobRetry_CancelledContext verifies JobRetry when cancelled context.
func TestJobRetry_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Retry(ctx, client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// ListProject: API error, canceled context, with scope and pagination
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies ListProject when API error.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_CancelledContext verifies ListProject when cancelled context.
func TestListProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListProject_ForwardsEveryListParameter verifies ListProject sends the
// scope, the retried flag, the offset page, the keyset cursor and the ordering
// as the caller set them, and reads the pagination block off the response.
// The test this replaces set the retried flag and the page and read neither
// back, so ListProject could drop include_retried and stay green.
func TestListProject_ForwardsEveryListParameter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			scopes := r.URL.Query()["scope[]"]
			if len(scopes) != 2 || scopes[0] != "running" || scopes[1] != "failed" {
				t.Errorf("expected scope[]=[running,failed], got %v", scopes)
			}
			assertListQuery(t, r, map[string]string{
				"include_retried": "true", "page": "3", "per_page": "10",
				"order_by": "id", "sort": "desc", "pagination": "keyset", "page_token": "tok-project",
			})
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "3", PerPage: "10", Total: "31", TotalPages: "4", NextPage: "4", PrevPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:      "42",
		Scope:          []string{"running", "failed"},
		IncludeRetried: true,
		OrderBy:        "id",
		Sort:           "desc",
		Page:           3, PerPage: 10,
		Pagination: "keyset", PageToken: "tok-project",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	wantPage := toolutil.PaginationOutput{Page: 3, PerPage: 10, TotalItems: 31, TotalPages: 4, NextPage: 4, PrevPage: 2, HasMore: true}
	if out.Pagination != wantPage {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, wantPage)
	}
}

// ---------------------------------------------------------------------------
// ListBridges: API error, canceled context (the routes, the filters and the
// pagination are held by the TestListBridges routing tests above)
// ---------------------------------------------------------------------------.

// TestListBridges_APIError verifies ListBridges when API error.
func TestListBridges_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListBridges_CancelledContext verifies ListBridges when cancelled context.
func TestListBridges_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListBridges(ctx, client, BridgeListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// GetArtifacts: API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetArtifacts_APIError verifies GetArtifacts when API error.
func TestGetArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetArtifacts(context.Background(), client, ArtifactsInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetArtifacts_CancelledContext verifies GetArtifacts when cancelled context.
func TestGetArtifacts_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK"))
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetArtifacts(ctx, client, ArtifactsInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// DownloadArtifacts: API error, canceled context
// ---------------------------------------------------------------------------.

// TestDownloadArtifacts_APIError verifies DownloadArtifacts when API error.
func TestDownloadArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := DownloadArtifacts(context.Background(), client, DownloadArtifactsInput{
		ProjectID: "42", RefName: "main", JobName: "build",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDownloadArtifacts_CancelledContext verifies DownloadArtifacts when cancelled context.
func TestDownloadArtifacts_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadArtifacts(ctx, client, DownloadArtifactsInput{
		ProjectID: "42", RefName: "main", JobName: "build",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// DownloadSingleArtifact: API error, canceled context
// ---------------------------------------------------------------------------.

// TestDownloadSingleArtifact_APIError verifies DownloadSingleArtifact when API error.
func TestDownloadSingleArtifact_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{
		ProjectID: "42", JobID: 100, ArtifactPath: "report.txt",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDownloadSingleArtifact_CancelledContext verifies DownloadSingleArtifact when cancelled context.
func TestDownloadSingleArtifact_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadSingleArtifact(ctx, client, SingleArtifactInput{
		ProjectID: "42", JobID: 100, ArtifactPath: "report.txt",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// DownloadSingleArtifactByRef: API error, canceled context
// ---------------------------------------------------------------------------.

// TestDownloadSingleArtifactByRef_APIError verifies DownloadSingleArtifactByRef when API error.
func TestDownloadSingleArtifactByRef_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: "report.txt", JobName: "build",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDownloadSingleArtifactByRef_CancelledContext verifies DownloadSingleArtifactByRef when cancelled context.
func TestDownloadSingleArtifactByRef_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: "report.txt", JobName: "build",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Erase: API error, canceled context
// ---------------------------------------------------------------------------.

// TestErase_APIError verifies a 403 on erase carries the erase hint.
func TestErase_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "erasing jobs requires Maintainer+ role")
}

// TestErase_NotFoundAPIError verifies Erase when GitLab returns not found.
func TestErase_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, actionJobList)
}

// TestErase_CancelledContext verifies Erase when cancelled context.
func TestErase_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Erase(ctx, client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// KeepArtifacts: API error, canceled context
// ---------------------------------------------------------------------------.

// TestKeepArtifacts_APIError verifies a 403 on keep carries the keep hint.
func TestKeepArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "keeping artifacts requires Maintainer+ role")
}

// TestKeepArtifacts_NotFoundAPIError verifies KeepArtifacts when GitLab returns not found.
func TestKeepArtifacts_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "artifacts")
}

// TestKeepArtifacts_CancelledContext verifies KeepArtifacts when cancelled context.
func TestKeepArtifacts_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := KeepArtifacts(ctx, client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Play: API error, canceled context, with variables
// ---------------------------------------------------------------------------.

// TestPlay_APIError verifies a 403 on play carries the play hint, which is
// about the role and not about the job's state, the 400's subject.
func TestPlay_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "playing manual jobs requires Developer+ role")
}

// TestPlay_BadRequestAPIError verifies Play when the job is not playable.
func TestPlay_BadRequestAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"job is not playable"}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "manual jobs")
}

// TestPlay_NotFoundAPIError verifies Play when GitLab returns not found.
func TestPlay_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, actionJobList)
}

// TestPlay_CancelledContext verifies Play when cancelled context.
func TestPlay_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Play(ctx, client, PlayInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// playBodyRecorder answers the play endpoint and keeps the request body it was
// sent, for the tests that assert what reached GitLab rather than what came
// back.
func playBodyRecorder(t *testing.T) (*gitlabclient.Client, *atomic.Value) {
	t.Helper()
	var gotBody atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			gotBody.Store(string(body))
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))
	return client, &gotBody
}

// TestPlay_WithVariables_ForwardsEachInTheRequestBody verifies the variables a
// caller gives reach GitLab as job_variables_attributes, each with its key and
// value, and with variable_type only where the caller set one: an unset type
// is GitLab's default, not an empty string to send. The test this replaces
// checked the job that came back and never read the body.
func TestPlay_WithVariables_ForwardsEachInTheRequestBody(t *testing.T) {
	client, gotBody := playBodyRecorder(t)

	_, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		JobVariablesAttributes: []JobVariableInput{
			{Key: "ENV", Value: "production", VariableType: "env_var"},
			{Key: "KUBECONFIG", Value: "/tmp/kubeconfig", VariableType: "file"},
			{Key: "PLAIN", Value: "untyped"},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var body struct {
		Vars   []map[string]any `json:"job_variables_attributes"`
		Inputs json.RawMessage  `json:"job_inputs"`
	}
	raw, _ := gotBody.Load().(string)
	if decodeErr := json.Unmarshal([]byte(raw), &body); decodeErr != nil {
		t.Fatalf("request body %q does not decode: %v", raw, decodeErr)
	}
	want := []map[string]any{
		{"key": "ENV", "value": "production", "variable_type": "env_var"},
		{"key": "KUBECONFIG", "value": "/tmp/kubeconfig", "variable_type": "file"},
		{"key": "PLAIN", "value": "untyped"},
	}
	if !reflect.DeepEqual(body.Vars, want) {
		t.Errorf("job_variables_attributes = %v, want %v", body.Vars, want)
	}
	if body.Inputs != nil {
		t.Errorf("job_inputs = %s, want none sent when the caller gave none", body.Inputs)
	}
}

// TestPlay_NoVariablesOrInputs_SendsNeitherKey verifies a plain play carries
// neither job_variables_attributes nor job_inputs: an empty array in the body
// is a request GitLab was never sent before, not the absence of one.
func TestPlay_NoVariablesOrInputs_SendsNeitherKey(t *testing.T) {
	client, gotBody := playBodyRecorder(t)

	if _, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var body map[string]json.RawMessage
	raw, _ := gotBody.Load().(string)
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("request body %q does not decode: %v", raw, err)
	}
	for _, key := range []string{"job_variables_attributes", "job_inputs"} {
		t.Run(key, func(t *testing.T) {
			if v, ok := body[key]; ok {
				t.Errorf("body carries %s = %s, want the key absent", key, v)
			}
		})
	}
}

// TestPlay_WithJobInputs_ForwardsInRequestBody verifies Play forwards
// job_inputs in the request body, converted to the SDK's typed
// pipeline-input values, string arrays included.
func TestPlay_WithJobInputs_ForwardsInRequestBody(t *testing.T) {
	var gotBody atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			gotBody.Store(string(body))
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		JobInputs: map[string]any{"environment": "staging", "replicas": float64(3), "debug": true, "regions": []any{"eu", "us"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	body, _ := gotBody.Load().(string)
	for _, want := range []string{`"job_inputs"`, `"environment":"staging"`, `"replicas":3`, `"debug":true`, `"regions":["eu","us"]`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("request body = %s, want it to contain %s", body, want)
			}
		})
	}
}

// TestPlay_InvalidJobInputs_RejectedBeforeRequest verifies Play rejects an
// unsupported job-input value type before any request is made.
func TestPlay_InvalidJobInputs_RejectedBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))

	_, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		JobInputs: map[string]any{"bad": map[string]any{"nested": true}},
	})
	assertContains(t, err, "job inputs must be string, number, boolean, or array of strings")
}

// ---------------------------------------------------------------------------
// DeleteArtifacts: API error
// ---------------------------------------------------------------------------.

// TestDeleteArtifacts_APIError verifies a 403 on delete carries the delete
// hint.
func TestDeleteArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "deleting artifacts requires Maintainer+ role")
}

// TestDeleteArtifacts_NotFoundAPIError verifies DeleteArtifacts when GitLab returns not found.
func TestDeleteArtifacts_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "no artifacts")
}

// ---------------------------------------------------------------------------
// DeleteProjectArtifacts: API error
// ---------------------------------------------------------------------------.

// TestDeleteProjectArtifacts_APIError verifies a 403 on the bulk delete
// carries its own hint, the one that says the deletion is project-wide.
func TestDeleteProjectArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	assertContains(t, err, "bulk-deleting all project artifacts requires Maintainer+ role")
}

// TestDeleteProjectArtifacts_NotFoundAPIError verifies DeleteProjectArtifacts when GitLab returns not found.
func TestDeleteProjectArtifacts_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))
	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	assertContains(t, err, "project.get")
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_AllFields verifies FormatOutputMarkdown when all fields.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:             100,
		Name:           "build",
		Stage:          "build",
		Status:         "success",
		Pipeline:       &PipelineObject{ID: 10},
		Ref:            "main",
		Commit:         &CommitObject{ID: "abcdef1234567890"},
		AllowFailure:   true,
		Duration:       new(45.5),
		QueuedDuration: new(2.1),
		FailureReason:  "script_failure",
		Coverage:       new(85.5),
		User:           &UserObject{Username: "testuser"},
		CreatedAt:      "2026-03-01T10:00:00Z",
		WebURL:         "https://gitlab.example.com/-/jobs/100",
	})

	want := "## ✅ Job #100: build\n\n" +
		"- **Pipeline**: #10\n" +
		"- **Stage**: build\n" +
		"- **Status**: ✅ success\n" +
		"- **Allow Failure**: ✅\n" +
		"- **Ref**: main\n" +
		"- **Tag**: ❌\n" +
		"- **Commit**: `abcdef123456`\n" +
		"- **Duration**: 45.5s\n" +
		"- **Queued**: 2.1s\n" +
		"- **Failure Reason**: script_failure\n" +
		"- **Coverage**: 85.5%\n" +
		"- **User**: @testuser\n" +
		"- **Created**: 1 Mar 2026 10:00 UTC\n" +
		"- **URL**: [https://gitlab.example.com/-/jobs/100](https://gitlab.example.com/-/jobs/100)\n" +
		jobCardHints
	if md != want {
		t.Errorf("FormatOutputMarkdown(all fields)\n got %q\nwant %q", md, want)
	}
}

// jobCardHints is the guidance section a job card closes with when the job is
// neither erased nor archived.
const jobCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.trace' to read this job's log\n" +
	"- Use action 'job.retry' to re-run this job\n" +
	"- Use action 'job.cancel' to cancel it while it is still running\n"

// TestFormatOutputMarkdown_ErasedAndArchivedRowsAndHints checks the three
// rows a job's own state decides, and that the hints follow them: GitLab keeps
// no log for an erased job and refuses both a retry and a cancel on an
// archived one, so offering those was advice that could only fail. The hints
// are replaced rather than dropped, so the card still closes with three.
func TestFormatOutputMarkdown_ErasedAndArchivedRowsAndHints(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                7,
		Name:              "build",
		Stage:             "build",
		Status:            "success",
		Ref:               "main",
		ErasedAt:          "2026-03-02T08:00:00Z",
		ArtifactsExpireAt: "2026-04-01T00:00:00Z",
		Archived:          true,
	})
	want := "## ✅ Job #7: build 📦\n\n" +
		"- **Stage**: build\n" +
		"- **Status**: ✅ success\n" +
		"- **Allow Failure**: ❌\n" +
		"- **Ref**: main\n" +
		"- **Tag**: ❌\n" +
		"- **Erased**: 2 Mar 2026 08:00 UTC\n" +
		"- **Artifacts Expire**: 1 Apr 2026 00:00 UTC\n" +
		"- 📦 **Archived**\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.list_project' to look at the project's other jobs, since this one's log was erased\n" +
		"- Use action 'pipeline.get' to open the pipeline this job ran in\n" +
		"- Use action 'job.list' to see the other jobs of that pipeline\n"
	if md != want {
		t.Errorf("FormatOutputMarkdown(erased and archived)\n got %q\nwant %q", md, want)
	}
}

// TestFormatOutputMarkdown_HintsFollowEachStateOnItsOwn drives the erased and
// archived states one at a time, since the cards above set neither or both: an
// erased job that is not archived keeps its retry and cancel, and an archived
// one whose log remains keeps the log.
func TestFormatOutputMarkdown_HintsFollowEachStateOnItsOwn(t *testing.T) {
	tests := []struct {
		name string
		job  Output
		want string
	}{
		{"erased only", Output{ID: 1, ErasedAt: "2026-03-02T08:00:00Z"}, "\n---\n💡 **Next steps:**\n" +
			"- Use action 'job.list_project' to look at the project's other jobs, since this one's log was erased\n" +
			"- Use action 'job.retry' to re-run this job\n" +
			"- Use action 'job.cancel' to cancel it while it is still running\n"},
		{"archived only", Output{ID: 1, Archived: true}, "\n---\n💡 **Next steps:**\n" +
			"- Use action 'job.trace' to read this job's log\n" +
			"- Use action 'pipeline.get' to open the pipeline this job ran in\n" +
			"- Use action 'job.list' to see the other jobs of that pipeline\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if md := FormatOutputMarkdown(tt.job); !strings.HasSuffix(md, tt.want) {
				t.Errorf("FormatOutputMarkdown(%s) hints\n got %q\nwant suffix %q", tt.name, md, tt.want)
			}
		})
	}
}

// TestFormatOutputMarkdown_PipelineWithoutID_OmitsTheRow checks that a
// pipeline object GitLab sent without an id, which the converter keeps for its
// other keys, is not shown as "#0": a reader would take that for a pipeline.
func TestFormatOutputMarkdown_PipelineWithoutID_OmitsTheRow(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 3, Name: "n", Stage: "s", Status: "success", Ref: "r", Pipeline: &PipelineObject{Ref: "main"}})
	if strings.Contains(md, "**Pipeline**") {
		t.Errorf("FormatOutputMarkdown(pipeline without id) shows a pipeline row:\n%s", md)
	}
}

// TestFormatWaitResult_TimedOutMarksError verifies timeout wait results are marked as tool errors.
func TestFormatWaitResult_TimedOutMarksError(t *testing.T) {
	result := formatWaitResult(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "running", Stage: "build", Ref: "main"},
		FinalStatus: "running",
		TimedOut:    true,
		WaitedFor:   "1s",
		PollCount:   1,
	})
	if result == nil {
		t.Fatal("formatWaitResult() returned nil")
	}
	if !result.IsError {
		t.Fatal("formatWaitResult() IsError = false, want true")
	}
}

// TestFormatWaitResult_FinishedIsNoError verifies a wait that ended before
// its timeout is an ordinary result, whatever status the job finished in:
// only running out of time is the tool's failure.
func TestFormatWaitResult_FinishedIsNoError(t *testing.T) {
	result := formatWaitResult(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "failed", Stage: "build", Ref: "main"},
		FinalStatus: "failed",
		WaitedFor:   "1s",
		PollCount:   1,
	})
	if result == nil || result.IsError {
		t.Fatalf("formatWaitResult() = %+v, want a result that is not an error", result)
	}
}

// TestFormatOutputMarkdown_CommitWithoutID_OmitsTheRow verifies a commit
// object carrying no SHA writes no Commit row, rather than an empty code span.
func TestFormatOutputMarkdown_CommitWithoutID_OmitsTheRow(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 3, Name: "n", Stage: "s", Status: "success", Ref: "r", Commit: &CommitObject{Title: "Add the child pipeline"}})
	if strings.Contains(md, "**Commit**") {
		t.Errorf("FormatOutputMarkdown(commit without id) shows a commit row:\n%s", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields checks that a job GitLab said little
// about renders only the rows it answered: no label stands without a value.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:     50,
		Name:   "test",
		Stage:  "test",
		Status: "running",
		Ref:    "develop",
	})
	want := "## 🔵 Job #50: test\n\n" +
		"- **Stage**: test\n" +
		"- **Status**: 🔵 running\n" +
		"- **Allow Failure**: ❌\n" +
		"- **Ref**: develop\n" +
		"- **Tag**: ❌\n" +
		jobCardHints
	if md != want {
		t.Errorf("FormatOutputMarkdown(minimal)\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithJobs verifies FormatListMarkdown when with jobs.
func TestFormatListMarkdown_WithJobs(t *testing.T) {
	out := ListOutput{
		Jobs: []Output{
			// A name that is not its stage in both rows: with the two equal,
			// the Name and Stage columns could trade places and the table
			// still read correctly.
			{ID: 100, Name: "compile", Stage: "build", Status: "success", Duration: new(45.5)},
			{ID: 101, Name: "unit", Stage: "test", Status: "failed", Duration: new(12.3)},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	// These jobs carry no web URL, and MdTitleLink renders a bare label rather
	// than the empty link the hand-written "[%d](%s)" produced.
	want := "## Jobs (2)\n\n" +
		"| ID | Name | Stage | Status | Duration |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| #100 | compile | build | ✅ success | 45.5s |\n" +
		"| #101 | unit | test | ❌ failed | 12.3s |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		jobListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// jobListHints is the guidance section a job list closes with.
const jobListHints = "\n---\n💡 **Next steps:**\n" +
	"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
	"- Use action 'job.get' to see one job in full\n" +
	"- Use action 'job.trace' to read a job's log\n"

// TestFormatListMarkdown_Empty checks that an empty page is the one sentence
// and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	const want = "No jobs found.\n"
	if got := FormatListMarkdown(ListOutput{}); got != want {
		t.Errorf("FormatListMarkdown(empty)\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdown_KeysetPageLinksAndMarksArchived checks the two things
// a keyset page decides: the heading counts the rows shown, since GitLab sends
// no total, and an archived job is marked in the row, without which a reader
// cannot tell one GitLab will not retry from a live one.
func TestFormatListMarkdown_KeysetPageLinksAndMarksArchived(t *testing.T) {
	out := ListOutput{
		Jobs: []Output{
			{
				ID: 200, Name: "ship", Stage: "deploy", Status: "success", Duration: new(10.0),
				Archived: true,
				WebURL:   "https://gitlab.example.com/-/jobs/200",
			},
		},
		Pagination: toolutil.PaginationOutput{NextPage: 2},
	}
	want := "## Jobs (1 shown, more available)\n\n" +
		"| ID | Name | Stage | Status | Duration |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| [#200](https://gitlab.example.com/-/jobs/200) | ship 📦 | deploy | ✅ success | 10.0s |\n" +
		jobListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("FormatListMarkdown(keyset)\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatTraceMarkdown
// ---------------------------------------------------------------------------.

// traceHints is the guidance section a job trace closes with.
const traceHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.get' to see this job's details\n" +
	"- Use action 'job.retry' to re-run it\n"

// TestFormatTraceMarkdown_WithData checks the whole trace rendering: the
// heading, the log in a fence sized to its own content, and no truncation
// note when nothing was cut.
func TestFormatTraceMarkdown_WithData(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{
		JobID: 100,
		Trace: "Running with gitlab-runner 15.0.0\nJob succeeded",
	})
	want := "## Job #100 Trace\n\n" +
		"```\nRunning with gitlab-runner 15.0.0\nJob succeeded\n```\n" +
		traceHints
	if md != want {
		t.Errorf("FormatTraceMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatTraceMarkdown_Truncated checks that the note says the end of the
// log is what is shown and how much of its beginning is not, and that the
// next steps offer tail_lines. The note used to say the first 100 KB were
// shown and the end, where the failure is, was not, which is what the read
// did and no longer does.
func TestFormatTraceMarkdown_Truncated(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{
		JobID:        100,
		Trace:        "...end of log",
		Truncated:    true,
		OmittedBytes: 58550,
	})
	want := "## Job #100 Trace\n\n" +
		"⚠️ Showing the end of the log, at most its last 100 KB. The first 58550 bytes are left out.\n" +
		"\n```\n...end of log\n```\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.trace' to read only the last lines of the log with tail_lines\n" +
		"- Use action 'job.get' to see this job's details\n" +
		"- Use action 'job.retry' to re-run it\n"
	if md != want {
		t.Errorf("FormatTraceMarkdown(truncated)\n got %q\nwant %q", md, want)
	}
}

// TestLogTail_InvalidUTF8_KeepsTheLastByte verifies that a log whose last
// bytes are not UTF-8 still keeps at least its last byte, rather than walking
// past the end looking for a character to start at.
func TestLogTail_InvalidUTF8_KeepsTheLastByte(t *testing.T) {
	if got := logTail(strings.Repeat("\x80", 10), 4); got != "\x80" {
		t.Errorf("logTail(continuation bytes) = %q, want the last byte", got)
	}
	if got := logTail("ab\ncd\x80\x80", 4); got != "cd\x80\x80" {
		t.Errorf("logTail(line then invalid bytes) = %q, want the last line", got)
	}
}

// TestLogTail_AtTheEdges verifies logTail where its comparisons turn: a log
// exactly the limit long comes back whole, its first line included; a line
// break on the first of the last limit bytes is dropped, so the kept part
// starts at the line after it rather than with an empty line; and a line that
// begins exactly where the last limit bytes do is kept whole, since it fits.
// That last one used to be dropped too, the search for a line start running
// on past a line that already began there.
func TestLogTail_AtTheEdges(t *testing.T) {
	cases := []struct {
		name, log string
		limit     int
		want      string
	}{
		{name: "exactly the limit", log: "a\nb", limit: 3, want: "a\nb"},
		{name: "line break on the first kept byte", log: "abc\ndef", limit: 4, want: "def"},
		{name: "line break just before the kept bytes", log: "abc\ndef\nghi", limit: 7, want: "def\nghi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := logTail(tc.log, tc.limit); got != tc.want {
				t.Errorf("logTail(%q, %d) = %q, want %q", tc.log, tc.limit, got, tc.want)
			}
		})
	}
}

// traceTailOf runs traceTail over log with the window given, as Trace runs it
// over the body client-go read.
func traceTailOf(log string, tailLines, window int) (string, int) {
	return traceTail(bytes.NewReader([]byte(log)), tailLines, window)
}

// assertTailOfLog holds what traceTail returned for log to what it promises:
// the log from omitted on, as GitLab stores it, strips to exactly the text
// returned, the log up to omitted strips to the text before it, so the two
// make the text of the whole log, and the text returned starts a line of that
// text whenever the text has a line break before it.
func assertTailOfLog(t *testing.T, log, kept string, omitted int) {
	t.Helper()
	if omitted < 0 || omitted > len(log) {
		t.Fatalf("omitted = %d, outside a log of %d bytes", omitted, len(log))
	}
	if got := toolutil.StripLogMarkup(log[omitted:]); got != kept {
		t.Errorf("the log from byte %d strips to %d bytes, not the %d returned", omitted, len(got), len(kept))
	}
	before := toolutil.StripLogMarkup(log[:omitted])
	if text := toolutil.StripLogMarkup(log); before+kept != text {
		t.Errorf("the log up to byte %d strips to %d bytes, which with the %d returned do not make the %d of the whole log", omitted, len(before), len(kept), len(text))
	}
	if strings.Contains(before, "\n") && !strings.HasSuffix(before, "\n") {
		t.Errorf("the text returned starts inside a line, at byte %d", omitted)
	}
}

// TestTraceTail_EndOfALongLog_IsWhatTheWholeLogGives verifies that reading
// only the end of a log returns what stripping and cutting the whole log
// returns, with and without tail_lines, when the end read holds more text
// than one response carries: the part read starts at a line, where the
// markup strips as it does in the whole log. A log the runner timestamped
// gives the text of the same log written without timestamps, its headers
// gone and the lines it wrote in pieces whole.
func TestTraceTail_EndOfALongLog_IsWhatTheWholeLogGives(t *testing.T) {
	plain := markupLog(600 << 10)
	text := toolutil.StripLogMarkup(plain)
	for _, written := range []struct{ name, log string }{
		{"untimestamped", plain},
		{"timestamped", timestamped(plain)},
	} {
		for _, tc := range []struct {
			name      string
			tailLines int
			window    int
		}{
			{"the end of the log", 0, 300 << 10},
			{"the last five lines", 5, 300 << 10},
			{"more lines than the part read holds", 10000, 300 << 10},
			{"a window over the whole log", 0, 1 << 20},
		} {
			t.Run(written.name+"/"+tc.name, func(t *testing.T) {
				want := text
				if tc.tailLines > 0 {
					want = lastLines(want, tc.tailLines)
				}
				want = logTail(want, maxTraceBytes)
				kept, omitted := traceTailOf(written.log, tc.tailLines, tc.window)
				if kept != want {
					t.Errorf("kept %d bytes starting %q, want the %d the whole log gives", len(kept), firstTraceLine(kept), len(want))
				}
				assertTailOfLog(t, written.log, kept, omitted)
			})
		}
	}
}

// TestTraceTail_TheEdgesOfThePartRead verifies the part read where it has no
// line to start at: a long line cut on a character rather than inside one, a
// line break only as the last byte, which leaves the part read whole rather
// than empty, a part holding no character start at all, which keeps the last
// byte, and a part that is nearly all markup, returned whole with what comes
// before it counted as left out. A part that starts inside a line the runner
// wrote in pieces starts at the line after it, past the pieces that continue
// it, which used to come back as a line of their own.
func TestTraceTail_TheEdgesOfThePartRead(t *testing.T) {
	t.Run("a part starting inside a line written in pieces", func(t *testing.T) {
		log := traceStdoutHeader + "first piece\r\n" + traceStdoutContinued + "second piece\r\n" + traceStdoutContinued + "third piece\n" + traceStdoutHeader + "last\n"
		kept, omitted := traceTailOf(log, 0, len(log)-5)
		if kept != "last\n" || omitted != strings.LastIndex(log, traceStdoutHeader) {
			t.Errorf("kept %q, %d omitted; want the last line and everything before its header omitted", kept, omitted)
		}
		assertTailOfLog(t, log, kept, omitted)
	})
	t.Run("one long line", func(t *testing.T) {
		log := strings.Repeat("é", 200<<10) + "x" // an odd length, so the part read starts on the second byte of a character
		kept, omitted := traceTailOf(log, 0, 150<<10)
		if !utf8.ValidString(kept) || !strings.HasSuffix(kept, "éx") || len(kept) > maxTraceBytes {
			t.Errorf("kept %d bytes, valid UTF-8 %v; want at most %d valid bytes ending in éx", len(kept), utf8.ValidString(kept), maxTraceBytes)
		}
		assertTailOfLog(t, log, kept, omitted)
	})
	t.Run("a short part starting inside a character", func(t *testing.T) {
		log := strings.Repeat("é", 10) // the last 5 bytes start on the second byte of a character
		kept, omitted := traceTailOf(log, 0, 5)
		if kept != "éé" || omitted != 16 {
			t.Errorf("kept %q, %d omitted; want the two whole characters read and the 16 bytes before them omitted", kept, omitted)
		}
		assertTailOfLog(t, log, kept, omitted)
	})
	t.Run("a line break only as the last byte", func(t *testing.T) {
		log := strings.Repeat("a", 300<<10) + "\n"
		kept, omitted := traceTailOf(log, 0, 200<<10)
		if len(kept) != maxTraceBytes || omitted != len(log)-maxTraceBytes {
			t.Errorf("kept %d bytes, %d omitted; want the last %d and the rest omitted", len(kept), omitted, maxTraceBytes)
		}
	})
	t.Run("no character start", func(t *testing.T) {
		log := strings.Repeat("\x80", 300<<10)
		kept, omitted := traceTailOf(log, 0, 200<<10)
		if kept != "\x80" || omitted != len(log)-1 {
			t.Errorf("kept %q, %d omitted; want the last byte and the rest omitted", kept, omitted)
		}
	})
	t.Run("a part that is nearly all markup", func(t *testing.T) {
		line := strings.Repeat("\x1b[0K", 1000) + "x\n"
		log := strings.Repeat(line, 100)
		kept, omitted := traceTailOf(log, 0, 200<<10)
		if kept == "" || strings.Trim(kept, "x\n") != "" || len(kept) >= 100*2 || omitted == 0 {
			t.Errorf("kept %q, %d omitted; want the text of the lines read and the rest omitted", kept, omitted)
		}
		assertTailOfLog(t, log, kept, omitted)
	})
}

// TestLastLines_ALeadingLineBreak_StaysOutOfTheLastLines verifies that a log
// starting with a line break has an empty first line, so asking for every
// line after it returns them without the break rather than the log whole.
func TestLastLines_ALeadingLineBreak_StaysOutOfTheLastLines(t *testing.T) {
	if got := lastLines("\na\nb\n", 2); got != "a\nb\n" {
		t.Errorf("lastLines(leading line break, 2) = %q, want %q", got, "a\nb\n")
	}
}

// TestFormatTraceMarkdown_Empty checks that a job with no log yet renders no
// fence at all: an empty fence is a glyph that reads as content.
func TestFormatTraceMarkdown_Empty(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{JobID: 99})
	want := "## Job #99 Trace\n" + traceHints
	if md != want {
		t.Errorf("FormatTraceMarkdown(empty)\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatBridgeListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatBridgeListMarkdown_WithData verifies FormatBridgeListMarkdown when with data.
func TestFormatBridgeListMarkdown_WithData(t *testing.T) {
	out := BridgeListOutput{
		Bridges: []BridgeOutput{
			{ID: 200, Name: "trigger-downstream", Stage: "deploy", Status: "success", Duration: new(10.0), DownstreamPipeline: &PipelineInfoObject{ID: 50}},
			{ID: 201, Name: "trigger-other", Stage: "deploy", Status: "failed", Duration: new(5.0)},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	want := "## Bridge Jobs (2)\n\n" +
		"| ID | Name | Stage | Status | Duration | Downstream |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| #200 | trigger-downstream | deploy | ✅ success | 10.0s | #50 |\n" +
		"| #201 | trigger-other | deploy | ❌ failed | 5.0s |  |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'pipeline.get' to open the downstream pipeline\n"
	if got := FormatBridgeListMarkdown(out); got != want {
		t.Errorf("FormatBridgeListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatBridgeListMarkdown_LinksTheDownstreamPipeline checks that the
// downstream cell is a link when GitLab sent the pipeline's address: the
// bridge exists to point at that pipeline, and the cell named it without
// reaching it.
func TestFormatBridgeListMarkdown_LinksTheDownstreamPipeline(t *testing.T) {
	out := BridgeListOutput{
		Bridges: []BridgeOutput{{
			ID: 300, Name: "trigger", Stage: "deploy", Status: "success", Duration: new(1.0),
			WebURL: "https://gitlab.example.com/-/jobs/300",
			DownstreamPipeline: &PipelineInfoObject{
				ID: 77, WebURL: "https://gitlab.example.com/downstream/-/pipelines/77",
			},
		}},
	}
	md := FormatBridgeListMarkdown(out)
	const wantRow = "| [#300](https://gitlab.example.com/-/jobs/300) | trigger | deploy | ✅ success | 1.0s | " +
		"[#77](https://gitlab.example.com/downstream/-/pipelines/77) |\n"
	if !strings.Contains(md, wantRow) {
		t.Errorf("FormatBridgeListMarkdown() missing %q:\n%s", wantRow, md)
	}
}

// TestFormatBridgeListMarkdown_DownstreamWithoutID_LeavesTheCellEmpty checks
// that a downstream pipeline object with no ID links nothing: a link to #0
// would name a pipeline that does not exist.
func TestFormatBridgeListMarkdown_DownstreamWithoutID_LeavesTheCellEmpty(t *testing.T) {
	const downstreamURL = "https://gitlab.example.com/downstream/-/pipelines/77"
	md := FormatBridgeListMarkdown(BridgeListOutput{
		Bridges: []BridgeOutput{{
			ID: 300, Name: "trigger", Stage: "deploy", Status: "created",
			WebURL:             "https://gitlab.example.com/-/jobs/300",
			DownstreamPipeline: &PipelineInfoObject{WebURL: downstreamURL},
		}},
	})
	if strings.Contains(md, "#0") || strings.Contains(md, downstreamURL) {
		t.Errorf("FormatBridgeListMarkdown(downstream without id) links it:\n%s", md)
	}
	if !strings.Contains(md, "[#300](https://gitlab.example.com/-/jobs/300)") {
		t.Errorf("FormatBridgeListMarkdown() lost the bridge row:\n%s", md)
	}
}

// TestFormatBridgeListMarkdown_Empty checks that an empty page is the one
// sentence and nothing else.
func TestFormatBridgeListMarkdown_Empty(t *testing.T) {
	const want = "No bridge jobs found.\n"
	if got := FormatBridgeListMarkdown(BridgeListOutput{}); got != want {
		t.Errorf("FormatBridgeListMarkdown(empty)\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatArtifactsMarkdown
// ---------------------------------------------------------------------------.

// TestFormatArtifactsMarkdown_WithJobID verifies FormatArtifactsMarkdown when with job ID.
func TestFormatArtifactsMarkdown_WithJobID(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{
		JobID: 100,
		Size:  2048,
	})
	want := "## Job #100 Artifacts\n\n" +
		"- **Size (bytes)**: 2048\n" +
		"\nThe content is base64-encoded as GitLab stores it: the zip archive, or the report file_type named, which may be gzip-compressed. Decode it before reading.\n" +
		artifactsHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// artifactsHints is the guidance section an artifacts download by job ID
// closes with.
const artifactsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.download_single_artifact' to fetch one file out of the archive instead\n"

// artifactsByRefHints is the guidance section the by-ref download closes
// with: the one-file read that takes the same ref and job name, since its
// caller has no job ID to give the read by job ID.
const artifactsByRefHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.download_single_artifact_by_ref' to fetch one file out of the archive instead, for the same ref and job\n"

// TestFormatArtifactsMarkdown_WithoutJobID checks the card of the by-ref
// download, which answers for a ref rather than for a job ID and must not
// print "Job #0", whose note names no file_type: that route declares none,
// so the content is always the zip archive, and whose next step is the
// one-file read by ref.
func TestFormatArtifactsMarkdown_WithoutJobID(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{Size: 512})
	want := "## Artifacts\n\n" +
		"- **Size (bytes)**: 512\n" +
		"\nThe zip archive is base64-encoded. Decode it to extract the files.\n" +
		artifactsByRefHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown(no job)\n got %q\nwant %q", md, want)
	}
}

// TestFormatArtifactsMarkdown_ByRefOverTheLimit_NamesTheReadByRef checks the
// card of a by-ref archive withheld for its size: the note sends the caller
// to the one-file read by ref, for the same ref and job, rather than to the
// read by job ID, which wants a job ID this caller does not have.
func TestFormatArtifactsMarkdown_ByRefOverTheLimit_NamesTheReadByRef(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{Size: 3000288, Truncated: true})
	want := "## Artifacts\n\n" +
		"- **Size (bytes)**: 3000288\n" +
		"- ⚠️ **Over the 1 MB one response carries, so no content is returned**\n" +
		"\nA prefix of an archive cannot be opened, so none is returned. Read one file of it with job.download_single_artifact_by_ref, for the same ref and job: its path is one of the job's artifacts:paths in .gitlab-ci.yml.\n" +
		artifactsByRefHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown(by ref, over the limit)\n got %q\nwant %q", md, want)
	}
}

// TestFormatArtifactsMarkdown_OverTheLimit_SaysWhereToReadInstead checks
// the card of an archive withheld for its size: a warning, not a tick, that
// no content came back, and a note naming the one-file read and where its
// path comes from, instead of telling a model to decode content that is not
// there.
func TestFormatArtifactsMarkdown_OverTheLimit_SaysWhereToReadInstead(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{
		JobID:     100,
		Size:      3000288,
		Truncated: true,
	})
	want := "## Job #100 Artifacts\n\n" +
		"- **Size (bytes)**: 3000288\n" +
		"- ⚠️ **Over the 1 MB one response carries, so no content is returned**\n" +
		"\nA prefix of an archive or a compressed report cannot be opened, so none is returned. Read one file of it with job.download_single_artifact: its path is one of the job's artifacts:paths in .gitlab-ci.yml.\n" +
		artifactsHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown(over the limit)\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatSingleArtifactMarkdown
// ---------------------------------------------------------------------------.

// TestFormatSingleArtifactMarkdown_WithJobID verifies FormatSingleArtifactMarkdown when with job ID.
func TestFormatSingleArtifactMarkdown_WithJobID(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		JobID:        100,
		ArtifactPath: "report.txt",
		Size:         256,
		Encoding:     "text",
		Content:      "test report content",
	})
	want := "## Job #100: report.txt\n\n" +
		"- **Size (bytes)**: 256\n" +
		"- **Encoding**: text\n" +
		"\n```\ntest report content\n```\n" +
		singleArtifactHints
	if md != want {
		t.Errorf("FormatSingleArtifactMarkdown()\n got %q\nwant %q", md, want)
	}
}

// singleArtifactHints is the guidance section one artifact file closes with.
const singleArtifactHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.artifacts' to download the whole artifacts archive\n"

// TestFormatSingleArtifactMarkdown_WithoutJobID checks that the by-ref
// download heads with the path alone rather than "Job #0".
func TestFormatSingleArtifactMarkdown_WithoutJobID(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		ArtifactPath: "output.log",
		Size:         64,
		Encoding:     "text",
		Content:      "log data",
	})
	want := "## output.log\n\n" +
		"- **Size (bytes)**: 64\n" +
		"- **Encoding**: text\n" +
		"\n```\nlog data\n```\n" +
		singleArtifactHints
	if md != want {
		t.Errorf("FormatSingleArtifactMarkdown(no job)\n got %q\nwant %q", md, want)
	}
}

// TestFormatSingleArtifactMarkdown_Truncated checks that a cut text file is
// marked with the warning sign and shows the prefix it kept.
func TestFormatSingleArtifactMarkdown_Truncated(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		JobID:        100,
		ArtifactPath: "big.log",
		Size:         2097152,
		Encoding:     "text",
		Content:      "...",
		Truncated:    true,
	})
	want := "## Job #100: big.log\n\n" +
		"- **Size (bytes)**: 2097152\n" +
		"- **Encoding**: text\n" +
		"- ⚠️ **Truncated at 1 MB**\n" +
		"\n```\n...\n```\n" +
		singleArtifactHints
	if md != want {
		t.Errorf("FormatSingleArtifactMarkdown(truncated)\n got %q\nwant %q", md, want)
	}
}

// TestFormatSingleArtifactMarkdown_Binary checks the two binary cards: one
// that fits says its content is base64 and fences it, and one over the limit
// says no content came back and where to get the file instead.
func TestFormatSingleArtifactMarkdown_Binary(t *testing.T) {
	fits := FormatSingleArtifactMarkdown(SingleArtifactOutput{JobID: 100, ArtifactPath: "logo.png", Size: 12, Encoding: "base64", Content: "iVBORw0KGgr//gCA"})
	wantFits := "## Job #100: logo.png\n\n" +
		"- **Size (bytes)**: 12\n" +
		"- **Encoding**: base64\n" +
		"\nThe file is not text, so its bytes are base64-encoded. Decode them before use.\n" +
		"\n```\niVBORw0KGgr//gCA\n```\n" +
		singleArtifactHints
	if fits != wantFits {
		t.Errorf("FormatSingleArtifactMarkdown(binary)\n got %q\nwant %q", fits, wantFits)
	}
	over := FormatSingleArtifactMarkdown(SingleArtifactOutput{JobID: 100, ArtifactPath: "big.bin", Size: 2097152, Truncated: true})
	wantOver := "## Job #100: big.bin\n\n" +
		"- **Size (bytes)**: 2097152\n" +
		"- ⚠️ **Over the 1 MB one response carries, so no content is returned**\n" +
		"\nA prefix of a binary file cannot be used, so none is returned. Download the file from GitLab directly.\n" +
		singleArtifactHints
	if over != wantOver {
		t.Errorf("FormatSingleArtifactMarkdown(binary over the limit)\n got %q\nwant %q", over, wantOver)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for job actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := jobSpecsByTool(t, specs)

	if len(specs) != 17 {
		t.Fatalf("len(ActionSpecs) = %d, want 17", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "jobs" {
			t.Fatalf("OwnerPackage for %s = %q, want jobs", spec.Name, spec.OwnerPackage)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpecsCallAllRoutes: route coverage for all 17 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates job routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newJobsRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_job_list", map[string]any{"project_id": "42", "pipeline_id": 10}},
		{"get", "gitlab_job_get", map[string]any{"project_id": "42", "job_id": 100}},
		{"trace", "gitlab_job_trace", map[string]any{"project_id": "42", "job_id": 100}},
		{"cancel", "gitlab_job_cancel", map[string]any{"project_id": "42", "job_id": 100}},
		{"retry", "gitlab_job_retry", map[string]any{"project_id": "42", "job_id": 100}},
		{"list_project", "gitlab_job_list_project", map[string]any{"project_id": "42"}},
		{"list_bridges", "gitlab_job_list_bridges", map[string]any{"project_id": "42", "pipeline_id": 10}},
		{"artifacts", "gitlab_job_artifacts", map[string]any{"project_id": "42", "job_id": 100}},
		{"download_artifacts", "gitlab_job_download_artifacts", map[string]any{"project_id": "42", "ref_name": "main", "job": "build"}},
		{"download_single_artifact", "gitlab_job_download_single_artifact", map[string]any{"project_id": "42", "job_id": 100, "artifact_path": "report.txt"}},
		{"download_single_artifact_by_ref", "gitlab_job_download_single_artifact_by_ref", map[string]any{"project_id": "42", "ref_name": "main", "artifact_path": "report.txt", "job": "build"}},
		{"erase", "gitlab_job_erase", map[string]any{"project_id": "42", "job_id": 100}},
		{"keep_artifacts", "gitlab_job_keep_artifacts", map[string]any{"project_id": "42", "job_id": 100}},
		{"play", "gitlab_job_play", map[string]any{"project_id": "42", "job_id": 100}},
		{"delete_artifacts", "gitlab_job_delete_artifacts", map[string]any{"project_id": "42", "job_id": 100}},
		{"delete_project_artifacts", "gitlab_job_delete_project_artifacts", map[string]any{"project_id": "42"}},
		{"wait", "gitlab_job_wait", map[string]any{"project_id": "42", "job_id": 100, "timeout_seconds": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: route spec factory
// ---------------------------------------------------------------------------.

// newJobsRouteSpecs constructs jobs route specs test fixtures.
func newJobsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	// List pipeline jobs
	handler.HandleFunc("GET /api/v4/projects/42/pipelines/10/jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON))
	})

	// Get job
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	})

	// Trace
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100/trace", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("job log output"))
	})

	// Cancel
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/cancel", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":100,"name":"build","stage":"build","status":"canceled",
			"ref":"main","tag":false,"duration":10.0,"queued_duration":1.0,
			"web_url":"https://gitlab.example.com/-/jobs/100",
			"pipeline":{"id":10},"created_at":"2026-03-01T10:00:00Z"
		}`)
	})

	// Retry
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/retry", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":101,"name":"build","stage":"build","status":"pending",
			"ref":"main","tag":false,"duration":0,"queued_duration":0,
			"web_url":"https://gitlab.example.com/-/jobs/101",
			"pipeline":{"id":10},"created_at":"2026-03-01T10:01:00Z"
		}`)
	})

	// List project jobs
	handler.HandleFunc("GET /api/v4/projects/42/jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON))
	})

	// List bridges
	handler.HandleFunc("GET /api/v4/projects/42/pipelines/10/bridges", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeJSON))
	})

	// Get artifacts
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04fake-zip"))
	})

	// Download artifacts by ref
	handler.HandleFunc("GET /api/v4/projects/42/jobs/artifacts/main/download", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04fake-zip"))
	})

	// Download single artifact by job ID
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100/artifacts/report.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("report content"))
	})

	// Download single artifact by ref
	handler.HandleFunc("GET /api/v4/projects/42/jobs/artifacts/main/raw/report.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ref report content"))
	})

	// Erase
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/erase", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, jobJSON)
	})

	// Keep artifacts
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/artifacts/keep", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	})

	// Play
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/play", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	})

	// Delete artifacts
	handler.HandleFunc("DELETE /api/v4/projects/42/jobs/100/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete project artifacts
	handler.HandleFunc("DELETE /api/v4/projects/42/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	client := testutil.NewTestClient(t, handler)
	return jobSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_JobGetRoute verifies the canonical job get route output.
func TestActionSpecs_JobGetRoute(t *testing.T) {
	const respJSON = `{"id":555,"name":"build","stage":"build","status":"success","ref":"main","tag":false}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/jobs/555") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := jobSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_job_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "job_id": 555})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 555 || out.Name != "build" {
		t.Fatalf("job output = %#v, want ID 555 name build", out)
	}
}

// TestListBridges_UnreadableCapturedProject verifies that the bridge list
// handler returns an error rather than a half-filled bridge when GitLab sends
// the project object as something that is not an object. The SDK ignores the
// key its own Bridge does not model, so the read of the captured response is
// the only thing that can notice.
func TestListBridges_UnreadableCapturedProject(t *testing.T) {
	cases := make([]testutil.CapturedCase, 0, len(bridgeRoutes))
	for _, route := range bridgeRoutes {
		client := testutil.NewTestClient(t, atVersion(route.version, `[{"id":1,"name":"trigger","status":"success","project":"not-an-object"}]`))
		cases = append(cases, testutil.CapturedCase{Name: route.name, Call: func() error {
			_, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// jobSpecsByTool supports job specs by tool assertions in jobs tests.
func jobSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestListProject_KeysetPage_FollowsTheNextPageToken verifies that a
// project's job list, which GitLab pages by cursor under keyset pagination,
// can be followed past the first page through the next_page_token it
// publishes (issue 1165).
func TestListProject_KeysetPage_FollowsTheNextPageToken(t *testing.T) {
	listing := &testutil.KeysetListing{
		Path: "/api/v4/projects/42/jobs", Param: "cursor", Value: "eyJpZCI6IjIifQ",
		First:  `[{"id":1,"name":"a"},{"id":2,"name":"b"}]`,
		Second: `[{"id":3,"name":"c"}]`,
	}
	client := testutil.NewTestClient(t, listing)
	testutil.FollowKeyset(t, listing, func(token string) (testutil.KeysetPage, error) {
		out, err := ListProject(t.Context(), client, ListProjectInput{
			ProjectID:  "42",
			Pagination: "keyset",
			PageToken:  token,
		})
		return testutil.KeysetPage{Token: out.Pagination.NextPageToken, HasMore: out.Pagination.HasMore, Rows: len(out.Jobs)}, err
	})
}
