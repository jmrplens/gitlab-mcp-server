#!/usr/bin/env bash
# Validate the Agent Plugins manifests (root plugin.json + mcp.json) and the
# legacy Open Plugins manifest (.plugin/plugin.json) kept for older hosts.
#
# Open Plugins was renamed to Agent Plugins 1.0 (open-plugins.com redirects to
# agent-plugins.org): the manifest moved to a ROOT plugin.json under a closed
# schema that forbids the legacy logo/mcpServers fields, and mcp.json now
# requires $schema plus a per-server type, with no ${VAR:-default} expansion.
# Both layouts ship side by side; this gate keeps each valid for its hosts.

set -euo pipefail

AGENT_PLUGIN_JSON="plugin.json"
LEGACY_PLUGIN_JSON=".plugin/plugin.json"
MCP_JSON="mcp.json"

PLUGIN_SCHEMA_URL="https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
MCP_SCHEMA_URL="https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

jq empty "$AGENT_PLUGIN_JSON" "$LEGACY_PLUGIN_JSON" "$MCP_JSON"

schema=$(jq -r '."$schema" // empty' "$AGENT_PLUGIN_JSON")
if [[ "$schema" != "$PLUGIN_SCHEMA_URL" ]]; then
  echo "ERROR: $AGENT_PLUGIN_JSON must declare \$schema $PLUGIN_SCHEMA_URL (got: $schema)" >&2
  exit 1
fi

if ! jq -e '.name == "gitlab-mcp-server"' "$AGENT_PLUGIN_JSON" > /dev/null; then
  echo "ERROR: $AGENT_PLUGIN_JSON name must be gitlab-mcp-server" >&2
  exit 1
fi

# The Agent Plugins schema is closed (additionalProperties: false); enforce the
# permitted top-level and author key sets so a legacy field (logo, mcpServers)
# cannot sneak back in.
if ! jq -e '(keys - ["$schema","name","version","description","author","homepage","repository","license","keywords","extensions"]) == []' "$AGENT_PLUGIN_JSON" > /dev/null; then
  echo "ERROR: $AGENT_PLUGIN_JSON carries fields the Agent Plugins schema forbids:" >&2
  jq -r '(keys - ["$schema","name","version","description","author","homepage","repository","license","keywords","extensions"])[]' "$AGENT_PLUGIN_JSON" >&2
  exit 1
fi

if ! jq -e '(.author | keys - ["name","email","url"]) == []' "$AGENT_PLUGIN_JSON" > /dev/null; then
  echo "ERROR: $AGENT_PLUGIN_JSON author allows only name, email, url" >&2
  exit 1
fi

plugin_mcp_path=$(jq -r '.mcpServers // empty' "$LEGACY_PLUGIN_JSON")
if [[ "$plugin_mcp_path" != "./mcp.json" ]]; then
  echo "ERROR: $LEGACY_PLUGIN_JSON must reference ./mcp.json in mcpServers (got: $plugin_mcp_path)" >&2
  exit 1
fi

mcp_schema=$(jq -r '."$schema" // empty' "$MCP_JSON")
if [[ "$mcp_schema" != "$MCP_SCHEMA_URL" ]]; then
  echo "ERROR: $MCP_JSON must declare \$schema $MCP_SCHEMA_URL (got: $mcp_schema)" >&2
  exit 1
fi

server_count=$(jq '.mcpServers | length' "$MCP_JSON")
if [[ "$server_count" -ne 1 ]]; then
  echo "ERROR: $MCP_JSON must define exactly one MCP server entry (got: $server_count)" >&2
  exit 1
fi

if ! jq -e '.mcpServers.gitlab.type == "stdio"' "$MCP_JSON" > /dev/null; then
  echo "ERROR: mcpServers.gitlab must declare type: stdio" >&2
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
  echo "ERROR: Docker stdio config must pass --http=false" >&2
  exit 1
fi

if ! jq -e '.mcpServers.gitlab.args | index("AUTO_UPDATE=false") as $i | $i != null and .[$i - 1] == "-e"' "$MCP_JSON" > /dev/null; then
  echo "ERROR: config must force -e AUTO_UPDATE=false (the package channel owns the binary)" >&2
  exit 1
fi

# Agent Plugins interpolation expands only ${PLUGIN_ROOT}/${PLUGIN_DATA}; any
# other ${...} would reach the container as a literal string.
if jq -e '[.mcpServers.gitlab | (.args[]?, (.env // {} | .[]?)) | select(test("\\$\\{(?!PLUGIN_ROOT\\}|PLUGIN_DATA\\})"))] | length > 0' "$MCP_JSON" > /dev/null; then
  echo "ERROR: mcp.json uses \${...} interpolation Agent Plugins does not expand" >&2
  exit 1
fi

echo "Agent Plugins and legacy Open Plugins manifests are valid"
