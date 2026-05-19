package main

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestOptionalEnvironmentScopeFromPrompt_ExtractsExplicitAndProductionScopes
// verifies CI variable fixture preparation uses the intended environment scope.
func TestOptionalEnvironmentScopeFromPrompt_ExtractsExplicitAndProductionScopes(t *testing.T) {
	if got := projectVariableEnvironmentScope("update variable with environment_scope `staging`"); got != "staging" {
		t.Fatalf("projectVariableEnvironmentScope(explicit) = %q, want staging", got)
	}
	if got := projectVariableEnvironmentScope("delete variable in production scope"); got != "production" {
		t.Fatalf("projectVariableEnvironmentScope(production) = %q, want production", got)
	}
	if got := projectVariableEnvironmentScope("delete unscoped variable"); got != "*" {
		t.Fatalf("projectVariableEnvironmentScope(default) = %q, want *", got)
	}
}

// TestPromptInt64After_ParsesBacktickInteger verifies live fixture prompt
// rewriting fails loudly when numeric placeholders are absent or malformed.
func TestPromptInt64After_ParsesBacktickInteger(t *testing.T) {
	got, err := promptInt64After("delete award emoji ID `42`", promptMarkerAwardEmojiID)
	if err != nil || got != 42 {
		t.Fatalf("promptInt64After() = %d, %v; want 42", got, err)
	}
	_, malformedErr := promptInt64After("delete award emoji ID `abc`", promptMarkerAwardEmojiID)
	if malformedErr == nil {
		t.Fatal("promptInt64After(malformed) error = nil, want error")
	}
	_, missingErr := promptInt64After("delete missing value", promptMarkerAwardEmojiID)
	if missingErr == nil {
		t.Fatal("promptInt64After(missing) error = nil, want error")
	}
}

// TestLiveAwardEmojiNames_ContainsFallbackCandidates verifies award fixture
// creation has multiple deterministic names to try.
func TestLiveAwardEmojiNames_ContainsFallbackCandidates(t *testing.T) {
	names := liveAwardEmojiNames()
	if len(names) < 3 || !slices.Contains(names, "thumbsup") || !slices.Contains(names, "tada") {
		t.Fatalf("liveAwardEmojiNames() = %v, want common GitLab emoji candidates", names)
	}
	if strings.Join(names, ",") != strings.ToLower(strings.Join(names, ",")) {
		t.Fatalf("liveAwardEmojiNames() = %v, want lowercase names", names)
	}
}

// TestLiveTargetURLHelpers_ValidateEnvAndEscaping verifies live target helpers
// reject unsafe URLs and construct escaped GitLab endpoints deterministically.
func TestLiveTargetURLHelpers_ValidateEnvAndEscaping(t *testing.T) {
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "")
	client, err := liveGitLabHTTPClient()
	if err != nil || client != http.DefaultClient {
		t.Fatalf("liveGitLabHTTPClient(default) = %v, %v; want default client", client, err)
	}
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "not-bool")
	_, invalidTLSErr := liveGitLabHTTPClient()
	if invalidTLSErr == nil {
		t.Fatal("liveGitLabHTTPClient(invalid bool) error = nil, want error")
	}

	t.Setenv("GITLAB_URL", "https://gitlab.example.com/root/")
	baseURL, err := liveDockerGitLabBaseURL()
	if err != nil || baseURL.String() != "https://gitlab.example.com/root" {
		t.Fatalf("liveDockerGitLabBaseURL() = %v, %v; want trimmed URL", baseURL, err)
	}
	t.Setenv("GITLAB_URL", "ftp://gitlab.example.com")
	_, invalidURLErr := liveDockerGitLabBaseURL()
	if invalidURLErr == nil {
		t.Fatal("liveDockerGitLabBaseURL(ftp) error = nil, want unsupported scheme")
	}

	endpoint := terraformStateLockEndpoint(&url.URL{Scheme: "https", Host: "gitlab.example.com"}, "group/project", "state one")
	if !strings.Contains(endpoint, "group%2Fproject") || !strings.Contains(endpoint, "state%20one/lock") {
		t.Fatalf("terraformStateLockEndpoint() = %q, want escaped project and state", endpoint)
	}
}

// TestLiveRemoteMirrorTargetURL_EmbedsTokenAndProjectPath verifies mirror target
// URLs use the internal GitLab base and OAuth2 credentials expected by Docker.
func TestLiveRemoteMirrorTargetURL_EmbedsTokenAndProjectPath(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "token-123")
	t.Setenv("E2E_GITLAB_INTERNAL_URL", "http://gitlab-internal/root")
	got, err := liveRemoteMirrorTargetURL(&gl.Project{PathWithNamespace: "/group/project"})
	if err != nil {
		t.Fatalf("liveRemoteMirrorTargetURL() error = %v", err)
	}
	if !strings.HasPrefix(got, "http://oauth2:token-123@gitlab-internal/root/group/project.git") {
		t.Fatalf("liveRemoteMirrorTargetURL() = %q, want internal OAuth URL", got)
	}
	_, emptyPathErr := liveRemoteMirrorTargetURL(&gl.Project{})
	if emptyPathErr == nil {
		t.Fatal("liveRemoteMirrorTargetURL(empty path) error = nil, want error")
	}
}
