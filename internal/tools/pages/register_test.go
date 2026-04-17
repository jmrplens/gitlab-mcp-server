package pages

import (
	"context"
	"net/http"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_ConfirmDeclined covers the ConfirmAction early-return
// branches in pages unpublish and domain delete handlers when the user declines.
func TestRegisterTools_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_pages_unpublish", map[string]any{"project_id": "42"}},
		{"gitlab_pages_domain_delete", map[string]any{"project_id": "42", "domain": "example.com"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

// TestToPagesOutput_Nil covers the nil guard in toPagesOutput.
func TestToPagesOutput_Nil(t *testing.T) {
	out := toPagesOutput(nil)
	if out.URL != "" {
		t.Error("expected empty URL for nil Pages")
	}
}

// TestToDomainOutput_NilAndOptionalFields covers nil guard and optional
// branches (EnabledUntil, Certificate.Expiration) in toDomainOutput.
func TestToDomainOutput_NilAndOptionalFields(t *testing.T) {
	out := toDomainOutput(nil)
	if out.Domain != "" {
		t.Error("expected empty Domain for nil PagesDomain")
	}

	now := time.Now()
	d := &gl.PagesDomain{
		Domain:         "example.com",
		AutoSslEnabled: true,
		URL:            "https://example.com",
		ProjectID:      42,
		Verified:       true,
		EnabledUntil:   &now,
		Certificate: struct {
			Subject         string     `json:"subject"`
			Expired         bool       `json:"expired"`
			Expiration      *time.Time `json:"expiration"`
			Certificate     string     `json:"certificate"`
			CertificateText string     `json:"certificate_text"`
		}{
			Subject:    "CN=example.com",
			Expiration: &now,
		},
	}
	out2 := toDomainOutput(d)
	if out2.EnabledUntil == "" {
		t.Error("expected non-empty EnabledUntil")
	}
	if out2.Certificate.Expiration == "" {
		t.Error("expected non-empty Certificate.Expiration")
	}
}
