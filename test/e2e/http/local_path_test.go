//go:build httpe2e

// local_path_test.go pins the transport rule for caller-supplied local paths:
// a server reached over HTTP honors none of them.
//
// The rule cannot be checked from inside the process. It is decided by
// toolutil.SetLocalFilesystemAccess, which cmd/server calls once with the
// transport it is about to serve, so a unit test on the helper asserts on a
// flag it set itself and a test on a handler never reaches the decision at
// all. What an operator needs to know is different in kind: this binary,
// started with --http and reachable by somebody with no files on this machine,
// must refuse every path that somebody can name, and must still accept the
// inline form so the legitimate remote upload keeps working. Only a running
// process answers that.
//
// The paths used here are inside the OS temporary directory, which is an
// always-allowed root. That is deliberate: the containment allow-list would
// accept them, so the refusal under test can only be the transport rule and
// nothing else.
package httpe2e

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// localPathProject is what the fake instance answers a project write with, so
// the accepted call has somewhere to land and a result to report.
const localPathProject = `{"id":42,"name":"widgets","path_with_namespace":"acme/widgets","web_url":"https://gitlab.example.com/acme/widgets"}`

// localPathGitLab is a fake instance that records everything a tool call asks
// it for.
//
// The record is the second half of every assertion here. A refusal that
// reaches GitLab first has already read the file and put it on the wire, so
// "the call was refused" is only worth asserting beside "and nothing left the
// process": the startup probes are excluded from the record precisely so that
// what remains is attributable to the tool call.
type localPathGitLab struct {
	url string

	mu       sync.Mutex
	requests []string
}

// startLocalPathGitLab serves the two startup probes plus a project write.
func startLocalPathGitLab(t *testing.T) *localPathGitLab {
	t.Helper()

	fake := &localPathGitLab{}
	record := func(r *http.Request) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.requests = append(fake.requests, r.Method+" "+r.URL.Path)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone","name":"Some One","state":"active"}`))
	})
	// The tier and scope probes are answered without being recorded. They are
	// the pool's, not the tool call's, and they arrive lazily on the first
	// request rather than at startup, so leaving them in the record would
	// charge the first case with two requests it never made.
	for _, probe := range []string{"/api/v4/license", "/api/v4/personal_access_tokens/self", "/oauth/token/info"} {
		mux.HandleFunc(probe, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
	}
	mux.HandleFunc("PUT /api/v4/projects/42", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(localPathProject))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	return fake
}

// seen returns every request the fake received that was not a startup probe.
func (f *localPathGitLab) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// localPathSecret writes a file a remote caller must never be able to read
// through this server and returns its path.
//
// It goes in the OS temporary directory rather than somewhere exotic because
// that directory is an always-allowed containment root: a path there passes
// every check except the transport rule, which is the one under test.
func localPathSecret(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "local-secret.png")
	if err := os.WriteFile(path, []byte("bytes belonging to whoever runs this server"), 0o600); err != nil {
		t.Fatalf("writing the local file: %v", err)
	}
	return path
}

// TestLocalPath_HTTPRefusesEveryCallerSuppliedPath drives one tools/call per
// local-path input over the real HTTP transport and asserts each is refused
// before anything reaches GitLab.
//
// Three distinct inputs are covered because they are three distinct helpers
// with three distinct refusal points, and the audit finding was that a remote
// caller can name a path on somebody else's machine through any of them:
// file_path reads a file, output_path writes one, and directory_path reads a
// whole tree. A test covering only the first would leave the other two able to
// regress in silence.
//
// The refusal must also be legible. A model told only "invalid input" retries
// with another path; one told the input is disabled on this transport, and
// which input to use instead, stops. That is why the message is asserted and
// not merely the failure.
func TestLocalPath_HTTPRefusesEveryCallerSuppliedPath(t *testing.T) {
	gitlab := startLocalPathGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	secret := localPathSecret(t)
	directory := filepath.Dir(secret)

	tests := []struct {
		name   string
		action string
		params string
		// wantInput is the parameter name the refusal must identify.
		wantInput string
	}{
		{
			name:      "file_path reads a file from the server's own disk",
			action:    "project.upload_avatar",
			params:    `{"project_id":"42","filename":"avatar.png","file_path":"` + secret + `"}`,
			wantInput: "file_path",
		},
		{
			name:      "output_path writes one to it",
			action:    "package.download",
			params:    `{"project_id":"42","package_name":"pkg","package_version":"1.0.0","file_name":"asset.bin","output_path":"` + filepath.Join(directory, "written.bin") + `"}`,
			wantInput: "output_path",
		},
		{
			name:      "directory_path reads a whole tree",
			action:    "package.publish_directory",
			params:    `{"project_id":"42","package_name":"pkg","package_version":"1.0.0","directory_path":"` + directory + `"}`,
			wantInput: "directory_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(gitlab.seen())

			body, isError := toolResultsText(t, toolResultsCall(t, srv, tt.action, tt.params))
			if !isError {
				t.Fatalf("%s was accepted over HTTP: %s", tt.wantInput, toolResultsTruncate(body))
			}
			if !strings.Contains(body, tt.wantInput) {
				t.Errorf("the refusal does not name %s, so a model cannot tell which input to drop: %s",
					tt.wantInput, toolResultsTruncate(body))
			}
			if !strings.Contains(body, "reached over HTTP") {
				t.Errorf("the refusal does not say the transport is the reason, so it reads as a bad path: %s",
					toolResultsTruncate(body))
			}

			// A refusal that has already sent the request has already leaked
			// whatever it was asked for.
			if after := gitlab.seen(); len(after) != before {
				t.Errorf("the refused call still reached GitLab: %v", after[before:])
			}
		})
	}
}

// TestLocalPath_HTTPStillAcceptsInlineContent is the other half of the same
// rule, and the reason the fix refuses at the shared helper instead of
// removing the actions from the catalog.
//
// project.upload_avatar takes either a local path or the bytes themselves. A
// remote caller has the bytes and has no files here, so the inline form is the
// one that was always meant for them; deregistering the action would have taken
// it away along with the path. Without this case the refusal above could be
// satisfied by an action that simply stopped working, which is a different and
// worse outcome that no assertion here would have noticed.
func TestLocalPath_HTTPStillAcceptsInlineContent(t *testing.T) {
	gitlab := startLocalPathGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	inline := base64.StdEncoding.EncodeToString([]byte("bytes the caller already holds"))
	payload := toolResultsCall(t, srv, "project.upload_avatar",
		`{"project_id":"42","filename":"avatar.png","content_base64":"`+inline+`"}`)

	body, isError := toolResultsText(t, payload)
	if isError {
		t.Fatalf("content_base64 was refused too, so the transport rule took the remote path with it: %s",
			toolResultsTruncate(body))
	}
	if !strings.Contains(body, "acme/widgets") {
		t.Errorf("the accepted upload did not return the project GitLab answered with: %s", toolResultsTruncate(body))
	}
	if seen := gitlab.seen(); !slices.Contains(seen, "PUT /api/v4/projects/42") {
		t.Errorf("the accepted upload never reached GitLab, so nothing was uploaded: %v", seen)
	}
}
