//go:build e2e

// terraform.go seeds a project's Terraform state, which no MCP action can
// create: the covered surface only reads, locks and deletes state, so a test
// of that surface needs a state pushed through the raw HTTP backend protocol
// first. The push authenticates with HTTP basic auth, which the Terraform
// backend requires in place of the PRIVATE-TOKEN header the REST API takes.

package fixture

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// SettingSkipTLSVerify is the configuration key that turns TLS verification
// off, as the raw backends the fixture drives read it beside the client.
const SettingSkipTLSVerify = "GITLAB_MCP_SKIP_TLS_VERIFY"

// terraformPushTimeout bounds one raw state push.
const terraformPushTimeout = 30 * time.Second

// PushTerraformState pushes one Terraform state version into a project through
// the raw backend protocol, so a test of the Terraform state surface has a
// state to read, lock and delete. It fails the test when the push is refused,
// since every scenario built on the state depends on it.
func PushTerraformState(e *harness.Env, project Project, name string, serial int) {
	e.T.Helper()

	token := e.Setting(SettingGitLabToken)
	if token == "" {
		e.T.Fatalf("%s is required to push Terraform state through the raw backend", SettingGitLabToken)
	}
	stateURL := terraformStateURL(e.Runtime().URL, project.ID, name)
	requireConfidentialTransport(e, stateURL, "the Terraform state backend")
	body := terraformStateBody(project.ID, serial)

	ctx, cancel := context.WithTimeout(e.Ctx, terraformPushTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stateURL, bytes.NewReader([]byte(body)))
	if err != nil {
		e.T.Fatalf("building the Terraform state push request: %v", err)
	}
	req.Header.Set("Authorization", terraformBasicAuth(e.Runtime().Username, token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: gitlabclient.HTTPTransport(strings.EqualFold(e.Setting(SettingSkipTLSVerify), "true")),
		Timeout:   terraformPushTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		e.T.Fatalf("pushing Terraform state serial %d to %s: %v", serial, name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.T.Fatalf("pushing Terraform state serial %d: HTTP %d, body %s", serial, resp.StatusCode, payload)
	}
}

// terraformLockID is the lock a fixture takes. Terraform's own client sends a
// UUID; GitLab stores whatever string it is given and hands it back to the
// holder, so a stable one is enough and makes a failed unlock readable.
const terraformLockID = "e2e-fixture-lock"

// LockTerraformState takes the state's lock, so that a case asked to clear one
// has a lock to clear.
//
// It goes through the raw backend for the reason the push does, and the reason
// is sharper here: the lock action this server publishes sends no Terraform
// lock-info body, so GitLab refuses every call it makes, and a fixture that
// locked through the tool under test would be asserting on a refusal. The
// body below is the one Terraform's HTTP backend sends.
func LockTerraformState(e *harness.Env, project Project, name string) {
	e.T.Helper()

	body := fmt.Sprintf(
		`{"ID":%q,"Operation":"e2e-fixture","Info":"","Who":%q,"Version":"1.5.0","Created":%q,"Path":""}`,
		terraformLockID, e.Runtime().Username, time.Now().UTC().Format(time.RFC3339),
	)
	status, payload := terraformBackendRequest(e, http.MethodPost,
		terraformStateURL(e.Runtime().URL, project.ID, name)+"/lock?ID="+terraformLockID, body)

	// A state already locked answers 409 and is the state the fixture wanted.
	if status != http.StatusOK && status != http.StatusConflict {
		e.T.Fatalf("locking Terraform state %q of project %d: HTTP %d, body %s", name, project.ID, status, payload)
	}

	e.Defer("terraform state lock "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, unlockErr := e.Client().GL().TerraformStates.Unlock(strconv.FormatInt(project.ID, 10), name, gl.WithContext(ctx))
		if unlockErr != nil && !IsStatus(unlockErr, http.StatusNotFound) && !IsStatus(unlockErr, http.StatusConflict) {
			return fmt.Errorf("unlocking Terraform state %q of project %d: %w", name, project.ID, unlockErr)
		}
		return nil
	})
}

// terraformStateLockQuery is how the lock is read. GitLab publishes it over
// GraphQL and nowhere in the REST surface.
const terraformStateLockQuery = `query($projectPath: ID!, $name: String!) {
  project(fullPath: $projectPath) {
    terraformState(name: $name) { lockedAt }
  }
}`

// TerraformStateIsLocked reports whether the state still carries a lock, which
// is what a case that clears one is verified against.
func TerraformStateIsLocked(ctx context.Context, client *gitlabclient.Client, projectPath, name string) (bool, error) {
	var answer struct {
		Project *struct {
			TerraformState *struct {
				LockedAt *string `json:"lockedAt"`
			} `json:"terraformState"`
		} `json:"project"`
	}
	if err := runGraphQL(ctx, client, terraformStateLockQuery, map[string]any{
		"projectPath": projectPath, "name": name,
	}, &answer); err != nil {
		return false, fmt.Errorf("reading the lock of Terraform state %q: %w", name, err)
	}
	if answer.Project == nil || answer.Project.TerraformState == nil {
		return false, nil
	}
	return answer.Project.TerraformState.LockedAt != nil, nil
}

// terraformBackendRequest sends one raw backend request with basic auth and
// returns the status and body it answered.
func terraformBackendRequest(e *harness.Env, method, url, body string) (status int, payload []byte) {
	e.T.Helper()

	token := e.Setting(SettingGitLabToken)
	if token == "" {
		e.T.Fatalf("%s is required to reach the Terraform state backend", SettingGitLabToken)
	}
	requireConfidentialTransport(e, url, "the Terraform state backend")

	ctx, cancel := context.WithTimeout(e.Ctx, terraformPushTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader([]byte(body)))
	if err != nil {
		e.T.Fatalf("building the Terraform backend request: %v", err)
	}
	req.Header.Set("Authorization", terraformBasicAuth(e.Runtime().Username, token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: gitlabclient.HTTPTransport(strings.EqualFold(e.Setting(SettingSkipTLSVerify), "true")),
		Timeout:   terraformPushTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		e.T.Fatalf("sending %s to the Terraform backend: %v", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, _ = io.ReadAll(resp.Body)
	return resp.StatusCode, payload
}

// terraformStateURL builds the raw backend URL one state version is pushed to.
func terraformStateURL(base string, projectID int64, name string) string {
	return fmt.Sprintf("%s/api/v4/projects/%d/terraform/state/%s", strings.TrimRight(base, "/"), projectID, name)
}

// terraformStateBody is the minimal, valid Terraform state document one push
// carries, at the given serial.
func terraformStateBody(projectID int64, serial int) string {
	return fmt.Sprintf(
		`{"version":4,"terraform_version":"1.5.0","serial":%d,"lineage":"e2e-lineage-%d","outputs":{},"resources":[]}`,
		serial, projectID,
	)
}

// terraformBasicAuth builds the HTTP basic auth header the Terraform backend
// requires, from the suite user and its token.
func terraformBasicAuth(username, token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+token))
}
