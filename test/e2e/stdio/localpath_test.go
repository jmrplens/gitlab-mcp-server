//go:build stdioe2e

// localpath_test.go pins which directories a caller-supplied local path may
// resolve into on stdio, where such paths are honored at all.
//
// The roots are computed from the process: its working directory, its
// os.TempDir, and the allow-list variables it was given. A unit test can only
// ask the helper about the directory the test binary happens to be running in,
// which is the package directory and never the interesting one. The case that
// matters is a server started in the user's home directory, and the only way
// to produce it is to start a process there.
//
// It is not a hypothetical arrangement. Claude Desktop starts its servers in
// "/", other clients start them in the user's home, and neither asks. A home
// directory kept as an implicit root allow-lists ~/.ssh, ~/.aws, the browser
// profiles and this server's own ~/.gitlab-mcp-server.env, so a file_path
// naming that last file uploads the very GITLAB_TOKEN the containment exists
// to protect, using that same token.
package stdioe2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// localPathHomeEnvFile is the home file this server documents for credentials.
// Uploading it is the concrete harm the containment prevents, which is why the
// case names that file rather than an anonymous one.
const localPathHomeEnvFile = ".gitlab-mcp-server.env"

// localPathGitLab is a fake instance that records the avatar upload.
//
// A refusal is only worth asserting beside evidence that nothing left the
// process: a call refused after the request went out has already put the
// file's contents on the wire. The startup probes are answered without being
// recorded so that whatever remains belongs to the tool call.
type localPathGitLab struct {
	url string

	mu      sync.Mutex
	uploads int
}

// startLocalPathGitLab serves the startup probes plus the project write that
// an avatar upload performs.
func startLocalPathGitLab(t *testing.T) *localPathGitLab {
	t.Helper()

	fake := &localPathGitLab{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"version":"17.0.0","revision":"abcdef"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":7,"username":"someone","name":"Some One"}`)
	})
	mux.HandleFunc("PUT /api/v4/projects/42", func(w http.ResponseWriter, _ *http.Request) {
		fake.mu.Lock()
		fake.uploads++
		fake.mu.Unlock()
		writeJSON(w, `{"id":42,"name":"widgets","path_with_namespace":"acme/widgets"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	return fake
}

// uploadCount returns how many avatar uploads reached the instance.
func (f *localPathGitLab) uploadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploads
}

// localPathUploadCall drives one avatar upload naming a local file and returns
// the text of the result plus whether it was reported as a failure.
func localPathUploadCall(t *testing.T, s *session, filePath string) (text string, isError bool) {
	t.Helper()

	params, err := json.Marshal(map[string]any{
		"project_id": "42",
		"filename":   "avatar.png",
		"file_path":  filePath,
	})
	if err != nil {
		t.Fatalf("building the call parameters: %v", err)
	}
	arguments := `{"action":"project.upload_avatar","params":` + string(params) + `}`
	got := s.call(t, request(1, "tools/call",
		`{"name":"gitlab_execute_action","arguments":`+arguments+`}`))

	return toolResultText(t, got)
}

// toolResultText pulls the text blocks and the failure flag out of a decoded
// tools/call response.
//
// The text blocks are what a client prints and a model reads, so they are what
// the refusal has to be legible in. A protocol-level error is returned rather
// than failed on, because a case that expects one should say so itself.
func toolResultText(t *testing.T, msg map[string]any) (text string, isError bool) {
	t.Helper()

	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("re-encoding the response: %v", err)
	}
	var decoded struct {
		Result *struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if unmarshalErr := json.Unmarshal(encoded, &decoded); unmarshalErr != nil {
		t.Fatalf("the response is not a tool result: %v: %s", unmarshalErr, encoded)
	}
	if decoded.Result == nil {
		if decoded.Error != nil {
			return decoded.Error.Message, true
		}
		t.Fatalf("the response carries neither a result nor an error: %s", encoded)
	}
	var b strings.Builder
	for _, block := range decoded.Result.Content {
		b.WriteString(block.Text)
	}
	return b.String(), decoded.Result.IsError
}

// localPathEnv is the environment for a case that chooses where the server
// believes its home and its temporary directory are.
//
// os.TempDir is always an allow-list root, and t.TempDir hands out
// subdirectories of it, so a home directory left under the real temporary
// directory would be allow-listed for a reason that has nothing to do with the
// behavior under test. Pointing the server's temporary directory somewhere
// else is what makes the home case observable. All three spellings are set
// because os.TempDir reads TMPDIR on unix and TMP or TEMP on Windows.
//
// It goes in the environment handed to the child rather than through a
// t.Setenv-style helper: what has to be isolated is the temporary directory of
// the server process, and this test process has no say in that one. A helper
// that set the variables here would change the wrong process and the case
// would assert nothing.
func localPathEnv(gitlabURL, home, tempDir string) map[string]string {
	env := baseEnv(gitlabURL)
	env["HOME"] = home
	env["TMPDIR"] = tempDir
	env["TMP"] = tempDir
	env["TEMP"] = tempDir
	return env
}

// TestLocalPath_HomeDirectoryIsNotAnImplicitAllowlistRoot starts the real
// binary in the user's home directory and asserts a file_path naming a file
// there is refused, while the same containment still accepts the directories
// it is meant to.
//
// The three rows are one decision seen from three sides, and any one of them
// alone would pass against a broken implementation. The first is the fix. The
// second is the escape hatch that keeps it a default rather than a policy: an
// operator whose workspace really is their home directory names it and gets it
// back. The third is the guard against over-correcting, because dropping the
// working directory as a root altogether would satisfy the first two rows and
// break every ordinary upload.
//
// The refusal is checked for the variable that widens the roots as well as for
// the refusal itself. Without it the case reads as a bad path, and the person
// whose setup just stopped working has nothing to act on.
func TestLocalPath_HomeDirectoryIsNotAnImplicitAllowlistRoot(t *testing.T) {
	gitlab := startLocalPathGitLab(t)

	home := t.TempDir()
	// Somewhere the server may legitimately read from, chosen so it is not an
	// ancestor of the home directory: otherwise the temporary root would allow
	// the home file and the first row could never refuse.
	serverTemp := t.TempDir()

	secret := filepath.Join(home, localPathHomeEnvFile)
	if err := os.WriteFile(secret, []byte("GITLAB_TOKEN=glpat-the-operators-own-token\n"), 0o600); err != nil {
		t.Fatalf("writing the home credential file: %v", err)
	}

	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		t.Fatalf("creating the workspace directory: %v", err)
	}
	inWorkspace := filepath.Join(workspace, "avatar.png")
	if err := os.WriteFile(inWorkspace, []byte("an ordinary file in the project being worked on"), 0o600); err != nil {
		t.Fatalf("writing the workspace file: %v", err)
	}

	tests := []struct {
		name string
		// dir is the working directory the client starts the server in.
		dir string
		// allowlist is what GITLAB_MCP_ALLOWED_UPLOAD_DIRS is set to, if
		// anything.
		allowlist string
		filePath  string
		// wantRefused says whether the upload must be refused.
		wantRefused bool
	}{
		{
			name:        "a file in the home directory the server was started in",
			dir:         home,
			filePath:    secret,
			wantRefused: true,
		},
		{
			name:      "the same file once the operator allow-lists that directory",
			dir:       home,
			allowlist: home,
			filePath:  secret,
		},
		{
			name:     "a file in the workspace the server was started in",
			dir:      workspace,
			filePath: inWorkspace,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := localPathEnv(gitlab.url, home, serverTemp)
			if tt.allowlist != "" {
				env["GITLAB_MCP_ALLOWED_UPLOAD_DIRS"] = tt.allowlist
			}
			before := gitlab.uploadCount()

			s := startSessionInDir(t, tt.dir, env)
			body, isError := localPathUploadCall(t, s, tt.filePath)

			if tt.wantRefused {
				assertUploadRefused(t, s, gitlab, before, body, isError)
				return
			}
			assertUploadAccepted(t, gitlab, before, body, isError)
		})
	}
}

// assertUploadRefused checks everything a containment refusal owes its caller:
// that it happened, that it says what it was and which variable widens the
// roots, that nothing left the process, and that the server explained why the
// working directory stopped being one.
func assertUploadRefused(t *testing.T, s *session, gitlab *localPathGitLab, before int, body string, isError bool) {
	t.Helper()

	if !isError {
		t.Fatalf("a file in the home directory was uploaded: %s", body)
	}
	if !strings.Contains(body, "outside allowed directories") {
		t.Errorf("the refusal does not say the path was outside the allowed roots: %s", body)
	}
	if !strings.Contains(body, "GITLAB_MCP_ALLOWED_UPLOAD_DIRS") {
		t.Errorf("the refusal does not name the variable that widens the roots, so nobody can act on it: %s", body)
	}
	if got := gitlab.uploadCount(); got != before {
		t.Errorf("the refused upload still reached GitLab (%d uploads, was %d)", got, before)
	}
	// The narrowing is silent otherwise, and a working setup that stops
	// working deserves the reason rather than a puzzle.
	awaitStderr(t, s, "working directory is the home directory", 10*time.Second)
}

// assertUploadAccepted checks that an allowed path was not merely tolerated
// but actually uploaded, since a refusal that reported success would satisfy
// the first half alone.
func assertUploadAccepted(t *testing.T, gitlab *localPathGitLab, before int, body string, isError bool) {
	t.Helper()

	if isError {
		t.Fatalf("an allowed path was refused: %s", body)
	}
	if gitlab.uploadCount() == before {
		t.Errorf("the accepted upload never reached GitLab, so nothing was uploaded: %s", body)
	}
}
