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
	server.AddReceivingMiddleware(firstToolsListMiddleware(visit))
}

// firstToolsListMiddleware is the middleware [onFirstToolsList] registers. It is
// built apart from the registration so that what it does with somebody else's
// handler (a failed call, a result of another type, a nil one) can be asked of
// it directly, which is not something a client driving a real server can ask.
func firstToolsListMiddleware(visit func([]*mcp.Tool)) mcp.Middleware {
	var once sync.Once
	return func(next mcp.MethodHandler) mcp.MethodHandler {
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
	}
}
