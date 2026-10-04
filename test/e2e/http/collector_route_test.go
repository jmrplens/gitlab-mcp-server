//go:build httpe2e

package httpe2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCollectorRoute_TheGitLabCallNamesTheRouteItReached holds the one line of
// cmd/server that turns url.template on: the startup call that hands the
// transport the fine-grained table's route matcher. Every unit test of the
// transport declares a matcher of its own, so without this a binary that never
// declared one would leave every client span named by its method alone and
// every test green.
//
// It reads the span the way a collector does, in the exported bytes: the
// template is both the client span's url.template and the second half of its
// name, and an instance installed under a path of its own (a relative URL
// root) gets the same template, since that path is the instance's and, where
// a caller names the instance, the caller's to choose.
func TestCollectorRoute_TheGitLabCallNamesTheRouteItReached(t *testing.T) {
	const (
		template = "/api/v4/projects/:id/issues"
		spanName = "GET " + template
	)
	for _, root := range []string{"", "/gitlab"} {
		name := "at the host root"
		if root != "" {
			name = "under the relative URL root " + root
		}
		t.Run(name, func(t *testing.T) {
			gitlab := startFakeGitLabUnder(t, root)
			c := startCollector(t)
			srv := startServer(t, collectorEnv(c), "--gitlab-url="+gitlab)

			srv.do(t, executeAction(1, "issue.list", `{"project_id":"a/b"}`))

			c.awaitExport(t, 20*time.Second)
			traces := awaitTracesHolding(c, spanName, 20*time.Second)
			if !strings.Contains(traces, spanName) {
				t.Fatalf("no exported span is named %q; the binary declared no route matcher, or it matched nothing", spanName)
			}
			if !strings.Contains(traces, "url.template") {
				t.Errorf("the span named %q carries no url.template attribute", spanName)
			}
			if root != "" && strings.Contains(traces, root+template) {
				t.Errorf("a span carries the instance's own path %s in front of the template", root)
			}
		})
	}
}

// awaitTracesHolding returns every trace payload received so far, concatenated,
// once one of them holds want or the wait is over, whichever comes first. The
// batch processor exports on a schedule, so the span a test is about can land
// in a later payload than the first one.
func awaitTracesHolding(c *collector, want string, within time.Duration) string {
	deadline := time.Now().Add(within)
	for {
		var traces strings.Builder
		for _, e := range c.received() {
			if e.path == "/v1/traces" {
				traces.Write(e.body)
			}
		}
		if strings.Contains(traces.String(), want) || time.Now().After(deadline) {
			return traces.String()
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// startFakeGitLabUnder runs a stand-in GitLab whose API sits under root, the
// way an instance installed at a relative URL root serves it, and returns the
// URL a deployment names it by. It answers the version and the user the server
// asks for when a credential first arrives, and 404 to everything else, which
// is all the route of a call needs: the span is named before the answer.
func startFakeGitLabUnder(t *testing.T, root string) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc(root+"/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc(root+"/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + root
}
