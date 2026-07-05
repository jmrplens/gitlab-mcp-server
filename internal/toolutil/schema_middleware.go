package toolutil

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// onFirstToolsList registers a receiving middleware that runs visit exactly
// once, over the tools of the first successful tools/list response. The
// sync.Once guard prevents concurrent tools/list calls from racing on the
// shared *Tool.InputSchema maps: the first call performs the mutation and
// later calls are pure reads on the (now stable) schemas.
func onFirstToolsList(server *mcp.Server, visit func([]*mcp.Tool)) {
	if server == nil {
		return
	}
	var once sync.Once
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || method != "tools/list" {
				return result, err
			}
			if listResult, ok := result.(*mcp.ListToolsResult); ok && listResult != nil {
				once.Do(func() { visit(listResult.Tools) })
			}
			return result, nil
		}
	})
}
