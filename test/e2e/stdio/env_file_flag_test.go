//go:build stdioe2e

// env_file_flag_test.go drives --env-file, the flag spelling of
// GITLAB_MCP_ENV_FILE, against the real binary: the file it names configures
// the process, and it wins over the variable when both are given.
package stdioe2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envFileNaming writes a dotenv file that points the server at gitlabURL and
// returns its path.
func envFileNaming(t *testing.T, gitlabURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deployment.env")
	body := "GITLAB_URL=" + gitlabURL + "\nGITLAB_TOKEN=glpat-stdio-e2e-token\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the env file: %v", err)
	}
	return path
}

// TestEnvFileFlag_NamesTheFileToLoad starts the binary with --env-file and no
// variable, and asserts the instance the file names is the one a call
// reaches.
func TestEnvFileFlag_NamesTheFileToLoad(t *testing.T) {
	genuine := startFakeGitLab(t)
	s := startSessionWithArgs(t, map[string]string{"LOG_LEVEL": "info"}, "--env-file="+envFileNaming(t, genuine.URL))

	called := s.call(t, request(1, "tools/call",
		`{"name":"gitlab_execute_action","arguments":{"action":"user.current","params":{}}}`))
	if called["error"] != nil {
		t.Fatalf("the tool call failed, so the file named by --env-file configured nothing: %v", called["error"])
	}
	if body := envFileJSON(t, called); !strings.Contains(body, "someone") {
		t.Errorf("the call was not answered by the instance --env-file names:\n%s", body)
	}
}

// TestEnvFileFlag_WinsOverTheVariable gives the variable a file naming one
// instance and the flag a file naming another, and asserts the flag's
// instance answers and the variable's is never contacted: a flag typed on
// the command line wins over its variable, as every other setting's does.
func TestEnvFileFlag_WinsOverTheVariable(t *testing.T) {
	genuine := startFakeGitLab(t)
	impostor := startEnvFileImpostor(t)
	s := startSessionWithArgs(t,
		map[string]string{"LOG_LEVEL": "info", "GITLAB_MCP_ENV_FILE": envFileNaming(t, impostor.URL)},
		"--env-file="+envFileNaming(t, genuine.URL))

	called := s.call(t, request(1, "tools/call",
		`{"name":"gitlab_execute_action","arguments":{"action":"user.current","params":{}}}`))
	if called["error"] != nil {
		t.Fatalf("the tool call failed: %v", called["error"])
	}
	if body := envFileJSON(t, called); !strings.Contains(body, "someone") {
		t.Errorf("the call was not answered by the instance the flag's file names:\n%s", body)
	}
	if got := impostor.requests.Load(); got != 0 {
		t.Errorf("the variable's file sent %d request(s) to the host it named while the flag named another: %v",
			got, impostor.seenPaths())
	}
}
