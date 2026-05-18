#!/usr/bin/env bash
# Validate the Open Plugins manifest and bundled MCP server configuration.

set -euo pipefail

PLUGIN_JSON=".plugin/plugin.json"
MCP_JSON="mcp.json"

jq empty "$PLUGIN_JSON" "$MCP_JSON"

plugin_mcp_path=$(jq -r '.mcpServers // empty' "$PLUGIN_JSON")
if [[ "$plugin_mcp_path" != "./mcp.json" ]]; then
  echo "ERROR: $PLUGIN_JSON must reference ./mcp.json in mcpServers (got: $plugin_mcp_path)" >&2
  exit 1
fi

server_count=$(jq '.mcpServers | length' "$MCP_JSON")
if [[ "$server_count" -ne 1 ]]; then
  echo "ERROR: $MCP_JSON must define exactly one MCP server entry (got: $server_count)" >&2
  exit 1
fi

command=$(jq -r '.mcpServers.gitlab.command // empty' "$MCP_JSON")
if [[ "$command" != "docker" ]]; then
  echo "ERROR: mcpServers.gitlab.command must be docker (got: $command)" >&2
  exit 1
fi

image=$(jq -r '.mcpServers.gitlab.args[] | select(startswith("ghcr.io/jmrplens/gitlab-mcp-server:"))' "$MCP_JSON")
if [[ ! "$image" =~ ^ghcr\.io/jmrplens/gitlab-mcp-server:(latest|[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?)$ ]]; then
  echo "ERROR: Docker image must use latest or a concrete semantic version tag (got: $image)" >&2
  exit 1
fi

if ! jq -e '.mcpServers.gitlab.args | index("--http=false") != null' "$MCP_JSON" > /dev/null; then
  echo "ERROR: Docker stdio Open Plugins config must pass --http=false" >&2
  exit 1
fi

missing_env=$(jq -r '
  .mcpServers.gitlab as $server
  | [range(0; ($server.args | length) - 1)
      | select($server.args[.] == "-e")
      | $server.args[. + 1]
      | . as $name
      | select(($server.env | has($name)) | not)]
  | .[]
' "$MCP_JSON")
if [[ -n "$missing_env" ]]; then
  echo "ERROR: Docker -e variables missing from env map:" >&2
  echo "$missing_env" >&2
  exit 1
fi

unused_env=$(jq -r '
  .mcpServers.gitlab as $server
  | ([range(0; ($server.args | length) - 1)
      | select($server.args[.] == "-e")
      | $server.args[. + 1]] | unique) as $dockerEnv
  | ($server.env | keys | map(select(. as $name | ($dockerEnv | index($name)) | not)))[]
' "$MCP_JSON")
if [[ -n "$unused_env" ]]; then
  echo "ERROR: env map entries are not forwarded with docker -e:" >&2
  echo "$unused_env" >&2
  exit 1
fi

echo "Open Plugins manifest and MCP config are valid"
