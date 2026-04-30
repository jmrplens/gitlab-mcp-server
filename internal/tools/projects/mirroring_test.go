// mirroring_test.go contains unit tests for project pull mirror operations.
package projects

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// Test paths for mirroring operations.
const (
	pathProject42MirrorPull = "/api/v4/projects/42/mirror/pull"

	pullMirrorJSON = `{
		"id":5,
		"enabled":true,
		"url":"https://github.com/example/repo.git",
		"update_status":"finished",
		"mirror_trigger_builds":true,
		"only_mirror_protected_branches":false,
		"mirror_overwrites_diverged_branches":false
	}`
)

// TestGetPullMirror_Success verifies GetPullMirror returns the configured pull mirror for a project when the GitLab API responds with HTTP 200.
func TestGetPullMirror_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42MirrorPull {
			testutil.RespondJSON(w, http.StatusOK, pullMirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetPullMirror(context.Background(), client, GetPullMirrorInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if !out.Enabled {
		t.Error("Enabled = false, want true")
	}
	if out.URL != "https://github.com/example/repo.git" {
		t.Errorf("URL = %q, want %q", out.URL, "https://github.com/example/repo.git")
	}
	if out.UpdateStatus != "finished" {
		t.Errorf("UpdateStatus = %q, want %q", out.UpdateStatus, "finished")
	}
}

// TestGetPullMirror_EmptyProjectID verifies GetPullMirror returns an error when ProjectID is empty.
func TestGetPullMirror_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetPullMirror(context.Background(), client, GetPullMirrorInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestGetPullMirror_APIError verifies GetPullMirror returns an error when the GitLab API responds with HTTP 400.
func TestGetPullMirror_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := GetPullMirror(context.Background(), client, GetPullMirrorInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestConfigurePullMirror_Success verifies ConfigurePullMirror issues PUT to the pull mirror endpoint and returns the updated configuration on HTTP 200.
func TestConfigurePullMirror_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProject42MirrorPull {
			testutil.RespondJSON(w, http.StatusOK, pullMirrorJSON)
			return
		}
		http.NotFound(w, r)
	}))
	enabled := true
	out, err := ConfigurePullMirror(context.Background(), client, ConfigurePullMirrorInput{
		ProjectID: "42", Enabled: &enabled, URL: "https://github.com/example/repo.git",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if !out.Enabled {
		t.Error("Enabled = false, want true")
	}
}

// TestConfigurePullMirror_EmptyProjectID verifies ConfigurePullMirror returns an error when ProjectID is empty.
func TestConfigurePullMirror_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ConfigurePullMirror(context.Background(), client, ConfigurePullMirrorInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestConfigurePullMirror_APIError verifies ConfigurePullMirror returns an error when the GitLab API responds with HTTP 400.
func TestConfigurePullMirror_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ConfigurePullMirror(context.Background(), client, ConfigurePullMirrorInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestConfigurePullMirror_ContextCancelled verifies ConfigurePullMirror returns an error when the request context is already cancelled.
func TestConfigurePullMirror_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestStartMirroring_Success verifies StartMirroring triggers an immediate pull mirror sync and succeeds on HTTP 200.
func TestStartMirroring_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42MirrorPull {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	err := StartMirroring(context.Background(), client, StartMirroringInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestStartMirroring_EmptyProjectID verifies StartMirroring returns an error when ProjectID is empty.
func TestStartMirroring_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := StartMirroring(context.Background(), client, StartMirroringInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestStartMirroring_APIError verifies StartMirroring returns an error when the GitLab API responds with HTTP 400.
func TestStartMirroring_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := StartMirroring(context.Background(), client, StartMirroringInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFormatPullMirrorMarkdown_NonEmpty verifies FormatPullMirrorMarkdown produces non-empty markdown describing the pull mirror configuration.
func TestFormatPullMirrorMarkdown_NonEmpty(t *testing.T) {
	md := FormatPullMirrorMarkdown(PullMirrorOutput{
		ID: 5, Enabled: true, URL: "https://github.com/example/repo.git",
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "https://github.com/example/repo.git") {
		t.Error("markdown missing mirror URL")
	}
}
