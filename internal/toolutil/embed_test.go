package toolutil

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resetEmbedToggle restores the default after a test that toggled the flag.
func resetEmbedToggle(t *testing.T) {
	t.Helper()
	prev := EmbeddedResourcesEnabled()
	t.Cleanup(func() { EnableEmbeddedResources(prev) })
}

func TestEmbedResource_AppendsContentBlock(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(true)

	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "hello"}}}
	EmbedResource(result, "gitlab://project/42/issue/7", "application/json", `{"iid":7}`)

	if got := len(result.Content); got != 2 {
		t.Fatalf("expected 2 content blocks after embed, got %d", got)
	}
	er, ok := result.Content[1].(*mcp.EmbeddedResource)
	if !ok {
		t.Fatalf("second content block is %T, want *mcp.EmbeddedResource", result.Content[1])
	}
	if er.Resource == nil {
		t.Fatal("EmbeddedResource.Resource is nil")
	}
	if er.Resource.URI != "gitlab://project/42/issue/7" {
		t.Errorf("URI = %q, want gitlab://project/42/issue/7", er.Resource.URI)
	}
	if er.Resource.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", er.Resource.MIMEType)
	}
	if er.Resource.Text != `{"iid":7}` {
		t.Errorf("Text = %q, want {\"iid\":7}", er.Resource.Text)
	}
}

func TestEmbedResource_DisabledIsNoOp(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(false)

	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "x"}}}
	EmbedResource(result, "gitlab://project/1/issue/2", "application/json", "{}")

	if got := len(result.Content); got != 1 {
		t.Errorf("expected content unchanged when disabled, got %d blocks", got)
	}
}

func TestEmbedResource_NilResultIsSafe(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(true)

	// Must not panic.
	EmbedResource(nil, "gitlab://x", "application/json", "{}")
}

func TestEmbedResource_EmptyURIIsNoOp(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(true)

	result := &mcp.CallToolResult{}
	EmbedResource(result, "", "application/json", "{}")

	if len(result.Content) != 0 {
		t.Errorf("expected no content for empty URI, got %d blocks", len(result.Content))
	}
}

func TestEnableEmbeddedResources_RoundTrip(t *testing.T) {
	resetEmbedToggle(t)

	EnableEmbeddedResources(false)
	if EmbeddedResourcesEnabled() {
		t.Error("expected disabled state after EnableEmbeddedResources(false)")
	}
	EnableEmbeddedResources(true)
	if !EmbeddedResourcesEnabled() {
		t.Error("expected enabled state after EnableEmbeddedResources(true)")
	}
}

func TestEmbedResourceJSON_MarshalsValue(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(true)

	value := struct {
		IID   int64  `json:"iid"`
		Title string `json:"title"`
	}{IID: 7, Title: "bug"}

	result := &mcp.CallToolResult{}
	EmbedResourceJSON(result, "gitlab://project/42/issue/7", value)

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	er := result.Content[0].(*mcp.EmbeddedResource)
	if er.Resource.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", er.Resource.MIMEType)
	}
	want := `{"iid":7,"title":"bug"}`
	if er.Resource.Text != want {
		t.Errorf("Text = %q, want %q", er.Resource.Text, want)
	}
}

func TestEmbedResourceJSON_DisabledIsNoOp(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(false)

	result := &mcp.CallToolResult{}
	EmbedResourceJSON(result, "gitlab://project/1", struct{ A int }{A: 1})
	if len(result.Content) != 0 {
		t.Errorf("expected no content when disabled, got %d", len(result.Content))
	}
}

func TestEmbedResourceJSON_MarshalErrorIsSilent(t *testing.T) {
	resetEmbedToggle(t)
	EnableEmbeddedResources(true)

	result := &mcp.CallToolResult{}
	// Channels are not JSON-marshalable.
	EmbedResourceJSON(result, "gitlab://project/1", make(chan int))
	if len(result.Content) != 0 {
		t.Errorf("expected no content on marshal error, got %d", len(result.Content))
	}
}
