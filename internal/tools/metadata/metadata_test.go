// metadata_test.go contains unit tests for the GitLab metadata MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package metadata

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/metadata" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"version": "16.8.0",
			"revision": "abc123",
			"kas": {
				"enabled": true,
				"externalUrl": "wss://kas.example.com",
				"externalK8sProxyUrl": "https://kas.example.com/k8s-proxy",
				"version": "16.8.0-rc1"
			},
			"enterprise": true
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Version != "16.8.0" {
		t.Errorf("Version = %q, want 16.8.0", out.Version)
	}
	if out.Revision != "abc123" {
		t.Errorf("Revision = %q, want abc123", out.Revision)
	}
	if !out.Enterprise {
		t.Error("Enterprise = false, want true")
	}
	if !out.KAS.Enabled {
		t.Error("KAS.Enabled = false, want true")
	}
	if out.KAS.Version != "16.8.0-rc1" {
		t.Errorf("KAS.Version = %q, want 16.8.0-rc1", out.KAS.Version)
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		Version:    "16.8.0",
		Revision:   "abc123",
		Enterprise: true,
		KAS:        KASInfo{Enabled: true, Version: "16.8.0-rc1", ExternalURL: "wss://kas"},
	}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "16.8.0") {
		t.Error("missing version")
	}
	if !strings.Contains(md, "abc123") {
		t.Error("missing revision")
	}
	if !strings.Contains(md, "KAS Enabled") {
		t.Error("missing KAS enabled")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const covCovMetaJSON = `{"version":"17.0.0","revision":"abc123","kas":{"enabled":true,"external_url":"https://kas.example.com","external_k8s_proxy_url":"https://k8s.example.com","version":"17.0.0"},"enterprise":true}`

// TestGet_APIError verifies the behavior of cov get a p i error.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success verifies the behavior of cov get success.
func TestGet_Success_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covCovMetaJSON)
	}))
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Version != "17.0.0" || !out.Enterprise || !out.KAS.Enabled {
		t.Errorf("unexpected: %+v", out)
	}
}

// TestFormatGetMarkdown_Full verifies the behavior of cov format get markdown full.
func TestFormatGetMarkdown_Full_Coverage(t *testing.T) {
	out := GetOutput{
		Version:    "17.0.0",
		Revision:   "abc123",
		Enterprise: true,
		KAS: KASInfo{
			Enabled:     true,
			Version:     "17.0.0",
			ExternalURL: "https://kas.example.com",
		},
	}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "17.0.0") || !strings.Contains(md, "abc123") || !strings.Contains(md, "kas.example.com") {
		t.Error("expected metadata in markdown")
	}
}

// TestFormatGetMarkdown_NoKAS_Coverage verifies the behavior of cov format get markdown no k a s.
func TestFormatGetMarkdown_NoKAS_Coverage(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Version: "17.0.0"})
	if strings.Contains(md, "KAS Version") || strings.Contains(md, "KAS URL") {
		t.Error("should not show KAS details when empty")
	}
}

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covCovMetaJSON)
	}))
	RegisterTools(server, client)
}

// TestMCPRound_Trip verifies the behavior of cov m c p round trip.
func TestMCPRound_Trip_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covCovMetaJSON)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_get_metadata",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}
