// Command eval_mcp_surfaces evaluates model behavior across MCP tool surfaces.
package main

import (
	"fmt"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/evaluator"
)

func main() {
	if err := evaluator.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "eval_mcp_surfaces: %v\n", err)
		os.Exit(1)
	}
}
