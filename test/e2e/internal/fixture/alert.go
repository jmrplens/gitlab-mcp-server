//go:build e2e

// alert.go seeds one alert on a project, which the covered surface needs but
// cannot create: alert metric images hang off an alert, and CE has no REST
// path to create one. The Free HTTP alert integration is provisioned through
// GraphQL and one payload is fired at its notify URL, which answers
// synchronously with the IID of the alert it created.

package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// httpIntegrationCreate provisions the single Free HTTP alert integration and
// returns its notify URL and token.
const httpIntegrationCreate = `mutation($path: ID!, $name: String!) {
	httpIntegrationCreate(input: {projectPath: $path, name: $name, active: true}) {
		integration { url token }
		errors
	}
}`

// alertNotifyTimeout bounds the one notify POST that creates the alert.
const alertNotifyTimeout = 30 * time.Second

// SeedAlert creates an HTTP alert integration on the project and fires one
// alert at it, returning the IID of the alert GitLab created. It fails the
// test when the integration cannot be created or the notify does not answer
// with exactly one alert, since the metric image scenario stands on that IID.
func SeedAlert(e *harness.Env, project Project) int64 {
	e.T.Helper()

	var created struct {
		HTTPIntegrationCreate struct {
			Integration struct {
				URL   string `json:"url"`
				Token string `json:"token"`
			} `json:"integration"`
			Errors []string `json:"errors"`
		} `json:"httpIntegrationCreate"`
	}
	if err := mutate(e, httpIntegrationCreate, map[string]any{"path": project.Path, "name": "e2e-adm-alerts"}, &created); err != nil {
		e.T.Fatalf("creating the HTTP alert integration on %s: %v", project.Path, err)
	}
	if len(created.HTTPIntegrationCreate.Errors) > 0 {
		e.T.Fatalf("httpIntegrationCreate answered errors: %v", created.HTTPIntegrationCreate.Errors)
	}
	integration := created.HTTPIntegrationCreate.Integration
	if integration.URL == "" || integration.Token == "" {
		e.T.Fatalf("httpIntegrationCreate answered no URL or token: %+v", integration)
	}

	ctx, cancel := context.WithTimeout(e.Ctx, alertNotifyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, integration.URL,
		bytes.NewReader([]byte(`{"title":"E2E metric image alert","description":"seeded by the e2e suite","severity":"high"}`)))
	if err != nil {
		e.T.Fatalf("building the alert notify request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+integration.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: gitlabclient.HTTPTransport(strings.EqualFold(e.Setting(SettingSkipTLSVerify), "true")),
		Timeout:   alertNotifyTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		e.T.Fatalf("firing the alert at %s: %v", integration.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.T.Fatalf("the alert notify answered HTTP %d, body %s", resp.StatusCode, payload)
	}

	iid, err := parseAlertIID(payload)
	if err != nil {
		e.T.Fatalf("reading the created alert IID: %v (body %s)", err, payload)
	}
	return iid
}

// parseAlertIID reads the IID of the single alert the notify endpoint reports
// it created, refusing an answer that is not exactly one alert with an IID.
func parseAlertIID(payload []byte) (int64, error) {
	var alerts []struct {
		IID int64 `json:"iid"`
	}
	if err := json.Unmarshal(payload, &alerts); err != nil {
		return 0, fmt.Errorf("decoding the alert notify response: %w", err)
	}
	if len(alerts) != 1 || alerts[0].IID == 0 {
		return 0, fmt.Errorf("expected exactly one alert with an IID, got %d", len(alerts))
	}
	return alerts[0].IID, nil
}
