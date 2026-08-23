// Package cachehints_test verifies the SEP-2549 cache-hint middleware policy.
package cachehints_test

import (
	"context"
	"errors"
	"fmt"
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

// TestMiddleware_TierDependentLists_TTLFollowsTierSource verifies the tool
// catalog policy. A detected tier can change under a running server when the
// instance license changes, so the catalog keeps the short window; a
// configured tier cannot, which earns it the same hour as the compiled-in
// catalogs. server/discover follows tools/list because it carries the
// capabilities and instructions that describe the same catalog.
func TestMiddleware_TierDependentLists_TTLFollowsTierSource(t *testing.T) {
	methods := []struct {
		name   string
		method string
		result func() mcp.Result
	}{
		{"tools_list", "tools/list", func() mcp.Result { return &mcp.ListToolsResult{} }},
		{"server_discover", "server/discover", func() mcp.Result { return &mcp.DiscoverResult{} }},
	}
	tierSources := []struct {
		name    string
		opts    cachehints.Options
		wantTTL int
	}{
		{"detected tier keeps the short window", cachehints.Options{TierPinned: false}, 300000},
		{"configured tier earns the long window", cachehints.Options{TierPinned: true}, 3600000},
	}

	for _, m := range methods {
		for _, ts := range tierSources {
			t.Run(m.name+"/"+ts.name, func(t *testing.T) {
				res, err := cachehints.Middleware(ts.opts)(stubHandler(m.result()))(
					context.Background(), m.method, nil,
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
				if got := cacheable.GetTTLMs(); got != ts.wantTTL {
					t.Errorf("TTLMs = %d, want %d", got, ts.wantTTL)
				}
			})
		}
	}
}

// TestMiddleware_StaticCatalogs_AlwaysHourTTL verifies that the catalogs
// compiled into the binary — prompts, resources and resource templates — carry
// the one-hour window regardless of how the tier was resolved. None of them is
// tier-gated, so a detected tier cannot change them.
func TestMiddleware_StaticCatalogs_AlwaysHourTTL(t *testing.T) {
	tests := []struct {
		name   string
		method string
		result func() mcp.Result
	}{
		{"prompts_list", "prompts/list", func() mcp.Result { return &mcp.ListPromptsResult{} }},
		{"resources_list", "resources/list", func() mcp.Result { return &mcp.ListResourcesResult{} }},
		{"resource_templates_list", "resources/templates/list", func() mcp.Result {
			return &mcp.ListResourceTemplatesResult{}
		}},
	}
	for _, tt := range tests {
		for _, pinned := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tier_pinned=%v", tt.name, pinned), func(t *testing.T) {
				res, err := cachehints.Middleware(cachehints.Options{TierPinned: pinned})(
					stubHandler(tt.result()),
				)(context.Background(), tt.method, nil)
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
				if got := cacheable.GetTTLMs(); got != 3600000 {
					t.Errorf("TTLMs = %d, want 3600000", got)
				}
			})
		}
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
		{"live_project", readRequest("gitlab://project/42"), 0},
		{"live_user", readRequest("gitlab://user/current"), 0},
		{"nil_request", nil, 0},
		{"other_request_type", &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}}, 0},
		{"nil_params", &mcp.ReadResourceRequest{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := cachehints.Middleware(cachehints.Options{})(stubHandler(&mcp.ReadResourceResult{}))(
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
	handler := cachehints.Middleware(cachehints.Options{})(stubHandler(want))
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
	res, err := cachehints.Middleware(cachehints.Options{})(stubHandler(want))(context.Background(), "tools/list", nil)
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
	res, err := cachehints.Middleware(cachehints.Options{})(stubHandler(nil))(context.Background(), "tools/list", nil)
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
	_, err := cachehints.Middleware(cachehints.Options{})(failing)(context.Background(), "tools/list", nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestMiddleware_ToolManifestRead_TTLFollowsTierSource verifies that reading
// gitlab://tools and gitlab://tools/{id} gets the tool catalog's window rather
// than the static one. Both are served from memory, but they describe the
// visible catalog and vary with the same tier input as tools/list; caching the
// manifest for an hour while the tool list refreshed every five minutes would
// let a client hold two disagreeing views of the same catalog.
func TestMiddleware_ToolManifestRead_TTLFollowsTierSource(t *testing.T) {
	uris := []struct {
		name string
		uri  string
	}{
		{"manifest_index", "gitlab://tools"},
		{"manifest_detail", "gitlab://tools/gitlab_find_action"},
	}
	tierSources := []struct {
		name    string
		opts    cachehints.Options
		wantTTL int
	}{
		{"detected tier keeps the short window", cachehints.Options{TierPinned: false}, 300000},
		{"configured tier earns the long window", cachehints.Options{TierPinned: true}, 3600000},
	}

	for _, u := range uris {
		for _, ts := range tierSources {
			t.Run(u.name+"/"+ts.name, func(t *testing.T) {
				res, err := cachehints.Middleware(ts.opts)(stubHandler(&mcp.ReadResourceResult{}))(
					context.Background(), "resources/read", readRequest(u.uri),
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
				if got := cacheable.GetTTLMs(); got != ts.wantTTL {
					t.Errorf("TTLMs = %d, want %d", got, ts.wantTTL)
				}
			})
		}
	}
}
