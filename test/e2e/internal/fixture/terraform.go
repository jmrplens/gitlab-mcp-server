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
	"strings"
	"time"

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
