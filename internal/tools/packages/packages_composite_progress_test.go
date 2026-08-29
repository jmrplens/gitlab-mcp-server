package packages

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestPublishDirectory_ProgressSequenceOnlyIncreases pins the progress
// invariant across the whole tool call.
//
// "The progress value MUST increase with each notification, even if the total
// is unknown."
//
// PublishDirectory used to build one Tracker to count files and call Publish,
// which built a second one from the same request to count bytes. Each Tracker
// carried its own monotonic state, so the guard inside Update compared a
// Tracker only against itself and neither could see the other. On one progress
// token a client received a sequence that ran 200000 -> 1 -> 0, with `total`
// oscillating between the file count and a file's byte count.
//
// No existing test could see it. The publish_directory tests pass a nil
// request, so no Tracker is ever active; the monotonicity test in
// internal/progress builds a single Tracker, which is exactly the case that
// worked; and the e2e progress case asserts only that some frame arrives.
// Watching the real notification stream of one call is what it takes.
func TestPublishDirectory_ProgressSequenceOnlyIncreases(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			// Drained so the ProgressReader reaches EOF and reports.
			buf := make([]byte, 32*1024)
			for {
				if _, err := r.Body.Read(buf); err != nil {
					break
				}
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"file_name":"f","size":1,"file_sha256":"abc"}`)
			return
		}
		http.NotFound(w, r)
	}))

	// Two files of different sizes, each well past the reader's report
	// interval, so both levels of the old design would have emitted.
	dir := t.TempDir()
	files := map[string]int{"a.bin": 200 * 1024, "b.bin": 320 * 1024}
	for name, size := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	var wantTotal float64
	for _, size := range files {
		wantTotal += float64(size)
	}

	got := progressOfOneCall(t, func(ctx context.Context, req *mcp.CallToolRequest) error {
		_, err := PublishDirectory(ctx, req, client, PublishDirInput{
			ProjectID:      "42",
			PackageName:    "pkg",
			PackageVersion: "1.0.0",
			DirectoryPath:  dir,
		})
		return err
	}, func(seen []mcp.ProgressNotificationParams) bool {
		// The job is done when the last frame reports the whole total, which
		// is the same thing the assertions below check for.
		return len(seen) > 0 && seen[len(seen)-1].Progress == wantTotal
	})

	if len(got) < 2 {
		t.Fatalf("got %d progress notifications, want several: the call reported almost nothing", len(got))
	}

	prev := -1.0
	for i, n := range got {
		if n.Progress <= prev {
			t.Errorf("notification %d went backwards: progress %v after %v (message %q)",
				i, n.Progress, prev, n.Message)
		}
		// A total that changes meaning mid-call is the same defect seen from
		// the other side: the client cannot render a bar against it.
		if n.Total != wantTotal {
			t.Errorf("notification %d has total %v, want %v (the whole job, on one scale)",
				i, n.Total, wantTotal)
		}
		prev = n.Progress
	}

	if last := got[len(got)-1]; last.Progress != wantTotal {
		t.Errorf("the last notification reports %v of %v, want the job finished", last.Progress, wantTotal)
	}
}

// progressOfOneCall runs handler as one MCP tool call with a progress token and
// returns every progress notification the client received, in order.
//
// The whole sequence is what matters: "the progress value MUST increase with
// each notification" is a property of the series a client observes on one
// token, so a test that inspects notifications one at a time, or that reads the
// tracker's own state, cannot check it.
func progressOfOneCall(
	t *testing.T,
	handler func(context.Context, *mcp.CallToolRequest) error,
	complete func([]mcp.ProgressNotificationParams) bool,
) []mcp.ProgressNotificationParams {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	var mu sync.Mutex
	var got []mcp.ProgressNotificationParams

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "progress-test", Version: "v0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "under_test", Description: "the handler under test"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			if err := handler(ctx, req); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})

	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "progress-client", Version: "v0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, r *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, *r.Params)
		},
	})
	cs, connErr := mcpClient.Connect(ctx, clientTransport, nil)
	if connErr != nil {
		t.Fatalf("client connect: %v", connErr)
	}
	t.Cleanup(func() { _ = cs.Close(); ss.Wait() })

	if _, callErr := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "under_test",
		Arguments: map[string]any{},
		Meta:      mcp.Meta{"progressToken": "tok"},
	}); callErr != nil {
		t.Fatalf("CallTool: %v", callErr)
	}

	// Progress notifications are one-way messages: the client queues them for
	// its handler while CallTool waits only for the tool response, so the call
	// can return before the last of them has been delivered. Waiting for the
	// caller's completion condition is what makes the sequence assertions
	// deterministic — a quiet period would only be a guess at how long delivery
	// takes, and a slow machine would make it wrong.
	waitFor(t, &mu, &got, complete)

	mu.Lock()
	defer mu.Unlock()
	return append([]mcp.ProgressNotificationParams(nil), got...)
}

// waitFor blocks until the notifications received satisfy complete, or fails
// the test with what it did see.
//
// Naming the terminal condition rather than waiting out a silence is the
// difference between a deterministic test and one that passes on a fast machine.
func waitFor(
	t *testing.T,
	mu *sync.Mutex,
	got *[]mcp.ProgressNotificationParams,
	complete func([]mcp.ProgressNotificationParams) bool,
) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		snapshot := append([]mcp.ProgressNotificationParams(nil), *got...)
		mu.Unlock()

		if complete(snapshot) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("the expected final progress notification never arrived; received %d: %+v", len(*got), *got)
}
