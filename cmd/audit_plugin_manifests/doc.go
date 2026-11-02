// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_plugin_manifests checks the plugin manifests this repository
// ships for hosts that install MCP servers as plugins: the Agent Plugins 1.0
// manifest (the root plugin.json and the mcp.json it implies) and the legacy
// Open Plugins manifest kept for older hosts (.plugin/plugin.json). It is
// item 3 of issue 1114, scripts/check-openplugin.sh moved into Go, and its
// tests hold it to a record of what that script accepted and refused
// (testdata/check_openplugin.json).
//
// Open Plugins was renamed to Agent Plugins 1.0 (open-plugins.com redirects
// to agent-plugins.org): the manifest moved to a root plugin.json under a
// closed schema that forbids the legacy logo and mcpServers fields, and
// mcp.json requires $schema and a type for each server, with no
// ${VAR:-default} expansion. Both layouts ship side by side, and this check
// keeps each valid for its hosts.
//
// # What the schemas say
//
// Each manifest is validated against the schema it declares, with
// github.com/google/jsonschema-go: plugin.json and mcp.json against the
// Agent Plugins 1.0.0 schemas, which this command carries (schemas.go says
// where they come from), so the check needs no network; .plugin/plugin.json
// against .plugin/plugin.schema.json, the tracked schema it names. That
// replaces the key sets the script checked by hand with everything the
// schemas say, types and required members included. One thing is held in
// code instead: the Agent Plugins name pattern needs a lookahead, which Go's
// regexp does not have.
//
// # What no schema says
//
// plugin.json and mcp.json declare exactly the schemas of Agent Plugins
// 1.0.0, plugin.json names the plugin gitlab-mcp-server, and the legacy
// manifest points its mcpServers at ./mcp.json. mcp.json holds exactly one
// server, under mcpServers.gitlab, that runs over stdio as docker with -i
// (the image's CMD infers the transport from file descriptor 0, and without
// a pipe there the container infers HTTP and the client waits at initialize
// forever), at ghcr.io/jmrplens/gitlab-mcp-server tagged latest or a concrete
// semantic version, and expanding no ${...} other than ${PLUGIN_ROOT} and
// ${PLUGIN_DATA}, the two Agent Plugins expands.
//
// Every problem is reported, where the script stopped at the first.
//
// # Usage
//
//	go run ./cmd/audit_plugin_manifests/
//
// It runs from the repository root and exits 0 when every manifest passes
// and 1 when any does not. `make check-openplugin` runs it.
package main
