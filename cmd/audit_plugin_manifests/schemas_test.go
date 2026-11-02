// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// schemas_test.go holds the vendored schemas to the files Agent Plugins
// publishes, and covers how a declared schema is found, read and resolved,
// and the name pattern the check holds in code.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVendoredSchemas_AreTheFilesAgentPluginsPublishes verifies each
// vendored schema byte for byte, by the SHA-256 of the file agent-plugins.org
// serves at its URL, which is also the $id the file declares, and that the
// license they are published under travels beside them.
func TestVendoredSchemas_AreTheFilesAgentPluginsPublishes(t *testing.T) {
	t.Parallel()

	for url, want := range map[string]string{
		pluginSchemaURL: "0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883",
		mcpSchemaURL:    "6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb",
	} {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			data, err := vendoredSchemas.ReadFile(vendoredFiles[url])
			if err != nil {
				t.Fatal(err)
			}
			if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != want {
				t.Errorf("%s has SHA-256 %x, want %s", vendoredFiles[url], sum, want)
			}
			var schema struct {
				ID string `json:"$id"`
			}
			if decodeErr := json.Unmarshal(data, &schema); decodeErr != nil || schema.ID != url {
				t.Errorf("%s declares $id %q (%v), want %q", vendoredFiles[url], schema.ID, decodeErr, url)
			}
		})
	}
	license, err := os.ReadFile(filepath.Join("schemas", "agent-plugins", "LICENSE-Apache-2.0.txt"))
	if err != nil || !strings.Contains(string(license), "Apache License\n                           Version 2.0, January 2004") {
		t.Errorf("the schemas' license is not beside them: %v", err)
	}
}

// TestSchemaFor_DeclaredSchemas_AreFoundOrRefused covers each kind of
// $schema a manifest can declare: one the check carries, a file beside the
// manifest, and the refusals of nothing, another URL, an absolute path, a
// file that is not there, one that is not JSON and one that cannot be
// resolved.
func TestSchemaFor_DeclaredSchemas_AreFoundOrRefused(t *testing.T) {
	t.Parallel()

	root := layOut(t, map[string]string{
		"a/ok.schema.json":       `{"type":"object"}`,
		"a/bad.schema.json":      `{`,
		"a/broken.schema.json":   `{"properties":{"x":{"pattern":"(?!x)"}}}`,
		"a/named.schema.json":    `{"properties":{"name":{"pattern":"` + strings.ReplaceAll(namePattern, `\`, `\\`) + `"}}}`,
		"a/manifest.json":        `{}`,
		"a/elsewhere/other.json": `{}`,
	})
	for _, tc := range []struct {
		name, declared, refused string
		taken                   bool
	}{
		{name: "the plugin schema", declared: pluginSchemaURL, taken: true},
		{name: "the mcp schema", declared: mcpSchemaURL},
		{name: "a file beside the manifest", declared: "./ok.schema.json"},
		{name: "a file whose name pattern is taken out", declared: "named.schema.json", taken: true},
		{name: "nothing", declared: "", refused: `it declares $schema "", which is neither a schema this check carries nor a file beside it`},
		{name: "another URL", declared: "https://example.com/s.json", refused: "which is neither a schema this check carries"},
		{name: "an absolute path", declared: "/etc/s.json", refused: "which is neither a schema this check carries"},
		{name: "a file that is not there", declared: "./absent.json", refused: "the schema it declares, a/absent.json, could not be read: "},
		{name: "a file that is not JSON", declared: "bad.schema.json", refused: "the schema a/bad.schema.json is not JSON: "},
		{name: "a file that cannot be resolved", declared: "broken.schema.json", refused: "the schema a/broken.schema.json could not be resolved: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			schema, taken, err := schemaFor(root, "a/manifest.json", tc.declared)
			if tc.refused != "" {
				if err == nil || !strings.Contains(err.Error(), tc.refused) {
					t.Errorf("schemaFor error = %v, want one naming %q", err, tc.refused)
				}
				return
			}
			if err != nil || schema == nil || taken != tc.taken {
				t.Errorf("schemaFor = %v, %v, %v, want a schema with the pattern taken out %v", schema, taken, err, tc.taken)
			}
		})
	}
}

// TestValidPluginName_Names_AreHeldToThePattern covers each part of the name
// pattern Go's regexp cannot compile.
func TestValidPluginName_Names_AreHeldToThePattern(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		"gitlab-mcp-server": true,
		"a":                 true,
		"a.b-c9":            true,
		"0":                 true,
		"":                  false,
		"-a":                false,
		"a-":                false,
		".a":                false,
		"a.":                false,
		"a--b":              false,
		"a..b":              false,
		"a_b":               false,
		"Ab":                false,
		"aB":                false,
		"aéb":               false,
		"z9":                true,
		"a/b":               false,
		"a:b":               false,
		"a`b":               false,
		"a{b":               false,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := validPluginName(name); got != want {
				t.Errorf("validPluginName(%q) = %v, want %v", name, got, want)
			}
		})
	}
}
