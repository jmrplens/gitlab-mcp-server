// Command gen_docker_tools generates a tools.json file in the format expected
// by the Docker MCP Registry (https://github.com/docker/mcp-registry).
//
// The format is:
//
//	[
//	  {
//	    "name": "tool_name",
//	    "description": "tool description",
//	    "arguments": [
//	      {"name": "arg", "type": "string", "desc": "..."}
//	    ]
//	  }
//	]
//
// Usage:
//
//	go run ./cmd/gen_docker_tools/ > tools.json
//
// By default it emits the base meta-tools (GITLAB_MCP_META_TOOLS=true, no enterprise).
// Pass --enterprise to include enterprise meta-tools, or --individual to emit
// the full 1000-tool surface.
package main
