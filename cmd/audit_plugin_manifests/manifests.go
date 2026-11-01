// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The manifests this check reads, relative to the repository root.
const (
	agentPluginPath  = "plugin.json"
	legacyPluginPath = ".plugin/plugin.json"
	mcpConfigPath    = "mcp.json"
)

// The values no schema states and this repository holds its manifests to.
const (
	pluginName      = "gitlab-mcp-server"
	legacyMCPConfig = "./mcp.json"
	serverKey       = "gitlab"
	dockerCommand   = "docker"
	imagePrefix     = "ghcr.io/jmrplens/gitlab-mcp-server:"
)

// imageTag is the image reference the Docker server may run: latest or a
// concrete semantic version, a prerelease suffix allowed.
var imageTag = regexp.MustCompile(`^ghcr\.io/jmrplens/gitlab-mcp-server:(latest|[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?)$`)

// manifest is one manifest as read: its document, or why it could not be
// read.
type manifest struct {
	path string
	doc  map[string]any
}

// checkManifests holds the three manifests to the schema each declares and
// to what no schema says, and returns every problem found, in the order the
// manifests and their rules are read.
func checkManifests(root string) []string {
	var problems []string
	read := func(rel string) *manifest {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //#nosec G304,G703 -- a committed manifest of the tree the check runs in
		var doc map[string]any
		if err == nil {
			err = json.Unmarshal(data, &doc)
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s could not be read as a JSON object: %v", rel, err))
			return nil
		}
		return &manifest{path: rel, doc: doc}
	}
	agent, legacy, mcp := read(agentPluginPath), read(legacyPluginPath), read(mcpConfigPath)

	if agent != nil {
		problems = append(problems, schemaProblems(root, agent, pluginSchemaURL)...)
		problems = append(problems, agentRules(agent)...)
	}
	if legacy != nil {
		problems = append(problems, schemaProblems(root, legacy, "")...)
		if got := stringField(legacy.doc, "mcpServers"); got != legacyMCPConfig {
			problems = append(problems, fmt.Sprintf("%s must reference %s in mcpServers (got: %s)", legacyPluginPath, legacyMCPConfig, got))
		}
	}
	if mcp != nil {
		problems = append(problems, schemaProblems(root, mcp, mcpSchemaURL)...)
		problems = append(problems, serverRules(mcp)...)
	}
	return problems
}

// schemaProblems validates a manifest against the schema it declares. When
// required is set, the manifest has to declare exactly that schema, the one
// its kind of manifest is published under.
func schemaProblems(root string, m *manifest, required string) []string {
	declared := stringField(m.doc, "$schema")
	if required != "" && declared != required {
		return []string{fmt.Sprintf("%s must declare $schema %s (got: %s)", m.path, required, declared)}
	}
	schema, namePatternTaken, err := schemaFor(root, m.path, declared)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", m.path, err)}
	}
	var problems []string
	if invalid := schema.Validate(m.doc); invalid != nil {
		problems = append(problems, fmt.Sprintf("%s does not match the schema it declares, %s: %v", m.path, declared, invalid))
	}
	if name, ok := m.doc["name"].(string); ok && namePatternTaken && !validPluginName(name) {
		problems = append(problems, fmt.Sprintf("%s does not match the schema it declares, %s: name %q does not match the pattern %s", m.path, declared, name, namePattern))
	}
	return problems
}

// agentRules are what the root plugin.json is held to beyond its schema:
// the name this plugin is published under.
func agentRules(m *manifest) []string {
	if stringField(m.doc, "name") != pluginName {
		return []string{agentPluginPath + " name must be " + pluginName}
	}
	return nil
}

// serverRules are what mcp.json is held to beyond its schema: exactly one
// server, the Docker image under the name the other manifests give it, run
// over stdio with -i, which puts a pipe on the container's file descriptor 0
// for the image's CMD to infer the transport from (without it the container
// is handed /dev/null, infers HTTP, and the client waits at initialize
// forever), at latest or a concrete version, and expanding nothing Agent
// Plugins does not expand, which is only ${PLUGIN_ROOT} and ${PLUGIN_DATA}:
// anything else would reach the container as a literal string.
func serverRules(m *manifest) []string {
	servers, _ := m.doc["mcpServers"].(map[string]any)
	var problems []string
	if len(servers) != 1 {
		problems = append(problems, fmt.Sprintf("%s must define exactly one MCP server entry (got: %d)", mcpConfigPath, len(servers)))
	}
	server, _ := servers[serverKey].(map[string]any)
	if stringField(server, "type") != "stdio" {
		return append(problems, "mcpServers."+serverKey+" must declare type: stdio")
	}
	if command := stringField(server, "command"); command != dockerCommand {
		problems = append(problems, fmt.Sprintf("mcpServers.%s.command must be %s (got: %s)", serverKey, dockerCommand, command))
	}
	args := stringItems(server["args"])
	var images []string
	for _, arg := range args {
		if strings.HasPrefix(arg, imagePrefix) {
			images = append(images, arg)
		}
	}
	if image := strings.Join(images, "\n"); !imageTag.MatchString(image) {
		problems = append(problems, "Docker image must use latest or a concrete semantic version tag (got: "+image+")")
	}
	if !slices.Contains(args, "-i") {
		problems = append(problems, "Docker stdio config must pass -i so the container gets a stdin to speak on")
	}
	env, _ := server["env"].(map[string]any)
	expanded := slices.Clone(args)
	for _, value := range env {
		if text, ok := value.(string); ok {
			expanded = append(expanded, text)
		}
	}
	if slices.ContainsFunc(expanded, unexpandedInterpolation) {
		problems = append(problems, mcpConfigPath+" uses ${...} interpolation Agent Plugins does not expand")
	}
	return problems
}

// unexpandedInterpolation reports whether s carries a ${...} other than
// ${PLUGIN_ROOT} and ${PLUGIN_DATA}.
func unexpandedInterpolation(s string) bool {
	for rest := s; ; {
		at := strings.Index(rest, "${")
		if at < 0 {
			return false
		}
		rest = rest[at+2:]
		if !strings.HasPrefix(rest, "PLUGIN_ROOT}") && !strings.HasPrefix(rest, "PLUGIN_DATA}") {
			return true
		}
	}
}

// stringField is a member of an object that is a string, or "" when it is
// missing or of another kind, as jq's `.key // empty` printed it.
func stringField(doc map[string]any, key string) string {
	text, _ := doc[key].(string)
	return text
}

// stringItems is the string items of a list.
func stringItems(value any) []string {
	items, _ := value.([]any)
	var out []string
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
