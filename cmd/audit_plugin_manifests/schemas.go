// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// The schemas the Agent Plugins manifests declare, by the URL each declares.
const (
	pluginSchemaURL = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	mcpSchemaURL    = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
)

// vendoredSchemas are the Agent Plugins 1.0.0 schemas, copied from
// schemas/1.0.0 of https://github.com/agentplugins/agent-plugins-spec at
// d92e6f443b8edcea42c039727a82afdc565779e2, byte for byte the files
// agent-plugins.org serves at the URLs they declare as their $id, so the
// check needs no network. They are Apache-2.0, as that repository licenses
// its schemas (LICENSE.md there), and its license travels beside them in
// schemas/agent-plugins/LICENSE-Apache-2.0.txt. Their SHA-256 is held by
// TestVendoredSchemas_AreTheFilesAgentPluginsPublishes.
//
//go:embed schemas/agent-plugins/1.0.0/plugin.schema.json schemas/agent-plugins/1.0.0/mcp.schema.json
var vendoredSchemas embed.FS

// vendoredFiles are the embedded file of each vendored schema, by its URL.
var vendoredFiles = map[string]string{
	pluginSchemaURL: "schemas/agent-plugins/1.0.0/plugin.schema.json",
	mcpSchemaURL:    "schemas/agent-plugins/1.0.0/mcp.schema.json",
}

// namePattern is the pattern the Agent Plugins plugin schema holds a name
// to. Go's regexp has no lookahead, so jsonschema-go cannot compile it: the
// pattern is taken out of the schema before it is resolved and the name is
// held to it by [validPluginName] instead, which is what the pattern says.
const namePattern = `^(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`

// schemaFor returns the resolved schema a manifest at manifestPath declares:
// one this check carries, by its URL, or a file of the tree, by a path
// relative to the manifest; and whether the name's pattern was taken out of
// it (see [namePattern]).
func schemaFor(root, manifestPath, declared string) (*jsonschema.Resolved, bool, error) {
	file, vendored := vendoredFiles[declared]
	if !vendored {
		if declared == "" || strings.Contains(declared, "://") || path.IsAbs(declared) {
			return nil, false, fmt.Errorf("it declares $schema %q, which is neither a schema this check carries nor a file beside it", declared)
		}
		file = path.Join(path.Dir(manifestPath), declared)
	}
	data, err := vendoredSchemas.ReadFile(file)
	if !vendored {
		data, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) //#nosec G304,G703 -- a schema file of the tree a committed manifest names
	}
	if err != nil {
		return nil, false, fmt.Errorf("the schema it declares, %s, could not be read: %w", file, err)
	}
	return resolveSchema(data, file)
}

// resolveSchema reads a schema and resolves it, with the plugin name's
// pattern taken out (see [namePattern]), and reports whether it was.
func resolveSchema(data []byte, file string) (*jsonschema.Resolved, bool, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, false, fmt.Errorf("the schema %s is not JSON: %w", file, err)
	}
	name := schema.Properties["name"]
	taken := name != nil && name.Pattern == namePattern
	if taken {
		name.Pattern = ""
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, false, fmt.Errorf("the schema %s could not be resolved: %w", file, err)
	}
	return resolved, taken, nil
}

// validPluginName reports whether a name matches [namePattern]: lowercase
// letters, digits, dots and hyphens, starting and ending with a letter or a
// digit, with no two hyphens or two dots in a row.
func validPluginName(name string) bool {
	if name == "" || strings.Contains(name, "--") || strings.Contains(name, "..") {
		return false
	}
	for i, r := range name {
		alnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		edge := i == 0 || i == len(name)-1
		if !alnum && (edge || (r != '.' && r != '-')) {
			return false
		}
	}
	return true
}
