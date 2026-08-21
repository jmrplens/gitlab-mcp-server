// Package cachehints_test verifies the SEP-2549 cache-hint middleware policy.
package cachehints_test

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cachehints"
)

// stubHandler returns a canned result so the middleware's mutations are observable.
func stubHandler(result mcp.Result) mcp.MethodHandler {
	return func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return result, nil
	}
}

// readRequest builds a resources/read request for the given resource URI.
func readRequest(uri string) mcp.Request {
	return &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: uri}}
}

// TestMiddleware_ToolsList_PrivateWithTTL verifies that tools/list results are
// stamped private (token/tier-dependent catalogs must not be shared) with the
// 5-minute freshness hint.
func TestMiddleware_ToolsList_PrivateWithTTL(t *testing.T) {
	handler := cachehints.Middleware()(stubHandler(&mcp.ListToolsResult{}))
	res, err := handler(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	cacheable, ok := res.(mcp.CacheableResult)
	if !ok {
		t.Fatalf("result %T does not implement CacheableResult", res)
	}
	if got := cacheable.GetCacheScope(); got != "private" {
		t.Errorf("CacheScope = %q, want private", got)
	}
	if got := cacheable.GetTTLMs(); got != 300000 {
		t.Errorf("TTLMs = %d, want 300000", got)
	}
}

// TestMiddleware_CatalogMethods_PrivateWithListTTL verifies that every catalog
// listing method (prompts, resources, resource templates, server discovery)
// receives the same private/5-minute hint as tools/list, because all of them
// are filtered by the caller's token scopes and licensing tier.
func TestMiddleware_CatalogMethods_PrivateWithListTTL(t *testing.T) {
	tests := []struct {
		name   string
		method string
		result mcp.Result
	}{
		{"prompts_list", "prompts/list", &mcp.ListPromptsResult{}},
		{"resources_list", "resources/list", &mcp.ListResourcesResult{}},
		{"resource_templates_list", "resources/templates/list", &mcp.ListResourceTemplatesResult{}},
		{"server_discover", "server/discover", &mcp.DiscoverResult{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := cachehints.Middleware()(stubHandler(tt.result))(context.Background(), tt.method, nil)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			cacheable, ok := res.(mcp.CacheableResult)
			if !ok {
				t.Fatalf("result %T does not implement CacheableResult", res)
			}
			if got := cacheable.GetCacheScope(); got != "private" {
				t.Errorf("CacheScope = %q, want private", got)
			}
			if got := cacheable.GetTTLMs(); got != 300000 {
				t.Errorf("TTLMs = %d, want 300000", got)
			}
		})
	}
}

// TestMiddleware_ResourcesRead_TTLDependsOnURI verifies the resources/read
// policy: static content (guides, schemas, tool manifests) is cacheable for an
// hour, while resources backed by live GitLab API calls are marked immediately
// stale. Both are private because their content is token- and tier-dependent.
func TestMiddleware_ResourcesRead_TTLDependsOnURI(t *testing.T) {
	tests := []struct {
		name    string
		req     mcp.Request
		wantTTL int
	}{
		{"workflow_guide", readRequest("gitlab://guides/git-workflow"), 3600000},
		{"dynamic_schema_index", readRequest("gitlab://schema/dynamic/"), 3600000},
		{"meta_schema_detail", readRequest("gitlab://schema/meta/gitlab_issue/list"), 3600000},
		{"tool_manifest", readRequest("gitlab://tools"), 3600000},
		{"tool_manifest_detail", readRequest("gitlab://tools/gitlab_find_action"), 3600000},
		{"live_project", readRequest("gitlab://project/42"), 0},
		{"live_user", readRequest("gitlab://user/current"), 0},
		{"nil_request", nil, 0},
		{"other_request_type", &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}}, 0},
		{"nil_params", &mcp.ReadResourceRequest{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := cachehints.Middleware()(stubHandler(&mcp.ReadResourceResult{}))(
				context.Background(), "resources/read", tt.req,
			)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			cacheable, ok := res.(mcp.CacheableResult)
			if !ok {
				t.Fatalf("result %T does not implement CacheableResult", res)
			}
			if got := cacheable.GetCacheScope(); got != "private" {
				t.Errorf("CacheScope = %q, want private", got)
			}
			if got := cacheable.GetTTLMs(); got != tt.wantTTL {
				t.Errorf("TTLMs = %d, want %d", got, tt.wantTTL)
			}
		})
	}
}

// TestMiddleware_NonCacheableResult_Passthrough verifies results without
// Cacheable embedding (e.g. tools/call) pass through untouched.
func TestMiddleware_NonCacheableResult_Passthrough(t *testing.T) {
	want := &mcp.CallToolResult{}
	handler := cachehints.Middleware()(stubHandler(want))
	res, err := handler(context.Background(), "tools/call", nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res != mcp.Result(want) {
		t.Errorf("result = %v, want passthrough of original", res)
	}
}

// TestMiddleware_UnexpectedResultType_Passthrough verifies that a covered
// method returning a result type without Cacheable embedding is left alone
// instead of panicking, so a future SDK change cannot break request handling.
func TestMiddleware_UnexpectedResultType_Passthrough(t *testing.T) {
	want := &mcp.CallToolResult{}
	res, err := cachehints.Middleware()(stubHandler(want))(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res != mcp.Result(want) {
		t.Errorf("result = %v, want passthrough of original", res)
	}
}

// TestMiddleware_NilResult_Passthrough verifies a nil result (notification
// handling) is returned as-is without dereferencing it.
func TestMiddleware_NilResult_Passthrough(t *testing.T) {
	res, err := cachehints.Middleware()(stubHandler(nil))(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res != nil {
		t.Errorf("result = %v, want nil", res)
	}
}

// TestMiddleware_ErrorPassthrough verifies errors from the wrapped handler are
// returned unchanged without touching the (nil) result.
func TestMiddleware_ErrorPassthrough(t *testing.T) {
	wantErr := context.DeadlineExceeded
	failing := func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, wantErr
	}
	_, err := cachehints.Middleware()(failing)(context.Background(), "tools/list", nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
